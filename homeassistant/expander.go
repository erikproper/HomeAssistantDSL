/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Expander
 *
 * Shared DSL-wide text utilities: Settings.def variable substitution, space-header/entity-
 * declaration parsing, entity full-name normalisation, and space/aggregate path helpers. The
 * macro-definition/expansion engine this file used to also hold (creation macros, "call X;"
 * invocations) was retired 2026-09-30, once PROJECT.md's own DSL obsolescence sweep migrated
 * every real "call <macro>;" site in both houses off it -- see Macros.def's own retirement note.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 30.09.2026
 *
 */

package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// resolveSettingsVar replaces a single ${varname} reference with its value from the settings map.
// Returns the original string unchanged if it contains no variable reference or the name is not found.
func resolveSettingsVar(s string, settings map[string]string) string {
	if strings.HasPrefix(s, "${") && strings.HasSuffix(s, "}") {
		name := strings.ToLower(s[2 : len(s)-1])
		if val, ok := settings[name]; ok {
			return strings.Trim(val, "\"")
		}
	}
	return s
}

// bareSettingsVariablePattern matches one "${name}" reference, no ".accessor" suffix -- unlike
// variableReferencePattern above (which also matches macro-parameter-style "${x.y}" references),
// every Settings.def variable is always a bare name.
var bareSettingsVariablePattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// substituteSettingsVariables replaces every "${name}" reference anywhere in text with its
// Settings.def value, leaving an unknown name's own literal "${name}" text unchanged (same
// silent-passthrough convention as resolveSettingsVar, rather than erroring: some future non-
// settings use of "${...}" syntax could exist elsewhere in the same text). Used by layers that
// have no per-field resolution of their own, unlike Conceptual.def (specific call sites, e.g.
// icon/input_number fields in parser.go) or Macros.def (resolved as part of ordinary macro-
// argument substitution during expansion) -- Logical.def is the first such case, 2026-09-18.
func substituteSettingsVariables(text string, settings map[string]string) string {
	return bareSettingsVariablePattern.ReplaceAllStringFunc(text, func(match string) string {
		return resolveSettingsVar(match, settings)
	})
}

// extractVariableName strips ${...} or $ prefix and any .accessor suffix, returning the bare name.
func extractVariableName(ref string) string {
	if strings.HasPrefix(ref, "${") && strings.HasSuffix(ref, "}") {
		inner := ref[2 : len(ref)-1]
		if dotIdx := strings.Index(inner, "."); dotIdx > 0 {
			return strings.ToLower(inner[:dotIdx])
		}
		return strings.ToLower(inner)
	}
	return strings.ToLower(strings.TrimPrefix(ref, "$"))
}


// parseSpaceHeader recognises "space <spec> with:", "virtual space <spec> with:", and the "as
// area" variant of either ("space <spec> as area with:") -- see administration.go's OpenSpace
// for what isArea then does. Returns spaceKind, spec (with any "as area" marker stripped),
// isArea, and whether line matched at all.
func parseSpaceHeader(line string) (string, string, bool, bool) {
	if !strings.HasSuffix(line, "with:") {
		return "", "", false, false
	}

	spaceKind := SpaceKindRegular
	header := line
	if strings.HasPrefix(header, "virtual space ") {
		spaceKind = SpaceKindVirtual
		header = strings.TrimPrefix(header, "virtual space ")
	} else if strings.HasPrefix(header, "space ") {
		header = strings.TrimPrefix(header, "space ")
	} else {
		return "", "", false, false
	}

	spec := ""
	if idx := strings.Index(header, " with"); idx > 0 {
		spec = header[:idx]
	}

	isArea := false
	if trimmed := strings.TrimSuffix(spec, " as area"); trimmed != spec {
		spec = trimmed
		isArea = true
	}

	return spaceKind, spec, isArea, true
}

func formatSpaceLabel(spaceKind string) string {
	if spaceKind == SpaceKindVirtual {
		return "Virtual space"
	}
	return "Space"
}

func parseSpaceCollectionItems(itemsStr string) []string {
	if itemsStr == "" {
		return []string{}
	}
	parts := strings.Fields(itemsStr)
	return parts
}

type TEntityDeclaration struct {
	Specification string
	NoCollect     bool
}

