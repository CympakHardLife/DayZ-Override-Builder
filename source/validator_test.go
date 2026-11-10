package validator

import "testing"

// --- XML: правильные файлы должны проходить ---

func TestXMLValidPasses(t *testing.T) {
	cases := map[string]string{
		"types с декларацией": `<?xml version="1.0" encoding="UTF-8"?>
<types>
    <type name="AKM">
        <nominal>5</nominal>
        <min>2</min>
        <lifetime>7200</lifetime>
        <flags count_in_cargo="0" count_in_map="1"/>
    </type>
</types>
`,
		"пустой корень":     `<types></types>`,
		"самозакрытый корень": `<types/>`,
		"с комментарием":    "<!-- шапка -->\n<types><type name=\"X\"><min>0</min></type></types>\n",
	}
	for name, in := range cases {
		rep := Validate("types.xml", []byte(in))
		if !rep.OK {
			t.Errorf("%s: ожидался OK, но найдены ошибки: %+v", name, rep.Problems)
		}
	}
}

// --- XML: ошибки, которые раньше МОЛЧА пропускались ---

func TestXMLStrictCatchesPreviouslyMissed(t *testing.T) {
	cases := map[string]string{
		"мусор после корня":        `<types><type name="X"></type></types> ЭТО ЛИШНИЙ ТЕКСТ`,
		"текст-хвост после закрытия": `<types/>trailing text`,
		"второй корневой элемент":   `<types></types><types></types>`,
		"элемент после корня":       `<types></types><extra/>`,
		"мусор перед корнем":        `лишний текст перед <types></types>`,
		"дубликат атрибута":         `<types><type name="A" name="B"></type></types>`,
	}
	for name, in := range cases {
		rep := Validate("types.xml", []byte(in))
		if rep.OK {
			t.Errorf("%s: ДОЛЖНА быть ошибка, но валидатор сказал OK. problems=%+v", name, rep.Problems)
		}
	}
}

// --- XML: ошибки, которые ловились и раньше (не должны сломаться) ---

func TestXMLStillCatchesBasic(t *testing.T) {
	cases := map[string]string{
		"несовпадение тегов": `<types><type></wrong></types>`,
		"незакрытый тег":     `<types><type name="X">`,
		"голый амперсанд":    `<types><type name="A & B"></type></types>`,
	}
	for name, in := range cases {
		rep := Validate("types.xml", []byte(in))
		if rep.OK {
			t.Errorf("%s: ожидалась ошибка, получено OK", name)
		}
	}
}

// --- Явно выбранный формат ---

func TestFormatForcesParser(t *testing.T) {
	// XML-содержимое, но пользователь сказал «JSON» — должна быть ошибка JSON.
	rep := ValidateAs("noext", []byte(`<types></types>`), "json")
	if rep.Kind != "json" {
		t.Errorf("ожидался kind=json, получено %q", rep.Kind)
	}
	if rep.OK {
		t.Errorf("XML, проверяемый как JSON, должен давать ошибку")
	}

	// Файл без расширения, но пользователь сказал «types» — XML-проверка.
	rep = ValidateAs("noext", []byte(`<types></types>`), "types")
	if rep.Kind != "xml" || !rep.OK {
		t.Errorf("types-формат: ожидался валидный xml, получено kind=%q ok=%v problems=%+v", rep.Kind, rep.OK, rep.Problems)
	}
}

func TestFormatRootMismatchWarns(t *testing.T) {
	// Выбрали types, а корень — spawnabletypes: файл валиден (well-formed),
	// но должно быть предупреждение о несоответствии.
	rep := ValidateAs("f.xml", []byte(`<spawnabletypes></spawnabletypes>`), "types")
	if !rep.OK {
		t.Errorf("well-formed XML не должен считаться ошибкой из-за несоответствия корня: %+v", rep.Problems)
	}
	foundWarn := false
	for _, p := range rep.Problems {
		if p.Level == "warn" {
			foundWarn = true
		}
	}
	if !foundWarn {
		t.Errorf("ожидалось предупреждение о несоответствии корня, problems=%+v", rep.Problems)
	}
}

// --- JSON: строгость (как jsonlint) ---

func TestJSONStrict(t *testing.T) {
	good := `{"Title":"t","ID":"1","Objectives":[]}`
	if rep := Validate("q.json", []byte(good)); !rep.OK {
		t.Errorf("валидный JSON помечен ошибочным: %+v", rep.Problems)
	}
	bad := map[string]string{
		"хвостовая запятая":  `{"a":1,}`,
		"одинарные кавычки":  `{'a':1}`,
		"комментарий":        "{\n// c\n\"a\":1}",
		"без запятой":        `{"a":1 "b":2}`,
		"мусор после json":   `{"a":1} лишнее`,
		"обрыв":              `{"a":1`,
	}
	for name, in := range bad {
		if rep := Validate("q.json", []byte(in)); rep.OK {
			t.Errorf("%s: ожидалась ошибка JSON, получено OK", name)
		}
	}
}

// --- Текст (config.cpp): баланс скобок ---

func TestTextBraces(t *testing.T) {
	good := `class CfgPatches { class Mod { units[] = {}; }; };`
	if rep := Validate("config.cpp", []byte(good)); !rep.OK {
		t.Errorf("валидный config.cpp помечен ошибочным: %+v", rep.Problems)
	}
	bad := `class CfgPatches { class Mod { units[] = {}; };`
	if rep := Validate("config.cpp", []byte(bad)); rep.OK {
		t.Errorf("незакрытая скобка должна давать ошибку")
	}
}

