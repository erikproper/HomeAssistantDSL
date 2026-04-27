/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Expander
 *
 * This component parses macro definitions, entity declarations, and spaces,
 * then expands macros with parameter type checking to generate a semantic report.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 21.03.2026
 *
 */

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type TParameterKind int

const (
	ParamEntity TParameterKind = iota
	ParamString
	ParamInt
	ParamEntityReference
	ParamBoolean
	ParamSetOfString
	ParamSetOfEntityReference
	ParamPath
	ParamOption
	ParamEntityName      // domain.sphere/path — full qualified entity name
	ParamEntityPath      // sphere/path — entity path including sphere, no domain
	ParamEntitySpacePath // path — entity path without domain or sphere
	ParamTime            // HH:MM:SS
)

type TMacroParameter struct {
	Name       string
	Kind       TParameterKind
	Optional   bool
	Positional bool // true = filled from positional call arg (invocation target), not from named with: params
	Default    string
}

type TParsedCreationMacro struct {
	Name       string
	Parameters []TMacroParameter // Note: $domain, $sphere, $entity are always available as implied parameters
	Body       []string
	SourceLine int
}

type TMacroExpansionContext struct {
	Macros   map[string]*TParsedCreationMacro
	Config   TExpanderConfig
	Settings map[string]string // bare variable names → values (from Settings.def), e.g. "wind_speed_min" → "0"
}

type TExpanderConfig struct {
	Verbose    bool
	CheckTypes bool
}

var variableReferencePattern = regexp.MustCompile(`\$\{[A-Za-z_][A-Za-z0-9_]*(?:\.[a-z_]+)?\}`)
var timePattern = regexp.MustCompile(`^\d{2}:\d{2}:\d{2}$`)
var ifProvidedPattern = regexp.MustCompile(`^if\s+(.*?)\s+is\s+(not\s+)?provided\s+then$`)
var forInDoPattern = regexp.MustCompile(`^for\s+(\$\{[^}]+\})\s+in\s+(.+?)\s+do$`)

// --- conditional and loop evaluation ---

// parseForValues splits a comma-separated list, optionally wrapped in { }, into trimmed values.
func parseForValues(listStr string) []string {
	listStr = strings.TrimSpace(listStr)
	listStr = strings.TrimPrefix(listStr, "{")
	listStr = strings.TrimSuffix(listStr, "}")
	listStr = strings.TrimSpace(listStr)
	parts := strings.Split(listStr, ",")
	values := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			values = append(values, v)
		}
	}
	return values
}

// collectOneStmt collects exactly one statement starting at lines[start] and returns
// the collected lines together with the index of the first line after the statement.
// Recognised statement forms:
//   - if … is [not] provided then <stmt> [else <stmt>]
//   - for … in … do <stmt>
//   - compound:  <header ending with "with:"> … end;
//   - simple:    any single line
func collectOneStmt(lines []string, start int) ([]string, int) {
	if start >= len(lines) {
		return nil, start
	}
	trimmed := strings.TrimSpace(lines[start])

	// if-block: collect the if-line, the then-stmt, and an optional else-stmt.
	if ifProvidedPattern.MatchString(trimmed) {
		collected := []string{lines[start]}
		thenLines, next := collectOneStmt(lines, start+1)
		collected = append(collected, thenLines...)
		if next < len(lines) && strings.TrimSpace(lines[next]) == "else" {
			collected = append(collected, lines[next])
			elseLines, afterElse := collectOneStmt(lines, next+1)
			collected = append(collected, elseLines...)
			next = afterElse
		}
		return collected, next
	}

	// for-block: collect the for-line and its single body statement.
	if forInDoPattern.MatchString(trimmed) {
		collected := []string{lines[start]}
		bodyLines, next := collectOneStmt(lines, start+1)
		return append(collected, bodyLines...), next
	}

	// compound statement: header ends with "with:", body ends at matching "end;".
	if strings.HasSuffix(trimmed, "with:") {
		collected := []string{lines[start]}
		i := start + 1
		depth := 1
		for i < len(lines) && depth > 0 {
			inner := strings.TrimSpace(lines[i])
			collected = append(collected, lines[i])
			i++
			if strings.HasSuffix(inner, "with:") {
				depth++
			} else if inner == "end;" {
				depth--
			}
		}
		return collected, i
	}

	// Simple statement: a single line.
	return []string{lines[start]}, start + 1
}

// processConditionals evaluates all "if … is [not] provided then" and "for … in … do"
// constructs in lines, returning only the lines that should be emitted.
// It operates recursively so that nested constructs are handled correctly.
func processConditionals(lines []string) []string {
	output := []string{}
	i := 0
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])

		if matches := ifProvidedPattern.FindStringSubmatch(trimmed); matches != nil {
			// A value still starting with ${ was not substituted (not provided).
			value := matches[1]
			negated := matches[2] != ""
			isProvided := !strings.HasPrefix(value, "${")
			emitting := isProvided != negated

			thenLines, next := collectOneStmt(lines, i+1)
			i = next

			var elseLines []string
			if i < len(lines) && strings.TrimSpace(lines[i]) == "else" {
				i++ // consume the "else" line
				elseLines, i = collectOneStmt(lines, i)
			}

			if emitting {
				output = append(output, processConditionals(thenLines)...)
			} else {
				output = append(output, processConditionals(elseLines)...)
			}

		} else if matches := forInDoPattern.FindStringSubmatch(trimmed); matches != nil {
			// Unroll the loop: substitute the loop variable for each value and recurse.
			loopVar := matches[1]
			values := parseForValues(matches[2])
			bodyLines, next := collectOneStmt(lines, i+1)
			i = next

			for _, val := range values {
				substituted := make([]string, len(bodyLines))
				for j, bl := range bodyLines {
					substituted[j] = strings.ReplaceAll(bl, loopVar, val)
				}
				output = append(output, processConditionals(substituted)...)
			}

		} else {
			output = append(output, lines[i])
			i++
		}
	}
	return output
}

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

// --- Parsing Macro Definitions ---

func (ctx *TMacroExpansionContext) ParseCreationMacro(lines []string, startIdx int) (*TParsedCreationMacro, int, error) {
	// macro <name> { params }:
	//   <body>
	// end;

	if startIdx >= len(lines) {
		return nil, startIdx, nil
	}

	line := strings.TrimSpace(lines[startIdx])

	// Recognise "macro" header.
	prefix := ""
	if strings.HasPrefix(line, "macro ") {
		prefix = "macro "
	} else {
		return nil, startIdx, nil
	}

	// Parse header: "macro <name> ( $p1 t1, ... ) { $opt1 t1, ... }:"
	macro := &TParsedCreationMacro{
		SourceLine: startIdx + 1,
		Parameters: []TMacroParameter{},
		Body:       []string{},
	}

	// Extract macro name and parameters
	headerLine := strings.TrimPrefix(line, prefix)
	if idx := strings.Index(headerLine, " "); idx > 0 {
		macro.Name = headerLine[:idx]
		paramsStr := headerLine[idx:]

		// Parse parameters from "{ ... }:" format
		if err := parseMacroParameters(paramsStr, macro); err != nil {
			return nil, startIdx, err
		}
	} else {
		macro.Name = strings.TrimSuffix(headerLine, ":")
	}

	// Collect body lines until macro-terminating end;.
	// A line "end;" terminates the macro only when the next non-empty, non-comment line
	// starts a new "macro ..." header (or there is no next line).
	idx := startIdx + 1
	for idx < len(lines) {
		bodyLine := strings.TrimSpace(lines[idx])

		if bodyLine == "end;" {
			nextIdx := idx + 1
			for nextIdx < len(lines) {
				nextLine := strings.TrimSpace(lines[nextIdx])
				if nextLine == "" || strings.HasPrefix(nextLine, "#") {
					nextIdx++
					continue
				}
				break
			}

			if nextIdx >= len(lines) {
				idx++
				break
			}
			nextLine := strings.TrimSpace(lines[nextIdx])
			if strings.HasPrefix(nextLine, "macro ") {
				idx++
				break
			}
		}

		if bodyLine != "" && !strings.HasPrefix(bodyLine, "#") {
			macro.Body = append(macro.Body, bodyLine)
		}

		idx++
	}

	// `power_switch` supports `icon ...;` in invocation blocks even when not explicitly
	// declared in the macro header, so treat `${icon}` as an implied optional string.
	if macro.Name == "power_switch" {
		hasIconParam := false
		for _, param := range macro.Parameters {
			if extractVariableName(param.Name) == "icon" {
				hasIconParam = true
				break
			}
		}
		if !hasIconParam {
			macro.Parameters = append(macro.Parameters, TMacroParameter{
				Name:     "${icon}",
				Kind:     ParamString,
				Optional: true,
			})
		}
	}

	return macro, idx, nil
}

// findBlockBrace finds the first '{' in s starting at offset that is NOT preceded by '$'
// (i.e. a block delimiter, not a variable reference like ${name}).
func findBlockBrace(s string, offset int) int {
	for i := offset; i < len(s); i++ {
		if s[i] == '{' && (i == 0 || s[i-1] != '$') {
			return i
		}
	}
	return -1
}

