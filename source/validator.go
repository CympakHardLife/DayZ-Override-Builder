// Package validator проверяет JSON/XML/текстовые файлы сервера DayZ:
// точные номера строк, фрагмент файла и подсказки по исправлению.
package validator

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
)

type VProblem struct {
	Level      string `json:"level"` // error | warn
	Line       int    `json:"line,omitempty"`
	Msg        string `json:"msg"`
	Suggestion string `json:"suggestion,omitempty"`
	Excerpt    string `json:"excerpt,omitempty"`
}

type FileReport struct {
	Name     string     `json:"name"`
	Kind     string     `json:"kind"` // json | xml | text
	DayZKind string     `json:"dayzKind,omitempty"`
	Problems []VProblem `json:"problems"`
	OK       bool       `json:"ok"`
}

func lineOfOffset(data []byte, off int64) int {
	if off > int64(len(data)) {
		off = int64(len(data))
	}
	return 1 + bytes.Count(data[:off], []byte("\n"))
}

// excerpt возвращает фрагмент файла вокруг строки line с маркером.
func excerpt(data []byte, line int) string {
	lines := strings.Split(string(data), "\n")
	if line < 1 {
		line = 1
	}
	if line > len(lines) {
		line = len(lines)
	}
	start := line - 3
	if start < 0 {
		start = 0
	}
	end := line + 2
	if end > len(lines) {
		end = len(lines)
	}
	var sb strings.Builder
	for i := start; i < end; i++ {
		mark := "  "
		if i+1 == line {
			mark = "→ "
		}
		l := lines[i]
		if len(l) > 160 {
			l = l[:160] + "…"
		}
		sb.WriteString(fmt.Sprintf("%s%4d | %s\n", mark, i+1, l))
	}
	return sb.String()
}

// Validate распознаёт тип файла автоматически и проверяет его.
func Validate(name string, data []byte) *FileReport {
	return ValidateAs(name, data, "")
}

// ValidateAs проверяет файл. Параметр format задаёт формат вручную
// (кнопки выбора формата на вкладке «Проверка файлов»):
//
//	""/"auto"        — определить по расширению и содержимому (как раньше);
//	"types"          — XML-файл types.xml (корень <types>);
//	"spawnabletypes" — XML cfgspawnabletypes.xml (корень <spawnabletypes>);
//	"randompresets"  — XML cfgrandompresets.xml (корень <randompresets>);
//	"xml"            — произвольный XML;
//	"json"           — JSON (квест/лодаут Expansion и т.п.);
//	"text"           — config.cpp / .layout / текстовый файл.
//
// При явно выбранном формате проверка идёт нужным парсером, даже если
// расширение файла отсутствует или указано неправильно.
func ValidateAs(name string, data []byte, format string) *FileReport {
	return validateAs(name, data, format, baseCatalog())
}

