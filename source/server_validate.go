package main

import (
	"errors"
	"io"
	"net/http"
	"path"
	"strings"

	"dayzoverride/validator"
)

func handleValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, 405, "метод не поддерживается")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 256<<20)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			jsonError(w, 413, "общий размер загрузки превышает 256 МБ")
			return
		}
		jsonError(w, 400, "ошибка загрузки файлов: "+err.Error())
		return
	}
	defer r.MultipartForm.RemoveAll()
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
	// файлы читаем до ответа (тело запроса живёт только в хендлере)
	paths := r.MultipartForm.Value["paths"]
	if len(paths) != 0 && len(paths) != len(fhs) {
		jsonError(w, 400, "список путей не соответствует списку файлов")
		return
	}
	var files []validator.InputFile
	for i, fh := range fhs {
		name := fh.Filename
		if len(paths) != 0 {
			name = paths[i]
		}
		var err error
		name, err = validationPath(name)
		if err != nil {
			jsonError(w, 400, err.Error())
			return
		}
		src, err := fh.Open()
		if err != nil {
			jsonError(w, 400, err.Error())
			return
		}
		data, err := io.ReadAll(io.LimitReader(src, (128<<20)+1))
		src.Close()
		if err != nil {
			jsonError(w, 500, err.Error())
			return
		}
		if len(data) > 128<<20 {
			jsonError(w, 413, "файл превышает 128 МБ")
			return
		}
		files = append(files, validator.InputFile{Name: name, Data: data})
	}
	jobID, job := newJob("проверка-файлов")
	go func() {
		defer job.guard()
		job.logf("Проверяю %d файлов и связи между ними…", len(files))
		reports := validator.ValidateSet(files, format)
		okCount := 0
		for _, rep := range reports {
			if rep.OK {
				okCount++
				kind := rep.DayZKind
				if kind == "" {
					kind = rep.Kind
				}
				job.logf("  %s: OK (%s), замечаний: %d", rep.Name, kind, len(rep.Problems))
			} else {
				job.logf("  %s: НАЙДЕНЫ ОШИБКИ (%d)", rep.Name, len(rep.Problems))
			}
		}
		job.logf("Проверка завершена: файлов %d, без ошибок %d", len(reports), okCount)
		job.finish(map[string]interface{}{"reports": reports, "ok": okCount, "total": len(reports)}, nil)
	}()
	writeJSON(w, map[string]string{"jobId": jobID})
}

func validationPath(name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	if path.IsAbs(name) || strings.ContainsAny(name, ":\x00") {
		return "", errors.New("ожидается относительный путь файла внутри миссии")
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return "", errors.New("путь файла не должен выходить за папку миссии")
		}
	}
	name = path.Clean(name)
	if name == "." {
		return "", errors.New("пустое имя файла")
	}
	return name, nil
}
