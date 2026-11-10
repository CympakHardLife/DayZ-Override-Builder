package validator

import (
	"strings"
	"testing"
)

func batchFile(name, data string) InputFile {
	return InputFile{Name: name, Data: []byte(data)}
}

func batchHas(report *FileReport, level, fragment string) bool {
	for _, problem := range report.Problems {
		if problem.Level == level && strings.Contains(problem.Msg, fragment) {
			return true
		}
	}
	return false
}

func TestBatchDuplicateTypesStayWithinKind(t *testing.T) {
	reports := ValidateSet([]InputFile{
		batchFile("db/types.xml", "<types>\n<type name=\"AKM\"/>\n</types>"),
		batchFile("mod/types.xml", "<types>\n\n<type name=\"AKM\"/>\n</types>"),
		batchFile("cfgspawnabletypes.xml", `<spawnabletypes><type name="AKM"/></spawnabletypes>`),
		batchFile("mod/spawn.xml", `<spawnabletypes><type name="AKM"/></spawnabletypes>`),
	}, "auto")
	if !batchHas(reports[1], "warn", "db/types.xml, строка 2") || !batchHas(reports[3], "warn", "cfgspawnabletypes.xml") {
		t.Fatalf("cross-file duplicates missing: %+v / %+v", reports[1], reports[3])
	}
	if batchHas(reports[2], "warn", "дубликат") {
		t.Fatalf("different XML kinds must not conflict: %+v", reports[2])
	}
	for _, problem := range reports[1].Problems {
		if strings.Contains(problem.Msg, "дубликат") && (problem.Line != 3 || !strings.Contains(problem.Excerpt, "AKM")) {
			t.Fatalf("duplicate location lost: %+v", problem)
		}
	}
}

func TestBatchInvalidXMLCannotContributeDeclarations(t *testing.T) {
	for _, broken := range []string{
		`<types><type name="AKM"/></types><other/>`,
		`<types><type name="AKM" name="Other"/></types>`,
		`<types><type name="AKM"/></types>trailing`,
		`<types><type name="AKM"/>`,
	} {
		reports := ValidateSet([]InputFile{batchFile("broken.xml", broken), batchFile("good.xml", `<types><type name="AKM"/></types>`)}, "auto")
		if reports[0].OK || batchHas(reports[1], "warn", "дубликат") {
			t.Fatalf("invalid XML contributed declarations: %+v / %+v", reports[0], reports[1])
		}
	}
}

func TestBatchPresetsResolveByKind(t *testing.T) {
	presets := batchFile("cfgrandompresets.xml", `<randompresets><cargo name="Food" chance="1"/><attachments name="Scope" chance="1"/></randompresets>`)
	good := batchFile("cfgspawnabletypes.xml", `<spawnabletypes><type name="UnknownModClass"><cargo preset="Food"/><attachments preset="Scope"/></type></spawnabletypes>`)
	reports := ValidateSet([]InputFile{good, presets}, "auto")
	if !reports[0].OK || len(reports[0].Problems) != 0 {
		t.Fatalf("valid references or unlisted game class rejected: %+v", reports[0])
	}
	bad := batchFile("cfgspawnabletypes.xml", "<spawnabletypes>\n<type name=\"AKM\"><attachments preset=\"Food\"/><cargo preset=\"Missing\"/></type>\n</spawnabletypes>")
	reports = ValidateSet([]InputFile{bad, presets}, "auto")
	if reports[0].OK || !batchHas(reports[0], "error", `attachments preset="Food"`) || !batchHas(reports[0], "error", `cargo preset="Missing"`) {
		t.Fatalf("unknown or wrong-kind presets not rejected: %+v", reports[0])
	}
	for _, problem := range reports[0].Problems {
		if strings.Contains(problem.Msg, "preset=") && problem.Line != 2 {
			t.Fatalf("preset line lost: %+v", problem)
		}
	}
}