func validateAs(name string, data []byte, format string, catalog nameCatalog) *FileReport {
	rep := &FileReport{Name: name}
	low := strings.ToLower(name)
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	trimmed := bytes.TrimLeft(data, " \t\r\n")

	// Ожидаемый корень XML, если пользователь явно выбрал DayZ-файл.
	expectRoot := ""
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "types":
		rep.Kind, expectRoot = "xml", "types"
	case "spawnabletypes", "cfgspawnabletypes":
		rep.Kind, expectRoot = "xml", "spawnabletypes"
	case "randompresets", "cfgrandompresets":
		rep.Kind, expectRoot = "xml", "randompresets"
	case "xml":
		rep.Kind = "xml"
	case "json":
		rep.Kind = "json"
	case "text", "cpp", "config":
		rep.Kind = "text"
	default: // auto
		switch {
		case strings.HasSuffix(low, ".json") || (len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[')):
			rep.Kind = "json"
		case strings.HasSuffix(low, ".xml") || (len(trimmed) > 1 && trimmed[0] == '<'):
			rep.Kind = "xml"
		default:
			rep.Kind = "text"
		}
	}

	switch rep.Kind {
	case "json":
		validateJSON(rep, data)
	case "xml":
		validateXMLDocument(rep, data, expectRoot, catalog)
	default:
		validateText(rep, data)
	}

	rep.OK = true
	for _, p := range rep.Problems {
		if p.Level == "error" {
			rep.OK = false
			break
		}
	}
	return rep
}

// ---------- JSON ----------

var trailingCommaRe = regexp.MustCompile(`,\s*[}\]]`)

func validateJSON(rep *FileReport, data []byte) {
	var v interface{}
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&v); err != nil {
		line, msg, sugg := explainJSONError(data, err)
		rep.Problems = append(rep.Problems, VProblem{
			Level: "error", Line: line, Msg: msg, Suggestion: sugg, Excerpt: excerpt(data, line),
		})
		// вторичная попытка: убрать хвостовые запятые
		fixed := trailingCommaRe.ReplaceAllFunc(data, func(m []byte) []byte {
			return m[bytes.IndexAny(m, "}]"):]
		})
		if !bytes.Equal(fixed, data) {
			if json.Unmarshal(fixed, &v) == nil {
				rep.Problems = append(rep.Problems, VProblem{
					Level:      "warn",
					Msg:        "после удаления лишних запятых перед } и ] файл становится валидным",
					Suggestion: "удалите запятую после последнего элемента объекта/списка — в JSON она запрещена",
				})
			}
		}
		return
	}
	// мусор после конца JSON
	if dec.More() {
		off := dec.InputOffset()
		line := lineOfOffset(data, off)
		rep.Problems = append(rep.Problems, VProblem{
			Level: "error", Line: line,
			Msg:        "после закрытия JSON в файле есть лишние данные",
			Suggestion: "удалите всё после последней закрывающей скобки (возможно, файл склеен из двух)",
			Excerpt:    excerpt(data, line),
		})
	}
	// дубликаты ключей
	findDuplicateKeys(rep, data)
	// подсказки по типу файла DayZ
	if m, ok := v.(map[string]interface{}); ok {
		switch {
		case m["Title"] != nil && m["ID"] != nil && m["Objectives"] != nil:
			rep.DayZKind = "квест Expansion"
		case m["ObjectiveType"] != nil && m["ID"] != nil:
			rep.DayZKind = "объектив Expansion"
		case m["NPCName"] != nil || m["NPCLoadoutFile"] != nil:
			rep.DayZKind = "NPC Expansion"
		case m["InventoryAttachments"] != nil || m["InventoryCargo"] != nil:
			rep.DayZKind = "лодаут Expansion"
		}
	}
}

func explainJSONError(data []byte, err error) (line int, msg, sugg string) {
	switch e := err.(type) {
	case *json.SyntaxError:
		line = lineOfOffset(data, e.Offset)
		msg = "синтаксическая ошибка JSON: " + e.Error()
		es := e.Error()
		ctx := ""
		if e.Offset > 0 && int(e.Offset) <= len(data) {
			start := int(e.Offset) - 20
			if start < 0 {
				start = 0
			}
			ctx = string(data[start:int(e.Offset)])
		}
		switch {
		case strings.Contains(es, "invalid character '}'") || strings.Contains(es, "invalid character ']'"):
			sugg = "скорее всего, перед этой скобкой стоит лишняя запятая — удалите её"
		case strings.Contains(es, "invalid character ','"):
			sugg = "лишняя или сдвоенная запятая — проверьте элементы до и после"
		case strings.Contains(es, "invalid character '\\''"):
			sugg = "в JSON строки пишутся только в двойных кавычках \" \", одинарные ' ' запрещены"
		case strings.Contains(es, "looking for beginning of object key string"):
			sugg = "ключ должен быть в двойных кавычках: \"ключ\": значение — либо перед этим местом лишняя запятая"
		case strings.Contains(es, "after object key"):
			sugg = "после имени ключа должно идти двоеточие: \"ключ\": значение"
		case strings.Contains(es, "after object key:value pair") || strings.Contains(es, "after array element"):
			sugg = "между элементами не хватает запятой, либо не закрыта кавычка строкой выше"
		case strings.Contains(es, "unexpected end of JSON input"):
			line = lineOfOffset(data, int64(len(data)))
			sugg = "файл обрывается — не хватает закрывающих скобок } или ] (проверьте баланс скобок)"
		case strings.Contains(es, "invalid character"):
			sugg = "недопустимый символ в этом месте; частые причины: комментарии (// или /* */ в JSON запрещены), незакрытая кавычка, число с запятой вместо точки"
		default:
			sugg = "проверьте синтаксис в указанной строке"
		}
		_ = ctx
	case *json.UnmarshalTypeError:
		line = lineOfOffset(data, e.Offset)
		msg = "неверный тип значения: " + e.Error()
		sugg = "проверьте значение в этой строке"
	default:
		msg = "ошибка JSON: " + err.Error()
		sugg = "проверьте синтаксис файла"
	}
	return
}

