// Package typesx parses, migrates, validates and optimises DayZ economy
// types.xml files.
package typesx

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"dayzoverride/validator"
)

// IntVal is an integer element that survives non-numeric garbage.
type IntVal struct {
	Set bool
	Ok  bool
	Raw string
	V   int64
}

func (iv *IntVal) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var s string
	if err := d.DecodeElement(&s, &start); err != nil {
		return err
	}
	iv.Set = true
	iv.Raw = strings.TrimSpace(s)
	if v, err := strconv.ParseInt(iv.Raw, 10, 64); err == nil {
		iv.V = v
		iv.Ok = true
	}
	return nil
}

func mkInt(v int64) IntVal { return IntVal{Set: true, Ok: true, V: v} }

type NameAttr struct {
	Name string `xml:"name,attr"`
}

type Flags struct {
	Set            bool   `xml:"-"`
	CountInCargo   string `xml:"count_in_cargo,attr"`
	CountInHoarder string `xml:"count_in_hoarder,attr"`
	CountInMap     string `xml:"count_in_map,attr"`
	CountInPlayer  string `xml:"count_in_player,attr"`
	Crafted        string `xml:"crafted,attr"`
	Deloot         string `xml:"deloot,attr"`
}

type flagsAlias Flags

func (f *Flags) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var a flagsAlias
	if err := d.DecodeElement(&a, &start); err != nil {
		return err
	}
	*f = Flags(a)
	f.Set = true
	return nil
}

// RawElem keeps unknown child elements so they are not silently lost.
type RawElem struct {
	XMLName xml.Name
	Attrs   []xml.Attr `xml:",any,attr"`
	Inner   string     `xml:",innerxml"`
}

type TypeEntry struct {
	Name     string     `xml:"name,attr"`
	Nominal  IntVal     `xml:"nominal"`
	Lifetime IntVal     `xml:"lifetime"`
	Restock  IntVal     `xml:"restock"`
	Min      IntVal     `xml:"min"`
	QuantMin IntVal     `xml:"quantmin"`
	QuantMax IntVal     `xml:"quantmax"`
	Cost     IntVal     `xml:"cost"`
	Flags    Flags      `xml:"flags"`
	Category *NameAttr  `xml:"category"`
	Usages   []NameAttr `xml:"usage"`
	Values   []NameAttr `xml:"value"`
	Tags     []NameAttr `xml:"tag"`
	Extra    []RawElem  `xml:",any"`

	simplified bool `xml:"-"`
}

type TypesFile struct {
	XMLName xml.Name    `xml:"types"`
	Types   []TypeEntry `xml:"type"`
}

// Parse reads a types.xml (BOM tolerated).
func Parse(data []byte) (*TypesFile, error) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if err := validator.ValidateXMLStructure(data); err != nil {
		return nil, err
	}
	var tf TypesFile
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = true
	if err := dec.Decode(&tf); err != nil {
		if se, ok := err.(*xml.SyntaxError); ok {
			return nil, fmt.Errorf("ошибка XML в строке %d: %s", se.Line, se.Msg)
		}
		return nil, fmt.Errorf("ошибка разбора XML: %v", err)
	}
	if !strings.EqualFold(tf.XMLName.Local, "types") {
		return nil, fmt.Errorf("корневой элемент %q — ожидался <types>", tf.XMLName.Local)
	}
	return &tf, nil
}

// ---------- emission (canonical clean formatting) ----------

