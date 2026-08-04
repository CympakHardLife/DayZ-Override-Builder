package main

// Вкладка «GUI мода»: открытие PBO с ресурсами интерфейса (лайоуты, картинки,
// звуки, локализация), правка, пересборка и подпись.

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"dayzoverride/edds"
	"dayzoverride/pbo"
	"dayzoverride/rap"
	"dayzoverride/tools"
)

type resFile struct {
	Path string `json:"path"`
	Size int    `json:"size"`
	Kind string `json:"kind"` // text | config | image | sound | binary
	Mod  bool   `json:"mod"`  // изменён пользователем
}

type resSession struct {
	mu       sync.Mutex
	created  time.Time
	pboPath  string // временная копия исходного PBO
	origName string
	prefix   string
	entries  []pbo.Entry
	modified map[string][]byte
}

var resSessions = struct {
	mu sync.Mutex
	m  map[string]*resSession
}{m: map[string]*resSession{}}

func getResSession(id string) *resSession {
	resSessions.mu.Lock()
	defer resSessions.mu.Unlock()
	return resSessions.m[id]
}

func classifyRes(name string) string {
	low := strings.ToLower(name)
	base := filepath.Base(strings.ReplaceAll(low, "\\", "/"))
	switch {
	case base == "config.bin":
		return "config"
	case strings.HasSuffix(low, ".layout"), strings.HasSuffix(low, ".xml"), strings.HasSuffix(low, ".csv"),
		strings.HasSuffix(low, ".txt"), strings.HasSuffix(low, ".json"), strings.HasSuffix(low, ".c"),
		strings.HasSuffix(low, ".cpp"), strings.HasSuffix(low, ".hpp"), strings.HasSuffix(low, ".cfg"),
		strings.HasSuffix(low, ".ini"), strings.HasSuffix(low, ".md"), strings.HasSuffix(low, ".meta"):
		return "text"
	case strings.HasSuffix(low, ".edds"), strings.HasSuffix(low, ".png"), strings.HasSuffix(low, ".jpg"),
		strings.HasSuffix(low, ".jpeg"), strings.HasSuffix(low, ".dds"), strings.HasSuffix(low, ".paa"):
		return "image"
	case strings.HasSuffix(low, ".ogg"), strings.HasSuffix(low, ".wav"), strings.HasSuffix(low, ".wss"):
		return "sound"
	}
	return "binary"
}

func (s *resSession) fileList() []resFile {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]resFile, 0, len(s.entries))
	for i := range s.entries {
		e := &s.entries[i]
		size := int(e.DataSize)
		if m, ok := s.modified[strings.ToLower(e.Name)]; ok {
			size = len(m)
		}
		out = append(out, resFile{
			Path: e.Name,
			Size: size,
			Kind: classifyRes(e.Name),
			Mod:  hasKey(s.modified, e.Name),
		})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Path) < strings.ToLower(out[j].Path) })
	return out
}

func hasKey(m map[string][]byte, name string) bool {
	_, ok := m[strings.ToLower(name)]
	return ok
}

// readEntryData возвращает актуальные байты файла (с учётом правок).
func (s *resSession) readEntryData(path string) ([]byte, error) {
	s.mu.Lock()
	if m, ok := s.modified[strings.ToLower(path)]; ok {
		s.mu.Unlock()
		return m, nil
	}
	s.mu.Unlock()
	p, err := pbo.Open(s.pboPath)
	if err != nil {
		return nil, err
	}
	defer p.Close()
	for i := range p.Entries {
		if strings.EqualFold(p.Entries[i].Name, path) {
			return p.ReadEntry(&p.Entries[i])
		}
	}
	return nil, fmt.Errorf("файл %s не найден в PBO", path)
}

// ---------- endpoints ----------

func handleResOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, 405, "метод не поддерживается")
		return
	}
	var srcPath, origName string
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			jsonError(w, 400, "ошибка загрузки: "+err.Error())
			return
		}
		fhs := r.MultipartForm.File["pbo"]
		if len(fhs) == 0 {
			jsonError(w, 400, "PBO не передан")
			return
		}
		src, err := fhs[0].Open()
		if err != nil {
			jsonError(w, 400, err.Error())
			return
		}
		defer src.Close()
		tmp, err := os.CreateTemp("", "dob-res-*.pbo")
		if err != nil {
			jsonError(w, 500, err.Error())
			return
		}
		if _, err := io.Copy(tmp, src); err != nil {
			tmp.Close()
			jsonError(w, 500, err.Error())
			return
		}
		tmp.Close()
		srcPath = tmp.Name()
		origName = filepath.Base(fhs[0].Filename)
	} else {
		var body struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, 400, err.Error())
			return
		}
		body.Path = strings.TrimSpace(strings.Trim(body.Path, `"`))
		st, err := os.Stat(body.Path)
		if err != nil || st.IsDir() {
			jsonError(w, 400, "файл не найден: "+body.Path)
			return
		}
		// копия, чтобы правки не зависели от исходника
		tmp, err := os.CreateTemp("", "dob-res-*.pbo")
		if err != nil {
			jsonError(w, 500, err.Error())
			return
		}
		src, err := os.Open(body.Path)
		if err != nil {
			tmp.Close()
			jsonError(w, 400, err.Error())
			return
		}
		if _, err := io.Copy(tmp, src); err != nil {
			src.Close()
			tmp.Close()
			jsonError(w, 500, err.Error())
			return
		}
		src.Close()
		tmp.Close()
		srcPath = tmp.Name()
		origName = filepath.Base(body.Path)
	}

	p, err := pbo.Open(srcPath)
	if err != nil {
		os.Remove(srcPath)
		jsonError(w, 422, "не удалось разобрать PBO: "+err.Error()+" (возможно, зашифрован/обфусцирован)")
		return
	}
	sess := &resSession{
		created:  time.Now(),
		pboPath:  srcPath,
		origName: origName,
		prefix:   p.Prefix(),
		entries:  append([]pbo.Entry(nil), p.Entries...),
		modified: map[string][]byte{},
	}
	p.Close()
	if sess.prefix == "" {
		sess.prefix = strings.TrimSuffix(origName, filepath.Ext(origName))
	}
	sid := randToken()
	resSessions.mu.Lock()
	for k, v := range resSessions.m {
		if time.Since(v.created) > 4*time.Hour {
			os.Remove(v.pboPath)
			delete(resSessions.m, k)
		}
	}
	resSessions.m[sid] = sess
	resSessions.mu.Unlock()
	glogf("gui-мод", "Открыт %s: %d файлов, prefix=%s", origName, len(sess.entries), sess.prefix)
	writeJSON(w, map[string]interface{}{
		"sid":    sid,
		"name":   origName,
		"prefix": sess.prefix,
		"files":  sess.fileList(),
	})
}

func resSessionFromReq(w http.ResponseWriter, r *http.Request) *resSession {
	sid := r.URL.Query().Get("sid")
	if sid == "" {
		sid = r.FormValue("sid")
	}
	s := getResSession(sid)
	if s == nil {
		jsonError(w, 404, "сессия не найдена или устарела — откройте PBO заново")
		return nil
	}
	return s
}

func handleResText(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s := resSessionFromReq(w, r)
		if s == nil {
			return
		}
		path := r.URL.Query().Get("path")
		data, err := s.readEntryData(path)
		if err != nil {
			jsonError(w, 404, err.Error())
			return
		}
		if len(data) > 8<<20 {
			jsonError(w, 413, "файл слишком большой для текстового редактора")
			return
		}
		readonly := false
		content := ""
		if classifyRes(path) == "config" {
			raw := data
			if !rap.IsBinarized(raw) {
				if dec, ok := rap.TryDecompressRaP(raw); ok {
					raw = dec
				}
			}
			if rap.IsBinarized(raw) {
				root, err := rap.ParseBin(raw)
				if err != nil {
					jsonError(w, 422, "config.bin: "+err.Error())
					return
				}
				content = "// config.bin (дебинаризован для просмотра, только чтение)\n// правки конфигов делайте через вкладку «1 Мод» -> оверрайд\n\n" + rap.ToCPP(root)
				readonly = true
			} else {
				content = rap.CleanString(raw)
			}
		} else {
			content = rap.CleanString(data)
		}
		writeJSON(w, map[string]interface{}{"content": content, "readonly": readonly})
	case http.MethodPost:
		var body struct {
			Sid     string `json:"sid"`
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, 400, err.Error())
			return
		}
		s := getResSession(body.Sid)
		if s == nil {
			jsonError(w, 404, "сессия не найдена или устарела — откройте PBO заново")
			return
		}
		if classifyRes(body.Path) == "config" {
			jsonError(w, 422, "config.bin доступен только для просмотра — правки конфигов делайте через вкладку «1 Мод»")
			return
		}
		s.mu.Lock()
		s.modified[strings.ToLower(body.Path)] = []byte(body.Content)
		s.mu.Unlock()
		glogf("gui-мод", "Изменён файл: %s (%.1f КБ)", body.Path, float64(len(body.Content))/1024)
		writeJSON(w, map[string]string{"status": "ok"})
	default:
		jsonError(w, 405, "метод не поддерживается")
	}
}

