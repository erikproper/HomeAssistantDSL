/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Defaults
 *
 * This component provides code-level defaults and icon lookup metadata used by the DSL runtime.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 21.03.2026
 *
 */

package main

// DefaultGlobalVariables holds the code-level defaults for globally scoped variables.
// Definition files can override these by rebinding the same variable names.
var DefaultGlobalVariables = map[string]string{
	"around_icon":        "mdi:city",
	"away_icon":          "mdi:airplane",
	"battery_alert_icon": "mdi:battery-alert",
	"battery_level_icon": "mdi:battery",
	"condition_icon":     "mdi:weather-cloudy",
	"co2_icon":           "mdi:cloud",
	"cover_icon":         "mdi:blinds-horizontal",
	"cover_close_icon":   "mdi:triangle-down",
	"cover_open_icon":    "mdi:triangle",
	"cover_stop_icon":    "mdi:rectangle",
	"consumes_icon":      "mdi:flash",
	"cpu_icon":           "mdi:cpu-64-bit",
	"daylight_icon":      "mdi:weather-sunset-up",
	"door_icon":          "mdi:door-open",
	"health_icon":        "mdi:cloud",
	"heating_icon":       "mdi:radiator",
	"humidity_icon":      "mdi:water-percent",
	"illuminance_icon":   "mdi:brightness-5",
	"light_icon":         "mdi:lightbulb",
	"media_switch_icon":  "mdi:monitor-speaker",
	"motion_icon":        "mdi:motion-sensor",
	"node_alert_icon":    "mdi:server-off",
	"node_icon":          "mdi:server-network",
	"noise_icon":         "mdi:volume-high",
	"pressure_icon":      "mdi:gauge",
	"radio_icon":         "mdi:signal",
	"sunny_icon":         "mdi:sunglasses",
	"temperature_icon":   "mdi:thermometer",
	"water_icon":         "mdi:water-off",
	"window_icon":        "mdi:window-open",
	"wind_direction_icon": "mdi:compass-outline",
	"wind_speed_icon":    "mdi:weather-windy",
}

// IconVariableOf maps object sub-domains to the global variable that currently defines their icon.
var IconVariableOf = map[string]string{
	"battery_alert":  "battery_alert_icon",
	"battery_level":  "battery_level_icon",
	"co2":            "co2_icon",
	"condition":      "condition_icon",
	"cover":          "cover_icon",
	"cover_close":    "cover_close_icon",
	"cover_stop":     "cover_stop_icon",
	"cover_open":     "cover_open_icon",
	"daylight":       "daylight_icon",
	"door":           "door_icon",
	"health":         "health_icon",
	"humidity":       "humidity_icon",
	"illuminance":    "illuminance_icon",
	"load":           "cpu_icon",
	"media":          "media_switch_icon",
	"motion":         "motion_icon",
	"node":           "node_icon",
	"node_alert":     "node_alert_icon",
	"noise":          "noise_icon",
	"pressure":       "pressure_icon",
	"radio":          "radio_icon",
	"sunny":          "sunny_icon",
	"temperature":    "temperature_icon",
	"water":          "water_icon",
	"window":         "window_icon",
	"wind_speed":     "wind_speed_icon",
	"wind_direction": "wind_direction_icon",
}

func lookupIcon(object string) (string, bool) {
	variableName, exists := IconVariableOf[object]
	if !exists {
		return "", false
	}
	return lookupGlobalVariable(variableName)
}
