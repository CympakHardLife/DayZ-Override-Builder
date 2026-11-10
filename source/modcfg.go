// Package modcfg — редакторы конфигов модов: InediaInfectedAI (JSON, описания
// из вики автора) и TerjeMods (.cfg с самодокументированными строками).
package modcfg

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"dayzoverride/mission"
)

// ---------- русские описания ключевых параметров ----------

var inediaDescRU = map[string]string{
	"Zombies.Groups":                          "Группы зомби: имя группы = список класснеймов. Группы с ~ — служебные объединения (на них ссылаются другие параметры). Почти все параметры мода настраиваются per-группа: ключ all = для всех, либо имя группы.",
	"Zombies.BreakingDoorsHandlerIsActive":    "Выламывание дверей зомби: 1 = включено для группы, 0 = выключено.",
	"Zombies.BreakingDoorsOpenChancePercent":  "Шанс (в %) выломать дверь за одну попытку, по группам.",
	"Zombies.BreakingDoorsDestroyAfterOpenChancePercent": "Шанс (в %) что дверь будет УНИЧТОЖЕНА (а не просто открыта) при выламывании.",
	"Zombies.AttackPlayersUnconsciousHandlerIsActive":    "Добивают ли зомби игроков без сознания (1/0).",
	"Zombies.AttackAnimalsHandlerIsActive":    "Нападают ли зомби на животных (1/0).",
	"Zombies.ThrowingProjectiles":             "Реакция зомби на брошенные предметы (гранаты, камни): классы предметов и параметры поведения.",
	"Zombies.CustomIrritants":                 "Пользовательские раздражители: предметы/действия, привлекающие зомби.",
}

var terjeDescRU = map[string]string{
	"Core.WaterDrainFromVomit":        "Сколько воды теряет игрок при рвоте.",
	"Core.EnergyDrainFromVomit":       "Сколько энергии (еды) теряет игрок при рвоте.",
	"Core.ColorizedHudBadges":         "Цветные значки состояния на HUD в зависимости от тяжести болезни.",
	"Core.DetailedDiseaseHudBadges":   "Отдельная детальная иконка для каждой болезни (true) или общая иконка (false).",
	"Core.DetailedHealingHudBadges":   "Отдельные иконки для каждого действующего лекарства (казуально) или общая «таблетка» (хардкор).",
	"Core.FixKillOnDisconnect":        "Убивать игрока при выходе с сервера без сознания/связанным (защита от абуза).",
	"Core.PersistBloodType":           "Сохранять группу крови игрока между смертями.",
	"Medicine.EnableMedicalBoiling":   "Стерилизация инструментов кипячением в воде.",
	"Medicine.EnableInjuryAnim":       "Анимация ранения: игрок двигается медленнее при ранении/боли.",
	"Medicine.EnableSleepingbagAction": "Действие сна на спальных мешках.",
	"Medicine.EnableMaskDrop":         "При рвоте маска/противогаз падает с лица на землю.",
	"Medicine.VisceraEnabled":         "Ранение внутренних органов (висцеральная травма) включено на сервере.",
	"Skills.AllowSkillsReset":         "Разрешить игроку сбрасывать перки через интерфейс.",
	"Skills.ResetAllOnDeath":          "Полный сброс всех навыков и перков после смерти (иначе — потеря опыта по модификатору).",
	"Skills.ExperienceGainModifier":   "Множитель получаемого опыта навыков (2.5 = в 2.5 раза быстрее).",
	"Skills.ExperienceLoseModifier":   "Множитель потери опыта при смерти (0 = не терять).",
}

// ---------- Inedia (JSON) ----------

func inediaDescs() map[string]string {
	out := map[string]string{}
	for k, v := range inediaDescEN {
		out[k] = v
	}
	for k, v := range inediaDescRU {
		if en, ok := out[k]; ok {
			out[k] = v + " || EN: " + en
		} else {
			out[k] = v
		}
	}
	return out
}

// OpenInedia строит форму для InediaInfectedAIConfig.json.
func OpenInedia(fileName string, data []byte) (*mission.Model, interface{}, error) {
	m, orig, err := mission.JSONForm(fileName, data, "inedia",
		"InediaInfectedAI — "+fileName,
		"Конфиг мода Inedia Infected AI (ИИ заражённых: зрение, слух, урон, выламывание дверей, реакции). Описания параметров взяты из вики автора (EN), ключевые переведены. Параметры вида «all» действуют на все группы зомби, имя группы — только на неё. Полный гайд: github.com/ysaroka/InediaInfectedAI/wiki",
		inediaDescs())
	if m != nil {
		m.FileName = fileName
	}
	return m, orig, err
}

