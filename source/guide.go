// Package guide скачивает гайды по ссылке (github wiki/страницы/markdown),
// кэширует их на диске и структурно извлекает пары «параметр → описание».
package guide

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Logf func(format string, args ...interface{})

func (f Logf) p(format string, args ...interface{}) {
	if f != nil {
		f(format, args...)
	}
}

var httpClient = &http.Client{Timeout: 45 * time.Second}

const userAgent = "DayZOverrideBuilder (guide reader; local tool)"

// ---------- скачивание ----------

type Doc struct {
	URL       string    `json:"url"`
	Text      string    `json:"-"`
	FromCache bool      `json:"fromCache"`
	FetchedAt time.Time `json:"fetchedAt"`
	Parts     int       `json:"parts"` // сколько страниц слито (для github-папок)
}

type cacheMeta struct {
	URL       string    `json:"url"`
	FetchedAt time.Time `json:"fetchedAt"`
	Parts     int       `json:"parts"`
}

func cacheKey(rawURL string) string {
	h := sha1.Sum([]byte(strings.TrimSpace(rawURL)))
	return hex.EncodeToString(h[:])
}

// Fetch скачивает гайд (или берёт из кэша). refresh=true — скачать заново.
func Fetch(rawURL, cacheDir string, refresh bool, logf Logf) (*Doc, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("ссылка на гайд пуста")
	}
	if !strings.Contains(rawURL, "://") {
		rawURL = "https://" + rawURL
	}
	key := cacheKey(rawURL)
	txtPath := filepath.Join(cacheDir, key+".txt")
	metaPath := filepath.Join(cacheDir, key+".json")

	if !refresh && cacheDir != "" {
		if data, err := os.ReadFile(txtPath); err == nil && len(data) > 0 {
			var m cacheMeta
			if mb, err2 := os.ReadFile(metaPath); err2 == nil {
				json.Unmarshal(mb, &m)
			}
			logf.p("Гайд взят из кэша (скачан %s) — %d КБ. Кнопка ⟳ скачает заново.",
				m.FetchedAt.Format("02.01.2006 15:04"), len(data)/1024)
			return &Doc{URL: rawURL, Text: string(data), FromCache: true, FetchedAt: m.FetchedAt, Parts: m.Parts}, nil
		}
	}

	text, parts, err := download(rawURL, logf)
	if err != nil {
		return nil, err
	}
	doc := &Doc{URL: rawURL, Text: text, FetchedAt: time.Now(), Parts: parts}
	if cacheDir != "" {
		if err := os.MkdirAll(cacheDir, 0755); err == nil {
			os.WriteFile(txtPath, []byte(text), 0644)
			mb, _ := json.Marshal(cacheMeta{URL: rawURL, FetchedAt: doc.FetchedAt, Parts: parts})
			os.WriteFile(metaPath, mb, 0644)
			logf.p("Гайд сохранён в кэш — повторные открытия не будут тратить интернет.")
		}
	}
	return doc, nil
}

// download понимает github-ссылки (wiki, blob, tree=папка) и обычные страницы.
func download(rawURL string, logf Logf) (string, int, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", 0, fmt.Errorf("некорректная ссылка: %v", err)
	}
	host := strings.ToLower(u.Hostname())
	segs := splitPath(u.Path)

	if host == "github.com" && len(segs) >= 2 {
		owner, repo := segs[0], segs[1]
		switch {
		case len(segs) >= 3 && segs[2] == "wiki":
			page := "Home"
			if len(segs) >= 4 && segs[3] != "" {
				page = segs[3]
			}
			raw := fmt.Sprintf("https://raw.githubusercontent.com/wiki/%s/%s/%s.md", owner, repo, page)
			logf.p("Ссылка на github wiki → скачиваю markdown: %s", raw)
			t, err := fetchText(raw)
			return t, 1, err
		case len(segs) >= 5 && segs[2] == "blob":
			raw := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s", owner, repo, strings.Join(segs[3:], "/"))
			logf.p("Ссылка на файл github → скачиваю: %s", raw)
			t, err := fetchText(raw)
			return t, 1, err
		case len(segs) >= 4 && segs[2] == "tree":
			branch := segs[3]
			sub := strings.Join(segs[4:], "/")
			logf.p("Ссылка на ПАПКУ github (%s) — запрашиваю список файлов…", sub)
			return downloadGithubDir(owner, repo, branch, sub, logf)
		}
	}
	logf.p("Скачиваю страницу: %s", rawURL)
	t, err := fetchText(rawURL)
	return t, 1, err
}

