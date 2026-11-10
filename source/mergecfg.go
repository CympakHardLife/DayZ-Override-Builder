// Package mergecfg merges several override addons (config.cpp / PBO) into one,
// detecting and reporting conflicts. Порядок входных файлов = приоритет:
// последний файл главнее при конфликте значений.
package mergecfg

import (
	"fmt"
	"strings"

	"dayzoverride/analyze"
	"dayzoverride/rap"
)

type Result struct {
	Root           *rap.Class // merged sections (без CfgPatches)
	RequiredAddons []string
	OwnPatches     []string
	Conflicts      []string
	Warnings       []string
}

// Merge объединяет разобранные конфиги оверрайдов.
func Merge(inputs []analyze.ParsedConfig, logf analyze.Logf) (*Result, error) {
	if len(inputs) == 0 {
		return nil, fmt.Errorf("не передано ни одного конфига для слияния")
	}
	res := &Result{Root: &rap.Class{}}
	reqSeen := map[string]bool{}
	ownSeen := map[string]bool{}
	// владельцы значений для сообщений о конфликтах: path(lower) -> имя файла
	owner := map[string]string{}

	for _, in := range inputs {
		logf("Сливаю: %s", in.PboName)
		for _, e := range in.Root.Entries {
			switch e.Kind {
			case rap.KindClass:
				switch strings.ToLower(e.Name) {
				case "cfgpatches":
					for _, pe := range e.Class.Classes() {
						if !ownSeen[strings.ToLower(pe.Name)] {
							ownSeen[strings.ToLower(pe.Name)] = true
							res.OwnPatches = append(res.OwnPatches, pe.Name)
						}
						for _, ra := range pe.Class.ArrayProp("requiredAddons") {
							v := ra.String()
							if !reqSeen[strings.ToLower(v)] {
								reqSeen[strings.ToLower(v)] = true
								res.RequiredAddons = append(res.RequiredAddons, v)
							}
						}
					}
				case "cfgmods":
					// служебная секция — в объединённом аддоне не нужна
				default:
					dst := res.Root.Sub(e.Name)
					if dst == nil {
						dst = &rap.Class{Name: e.Name}
						res.Root.Entries = append(res.Root.Entries, &rap.Entry{Kind: rap.KindClass, Name: e.Name, Class: dst})
					}
					mergeClass(dst, e.Class, strings.ToLower(e.Name), e.Name, in.PboName, owner, res, logf)
				}
			case rap.KindValue, rap.KindArray:
				res.Warnings = append(res.Warnings, fmt.Sprintf("%s: параметр %q на верхнем уровне конфига пропущен", in.PboName, e.Name))
			case rap.KindExtern, rap.KindDelete:
				// глобальные extern/delete редки — переносим как есть
				appendUnique(res.Root, e)
			}
		}
	}
	// исключаем собственные патчи входных аддонов из requiredAddons
	var cleaned []string
	for _, ra := range res.RequiredAddons {
		if !ownSeen[strings.ToLower(ra)] {
			cleaned = append(cleaned, ra)
		}
	}
	res.RequiredAddons = cleaned
	logf("Слияние завершено: %d конфликтов, %d предупреждений", len(res.Conflicts), len(res.Warnings))
	return res, nil
}

func appendUnique(dst *rap.Class, e *rap.Entry) {
	for _, x := range dst.Entries {
		if x.Kind == e.Kind && strings.EqualFold(x.Name, e.Name) {
			return
		}
	}
	c := *e
	dst.Entries = append(dst.Entries, &c)
}

func render(e *rap.Entry) string {
	switch e.Kind {
	case rap.KindValue:
		return e.Value.String()
	case rap.KindArray:
		av := &rap.Value{Kind: rap.VArray, Arr: e.Array}
		return av.String()
	}
	return "class{…}"
}