// findClosingBlockBrace finds the matching '}' that closes a block opened at offset,
// skipping over nested ${...} variable references.
func findClosingBlockBrace(s string, offset int) int {
	depth := 1
	for i := offset; i < len(s); i++ {
		if s[i] == '$' && i+1 < len(s) && s[i+1] == '{' {
			// Skip variable reference ${...}
			end := strings.Index(s[i+2:], "}")
			if end >= 0 {
				i = i + 2 + end
				continue
			}
		}
		if s[i] == '{' {
			depth++
		} else if s[i] == '}' {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func parseMacroParameters(paramsStr string, macro *TParsedCreationMacro) error {
	// New format: "( $pos1 type, $pos2 type ) { $opt1 type, $opt2 type }:"
	//   Positional params from ( ) are mandatory and filled from call-site positional args.
	//   Optional params from { } are always optional and filled from the with: named block.
	// Old format: "{ $p1 type op, $p2 type }:"
	//   All params in { }; optional only when marked with "op".

	// Parse positional params from ( ).
	openParen := strings.Index(paramsStr, "(")
	closeParen := strings.Index(paramsStr, ")")
	if openParen >= 0 && closeParen > openParen {
		posContent := paramsStr[openParen+1 : closeParen]
		for _, part := range strings.Split(posContent, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			param, err := parseParameter(part)
			if err != nil {
				return err
			}
			param.Positional = true
			param.Optional = false
			macro.Parameters = append(macro.Parameters, param)
		}
	}

	// Parse optional named params from { }.
	// Must skip ${...} variable references — look for '{' not preceded by '$'.
	openBrace := findBlockBrace(paramsStr, closeParen+1)
	closeBrace := -1
	if openBrace >= 0 {
		closeBrace = findClosingBlockBrace(paramsStr, openBrace+1)
	}
	if openBrace >= 0 && closeBrace > openBrace {
		optContent := paramsStr[openBrace+1 : closeBrace]
		for _, part := range strings.Split(optContent, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			param, err := parseParameter(part)
			if err != nil {
				return err
			}
			param.Positional = false
			param.Optional = true // { } params are always optional in both old and new format
			macro.Parameters = append(macro.Parameters, param)
		}
	}

	return nil
}

func parseParameter(paramStr string) (TMacroParameter, error) {
	// Format: "$name type" or "$name type op"
	parts := strings.Fields(paramStr)
	if len(parts) < 1 {
		return TMacroParameter{}, fmt.Errorf("empty parameter")
	}

	param := TMacroParameter{
		Name:     parts[0],
		Optional: false,
	}

	// Check for "op" (optional) marker at the end
	lastPart := parts[len(parts)-1]
	if lastPart == "op" {
		param.Optional = true
		parts = parts[:len(parts)-1]
	}

	// Parse type from remaining parts; multi-token forms such as
	// "set of string" and "set of entityReference" are supported.
	if len(parts) > 1 {
		param.Kind = parseParameterKind(strings.Join(parts[1:], " "))
	} else {
		param.Kind = ParamEntity // default
	}
	if param.Kind == ParamOption {
		param.Optional = true
	}

	return param, nil
}

func parseParameterKind(typeStr string) TParameterKind {
	normalized := strings.ToLower(strings.Join(strings.Fields(typeStr), " "))
	// Strip qualifiers like "context" that appear after the base type name.
	normalized = strings.TrimSuffix(strings.TrimSpace(normalized), " context")
	switch normalized {
	case "string":
		return ParamString
	case "int":
		return ParamInt
	case "entityreference":
		return ParamEntityReference
	case "boolean", "bool":
		return ParamBoolean
	case "path":
		return ParamPath
	case "option":
		return ParamOption
	case "set of entityreference":
		return ParamSetOfEntityReference
	case "set of string":
		return ParamSetOfString
	case "entity_name", "entityname":
		return ParamEntityName
	case "entity_path", "entitypath":
		return ParamEntityPath
	case "entity_space_path", "entityspacepath":
		return ParamEntitySpacePath
	case "time":
		return ParamTime
	default:
		if strings.HasPrefix(normalized, "set") {
			if strings.Contains(normalized, "entityreference") {
				return ParamSetOfEntityReference
			}
			return ParamSetOfString
		}
		return ParamEntity
	}
}

// --- Macro Expansion ---

type TMacroInvocation struct {
	Name             string
	Target           string
	ExtraPositionals []string // additional positional args beyond the first (Target)
	Parameters       map[string]string
}

func (ctx *TMacroExpansionContext) ParseMacroInvocation(line string) (*TMacroInvocation, error) {
	// Parse:
	// - "call macro_name target with param value;"
	// - "call macro_name target with:\n    key value;\n  end;"
	invocationLines := strings.Split(line, "\n")
	header := strings.TrimSpace(invocationLines[0])
	parts := strings.Fields(header)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid macro invocation")
	}

	// Handle optional "call" keyword prefix
	idx := 0
	if parts[0] == "call" {
		idx = 1
	}

	if idx >= len(parts)-1 {
		return nil, fmt.Errorf("invalid macro invocation")
	}

	invocation := &TMacroInvocation{
		Name:       parts[idx],
		Target:     strings.TrimSuffix(parts[idx+1], ";"),
		Parameters: make(map[string]string),
	}

	// Collect extra positional args: tokens after the target, before "with" or "with:".
	for i := idx + 2; i < len(parts); i++ {
		token := strings.TrimSuffix(parts[i], ";")
		if strings.ToLower(token) == "with" || strings.ToLower(token) == "with:" {
			break
		}
		invocation.ExtraPositionals = append(invocation.ExtraPositionals, token)
	}

	// Parse inline "with ..." options in the header line.
	if withIdx := strings.Index(header, " with "); withIdx >= 0 {
		withTail := strings.TrimSpace(header[withIdx+len(" with "):])
		if withTail != "" && withTail != ":" && withTail != "with:" {
			parseInvocationStatement(withTail, invocation.Parameters)
		}
	}

	// Parse multiline "with:" block options.
	for _, rawLine := range invocationLines[1:] {
		trimmed := strings.TrimSpace(rawLine)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if trimmed == "end;" || trimmed == "end" {
			break
		}
		parseInvocationStatement(trimmed, invocation.Parameters)
	}

	return invocation, nil
}

func parseInvocationStatement(statement string, parameters map[string]string) {
	clean := strings.TrimSpace(statement)
	clean = strings.TrimSuffix(clean, ";")
	clean = strings.TrimSpace(clean)
	if clean == "" {
		return
	}

	fields := strings.Fields(clean)
	if len(fields) == 0 {
		return
	}

	key := strings.ToLower(fields[0])
	if len(fields) == 1 {
		parameters[key] = "true"
		return
	}

	parameters[key] = strings.Join(fields[1:], " ")
}

func normalizeParameterKey(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(extractVariableName(name)))
	normalized = strings.ReplaceAll(normalized, "_", "")
	normalized = strings.ReplaceAll(normalized, "-", "")
	return normalized
}

func canonicalInvocationParameters(parameters map[string]string) map[string]string {
	canonical := make(map[string]string, len(parameters))
	for key, value := range parameters {
		canonical[normalizeParameterKey(key)] = value
	}
	return canonical
}

func (ctx *TMacroExpansionContext) ExpandMacro(invocation *TMacroInvocation, spacePath []string) ([]string, []string, error) {
	// Callers validate at the top level; ExpandMacro does not self-validate so that
	// nested macro calls from macro bodies are not held to the same strict rules.
	macro, exists := ctx.Macros[invocation.Name]
	if !exists {
		return nil, nil, fmt.Errorf("unknown macro: %s", invocation.Name)
	}

	canonicalParameters := canonicalInvocationParameters(invocation.Parameters)

	// --- Implied parameters: always inject ${domain}, ${sphere}, ${entity} ---
	// ${domain}: Home Assistant domain (e.g., "switch", "sensor")
	// ${sphere}: sphere (e.g., "social", "physical")
	// ${entity}: full entity path (space path + subdomain)
	// These are always available in macro bodies, even if not declared in the header.
	substitutions := make(map[string]string)
	// Parse domain, sphere, and entity from the invocation target using the provided spacePath
	domain, sphere, entityPath := parseDomainSphereEntity(invocation.Target, spacePath)
	substitutions["${domain}"] = domain
	substitutions["${sphere}"] = sphere
	substitutions["${entity}"] = entityPath

	// Map positional params from the invocation target (first positional arg), applying
	// implicit type casting where allowed: entity_name → entity_path/entity_space_path,
	// entity_path → entity_space_path.
	// A ':'-prefixed entity_space_path argument is space-relative: castParamValue strips the ':'
	// prefix, leaving a bare relative path. normalizeEntityFullName later prepends the context
	// (spacePath) when the path appears in entity declarations in the expanded body.
	positionalArgs := append([]string{invocation.Target}, invocation.ExtraPositionals...)
	posArgIdx := 0
	for _, param := range macro.Parameters {
		if param.Positional && posArgIdx < len(positionalArgs) {
			rawArg := positionalArgs[posArgIdx]
			castedArg := resolveParamValue(rawArg, param.Kind, spacePath)
			substitutions[param.Name] = castedArg
			posArgIdx++
		}
	}

	// Add explicit named parameter substitutions, allowing snake_case to match camelCase.
	// Apply implicit type casting (e.g. entity_name → entity_space_path) when assigning.
	for _, param := range macro.Parameters {
		if param.Positional {
			continue
		}
		paramKey := normalizeParameterKey(param.Name)
		if value, provided := canonicalParameters[paramKey]; provided {
			substitutions[param.Name] = resolveParamValue(value, param.Kind, spacePath)
			continue
		}
		snakeKey := toSnakeCase(paramKey)
		if value, provided := invocation.Parameters[snakeKey]; provided {
			substitutions[param.Name] = resolveParamValue(value, param.Kind, spacePath)
		}
	}

	// Inject settings variables so macro bodies can reference e.g. ${wind_speed_min}.
	for name, value := range ctx.Settings {
		key := "${" + name + "}"
		if _, already := substitutions[key]; !already {
			substitutions[key] = value
		}
	}

	// Pre-populate ${name.accessor} variants for all base substitution entries.
	// With ${...} delimiters, ${entity} and ${entity.domain} are unambiguous — no length-sort needed.
	baseKeys := make([]string, 0, len(substitutions))
	for k := range substitutions {
		baseKeys = append(baseKeys, k)
	}
	for _, key := range baseKeys {
		if !strings.HasPrefix(key, "${") || !strings.HasSuffix(key, "}") {
			continue
		}
		bareName := key[2 : len(key)-1]
		if strings.Contains(bareName, ".") {
			continue // already an accessor key
		}
		value := substitutions[key]
		// Normalise colon-form entity specs (e.g. "light.social:main") to their fully
		// qualified slash form before computing path-based accessors, so that accessors
		// like ${entity.path} and ${entity.space_path} reflect the full context path
		// rather than just the bare local name after the colon.
		normalizedForAccessors := normalizeEntityFullName(value, spacePath)
		for _, accessor := range []string{"domain", "sphere", "path", "space_path", "sub_domain"} {
			accessorKey := "${" + bareName + "." + accessor + "}"
			if _, exists := substitutions[accessorKey]; !exists {
				substitutions[accessorKey] = extractEntityAccessor(normalizedForAccessors, accessor)
			}
		}
	}

	// Substitute all placeholders in the body first, then evaluate conditionals.
	substituted := make([]string, 0, len(macro.Body))
	for _, bodyLine := range macro.Body {
		expandedLine := bodyLine
		for placeholder, value := range substitutions {
			expandedLine = strings.ReplaceAll(expandedLine, placeholder, value)
		}
		// Handle "maybe <param>;" — emit the directive only if the named parameter was provided.
		// "maybe X;" with ${X}="val" → "X val;"; with ${X}=true → "X;"; with ${X}="" → dropped.
		trimmedLine := strings.TrimSpace(expandedLine)
		if strings.HasPrefix(trimmedLine, "maybe ") && strings.HasSuffix(trimmedLine, ";") {
			paramName := strings.TrimSuffix(strings.TrimPrefix(trimmedLine, "maybe "), ";")
			paramName = strings.TrimSpace(paramName)
			value := substitutions["${"+paramName+"}"]
			switch {
			case value == "" || strings.EqualFold(value, "false"):
				expandedLine = "" // drop
			case strings.EqualFold(value, "true"):
				expandedLine = paramName + ";"
			default:
				expandedLine = paramName + " " + value + ";"
			}
		}
		substituted = append(substituted, expandedLine)
	}

	return resolveLocalVarAssignments(processConditionals(substituted), spacePath), nil, nil
}

// resolveLocalVarAssignments does a sequential second pass over expanded macro body lines to
// substitute macro-local variable assignments of the form "${var} = entity <spec> ...".
// Parameters and settings are already resolved at this point; local vars are needed so that
// subsequent references like "media_switch ${media_player}" see the fully-qualified entity name.
func resolveLocalVarAssignments(lines []string, spacePath []string) []string {
	localVars := map[string]string{}
	result := make([]string, len(lines))
	for i, line := range lines {
		for k, v := range localVars {
			line = strings.ReplaceAll(line, k, v)
		}
		result[i] = line
		trimmed := strings.TrimSpace(line)
		if eqIdx := strings.Index(trimmed, " = entity "); eqIdx > 0 {
			varName := strings.TrimSpace(trimmed[:eqIdx])
			if strings.HasPrefix(varName, "${") && strings.HasSuffix(varName, "}") {
				rest := strings.TrimSpace(trimmed[eqIdx+len(" = entity "):])
				entitySpec := rest
				if idx := strings.Index(rest, " with:"); idx >= 0 {
					entitySpec = rest[:idx]
				} else if idx := strings.Index(rest, " with "); idx >= 0 {
					entitySpec = rest[:idx]
				}
				entitySpec = strings.TrimSuffix(strings.TrimSpace(entitySpec), ";")
				if entitySpec != "" && !strings.Contains(entitySpec, "${") {
					localVars[varName] = normalizeEntityFullName(entitySpec, spacePath)
				}
			}
		}
	}
	return result
}

// parseEntityComponents splits an entity spec into domain, sphere, and path.
// Handles both slash form "domain.sphere/path" and colon form "domain.sphere:path".
// For "light.:main" the sphere is empty and path is "main".
func parseEntityComponents(entityValue string) (domain, sphere, path string) {
	path = entityValue
	dotIdx := strings.Index(entityValue, ".")
	if dotIdx <= 0 {
		// No domain. Detect sphere/path or sphere:path form (e.g. "social/apartment/kitchen").
		if sepIdx := strings.IndexAny(entityValue, ":/"); sepIdx > 0 {
			if isKnownSphereName(entityValue[:sepIdx]) {
				sphere = entityValue[:sepIdx]
				path = entityValue[sepIdx+1:]
			}
		}
		return
	}
	domain = entityValue[:dotIdx]
	rest := entityValue[dotIdx+1:]
	sepIdx := strings.IndexAny(rest, ":/")
	if sepIdx < 0 {
		sphere = rest
		path = ""
		return
	}
	sphere = rest[:sepIdx]
	path = rest[sepIdx+1:]
	return
}

// extractEntityAccessor returns the requested component of an entity spec.
// Supported accessors: domain, sphere, path, space_path, sub_domain.
//
// For an entity_name value like "light.social/apartment/living_room/vidja/left":
//   domain      = "light"
//   sphere      = "social"
//   path        = "social/apartment/living_room/vidja/left"  (entire path behind the dot)
//   space_path  = "apartment/living_room/vidja/left"         (path behind sphere/)
//   sub_domain  = "left" only if "left" is a known sub-domain keyword, else ""
//
// When sphere is empty (e.g. "light.:main"), path is returned with a leading ":"
// so that callers using it as an entity_space_path argument preserve the
// space-relative semantics (":main" → expanded to "<currentSpace>/main" by ExpandMacro).
func extractEntityAccessor(entityValue, accessor string) string {
	domain, sphere, path := parseEntityComponents(entityValue)
	switch accessor {
	case "domain":
		return domain
	case "sphere":
		return sphere
	case "path":
		// Entire path behind the dot: sphere + "/" + path_after_sphere.
		if sphere == "" && path != "" {
			return ":" + path // preserve space-relative marker
		}
		if sphere != "" {
			return sphere + "/" + path
		}
		return path
	case "space_path":
		// Entire path behind the sphere prefix (i.e. path_after_sphere, without dropping the last component).
		// This is what ${x.path} used to return; ${x.path} now includes the sphere.
		if sphere == "" && path != "" {
			return ":" + path // preserve space-relative marker for sphere-less specs
		}
		return path
	case "sub_domain":
		// Last slash-separated component, but only when it is a recognised sub-domain keyword.
		// Location/device names (e.g. "main", "left", "kitchen") are not sub-domains.
		var last string
		if idx := strings.LastIndex(path, "/"); idx >= 0 {
			last = path[idx+1:]
		} else {
			last = path
		}
		if isKnownSubDomain(last) {
			return last
		}
		return ""
	}
	return entityValue
}

// isKnownSubDomain reports whether name is a recognised terminal attribute keyword
// that identifies WHAT is measured or controlled, as opposed to a location/device name.
func isKnownSubDomain(name string) bool {
	switch name {
	case "battery_level", "battery_alert", "node", "status", "radio", "load",
		"temperature", "humidity", "co2", "pressure", "illuminance", "noise",
		"wind_speed", "wind_direction", "motion", "water", "door", "power",
		"consumes", "media", "space":
		return true
	}
	return false
}

// resolveParamValue extends castParamValue with space-context resolution for entity_space_path.
// A ':'-prefixed value is space-relative: the ':' is replaced by the normalised space context
// path (sphere excluded), so that ${entity_space_path} always holds the fully-qualified path.
func resolveParamValue(value string, targetKind TParameterKind, spacePath []string) string {
	if targetKind == ParamEntitySpacePath && strings.HasPrefix(value, ":") {
		rel := strings.TrimPrefix(value, ":")
		_, contextPath := normalizeSpaceContext(spacePath)
		if contextPath == "" {
			return rel
		}
		if rel == "" {
			return contextPath
		}
		return contextPath + "/" + rel
	}
	return castParamValue(value, targetKind)
}

// castParamValue applies implicit type casting so wider types can satisfy narrower param types:
//
//	entity_name  → entity_path:       strip "domain." prefix
//	entity_name  → entity_space_path: strip "domain.sphere/" prefix
//	entity_path  → entity_space_path: strip "sphere/" prefix
//
// Values that already match the target kind are returned unchanged.
// Space-relative ':'-prefixed entity_space_path values are handled by resolveParamValue.
func castParamValue(value string, targetKind TParameterKind) string {
	switch targetKind {
	case ParamEntityPath:
		// entity_name (has '.') → entity_path: drop "domain." prefix
		if dotIdx := strings.Index(value, "."); dotIdx > 0 {
			return value[dotIdx+1:]
		}
	case ParamEntitySpacePath:
		// entity_name (has '.') → entity_space_path: drop "domain.sphere/" prefix
		if dotIdx := strings.Index(value, "."); dotIdx > 0 {
			afterDot := value[dotIdx+1:]
			if slashIdx := strings.Index(afterDot, "/"); slashIdx > 0 {
				return afterDot[slashIdx+1:]
			}
			return afterDot
		}
		// An absolute entity_space_path written with a leading '/' (e.g. "/house/server_room/zwave/027")
		// is already fully resolved — just strip the leading slash.
		if strings.HasPrefix(value, "/") {
			return strings.TrimPrefix(value, "/")
		}
		// entity_path (has '/') → entity_space_path: drop "sphere/" prefix, but only when
		// the first segment is a known sphere name. An already-resolved entity_space_path
		// (e.g. "apartment/living_room/device") must not have its first segment stripped.
		if slashIdx := strings.Index(value, "/"); slashIdx > 0 {
			if isKnownSphereName(value[:slashIdx]) {
				return value[slashIdx+1:]
			}
		}
	}
	return value
}

// isKnownSphereName reports whether name is one of the recognised sphere identifiers.
func isKnownSphereName(name string) bool {
	switch name {
	case "social", "physical", "infrastructural":
		return true
	}
	return false
}

// isAbsoluteSpaceName reports whether spaceName is an absolute-path space declaration
// that should replace the current space context rather than nest inside it.
// An absolute-path space has no ':' separator but starts with a known sphere name
// followed by '/', e.g. "physical/apartment/living_room".
func isAbsoluteSpaceName(spaceName string) bool {
	if strings.Contains(spaceName, ":") {
		return false
	}
	slashIdx := strings.Index(spaceName, "/")
	if slashIdx <= 0 {
		return false
	}
	return isKnownSphereName(spaceName[:slashIdx])
}

// Helper: convert normalized key to snake_case
func toSnakeCase(s string) string {
	var out []rune
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			out = append(out, '_', r+('a'-'A'))
		} else {
			out = append(out, r)
		}
	}
	return string(out)
}

