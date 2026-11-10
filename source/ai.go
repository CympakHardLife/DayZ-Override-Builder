// Package ai — универсальный клиент OpenAI-совместимых API (Groq, OpenRouter,
// Google Gemini, GitHub Models, локальный Ollama и любые другие).
// Используется опционально: для разбора гайдов со свободным текстом,
// перевода описаний на русский и ответов на вопросы по гайду.
package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

type Config struct {
	BaseURL string
	APIKey  string
	Model   string
}

func (c Config) host() string {
	if u, err := url.Parse(c.BaseURL); err == nil && u.Hostname() != "" {
		return u.Hostname()
	}
	return c.BaseURL
}

func (c Config) endpoint(path string) string {
	return strings.TrimRight(strings.TrimSpace(c.BaseURL), "/") + path
}

var httpClient = &http.Client{Timeout: 180 * time.Second}

// ---------- пресеты бесплатных провайдеров ----------

type Preset struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	BaseURL string `json:"baseURL"`
	Model   string `json:"model"`
	Hint    string `json:"hint"`
}

var Presets = []Preset{
	{ID: "groq", Name: "Groq — быстрый, до 1000+ запросов/день бесплатно",
		BaseURL: "https://api.groq.com/openai/v1", Model: "llama-3.3-70b-versatile",
		Hint: "Ключ: console.groq.com/keys (регистрация, карта не нужна)."},
	{ID: "openrouter", Name: "OpenRouter — модели с пометкой :free",
		BaseURL: "https://openrouter.ai/api/v1", Model: "deepseek/deepseek-chat-v3.1:free",
		Hint: "Ключ: openrouter.ai/settings/keys. Бесплатно ~50 запросов/день. Выбирайте модели с «:free»."},
	{ID: "gemini", Name: "Google Gemini (AI Studio) — щедрый бесплатный лимит",
		BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai", Model: "gemini-2.5-flash",
		Hint: "Ключ: aistudio.google.com/apikey (Google-аккаунт). Из некоторых стран API недоступен напрямую."},
	{ID: "github", Name: "GitHub Models — по GitHub-аккаунту",
		BaseURL: "https://models.github.ai/inference", Model: "openai/gpt-4o-mini",
		Hint: "Ключ: github.com/settings/tokens → Fine-grained token с разрешением «Models: read»."},
	{ID: "ollama", Name: "Локальный Ollama — без интернета и ключей",
		BaseURL: "http://localhost:11434/v1", Model: "llama3.1",
		Hint: "Нужен установленный Ollama (ollama.com) и скачанная модель. Ключ не требуется."},
	{ID: "custom", Name: "Свой сервер (OpenAI-совместимый API)",
		BaseURL: "", Model: "",
		Hint: "Любой сервис с OpenAI-совместимым API: адрес, ключ и имя модели."},
}

// ---------- низкий уровень: chat completions ----------

type chatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatReq struct {
	Model       string    `json:"model"`
	Messages    []chatMsg `json:"messages"`
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
}

func httpErrToHuman(cfg Config, status int, body []byte) error {
	snippet := strings.TrimSpace(string(body))
	if len(snippet) > 300 {
		snippet = snippet[:300] + "…"
	}
	switch status {
	case 401:
		return fmt.Errorf("нейросеть отклонила API-ключ (401) — проверьте ключ в настройках 🤖")
	case 402:
		return fmt.Errorf("провайдер требует оплату/пополнение (402) — выберите бесплатную модель или другого провайдера. Ответ: %s", snippet)
	case 403:
		return fmt.Errorf("доступ запрещён (403) — возможно, модель недоступна на бесплатном тарифе или регион не поддерживается. Ответ: %s", snippet)
	case 404:
		return fmt.Errorf("модель %q не найдена (404) — нажмите «Список моделей» в настройках 🤖 и выберите существующую", cfg.Model)
	case 429:
		return fmt.Errorf("превышен лимит бесплатного тарифа (429) — подождите минуту-другую или смените модель/провайдера")
	default:
		return fmt.Errorf("нейросеть ответила ошибкой %d: %s", status, snippet)
	}
}