func extractEntityDeclaration(line string) (*TEntityDeclaration, bool) {
	if !strings.HasPrefix(line, "entity ") {
		return nil, false
	}

	rest := strings.TrimSpace(strings.TrimPrefix(line, "entity "))
	if rest == "" {
		return nil, false
	}

	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return nil, false
	}

	decl := &TEntityDeclaration{
		Specification: strings.TrimSuffix(fields[0], ";"),
		NoCollect:     false,
	}

	for _, field := range fields[1:] {
		if strings.TrimSuffix(field, ";") == "no_collect" {
			decl.NoCollect = true
			break
		}
	}

	return decl, true
}

func analyzeEntityDefinitionContext(lines []string, entityLineIdx int) (bool, []string) {
	if entityLineIdx < 0 || entityLineIdx >= len(lines) {
		return false, nil
	}

	line := strings.TrimSpace(lines[entityLineIdx])
	if !strings.HasPrefix(line, "entity ") {
		return false, nil
	}

	hasDefinitionOrImported := false
	optionKeys := map[string]bool{}

	consume := func(statement string) {
		clean := strings.TrimSpace(strings.TrimSuffix(statement, ";"))
		if clean == "" {
			return
		}

		lower := strings.ToLower(clean)
		// "definition as ..." (legacy prefix), "imported ...", and bare definitional
		// keywords all mark the entity as defined by this system (not assumed from HA).
		for _, kw := range []string{
			"definition as ", "imported ",
			"adjustment ", "cli_switch ", "condition ", "value ", "available ",
		} {
			if strings.HasPrefix(lower, kw) {
				hasDefinitionOrImported = true
				return
			}
		}

		fields := strings.Fields(lower)
		if len(fields) == 0 {
			return
		}

		optionKeys[fields[0]] = true
	}

	if withIdx := strings.Index(line, " with "); withIdx >= 0 && !strings.HasSuffix(line, " with:") {
		consume(line[withIdx+len(" with "):])
	}

	if strings.HasSuffix(line, " with") {
		for i := entityLineIdx + 1; i < len(lines); i++ {
			next := strings.TrimSpace(lines[i])
			if next == "" || strings.HasPrefix(next, "#") {
				continue
			}
			consume(next)
			break
		}
	}

	if strings.HasSuffix(line, " with:") {
		depth := 1
		for i := entityLineIdx + 1; i < len(lines); i++ {
			next := strings.TrimSpace(lines[i])
			if next == "" || strings.HasPrefix(next, "#") {
				continue
			}

			if strings.HasSuffix(next, " with:") {
				depth++
				continue
			}

			if next == "end;" {
				depth--
				if depth == 0 {
					break
				}
				continue
			}

			if depth == 1 {
				consume(next)
			}
		}
	}

	keys := []string{}
	for key := range optionKeys {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return hasDefinitionOrImported, keys
}

func normalizeEntityFullName(spec string, spacePath []string) string {
	spec = strings.TrimSpace(strings.TrimSuffix(spec, ";"))
	if spec == "" {
		return spec
	}

	if strings.Contains(spec, ".[") {
		return spec
	}

	if isExtensionalEntityReference(spec) {
		return spec
	}

	dotIdx := strings.Index(spec, ".")
	if dotIdx <= 0 || dotIdx >= len(spec)-1 {
		// Handle sphere:path form (entity_path without domain, e.g. "social:dishwasher").
		// Delegate by prepending a dummy domain so the sphere/path logic below handles context.
		if colonIdx := strings.Index(spec, ":"); colonIdx > 0 {
			if isKnownSphere(spec[:colonIdx]) {
				full := normalizeEntityFullName("_."+spec, spacePath)
				return strings.TrimPrefix(full, "_.")
			}
		}
		return strings.ReplaceAll(spec, ":", "/")
	}

	typePart := spec[:dotIdx]
	remainder := spec[dotIdx+1:]
	colonIdx := strings.Index(remainder, ":")
	var spherePart, rawPathPart string
	if colonIdx < 0 {
		// If remainder is itself a sphere name (e.g. "cover.social" with no colon or slash),
		// treat it as type.sphere with an empty path — the path comes entirely from the space context.
		if isKnownSphere(remainder) {
			spherePart = remainder
			rawPathPart = ""
		} else {
			spherePart, _ = lookupDefaultSphere(typePart)
			if spherePart == "" {
				spherePart = "social"
			}
			rawPathPart = remainder
		}
	} else {
		spherePart = remainder[:colonIdx]
		rawPathPart = remainder[colonIdx+1:]
		if spherePart == "" {
			// An empty sphere segment before the colon (e.g. "device.:air-4") explicitly asks for
			// the domain's default sphere, same fallback the no-colon branch above already uses --
			// not a literal empty sphere, which would otherwise produce a malformed "type./path".
			spherePart, _ = lookupDefaultSphere(typePart)
			if spherePart == "" {
				spherePart = "social"
			}
		}
	}

	if spherePart == "_" {
		rawName := strings.TrimPrefix(rawPathPart, "/")
		if rawName != "" {
			return fmt.Sprintf("%s.[%s]", typePart, rawName)
		}
	}
	pathPart := rawPathPart
	pathPart = strings.ReplaceAll(pathPart, ":", "/")
	_, contextPath := normalizeSpaceContext(spacePath)

	if contextPath != "" {
		contextParts := strings.Split(contextPath, "/")
		if len(contextParts) > 0 && contextParts[0] == spherePart {
			contextPath = strings.Join(contextParts[1:], "/")
		}
	}

	if strings.HasPrefix(rawPathPart, "/") {
		pathPart = strings.TrimPrefix(pathPart, "/")
	} else {
		pathPart = strings.TrimPrefix(pathPart, "/")
		pathPart = strings.Trim(pathPart, "/")
		if contextPath != "" {
			if pathPart == "" {
				pathPart = contextPath
			} else {
				pathPart = contextPath + "/" + pathPart
			}
		}
	}
	pathPart = strings.Trim(pathPart, "/")

	if pathPart == "" {
		return fmt.Sprintf("%s.%s", typePart, spherePart)
	}

	return fmt.Sprintf("%s.%s/%s", typePart, spherePart, pathPart)
}

func isExtensionalEntityReference(spec string) bool {
	dotIdx := strings.Index(spec, ".")
	slashIdx := strings.Index(spec, "/")
	if dotIdx > 0 && slashIdx > dotIdx {
		afterDot := spec[dotIdx+1:]
		// A spec is only extensional if it has at least one '/' after the dot AND
		// contains no ':' — a remaining ':' means the space path still needs inserting.
		// A leading '/' right after the dot is never a real extensional reference either (a
		// resolved reference's sphere segment is always a bare word, e.g.
		// "sensor.infrastructural/co2" -- never "sensor./co2") -- it's a bare "device.<leaf-path>"
		// spec whose own leaf happens to start with '/' (the "absolute, no space-prefix" leaf
		// shape a device's own "as <leaf-path>" clause can carry, since it can no longer carry an
		// explicit sphere at all -- confirmed 2026-09-24, real bug found live: without this guard,
		// "device./smarty" was misdetected as already-resolved and returned verbatim, skipping
		// lookupDefaultSphere("device") entirely and leaving every attribute anchored to that
		// device with an empty sphere segment in its own final name).
		if strings.Count(afterDot, "/") >= 1 && !strings.Contains(afterDot, ":") && !strings.HasPrefix(afterDot, "/") {
			return true
		}
	}
	return false
}

func extractEntityIdentity(fullName string) TEntityIdentity {
	identity := TEntityIdentity{}

	dotIdx := strings.Index(fullName, ".")
	if dotIdx <= 0 || dotIdx >= len(fullName)-1 {
		return identity
	}

	identity.Domain = fullName[:dotIdx]
	remainder := fullName[dotIdx+1:]

	slashIdx := strings.Index(remainder, "/")
	if slashIdx < 0 {
		identity.Sphere = remainder
		return identity
	}

	identity.Sphere = remainder[:slashIdx]
	if slashIdx+1 < len(remainder) {
		identity.Path = remainder[slashIdx+1:]
	}

	return identity
}

func formatNestedSpaceName(spacePath []string) string {
	if len(spacePath) == 0 {
		return "root"
	}

	spaceType, contextPath := normalizeSpaceContext(spacePath)
	if spaceType == "" {
		if contextPath == "" {
			return "root"
		}
		return contextPath
	}
	if contextPath == "" {
		return spaceType
	}
	return fmt.Sprintf("%s/%s", spaceType, contextPath)
}

func nestedSpaceDepth(spacePath []string) int {
	if len(spacePath) == 0 {
		return 0
	}

	_, contextPath := normalizeSpaceContext(spacePath)
	if contextPath == "" {
		return 0
	}

	parts := strings.Split(contextPath, "/")
	if len(parts) <= 1 {
		return 0
	}
	return len(parts) - 1
}

func normalizeSpaceContext(spacePath []string) (string, string) {
	spaceType := ""
	contextParts := []string{}
	first := true
	for _, segment := range spacePath {
		segmentType, segmentName := splitSpaceSegment(segment)
		if first {
			if segmentType != "" {
				spaceType = segmentType
			}
			segmentName = strings.ReplaceAll(segmentName, ":", "/")
			segmentName = strings.Trim(segmentName, "/")
			if segmentName != "" {
				contextParts = append(contextParts, strings.Split(segmentName, "/")...)
			}
			first = false
		} else {
			segmentName = strings.ReplaceAll(segmentName, ":", "/")
			segmentName = strings.Trim(segmentName, "/")
			if segmentName != "" {
				contextParts = append(contextParts, strings.Split(segmentName, "/")...)
			}
		}
	}
	return spaceType, strings.Join(contextParts, "/")
}

func splitSpaceSegment(segment string) (string, string) {
	if idx := strings.Index(segment, ":"); idx >= 0 {
		return segment[:idx], segment[idx+1:]
	}
	return "", segment
}

func impliedAggregatesForSpace(spaceName, spaceKind string, spaceOrder []string, entityRecordsBySpace map[string][]TEntityRecord) []string {
	aggregates := []string{}

	sensorMetricAggregates := map[string]bool{}
	includeSensorAggregates := spaceKind != SpaceKindVirtual

	hasLightEntities := false
	hasMediaSwitch := false
	hasSpaceSwitch := false
	for _, record := range entityRecordsBySpace[spaceName] {
		if record.NoCollect {
			continue
		}
		if record.Identity.Domain == "light" {
			hasLightEntities = true
		}
		if record.Identity.Domain == "switch" && record.Identity.Sphere == "social" {
			if lastPathSegment(record.Identity.Path) == "media" {
				hasMediaSwitch = true
			}
			if lastPathSegment(record.Identity.Path) == "space" {
				hasSpaceSwitch = true
			}
		}
		if record.Identity.Domain == "sensor" && includeSensorAggregates {
			metric := lastPathSegment(record.Identity.Path)
			if isAggregateSensorMetric(metric) {
				sensorMetricAggregates[metric] = true
			}
		}
	}
	spacePath := spaceName
	if strings.HasPrefix(spacePath, "social/") {
		spacePath = strings.TrimPrefix(spacePath, "social/")
	}
	if hasLightEntities {
		aggregates = append(aggregates, fmt.Sprintf("light.social/%s", spacePath))
	}
	if hasMediaSwitch {
		aggregates = append(aggregates, fmt.Sprintf("switch.social/%s/media", spacePath))
	}
	if hasSpaceSwitch {
		aggregates = append(aggregates, fmt.Sprintf("switch.social/%s/space", spacePath))
	}

	metrics := []string{}
	for metric := range sensorMetricAggregates {
		metrics = append(metrics, metric)
	}
	sort.Strings(metrics)
	for _, metric := range metrics {
		aggregates = append(aggregates, fmt.Sprintf("sensor.%s/%s", spaceName, metric))
	}

	return aggregates
}

func lastPathSegment(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

func findAggregateConstituents(aggregateName, spaceName string, spaceOrder []string, entityRecordsBySpace map[string][]TEntityRecord) []string {
	constituents := []string{}

	if strings.HasPrefix(aggregateName, "switch.social/") {
		for _, rec := range entityRecordsBySpace[spaceName] {
			if rec.NoCollect {
				continue
			}
			if rec.Identity.Domain == "switch" && rec.Identity.Sphere == "social" {
				if rec.Name == aggregateName {
					continue
				}
				if aggregateName == fmt.Sprintf("switch.social/%s/media", spaceName) && lastPathSegment(rec.Identity.Path) == "media" {
					constituents = append(constituents, rec.Name)
				}
				if aggregateName == fmt.Sprintf("switch.social/%s/space", spaceName) && lastPathSegment(rec.Identity.Path) == "space" {
					constituents = append(constituents, rec.Name)
				}
			}
		}
		return normalizeFullNames(constituents, spaceName)
	}

	if strings.HasPrefix(aggregateName, "sensor.") {
		idx := strings.LastIndex(aggregateName, "/")
		if idx < 0 {
			return constituents
		}
		metric := aggregateName[idx+1:]
		for _, rec := range entityRecordsBySpace[spaceName] {
			if rec.NoCollect {
				continue
			}
			if rec.Identity.Domain == "sensor" && lastPathSegment(rec.Identity.Path) == metric {
				constituents = append(constituents, rec.Name)
			}
		}
		return normalizeFullNames(constituents, spaceName)
	}

	return normalizeFullNames(constituents, spaceName)
}

func normalizeFullNames(names []string, spaceName string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, normalizeEntityFullName(n, strings.Split(spaceName, "/")))
	}
	return out
}