// --- Utility: parseDomainSphereEntity ---
// Splits an entity spec like "switch.social:nespresso" into domain, sphere, entityPath
func parseDomainSphereEntity(entitySpec string, spacePath []string) (domain, sphere, entityPath string) {
	// entitySpec: "domain.sphere:path" or "domain.sphere:space/subdomain"
	domain, sphere, entityPath = "", "", entitySpec
	dotIdx := strings.Index(entitySpec, ".")
	colonIdx := strings.Index(entitySpec, ":")
	if dotIdx > 0 && colonIdx > dotIdx {
		domain = entitySpec[:dotIdx]
		sphere = entitySpec[dotIdx+1 : colonIdx]
		entityPath = entitySpec[colonIdx+1:]
	} else if dotIdx > 0 {
		domain = entitySpec[:dotIdx]
		rest := entitySpec[dotIdx+1:]
		if colonIdx := strings.Index(rest, ":"); colonIdx > 0 {
			sphere = rest[:colonIdx]
			entityPath = rest[colonIdx+1:]
		} else {
			sphere = rest
			entityPath = ""
		}
	}
	// Generic normalization: if entityPath is non-empty and does not contain a slash, and context is available, expand to <space>/<entityPath>
	if entityPath != "" && !strings.Contains(entityPath, "/") && len(spacePath) > 0 {
		entityPath = strings.Join(spacePath, "/") + "/" + entityPath
	}
	return
}