func TestBatchMissingPresetFileWarns(t *testing.T) {
	spawn := batchFile("spawn.xml", `<spawnabletypes><type name="X"><cargo preset="Missing"/></type></spawnabletypes>`)
	for _, files := range [][]InputFile{
		{spawn},
		{spawn, batchFile("presets.xml", `<randompresets><cargo name="Missing" chance="1"/></randompresets>garbage`)},
	} {
		report := ValidateSet(files, "auto")[0]
		if !report.OK || !batchHas(report, "warn", "нельзя проверить") {
			t.Fatalf("unavailable preset declarations need a warning: %+v", report)
		}
	}
}

func TestBatchEmptyPresetReferenceIsAnError(t *testing.T) {
	report := ValidateSet([]InputFile{batchFile("spawn.xml", `<spawnabletypes><type name="X"><cargo preset=" "/><attachments/></type></spawnabletypes>`)}, "auto")[0]
	if report.OK || !batchHas(report, "error", "пустое имя preset") {
		t.Fatalf("empty explicit preset reference accepted: %+v", report)
	}
}

func TestBatchMissionLimitsAndUserLists(t *testing.T) {
	types := batchFile("db/types.xml", `<types><type name="Custom"><nominal>1</nominal><min>0</min><lifetime>1</lifetime><flags count_in_map="1"/><category name="CustomCategory"/><tag name="CustomTag"/><usage name="CustomUsage"/><value name="CustomTier"/><usage user="TownVillage"/><value user="TierCustom"/></type></types>`)
	lists := batchFile("cfglimitsdefinition.xml", `<lists><categories><category name="CustomCategory"/></categories><tags><tag name="CustomTag"/></tags><usageflags><usage name="CustomUsage"/></usageflags><valueflags><value name="CustomTier"/></valueflags></lists>`)
	users := batchFile("cfglimitsdefinitionuser.xml", `<user_lists><usageflags><user name="TownVillage"><usage name="Town"/><usage name="Village"/></user></usageflags><valueflags><user name="TierCustom"><value name="CustomTier"/></user></valueflags></user_lists>`)
	reports := ValidateSet([]InputFile{types, lists, users}, "auto")
	if !reports[0].OK || len(reports[0].Problems) != 0 {
		t.Fatalf("mission-defined names should resolve before checking types: %+v", reports[0])
	}
	regularGroup := batchFile("types.xml", `<types><type name="Custom"><usage name="TownVillage"/><value name="TierCustom"/></type></types>`)
	report := ValidateSet([]InputFile{regularGroup, lists, users}, "auto")[0]
	if !batchHas(report, "warn", `usage name="TownVillage"`) || !batchHas(report, "warn", `value name="TierCustom"`) {
		t.Fatalf("user groups must not become regular flags: %+v", report)
	}
	// A subsequent request must start with a fresh catalog.
	report = ValidateSet([]InputFile{types}, "auto")[0]
	if !batchHas(report, "warn", "CustomCategory") {
		t.Fatalf("catalog leaked between requests: %+v", report)
	}
}

func TestBatchInvalidLimitsDoNotSuppressWarnings(t *testing.T) {
	reports := ValidateSet([]InputFile{
		batchFile("types.xml", `<types><type name="X"><usage name="Fake"/></type></types>`),
		batchFile("cfglimitsdefinition.xml", `<lists><usageflags><usage name="Fake"/></usageflags></lists><extra/>`),
	}, "auto")
	if !batchHas(reports[0], "warn", "Fake") || reports[1].OK {
		t.Fatalf("broken catalog must not define flags: %+v", reports)
	}
}