// findDuplicateKeys ищет повторяющиеся ключи в одном объекте.
func findDuplicateKeys(rep *FileReport, data []byte) {
	dec := json.NewDecoder(bytes.NewReader(data))
	type frame struct {
		isObj bool
		keys  map[string]bool
		key   bool // следующий строковый токен в объекте — ключ
	}
	var stack []*frame
	for {
		off := dec.InputOffset()
		tok, err := dec.Token()
		if err != nil {
			return
		}
		switch t := tok.(type) {
		case json.Delim:
			switch t {
			case '{':
				stack = append(stack, &frame{isObj: true, keys: map[string]bool{}, key: true})
			case '[':
				stack = append(stack, &frame{})
			case '}', ']':
				stack = stack[:len(stack)-1]
				if len(stack) > 0 && stack[len(stack)-1].isObj {
					stack[len(stack)-1].key = true
				}
			}
		case string:
			if len(stack) > 0 && stack[len(stack)-1].isObj && stack[len(stack)-1].key {
				f := stack[len(stack)-1]
				if f.keys[t] {
					line := lineOfOffset(data, off)
					rep.Problems = append(rep.Problems, VProblem{
						Level: "warn", Line: line,
						Msg:        fmt.Sprintf("дубликат ключа %q в одном объекте", t),
						Suggestion: "останется только последнее значение — удалите один из дубликатов",
						Excerpt:    excerpt(data, line),
					})
				}
				f.keys[t] = true
				f.key = false
			} else if len(stack) > 0 && stack[len(stack)-1].isObj {
				stack[len(stack)-1].key = true
			}
		default:
			if len(stack) > 0 && stack[len(stack)-1].isObj {
				stack[len(stack)-1].key = true
			}
		}
	}
}

// ---------- XML ----------

var bareAmpRe = regexp.MustCompile(`&(?:[^a-zA-Z#]|$)`)

// dayzRootLabel — человекочитаемые имена ожидаемых корней DayZ-файлов.
var dayzRootLabel = map[string]string{
	"types":          "types.xml",
	"spawnabletypes": "cfgspawnabletypes.xml",
	"randompresets":  "cfgrandompresets.xml",
}

// Известные (ванильные) значения DayZ. Несовпадение даёт ПРЕДУПРЕЖДЕНИЕ,
// а не ошибку: моды и кастомные карты добавляют свои категории/юзейджи/тиры.
var (
	knownCategories = lowerSet("weapons", "tools", "clothes", "containers", "explosives", "food", "lootdispatch", "books")
	knownUsages     = lowerSet("Military", "Police", "Medic", "Firefighter", "Industrial", "Farm", "Coast", "Town", "Village", "Hunting", "Office", "School", "Prison", "Lunapark", "SeasonalEvent", "ContaminatedArea", "Historical")
	knownValues     = lowerSet("Tier1", "Tier2", "Tier3", "Tier4", "Unique")
	knownTags       = lowerSet("floor", "shelves", "ground")
	knownFlags      = lowerSet("count_in_cargo", "count_in_hoarder", "count_in_map", "count_in_player", "crafted", "deloot")
)

type nameCatalog map[string]map[string]bool

func baseCatalog() nameCatalog {
	catalog := nameCatalog{}
	for kind, values := range map[string]map[string]bool{
		"category": knownCategories, "usage": knownUsages,
		"value": knownValues, "tag": knownTags,
	} {
		catalog[kind] = make(map[string]bool, len(values))
		for value := range values {
			catalog[kind][value] = true
		}
	}
	catalog["usage:user"] = map[string]bool{}
	catalog["value:user"] = map[string]bool{}
	return catalog
}

// ValidateXMLStructure проверяет весь документ, не отвергая значения,
// которые пользователь ещё собирается исправить в редакторе types.
func ValidateXMLStructure(data []byte) error {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	rep := &FileReport{}
	validateXMLDocument(rep, data, "", nil)
	for _, problem := range rep.Problems {
		if problem.Level == "error" {
			return fmt.Errorf("строка %d: %s", problem.Line, problem.Msg)
		}
	}
	return nil
}

func lowerSet(items ...string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, it := range items {
		m[strings.ToLower(it)] = true
	}
	return m
}

// addProblem — короткий помощник для добавления проблемы в отчёт.
func addProblem(rep *FileReport, level string, line int, msg, sugg string) {
	rep.Problems = append(rep.Problems, VProblem{Level: level, Line: line, Msg: msg, Suggestion: sugg})
}