func extractSphere(entitySpec string) string {
	// From "type.sphere:path" extract "sphere"
	// Handle the case where entity spec might not have all components
	if idx := strings.Index(entitySpec, "."); idx > 0 {
		afterDot := entitySpec[idx+1:]
		if idx2 := strings.Index(afterDot, ":"); idx2 > 0 {
			return afterDot[:idx2]
		}
		return afterDot
	}
	return ""
}

func (ctx *TMacroExpansionContext) ValidateInvocationParameters(invocation *TMacroInvocation, macro *TParsedCreationMacro) []string {
	errors := []string{}
	knownParameters := map[string]bool{}
	canonicalParameters := canonicalInvocationParameters(invocation.Parameters)
	for _, param := range macro.Parameters {
		knownParameters[normalizeParameterKey(param.Name)] = true
	}

	for invocationParameter := range invocation.Parameters {
		if !knownParameters[normalizeParameterKey(invocationParameter)] {
			errors = append(errors, fmt.Sprintf("unknown parameter %s for macro %s", invocationParameter, invocation.Name))
		}
	}

	// Check required parameters; positional params are matched in order from the call-site args.
	allPositionals := append([]string{invocation.Target}, invocation.ExtraPositionals...)
	posArgIdx := 0
	for _, param := range macro.Parameters {
		if param.Positional {
			if ctx.Config.CheckTypes && posArgIdx < len(allPositionals) {
				rawArg := allPositionals[posArgIdx]
				castedValue := castParamValue(rawArg, param.Kind)
				if typeErr := validateParameterType(castedValue, param.Kind); typeErr != "" {
					errors = append(errors, fmt.Sprintf("positional parameter %s: %s", param.Name, typeErr))
				}
			}
			posArgIdx++
			continue
		}

		paramKey := normalizeParameterKey(param.Name)
		value, provided := canonicalParameters[paramKey]

		if !provided && !param.Optional {
			errors = append(errors, fmt.Sprintf("missing required parameter %s for macro %s", param.Name, invocation.Name))
			continue
		}

		if provided && ctx.Config.CheckTypes {
			// Validate parameter type
			typeErr := validateParameterType(value, param.Kind)
			if typeErr != "" {
				errors = append(errors, fmt.Sprintf("parameter %s type mismatch: %s (expected %s)", param.Name, typeErr, paramKindString(param.Kind)))
			}
		}
	}

	return errors
}

func (ctx *TMacroExpansionContext) ValidateInvocationStrict(invocation *TMacroInvocation) error {
	macro, exists := ctx.Macros[invocation.Name]
	if !exists {
		return fmt.Errorf("unknown macro: %s", invocation.Name)
	}

	validationErrors := ctx.ValidateInvocationParameters(invocation, macro)
	if len(validationErrors) > 0 {
		return fmt.Errorf(strings.Join(validationErrors, "; "))
	}

	return nil
}

func validateParameterType(value string, expectedKind TParameterKind) string {
	switch expectedKind {
	case ParamBoolean:
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized != "true" && normalized != "false" {
			return fmt.Sprintf("value %q is not a valid boolean", value)
		}
	case ParamPath:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return "empty path"
		}
		if !strings.ContainsAny(trimmed, "/:") {
			return fmt.Sprintf("value %q does not look like a path", value)
		}
	case ParamOption:
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized != "true" && normalized != "false" {
			return fmt.Sprintf("value %q is not a valid option flag", value)
		}
	case ParamInt:
		if _, err := parseIntLenient(value); err != nil {
			return fmt.Sprintf("value %q is not a valid integer", value)
		}
	case ParamEntityReference:
		if value == "" {
			return "empty entity reference"
		}
		if !isValidEntityReference(value) {
			return fmt.Sprintf("value %q has invalid entity reference syntax", value)
		}
	case ParamSetOfString:
		values := parseSetValues(value)
		if len(values) == 0 {
			return fmt.Sprintf("value %q is not a valid non-empty set", value)
		}
	case ParamSetOfEntityReference:
		values := parseSetValues(value)
		if len(values) == 0 {
			return fmt.Sprintf("value %q is not a valid non-empty set", value)
		}
		for _, setValue := range values {
			if !isValidEntityReference(setValue) {
				return fmt.Sprintf("set member %q is not a valid entity reference", setValue)
			}
		}
	case ParamEntityName:
		// Must have domain.sphere/path or domain.sphere:path — contains '.' and '/' or ':'.
		if !strings.Contains(value, ".") || (!strings.Contains(value, "/") && !strings.Contains(value, ":")) {
			return fmt.Sprintf("value %q does not look like an entity_name (expected domain.sphere/path or domain.sphere:path)", value)
		}
	case ParamEntityPath:
		// Must have sphere/path or sphere:path — contains '/' or ':' but no leading domain '.'.
		if !strings.Contains(value, "/") && !strings.Contains(value, ":") {
			return fmt.Sprintf("value %q does not look like an entity_path (expected sphere/path or sphere:path)", value)
		}
	case ParamEntitySpacePath:
		// A path without domain or sphere; any non-empty identifier or slash-separated path is valid.
		if strings.TrimSpace(value) == "" {
			return "empty entity_space_path"
		}
	case ParamTime:
		if !timePattern.MatchString(strings.TrimSpace(value)) {
			return fmt.Sprintf("value %q does not look like a time (expected HH:MM:SS)", value)
		}
	}
	return ""
}

func parseSetValues(value string) []string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.TrimPrefix(trimmed, "{")
	trimmed = strings.TrimSuffix(trimmed, "}")
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return nil
	}
	rawItems := strings.Split(trimmed, ",")
	values := []string{}
	for _, item := range rawItems {
		normalized := strings.TrimSpace(item)
		if normalized != "" {
			values = append(values, normalized)
		}
	}
	return values
}

func parseIntLenient(s string) (int64, error) {
	s = strings.TrimSpace(s)
	var result int64
	_, err := fmt.Sscanf(s, "%d", &result)
	return result, err
}

func isValidEntityReference(ref string) bool {
	// Basic check: should contain . or / or :, typical entity reference patterns
	return strings.ContainsAny(ref, ".:/") || strings.Contains(ref, "$")
}

// --- Expanded Output Generation ---

func (ctx *TMacroExpansionContext) GenerateExpansionReport() string {
	var report strings.Builder

	report.WriteString("=== MACRO DEFINITIONS ===\n\n")
	for name, macro := range ctx.Macros {
		report.WriteString(fmt.Sprintf("macro %s { ", name))
		for i, param := range macro.Parameters {
			if i > 0 {
				report.WriteString(", ")
			}
			report.WriteString(fmt.Sprintf("%s %s", param.Name, paramKindString(param.Kind)))
			if param.Optional {
				report.WriteString(" op")
			}
		}
		report.WriteString(" }:\n")
		report.WriteString(fmt.Sprintf("  SourceLine: %d\n", macro.SourceLine))
		report.WriteString(fmt.Sprintf("  Body lines: %d\n\n", len(macro.Body)))
	}

	return report.String()
}

func paramKindString(kind TParameterKind) string {
	switch kind {
	case ParamString:
		return "string"
	case ParamInt:
		return "int"
	case ParamEntityReference:
		return "entityReference"
	case ParamBoolean:
		return "boolean"
	case ParamPath:
		return "path"
	case ParamOption:
		return "option"
	case ParamSetOfString:
		return "set of string"
	case ParamSetOfEntityReference:
		return "set of entityReference"
	case ParamEntityName:
		return "entity_name"
	case ParamEntityPath:
		return "entity_path"
	case ParamEntitySpacePath:
		return "entity_space_path"
	case ParamTime:
		return "time"
	default:
		return "entity"
	}
}

