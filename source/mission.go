// Package mission — редактор миссионных файлов DayZ (globals.xml, economy.xml,
// events.xml, cfgweather.xml, cfggameplay.json, cfgeffectarea.json,
// cfgplayerspawnpoints.xml, cfgeventspawns.xml) со встроенным гайдом.
package mission

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

func regexpMustCompile(s string) *regexp.Regexp { return regexp.MustCompile(s) }

// ---------- модель формы ----------

type Field struct {
	ID    string `json:"id"` // путь значения
	Label string `json:"label"`
	Value string `json:"value"`
	Desc  string `json:"desc,omitempty"`
	Kind  string `json:"kind"` // text | number | bool01 | boolTF
}

type Group struct {
	Name   string  `json:"name"`
	Desc   string  `json:"desc,omitempty"`
	Fields []Field `json:"fields"`
}

type EventRow struct {
	Name     string `json:"name"`
	Active   string `json:"active"`
	Nominal  string `json:"nominal"`
	Min      string `json:"min"`
	Max      string `json:"max"`
	Lifetime string `json:"lifetime"`
	Restock  string `json:"restock"`
	SafeR    string `json:"saferadius"`
	Children int    `json:"children"`
}

type Model struct {
	Kind     string     `json:"kind"` // globals | economy | weather | gameplay | effectarea | spawnpoints | events | eventspawns
	Title    string     `json:"title"`
	FileDesc string     `json:"fileDesc"`
	Groups   []Group    `json:"groups,omitempty"`
	Events   []EventRow `json:"events,omitempty"`
	Info     []string   `json:"info,omitempty"`
	FileName string     `json:"fileName"`
}

// ---------- гайд (объяснения параметров) ----------

var globalsDesc = map[string]string{
	"AnimalMaxCount":              "Максимум животных на сервере одновременно",
	"CleanupAvoidance":            "Не удалять объекты ближе этого расстояния (м) к игроку",
	"CleanupLifetimeDeadAnimal":   "Через сколько секунд удаляется труп животного",
	"CleanupLifetimeDeadInfected": "Через сколько секунд удаляется труп заражённого",
	"CleanupLifetimeDeadPlayer":   "Через сколько секунд удаляется труп игрока (3600 = 1 час)",
	"CleanupLifetimeDefault":      "Время жизни прочего мусора (сек)",
	"CleanupLifetimeLimit":        "Максимум объектов, удаляемых за один проход чистки",
	"CleanupLifetimeRuined":       "Время жизни полностью сломанных (ruined) предметов, сек",
	"FlagRefreshFrequency":        "Период «обновления» флага на флагштоке, сек (432000 = 5 дней)",
	"FlagRefreshMaxDuration":      "Максимальный срок действия флага, сек (3456000 = 40 дней)",
	"IdleModeCountdown":           "Через сколько секунд без игроков экономика засыпает",
	"IdleModeStartup":             "1 = при старте без игроков экономика в спящем режиме",
	"InitialSpawn":                "Процент наполнения лута при первом старте мира (100 = полный)",
	"LootDamageMax":               "Максимальное повреждение спавнящегося лута (0..1)",
	"LootDamageMin":               "Минимальное повреждение спавнящегося лута (0..1)",
	"LootProxyPlacement":          "1 = разрешить спавн лута в прокси-объектах (мебель и т.п.)",
	"LootSpawnAvoidance":          "Не спавнить лут ближе этого расстояния (м) к игроку",
	"RespawnAttempt":              "Попыток респавна лута за один проход",
	"RespawnLimit":                "Максимум предметов, респавнящихся за один проход",
	"RespawnTypes":                "Максимум разных типов предметов за один проход респавна",
	"RestartSpawn":                "Респавн лута при рестарте сервера (0/1)",
	"SpawnInitial":                "Начальное количество предметов при создании экономики",
	"TimeHopping":                 "Штрафной таймер за смену сервера (server hop), сек",
	"TimeLogin":                   "Таймер входа на сервер, сек",
	"TimeLogout":                  "Таймер выхода с сервера, сек",
	"TimePenalty":                 "Штрафной таймер (после боя и т.п.), сек",
	"WorldWetTempUpdate":          "Период пересчёта намокания/температуры предметов",
	"ZombieMaxCount":              "Максимум заражённых на сервере одновременно",
	"ZoneSpawnDist":               "Дистанция (м), с которой игрок активирует зоны спавна зомби",
}

var economyDesc = map[string]string{
	"dynamic":  "Динамические события (крушения вертолётов, полицейские машины и т.п.)",
	"animals":  "Животные",
	"zombies":  "Заражённые",
	"vehicles": "Транспорт",
	"randoms":  "Случайные объекты",
	"custom":   "Пользовательские объекты",
	"building": "Постройки/базостроение",
	"player":   "Игроки (позиции, инвентарь)",
}

var economyAttrDesc = map[string]string{
	"init":    "создавать при старте сервера",
	"load":    "загружать из сохранения (storage)",
	"respawn": "поддерживать респавн",
	"save":    "сохранять в storage при выключении",
}

