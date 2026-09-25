/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: PhysicalDerivedCapability
 *
 * Shared parsing for Physical.def's "derived DDD.NNN from EEE.MMM via TTT;" line --
 * plans/derived-capability-mechanism.md's Phase 2. Declares a new capability (DDD.NNN) whose
 * value is a function of a SIBLING capability of the same device (EEE.MMM), via a value_template
 * string TTT ("$" standing for EEE.MMM's own resolved value, same convention "condition" already
 * uses). One shared recognizer. home_assistant/hassbridge devices call parseDerivedCapabilityLine
 * as the first check in their own capability-line dispatch (integration_hassbridge_parser.go),
 * before any of their own domain-prefixed capability patterns. The "import" integration
 * (integration_import_parser.go) matches this same derivedCapabilityLinePattern too, but only to
 * REJECT the line with a specific warning -- an imported device may not declare "derived"
 * capabilities of its own; the exporting side owns all device-specific knowledge and must provide
 * it already-resolved (2026-09-12, withdrawing an earlier, temporary allowance -- see that file's
 * own header comment).
 *
 * Grammar dropped the redundant "entity" keyword 2026-09-14 (user: "the word entity is
 * unneeded there") -- "derived DDD.NNN from EEE.MMM via TTT;" now, no back-compat kept for the
 * older "derived entity ..." shape since every real declaration (both houses' Physical.def) was
 * updated in the same change.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 14.09.2026
 *
 */

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var derivedCapabilityLinePattern = regexp.MustCompile(`^derived\s+([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_/]*)\s+from\s+([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_/]*)\s+via\s+"([^"]*)"\s*;\s*$`)

// TDerivedCapabilityDeclaration is one parsed "derived DDD.NNN from EEE.MMM via TTT;"
// line. FromDomain is kept only for a human-readable cross-check against the sibling capability's
// own already-declared Domain (see validateDerivedCapabilities) -- domain-compatibility
// validation for "derived" is an explicitly open question (derived-capability-mechanism.md's
// "Domain-compatibility validation stays an open sub-question"), so a mismatch is not flagged
// here, only the sibling's mere presence/absence and cycle-freedom are.
type TDerivedCapabilityDeclaration struct {
	Domain     string // DDD -- the new capability's own domain
	Label      string // NNN -- the new capability's own label (map key)
	FromDomain string // EEE -- the sibling capability's domain, as written by the DSL author
	FromLabel  string // MMM -- the sibling capability's own label (map key)
	Template   string // TTT -- "$" stands for the sibling's own resolved value
}

// parseDerivedCapabilityLine attempts to parse line (already trimmed, comment-stripped) as a
// "derived ..." declaration. Returns ok=false for any other line shape, leaving the caller
// to fall through to its own integration-specific patterns.
func parseDerivedCapabilityLine(line string) (TDerivedCapabilityDeclaration, bool) {
	m := derivedCapabilityLinePattern.FindStringSubmatch(line)
	if m == nil {
		return TDerivedCapabilityDeclaration{}, false
	}
	return TDerivedCapabilityDeclaration{
		Domain: m[1], Label: m[2], FromDomain: m[3], FromLabel: m[4], Template: m[5],
	}, true
}

// substituteDerivedTemplate renders a "derived ... via TTT;" template against its own
// sibling capability's already-resolved Jinja expression, parenthesizing it as a sub-expression
// (derived-capability-mechanism.md: "K with $ replaced by T (parenthesized as a sub-expression)")
// -- unlike buildConditionStateExpr's own bare (non-parenthesizing) "$" substitution, "derived"
// composes recursively (a derived capability can itself be the base of a further derived one), so
// an unparenthesized substitution could silently change operator precedence a level down.
func substituteDerivedTemplate(template, baseExpr string) string {
	return strings.ReplaceAll(template, "$", "("+baseExpr+")")
}

// jinjaTemplateDefinitionPattern is Settings.def's "jinja ${name}(param1, param2, ...) =
// "template";" declaration (2026-09-15) -- a small, named, parameterized alternative to writing
// out a "derived ... via "...";" template's own text every time (e.g. "int_less_then"/
// "add_float_value" in Shared/Settings.def, reused across many devices' own battery_alert/pressure
// declarations). Deliberately NOT the full macro system (Macros.def/expander.go): no typed
// parameters, no entity generation, just named string-template substitution -- "derived"'s own
// "via" clause is the only consumer today.
var jinjaTemplateDefinitionPattern = regexp.MustCompile(`^jinja\s+\$\{([A-Za-z_][A-Za-z0-9_]*)\}\(([^)]*)\)\s*=\s*"([^"]*)"\s*;$`)

// derivedViaJinjaCallPattern matches a "derived DDD.NNN from EEE.MMM via jinja ${name}(args);"
// line -- group 1 is everything up to and including "via " (preserved verbatim), group 2 the
// jinja template's own name, group 3 its comma-separated argument list.
var derivedViaJinjaCallPattern = regexp.MustCompile(`^(derived\s+[A-Za-z_][A-Za-z0-9_]*\.[A-Za-z_][A-Za-z0-9_/]*\s+from\s+[A-Za-z_][A-Za-z0-9_]*\.[A-Za-z_][A-Za-z0-9_/]*\s+via\s+)jinja\s+\$\{([A-Za-z_][A-Za-z0-9_]*)\}\(([^)]*)\)\s*;\s*$`)

// TJinjaTemplateDefinition is one parsed "jinja ${name}(params) = "template";" declaration.
type TJinjaTemplateDefinition struct {
	Params   []string // parameter names, in declared order
	Template string   // "$" stands for the derived capability's own sibling value, "${param}" for each argument
}

// parseJinjaTemplateDefinitions scans content (Settings.def, shared+local combined) for every
// "jinja ${name}(params) = "template";" line. Lines that don't match are silently ignored --
// Settings.def already carries plenty of unrelated "${name} = "value";" assignments
// (parseDefinitionAssignments) this function has no business warning about.
func parseJinjaTemplateDefinitions(content string) map[string]TJinjaTemplateDefinition {
	defs := map[string]TJinjaTemplateDefinition{}
	for _, rawLine := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m := jinjaTemplateDefinitionPattern.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		var params []string
		for _, p := range strings.Split(m[2], ",") {
			if p = strings.TrimSpace(p); p != "" {
				params = append(params, p)
			}
		}
		defs[m[1]] = TJinjaTemplateDefinition{Params: params, Template: m[3]}
	}
	return defs
}

// loadJinjaTemplateDefinitions reads Shared/Settings.def and definitionDir's own Settings.def
// (same combining convention and relative shared-directory derivation as
// generator.go's parseAdministrationFromPaths) and parses every "jinja ${name}(params) =
// "template";" declaration found across both.
func loadJinjaTemplateDefinitions(definitionDir string) map[string]TJinjaTemplateDefinition {
	sharedDefinitionDir := filepath.Clean(filepath.Join(definitionDir, "..", "..", "Shared", "Definitions"))
	sharedBytes, _ := os.ReadFile(filepath.Join(sharedDefinitionDir, "Settings.def"))
	localBytes, _ := os.ReadFile(filepath.Join(definitionDir, "Settings.def"))
	return parseJinjaTemplateDefinitions(string(sharedBytes) + "\n" + string(localBytes))
}

// resolveJinjaTemplateCallsInDerivedLines rewrites every "derived ... via jinja ${name}(args);"
// line in content into the plain "derived ... via "<resolved>";" shape
// parseDerivedCapabilityLine already understands -- a one-line-for-one-line text rewrite (never
// changing the number of lines, so callers' own line-number bookkeeping, e.g.
// collectLayerContent's mergedLineNos, stays valid unchanged) applied once, before Physical.def's
// content ever reaches any "derived"-aware parser. A jinja call naming an undeclared template, or
// passing the wrong number of arguments, is reported as a warning and left unrewritten -- it then
// falls through to parseDerivedCapabilityLine's own pattern, which won't match an unresolved
// "via jinja ...;" shape either, surfacing as that parser's own "unrecognised line" warning too
// (redundant but harmless; better than silently dropping the capability).
func resolveJinjaTemplateCallsInDerivedLines(content string, jinjaTemplates map[string]TJinjaTemplateDefinition) (string, []string) {
	var warnings []string
	lines := strings.Split(content, "\n")
	for i, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		m := derivedViaJinjaCallPattern.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		prefix, name, argsText := m[1], m[2], m[3]
		def, found := jinjaTemplates[name]
		if !found {
			warnings = append(warnings, fmt.Sprintf("Physical.def: %q references undeclared \"jinja ${%s}(...)\" -- add a \"jinja ${%s}(...) = \"...\";\" line to Settings.def", line, name, name))
			continue
		}
		var args []string
		if strings.TrimSpace(argsText) != "" {
			for _, a := range strings.Split(argsText, ",") {
				args = append(args, strings.TrimSpace(a))
			}
		}
		if len(args) != len(def.Params) {
			warnings = append(warnings, fmt.Sprintf("Physical.def: %q calls \"jinja ${%s}(...)\" with %d argument(s), want %d", line, name, len(args), len(def.Params)))
			continue
		}
		resolved := def.Template
		for paramIdx, param := range def.Params {
			resolved = strings.ReplaceAll(resolved, "${"+param+"}", args[paramIdx])
		}
		lines[i] = prefix + "\"" + resolved + "\";"
	}
	return strings.Join(lines, "\n"), warnings
}

