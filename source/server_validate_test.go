package main

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestValidationPaths(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{`mission\db\types.xml`, "mission/db/types.xml"},
		{"db/types.xml", "db/types.xml"},
		{"types.xml", "types.xml"},
	} {
		got, err := validationPath(tc.name)
		if err != nil || got != tc.want {
			t.Fatalf("%q: got %q, %v", tc.name, got, err)
		}
	}
	for _, name := range []string{"", "..", "../types.xml", "a/../types.xml", "/types.xml", `C:\types.xml`, `\\server\types.xml`, "bad\x00.xml"} {
		if _, err := validationPath(name); err == nil {
			t.Errorf("unsafe path accepted: %q", name)
		}
	}
}

func TestValidateUploadKeepsMissionPaths(t *testing.T) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for _, file := range []struct{ name, path, data string }{
		{"cfgeconomycore.xml", "mission/cfgeconomycore.xml", `<economycore><ce folder="custom"><file name="extra.xml" type="types"/></ce></economycore>`},
		{"extra.xml", "mission/custom/extra.xml", `<types><type name="Example"><lifetime>3600</lifetime></type></types>`},
	} {
		part, err := w.CreateFormFile("files", file.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte(file.data)); err != nil {
			t.Fatal(err)
		}
		if err := w.WriteField("paths", file.path); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/validate", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	handleValidate(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload status %d: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		JobID string `json:"jobId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		poll := httptest.NewRecorder()
		handleJob(poll, httptest.NewRequest(http.MethodGet, "/api/job/"+response.JobID, nil))
		var result struct {
			Done   bool `json:"done"`
			Result struct {
				Total   int `json:"total"`
				Reports []struct {
					Name string `json:"name"`
				} `json:"reports"`
			} `json:"result"`
		}
		if err := json.Unmarshal(poll.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Done {
			if result.Result.Total != 2 || result.Result.Reports[1].Name != "mission/custom/extra.xml" {
				t.Fatalf("paths lost: %s", poll.Body.String())
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("validation job did not finish")
}
