// Package analyze extracts item classes and their key gameplay parameters
// from parsed DayZ mod configs.
package analyze

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"dayzoverride/pbo"
	"dayzoverride/rap"
)

// Sections scanned for item classes.
var Sections = []string{"CfgVehicles", "CfgWeapons", "CfgMagazines", "CfgAmmo"}

// Logf receives human-readable progress lines (nil-safe via p).
type Logf func(format string, args ...interface{})

func (f Logf) p(format string, args ...interface{}) {
	if f != nil {
		f(format, args...)
	}
}

// ParamValue is one extracted parameter for the UI.
type ParamValue struct {
	Display string `json:"display"`       // human-readable value
	Source  string `json:"source"`        // classname where defined ("" if this class itself)
	Exists  bool   `json:"exists"`        // found anywhere in the in-mod chain
	Raw     string `json:"raw,omitempty"` // editable raw form
}

// Item is one class found in the mod.
type Item struct {
	ID          int                    `json:"id"`
	Classname   string                 `json:"classname"`
	Category    string                 `json:"category"`
	Parent      string                 `json:"parent"`
	Chain       []string               `json:"chain"`       // in-mod parent chain (excluding self)
	VanillaBase string                 `json:"vanillaBase"` // first parent NOT found in mod ("" if chain closed in-mod)
	Scope       *ParamValue            `json:"scope"`
	DisplayName string                 `json:"displayName"`
	Params      map[string]*ParamValue `json:"params"`
	PboName     string                 `json:"pboName"`
}

// Patch is one CfgPatches class of the analyzed mod.
type Patch struct {
	Name           string   `json:"name"`
	RequiredAddons []string `json:"requiredAddons"`
}

// PboInfo describes one analyzed PBO.
type PboInfo struct {
	FileName     string `json:"fileName"`
	Prefix       string `json:"prefix"`
	NumFiles     int    `json:"numFiles"`
	ConfigSrc    string `json:"configSrc"`    // "config.bin (raP)" / "config.cpp (текст)" / описание причины отсутствия
	Status       string `json:"status"`       // ok | resources | error
	ConfigsFound int    `json:"configsFound"` // сколько конфигов разобрано
	ItemClasses  int    `json:"itemClasses"`  // сколько редактируемых классов они дали
}

// Analysis is the full result returned to the UI.
type Analysis struct {
	Pbos      []PboInfo         `json:"pbos"`
	Patches   []Patch           `json:"patches"`
	Items     []*Item           `json:"items"`
	ConfigCPP map[string]string `json:"configCpp"` // pbo file name -> derapped config text
	Warnings  []string          `json:"warnings"`
}

// ---------- parameter registry (shared with generator) ----------

type ParamType int

const (
	PTFloat ParamType = iota
	PTInt
	PTIntPair  // {W,H}
	PTStrArray // {"a","b"}
)

type ParamDef struct {
	Key      string    `json:"key"`
	Label    string    `json:"label"`
	Path     []string  `json:"path"`
	Type     ParamType `json:"type"`
	Sections []string  `json:"sections,omitempty"` // empty = all
	// ExtraWrite: additional paths written with the same value on generation
	ExtraWrite [][]string `json:"-"`
	Hint       string     `json:"hint"`
}