func TestBatchCEPathsAndRootTypes(t *testing.T) {
	core := batchFile("mission/cfgeconomycore.xml", "<economycore>\n<ce folder=\"custom\\pack\"><file name=\"types.xml\" type=\"types\"/></ce>\n</economycore>")
	types := batchFile("mission/custom/pack/types.xml", `<types/>`)
	reports := ValidateSet([]InputFile{core, types}, "auto")
	if len(reports[0].Problems) != 0 || len(reports[1].Problems) != 0 || !reports[0].OK || !reports[1].OK {
		t.Fatalf("mission-relative path resolution failed: %+v / %+v", reports[0], reports[1])
	}
	reports = ValidateSet([]InputFile{core, batchFile(types.Name, `<events/>`)}, "auto")
	if reports[0].OK || !batchHas(reports[0], "error", "корень файла — <events>") {
		t.Fatalf("CE root/type mismatch must be an error: %+v", reports[0])
	}
	for _, problem := range reports[0].Problems {
		if problem.Level == "error" && problem.Line != 2 {
			t.Fatalf("CE line lost: %+v", problem)
		}
	}
	reports = ValidateSet([]InputFile{core, batchFile(types.Name, `{}`)}, "auto")
	if reports[0].OK || !batchHas(reports[0], "error", "требует XML") {
		t.Fatalf("non-XML CE target accepted: %+v", reports[0])
	}
}

func TestBatchCEPartialUploadsAndDuplicates(t *testing.T) {
	core := batchFile("cfgeconomycore.xml", `<economycore><ce folder="custom"><file name="types.xml" type="types"/><file name="./TYPES.xml" type="types"/></ce></economycore>`)
	report := ValidateSet([]InputFile{core}, "auto")[0]
	if !report.OK || !batchHas(report, "warn", "не загружен") || !batchHas(report, "warn", "повторно") {
		t.Fatalf("partial upload/duplicate registration diagnostics missing: %+v", report)
	}
	// A matching basename is insufficient when the registration names a folder.
	reports := ValidateSet([]InputFile{core, batchFile("types.xml", `<types/>`)}, "auto")
	if !batchHas(reports[0], "warn", "не загружен") {
		t.Fatalf("basename fallback incorrectly resolved a CE path: %+v", reports[0])
	}
}

func TestBatchCERejectsTraversalAndAbsolutePaths(t *testing.T) {
	for _, unsafe := range []string{"../outside", "custom/../outside", "/absolute", `C:\absolute`, `\\server\share`} {
		for _, field := range []string{"folder", "name"} {
			folder, name := "custom", "types.xml"
			if field == "folder" {
				folder = unsafe
			} else {
				name = unsafe
			}
			core := `<economycore><ce folder="` + folder + `"><file name="` + name + `" type="types"/></ce></economycore>`
			report := ValidateSet([]InputFile{batchFile("cfgeconomycore.xml", core)}, "auto")[0]
			if report.OK || !batchHas(report, "error", field+" должен") {
				t.Errorf("unsafe %s=%q accepted: %+v", field, unsafe, report)
			}
		}
	}
}

func TestBatchCECommentsAndVanillaDefaults(t *testing.T) {
	reports := ValidateSet([]InputFile{
		batchFile("mission/cfgeconomycore.xml", `<economycore><!-- <ce folder="bad"><file name="missing.xml" type="types"/></ce> & --></economycore>`),
		batchFile("mission/db/types.xml", `<types/>`),
		batchFile("mission/cfgspawnabletypes.xml", `<spawnabletypes/>`),
		batchFile("mission/custom/extra.xml", `<types/>`),
		batchFile("db/types.xml", `<types/>`),
	}, "auto")
	if len(reports[0].Problems) != 0 || len(reports[1].Problems) != 0 || len(reports[2].Problems) != 0 {
		t.Fatalf("commented registrations or vanilla files caused warnings: %+v / %+v / %+v", reports[0], reports[1], reports[2])
	}
	if !batchHas(reports[3], "warn", "не зарегистрирован") {
		t.Fatalf("unregistered custom file not identified: %+v", reports[3])
	}
	if batchHas(reports[4], "warn", "не зарегистрирован") {
		t.Fatalf("standard mission-relative db/types.xml must be exempt: %+v", reports[4])
	}
}

func TestBatchRespectsExplicitNonXMLFormat(t *testing.T) {
	reports := ValidateSet([]InputFile{
		batchFile("a.xml", `<types><type name="X"/></types>`),
		batchFile("b.xml", `<types><type name="X"/></types>`),
	}, "text")
	if reports[0].Kind != "text" || batchHas(reports[1], "warn", "дубликат") {
		t.Fatalf("forced text files must not enter mission checks: %+v", reports)
	}
}