func esc(s string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

func writeInt(sb *strings.Builder, tag string, iv IntVal) {
	if !iv.Set {
		return
	}
	v := iv.Raw
	if iv.Ok {
		v = strconv.FormatInt(iv.V, 10)
	}
	sb.WriteString("        <" + tag + ">" + esc(v) + "</" + tag + ">\n")
}

func flagVal(s string) string {
	if strings.TrimSpace(s) == "1" {
		return "1"
	}
	return "0"
}

// Emit renders the file with canonical formatting (4-space indent, stable
// element order, no stray whitespace).
func Emit(types []TypeEntry) []byte {
	var sb strings.Builder
	sb.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"yes\"?>\n")
	sb.WriteString("<types>\n")
	for i := range types {
		t := &types[i]
		sb.WriteString("    <type name=\"" + esc(t.Name) + "\">\n")
		if !t.simplified {
			writeInt(&sb, "nominal", t.Nominal)
		}
		writeInt(&sb, "lifetime", t.Lifetime)
		if !t.simplified {
			writeInt(&sb, "restock", t.Restock)
			writeInt(&sb, "min", t.Min)
			writeInt(&sb, "quantmin", t.QuantMin)
			writeInt(&sb, "quantmax", t.QuantMax)
			writeInt(&sb, "cost", t.Cost)
		}
		if t.Flags.Set {
			f := t.Flags
			sb.WriteString(fmt.Sprintf("        <flags count_in_cargo=\"%s\" count_in_hoarder=\"%s\" count_in_map=\"%s\" count_in_player=\"%s\" crafted=\"%s\" deloot=\"%s\"/>\n",
				flagVal(f.CountInCargo), flagVal(f.CountInHoarder), flagVal(f.CountInMap), flagVal(f.CountInPlayer), flagVal(f.Crafted), flagVal(f.Deloot)))
		}
		if !t.simplified {
			if t.Category != nil && t.Category.Name != "" {
				sb.WriteString("        <category name=\"" + esc(t.Category.Name) + "\"/>\n")
			}
			for _, u := range t.Usages {
				sb.WriteString("        <usage name=\"" + esc(u.Name) + "\"/>\n")
			}
			for _, v := range t.Values {
				sb.WriteString("        <value name=\"" + esc(v.Name) + "\"/>\n")
			}
			for _, g := range t.Tags {
				sb.WriteString("        <tag name=\"" + esc(g.Name) + "\"/>\n")
			}
			for _, x := range t.Extra {
				sb.WriteString("        <" + x.XMLName.Local)
				for _, a := range x.Attrs {
					sb.WriteString(" " + a.Name.Local + "=\"" + esc(a.Value) + "\"")
				}
				inner := strings.TrimSpace(x.Inner)
				if inner == "" {
					sb.WriteString("/>\n")
				} else {
					sb.WriteString(">" + inner + "</" + x.XMLName.Local + ">\n")
				}
			}
		}
		sb.WriteString("    </type>\n")
	}
	sb.WriteString("</types>\n")
	return []byte(sb.String())
}

// ---------- migration (после вайпа / обновления игры) ----------

type MigrateResult struct {
	MergedXML   string   `json:"mergedXml"`
	Report      string   `json:"report"`
	Total       int      `json:"total"`
	Transferred int      `json:"transferred"`
	NewNames    []string `json:"newNames"`    // есть в новом, не было у вас
	MissingKept []string `json:"missingKept"` // есть у вас, нет в новом (сохранены/нет)
	KeepMissing bool     `json:"keepMissing"`
}

type Logf func(format string, args ...interface{})

func (f Logf) p(format string, args ...interface{}) {
	if f != nil {
		f(format, args...)
	}
}

