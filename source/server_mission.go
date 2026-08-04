package main

// Вкладка «Миссия»: редактор миссионных файлов со встроенным гайдом.

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"dayzoverride/mission"
)

type msnSession struct {
	created  time.Time
	kind     string
	fileName string
	orig     interface{}
}

var msnSessions = struct {
	mu sync.Mutex
	m  map[string]*msnSession
}{m: map[string]*msnSession{}}

func handleMissionOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, 405, "метод не поддерживается")
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		jsonError(w, 400, err.Error())
		return
	}
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
	data, err := io.ReadAll(io.LimitReader(src, 128<<20))
	src.Close()
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	model, orig, err := mission.Open(fhs[0].Filename, data)
	if err != nil {
		jsonError(w, 422, err.Error())
		return
	}
	tok := randToken()
	msnSessions.mu.Lock()
	for k, v := range msnSessions.m {
		if time.Since(v.created) > 4*time.Hour {
			delete(msnSessions.m, k)
		}
	}
	msnSessions.m[tok] = &msnSession{created: time.Now(), kind: model.Kind, fileName: model.FileName, orig: orig}
	msnSessions.mu.Unlock()
	glogf("миссия", "Открыт %s (%s): групп %d, событий %d", model.FileName, model.Kind, len(model.Groups), len(model.Events))
	writeJSON(w, map[string]interface{}{"token": tok, "model": model})
}

func handleMissionSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, 405, "метод не поддерживается")
		return
	}
	var body struct {
		Token  string             `json:"token"`
		Values map[string]string  `json:"values"`
		Events []mission.EventRow `json:"events"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	msnSessions.mu.Lock()
	s := msnSessions.m[body.Token]
	msnSessions.mu.Unlock()
	if s == nil {
		jsonError(w, 404, "сессия устарела — откройте файл заново")
		return
	}
	out, err := mission.Apply(s.kind, s.orig, body.Values, body.Events)
	if err != nil {
		jsonError(w, 422, err.Error())
		return
	}
	glogf("миссия", "Сформирован изменённый %s (%.1f КБ)", s.fileName, float64(len(out))/1024)
	writeJSON(w, map[string]string{"fileName": s.fileName, "content": string(out)})
}
