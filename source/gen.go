// Package gen builds the override ("require") addon from user edits,
// optionally using external DayZ Tools (CfgConvert / FileBank / DSSignFile).
package gen

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"dayzoverride/analyze"
	"dayzoverride/pbo"
	"dayzoverride/rap"
	"dayzoverride/tools"
)

// Change is one parameter edit for one item.
type Change struct {
	Param string `json:"param"`          // registry key, or "custom"
	Value string `json:"value"`          // raw user input
	Path  string `json:"path,omitempty"` // for custom: "Protection/biological" or "weight"
	Type  string `json:"type,omitempty"` // for custom: float|int|string|raw|intpair|strarray
}

// ItemReq is one item selected for the override.
type ItemReq struct {
	Classname string   `json:"classname"`
	Category  string   `json:"category"`
	Parent    string   `json:"parent"`
	Changes   []Change `json:"changes"`
}

// Request is the full generation request from the UI.
type Request struct {
	AddonName      string    `json:"addonName"`
	RequiredAddons []string  `json:"requiredAddons"`
	MakePbo        bool      `json:"makePbo"`
	Items          []ItemReq `json:"items"`

	// DayZ Tools options (all optional)
	ToolsPath      string `json:"toolsPath,omitempty"`
	Sign           bool   `json:"sign,omitempty"`
	PrivateKeyPath string `json:"privateKeyPath,omitempty"`
	Binarize       bool   `json:"binarize,omitempty"`
	UseFileBank    bool   `json:"useFileBank,omitempty"`
}

// OutFile is one generated file (relative path inside the package).
type OutFile struct {
	Path string
	Data []byte
}

// Result of generation.
type Result struct {
	ConfigCPP string
	Readme    string
	Files     []OutFile
	Warnings  []string
	ZipData   []byte
	FileList  []string
}

var classnameRe = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// ValidateAddonName checks the addon/classname.
func ValidateAddonName(name string) error {
	if name == "" {
		return fmt.Errorf("имя аддона пустое")
	}
	if !classnameRe.MatchString(name) {
		return fmt.Errorf("имя аддона может содержать только латиницу, цифры и _")
	}
	if name[0] >= '0' && name[0] <= '9' {
		return fmt.Errorf("имя аддона не должно начинаться с цифры")
	}
	return nil
}

// Logf receives human-readable progress lines (nil-safe via p).
type Logf func(format string, args ...interface{})

func (f Logf) p(format string, args ...interface{}) {
	if f != nil {
		f(format, args...)
	}
}

// Generate builds config.cpp, runs the packaging pipeline and fills the ZIP.
// logf (может быть nil) получает строки прогресса для живого журнала.
func Generate(req *Request, logf Logf) (*Result, error) {
	if err := ValidateAddonName(req.AddonName); err != nil {
		return nil, err
	}
	if len(req.Items) == 0 {
		return nil, fmt.Errorf("не выбран ни один предмет с изменениями")
	}
	nChanges := 0
	for _, it := range req.Items {
		nChanges += len(it.Changes)
	}
	logf.p("Строю config.cpp: аддон %s, предметов: %d, правок: %d", req.AddonName, len(req.Items), nChanges)
	cfg, err := buildConfig(req)
	if err != nil {
		logf.p("ОШИБКА построения конфига: %v", err)
		return nil, err
	}
	logf.p("config.cpp готов (%d строк, %.1f КБ)", strings.Count(cfg, "\n")+1, float64(len(cfg))/1024)
	return PackageConfig(req, cfg, logf)
}