// ---------- Terje (.cfg) ----------

var terjeLineRe = regexp.MustCompile(`^(\s*)([A-Za-z0-9_.]+)\s*=\s*(.*?);`)
var terjeMetaRe = regexp.MustCompile(`\[type:\s*([a-zA-Z]+);\s*default:\s*([^\]]*)\]\s*(.*)`)

type TerjeFile struct {
	FileName string
	Lines    []string
}

// OpenTerje парсит Terje .cfg: каждая строка самодокументирована
// комментарием // [type: X; default: Y] Description.
func OpenTerje(fileName string, data []byte) (*mission.Model, *TerjeFile, error) {
	text := strings.ReplaceAll(string(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})), "\r\n", "\n")
	tf := &TerjeFile{FileName: fileName, Lines: strings.Split(text, "\n")}
	m := &mission.Model{
		Kind:     "terje",
		Title:    "TerjeMods — " + fileName,
		FileName: fileName,
		FileDesc: "Настройки модов Terje (медицина, навыки, радиация и т.д.). Каждый параметр подписан автором: [тип; значение по умолчанию] и описание. Ключевые параметры переведены на русский. Гайды на русском: github.com/TerjeBruoygard/TerjeMods/tree/master/WIKI/ru",
	}
	group := mission.Group{Name: strings.TrimSuffix(fileName, ".cfg")}
	nOpts := 0
	for i, line := range tf.Lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "//") && !strings.Contains(trim, "=") {
			// заголовок секции
			name := strings.TrimSpace(strings.TrimPrefix(trim, "//"))
			if name != "" {
				if len(group.Fields) > 0 {
					m.Groups = append(m.Groups, group)
				}
				group = mission.Group{Name: name}
			}
			continue
		}
		lm := terjeLineRe.FindStringSubmatch(line)
		if lm == nil {
			continue
		}
		key := lm[2]
		val := strings.TrimSpace(lm[3])
		kind := "text"
		desc := ""
		if ci := strings.Index(line, "//"); ci >= 0 {
			comment := strings.TrimSpace(line[ci+2:])
			if mm := terjeMetaRe.FindStringSubmatch(comment); mm != nil {
				typ := strings.ToLower(mm[1])
				def := strings.TrimSpace(mm[2])
				en := strings.TrimSpace(mm[3])
				switch typ {
				case "bool":
					kind = "boolTF"
				case "float", "int":
					kind = "number"
				}
				desc = "[тип: " + typ + "; по умолчанию: " + def + "] "
				if ru, ok := terjeDescRU[key]; ok {
					desc += ru + " || EN: " + en
				} else {
					desc += en
				}
			} else {
				desc = comment
			}
		}
		val = strings.Trim(val, `"`)
		group.Fields = append(group.Fields, mission.Field{
			ID: fmt.Sprintf("L%d", i), Label: key, Value: val, Kind: kind, Desc: desc,
		})
		nOpts++
	}
	if len(group.Fields) > 0 {
		m.Groups = append(m.Groups, group)
	}
	if nOpts == 0 {
		return nil, nil, fmt.Errorf("%s: не найдено параметров формата Terje (Ключ = значение; // [type: ...])", fileName)
	}
	m.Info = append(m.Info, fmt.Sprintf("Параметров: %d", nOpts))
	return m, tf, nil
}

// ApplyTerje точечно заменяет значения в строках, сохраняя комментарии.
func ApplyTerje(tf *TerjeFile, values map[string]string) []byte {
	for id, val := range values {
		if !strings.HasPrefix(id, "L") {
			continue
		}
		idx, err := strconv.Atoi(id[1:])
		if err != nil || idx < 0 || idx >= len(tf.Lines) {
			continue
		}
		line := tf.Lines[idx]
		lm := terjeLineRe.FindStringSubmatchIndex(line)
		if lm == nil {
			continue
		}
		// границы значения: lm[6]..lm[7] (группа 3)
		oldVal := strings.TrimSpace(line[lm[6]:lm[7]])
		newVal := strings.TrimSpace(val)
		// строки в кавычках сохраняем в кавычках
		if strings.HasPrefix(oldVal, `"`) && !strings.HasPrefix(newVal, `"`) {
			newVal = `"` + strings.Trim(newVal, `"`) + `"`
		}
		tf.Lines[idx] = line[:lm[6]] + newVal + line[lm[7]:]
	}
	return []byte(strings.Join(tf.Lines, "\n"))
}

// IsTerjeCfg распознаёт формат Terje.
func IsTerjeCfg(data []byte) bool {
	return bytes.Contains(data, []byte("// [type:")) || bytes.Contains(data, []byte("//[type:"))
}
