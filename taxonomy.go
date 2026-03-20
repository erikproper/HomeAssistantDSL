/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Taxonomy
 *
 * This component defines canonical semantic object metadata such as per-object device class, aggregation-domain, icon, and default-sphere mappings.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 20.03.2026
 *xx1do
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

// IconOf keeps object-level icon defaults from the legacy DSL settings.
var IconOf = map[string]string{
	"battery_alert": "mdi:battery-alert",
	"battery_level": "mdi:battery",
	"co2":           "mdi:cloud",
	"condition":     "mdi:weather-cloudy",
	"cover":         "mdi:blinds-horizontal",
	"cover_close":   "mdi:triangle-down",
	"cover_stop":    "mdi:rectangle",
	"cover_open":    "mdi:triangle",
	"daylight":      "mdi:weather-sunset-up",
	"door":          "mdi:door-open",
	"health":        "mdi:cloud",
	"humidity":      "mdi:water-percent",
	"illuminance":   "mdi:brightness-5",
	"load":          "mdi:cpu-64-bit",
	"media":         "mdi:monitor-speaker",
	"motion":        "mdi:motion-sensor",
	"node":          "mdi:server-network",
	"node_alert":    "mdi:server-off",
	"noise":         "mdi:volume-high",
	"pressure":      "mdi:gauge",
	"radio":         "mdi:signal",
	"sunny":         "mdi:sunglasses",
	"temperature":   "mdi:thermometer",
	"water":         "mdi:water-off",
	"window":        "mdi:window-open",
	"wind_speed":    "mdi:weather-windy",
	"wind_direction": "mdi:compass-outline",
}

func lookupIcon(object string) (string, bool) {
	icon, exists := IconOf[object]
	return icon, exists
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