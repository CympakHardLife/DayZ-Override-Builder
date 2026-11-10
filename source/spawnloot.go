// Package spawnloot проверяет и сливает ванильные cfgspawnabletypes.xml
// (лодауты предметов/зомби при спавне) и cfgrandompresets.xml (пресеты).
package spawnloot

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type Logf func(format string, args ...interface{})

func (f Logf) p(format string, args ...interface{}) {
	if f != nil {
		f(format, args...)
	}
}

// ---------- модель ----------

type PItem struct {
	Name     string `xml:"name,attr"`
	Chance   string `xml:"chance,attr"`
	QuantMin string `xml:"quantmin,attr"`
	QuantMax string `xml:"quantmax,attr"`
}

type Group struct {
	Chance string  `xml:"chance,attr"`
	Preset string  `xml:"preset,attr"`
	Items  []PItem `xml:"item"`
}

type DamageEl struct {
	Min string `xml:"min,attr"`
	Max string `xml:"max,attr"`
}

type TagEl struct {
	Name string `xml:"name,attr"`
}

type SpawnType struct {
	Name        string    `xml:"name,attr"`
	Hoarder     *struct{} `xml:"hoarder"`
	Damage      []DamageEl `xml:"damage"`
	Tags        []TagEl   `xml:"tag"`
	Attachments []Group   `xml:"attachments"`
	Cargo       []Group   `xml:"cargo"`
}

type SpawnFile struct {
	XMLName xml.Name    `xml:"spawnabletypes"`
	Types   []SpawnType `xml:"type"`
}

type Preset struct {
	Name   string  `xml:"name,attr"`
	Chance string  `xml:"chance,attr"`
	Items  []PItem `xml:"item"`
}

type PresetsFile struct {
	XMLName     xml.Name `xml:"randompresets"`
	Cargo       []Preset `xml:"cargo"`
	Attachments []Preset `xml:"attachments"`
}

func parseErr(data []byte, err error, what string) error {
	if se, ok := err.(*xml.SyntaxError); ok {
		return fmt.Errorf("%s: ошибка XML в строке %d: %s", what, se.Line, se.Msg)
	}
	return fmt.Errorf("%s: %v", what, err)
}

func ParseSpawn(data []byte) (*SpawnFile, error) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	var f SpawnFile
	if err := xml.Unmarshal(data, &f); err != nil {
		return nil, parseErr(data, err, "cfgspawnabletypes.xml")
	}
	if !strings.EqualFold(f.XMLName.Local, "spawnabletypes") {
		return nil, fmt.Errorf("корневой элемент %q — ожидался <spawnabletypes>", f.XMLName.Local)
	}
	return &f, nil
}

func ParsePresets(data []byte) (*PresetsFile, error) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	var f PresetsFile
	if err := xml.Unmarshal(data, &f); err != nil {
		return nil, parseErr(data, err, "cfgrandompresets.xml")
	}
	if !strings.EqualFold(f.XMLName.Local, "randompresets") {
		return nil, fmt.Errorf("корневой элемент %q — ожидался <randompresets>", f.XMLName.Local)
	}
	return &f, nil
}

// ---------- проверка ----------

type Problem struct {
	Level string `json:"level"`
	Msg   string `json:"msg"`
}

type CheckResult struct {
	Stats         map[string]int `json:"stats"`
	Problems      []Problem      `json:"problems"`
	UnusedPresets []string       `json:"unusedPresets,omitempty"`
	MissingTypes  []string       `json:"missingTypes,omitempty"`  // типы вне types.xml
	UnknownItems  []string       `json:"unknownItems,omitempty"`  // вложенные предметы вне types.xml
}

func chanceOK(s string) (float64, bool, bool) { // val, задан, валиден
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false, true
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, true, err == nil && v >= 0 && v <= 1
}