// textSnippet превращает «сырой» текст в короткий однострочный фрагмент.
func textSnippet(b []byte) string {
	s := strings.Join(strings.Fields(string(b)), " ")
	if r := []rune(s); len(r) > 40 {
		s = string(r[:40]) + "…"
	}
	return s
}

// validateXML выполняет СТРОГУЮ проверку правильности XML (well-formedness),
// как это делают онлайн-валидаторы (codebeautify и т.п.):
//   - ровно один корневой элемент;
//   - никакого постороннего текста вне корневого тега (до или после него);
//   - никаких повторяющихся атрибутов в одном теге;
//   - корректные пары тегов и сущности (это ловит и стандартный парсер Go).
//
// Стандартный encoding/xml Go сам по себе МОЛЧА пропускает первые три ошибки —
// именно поэтому раньше битый types.xml проходил как «всё нормально».
func validateXMLDocument(rep *FileReport, data []byte, expectRoot string, catalog nameCatalog) {
	// Амперсанды проверяет XML-парсер: в комментариях и CDATA они допустимы.
	// 2) строгий разбор: один корень, никакого «голого» текста вне корня
	//    И между дочерними тегами (mixed content), без дубликатов атрибутов.
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = true
	var rootName string
	rootCount := 0
	type xmlFrame struct {
		name         string
		hasChildElem bool
		textLine     int
		textSnippet  string
	}
	var stack []*xmlFrame
	addMixed := func(elem string, line int, snippet string) {
		rep.Problems = append(rep.Problems, VProblem{
			Level: "error", Line: line,
			Msg:        fmt.Sprintf("посторонний текст внутри тега <%s> между дочерними тегами: %q", elem, snippet),
			Suggestion: "внутри тега, у которого есть вложенные теги, не должно быть «голого» текста — удалите лишние символы (частая причина: случайно набранные символы или удалён открывающий/закрывающий тег)",
			Excerpt:    excerpt(data, line),
		})
	}
	for {
		off := dec.InputOffset()
		tok, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			line := 0
			if se, ok := err.(*xml.SyntaxError); ok {
				line = se.Line
			}
			es := err.Error()
			msg := "ошибка XML: " + es
			sugg := "проверьте указанную строку"
			switch {
			case strings.Contains(es, "unexpected end element") || strings.Contains(es, "element <") && strings.Contains(es, "closed by"):
				sugg = "теги открыты и закрыты в разном порядке или с разными именами — проверьте пары <тег> … </тег>"
			case strings.Contains(es, "unexpected EOF"):
				line = lineOfOffset(data, int64(len(data)))
				sugg = "файл обрывается — какой-то тег не закрыт (ищите тег без </...> или без /> )"
			case strings.Contains(es, "invalid character entity"):
				sugg = "после & должна идти сущность (&amp; &lt; &gt; &quot;) — либо экранируйте сам символ &"
			case strings.Contains(es, "attribute name without ="):
				sugg = "у атрибута нет значения — пишите имя=\"значение\""
			case strings.Contains(es, "unquoted or missing attribute value"):
				sugg = "значение атрибута должно быть в кавычках: имя=\"значение\""
			case strings.Contains(es, "expected element name after <"):
				sugg = "после < сразу должно идти имя тега; если нужен символ «меньше» в тексте — пишите &lt;"
			}
			rep.Problems = append(rep.Problems, VProblem{
				Level: "error", Line: line, Msg: msg, Suggestion: sugg, Excerpt: excerpt(data, line),
			})
			return // после синтаксической ошибки дальше разбирать нечего
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if len(stack) == 0 {
				rootCount++
				if rootCount == 1 {
					rootName = t.Name.Local
				} else {
					line := lineOfOffset(data, off)
					rep.Problems = append(rep.Problems, VProblem{
						Level: "error", Line: line,
						Msg:        fmt.Sprintf("в файле несколько корневых элементов: после </%s> начинается ещё один тег <%s>", rootName, t.Name.Local),
						Suggestion: "в XML допускается ровно один корневой тег — всё содержимое должно быть внутри него (для types.xml это <types> … </types>)",
						Excerpt:    excerpt(data, line),
					})
					return
				}
			} else {
				parent := stack[len(stack)-1]
				if parent.textLine != 0 { // текст был ПЕРЕД этим дочерним тегом
					addMixed(parent.name, parent.textLine, parent.textSnippet)
					return
				}
				parent.hasChildElem = true
			}
			if dup, ok := firstDuplicateAttr(t.Attr); ok {
				line := lineOfOffset(data, off)
				rep.Problems = append(rep.Problems, VProblem{
					Level: "error", Line: line,
					Msg:        fmt.Sprintf("тег <%s> содержит повторяющийся атрибут %q", t.Name.Local, dup),
					Suggestion: "уберите лишний атрибут — у одного тега имя атрибута должно быть уникальным",
					Excerpt:    excerpt(data, line),
				})
			}
			stack = append(stack, &xmlFrame{name: t.Name.Local})
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if len(bytes.TrimSpace([]byte(t))) == 0 {
				break // пробелы/переводы строк между тегами — это нормально
			}
			line := lineOfOffset(data, off)
			snippet := textSnippet([]byte(t))
			if len(stack) == 0 {
				where := "после корневого элемента"
				if rootCount == 0 {
					where = "перед корневым элементом"
				}
				rep.Problems = append(rep.Problems, VProblem{
					Level: "error", Line: line,
					Msg:        fmt.Sprintf("посторонний текст вне тегов (%s): %q", where, snippet),
					Suggestion: "в правильном XML вне корневого тега не может быть текста — удалите этот фрагмент или оберните его в теги",
					Excerpt:    excerpt(data, line),
				})
				return
			}
			cur := stack[len(stack)-1]
			if cur.hasChildElem { // у тега уже есть дочерние теги, а тут ещё текст
				addMixed(cur.name, line, snippet)
				return
			}
			if cur.textLine == 0 { // запомним текст: если дальше появится дочерний тег — это mixed content
				cur.textLine = line
				cur.textSnippet = snippet
			}
		}
	}

	if rootCount == 0 {
		line := lineOfOffset(data, int64(len(data)))
		rep.Problems = append(rep.Problems, VProblem{
			Level: "error", Line: line,
			Msg:        "в файле нет ни одного XML-тега",
			Suggestion: "XML-файл должен содержать корневой элемент, например <types> … </types>",
		})
		return
	}

	// 3) выбран конкретный формат DayZ, но корень оказался другим
	if expectRoot != "" && !strings.EqualFold(rootName, expectRoot) {
		want := dayzRootLabel[expectRoot]
		if want == "" {
			want = "<" + expectRoot + ">"
		}
		rep.Problems = append(rep.Problems, VProblem{
			Level:      "warn",
			Msg:        fmt.Sprintf("выбран формат %s, но корневой тег файла — <%s>", want, rootName),
			Suggestion: "проверьте, что выбрали правильный тип файла (или что это действительно нужный файл)",
		})
	}

	// 4) углублённые проверки конкретных файлов DayZ (по фактическому корню)
	if catalog == nil {
		return
	}
	switch strings.ToLower(rootName) {
	case "types":
		rep.DayZKind = "types.xml"
		checkTypesXML(rep, data, catalog)
	case "spawnabletypes":
		rep.DayZKind = "cfgspawnabletypes.xml"
		checkChanceFile(rep, data, "type")
	case "randompresets":
		rep.DayZKind = "cfgrandompresets.xml"
		checkChanceFile(rep, data, "")
	}
}

