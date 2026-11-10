package validator

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Не скачивает файлы: путь к локальной официальной CE задаётся явно.
func TestOfficialDayZ130Economy(t *testing.T) {
	root := os.Getenv("DAYZ_CE_TEST_ROOT")
	if root == "" {
		t.Skip("DAYZ_CE_TEST_ROOT is not set")
	}
	for _, mission := range []string{"dayzOffline.chernarusplus", "dayzOffline.enoch", "dayzOffline.sakhal", "dayzOffline.nasdara"} {
		t.Run(mission, func(t *testing.T) {
			var files []InputFile
			for _, name := range []string{"db/types.xml", "cfgspawnabletypes.xml", "cfgrandompresets.xml", "cfglimitsdefinition.xml", "cfglimitsdefinitionuser.xml", "cfgeconomycore.xml", "cfggameplay.json"} {
				data, err := os.ReadFile(filepath.Join(root, mission, filepath.FromSlash(name)))
				if err != nil {
					t.Fatal(err)
				}
				files = append(files, InputFile{Name: name, Data: data})
			}
			reports := ValidateSet(files, "")
			warnings := 0
			messages := map[string]int{}
			for _, report := range reports {
				for _, problem := range report.Problems {
					if problem.Level == "error" {
						t.Errorf("%s:%d: %s", report.Name, problem.Line, problem.Msg)
					} else {
						warnings++
						message := problem.Msg
						if i := strings.Index(message, ": "); i >= 0 {
							message = message[i+2:]
						}
						messages[message]++
					}
				}
			}
			t.Logf("checked %d official files, %d warnings", len(files), warnings)
			var keys []string
			for key := range messages {
				keys = append(keys, key)
			}
			sort.Slice(keys, func(i, j int) bool { return messages[keys[i]] > messages[keys[j]] })
			for i, key := range keys {
				if i == 4 {
					break
				}
				t.Logf("warning x%d: %s", messages[key], key)
			}
		})
	}
}
