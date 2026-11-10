// Package tools integrates external DayZ Tools (Steam) executables:
// discovery by path, Steam auto-detection, and wrappers for
// DSCreateKey / DSSignFile / CfgConvert / FileBank.
//
// The program never copies the tools: it stores the path and re-scans it on
// every use, so Steam updates of DayZ Tools are picked up automatically.
package tools

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"
)

// Tool describes one discovered (or missing) executable.
type Tool struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Path    string `json:"path,omitempty"`
	ModTime string `json:"modTime,omitempty"`
	Found   bool   `json:"found"`
}

var wanted = []struct{ key, exe, label string }{
	{"dssignfile", "DSSignFile.exe", "DSSignFile — подпись PBO"},
	{"dscreatekey", "DSCreateKey.exe", "DSCreateKey — создание ключей"},
	{"dscheck", "DSCheckSignatures.exe", "DSCheckSignatures — проверка подписей"},
	{"cfgconvert", "CfgConvert.exe", "CfgConvert — бинаризация конфигов"},
	{"filebank", "FileBank.exe", "FileBank — официальная упаковка PBO"},
	{"addonbuilder", "AddonBuilder.exe", "AddonBuilder — сборщик аддонов"},
}

// Toolset is the result of scanning a DayZ Tools folder.
type Toolset struct {
	Root  string
	tools map[string]Tool
}

func (ts *Toolset) Get(key string) (Tool, bool) {
	if ts == nil {
		return Tool{}, false
	}
	t, ok := ts.tools[key]
	return t, ok
}

func (ts *Toolset) Has(key string) bool {
	_, ok := ts.Get(key)
	return ok
}

// Detect recursively scans root for the known executables.
// The scan is layout-independent so it survives folder reorganisations.
func Detect(root string) (*Toolset, []Tool, error) {
	root = strings.TrimSpace(strings.Trim(strings.TrimSpace(root), `"`))
	if root == "" {
		return nil, nil, errors.New("путь к DayZ Tools не указан")
	}
	st, err := os.Stat(root)
	if err != nil || !st.IsDir() {
		return nil, nil, fmt.Errorf("папка не найдена: %s", root)
	}
	found := map[string]Tool{}
	sep := string(os.PathSeparator)
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return nil
		}
		if d.IsDir() {
			rel, err := filepath.Rel(root, p)
			if err == nil && strings.Count(rel, sep) > 6 {
				return fs.SkipDir
			}
			return nil
		}
		name := d.Name()
		for _, w := range wanted {
			if strings.EqualFold(name, w.exe) {
				if _, ok := found[w.key]; !ok {
					mt := ""
					if info, err := d.Info(); err == nil {
						mt = info.ModTime().Format("02.01.2006")
					}
					found[w.key] = Tool{Key: w.key, Label: w.label, Path: p, ModTime: mt, Found: true}
				}
			}
		}
		return nil
	})
	list := make([]Tool, 0, len(wanted))
	for _, w := range wanted {
		if t, ok := found[w.key]; ok {
			list = append(list, t)
		} else {
			list = append(list, Tool{Key: w.key, Label: w.label, Found: false})
		}
	}
	if len(found) == 0 {
		return nil, list, fmt.Errorf("в папке %s не найдено ни одного инструмента DayZ Tools", root)
	}
	return &Toolset{Root: root, tools: found}, list, nil
}

// ---------- Steam auto-detection ----------

// AutoDetect tries to locate the DayZ Tools installation via the Steam
// registry keys, libraryfolders.vdf and common paths.
func AutoDetect() (string, []Tool, error) {
	for _, c := range candidates() {
		st, err := os.Stat(c)
		if err != nil || !st.IsDir() {
			continue
		}
		if ts, list, err := Detect(c); err == nil && ts != nil {
			return c, list, nil
		}
	}
	return "", nil, errors.New("установка DayZ Tools не найдена автоматически — укажите путь вручную (обычно Steam\\steamapps\\common\\DayZ Tools)")
}