func handleResImg(w http.ResponseWriter, r *http.Request) {
	s := resSessionFromReq(w, r)
	if s == nil {
		return
	}
	path := r.URL.Query().Get("path")
	data, err := s.readEntryData(path)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	low := strings.ToLower(path)
	var img image.Image
	switch {
	case strings.HasSuffix(low, ".png"), strings.HasSuffix(low, ".jpg"), strings.HasSuffix(low, ".jpeg"):
		w.Header().Set("Content-Type", "image/*")
		w.Write(data)
		return
	case strings.HasSuffix(low, ".edds"), strings.HasSuffix(low, ".dds"):
		img, err = edds.Decode(data)
	default:
		err = fmt.Errorf("превью для этого формата не поддерживается")
	}
	if err != nil {
		http.Error(w, err.Error(), 415)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	png.Encode(w, img)
}

func handleResRaw(w http.ResponseWriter, r *http.Request) {
	s := resSessionFromReq(w, r)
	if s == nil {
		return
	}
	path := r.URL.Query().Get("path")
	data, err := s.readEntryData(path)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	low := strings.ToLower(path)
	ct := "application/octet-stream"
	switch {
	case strings.HasSuffix(low, ".ogg"):
		ct = "audio/ogg"
	case strings.HasSuffix(low, ".wav"):
		ct = "audio/wav"
	}
	w.Header().Set("Content-Type", ct)
	w.Write(data)
}

func handleResReplace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, 405, "метод не поддерживается")
		return
	}
	if err := r.ParseMultipartForm(256 << 20); err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	s := resSessionFromReq(w, r)
	if s == nil {
		return
	}
	path := r.FormValue("path")
	fhs := r.MultipartForm.File["file"]
	if len(fhs) == 0 {
		jsonError(w, 400, "файл не передан")
		return
	}
	src, err := fhs[0].Open()
	if err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	data, err := io.ReadAll(io.LimitReader(src, 512<<20))
	src.Close()
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	upName := strings.ToLower(fhs[0].Filename)
	targetLow := strings.ToLower(path)
	note := ""
	// PNG/JPG -> EDDS автоконвертация
	if strings.HasSuffix(targetLow, ".edds") &&
		(strings.HasSuffix(upName, ".png") || strings.HasSuffix(upName, ".jpg") || strings.HasSuffix(upName, ".jpeg")) {
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			jsonError(w, 422, "не удалось прочитать картинку: "+err.Error())
			return
		}
		data = edds.Encode(img)
		note = " (PNG сконвертирован в EDDS: несжатый BGRA8 с мип-уровнями)"
	}
	s.mu.Lock()
	s.modified[targetLow] = data
	s.mu.Unlock()
	glogf("gui-мод", "Заменён файл: %s <- %s (%.1f КБ)%s", path, fhs[0].Filename, float64(len(data))/1024, note)
	writeJSON(w, map[string]string{"status": "ok", "note": note})
}

