package mergecfg

import (
	"strings"
	"testing"

	"dayzoverride/analyze"
	"dayzoverride/rap"
)

func parse(t *testing.T, name, src string) analyze.ParsedConfig {
	root, _, err := rap.ParseText(src)
	if err != nil {
		t.Fatal(err)
	}
	return analyze.ParsedConfig{Root: root, PboName: name}
}

func TestMergeConflictLastWins(t *testing.T) {
	a := parse(t, "ov1.cpp", `
class CfgPatches { class Ov1 { requiredAddons[] = {"ModA"}; }; };
class CfgVehicles {
	class GhillieSuit;
	class Item1: GhillieSuit { itemSize[] = {3,3}; weight = 500; };
};`)
	b := parse(t, "ov2.cpp", `
class CfgPatches { class Ov2 { requiredAddons[] = {"ModB","ModA"}; }; };
class CfgVehicles {
	class GhillieSuit;
	class Item1: GhillieSuit { itemSize[] = {5,5}; };
	class Item2: GhillieSuit { weight = 100; };
};`)
	res, err := Merge([]analyze.ParsedConfig{a, b}, func(string, ...interface{}) {})
	if err != nil {
		t.Fatal(err)
	}
	// requiredAddons: union минус собственные патчи
	req := strings.Join(res.RequiredAddons, ",")
	if !strings.Contains(req, "ModA") || !strings.Contains(req, "ModB") {
		t.Fatalf("required=%v", res.RequiredAddons)
	}
	if strings.Contains(req, "Ov1") || strings.Contains(req, "Ov2") {
		t.Fatalf("own patches must be excluded: %v", res.RequiredAddons)
	}
	// конфликт itemSize зафиксирован
	found := false
	for _, c := range res.Conflicts {
		if strings.Contains(c, "Item1") && strings.Contains(c, "itemSize") {
			found = true
		}
	}
	if !found {
		t.Fatalf("conflict not recorded: %v", res.Conflicts)
	}
	cfg := BuildConfig(res, "Merged")
	// последний файл победил
	if !strings.Contains(cfg, "itemSize[] = {5,5}") {
		t.Fatalf("last must win:\n%s", cfg)
	}
	// weight из первого сохранился (не конфликтовал)
	if !strings.Contains(cfg, "weight = 500") {
		t.Fatalf("non-conflicting prop lost:\n%s", cfg)
	}
	// оба класса присутствуют
	if !strings.Contains(cfg, "class Item2: GhillieSuit") {
		t.Fatalf("Item2 lost:\n%s", cfg)
	}
	// сгенерированный конфиг снова парсится
	if _, _, err := rap.ParseText(cfg); err != nil {
		t.Fatalf("merged config unparsable: %v", err)
	}
}

func TestMergeNestedClasses(t *testing.T) {
	a := parse(t, "a.cpp", `
class CfgPatches { class A { requiredAddons[] = {"X"}; }; };
class CfgVehicles { class Base; class It: Base { class Protection { biological = 1; }; }; };`)
	b := parse(t, "b.cpp", `
class CfgPatches { class B { requiredAddons[] = {"X"}; }; };
class CfgVehicles { class Base; class It: Base { class Protection { chemical = 0.5; }; }; };`)
	res, err := Merge([]analyze.ParsedConfig{a, b}, func(string, ...interface{}) {})
	if err != nil {
		t.Fatal(err)
	}
	cfg := BuildConfig(res, "Merged")
	if !strings.Contains(cfg, "biological = 1") || !strings.Contains(cfg, "chemical = 0.5") {
		t.Fatalf("nested merge failed:\n%s", cfg)
	}
	if len(res.Conflicts) != 0 {
		t.Fatalf("unexpected conflicts: %v", res.Conflicts)
	}
}
