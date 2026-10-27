/*
 *
 * Module:    HomeAssistant
 * Package:   HAMQTT
 * Component: Fields
 *
 * Home Assistant's MQTT field table (PROJECT.md 19, moved here 2026-10-27): which fields each MQTT
 * domain accepts, and MQTT discovery's abbreviations. Shared by the generator (its YAML "mqtt:"
 * entities are what discovery would have given, and are validated against it) and the coordinator
 * (the device-block keys a gateway may use), so both follow the one Home Assistant version it was
 * taken from. Home Assistant's schema is therefore a code dependency (Architecture.md §6.16).
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 27.10.2026
 *
 */

package hamqtt

import (
	_ "embed"
	"encoding/json"
	"sort"
	"strings"
)

// fieldsJSON is the fields every Home Assistant MQTT domain accepts, taken from Home
// Assistant's own code by tools/ha_mqtt_fields.py (re-run it after a Home Assistant upgrade that
// changes components/mqtt).
//
//go:embed ha_mqtt_fields.json
var fieldsJSON []byte

// TFields is ha_mqtt_fields.json's shape.
type TFields struct {
	HAVersion           string              `json:"ha_version"`
	Domains             map[string][]string `json:"domains"`
	LightSchemas        map[string][]string `json:"light_schemas"`
	Abbreviations       map[string]string   `json:"abbreviations"`
	DeviceAbbreviations map[string]string   `json:"device_abbreviations"`
}

// Fields is the table itself.
var Fields = func() TFields {
	var fields TFields
	if err := json.Unmarshal(fieldsJSON, &fields); err != nil {
		panic("ha_mqtt_fields.json: " + err.Error())
	}
	return fields
}()

// DiscoveredConfig returns the configuration Home Assistant's own MQTT discovery would take
// from a discovery payload (homeassistant/components/mqtt/discovery.py): abbreviations expanded
// (top level, availability, device), the "~" topic base substituted into every "...topic", and
// then -- the domain's DISCOVERY_SCHEMA being its YAML schema with REMOVE_EXTRA -- every field the
// domain's schema doesn't know dropped. Exactly what Home Assistant does with it, so the YAML
// entity is equivalent to the discovered one (verified against Home Assistant 2026.10.3's
// sources, 2026-10-27). "origin" (discovery's own logging) and the deprecated "object_id" go too:
// default_entity_id takes its place.
func DiscoveredConfig(payload map[string]interface{}, domain string) map[string]interface{} {
	expanded := map[string]interface{}{}
	for key, value := range payload {
		if full, abbreviated := Fields.Abbreviations[key]; abbreviated {
			key = full
		}
		expanded[key] = value
	}
	// Every abbreviation first, then the topic base -- Home Assistant's own order.
	if list, isList := expanded["availability"].([]interface{}); isList {
		for i, item := range list {
			if entry, isMap := item.(map[string]interface{}); isMap {
				list[i] = expandAbbreviations(entry, Fields.Abbreviations)
			}
		}
	}
	if device, isMap := expanded["device"].(map[string]interface{}); isMap {
		expanded["device"] = expandAbbreviations(device, Fields.DeviceAbbreviations)
	}
	if base, has := expanded["~"].(string); has {
		delete(expanded, "~")
		for key, value := range expanded {
			if text, isText := value.(string); isText && text != "" && strings.HasSuffix(key, "topic") {
				if strings.HasPrefix(text, "~") {
					expanded[key] = base + text[1:]
				} else if strings.HasSuffix(text, "~") {
					expanded[key] = text[:len(text)-1] + base
				}
			}
		}
		if list, isList := expanded["availability"].([]interface{}); isList {
			for _, item := range list {
				if entry, isMap := item.(map[string]interface{}); isMap {
					if topic, isText := entry["topic"].(string); isText && topic != "" {
						if strings.HasPrefix(topic, "~") {
							entry["topic"] = base + topic[1:]
						} else if strings.HasSuffix(topic, "~") {
							entry["topic"] = topic[:len(topic)-1] + base
						}
					}
				}
			}
		}
	}

	config := map[string]interface{}{}
	for key, value := range expanded {
		if Accepts(domain, expanded, key) && key != "origin" && key != "object_id" {
			config[key] = value
		}
	}
	return config
}

// expandAbbreviations returns entry with its abbreviated keys written out.
func expandAbbreviations(entry map[string]interface{}, abbreviations map[string]string) map[string]interface{} {
	out := map[string]interface{}{}
	for key, value := range entry {
		if full, abbreviated := abbreviations[key]; abbreviated {
			key = full
		}
		out[key] = value
	}
	return out
}

// DroppedFields returns the fields of payload that DiscoveredConfig drops because domain's schema
// doesn't know them, written out and sorted -- "origin", "object_id" and the "~" topic base
// excluded, which go by design.
func DroppedFields(payload map[string]interface{}, domain string) []string {
	config := DiscoveredConfig(payload, domain)
	var dropped []string
	for key := range payload {
		if full, abbreviated := Fields.Abbreviations[key]; abbreviated {
			key = full
		}
		if _, kept := config[key]; !kept && key != "origin" && key != "object_id" && key != "~" {
			dropped = append(dropped, key)
		}
	}
	sort.Strings(dropped)
	return dropped
}

// Accepts reports whether domain's schema knows field -- for a light, the schema config names
// ("basic" when none).
func Accepts(domain string, config map[string]interface{}, field string) bool {
	known := Fields.Domains[domain]
	if domain == "light" {
		schema, _ := config["schema"].(string)
		if schema == "" {
			schema = "basic"
		}
		known = Fields.LightSchemas[schema]
	}
	for _, name := range known {
		if name == field {
			return true
		}
	}
	return false
}