var Registry = []ParamDef{
	{Key: "itemSize", Label: "Размер", Path: []string{"itemSize"}, Type: PTIntPair,
		Hint: "Размер в инвентаре, клетки: Ш,В — например 3,3"},
	{Key: "weight", Label: "Вес", Path: []string{"weight"}, Type: PTInt,
		Hint: "Вес в граммах"},
	{Key: "itemsCargoSize", Label: "Карго (слоты)", Path: []string{"itemsCargoSize"}, Type: PTIntPair,
		Hint: "Внутреннее хранилище, клетки: Ш,В — например 10,4. 0,0 = нет карго"},
	{Key: "attachments", Label: "Аттачменты", Path: []string{"attachments"}, Type: PTStrArray,
		Hint: "Список слотов через запятую, например: Shoulder,Melee,Vest"},
	{Key: "count", Label: "Кол-во (магазин)", Path: []string{"count"}, Type: PTInt, Sections: []string{"CfgMagazines"},
		Hint: "Ёмкость магазина / размер стака"},
	{Key: "varQuantityMax", Label: "Кол-во (макс)", Path: []string{"varQuantityMax"}, Type: PTInt,
		Sections: []string{"CfgVehicles"},
		Hint:     "Максимальное количество (стак, ёмкость и т.п.)"},
	{Key: "hitpoints", Label: "ХП", Path: []string{"DamageSystem", "GlobalHealth", "Health", "hitpoints"}, Type: PTFloat,
		Hint: "Прочность предмета (hitpoints)"},
	{Key: "armorProjectile", Label: "Броня: пуля", Path: []string{"DamageSystem", "GlobalArmor", "Projectile", "Health", "damage"}, Type: PTFloat,
		ExtraWrite: [][]string{{"DamageSystem", "GlobalArmor", "Projectile", "Blood", "damage"}, {"DamageSystem", "GlobalArmor", "Projectile", "Shock", "damage"}},
		Hint:       "Множитель урона от пуль (0..1, меньше = крепче броня). Записывается в Health/Blood/Shock"},
	{Key: "armorMelee", Label: "Броня: ближ. бой", Path: []string{"DamageSystem", "GlobalArmor", "Melee", "Health", "damage"}, Type: PTFloat,
		ExtraWrite: [][]string{{"DamageSystem", "GlobalArmor", "Melee", "Blood", "damage"}, {"DamageSystem", "GlobalArmor", "Melee", "Shock", "damage"}},
		Hint:       "Множитель урона в ближнем бою (0..1). Записывается в Health/Blood/Shock"},
	{Key: "armorFrag", Label: "Броня: осколки", Path: []string{"DamageSystem", "GlobalArmor", "FragGrenade", "Health", "damage"}, Type: PTFloat,
		ExtraWrite: [][]string{{"DamageSystem", "GlobalArmor", "FragGrenade", "Blood", "damage"}, {"DamageSystem", "GlobalArmor", "FragGrenade", "Shock", "damage"}},
		Hint:       "Множитель урона от осколков (0..1). Записывается в Health/Blood/Shock"},
	{Key: "armorInfected", Label: "Броня: зомби", Path: []string{"DamageSystem", "GlobalArmor", "Infected", "Health", "damage"}, Type: PTFloat,
		ExtraWrite: [][]string{{"DamageSystem", "GlobalArmor", "Infected", "Blood", "damage"}, {"DamageSystem", "GlobalArmor", "Infected", "Shock", "damage"}},
		Hint:       "Множитель урона от заражённых (0..1). Записывается в Health/Blood/Shock"},
	{Key: "protBiological", Label: "Защита: био", Path: []string{"Protection", "biological"}, Type: PTFloat,
		Hint: "Защита от биологической заразы (0..1, 1 = полная)"},
	{Key: "protChemical", Label: "Защита: хим", Path: []string{"Protection", "chemical"}, Type: PTFloat,
		Hint: "Защита от химического заражения (0..1, 1 = полная)"},
	{Key: "dmgHealth", Label: "Урон: ХП", Path: []string{"DamageApplied", "Health", "damage"}, Type: PTFloat, Sections: []string{"CfgAmmo"},
		Hint: "Урон здоровью (боеприпас)"},
	{Key: "dmgBlood", Label: "Урон: кровь", Path: []string{"DamageApplied", "Blood", "damage"}, Type: PTFloat, Sections: []string{"CfgAmmo"},
		Hint: "Урон крови (боеприпас)"},
	{Key: "dmgShock", Label: "Урон: шок", Path: []string{"DamageApplied", "Shock", "damage"}, Type: PTFloat, Sections: []string{"CfgAmmo"},
		Hint: "Урон шоку (боеприпас)"},
}

func RegistryByKey(key string) *ParamDef {
	for i := range Registry {
		if Registry[i].Key == key {
			return &Registry[i]
		}
	}
	return nil
}

// ---------- class index & inheritance ----------

type classRec struct {
	cls     *rap.Class
	section string
	pboName string
}

type index struct {
	// section (lower) -> classname (lower) -> record
	bySection map[string]map[string]*classRec
}

func newIndex() *index {
	return &index{bySection: map[string]map[string]*classRec{}}
}

func (ix *index) add(section string, c *rap.Class, pboName string) {
	s := strings.ToLower(section)
	m := ix.bySection[s]
	if m == nil {
		m = map[string]*classRec{}
		ix.bySection[s] = m
	}
	m[strings.ToLower(c.Name)] = &classRec{cls: c, section: section, pboName: pboName}
}

