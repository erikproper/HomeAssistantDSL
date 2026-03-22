/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Expander
 *
 * This component parses creation macro definitions, entity declarations, and spaces,
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
	"regexp"
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
)

type TMacroParameter struct {
	Name     string
	Kind     TParameterKind
	Optional bool
	Default  string
}

type TParsedCreationMacro struct {
	Name       string
	Parameters []TMacroParameter // Note: $domain, $sphere, $entity are always available as implied parameters
	Body       []string
	SourceLine int
}

type TMacroExpansionContext struct {
	Macros map[string]*TParsedCreationMacro
	Config TExpanderConfig
}

type TExpanderConfig struct {
	Verbose    bool
	CheckTypes bool
}

var variableReferencePattern = regexp.MustCompile(`\$[A-Za-z_][A-Za-z0-9_]*`)

// --- Parsing Creation Macro Definitions ---

func (ctx *TMacroExpansionContext) ParseCreationMacro(lines []string, startIdx int) (*TParsedCreationMacro, int, error) {
	// creation macro <name> { params }:
	//   <body>
	// end;

	if startIdx >= len(lines) {
		return nil, startIdx, nil
	}

	line := strings.TrimSpace(lines[startIdx])
	if !strings.HasPrefix(line, "creation macro ") {
		return nil, startIdx, nil
	}

	// Parse header: "creation macro <name> { $p1 t1 op, ... }:"
	macro := &TParsedCreationMacro{
		SourceLine: startIdx + 1,
		Parameters: []TMacroParameter{},
		Body:       []string{},
	}

	// Extract macro name and parameters
	headerLine := strings.TrimPrefix(line, "creation macro ")
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
	// starts a new "creation macro ..." header (or there is no next line).
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

			if nextIdx >= len(lines) || strings.HasPrefix(strings.TrimSpace(lines[nextIdx]), "creation macro ") {
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
	// declared in the macro header, so treat `$icon` as an implied optional string.
	if macro.Name == "power_switch" {
		hasIconParam := false
		for _, param := range macro.Parameters {
			if strings.TrimPrefix(strings.ToLower(param.Name), "$") == "icon" {
				hasIconParam = true
				break
			}
		}
		if !hasIconParam {
			macro.Parameters = append(macro.Parameters, TMacroParameter{
				Name:     "$icon",
				Kind:     ParamString,
				Optional: true,
			})
		}
	}

	return macro, idx, nil
}

func parseMacroParameters(paramsStr string, macro *TParsedCreationMacro) error {
	// Format: " { $p1 t1 op, $p2 t2, ... }:"
	// Extract content between { and }
	startIdx := strings.Index(paramsStr, "{")
	endIdx := strings.Index(paramsStr, "}")

	if startIdx < 0 || endIdx < 0 || startIdx >= endIdx {
		return nil // No parameters
	}

	paramContent := paramsStr[startIdx+1 : endIdx]
	if strings.TrimSpace(paramContent) == "" {
		return nil // Empty parameter list
	}

	// Split by comma and parse each parameter
	paramParts := strings.Split(paramContent, ",")
	for _, part := range paramParts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		param, err := parseParameter(part)
		if err != nil {
			return err
		}
		macro.Parameters = append(macro.Parameters, param)
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
	Name       string
	Target     string
	Parameters map[string]string
}

func (ctx *TMacroExpansionContext) ParseMacroInvocation(line string) (*TMacroInvocation, error) {
	// Parse:
	// - "create macro_name target with param value;"
	// - "create macro_name target with:\n    key value;\n  end;"
	invocationLines := strings.Split(line, "\n")
	header := strings.TrimSpace(invocationLines[0])
	parts := strings.Fields(header)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid macro invocation")
	}

	// Handle optional "create" keyword
	idx := 0
	if parts[0] == "create" {
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
	normalized := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(name, "$")))
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

func (ctx *TMacroExpansionContext) ExpandMacro(invocation *TMacroInvocation) ([]string, []string, error) {
	if strictErr := ctx.ValidateInvocationStrict(invocation); strictErr != nil {
		return nil, nil, strictErr
	}

	       macro := ctx.Macros[invocation.Name]
	       canonicalParameters := canonicalInvocationParameters(invocation.Parameters)

	       // --- Implied parameters: always inject $domain, $sphere, $entity ---
	       // $domain: Home Assistant domain (e.g., "switch", "sensor")
	       // $sphere: sphere (e.g., "social", "physical")
	       // $entity: full entity path (space path + subdomain)
	       // These are always available in macro bodies, even if not declared in the header.
	       expanded := []string{}
	       substitutions := make(map[string]string)
		// Parse domain, sphere, and entity from the invocation target
		domain, sphere, entityPath := parseDomainSphereEntity(invocation.Target)
		substitutions["$domain"] = domain
		substitutions["$sphere"] = sphere
		substitutions["$entity"] = entityPath

	       // Add explicit parameter substitutions
	       for _, param := range macro.Parameters {
		       if value, provided := canonicalParameters[normalizeParameterKey(param.Name)]; provided {
			       substitutions[param.Name] = value
		       }
	       }

	       for _, bodyLine := range macro.Body {
		       expandedLine := bodyLine
		       for placeholder, value := range substitutions {
			       expandedLine = strings.ReplaceAll(expandedLine, placeholder, value)
		       }
		       expanded = append(expanded, expandedLine)
	       }

	       return expanded, nil, nil

// --- Utility: parseDomainSphereEntity ---
// Splits an entity spec like "switch.social:nespresso" into domain, sphere, entityPath
func parseDomainSphereEntity(entitySpec string) (domain, sphere, entityPath string) {
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

	// Check required parameters
	for _, param := range macro.Parameters {
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

	report.WriteString("=== CREATION MACRO DEFINITIONS ===\n\n")
	for name, macro := range ctx.Macros {
		report.WriteString(fmt.Sprintf("creation macro %s { ", name))
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
	default:
		return "entity"
	}
}

func (ctx *TMacroExpansionContext) MacroDefinitionWarnings(macro *TParsedCreationMacro) []string {
	warnings := []string{}
	parameterKindByName := map[string]TParameterKind{}
	loopVariableNames := map[string]bool{}
	for _, parameter := range macro.Parameters {
		parameterName := strings.ToLower(strings.TrimPrefix(parameter.Name, "$"))
		parameterKindByName[parameterName] = parameter.Kind
	}

	for _, bodyLine := range macro.Body {
		trimmedLine := strings.TrimSpace(bodyLine)
		if strings.HasPrefix(trimmedLine, "for ") {
			parts := strings.Fields(trimmedLine)
			if len(parts) >= 2 && strings.HasPrefix(parts[1], "$") {
				loopVariableNames[strings.ToLower(strings.TrimPrefix(parts[1], "$"))] = true
			}
		}
	}

	for bodyLineIndex, bodyLine := range macro.Body {
		specificationToken, hasSpecification := entitySpecificationToken(bodyLine)
		if !hasSpecification || specificationToken == "" {
			continue
		}

		for _, variableReference := range variableReferencePattern.FindAllString(specificationToken, -1) {
			variableName := strings.ToLower(strings.TrimPrefix(variableReference, "$"))
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

	if parts[0] == "create" && len(parts) >= 3 {
		return parts[2], true
	}

	return "", false
}