var weatherDesc = map[string]string{
	"weather.enable":   "1 = сервер управляет погодой по этому файлу",
	"overcast":         "Облачность (0 = ясно, 1 = полностью пасмурно)",
	"fog":              "Туман (0 = нет, 1 = максимальный)",
	"rain":             "Дождь (0 = нет, 1 = ливень). Дождь возможен только при высокой облачности",
	"windMagnitude":    "Сила ветра, м/с",
	"windDirection":    "Направление ветра, радианы (-3.14..3.14)",
	"snowfall":         "Снегопад (на картах с зимой)",
	"current.actual":   "Текущее значение при старте",
	"current.time":     "Время (сек) до следующей смены значения",
	"current.duration": "Длительность перехода к новому значению, сек",
	"limits":           "Минимум и максимум допустимых значений",
	"timelimits":       "Диапазон (сек) между сменами значения",
	"changelimits":     "На сколько максимум/минимум значение меняется за раз",
}

var gameplayDesc = map[string]string{
	"version":                          "Версия формата файла — не менять",
	"disableBaseDamage":                "true = базы не получают урон",
	"disableContainerDamage":           "true = контейнеры (сейфы, бочки) не получают урон",
	"disableRespawnDialog":             "true = скрыть диалог выбора точки респавна",
	"disablePersonalLight":             "true = отключить «личную подсветку» игрока ночью",
	"staminaMax":                       "Максимальный запас выносливости",
	"staminaKgToStaminaPercentPenalty": "Штраф выносливости за кг веса",
	"staminaWeightLimitThreshold":      "Порог веса (г), после которого начинается штраф",
	"sprintStaminaModifierErc":         "Расход стамины при спринте стоя (множитель)",
	"sprintStaminaModifierCro":         "Расход стамины при спринте пригнувшись",
	"staminaMinCap":                    "Минимальный «потолок» стамины при перегрузе",
	"shockRefillSpeedConscious":        "Скорость восстановления шока в сознании",
	"shockRefillSpeedUnconscious":      "Скорость восстановления шока без сознания",
	"lightingConfig":                   "0 = светлые ночи, 1 = тёмные ночи, 2 = Sakhal, 3 = Nasdara (DayZ 1.30)",
	"bunkerBroadcastEnabled":           "true = включить трансляцию бункера (Nasdara)",
	"objectSpawnersArr":                "Пути к JSON-файлам спавна объектов (относительно миссии)",
	"environmentMinTemps":              "Мин. температура по месяцам (12 значений)",
	"environmentMaxTemps":              "Макс. температура по месяцам (12 значений)",
	"wetDataUpdateFrequency":           "Период пересчёта намокания, сек",
	"disableIsCollidingBBoxCheck":      "Отключить проверку коллизии bbox при строительстве",
	"disableIsCollidingPlayerCheck":    "Отключить проверку коллизии с игроком при строительстве",
	"disablePerformRoofCheck":          "Отключить проверку крыши при строительстве",
	"disableIsClippingRoofCheck":       "Отключить проверку пересечения крыши",
	"disableColdAreaPlacementCheck":    "true = отключить проверку размещения в холодной зоне",
	"disableColdAreaBuildingCheck":     "true = отключить проверку строительства в холодной зоне (имя из миссии Chernarus)",
	"disallowedTypesInUnderground":     "Классы комплектов, размещение которых запрещено под землёй",
	"disableIsCollidingCheck":          "true = отключить проверку коллизии при строительстве",
	"disableDistanceCheck":             "true = отключить проверку расстояния при строительстве",
	"disableItemFalling":               "true = отключить падение строительных элементов",
	"disableDropAttachmentsOnFall":     "true = отключить сброс вложений при падении строительного элемента",
	"disableRefundMaterialsOnFall":     "true = отключить возврат материалов при падении строительного элемента",
	"canDetachAttachedCarWheels":       "true = разрешить снятие установленных колёс автомобилей",
	"canDamageAttachedCarWheels":       "true = разрешить повреждение установленных колёс автомобилей",
	"canDetachAttachedBikeWheels":      "true = разрешить снятие установленных колёс мотоциклов",
	"canDamageAttachedBikeWheels":      "true = разрешить повреждение установленных колёс мотоциклов",
	"use3DMap":                         "true = карта в руках как 3D-предмет (без интерфейса)",
	"ignoreMapOwnership":               "true = карта в интерфейсе доступна даже без предмета карты",
	"ignoreNavItemsOwnership":          "true = навигация без компаса/GPS в инвентаре",
	"displayPlayerPosition":            "true = показывать позицию игрока на карте",
	"displayNavInfo":                   "true = показывать инфо навигации",
	"hitIndicationPPEEnabled":          "Эффект экрана при попадании",
	"disableHostileParticles":          "Не показывать частицы враждебных зон",
}

var effectDesc = map[string]string{
	"AreaName":         "Название зоны (любое, для себя)",
	"Type":             "Тип зоны: ContaminatedArea_Static / Dynamic и т.п.",
	"TriggerType":      "Тип триггера (обычно ContaminatedTrigger)",
	"Pos":              "Координаты центра зоны [X, Y(высота), Z]",
	"Radius":           "Радиус зоны, м",
	"PosHeight":        "Высота зоны вверх от центра, м",
	"NegHeight":        "Глубина зоны вниз от центра, м",
	"InnerRingCount":   "Число внутренних колец частиц",
	"InnerPartDist":    "Расстояние между частицами внутреннего кольца",
	"OuterRingToggle":  "1 = включить внешнее кольцо частиц",
	"OuterPartDist":    "Расстояние частиц внешнего кольца",
	"OuterOffset":      "Смещение внешнего кольца",
	"VerticalLayers":   "Вертикальные слои частиц",
	"ParticleName":     "Имя эффекта частиц (graphics/particles/...)",
	"SafePositions":    "Безопасные точки [X, Z] — куда телепортирует из зоны (для динамических)",
	"AroundPartName":   "Частицы вокруг игрока в зоне",
	"TinyPartName":     "Мелкие частицы у игрока",
	"PPERequesterType": "Пост-эффект экрана в зоне",
}