func (ctx *TMacroExpansionContext) MacroDefinitionWarnings(macro *TParsedCreationMacro) []string {
	warnings := []string{}
	parameterKindByName := map[string]TParameterKind{}
	loopVariableNames := map[string]bool{}
	for _, parameter := range macro.Parameters {
		parameterName := extractVariableName(parameter.Name)
		parameterKindByName[parameterName] = parameter.Kind
	}

	for _, bodyLine := range macro.Body {
		trimmedLine := strings.TrimSpace(bodyLine)
		if strings.HasPrefix(trimmedLine, "for ") {
			parts := strings.Fields(trimmedLine)
			if len(parts) >= 2 && strings.HasPrefix(parts[1], "${") {
				loopVariableNames[extractVariableName(parts[1])] = true
			}
		}
	}

	for bodyLineIndex, bodyLine := range macro.Body {
		// Relative-path arguments (tokens starting with ":") must not appear in macro bodies.
		// The ":" prefix is a call-site convention meaning "relative to the current space";
		// inside a macro definition all paths must be absolute or parameter-based.
		trimmedBodyLine := strings.TrimSpace(bodyLine)
		bodyParts := strings.Fields(trimmedBodyLine)
		switch {
		case len(bodyParts) >= 2 && bodyParts[0] == "entity":
			// entity <name-or-path> ...
			tok := strings.TrimSuffix(bodyParts[1], ";")
			if strings.HasPrefix(tok, ":") {
				warnings = append(warnings,
					fmt.Sprintf("line %d: relative entity path %q (starting with ':') is not allowed in a macro body; resolve the space path at the call site instead",
						bodyLineIndex+1, tok),
				)
			}

			// entity X with <macro_name> — macro names are not built-in keywords; use "call".
			// Find the "with" token (not "with:") and check the token that follows it.
			for i, part := range bodyParts {
				if strings.ToLower(strings.TrimSuffix(part, ";")) == "with" && i+1 < len(bodyParts) {
					candidate := strings.ToLower(strings.TrimSuffix(bodyParts[i+1], ";"))
					if _, isMacro := ctx.Macros[candidate]; isMacro {
						warnings = append(warnings,
							fmt.Sprintf("line %d: %q after 'with' is a macro, not a built-in keyword; use 'with call %s ...' instead",
								bodyLineIndex+1, bodyParts[i+1], bodyParts[i+1]),
						)
					}
					break
				}
			}

		case len(bodyParts) >= 3 && bodyParts[0] == "call":
			// call <macro> <arg1> <arg2> ...  — check each positional argument
			for _, arg := range bodyParts[2:] {
				tok := strings.TrimSuffix(arg, ";")
				if tok == "with" || tok == "with:" {
					break
				}
				if strings.HasPrefix(tok, ":") {
					warnings = append(warnings,
						fmt.Sprintf("line %d: relative path argument %q (starting with ':') is not allowed in a macro body; resolve the space path at the call site instead",
							bodyLineIndex+1, tok),
					)
				}
			}
		}

		specificationToken, hasSpecification := entitySpecificationToken(bodyLine)
		if !hasSpecification || specificationToken == "" {
			continue
		}

		// A second ':' in an entity spec is legacy sub-domain notation; '/' must be used instead.
		if hasSecondColonSeparator(specificationToken) {
			warnings = append(warnings,
				fmt.Sprintf("line %d: entity specification %q uses a second ':' sub-domain separator (legacy); use '/' instead",
					bodyLineIndex+1, specificationToken),
			)
		}

		for _, variableReference := range variableReferencePattern.FindAllString(specificationToken, -1) {
			variableName := extractVariableName(variableReference)
			if variableName == "entity" || variableName == "sphere" {
				continue
			}

			parameterKind, exists := parameterKindByName[variableName]
			if !exists {
				if loopVariableNames[variableName] {
					continue
				}
				warnings = append(warnings,
					fmt.Sprintf("line %d: variable %s is used in entity specification %q but is not a declared macro parameter",
						bodyLineIndex+1,
						variableReference,
						specificationToken,
					),
				)
				continue
			}

			if parameterKind != ParamString {
				warnings = append(warnings,
					fmt.Sprintf("line %d: variable %s is used in entity specification %q and should be typed as string (currently %s)",
						bodyLineIndex+1,
						variableReference,
						specificationToken,
						paramKindString(parameterKind),
					),
				)
			}
		}
	}

	return warnings
}

func entitySpecificationToken(bodyLine string) (string, bool) {
	trimmedLine := strings.TrimSpace(bodyLine)
	parts := strings.Fields(trimmedLine)
	if len(parts) < 2 {
		return "", false
	}

	if parts[0] == "entity" {
		return parts[1], true
	}

	return "", false
}

// --- expansion runner and entity/space collection helpers (moved from main.go) ---

func runExpansion(root string, houses []string) error {
	for _, house := range houses {
		if err := expandHouse(root, house); err != nil {
			return fmt.Errorf("error expanding %s: %w", house, err)
		}
		fmt.Printf("expanded %s\n", house)
	}
	return nil
}

// loadMacroContext reads and parses the shared Macros.def, returning a ready-to-use expansion context.
func loadMacroContext(sharedDefinitionDir string) (*TMacroExpansionContext, error) {
	macrosPath := filepath.Join(sharedDefinitionDir, "Macros.def")
	macrosContent, err := os.ReadFile(macrosPath)
	if err != nil {
		return nil, fmt.Errorf("error reading macros: %w", err)
	}

	macroLines := strings.Split(string(macrosContent), "\n")
	ctx := &TMacroExpansionContext{
		Macros: make(map[string]*TParsedCreationMacro),
		Config: TExpanderConfig{Verbose: false, CheckTypes: true},
	}

	idx := 0
	for idx < len(macroLines) {
		macro, nextIdx, err := ctx.ParseCreationMacro(macroLines, idx)
		if err != nil {
			return nil, fmt.Errorf("error parsing macro at line %d: %w", idx+1, err)
		}
		if macro != nil {
			ctx.Macros[macro.Name] = macro
		}
		if nextIdx == idx {
			idx++
		} else {
			idx = nextIdx
		}
	}

	if err := validateMacroDefinitionOrdering(ctx); err != nil {
		return nil, fmt.Errorf("strict macro validation failed in %s: %w", macrosPath, err)
	}
	return ctx, nil
}