// PackageConfig runs the packaging pipeline (binarize -> pack -> sign -> zip)
// for an уже готового config.cpp. Используется и генератором, и слиянием.
func PackageConfig(req *Request, cfg string, logf Logf) (*Result, error) {
	if err := ValidateAddonName(req.AddonName); err != nil {
		return nil, err
	}
	res := &Result{ConfigCPP: cfg}

	if req.Sign && !req.MakePbo {
		req.MakePbo = true
		res.Warnings = append(res.Warnings, "Для подписи нужен PBO — сборка PBO включена автоматически.")
		logf.p("Для подписи нужен PBO — включаю сборку PBO автоматически")
	}

	// external tools, when requested
	var ts *tools.Toolset
	if req.Sign || req.Binarize || req.UseFileBank {
		logf.p("Сканирую папку DayZ Tools: %s", req.ToolsPath)
		var terr error
		ts, _, terr = tools.Detect(req.ToolsPath)
		if terr != nil {
			logf.p("ОШИБКА: %v", terr)
			return nil, fmt.Errorf("нужны DayZ Tools (подпись/бинаризация/FileBank): %v", terr)
		}
		logf.p("DayZ Tools найдены")
	}

	base := req.AddonName
	// always ship the text source and readme
	if req.MakePbo {
		if err := buildPboFiles(req, res, ts, logf); err != nil {
			logf.p("ОШИБКА сборки: %v", err)
			return nil, err
		}
	}
	res.Readme = readmeText(req, res)
	res.Files = append(res.Files,
		OutFile{Path: base + "/README_RU.txt", Data: []byte(res.Readme)},
		OutFile{Path: base + "/src/" + base + "/config.cpp", Data: []byte(cfg)},
	)

	// stable order: readme, src, then mod files
	sort.SliceStable(res.Files, func(i, j int) bool { return res.Files[i].Path < res.Files[j].Path })

	// zip
	logf.p("Формирую ZIP-архив (%d файлов)…", len(res.Files))
	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	for _, f := range res.Files {
		w, err := zw.Create(f.Path)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(f.Data); err != nil {
			return nil, err
		}
		res.FileList = append(res.FileList, f.Path)
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	res.ZipData = zbuf.Bytes()
	logf.p("Сборка пакета завершена: ZIP %.1f КБ", float64(len(res.ZipData))/1024)
	return res, nil
}

// buildPboFiles runs the packaging pipeline (binarize -> pack -> sign) and
// appends @Addon files to the result.
func buildPboFiles(req *Request, res *Result, ts *tools.Toolset, logf Logf) error {
	base := req.AddonName
	cfgData := []byte(res.ConfigCPP)
	cfgName := "config.cpp"

	needTemp := req.Binarize || req.UseFileBank || req.Sign
	var work string
	if needTemp {
		var err error
		work, err = os.MkdirTemp("", "dob-build-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(work)
	}

	// 1) binarize config via CfgConvert
	if req.Binarize {
		t, _ := ts.Get("cfgconvert")
		logf.p("Бинаризация: запускаю CfgConvert (%s)…", t.Path)
		srcDir := filepath.Join(work, "src")
		if err := os.MkdirAll(srcDir, 0755); err != nil {
			return err
		}
		srcCpp := filepath.Join(srcDir, "config.cpp")
		dstBin := filepath.Join(srcDir, "config.bin")
		if err := os.WriteFile(srcCpp, cfgData, 0644); err != nil {
			return err
		}
		if _, err := tools.ConvertToBin(ts, srcCpp, dstBin); err != nil {
			return fmt.Errorf("бинаризация: %w", err)
		}
		binData, err := os.ReadFile(dstBin)
		if err != nil {
			return err
		}
		cfgData = binData
		cfgName = "config.bin"
		logf.p("config.bin получен (%.1f КБ)", float64(len(binData))/1024)
	}

	// 2) pack PBO
	var pboData []byte
	if req.UseFileBank {
		t, _ := ts.Get("filebank")
		logf.p("Упаковка: запускаю FileBank (%s)…", t.Path)
		fbSrc := filepath.Join(work, base)
		if err := os.MkdirAll(fbSrc, 0755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(fbSrc, cfgName), cfgData, 0644); err != nil {
			return err
		}
		outDir := filepath.Join(work, "out")
		pboPath, _, err := tools.FileBankPack(ts, fbSrc, outDir, base)
		if err != nil {
			return fmt.Errorf("упаковка FileBank: %w", err)
		}
		pboData, err = os.ReadFile(pboPath)
		if err != nil {
			return err
		}
	} else {
		logf.p("Упаковка PBO встроенным упаковщиком (prefix=%s, %s внутри)…", base, cfgName)
		pboData = pbo.Build(base, []pbo.File{{Name: cfgName, Data: cfgData}})
	}
	logf.p("PBO готов: %s.pbo (%.1f КБ)", base, float64(len(pboData))/1024)

	pboRel := base + "/@" + base + "/addons/" + base + ".pbo"
	res.Files = append(res.Files, OutFile{Path: pboRel, Data: pboData})

	// 3) sign via DSSignFile
	if req.Sign {
		keyName := strings.TrimSuffix(filepath.Base(req.PrivateKeyPath), filepath.Ext(req.PrivateKeyPath))
		logf.p("Подпись: запускаю DSSignFile, ключ %s…", keyName)
		signDir := filepath.Join(work, "sign")
		if err := os.MkdirAll(signDir, 0755); err != nil {
			return err
		}
		pboOnDisk := filepath.Join(signDir, base+".pbo")
		if err := os.WriteFile(pboOnDisk, pboData, 0644); err != nil {
			return err
		}
		bisignPath, _, err := tools.SignPbo(ts, req.PrivateKeyPath, pboOnDisk)
		if err != nil {
			return fmt.Errorf("подпись: %w", err)
		}
		bisignData, err := os.ReadFile(bisignPath)
		if err != nil {
			return err
		}
		logf.p("Подпись создана: %s", filepath.Base(bisignPath))
		res.Files = append(res.Files, OutFile{
			Path: base + "/@" + base + "/addons/" + filepath.Base(bisignPath),
			Data: bisignData,
		})
		if pub := tools.SiblingBikey(req.PrivateKeyPath); pub != "" {
			pubData, err := os.ReadFile(pub)
			if err == nil {
				logf.p("Публичный ключ добавлен: keys/%s", filepath.Base(pub))
				res.Files = append(res.Files, OutFile{
					Path: base + "/@" + base + "/keys/" + filepath.Base(pub),
					Data: pubData,
				})
			}
		} else {
			res.Warnings = append(res.Warnings,
				"Публичный ключ .bikey не найден рядом с приватным — положите его в папку keys сервера вручную.")
			logf.p("ПРЕДУПРЕЖДЕНИЕ: .bikey не найден рядом с приватным ключом")
		}
	}
	return nil
}

// SaveToDir writes the generated package into a directory on disk.
func SaveToDir(req *Request, res *Result, dir string) (string, error) {
	dir = strings.TrimSpace(strings.Trim(dir, `"`))
	if dir == "" {
		return "", fmt.Errorf("не указана папка для сохранения")
	}
	for _, f := range res.Files {
		full := filepath.Join(dir, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			return "", err
		}
		if err := os.WriteFile(full, f.Data, 0644); err != nil {
			return "", err
		}
	}
	return filepath.Join(dir, req.AddonName), nil
}

// ---------- config building (unchanged core) ----------

func buildConfig(req *Request) (string, error) {
	root := &rap.Class{}

	patches := &rap.Class{Name: "CfgPatches"}
	me := &rap.Class{Name: req.AddonName}
	me.Entries = append(me.Entries,
		&rap.Entry{Kind: rap.KindArray, Name: "units", Array: nil},
		&rap.Entry{Kind: rap.KindArray, Name: "weapons", Array: nil},
		&rap.Entry{Kind: rap.KindValue, Name: "requiredVersion", Value: &rap.Value{Kind: rap.VFloat, F: 0.1}},
	)
	var reqArr []*rap.Value
	seen := map[string]bool{}
	for _, ra := range req.RequiredAddons {
		ra = strings.TrimSpace(ra)
		if ra == "" || seen[strings.ToLower(ra)] {
			continue
		}
		seen[strings.ToLower(ra)] = true
		reqArr = append(reqArr, &rap.Value{Kind: rap.VString, Str: ra})
	}
	if len(reqArr) == 0 {
		reqArr = append(reqArr, &rap.Value{Kind: rap.VString, Str: "DZ_Data"})
	}
	me.Entries = append(me.Entries, &rap.Entry{Kind: rap.KindArray, Name: "requiredAddons", Array: reqArr})
	patches.Entries = append(patches.Entries, &rap.Entry{Kind: rap.KindClass, Name: me.Name, Class: me})
	root.Entries = append(root.Entries, &rap.Entry{Kind: rap.KindClass, Name: patches.Name, Class: patches})

	bySection := map[string][]ItemReq{}
	for _, it := range req.Items {
		if !classnameRe.MatchString(it.Classname) {
			return "", fmt.Errorf("некорректный класснейм: %q", it.Classname)
		}
		if it.Parent != "" && !classnameRe.MatchString(it.Parent) {
			return "", fmt.Errorf("некорректный родитель у %s: %q", it.Classname, it.Parent)
		}
		if len(it.Changes) == 0 {
			continue
		}
		bySection[it.Category] = append(bySection[it.Category], it)
	}

	for _, secName := range analyze.Sections {
		items := bySection[secName]
		if len(items) == 0 {
			continue
		}
		sec := &rap.Class{Name: secName}
		ordered := topoSort(items)
		overridden := map[string]bool{}
		for _, it := range ordered {
			overridden[strings.ToLower(it.Classname)] = true
		}
		externSeen := map[string]bool{}
		for _, it := range ordered {
			p := strings.ToLower(it.Parent)
			if it.Parent == "" || overridden[p] || externSeen[p] {
				continue
			}
			externSeen[p] = true
			sec.Entries = append(sec.Entries, &rap.Entry{Kind: rap.KindExtern, Name: it.Parent})
		}
		for _, it := range ordered {
			cls, err := buildItemClass(it)
			if err != nil {
				return "", err
			}
			sec.Entries = append(sec.Entries, &rap.Entry{Kind: rap.KindClass, Name: cls.Name, Class: cls})
		}
		root.Entries = append(root.Entries, &rap.Entry{Kind: rap.KindClass, Name: secName, Class: sec})
	}

	header := "////////////////////////////////////////////////////////////////////\n" +
		"// Override-аддон, сгенерирован DayZ Override Builder\n" +
		"// Перекрывает параметры предметов исходного мода через requiredAddons\n" +
		"////////////////////////////////////////////////////////////////////\n\n"
	return header + rap.ToCPP(root), nil
}

func topoSort(items []ItemReq) []ItemReq {
	byName := map[string]int{}
	for i, it := range items {
		byName[strings.ToLower(it.Classname)] = i
	}
	visited := make([]int, len(items))
	var out []ItemReq
	var visit func(i int)
	visit = func(i int) {
		if visited[i] != 0 {
			return
		}
		visited[i] = 1
		if pi, ok := byName[strings.ToLower(items[i].Parent)]; ok && visited[pi] == 0 {
			visit(pi)
		}
		visited[i] = 2
		out = append(out, items[i])
	}
	idx := make([]int, len(items))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool {
		return strings.ToLower(items[idx[a]].Classname) < strings.ToLower(items[idx[b]].Classname)
	})
	for _, i := range idx {
		visit(i)
	}
	return out
}

func buildItemClass(it ItemReq) (*rap.Class, error) {
	cls := &rap.Class{Name: it.Classname, Parent: it.Parent}
	for _, ch := range it.Changes {
		var paths [][]string
		var typ string
		if ch.Param == "custom" {
			p := strings.Trim(strings.ReplaceAll(ch.Path, "\\", "/"), "/ ")
			if p == "" {
				return nil, fmt.Errorf("%s: пустой путь у произвольного параметра", it.Classname)
			}
			segs := strings.Split(p, "/")
			for _, s := range segs {
				if !classnameRe.MatchString(s) {
					return nil, fmt.Errorf("%s: некорректный сегмент пути %q", it.Classname, s)
				}
			}
			paths = [][]string{segs}
			typ = ch.Type
			if typ == "" {
				typ = "raw"
			}
		} else {
			def := analyze.RegistryByKey(ch.Param)
			if def == nil {
				return nil, fmt.Errorf("%s: неизвестный параметр %q", it.Classname, ch.Param)
			}
			paths = [][]string{def.Path}
			paths = append(paths, def.ExtraWrite...)
			switch def.Type {
			case analyze.PTFloat:
				typ = "float"
			case analyze.PTInt:
				typ = "int"
			case analyze.PTIntPair:
				typ = "intpair"
			case analyze.PTStrArray:
				typ = "strarray"
			}
		}
		entry, err := makeEntry(typ, ch.Value)
		if err != nil {
			return nil, fmt.Errorf("%s / %s: %w", it.Classname, ch.Param, err)
		}
		for _, path := range paths {
			e := cloneEntry(entry)
			e.Name = path[len(path)-1]
			insertPath(cls, path[:len(path)-1], e)
		}
	}
	return cls, nil
}

func cloneEntry(e *rap.Entry) *rap.Entry {
	c := *e
	return &c
}

func insertPath(cls *rap.Class, classPath []string, leaf *rap.Entry) {
	cur := cls
	for _, seg := range classPath {
		next := cur.Sub(seg)
		if next == nil {
			next = &rap.Class{Name: seg}
			cur.Entries = append(cur.Entries, &rap.Entry{Kind: rap.KindClass, Name: seg, Class: next})
		}
		cur = next
	}
	for i, e := range cur.Entries {
		if strings.EqualFold(e.Name, leaf.Name) {
			cur.Entries[i] = leaf
			return
		}
	}
	cur.Entries = append(cur.Entries, leaf)
}

func makeEntry(typ, raw string) (*rap.Entry, error) {
	raw = strings.TrimSpace(raw)
	switch typ {
	case "float":
		f, err := strconv.ParseFloat(strings.ReplaceAll(raw, ",", "."), 64)
		if err != nil {
			return nil, fmt.Errorf("ожидалось число, получено %q", raw)
		}
		return &rap.Entry{Kind: rap.KindValue, Value: &rap.Value{Kind: rap.VFloat, F: f}}, nil
	case "int":
		f, err := strconv.ParseFloat(strings.ReplaceAll(raw, ",", "."), 64)
		if err != nil {
			return nil, fmt.Errorf("ожидалось целое число, получено %q", raw)
		}
		return &rap.Entry{Kind: rap.KindValue, Value: &rap.Value{Kind: rap.VInt, I: int64(f)}}, nil
	case "string":
		return &rap.Entry{Kind: rap.KindValue, Value: &rap.Value{Kind: rap.VString, Str: raw}}, nil
	case "intpair":
		parts := splitList(raw)
		if len(parts) != 2 {
			return nil, fmt.Errorf("ожидались два числа через запятую (Ш,В), получено %q", raw)
		}
		var arr []*rap.Value
		for _, p := range parts {
			n, err := strconv.ParseFloat(p, 64)
			if err != nil {
				return nil, fmt.Errorf("ожидалось число, получено %q", p)
			}
			arr = append(arr, &rap.Value{Kind: rap.VInt, I: int64(n)})
		}
		return &rap.Entry{Kind: rap.KindArray, Array: arr}, nil
	case "strarray":
		parts := splitList(raw)
		var arr []*rap.Value
		for _, p := range parts {
			p = strings.Trim(p, `"' `)
			if p == "" {
				continue
			}
			arr = append(arr, &rap.Value{Kind: rap.VString, Str: p})
		}
		return &rap.Entry{Kind: rap.KindArray, Array: arr}, nil
	case "raw":
		if raw == "" {
			return nil, fmt.Errorf("пустое значение")
		}
		if strings.HasPrefix(raw, "{") {
			src := "x[] = " + raw + ";"
			cls, _, err := rap.ParseText(src)
			if err != nil || len(cls.Entries) == 0 || cls.Entries[0].Kind != rap.KindArray {
				return nil, fmt.Errorf("не удалось разобрать массив %q", raw)
			}
			return &rap.Entry{Kind: rap.KindArray, Array: cls.Entries[0].Array}, nil
		}
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
			return &rap.Entry{Kind: rap.KindValue, Value: &rap.Value{Kind: rap.VInt, I: n}}, nil
		}
		if f, err := strconv.ParseFloat(strings.ReplaceAll(raw, ",", "."), 64); err == nil {
			return &rap.Entry{Kind: rap.KindValue, Value: &rap.Value{Kind: rap.VFloat, F: f}}, nil
		}
		return &rap.Entry{Kind: rap.KindValue, Value: &rap.Value{Kind: rap.VString, Str: strings.Trim(raw, `"`)}}, nil
	}
	return nil, fmt.Errorf("неизвестный тип значения %q", typ)
}

