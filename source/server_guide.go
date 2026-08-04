package main

// «Гайд по ссылке»: скачивание гайдов, структурный разбор, режим через ИИ
// и вопросы по гайду. Работает на вкладках Inedia, Terje, Exp.Квесты, Exp.Лодауты.

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"dayzoverride/ai"
	"dayzoverride/guide"
)

// guidesCacheDir — папка кэша гайдов рядом с settings.json.
func guidesCacheDir() string {
	for _, p := range settingsCandidates() {
		dir := filepath.Join(filepath.Dir(p), "guides_cache")
		if err := os.MkdirAll(dir, 0755); err == nil {
			return dir
		}
	}
	return ""
}

func aiConfig() (ai.Config, bool) {
	s := getSettings()
	cfg := ai.Config{
		BaseURL: strings.TrimSpace(s.AiBaseURL),
		APIKey:  strings.TrimSpace(s.AiKey),
		Model:   strings.TrimSpace(s.AiModel),
	}
	ready := s.AiEnabled && cfg.BaseURL != "" && cfg.Model != ""
	return cfg, ready
}

// POST /api/guide/load {url, mode: "struct"|"ai", refresh, fields:[{id,path}]}
func handleGuideLoad(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, 405, "метод не поддерживается")
		return
	}
	var body struct {
		URL     string           `json:"url"`
		Mode    string           `json:"mode"`
		Refresh bool             `json:"refresh"`
		Fields  []guide.FieldRef `json:"fields"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(body.URL) == "" {
		jsonError(w, 400, "вставьте ссылку на гайд")
		return
	}
	useAI := body.Mode == "ai"
	cfg, ready := aiConfig()
	if useAI && !ready {
		jsonError(w, 422, "режим «через ИИ» недоступен: ИИ-помощник выключен или не настроен — нажмите кнопку 🤖 вверху и заполните настройки")
		return
	}
	if useAI && len(body.Fields) == 0 {
		jsonError(w, 422, "режим «через ИИ» подставляет описания к параметрам — сначала откройте файл конфига")
		return
	}

	jobID, job := newJob("гайд")
	go func() {
		defer job.guard()
		doc, err := guide.Fetch(body.URL, guidesCacheDir(), body.Refresh, job.logf)
		if err != nil {
			job.finish(nil, err)
			return
		}
		job.logf("Текст гайда: %d КБ. Структурный разбор…", len(doc.Text)/1024)
		kvs := guide.Extract(doc.Text)
		job.logf("Найдено пар «параметр → описание»: %d", len(kvs))

		type matchVal struct {
			D  string `json:"d"`
			AI bool   `json:"ai"`
		}
		matches := map[string]matchVal{}
		structMatches := guide.Match(kvs, body.Fields)
		for id, d := range structMatches {
			matches[id] = matchVal{D: d}
		}
		if len(body.Fields) > 0 {
			job.logf("Структурно совпало с параметрами вашего файла: %d из %d", len(structMatches), len(body.Fields))
		} else {
			job.logf("Файл конфига не открыт — показываю только извлечённый список. Откройте файл и загрузите гайд ещё раз, чтобы подставить описания.")
		}

		var warnings []string
		aiCount := 0
		if useAI {
			var params []string
			seen := map[string]bool{}
			for _, f := range body.Fields {
				p := strings.TrimSpace(f.Path)
				if p != "" && !seen[p] {
					seen[p] = true
					params = append(params, p)
				}
			}
			job.logf("Режим «через ИИ»: прошу нейросеть описать %d параметров по гайду (на русском)…", len(params))
			descs, err := ai.DescribeParams(cfg, doc.Text, params, job.logf)
			if err != nil && len(descs) == 0 {
				warnings = append(warnings, "ИИ не ответил: "+err.Error()+" — использован структурный разбор.")
			}
			byPath := map[string]string{}
			for p, d := range descs {
				byPath[strings.ToLower(p)] = d
			}
			for _, f := range body.Fields {
				if d, ok := byPath[strings.ToLower(f.Path)]; ok {
					matches[f.ID] = matchVal{D: d, AI: true}
					aiCount++
				}
			}
			job.logf("ИИ подставил описаний: %d (плюс структурных: %d)", aiCount, len(matches)-aiCount)
		}

		list := kvs
		if len(list) > 600 {
			list = list[:600]
		}
		job.finish(map[string]interface{}{
			"url":        doc.URL,
			"fromCache":  doc.FromCache,
			"fetchedAt":  doc.FetchedAt.Format("02.01.2006 15:04"),
			"parts":      doc.Parts,
			"kvCount":    len(kvs),
			"matches":    matches,
			"matchCount": len(matches),
			"aiCount":    aiCount,
			"fieldCount": len(body.Fields),
			"list":       list,
			"warnings":   warnings,
		}, nil)
	}()
	writeJSON(w, map[string]string{"jobId": jobID})
}

// POST /api/guide/ask {url, question}
func handleGuideAsk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, 405, "метод не поддерживается")
		return
	}
	var body struct {
		URL      string `json:"url"`
		Question string `json:"question"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	cfg, ready := aiConfig()
	if !ready {
		jsonError(w, 422, "вопросы по гайду работают через ИИ — нажмите кнопку 🤖 вверху и настройте ИИ-помощника")
		return
	}
	if strings.TrimSpace(body.URL) == "" || strings.TrimSpace(body.Question) == "" {
		jsonError(w, 400, "нужны ссылка на гайд и текст вопроса")
		return
	}
	jobID, job := newJob("гайд-вопрос")
	go func() {
		defer job.guard()
		doc, err := guide.Fetch(body.URL, guidesCacheDir(), false, job.logf)
		if err != nil {
			job.finish(nil, err)
			return
		}
		ans, err := ai.Ask(cfg, doc.Text, body.Question, job.logf)
		if err != nil {
			job.finish(nil, err)
			return
		}
		job.finish(map[string]string{"answer": strings.TrimSpace(ans)}, nil)
	}()
	writeJSON(w, map[string]string{"jobId": jobID})
}

