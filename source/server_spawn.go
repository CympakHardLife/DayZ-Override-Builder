package main

// Вкладка «Спавн-лут»: cfgspawnabletypes.xml + cfgrandompresets.xml.

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"dayzoverride/spawnloot"
	"dayzoverride/typesx"
)

func readUploadOpt(r *http.Request, field string) ([]byte, error) {
	if r.MultipartForm == nil || len(r.MultipartForm.File[field]) == 0 {
		return nil, nil
	}
	src, err := r.MultipartForm.File[field][0].Open()
	if err != nil {
		return nil, err
	}
	defer src.Close()
	return io.ReadAll(io.LimitReader(src, 256<<20))
}

func handleSpawnCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, 405, "метод не поддерживается")
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	spawnData, err := readUploadOpt(r, "spawnFile")
	if err != nil || spawnData == nil {
		jsonError(w, 400, "прикрепите cfgspawnabletypes.xml")
		return
	}
	presetData, err := readUploadOpt(r, "presetsFile")
	if err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	var known map[string]bool
	if td, err := readUploadOpt(r, "typesFile"); err == nil && td != nil {
		tf, terr := typesx.Parse(td)
		if terr != nil {
			jsonError(w, 400, "types.xml: "+terr.Error())
			return
		}
		known = map[string]bool{}
		for _, t := range tf.Types {
			known[strings.ToLower(t.Name)] = true
		}
	}
	jobID, job := newJob("спавн-лут")
	go func() {
		defer job.guard()
		job.logf("Разбираю cfgspawnabletypes.xml (%.1f КБ)…", float64(len(spawnData))/1024)
		spawn, err := spawnloot.ParseSpawn(spawnData)
		if err != nil {
			job.finish(nil, err)
			return
		}
		job.logf("Типов: %d", len(spawn.Types))
		var presets *spawnloot.PresetsFile
		if presetData != nil {
			job.logf("Разбираю cfgrandompresets.xml (%.1f КБ)…", float64(len(presetData))/1024)
			presets, err = spawnloot.ParsePresets(presetData)
			if err != nil {
				job.finish(nil, err)
				return
			}
			job.logf("Пресетов: cargo %d, attachments %d", len(presets.Cargo), len(presets.Attachments))
		} else {
			job.logf("cfgrandompresets.xml не приложен — ссылки на пресеты проверить не смогу")
		}
		if known != nil {
			job.logf("Класснеймов в types.xml: %d", len(known))
		}
		res := spawnloot.Check(spawn, presets, known, job.logf)
		job.finish(res, nil)
	}()
	writeJSON(w, map[string]string{"jobId": jobID})
}

func handleSpawnMerge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, 405, "метод не поддерживается")
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	aSpawn, _ := readUploadOpt(r, "spawnA")
	bSpawn, _ := readUploadOpt(r, "spawnB")
	if aSpawn == nil || bSpawn == nil {
		jsonError(w, 400, "нужны оба cfgspawnabletypes.xml: ваш и новый")
		return
	}
	aPre, _ := readUploadOpt(r, "presetsA")
	bPre, _ := readUploadOpt(r, "presetsB")
	preferFriend := r.FormValue("prefer") == "friend"
	outputPath := strings.TrimSpace(r.FormValue("outputPath"))

	jobID, job := newJob("спавн-лут-слияние")
	go func() {
		defer job.guard()
		as, err := spawnloot.ParseSpawn(aSpawn)
		if err != nil {
			job.finish(nil, fmt.Errorf("ваш файл: %v", err))
			return
		}
		bs, err := spawnloot.ParseSpawn(bSpawn)
		if err != nil {
			job.finish(nil, fmt.Errorf("новый файл: %v", err))
			return
		}
		job.logf("Ваших типов: %d, новых типов: %d", len(as.Types), len(bs.Types))
		var ap, bp *spawnloot.PresetsFile
		if aPre != nil {
			if ap, err = spawnloot.ParsePresets(aPre); err != nil {
				job.finish(nil, fmt.Errorf("ваши пресеты: %v", err))
				return
			}
		}
		if bPre != nil {
			if bp, err = spawnloot.ParsePresets(bPre); err != nil {
				job.finish(nil, fmt.Errorf("новые пресеты: %v", err))
				return
			}
		}
		res := spawnloot.Merge(as, bs, ap, bp, preferFriend, job.logf)

		var zbuf bytes.Buffer
		zw := zip.NewWriter(&zbuf)
		add := func(name string, data []byte) {
			w, _ := zw.Create(name)
			w.Write(data)
		}
		add("cfgspawnabletypes.xml", res.SpawnXML)
		if aPre != nil || bPre != nil {
			add("cfgrandompresets.xml", res.PresetsXML)
		}
		add("ОТЧЁТ.txt", []byte(res.Report))
		zw.Close()
		tok := zips.put("spawnloot_merged.zip", zbuf.Bytes())
		resp := map[string]interface{}{
			"stats":     res.Stats,
			"conflicts": res.Conflicts,
			"report":    res.Report,
			"zipToken":  tok,
		}
		if outputPath != "" {
			job.logf("Сохраняю в папку: %s", outputPath)
			if err := os.MkdirAll(outputPath, 0755); err != nil {
				resp["saveError"] = err.Error()
			} else {
				os.WriteFile(filepath.Join(outputPath, "cfgspawnabletypes.xml"), res.SpawnXML, 0644)
				if aPre != nil || bPre != nil {
					os.WriteFile(filepath.Join(outputPath, "cfgrandompresets.xml"), res.PresetsXML, 0644)
				}
				os.WriteFile(filepath.Join(outputPath, "ОТЧЁТ.txt"), []byte(res.Report), 0644)
				resp["savedTo"] = outputPath
				job.logf("Сохранено: %s", outputPath)
			}
		}
		job.finish(resp, nil)
	}()
	writeJSON(w, map[string]string{"jobId": jobID})
}