type ghEntry struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Type        string `json:"type"`
	DownloadURL string `json:"download_url"`
}

func githubList(owner, repo, branch, sub string) ([]ghEntry, error) {
	api := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s?ref=%s",
		owner, repo, url.PathEscape(sub), url.QueryEscape(branch))
	body, err := fetchBytes(api)
	if err != nil {
		return nil, fmt.Errorf("не удалось получить список файлов папки: %v", err)
	}
	var list []ghEntry
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("github вернул неожиданный ответ (возможно, лимит запросов — попробуйте позже или дайте ссылку на конкретную страницу)")
	}
	return list, nil
}

// downloadGithubDir сливает .md/.txt файлы папки репозитория (и её подпапок,
// один уровень вглубь) в один текст.
func downloadGithubDir(owner, repo, branch, sub string, logf Logf) (string, int, error) {
	list, err := githubList(owner, repo, branch, sub)
	if err != nil {
		return "", 0, err
	}
	// подпапки: раскрываем один уровень (гайды часто разложены по папкам модов)
	var files []ghEntry
	dirs := 0
	for _, f := range list {
		if f.Type == "dir" && dirs < 25 {
			dirs++
			subList, err := githubList(owner, repo, branch, f.Path)
			if err != nil {
				logf.p("  папка %s — пропускаю (%v)", f.Name, err)
				continue
			}
			for _, sf := range subList {
				if sf.Type == "file" {
					sf.Name = f.Name + "/" + sf.Name
					files = append(files, sf)
				}
			}
			continue
		}
		if f.Type == "file" {
			files = append(files, f)
		}
	}
	var sb strings.Builder
	parts := 0
	total := 0
	for _, f := range files {
		low := strings.ToLower(f.Name)
		if !strings.HasSuffix(low, ".md") && !strings.HasSuffix(low, ".txt") {
			continue
		}
		if parts >= 60 || total > 4<<20 {
			logf.p("Файлов слишком много — беру первые %d", parts)
			break
		}
		t, err := fetchText(f.DownloadURL)
		if err != nil {
			logf.p("  %s — пропускаю (%v)", f.Name, err)
			continue
		}
		parts++
		total += len(t)
		logf.p("  + %s (%d КБ)", f.Name, len(t)/1024)
		sb.WriteString("\n\n# ФАЙЛ: " + f.Name + "\n\n")
		sb.WriteString(t)
	}
	if parts == 0 {
		return "", 0, fmt.Errorf("в папке не нашлось .md/.txt файлов гайда")
	}
	logf.p("Слито страниц гайда: %d (%d КБ)", parts, total/1024)
	return sb.String(), parts, nil
}