// Chat выполняет один запрос к нейросети и возвращает текст ответа.
func Chat(cfg Config, system, user string, maxTokens int, logf Logf) (string, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.Model) == "" {
		return "", fmt.Errorf("ИИ не настроен: укажите адрес API и модель в настройках 🤖")
	}
	body, _ := json.Marshal(chatReq{
		Model:       cfg.Model,
		Temperature: 0.2,
		MaxTokens:   maxTokens,
		Messages: []chatMsg{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
	})
	req, err := http.NewRequest("POST", cfg.endpoint("/chat/completions"), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(cfg.APIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.APIKey))
	}
	// вежливые заголовки для OpenRouter (обязательны для части бесплатных моделей)
	req.Header.Set("HTTP-Referer", "https://localhost/dayz-override-builder")
	req.Header.Set("X-Title", "DayZ Override Builder")

	start := time.Now()
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("нет доступа к %s — проверьте интернет/файрвол (%v)", cfg.host(), err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 400 {
		return "", httpErrToHuman(cfg, resp.StatusCode, data)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &out); err != nil || len(out.Choices) == 0 {
		return "", fmt.Errorf("нейросеть вернула неожиданный ответ (не по протоколу OpenAI) — проверьте адрес API")
	}
	logf.p("Ответ ИИ получен за %.1f с (израсходовано токенов: %d)", time.Since(start).Seconds(), out.Usage.TotalTokens)
	return out.Choices[0].Message.Content, nil
}