// GET /api/ai/presets — пресеты провайдеров (для настроек).
func handleAiPresets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, ai.Presets)
}

// POST /api/ai/test {baseURL, apiKey, model} — проверка подключения и списка моделей.
func handleAiTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, 405, "метод не поддерживается")
		return
	}
	var body struct {
		BaseURL string `json:"baseURL"`
		APIKey  string `json:"apiKey"`
		Model   string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	cfg := ai.Config{BaseURL: strings.TrimSpace(body.BaseURL), APIKey: strings.TrimSpace(body.APIKey), Model: strings.TrimSpace(body.Model)}
	glogf("ИИ", "Проверка подключения: %s", cfg.BaseURL)
	models, err := ai.ListModels(cfg)
	if err != nil {
		// часть серверов не отдаёт /models — пробуем крошечный запрос
		if cfg.Model != "" {
			if _, cerr := ai.Chat(cfg, "Отвечай одним словом.", "Скажи: ок", 10, nil); cerr == nil {
				glogf("ИИ", "Подключение работает (модель %s отвечает)", cfg.Model)
				writeJSON(w, map[string]interface{}{"ok": true, "message": "Подключение работает, модель отвечает."})
				return
			}
		}
		glogf("ИИ", "Проверка не удалась: %v", err)
		jsonError(w, 422, err.Error())
		return
	}
	hasModel := false
	for _, m := range models {
		if strings.EqualFold(m, cfg.Model) {
			hasModel = true
			break
		}
	}
	msg := "Подключение работает."
	if cfg.Model != "" && !hasModel {
		msg = "Подключение работает, но модели «" + cfg.Model + "» нет в списке — выберите из выпадающего списка."
	}
	glogf("ИИ", "Подключение работает, моделей доступно: %d", len(models))
	if len(models) > 300 {
		models = models[:300]
	}
	writeJSON(w, map[string]interface{}{"ok": true, "message": msg, "models": models, "modelListed": hasModel})
}