var spawnDesc = map[string]string{
	"fresh":             "Новый персонаж (после смерти)",
	"hop":               "Персонаж после смены сервера (server hop)",
	"travel":            "Персонаж при путешествии между серверами",
	"min_dist_infected": "Мин. дистанция до заражённых при выборе точки, м",
	"max_dist_infected": "Макс. дистанция до заражённых (для оценки), м",
	"min_dist_player":   "Мин. дистанция до других игроков, м",
	"max_dist_player":   "Макс. дистанция до других игроков, м",
	"min_dist_static":   "Мин. дистанция до статических объектов (домов), м",
	"max_dist_static":   "Макс. дистанция до статических объектов, м",
}

var eventFieldDesc = map[string]string{
	"nominal":        "Сколько экземпляров события должно существовать одновременно",
	"min":            "Минимум активных экземпляров",
	"max":            "Максимум активных экземпляров",
	"lifetime":       "Время жизни события, сек",
	"restock":        "Пауза перед респавном после удаления, сек (0 = сразу)",
	"saferadius":     "Не спавнить ближе этого расстояния к игрокам, м",
	"distanceradius": "Мин. расстояние между экземплярами этого события, м",
	"cleanupradius":  "Радиус очистки остатков, м",
	"active":         "1 = событие включено, 0 = выключено",
	"position":       "fixed = точки из cfgeventspawns.xml, player = возле игроков",
	"limit":          "Лимит: child / parent / mixed",
}

// ---------- globals.xml ----------

type gVar struct {
	Name  string `xml:"name,attr"`
	Type  string `xml:"type,attr"`
	Value string `xml:"value,attr"`
}
type gFile struct {
	XMLName xml.Name `xml:"variables"`
	Vars    []gVar   `xml:"var"`
}

func parseGlobals(data []byte) (*Model, interface{}, error) {
	var f gFile
	if err := xml.Unmarshal(data, &f); err != nil {
		return nil, nil, xmlErr("globals.xml", err)
	}
	m := &Model{Kind: "globals", Title: "globals.xml — глобальные параметры экономики",
		FileDesc: "Главные «ручки» центральной экономики: лимиты зомби и животных, таймеры входа/выхода, чистка трупов, наполнение лута. type=0 — целое число, type=1 — дробное."}
	g := Group{Name: "Переменные"}
	for _, v := range f.Vars {
		g.Fields = append(g.Fields, Field{
			ID: "var." + v.Name, Label: v.Name, Value: v.Value, Kind: "number",
			Desc: globalsDesc[v.Name],
		})
	}
	m.Groups = []Group{g}
	return m, &f, nil
}

func applyGlobals(orig interface{}, values map[string]string) []byte {
	f := orig.(*gFile)
	for i := range f.Vars {
		if v, ok := values["var."+f.Vars[i].Name]; ok {
			f.Vars[i].Value = strings.TrimSpace(v)
		}
	}
	var sb strings.Builder
	sb.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"yes\" ?>\n<variables>\n")
	for _, v := range f.Vars {
		sb.WriteString(fmt.Sprintf("    <var name=\"%s\" type=\"%s\" value=\"%s\"/>\n", xe(v.Name), xe(v.Type), xe(v.Value)))
	}
	sb.WriteString("</variables>\n")
	return []byte(sb.String())
}

// ---------- economy.xml ----------

type ecoSection struct {
	XMLName xml.Name
	Init    string `xml:"init,attr"`
	Load    string `xml:"load,attr"`
	Respawn string `xml:"respawn,attr"`
	Save    string `xml:"save,attr"`
}
type ecoFile struct {
	XMLName  xml.Name     `xml:"economy"`
	Sections []ecoSection `xml:",any"`
}

func parseEconomy(data []byte) (*Model, interface{}, error) {
	var f ecoFile
	if err := xml.Unmarshal(data, &f); err != nil {
		return nil, nil, xmlErr("economy.xml", err)
	}
	m := &Model{Kind: "economy", Title: "economy.xml — включатели подсистем экономики",
		FileDesc: "Что серверу создавать, загружать, респавнить и сохранять. 1 = включено, 0 = выключено. Например, отключив vehicles respawn, транспорт перестанет респавниться."}
	for _, s := range f.Sections {
		g := Group{Name: s.XMLName.Local, Desc: economyDesc[s.XMLName.Local]}
		g.Fields = append(g.Fields,
			Field{ID: s.XMLName.Local + ".init", Label: "init", Value: s.Init, Kind: "bool01", Desc: economyAttrDesc["init"]},
			Field{ID: s.XMLName.Local + ".load", Label: "load", Value: s.Load, Kind: "bool01", Desc: economyAttrDesc["load"]},
			Field{ID: s.XMLName.Local + ".respawn", Label: "respawn", Value: s.Respawn, Kind: "bool01", Desc: economyAttrDesc["respawn"]},
			Field{ID: s.XMLName.Local + ".save", Label: "save", Value: s.Save, Kind: "bool01", Desc: economyAttrDesc["save"]},
		)
		m.Groups = append(m.Groups, g)
	}
	return m, &f, nil
}