func expandHouse(root string, house string) error {
	definitionDir := filepath.Join(root, "New", house, "Definitions")
	sharedDefinitionDir := filepath.Join(root, "New", "Shared", "Definitions")

	ctx, err := loadMacroContext(sharedDefinitionDir)
	if err != nil {
		return err
	}

	entitiesPath := filepath.Join(definitionDir, "Entities.def")
	entitiesContent, err := os.ReadFile(entitiesPath)
	if err != nil {
		return fmt.Errorf("error reading entities: %w", err)
	}

	output := strings.Builder{}

	output.WriteString("=== MACRO EXPANSION REPORT ===\n")
	output.WriteString(fmt.Sprintf("House: %s\n", house))
	output.WriteString("Generated: 20.03.2026\n\n")

	output.WriteString("=== MACRO DEFINITIONS ===\n\n")
	for name := range ctx.Macros {
		macro := ctx.Macros[name]
		output.WriteString(fmt.Sprintf("macro %s { ", name))
		for i, param := range macro.Parameters {
			if i > 0 {
				output.WriteString(", ")
			}
			output.WriteString(fmt.Sprintf("%s %s", param.Name, paramKindString(param.Kind)))
			if param.Optional {
				output.WriteString(" op")
			}
		}
		output.WriteString(fmt.Sprintf(" }:\n  SourceLine: %d, Body lines: %d\n", macro.SourceLine, len(macro.Body)))

		definitionWarnings := ctx.MacroDefinitionWarnings(macro)
		if len(definitionWarnings) > 0 {
			output.WriteString("  Definition Warnings:\n")
			for _, warning := range definitionWarnings {
				output.WriteString(fmt.Sprintf("    - %s\n", warning))
			}
		}
		output.WriteString("\n")
	}

	output.WriteString("\n=== MACRO INVOCATIONS IN ENTITIES.DEF ===\n\n")
	parseResult, err := ParseEntitiesAndFillAdministration(strings.Split(string(entitiesContent), "\n"), entitiesPath, ctx, &output)
	if err != nil {
		return err
	}
	admin := parseResult.Administration
	invocationCount := parseResult.InvocationCount
	validInvocations := parseResult.ValidInvocations
	typeErrors := parseResult.TypeErrors

	output.WriteString("\n=== ENTITIES BY SPACE (FULL NAMES) ===\n\n")
	entityCount := 0
	for _, spaceName := range admin.SpaceOrder {
		indent := strings.Repeat("  ", admin.SpaceDepthByName[spaceName])
		output.WriteString(fmt.Sprintf("%s%s %q:\n", indent, formatSpaceLabel(admin.SpaceKindByName[spaceName]), spaceName))
		for _, entityLine := range admin.EntitiesBySpace[spaceName] {
			entityCount++
			output.WriteString(fmt.Sprintf("%s  - %s\n", indent, entityLine))
		}
		output.WriteString("\n")
	}

	output.WriteString("\n=== EXTERNAL ENTITIES (NO DEFINITION/IMPORTED) ===\n")
	output.WriteString("Assumption: these entities are already defined in Home Assistant; no core entity YAML generation needed here.\n")
	output.WriteString("Note: entities with local options (e.g., icon/providing) may still require configuration/customization YAML.\n")
	output.WriteString("Defined check status: not checked (offline mode).\n\n")
	externalCount := 0
	externalWithConfigCount := 0
	for _, spaceName := range admin.SpaceOrder {
		externalEntities := admin.ExternalEntitiesBySpace[spaceName]
		if len(externalEntities) == 0 {
			continue
		}

		indent := strings.Repeat("  ", admin.SpaceDepthByName[spaceName])
		output.WriteString(fmt.Sprintf("%s%s %q:\n", indent, formatSpaceLabel(admin.SpaceKindByName[spaceName]), spaceName))
		for _, item := range externalEntities {
			externalCount++
			if strings.Contains(item, "[config options:") {
				externalWithConfigCount++
			}
			output.WriteString(fmt.Sprintf("%s  - %s\n", indent, item))
		}
		output.WriteString("\n")
	}
	if externalCount == 0 {
		output.WriteString("(none)\n\n")
	}

	output.WriteString("\n=== IMPLIED SPACE AGGREGATE ENTITIES (EXCLUDING no_collect) ===\n\n")
	aggregateCount := 0
	for _, spaceName := range admin.SpaceOrder {
		if spaceName == "root" {
			continue
		}

		aggregates := impliedAggregatesForSpace(spaceName, admin.SpaceKindByName[spaceName], admin.SpaceOrder, admin.EntityRecordsBySpace)
		if len(aggregates) == 0 {
			continue
		}

		indent := strings.Repeat("  ", admin.SpaceDepthByName[spaceName])
		for _, aggregate := range aggregates {
			aggregateCount++
			output.WriteString(fmt.Sprintf("%s- %s\n", indent, aggregate))
		}
	}
	if aggregateCount == 0 {
		output.WriteString("(none)\n")
	}
	output.WriteString("\n")

	output.WriteString("\n=== SUMMARY ===\n")
	output.WriteString(fmt.Sprintf("Total macro invocations: %d\n", invocationCount))
	output.WriteString(fmt.Sprintf("Valid invocations: %d\n", validInvocations))
	output.WriteString(fmt.Sprintf("Type/validation errors: %d\n", typeErrors))
	output.WriteString(fmt.Sprintf("Creation macros defined: %d\n", len(ctx.Macros)))
	output.WriteString(fmt.Sprintf("Entities listed by space: %d\n", entityCount))
	output.WriteString(fmt.Sprintf("External entities (no definition/imported): %d\n", externalCount))
	output.WriteString(fmt.Sprintf("External entities with config options: %d\n", externalWithConfigCount))
	output.WriteString(fmt.Sprintf("Implied aggregates: %d\n", aggregateCount))

	expansionReportPath, outputErr := writeDebugReport(root, house, DebugReportExpansion, output.String())
	if outputErr != nil {
		return fmt.Errorf("error writing expansion report: %w", outputErr)
	}
	if expansionReportPath != "" {
		fmt.Printf("  expansion report: %s\n", expansionReportPath)
	}

	collections := strings.Builder{}

	collections.WriteString("=== ENTITY COLLECTIONS REPORT ===\n")
	collections.WriteString(fmt.Sprintf("House: %s\n", house))
	collections.WriteString("Shows explicit aggregations: which constituent entities are collected into aggregated entities.\n")
	collections.WriteString("Entities marked [no_collect] are excluded from aggregation.\n\n")

	collections.WriteString("=== AGGREGATIONS BY SPACE ===\n\n")
	allAggregations := []struct {
		spaceName     string
		depth         int
		aggregateName string
		constituents  []string
	}{}

	for _, spaceName := range admin.SpaceOrder {
		aggregates := impliedAggregatesForSpace(spaceName, admin.SpaceKindByName[spaceName], admin.SpaceOrder, admin.EntityRecordsBySpace)
		if len(aggregates) == 0 {
			continue
		}

		for _, aggregateName := range aggregates {
			constituents := findAggregateConstituents(aggregateName, spaceName, admin.SpaceOrder, admin.EntityRecordsBySpace)
			if len(constituents) > 0 {
				allAggregations = append(allAggregations, struct {
					spaceName     string
					depth         int
					aggregateName string
					constituents  []string
				}{
					spaceName:     spaceName,
					depth:         admin.SpaceDepthByName[spaceName],
					aggregateName: aggregateName,
					constituents:  constituents,
				})
			}
		}
	}

	sort.Slice(allAggregations, func(i, j int) bool {
		if allAggregations[i].spaceName != allAggregations[j].spaceName {
			return allAggregations[i].spaceName < allAggregations[j].spaceName
		}
		return allAggregations[i].aggregateName < allAggregations[j].aggregateName
	})

	currentSpace := ""
	for _, agg := range allAggregations {
		if currentSpace != agg.spaceName {
			if currentSpace != "" {
				collections.WriteString("\n")
			}
			currentSpace = agg.spaceName
			indent := strings.Repeat("  ", agg.depth)
			collections.WriteString(fmt.Sprintf("%s%s %q:\n", indent, formatSpaceLabel(admin.SpaceKindByName[agg.spaceName]), agg.spaceName))
		}

		indent := strings.Repeat("  ", agg.depth+1)
		collections.WriteString(fmt.Sprintf("%s%s as aggregation of:\n", indent, agg.aggregateName))
		sort.Strings(agg.constituents)
		for _, constituent := range agg.constituents {
			collections.WriteString(fmt.Sprintf("%s  - %s\n", indent, constituent))
		}
	}

	collections.WriteString("\n=== SPACE LEVEL CONTROLS ===\n\n")
	for _, spaceName := range admin.SpaceOrder {
		hasSpaceOn := len(admin.SpaceOnByName[spaceName]) > 0
		hasSpaceOff := len(admin.SpaceOffByName[spaceName]) > 0

		if hasSpaceOn || hasSpaceOff {
			indent := strings.Repeat("  ", admin.SpaceDepthByName[spaceName])
			collections.WriteString(fmt.Sprintf("%s%s %q:\n", indent, formatSpaceLabel(admin.SpaceKindByName[spaceName]), spaceName))

			if hasSpaceOn {
				collections.WriteString(fmt.Sprintf("%s  Lights to turn ON:\n", indent))
				for _, light := range admin.SpaceOnByName[spaceName] {
					collections.WriteString(fmt.Sprintf("%s    - %s\n", indent, light))
				}
			}

			if hasSpaceOff {
				collections.WriteString(fmt.Sprintf("%s  Controls to turn OFF:\n", indent))
				for _, control := range admin.SpaceOffByName[spaceName] {
					collections.WriteString(fmt.Sprintf("%s    - %s\n", indent, control))
				}
			}

			collections.WriteString("\n")
		}
	}

	collections.WriteString("\n=== COLLECTION STATISTICS ===\n")
	collections.WriteString(fmt.Sprintf("Total aggregations: %d\n", len(allAggregations)))

	collectionsReportPath, collectionsErr := writeDebugReport(root, house, DebugReportCollections, collections.String())
	if collectionsErr != nil {
		return fmt.Errorf("error writing collections report: %w", collectionsErr)
	}
	if collectionsReportPath != "" {
		fmt.Printf("  collections report: %s\n", collectionsReportPath)
	}
	return nil
}

func parseSpaceHeader(line string) (string, string, bool) {
	if !strings.HasSuffix(line, "with:") {
		return "", "", false
	}

	spaceKind := SpaceKindRegular
	header := line
	if strings.HasPrefix(header, "virtual space ") {
		spaceKind = SpaceKindVirtual
		header = strings.TrimPrefix(header, "virtual space ")
	} else if strings.HasPrefix(header, "space ") {
		header = strings.TrimPrefix(header, "space ")
	} else {
		return "", "", false
	}

	if idx := strings.Index(header, " with"); idx > 0 {
		return spaceKind, header[:idx], true
	}

	return spaceKind, "", true
}

func formatSpaceLabel(spaceKind string) string {
	if spaceKind == SpaceKindVirtual {
		return "Virtual space"
	}
	return "Space"
}

func formatSpacePath(spacePath []string) string {
	if len(spacePath) == 0 {
		return "root"
	}
	return strings.Join(spacePath, " / ")
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
			"adjustment ", "cli_switch ", "cli_sensor ", "condition ", "value ", "available ",
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
		if strings.Count(afterDot, "/") >= 1 && !strings.Contains(afterDot, ":") {
			return true
		}
	}
	return false
}

// hasSecondColonSeparator reports whether an entity specification contains a legacy
// sub-domain separator: a second ':' that is not part of a '::' relative-path prefix.
// Example: "sensor.physical:dishwasher/robb:temperature" is invalid — use '/' instead.
func hasSecondColonSeparator(spec string) bool {
	dotIdx := strings.Index(spec, ".")
	if dotIdx < 0 {
		return false
	}
	remainder := spec[dotIdx+1:]
	colonIdx := strings.Index(remainder, ":")
	if colonIdx < 0 {
		return false
	}
	pathPart := remainder[colonIdx+1:]
	// '::' relative-path prefix: skip the second colon and check the rest.
	if strings.HasPrefix(pathPart, ":") {
		pathPart = pathPart[1:]
	}
	return strings.Contains(pathPart, ":")
}

func extractEntityIdentity(fullName string) TEntityIdentity {
	identity := TEntityIdentity{}

	dotIdx := strings.Index(fullName, ".")
	if dotIdx <= 0 || dotIdx >= len(fullName)-1 {
		return identity
	}

	identity.Domain = fullName[:dotIdx]
	remainder := fullName[dotIdx+1:]

	if strings.HasPrefix(remainder, "[") && strings.HasSuffix(remainder, "]") {
		identity.IsRaw = true
		identity.RawName = strings.TrimSuffix(strings.TrimPrefix(remainder, "["), "]")
		return identity
	}

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

func extractSubdomain(identity TEntityIdentity) string {
	if identity.IsRaw {
		return "raw"
	}

	if identity.Domain != "" {
		if identity.Sphere != "" {
			return fmt.Sprintf("%s:%s", identity.Domain, identity.Sphere)
		}
		return identity.Domain
	}

	return ""
}

func extractDomainFromAggregate(aggregate string) string {
	parts := strings.Split(aggregate, ".")
	if len(parts) > 0 {
		return parts[0]
	}
	return "unknown"
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

type TCollectedExpandedEntityRecord struct {
	SpacePath []string
	Record    TEntityRecord
}

// scanNodeBody scans the body lines of a providing-macro node entity for enabler and delay_off.
// Returns the enabler HA entity ID (empty if none) and the delay_off value (empty if none).
// startIdx points to the entity declaration line; body scanning starts at startIdx+1.
func scanNodeBody(lines []string, startIdx int, spacePath []string) (enablerEntityID, delayOff string) {
	depth := 0
	for i := startIdx + 1; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
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
		if strings.HasPrefix(t, "condition ") {
			// Condition format: condition entity [enabler] "template";
			// Find the opening quote to separate entity refs from the template string.
			quoteIdx := strings.Index(t, "\"")
			if quoteIdx > len("condition ") {
				beforeQuote := strings.TrimSpace(t[len("condition "):quoteIdx])
				fields := strings.Fields(beforeQuote)
				// fields[0] is the representative (may be unresolved ${representative_entity}).
				// fields[1], if present, is the enabler entity spec.
				if len(fields) >= 2 {
					enablerSpec := fields[len(fields)-1]
					enablerSpec = strings.TrimSuffix(enablerSpec, ";")
					normalized := normalizeEntityFullName(enablerSpec, spacePath)
					enablerEntityID = toHomeAssistantEntityID(normalized)
				}
			}
		}
		if strings.HasPrefix(t, "delay_off ") {
			delayOff = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(t, "delay_off "), ";"))
		}
	}
	return
}

