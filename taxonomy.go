/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Taxonomy
 *
 * This component defines canonical semantic object metadata such as per-object device class, aggregation-domain, and default-sphere mappings.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 21.03.2026
 *
 */

package main

// DeviceClassOf keeps the object-level device class mapping from the legacy DSL settings.
var DeviceClassOf = map[string]string{
	"battery_alert": "battery",
	"battery_level": "battery",
	"co2":           "carbon_dioxide",
	"illuminance":   "light",
	"motion":        "motion",
	"node":          "connectivity",
	"noise":         "signal_strength",
	"pressure":      "atmospheric_pressure",
	"temperature":   "temperature",
	"wind_speed":    "wind_speed",
	"windy":         "safety",
	"sunny":         "light",
}

func lookupDeviceClass(object string) (string, bool) {
	deviceClass, exists := DeviceClassOf[object]
	return deviceClass, exists
}

// AggregatedDomainOf defines which Home Assistant domain an aggregate object belongs to for upward space aggregation.
var AggregatedDomainOf = map[string]string{
	"battery_alert": "binary_sensor",
	"battery_level": "sensor",
	"climate":       "climate",
	"co2":           "sensor",
	"cover_close":   "script",
	"cover_open":    "script",
	"cover_stop":    "script",
	"door":          "binary_sensor",
	"heating":       "switch",
	"humidity":      "sensor",
	"illuminance":   "sensor",
	"light":         "light",
	"media":         "switch",
	"media_player":  "switch",
	"motion":        "binary_sensor",
	"node":          "binary_sensor",
	"noise":         "sensor",
	"pressure":      "sensor",
	"temperature":   "sensor",
	"water":         "binary_sensor",
	"window":        "binary_sensor",
}

func lookupAggregatedDomain(object string) (string, bool) {
	domain, exists := AggregatedDomainOf[object]
	return domain, exists
}

// SphereOf defines the default sphere for objects when no sphere is provided explicitly.
var SphereOf = map[string]string{
	"battery_alert": "infrastructural",
	"door":          "social",
	"motion":        "social",
	"water":         "social",
	"sunny":         "social",
	"node_alert":    "infrastructural",
	"window":        "social",
}

func lookupDefaultSphere(object string) (string, bool) {
	sphere, exists := SphereOf[object]
	return sphere, exists
}