func splitList(raw string) []string {
	raw = strings.Trim(strings.TrimSpace(raw), "{}")
	var out []string
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func readmeText(req *Request, res *Result) string {
	var sb strings.Builder
	sb.WriteString("DayZ Override Builder — сгенерированный override-аддон \"" + req.AddonName + "\"\n")
	sb.WriteString(strings.Repeat("=", 70) + "\n\n")
	sb.WriteString("ЧТО ЭТО\n" +
		"Аддон перекрывает параметры предметов исходного мода. В CfgPatches прописан\n" +
		"requiredAddons со ссылками на аддоны исходного мода, поэтому игра гарантированно\n" +
		"загрузит этот аддон ПОСЛЕ исходного, и его значения будут иметь приоритет.\n\n")

	sb.WriteString("СОДЕРЖИМОЕ\n")
	sb.WriteString("  src\\" + req.AddonName + "\\config.cpp — исходник аддона (текст)\n")
	if req.MakePbo {
		cfgKind := "config.cpp (текст)"
		if req.Binarize {
			cfgKind = "config.bin (бинаризован CfgConvert)"
		}
		packer := "встроенным упаковщиком"
		if req.UseFileBank {
			packer = "официальным FileBank"
		}
		sb.WriteString("  @" + req.AddonName + "\\addons\\" + req.AddonName + ".pbo — готовый PBO (" + cfgKind + ", собран " + packer + ")\n")
		if req.Sign {
			sb.WriteString("  @" + req.AddonName + "\\addons\\*.bisign — подпись (DSSignFile)\n")
			sb.WriteString("  @" + req.AddonName + "\\keys\\*.bikey — публичный ключ для папки keys сервера\n")
		}
	}
	sb.WriteString("  README_RU.txt — эта инструкция\n\n")

	if req.Sign {
		sb.WriteString("УСТАНОВКА (аддон уже подписан)\n" +
			"1. Папку @" + req.AddonName + " положите на сервер и клиентам.\n" +
			"2. Файл .bikey из @" + req.AddonName + "\\keys скопируйте в папку keys СЕРВЕРА.\n" +
			"3. Добавьте в параметры запуска: -mod=...;@ИсходныйМод;@" + req.AddonName + "\n" +
			"   Порядок в -mod не критичен — requiredAddons сам обеспечит порядок загрузки.\n\n")
	} else {
		sb.WriteString("УСТАНОВКА (кратко)\n" +
			"1. Если нет готового PBO: DayZ Tools -> Addon Builder, источник src\\" + req.AddonName + ",\n" +
			"   выход @" + req.AddonName + "\\addons\\ (или включите сборку PBO в программе).\n" +
			"2. Ключ (один раз): DSUtils -> DSCreateKey MyServerKey (или кнопка в программе).\n" +
			"3. Подпись: DSUtils -> DSSignFile ключ.biprivatekey @" + req.AddonName + "\\addons\\" + req.AddonName + ".pbo\n" +
			"   (или включите подпись в программе, указав путь к DayZ Tools и ключу).\n" +
			"4. Папку @" + req.AddonName + " — на сервер и клиентам; .bikey — в keys сервера;\n" +
			"   -mod=...;@ИсходныйМод;@" + req.AddonName + "\n\n")
	}

	sb.WriteString("ВАЖНО\n" +
		"- Изменения конфигов (размер, слоты, броня и т.д.) влияют и на клиент. Чтобы не было\n" +
		"  рассинхрона и киков, аддон должен стоять и на сервере, и у игроков.\n" +
		"- Приватный ключ .biprivatekey храните в секрете и никому не передавайте.\n" +
		"- При обновлении исходного мода перепроверьте класснеймы (могут поменяться).\n")
	if len(res.Warnings) > 0 {
		sb.WriteString("\nПРЕДУПРЕЖДЕНИЯ СБОРКИ\n")
		for _, w := range res.Warnings {
			sb.WriteString("- " + w + "\n")
		}
	}
	sb.WriteString("\nСгенерировано автоматически. Удачи в моддинге!\n")
	return sb.String()
}