func (ix *index) get(section, name string) *classRec {
	m := ix.bySection[strings.ToLower(section)]
	if m == nil {
		return nil
	}
	return m[strings.ToLower(name)]
}

// chain returns the class itself plus in-mod ancestors, and the first
// unresolved (vanilla) parent name.
func (ix *index) chain(section string, c *rap.Class) (chain []*rap.Class, vanillaBase string) {
	chain = append(chain, c)
	seen := map[string]bool{strings.ToLower(c.Name): true}
	cur := c
	for cur.Parent != "" {
		key := strings.ToLower(cur.Parent)
		if seen[key] {
			break // cycle
		}
		rec := ix.get(section, cur.Parent)
		if rec == nil {
			return chain, cur.Parent
		}
		seen[key] = true
		chain = append(chain, rec.cls)
		cur = rec.cls
	}
	return chain, ""
}

// findPath looks for a nested path (classes... leaf) inside a single class.
func findPath(c *rap.Class, path []string) *rap.Entry {
	cur := c
	for i := 0; i < len(path)-1; i++ {
		cur = cur.Sub(path[i])
		if cur == nil {
			return nil
		}
	}
	return cur.Get(path[len(path)-1])
}

// resolve walks the in-mod chain and returns the first definition of path.
func resolve(chain []*rap.Class, path []string) (*rap.Entry, string) {
	for i, c := range chain {
		if e := findPath(c, path); e != nil {
			src := ""
			if i > 0 {
				src = c.Name
			}
			return e, src
		}
	}
	return nil, ""
}

// ---------- config loading (общий для анализа, сверки и слияния) ----------

// ParsedConfig is one successfully parsed config with its source PBO name.
type ParsedConfig struct {
	Root    *rap.Class
	PboName string
}

const (
	maxConfigEntry = 64 << 20 // конфиг больше 64 МБ — почти наверняка мусор/обфускация
	maxViewerLen   = 2 << 20  // обрезка текста для просмотра
)

// LoadConfigs opens each path (PBO или текстовый config.cpp) and extracts
// parsed configs. Каждый файл разбирается изолированно: ошибка одного
// становится предупреждением и НЕ прерывает разбор остальных.
func LoadConfigs(paths []string, logf Logf) (configs []ParsedConfig, pbos []PboInfo, warnings []string) {
	logf.p("Файлов к разбору: %d", len(paths))
	for _, path := range paths {
		func() {
			defer func() {
				if r := recover(); r != nil {
					msg := fmt.Sprintf("%s: внутренний сбой разбора (%v) — файл пропущен; вероятно, файл повреждён или обфусцирован", filepath.Base(path), r)
					warnings = append(warnings, msg)
					logf.p("КРИТИЧЕСКАЯ ОШИБКА: %s", msg)
				}
			}()
			loadOne(path, &configs, &pbos, &warnings, logf)
		}()
	}
	if len(configs) == 0 {
		logf.p("ИТОГ: ни в одном файле не найдено читаемых конфигов — смотрите предупреждения выше")
	}
	return
}