// scanBatteryAlertBody extracts the integer alert threshold from a battery_alert entity body.
// Returns 0 if the threshold cannot be parsed.
func scanBatteryAlertBody(lines []string, startIdx int) int {
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
		if strings.HasPrefix(t, "condition ") {
			if ltIdx := strings.Index(t, "< "); ltIdx >= 0 {
				rest := t[ltIdx+2:]
				end := 0
				for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
					end++
				}
				if end > 0 {
					if n, err := strconv.Atoi(rest[:end]); err == nil {
						return n
					}
				}
			}
		}
	}
	return 0
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

// parseConditionDirective extracts sources and expression from a "condition ..." directive string.
func parseConditionDirective(t string, spacePath []string) (sources []string, expr string) {
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

// scanConditionBody extracts generic condition directive fields from an entity body.
// Also checks the entity declaration line itself for an inline "with condition ..." clause.
// Returns sources (normalised HA entity IDs, optionally with "!attr" suffix), expression,
// device_class, delay_on, delay_off.
func scanConditionBody(lines []string, startIdx int, spacePath []string) (sources []string, expr, deviceClass, delayOn, delayOff string) {
	// Check for inline condition on the entity declaration line itself.
	decl := strings.TrimSpace(lines[startIdx])
	if withIdx := strings.Index(decl, " with condition "); withIdx >= 0 {
		condPart := strings.TrimSpace(decl[withIdx+len(" with condition "):])
		sources, expr = parseConditionDirective("condition "+condPart, spacePath)
		return // inline form has no block body
	}

	depth := 0
	for i := startIdx + 1; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
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
		if strings.HasPrefix(t, "condition ") && depth == 0 {
			sources, expr = parseConditionDirective(t, spacePath)
		}
		if strings.HasPrefix(t, "device_class ") && depth == 0 {
			deviceClass = strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(t, "device_class ")), ";")
			deviceClass = strings.Trim(deviceClass, "\"")
		}
		if strings.HasPrefix(t, "delay_on ") && depth == 0 {
			delayOn = strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(t, "delay_on ")), ";")
		}
		if strings.HasPrefix(t, "delay_off ") && depth == 0 {
			delayOff = strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(t, "delay_off ")), ";")
		}
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

// scanValueDirective returns the "value <expr>" body content with the entity spec
// normalised to the fully qualified path form using spacePath.
func scanValueDirective(lines []string, startIdx int, spacePath []string) string {
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
		if strings.HasPrefix(t, "value ") {
			raw := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(t, "value "), ";"))
			exclamIdx := strings.Index(raw, "!")
			if exclamIdx > 0 {
				entitySpec := raw[:exclamIdx]
				attribute := raw[exclamIdx+1:]
				normalised := normalizeEntityFullName(entitySpec, spacePath)
				return normalised + "!" + attribute
			}
			return normalizeEntityFullName(raw, spacePath)
		}
	}
	return ""
}

// scanMediaSwitchBody extracts the controlled media_player name and optional no-play-input
// source name from a media_switch directive inside a switch entity body.
// A no_play_input value that still contains "${" (unfilled optional parameter) is ignored.
func scanMediaSwitchBody(lines []string, startIdx int, spacePath []string) (playerName, noPlayInput string) {
	for i := startIdx + 1; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if t == "end;" {
			break
		}
		if strings.HasPrefix(t, "media_switch ") {
			fields := strings.Fields(strings.TrimSuffix(t, ";"))
			if len(fields) >= 2 {
				playerName = normalizeEntityFullName(fields[1], spacePath)
			}
			if len(fields) >= 3 && !strings.Contains(fields[2], "${") {
				noPlayInput = strings.Trim(fields[2], `"`)
			}
			return
		}
	}
	return
}

