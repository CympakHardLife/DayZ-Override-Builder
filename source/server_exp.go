package main

// Вкладки «Exp. Квесты» и «Exp. Лодауты» (DayZ Expansion).

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"dayzoverride/expansion"
	"dayzoverride/typesx"
)

// unzipTo распаковывает zip в dir с защитой от zip-slip.
func unzipTo(data []byte, dir string) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("не удалось открыть ZIP: %v", err)
	}
	if len(zr.File) > 20000 {
		return fmt.Errorf("слишком много файлов в архиве")
	}
	var total int64
	for _, f := range zr.File {
		name := filepath.FromSlash(strings.ReplaceAll(f.Name, "\\", "/"))
		if strings.Contains(name, "..") || filepath.IsAbs(name) {
			continue
		}
		dst := filepath.Join(dir, name)
		if f.FileInfo().IsDir() {
			os.MkdirAll(dst, 0755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(dst)
		if err != nil {
			rc.Close()
			return err
		}
		n, err := io.Copy(out, io.LimitReader(rc, 256<<20))
		out.Close()
		rc.Close()
		if err != nil {
			return err
		}
		total += n
		if total > 1<<30 {
			return fmt.Errorf("архив слишком большой в распакованном виде")
		}
	}
	return nil
}

// packInput принимает zip-файл (поле fileField) или путь (поле pathField)
// и возвращает корневую папку с данными.
func packInput(r *http.Request, fileField, pathField, tmpBase string) (string, error) {
	if r.MultipartForm != nil {
		if fhs := r.MultipartForm.File[fileField]; len(fhs) > 0 {
			src, err := fhs[0].Open()
			if err != nil {
				return "", err
			}
			data, err := io.ReadAll(io.LimitReader(src, 512<<20))
			src.Close()
			if err != nil {
				return "", err
			}
			dir := filepath.Join(tmpBase, fileField)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return "", err
			}
			if err := unzipTo(data, dir); err != nil {
				return "", fmt.Errorf("%s: %v", fhs[0].Filename, err)
			}
			return dir, nil
		}
	}
	p := strings.TrimSpace(strings.Trim(r.FormValue(pathField), `"`))
	if p == "" {
		return "", nil
	}
	st, err := os.Stat(p)
	if err != nil || !st.IsDir() {
		return "", fmt.Errorf("папка не найдена: %s", p)
	}
	return p, nil
}

// typesClassesFromUpload читает опциональный types.xml и возвращает множество класснеймов.
func typesClassesFromUpload(r *http.Request, field string) (map[string]bool, error) {
	if r.MultipartForm == nil || len(r.MultipartForm.File[field]) == 0 {
		return nil, nil
	}
	src, err := r.MultipartForm.File[field][0].Open()
	if err != nil {
		return nil, err
	}
	defer src.Close()
	data, err := io.ReadAll(io.LimitReader(src, 128<<20))
	if err != nil {
		return nil, err
	}
	tf, err := typesx.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("types.xml: %v", err)
	}
	out := map[string]bool{}
	for _, t := range tf.Types {
		out[strings.ToLower(t.Name)] = true
	}
	return out, nil
}

// ---------- Expansion Quests ----------

func handleExpqCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, 405, "метод не поддерживается")
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	tmp, err := os.MkdirTemp("", "dob-expq-*")
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	cleanup := func() { os.RemoveAll(tmp) }
	packDir, err := packInput(r, "pack", "packPath", tmp)
	if err != nil || packDir == "" {
		cleanup()
		if err == nil {
			err = fmt.Errorf("прикрепите ZIP папки квестов или укажите путь")
		}
		jsonError(w, 400, err.Error())
		return
	}
	loadDir, err := packInput(r, "loadouts", "loadoutsPath", tmp)
	if err != nil {
		cleanup()
		jsonError(w, 400, "лодауты: "+err.Error())
		return
	}
	known, err := typesClassesFromUpload(r, "typesFile")
	if err != nil {
		cleanup()
		jsonError(w, 400, err.Error())
		return
	}
	jobID, job := newJob("exp-квесты")
	go func() {
		defer cleanup()
		defer job.guard()
		pack, err := expansion.LoadQuestPack(packDir, job.logf)
		if err != nil {
			job.finish(nil, err)
			return
		}
		var loadoutNames map[string]bool
		if loadDir != "" {
			loadoutNames = expansion.LoadoutNamesFromDir(loadDir)
			job.logf("Лодаутов для сверки: %d", len(loadoutNames))
		}
		if known != nil {
			job.logf("Класснеймов в types.xml: %d", len(known))
		}
		res := expansion.Check(pack, loadoutNames, known, job.logf)
		job.finish(res, nil)
	}()
	writeJSON(w, map[string]string{"jobId": jobID})
}