func applyEconomy(orig interface{}, values map[string]string) []byte {
	f := orig.(*ecoFile)
	get := func(sec, attr, cur string) string {
		if v, ok := values[sec+"."+attr]; ok {
			return strings.TrimSpace(v)
		}
		return cur
	}
	var sb strings.Builder
	sb.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"yes\" ?>\n<economy>\n")
	for _, s := range f.Sections {
		n := s.XMLName.Local
		sb.WriteString(fmt.Sprintf("    <%s init=\"%s\" load=\"%s\" respawn=\"%s\" save=\"%s\"/>\n",
			n, get(n, "init", s.Init), get(n, "load", s.Load), get(n, "respawn", s.Respawn), get(n, "save", s.Save)))
	}
	sb.WriteString("</economy>\n")
	return []byte(sb.String())
}

// ---------- cfgweather.xml ----------

type wCurrent struct {
	Actual   string `xml:"actual,attr"`
	Time     string `xml:"time,attr"`
	Duration string `xml:"duration,attr"`
}
type wMinMax struct {
	Min string `xml:"min,attr"`
	Max string `xml:"max,attr"`
}
type wParam struct {
	XMLName      xml.Name
	Current      *wCurrent `xml:"current"`
	Limits       *wMinMax  `xml:"limits"`
	TimeLimits   *wMinMax  `xml:"timelimits"`
	ChangeLimits *wMinMax  `xml:"changelimits"`
}
type wFile struct {
	XMLName xml.Name `xml:"weather"`
	Enable  string   `xml:"enable,attr"`
	Reset   string   `xml:"reset,attr"`
	Params  []wParam `xml:",any"`
}

func parseWeather(data []byte) (*Model, interface{}, error) {
	var f wFile
	if err := xml.Unmarshal(data, &f); err != nil {
		return nil, nil, xmlErr("cfgweather.xml", err)
	}
	m := &Model{Kind: "weather", Title: "cfgweather.xml — погода",
		FileDesc: "Управление облачностью, туманом, дождём и ветром: стартовые значения, допустимые пределы, частота и величина изменений."}
	root := Group{Name: "weather", Fields: []Field{
		{ID: "weather.enable", Label: "enable", Value: f.Enable, Kind: "bool01", Desc: weatherDesc["weather.enable"]},
		{ID: "weather.reset", Label: "reset", Value: f.Reset, Kind: "bool01", Desc: "1 = сбрасывать погоду при рестарте"},
	}}
	m.Groups = append(m.Groups, root)
	for _, p := range f.Params {
		n := p.XMLName.Local
		g := Group{Name: n, Desc: weatherDesc[n]}
		if p.Current != nil {
			g.Fields = append(g.Fields,
				Field{ID: n + ".current.actual", Label: "current actual", Value: p.Current.Actual, Kind: "number", Desc: weatherDesc["current.actual"]},
				Field{ID: n + ".current.time", Label: "current time", Value: p.Current.Time, Kind: "number", Desc: weatherDesc["current.time"]},
				Field{ID: n + ".current.duration", Label: "current duration", Value: p.Current.Duration, Kind: "number", Desc: weatherDesc["current.duration"]},
			)
		}
		addMM := func(tag string, mm *wMinMax, desc string) {
			if mm != nil {
				g.Fields = append(g.Fields,
					Field{ID: n + "." + tag + ".min", Label: tag + " min", Value: mm.Min, Kind: "number", Desc: desc},
					Field{ID: n + "." + tag + ".max", Label: tag + " max", Value: mm.Max, Kind: "number"},
				)
			}
		}
		addMM("limits", p.Limits, weatherDesc["limits"])
		addMM("timelimits", p.TimeLimits, weatherDesc["timelimits"])
		addMM("changelimits", p.ChangeLimits, weatherDesc["changelimits"])
		m.Groups = append(m.Groups, g)
	}
	return m, &f, nil
}

func applyWeather(orig interface{}, values map[string]string) []byte {
	f := orig.(*wFile)
	g := func(id, cur string) string {
		if v, ok := values[id]; ok {
			return strings.TrimSpace(v)
		}
		return cur
	}
	var sb strings.Builder
	sb.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	sb.WriteString(fmt.Sprintf("<weather enable=\"%s\" reset=\"%s\">\n", g("weather.enable", f.Enable), g("weather.reset", f.Reset)))
	for _, p := range f.Params {
		n := p.XMLName.Local
		sb.WriteString("    <" + n + ">\n")
		if p.Current != nil {
			sb.WriteString(fmt.Sprintf("        <current actual=\"%s\" time=\"%s\" duration=\"%s\"/>\n",
				g(n+".current.actual", p.Current.Actual), g(n+".current.time", p.Current.Time), g(n+".current.duration", p.Current.Duration)))
		}
		wmm := func(tag string, mm *wMinMax) {
			if mm != nil {
				sb.WriteString(fmt.Sprintf("        <%s min=\"%s\" max=\"%s\"/>\n", tag, g(n+"."+tag+".min", mm.Min), g(n+"."+tag+".max", mm.Max)))
			}
		}
		wmm("limits", p.Limits)
		wmm("timelimits", p.TimeLimits)
		wmm("changelimits", p.ChangeLimits)
		sb.WriteString("    </" + n + ">\n")
	}
	sb.WriteString("</weather>\n")
	return []byte(sb.String())
}