// ListModels запрашивает у провайдера список доступных моделей.
func ListModels(cfg Config) ([]string, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, fmt.Errorf("укажите адрес API")
	}
	req, err := http.NewRequest("GET", cfg.endpoint("/models"), nil)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.APIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.APIKey))
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("нет доступа к %s — проверьте интернет/файрвол (%v)", cfg.host(), err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 400 {
		return nil, httpErrToHuman(cfg, resp.StatusCode, data)
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &out); err != nil || len(out.Data) == 0 {
		return nil, fmt.Errorf("провайдер не вернул список моделей — впишите имя модели вручную")
	}
	var ids []string
	for _, m := range out.Data {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	// бесплатные модели (":free" у OpenRouter) — в начало списка
	sort.SliceStable(ids, func(i, j int) bool {
		fi, fj := strings.HasSuffix(ids[i], ":free"), strings.HasSuffix(ids[j], ":free")
		if fi != fj {
			return fi
		}
		return strings.ToLower(ids[i]) < strings.ToLower(ids[j])
	})
	return ids, nil
}

// ---------- выжимка гайда под лимиты бесплатных тарифов ----------

// Excerpt отбирает из текста абзацы, где встречаются искомые термины,
// пока не наберётся budget символов. Если текст короче бюджета — целиком.
func Excerpt(text string, terms []string, budget int) string {
	if len(text) <= budget {
		return text
	}
	tset := map[string]bool{}
	for _, t := range terms {
		t = strings.ToLower(strings.TrimSpace(t))
		if len(t) > 3 {
			tset[t] = true
		}
	}
	paras := strings.Split(text, "\n\n")
	type scored struct {
		idx, score int
	}
	var sc []scored
	for i, p := range paras {
		low := strings.ToLower(p)
		n := 0
		for t := range tset {
			if strings.Contains(low, t) {
				n++
			}
		}
		if n > 0 {
			sc = append(sc, scored{i, n})
		}
	}
	if len(sc) == 0 {
		if budget > len(text) {
			budget = len(text)
		}
		return text[:budget]
	}
	sort.SliceStable(sc, func(a, b int) bool { return sc[a].score > sc[b].score })
	take := map[int]bool{}
	total := 0
	for _, s := range sc {
		l := len(paras[s.idx])
		if total+l > budget {
			continue
		}
		take[s.idx] = true
		total += l
	}
	var sb strings.Builder
	for i, p := range paras {
		if take[i] {
			sb.WriteString(p)
			sb.WriteString("\n\n")
		}
	}
	return sb.String()
}

// ---------- разбор гайда нейросетью: параметр → описание на русском ----------

const descSystem = "Ты эксперт по настройке серверов DayZ и модов к ним. Отвечаешь ТОЛЬКО валидным JSON без пояснений, без markdown."

// DescribeParams просит нейросеть описать параметры по тексту гайда.
// Работает батчами, чтобы влезать в лимиты бесплатных тарифов.
func DescribeParams(cfg Config, guideText string, params []string, logf Logf) (map[string]string, error) {
	const batchSize = 80
	const excerptBudget = 15000
	out := map[string]string{}
	var firstErr error
	batches := (len(params) + batchSize - 1) / batchSize
	for b := 0; b < batches; b++ {
		lo, hi := b*batchSize, (b+1)*batchSize
		if hi > len(params) {
			hi = len(params)
		}
		batch := params[lo:hi]
		var terms []string
		for _, p := range batch {
			terms = append(terms, p)
			if i := strings.LastIndex(p, "."); i >= 0 {
				terms = append(terms, p[i+1:])
			}
		}
		ex := Excerpt(guideText, terms, excerptBudget)
		logf.p("Запрос к ИИ %d/%d: параметров %d, фрагмент гайда %d КБ, модель %s…", b+1, batches, len(batch), len(ex)/1024, cfg.Model)
		user := "Ниже фрагмент гайда по настройке мода DayZ и список параметров конфига.\n" +
			"Для КАЖДОГО параметра, который объясняется в гайде, дай краткое описание НА РУССКОМ ЯЗЫКЕ (до 200 символов): что делает и в каких единицах/пределах. Переведи, если гайд не на русском.\n" +
			"Параметры, которых нет в гайде, ПРОПУСТИ. Не выдумывай.\n" +
			"Ответ: СТРОГО JSON-массив вида [{\"p\":\"Имя.Параметра\",\"d\":\"описание\"}] без других слов.\n\n" +
			"ПАРАМЕТРЫ:\n" + strings.Join(batch, "\n") + "\n\nГАЙД:\n" + ex
		resp, err := Chat(cfg, descSystem, user, 4000, logf)
		if err != nil {
			logf.p("ИИ-запрос %d/%d не удался: %v", b+1, batches, err)
			if firstErr == nil {
				firstErr = err
			}
			break // при лимитах нет смысла добивать остальные батчи
		}
		pairs, err := parsePairs(resp)
		if err != nil {
			logf.p("Не удалось разобрать ответ ИИ (%v) — начало ответа: %.120s", err, strings.TrimSpace(resp))
			if firstErr == nil {
				firstErr = fmt.Errorf("нейросеть ответила не в том формате — попробуйте другую модель")
			}
			continue
		}
		valid := map[string]bool{}
		for _, p := range batch {
			valid[strings.ToLower(p)] = true
		}
		got := 0
		for _, kv := range pairs {
			if kv.D == "" || kv.P == "" {
				continue
			}
			if valid[strings.ToLower(kv.P)] {
				out[kv.P] = clip(kv.D, 300)
				got++
			}
		}
		logf.p("ИИ описал параметров: %d из %d в этой части", got, len(batch))
	}
	if len(out) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return out, nil
}

type pair struct {
	P string `json:"p"`
	D string `json:"d"`
}

var reTrailingComma = regexp.MustCompile(`,\s*([}\]])`)

func parsePairs(s string) ([]pair, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	i := strings.Index(s, "[")
	j := strings.LastIndex(s, "]")
	if i < 0 || j <= i {
		return nil, fmt.Errorf("в ответе нет JSON-массива")
	}
	body := s[i : j+1]
	var out []pair
	if err := json.Unmarshal([]byte(body), &out); err == nil {
		return out, nil
	}
	body = reTrailingComma.ReplaceAllString(body, "$1")
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// ---------- вопрос по гайду ----------

const askSystem = "Ты помощник администратора сервера DayZ. Отвечай кратко и по делу НА РУССКОМ ЯЗЫКЕ, опираясь ТОЛЬКО на присланный фрагмент гайда. Указывай точные имена параметров и значения из гайда. Если в гайде ответа нет — так и скажи."

// Ask отвечает на вопрос по тексту гайда.
func Ask(cfg Config, guideText, question string, logf Logf) (string, error) {
	terms := strings.FieldsFunc(question, func(r rune) bool {
		return !('a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || '0' <= r && r <= '9' ||
			'а' <= r && r <= 'я' || 'А' <= r && r <= 'Я' || r == '_' || r == '.')
	})
	ex := Excerpt(guideText, terms, 18000)
	logf.p("Отправляю вопрос ИИ (фрагмент гайда %d КБ, модель %s)…", len(ex)/1024, cfg.Model)
	user := "ВОПРОС: " + strings.TrimSpace(question) + "\n\nФРАГМЕНТ ГАЙДА:\n" + ex
	return Chat(cfg, askSystem, user, 1500, logf)
}