func handleExpqMerge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, 405, "метод не поддерживается")
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	tmp, err := os.MkdirTemp("", "dob-expqm-*")
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	cleanup := func() { os.RemoveAll(tmp) }
	aDir, err := packInput(r, "packA", "packAPath", tmp)
	if err != nil || aDir == "" {
		cleanup()
		if err == nil {
			err = fmt.Errorf("укажите ВАШ пак квестов (ZIP или путь)")
		}
		jsonError(w, 400, "пак А: "+fmt.Sprint(err))
		return
	}
	bDir, err := packInput(r, "packB", "packBPath", tmp)
	if err != nil || bDir == "" {
		cleanup()
		if err == nil {
			err = fmt.Errorf("укажите НОВЫЙ пак (ZIP или путь)")
		}
		jsonError(w, 400, "пак Б: "+fmt.Sprint(err))
		return
	}
	outputPath := strings.TrimSpace(r.FormValue("outputPath"))
	jobID, job := newJob("exp-квесты-слияние")
	go func() {
		defer cleanup()
		defer job.guard()
		job.logf("Загружаю ВАШ пак…")
		a, err := expansion.LoadQuestPack(aDir, job.logf)
		if err != nil {
			job.finish(nil, fmt.Errorf("ваш пак: %v", err))
			return
		}
		job.logf("Загружаю НОВЫЙ пак…")
		b, err := expansion.LoadQuestPack(bDir, job.logf)
		if err != nil {
			job.finish(nil, fmt.Errorf("новый пак: %v", err))
			return
		}
		res, err := expansion.MergeQuestPacks(a, b, job.logf)
		if err != nil {
			job.finish(nil, err)
			return
		}
		finishMergeJob(job, res, "Quests_merged", outputPath)
	}()
	writeJSON(w, map[string]string{"jobId": jobID})
}

// finishMergeJob упаковывает результат слияния в ZIP (+ сохранение в папку).
func finishMergeJob(job *jobRec, res *expansion.MergeResult, zipName, outputPath string) {
	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	rw, _ := zw.Create("ОТЧЁТ.txt")
	rw.Write([]byte(res.Report))
	for _, f := range res.Files {
		w, err := zw.Create(f.Path)
		if err != nil {
			job.finish(nil, err)
			return
		}
		if _, err := w.Write(f.Data); err != nil {
			job.finish(nil, err)
			return
		}
	}
	if err := zw.Close(); err != nil {
		job.finish(nil, err)
		return
	}
	tok := zips.put(zipName+".zip", zbuf.Bytes())
	resp := map[string]interface{}{
		"files":    res.FileList,
		"renames":  res.Renames,
		"stats":    res.Stats,
		"report":   res.Report,
		"zipToken": tok,
	}
	if outputPath != "" {
		job.logf("Сохраняю в папку: %s", outputPath)
		ok := true
		for _, f := range res.Files {
			full := filepath.Join(outputPath, filepath.FromSlash(f.Path))
			if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
				resp["saveError"] = err.Error()
				ok = false
				break
			}
			if err := os.WriteFile(full, f.Data, 0644); err != nil {
				resp["saveError"] = err.Error()
				ok = false
				break
			}
		}
		if ok {
			os.WriteFile(filepath.Join(outputPath, "ОТЧЁТ.txt"), []byte(res.Report), 0644)
			resp["savedTo"] = outputPath
			job.logf("Сохранено: %s", outputPath)
		}
	}
	job.logf("Слияние завершено: %d файлов", len(res.Files))
	job.finish(resp, nil)
}

// ---------- Expansion Loadouts ----------

func handleExplCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, 405, "метод не поддерживается")
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	tmp, err := os.MkdirTemp("", "dob-expl-*")
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	cleanup := func() { os.RemoveAll(tmp) }
	dir, err := packInput(r, "pack", "packPath", tmp)
	if err != nil || dir == "" {
		cleanup()
		if err == nil {
			err = fmt.Errorf("прикрепите ZIP папки лодаутов или укажите путь")
		}
		jsonError(w, 400, err.Error())
		return
	}
	known, err := typesClassesFromUpload(r, "typesFile")
	if err != nil {
		cleanup()
		jsonError(w, 400, err.Error())
		return
	}
	jobID, job := newJob("exp-лодауты")
	go func() {
		defer cleanup()
		defer job.guard()
		files, err := expansion.LoadLoadouts(dir, job.logf)
		if err != nil {
			job.finish(nil, err)
			return
		}
		res := expansion.CheckLoadouts(files, known, job.logf)
		job.finish(res, nil)
	}()
	writeJSON(w, map[string]string{"jobId": jobID})
}

func handleExplMerge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, 405, "метод не поддерживается")
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	tmp, err := os.MkdirTemp("", "dob-explm-*")
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	cleanup := func() { os.RemoveAll(tmp) }
	aDir, err := packInput(r, "packA", "packAPath", tmp)
	if err != nil || aDir == "" {
		cleanup()
		if err == nil {
			err = fmt.Errorf("укажите ВАШУ папку лодаутов")
		}
		jsonError(w, 400, "лодауты А: "+fmt.Sprint(err))
		return
	}
	bDir, err := packInput(r, "packB", "packBPath", tmp)
	if err != nil || bDir == "" {
		cleanup()
		if err == nil {
			err = fmt.Errorf("укажите папку НОВЫХ лодаутов")
		}
		jsonError(w, 400, "новые лодауты: "+fmt.Sprint(err))
		return
	}
	outputPath := strings.TrimSpace(r.FormValue("outputPath"))
	jobID, job := newJob("exp-лодауты-слияние")
	go func() {
		defer cleanup()
		defer job.guard()
		a, err := expansion.LoadLoadouts(aDir, job.logf)
		if err != nil {
			job.finish(nil, fmt.Errorf("ваши лодауты: %v", err))
			return
		}
		b, err := expansion.LoadLoadouts(bDir, job.logf)
		if err != nil {
			job.finish(nil, fmt.Errorf("новые лодауты: %v", err))
			return
		}
		res, err := expansion.MergeLoadouts(a, b, job.logf)
		if err != nil {
			job.finish(nil, err)
			return
		}
		finishMergeJob(job, res, "Loadouts_merged", outputPath)
	}()
	writeJSON(w, map[string]string{"jobId": jobID})
}