// firstDuplicateAttr возвращает имя первого повторяющегося атрибута в теге.
// Стандартный encoding/xml Go повторяющиеся атрибуты не отвергает, поэтому
// проверяем их вручную.
func firstDuplicateAttr(attrs []xml.Attr) (string, bool) {
	seen := make(map[string]bool, len(attrs))
	for _, a := range attrs {
		key := a.Name.Local
		if a.Name.Space != "" {
			key = a.Name.Space + ":" + a.Name.Local
		}
		if seen[key] {
			return a.Name.Local, true
		}
		seen[key] = true
	}
	return "", false
}

// ---------- DayZ: смысловая проверка types.xml (правила экономики) ----------

type rawText struct {
	Raw string `xml:",chardata"`
}
type nameAttr struct {
	Name string `xml:"name,attr"`
	User string `xml:"user,attr"`
}
type flagsXML struct {
	Attrs []xml.Attr `xml:",any,attr"`
}
type anyElem struct {
	XMLName xml.Name
}

// typeXML — одна запись <type> в types.xml.
type typeXML struct {
	Name     string     `xml:"name,attr"`
	Nominal  *rawText   `xml:"nominal"`
	Min      *rawText   `xml:"min"`
	Lifetime *rawText   `xml:"lifetime"`
	Restock  *rawText   `xml:"restock"`
	QuantMin *rawText   `xml:"quantmin"`
	QuantMax *rawText   `xml:"quantmax"`
	Cost     *rawText   `xml:"cost"`
	Flags    *flagsXML  `xml:"flags"`
	Category []nameAttr `xml:"category"`
	Usage    []nameAttr `xml:"usage"`
	Value    []nameAttr `xml:"value"`
	Tag      []nameAttr `xml:"tag"`
	Unknown  []anyElem  `xml:",any"`
}