func fetchBytes(rawURL string) ([]byte, error) {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, humanNetErr(rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("страница не найдена (404) — проверьте ссылку")
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("сервер ответил ошибкой %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	return data, nil
}

func fetchText(rawURL string) (string, error) {
	data, err := fetchBytes(rawURL)
	if err != nil {
		return "", err
	}
	s := string(data)
	if looksLikeHTML(s) {
		s = htmlToText(s)
	}
	if strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("страница пуста после разбора — возможно, сайт требует JavaScript; сохраните текст гайда в файл и дайте ссылку на raw-версию")
	}
	return s, nil
}

func humanNetErr(rawURL string, err error) error {
	host := rawURL
	if u, e := url.Parse(rawURL); e == nil {
		host = u.Hostname()
	}
	return fmt.Errorf("нет доступа к %s — проверьте интернет/файрвол (%v)", host, err)
}

func splitPath(p string) []string {
	var out []string
	for _, s := range strings.Split(p, "/") {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// ---------- HTML → текст ----------

var (
	reScript  = regexp.MustCompile(`(?is)<(script|style|noscript|svg)[^>]*>.*?</\s*(script|style|noscript|svg)\s*>`)
	reComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	reHead    = regexp.MustCompile(`(?is)<h([1-6])[^>]*>`)
	reBreak   = regexp.MustCompile(`(?i)<(br|/p|/div|/li|/tr|/h[1-6]|/table|/ul|/ol)[^>]*>`)
	reCell    = regexp.MustCompile(`(?i)<(td|th)[^>]*>`)
	reLi      = regexp.MustCompile(`(?i)<li[^>]*>`)
	reCode    = regexp.MustCompile(`(?i)</?(code|tt)[^>]*>`)
	reTag     = regexp.MustCompile(`(?s)<[^>]*>`)
	reBlank   = regexp.MustCompile(`\n{3,}`)
	reSpaces  = regexp.MustCompile(`[ \t]{2,}`)
)

func looksLikeHTML(s string) bool {
	head := strings.ToLower(s[:min(len(s), 2048)])
	return strings.Contains(head, "<html") || strings.Contains(head, "<!doctype html") ||
		strings.Contains(head, "<body") || strings.Contains(head, "<div")
}

func htmlToText(s string) string {
	s = reScript.ReplaceAllString(s, " ")
	s = reComment.ReplaceAllString(s, " ")
	s = reHead.ReplaceAllStringFunc(s, func(m string) string {
		sub := reHead.FindStringSubmatch(m)
		n := 2
		if len(sub) > 1 {
			fmt.Sscanf(sub[1], "%d", &n)
		}
		return "\n\n" + strings.Repeat("#", n) + " "
	})
	s = reCell.ReplaceAllString(s, " | ")
	s = reLi.ReplaceAllString(s, "\n- ")
	s = reBreak.ReplaceAllString(s, "\n")
	s = reCode.ReplaceAllString(s, "`")
	s = reTag.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, " ", " ")
	s = reSpaces.ReplaceAllString(s, " ")
	var lines []string
	for _, l := range strings.Split(s, "\n") {
		lines = append(lines, strings.TrimRight(l, " \t"))
	}
	s = strings.Join(lines, "\n")
	return strings.TrimSpace(reBlank.ReplaceAllString(s, "\n\n"))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ---------- структурное извлечение «параметр → описание» ----------

type KV struct {
	Key  string `json:"k"`
	Desc string `json:"d"`
}

var (
	// имя параметра: Word, Word.Word, snake_case, camelCase, с [] и *
	reParamName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(?:\.[A-Za-z0-9_\[\]\*]+)*$`)
	reHeading   = regexp.MustCompile("^\\s{0,3}#{1,6}\\s*(.+?)\\s*#*\\s*$")
	reDefLine   = regexp.MustCompile("^\\s*(?:[-*+]\\s*)?(?:\\*\\*|`)([A-Za-z][A-Za-z0-9_.\\[\\]\\*]*)(?:\\*\\*|`)\\s*(?:\\((?:[^)]*)\\))?\\s*[—–:=-]\\s+(.+)$")
	reCfgLine   = regexp.MustCompile(`^\s*([A-Za-z][A-Za-z0-9_.]*)\s*=\s*[^;]*;\s*//\s*(.+)$`)
	reTableSep  = regexp.MustCompile(`^\s*\|?[\s:|-]+\|?\s*$`)
)

func cleanKeyToken(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "`*_ \t")
	// «Key (type)» → Key
	if i := strings.IndexAny(s, " ("); i > 0 {
		head := strings.TrimSpace(s[:i])
		if reParamName.MatchString(head) {
			return head
		}
	}
	return s
}

const maxDescLen = 400

func clipDesc(s string) string {
	s = strings.TrimSpace(strings.Trim(s, "|"))
	s = reSpaces.ReplaceAllString(s, " ")
	r := []rune(s)
	if len(r) > maxDescLen {
		return string(r[:maxDescLen-1]) + "…"
	}
	return s
}

// Extract вытаскивает пары «параметр → описание» из markdown/текста гайда.
func Extract(text string) []KV {
	found := map[string]KV{} // lower key -> kv (берём самое длинное описание)
	put := func(key, desc string) {
		key = cleanKeyToken(key)
		desc = clipDesc(desc)
		if key == "" || desc == "" || !reParamName.MatchString(key) {
			return
		}
		if len(key) < 3 || len(desc) < 8 {
			return
		}
		low := strings.ToLower(key)
		if old, ok := found[low]; !ok || len(desc) > len(old.Desc) {
			found[low] = KV{Key: key, Desc: desc}
		}
	}

	lines := strings.Split(text, "\n")
	inFence := false
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		// строки конфигов вида «Key = value; // описание» (в т.ч. внутри код-блоков)
		if m := reCfgLine.FindStringSubmatch(line); m != nil {
			desc := m[2]
			// убираем терье-мету «[type: ...; default: ...]» из описания
			if j := strings.Index(desc, "]"); strings.HasPrefix(strings.TrimSpace(desc), "[") && j > 0 {
				desc = strings.TrimSpace(desc[j+1:])
			}
			put(m[1], desc)
			continue
		}
		if inFence {
			continue
		}
		// заголовок с именем параметра → описание из абзаца ниже
		if m := reHeading.FindStringSubmatch(line); m != nil {
			key := cleanKeyToken(m[1])
			if !reParamName.MatchString(key) {
				continue
			}
			var buf []string
			for j := i + 1; j < len(lines) && j < i+16; j++ {
				t := strings.TrimSpace(lines[j])
				if t == "" {
					continue // описание бывает разбито на абзацы (тип + суть)
				}
				if reHeading.MatchString(lines[j]) || strings.HasPrefix(t, "```") {
					break
				}
				if strings.HasPrefix(t, "|") { // таблицы разберём отдельно
					break
				}
				buf = append(buf, t)
			}
			if len(buf) > 0 {
				put(key, strings.Join(buf, " "))
			}
			continue
		}
		// строка таблицы: | `Key` | что-то | описание |
		if strings.HasPrefix(trimmed, "|") && strings.Count(trimmed, "|") >= 2 && !reTableSep.MatchString(trimmed) {
			cells := strings.Split(strings.Trim(trimmed, "|"), "|")
			var key string
			var descCells []string
			for ci, c := range cells {
				c = strings.TrimSpace(c)
				if c == "" {
					continue
				}
				tok := cleanKeyToken(c)
				if key == "" && ci <= 1 && reParamName.MatchString(tok) && len(tok) >= 3 {
					key = tok
					continue
				}
				descCells = append(descCells, c)
			}
			if key != "" && len(descCells) > 0 {
				sort.SliceStable(descCells, func(a, b int) bool { return len(descCells[a]) > len(descCells[b]) })
				put(key, descCells[0])
			}
			continue
		}
		// определение: **Key** — описание / `Key`: описание
		if m := reDefLine.FindStringSubmatch(line); m != nil {
			put(m[1], m[2])
		}
	}

	out := make([]KV, 0, len(found))
	for _, kv := range found {
		out = append(out, kv)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Key) < strings.ToLower(out[j].Key) })
	return out
}