// Check проверяет пару файлов (пресеты могут отсутствовать).
func Check(spawn *SpawnFile, presets *PresetsFile, knownClasses map[string]bool, logf Logf) *CheckResult {
	res := &CheckResult{Stats: map[string]int{}}
	prob := func(level, format string, args ...interface{}) {
		res.Problems = append(res.Problems, Problem{Level: level, Msg: fmt.Sprintf(format, args...)})
	}

	cargoP := map[string]bool{}
	attP := map[string]bool{}
	usedCargo := map[string]bool{}
	usedAtt := map[string]bool{}
	if presets != nil {
		for _, p := range presets.Cargo {
			key := strings.ToLower(p.Name)
			if cargoP[key] {
				prob("error", "cfgrandompresets: дубликат cargo-пресета %q", p.Name)
			}
			cargoP[key] = true
			checkPresetItems(&res.Problems, "cargo-пресет "+p.Name, p, knownClasses, res)
		}
		for _, p := range presets.Attachments {
			key := strings.ToLower(p.Name)
			if attP[key] {
				prob("error", "cfgrandompresets: дубликат attachments-пресета %q", p.Name)
			}
			attP[key] = true
			checkPresetItems(&res.Problems, "attachments-пресет "+p.Name, p, knownClasses, res)
		}
		res.Stats["cargoPresets"] = len(presets.Cargo)
		res.Stats["attachPresets"] = len(presets.Attachments)
	}

	seenTypes := map[string]bool{}
	unknownItems := map[string]bool{}
	missingTypes := map[string]bool{}
	for _, t := range spawn.Types {
		tag := "тип " + t.Name
		key := strings.ToLower(t.Name)
		if seenTypes[key] {
			prob("error", "cfgspawnabletypes: дубликат типа %q — записи перекроют друг друга", t.Name)
		}
		seenTypes[key] = true
		if knownClasses != nil && !knownClasses[key] {
			missingTypes[t.Name] = true
		}
		for _, d := range t.Damage {
			mn, _ := strconv.ParseFloat(strings.TrimSpace(d.Min), 64)
			mx, _ := strconv.ParseFloat(strings.TrimSpace(d.Max), 64)
			if mn > mx {
				prob("error", "%s: damage min (%s) больше max (%s)", tag, d.Min, d.Max)
			}
		}
		checkGroups := func(kind string, groups []Group, presetSet map[string]bool, used map[string]bool) {
			for _, g := range groups {
				if g.Preset != "" {
					pkey := strings.ToLower(g.Preset)
					used[pkey] = true
					if presets == nil {
						prob("warn", "%s: ссылка на пресет %q, но cfgrandompresets.xml не приложен — проверить не могу", tag, g.Preset)
					} else if !presetSet[pkey] {
						prob("error", "%s: %s-пресет %q НЕ НАЙДЕН в cfgrandompresets.xml — лут не заспавнится", tag, kind, g.Preset)
					}
					if len(g.Items) > 0 {
						prob("warn", "%s: у группы одновременно preset=%q и вложенные <item> — предметы внутри будут проигнорированы", tag, g.Preset)
					}
					continue
				}
				if _, set, ok := chanceOK(g.Chance); set && !ok {
					prob("error", "%s: chance=%q у группы %s вне диапазона 0..1", tag, g.Chance, kind)
				}
				if len(g.Items) == 0 {
					prob("warn", "%s: пустая группа <%s> без preset и без предметов", tag, kind)
				}
				for _, it := range g.Items {
					if strings.TrimSpace(it.Name) == "" {
						prob("error", "%s: <item> без name в группе %s", tag, kind)
						continue
					}
					if _, set, ok := chanceOK(it.Chance); set && !ok {
						prob("error", "%s: предмет %s: chance=%q вне 0..1", tag, it.Name, it.Chance)
					}
					checkQuant(&res.Problems, tag, it)
					if knownClasses != nil && !knownClasses[strings.ToLower(it.Name)] {
						unknownItems[it.Name] = true
					}
				}
			}
		}
		checkGroups("attachments", t.Attachments, attP, usedAtt)
		checkGroups("cargo", t.Cargo, cargoP, usedCargo)
	}

	if presets != nil {
		for _, p := range presets.Cargo {
			if !usedCargo[strings.ToLower(p.Name)] {
				res.UnusedPresets = append(res.UnusedPresets, "cargo: "+p.Name)
			}
		}
		for _, p := range presets.Attachments {
			if !usedAtt[strings.ToLower(p.Name)] {
				res.UnusedPresets = append(res.UnusedPresets, "attachments: "+p.Name)
			}
		}
		sort.Strings(res.UnusedPresets)
		if len(res.UnusedPresets) > 0 {
			prob("warn", "пресетов не используется ни одним типом: %d (см. список — возможно, мусор)", len(res.UnusedPresets))
		}
	}
	for n := range missingTypes {
		res.MissingTypes = append(res.MissingTypes, n)
	}
	sort.Strings(res.MissingTypes)
	if len(res.MissingTypes) > 0 {
		prob("warn", "типов из cfgspawnabletypes нет в приложенном types.xml: %d — CE не заспавнит их сам (норма для предметов, спавнящихся только как вложения)", len(res.MissingTypes))
	}
	for n := range unknownItems {
		res.UnknownItems = append(res.UnknownItems, n)
	}
	sort.Strings(res.UnknownItems)

	res.Stats["types"] = len(spawn.Types)
	nErr := 0
	for _, p := range res.Problems {
		if p.Level == "error" {
			nErr++
		}
	}
	res.Stats["errors"] = nErr
	res.Stats["warnings"] = len(res.Problems) - nErr
	logf.p("Проверка спавн-лута: типов %d, ошибок %d, предупреждений %d", len(spawn.Types), nErr, len(res.Problems)-nErr)
	return res
}

