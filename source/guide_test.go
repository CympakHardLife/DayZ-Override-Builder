package guide

import (
	"strings"
	"testing"
)

const sampleInedia = "## `Zombies.BreakingDoorsOpenChancePercent`\n\nThe parameter takes float values >=0 and <=100. Chance in percent to break open the door per one hit.\n\n### Zombies.Groups\n\nDefines infected group sizes per area type.\n\n| Parameter | Type | Description |\n|---|---|---|\n| `Sound.EnableScreams` | bool | Enables zombie scream sounds at night |\n| MaxDistance | float | Maximum hearing distance in meters |\n\n- **Loot.QuantityPercent** — how full spawned loot items are, percent 0..100\n"

const sampleTerje = "```\nMedicine.EnableMedicalBoiling = true; // [type: bool; default: true] Enables sterilization of instruments by boiling in water.\nSkills.Hunting.ExpMultiplier = 1.0; // [type: float; default: 1.0] Experience multiplier for hunting skill.\n```\n"

func TestExtractMarkdown(t *testing.T) {
	kvs := Extract(sampleInedia)
	got := map[string]string{}
	for _, kv := range kvs {
		got[kv.Key] = kv.Desc
	}
	for _, want := range []string{
		"Zombies.BreakingDoorsOpenChancePercent",
		"Zombies.Groups",
		"Sound.EnableScreams",
		"MaxDistance",
		"Loot.QuantityPercent",
	} {
		if got[want] == "" {
			t.Errorf("не извлечён параметр %s; всего: %v", want, got)
		}
	}
	if !strings.Contains(got["Zombies.BreakingDoorsOpenChancePercent"], "break open the door") {
		t.Errorf("описание из абзаца не собрано: %q", got["Zombies.BreakingDoorsOpenChancePercent"])
	}
}

func TestExtractTerjeCfg(t *testing.T) {
	kvs := Extract(sampleTerje)
	got := map[string]string{}
	for _, kv := range kvs {
		got[kv.Key] = kv.Desc
	}
	if !strings.Contains(got["Medicine.EnableMedicalBoiling"], "boiling") {
		t.Errorf("cfg-строка не разобрана: %v", got)
	}
	if strings.Contains(got["Medicine.EnableMedicalBoiling"], "[type:") {
		t.Errorf("мета [type:...] не удалена из описания: %q", got["Medicine.EnableMedicalBoiling"])
	}
}

func TestMatch(t *testing.T) {
	kvs := Extract(sampleInedia)
	fields := []FieldRef{
		{ID: "1", Path: "Zombies.BreakingDoorsOpenChancePercent.all"}, // суффиксный сегмент .all
		{ID: "2", Path: "Zombies.Groups"},                             // точное
		{ID: "3", Path: "General.Sound.EnableScreams"},                // ключ гайда — суффикс пути
		{ID: "4", Path: "Zombies.Speed"},                              // нет в гайде
	}
	m := Match(kvs, fields)
	if m["1"] == "" || m["2"] == "" || m["3"] == "" {
		t.Errorf("совпадения не найдены: %v", m)
	}
	if m["4"] != "" {
		t.Errorf("ложное совпадение для Zombies.Speed: %q", m["4"])
	}
}

func TestHtmlToText(t *testing.T) {
	txt := htmlToText("<html><body><h2>Zombies.Speed</h2><p>Movement speed &amp; multiplier</p><script>bad()</script></body></html>")
	if !strings.Contains(txt, "## Zombies.Speed") || !strings.Contains(txt, "Movement speed & multiplier") || strings.Contains(txt, "bad()") {
		t.Errorf("htmlToText: %q", txt)
	}
}
