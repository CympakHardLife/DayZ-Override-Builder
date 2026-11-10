// Package diffcfg compares two versions of a mod (old vs new PBO set)
// and reports added / removed / changed classes with property-level detail.
package diffcfg

import (
	"fmt"
	"sort"
	"strings"

	"dayzoverride/analyze"
	"dayzoverride/rap"
)

type PropChange struct {
	Path string `json:"path"`
	Kind string `json:"kind"` // added | removed | changed
	Old  string `json:"old,omitempty"`
	New  string `json:"new,omitempty"`
}

type ClassDiff struct {
	Classname     string       `json:"classname"`
	Category      string       `json:"category"`
	OldParent     string       `json:"oldParent,omitempty"`
	NewParent     string       `json:"newParent,omitempty"`
	ParentChanged bool         `json:"parentChanged,omitempty"`
	Changes       []PropChange `json:"changes"`
}

type ClassBrief struct {
	Classname   string `json:"classname"`
	Category    string `json:"category"`
	Parent      string `json:"parent"`
	DisplayName string `json:"displayName,omitempty"`
}

type Result struct {
	OldPbos    []analyze.PboInfo `json:"oldPbos"`
	NewPbos    []analyze.PboInfo `json:"newPbos"`
	PatchesOld []string          `json:"patchesOld"`
	PatchesNew []string          `json:"patchesNew"`
	Added      []ClassBrief      `json:"added"`
	Removed    []ClassBrief      `json:"removed"`
	Changed    []ClassDiff       `json:"changed"`
	Unchanged  int               `json:"unchanged"`
	Warnings   []string          `json:"warnings"`
	Report     string            `json:"report"`
}

type classRec struct {
	cls     *rap.Class
	section string
}

const (
	maxChangesPerClass = 200
	maxTotalChanges    = 20000
)

// Compare loads both PBO sets and diffs their item classes.
func Compare(oldPaths, newPaths []string, logf analyze.Logf) (*Result, error) {
	res := &Result{}

	logf("=== Старая версия ===")
	oldCfgs, oldPbos, oldWarns := analyze.LoadConfigs(oldPaths, analyze.Logf(logf))
	logf("=== Новая версия ===")
	newCfgs, newPbos, newWarns := analyze.LoadConfigs(newPaths, analyze.Logf(logf))

	res.OldPbos, res.NewPbos = oldPbos, newPbos
	for _, w := range oldWarns {
		res.Warnings = append(res.Warnings, "[старая] "+w)
	}
	for _, w := range newWarns {
		res.Warnings = append(res.Warnings, "[новая] "+w)
	}
	if len(oldCfgs) == 0 {
		return nil, fmt.Errorf("в старой версии не найдено читаемых конфигов")
	}
	if len(newCfgs) == 0 {
		return nil, fmt.Errorf("в новой версии не найдено читаемых конфигов")
	}

	oldMap, oldPatches := collect(oldCfgs)
	newMap, newPatches := collect(newCfgs)
	res.PatchesOld, res.PatchesNew = oldPatches, newPatches

	logf("Сравниваю классы: старых %d, новых %d…", len(oldMap), len(newMap))

	totalChanges := 0
	// removed & changed (стабильный порядок по имени)
	var oldKeys []string
	for k := range oldMap {
		oldKeys = append(oldKeys, k)
	}
	sort.Strings(oldKeys)
	for _, k := range oldKeys {
		o := oldMap[k]
		n, ok := newMap[k]
		if !ok {
			res.Removed = append(res.Removed, brief(o))
			continue
		}
		cd := ClassDiff{
			Classname: o.cls.Name,
			Category:  o.section,
			OldParent: o.cls.Parent,
			NewParent: n.cls.Parent,
		}
		if !strings.EqualFold(o.cls.Parent, n.cls.Parent) {
			cd.ParentChanged = true
		}
		diffBody("", o.cls, n.cls, &cd.Changes)
		if len(cd.Changes) > maxChangesPerClass {
			cd.Changes = cd.Changes[:maxChangesPerClass]
			cd.Changes = append(cd.Changes, PropChange{Path: "…", Kind: "changed", New: "слишком много изменений, список обрезан"})
		}
		if cd.ParentChanged || len(cd.Changes) > 0 {
			totalChanges += len(cd.Changes)
			res.Changed = append(res.Changed, cd)
			if totalChanges > maxTotalChanges {
				res.Warnings = append(res.Warnings, "изменений слишком много — отчёт обрезан")
				break
			}
		} else {
			res.Unchanged++
		}
	}
	// added
	var newKeys []string
	for k := range newMap {
		if _, ok := oldMap[k]; !ok {
			newKeys = append(newKeys, k)
		}
	}
	sort.Strings(newKeys)
	for _, k := range newKeys {
		res.Added = append(res.Added, brief(newMap[k]))
	}

	logf("Сверка завершена: новых %d, удалённых %d, изменённых %d, без изменений %d",
		len(res.Added), len(res.Removed), len(res.Changed), res.Unchanged)

	res.Report = buildReport(res)
	return res, nil
}

func collect(cfgs []analyze.ParsedConfig) (map[string]classRec, []string) {
	m := map[string]classRec{}
	var patches []string
	for _, pc := range cfgs {
		for _, secName := range analyze.Sections {
			sec := pc.Root.Sub(secName)
			if sec == nil {
				continue
			}
			for _, ce := range sec.Classes() {
				key := strings.ToLower(secName) + "/" + strings.ToLower(ce.Name)
				m[key] = classRec{cls: ce.Class, section: secName}
			}
		}
		if cp := pc.Root.Sub("CfgPatches"); cp != nil {
			for _, pe := range cp.Classes() {
				patches = append(patches, pe.Name)
			}
		}
	}
	return m, patches
}

