package main

import (
	"bufio"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"dayzoverride/analyze"
	"dayzoverride/diffcfg"
	"dayzoverride/gen"
	"dayzoverride/mergecfg"
	"dayzoverride/tools"
	"dayzoverride/typesx"
)

//go:embed web/static
var staticFS embed.FS

// ---------- settings ----------

type Settings struct {
	ToolsPath      string `json:"toolsPath"`
	PrivateKeyPath string `json:"privateKeyPath"`
	Sign           bool   `json:"sign"`
	Binarize       bool   `json:"binarize"`
	UseFileBank    bool   `json:"useFileBank"`
	OutputPath     string `json:"outputPath"`
	// ИИ-помощник (гайды по ссылке). Ключ хранится только в этом файле на ПК пользователя.
	AiEnabled bool   `json:"aiEnabled"`
	AiPreset  string `json:"aiPreset"`
	AiBaseURL string `json:"aiBaseURL"`
	AiKey     string `json:"aiKey"`
	AiModel   string `json:"aiModel"`
}

var (
	settingsMu sync.Mutex
	settings   Settings
)

func settingsCandidates() []string {
	var out []string
	if exe, err := os.Executable(); err == nil {
		out = append(out, filepath.Join(filepath.Dir(exe), "settings.json"))
	}
	if dir, err := os.UserConfigDir(); err == nil {
		out = append(out, filepath.Join(dir, "DayZOverrideBuilder", "settings.json"))
	}
	return out
}

func loadSettings() {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	for _, p := range settingsCandidates() {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var s Settings
		if json.Unmarshal(data, &s) == nil {
			settings = s
			return
		}
	}
}

func saveSettings() {
	settingsMu.Lock()
	data, _ := json.MarshalIndent(settings, "", "  ")
	settingsMu.Unlock()
	for _, p := range settingsCandidates() {
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			continue
		}
		if err := os.WriteFile(p, data, 0644); err == nil {
			return
		}
	}
}

func getSettings() Settings {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	return settings
}

func updateSettings(fn func(*Settings)) {
	settingsMu.Lock()
	fn(&settings)
	settingsMu.Unlock()
	saveSettings()
}

// ---------- пульс вкладок браузера (живо ли окно интерфейса) ----------

var clientPings = struct {
	mu sync.Mutex
	m  map[string]time.Time
}{m: map[string]time.Time{}}

func handlePing(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		id = "anon"
	}
	now := time.Now()
	clientPings.mu.Lock()
	clientPings.m[id] = now
	for k, v := range clientPings.m {
		if now.Sub(v) > 30*time.Second {
			delete(clientPings.m, k)
		}
	}
	clientPings.mu.Unlock()
	writeJSON(w, map[string]string{"status": "ok"})
}

func activeClients() int {
	now := time.Now()
	n := 0
	clientPings.mu.Lock()
	for _, v := range clientPings.m {
		if now.Sub(v) <= 6*time.Second {
			n++
		}
	}
	clientPings.mu.Unlock()
	return n
}

// watchClients следит за вкладками: когда последняя закрылась — предлагает
// в консоли восстановить интерфейс или выйти.
func watchClients(url string) {
	hadClient := false
	notified := false
	for {
		time.Sleep(2 * time.Second)
		n := activeClients()
		if n > 0 {
			if notified {
				fmt.Println()
				fmt.Println("[OK] Интерфейс снова открыт — продолжаем работу.")
				notified = false
			}
			hadClient = true
			continue
		}
		if hadClient && !notified {
			notified = true
			fmt.Println()
			fmt.Println("--------------------------------------------------------")
			fmt.Println("  [!] Вкладка браузера закрыта. Программа ещё работает.")
			fmt.Println("      ENTER      - снова открыть интерфейс в браузере")
			fmt.Println("      Q + ENTER  - завершить программу")
			fmt.Println("      (или откройте вручную: " + url + ")")
			fmt.Println("--------------------------------------------------------")
		}
	}
}