// ---------- events.xml ----------

type evFlags struct {
	Deletable     string `xml:"deletable,attr"`
	InitRandom    string `xml:"init_random,attr"`
	RemoveDamaged string `xml:"remove_damaged,attr"`
}
type evChild struct {
	Lootmax string `xml:"lootmax,attr"`
	Lootmin string `xml:"lootmin,attr"`
	Max     string `xml:"max,attr"`
	Min     string `xml:"min,attr"`
	Type    string `xml:"type,attr"`
}
type evEvent struct {
	Name           string    `xml:"name,attr"`
	Nominal        string    `xml:"nominal"`
	Min            string    `xml:"min"`
	Max            string    `xml:"max"`
	Lifetime       string    `xml:"lifetime"`
	Restock        string    `xml:"restock"`
	SafeRadius     string    `xml:"saferadius"`
	DistanceRadius string    `xml:"distanceradius"`
	CleanupRadius  string    `xml:"cleanupradius"`
	Flags          *evFlags  `xml:"flags"`
	Position       string    `xml:"position"`
	Limit          string    `xml:"limit"`
	Active         string    `xml:"active"`
	Children       []evChild `xml:"children>child"`
	Secondary      []string  `xml:"secondary>event"`
}
type evFile struct {
	XMLName xml.Name  `xml:"events"`
	Events  []evEvent `xml:"event"`
}

func parseEvents(data []byte) (*Model, interface{}, error) {
	var f evFile
	if err := xml.Unmarshal(data, &f); err != nil {
		return nil, nil, xmlErr("events.xml", err)
	}
	m := &Model{Kind: "events", Title: "events.xml — динамические события",
		FileDesc: "Крушения вертолётов, машины, стада животных и т.д. nominal — сколько существует одновременно; active — вкл/выкл; restock — пауза до респавна; saferadius — не спавнить рядом с игроками. Точки спавна задаются в cfgeventspawns.xml."}
	for _, e := range f.Events {
		m.Events = append(m.Events, EventRow{
			Name: e.Name, Active: e.Active, Nominal: e.Nominal, Min: e.Min, Max: e.Max,
			Lifetime: e.Lifetime, Restock: e.Restock, SafeR: e.SafeRadius, Children: len(e.Children),
		})
	}
	for k, v := range eventFieldDesc {
		m.Info = append(m.Info, k+" — "+v)
	}
	sort.Strings(m.Info)
	return m, &f, nil
}

func applyEvents(orig interface{}, rows []EventRow) []byte {
	f := orig.(*evFile)
	byName := map[string]EventRow{}
	for _, r := range rows {
		byName[r.Name] = r
	}
	var sb strings.Builder
	sb.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"yes\" ?>\n<events>\n")
	for _, e := range f.Events {
		if r, ok := byName[e.Name]; ok {
			e.Active, e.Nominal, e.Min, e.Max = r.Active, r.Nominal, r.Min, r.Max
			e.Lifetime, e.Restock, e.SafeRadius = r.Lifetime, r.Restock, r.SafeR
		}
		sb.WriteString("    <event name=\"" + xe(e.Name) + "\">\n")
		w := func(tag, v string) {
			if v != "" {
				sb.WriteString("        <" + tag + ">" + xe(v) + "</" + tag + ">\n")
			}
		}
		w("nominal", e.Nominal)
		w("min", e.Min)
		w("max", e.Max)
		w("lifetime", e.Lifetime)
		w("restock", e.Restock)
		w("saferadius", e.SafeRadius)
		w("distanceradius", e.DistanceRadius)
		w("cleanupradius", e.CleanupRadius)
		if e.Flags != nil {
			sb.WriteString(fmt.Sprintf("        <flags deletable=\"%s\" init_random=\"%s\" remove_damaged=\"%s\"/>\n",
				e.Flags.Deletable, e.Flags.InitRandom, e.Flags.RemoveDamaged))
		}
		w("position", e.Position)
		w("limit", e.Limit)
		w("active", e.Active)
		if len(e.Children) > 0 {
			sb.WriteString("        <children>\n")
			for _, c := range e.Children {
				sb.WriteString("            <child")
				if c.Lootmax != "" {
					sb.WriteString(" lootmax=\"" + xe(c.Lootmax) + "\"")
				}
				if c.Lootmin != "" {
					sb.WriteString(" lootmin=\"" + xe(c.Lootmin) + "\"")
				}
				sb.WriteString(" max=\"" + xe(c.Max) + "\" min=\"" + xe(c.Min) + "\" type=\"" + xe(c.Type) + "\"/>\n")
			}
			sb.WriteString("        </children>\n")
		} else {
			sb.WriteString("        <children/>\n")
		}
		if len(e.Secondary) > 0 {
			sb.WriteString("        <secondary>\n")
			for _, s := range e.Secondary {
				sb.WriteString("            <event>" + xe(s) + "</event>\n")
			}
			sb.WriteString("        </secondary>\n")
		}
		sb.WriteString("    </event>\n")
	}
	sb.WriteString("</events>\n")
	return []byte(sb.String())
}

// ---------- JSON файлы (cfggameplay.json, cfgeffectarea.json) ----------

