package main

import (
	"io"
	"net/http"

	"dayzoverride/validator"
)

func handleValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, 405, "метод не поддерживается")
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		jsonError(w, 400, "ошибка загрузки файлов: "+err.Error())
		return
	}
	fhs := r.MultipartForm.File["files"]
	if len(fhs) == 0 {
		jsonError(w, 400, "файлы не переданы")
		return
	}
	if len(fhs) > 200 {
		jsonError(w, 400, "слишком много файлов за раз (максимум 200)")
		return
	}
	// формат выбирается пользователем на вкладке (кнопки «types / config / …»);
	// пусто = определить автоматически. Читаем до старта горутины.
	format := r.FormValue("format")
	jobID, job := newJob("проверка-файлов")
	// файлы читаем до ответа (тело запроса живёт только в хендлере)
	type inFile struct {
		name string
		data []byte
	}
	var files []inFile
	for _, fh := range fhs {
		src, err := fh.Open()
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
		files = append(files, inFile{name: fh.Filename, data: data})
	}
	go func() {
		defer job.guard()
		var reports []*validator.FileReport
		okCount := 0
		for _, f := range files {
			job.logf("Проверяю %s (%.1f КБ)…", f.name, float64(len(f.data))/1024)
			rep := validator.ValidateAs(f.name, f.data, format)
			if rep.OK {
				okCount++
				kind := rep.DayZKind
				if kind == "" {
					kind = rep.Kind
				}
				job.logf("  %s: OK (%s), замечаний: %d", f.name, kind, len(rep.Problems))
			} else {
				job.logf("  %s: НАЙДЕНЫ ОШИБКИ (%d)", f.name, len(rep.Problems))
			}
			reports = append(reports, rep)
		}
		job.logf("Проверка завершена: файлов %d, без ошибок %d", len(reports), okCount)
		job.finish(map[string]interface{}{"reports": reports, "ok": okCount, "total": len(reports)}, nil)
	}()
	writeJSON(w, map[string]string{"jobId": jobID})
}