// resolveConditionEntityID resolves an entity reference in a condition spec to a HA
// entity ID.  If the reference is already a complete HA entity ID (no colon, maps to
// itself under toHomeAssistantEntityID), it is used directly — bypassing space-context
// normalisation.  This is required for raw-form references such as "sun.sun", which
// must not be normalised to "sun.social_sun" by the space context.
func resolveConditionEntityID(entityPart string, spacePath []string) string {
	if !strings.Contains(entityPart, ":") {
		if id := toHomeAssistantEntityID(entityPart); id == entityPart {
			return id
		}
	}
	return toHomeAssistantEntityID(normalizeEntityFullName(entityPart, spacePath))
}

// parseConditionSpec parses a single condition source spec which may be either
// a plain entity reference or an entity with an attribute suffix ("entity!attr").
// Returns a source token: "entity_id" for plain state access, or "entity_id!attr"
// for attribute access via state_attr().
func parseConditionSpec(spec string, spacePath []string) string {
	spec = strings.TrimSuffix(spec, ";")
	if strings.Contains(spec, "${") {
		return "" // unresolved variable — skip
	}
	if bangIdx := strings.Index(spec, "!"); bangIdx > 0 {
		entityPart := spec[:bangIdx]
		attr := spec[bangIdx+1:]
		entityID := resolveConditionEntityID(entityPart, spacePath)
		if entityID == "" {
			return ""
		}
		return entityID + "!" + attr
	}
	return resolveConditionEntityID(spec, spacePath)
}

