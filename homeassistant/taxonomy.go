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

// postfixCapabilityDefaults returns the seed icon for a capability, keyed by its path's last
// segment (e.g. "node", "cpu/temperature" -> "temperature"), from subdomainIcons -- the
// remaining domain-agnostic fallback for subdomains with no settled domain. device_class/unit/
// state_class have no code-level fallback left: every domain-settled subdomain's full typing
// (including its own icon) now lives in Shared/Definitions/Defaults.def's "defaults:" block
// instead (capability_defaults.go's resolveCapabilityDefaults, checked first, domain-gated).
// This is the lowest-priority tier: called only to fill in a field neither the capability's own
// explicit metadata nor a "defaults: for ...;" rule already supplied.
func postfixCapabilityDefaults(domain, path string) (deviceClass, unit, stateClass, icon string) {
	icon = subdomainIcons[lastPathSegment(path)]
	return
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
	"device":        "infrastructural",
	"door":          "social",
	"motion":        "social",
	"water":         "social",
	"sunny":         "social",
	"window":        "social",
}

func lookupDefaultSphere(object string) (string, bool) {
	sphere, exists := SphereOf[object]
	return sphere, exists
}

// isKnownSphere reports whether s is one of the three recognised entity spheres.
func isKnownSphere(s string) bool {
	return s == "social" || s == "physical" || s == "infrastructural"
}