// derivedConditionOneLinerPattern matches "derived DDD.NNN with condition VVV;" -- Physical.def's
// own way of writing what has always been the plain "DDD.NNN VVV;" capability line (VVV in every
// real case so far being "<entity> is available"), reusing Logical.def's own
// "derived <domain>.<label> with: condition <expr>; end;" wording (2026-09-25, the user's explicit
// "to be consistent" request -- full-sweep scope, both houses). This is a PURE syntactic alias, not
// a new capability mechanism: at the Physical layer there is no boolean composition to do (AND/OR
// over several sources is Logical.def's own job, via "dependency on"/its own "derived ...
// condition ...;" block) -- a Physical.def capability is always exactly one raw value, so
// "condition" here can only ever wrap that one value verbatim.
var derivedConditionOneLinerPattern = regexp.MustCompile(`^derived\s+([A-Za-z_][A-Za-z0-9_]*\.[A-Za-z_][A-Za-z0-9_/]*)\s+with\s+condition\s+(.+?)\s*;\s*$`)

// resolveDerivedConditionOneLiners rewrites every "derived DDD.NNN with condition VVV;" line in
// content into the plain "DDD.NNN VVV;" shape every Physical.def capability parser already
// understands -- a one-line-for-one-line text rewrite (see resolveJinjaTemplateCallsInDerivedLines's
// own doc comment for why line count must never change), applied once before Physical.def's content
// ever reaches any integration-specific parser. Letting all ten independent Physical.def scanners
// (hosts/discovery/hassbridge/commandline/local/import/capability_defaults/defined.go's two) go on
// understanding only the single already-existing "DDD.NNN VVV;" shape, rather than teaching all ten
// a second one, is the whole point of doing this as a rewrite instead of a parser change -- see
// derivedConditionOneLinerPattern's own doc comment for why it's safe to always unwrap unconditionally
// (no ambiguity, no failure mode that needs a warning the way an undeclared jinja template name does).
func resolveDerivedConditionOneLiners(content string) string {
	lines := strings.Split(content, "\n")
	for i, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		m := derivedConditionOneLinerPattern.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		lines[i] = m[1] + " " + m[2] + ";"
	}
	return strings.Join(lines, "\n")
}