func collectExpandedEntityRecords(ctx *TMacroExpansionContext, invocation *TMacroInvocation, spacePath []string, inheritedNoCollect bool, callChain string) ([]TCollectedExpandedEntityRecord, error) {
	if _, exists := ctx.Macros[invocation.Name]; !exists {
		return nil, fmt.Errorf("unknown macro: %s", invocation.Name)
	}

	expandedLines, _, err := ctx.ExpandMacro(invocation, spacePath)
	if err != nil {
		return nil, err
	}

	records := []TCollectedExpandedEntityRecord{}
	currentSpacePath := append([]string{}, spacePath...)
	openBlocks := []string{}
	// savedSpacePaths[i] is non-nil when openBlocks[i] is an absolute-path space: it holds
	// the currentSpacePath to restore when that block is closed.
	savedSpacePaths := [][]string{}
	// savedNoCollect[i] holds the currentNoCollect value active before block i was opened,
	// so it can be restored when that block closes.
	savedNoCollect := []bool{}
	currentNoCollect := inheritedNoCollect || invocationHasNoCollect(invocation)

	for idx := 0; idx < len(expandedLines); idx++ {
		rawLine := expandedLines[idx]
		trimmed := strings.TrimSpace(rawLine)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if entityDecl, ok := extractExpandedEntityDeclaration(trimmed); ok {
			fullName := normalizeEntityFullName(entityDecl.Specification, currentSpacePath)
			// Use the same logic as ParseEntitiesAndFillAdministration: inspect the entity
			// line and its inline/block body to determine whether it is self-defined or assumed.
			hasDefOrImport, _ := analyzeEntityDefinitionContext(expandedLines, idx)
			rec := TEntityRecord{
				Name:                  fullName,
				Identity:              extractEntityIdentity(fullName),
				NoCollect:             entityDecl.NoCollect || currentNoCollect,
				HasDefinitionOrImport: hasDefOrImport,
				Provenance:            callChain,
			}
			// Scan body for providing-macro node fields (enabler, delay_off).
			if strings.HasSuffix(fullName, "/node") {
				rec.NodeEnablerEntityID, rec.NodeDelayOff = scanNodeBody(expandedLines, idx, currentSpacePath)
			}
			// Scan body for battery_alert threshold.
			if strings.HasSuffix(fullName, "/battery_alert") {
				rec.BatteryAlertLevel = scanBatteryAlertBody(expandedLines, idx)
			}
			// Scan body for input_number properties.
			if rec.Identity.Domain == "input_number" {
				rec.InputNumberMin, rec.InputNumberMax, rec.InputNumberStep, rec.InputNumberUnit, rec.InputNumberIcon = scanInputNumberBody(expandedLines, idx)
			}
			// Scan body for value directive (template sensor with attribute access).
			if rec.Identity.Domain == "sensor" {
				rec.ValueExpr = scanValueDirective(expandedLines, idx, currentSpacePath)
			}
			// Scan body for media_switch directive (media switch entity).
			if rec.Identity.Domain == "switch" && strings.HasSuffix(rec.Identity.Path, "/media") {
				rec.MediaSwitchPlayerName, rec.MediaSwitchNoPlayInput = scanMediaSwitchBody(expandedLines, idx, currentSpacePath)
			}
			// Scan body for generic condition directive (not node or battery_alert — those are handled above).
			if !strings.HasSuffix(fullName, "/node") && !strings.HasSuffix(fullName, "/battery_alert") {
				rec.ConditionSources, rec.ConditionExpr, rec.ConditionDevClass, rec.ConditionDelayOn, rec.ConditionDelayOff = scanConditionBody(expandedLines, idx, currentSpacePath)
			}
			// Extract inline "with adjustment <offset> <scale>;" on the entity declaration line.
			if rec.Identity.Domain == "sensor" {
				if adjIdx := strings.Index(trimmed, " with adjustment "); adjIdx >= 0 {
					adjPart := strings.TrimSuffix(strings.TrimSpace(trimmed[adjIdx+len(" with adjustment "):]), ";")
					adjFields := strings.Fields(adjPart)
					if len(adjFields) >= 2 {
						rec.AdjustmentOffset = adjFields[0]
						rec.AdjustmentScale = adjFields[1]
					}
				}
			}
			records = append(records, TCollectedExpandedEntityRecord{
				SpacePath: append([]string{}, currentSpacePath...),
				Record:    rec,
			})
			// Process an inline "with call providing <target>;" on the same entity line.
			// This pattern arises from the `device` macro: "entity ${entity} with call providing ${entity.path};"
			// The inline providing creates the node sub-entity with this entity as its representative.
			if withIdx := strings.Index(trimmed, " with "); withIdx > 0 {
				inlineStmt := strings.TrimSpace(trimmed[withIdx+len(" with "):])
				if strings.HasPrefix(inlineStmt, "call providing ") {
					parsedProviding, parseErr := ctx.ParseMacroInvocation(inlineStmt)
					if parseErr == nil {
						if _, exists := ctx.Macros[parsedProviding.Name]; exists {
							hostIdx := len(records) - 1
							nestedChain := callChain + " → " + parsedProviding.Name + " " + parsedProviding.Target
							nestedRecords, nestedErr := collectExpandedEntityRecords(ctx, parsedProviding, currentSpacePath, currentNoCollect, nestedChain)
							if nestedErr == nil {
								hostID := toHomeAssistantEntityID(records[hostIdx].Record.Name)
								for i := range nestedRecords {
									if strings.HasSuffix(nestedRecords[i].Record.Name, "/node") {
										nestedRecords[i].Record.NodeRepresentativeEntityID = hostID
									}
								}
								records = append(records, nestedRecords...)
							}
						}
					}
				}
			}
			continue
		}

		if nestedInvocation, ok := extractNestedMacroInvocation(trimmed); ok {
			parsedInvocation, parseErr := ctx.ParseMacroInvocation(nestedInvocation)
			if parseErr != nil {
				return nil, fmt.Errorf("cannot parse nested macro invocation %q: %w", nestedInvocation, parseErr)
			}
			if isTemplatedMacroName(parsedInvocation.Name) {
				continue
			}
			if _, exists := ctx.Macros[parsedInvocation.Name]; !exists {
				return nil, fmt.Errorf("unknown macro: %s", parsedInvocation.Name)
			}

			nestedChain := callChain + " → " + parsedInvocation.Name + " " + parsedInvocation.Target
			nestedRecords, nestedErr := collectExpandedEntityRecords(ctx, parsedInvocation, currentSpacePath, currentNoCollect, nestedChain)
			if nestedErr != nil {
				return nil, nestedErr
			}
			records = append(records, nestedRecords...)
			continue
		}

		// Multi-line "call <macro> <concrete-target> with: ... end;" forms in expanded bodies.
		// Collect the full block, parse it as a single invocation, and recurse.
		// Relative-path targets (starting with ':') and variable targets ('$') are skipped.
		if strings.HasPrefix(trimmed, "call ") && strings.HasSuffix(trimmed, "with:") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 3 {
				macroName := parts[1]
				target := parts[2]
				if _, exists := ctx.Macros[macroName]; exists &&
					!strings.HasPrefix(target, ":") && !strings.Contains(target, "${") &&
					!isTemplatedMacroName(macroName) {
					multiLineText := trimmed
					j := idx + 1
					for ; j < len(expandedLines); j++ {
						nextTrimmed := strings.TrimSpace(expandedLines[j])
						multiLineText += "\n" + nextTrimmed
						if nextTrimmed == "end;" {
							break
						}
					}
					idx = j
					parsedNestedInvocation, parseErr := ctx.ParseMacroInvocation(multiLineText)
					if parseErr == nil {
						nestedChain := callChain + " → " + macroName + " " + target
						hostIdx := len(records) - 1
						nestedRecords, nestedErr := collectExpandedEntityRecords(ctx, parsedNestedInvocation, currentSpacePath, currentNoCollect, nestedChain)
						if nestedErr == nil {
							if macroName == "providing" && hostIdx >= 0 {
								hostID := toHomeAssistantEntityID(records[hostIdx].Record.Name)
								for i := range nestedRecords {
									if strings.HasSuffix(nestedRecords[i].Record.Name, "/node") {
										nestedRecords[i].Record.NodeRepresentativeEntityID = hostID
									}
								}
							}
							records = append(records, nestedRecords...)
						}
					}
				}
			}
			continue
		}

		// Follow single-line "call <macro> <concrete-target> ...;" invocations.
		// Relative-path targets (starting with ':') and variable targets ('$') cannot be
		// re-expanded correctly here — they depend on the original call-site context and
		// are handled at the top level by ParseEntitiesAndFillAdministration.
		if strings.HasPrefix(trimmed, "call ") && strings.HasSuffix(trimmed, ";") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 3 {
				macroName := parts[1]
				target := strings.TrimSuffix(parts[2], ";")
				if _, exists := ctx.Macros[macroName]; exists &&
					!strings.HasPrefix(target, ":") && !strings.Contains(target, "${") &&
					!isTemplatedMacroName(macroName) {
					parsedInvocation, parseErr := ctx.ParseMacroInvocation(trimmed)
					if parseErr == nil {
						nestedChain := callChain + " → " + macroName + " " + target
						hostIdx := len(records) - 1
						nestedRecords, nestedErr := collectExpandedEntityRecords(ctx, parsedInvocation, currentSpacePath, currentNoCollect, nestedChain)
						if nestedErr == nil {
							if macroName == "providing" && hostIdx >= 0 {
								hostID := toHomeAssistantEntityID(records[hostIdx].Record.Name)
								for i := range nestedRecords {
									if strings.HasSuffix(nestedRecords[i].Record.Name, "/node") {
										nestedRecords[i].Record.NodeRepresentativeEntityID = hostID
									}
								}
							}
							records = append(records, nestedRecords...)
						}
					}
				}
			}
			continue
		}

		if spaceKind, spaceName, ok := parseSpaceHeader(trimmed); ok {
			openBlocks = append(openBlocks, spaceKind)
			savedNoCollect = append(savedNoCollect, currentNoCollect)
			// An absolute-path space (e.g. "physical/apartment/living_room") has no ':'
			// separator but starts with a known sphere name followed by '/'.
			// It replaces the current space context rather than nesting inside it.
			if isAbsoluteSpaceName(spaceName) {
				savedSpacePaths = append(savedSpacePaths, append([]string{}, currentSpacePath...))
				// Convert "sphere/rest" → "sphere:rest" so normalizeSpaceContext parses it correctly.
				slashIdx := strings.Index(spaceName, "/")
				currentSpacePath = []string{spaceName[:slashIdx] + ":" + spaceName[slashIdx+1:]}
			} else {
				savedSpacePaths = append(savedSpacePaths, nil)
				if spaceName == "" {
					currentSpacePath = append(currentSpacePath, "?")
				} else {
					currentSpacePath = append(currentSpacePath, spaceName)
				}
			}
			continue
		}

		if strings.HasSuffix(trimmed, "with:") {
			openBlocks = append(openBlocks, "other")
			savedSpacePaths = append(savedSpacePaths, nil)
			savedNoCollect = append(savedNoCollect, currentNoCollect)
			continue
		}

		// A bare "no_collect;" statement inside a space body marks all entities
		// in the current block (and nested blocks) as no_collect.
		if trimmed == "no_collect;" {
			currentNoCollect = true
			continue
		}

		if trimmed == "end;" {
			if len(openBlocks) == 0 {
				continue
			}

			last := openBlocks[len(openBlocks)-1]
			openBlocks = openBlocks[:len(openBlocks)-1]
			var savedPath []string
			if len(savedSpacePaths) > 0 {
				savedPath = savedSpacePaths[len(savedSpacePaths)-1]
				savedSpacePaths = savedSpacePaths[:len(savedSpacePaths)-1]
			}
			// Restore no_collect state for the enclosing block.
			if len(savedNoCollect) > 0 {
				currentNoCollect = savedNoCollect[len(savedNoCollect)-1]
				savedNoCollect = savedNoCollect[:len(savedNoCollect)-1]
			}
			if last == SpaceKindRegular || last == SpaceKindVirtual {
				if savedPath != nil {
					// Restore the space path saved when the absolute-path space was opened.
					currentSpacePath = savedPath
				} else if len(currentSpacePath) > 0 {
					currentSpacePath = currentSpacePath[:len(currentSpacePath)-1]
				}
			}
		}
	}

	return records, nil
}

func extractExpandedEntityDeclaration(line string) (*TEntityDeclaration, bool) {
	trimmed := strings.TrimSpace(line)
	if strings.Contains(trimmed, "= entity ") {
		parts := strings.SplitN(trimmed, "= entity ", 2)
		if len(parts) == 2 {
			trimmed = "entity " + strings.TrimSpace(parts[1])
		}
	}

	decl, ok := extractEntityDeclaration(trimmed)
	if !ok {
		return nil, false
	}
	if !isConcreteEntitySpecification(decl.Specification) {
		return nil, false
	}
	return decl, true
}

func extractNestedMacroInvocation(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "entity ") {
		rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "entity "))
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			return "", false
		}
		// A variable reference (${foo}) is a parameterised entity name, not a macro call.
		if strings.HasPrefix(fields[0], "${") {
			return "", false
		}
		if !isConcreteEntitySpecification(fields[0]) {
			return rest, true
		}
	}
	return "", false
}

func validateMacroDefinitionOrdering(ctx *TMacroExpansionContext) error {
	for _, macro := range ctx.Macros {
		for _, rawLine := range macro.Body {
			trimmed := strings.TrimSpace(rawLine)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}

			nestedInvocation, ok := extractNestedMacroInvocation(trimmed)
			if !ok {
				continue
			}

			invocation, err := ctx.ParseMacroInvocation(nestedInvocation)
			if err != nil {
				return fmt.Errorf("macro %q contains invalid nested invocation %q: %w", macro.Name, nestedInvocation, err)
			}

			if isTemplatedMacroName(invocation.Name) {
				continue
			}

			calledMacro, exists := ctx.Macros[invocation.Name]
			if !exists {
				return fmt.Errorf("macro %q invokes unknown macro %q", macro.Name, invocation.Name)
			}

			if calledMacro.SourceLine >= macro.SourceLine {
				return fmt.Errorf("macro %q invokes %q before textual definition (caller line %d, callee line %d)", macro.Name, invocation.Name, macro.SourceLine, calledMacro.SourceLine)
			}
		}
	}

	return nil
}

func isTemplatedMacroName(name string) bool {
	return strings.Contains(name, "$") || strings.Contains(name, "<") || strings.Contains(name, ">")
}

func isConcreteEntitySpecification(spec string) bool {
	trimmed := strings.TrimSpace(strings.TrimSuffix(spec, ";"))
	return strings.Contains(trimmed, ".[") || strings.Contains(trimmed, ".")
}

func invocationHasNoCollect(invocation *TMacroInvocation) bool {
	for key, value := range canonicalInvocationParameters(invocation.Parameters) {
		if key == "nocollect" {
			return strings.EqualFold(strings.TrimSpace(value), "true")
		}
	}
	return false
}

func isAggregateSensorMetric(metric string) bool {
	switch metric {
	case "temperature", "humidity", "co2", "pressure", "illuminance", "noise", "wind_speed", "wind_direction":
		return true
	default:
		return false
	}
}

func isMacroInvocation(line string, macros map[string]*TParsedCreationMacro) bool {
	parts := strings.Fields(line)
	if len(parts) < 1 {
		return false
	}

	startIdx := 0
	if parts[0] == "call" && len(parts) > 1 {
		startIdx = 1
	}

	macroName := parts[startIdx]
	_, exists := macros[macroName]
	return exists
}

func extractCallInvocationMacroName(line string) (string, bool) {
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return "", false
	}
	if parts[0] == "call" {
		return parts[1], true
	}
	return "", false
}