func parseJSONForm(name string, data []byte, kind, title, fileDesc string, descs map[string]string) (*Model, interface{}, error) {
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, nil, fmt.Errorf("%s: битый JSON: %v (проверьте во вкладке «Проверка файлов»)", name, err)
	}
	m := &Model{Kind: kind, Title: title, FileDesc: fileDesc}
	g := Group{Name: "Параметры"}
	flattenJSON("", v, &g, descs)
	m.Groups = []Group{g}
	return m, v, nil
}

func flattenJSON(prefix string, v interface{}, g *Group, descs map[string]string) {
	switch x := v.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			p := k
			if prefix != "" {
				p = prefix + "." + k
			}
			flattenJSON(p, x[k], g, descs)
		}
	case []interface{}:
		allPrim := true
		for _, e := range x {
			switch e.(type) {
			case map[string]interface{}, []interface{}:
				allPrim = false
			}
		}
		if allPrim {
			g.Fields = append(g.Fields, Field{ID: prefix, Label: prefix, Value: primitiveListString(x),
				Kind: "text", Desc: descFor(prefix, descs) + " (список через запятую)"})
			return
		}
		for i, e := range x {
			flattenJSON(fmt.Sprintf("%s[%d]", prefix, i), e, g, descs)
		}
	default:
		kind := "text"
		switch v.(type) {
		case float64:
			kind = "number"
		case bool:
			kind = "boolTF"
		}
		g.Fields = append(g.Fields, Field{ID: prefix, Label: prefix, Value: primStr(v), Kind: kind, Desc: descFor(prefix, descs)})
	}
}

func descFor(path string, descs map[string]string) string {
	// сначала полный путь (без индексов массивов), затем отбрасываем хвосты
	clean := arrIdxRe.ReplaceAllString(path, "")
	p := clean
	for {
		if d, ok := descs[p]; ok {
			return d
		}
		i := strings.LastIndex(p, ".")
		if i < 0 {
			break
		}
		p = p[:i]
	}
	segs := strings.Split(clean, ".")
	return descs[segs[len(segs)-1]]
}

var arrIdxRe = regexpMustCompile(`\[\d+\]`)

// JSONForm / ApplyJSONValues — экспорт для вкладок настройки модов.
func JSONForm(name string, data []byte, kind, title, fileDesc string, descs map[string]string) (*Model, interface{}, error) {
	return parseJSONForm(name, data, kind, title, fileDesc, descs)
}

func ApplyJSONValues(orig interface{}, values map[string]string) []byte {
	return applyJSON(orig, values)
}

func primStr(v interface{}) string {
	switch x := v.(type) {
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		b, _ := json.Marshal(x)
		return string(b)
	case nil:
		return "null"
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func primitiveListString(values []interface{}) string {
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = primStr(value)
	}
	return strings.Join(parts, ", ")
}

// copyJSON сохраняет исходную модель для повторного применения правок.
func copyJSON(v interface{}) interface{} {
	switch x := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(x))
		for key, value := range x {
			out[key] = copyJSON(value)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(x))
		for i, value := range x {
			out[i] = copyJSON(value)
		}
		return out
	default:
		return v
	}
}

// applyJSON применяет значения по путям к дереву и сериализует.
func applyJSON(orig interface{}, values map[string]string) []byte {
	orig = copyJSON(orig)
	for path, val := range values {
		setJSONPath(&orig, path, val)
	}
	out, _ := json.MarshalIndent(orig, "", "    ")
	return append(out, '\n')
}

func setJSONPath(root *interface{}, path, val string) {
	setSeg(root, splitPath(path), val)
}

func setSeg(cur *interface{}, segs []pathSeg, val string) {
	if len(segs) == 0 {
		return
	}
	s := segs[0]
	if s.idx >= 0 {
		arr, ok := (*cur).([]interface{})
		if !ok || s.idx >= len(arr) {
			return
		}
		if len(segs) == 1 {
			arr[s.idx] = coerce(val, arr[s.idx])
			return
		}
		setSeg(&arr[s.idx], segs[1:], val)
		return
	}
	m, ok := (*cur).(map[string]interface{})
	if !ok {
		return
	}
	if len(segs) == 1 {
		m[s.key] = coerce(val, m[s.key])
		return
	}
	v, ok := m[s.key]
	if !ok {
		return
	}
	setSeg(&v, segs[1:], val)
	m[s.key] = v
}

type pathSeg struct {
	key string
	idx int
}

func splitPath(path string) []pathSeg {
	var out []pathSeg
	for _, part := range strings.Split(path, ".") {
		for {
			br := strings.IndexByte(part, '[')
			if br < 0 {
				if part != "" {
					out = append(out, pathSeg{key: part, idx: -1})
				}
				break
			}
			if br > 0 {
				out = append(out, pathSeg{key: part[:br], idx: -1})
			}
			end := strings.IndexByte(part, ']')
			var idx int
			fmt.Sscanf(part[br+1:end], "%d", &idx)
			out = append(out, pathSeg{idx: idx})
			part = part[end+1:]
		}
	}
	return out
}