// mergeClass рекурсивно вливает src в dst; конфликты решаются в пользу src
// (более позднего файла) и записываются в отчёт.
func mergeClass(dst, src *rap.Class, pathKey, pathShow, srcName string, owner map[string]string, res *Result, logf analyze.Logf) {
	for _, e := range src.Entries {
		key := pathKey + "/" + strings.ToLower(e.Name)
		show := pathShow + "/" + e.Name
		var existing *rap.Entry
		var existIdx int = -1
		for i, x := range dst.Entries {
			if strings.EqualFold(x.Name, e.Name) {
				existing = x
				existIdx = i
				break
			}
		}
		switch e.Kind {
		case rap.KindExtern:
			if existing == nil {
				c := *e
				dst.Entries = append(dst.Entries, &c)
				owner[key] = srcName
			}
			// если уже есть класс/extern — определение главнее, дубликат не нужен
		case rap.KindDelete:
			if existing == nil || existing.Kind != rap.KindDelete {
				c := *e
				dst.Entries = append(dst.Entries, &c)
				owner[key] = srcName
			}
		case rap.KindClass:
			if existing == nil || existing.Kind == rap.KindExtern {
				child := &rap.Class{Name: e.Name, Parent: e.Class.Parent}
				ne := &rap.Entry{Kind: rap.KindClass, Name: e.Name, Class: child}
				if existing != nil {
					dst.Entries[existIdx] = ne
				} else {
					dst.Entries = append(dst.Entries, ne)
				}
				owner[key] = srcName
				mergeClass(child, e.Class, key, show, srcName, owner, res, logf)
				continue
			}
			if existing.Kind != rap.KindClass {
				res.Conflicts = append(res.Conflicts, fmt.Sprintf("%s: %q был параметром (из %s), стал классом (из %s) — взято определение класса", show, e.Name, owner[key], srcName))
				child := &rap.Class{Name: e.Name, Parent: e.Class.Parent}
				dst.Entries[existIdx] = &rap.Entry{Kind: rap.KindClass, Name: e.Name, Class: child}
				owner[key] = srcName
				mergeClass(child, e.Class, key, show, srcName, owner, res, logf)
				continue
			}
			// оба — классы: проверяем родителя
			if e.Class.Parent != "" && existing.Class.Parent != "" &&
				!strings.EqualFold(e.Class.Parent, existing.Class.Parent) {
				res.Conflicts = append(res.Conflicts, fmt.Sprintf("КОНФЛИКТ РОДИТЕЛЯ %s: %s (из %s) -> %s (из %s) — взят %s",
					show, existing.Class.Parent, owner[key+"#parent"], e.Class.Parent, srcName, e.Class.Parent))
				existing.Class.Parent = e.Class.Parent
				owner[key+"#parent"] = srcName
			} else if existing.Class.Parent == "" && e.Class.Parent != "" {
				existing.Class.Parent = e.Class.Parent
				owner[key+"#parent"] = srcName
			}
			mergeClass(existing.Class, e.Class, key, show, srcName, owner, res, logf)
		case rap.KindValue, rap.KindArray:
			if existing == nil {
				c := *e
				dst.Entries = append(dst.Entries, &c)
				owner[key] = srcName
				continue
			}
			if existing.Kind == rap.KindClass {
				res.Conflicts = append(res.Conflicts, fmt.Sprintf("%s: был классом (из %s), стал параметром (из %s) — взят параметр", show, owner[key], srcName))
				c := *e
				dst.Entries[existIdx] = &c
				owner[key] = srcName
				continue
			}
			oldV, newV := render(existing), render(e)
			if oldV != newV {
				from := owner[key]
				if from == "" {
					from = "ранее"
				}
				res.Conflicts = append(res.Conflicts, fmt.Sprintf("КОНФЛИКТ %s: %s (из %s) -> %s (из %s) — взято значение из %s",
					show, oldV, from, newV, srcName, srcName))
			}
			c := *e
			dst.Entries[existIdx] = &c
			owner[key] = srcName
		}
	}
	if _, ok := owner[pathKey+"#parent"]; !ok && dst.Parent != "" {
		owner[pathKey+"#parent"] = srcName
	}
}

// BuildConfig собирает итоговый config.cpp объединённого аддона.
func BuildConfig(res *Result, addonName string) string {
	root := &rap.Class{}
	patches := &rap.Class{Name: "CfgPatches"}
	me := &rap.Class{Name: addonName}
	me.Entries = append(me.Entries,
		&rap.Entry{Kind: rap.KindArray, Name: "units"},
		&rap.Entry{Kind: rap.KindArray, Name: "weapons"},
		&rap.Entry{Kind: rap.KindValue, Name: "requiredVersion", Value: &rap.Value{Kind: rap.VFloat, F: 0.1}},
	)
	var reqArr []*rap.Value
	seen := map[string]bool{strings.ToLower(addonName): true}
	for _, ra := range res.RequiredAddons {
		if seen[strings.ToLower(ra)] {
			continue
		}
		seen[strings.ToLower(ra)] = true
		reqArr = append(reqArr, &rap.Value{Kind: rap.VString, Str: ra})
	}
	if len(reqArr) == 0 {
		reqArr = append(reqArr, &rap.Value{Kind: rap.VString, Str: "DZ_Data"})
	}
	me.Entries = append(me.Entries, &rap.Entry{Kind: rap.KindArray, Name: "requiredAddons", Array: reqArr})
	patches.Entries = append(patches.Entries, &rap.Entry{Kind: rap.KindClass, Name: addonName, Class: me})
	root.Entries = append(root.Entries, &rap.Entry{Kind: rap.KindClass, Name: "CfgPatches", Class: patches})
	root.Entries = append(root.Entries, res.Root.Entries...)

	header := "////////////////////////////////////////////////////////////////////\n" +
		"// Объединённый override-аддон, сгенерирован DayZ Override Builder\n" +
		fmt.Sprintf("// Слито оверрайдов: см. README. Исходные CfgPatches: %s\n", strings.Join(res.OwnPatches, ", ")) +
		"////////////////////////////////////////////////////////////////////\n\n"
	return header + rap.ToCPP(root)
}
