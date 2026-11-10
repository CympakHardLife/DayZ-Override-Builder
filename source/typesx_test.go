package typesx

import (
	"fmt"
	"strings"
	"testing"
)

func TestOptimizePreservesDisabledSpawnThreshold(t *testing.T) {
	src := []byte(`<types><type name="Disabled"><nominal>0</nominal><lifetime>0</lifetime><min>1</min><flags count_in_map="0"/></type></types>`)
	res, err := Optimize(src, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.FixedXML, "<min>1</min>") || !strings.Contains(res.FixedXML, "<nominal>0</nominal>") {
		t.Fatalf("disabled entry changed: %s", res.FixedXML)
	}
}

const acog = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<types>
    <type name="ACOGOptic">
        <nominal>0</nominal>
        <lifetime>14400</lifetime>
        <restock>0</restock>
        <min>0</min>
        <quantmin>-1</quantmin>
        <quantmax>-1</quantmax>
        <cost>100</cost>
        <flags count_in_cargo="0" count_in_hoarder="0" count_in_map="1" count_in_player="0" crafted="0" deloot="0"/>
        <category name="weapons"/>
        <usage name="Military"/>
    </type>
</types>`

func TestParseRejectsMalformedXML(t *testing.T) {
	cases := map[string]string{
		"empty":               "",
		"truncated":           `<types><type name="A">`,
		"mismatched tags":     `<types><type name="A"></types>`,
		"unquoted attribute":  `<types><type name=A/></types>`,
		"unknown entity":      `<types><type name="A&bad;"/></types>`,
		"duplicate attribute": `<types><type name="A" name="B"/></types>`,
		"second root":         `<types/><types/>`,
		"trailing broken XML": `<types/><broken`,
		"text before root":    `trash<types/>`,
		"text after root":     `<types/>trash`,
		"mixed before child":  `<types>trash<type name="A"/></types>`,
		"mixed after child":   `<types><type name="A"/>trash</types>`,
		"mixed inside record": `<types><type name="A"><nominal>1</nominal>trash</type></types>`,
		"wrong root":          `<spawnabletypes/>`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(src)); err == nil {
				t.Fatal("Parse accepted malformed types.xml")
			}
			if _, err := Optimize([]byte(src), false, nil); err == nil {
				t.Fatal("Optimize accepted malformed types.xml")
			}
			if _, err := Migrate([]byte(src), []byte(`<types/>`), true, nil); err == nil {
				t.Fatal("Migrate accepted malformed saved types.xml")
			}
			if _, err := Migrate([]byte(`<types/>`), []byte(src), true, nil); err == nil {
				t.Fatal("Migrate accepted malformed fresh types.xml")
			}
		})
	}
}

func TestParseAcceptsBOMCommentsAndRepairableValues(t *testing.T) {
	src := "\xef\xbb\xbf" + `<?xml version="1.0" encoding="UTF-8"?>
<!-- before -->
<types><type name="A&amp;B"><nominal>not-a-number</nominal><min>-2</min><flags count_in_map="2"/><custom><value>3</value></custom></type></types>
<!-- after -->`
	tf, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(tf.Types) != 1 || tf.Types[0].Name != "A&B" || tf.Types[0].Nominal.Ok || tf.Types[0].Min.V != -2 {
		t.Fatalf("parsed values changed: %+v", tf.Types)
	}
	if len(tf.Types[0].Extra) != 1 || tf.Types[0].Extra[0].XMLName.Local != "custom" {
		t.Fatalf("custom element lost: %+v", tf.Types[0].Extra)
	}
}

// пример пользователя: неспавнящаяся запись сводится к lifetime + flags
func TestOptimizeSimplifyACOG(t *testing.T) {
	res, err := Optimize([]byte(acog), true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Simplified != 1 {
		t.Fatalf("simplified=%d", res.Simplified)
	}
	x := res.FixedXML
	for _, must := range []string{`<type name="ACOGOptic">`, "<lifetime>14400</lifetime>", `count_in_map="1"`} {
		if !strings.Contains(x, must) {
			t.Fatalf("missing %q in:\n%s", must, x)
		}
	}
	for _, absent := range []string{"<nominal>", "<min>", "<restock>", "<quantmin>", "<quantmax>", "<cost>", "<category", "<usage"} {
		if strings.Contains(x, absent) {
			t.Fatalf("must be simplified away: %q in:\n%s", absent, x)
		}
	}
}

func TestOptimizeNominalMin(t *testing.T) {
	src := `<types><type name="M4A1"><nominal>5</nominal><lifetime>3600</lifetime><min>15</min><flags count_in_cargo="0" count_in_hoarder="0" count_in_map="1" count_in_player="0" crafted="0" deloot="0"/></type></types>`
	res, err := Optimize([]byte(src), true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.FixedXML, "<nominal>5</nominal>") || !strings.Contains(res.FixedXML, "<min>5</min>") {
		t.Fatalf("nominal<min not fixed:\n%s", res.FixedXML)
	}
	found := false
	for _, p := range res.Problems {
		if strings.Contains(p, "МЕНЬШЕ min") {
			found = true
		}
	}
	if !found {
		t.Fatalf("problem not reported: %v", res.Problems)
	}
}

func TestOptimizeQuantAndDuplicates(t *testing.T) {
	src := `<types>