// coerce приводит строку к типу прежнего значения.
func coerce(val string, prev interface{}) interface{} {
	if val == primStr(prev) {
		return prev
	}
	if list, ok := prev.([]interface{}); ok && val == primitiveListString(list) {
		return prev
	}
	val = strings.TrimSpace(val)
	switch previous := prev.(type) {
	case bool:
		return val == "true" || val == "1"
	case float64:
		var f float64
		if _, err := fmt.Sscanf(strings.ReplaceAll(val, ",", "."), "%g", &f); err == nil {
			return f
		}
		return prev
	case []interface{}: // примитивный список через запятую
		parts := strings.Split(val, ",")
		out := make([]interface{}, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			// Используем тип прежнего элемента; новые элементы продолжают тип списка.
			if len(previous) > 0 {
				i := len(out)
				if i >= len(previous) {
					i = 0
				}
				out = append(out, coerce(strings.Trim(p, `"`), previous[i]))
				continue
			}
			var value interface{}
			if err := json.Unmarshal([]byte(p), &value); err != nil {
				value = strings.Trim(p, `"`)
			}
			out = append(out, value)
		}
		return out
	}
	return val
}

// ---------- cfgplayerspawnpoints.xml ----------

type spParams struct {
	MinDistInfected string `xml:"min_dist_infected"`
	MaxDistInfected string `xml:"max_dist_infected"`
	MinDistPlayer   string `xml:"min_dist_player"`
	MaxDistPlayer   string `xml:"max_dist_player"`
	MinDistStatic   string `xml:"min_dist_static"`
	MaxDistStatic   string `xml:"max_dist_static"`
}
type spPos struct {
	X string `xml:"x,attr"`
	Z string `xml:"z,attr"`
}
type spSection struct {
	XMLName xml.Name
	Params  *spParams `xml:"spawn_params"`
	GenPos  []spPos   `xml:"generator_posbubbles>pos"`
	Groups  []struct {
		Name string  `xml:"name,attr"`
		Pos  []spPos `xml:"pos"`
	} `xml:"generator_posbubbles>group"`
}
type spFile struct {
	XMLName  xml.Name    `xml:"playerspawnpoints"`
	Sections []spSection `xml:",any"`
	raw      []byte
}

func parseSpawnPoints(data []byte) (*Model, interface{}, error) {
	var f spFile
	if err := xml.Unmarshal(data, &f); err != nil {
		return nil, nil, xmlErr("cfgplayerspawnpoints.xml", err)
	}
	f.raw = data
	m := &Model{Kind: "spawnpoints", Title: "cfgplayerspawnpoints.xml — точки спавна игроков",
		FileDesc: "Правила выбора точки появления: дистанции до зомби/игроков/строений и список координат. Здесь редактируются дистанции; сами координаты (posbubbles) остаются без изменений."}
	for _, s := range f.Sections {
		g := Group{Name: s.XMLName.Local, Desc: spawnDesc[s.XMLName.Local]}
		if s.Params != nil {
			add := func(id, label, v string) {
				g.Fields = append(g.Fields, Field{ID: s.XMLName.Local + "." + id, Label: label, Value: v, Kind: "number", Desc: spawnDesc[id]})
			}
			add("min_dist_infected", "min_dist_infected", s.Params.MinDistInfected)
			add("max_dist_infected", "max_dist_infected", s.Params.MaxDistInfected)
			add("min_dist_player", "min_dist_player", s.Params.MinDistPlayer)
			add("max_dist_player", "max_dist_player", s.Params.MaxDistPlayer)
			add("min_dist_static", "min_dist_static", s.Params.MinDistStatic)
			add("max_dist_static", "max_dist_static", s.Params.MaxDistStatic)
		}
		nPos := len(s.GenPos)
		for _, gr := range s.Groups {
			nPos += len(gr.Pos)
		}
		if nPos > 0 {
			m.Info = append(m.Info, fmt.Sprintf("%s: точек спавна %d", s.XMLName.Local, nPos))
		}
		if len(g.Fields) > 0 {
			m.Groups = append(m.Groups, g)
		}
	}
	return m, &f, nil
}

// applySpawnPoints точечно заменяет значения дистанций в исходном тексте,
// сохраняя координаты и форматирование нетронутыми.
func applySpawnPoints(orig interface{}, values map[string]string) []byte {
	f := orig.(*spFile)
	text := string(f.raw)
	for _, s := range f.Sections {
		if s.Params == nil {
			continue
		}
		sec := s.XMLName.Local
		// границы секции
		start := strings.Index(text, "<"+sec+">")
		end := strings.Index(text, "</"+sec+">")
		if start < 0 || end < 0 {
			continue
		}
		chunk := text[start:end]
		repl := func(tag, old string) {
			if v, ok := values[sec+"."+tag]; ok && strings.TrimSpace(v) != old {
				chunk = strings.Replace(chunk, "<"+tag+">"+old+"</"+tag+">", "<"+tag+">"+strings.TrimSpace(v)+"</"+tag+">", 1)
			}
		}
		repl("min_dist_infected", s.Params.MinDistInfected)
		repl("max_dist_infected", s.Params.MaxDistInfected)
		repl("min_dist_player", s.Params.MinDistPlayer)
		repl("max_dist_player", s.Params.MaxDistPlayer)
		repl("min_dist_static", s.Params.MinDistStatic)
		repl("max_dist_static", s.Params.MaxDistStatic)
		text = text[:start] + chunk + text[end:]
	}
	return []byte(text)
}

// ---------- cfgeventspawns.xml (обзор) ----------

