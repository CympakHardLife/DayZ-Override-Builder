package validator

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strings"
)

// InputFile preserves the mission-relative path supplied by the uploader.
type InputFile struct {
	Name string
	Data []byte
}

type batchElement struct {
	name   string
	attrs  []xml.Attr
	line   int
	parent *batchElement
}

type batchDocument struct {
	root     string
	elements []*batchElement
}

// ValidateSet validates each file and the links between the uploaded files.
// An incomplete upload is allowed: dependencies absent from the set are warnings.
func ValidateSet(files []InputFile, format string) []*FileReport {
	docs := make([]*batchDocument, len(files))
	catalog := baseCatalog()
	for i, file := range files {
		docs[i] = readBatchXML(file.Data)
		mergeBatchCatalog(catalog, docs[i])
	}
	reports := make([]*FileReport, len(files))
	for i, file := range files {
		reports[i] = validateAs(file.Name, file.Data, format, catalog)
		if reports[i].Kind != "xml" {
			docs[i] = nil
		}
	}
	checkBatchDuplicates(files, docs, reports)
	checkBatchPresets(files, docs, reports)
	checkBatchCE(files, docs, reports)
	return reports
}

// Only documents that pass the complete strict structure check may contribute
// declarations. A valid prefix of a broken XML file must not resolve a link.
func readBatchXML(data []byte) *batchDocument {
	if ValidateXMLStructure(data) != nil {
		return nil
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	dec := xml.NewDecoder(bytes.NewReader(data))
	doc := &batchDocument{}
	var stack []*batchElement
	for {
		off := dec.InputOffset()
		token, err := dec.Token()
		if err == io.EOF {
			return doc
		}
		if err != nil {
			return nil
		}
		switch token := token.(type) {
		case xml.StartElement:
			elem := &batchElement{name: token.Name.Local, attrs: token.Attr, line: lineOfOffset(data, off)}
			if len(stack) > 0 {
				elem.parent = stack[len(stack)-1]
			} else {
				doc.root = elem.name
			}
			doc.elements = append(doc.elements, elem)
			stack = append(stack, elem)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		}
	}
}

func mergeBatchCatalog(catalog nameCatalog, doc *batchDocument) {
	if doc == nil || doc.root != "lists" && doc.root != "user_lists" {
		return
	}
	groups := map[string]string{"categories": "category", "tags": "tag", "usageflags": "usage", "valueflags": "value"}
	for _, elem := range doc.elements {
		if elem.parent == nil || elem.parent.parent == nil || elem.parent.parent.parent != nil {
			continue
		}
		kind := groups[elem.parent.name]
		if kind == "" {
			continue
		}
		if doc.root == "user_lists" {
			if elem.name != "user" || kind != "usage" && kind != "value" {
				continue
			}
			kind += ":user"
		} else if elem.name != kind {
			continue
		}
		name := strings.TrimSpace(attrVal(elem.attrs, "name"))
		if name != "" {
			catalog[kind][strings.ToLower(name)] = true
		}
	}
}

func batchProblem(files []InputFile, reports []*FileReport, index, line int, level, message, suggestion string) {
	report := reports[index]
	report.Problems = append(report.Problems, VProblem{
		Level: level, Line: line, Msg: message, Suggestion: suggestion,
		Excerpt: excerpt(files[index].Data, line),
	})
	if level == "error" {
		report.OK = false
	}
}

type batchLocation struct {
	file int
	line int
}

func checkBatchDuplicates(files []InputFile, docs []*batchDocument, reports []*FileReport) {
	seen := map[string]batchLocation{}
	for i, doc := range docs {
		if doc == nil || doc.root != "types" && doc.root != "spawnabletypes" {
			continue
		}
		for _, elem := range doc.elements {
			if elem.name != "type" || elem.parent == nil || elem.parent.parent != nil {
				continue
			}
			name := strings.TrimSpace(attrVal(elem.attrs, "name"))
			if name == "" {
				continue
			}
			key := doc.root + "\x00" + strings.ToLower(name)
			if first, ok := seen[key]; ok {
				if first.file != i {
					batchProblem(files, reports, i, elem.line, "warn",
						fmt.Sprintf("дубликат <type name=%q> среди файлов <%s>: уже есть в %s, строка %d", name, doc.root, files[first.file].Name, first.line),
						"проверьте порядок загрузки CE и удалите непреднамеренное повторение")
				}
			} else {
				seen[key] = batchLocation{i, elem.line}
			}
		}
	}
}

func checkBatchPresets(files []InputFile, docs []*batchDocument, reports []*FileReport) {
	presets := map[string]bool{}
	havePresets := false
	for _, doc := range docs {
		if doc == nil || doc.root != "randompresets" {
			continue
		}
		havePresets = true
		for _, elem := range doc.elements {
			if elem.parent == nil || elem.parent.parent != nil || elem.name != "cargo" && elem.name != "attachments" {
				continue
			}
			if name := strings.TrimSpace(attrVal(elem.attrs, "name")); name != "" {
				presets[elem.name+"\x00"+name] = true
			}
		}
	}
	for i, doc := range docs {
		if doc == nil || doc.root != "spawnabletypes" {
			continue
		}
		for _, elem := range doc.elements {
			if elem.name != "cargo" && elem.name != "attachments" {
				continue
			}
			name := strings.TrimSpace(attrVal(elem.attrs, "preset"))
			if name == "" {
				for _, attr := range elem.attrs {
					if attr.Name.Local == "preset" {
						batchProblem(files, reports, i, elem.line, "error", fmt.Sprintf("<%s>: пустое имя preset", elem.name), "укажите имя пресета или удалите атрибут preset")
					}
				}
				continue
			}
			if presets[elem.name+"\x00"+name] {
				continue
			}
			level, detail := "warn", "cfgrandompresets.xml не загружен: ссылку нельзя проверить"
			if havePresets {
				level, detail = "error", "пресет такого вида отсутствует в загруженных randompresets"
			}
			batchProblem(files, reports, i, elem.line, level,
				fmt.Sprintf("<%s preset=%q>: %s", elem.name, name, detail),
				"загрузите файл с определением пресета или исправьте его имя и вид (cargo/attachments)")
		}
	}
}

var ceFileRoots = map[string]string{
	"types": "types", "spawnabletypes": "spawnabletypes", "randompresets": "randompresets",
	"events": "events", "globals": "variables", "economy": "economy", "messages": "messages",
}

var ceVanillaFiles = map[string]string{
	"types": "db/types.xml", "spawnabletypes": "cfgspawnabletypes.xml", "randompresets": "cfgrandompresets.xml",
	"events": "db/events.xml", "variables": "db/globals.xml", "economy": "db/economy.xml", "messages": "messages.xml",
}

// missionPath accepts either slash style but never absolute paths or traversal.
func missionPath(value string, allowEmpty bool) (string, bool) {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	if value == "" {
		return "", allowEmpty
	}
	if strings.HasPrefix(value, "/") || strings.ContainsAny(value, ":\x00") {
		return "", false
	}
	for _, part := range strings.Split(value, "/") {
		if part == ".." {
			return "", false
		}
	}
	value = path.Clean(value)
	if value == "." {
		return "", allowEmpty
	}
	return strings.ToLower(value), true
}

func checkBatchCE(files []InputFile, docs []*batchDocument, reports []*FileReport) {
	byPath := map[string][]int{}
	for i, file := range files {
		if name, ok := missionPath(file.Name, false); ok {
			byPath[name] = append(byPath[name], i)
		}
	}
	registered := map[string]batchLocation{}
	var coreDirs []string
	for i, doc := range docs {
		if doc == nil || doc.root != "economycore" {
			continue
		}
		coreName, validName := missionPath(files[i].Name, false)
		coreDir := ""
		if validName {
			coreDir = path.Dir(coreName)
			if coreDir == "." {
				coreDir = ""
			}
		}
		coreDirs = append(coreDirs, coreDir)
		for _, elem := range doc.elements {
			if elem.name == "ce" && elem.parent != nil && elem.parent.parent == nil {
				if _, ok := missionPath(attrVal(elem.attrs, "folder"), true); !ok {
					batchProblem(files, reports, i, elem.line, "error", "<ce>: folder должен быть относительным путём внутри миссии", "уберите абсолютный путь и переходы ..")
				}
			}
			if elem.name != "file" || elem.parent == nil || elem.parent.name != "ce" || elem.parent.parent == nil || elem.parent.parent.parent != nil {
				continue
			}
			folder, folderOK := missionPath(attrVal(elem.parent.attrs, "folder"), true)
			fileName, nameOK := missionPath(attrVal(elem.attrs, "name"), false)
			if !nameOK {
				batchProblem(files, reports, i, elem.line, "error", "<file>: name должен задавать непустой относительный путь внутри миссии", "укажите имя XML-файла без абсолютного пути и переходов ..")
			}
			kind := strings.ToLower(strings.TrimSpace(attrVal(elem.attrs, "type")))
			wantRoot, kindOK := ceFileRoots[kind]
			if !kindOK {
				batchProblem(files, reports, i, elem.line, "error", fmt.Sprintf("<file>: неизвестный или пустой type=%q", kind), "укажите тип CE: types, spawnabletypes, randompresets, events, globals, economy или messages")
			}
			if !folderOK || !nameOK {
				continue
			}
			target := path.Join(coreDir, folder, fileName)
			if first, ok := registered[target]; ok {
				batchProblem(files, reports, i, elem.line, "warn", fmt.Sprintf("файл %q зарегистрирован в CE повторно: %s, строка %d", target, files[first.file].Name, first.line), "оставьте одну регистрацию файла")
			} else {
				registered[target] = batchLocation{i, elem.line}
			}
			indices := byPath[target]
			if len(indices) == 0 {
				batchProblem(files, reports, i, elem.line, "warn", fmt.Sprintf("CE ссылается на %q, но этот файл не загружен", target), "загрузите файл с его относительным путём; при частичной проверке это предупреждение допустимо")
				continue
			}
			for _, index := range indices {
				if targetDoc := docs[index]; targetDoc != nil && kindOK && targetDoc.root != wantRoot {
					batchProblem(files, reports, i, elem.line, "error", fmt.Sprintf("CE type=%q для %q требует <%s>, но корень файла — <%s>", kind, files[index].Name, wantRoot, targetDoc.root), "исправьте type в регистрации CE или выберите нужный файл")
				} else if kindOK && reports[index].Kind != "xml" {
					batchProblem(files, reports, i, elem.line, "error", fmt.Sprintf("CE type=%q для %q требует XML с корнем <%s>", kind, files[index].Name, wantRoot), "загрузите XML-файл нужного вида или исправьте регистрацию CE")
				}
			}
		}
	}
	if len(coreDirs) == 0 {
		return
	}
	for i, doc := range docs {
		if doc == nil || ceVanillaFiles[doc.root] == "" {
			continue
		}
		name, ok := missionPath(files[i].Name, false)
		if !ok {
			continue
		}
		if _, ok := registered[name]; ok || isVanillaCEFile(name, doc.root, coreDirs) {
			continue
		}
		batchProblem(files, reports, i, doc.elements[0].line, "warn", "дополнительный XML-файл не зарегистрирован в загруженном cfgeconomycore.xml", "добавьте <ce folder=\"…\"><file name=\"…\" type=\"…\"/></ce>, если этот файл должен загружаться экономикой")
	}
}

func isVanillaCEFile(name, root string, coreDirs []string) bool {
	standard := ceVanillaFiles[root]
	// A picker that supplies only a basename does not give enough evidence to
	// distinguish a default file from an unregistered custom directory.
	if name == standard || name == path.Base(standard) {
		return true
	}
	for _, dir := range coreDirs {
		if name == path.Join(dir, standard) {
			return true
		}
	}
	return false
}