func checkPresetItems(problems *[]Problem, tag string, p Preset, known map[string]bool, res *CheckResult) {
	if _, set, ok := chanceOK(p.Chance); set && !ok {
		*problems = append(*problems, Problem{Level: "error", Msg: fmt.Sprintf("%s: chance=%q вне 0..1", tag, p.Chance)})
	}
	if len(p.Items) == 0 {
		*problems = append(*problems, Problem{Level: "warn", Msg: tag + ": пустой пресет (нет предметов)"})
	}
	for _, it := range p.Items {
		if strings.TrimSpace(it.Name) == "" {
			*problems = append(*problems, Problem{Level: "error", Msg: tag + ": <item> без name"})
			continue
		}
		if _, set, ok := chanceOK(it.Chance); set && !ok {
			*problems = append(*problems, Problem{Level: "error", Msg: fmt.Sprintf("%s: предмет %s: chance=%q вне 0..1", tag, it.Name, it.Chance)})
		}
		checkQuant(problems, tag, it)
		if known != nil && !known[strings.ToLower(it.Name)] {
			res.UnknownItems = append(res.UnknownItems, it.Name)
		}
	}
}

func checkQuant(problems *[]Problem, tag string, it PItem) {
	if it.QuantMin == "" && it.QuantMax == "" {
		return
	}
	mn, errN := strconv.ParseFloat(strings.TrimSpace(it.QuantMin), 64)
	mx, errX := strconv.ParseFloat(strings.TrimSpace(it.QuantMax), 64)
	if errN != nil || errX != nil {
		*problems = append(*problems, Problem{Level: "error", Msg: fmt.Sprintf("%s: предмет %s: quantmin/quantmax не числа", tag, it.Name)})
		return
	}
	if (mn == -1) != (mx == -1) {
		*problems = append(*problems, Problem{Level: "error", Msg: fmt.Sprintf("%s: предмет %s: quantmin=%v и quantmax=%v несогласованы (-1 у обоих)", tag, it.Name, mn, mx)})
	} else if mn > mx {
		*problems = append(*problems, Problem{Level: "error", Msg: fmt.Sprintf("%s: предмет %s: quantmin (%v) больше quantmax (%v)", tag, it.Name, mn, mx)})
	}
}

// ---------- вывод (нормализованный XML) ----------

func esc(s string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

func writeItem(sb *strings.Builder, indent string, it PItem) {
	sb.WriteString(indent + `<item name="` + esc(it.Name) + `"`)
	if it.Chance != "" {
		sb.WriteString(` chance="` + esc(it.Chance) + `"`)
	}
	if it.QuantMin != "" {
		sb.WriteString(` quantmin="` + esc(it.QuantMin) + `"`)
	}
	if it.QuantMax != "" {
		sb.WriteString(` quantmax="` + esc(it.QuantMax) + `"`)
	}
	sb.WriteString(" />\n")
}

func EmitSpawn(f *SpawnFile) []byte {
	var sb strings.Builder
	sb.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"yes\" ?>\n<spawnabletypes>\n")
	for _, t := range f.Types {
		sb.WriteString("\t<type name=\"" + esc(t.Name) + "\">\n")
		if t.Hoarder != nil {
			sb.WriteString("\t\t<hoarder />\n")
		}
		for _, d := range t.Damage {
			sb.WriteString("\t\t<damage min=\"" + esc(d.Min) + "\" max=\"" + esc(d.Max) + "\" />\n")
		}
		for _, g := range t.Tags {
			sb.WriteString("\t\t<tag name=\"" + esc(g.Name) + "\" />\n")
		}
		writeGroup := func(kind string, g Group) {
			if g.Preset != "" {
				sb.WriteString("\t\t<" + kind + " preset=\"" + esc(g.Preset) + "\" />\n")
				return
			}
			sb.WriteString("\t\t<" + kind)
			if g.Chance != "" {
				sb.WriteString(" chance=\"" + esc(g.Chance) + "\"")
			}
			sb.WriteString(">\n")
			for _, it := range g.Items {
				writeItem(&sb, "\t\t\t", it)
			}
			sb.WriteString("\t\t</" + kind + ">\n")
		}
		for _, g := range t.Attachments {
			writeGroup("attachments", g)
		}
		for _, g := range t.Cargo {
			writeGroup("cargo", g)
		}
		sb.WriteString("\t</type>\n")
	}
	sb.WriteString("</spawnabletypes>\n")
	return []byte(sb.String())
}