// readConsole обрабатывает команды в окне программы.
func readConsole(url string) {
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		switch strings.TrimSpace(strings.ToLower(sc.Text())) {
		case "":
			fmt.Println("Открываю интерфейс: " + url)
			openBrowser(url)
		case "q", "й", "quit", "exit", "выход":
			fmt.Println("Завершение работы. Пока!")
			os.Exit(0)
		default:
			fmt.Println("ENTER — открыть интерфейс, Q + ENTER — выйти.")
		}
	}
	// stdin закрыт (запуск без консоли) — просто выходим из горутины
}

// ---------- глобальный журнал (вкладка «Журнал») ----------

var glog = struct {
	mu    sync.Mutex
	lines []string
}{}

func glogAdd(line string) {
	glog.mu.Lock()
	glog.lines = append(glog.lines, line)
	if len(glog.lines) > 4000 {
		glog.lines = append([]string(nil), glog.lines[len(glog.lines)-3000:]...)
	}
	glog.mu.Unlock()
}

func glogf(tag, format string, args ...interface{}) {
	glogAdd(time.Now().Format("15:04:05") + "  [" + tag + "] " + fmt.Sprintf(format, args...))
}

// GET /api/log?from=N — инкрементальное чтение глобального журнала.
// POST /api/log/clear — очистка.
func handleGlobalLog(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/clear") {
		glog.mu.Lock()
		glog.lines = nil
		glog.mu.Unlock()
		glogf("система", "Журнал очищен")
		writeJSON(w, map[string]string{"status": "ok"})
		return
	}
	from := 0
	fmt.Sscanf(r.URL.Query().Get("from"), "%d", &from)
	glog.mu.Lock()
	total := len(glog.lines)
	if from < 0 || from > total {
		from = 0
	}
	resp := map[string]interface{}{
		"lines": append([]string(nil), glog.lines[from:]...),
		"total": total,
	}
	glog.mu.Unlock()
	writeJSON(w, resp)
}

// ---------- job store (живой журнал длительных операций) ----------

type jobRec struct {
	mu      sync.Mutex
	created time.Time
	tag     string
	lines   []string
	done    bool
	errMsg  string
	result  interface{}
}

var jobsStore = struct {
	mu sync.Mutex
	m  map[string]*jobRec
}{m: map[string]*jobRec{}}

func newJob(tag string) (string, *jobRec) {
	id := randToken()
	j := &jobRec{created: time.Now(), tag: tag}
	jobsStore.mu.Lock()
	for k, v := range jobsStore.m {
		if time.Since(v.created) > 30*time.Minute {
			delete(jobsStore.m, k)
		}
	}
	jobsStore.m[id] = j
	jobsStore.mu.Unlock()
	glogf(tag, "— запуск —")
	return id, j
}

func getJob(id string) *jobRec {
	jobsStore.mu.Lock()
	defer jobsStore.mu.Unlock()
	return jobsStore.m[id]
}

func (j *jobRec) logf(format string, args ...interface{}) {
	line := time.Now().Format("15:04:05") + "  " + fmt.Sprintf(format, args...)
	j.mu.Lock()
	j.lines = append(j.lines, line)
	j.mu.Unlock()
	glogAdd(time.Now().Format("15:04:05") + "  [" + j.tag + "] " + fmt.Sprintf(format, args...))
}

func (j *jobRec) finish(result interface{}, err error) {
	j.mu.Lock()
	j.result = result
	if err != nil {
		j.errMsg = err.Error()
	}
	j.done = true
	j.mu.Unlock()
	if err != nil {
		glogf(j.tag, "ЗАВЕРШЕНО С ОШИБКОЙ: %v", err)
	} else {
		glogf(j.tag, "— успешно завершено —")
	}
}

// guard оборачивает горутину задачи: паника становится ошибкой в журнале,
// а не молчаливым зависанием интерфейса.
func (j *jobRec) guard() {
	if r := recover(); r != nil {
		j.logf("КРИТИЧЕСКАЯ ОШИБКА (внутренний сбой): %v", r)
		j.finish(nil, fmt.Errorf("внутренний сбой программы: %v — скопируйте журнал и отправьте разработчику", r))
	}
}

