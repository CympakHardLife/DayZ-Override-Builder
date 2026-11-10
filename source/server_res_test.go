package main

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"dayzoverride/pbo"
)

func newResTestSession(t *testing.T, files []pbo.File) (*resSession, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "resources.pbo")
	if err := os.WriteFile(path, pbo.Build(`TestMod\GUI`, files), 0644); err != nil {
		t.Fatal(err)
	}
	p, err := pbo.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s := &resSession{
		pboPath:  path,
		origName: "resources.pbo",
		prefix:   p.Prefix(),
		entries:  append([]pbo.Entry(nil), p.Entries...),
		modified: map[string][]byte{},
	}
	p.Close()
	sid := t.Name()
	resSessions.mu.Lock()
	resSessions.m[sid] = s
	resSessions.mu.Unlock()
	t.Cleanup(func() {
		resSessions.mu.Lock()
		delete(resSessions.m, sid)
		resSessions.mu.Unlock()
	})
	return s, sid
}

func resTestSaveText(t *testing.T, sid, path, content string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]string{"sid": sid, "path": path, "content": content})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	handleResText(w, httptest.NewRequest(http.MethodPost, "/api/res/text", bytes.NewReader(body)))
	return w
}

func resTestReplace(t *testing.T, sid, path string, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if err := mw.WriteField("sid", sid); err != nil {
		t.Fatal(err)
	}
	if err := mw.WriteField("path", path); err != nil {
		t.Fatal(err)
	}
	part, err := mw.CreateFormFile("file", "replacement.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/res/replace", &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	handleResReplace(w, r)
	return w
}

func checkResTestBuild(t *testing.T, s *resSession, files []pbo.File, wantModified int) {
	t.Helper()
	data, modified, err := s.buildPBO()
	if err != nil {
		t.Fatal(err)
	}
	if modified != wantModified {
		t.Fatalf("modified=%d, want %d", modified, wantModified)
	}
	p, err := pbo.FromBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if p.Prefix() != s.prefix || len(p.Entries) != len(files) {
		t.Fatalf("archive metadata changed: prefix=%q entries=%d", p.Prefix(), len(p.Entries))
	}
	for i, want := range files {
		if p.Entries[i].Name != want.Name {
			t.Fatalf("entry %d name=%q, want original %q", i, p.Entries[i].Name, want.Name)
		}
		got, err := p.ReadEntry(&p.Entries[i])
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want.Data) {
			t.Fatalf("entry %q content=%q, want %q", want.Name, got, want.Data)
		}
	}
}

func TestResSeparatorAliasesReadSaveAndRebuild(t *testing.T) {
	cases := []struct {
		name, archived, requested string
	}{
		{"backslash archive", `GUI\layout.layout`, "gui/layout.layout"},
		{"slash archive", "GUI/layout.layout", `gui\layout.layout`},
		{"mixed archive", `GUI\Layouts/main.layout`, `gui/layouts\MAIN.LAYOUT`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			original := []byte(`FrameWidgetClass { image0 "gui\images\original.edds"; }`)
			files := []pbo.File{
				{Name: tc.archived, Data: original},
				{Name: `Scripts\init.c`, Data: []byte(`Print("keep/path\\unchanged");`)},
			}
			s, sid := newResTestSession(t, files)
			got, err := s.readEntryData(tc.requested)
			if err != nil || !bytes.Equal(got, original) {
				t.Fatalf("read alias: content=%q err=%v", got, err)
			}
			edited := `FrameWidgetClass { image0 "gui\images/new.edds"; }`
			w := resTestSaveText(t, sid, tc.requested, edited)
			if w.Code != http.StatusOK {
				t.Fatalf("save status=%d: %s", w.Code, w.Body.String())
			}
			for _, path := range []string{tc.requested, tc.archived} {
				got, err = s.readEntryData(path)
				if err != nil || string(got) != edited {
					t.Fatalf("read saved %q: content=%q err=%v", path, got, err)
				}
			}
			found := false
			for _, file := range s.fileList() {
				if file.Path == tc.archived {
					found = file.Mod && file.Size == len(edited)
				}
			}
			if !found {
				t.Fatalf("edited entry not marked in file list: %+v", s.fileList())
			}
			files[0].Data = []byte(edited)
			checkResTestBuild(t, s, files, 1)
		})
	}
}

func TestResReplaceSeparatorAliasRebuild(t *testing.T) {
	files := []pbo.File{{Name: `GUI\sounds\click.ogg`, Data: []byte("old")}}
	s, sid := newResTestSession(t, files)
	data := []byte{0, 1, '\\', '/', 255}
	w := resTestReplace(t, sid, "gui/sounds/click.ogg", data)
	if w.Code != http.StatusOK {
		t.Fatalf("replace status=%d: %s", w.Code, w.Body.String())
	}
	got, err := s.readEntryData(files[0].Name)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("read replaced: content=%q err=%v", got, err)
	}
	files[0].Data = data
	checkResTestBuild(t, s, files, 1)
}

func TestResRejectsUnknownAndAmbiguousTargets(t *testing.T) {
	cases := []struct {
		name  string
		files []pbo.File
		path  string
	}{
		{"missing", []pbo.File{{Name: `gui\other.layout`, Data: []byte("original")}}, "gui/layout.layout"},
		{"separator collision", []pbo.File{{Name: `gui\layout.layout`, Data: []byte("first")}, {Name: "gui/layout.layout", Data: []byte("second")}}, `gui\layout.layout`},
		{"case collision", []pbo.File{{Name: `GUI\Layout.layout`, Data: []byte("first")}, {Name: `gui\layout.layout`, Data: []byte("second")}}, `GUI\Layout.layout`},
		{"exact duplicate", []pbo.File{{Name: `gui\layout.layout`, Data: []byte("first")}, {Name: `gui\layout.layout`, Data: []byte("second")}}, `gui\layout.layout`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, sid := newResTestSession(t, tc.files)
			if _, err := s.readEntryData(tc.path); err == nil {
				t.Fatal("read accepted invalid target")
			}
			if err := s.writeEntryData(tc.path, []byte("changed")); err == nil {
				t.Fatal("write accepted invalid target")
			}
			for _, w := range []*httptest.ResponseRecorder{
				resTestSaveText(t, sid, tc.path, "changed"),
				resTestReplace(t, sid, tc.path, []byte("changed")),
			} {
				if w.Code < 400 {
					t.Fatalf("mutation reported success: status=%d body=%s", w.Code, w.Body.String())
				}
			}
			if len(s.modified) != 0 {
				t.Fatalf("invalid mutation stored: %v", s.modified)
			}
			checkResTestBuild(t, s, tc.files, 0)
			s.modified[resPathKey(tc.path)] = []byte("invalid stored edit")
			if _, err := s.readEntryData(tc.path); err == nil {
				t.Fatal("stored edit bypassed target validation")
			}
			if _, _, err := s.buildPBO(); err == nil {
				t.Fatal("rebuild silently accepted invalid stored edit")
			}
		})
	}
}