func EmitPresets(f *PresetsFile) []byte {
	var sb strings.Builder
	sb.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"yes\" ?>\n<randompresets>\n")
	writeP := func(kind string, p Preset) {
		sb.WriteString("\t<" + kind + " chance=\"" + esc(p.Chance) + "\" name=\"" + esc(p.Name) + "\">\n")
		for _, it := range p.Items {
			writeItem(&sb, "\t\t", it)
		}
		sb.WriteString("\t</" + kind + ">\n")
	}
	for _, p := range f.Cargo {
		writeP("cargo", p)
	}
	for _, p := range f.Attachments {
		writeP("attachments", p)
	}
	sb.WriteString("</randompresets>\n")
	return []byte(sb.String())
}

// ---------- слияние ----------

type MergeResult struct {
	SpawnXML   []byte         `json:"-"`
	PresetsXML []byte         `json:"-"`
	Report     string         `json:"report"`
	Stats      map[string]int `json:"stats"`
	Conflicts  []string       `json:"conflicts"`
}

func canon(v interface{}) string {
	b, _ := xml.Marshal(v)
	return string(b)
}

// Merge объединяет пары файлов. preferFriend=true — при конфликте побеждают новые записи.
func Merge(aSpawn, bSpawn *SpawnFile, aPresets, bPresets *PresetsFile, preferFriend bool, logf Logf) *MergeResult {
	res := &MergeResult{Stats: map[string]int{}}
	skipped := 0

	// типы
	outTypes := append([]SpawnType(nil), aSpawn.Types...)
	index := map[string]int{}
	for i, t := range outTypes {
		index[strings.ToLower(t.Name)] = i
	}
	added := 0
	for _, bt := range bSpawn.Types {
		key := strings.ToLower(bt.Name)
		if i, ok := index[key]; ok {
			if canon(outTypes[i]) == canon(bt) {
				skipped++
				continue
			}
			winner := "ваша"
			if preferFriend {
				outTypes[i] = bt
				winner = "новая"
			}
			res.Conflicts = append(res.Conflicts, fmt.Sprintf("тип %q отличается — взята запись %s", bt.Name, winner))
			continue
		}
		index[key] = len(outTypes)
		outTypes = append(outTypes, bt)
		added++
	}
	// пресеты
	mergePresets := func(kind string, a, b []Preset) []Preset {
		out := append([]Preset(nil), a...)
		idx := map[string]int{}
		for i, p := range out {
			idx[strings.ToLower(p.Name)] = i
		}
		for _, bp := range b {
			key := strings.ToLower(bp.Name)
			if i, ok := idx[key]; ok {
				if canon(out[i]) == canon(bp) {
					skipped++
					continue
				}
				winner := "ваша"
				if preferFriend {
					out[i] = bp
					winner = "новая"
				}
				res.Conflicts = append(res.Conflicts, fmt.Sprintf("%s-пресет %q отличается — взята запись %s", kind, bp.Name, winner))
				continue
			}
			idx[key] = len(out)
			out = append(out, bp)
			added++
		}
		return out
	}
	outPresets := &PresetsFile{}
	if aPresets == nil {
		aPresets = &PresetsFile{}
	}
	if bPresets == nil {
		bPresets = &PresetsFile{}
	}
	outPresets.Cargo = mergePresets("cargo", aPresets.Cargo, bPresets.Cargo)
	outPresets.Attachments = mergePresets("attachments", aPresets.Attachments, bPresets.Attachments)

	res.SpawnXML = EmitSpawn(&SpawnFile{Types: outTypes})
	res.PresetsXML = EmitPresets(outPresets)
	res.Stats["types"] = len(outTypes)
	res.Stats["cargoPresets"] = len(outPresets.Cargo)
	res.Stats["attachPresets"] = len(outPresets.Attachments)
	res.Stats["added"] = added
	res.Stats["skipped"] = skipped
	res.Stats["conflicts"] = len(res.Conflicts)

	var rb strings.Builder
	rb.WriteString("ОТЧЁТ СЛИЯНИЯ СПАВН-ЛУТА (DayZ Override Builder)\n" + strings.Repeat("=", 68) + "\n\n")
	rb.WriteString(fmt.Sprintf("Типов в итоге: %d\nCargo-пресетов: %d, attachments-пресетов: %d\nДобавлено новых: %d\nИдентичных пропущено: %d\nКонфликтов: %d (побеждала запись %s)\n\n",
		len(outTypes), len(outPresets.Cargo), len(outPresets.Attachments), added, skipped, len(res.Conflicts),
		map[bool]string{false: "ВАША", true: "НОВАЯ"}[preferFriend]))
	for _, c := range res.Conflicts {
		rb.WriteString("  ! " + c + "\n")
	}
	rb.WriteString("\nПосле установки прогоните объединённые файлы через «Проверить» — ссылки на пресеты будут перепроверены.\n")
	res.Report = rb.String()
	logf.p("Слияние спавн-лута: типов %d, добавлено %d, конфликтов %d", len(outTypes), added, len(res.Conflicts))
	return res
}