// GET /api/job/{id}?from=N — инкрементальный опрос журнала и результата.
func handleJob(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/job/")
	from := 0
	fmt.Sscanf(r.URL.Query().Get("from"), "%d", &from)
	j := getJob(id)
	if j == nil {
		jsonError(w, 404, "задача не найдена или устарела")
		return
	}
	j.mu.Lock()
	total := len(j.lines)
	if from < 0 || from > total {
		from = total
	}
	resp := map[string]interface{}{
		"lines": append([]string(nil), j.lines[from:]...),
		"total": total,
		"done":  j.done,
	}
	if j.done {
		if j.errMsg != "" {
			resp["error"] = j.errMsg
		} else {
			resp["result"] = j.result
		}
	}
	j.mu.Unlock()
	writeJSON(w, resp)
}

// ---------- zip store ----------

type zipStore struct {
	mu   sync.Mutex
	data map[string]zipItem
}

type zipItem struct {
	name    string
	bytes   []byte
	created time.Time
}

var zips = &zipStore{data: map[string]zipItem{}}

func (z *zipStore) put(name string, b []byte) string {
	tok := randToken()
	z.mu.Lock()
	defer z.mu.Unlock()
	for k, v := range z.data {
		if time.Since(v.created) > time.Hour {
			delete(z.data, k)
		}
	}
	z.data[tok] = zipItem{name: name, bytes: b, created: time.Now()}
	return tok
}

func (z *zipStore) get(tok string) (zipItem, bool) {
	z.mu.Lock()
	defer z.mu.Unlock()
	it, ok := z.data[tok]
	return it, ok
}

func randToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func jsonError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}

// ---------- analyze ----------

