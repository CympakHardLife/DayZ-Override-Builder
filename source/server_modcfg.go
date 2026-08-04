package main

// Вкладки «Inedia AI» и «Terje»: настройка конфигов модов с подсказками.

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"dayzoverride/mission"
	"dayzoverride/modcfg"
)

type modcfgSession struct {
	created  time.Time
	kind     string // inedia | terje
	fileName string
	origJSON interface{}
	origTrj  *modcfg.TerjeFile
	zipFiles map[string][]byte // если загружен zip
}

var modcfgSessions = struct {
	mu sync.Mutex
	m  map[string]*modcfgSession
}{m: map[string]*modcfgSession{}}

func modcfgEditable(name string) bool {
	low := strings.ToLower(name)
	return strings.HasSuffix(low, ".cfg") || strings.HasSuffix(low, ".json")
}

// handleModcfgOpen: multipart file (+ mode=inedia|terje, inner=имя файла в zip, token=сессия zip)
func handleModcfgOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, 405, "метод не поддерживается")
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	mode := r.FormValue("mode")
	inner := r.FormValue("inner")
	token := r.FormValue("token")

	var fileName string
	var data []byte
	var sess *modcfgSession

	if token != "" && inner != "" {
		// открытие файла из ранее загруженного zip
		modcfgSessions.mu.Lock()
		sess = modcfgSessions.m[token]
		modcfgSessions.mu.Unlock()
		if sess == nil {
			jsonError(w, 404, "сессия устарела — загрузите архив заново")
			return
		}
		d, ok := sess.zipFiles[inner]
		if !ok {
			jsonError(w, 404, "файл не найден в архиве")
			return
		}
		fileName = filepath.Base(inner)
		data = d
	} else {
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
		data, err = io.ReadAll(io.LimitReader(src, 128<<20))
		src.Close()
		if err != nil {
			jsonError(w, 500, err.Error())
			return
		}
		fileName = filepath.Base(fhs[0].Filename)
		// ZIP → список файлов
		if strings.HasSuffix(strings.ToLower(fileName), ".zip") {
			zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
			if err != nil {
				jsonError(w, 422, "не удалось открыть ZIP: "+err.Error())
				return
			}
			s := &modcfgSession{created: time.Now(), kind: mode, zipFiles: map[string][]byte{}}
			var names []string
			for _, f := range zr.File {
				if f.FileInfo().IsDir() || !modcfgEditable(f.Name) {
					continue
				}
				rc, err := f.Open()
				if err != nil {
					continue
				}
				d, _ := io.ReadAll(io.LimitReader(rc, 64<<20))
				rc.Close()
				name := strings.ReplaceAll(f.Name, "\\", "/")
				s.zipFiles[name] = d
				names = append(names, name)
			}
			if len(names) == 0 {
				jsonError(w, 422, "в архиве нет .cfg/.json файлов")
				return
			}
			sort.Strings(names)
			tok := randToken()
			modcfgSessions.mu.Lock()
			for k, v := range modcfgSessions.m {
				if time.Since(v.created) > 4*time.Hour {
					delete(modcfgSessions.m, k)
				}
			}
			modcfgSessions.m[tok] = s
			modcfgSessions.mu.Unlock()
			writeJSON(w, map[string]interface{}{"picker": names, "token": tok})
			return
		}
	}

	// определяем формат и строим модель
	var (
		model interface{}
		err   error
	)
	newSess := &modcfgSession{created: time.Now(), fileName: fileName}
	if sess != nil {
		newSess.zipFiles = sess.zipFiles
	}
	low := strings.ToLower(fileName)
	switch {
	case modcfg.IsTerjeCfg(data) || (mode == "terje" && strings.HasSuffix(low, ".cfg")):
		m, tf, e := modcfg.OpenTerje(fileName, data)
		model, err = m, e
		newSess.kind = "terje"
		newSess.origTrj = tf
	case strings.HasSuffix(low, ".json"):
		m, orig, e := modcfg.OpenInedia(fileName, data)
		model, err = m, e
		newSess.kind = "inedia"
		newSess.origJSON = orig
	default:
		jsonError(w, 422, fmt.Sprintf("не узнаю формат файла %q: ожидается Terje .cfg (строки с // [type: ...]) или .json", fileName))
		return
	}
	if err != nil {
		jsonError(w, 422, err.Error())
		return
	}
	tok := randToken()
	modcfgSessions.mu.Lock()
	modcfgSessions.m[tok] = newSess
	modcfgSessions.mu.Unlock()
	glogf("мод-конфиг", "Открыт %s (%s)", fileName, newSess.kind)
	writeJSON(w, map[string]interface{}{"token": tok, "model": model})
}

func handleModcfgSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, 405, "метод не поддерживается")
		return
	}
	var body struct {
		Token  string            `json:"token"`
		Values map[string]string `json:"values"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	modcfgSessions.mu.Lock()
	s := modcfgSessions.m[body.Token]
	modcfgSessions.mu.Unlock()
	if s == nil {
		jsonError(w, 404, "сессия устарела — откройте файл заново")
		return
	}
	var out []byte
	switch s.kind {
	case "terje":
		out = modcfg.ApplyTerje(s.origTrj, body.Values)
	case "inedia":
		out = mission.ApplyJSONValues(s.origJSON, body.Values)
	default:
		jsonError(w, 422, "неизвестный тип сессии")
		return
	}
	glogf("мод-конфиг", "Сформирован изменённый %s (%.1f КБ)", s.fileName, float64(len(out))/1024)
	writeJSON(w, map[string]string{"fileName": s.fileName, "content": string(out)})
}