// Migrate applies the user's saved types over a fresh (post-update) types.
func Migrate(myData, newData []byte, keepMissing bool, logf Logf) (*MigrateResult, error) {
	logf.p("Разбираю ВАШ сохранённый types.xml…")
	mine, err := Parse(myData)
	if err != nil {
		return nil, fmt.Errorf("ваш types.xml: %v", err)
	}
	logf.p("Ваших записей: %d", len(mine.Types))
	logf.p("Разбираю НОВЫЙ (ванильный) types.xml…")
	fresh, err := Parse(newData)
	if err != nil {
		return nil, fmt.Errorf("новый types.xml: %v", err)
	}
	logf.p("Записей в новом: %d", len(fresh.Types))

	myMap := map[string]*TypeEntry{}
	for i := range mine.Types {
		myMap[strings.ToLower(mine.Types[i].Name)] = &mine.Types[i]
	}

	res := &MigrateResult{KeepMissing: keepMissing}
	var out []TypeEntry
	usedMine := map[string]bool{}
	for i := range fresh.Types {
		key := strings.ToLower(fresh.Types[i].Name)
		if m, ok := myMap[key]; ok {
			out = append(out, *m) // ваши настройки побеждают
			usedMine[key] = true
			res.Transferred++
		} else {
			out = append(out, fresh.Types[i]) // новый класснейм — ванильные значения
			res.NewNames = append(res.NewNames, fresh.Types[i].Name)
		}
	}
	// ваши записи, которых нет в новом файле (моды / удалённые предметы)
	for i := range mine.Types {
		key := strings.ToLower(mine.Types[i].Name)
		if !usedMine[key] {
			res.MissingKept = append(res.MissingKept, mine.Types[i].Name)
			if keepMissing {
				out = append(out, mine.Types[i])
			}
		}
	}
	res.Total = len(out)
	sort.Strings(res.NewNames)
	sort.Strings(res.MissingKept)

	logf.p("Перенесено ваших настроек: %d", res.Transferred)
	logf.p("Новых класснеймов (появились после обновления): %d", len(res.NewNames))
	if keepMissing {
		logf.p("Ваших записей, отсутствующих в новом (сохранены в конце файла): %d", len(res.MissingKept))
	} else {
		logf.p("Ваших записей, отсутствующих в новом (НЕ включены): %d", len(res.MissingKept))
	}

	res.MergedXML = string(Emit(out))

	var rb strings.Builder
	rb.WriteString("ОТЧЁТ МИГРАЦИИ TYPES.XML (DayZ Override Builder)\n" + strings.Repeat("=", 68) + "\n\n")
	rb.WriteString(fmt.Sprintf("Записей в вашем файле: %d\nЗаписей в новом файле: %d\nИтоговых записей: %d\nПеренесено ваших настроек: %d\n\n",
		len(mine.Types), len(fresh.Types), res.Total, res.Transferred))
	if len(res.NewNames) > 0 {
		rb.WriteString(fmt.Sprintf("НОВЫЕ КЛАССНЕЙМЫ (%d) — появились в новом файле, оставлены с ванильными значениями:\n", len(res.NewNames)))
		for _, n := range res.NewNames {
			rb.WriteString("  + " + n + "\n")
		}
		rb.WriteString("\n")
	}
	if len(res.MissingKept) > 0 {
		if keepMissing {
			rb.WriteString(fmt.Sprintf("ВАШИ ЗАПИСИ, КОТОРЫХ НЕТ В НОВОМ ФАЙЛЕ (%d) — сохранены в конце итогового файла (обычно это предметы модов):\n", len(res.MissingKept)))
		} else {
			rb.WriteString(fmt.Sprintf("ВАШИ ЗАПИСИ, КОТОРЫХ НЕТ В НОВОМ ФАЙЛЕ (%d) — НЕ включены по вашему выбору:\n", len(res.MissingKept)))
		}
		for _, n := range res.MissingKept {
			rb.WriteString("  ? " + n + "\n")
		}
	}
	res.Report = rb.String()
	return res, nil
}

// ---------- optimisation / validation ----------

type OptimizeResult struct {
	FixedXML   string   `json:"fixedXml"`
	Report     string   `json:"report"`
	Total      int      `json:"total"`
	Fixes      int      `json:"fixes"`
	Simplified int      `json:"simplified"`
	Duplicates int      `json:"duplicates"`
	Problems   []string `json:"problems"`
}

