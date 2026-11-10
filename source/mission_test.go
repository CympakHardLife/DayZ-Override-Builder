package mission

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// Representative values from the official DayZ 1.30 Nasdara gameplay file,
// plus unknown fields to verify that the dynamic editor keeps extensions.
const gameplay130 = `{
    "version": 130,
    "WorldsData": {
        "lightingConfig": 3,
        "bunkerBroadcastEnabled": true,
        "objectSpawnersArr": [],
        "environmentMinTemps": [3.4, 5.4, 7.4]
    },
    "BaseBuildingData": {
        "HologramData": {
            "disableColdAreaPlacementCheck": true,
            "disallowedTypesInUnderground": ["FenceKit", "TerritoryFlagKit", "WatchtowerKit"]
        },
        "ConstructionData": {
            "disablePerformRoofCheck": false,
            "disableIsCollidingCheck": false,
            "disableDistanceCheck": false,
            "disableItemFalling": false,
            "disableDropAttachmentsOnFall": false,
            "disableRefundMaterialsOnFall": false
        }
    },
    "VehicleData": {
        "boatDecayMultiplier": 1,
        "canDetachAttachedCarWheels": true,
        "canDamageAttachedCarWheels": true,
        "canDetachAttachedBikeWheels": true,
        "canDamageAttachedBikeWheels": true
    },
    "FutureData": {
        "flags": [true, false],
        "names": ["001", "true", "a,b", " padded "],
        "optional": null,
        "empty": {},
        "rows": [{"enabled": true, "extension": {"value": 7}}]
    }
}`

func decodeJSON(t *testing.T, data []byte) interface{} {
	t.Helper()
	var value interface{}
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	return value
}

func modelFields(model *Model) map[string]Field {
	fields := make(map[string]Field)
	for _, group := range model.Groups {
		for _, field := range group.Fields {
			fields[field.ID] = field
		}
	}
	return fields
}

func TestGameplay130GuidanceAndRoundTrip(t *testing.T) {
	model, original, err := Open("cfggameplay.json", []byte(gameplay130))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(model.FileDesc, "1.30") || strings.Contains(model.FileDesc, "enableCfgGameplayFile") {
		t.Fatalf("outdated gameplay guidance: %s", model.FileDesc)
	}
	fields := modelFields(model)
	if field := fields["WorldsData.lightingConfig"]; field.Value != "3" || !strings.Contains(field.Desc, "Nasdara") {
		t.Fatalf("missing Nasdara lighting guidance: %+v", field)
	}
	for _, path := range []string{
		"WorldsData.bunkerBroadcastEnabled",
		"VehicleData.canDetachAttachedCarWheels",
		"VehicleData.canDamageAttachedCarWheels",
		"VehicleData.canDetachAttachedBikeWheels",
		"VehicleData.canDamageAttachedBikeWheels",
		"BaseBuildingData.HologramData.disableColdAreaPlacementCheck",
		"BaseBuildingData.ConstructionData.disablePerformRoofCheck",
		"BaseBuildingData.ConstructionData.disableIsCollidingCheck",
		"BaseBuildingData.ConstructionData.disableDistanceCheck",
		"BaseBuildingData.ConstructionData.disableItemFalling",
		"BaseBuildingData.ConstructionData.disableDropAttachmentsOnFall",
		"BaseBuildingData.ConstructionData.disableRefundMaterialsOnFall",
	} {
		field, ok := fields[path]
		if !ok || field.Kind != "boolTF" || field.Desc == "" {
			t.Errorf("missing boolean guidance for %s: %+v", path, field)
		}
	}
	if field := fields["BaseBuildingData.HologramData.disallowedTypesInUnderground"]; field.Kind != "text" || !strings.Contains(field.Desc, "под землёй") {
		t.Errorf("missing underground list guidance: %+v", field)
	}
	values := make(map[string]string, len(fields))
	for path, field := range fields {
		values[path] = field.Value
	}
	output, err := Apply(model.Kind, original, values, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := decodeJSON(t, output), decodeJSON(t, []byte(gameplay130)); !reflect.DeepEqual(got, want) {
		t.Fatalf("full form save changed unedited values:\n%s", output)
	}
}

func TestGameplay130EditsPreserveUnknownFieldsAndOriginal(t *testing.T) {
	model, original, err := Open("cfggameplay.json", []byte(gameplay130))
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]string{
		"WorldsData.bunkerBroadcastEnabled":                          "false",
		"WorldsData.environmentMinTemps":                             "1, 2.5, -3",
		"VehicleData.canDetachAttachedCarWheels":                     "false",
		"VehicleData.canDamageAttachedBikeWheels":                    "false",
		"BaseBuildingData.ConstructionData.disableItemFalling":       "true",
		"BaseBuildingData.HologramData.disallowedTypesInUnderground": "FenceKit, 130",
		"FutureData.flags":                                           "false, true, false",
		"FutureData.rows[0].enabled":                                 "false",
	}
	output, err := Apply(model.Kind, original, values, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := decodeJSON(t, []byte(gameplay130)).(map[string]interface{})
	world := want["WorldsData"].(map[string]interface{})
	world["bunkerBroadcastEnabled"] = false
	world["environmentMinTemps"] = []interface{}{float64(1), 2.5, float64(-3)}
	vehicles := want["VehicleData"].(map[string]interface{})
	vehicles["canDetachAttachedCarWheels"] = false
	vehicles["canDamageAttachedBikeWheels"] = false
	building := want["BaseBuildingData"].(map[string]interface{})
	building["ConstructionData"].(map[string]interface{})["disableItemFalling"] = true
	building["HologramData"].(map[string]interface{})["disallowedTypesInUnderground"] = []interface{}{"FenceKit", "130"}
	future := want["FutureData"].(map[string]interface{})
	future["flags"] = []interface{}{false, true, false}
	future["rows"].([]interface{})[0].(map[string]interface{})["enabled"] = false
	if got := decodeJSON(t, output); !reflect.DeepEqual(got, want) {
		t.Fatalf("edited gameplay lost values or types:\n%s", output)
	}
	if wantOriginal := decodeJSON(t, []byte(gameplay130)); !reflect.DeepEqual(original, wantOriginal) {
		t.Fatal("applying values mutated the original parsed model")
	}
	if got := decodeJSON(t, ApplyJSONValues(original, nil)); !reflect.DeepEqual(got, original) {
		t.Fatal("a second save unexpectedly retained the previous edits")
	}
}

func TestGameplayKeepsAbsent130SettingsAbsent(t *testing.T) {
	input := []byte(`{"version":123,"WorldsData":{"lightingConfig":0},"Extension":{"keep":true}}`)
	model, original, err := Open("cfggameplay.json", input)
	if err != nil {
		t.Fatal(err)
	}
	output, err := Apply(model.Kind, original, map[string]string{"WorldsData.lightingConfig": "3"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := decodeJSON(t, input).(map[string]interface{})
	want["WorldsData"].(map[string]interface{})["lightingConfig"] = float64(3)
	if got := decodeJSON(t, output); !reflect.DeepEqual(got, want) {
		t.Fatalf("save added settings or changed the file version:\n%s", output)
	}
}