func candidates() []string {
	var roots []string
	if runtime.GOOS == "windows" {
		for _, q := range [][2]string{
			{`HKCU\Software\Valve\Steam`, "SteamPath"},
			{`HKLM\SOFTWARE\WOW6432Node\Valve\Steam`, "InstallPath"},
			{`HKLM\SOFTWARE\Valve\Steam`, "InstallPath"},
		} {
			if v := regQuery(q[0], q[1]); v != "" {
				roots = append(roots, filepath.Clean(v))
			}
		}
		roots = append(roots,
			`C:\Program Files (x86)\Steam`,
			`C:\Steam`,
		)
		for d := 'C'; d <= 'L'; d++ {
			roots = append(roots,
				fmt.Sprintf(`%c:\SteamLibrary`, d),
				fmt.Sprintf(`%c:\Steam`, d),
				fmt.Sprintf(`%c:\Games\Steam`, d),
				fmt.Sprintf(`%c:\Games\SteamLibrary`, d),
			)
		}
	}
	if env := os.Getenv("DAYZ_TOOLS_PATH"); env != "" {
		return append([]string{env}, expandLibraries(roots)...)
	}
	return expandLibraries(roots)
}

func expandLibraries(steamRoots []string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(lib string) {
		p := filepath.Join(lib, "steamapps", "common", "DayZ Tools")
		k := strings.ToLower(p)
		if !seen[k] {
			seen[k] = true
			out = append(out, p)
		}
	}
	for _, r := range steamRoots {
		add(r)
		for _, lib := range ParseLibraryFolders(filepath.Join(r, "steamapps", "libraryfolders.vdf")) {
			add(lib)
		}
	}
	return out
}

var regSZRe = regexp.MustCompile(`REG_SZ\s+(.+)`)

func regQuery(key, value string) string {
	out, err := exec.Command("reg", "query", key, "/v", value).Output()
	if err != nil {
		return ""
	}
	m := regSZRe.FindStringSubmatch(string(out))
	if m == nil {
		return ""
	}
	return strings.ReplaceAll(strings.TrimSpace(m[1]), "/", `\`)
}

var vdfPathRe = regexp.MustCompile(`"path"\s+"((?:[^"\\]|\\.)*)"`)