// Optimize validates and normalises a types.xml.
func Optimize(data []byte, simplify bool, logf Logf) (*OptimizeResult, error) {
	logf.p("Разбираю types.xml…")
	tf, err := Parse(data)
	if err != nil {
		return nil, err
	}
	logf.p("Записей: %d. Проверяю правила…", len(tf.Types))

	res := &OptimizeResult{}
	report := func(format string, args ...interface{}) {
		res.Problems = append(res.Problems, fmt.Sprintf(format, args...))
		res.Fixes++
	}

	// дубликаты: остаётся последняя запись
	byName := map[string]int{}
	var out []TypeEntry
	for i := range tf.Types {
		t := tf.Types[i]
		if t.Name == "" {
			report("запись без имени (name) — удалена")
			continue
		}
		key := strings.ToLower(t.Name)
		if idx, ok := byName[key]; ok {
			out[idx] = t
			res.Duplicates++
			report("%s: дубликат записи — оставлена последняя версия", t.Name)
			continue
		}
		byName[key] = len(out)
		out = append(out, t)
	}

	for i := range out {
		t := &out[i]
		name := t.Name

		// нечисловые значения
		fixNaN := func(tag string, iv *IntVal, def int64) {
			if iv.Set && !iv.Ok {
				report("%s: <%s> не число (%q) — заменено на %d", name, tag, iv.Raw, def)
				*iv = mkInt(def)
			}
		}
		fixNaN("nominal", &t.Nominal, 0)
		fixNaN("lifetime", &t.Lifetime, 14400)
		fixNaN("restock", &t.Restock, 0)
		fixNaN("min", &t.Min, 0)
		fixNaN("quantmin", &t.QuantMin, -1)
		fixNaN("quantmax", &t.QuantMax, -1)
		fixNaN("cost", &t.Cost, 100)

		// обязательные поля
		if !t.Lifetime.Set {
			report("%s: нет <lifetime> — установлен 14400", name)
			t.Lifetime = mkInt(14400)
		}
		if !t.Nominal.Set {
			report("%s: нет <nominal> — установлен 0", name)
			t.Nominal = mkInt(0)
		}
		if !t.Min.Set {
			report("%s: нет <min> — установлен 0", name)
			t.Min = mkInt(0)
		}
		if !t.Flags.Set {
			report("%s: нет <flags> — добавлены нулевые флаги", name)
			t.Flags = Flags{Set: true}
		}

		// отрицательные значения
		negFix := func(tag string, iv *IntVal) {
			if iv.Set && iv.Ok && iv.V < 0 {
				report("%s: <%s> отрицательный (%d) — заменён на 0", name, tag, iv.V)
				*iv = mkInt(0)
			}
		}
		negFix("nominal", &t.Nominal)
		negFix("min", &t.Min)
		negFix("lifetime", &t.Lifetime)
		negFix("restock", &t.Restock)

		// nominal=0 отключает обычный спавн; такие записи есть в ванильной CE.
		if t.Nominal.Ok && t.Nominal.V > 0 && t.Min.Ok && t.Nominal.V < t.Min.V {
			report("%s: nominal (%d) МЕНЬШЕ min (%d) — min понижен до %d", name, t.Nominal.V, t.Min.V, t.Nominal.V)
			t.Min = mkInt(t.Nominal.V)
		}

		// quantmin / quantmax согласованность
		qmin, qmax := t.QuantMin, t.QuantMax
		if qmin.Set && qmax.Set && qmin.Ok && qmax.Ok && (qmin.V == -1) != (qmax.V == -1) {
			report("%s: quantmin=%d и quantmax=%d несогласованы (-1 должен быть у обоих) — оба установлены в -1", name, qmin.V, qmax.V)
			t.QuantMin, t.QuantMax = mkInt(-1), mkInt(-1)
		} else {
			clampQuant := func(tag string, iv *IntVal) {
				if !iv.Set || !iv.Ok || iv.V == -1 {
					return
				}
				if iv.V < 0 {
					report("%s: <%s> %d меньше 0%% — ограничен до 0", name, tag, iv.V)
					*iv = mkInt(0)
				} else if iv.V > 100 {
					report("%s: <%s> %d больше 100%% — ограничен до 100", name, tag, iv.V)
					*iv = mkInt(100)
				}
			}
			clampQuant("quantmin", &t.QuantMin)
			clampQuant("quantmax", &t.QuantMax)
			qmin, qmax = t.QuantMin, t.QuantMax
			if qmin.Set && qmax.Set && qmin.Ok && qmax.Ok && qmin.V > qmax.V && qmin.V != -1 {
				report("%s: quantmin (%d) больше quantmax (%d) — значения поменяны местами", name, qmin.V, qmax.V)
				t.QuantMin, t.QuantMax = qmax, qmin
			}
		}

		// упрощение неспавнящихся записей (nominal=0 и min=0)
		if simplify && t.Nominal.Ok && t.Min.Ok && t.Nominal.V == 0 && t.Min.V == 0 {
			t.simplified = true
			res.Simplified++
		}
	}
	res.Total = len(out)
	logf.p("Проверка завершена: исправлений %d, дубликатов %d, упрощено неспавнящихся записей %d",
		res.Fixes, res.Duplicates, res.Simplified)

	res.FixedXML = string(Emit(out))

	var rb strings.Builder
	rb.WriteString("ОТЧЁТ ПРОВЕРКИ И ОПТИМИЗАЦИИ TYPES.XML (DayZ Override Builder)\n" + strings.Repeat("=", 68) + "\n\n")
	rb.WriteString(fmt.Sprintf("Записей: %d\nИсправлений: %d\nДубликатов удалено: %d\nУпрощено неспавнящихся записей (nominal=0, min=0 -> только lifetime и flags): %d\n\n",
		res.Total, res.Fixes, res.Duplicates, res.Simplified))
	if len(res.Problems) > 0 {
		rb.WriteString("СПИСОК ПРАВОК И ПРОБЛЕМ:\n")
		for _, p := range res.Problems {
			rb.WriteString("  ~ " + p + "\n")
		}
	} else {
		rb.WriteString("Проблем не найдено — файл был корректен, выполнено только форматирование.\n")
	}
	rb.WriteString("\nПримечание: XML-комментарии исходного файла в итоговый не переносятся.\n")
	res.Report = rb.String()
	return res, nil
}