func handleResBuild(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, 405, "метод не поддерживается")
		return
	}
	var body struct {
		Sid            string `json:"sid"`
		Sign           bool   `json:"sign"`
		ToolsPath      string `json:"toolsPath"`
		PrivateKeyPath string `json:"privateKeyPath"`
		OutputPath     string `json:"outputPath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	s := getResSession(body.Sid)
	if s == nil {
		jsonError(w, 404, "сессия не найдена или устарела — откройте PBO заново")
		return
	}
	if body.ToolsPath == "" {
		body.ToolsPath = getSettings().ToolsPath
	}
	if body.PrivateKeyPath == "" {
		body.PrivateKeyPath = getSettings().PrivateKeyPath
	}
	jobID, job := newJob("gui-сборка")
	go func() {
		defer job.guard()
		s.mu.Lock()
		modCount := len(s.modified)
		s.mu.Unlock()
		job.logf("Пересобираю %s: заменено файлов %d", s.origName, modCount)
		p, err := pbo.Open(s.pboPath)
		if err != nil {
			job.finish(nil, err)
			return
		}
		var files []pbo.File
		for i := range p.Entries {
			e := &p.Entries[i]
			s.mu.Lock()
			m, ok := s.modified[strings.ToLower(e.Name)]
			s.mu.Unlock()
			if ok {
				files = append(files, pbo.File{Name: e.Name, Data: m})
				continue
			}
			data, err := p.ReadEntry(e)
			if err != nil {
				p.Close()
				job.finish(nil, fmt.Errorf("%s: %v", e.Name, err))
				return
			}
			files = append(files, pbo.File{Name: e.Name, Data: data})
		}
		p.Close()
		pboData := pbo.Build(s.prefix, files)
		job.logf("PBO собран: %.1f МБ (prefix=%s)", float64(len(pboData))/1e6, s.prefix)

		outFiles := []struct {
			Name string
			Data []byte
		}{{s.origName, pboData}}

		if body.Sign {
			job.logf("Подпись: сканирую DayZ Tools…")
			ts, _, terr := tools.Detect(body.ToolsPath)
			if terr != nil {
				job.finish(nil, fmt.Errorf("DayZ Tools: %v", terr))
				return
			}
			work, err := os.MkdirTemp("", "dob-guisign-*")
			if err != nil {
				job.finish(nil, err)
				return
			}
			defer os.RemoveAll(work)
			pboOnDisk := filepath.Join(work, s.origName)
			if err := os.WriteFile(pboOnDisk, pboData, 0644); err != nil {
				job.finish(nil, err)
				return
			}
			bisign, _, err := tools.SignPbo(ts, body.PrivateKeyPath, pboOnDisk)
			if err != nil {
				job.finish(nil, fmt.Errorf("подпись: %v", err))
				return
			}
			bs, err := os.ReadFile(bisign)
			if err != nil {
				job.finish(nil, err)
				return
			}
			job.logf("Подпись создана: %s", filepath.Base(bisign))
			outFiles = append(outFiles, struct {
				Name string
				Data []byte
			}{filepath.Base(bisign), bs})
			if pub := tools.SiblingBikey(body.PrivateKeyPath); pub != "" {
				if pd, err := os.ReadFile(pub); err == nil {
					outFiles = append(outFiles, struct {
						Name string
						Data []byte
					}{"keys/" + filepath.Base(pub), pd})
					job.logf("Публичный ключ добавлен: keys/%s", filepath.Base(pub))
				}
			}
		}

		// zip
		var zbuf bytes.Buffer
		zw := zip.NewWriter(&zbuf)
		var names []string
		for _, f := range outFiles {
			wr, err := zw.Create(f.Name)
			if err != nil {
				job.finish(nil, err)
				return
			}
			if _, err := wr.Write(f.Data); err != nil {
				job.finish(nil, err)
				return
			}
			names = append(names, f.Name)
		}
		if err := zw.Close(); err != nil {
			job.finish(nil, err)
			return
		}
		tok := zips.put(strings.TrimSuffix(s.origName, ".pbo")+"_edited.zip", zbuf.Bytes())
		resp := map[string]interface{}{
			"files":    names,
			"zipToken": tok,
			"modCount": modCount,
		}
		if body.OutputPath != "" {
			job.logf("Сохраняю в папку: %s", body.OutputPath)
			for _, f := range outFiles {
				full := filepath.Join(body.OutputPath, filepath.FromSlash(f.Name))
				if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
					resp["saveError"] = err.Error()
					break
				}
				if err := os.WriteFile(full, f.Data, 0644); err != nil {
					resp["saveError"] = err.Error()
					break
				}
			}
			if _, bad := resp["saveError"]; !bad {
				resp["savedTo"] = body.OutputPath
				job.logf("Сохранено: %s", body.OutputPath)
			}
		}
		job.logf("Готово. ВАЖНО: пересобранный PBO нужно подписывать ВАШИМ ключом, а ваш .bikey положить в keys сервера и клиентам.")
		job.finish(resp, nil)
	}()
	writeJSON(w, map[string]string{"jobId": jobID})
}