// checkTypesXML применяет правила DayZ к каждой записи <type>, сообщая
// проблемы с номером строки этой записи. Файл уже прошёл проверку
// правильности XML, поэтому здесь ошибок парсинга не будет.
func checkTypesXML(rep *FileReport, data []byte, catalog nameCatalog) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = true
	seen := map[string]bool{}
	depth := 0
	for {
		off := dec.InputOffset()
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch se := tok.(type) {
		case xml.StartElement:
			if depth == 1 && strings.EqualFold(se.Name.Local, "type") {
				line := lineOfOffset(data, off)
				var te typeXML
				if dec.DecodeElement(&te, &se) == nil {
					checkOneType(rep, &te, line, seen, catalog)
				}
				continue // DecodeElement поглотил </type>, глубину не меняем
			}
			depth++
		case xml.EndElement:
			depth--
		}
	}
}

type intRes struct {
	v  int64
	ok bool
}

// parseIntField разбирает целочисленное поле <tag>значение</tag>.
func parseIntField(rep *FileReport, line int, typeName, field string, rv *rawText, allowNeg bool) intRes {
	if rv == nil {
		return intRes{}
	}
	raw := strings.TrimSpace(rv.Raw)
	if raw == "" {
		return intRes{}
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		addProblem(rep, "error", line, fmt.Sprintf("%s: <%s> — не целое число (%q)", typeName, field, raw), "впишите целое число")
		return intRes{}
	}
	if !allowNeg && v < 0 {
		addProblem(rep, "error", line, fmt.Sprintf("%s: <%s> отрицательное (%d)", typeName, field, v), "значение не может быть меньше 0")
	}
	return intRes{v: v, ok: true}
}

