package gen

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"dayzoverride/pbo"
)

// mockTools creates shell scripts named like the real Windows tools.
// On Linux they execute via shebang, emulating the pipeline end-to-end.
func mockTools(t *testing.T) string {
	if runtime.GOOS == "windows" {
		t.Skip("mock scripts are POSIX-only")
	}
	root := t.TempDir()
	mk := func(name, script string) {
		p := filepath.Join(root, name)
		if err := os.WriteFile(p, []byte(script), 0755); err != nil {
			t.Fatal(err)
		}
	}
	// CfgConvert -bin -dst DST SRC  -> writes fake raP file
	mk("CfgConvert.exe", `#!/bin/sh
# args: -bin -dst DST SRC
DST="$3"; SRC="$4"
printf '\0raP' > "$DST"
cat "$SRC" >> "$DST"
`)
	// DSSignFile KEY PBO -> creates PBO.KeyName.bisign
	mk("DSSignFile.exe", `#!/bin/sh
KEY="$1"; PBO="$2"
KN=$(basename "$KEY" .biprivatekey)
printf 'FAKESIGNATURE' > "$PBO.$KN.bisign"
`)
	// DSCreateKey NAME -> creates pair in CWD
	mk("DSCreateKey.exe", `#!/bin/sh
printf 'PRIVATEKEY' > "$1.biprivatekey"
printf 'PUBLICKEY' > "$1.bikey"
`)
	// FileBank -dst DST [-property prefix=X] SRC -> creates DST/$(basename SRC).pbo
	mk("FileBank.exe", `#!/bin/sh
DST=""; SRC=""
while [ $# -gt 0 ]; do
  case "$1" in
    -dst) DST="$2"; shift 2;;
    -property) shift 2;;
    *) SRC="$1"; shift;;
  esac
done
mkdir -p "$DST"
printf 'FAKEPBO:' > "$DST/$(basename "$SRC").pbo"
cat "$SRC"/* >> "$DST/$(basename "$SRC").pbo"
`)
	return root
}

func baseRequest() *Request {
	return &Request{
		AddonName:      "TestOverride",
		RequiredAddons: []string{"SourceMod"},
		MakePbo:        true,
		Items: []ItemReq{{
			Classname: "TestItem", Category: "CfgVehicles", Parent: "Clothing",
			Changes: []Change{{Param: "itemSize", Value: "3,3"}},
		}},
	}
}

func zipNames(t *testing.T, data []byte) map[string][]byte {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		out[f.Name] = b
	}
	return out
}

// full pipeline: binarize (mock CfgConvert) + sign (mock DSSignFile) + internal packer
func TestPipelineSignAndBinarize(t *testing.T) {
	toolsRoot := mockTools(t)
	keyDir := t.TempDir()
	os.WriteFile(filepath.Join(keyDir, "MyKey.biprivatekey"), []byte("PRIV"), 0644)
	os.WriteFile(filepath.Join(keyDir, "MyKey.bikey"), []byte("PUB"), 0644)

	req := baseRequest()
	req.ToolsPath = toolsRoot
	req.Sign = true
	req.PrivateKeyPath = filepath.Join(keyDir, "MyKey.biprivatekey")
	req.Binarize = true

	res, err := Generate(req, nil)
	if err != nil {
		t.Fatal(err)
	}
	files := zipNames(t, res.ZipData)
	pboData, ok := files["TestOverride/@TestOverride/addons/TestOverride.pbo"]
	if !ok {
		t.Fatalf("pbo missing, files: %v", res.FileList)
	}
	if _, ok := files["TestOverride/@TestOverride/addons/TestOverride.pbo.MyKey.bisign"]; !ok {
		t.Fatalf("bisign missing, files: %v", res.FileList)
	}
	if b, ok := files["TestOverride/@TestOverride/keys/MyKey.bikey"]; !ok || string(b) != "PUB" {
		t.Fatalf("bikey missing/wrong, files: %v", res.FileList)
	}
	// pbo built by internal packer must contain binarized config.bin
	p, err := pbo.FromBytes(pboData)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Entries) != 1 || p.Entries[0].Name != "config.bin" {
		t.Fatalf("entries: %+v", p.Entries)
	}
	data, _ := p.ReadEntry(&p.Entries[0])
	if !bytes.HasPrefix(data, []byte{0, 'r', 'a', 'P'}) {
		t.Fatal("config.bin not binarized by mock CfgConvert")
	}
	if p.Prefix() != "TestOverride" {
		t.Fatalf("prefix: %q", p.Prefix())
	}
}

