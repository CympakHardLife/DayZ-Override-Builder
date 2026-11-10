// Package expansion работает с серверными профилями DayZ Expansion:
// квесты (проверка связей, цепочки, слияние паков) и лодауты.
package expansion

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Logf func(format string, args ...interface{})

func (f Logf) p(format string, args ...interface{}) {
	if f != nil {
		f(format, args...)
	}
}

// ---------- модель ----------

type QuestFile struct {
	Path     string                 `json:"path"` // относительный путь внутри пака
	Raw      map[string]interface{} `json:"-"`
	RawBytes []byte                 `json:"-"`
	ID       int                    `json:"id"`
	Title    string                 `json:"title"`
	FollowUp int                    `json:"followUp"`
	Pre      []int                  `json:"pre"`
	Givers   []int                  `json:"givers"`
	TurnIns  []int                  `json:"turnIns"`
	Objs     []ObjRef               `json:"objectives"`
	Daily    bool                   `json:"daily"`
	Weekly   bool                   `json:"weekly"`
	Repeat   bool                   `json:"repeatable"`
	Achieve  bool                   `json:"achievement"`
}

type ObjRef struct {
	ID   int `json:"id"`
	Type int `json:"type"`
}

type ObjectiveFile struct {
	Path       string                 `json:"path"`
	Folder     string                 `json:"folder"` // имя папки типа (Collection, Target, ...)
	Raw        map[string]interface{} `json:"-"`
	RawBytes   []byte                 `json:"-"`
	ID         int                    `json:"id"`
	OType      int                    `json:"otype"`
	Text       string                 `json:"text"`
	ClassNames []string               `json:"classNames"`
}

type NPCFile struct {
	Path     string                 `json:"path"`
	Raw      map[string]interface{} `json:"-"`
	RawBytes []byte                 `json:"-"`
	ID       int                    `json:"id"`
	Name     string                 `json:"name"`
	Class    string                 `json:"className"`
	Loadout  string                 `json:"loadout"`
}

type ExtraFile struct {
	Path string
	Data []byte
}

type QuestPack struct {
	Quests     map[int]*QuestFile
	Objectives map[int]map[int]*ObjectiveFile // OType -> ID -> file
	NPCs       map[int]*NPCFile
	Extras     []ExtraFile // PersistentServerData.json и прочее из корня
	Warnings   []string
	// folder name per objective type (наблюдаемое соответствие)
	TypeFolder map[int]string
}

// ---------- загрузка ----------

func intOf(v interface{}) (int, bool) {
	switch x := v.(type) {
	case float64:
		return int(x), true
	case json.Number:
		i, err := x.Int64()
		return int(i), err == nil
	}
	return 0, false
}

func intsOf(v interface{}) []int {
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	var out []int
	for _, e := range arr {
		if n, ok := intOf(e); ok {
			out = append(out, n)
		}
	}
	return out
}

func strOf(v interface{}) string {
	s, _ := v.(string)
	return s
}

func boolish(v interface{}) bool {
	n, ok := intOf(v)
	return ok && n != 0
}

// collectClassNames рекурсивно собирает значения полей ClassName.
func collectClassNames(v interface{}, out *[]string) {
	switch x := v.(type) {
	case map[string]interface{}:
		for k, val := range x {
			if strings.EqualFold(k, "ClassName") {
				if s, ok := val.(string); ok && s != "" {
					*out = append(*out, s)
				}
			} else {
				collectClassNames(val, out)
			}
		}
	case []interface{}:
		for _, e := range x {
			collectClassNames(e, out)
		}
	}
}