// AnalyzePBOs opens each PBO path, extracts configs and builds the Analysis.
// logf (может быть nil) получает строки прогресса для живого журнала.
func AnalyzePBOs(paths []string, logf Logf) (*Analysis, error) {
	res := &Analysis{ConfigCPP: map[string]string{}}
	configs, pbos, warns := LoadConfigs(paths, logf)
	res.Pbos = pbos
	res.Warnings = warns
	for _, pc := range configs {
		if _, ok := res.ConfigCPP[pc.PboName]; !ok {
			text := rap.ToCPP(pc.Root)
			if len(text) > maxViewerLen {
				text = text[:maxViewerLen] + "\n\n// … (обрезано для просмотра: конфиг слишком большой)"
			}
			res.ConfigCPP[pc.PboName] = text
		}
	}

	// build index across all configs
	logf.p("Строю индекс классов и разбираю наследование…")
	ix := newIndex()
	secTotals := map[string]int{}
	for _, pc := range configs {
		for _, secName := range Sections {
			sec := pc.Root.Sub(secName)
			if sec == nil {
				continue
			}
			for _, ce := range sec.Classes() {
				ix.add(secName, ce.Class, pc.PboName)
				secTotals[secName]++
			}
		}
		// CfgPatches
		if cp := pc.Root.Sub("CfgPatches"); cp != nil {
			for _, pe := range cp.Classes() {
				patch := Patch{Name: pe.Name}
				for _, ra := range pe.Class.ArrayProp("requiredAddons") {
					patch.RequiredAddons = append(patch.RequiredAddons, ra.String())
				}
				res.Patches = append(res.Patches, patch)
			}
		}
	}
	for _, s := range Sections {
		if secTotals[s] > 0 {
			logf.p("%s: всего %d классов", s, secTotals[s])
		}
	}
	logf.p("CfgPatches (аддоны мода, пойдут в requiredAddons): %d", len(res.Patches))

	// extract items in stable order
	logf.p("Извлекаю параметры предметов (размер, слоты, ХП, броня, защита)…")
	id := 0
	for _, secName := range Sections {
		m := ix.bySection[strings.ToLower(secName)]
		if m == nil {
			continue
		}
		var names []string
		for _, rec := range m {
			names = append(names, rec.cls.Name)
		}
		sort.Slice(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })
		for _, name := range names {
			rec := m[strings.ToLower(name)]
			item := buildItem(ix, secName, rec)
			item.ID = id
			id++
			res.Items = append(res.Items, item)
		}
	}
	logf.p("Анализ завершён: %d предметов, %d предупреждений", len(res.Items), len(res.Warnings))
	return res, nil
}