<type name="Mag_STANAG_30Rnd"><nominal>10</nominal><lifetime>3600</lifetime><min>5</min><quantmin>-1</quantmin><quantmax>60</quantmax><flags count_in_cargo="0" count_in_hoarder="0" count_in_map="1" count_in_player="0" crafted="0" deloot="0"/></type>
<type name="Mag_STANAG_30Rnd"><nominal>20</nominal><lifetime>3600</lifetime><min>5</min><flags count_in_cargo="0" count_in_hoarder="0" count_in_map="1" count_in_player="0" crafted="0" deloot="0"/></type>
</types>`
	res, err := Optimize([]byte(src), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Duplicates != 1 {
		t.Fatalf("duplicates=%d", res.Duplicates)
	}
	if !strings.Contains(res.FixedXML, "<nominal>20</nominal>") {
		t.Fatalf("last duplicate must win:\n%s", res.FixedXML)
	}
	if strings.Count(res.FixedXML, `<type name="Mag_STANAG_30Rnd">`) != 1 {
		t.Fatal("duplicate not removed")
	}
}

func TestOptimizeQuantRanges(t *testing.T) {
	cases := []struct {
		name             string
		qmin, qmax       string
		wantMin, wantMax int64
	}{
		{"both above limit", "150", "200", 100, 100},
		{"reversed above limit", "200", "150", 100, 100},
		{"both below limit", "-2", "-2", 0, 0},
		{"minimum sentinel", "-1", "50", -1, -1},
		{"maximum sentinel", "50", "-1", -1, -1},
		{"sentinel and invalid negative", "-1", "-2", -1, -1},
		{"valid sentinel", "-1", "-1", -1, -1},
		{"valid percentages", "10", "90", 10, 90},
		{"valid boundaries", "0", "100", 0, 100},
		{"reversed percentages", "80", "20", 20, 80},
		{"negative minimum", "-2", "20", 0, 20},
		{"negative maximum", "20", "-2", 0, 20},
		{"extreme integers", "9223372036854775807", "-9223372036854775808", 0, 100},
		{"nonnumeric minimum", "invalid", "50", -1, -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := fmt.Sprintf(`<types><type name="A"><nominal>1</nominal><lifetime>3600</lifetime><min>0</min><quantmin>%s</quantmin><quantmax>%s</quantmax><flags/></type></types>`, tc.qmin, tc.qmax)
			res, err := Optimize([]byte(src), false, nil)
			if err != nil {
				t.Fatal(err)
			}
			tf, err := Parse([]byte(res.FixedXML))
			if err != nil {
				t.Fatal(err)
			}
			entry := tf.Types[0]
			if entry.QuantMin.V != tc.wantMin || entry.QuantMax.V != tc.wantMax {
				t.Fatalf("quantmin/quantmax=%d/%d, want %d/%d", entry.QuantMin.V, entry.QuantMax.V, tc.wantMin, tc.wantMax)
			}
			again, err := Optimize([]byte(res.FixedXML), false, nil)
			if err != nil {
				t.Fatal(err)
			}
			if again.FixedXML != res.FixedXML || again.Fixes != 0 {
				t.Fatalf("second optimization changed the result: fixes=%d\n%s", again.Fixes, again.FixedXML)
			}
		})
	}
}

func TestMigrate(t *testing.T) {
	my := `<types>
<type name="M4A1"><nominal>44</nominal><lifetime>7200</lifetime><min>22</min><flags count_in_cargo="0" count_in_hoarder="0" count_in_map="1" count_in_player="0" crafted="0" deloot="0"/></type>
<type name="ModGun_AK105"><nominal>7</nominal><lifetime>3600</lifetime><min>3</min><flags count_in_cargo="0" count_in_hoarder="0" count_in_map="1" count_in_player="0" crafted="0" deloot="0"/></type>
</types>`
	fresh := `<types>
<type name="M4A1"><nominal>10</nominal><lifetime>14400</lifetime><min>5</min><flags count_in_cargo="0" count_in_hoarder="0" count_in_map="1" count_in_player="0" crafted="0" deloot="0"/></type>
<type name="NewVanillaSMG"><nominal>8</nominal><lifetime>14400</lifetime><min>4</min><flags count_in_cargo="0" count_in_hoarder="0" count_in_map="1" count_in_player="0" crafted="0" deloot="0"/></type>
</types>`
	res, err := Migrate([]byte(my), []byte(fresh), true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Transferred != 1 {
		t.Fatalf("transferred=%d", res.Transferred)
	}
	if len(res.NewNames) != 1 || res.NewNames[0] != "NewVanillaSMG" {
		t.Fatalf("newNames=%v", res.NewNames)
	}
	if len(res.MissingKept) != 1 || res.MissingKept[0] != "ModGun_AK105" {
		t.Fatalf("missing=%v", res.MissingKept)
	}
	// ваши значения победили
	if !strings.Contains(res.MergedXML, "<nominal>44</nominal>") {
		t.Fatalf("user value lost:\n%s", res.MergedXML)
	}
	// модовый предмет сохранён
	if !strings.Contains(res.MergedXML, "ModGun_AK105") {
		t.Fatal("mod item lost")
	}
	// новый ванильный включён
	if !strings.Contains(res.MergedXML, "NewVanillaSMG") {
		t.Fatal("new vanilla lost")
	}
}