// LoadQuestPack читает папку с профилем квестов (корень с подпапками
// Quests / Objectives / NPCs, либо сама папка Quests).
func LoadQuestPack(root string, logf Logf) (*QuestPack, error) {
	pack := &QuestPack{
		Quests:     map[int]*QuestFile{},
		Objectives: map[int]map[int]*ObjectiveFile{},
		NPCs:       map[int]*NPCFile{},
		TypeFolder: map[int]string{},
	}
	// найти базу: папку, в которой лежат подпапки Quests/NPCs/Objectives
	base := root
	if st, err := os.Stat(filepath.Join(root, "Quests")); err == nil && st.IsDir() {
		if st2, err2 := os.Stat(filepath.Join(root, "Quests", "Quests")); err2 == nil && st2.IsDir() {
			base = filepath.Join(root, "Quests") // корень содержит папку Quests с профилем
		}
	}
	nq, no, nn := 0, 0, 0
	err := filepath.Walk(base, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(base, p)
		rel = filepath.ToSlash(rel)
		segs := strings.Split(rel, "/")
		low := strings.ToLower(rel)
		if !strings.HasSuffix(low, ".json") {
			if strings.Contains(low, "playerdata/") || strings.Contains(low, "groupdata/") {
				return nil // рантайм-данные игроков — не конфиги
			}
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			pack.Warnings = append(pack.Warnings, fmt.Sprintf("%s: %v", rel, err))
			return nil
		}
		var raw map[string]interface{}
		if err := json.Unmarshal(data, &raw); err != nil {
			pack.Warnings = append(pack.Warnings, fmt.Sprintf("%s: битый JSON: %v", rel, jsonErrLine(data, err)))
			return nil
		}
		switch {
		case len(segs) >= 2 && strings.EqualFold(segs[0], "Quests"):
			q := &QuestFile{Path: rel, Raw: raw, RawBytes: data}
			q.ID, _ = intOf(raw["ID"])
			q.Title = strOf(raw["Title"])
			q.FollowUp = -1
			if n, ok := intOf(raw["FollowUpQuest"]); ok {
				q.FollowUp = n
			}
			q.Pre = intsOf(raw["PreQuestIDs"])
			q.Givers = intsOf(raw["QuestGiverIDs"])
			q.TurnIns = intsOf(raw["QuestTurnInIDs"])
			q.Daily = boolish(raw["IsDailyQuest"])
			q.Weekly = boolish(raw["IsWeeklyQuest"])
			q.Repeat = boolish(raw["Repeatable"])
			q.Achieve = boolish(raw["IsAchievement"])
			if arr, ok := raw["Objectives"].([]interface{}); ok {
				for _, e := range arr {
					if m, ok := e.(map[string]interface{}); ok {
						id, _ := intOf(m["ID"])
						ot, _ := intOf(m["ObjectiveType"])
						q.Objs = append(q.Objs, ObjRef{ID: id, Type: ot})
					}
				}
			}
			if old, dup := pack.Quests[q.ID]; dup {
				pack.Warnings = append(pack.Warnings, fmt.Sprintf("ДУБЛИКАТ ID квеста %d: %s и %s — используется последний", q.ID, old.Path, rel))
			}
			pack.Quests[q.ID] = q
			nq++
		case len(segs) >= 3 && strings.EqualFold(segs[0], "Objectives"):
			o := &ObjectiveFile{Path: rel, Folder: segs[1], Raw: raw, RawBytes: data}
			o.ID, _ = intOf(raw["ID"])
			o.OType, _ = intOf(raw["ObjectiveType"])
			o.Text = strOf(raw["ObjectiveText"])
			collectClassNames(raw, &o.ClassNames)
			m := pack.Objectives[o.OType]
			if m == nil {
				m = map[int]*ObjectiveFile{}
				pack.Objectives[o.OType] = m
			}
			if old, dup := m[o.ID]; dup {
				pack.Warnings = append(pack.Warnings, fmt.Sprintf("ДУБЛИКАТ объектива тип %d ID %d: %s и %s", o.OType, o.ID, old.Path, rel))
			}
			m[o.ID] = o
			pack.TypeFolder[o.OType] = segs[1]
			no++
		case len(segs) >= 2 && strings.EqualFold(segs[0], "NPCs"):
			n := &NPCFile{Path: rel, Raw: raw, RawBytes: data}
			n.ID, _ = intOf(raw["ID"])
			n.Name = strOf(raw["NPCName"])
			n.Class = strOf(raw["ClassName"])
			n.Loadout = strOf(raw["NPCLoadoutFile"])
			if old, dup := pack.NPCs[n.ID]; dup {
				pack.Warnings = append(pack.Warnings, fmt.Sprintf("ДУБЛИКАТ ID NPC %d: %s и %s", n.ID, old.Path, rel))
			}
			pack.NPCs[n.ID] = n
			nn++
		case len(segs) == 1:
			pack.Extras = append(pack.Extras, ExtraFile{Path: rel, Data: data})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if nq == 0 {
		return nil, fmt.Errorf("в папке не найдено квестов (ожидается структура Quests/Quests/*.json, Quests/Objectives/..., Quests/NPCs/...)")
	}
	logf.p("Пак загружен: квестов %d, объективов %d, NPC %d, предупреждений %d", nq, no, nn, len(pack.Warnings))
	return pack, nil
}

func jsonErrLine(data []byte, err error) string {
	if se, ok := err.(*json.SyntaxError); ok {
		line := 1 + bytes.Count(data[:se.Offset], []byte("\n"))
		return fmt.Sprintf("строка %d: %v", line, err)
	}
	return err.Error()
}

// ---------- проверка ----------

type Problem struct {
	Level string `json:"level"` // error | warn
	Msg   string `json:"msg"`
}

type ChainNode struct {
	ID     int    `json:"id"`
	Title  string `json:"title"`
	Daily  bool   `json:"daily"`
	Weekly bool   `json:"weekly"`
	Repeat bool   `json:"repeat"`
	Pre    []int  `json:"pre"`
	Broken bool   `json:"broken"` // FollowUp указывает в никуда
}

type CheckResult struct {
	Stats            map[string]int `json:"stats"`
	Problems         []Problem      `json:"problems"`
	Chains           [][]ChainNode  `json:"chains"`
	RequiredLoadouts []string       `json:"requiredLoadouts"`
	MissingLoadouts  []string       `json:"missingLoadouts,omitempty"`
	ClassNames       []string       `json:"classNames"`
	UnknownClasses   []string       `json:"unknownClasses,omitempty"`
}

// Check проверяет все связи пака. loadoutNames — имена доступных лодаутов
// (без .json, опционально); knownClasses — класснеймы из types.xml (опц.).
func Check(pack *QuestPack, loadoutNames map[string]bool, knownClasses map[string]bool, logf Logf) *CheckResult {
	res := &CheckResult{Stats: map[string]int{}}
	for _, w := range pack.Warnings {
		res.Problems = append(res.Problems, Problem{Level: "warn", Msg: w})
	}
	nObj := 0
	for _, m := range pack.Objectives {
		nObj += len(m)
	}
	res.Stats["quests"] = len(pack.Quests)
	res.Stats["objectives"] = nObj
	res.Stats["npcs"] = len(pack.NPCs)

	classSet := map[string]bool{}

	ids := sortedQuestIDs(pack)
	logf.p("Проверяю связи %d квестов…", len(ids))
	for _, id := range ids {
		q := pack.Quests[id]
		tag := fmt.Sprintf("квест %d «%s» (%s)", q.ID, q.Title, q.Path)
		// объективы
		if len(q.Objs) == 0 {
			res.Problems = append(res.Problems, Problem{Level: "warn", Msg: tag + ": нет ни одного объектива"})
		}
		for _, or := range q.Objs {
			m := pack.Objectives[or.Type]
			if m == nil || m[or.ID] == nil {
				res.Problems = append(res.Problems, Problem{Level: "error",
					Msg: fmt.Sprintf("%s: объектив ID %d (тип %d) НЕ НАЙДЕН в папке Objectives", tag, or.ID, or.Type)})
			}
		}
		// NPC
		for _, nid := range q.Givers {
			if pack.NPCs[nid] == nil {
				res.Problems = append(res.Problems, Problem{Level: "error",
					Msg: fmt.Sprintf("%s: QuestGiver NPC %d не найден", tag, nid)})
			}
		}
		for _, nid := range q.TurnIns {
			if pack.NPCs[nid] == nil {
				res.Problems = append(res.Problems, Problem{Level: "error",
					Msg: fmt.Sprintf("%s: TurnIn NPC %d не найден", tag, nid)})
			}
		}
		if len(q.Givers) == 0 && !q.Achieve {
			res.Problems = append(res.Problems, Problem{Level: "warn", Msg: tag + ": нет QuestGiverIDs — квест никто не выдаёт (норма только для ачивок/автоквестов)"})
		}
		// цепочки
		if q.FollowUp > 0 && pack.Quests[q.FollowUp] == nil {
			res.Problems = append(res.Problems, Problem{Level: "error",
				Msg: fmt.Sprintf("%s: FollowUpQuest %d не существует — цепочка ОБОРВАНА", tag, q.FollowUp)})
		}
		for _, pid := range q.Pre {
			if pack.Quests[pid] == nil {
				res.Problems = append(res.Problems, Problem{Level: "error",
					Msg: fmt.Sprintf("%s: PreQuestID %d не существует — квест невозможно открыть", tag, pid)})
			}
		}
		// описания
		if arr, ok := q.Raw["Descriptions"].([]interface{}); ok && len(arr) != 3 {
			res.Problems = append(res.Problems, Problem{Level: "warn",
				Msg: fmt.Sprintf("%s: Descriptions содержит %d элементов (ожидается 3)", tag, len(arr))})
		}
		var rcls []string
		collectClassNames(q.Raw["Rewards"], &rcls)
		collectClassNames(q.Raw["QuestItems"], &rcls)
		for _, c := range rcls {
			classSet[c] = true
		}
	}
	// классы из объективов
	for _, m := range pack.Objectives {
		for _, o := range m {
			for _, c := range o.ClassNames {
				classSet[c] = true
			}
		}
	}
	// NPC → лодауты
	needLoadout := map[string]bool{}
	for _, n := range pack.NPCs {
		if n.Loadout != "" {
			needLoadout[strings.TrimSuffix(n.Loadout, ".json")] = true
		}
		if n.Class == "" {
			res.Problems = append(res.Problems, Problem{Level: "warn", Msg: fmt.Sprintf("NPC %d (%s): пустой ClassName", n.ID, n.Path)})
		}
	}
	for l := range needLoadout {
		res.RequiredLoadouts = append(res.RequiredLoadouts, l)
		if loadoutNames != nil && !loadoutNames[strings.ToLower(l)] {
			res.MissingLoadouts = append(res.MissingLoadouts, l)
			res.Problems = append(res.Problems, Problem{Level: "error",
				Msg: fmt.Sprintf("лодаут «%s» нужен NPC, но отсутствует в папке Loadouts", l)})
		}
	}
	sort.Strings(res.RequiredLoadouts)
	sort.Strings(res.MissingLoadouts)

	for c := range classSet {
		res.ClassNames = append(res.ClassNames, c)
		if knownClasses != nil && !knownClasses[strings.ToLower(c)] {
			res.UnknownClasses = append(res.UnknownClasses, c)
		}
	}
	sort.Strings(res.ClassNames)
	sort.Strings(res.UnknownClasses)
	if knownClasses != nil && len(res.UnknownClasses) > 0 {
		res.Problems = append(res.Problems, Problem{Level: "warn",
			Msg: fmt.Sprintf("класснеймы, которых нет в приложенном types.xml: %d шт. (см. список — это не всегда ошибка)", len(res.UnknownClasses))})
	}

	res.Chains = buildChains(pack, res)
	nErr := 0
	for _, p := range res.Problems {
		if p.Level == "error" {
			nErr++
		}
	}
	res.Stats["errors"] = nErr
	res.Stats["warnings"] = len(res.Problems) - nErr
	res.Stats["chains"] = len(res.Chains)
	logf.p("Проверка завершена: ошибок %d, предупреждений %d, цепочек %d", nErr, len(res.Problems)-nErr, len(res.Chains))
	return res
}

func sortedQuestIDs(pack *QuestPack) []int {
	var ids []int
	for id := range pack.Quests {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}

// buildChains строит цепочки по FollowUpQuest.
func buildChains(pack *QuestPack, res *CheckResult) [][]ChainNode {
	isFollowTarget := map[int]bool{}
	for _, q := range pack.Quests {
		if q.FollowUp > 0 {
			isFollowTarget[q.FollowUp] = true
		}
	}
	var chains [][]ChainNode
	for _, id := range sortedQuestIDs(pack) {
		q := pack.Quests[id]
		if isFollowTarget[q.ID] {
			continue // не начало цепочки
		}
		var chain []ChainNode
		visited := map[int]bool{}
		cur := q
		for cur != nil {
			if visited[cur.ID] {
				res.Problems = append(res.Problems, Problem{Level: "error",
					Msg: fmt.Sprintf("ЦИКЛ в цепочке квестов на ID %d — FollowUpQuest зациклен", cur.ID)})
				break
			}
			visited[cur.ID] = true
			node := ChainNode{ID: cur.ID, Title: cur.Title, Daily: cur.Daily, Weekly: cur.Weekly, Repeat: cur.Repeat, Pre: cur.Pre}
			next := (*QuestFile)(nil)
			if cur.FollowUp > 0 {
				next = pack.Quests[cur.FollowUp]
				if next == nil {
					node.Broken = true
				}
			}
			chain = append(chain, node)
			cur = next
		}
		chains = append(chains, chain)
	}
	return chains
}

// ---------- слияние ----------

type Rename struct {
	Kind  string `json:"kind"` // quest | objective | npc
	OType int    `json:"otype,omitempty"`
	Old   int    `json:"old"`
	New   int    `json:"new"`
	Title string `json:"title,omitempty"`
}

type MergeResult struct {
	Files    []OutFile `json:"-"`
	FileList []string  `json:"files"`
	Renames  []Rename  `json:"renames"`
	Skipped  int       `json:"skipped"` // одинаковые в обоих паках
	Stats    map[string]int `json:"stats"`
	Report   string    `json:"report"`
}

type OutFile struct {
	Path string
	Data []byte
}

func canonJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// MergeQuestPacks объединяет пак A (ваш, базовый) и пак B (новый): конфликты ID
// перенумеровываются в B со сквозным переписыванием ссылок.
func MergeQuestPacks(a, b *QuestPack, logf Logf) (*MergeResult, error) {
	res := &MergeResult{Stats: map[string]int{}}
	questMap := map[int]int{} // old B id -> new id
	npcMap := map[int]int{}
	objMap := map[int]map[int]int{} // otype -> old -> new

	nextFree := func(used func(int) bool, start int) int {
		id := start
		for used(id) {
			id++
		}
		return id
	}

	// квесты
	maxQ := 0
	for id := range a.Quests {
		if id > maxQ {
			maxQ = id
		}
	}
	usedQ := map[int]bool{}
	for id := range a.Quests {
		usedQ[id] = true
	}
	for _, id := range sortedQuestIDs(b) {
		q := b.Quests[id]
		if aq, ok := a.Quests[id]; ok {
			if canonJSON(aq.Raw) == canonJSON(q.Raw) {
				res.Skipped++
				continue // идентичный квест — берём из A
			}
			nid := nextFree(func(i int) bool { return usedQ[i] }, maxQ+1)
			usedQ[nid] = true
			questMap[id] = nid
			res.Renames = append(res.Renames, Rename{Kind: "quest", Old: id, New: nid, Title: q.Title})
		} else {
			usedQ[id] = true
		}
	}
	// NPC
	maxN := 0
	usedN := map[int]bool{}
	for id := range a.NPCs {
		usedN[id] = true
		if id > maxN {
			maxN = id
		}
	}
	var bNpcIDs []int
	for id := range b.NPCs {
		bNpcIDs = append(bNpcIDs, id)
	}
	sort.Ints(bNpcIDs)
	for _, id := range bNpcIDs {
		n := b.NPCs[id]
		if an, ok := a.NPCs[id]; ok {
			if canonJSON(an.Raw) == canonJSON(n.Raw) {
				res.Skipped++
				continue
			}
			nid := nextFree(func(i int) bool { return usedN[i] }, maxN+1)
			usedN[nid] = true
			npcMap[id] = nid
			res.Renames = append(res.Renames, Rename{Kind: "npc", Old: id, New: nid, Title: n.Name})
		} else {
			usedN[id] = true
		}
	}
	// объективы (по типам)
	for otype, bm := range b.Objectives {
		am := a.Objectives[otype]
		maxO := 0
		usedO := map[int]bool{}
		for id := range am {
			usedO[id] = true
			if id > maxO {
				maxO = id
			}
		}
		var ids []int
		for id := range bm {
			ids = append(ids, id)
		}
		sort.Ints(ids)
		for _, id := range ids {
			o := bm[id]
			if am != nil {
				if ao, ok := am[id]; ok {
					if canonJSON(ao.Raw) == canonJSON(o.Raw) {
						res.Skipped++
						continue
					}
					nid := nextFree(func(i int) bool { return usedO[i] }, maxO+1)
					usedO[nid] = true
					if objMap[otype] == nil {
						objMap[otype] = map[int]int{}
					}
					objMap[otype][id] = nid
					res.Renames = append(res.Renames, Rename{Kind: "objective", OType: otype, Old: id, New: nid, Title: o.Text})
					continue
				}
			}
			usedO[id] = true
		}
	}
	logf.p("Конфликтов ID к перенумерации: %d, идентичных файлов пропущено: %d", len(res.Renames), res.Skipped)

	// --- вывод ---
	seenPath := map[string]bool{}
	addFile := func(path string, data []byte) {
		path = "Quests/" + path
		base := path
		n := 2
		for seenPath[strings.ToLower(path)] {
			ext := filepath.Ext(base)
			path = strings.TrimSuffix(base, ext) + fmt.Sprintf("_%d", n) + ext
			n++
		}
		seenPath[strings.ToLower(path)] = true
		res.Files = append(res.Files, OutFile{Path: path, Data: data})
	}

	// A: без изменений (байты как есть)
	for _, id := range sortedQuestIDs(a) {
		addFile(a.Quests[id].Path, a.Quests[id].RawBytes)
	}
	for _, m := range a.Objectives {
		for _, o := range sortedObjs(m) {
			addFile(o.Path, o.RawBytes)
		}
	}
	for _, n := range sortedNPCs(a.NPCs) {
		addFile(n.Path, n.RawBytes)
	}
	for _, e := range a.Extras {
		addFile(e.Path, e.Data)
	}

	// B: с учётом переименований
	remapArr := func(raw map[string]interface{}, key string, mp map[int]int) bool {
		hitAny := false
		if arr, ok := raw[key].([]interface{}); ok {
			for i, e := range arr {
				if n, ok2 := intOf(e); ok2 {
					if nn, hit := mp[n]; hit {
						arr[i] = float64(nn)
						hitAny = true
					}
				}
			}
		}
		return hitAny
	}
	for _, id := range sortedQuestIDs(b) {
		q := b.Quests[id]
		if _, isDup := a.Quests[id]; isDup && questMap[id] == 0 {
			continue // идентичный — пропущен
		}
		changed := false
		newID := q.ID
		if nid, hit := questMap[q.ID]; hit {
			newID = nid
			q.Raw["ID"] = float64(nid)
			changed = true
		}
		if fu, ok := intOf(q.Raw["FollowUpQuest"]); ok && fu > 0 {
			if nn, hit := questMap[fu]; hit {
				q.Raw["FollowUpQuest"] = float64(nn)
				changed = true
			}
		}
		if remapArr(q.Raw, "PreQuestIDs", questMap) {
			changed = true
		}
		if remapArr(q.Raw, "QuestGiverIDs", npcMap) {
			changed = true
		}
		if remapArr(q.Raw, "QuestTurnInIDs", npcMap) {
			changed = true
		}
		if arr, ok := q.Raw["Objectives"].([]interface{}); ok {
			for _, e := range arr {
				if m, ok2 := e.(map[string]interface{}); ok2 {
					oid, _ := intOf(m["ID"])
					ot, _ := intOf(m["ObjectiveType"])
					if objMap[ot] != nil {
						if nn, hit := objMap[ot][oid]; hit {
							m["ID"] = float64(nn)
							changed = true
						}
					}
				}
			}
		}
		path := q.Path
		if newID != id {
			path = fmt.Sprintf("Quests/Quest_%d_merged.json", newID)
		}
		if changed {
			data, _ := json.MarshalIndent(q.Raw, "", "    ")
			addFile(path, data)
		} else {
			addFile(path, q.RawBytes)
		}
	}
	for otype, bm := range b.Objectives {
		for _, o := range sortedObjs(bm) {
			am := a.Objectives[otype]
			if am != nil && am[o.ID] != nil && (objMap[otype] == nil || objMap[otype][o.ID] == 0) {
				continue // идентичный
			}
			newID := o.ID
			if objMap[otype] != nil {
				if nn, hit := objMap[otype][o.ID]; hit {
					newID = nn
				}
			}
			if newID != o.ID {
				o.Raw["ID"] = float64(newID)
				data, _ := json.MarshalIndent(o.Raw, "", "    ")
				addFile(fmt.Sprintf("Objectives/%s/Objective_%d_merged.json", o.Folder, newID), data)
			} else {
				addFile(o.Path, o.RawBytes)
			}
		}
	}
	for _, n := range sortedNPCs(b.NPCs) {
		if a.NPCs[n.ID] != nil && npcMap[n.ID] == 0 {
			continue
		}
		newID := n.ID
		if nn, hit := npcMap[n.ID]; hit {
			newID = nn
		}
		if newID != n.ID {
			n.Raw["ID"] = float64(newID)
			data, _ := json.MarshalIndent(n.Raw, "", "    ")
			addFile(fmt.Sprintf("NPCs/QuestNPC_%d_merged.json", newID), data)
		} else {
			addFile(n.Path, n.RawBytes)
		}
	}

	for _, f := range res.Files {
		res.FileList = append(res.FileList, f.Path)
	}
	res.Stats["files"] = len(res.Files)
	res.Stats["renames"] = len(res.Renames)
	res.Stats["skipped"] = res.Skipped

	var rb strings.Builder
	rb.WriteString("ОТЧЁТ СЛИЯНИЯ ПАКОВ КВЕСТОВ EXPANSION (DayZ Override Builder)\n" + strings.Repeat("=", 68) + "\n\n")
	rb.WriteString(fmt.Sprintf("Файлов в объединённом паке: %d\nИдентичных пропущено: %d\nПеренумеровано конфликтов: %d\n\n", len(res.Files), res.Skipped, len(res.Renames)))
	if len(res.Renames) > 0 {
		rb.WriteString("ПЕРЕНУМЕРАЦИЯ (новый пак):\n")
		for _, r := range res.Renames {
			switch r.Kind {
			case "quest":
				rb.WriteString(fmt.Sprintf("  квест %d -> %d  «%s»\n", r.Old, r.New, r.Title))
			case "npc":
				rb.WriteString(fmt.Sprintf("  NPC   %d -> %d  «%s»\n", r.Old, r.New, r.Title))
			default:
				rb.WriteString(fmt.Sprintf("  объектив (тип %d) %d -> %d  «%s»\n", r.OType, r.Old, r.New, r.Title))
			}
		}
		rb.WriteString("\nВсе ссылки (FollowUpQuest, PreQuestIDs, QuestGiverIDs, QuestTurnInIDs, Objectives)\nв новом паке переписаны автоматически.\n")
	}
	rb.WriteString("\nPlayerData/GroupData/PersistentServerData взяты из ВАШЕГО пака (прогресс игроков не трогаем).\nПосле установки проверьте объединённый пак вкладкой «Проверка».\n")
	res.Report = rb.String()
	return res, nil
}

func sortedObjs(m map[int]*ObjectiveFile) []*ObjectiveFile {
	var out []*ObjectiveFile
	for _, o := range m {
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func sortedNPCs(m map[int]*NPCFile) []*NPCFile {
	var out []*NPCFile
	for _, n := range m {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
