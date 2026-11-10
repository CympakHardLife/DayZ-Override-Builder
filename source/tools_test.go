package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectRecursive(t *testing.T) {
	root := t.TempDir()
	mk := func(rel string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("stub"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	mk("Bin/DsUtils/DSSignFile.exe")
	mk("Bin/DsUtils/DSCreateKey.exe")
	mk("SomeOther/Deep/CfgConvert.exe") // non-standard location — must still be found
	ts, list, err := Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, x := range list {
		found[x.Key] = x.Found
	}
	if !found["dssignfile"] || !found["dscreatekey"] || !found["cfgconvert"] {
		t.Fatalf("detection failed: %+v", list)
	}
	if found["filebank"] {
		t.Fatal("filebank must be missing")
	}
	if !ts.Has("dssignfile") {
		t.Fatal("toolset lookup failed")
	}
}

func TestDetectEmpty(t *testing.T) {
	root := t.TempDir()
	if _, _, err := Detect(root); err == nil {
		t.Fatal("expected error for empty dir")
	}
	if _, _, err := Detect(""); err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestParseLibraryFolders(t *testing.T) {
	dir := t.TempDir()
	vdf := filepath.Join(dir, "libraryfolders.vdf")
	content := `"libraryfolders"
{
	"0"
	{
		"path"		"C:\\Program Files (x86)\\Steam"
	}
	"1"
	{
		"path"		"D:\\SteamLibrary"
	}
}`
	if err := os.WriteFile(vdf, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	libs := ParseLibraryFolders(vdf)
	if len(libs) != 2 || libs[0] != `C:\Program Files (x86)\Steam` || libs[1] != `D:\SteamLibrary` {
		t.Fatalf("got %v", libs)
	}
}

func TestCreateKeyValidation(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "DSCreateKey.exe")
	os.WriteFile(p, []byte("#!/bin/sh\nexit 0\n"), 0755)
	ts, _, err := Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := CreateKey(ts, "bad name!", t.TempDir()); err == nil {
		t.Fatal("expected key name validation error")
	}
	if _, _, _, err := CreateKey(ts, "GoodKey", ""); err == nil {
		t.Fatal("expected out dir validation error")
	}
}