// parseConditionDirective extracts sources and expression from a "condition ..." directive
// string. Also recognises the unquoted "condition <source> is available;" sugar -- shorthand for
// the standard "states('$1') not in ['unavailable', 'unknown']" liveness-check boilerplate that
// used to have to be spelled out by hand in every Macros.def condition body needing it.
func parseConditionDirective(t string, spacePath []string) (sources []string, expr string) {
	if rest, ok := strings.CutPrefix(t, "condition "); ok {
		trimmed := strings.TrimSuffix(strings.TrimSpace(rest), ";")
		if bareSpec, ok := strings.CutSuffix(trimmed, " is available"); ok {
			if tok := parseConditionSpec(bareSpec, spacePath); tok != "" {
				// $1 gets substituted with sourceToJinja2(sources[0]) -- already a full
				// states('...')/state_attr('...') call -- so this must NOT pre-wrap $1 itself
				// (that would double-wrap into states('states('...')')).
				return []string{tok}, "$1 not in ['unavailable', 'unknown']"
			}
			return nil, ""
		}
	}

	quoteIdx := strings.Index(t, "\"")
	if quoteIdx <= len("condition ") {
		return
	}
	beforeQuote := strings.TrimSpace(t[len("condition "):quoteIdx])
	for _, spec := range strings.Fields(beforeQuote) {
		if tok := parseConditionSpec(spec, spacePath); tok != "" {
			sources = append(sources, tok)
		}
	}
	afterQuote := t[quoteIdx+1:]
	if closeIdx := strings.LastIndex(afterQuote, "\""); closeIdx >= 0 {
		expr = afterQuote[:closeIdx]
	}
	return
}

// scanInputNumberBody extracts minimum, maximum, step, units, icon from an input_number entity body.
func scanInputNumberBody(lines []string, startIdx int) (min, max, step, unit, icon string) {
	depth := 0
	for i := startIdx + 1; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if strings.HasSuffix(t, "with:") {
			depth++
			continue
		}
		if t == "end;" {
			if depth == 0 {
				break
			}
			depth--
			continue
		}
		val := func(prefix string) string {
			return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(t, prefix), ";"))
		}
		if strings.HasPrefix(t, "minimum ") {
			min = val("minimum ")
		} else if strings.HasPrefix(t, "maximum ") {
			max = val("maximum ")
		} else if strings.HasPrefix(t, "step ") {
			step = val("step ")
		} else if strings.HasPrefix(t, "units ") {
			unit = strings.Trim(val("units "), "\"")
		} else if strings.HasPrefix(t, "icon ") {
			icon = strings.Trim(val("icon "), "\"")
		}
	}
	return
}


func isAggregateSensorMetric(metric string) bool {
	switch metric {
	case "temperature", "humidity", "co2", "pressure", "illuminance", "noise", "wind_speed", "wind_direction":
		return true
	default:
		return false
	}
}