func collectPBOPaths(inputs []string) ([]string, error) {
	var out []string
	for _, in := range inputs {
		in = strings.TrimSpace(strings.Trim(in, `"`))
		if in == "" {
			continue
		}
		st, err := os.Stat(in)
		if err != nil {
			return nil, fmt.Errorf("путь не найден: %s", in)
		}
		if st.IsDir() {
			count := 0
			err := filepath.WalkDir(in, func(p string, d fs.DirEntry, err error) error {
				if err != nil {
					return nil
				}
				if d.IsDir() {
					return nil
				}
				if strings.EqualFold(filepath.Ext(p), ".pbo") {
					out = append(out, p)
					count++
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
			if count == 0 {
				return nil, fmt.Errorf("в папке %s не найдено .pbo файлов", in)
			}
		} else {
			out = append(out, in)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("не указано ни одного .pbo")
	}
	if len(out) > 200 {
		return nil, fmt.Errorf("слишком много PBO (%d), максимум 200", len(out))
	}
	return out, nil
}

func handleAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, 405, "метод не поддерживается")
		return
	}
	ct := r.Header.Get("Content-Type")
	var paths []string
	var tmpDir string
	cleanup := func() {
		if tmpDir != "" {
			os.RemoveAll(tmpDir)
		}
	}
	if strings.HasPrefix(ct, "multipart/") {
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			jsonError(w, 400, "ошибка загрузки файлов: "+err.Error())
			return
		}
		files := r.MultipartForm.File["pbos"]
		if len(files) == 0 {
			jsonError(w, 400, "файлы не переданы")
			return
		}
		var err error
		tmpDir, err = os.MkdirTemp("", "dob-upload-*")
		if err != nil {
			jsonError(w, 500, err.Error())
			return
		}
		for _, fh := range files {
			src, err := fh.Open()
			if err != nil {
				cleanup()
				jsonError(w, 400, err.Error())
				return
			}
			name := filepath.Base(fh.Filename)
			if name == "" || name == "." {
				name = "upload.pbo"
			}
			dst := filepath.Join(tmpDir, name)
			out, err := os.Create(dst)
			if err != nil {
				src.Close()
				cleanup()
				jsonError(w, 500, err.Error())
				return
			}
			if _, err := io.Copy(out, src); err != nil {
				out.Close()
				src.Close()
				cleanup()
				jsonError(w, 500, err.Error())
				return
			}
			out.Close()
			src.Close()
			paths = append(paths, dst)
		}
	} else {
		var body struct {
			Paths []string `json:"paths"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, 400, "некорректный запрос: "+err.Error())
			return
		}
		var err error
		paths, err = collectPBOPaths(body.Paths)
		if err != nil {
			jsonError(w, 400, err.Error())
			return
		}
	}
	jobID, job := newJob("анализ")
	if tmpDir != "" {
		job.logf("Файлы получены (%d шт.)", len(paths))
	}
	go func() {
		defer cleanup()
		defer job.guard()
		res, err := analyze.AnalyzePBOs(paths, job.logf)
		if err != nil {
			job.finish(nil, err)
			return
		}
		job.finish(res, nil)
	}()
	writeJSON(w, map[string]string{"jobId": jobID})
}

// ---------- diff (сверка версий мода) ----------

func handleDiff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, 405, "метод не поддерживается")
		return
	}
	var oldPaths, newPaths []string
	var tmpDir string
	cleanup := func() {
		if tmpDir != "" {
			os.RemoveAll(tmpDir)
		}
	}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			jsonError(w, 400, "ошибка загрузки файлов: "+err.Error())
			return
		}
		var err error
		tmpDir, err = os.MkdirTemp("", "dob-diff-*")
		if err != nil {
			jsonError(w, 500, err.Error())
			return
		}
		os.MkdirAll(filepath.Join(tmpDir, "old"), 0755)
		os.MkdirAll(filepath.Join(tmpDir, "new"), 0755)
		oldUp, err := saveUploadsTo(r, "oldPbos", filepath.Join(tmpDir, "old"))
		if err == nil {
			var e2 error
			newUp, e2 := saveUploadsTo(r, "newPbos", filepath.Join(tmpDir, "new"))
			if e2 != nil {
				err = e2
			} else {
				oldPaths, newPaths = oldUp, newUp
			}
		}
		if err != nil {
			cleanup()
			jsonError(w, 400, err.Error())
			return
		}
		// пути как альтернатива файлам
		if v := strings.TrimSpace(r.FormValue("oldPath")); v != "" {
			p, err := collectPBOPaths([]string{v})
			if err != nil {
				cleanup()
				jsonError(w, 400, "старая версия: "+err.Error())
				return
			}
			oldPaths = append(oldPaths, p...)
		}
		if v := strings.TrimSpace(r.FormValue("newPath")); v != "" {
			p, err := collectPBOPaths([]string{v})
			if err != nil {
				cleanup()
				jsonError(w, 400, "новая версия: "+err.Error())
				return
			}
			newPaths = append(newPaths, p...)
		}
	}
	if len(oldPaths) == 0 || len(newPaths) == 0 {
		cleanup()
		jsonError(w, 400, "нужно указать файлы СТАРОЙ и НОВОЙ версии мода")
		return
	}
	jobID, job := newJob("сверка")
	go func() {
		defer cleanup()
		defer job.guard()
		res, err := diffcfg.Compare(oldPaths, newPaths, job.logf)
		if err != nil {
			job.finish(nil, err)
			return
		}
		job.finish(res, nil)
	}()
	writeJSON(w, map[string]string{"jobId": jobID})
}

func saveUploadsTo(r *http.Request, field, dir string) ([]string, error) {
	var paths []string
	if r.MultipartForm == nil {
		return nil, nil
	}
	for i, fh := range r.MultipartForm.File[field] {
		src, err := fh.Open()
		if err != nil {
			return nil, err
		}
		name := filepath.Base(fh.Filename)
		if name == "" || name == "." {
			name = "upload.bin"
		}
		dst := filepath.Join(dir, fmt.Sprintf("%02d_%s", i, name))
		out, err := os.Create(dst)
		if err != nil {
			src.Close()
			return nil, err
		}
		if _, err := io.Copy(out, src); err != nil {
			out.Close()
			src.Close()
			return nil, err
		}
		out.Close()
		src.Close()
		paths = append(paths, dst)
	}
	return paths, nil
}

// ---------- types.xml ----------

func readUpload(r *http.Request, field string) ([]byte, string, error) {
	fhs := r.MultipartForm.File[field]
	if len(fhs) == 0 {
		return nil, "", fmt.Errorf("файл %s не передан", field)
	}
	src, err := fhs[0].Open()
	if err != nil {
		return nil, "", err
	}
	defer src.Close()
	data, err := io.ReadAll(io.LimitReader(src, 256<<20))
	return data, filepath.Base(fhs[0].Filename), err
}

func handleTypesMigrate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, 405, "метод не поддерживается")
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		jsonError(w, 400, "ошибка загрузки файлов: "+err.Error())
		return
	}
	myData, myName, err := readUpload(r, "myTypes")
	if err != nil {
		jsonError(w, 400, "ваш types.xml: "+err.Error())
		return
	}
	newData, newName, err := readUpload(r, "newTypes")
	if err != nil {
		jsonError(w, 400, "новый types.xml: "+err.Error())
		return
	}
	keepMissing := r.FormValue("keepMissing") == "1"
	jobID, job := newJob("types-миграция")
	go func() {
		defer job.guard()
		job.logf("Ваш файл: %s (%.1f КБ), новый файл: %s (%.1f КБ)", myName, float64(len(myData))/1024, newName, float64(len(newData))/1024)
		res, err := typesx.Migrate(myData, newData, keepMissing, job.logf)
		if err != nil {
			job.finish(nil, err)
			return
		}
		job.finish(res, nil)
	}()
	writeJSON(w, map[string]string{"jobId": jobID})
}

func handleTypesOptimize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, 405, "метод не поддерживается")
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		jsonError(w, 400, "ошибка загрузки файлов: "+err.Error())
		return
	}
	data, name, err := readUpload(r, "typesFile")
	if err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	simplify := r.FormValue("simplify") == "1"
	jobID, job := newJob("types-проверка")
	go func() {
		defer job.guard()
		job.logf("Файл: %s (%.1f КБ), упрощение неспавнящихся: %v", name, float64(len(data))/1024, simplify)
		res, err := typesx.Optimize(data, simplify, job.logf)
		if err != nil {
			job.finish(nil, err)
			return
		}
		job.finish(res, nil)
	}()
	writeJSON(w, map[string]string{"jobId": jobID})
}

// ---------- merge (слияние оверрайдов) ----------

func handleMerge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, 405, "метод не поддерживается")
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		jsonError(w, 400, "ошибка загрузки файлов: "+err.Error())
		return
	}
	tmpDir, err := os.MkdirTemp("", "dob-merge-*")
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	cleanup := func() { os.RemoveAll(tmpDir) }
	paths, err := saveUploadsTo(r, "overrides", tmpDir)
	if err != nil {
		cleanup()
		jsonError(w, 400, err.Error())
		return
	}
	if len(paths) < 2 {
		cleanup()
		jsonError(w, 400, "для слияния нужно минимум два файла (.pbo или config.cpp)")
		return
	}
	req := &gen.Request{
		AddonName:   strings.TrimSpace(r.FormValue("addonName")),
		MakePbo:     r.FormValue("makePbo") == "1",
		Sign:        r.FormValue("sign") == "1",
		Binarize:    r.FormValue("binarize") == "1",
		UseFileBank: r.FormValue("useFileBank") == "1",
		ToolsPath:   strings.TrimSpace(r.FormValue("toolsPath")),
	}
	req.PrivateKeyPath = strings.TrimSpace(r.FormValue("privateKeyPath"))
	if req.ToolsPath == "" {
		req.ToolsPath = getSettings().ToolsPath
	}
	if req.PrivateKeyPath == "" {
		req.PrivateKeyPath = getSettings().PrivateKeyPath
	}
	outputPath := strings.TrimSpace(r.FormValue("outputPath"))

	jobID, job := newJob("слияние")
	go func() {
		defer cleanup()
		defer job.guard()
		job.logf("Файлов к слиянию: %d (порядок = приоритет, последний главнее)", len(paths))
		configs, _, warns := analyze.LoadConfigs(paths, job.logf)
		if len(configs) < 2 {
			job.finish(nil, fmt.Errorf("удалось разобрать только %d из %d файлов — для слияния нужно минимум два читаемых конфига", len(configs), len(paths)))
			return
		}
		mres, err := mergecfg.Merge(configs, job.logf)
		if err != nil {
			job.finish(nil, err)
			return
		}
		for _, c := range mres.Conflicts {
			job.logf("⚠ %s", c)
		}
		cfg := mergecfg.BuildConfig(mres, req.AddonName)
		job.logf("Объединённый config.cpp готов (%.1f КБ)", float64(len(cfg))/1024)
		pres, err := gen.PackageConfig(req, cfg, job.logf)
		if err != nil {
			job.finish(nil, err)
			return
		}
		resp := map[string]interface{}{
			"configCpp": pres.ConfigCPP,
			"files":     pres.FileList,
			"conflicts": mres.Conflicts,
			"warnings":  append(append([]string{}, warns...), append(mres.Warnings, pres.Warnings...)...),
			"required":  mres.RequiredAddons,
			"merged":    mres.OwnPatches,
		}
		tok := zips.put(req.AddonName+".zip", pres.ZipData)
		resp["zipToken"] = tok
		if outputPath != "" {
			job.logf("Сохраняю в папку: %s", outputPath)
			saved, err := gen.SaveToDir(req, pres, outputPath)
			if err != nil {
				job.logf("ОШИБКА сохранения: %v", err)
				resp["saveError"] = err.Error()
			} else {
				job.logf("Сохранено: %s", saved)
				resp["savedTo"] = saved
			}
		}
		job.finish(resp, nil)
	}()
	writeJSON(w, map[string]string{"jobId": jobID})
}

// ---------- generate ----------

type generateBody struct {
	gen.Request
	OutputPath string `json:"outputPath,omitempty"`
}

func handleGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, 405, "метод не поддерживается")
		return
	}
	var body generateBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, 400, "некорректный запрос: "+err.Error())
		return
	}
	if body.ToolsPath == "" {
		body.ToolsPath = getSettings().ToolsPath
	}
	jobID, job := newJob("генерация")
	go func() {
		defer job.guard()
		res, err := gen.Generate(&body.Request, job.logf)
		if err != nil {
			job.finish(nil, err)
			return
		}
		// remember choices
		updateSettings(func(s *Settings) {
			if body.ToolsPath != "" {
				s.ToolsPath = body.ToolsPath
			}
			if body.PrivateKeyPath != "" {
				s.PrivateKeyPath = body.PrivateKeyPath
			}
			s.Sign = body.Sign
			s.Binarize = body.Binarize
			s.UseFileBank = body.UseFileBank
			if body.OutputPath != "" {
				s.OutputPath = body.OutputPath
			}
		})
		resp := map[string]interface{}{
			"configCpp": res.ConfigCPP,
			"readme":    res.Readme,
			"files":     res.FileList,
			"warnings":  res.Warnings,
		}
		tok := zips.put(body.AddonName+".zip", res.ZipData)
		resp["zipToken"] = tok
		if body.OutputPath != "" {
			job.logf("Сохраняю в папку: %s", body.OutputPath)
			saved, err := gen.SaveToDir(&body.Request, res, body.OutputPath)
			if err != nil {
				job.logf("ОШИБКА сохранения: %v", err)
				resp["saveError"] = err.Error()
			} else {
				job.logf("Сохранено: %s", saved)
				resp["savedTo"] = saved
			}
		}
		job.finish(resp, nil)
	}()
	writeJSON(w, map[string]string{"jobId": jobID})
}

func handleZip(w http.ResponseWriter, r *http.Request) {
	tok := strings.TrimPrefix(r.URL.Path, "/api/zip/")
	it, ok := zips.get(tok)
	if !ok {
		http.Error(w, "архив не найден или устарел — сгенерируйте заново", 404)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+it.name+`"`)
	w.Write(it.bytes)
}

func handleRegistry(w http.ResponseWriter, r *http.Request) {
	type pd struct {
		Key      string   `json:"key"`
		Label    string   `json:"label"`
		Type     string   `json:"type"`
		Sections []string `json:"sections"`
		Hint     string   `json:"hint"`
		Path     string   `json:"path"`
	}
	var out []pd
	for _, d := range analyze.Registry {
		t := "float"
		switch d.Type {
		case analyze.PTInt:
			t = "int"
		case analyze.PTIntPair:
			t = "intpair"
		case analyze.PTStrArray:
			t = "strarray"
		}
		out = append(out, pd{Key: d.Key, Label: d.Label, Type: t, Sections: d.Sections, Hint: d.Hint, Path: strings.Join(d.Path, "/")})
	}
	writeJSON(w, out)
}

// ---------- tools endpoints ----------

func handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]interface{}{"settings": getSettings(), "os": runtime.GOOS})
	case http.MethodPost:
		// merge-семантика: поля, отсутствующие в запросе, сохраняют текущее значение
		s := getSettings()
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			jsonError(w, 400, err.Error())
			return
		}
		updateSettings(func(cur *Settings) { *cur = s })
		writeJSON(w, map[string]string{"status": "ok"})
	default:
		jsonError(w, 405, "метод не поддерживается")
	}
}