func loadOne(path string, configs *[]ParsedConfig, pbos *[]PboInfo, warnings *[]string, logf Logf) {
	fileName := filepath.Base(path)

	// текстовый config.cpp/hpp напрямую (для слияния оверрайдов и сверки)
	lowExt := strings.ToLower(filepath.Ext(path))
	if lowExt == ".cpp" || lowExt == ".hpp" || lowExt == ".txt" {
		logf.p("Читаю текстовый конфиг: %s", fileName)
		data, err := os.ReadFile(path)
		if err != nil {
			*warnings = append(*warnings, fmt.Sprintf("%s: %v", fileName, err))
			logf.p("ОШИБКА: %s: %v", fileName, err)
			return
		}
		root, warns, err := rap.ParseText(rap.CleanString(data))
		for _, w := range warns {
			*warnings = append(*warnings, fmt.Sprintf("%s: %s", fileName, w))
		}
		if err != nil {
			*warnings = append(*warnings, fmt.Sprintf("%s: %v", fileName, err))
			logf.p("ОШИБКА разбора %s: %v", fileName, err)
			return
		}
		*configs = append(*configs, ParsedConfig{Root: root, PboName: fileName})
		*pbos = append(*pbos, PboInfo{FileName: fileName, ConfigSrc: "config.cpp (текст)", Status: "ok", NumFiles: 1})
		logf.p("Конфиг разобран: %s", fileName)
		return
	}

	if st, err := os.Stat(path); err == nil {
		logf.p("Открываю PBO: %s (%.1f МБ)", fileName, float64(st.Size())/1e6)
	} else {
		logf.p("Открываю PBO: %s", fileName)
	}
	p, err := pbo.Open(path)
	if err != nil {
		msg := fmt.Sprintf("%s: не удалось разобрать PBO: %v. Вероятно, PBO зашифрован/обфусцирован — конфиги из него прочитать нельзя. Остальные файлы разбираются дальше.", fileName, err)
		*warnings = append(*warnings, msg)
		*pbos = append(*pbos, PboInfo{FileName: fileName, ConfigSrc: "не читается (шифрование/обфускация?)", Status: "error"})
		logf.p("ПРОПУСКАЮ %s: %v — возможно, защита/обфускация. Продолжаю со следующим файлом.", fileName, err)
		return
	}
	defer p.Close()

	info := PboInfo{FileName: fileName, Prefix: p.Prefix(), NumFiles: len(p.Entries), Status: "ok"}
	logf.p("Заголовок прочитан: %d файлов внутри, префикс: %s", len(p.Entries), orDash(info.Prefix))
	if info.Prefix == "" {
		info.Prefix = strings.TrimSuffix(fileName, filepath.Ext(fileName))
		*warnings = append(*warnings, fmt.Sprintf("%s: свойство prefix не найдено, использую имя файла", fileName))
		logf.p("Свойство prefix отсутствует — использую имя файла: %s", info.Prefix)
	}

	found := 0
	for i := range p.Entries {
		e := &p.Entries[i]
		base := strings.ToLower(filepath.Base(strings.ReplaceAll(e.Name, "\\", "/")))
		isBin := strings.HasSuffix(base, ".bin")
		isCpp := base == "config.cpp"
		if !isBin && !isCpp {
			continue
		}
		if e.DataSize > maxConfigEntry {
			*warnings = append(*warnings, fmt.Sprintf("%s / %s: запись подозрительно велика (%.0f МБ) — пропускаю", fileName, e.Name, float64(e.DataSize)/1e6))
			logf.p("ПРЕДУПРЕЖДЕНИЕ: %s слишком велик (%.0f МБ) — пропускаю", e.Name, float64(e.DataSize)/1e6)
			continue
		}
		data, err := p.ReadEntry(e)
		if err != nil {
			*warnings = append(*warnings, fmt.Sprintf("%s / %s: %v", fileName, e.Name, err))
			logf.p("ПРЕДУПРЕЖДЕНИЕ: %s: %v", e.Name, err)
			continue
		}
		var root *rap.Class
		if !rap.IsBinarized(data) && isBin && strings.EqualFold(base, "config.bin") {
			// Mikero может сжимать config.bin (LZSS) — пробуем распаковать
			if dec, ok := rap.TryDecompressRaP(data); ok {
				logf.p("Конфиг %s сжат (LZSS) — распакован: %.1f КБ -> %.1f КБ", e.Name, float64(len(data))/1024, float64(len(dec))/1024)
				data = dec
			}
		}
		if rap.IsBinarized(data) {
			root, err = rap.ParseBin(data)
			if err != nil {
				*warnings = append(*warnings, fmt.Sprintf("%s / %s: %v", fileName, e.Name, err))
				logf.p("ОШИБКА дебинаризации %s (%.1f КБ, raP): %v — пропускаю этот конфиг", e.Name, float64(len(data))/1024, err)
				continue
			}
			if found == 0 {
				info.ConfigSrc = "config.bin (raP)"
			}
		} else if isCpp {
			var warns []string
			root, warns, err = rap.ParseText(rap.CleanString(data))
			for _, w := range warns {
				*warnings = append(*warnings, fmt.Sprintf("%s / %s: %s", fileName, e.Name, w))
			}
			if err != nil {
				*warnings = append(*warnings, fmt.Sprintf("%s / %s: %v", fileName, e.Name, err))
				logf.p("ОШИБКА разбора %s (%.1f КБ): %v — пропускаю этот конфиг", e.Name, float64(len(data))/1024, err)
				continue
			}
			if found == 0 {
				info.ConfigSrc = "config.cpp (текст)"
			}
		} else {
			continue // non-raP .bin (например texHeaders.bin)
		}
		found++
		n, desc := describeConfig(root)
		info.ItemClasses += n
		logf.p("Разобран %s (%.1f КБ) — %s", e.Name, float64(len(data))/1024, desc)
		*configs = append(*configs, ParsedConfig{Root: root, PboName: fileName})
	}

	info.ConfigsFound = found
	if found > 0 {
		if info.ItemClasses > 0 {
			logf.p("✔ ИТОГ по %s: конфигов разобрано %d, редактируемых классов: %d — ЭТОТ PBO НУЖЕН для оверрайдов", fileName, found, info.ItemClasses)
		} else {
			logf.p("ИТОГ по %s: конфигов разобрано %d, но редактируемых предметов НЕТ (звуковые/служебные секции) — для оверрайдов этот PBO не нужен, это нормально", fileName, found)
		}
	}

	if found == 0 {
		kind := classifyResourcePbo(p)
		if kind != "" {
			info.ConfigSrc = "без конфигов: " + kind
			info.Status = "resources"
			logf.p("В %s конфигов нет — это %s. Это нормально для ган-паков, пропускаю.", fileName, kind)
		} else {
			info.ConfigSrc = "конфиг не найден"
			info.Status = "resources"
			*warnings = append(*warnings, fmt.Sprintf("%s: конфиг не найден (нет config.bin/config.cpp) — PBO с ресурсами или обфусцирован", fileName))
			logf.p("Конфиг в %s не найден — PBO с ресурсами или обфусцирован, пропускаю.", fileName)
		}
	}
	*pbos = append(*pbos, info)
}

