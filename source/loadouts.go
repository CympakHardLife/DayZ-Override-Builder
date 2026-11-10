package expansion

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ---------- лодауты Expansion (JSON) ----------

type LoadoutFile struct {
	Name     string   `json:"name"` // имя файла
	Problems []string `json:"problems"`
	Classes  []string `json:"classes"`
	raw      interface{}
	rawBytes []byte
}

type LoadoutCheckResult struct {
	Stats    map[string]int `json:"stats"`
	Files    []*LoadoutFile `json:"files"`
	Problems []Problem      `json:"problems"`
	Classes  []string       `json:"classNames"`
	Unknown  []string       `json:"unknownClasses,omitempty"`
}

// LoadLoadouts читает все *.json лодауты из папки (рекурсивно).
func LoadLoadouts(root string, logf Logf) ([]*LoadoutFile, error) {
	var out []*LoadoutFile
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(strings.ToLower(p), ".json") {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		lf := &LoadoutFile{Name: filepath.Base(p), rawBytes: data}
		var raw interface{}
		if err := json.Unmarshal(data, &raw); err != nil {
			lf.Problems = append(lf.Problems, "битый JSON: "+jsonErrLine(data, err))
		} else {
			lf.raw = raw
			validateLoadoutNode(raw, "", lf, true)
			collectClassNames(raw, &lf.Classes)
		}
		out = append(out, lf)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("в папке не найдено ни одного .json лодаута")
	}
	logf.p("Лодаутов загружено: %d", len(out))
	return out, nil
}

func numOf(v interface{}) (float64, bool) {
	f, ok := v.(float64)
	return f, ok
}

// validateLoadoutNode рекурсивно проверяет узел лодаута.
func validateLoadoutNode(v interface{}, path string, lf *LoadoutFile, isRoot bool) {
	m, ok := v.(map[string]interface{})
	if !ok {
		if arr, ok2 := v.([]interface{}); ok2 {
			for i, e := range arr {
				validateLoadoutNode(e, fmt.Sprintf("%s[%d]", path, i), lf, false)
			}
		}
		return
	}
	name := strOf(m["ClassName"])
	where := path
	if name != "" {
		where = path + "/" + name
	}
	if ch, ok := numOf(m["Chance"]); ok && (ch < 0 || ch > 1) {
		lf.Problems = append(lf.Problems, fmt.Sprintf("%s: Chance=%v вне диапазона 0..1", where, ch))
	}
	if q, ok := m["Quantity"].(map[string]interface{}); ok {
		qmin, okMin := numOf(q["Min"])
		qmax, okMax := numOf(q["Max"])
		if okMin && okMax && qmin > qmax {
			lf.Problems = append(lf.Problems, fmt.Sprintf("%s: Quantity Min (%v) больше Max (%v)", where, qmin, qmax))
		}
	}
	if hs, ok := m["Health"].([]interface{}); ok {
		for _, he := range hs {
			if hm, ok2 := he.(map[string]interface{}); ok2 {
				hmin, okMin := numOf(hm["Min"])
				hmax, okMax := numOf(hm["Max"])
				if okMin && okMax && hmin > hmax {
					lf.Problems = append(lf.Problems, fmt.Sprintf("%s: Health Min (%v) больше Max (%v)", where, hmin, hmax))
				}
			}
		}
	}
	// пустой ClassName допустим только у корня-обёртки или при Include/Sets
	if !isRoot && name == "" {
		hasInclude := strOf(m["Include"]) != ""
		sets, _ := m["Sets"].([]interface{})
		if _, isItemNode := m["Chance"]; isItemNode && !hasInclude && len(sets) == 0 {
			lf.Problems = append(lf.Problems, fmt.Sprintf("%s: пустой ClassName без Include/Sets — запись ничего не заспавнит", path))
		}
	}
	for _, key := range []string{"InventoryAttachments", "InventoryCargo", "Sets"} {
		if arr, ok := m[key].([]interface{}); ok {
			for i, e := range arr {
				validateLoadoutNode(e, fmt.Sprintf("%s/%s[%d]", where, key, i), lf, false)
			}
		}
	}
	if arr, ok := m["Items"].([]interface{}); ok {
		slot := strOf(m["SlotName"])
		for i, e := range arr {
			validateLoadoutNode(e, fmt.Sprintf("%s/%s[%d]", where, slot, i), lf, false)
		}
	}
}