func brief(r classRec) ClassBrief {
	b := ClassBrief{Classname: r.cls.Name, Category: r.section, Parent: r.cls.Parent}
	if v := r.cls.Prop("displayName"); v != nil {
		b.DisplayName = v.String()
	}
	return b
}

func render(e *rap.Entry) string {
	switch e.Kind {
	case rap.KindValue:
		return e.Value.String()
	case rap.KindArray:
		av := &rap.Value{Kind: rap.VArray, Arr: e.Array}
		return av.String()
	case rap.KindExtern:
		return "class " + e.Name + ";"
	case rap.KindDelete:
		return "delete " + e.Name + ";"
	case rap.KindClass:
		return "class { … }"
	}
	return ""
}

// diffBody сравнивает СОБСТВЕННЫЕ записи класса (без наследования) рекурсивно.
func diffBody(prefix string, a, b *rap.Class, out *[]PropChange) {
	if len(*out) > maxChangesPerClass {
		return
	}
	bmap := map[string]*rap.Entry{}
	for _, e := range b.Entries {
		bmap[strings.ToLower(e.Name)] = e
	}
	amap := map[string]*rap.Entry{}
	seen := map[string]bool{}
	for _, e := range a.Entries {
		key := strings.ToLower(e.Name)
		amap[key] = e
		if seen[key] {
			continue
		}
		seen[key] = true
		be := bmap[key]
		path := prefix + e.Name
		if be == nil {
			*out = append(*out, PropChange{Path: path, Kind: "removed", Old: render(e)})
			continue
		}
		if e.Kind != be.Kind {
			*out = append(*out, PropChange{Path: path, Kind: "changed", Old: render(e), New: render(be)})
			continue
		}
		switch e.Kind {
		case rap.KindValue, rap.KindArray:
			ra, rb := render(e), render(be)
			if ra != rb {
				*out = append(*out, PropChange{Path: path, Kind: "changed", Old: ra, New: rb})
			}
		case rap.KindClass:
			if !strings.EqualFold(e.Class.Parent, be.Class.Parent) {
				*out = append(*out, PropChange{Path: path + " (родитель)", Kind: "changed", Old: e.Class.Parent, New: be.Class.Parent})
			}
			diffBody(path+"/", e.Class, be.Class, out)
		}
	}
	for _, e := range b.Entries {
		key := strings.ToLower(e.Name)
		if _, ok := amap[key]; !ok {
			*out = append(*out, PropChange{Path: prefix + e.Name, Kind: "added", New: render(e)})
		}
	}
}

func buildReport(r *Result) string {
	var sb strings.Builder
	sb.WriteString("ОТЧЁТ СВЕРКИ ВЕРСИЙ МОДА (DayZ Override Builder)\n")
	sb.WriteString(strings.Repeat("=", 68) + "\n\n")
	sb.WriteString(fmt.Sprintf("CfgPatches старой версии: %s\n", strings.Join(r.PatchesOld, ", ")))
	sb.WriteString(fmt.Sprintf("CfgPatches новой версии:  %s\n\n", strings.Join(r.PatchesNew, ", ")))
	sb.WriteString(fmt.Sprintf("ИТОГО: новых классов %d, удалённых %d, изменённых %d, без изменений %d\n\n",
		len(r.Added), len(r.Removed), len(r.Changed), r.Unchanged))

	if len(r.Added) > 0 {
		sb.WriteString("НОВЫЕ КЛАССНЕЙМЫ (появились в новой версии)\n" + strings.Repeat("-", 60) + "\n")
		for _, c := range r.Added {
			sb.WriteString(fmt.Sprintf("  + [%s] %s : %s", c.Category, c.Classname, c.Parent))
			if c.DisplayName != "" {
				sb.WriteString("  (" + c.DisplayName + ")")
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}
	if len(r.Removed) > 0 {
		sb.WriteString("УДАЛЁННЫЕ КЛАССНЕЙМЫ (были в старой, нет в новой — ваши оверрайды по ним «повиснут»)\n" + strings.Repeat("-", 60) + "\n")
		for _, c := range r.Removed {
			sb.WriteString(fmt.Sprintf("  - [%s] %s : %s\n", c.Category, c.Classname, c.Parent))
		}
		sb.WriteString("\n")
	}
	if len(r.Changed) > 0 {
		sb.WriteString("ИЗМЕНЁННЫЕ КЛАССЫ\n" + strings.Repeat("-", 60) + "\n")
		for _, c := range r.Changed {
			sb.WriteString(fmt.Sprintf("~ [%s] %s\n", c.Category, c.Classname))
			if c.ParentChanged {
				sb.WriteString(fmt.Sprintf("    БАЗОВЫЙ КЛАСС: %s -> %s\n", c.OldParent, c.NewParent))
			}
			for _, ch := range c.Changes {
				switch ch.Kind {
				case "added":
					sb.WriteString(fmt.Sprintf("    + %s = %s\n", ch.Path, ch.New))
				case "removed":
					sb.WriteString(fmt.Sprintf("    - %s (было: %s)\n", ch.Path, ch.Old))
				default:
					sb.WriteString(fmt.Sprintf("    ~ %s: %s -> %s\n", ch.Path, ch.Old, ch.New))
				}
			}
		}
		sb.WriteString("\n")
	}
	if len(r.Warnings) > 0 {
		sb.WriteString("ПРЕДУПРЕЖДЕНИЯ\n" + strings.Repeat("-", 60) + "\n")
		for _, w := range r.Warnings {
			sb.WriteString("  ! " + w + "\n")
		}
	}
	return sb.String()
}