func handleToolsDetect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, 405, "метод не поддерживается")
		return
	}
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	glogf("dayz-tools", "Проверяю папку: %s", body.Path)
	_, list, err := tools.Detect(body.Path)
	if err != nil {
		glogf("dayz-tools", "ОШИБКА: %v", err)
		resp := map[string]interface{}{"error": err.Error()}
		if list != nil {
			resp["tools"] = list
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(422)
		json.NewEncoder(w).Encode(resp)
		return
	}
	found := 0
	for _, t := range list {
		if t.Found {
			found++
		}
	}
	glogf("dayz-tools", "Найдено инструментов: %d из %d", found, len(list))
	updateSettings(func(s *Settings) { s.ToolsPath = strings.TrimSpace(strings.Trim(body.Path, `"`)) })
	writeJSON(w, map[string]interface{}{"path": body.Path, "tools": list})
}

func handleToolsAutodetect(w http.ResponseWriter, r *http.Request) {
	glogf("dayz-tools", "Автопоиск установки DayZ Tools (реестр Steam, библиотеки, стандартные пути)…")
	path, list, err := tools.AutoDetect()
	if err != nil {
		glogf("dayz-tools", "Автопоиск не дал результата: %v", err)
		jsonError(w, 404, err.Error())
		return
	}
	glogf("dayz-tools", "Найдено: %s", path)
	updateSettings(func(s *Settings) { s.ToolsPath = path })
	writeJSON(w, map[string]interface{}{"path": path, "tools": list})
}

func handleCreateKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, 405, "метод не поддерживается")
		return
	}
	var body struct {
		ToolsPath string `json:"toolsPath"`
		Name      string `json:"name"`
		Dir       string `json:"dir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	if body.ToolsPath == "" {
		body.ToolsPath = getSettings().ToolsPath
	}
	ts, _, err := tools.Detect(body.ToolsPath)
	if err != nil {
		jsonError(w, 422, "DayZ Tools: "+err.Error())
		return
	}
	glogf("dayz-tools", "Создаю пару ключей: %s в %s", body.Name, body.Dir)
	priv, pub, out, err := tools.CreateKey(ts, body.Name, body.Dir)
	if err != nil {
		glogf("dayz-tools", "ОШИБКА создания ключа: %v", err)
		jsonError(w, 422, err.Error())
		return
	}
	glogf("dayz-tools", "Ключи созданы: %s, %s", priv, pub)
	updateSettings(func(s *Settings) { s.PrivateKeyPath = priv })
	writeJSON(w, map[string]string{"privateKey": priv, "publicKey": pub, "output": out})
}

// ---------- server ----------

func runServer(port int, noOpen bool) {
	loadSettings()
	sub, err := fs.Sub(staticFS, "web/static")
	if err != nil {
		fmt.Fprintln(os.Stderr, "внутренняя ошибка ресурсов:", err)
		os.Exit(1)
	}
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("/api/analyze", handleAnalyze)
	mux.HandleFunc("/api/generate", handleGenerate)
	mux.HandleFunc("/api/diff", handleDiff)
	mux.HandleFunc("/api/types/migrate", handleTypesMigrate)
	mux.HandleFunc("/api/types/optimize", handleTypesOptimize)
	mux.HandleFunc("/api/merge", handleMerge)
	mux.HandleFunc("/api/job/", handleJob)
	mux.HandleFunc("/api/log", handleGlobalLog)
	mux.HandleFunc("/api/log/clear", handleGlobalLog)
	mux.HandleFunc("/api/zip/", handleZip)
	mux.HandleFunc("/api/registry", handleRegistry)
	mux.HandleFunc("/api/settings", handleSettings)
	mux.HandleFunc("/api/tools/detect", handleToolsDetect)
	mux.HandleFunc("/api/tools/autodetect", handleToolsAutodetect)
	mux.HandleFunc("/api/tools/createkey", handleCreateKey)
	mux.HandleFunc("/api/validate", handleValidate)
	mux.HandleFunc("/api/modcfg/open", handleModcfgOpen)
	mux.HandleFunc("/api/modcfg/save", handleModcfgSave)
	mux.HandleFunc("/api/mission/open", handleMissionOpen)
	mux.HandleFunc("/api/mission/save", handleMissionSave)
	mux.HandleFunc("/api/spawn/check", handleSpawnCheck)
	mux.HandleFunc("/api/spawn/merge", handleSpawnMerge)
	mux.HandleFunc("/api/expq/check", handleExpqCheck)
	mux.HandleFunc("/api/expq/merge", handleExpqMerge)
	mux.HandleFunc("/api/expl/check", handleExplCheck)
	mux.HandleFunc("/api/expl/merge", handleExplMerge)
	mux.HandleFunc("/api/guide/load", handleGuideLoad)
	mux.HandleFunc("/api/guide/ask", handleGuideAsk)
	mux.HandleFunc("/api/ai/presets", handleAiPresets)
	mux.HandleFunc("/api/ai/test", handleAiTest)
	mux.HandleFunc("/api/res/open", handleResOpen)
	mux.HandleFunc("/api/res/text", handleResText)
	mux.HandleFunc("/api/res/img", handleResImg)
	mux.HandleFunc("/api/res/raw", handleResRaw)
	mux.HandleFunc("/api/res/replace", handleResReplace)
	mux.HandleFunc("/api/res/build", handleResBuild)
	mux.HandleFunc("/api/ping", handlePing)
	mux.HandleFunc("/api/version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"version": version})
	})

	var ln net.Listener
	p := port
	for try := 0; try < 20; try++ {
		ln, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err == nil {
			break
		}
		p++
	}
	if ln == nil {
		fmt.Fprintln(os.Stderr, "не удалось открыть порт:", err)
		os.Exit(1)
	}
	url := fmt.Sprintf("http://127.0.0.1:%d", p)
	glogf("система", "DayZ Override Builder v%s запущен: %s", version, url)
	fmt.Println("========================================================")
	fmt.Println("  DayZ Override Builder v" + version)
	fmt.Println("  Интерфейс запущен: " + url)
	fmt.Println("  Это окно — сама программа, его можно СВЕРНУТЬ.")
	fmt.Println("  Команды здесь: ENTER - открыть интерфейс в браузере,")
	fmt.Println("                 Q + ENTER - завершить программу.")
	fmt.Println("========================================================")
	if !noOpen {
		openBrowser(url)
	}
	go watchClients(url)
	go readConsole(url)
	srv := &http.Server{Handler: mux}
	if err := srv.Serve(ln); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
