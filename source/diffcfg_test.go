package diffcfg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, dir, name, src string) string {
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCompare(t *testing.T) {
	dir := t.TempDir()
	oldCfg := write(t, dir, "old.cpp", `
class CfgPatches { class GunPack { requiredAddons[] = {"DZ_Data"}; }; };
class CfgWeapons {
	class Rifle_Base;
	class GP_M4: Rifle_Base { displayName = "M4"; itemSize[] = {5,2};
		class DamageSystem { class GlobalHealth { class Health { hitpoints = 200; }; }; };
	};
	class GP_OldGun: Rifle_Base { displayName = "Old"; };
	class GP_Same: Rifle_Base { displayName = "Same"; };
};`)
	newCfg := write(t, dir, "new.cpp", `
class CfgPatches { class GunPack { requiredAddons[] = {"DZ_Data","DZ_Weapons_Firearms"}; }; };
class CfgWeapons {
	class Rifle_Base;
	class Pistol_Base;
	class GP_M4: Pistol_Base { displayName = "M4 Block II"; itemSize[] = {6,2};
		class DamageSystem { class GlobalHealth { class Health { hitpoints = 300; }; }; };
	};
	class GP_Same: Rifle_Base { displayName = "Same"; };
	class GP_NewGun: Rifle_Base { displayName = "Novinka"; };
};`)
	res, err := Compare([]string{oldCfg}, []string{newCfg}, func(string, ...interface{}) {})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Added) != 1 || res.Added[0].Classname != "GP_NewGun" {
		t.Fatalf("added=%+v", res.Added)
	}
	if len(res.Removed) != 1 || res.Removed[0].Classname != "GP_OldGun" {
		t.Fatalf("removed=%+v", res.Removed)
	}
	if len(res.Changed) != 1 || res.Changed[0].Classname != "GP_M4" {
		t.Fatalf("changed=%+v", res.Changed)
	}
	cd := res.Changed[0]
	if !cd.ParentChanged || cd.NewParent != "Pistol_Base" {
		t.Fatalf("parent change missed: %+v", cd)
	}
	joined := ""
	for _, ch := range cd.Changes {
		joined += ch.Path + ": " + ch.Old + " -> " + ch.New + "\n"
	}
	for _, must := range []string{"displayName", "itemSize", "DamageSystem/GlobalHealth/Health/hitpoints: 200 -> 300"} {
		if !strings.Contains(joined, must) {
			t.Fatalf("missing %q in changes:\n%s", must, joined)
		}
	}
	if res.Unchanged != 1 {
		t.Fatalf("unchanged=%d", res.Unchanged)
	}
	if !strings.Contains(res.Report, "GP_NewGun") || !strings.Contains(res.Report, "БАЗОВЫЙ КЛАСС") {
		t.Fatalf("report incomplete:\n%s", res.Report)
	}
}
