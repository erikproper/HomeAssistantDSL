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
 * Version of: 20.03.2026
 *
 */

package main

import (
	"fmt"
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
	Parameters []TMacroParameter
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

	// Collect body lines until "end;"
	idx := startIdx + 1
	for idx < len(lines) {
		bodyLine := strings.TrimSpace(lines[idx])

		if bodyLine == "end;" {
			idx++
			break
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

	// Parse type from remaining parts
	if len(parts) > 1 {
		param.Kind = parseParameterKind(parts[1])
	} else {
		param.Kind = ParamEntity // default
	}

	return param, nil
}

func parseParameterKind(typeStr string) TParameterKind {
	switch strings.ToLower(typeStr) {
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
	default:
		if strings.HasPrefix(typeStr, "set") {
			if strings.Contains(typeStr, "entityreference") {
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
		Target:     parts[idx+1],
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

func (ctx *TMacroExpansionContext) ExpandMacro(invocation *TMacroInvocation) ([]string, []string, error) {
	macro, ok := ctx.Macros[invocation.Name]
	if !ok {
		return nil, []string{fmt.Sprintf("unknown macro: %s", invocation.Name)}, nil
	}

	errors := []string{}

	// Type-check parameters
	for _, param := range macro.Parameters {
		val, paramProvided := invocation.Parameters[strings.ToLower(param.Name)]
		if !paramProvided && !param.Optional && val == "" {
			errors = append(errors, fmt.Sprintf("missing required parameter %s for macro %s", param.Name, invocation.Name))
		}
	}

	// Expand body with parameter substitution
	expanded := []string{}
	substitutions := make(map[string]string)
	substitutions["$entity"] = invocation.Target
	substitutions["$sphere"] = extractSphere(invocation.Target)

	// Add parameter substitutions
	for k, v := range invocation.Parameters {
		substitutions["$"+k] = v
	}

	for _, bodyLine := range macro.Body {
		expandedLine := bodyLine
		for placeholder, value := range substitutions {
			expandedLine = strings.ReplaceAll(expandedLine, placeholder, value)
		}
		expanded = append(expanded, expandedLine)
	}

	return expanded, errors, nil
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

	// Check required parameters
	for _, param := range macro.Parameters {
		paramKey := strings.ToLower(strings.TrimPrefix(param.Name, "$"))
		value, provided := invocation.Parameters[paramKey]

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

func validateParameterType(value string, expectedKind TParameterKind) string {
	switch expectedKind {
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
	}
	return ""
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