// ==================== DayZ-специфичные проверки ====================

func hasWarn(rep *FileReport) bool {
	for _, p := range rep.Problems {
		if p.Level == "warn" {
			return true
		}
	}
	return false
}

// Главный кейс из отчёта пользователя: мусор МЕЖДУ блоками внутри <types>.
func TestDayZMixedContentBetweenTags(t *testing.T) {
	in := `<types>
  <type name="A"><nominal>1</nominal><min>0</min></type>1231
  <type name="B"><nominal>1</nominal><min>0</min></type>
</types>`
	rep := Validate("types.xml", []byte(in))
	if rep.OK {
		t.Errorf("посторонний текст между тегами должен быть ошибкой; problems=%+v", rep.Problems)
	}
}

func TestDayZFlagsMustBe01(t *testing.T) {
	in := `<types><type name="A"><nominal>1</nominal><min>0</min><lifetime>100</lifetime><flags count_in_map="2" count_in_cargo="0"/></type></types>`
	if rep := Validate("types.xml", []byte(in)); rep.OK {
		t.Errorf("флаг=2 должен быть ошибкой")
	}
}

func TestDayZNominalLessMinIsWarning(t *testing.T) {
	// корректно по форме, в игре грузится → предупреждение, НЕ ошибка
	in := `<types><type name="A"><nominal>1</nominal><min>5</min><lifetime>100</lifetime><flags count_in_map="1"/></type></types>`
	rep := Validate("types.xml", []byte(in))
	if !rep.OK {
		t.Errorf("nominal<min должно быть предупреждением, не ошибкой: %+v", rep.Problems)
	}
	if !hasWarn(rep) {
		t.Errorf("ожидалось предупреждение для nominal<min")
	}
}

func TestDayZNonIntegerIsError(t *testing.T) {
	in := `<types><type name="A"><nominal>abc</nominal><min>0</min></type></types>`
	if rep := Validate("types.xml", []byte(in)); rep.OK {
		t.Errorf("нечисловой nominal должен быть ошибкой")
	}
}

func TestDayZUnknownElementWarns(t *testing.T) {
	in := `<types><type name="A"><nominl>1</nominl><min>0</min><lifetime>100</lifetime><flags count_in_map="1"/></type></types>`
	rep := Validate("types.xml", []byte(in))
	if !rep.OK {
		t.Errorf("неизвестный тег должен быть предупреждением, не ошибкой: %+v", rep.Problems)
	}
	if !hasWarn(rep) {
		t.Errorf("ожидалось предупреждение для <nominl>")
	}
}

func TestDayZMissingNameIsError(t *testing.T) {
	in := `<types><type><nominal>1</nominal><min>0</min></type></types>`
	if rep := Validate("types.xml", []byte(in)); rep.OK {
		t.Errorf("<type> без name должен быть ошибкой")
	}
}

func TestDayZUnknownUsageWarns(t *testing.T) {
	in := `<types><type name="A"><nominal>1</nominal><min>0</min><lifetime>100</lifetime><flags count_in_map="1"/><usage name="ВыдуманныйЮзейдж"/></type></types>`
	rep := Validate("types.xml", []byte(in))
	if !rep.OK {
		t.Errorf("неизвестный usage не должен считаться ошибкой (моды): %+v", rep.Problems)
	}
	if !hasWarn(rep) {
		t.Errorf("ожидалось предупреждение для неизвестного usage")
	}
}

func TestSpawnableChanceRange(t *testing.T) {
	good := `<spawnabletypes><type name="X"><attachments chance="0.5"><item name="Y" chance="0.3"/></attachments></type></spawnabletypes>`
	if rep := Validate("cfgspawnabletypes.xml", []byte(good)); !rep.OK {
		t.Errorf("валидный cfgspawnabletypes помечен ошибочным: %+v", rep.Problems)
	}
	bad := `<spawnabletypes><type name="X"><cargo chance="1.5"><item name="Y" chance="0.3"/></cargo></type></spawnabletypes>`
	if rep := Validate("cfgspawnabletypes.xml", []byte(bad)); rep.OK {
		t.Errorf("chance=1.5 должен быть ошибкой")
	}
	bad2 := `<spawnabletypes><type name="X"><cargo chance="abc"/></type></spawnabletypes>`
	if rep := Validate("cfgspawnabletypes.xml", []byte(bad2)); rep.OK {
		t.Errorf("нечисловой chance должен быть ошибкой")
	}
}

func TestRandomPresetsNameAndChance(t *testing.T) {
	good := `<randompresets><cargo chance="1.0" name="P1"><item name="X" chance="0.8"/></cargo></randompresets>`
	if rep := Validate("cfgrandompresets.xml", []byte(good)); !rep.OK {
		t.Errorf("валидный cfgrandompresets помечен ошибочным: %+v", rep.Problems)
	}
	noName := `<randompresets><cargo chance="1.0"><item name="X" chance="0.8"/></cargo></randompresets>`
	if rep := Validate("cfgrandompresets.xml", []byte(noName)); rep.OK {
		t.Errorf("пресет без name должен быть ошибкой")
	}
}