// ParseLibraryFolders extracts Steam library paths from libraryfolders.vdf.
func ParseLibraryFolders(vdfPath string) []string {
	data, err := os.ReadFile(vdfPath)
	if err != nil {
		return nil
	}
	var libs []string
	for _, m := range vdfPathRe.FindAllStringSubmatch(string(data), -1) {
		p := strings.ReplaceAll(m[1], `\\`, `\`)
		if p != "" {
			libs = append(libs, p)
		}
	}
	return libs
}

// ---------- running tools ----------

func sanitize(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	var sb strings.Builder
	for _, c := range b {
		if c < 0x80 {
			sb.WriteByte(c)
		} else {
			sb.WriteRune(rune(c))
		}
	}
	return sb.String()
}

func run(exePath string, args []string, dir string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, exePath, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(sanitize(out))
	if ctx.Err() == context.DeadlineExceeded {
		return text, fmt.Errorf("%s: инструмент не ответил за %v", filepath.Base(exePath), timeout)
	}
	if err != nil {
		msg := fmt.Sprintf("%s завершился с ошибкой: %v", filepath.Base(exePath), err)
		if text != "" {
			msg += "\nВывод инструмента:\n" + text
		}
		return text, errors.New(msg)
	}
	return text, nil
}

var keyNameRe = regexp.MustCompile(`^[A-Za-z0-9_\-]+$`)

// CreateKey runs DSCreateKey in outDir and returns the key pair paths.
func CreateKey(ts *Toolset, name, outDir string) (privPath, pubPath, output string, err error) {
	t, ok := ts.Get("dscreatekey")
	if !ok {
		return "", "", "", errors.New("DSCreateKey.exe не найден в папке DayZ Tools")
	}
	name = strings.TrimSpace(name)
	if !keyNameRe.MatchString(name) {
		return "", "", "", errors.New("имя ключа: только латиница, цифры, _ и -")
	}
	outDir = strings.TrimSpace(strings.Trim(outDir, `"`))
	if outDir == "" {
		return "", "", "", errors.New("укажите папку для сохранения ключей")
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return "", "", "", fmt.Errorf("не удалось создать папку %s: %w", outDir, err)
	}
	privPath = filepath.Join(outDir, name+".biprivatekey")
	pubPath = filepath.Join(outDir, name+".bikey")
	if _, e := os.Stat(privPath); e == nil {
		return "", "", "", fmt.Errorf("ключ %s уже существует в этой папке — выберите другое имя", name)
	}
	// DSCreateKey writes the pair into the current working directory.
	output, err = run(t.Path, []string{name}, outDir, 60*time.Second)
	if err != nil {
		return "", "", output, err
	}
	if _, e := os.Stat(privPath); e != nil {
		return "", "", output, fmt.Errorf("DSCreateKey отработал, но файл %s не появился.\nВывод:\n%s", privPath, output)
	}
	return privPath, pubPath, output, nil
}

// SignPbo runs DSSignFile and returns the created .bisign path.
func SignPbo(ts *Toolset, privateKey, pboPath string) (bisignPath, output string, err error) {
	t, ok := ts.Get("dssignfile")
	if !ok {
		return "", "", errors.New("DSSignFile.exe не найден в папке DayZ Tools")
	}
	privateKey = strings.TrimSpace(strings.Trim(privateKey, `"`))
	if privateKey == "" {
		return "", "", errors.New("не указан приватный ключ (.biprivatekey)")
	}
	if _, e := os.Stat(privateKey); e != nil {
		return "", "", fmt.Errorf("файл ключа не найден: %s", privateKey)
	}
	output, err = run(t.Path, []string{privateKey, pboPath}, filepath.Dir(pboPath), 180*time.Second)
	if err != nil {
		return "", output, err
	}
	keyName := strings.TrimSuffix(filepath.Base(privateKey), filepath.Ext(privateKey))
	bisignPath = pboPath + "." + keyName + ".bisign"
	if _, e := os.Stat(bisignPath); e != nil {
		if matches, _ := filepath.Glob(pboPath + ".*.bisign"); len(matches) > 0 {
			bisignPath = matches[0]
		} else {
			return "", output, fmt.Errorf("DSSignFile отработал, но подпись .bisign не появилась.\nВывод:\n%s", output)
		}
	}
	return bisignPath, output, nil
}

// SiblingBikey returns the .bikey lying next to a .biprivatekey, if present.
func SiblingBikey(privateKey string) string {
	base := strings.TrimSuffix(privateKey, filepath.Ext(privateKey))
	pub := base + ".bikey"
	if _, err := os.Stat(pub); err == nil {
		return pub
	}
	return ""
}

// ConvertToBin runs CfgConvert -bin (config.cpp -> config.bin).
func ConvertToBin(ts *Toolset, srcCpp, dstBin string) (string, error) {
	t, ok := ts.Get("cfgconvert")
	if !ok {
		return "", errors.New("CfgConvert.exe не найден в папке DayZ Tools")
	}
	output, err := run(t.Path, []string{"-bin", "-dst", dstBin, srcCpp}, filepath.Dir(srcCpp), 120*time.Second)
	if err != nil {
		return output, err
	}
	if st, e := os.Stat(dstBin); e != nil || st.Size() == 0 {
		return output, fmt.Errorf("CfgConvert отработал, но %s не появился (проверьте синтаксис конфига).\nВывод:\n%s", filepath.Base(dstBin), output)
	}
	return output, nil
}

// FileBankPack packs srcFolder into dstDir/<folder>.pbo with the given prefix.
func FileBankPack(ts *Toolset, srcFolder, dstDir, prefix string) (pboPath, output string, err error) {
	t, ok := ts.Get("filebank")
	if !ok {
		return "", "", errors.New("FileBank.exe не найден в папке DayZ Tools")
	}
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return "", "", err
	}
	args := []string{"-dst", dstDir}
	if prefix != "" {
		args = append(args, "-property", "prefix="+prefix)
	}
	args = append(args, srcFolder)
	output, err = run(t.Path, args, filepath.Dir(srcFolder), 300*time.Second)
	if err != nil {
		return "", output, err
	}
	pboPath = filepath.Join(dstDir, filepath.Base(srcFolder)+".pbo")
	if _, e := os.Stat(pboPath); e != nil {
		return "", output, fmt.Errorf("FileBank отработал, но PBO не появился: %s\nВывод:\n%s", pboPath, output)
	}
	return pboPath, output, nil
}