// FileBank packing path
func TestPipelineFileBank(t *testing.T) {
	toolsRoot := mockTools(t)
	req := baseRequest()
	req.ToolsPath = toolsRoot
	req.UseFileBank = true

	res, err := Generate(req, nil)
	if err != nil {
		t.Fatal(err)
	}
	files := zipNames(t, res.ZipData)
	pboData, ok := files["TestOverride/@TestOverride/addons/TestOverride.pbo"]
	if !ok || !bytes.HasPrefix(pboData, []byte("FAKEPBO:")) {
		t.Fatalf("filebank pbo wrong: %q...", string(pboData[:min(20, len(pboData))]))
	}
}

// sign without makePbo must auto-enable pbo with a warning
func TestSignForcesPbo(t *testing.T) {
	toolsRoot := mockTools(t)
	keyDir := t.TempDir()
	os.WriteFile(filepath.Join(keyDir, "K.biprivatekey"), []byte("PRIV"), 0644)

	req := baseRequest()
	req.MakePbo = false
	req.ToolsPath = toolsRoot
	req.Sign = true
	req.PrivateKeyPath = filepath.Join(keyDir, "K.biprivatekey")

	res, err := Generate(req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("expected warning about auto-enabled pbo")
	}
	joined := strings.Join(res.FileList, "\n")
	if !strings.Contains(joined, "TestOverride.pbo") || !strings.Contains(joined, ".bisign") {
		t.Fatalf("files: %v", res.FileList)
	}
	// bikey missing -> warning expected
	hasKeyWarn := false
	for _, w := range res.Warnings {
		if strings.Contains(w, ".bikey") {
			hasKeyWarn = true
		}
	}
	if !hasKeyWarn {
		t.Fatalf("expected bikey warning, got: %v", res.Warnings)
	}
}

// tools requested but folder invalid -> clear error
func TestToolsMissingError(t *testing.T) {
	req := baseRequest()
	req.Sign = true
	req.PrivateKeyPath = "/nonexistent/key.biprivatekey"
	req.ToolsPath = "/nonexistent/dayz-tools"
	if _, err := Generate(req, nil); err == nil || !strings.Contains(err.Error(), "DayZ Tools") {
		t.Fatalf("expected tools error, got: %v", err)
	}
}

// no tools options -> classic flow untouched
func TestClassicFlowNoTools(t *testing.T) {
	req := baseRequest()
	res, err := Generate(req, nil)
	if err != nil {
		t.Fatal(err)
	}
	files := zipNames(t, res.ZipData)
	pboData, ok := files["TestOverride/@TestOverride/addons/TestOverride.pbo"]
	if !ok {
		t.Fatalf("files: %v", res.FileList)
	}
	p, err := pbo.FromBytes(pboData)
	if err != nil {
		t.Fatal(err)
	}
	if p.Entries[0].Name != "config.cpp" {
		t.Fatalf("expected text config, got %s", p.Entries[0].Name)
	}
	if _, ok := files["TestOverride/src/TestOverride/config.cpp"]; !ok {
		t.Fatal("src config missing")
	}
	if _, ok := files["TestOverride/README_RU.txt"]; !ok {
		t.Fatal("readme missing")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