// CheckLoadouts агрегирует проверку папки лодаутов.
func CheckLoadouts(files []*LoadoutFile, knownClasses map[string]bool, logf Logf) *LoadoutCheckResult {
	res := &LoadoutCheckResult{Stats: map[string]int{}, Files: files}
	classSet := map[string]bool{}
	nProb := 0
	for _, f := range files {
		sort.Strings(f.Classes)
		f.Classes = dedupe(f.Classes)
		for _, c := range f.Classes {
			classSet[c] = true
		}
		for _, p := range f.Problems {
			res.Problems = append(res.Problems, Problem{Level: "error", Msg: f.Name + ": " + p})
			nProb++
		}
	}
	for c := range classSet {
		res.Classes = append(res.Classes, c)
		if knownClasses != nil && !knownClasses[strings.ToLower(c)] {
			res.Unknown = append(res.Unknown, c)
		}
	}
	sort.Strings(res.Classes)
	sort.Strings(res.Unknown)
	res.Stats["files"] = len(files)
	res.Stats["problems"] = nProb
	res.Stats["classes"] = len(res.Classes)
	if knownClasses != nil {
		res.Stats["unknown"] = len(res.Unknown)
		if len(res.Unknown) > 0 {
			res.Problems = append(res.Problems, Problem{Level: "warn",
				Msg: fmt.Sprintf("класснеймов нет в приложенном types.xml: %d (одежда/аттачменты могут отсутствовать в types легально)", len(res.Unknown))})
		}
	}
	logf.p("Проверка лодаутов: файлов %d, проблем %d, класснеймов %d", len(files), nProb, len(res.Classes))
	return res
}

func dedupe(in []string) []string {
	var out []string
	var prev string
	for i, s := range in {
		if i == 0 || s != prev {
			out = append(out, s)
		}
		prev = s
	}
	return out
}

// MergeLoadouts объединяет две папки лодаутов: одинаковые имена с одинаковым
// содержимым пропускаются, при различии новый файл получает суффикс _v2.
func MergeLoadouts(aFiles, bFiles []*LoadoutFile, logf Logf) (*MergeResult, error) {
	res := &MergeResult{Stats: map[string]int{}}
	byName := map[string]*LoadoutFile{}
	for _, f := range aFiles {
		byName[strings.ToLower(f.Name)] = f
		res.Files = append(res.Files, OutFile{Path: "Loadouts/" + f.Name, Data: f.rawBytes})
	}
	var conflicts []string
	for _, f := range bFiles {
		key := strings.ToLower(f.Name)
		if af, ok := byName[key]; ok {
			if canonJSON(af.raw) == canonJSON(f.raw) && af.raw != nil {
				res.Skipped++
				continue
			}
			ext := filepath.Ext(f.Name)
			newName := strings.TrimSuffix(f.Name, ext) + "_v2" + ext
			n := 3
			for byName[strings.ToLower(newName)] != nil {
				newName = strings.TrimSuffix(f.Name, ext) + fmt.Sprintf("_v%d", n) + ext
				n++
			}
			conflicts = append(conflicts, fmt.Sprintf("%s: содержимое отличается — новый файл сохранён как %s (обновите NPCLoadoutFile у его NPC, если нужен именно новый вариант)", f.Name, newName))
			res.Renames = append(res.Renames, Rename{Kind: "loadout", Title: f.Name + " -> " + newName})
			byName[strings.ToLower(newName)] = f
			res.Files = append(res.Files, OutFile{Path: "Loadouts/" + newName, Data: f.rawBytes})
			continue
		}
		byName[key] = f
		res.Files = append(res.Files, OutFile{Path: "Loadouts/" + f.Name, Data: f.rawBytes})
	}
	for _, f := range res.Files {
		res.FileList = append(res.FileList, f.Path)
	}
	res.Stats["files"] = len(res.Files)
	res.Stats["skipped"] = res.Skipped
	res.Stats["conflicts"] = len(conflicts)

	var rb strings.Builder
	rb.WriteString("ОТЧЁТ СЛИЯНИЯ ЛОДАУТОВ EXPANSION (DayZ Override Builder)\n" + strings.Repeat("=", 68) + "\n\n")
	rb.WriteString(fmt.Sprintf("Файлов в объединённой папке: %d\nИдентичных пропущено: %d\nКонфликтов имён: %d\n\n", len(res.Files), res.Skipped, len(conflicts)))
	for _, c := range conflicts {
		rb.WriteString("  ! " + c + "\n")
	}
	res.Report = rb.String()
	logf.p("Слияние лодаутов: файлов %d, пропущено %d, конфликтов %d", len(res.Files), res.Skipped, len(conflicts))
	return res, nil
}

// LoadoutNamesFromDir возвращает имена лодаутов (без .json) для проверки квестов.
func LoadoutNamesFromDir(root string) map[string]bool {
	out := map[string]bool{}
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(strings.ToLower(p), ".json") {
			base := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
			out[strings.ToLower(base)] = true
		}
		return nil
	})
	return out
}