func checkOneType(rep *FileReport, te *typeXML, line int, seen map[string]bool, catalog nameCatalog) {
	label := strings.TrimSpace(te.Name)
	if label == "" {
		label = "(без имени)"
		addProblem(rep, "error", line, "запись <type> без атрибута name", "у каждого <type> должен быть уникальный name=\"…\"")
	} else {
		key := strings.ToLower(label)
		if seen[key] {
			addProblem(rep, "warn", line, fmt.Sprintf("дубликат записи name=%q", label), "в игре сработает только последняя такая запись — удалите лишние")
		}
		seen[key] = true
	}

	nominal := parseIntField(rep, line, label, "nominal", te.Nominal, false)
	minv := parseIntField(rep, line, label, "min", te.Min, false)
	life := parseIntField(rep, line, label, "lifetime", te.Lifetime, false)
	parseIntField(rep, line, label, "restock", te.Restock, false)
	parseIntField(rep, line, label, "cost", te.Cost, false)

	if te.Nominal == nil {
		addProblem(rep, "warn", line, fmt.Sprintf("%s: нет <nominal>", label), "добавьте <nominal> — сколько таких предметов держать в мире")
	}
	if te.Lifetime == nil {
		addProblem(rep, "warn", line, fmt.Sprintf("%s: нет <lifetime>", label), "добавьте <lifetime> в секундах (сколько предмет лежит до исчезновения)")
	} else if life.ok && life.v == 0 && nominal.ok && nominal.v > 0 {
		addProblem(rep, "warn", line, fmt.Sprintf("%s: <lifetime>=0 при включённом спавне", label), "проверьте срок жизни и назначение записи; нулевой lifetime не означает обычное время хранения лута")
	}
	if te.Flags == nil {
		addProblem(rep, "warn", line, fmt.Sprintf("%s: нет <flags>", label), "добавьте <flags .../> — иначе применяются значения по умолчанию")
	}

	// главное правило экономики (файл загружается, но логика неверная → warn)
	if nominal.ok && nominal.v > 0 && minv.ok && nominal.v < minv.v {
		addProblem(rep, "warn", line, fmt.Sprintf("%s: nominal (%d) меньше min (%d)", label, nominal.v, minv.v), "обычно nominal должен быть не меньше min — иначе экономика спавна работает неверно")
	}

	// quantmin / quantmax (значения корректны по форме, но логика спорная → warn)
	qmin := parseIntField(rep, line, label, "quantmin", te.QuantMin, true)
	qmax := parseIntField(rep, line, label, "quantmax", te.QuantMax, true)
	if qmin.ok && qmax.ok {
		min1, max1 := qmin.v == -1, qmax.v == -1
		switch {
		case min1 != max1:
			addProblem(rep, "warn", line, fmt.Sprintf("%s: quantmin=%d и quantmax=%d несогласованы", label, qmin.v, qmax.v), "-1 должен стоять у обоих или ни у одного")
		case !min1 && (qmin.v < 0 || qmin.v > 100 || qmax.v < 0 || qmax.v > 100):
			addProblem(rep, "warn", line, fmt.Sprintf("%s: quantmin/quantmax вне диапазона 0..100", label), "используйте проценты 0..100 либо -1 (не задавать)")
		case !min1 && qmin.v > qmax.v:
			addProblem(rep, "warn", line, fmt.Sprintf("%s: quantmin (%d) больше quantmax (%d)", label, qmin.v, qmax.v), "минимум не может быть больше максимума — поменяйте местами")
		}
	}

	// флаги: только 0 или 1
	if te.Flags != nil {
		for _, a := range te.Flags.Attrs {
			val := strings.TrimSpace(a.Value)
			if val != "0" && val != "1" {
				addProblem(rep, "error", line, fmt.Sprintf("%s: флаг %s=%q — допустимы только 0 или 1", label, a.Name.Local, a.Value), "поставьте 0 (выкл) или 1 (вкл)")
			}
			if !knownFlags[strings.ToLower(a.Name.Local)] {
				addProblem(rep, "warn", line, fmt.Sprintf("%s: неизвестный флаг %q", label, a.Name.Local), "проверьте написание флага")
			}
		}
	}

	checkNameList(rep, line, label, "category", te.Category, catalog)
	checkNameList(rep, line, label, "usage", te.Usage, catalog)
	checkNameList(rep, line, label, "value", te.Value, catalog)
	checkNameList(rep, line, label, "tag", te.Tag, catalog)

	for _, u := range te.Unknown {
		addProblem(rep, "warn", line, fmt.Sprintf("%s: неизвестный тег <%s> внутри <type>", label, u.XMLName.Local), "проверьте написание — игра игнорирует незнакомые теги (возможно опечатка)")
	}
}

func checkNameList(rep *FileReport, line int, typeName, kind string, items []nameAttr, catalog nameCatalog) {
	for _, it := range items {
		v := strings.TrimSpace(it.Name)
		known := catalog[kind]
		if it.User != "" {
			if v != "" {
				addProblem(rep, "warn", line, fmt.Sprintf("%s: <%s> содержит одновременно name и user", typeName, kind), "оставьте обычное имя либо ссылку на пользовательскую группу")
			}
			v = strings.TrimSpace(it.User)
			known = catalog[kind+":user"]
		}
		if v == "" {
			addProblem(rep, "error", line, fmt.Sprintf("%s: пустое имя в <%s name=\"\"/>", typeName, kind), "укажите имя или удалите тег")
			continue
		}
		if !known[strings.ToLower(v)] {
			addProblem(rep, "warn", line, fmt.Sprintf("%s: неизвестное значение <%s name=%q>", typeName, kind, v), "проверьте написание; кастомные значения из модов/карт можно игнорировать")
		}
	}
}

// ---------- DayZ: проверка cfgspawnabletypes.xml и cfgrandompresets.xml ----------

// checkChanceFile проверяет файлы со «шансами»: каждый атрибут chance должен
// быть числом 0.0–1.0. Для cfgspawnabletypes требуется name у <type>
// (requireNameElem="type"); для cfgrandompresets — у <cargo>/<attachments>
// верхнего уровня (requireNameElem="").
func checkChanceFile(rep *FileReport, data []byte, requireNameElem string) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = true
	depth := 0
	for {
		off := dec.InputOffset()
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch se := tok.(type) {
		case xml.StartElement:
			line := lineOfOffset(data, off)
			ln := strings.ToLower(se.Name.Local)
			for _, a := range se.Attr {
				if strings.EqualFold(a.Name.Local, "chance") {
					checkChance(rep, line, se.Name.Local, a.Value, ln == "item")
				}
			}
			if ln == "item" && attrVal(se.Attr, "name") == "" {
				addProblem(rep, "error", line, "<item> без атрибута name", "укажите classname предмета")
			}
			if depth == 1 {
				needName := (requireNameElem != "" && ln == requireNameElem) ||
					(requireNameElem == "" && (ln == "cargo" || ln == "attachments"))
				if needName && attrVal(se.Attr, "name") == "" {
					addProblem(rep, "error", line, fmt.Sprintf("<%s> без атрибута name", se.Name.Local), "у записи должен быть name=\"…\"")
				}
			}
			depth++
		case xml.EndElement:
			depth--
		}
	}
}