type esEvent struct {
	Name string `xml:"name,attr"`
	Pos  []struct {
		X string `xml:"x,attr"`
		Z string `xml:"z,attr"`
		A string `xml:"a,attr"`
	} `xml:"pos"`
	Zones []struct {
		Smin string `xml:"smin,attr"`
		Smax string `xml:"smax,attr"`
	} `xml:"zone"`
}
type esFile struct {
	XMLName xml.Name  `xml:"eventposdef"`
	Events  []esEvent `xml:"event"`
}

func parseEventSpawns(data []byte) (*Model, interface{}, error) {
	var f esFile
	if err := xml.Unmarshal(data, &f); err != nil {
		return nil, nil, xmlErr("cfgeventspawns.xml", err)
	}
	m := &Model{Kind: "eventspawns", Title: "cfgeventspawns.xml — точки спавна событий (обзор)",
		FileDesc: "Координаты, где могут появляться события из events.xml. Редактировать координаты удобнее через DayZ Editor / iZurvive; здесь — обзор и поиск проблем."}
	seen := map[string]bool{}
	for _, e := range f.Events {
		if seen[strings.ToLower(e.Name)] {
			m.Info = append(m.Info, "ДУБЛИКАТ события: "+e.Name)
		}
		seen[strings.ToLower(e.Name)] = true
		if len(e.Pos) == 0 && len(e.Zones) == 0 {
			m.Info = append(m.Info, fmt.Sprintf("%s: НЕТ точек спавна — событие с position=fixed не заспавнится", e.Name))
		} else {
			m.Info = append(m.Info, fmt.Sprintf("%s: точек %d", e.Name, len(e.Pos)+len(e.Zones)))
		}
	}
	return m, &f, nil
}

// ---------- общий вход ----------

func xe(s string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

func xmlErr(what string, err error) error {
	if se, ok := err.(*xml.SyntaxError); ok {
		return fmt.Errorf("%s: ошибка XML в строке %d: %s (проверьте во вкладке «Проверка файлов»)", what, se.Line, se.Msg)
	}
	return fmt.Errorf("%s: %v", what, err)
}

// Open распознаёт файл по имени/содержимому и строит модель.
func Open(fileName string, data []byte) (*Model, interface{}, error) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	low := strings.ToLower(fileName)
	trimmed := bytes.TrimLeft(data, " \t\r\n\xef\xbb\xbf")
	isJSON := strings.HasSuffix(low, ".json") || (len(trimmed) > 0 && trimmed[0] == '{')

	if isJSON {
		if strings.Contains(low, "effectarea") || bytes.Contains(data, []byte("\"Areas\"")) {
			m, v, err := parseJSONForm(fileName, data, "effectarea", "cfgeffectarea.json — заражённые/эффектные зоны",
				"Газовые зоны и их визуал: центр (Pos), радиус, высота, частицы, безопасные позиции. Защита от зон настраивается в конфиге одежды (вкладка «1 Мод» — Protection).", effectDesc)
			if m != nil {
				m.FileName = fileName
			}
			return m, v, err
		}
		m, v, err := parseJSONForm(fileName, data, "gameplay", "cfggameplay.json — игровые правила сервера",
			"Стамина, урон базам, ночное освещение (lightingConfig), карта, HUD, строительство и колёса транспорта. В DayZ 1.30 cfggameplay.json включён по умолчанию. Редактор сохраняет параметры загруженного файла без добавления отсутствующих настроек.", gameplayDesc)
		if m != nil {
			m.FileName = fileName
		}
		return m, v, err
	}
	root := xmlRootName(data)
	var m *Model
	var v interface{}
	var err error
	switch {
	case root == "variables" || strings.Contains(low, "globals"):
		m, v, err = parseGlobals(data)
	case root == "economy":
		m, v, err = parseEconomy(data)
	case root == "weather" || strings.Contains(low, "weather"):
		m, v, err = parseWeather(data)
	case root == "events":
		m, v, err = parseEvents(data)
	case root == "eventposdef" || strings.Contains(low, "eventspawns"):
		m, v, err = parseEventSpawns(data)
	case root == "playerspawnpoints" || strings.Contains(low, "spawnpoints"):
		m, v, err = parseSpawnPoints(data)
	default:
		return nil, nil, fmt.Errorf("не узнаю файл %q (корень <%s>). Поддерживаются: globals.xml, economy.xml, events.xml, cfgweather.xml, cfggameplay.json, cfgeffectarea.json, cfgplayerspawnpoints.xml, cfgeventspawns.xml", fileName, root)
	}
	if m != nil {
		m.FileName = fileName
	}
	return m, v, err
}

func xmlRootName(data []byte) string {
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err != nil {
			return ""
		}
		if se, ok := tok.(xml.StartElement); ok {
			return strings.ToLower(se.Name.Local)
		}
	}
}

// Apply применяет правки к исходной модели и возвращает новый файл.
func Apply(kind string, orig interface{}, values map[string]string, events []EventRow) ([]byte, error) {
	switch kind {
	case "globals":
		return applyGlobals(orig, values), nil
	case "economy":
		return applyEconomy(orig, values), nil
	case "weather":
		return applyWeather(orig, values), nil
	case "events":
		return applyEvents(orig, events), nil
	case "gameplay", "effectarea":
		return applyJSON(orig, values), nil
	case "spawnpoints":
		return applySpawnPoints(orig, values), nil
	}
	return nil, fmt.Errorf("файл типа %q доступен только для просмотра", kind)
}
