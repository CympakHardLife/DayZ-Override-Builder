package validator

import (
	"fmt"
	"testing"
)

func TestChanceWeightsAndProbabilities(t *testing.T) {
	for _, tc := range []struct {
		name   string
		group  string
		weight string
		ok     bool
	}{
		{"vanilla Sakhal weight", "1.0", "40.00", true},
		{"zero", "0", "0", true},
		{"invalid group", "1.5", "40", false},
		{"negative weight", "1", "-2", false},
		{"NaN group", "NaN", "1", false},
		{"NaN weight", "1", "NaN", false},
		{"infinite weight", "1", "+Inf", false},
		{"empty weight", "1", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := fmt.Sprintf(`<spawnabletypes><type name="AK74"><attachments chance="%s"><item name="AK_FoldingBttstck" chance="%s"/></attachments></type></spawnabletypes>`, tc.group, tc.weight)
			report := Validate("cfgspawnabletypes.xml", []byte(data))
			if report.OK != tc.ok {
				t.Fatalf("OK=%v, want %v: %+v", report.OK, tc.ok, report.Problems)
			}
		})
	}
}

func TestXMLCommentsAndCDATAAmpersands(t *testing.T) {
	for _, data := range []string{
		`<types><!-- food & tools --><type name="A"><lifetime>3600</lifetime></type></types>`,
		`<description><![CDATA[food & tools]]></description>`,
	} {
		if report := Validate("file.xml", []byte(data)); !report.OK {
			t.Fatalf("valid XML rejected: %+v", report.Problems)
		}
	}
	if err := ValidateXMLStructure([]byte(`<root>food & tools</root>`)); err == nil {
		t.Fatal("unescaped ampersand accepted")
	}
}

func TestBooksAndMissingItemName(t *testing.T) {
	report := Validate("types.xml", []byte(`<types><type name="Book"><nominal>1</nominal><min>0</min><lifetime>3600</lifetime><flags count_in_map="1"/><category name="books"/></type></types>`))
	if !report.OK || len(report.Problems) != 0 {
		t.Fatalf("vanilla books category rejected: %+v", report.Problems)
	}
	if report := Validate("cfgspawnabletypes.xml", []byte(`<spawnabletypes><type name="A"><cargo chance="1"><item chance="1"/></cargo></type></spawnabletypes>`)); report.OK {
		t.Fatal("nameless item accepted")
	}
}

func TestDisabledVanillaEntriesDoNotWarnAboutSpawn(t *testing.T) {
	data := []byte(`<types><type name="Disabled"><nominal>0</nominal><min>1</min><lifetime>0</lifetime><flags count_in_map="0"/></type></types>`)
	report := Validate("types.xml", data)
	if !report.OK || len(report.Problems) != 0 {
		t.Fatalf("disabled vanilla entry flagged: %+v", report.Problems)
	}
}