func checkChance(rep *FileReport, line int, elem, raw string, weight bool) {
	raw = strings.TrimSpace(raw)
	rangeHint := "chance — дробное число от 0.0 до 1.0"
	if weight {
		rangeHint = "вес предмета должен быть конечным числом не меньше 0; значения больше 1 допустимы"
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		addProblem(rep, "error", line, fmt.Sprintf("<%s>: chance=%q — не конечное число", elem, raw), rangeHint)
		return
	}
	if weight && f >= 0 {
		return // У предметов это относительный вес, например 40.00 в Sakhal 1.30.
	}
	if weight {
		addProblem(rep, "error", line, fmt.Sprintf("<%s>: отрицательный вес chance=%s", elem, raw), rangeHint)
		return
	}
	if f < 0 || f > 1 {
		addProblem(rep, "error", line, fmt.Sprintf("<%s>: chance=%s вне диапазона 0.0–1.0", elem, raw), "поставьте значение от 0.0 (никогда) до 1.0 (всегда)")
	}
}

func attrVal(attrs []xml.Attr, name string) string {
	for _, a := range attrs {
		if strings.EqualFold(a.Name.Local, name) {
			return strings.TrimSpace(a.Value)
		}
	}
	return ""
}

// ---------- текст (config.cpp, .layout, .c) ----------

func validateText(rep *FileReport, data []byte) {
	// баланс скобок с учётом строк и комментариев
	line := 1
	var stack []struct {
		ch   byte
		line int
	}
	inStr := false
	inLine := false
	inBlock := false
	var prev byte
	for i := 0; i < len(data); i++ {
		c := data[i]
		if c == '\n' {
			line++
			inLine = false
			prev = c
			continue
		}
		if inLine {
			continue
		}
		if inBlock {
			if prev == '*' && c == '/' {
				inBlock = false
			}
			prev = c
			continue
		}
		if inStr {
			if c == '"' {
				if i+1 < len(data) && data[i+1] == '"' {
					i++
				} else {
					inStr = false
				}
			}
			prev = c
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '/':
			if i+1 < len(data) {
				if data[i+1] == '/' {
					inLine = true
				} else if data[i+1] == '*' {
					inBlock = true
				}
			}
		case '{', '(', '[':
			stack = append(stack, struct {
				ch   byte
				line int
			}{c, line})
		case '}', ')', ']':
			want := map[byte]byte{'}': '{', ')': '(', ']': '['}[c]
			if len(stack) == 0 {
				rep.Problems = append(rep.Problems, VProblem{
					Level: "error", Line: line,
					Msg:        fmt.Sprintf("лишняя закрывающая скобка %q", string(c)),
					Suggestion: "удалите её или найдите пропущенную открывающую выше",
					Excerpt:    excerpt(data, line),
				})
			} else if stack[len(stack)-1].ch != want {
				rep.Problems = append(rep.Problems, VProblem{
					Level: "error", Line: line,
					Msg:        fmt.Sprintf("скобка %q не соответствует открытой %q (строка %d)", string(c), string(stack[len(stack)-1].ch), stack[len(stack)-1].line),
					Suggestion: "проверьте порядок скобок",
					Excerpt:    excerpt(data, line),
				})
				stack = stack[:len(stack)-1]
			} else {
				stack = stack[:len(stack)-1]
			}
		}
		prev = c
	}
	for _, s := range stack {
		rep.Problems = append(rep.Problems, VProblem{
			Level: "error", Line: s.line,
			Msg:        fmt.Sprintf("скобка %q открыта, но не закрыта", string(s.ch)),
			Suggestion: "добавьте закрывающую скобку (для config.cpp обычно «};»)",
			Excerpt:    excerpt(data, s.line),
		})
	}
	if inStr {
		rep.Problems = append(rep.Problems, VProblem{
			Level: "error", Line: line,
			Msg:        "незакрытая кавычка \" до конца файла",
			Suggestion: "найдите строку без закрывающей кавычки",
		})
	}
}
