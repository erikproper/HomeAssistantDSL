/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Taxonomy
 *
 * This component defines canonical semantic object metadata such as per-object icon and default-sphere mappings.
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