// classifyResourcePbo describes what a config-less PBO contains.
func classifyResourcePbo(p *pbo.PBO) string {
	counts := map[string]int{}
	for i := range p.Entries {
		name := strings.ToLower(p.Entries[i].Name)
		switch {
		case strings.HasSuffix(name, ".ogg"), strings.HasSuffix(name, ".wss"), strings.HasSuffix(name, ".wav"):
			counts["звуки"]++
		case strings.HasSuffix(name, ".paa"), strings.HasSuffix(name, ".edds"), strings.HasSuffix(name, ".jpg"), strings.HasSuffix(name, ".png"):
			counts["текстуры"]++
		case strings.HasSuffix(name, ".p3d"):
			counts["модели"]++
		case strings.HasSuffix(name, ".c"), strings.HasSuffix(name, ".layout"), strings.HasSuffix(name, ".csv"):
			counts["скрипты"]++
		case strings.HasSuffix(name, ".rtm"):
			counts["анимации"]++
		}
	}
	if len(counts) == 0 {
		return ""
	}
	order := []string{"скрипты", "звуки", "текстуры", "модели", "анимации"}
	var parts []string
	for _, k := range order {
		if counts[k] > 0 {
			parts = append(parts, fmt.Sprintf("%s: %d", k, counts[k]))
		}
	}
	return "PBO с ресурсами (" + strings.Join(parts, ", ") + ")"
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// describeConfig сообщает, что внутри конфига: редактируемые предметы или
// только звуковые/служебные секции.
func describeConfig(root *rap.Class) (itemCount int, desc string) {
	var parts []string
	for _, s := range Sections {
		if sec := root.Sub(s); sec != nil {
			n := len(sec.Classes())
			if n > 0 {
				itemCount += n
				parts = append(parts, fmt.Sprintf("%s: %d", s, n))
			}
		}
	}
	if itemCount > 0 {
		return itemCount, "ПРЕДМЕТЫ (" + strings.Join(parts, ", ") + ")"
	}
	var others []string
	for _, e := range root.Entries {
		if e.Kind == rap.KindClass && !strings.EqualFold(e.Name, "CfgPatches") && !strings.EqualFold(e.Name, "CfgMods") {
			others = append(others, e.Name)
			if len(others) == 3 {
				break
			}
		}
	}
	if len(others) > 0 {
		return 0, "предметов нет, только служебные секции (" + strings.Join(others, ", ") + ")"
	}
	return 0, "предметов нет (только CfgPatches)"
}

func buildItem(ix *index, section string, rec *classRec) *Item {
	c := rec.cls
	chain, vanilla := ix.chain(section, c)
	item := &Item{
		Classname:   c.Name,
		Category:    section,
		Parent:      c.Parent,
		VanillaBase: vanilla,
		Params:      map[string]*ParamValue{},
		PboName:     rec.pboName,
	}
	for _, a := range chain[1:] {
		item.Chain = append(item.Chain, a.Name)
	}
	// scope
	if e, src := resolve(chain, []string{"scope"}); e != nil && e.Kind == rap.KindValue {
		item.Scope = &ParamValue{Display: e.Value.String(), Source: src, Exists: true, Raw: e.Value.String()}
	} else {
		item.Scope = &ParamValue{Display: "?", Exists: false}
	}
	// displayName
	if e, _ := resolve(chain, []string{"displayName"}); e != nil && e.Kind == rap.KindValue {
		item.DisplayName = e.Value.String()
	}
	// registry params
	for i := range Registry {
		def := &Registry[i]
		if len(def.Sections) > 0 && !contains(def.Sections, section) {
			continue
		}
		e, src := resolve(chain, def.Path)
		if e == nil {
			item.Params[def.Key] = &ParamValue{Exists: false}
			continue
		}
		pv := &ParamValue{Source: src, Exists: true}
		switch e.Kind {
		case rap.KindValue:
			pv.Display = e.Value.String()
			pv.Raw = e.Value.String()
		case rap.KindArray:
			av := &rap.Value{Kind: rap.VArray, Arr: e.Array}
			s := av.String()
			pv.Display = strings.TrimSuffix(strings.TrimPrefix(s, "{"), "}")
			pv.Raw = pv.Display
		}
		item.Params[def.Key] = pv
	}
	return item
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}