// ---------- сопоставление с параметрами открытого файла ----------

type FieldRef struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

// isSpecific — достаточно ли «характерно» имя, чтобы матчить по последнему сегменту.
func isSpecific(key string) bool {
	if strings.Contains(key, ".") {
		return true
	}
	if len(key) >= 10 {
		return true
	}
	// camelCase или snake_case длиной от 6
	if len(key) >= 6 && (strings.ContainsAny(key, "_") || strings.ToLower(key) != key) {
		return true
	}
	return false
}

func lastSeg(path string) string {
	if i := strings.LastIndex(path, "."); i >= 0 {
		return path[i+1:]
	}
	return path
}

// Match возвращает id поля → описание из гайда.
func Match(kvs []KV, fields []FieldRef) map[string]string {
	if len(kvs) == 0 || len(fields) == 0 {
		return map[string]string{}
	}
	exact := map[string]string{}
	specific := map[string]string{} // только характерные ключи (для суффиксных матчей)
	bySeg := map[string][]KV{}
	for _, kv := range kvs {
		low := strings.ToLower(kv.Key)
		exact[low] = kv.Desc
		if isSpecific(kv.Key) || len(kv.Key) >= 6 {
			specific[low] = kv.Desc
			seg := strings.ToLower(lastSeg(kv.Key))
			bySeg[seg] = append(bySeg[seg], kv)
		}
	}
	// try: точное совпадение, затем суффиксные (только по характерным ключам)
	try := func(p string) (string, bool) {
		if d, ok := exact[p]; ok {
			return d, true
		}
		best, bestLen := "", 0
		for low, d := range specific {
			if len(low) > bestLen && (strings.HasSuffix(p, "."+low) || strings.HasSuffix(low, "."+p)) {
				best, bestLen = d, len(low)
			}
		}
		return best, best != ""
	}
	out := map[string]string{}
	for _, f := range fields {
		p := strings.ToLower(strings.TrimSpace(f.Path))
		if p == "" {
			continue
		}
		if d, ok := try(p); ok {
			out[f.ID] = d
			continue
		}
		// поля вида «Путь.all» / «Путь.city»: короткий хвост отбрасываем и пробуем родителя
		seg := lastSeg(p)
		if len(seg) <= 4 && strings.Contains(p, ".") {
			parent := strings.TrimSuffix(p, "."+seg)
			if d, ok := try(parent); ok {
				out[f.ID] = d
				continue
			}
			seg = lastSeg(parent)
		}
		// последний сегмент — только характерные имена с обеих сторон
		if isSpecific(seg) {
			if cands, ok := bySeg[seg]; ok && len(cands) > 0 {
				out[f.ID] = cands[0].Desc
			}
		}
	}
	return out
}
