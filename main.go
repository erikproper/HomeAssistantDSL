/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Main
 *
 * This component migrates the legacy bash-based home-assistant source files into normalized definition files for review.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 21.03.2026
 *
 */

package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var THouseNames = []string{"Vienna", "Junglinster"}

func main() {
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error determining working directory: %v\n", err)
		os.Exit(1)
	}

	if err := runCLI(root, os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func runCLI(root string, args []string) error {
	args = applyDebugOptionArgs(args)
	if DebugEnabled {
		if err := consolidateLegacyDebugReports(root, THouseNames); err != nil {
			return err
		}
	}

	if len(args) == 0 {
		return runMigration(root, THouseNames)
	}

	command := strings.ToLower(strings.TrimSpace(args[0]))
	switch command {
	case "migrate":
		houses, err := resolveRequestedHouses(args[1:])
		if err != nil {
			return err
		}
		return runMigration(root, houses)
	case "interpret", "parse":
		houses, err := resolveRequestedHouses(args[1:])
		if err != nil {
			return err
		}
		return runInterpretation(root, houses)
	case "refresh":
		houses, err := resolveRequestedHouses(args[1:])
		if err != nil {
			return err
		}
		if err := runMigration(root, houses); err != nil {
			return err
		}
		return runInterpretation(root, houses)
	case "expand":
		houses, err := resolveRequestedHouses(args[1:])
		if err != nil {
			return err
		}
		return runExpansion(root, houses)
	case "check":
		houses, err := resolveRequestedHouses(args[1:])
		if err != nil {
			return err
		}
		return runAvailabilityCheck(root, houses)
	default:
		// Backward compatibility: go run . Vienna Junglinster
		houses, err := resolveRequestedHouses(args)
		if err != nil {
			return err
		}
		return runMigration(root, houses)
	}
}

func resolveRequestedHouses(args []string) ([]string, error) {
	if len(args) == 0 {
		return append([]string{}, THouseNames...), nil
	}

	knownByLower := map[string]string{}
	for _, houseName := range THouseNames {
		knownByLower[strings.ToLower(houseName)] = houseName
	}

	resolvedHouses := []string{}
	seen := map[string]bool{}
	for _, rawArg := range args {
		normalizedArg := strings.ToLower(strings.TrimSpace(rawArg))
		houseName, ok := knownByLower[normalizedArg]
		if !ok {
			return nil, fmt.Errorf("unknown house %q (allowed: %s)", rawArg, strings.Join(THouseNames, ", "))
		}
		if !seen[houseName] {
			seen[houseName] = true
			resolvedHouses = append(resolvedHouses, houseName)
		}
	}
	return resolvedHouses, nil
}

func normalizeCuratedMacrosContent(content string) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	headerPattern := regexp.MustCompile(`^(\s*)macro\s+([A-Za-z_][A-Za-z0-9_]*)\s*(?:\(([^)]*)\))?:\s*$`)
	for index, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if matches := headerPattern.FindStringSubmatch(line); matches != nil {
			parameterNames := []string{}
			for _, token := range strings.Fields(strings.TrimSpace(matches[3])) {
				token = strings.TrimPrefix(strings.TrimSpace(token), "$")
				if token != "" {
					parameterNames = append(parameterNames, token)
				}
			}
			parameterDeclarations := inferMacroParameterDeclarations(matches[2], parameterNames)
			lines[index] = matches[1] + formatMacroHeader(matches[2], parameterDeclarations)
			continue
		}
		if trimmedLine == "no_collect" || trimmedLine == "open_stop_close" {
			lines[index] = line + StatementEndToken
		}
	}
	return strings.Join(lines, "\n")
}

func parseMacro(content, source string) (TMacro, error) {
	macro := TMacro{Source: source}
	functionPattern := regexp.MustCompile(`^function\s+([A-Za-z_][A-Za-z0-9_]*)\s*\{\s*(#.*)?$`)
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	insideBody := false

	for _, rawLine := range lines {
		trimmedLine := strings.TrimSpace(rawLine)
		if !insideBody {
			if trimmedLine == "" {
				continue
			}
			matches := functionPattern.FindStringSubmatch(trimmedLine)
			if matches == nil {
				return macro, fmt.Errorf("unexpected macro declaration in %s", source)
			}
			macro.Name = matches[1]
			macro.Signature = strings.TrimSpace(matches[2])
			insideBody = true
			continue
		}

		if trimmedLine == "}" {
			return macro, nil
		}
		macro.Body = append(macro.Body, rawLine)
	}

	return macro, fmt.Errorf("unterminated macro in %s", source)
}

type TMacroBlock struct {
	Kind       string
	BaseIndent int
}

func normalizeMacroBody(bodyLines []string, parameterNames []string) []string {
	parameterByIndex := positionalParameterMap(parameterNames)
	preprocessedLines := []string{}
	for _, rawLine := range bodyLines {
		preprocessedLine := replacePositionalParameters(rawLine, parameterByIndex)
		preprocessedLines = append(preprocessedLines, preprocessedLine)
	}

	assignedVariableMap := detectAssignedVariableMap(preprocessedLines)
	for index, preprocessedLine := range preprocessedLines {
		preprocessedLines[index] = applyVariableNameMap(preprocessedLine, assignedVariableMap)
	}

	normalizedLines := []string{}
	blockStack := []TMacroBlock{}
	contentIndent := 0

	ifPattern := regexp.MustCompile(`^if\s+\[\s*(.*?)\s*\]\s*;?\s*then$`)
	forPattern := regexp.MustCompile(`^for\s+(.+?)\s+in\s+(.+?)\s*;\s*do$`)

	emit := func(indentLevel int, text string) {
		if indentLevel < 0 {
			indentLevel = 0
		}
		normalizedLines = append(normalizedLines, strings.Repeat(" ", indentLevel*2)+text)
	}
	closeTopBlock := func(expectedKinds ...string) bool {
		if len(blockStack) == 0 {
			return false
		}
		top := blockStack[len(blockStack)-1]
		if len(expectedKinds) > 0 {
			matched := false
			for _, expectedKind := range expectedKinds {
				if top.Kind == expectedKind {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		}
		emit(top.BaseIndent, "end;")
		blockStack = blockStack[:len(blockStack)-1]
		contentIndent = top.BaseIndent
		return true
	}
	openBlock := func(kind, header string) {
		baseIndent := contentIndent
		emit(baseIndent, header)
		blockStack = append(blockStack, TMacroBlock{Kind: kind, BaseIndent: baseIndent})
		contentIndent = baseIndent + 1
	}

	for _, rawLine := range preprocessedLines {
		trimmedLine := strings.TrimSpace(rawLine)
		if trimmedLine == "" {
			normalizedLines = append(normalizedLines, "")
			continue
		}

		if strings.HasPrefix(trimmedLine, "#") {
			emit(contentIndent, trimmedLine)
			continue
		}

		if matches := ifPattern.FindStringSubmatch(trimmedLine); matches != nil {
			openBlock("if", fmt.Sprintf("if %s then", strings.TrimSpace(matches[1])))
			continue
		}

		if matches := forPattern.FindStringSubmatch(trimmedLine); matches != nil {
			header := fmt.Sprintf("for %s in %s do", strings.TrimSpace(matches[1]), strings.TrimSpace(matches[2]))
			openBlock("for", header)
			continue
		}

		if strings.HasPrefix(trimmedLine, "begin space ") {
			spaceName := strings.TrimSpace(strings.TrimPrefix(trimmedLine, "begin space "))
			openBlock("space", "space "+trimTrailingPunctuation(spaceName)+" with:")
			continue
		}

		switch trimmedLine {
		case "else":
			if len(blockStack) > 0 && blockStack[len(blockStack)-1].Kind == "if" {
				top := blockStack[len(blockStack)-1]
				emit(top.BaseIndent, "else")
				contentIndent = top.BaseIndent + 1
			} else {
				emit(contentIndent, "else;")
			}
			continue
		case "fi":
			if closeTopBlock("if") {
				continue
			}
			emit(contentIndent, "fi;")
			continue
		case "done":
			if closeTopBlock("for") {
				continue
			}
			emit(contentIndent, "done;")
			continue
		case "end space":
			if closeTopBlock("space") {
				continue
			}
			emit(contentIndent, "end;")
			continue
		}

		emit(contentIndent, ensureStatementSemicolon(normalizeEntitySyntaxText(trimmedLine)))
	}

	for len(blockStack) > 0 {
		closeTopBlock()
	}

	return normalizedLines
}

func normalizeMacroSignatureComment(signature string) string {
	trimmedSignature := strings.TrimSpace(signature)
	if trimmedSignature == "" {
		return ""
	}
	if strings.HasPrefix(trimmedSignature, "#") {
		return trimmedSignature
	}
	return "# " + trimmedSignature
}

func formatMacroHeader(macroName string, parameterDeclarations []string) string {
	if len(parameterDeclarations) == 0 {
		return fmt.Sprintf("creation macro %s:", macroName)
	}
	return fmt.Sprintf("creation macro %s { %s }:", macroName, strings.Join(parameterDeclarations, ", "))
}

func inferMacroParameterDeclarations(macroName string, parameterNames []string) []string {
	knownByMacroAndName := map[string]map[string]string{
		"entity_battery_alert": {
			"entity":     "entityReference",
			"alertLevel": "int op",
		},
		"entity_battery_from_device": {
			"entity":       "entityReference",
			"sourceEntity": "entityReference",
			"alertLevel":   "int op",
		},
		"entity_battery_level": {
			"entity":     "entityReference",
			"alertLevel": "int op",
		},
		"entity_battery_level_device": {
			"entity":     "entityReference",
			"alertLevel": "int op",
		},
		"entity_light_device": {
			"sphereOrName": "string",
			"name":         "string op",
		},
		"entity_media_player": {
			"entity": "entityReference",
			"option": "string",
			"value":  "string",
		},
		"entity_power_switch": {
			"sphere":         "string",
			"entity":         "entityReference",
			"deviceNode":     "string",
			"powerThreshold": "int",
		},
		"entity_switch_device": {
			"sphereOrName": "string",
			"name":         "string op",
		},
		"entity_thermostat": {
			"entity": "entityReference",
		},
		"entity_zigbee_group": {
			"domain":   "string",
			"sphere":   "string",
			"location": "string",
			"entities": "set of string",
		},
		"entity_zwave_node": {
			"node": "string",
		},
	}

	declarations := []string{}
	for _, parameterName := range parameterNames {
		trimmedParameterName := strings.TrimSpace(parameterName)
		if trimmedParameterName == "" {
			continue
		}

		isVariadic := strings.HasSuffix(trimmedParameterName, "...")
		baseName := strings.TrimSuffix(trimmedParameterName, "...")
		typeDeclaration := "string"
		if byName, found := knownByMacroAndName[macroName]; found {
			if knownType, typed := byName[baseName]; typed {
				typeDeclaration = knownType
			}
		}
		if isVariadic && typeDeclaration == "string" {
			typeDeclaration = "set of string"
		}

		declarations = append(declarations, fmt.Sprintf("$%s %s", baseName, typeDeclaration))
	}

	return declarations
}

func positionalParameterMap(parameterNames []string) map[int]string {
	result := map[int]string{}
	for parameterIndex, parameterName := range parameterNames {
		baseName := strings.TrimSuffix(strings.TrimSpace(parameterName), "...")
		if baseName == "" {
			continue
		}
		result[parameterIndex+1] = baseName
	}
	return result
}

func replacePositionalParameters(line string, parameterByIndex map[int]string) string {
	pattern := regexp.MustCompile(`\$(\d+)`)
	return pattern.ReplaceAllStringFunc(line, func(match string) string {
		matches := pattern.FindStringSubmatch(match)
		if matches == nil {
			return match
		}
		parameterIndex := 0
		_, err := fmt.Sscanf(matches[1], "%d", &parameterIndex)
		if err != nil {
			return match
		}
		parameterName, ok := parameterByIndex[parameterIndex]
		if !ok || parameterName == "" {
			return match
		}
		return "$" + parameterName
	})
}

func detectAssignedVariableMap(lines []string) map[string]string {
	assignmentPattern := regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=`)
	variableMap := map[string]string{}
	for _, line := range lines {
		matches := assignmentPattern.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		originalName := matches[1]
		normalizedName := toLowerCamelIdentifier(originalName)
		if originalName != normalizedName {
			variableMap[originalName] = normalizedName
		}
	}
	return variableMap
}

func applyVariableNameMap(line string, variableMap map[string]string) string {
	updatedLine := line

	bracedPattern := regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	updatedLine = bracedPattern.ReplaceAllStringFunc(updatedLine, func(match string) string {
		matches := bracedPattern.FindStringSubmatch(match)
		if matches == nil {
			return match
		}
		if replacementName, ok := variableMap[matches[1]]; ok {
			return "${" + replacementName + "}"
		}
		return match
	})

	plainPattern := regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`)
	updatedLine = plainPattern.ReplaceAllStringFunc(updatedLine, func(match string) string {
		matches := plainPattern.FindStringSubmatch(match)
		if matches == nil {
			return match
		}
		if replacementName, ok := variableMap[matches[1]]; ok {
			return "$" + replacementName
		}
		return match
	})

	assignmentPattern := regexp.MustCompile(`^(\s*)([A-Za-z_][A-Za-z0-9_]*)(\s*=.*)$`)
	assignmentMatches := assignmentPattern.FindStringSubmatch(updatedLine)
	if assignmentMatches != nil {
		if replacementName, ok := variableMap[assignmentMatches[2]]; ok {
			updatedLine = assignmentMatches[1] + replacementName + assignmentMatches[3]
		}
	}

	return updatedLine
}

func inferMacroParameterNames(macro TMacro) []string {
	knownMacroParameters := map[string][]string{
		"entity_battery_alert":       {"entity", "alertLevel"},
		"entity_battery_from_device": {"entity", "sourceEntity", "alertLevel"},
		"entity_battery_level":       {"entity", "alertLevel"},
		"entity_battery_level_device": {
			"entity", "alertLevel",
		},
		"entity_light_device":  {"sphereOrName", "name"},
		"entity_media_player":  {"entity", "option", "value"},
		"entity_power_switch":  {"sphere", "entity", "deviceNode", "powerThreshold"},
		"entity_switch_device": {"sphereOrName", "name"},
		"entity_thermostat":    {"entity"},
		"entity_zigbee_group":  {"domain", "sphere", "location", "entities..."},
		"entity_zwave_node":    {"node"},
	}

	maxPosition := 0
	positionPattern := regexp.MustCompile(`\$(\d+)`)
	for _, line := range macro.Body {
		allMatches := positionPattern.FindAllStringSubmatch(line, -1)
		for _, match := range allMatches {
			position := 0
			if _, err := fmt.Sscanf(match[1], "%d", &position); err == nil && position > maxPosition {
				maxPosition = position
			}
		}
	}

	parameterByPosition := map[int]string{}
	if known, ok := knownMacroParameters[macro.Name]; ok {
		for index, name := range known {
			normalizedName := toLowerCamelIdentifier(strings.TrimSuffix(name, "..."))
			if normalizedName == "" {
				normalizedName = fmt.Sprintf("arg%d", index+1)
			}
			if strings.HasSuffix(name, "...") {
				normalizedName += "..."
			}
			parameterByPosition[index+1] = normalizedName
		}
	}

	if macro.Signature != "" {
		signaturePattern := regexp.MustCompile(`\$([0-9]+)\s*=\s*([A-Za-z_][A-Za-z0-9_]*)`)
		for _, match := range signaturePattern.FindAllStringSubmatch(macro.Signature, -1) {
			position := 0
			if _, err := fmt.Sscanf(match[1], "%d", &position); err != nil || position <= 0 {
				continue
			}
			if _, alreadyKnown := parameterByPosition[position]; alreadyKnown {
				continue
			}
			parameterByPosition[position] = toLowerCamelIdentifier(match[2])
		}
	}

	maxDeclaredPosition := maxPosition
	for position := range parameterByPosition {
		if position > maxDeclaredPosition {
			maxDeclaredPosition = position
		}
	}

	parameterNames := []string{}
	usedNames := map[string]bool{}
	for position := 1; position <= maxDeclaredPosition; position++ {
		name, ok := parameterByPosition[position]
		if !ok || strings.TrimSpace(name) == "" {
			name = fmt.Sprintf("arg%d", position)
		}

		baseName := strings.TrimSuffix(name, "...")
		normalizedBase := toLowerCamelIdentifier(baseName)
		if normalizedBase == "" {
			normalizedBase = fmt.Sprintf("arg%d", position)
		}
		for usedNames[normalizedBase] {
			normalizedBase += "Value"
		}
		usedNames[normalizedBase] = true

		if strings.HasSuffix(name, "...") {
			parameterNames = append(parameterNames, normalizedBase+"...")
		} else {
			parameterNames = append(parameterNames, normalizedBase)
		}
	}

	return parameterNames
}

func toLowerCamelIdentifier(name string) string {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return ""
	}

	tokens := []string{}
	currentToken := strings.Builder{}
	for _, currentRune := range trimmedName {
		if (currentRune >= 'A' && currentRune <= 'Z') || (currentRune >= 'a' && currentRune <= 'z') || (currentRune >= '0' && currentRune <= '9') {
			currentToken.WriteRune(currentRune)
		} else {
			if currentToken.Len() > 0 {
				tokens = append(tokens, currentToken.String())
				currentToken.Reset()
			}
		}
	}
	if currentToken.Len() > 0 {
		tokens = append(tokens, currentToken.String())
	}
	if len(tokens) == 0 {
		return ""
	}

	if len(tokens) == 1 && !strings.ContainsAny(trimmedName, "_- ") {
		singleToken := tokens[0]
		if len(singleToken) == 1 {
			return strings.ToLower(singleToken)
		}
		return strings.ToLower(singleToken[:1]) + singleToken[1:]
	}

	normalized := strings.ToLower(tokens[0])
	for _, token := range tokens[1:] {
		lowerToken := strings.ToLower(token)
		if lowerToken == "" {
			continue
		}
		normalized += strings.ToUpper(lowerToken[:1]) + lowerToken[1:]
	}
	return normalized
}

func toUpperCamelIdentifier(name string) string {
	lowerCamelName := toLowerCamelIdentifier(name)
	if lowerCamelName == "" {
		return ""
	}
	if len(lowerCamelName) == 1 {
		return strings.ToUpper(lowerCamelName)
	}
	return strings.ToUpper(lowerCamelName[:1]) + lowerCamelName[1:]
}

func ensureStatementSemicolon(line string) string {
	trimmedLine := strings.TrimSpace(line)
	if trimmedLine == "" || strings.HasSuffix(trimmedLine, ";") {
		return trimmedLine
	}
	return trimmedLine + ";"
}

func stripCreatePrefix(statement string) string {
	if strings.HasPrefix(statement, "create ") {
		return strings.TrimPrefix(statement, "create ")
	}
	return statement
}

func transformEntitiesDefinition(content string) string {
	type TBlock struct {
		SourceIndent int
		EndIndent    int
		Kind         string
	}

	canonicalIndent := func(depth int) int {
		return depth * 2
	}

	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	outputLines := []string{}
	openBlocks := []TBlock{}

	closeSyntheticBlocks := func(indentWidth int) {
		for len(openBlocks) > 0 {
			lastBlock := openBlocks[len(openBlocks)-1]
			if lastBlock.Kind != "synthetic" || indentWidth > lastBlock.SourceIndent {
				return
			}
			outputLines = append(outputLines, strings.Repeat(" ", lastBlock.EndIndent)+"end;")
			openBlocks = openBlocks[:len(openBlocks)-1]
		}
	}

	for index, rawLine := range lines {
		trimmedLine := strings.TrimSpace(rawLine)
		if trimmedLine == "" {
			// Keep user-intended visual grouping, but close synthetic sub-blocks first
			// so blank separators do not end up inside generated with:/end; bodies.
			closeSyntheticBlocks(0)
			outputLines = append(outputLines, "")
			continue
		}

		indentWidth := len(leadingWhitespace(rawLine))
		closeSyntheticBlocks(indentWidth)
		statementIndent := canonicalIndent(len(openBlocks))

		switch {
		case strings.HasPrefix(trimmedLine, "#"):
			normalizedComment := normalizeEntitySyntaxText(trimmedLine)
			outputLines = append(outputLines, strings.Repeat(" ", statementIndent)+normalizedComment)
		case trimmedLine == "end space":
			if len(openBlocks) > 0 && openBlocks[len(openBlocks)-1].Kind == "space" {
				spaceBlock := openBlocks[len(openBlocks)-1]
				openBlocks = openBlocks[:len(openBlocks)-1]
				outputLines = append(outputLines, strings.Repeat(" ", spaceBlock.EndIndent)+"end;")
				continue
			}
			outputLines = append(outputLines, strings.Repeat(" ", statementIndent)+"end;")
		case strings.HasPrefix(trimmedLine, "begin space "):
			spaceName := strings.TrimSpace(strings.TrimPrefix(trimmedLine, "begin space "))
			headerIndent := canonicalIndent(len(openBlocks))
			outputLines = append(outputLines, strings.Repeat(" ", headerIndent)+"space "+trimTrailingPunctuation(spaceName)+" with:")
			openBlocks = append(openBlocks, TBlock{SourceIndent: indentWidth, EndIndent: headerIndent, Kind: "space"})
		default:
			statement := normalizeEntitySyntaxText(trimTrailingPunctuation(trimmedLine))
			statementNoCreate := stripCreatePrefix(statement)
			if hasIndentedContinuation(lines, index) {
				statementFields := strings.Fields(statementNoCreate)
				headerIndent := canonicalIndent(len(openBlocks))
				if len(statementFields) == 5 && statementFields[0] == "power_switch" {
					outputLines = append(outputLines, strings.Repeat(" ", headerIndent)+"power_switch switch."+statementFields[1]+":"+statementFields[2]+" with:")
					outputLines = append(outputLines, strings.Repeat(" ", headerIndent+2)+"node "+statementFields[3]+";")
					outputLines = append(outputLines, strings.Repeat(" ", headerIndent+2)+"threshold "+statementFields[4]+";")
				} else {
					outputLines = append(outputLines, strings.Repeat(" ", headerIndent)+statementNoCreate+" with:")
				}
				openBlocks = append(openBlocks, TBlock{SourceIndent: indentWidth, EndIndent: headerIndent, Kind: "synthetic"})
			} else {
				statementFields := strings.Fields(statementNoCreate)
				if len(statementFields) == 5 && statementFields[0] == "power_switch" {
					outputLines = append(outputLines, strings.Repeat(" ", statementIndent)+"power_switch switch."+statementFields[1]+":"+statementFields[2]+" with:")
					outputLines = append(outputLines, strings.Repeat(" ", statementIndent+2)+"node "+statementFields[3]+";")
					outputLines = append(outputLines, strings.Repeat(" ", statementIndent+2)+"threshold "+statementFields[4]+";")
					outputLines = append(outputLines, strings.Repeat(" ", statementIndent)+"end;")
					continue
				}
				if len(statementFields) == 2 && statementFields[0] == "lights_motion_guarded" {
					outputLines = append(outputLines, strings.Repeat(" ", statementIndent)+"lights_motion_guarded with delay "+statementFields[1]+";")
					continue
				}
				if len(statementFields) == 4 && (statementFields[0] == "sunny" || statementFields[0] == "windy") {
					outputLines = append(outputLines, strings.Repeat(" ", statementIndent)+statementFields[0]+" "+statementFields[1]+" with:")
					outputLines = append(outputLines, strings.Repeat(" ", statementIndent+2)+"delay_on "+statementFields[2]+";")
					outputLines = append(outputLines, strings.Repeat(" ", statementIndent+2)+"delay_off "+statementFields[3]+";")
					outputLines = append(outputLines, strings.Repeat(" ", statementIndent)+"end;")
					continue
				}
				if len(statementFields) >= 6 && statementFields[0] == "zigbee_group" {
					target := statementFields[1] + "." + statementFields[2] + ":" + statementFields[3]
					groupValues := strings.Join(statementFields[4:], ", ")
					outputLines = append(outputLines, strings.Repeat(" ", statementIndent)+"zigbee_group "+target+" with:")
					outputLines = append(outputLines, strings.Repeat(" ", statementIndent+2)+"group { "+groupValues+" };")
					outputLines = append(outputLines, strings.Repeat(" ", statementIndent)+"end;")
					continue
				}
				if len(statementFields) == 3 && statementFields[0] == "battery_alert" {
					outputLines = append(outputLines, strings.Repeat(" ", statementIndent)+"battery_alert "+statementFields[1]+" with:")
					outputLines = append(outputLines, strings.Repeat(" ", statementIndent+2)+"alert_level "+statementFields[2]+";")
					outputLines = append(outputLines, strings.Repeat(" ", statementIndent)+"end;")
					continue
				}
				if len(statementFields) == 3 && statementFields[0] == "battery_level_device" {
					outputLines = append(outputLines, strings.Repeat(" ", statementIndent)+"battery_level_device "+statementFields[1]+" with:")
					outputLines = append(outputLines, strings.Repeat(" ", statementIndent+2)+"alert_level "+statementFields[2]+";")
					outputLines = append(outputLines, strings.Repeat(" ", statementIndent)+"end;")
					continue
				}
				if len(statementFields) == 4 && statementFields[0] == "media_player" {
					outputLines = append(outputLines, strings.Repeat(" ", statementIndent)+"media_player "+statementFields[1]+" with:")
					switch statementFields[2] {
					case "no_collect":
						outputLines = append(outputLines, strings.Repeat(" ", statementIndent+2)+"no_collect;")
						outputLines = append(outputLines, strings.Repeat(" ", statementIndent+2)+"enabler "+statementFields[3]+";")
					case "no_play":
						outputLines = append(outputLines, strings.Repeat(" ", statementIndent+2)+"no_play_input "+statementFields[3]+";")
					default:
						outputLines = append(outputLines, strings.Repeat(" ", statementIndent+2)+"enabler "+statementFields[2]+";")
						outputLines = append(outputLines, strings.Repeat(" ", statementIndent+2)+"delay_off "+statementFields[3]+";")
					}
					outputLines = append(outputLines, strings.Repeat(" ", statementIndent)+"end;")
					continue
				}
				if len(statementFields) == 3 && statementFields[0] == "media_player" {
					outputLines = append(outputLines, strings.Repeat(" ", statementIndent)+"media_player "+statementFields[1]+" with:")
					outputLines = append(outputLines, strings.Repeat(" ", statementIndent+2)+"enabler "+statementFields[2]+";")
					outputLines = append(outputLines, strings.Repeat(" ", statementIndent)+"end;")
					continue
				}
				if len(statementFields) == 3 && statementFields[0] == "light_device" {
					outputLines = append(outputLines, strings.Repeat(" ", statementIndent)+"light_device "+statementFields[1]+":"+statementFields[2]+";")
					continue
				}
				if len(statementFields) == 2 && statementFields[0] == "open_stop_close" && statementFields[1] == "no_collect" {
					outputLines = append(outputLines, strings.Repeat(" ", statementIndent)+"open_stop_close;")
					outputLines = append(outputLines, strings.Repeat(" ", statementIndent)+"no_collect;")
				} else {
					outputLines = append(outputLines, strings.Repeat(" ", statementIndent)+statementNoCreate+";")
				}
			}
		}
	}

	for len(openBlocks) > 0 {
		lastBlock := openBlocks[len(openBlocks)-1]
		outputLines = append(outputLines, strings.Repeat(" ", lastBlock.EndIndent)+"end;")
		openBlocks = openBlocks[:len(openBlocks)-1]
	}

	// Collapse single-statement "with:" blocks into inline "with" form.
	// Pattern: "<indent>header with:\n<indent+>body;\n<indent>end;" → "<indent>header with body;"
	collapsed := []string{}
	i := 0
	for i < len(outputLines) {
		line := outputLines[i]
		trimmed := strings.TrimSpace(line)
		if strings.HasSuffix(trimmed, " with:") && i+2 < len(outputLines) {
			bodyLine := outputLines[i+1]
			endLine := outputLines[i+2]
			bodyTrimmed := strings.TrimSpace(bodyLine)
			endTrimmed := strings.TrimSpace(endLine)
			indent := len(line) - len(strings.TrimLeft(line, " \t"))
			endIndent := len(endLine) - len(strings.TrimLeft(endLine, " \t"))
			// Only collapse if body has no nested "with:" and end; is at the same indent.
			if endTrimmed == "end;" && endIndent == indent && !strings.Contains(bodyTrimmed, " with:") {
				header := strings.TrimSuffix(trimmed, " with:")
				inlineLine := strings.Repeat(" ", indent) + header + " with " + bodyTrimmed
				if len(inlineLine) > 100 {
					// Long lines stay explicit to keep structure clear.
					collapsed = append(collapsed, strings.Repeat(" ", indent)+header+" with:")
					collapsed = append(collapsed, strings.Repeat(" ", indent+2)+bodyTrimmed)
					collapsed = append(collapsed, strings.Repeat(" ", indent)+"end;")
				} else {
					collapsed = append(collapsed, inlineLine)
				}
				i += 3
				continue
			}
		}
		collapsed = append(collapsed, line)
		i++
	}

	return strings.TrimRight(strings.Join(collapsed, "\n"), "\n") + "\n"
}

func transformListsDefinition(content string) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	outputLines := []string{}
	pendingBlankLines := []string{}
	insideList := false
	listHeaderIndent := 0

	flushBlankLines := func() {
		if len(pendingBlankLines) == 0 {
			return
		}
		outputLines = append(outputLines, pendingBlankLines...)
		pendingBlankLines = nil
	}
	closeList := func() {
		if !insideList {
			return
		}
		outputLines = append(outputLines, strings.Repeat(" ", listHeaderIndent)+"end;")
		insideList = false
	}

	for _, rawLine := range lines {
		trimmedLine := strings.TrimSpace(rawLine)
		if trimmedLine == "" {
			pendingBlankLines = append(pendingBlankLines, "")
			continue
		}

		indentWidth := len(leadingWhitespace(rawLine))

		if strings.HasPrefix(trimmedLine, "list ") {
			closeList()
			flushBlankLines()
			listHeaderIndent = 0
			outputLines = append(outputLines, strings.Repeat(" ", listHeaderIndent)+trimTrailingPunctuation(trimmedLine)+" with:")
			insideList = true
			continue
		}

		if insideList && indentWidth <= listHeaderIndent && !strings.HasPrefix(trimmedLine, "#") {
			closeList()
		}
		flushBlankLines()

		if strings.HasPrefix(trimmedLine, "#") {
			if insideList {
				outputLines = append(outputLines, strings.Repeat(" ", listHeaderIndent+2)+trimmedLine)
			} else {
				outputLines = append(outputLines, trimmedLine)
			}
			continue
		}
		if insideList {
			outputLines = append(outputLines, strings.Repeat(" ", listHeaderIndent+2)+trimTrailingPunctuation(trimmedLine)+";")
		} else {
			outputLines = append(outputLines, trimTrailingPunctuation(trimmedLine)+";")
		}
	}

	closeList()
	flushBlankLines()

	return strings.TrimRight(strings.Join(outputLines, "\n"), "\n") + "\n"
}

func parseAssignments(content string) []TAssignment {
	assignments := []TAssignment{}
	for _, rawLine := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		assignment, matched := parseAssignmentLine(strings.TrimSpace(rawLine))
		if matched {
			assignments = append(assignments, assignment)
		}
	}
	return assignments
}

func parseAssignmentLine(line string) (TAssignment, bool) {
	assignmentPattern := regexp.MustCompile(`^(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)=(.*)$`)
	matches := assignmentPattern.FindStringSubmatch(line)
	if matches == nil {
		return TAssignment{}, false
	}
	return TAssignment{Name: matches[1], Value: unquoteShellValue(matches[2])}, true
}

func unquoteShellValue(value string) string {
	trimmedValue := strings.TrimSpace(value)
	if len(trimmedValue) >= 2 {
		if strings.HasPrefix(trimmedValue, `"`) && strings.HasSuffix(trimmedValue, `"`) {
			return strings.TrimSuffix(strings.TrimPrefix(trimmedValue, `"`), `"`)
		}
		if strings.HasPrefix(trimmedValue, `'`) && strings.HasSuffix(trimmedValue, `'`) {
			return strings.TrimSuffix(strings.TrimPrefix(trimmedValue, `'`), `'`)
		}
	}
	return trimmedValue
}

func hasIndentedContinuation(lines []string, index int) bool {
	currentIndentWidth := len(leadingWhitespace(lines[index]))
	for nextIndex := index + 1; nextIndex < len(lines); nextIndex++ {
		nextTrimmedLine := strings.TrimSpace(lines[nextIndex])
		if nextTrimmedLine == "" || strings.HasPrefix(nextTrimmedLine, "#") {
			continue
		}
		nextIndentWidth := len(leadingWhitespace(lines[nextIndex]))
		return nextIndentWidth > currentIndentWidth
	}
	return false
}

func leadingWhitespace(line string) string {
	trimmedLeftLine := strings.TrimLeft(line, " \t")
	return line[:len(line)-len(trimmedLeftLine)]
}

func trimTrailingPunctuation(line string) string {
	return strings.TrimRight(strings.TrimRight(strings.TrimSpace(line), ";"), ":")
}

func normalizeEntitySyntaxText(line string) string {
	trimmedLine := strings.TrimSpace(line)
	if strings.HasPrefix(trimmedLine, "entity declare ") {
		trimmedLine = "declare entity " + strings.TrimPrefix(trimmedLine, "entity declare ")
	}
	if strings.HasPrefix(trimmedLine, "entity ") {
		trimmedLine = "create " + strings.TrimPrefix(trimmedLine, "entity ")
	}
	if strings.HasPrefix(trimmedLine, "define value ") {
		trimmedLine = "definition as value " + strings.TrimPrefix(trimmedLine, "define value ")
	}
	if strings.HasPrefix(trimmedLine, "defining value ") {
		trimmedLine = "definition as value " + strings.TrimPrefix(trimmedLine, "defining value ")
	}
	if strings.HasPrefix(trimmedLine, "value ") {
		trimmedLine = "definition as value " + strings.TrimPrefix(trimmedLine, "value ")
	}
	if strings.HasPrefix(trimmedLine, "define satisfies ") {
		trimmedLine = "definition as condition " + strings.TrimPrefix(trimmedLine, "define satisfies ")
	}
	if strings.HasPrefix(trimmedLine, "condition ") {
		trimmedLine = "definition as condition " + strings.TrimPrefix(trimmedLine, "condition ")
	}
	if strings.HasPrefix(trimmedLine, "define adjust ") {
		trimmedLine = "definition as adjustment " + strings.TrimPrefix(trimmedLine, "define adjust ")
	}
	if strings.HasPrefix(trimmedLine, "adjustment ") {
		trimmedLine = "definition as adjustment " + strings.TrimPrefix(trimmedLine, "adjustment ")
	}
	if strings.HasPrefix(trimmedLine, "define flip ") {
		trimmedLine = "definition as flipped " + strings.TrimPrefix(trimmedLine, "define flip ")
	}
	if strings.HasPrefix(trimmedLine, "flipped ") {
		trimmedLine = "definition as flipped " + strings.TrimPrefix(trimmedLine, "flipped ")
	}
	trimmedLine = strings.Replace(trimmedLine, " with condition ", " with definition as condition ", 1)
	trimmedLine = strings.Replace(trimmedLine, " with value ", " with definition as value ", 1)
	trimmedLine = strings.Replace(trimmedLine, " with adjustment ", " with definition as adjustment ", 1)
	if strings.HasPrefix(trimmedLine, "begin_virtual_space ") {
		trimmedLine = "virtual space " + strings.TrimPrefix(trimmedLine, "begin_virtual_space ")
	}
	if strings.HasPrefix(trimmedLine, "light_on ") {
		trimmedLine = "light on: " + strings.TrimPrefix(trimmedLine, "light_on ")
	}
	if strings.HasPrefix(trimmedLine, "space_off ") {
		trimmedLine = "space off: " + strings.TrimPrefix(trimmedLine, "space_off ")
	}
	if strings.HasPrefix(trimmedLine, "space_on ") {
		trimmedLine = "space on: " + strings.TrimPrefix(trimmedLine, "space_on ")
	}
	if strings.HasPrefix(trimmedLine, "heating_leak ") {
		trimmedLine = "heating leak " + strings.TrimPrefix(trimmedLine, "heating_leak ")
	}
	if strings.HasPrefix(trimmedLine, "import rest ") {
		trimmedLine = "imported rest " + strings.TrimPrefix(trimmedLine, "import rest ")
	}
	if strings.HasPrefix(trimmedLine, "define ") {
		trimmedLine = "definition as " + strings.TrimPrefix(trimmedLine, "define ")
	}
	if strings.HasPrefix(trimmedLine, "definition ") && !strings.HasPrefix(trimmedLine, "definition as ") {
		trimmedLine = "definition as " + strings.TrimPrefix(trimmedLine, "definition ")
	}
	trimmedLine = strings.ReplaceAll(trimmedLine, "defined as ", "definition as ")
	if strings.HasPrefix(trimmedLine, "provides device_node ") {
		trimmedLine = "providing node " + strings.TrimPrefix(trimmedLine, "provides device_node ")
	}
	if strings.HasPrefix(trimmedLine, "declare entity ") {
		trimmedLine = "entity " + strings.TrimPrefix(trimmedLine, "declare entity ")
	}
	legacyRawReferencePattern := regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\._:/([A-Za-z0-9_]+)\b`)
	trimmedLine = legacyRawReferencePattern.ReplaceAllString(trimmedLine, `${1}.[${2}]`)
	trimmedLine = strings.ReplaceAll(trimmedLine, "sun.sun:/!", "sun.[sun]!")
	if trimmedLine == "entity sun.sun" {
		trimmedLine = "entity sun.[sun]"
	}
	return trimmedLine
}

func splitShellFields(line string) []string {
	fields := []string{}
	current := strings.Builder{}
	inQuote := false
	var quoteChar rune

	for _, currentRune := range line {
		switch {
		case inQuote && currentRune == quoteChar:
			inQuote = false
		case !inQuote && (currentRune == '"' || currentRune == '\''):
			inQuote = true
			quoteChar = currentRune
		case !inQuote && (currentRune == ' ' || currentRune == '\t'):
			if current.Len() > 0 {
				fields = append(fields, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(currentRune)
		}
	}
	if current.Len() > 0 {
		fields = append(fields, current.String())
	}
	return fields
}

func orderedAssignmentNames(assignments []TAssignment) []string {
	names := []string{}
	for _, assignment := range assignments {
		names = appendUnique(names, assignment.Name)
	}
	return names
}

func filterAssignments(assignments []TAssignment, include func(TAssignment) bool) []TAssignment {
	filteredAssignments := []TAssignment{}
	for _, assignment := range assignments {
		if include(assignment) {
			filteredAssignments = append(filteredAssignments, assignment)
		}
	}
	return filteredAssignments
}

func appendMissingAssignments(existingAssignments []TAssignment, newAssignments []TAssignment) []TAssignment {
	seenAssignments := map[string]bool{}
	for _, assignment := range existingAssignments {
		seenAssignments[assignment.Name] = true
	}
	for _, assignment := range newAssignments {
		if seenAssignments[assignment.Name] {
			continue
		}
		existingAssignments = append(existingAssignments, assignment)
		seenAssignments[assignment.Name] = true
	}
	return existingAssignments
}

func isLegacyIconSetting(assignment TAssignment) bool {
	return strings.HasSuffix(assignment.Name, "Icon")
}

func appendUnique(items []string, item string) []string {
	for _, existingItem := range items {
		if existingItem == item {
			return items
		}
	}
	return append(items, item)
}

func looksSensitive(name string) bool {
	lowerName := strings.ToLower(name)
	return strings.Contains(lowerName, "token") ||
		strings.Contains(lowerName, "password") ||
		strings.Contains(lowerName, "authorization") ||
		strings.HasSuffix(lowerName, "key")
}

func isObsoleteLegacySecretName(name string) bool {
	switch name {
	case "smarty_key", "telnet_password", "telnet_port", "volvo_login", "volvo_password", "xiaomi_token", "zigbee_deconz_key", "zigbee_importer_key", "zwave_deconz_home_id", "zwave_zwave_home_id":
		return true
	case "junglinster_authorization":
		return true
	case "rest_authorization_xanadu":
		return true
	default:
		return false
	}
}

func sanitizeName(name string) string {
	sanitized := strings.Builder{}
	for _, currentRune := range name {
		if (currentRune >= 'A' && currentRune <= 'Z') || (currentRune >= 'a' && currentRune <= 'z') || (currentRune >= '0' && currentRune <= '9') || currentRune == '_' {
			sanitized.WriteRune(currentRune)
		} else {
			sanitized.WriteRune('_')
		}
	}
	if sanitized.Len() == 0 {
		return "value"
	}
	return sanitized.String()
}

func writeGeneratedHeader(builder *strings.Builder, house, fileName, source string) {
	builder.WriteString("# Migrated preview; generated on 18.03.2026.\n")
	fmt.Fprintf(builder, "# House: %s\n", house)
	fmt.Fprintf(builder, "# Target: %s\n", fileName)
	fmt.Fprintf(builder, "# Source: %s\n\n", source)
}

func readOptionalFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(content), nil
}

func runExpansion(root string, houses []string) error {
	for _, house := range houses {
		if err := expandHouse(root, house); err != nil {
			return fmt.Errorf("error expanding %s: %w", house, err)
		}
		fmt.Printf("expanded %s\n", house)
	}
	return nil
}

func expandHouse(root string, house string) error {
	definitionDir := filepath.Join(root, "New", house, "Definitions")

	// Read Macros.def
	macrosPath := filepath.Join(definitionDir, "Macros.def")
	macrosContent, err := os.ReadFile(macrosPath)
	if err != nil {
		return fmt.Errorf("error reading macros: %w", err)
	}

	// Parse creation macros
	macroLines := strings.Split(string(macrosContent), "\n")
	ctx := &TMacroExpansionContext{
		Macros: make(map[string]*TParsedCreationMacro),
		Config: TExpanderConfig{Verbose: false, CheckTypes: true},
	}

	idx := 0
	for idx < len(macroLines) {
		macro, nextIdx, err := ctx.ParseCreationMacro(macroLines, idx)
		if err != nil {
			return fmt.Errorf("error parsing macro at line %d: %w", idx+1, err)
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
		return fmt.Errorf("strict macro validation failed in %s: %w", macrosPath, err)
	}

	// Read Entities.def
	entitiesPath := filepath.Join(definitionDir, "Entities.def")
	entitiesContent, err := os.ReadFile(entitiesPath)
	if err != nil {
		return fmt.Errorf("error reading entities: %w", err)
	}

	output := strings.Builder{}

	output.WriteString("=== MACRO EXPANSION REPORT ===\n")
	output.WriteString(fmt.Sprintf("House: %s\n", house))
	output.WriteString("Generated: 20.03.2026\n\n")

	// Write macro definitions summary
	output.WriteString("=== CREATION MACRO DEFINITIONS ===\n\n")
	for name := range ctx.Macros {
		macro := ctx.Macros[name]
		output.WriteString(fmt.Sprintf("creation macro %s { ", name))
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
	output.WriteString("Availability check status: not checked (offline mode).\n\n")
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

	// --- Generate Collections report ---
	collections := strings.Builder{}

	collections.WriteString("=== ENTITY COLLECTIONS REPORT ===\n")
	collections.WriteString(fmt.Sprintf("House: %s\n", house))
	collections.WriteString("Shows explicit aggregations: which constituent entities are collected into aggregated entities.\n")
	collections.WriteString("Entities marked [no_collect] are excluded from aggregation.\n\n")

	// Per-space aggregations
	collections.WriteString("=== AGGREGATIONS BY SPACE ===\n\n")
	allAggregations := []struct {
		spaceName     string
		depth         int
		aggregateName string
		constituents  []string
	}{}

	for _, spaceName := range admin.SpaceOrder {
		// Get aggregates for this space
		aggregates := impliedAggregatesForSpace(spaceName, admin.SpaceKindByName[spaceName], admin.SpaceOrder, admin.EntityRecordsBySpace)
		if len(aggregates) == 0 {
			continue
		}

		// For each aggregate, find its constituent entities
		for _, aggregateName := range aggregates {
			// Determine which entities contribute to this aggregate
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

	// Sort aggregations by space name for consistent output
	sort.Slice(allAggregations, func(i, j int) bool {
		if allAggregations[i].spaceName != allAggregations[j].spaceName {
			return allAggregations[i].spaceName < allAggregations[j].spaceName
		}
		return allAggregations[i].aggregateName < allAggregations[j].aggregateName
	})

	// Output aggregations grouped by space
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

	// Space-level collections (SpaceOn and SpaceOff)
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

// parseSpaceCollectionItems parses items like "social:main @light switch.social:picture_frame"
// Returns a list of item names
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
		if strings.HasPrefix(lower, "definition as ") || strings.HasPrefix(lower, "imported ") {
			hasDefinitionOrImported = true
			return
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

	// Keep raw-name entities as-is, e.g. sun.[sun]
	if strings.Contains(spec, ".[") {
		return spec
	}

	dotIdx := strings.Index(spec, ".")
	if dotIdx <= 0 || dotIdx >= len(spec)-1 {
		return strings.ReplaceAll(spec, ":", "/")
	}

	typePart := spec[:dotIdx]
	remainder := spec[dotIdx+1:]
	colonIdx := strings.Index(remainder, ":")
	var spherePart, rawPathPart string
	if colonIdx < 0 {
		// No sphere specified: fill in default sphere
		// Try lookupDefaultSphere(typePart), else default to "social"
		spherePart, _ = lookupDefaultSphere(typePart)
		if spherePart == "" {
			spherePart = "social"
		}
		rawPathPart = remainder
	} else {
		spherePart = remainder[:colonIdx]
		rawPathPart = remainder[colonIdx+1:]
	}

	// Internal normalized raw references use sphere "_" and a leading slash path.
	// Render these back to the external raw syntax expected in reports.
	if spherePart == "_" {
		rawName := strings.TrimPrefix(rawPathPart, "/")
		if rawName != "" {
			return fmt.Sprintf("%s.[%s]", typePart, rawName)
		}
	}
	pathPart := rawPathPart
	pathPart = strings.ReplaceAll(pathPart, ":", "/")
	_, contextPath := normalizeSpaceContext(spacePath)

	// Avoid double sphere (e.g., social.social) in the normalized name
	// If the first segment of contextPath matches the spherePart, skip it
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
			// For the first segment, use the name as-is
			segmentName = strings.ReplaceAll(segmentName, ":", "/")
			segmentName = strings.Trim(segmentName, "/")
			if segmentName != "" {
				contextParts = append(contextParts, strings.Split(segmentName, "/")...)
			}
			first = false
		} else {
			// For subsequent segments, ignore any type/sphere prefix, just use the name
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

	// Restore sensor aggregation (by metric)
	sensorMetricAggregates := map[string]bool{}
	includeSensorAggregates := spaceKind != SpaceKindVirtual

	// Check for any switch.social/<space>/media or /space entities
	hasMediaSwitch := false
	hasSpaceSwitch := false
	for _, record := range entityRecordsBySpace[spaceName] {
		if record.NoCollect {
			continue
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
	// Avoid double sphere (e.g., switch.social/social/...) in aggregate names
	spacePath := spaceName
	if strings.HasPrefix(spacePath, "social/") {
		spacePath = strings.TrimPrefix(spacePath, "social/")
	}
	if hasMediaSwitch {
		aggregates = append(aggregates, fmt.Sprintf("switch.social/%s/media", spacePath))
	}
	if hasSpaceSwitch {
		aggregates = append(aggregates, fmt.Sprintf("switch.social/%s/space", spacePath))
	}

	// Add sensor aggregates
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
		// Raw names like [some_raw_value] don't have a traditional domain:sphere
		return "raw"
	}

	// For normal identities, return domain:sphere as subdomain
	if identity.Domain != "" {
		if identity.Sphere != "" {
			return fmt.Sprintf("%s:%s", identity.Domain, identity.Sphere)
		}
		return identity.Domain
	}

	return ""
}

func extractDomainFromAggregate(aggregate string) string {
	// aggregate format: "domain.spaceName" or "sensor.spaceName/metric"
	parts := strings.Split(aggregate, ".")
	if len(parts) > 0 {
		return parts[0]
	}
	return "unknown"
}

// findAggregateConstituents finds constituents for an aggregation in a hierarchical way.
// For a space, includes:
// - Entities directly in that space (not in child spaces)
// - Aggregated entities from direct child spaces
func findAggregateConstituents(aggregateName, spaceName string, spaceOrder []string, entityRecordsBySpace map[string][]TEntityRecord) []string {
	constituents := []string{}

	// Handle switch.social/<space>/media and /space
	if strings.HasPrefix(aggregateName, "switch.social/") {
		for _, rec := range entityRecordsBySpace[spaceName] {
			if rec.NoCollect {
				continue
			}
			if rec.Identity.Domain == "switch" && rec.Identity.Sphere == "social" {
				if rec.Name == aggregateName {
					continue // skip self
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

	// Handle sensor.<space>/<metric> aggregates
	if strings.HasPrefix(aggregateName, "sensor.") {
		// Parse metric
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

// normalizeFullNames ensures all names are in full normalized format (e.g., social:living_room)
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

func collectExpandedEntityRecords(ctx *TMacroExpansionContext, invocation *TMacroInvocation, spacePath []string, inheritedNoCollect bool) ([]TCollectedExpandedEntityRecord, error) {
	macro, exists := ctx.Macros[invocation.Name]
	if !exists {
		return nil, fmt.Errorf("unknown macro: %s", invocation.Name)
	}

	canonicalParameters := canonicalInvocationParameters(invocation.Parameters)
	substitutions := map[string]string{
		"$entity": invocation.Target,
		"$sphere": extractSphere(invocation.Target),
	}
	for _, param := range macro.Parameters {
		if value, provided := canonicalParameters[normalizeParameterKey(param.Name)]; provided {
			substitutions[param.Name] = value
		}
	}

	expandedLines := []string{}
	for _, bodyLine := range macro.Body {
		expandedLine := bodyLine
		for placeholder, value := range substitutions {
			expandedLine = strings.ReplaceAll(expandedLine, placeholder, value)
		}
		expandedLines = append(expandedLines, expandedLine)
	}

	records := []TCollectedExpandedEntityRecord{}
	currentSpacePath := append([]string{}, spacePath...)
	openBlocks := []string{}
	currentNoCollect := inheritedNoCollect || invocationHasNoCollect(invocation)

	for _, rawLine := range expandedLines {
		trimmed := strings.TrimSpace(rawLine)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if entityDecl, ok := extractExpandedEntityDeclaration(trimmed); ok {
			fullName := normalizeEntityFullName(entityDecl.Specification, currentSpacePath)
			records = append(records, TCollectedExpandedEntityRecord{
				SpacePath: append([]string{}, currentSpacePath...),
				Record: TEntityRecord{
					Name:                  fullName,
					Identity:              extractEntityIdentity(fullName),
					NoCollect:             entityDecl.NoCollect || currentNoCollect,
					HasDefinitionOrImport: true,
				},
			})
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

			nestedRecords, nestedErr := collectExpandedEntityRecords(ctx, parsedInvocation, currentSpacePath, currentNoCollect)
			if nestedErr != nil {
				return nil, nestedErr
			}
			records = append(records, nestedRecords...)
			continue
		}

		if spaceKind, spaceName, ok := parseSpaceHeader(trimmed); ok {
			openBlocks = append(openBlocks, spaceKind)
			if spaceName == "" {
				currentSpacePath = append(currentSpacePath, "?")
			} else {
				currentSpacePath = append(currentSpacePath, spaceName)
			}
			continue
		}

		if strings.HasSuffix(trimmed, "with:") {
			openBlocks = append(openBlocks, "other")
			continue
		}

		if trimmed == "end;" {
			if len(openBlocks) == 0 {
				continue
			}

			last := openBlocks[len(openBlocks)-1]
			openBlocks = openBlocks[:len(openBlocks)-1]
			if (last == SpaceKindRegular || last == SpaceKindVirtual) && len(currentSpacePath) > 0 {
				currentSpacePath = currentSpacePath[:len(currentSpacePath)-1]
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
	if strings.HasPrefix(trimmed, "create ") {
		return trimmed, true
	}
	if strings.HasPrefix(trimmed, "entity ") {
		rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "entity "))
		fields := strings.Fields(rest)
		if len(fields) == 0 {
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

	// Check if line starts with "create" followed by a macro name
	startIdx := 0
	if parts[0] == "create" && len(parts) > 1 {
		startIdx = 1
	}

	macroName := parts[startIdx]
	_, exists := macros[macroName]
	return exists
}

func extractCreateInvocationMacroName(line string) (string, bool) {
	parts := strings.Fields(line)
	if len(parts) < 2 || parts[0] != "create" {
		return "", false
	}
	return parts[1], true
}

type THomeAssistantTarget struct {
	BaseURL         string
	Token           string
	InsecureSkipTLS bool
	StatesPath      string
}

type TImportedRestDependency struct {
	BridgeName string
	EntityID   string
	ScanEveryS int
}

type TBridgeRestDefinition struct {
	BridgeName    string
	EndpointExpr  string
	TokenExpr     string
	InsecureTLS   bool
	ResolvedURL   string
	ResolvedToken string
}

func runAvailabilityCheck(root string, houses []string) error {
	for _, house := range houses {
		if err := checkHouseAvailability(root, house); err != nil {
			return fmt.Errorf("error checking availability for %s: %w", house, err)
		}
		fmt.Printf("checked %s\n", house)
	}
	return nil
}

func checkHouseAvailability(root, house string) error {
	definitionDir := filepath.Join(root, "New", house, "Definitions")
	externalEntities, err := collectExternalEntities(definitionDir)
	if err != nil {
		return err
	}

	// Detect DSL paths that collide on the same derived entity ID (e.g. d/x_y/z vs d/x/y_z).
	entityIDToDSLNames := map[string][]string{}
	for _, fullName := range externalEntities {
		if id := toHomeAssistantEntityID(fullName); id != "" {
			entityIDToDSLNames[id] = append(entityIDToDSLNames[id], fullName)
		}
	}
	ambiguousIDs := []string{}
	for id, names := range entityIDToDSLNames {
		if len(names) > 1 {
			ambiguousIDs = append(ambiguousIDs, id)
		}
	}
	sort.Strings(ambiguousIDs)
	ambiguities := []string{}
	for _, id := range ambiguousIDs {
		names := entityIDToDSLNames[id]
		ambiguities = append(ambiguities, fmt.Sprintf("%s <- %s", id, strings.Join(names, " AND ")))
	}

	importedDependencies, err := collectImportedRestDependencies(definitionDir)
	if err != nil {
		return err
	}

	target, err := resolveHomeAssistantTarget(definitionDir)
	if err != nil {
		return err
	}
	bridgeTargets, err := resolveBridgeTargets(definitionDir)
	if err != nil {
		return err
	}

	httpTransport := http.DefaultTransport.(*http.Transport).Clone()
	if target.InsecureSkipTLS {
		httpTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	client := &http.Client{
		Timeout:   12 * time.Second,
		Transport: httpTransport,
	}
	availableEntityIDs, bulkErr := fetchAllEntityIDs(client, target)
	foundEntities := []string{}
	missing := []string{}
	requestErrors := []string{}

	for _, entityFullName := range externalEntities {
		entityID := toHomeAssistantEntityID(entityFullName)
		if entityID == "" {
			requestErrors = append(requestErrors, fmt.Sprintf("%s -> unable to map to Home Assistant entity_id", entityFullName))
			continue
		}

		if bulkErr == nil {
			if availableEntityIDs[entityID] {
				foundEntities = append(foundEntities, fmt.Sprintf("%s (%s)", entityFullName, entityID))
			} else {
				missing = append(missing, fmt.Sprintf("%s (%s)", entityFullName, entityID))
			}
			continue
		}

		exists, checkErr := checkEntityExists(client, target, entityID)
		if checkErr != nil {
			requestErrors = append(requestErrors, fmt.Sprintf("%s (%s): %v", entityFullName, entityID, checkErr))
			continue
		}
		if exists {
			foundEntities = append(foundEntities, fmt.Sprintf("%s (%s)", entityFullName, entityID))
		} else {
			missing = append(missing, fmt.Sprintf("%s (%s)", entityFullName, entityID))
		}
	}

	report := strings.Builder{}
	report.WriteString("=== ASSUMED ENTITY AVAILABILITY CHECK ===\n\n")
	report.WriteString(fmt.Sprintf("House: %s\n", house))
	report.WriteString(fmt.Sprintf("Target base URL: %s\n", target.BaseURL))
	if target.InsecureSkipTLS {
		report.WriteString("TLS verification: disabled (insecure mode)\n")
	} else {
		report.WriteString("TLS verification: strict\n")
	}
	if bulkErr == nil {
		report.WriteString("Check mode: bulk /api/states snapshot\n")
	} else {
		report.WriteString("Check mode: per-entity /api/states/<entity_id> fallback\n")
		report.WriteString(fmt.Sprintf("Bulk states endpoint unavailable: %v\n", bulkErr))
	}
	report.WriteString(fmt.Sprintf("Checked endpoint: %s\n", target.StatesPath))
	report.WriteString(fmt.Sprintf("Assumed external entities checked: %d\n", len(externalEntities)))
	report.WriteString(fmt.Sprintf("Found: %d\n", len(foundEntities)))
	report.WriteString(fmt.Sprintf("Missing: %d\n", len(missing)))
	report.WriteString(fmt.Sprintf("Request errors: %d\n", len(requestErrors)))
	report.WriteString(fmt.Sprintf("Entity ID ambiguities: %d\n\n", len(ambiguities)))

	report.WriteString("=== ENTITY ID AMBIGUITIES ===\n")
	if len(ambiguities) == 0 {
		report.WriteString("(none)\n")
	} else {
		report.WriteString("WARNING: the following entity IDs are derived from multiple distinct DSL paths.\n")
		report.WriteString("Consider renaming path segments that contain '_' to use '-' instead (e.g. living-room).\n")
		for _, item := range ambiguities {
			report.WriteString(fmt.Sprintf("- %s\n", item))
		}
	}
	report.WriteString("\n")

	report.WriteString("=== FOUND ASSUMED ENTITIES ===\n")
	if len(foundEntities) == 0 {
		report.WriteString("(none)\n")
	} else {
		for _, item := range foundEntities {
			report.WriteString(fmt.Sprintf("- %s\n", item))
		}
	}
	report.WriteString("\n")

	report.WriteString("=== MISSING ASSUMED ENTITIES ===\n")
	if len(missing) == 0 {
		report.WriteString("(none)\n")
	} else {
		for _, item := range missing {
			report.WriteString(fmt.Sprintf("- %s\n", item))
		}
	}
	report.WriteString("\n")

	report.WriteString("=== REQUEST ERRORS ===\n")
	if len(requestErrors) == 0 {
		report.WriteString("(none)\n")
	} else {
		for _, item := range requestErrors {
			report.WriteString(fmt.Sprintf("- %s\n", item))
		}
	}
	report.WriteString("\n")

	importedFoundEntities := []string{}
	importedMissing := []string{}
	importedErrors := []string{}
	// Cache bulk fetches per bridge to avoid repeated /api/states calls.
	bridgeBulkCache := map[string]map[string]bool{}
	bridgeBulkErrCache := map[string]error{}
	for _, dependency := range importedDependencies {
		bridgeTarget, exists := bridgeTargets[dependency.BridgeName]
		if !exists {
			importedErrors = append(importedErrors, fmt.Sprintf("bridge %q not declared in Bridges.def", dependency.BridgeName))
			continue
		}
		if bridgeTarget.Token == "" {
			importedErrors = append(importedErrors, fmt.Sprintf("bridge %q has no authorization token", dependency.BridgeName))
			continue
		}

		bridgeHTTPTransport := http.DefaultTransport.(*http.Transport).Clone()
		if bridgeTarget.InsecureSkipTLS {
			bridgeHTTPTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		}
		bridgeClient := &http.Client{Timeout: 12 * time.Second, Transport: bridgeHTTPTransport}

		if _, seen := bridgeBulkCache[dependency.BridgeName]; !seen {
			ids, bulkFetchErr := fetchAllEntityIDs(bridgeClient, bridgeTarget)
			bridgeBulkCache[dependency.BridgeName] = ids
			bridgeBulkErrCache[dependency.BridgeName] = bulkFetchErr
		}
		availableOnBridge := bridgeBulkCache[dependency.BridgeName]
		bridgeBulkErr := bridgeBulkErrCache[dependency.BridgeName]

		if bridgeBulkErr == nil {
			if availableOnBridge[dependency.EntityID] {
				importedFoundEntities = append(importedFoundEntities, fmt.Sprintf("%s (scan_interval=%ds, bridge=%s, url=%s)", dependency.EntityID, dependency.ScanEveryS, dependency.BridgeName, bridgeTarget.BaseURL))
			} else {
				importedMissing = append(importedMissing, fmt.Sprintf("%s (scan_interval=%ds) on bridge %s (%s)", dependency.EntityID, dependency.ScanEveryS, dependency.BridgeName, bridgeTarget.BaseURL))
			}
			continue
		}

		existsOnBridge, bridgeCheckErr := checkEntityExists(bridgeClient, bridgeTarget, dependency.EntityID)
		if bridgeCheckErr != nil {
			importedErrors = append(importedErrors, fmt.Sprintf("%s (scan_interval=%ds) on bridge %s (%s): %v", dependency.EntityID, dependency.ScanEveryS, dependency.BridgeName, bridgeTarget.BaseURL, bridgeCheckErr))
			continue
		}
		if existsOnBridge {
			importedFoundEntities = append(importedFoundEntities, fmt.Sprintf("%s (scan_interval=%ds, bridge=%s, url=%s)", dependency.EntityID, dependency.ScanEveryS, dependency.BridgeName, bridgeTarget.BaseURL))
		} else {
			importedMissing = append(importedMissing, fmt.Sprintf("%s (scan_interval=%ds) on bridge %s (%s)", dependency.EntityID, dependency.ScanEveryS, dependency.BridgeName, bridgeTarget.BaseURL))
		}
	}

	report.WriteString("=== IMPORTED REST SOURCE CHECK ===\n")
	report.WriteString(fmt.Sprintf("Imported dependencies checked: %d\n", len(importedDependencies)))
	report.WriteString("Note: the numeric parameter in 'imported rest' is scan_interval (seconds).\n")
	report.WriteString(fmt.Sprintf("Found on source bridge: %d\n", len(importedFoundEntities)))
	report.WriteString(fmt.Sprintf("Missing on source bridge: %d\n", len(importedMissing)))
	report.WriteString(fmt.Sprintf("Bridge request errors: %d\n\n", len(importedErrors)))

	report.WriteString("=== FOUND IMPORTED SOURCES ===\n")
	if len(importedFoundEntities) == 0 {
		report.WriteString("(none)\n")
	} else {
		for _, item := range importedFoundEntities {
			report.WriteString(fmt.Sprintf("- %s\n", item))
		}
	}
	report.WriteString("\n")

	report.WriteString("=== MISSING IMPORTED SOURCES ===\n")
	if len(importedMissing) == 0 {
		report.WriteString("(none)\n")
	} else {
		for _, item := range importedMissing {
			report.WriteString(fmt.Sprintf("- %s\n", item))
		}
	}
	report.WriteString("\n")

	report.WriteString("=== IMPORTED SOURCE ERRORS ===\n")
	if len(importedErrors) == 0 {
		report.WriteString("(none)\n")
	} else {
		for _, item := range importedErrors {
			report.WriteString(fmt.Sprintf("- %s\n", item))
		}
	}
	report.WriteString("\n")

	availabilityReportPath, writeErr := writeDebugReport(root, house, DebugReportAvailability, report.String())
	if writeErr != nil {
		return fmt.Errorf("error writing availability report: %w", writeErr)
	}
	if availabilityReportPath != "" {
		fmt.Printf("  availability report: %s\n", availabilityReportPath)
	}
	return nil
}

func collectExternalEntities(definitionDir string) ([]string, error) {
	entitiesPath := filepath.Join(definitionDir, "Entities.def")
	content, err := os.ReadFile(entitiesPath)
	if err != nil {
		return nil, fmt.Errorf("error reading entities: %w", err)
	}

	entityLines := strings.Split(string(content), "\n")
	spacePath := []string{}
	openBlocks := []string{}
	externalByName := map[string]bool{}

	for i := 0; i < len(entityLines); i++ {
		trimmed := strings.TrimSpace(entityLines[i])
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if entityDecl, ok := extractEntityDeclaration(trimmed); ok {
			fullName := normalizeEntityFullName(entityDecl.Specification, spacePath)
			hasDefOrImport, _ := analyzeEntityDefinitionContext(entityLines, i)
			if !hasDefOrImport {
				externalByName[fullName] = true
			}
		}

		if spaceKind, spaceName, ok := parseSpaceHeader(trimmed); ok {
			openBlocks = append(openBlocks, spaceKind)
			if spaceName == "" {
				spacePath = append(spacePath, "?")
			} else {
				spacePath = append(spacePath, spaceName)
			}
			continue
		}

		if strings.HasSuffix(trimmed, "with:") {
			openBlocks = append(openBlocks, "other")
			continue
		}

		if trimmed == "end;" {
			if len(openBlocks) == 0 {
				continue
			}

			last := openBlocks[len(openBlocks)-1]
			openBlocks = openBlocks[:len(openBlocks)-1]
			if (last == SpaceKindRegular || last == SpaceKindVirtual) && len(spacePath) > 0 {
				spacePath = spacePath[:len(spacePath)-1]
			}
		}
	}

	externalEntities := []string{}
	for fullName := range externalByName {
		externalEntities = append(externalEntities, fullName)
	}
	sort.Strings(externalEntities)
	return externalEntities, nil
}

func collectImportedRestDependencies(definitionDir string) ([]TImportedRestDependency, error) {
	entitiesPath := filepath.Join(definitionDir, "Entities.def")
	content, err := os.ReadFile(entitiesPath)
	if err != nil {
		return nil, fmt.Errorf("error reading entities: %w", err)
	}

	dependencies := []TImportedRestDependency{}
	seen := map[string]bool{}
	for _, rawLine := range strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(rawLine)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 5 || fields[0] != "imported" || fields[1] != "rest" {
			continue
		}
		bridgeName := strings.TrimSpace(fields[2])
		entityID := strings.TrimSpace(strings.TrimSuffix(fields[3], ";"))
		scanRaw := strings.TrimSpace(strings.TrimSuffix(fields[4], ";"))
		scanEveryS, scanErr := strconv.Atoi(scanRaw)
		if scanErr != nil || scanEveryS <= 0 {
			continue
		}
		if bridgeName == "" || entityID == "" {
			continue
		}
		key := bridgeName + "::" + entityID + "::" + strconv.Itoa(scanEveryS)
		if seen[key] {
			continue
		}
		seen[key] = true
		dependencies = append(dependencies, TImportedRestDependency{BridgeName: bridgeName, EntityID: entityID, ScanEveryS: scanEveryS})
	}

	sort.Slice(dependencies, func(i, j int) bool {
		if dependencies[i].BridgeName != dependencies[j].BridgeName {
			return dependencies[i].BridgeName < dependencies[j].BridgeName
		}
		return dependencies[i].EntityID < dependencies[j].EntityID
	})

	return dependencies, nil
}

func resolveBridgeTargets(definitionDir string) (map[string]THomeAssistantTarget, error) {
	serverPath := filepath.Join(definitionDir, "Server.def")
	settingsPath := filepath.Join(definitionDir, "Settings.def")
	secretsPath := filepath.Join(definitionDir, "Secrets.def")
	bridgesPath := filepath.Join(definitionDir, "Bridges.def")

	serverContent, _ := readOptionalFile(serverPath)
	settingsContent, _ := readOptionalFile(settingsPath)
	secretsContent, _ := readOptionalFile(secretsPath)
	bridgesContent, _ := readOptionalFile(bridgesPath)

	vars := parseServerAssignmentsWithAvailability(serverContent, isServerHostUp)
	for name, value := range parseDefinitionAssignments(settingsContent) {
		vars[name] = value
	}
	for name, value := range parseDefinitionAssignments(secretsContent) {
		vars[name] = value
	}

	bridgeTargets := map[string]THomeAssistantTarget{}
	for _, bridgeDef := range parseBridgeRestDefinitions(bridgesContent) {
		resolvedURL := resolveInterpolatedDefinitionValue(bridgeDef.EndpointExpr, vars)
		baseURL, statesPath := splitStatesEndpointURL(resolvedURL)
		if baseURL == "" {
			continue
		}
		token := resolveDefinitionReference(bridgeDef.TokenExpr, vars)
		bridgeTargets[bridgeDef.BridgeName] = THomeAssistantTarget{
			BaseURL:         strings.TrimSpace(strings.TrimSuffix(baseURL, "/")),
			Token:           strings.TrimSpace(token),
			InsecureSkipTLS: resolveDefinitionBoolReference([]string{"$MainAPITLSInsecure", "$MainAPITLSSkipVerify", "$MainAPIInsecureTLS"}, vars),
			StatesPath:      statesPath,
		}
	}

	return bridgeTargets, nil
}

func parseBridgeRestDefinitions(bridgesContent string) []TBridgeRestDefinition {
	definitions := []TBridgeRestDefinition{}
	pattern := regexp.MustCompile(`^bridge\s+rest\s+([A-Za-z_][A-Za-z0-9_]*)\s+(.+?)\s+authorization\s+(.+?)\s*;\s*$`)
	for _, rawLine := range strings.Split(strings.ReplaceAll(bridgesContent, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(rawLine)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		matches := pattern.FindStringSubmatch(trimmed)
		if matches == nil {
			continue
		}
		definitions = append(definitions, TBridgeRestDefinition{
			BridgeName:   strings.TrimSpace(matches[1]),
			EndpointExpr: strings.TrimSpace(matches[2]),
			TokenExpr:    strings.TrimSpace(matches[3]),
		})
	}
	return definitions
}

func resolveInterpolatedDefinitionValue(expression string, vars map[string]string) string {
	value := strings.TrimSpace(unquoteShellValue(expression))
	if !strings.HasPrefix(value, "$") {
		return value
	}

	nameEnd := 1
	for ; nameEnd < len(value); nameEnd++ {
		current := value[nameEnd]
		if !((current >= 'A' && current <= 'Z') || (current >= 'a' && current <= 'z') || (current >= '0' && current <= '9') || current == '_') {
			break
		}
	}
	name := value[1:nameEnd]
	resolvedVar, exists := vars[name]
	if !exists {
		return ""
	}

	return strings.TrimSpace(resolvedVar) + value[nameEnd:]
}

func splitStatesEndpointURL(endpoint string) (string, string) {
	trimmed := strings.TrimSpace(endpoint)
	if trimmed == "" {
		return "", ""
	}

	parsedURL, err := url.Parse(trimmed)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return "", ""
	}

	baseURL := fmt.Sprintf("%s://%s", parsedURL.Scheme, parsedURL.Host)
	statesPath := parsedURL.Path
	if statesPath == "" {
		statesPath = "/api/states"
	}
	if !strings.HasPrefix(statesPath, "/") {
		statesPath = "/" + statesPath
	}

	return baseURL, statesPath
}

func resolveHomeAssistantTarget(definitionDir string) (THomeAssistantTarget, error) {
	baseURL := ""
	token := ""
	insecureSkipTLS := false

	serverPath := filepath.Join(definitionDir, "Server.def")
	settingsPath := filepath.Join(definitionDir, "Settings.def")
	secretsPath := filepath.Join(definitionDir, "Secrets.def")

	serverContent, _ := readOptionalFile(serverPath)
	settingsContent, _ := readOptionalFile(settingsPath)
	secretsContent, _ := readOptionalFile(secretsPath)

	vars := parseServerAssignmentsWithAvailability(serverContent, isServerHostUp)
	for name, value := range parseDefinitionAssignments(settingsContent) {
		vars[name] = value
	}
	for name, value := range parseDefinitionAssignments(secretsContent) {
		vars[name] = value
	}

	mainTargetExpr := parseMainTargetExpression(serverContent)
	baseURL = resolveDefinitionReference(mainTargetExpr, vars)
	token = resolveDefinitionReference("$MainAPIToken", vars)
	insecureSkipTLS = resolveDefinitionBoolReference([]string{"$MainAPITLSInsecure", "$MainAPITLSSkipVerify", "$MainAPIInsecureTLS"}, vars)

	// Definitions are authoritative. Environment variables are only fallback when
	// required values are not present in Server.def / Secrets.def.
	if baseURL == "" {
		baseURL = firstNonEmptyEnv("HASS_BASE_URL", "HOMEASSISTANT_BASE_URL")
	}
	if token == "" {
		token = firstNonEmptyEnv("HASS_TOKEN", "HOMEASSISTANT_TOKEN")
	}
	if !insecureSkipTLS {
		insecureSkipTLS = parseBoolLike(firstNonEmptyEnv("HASS_INSECURE_SKIP_TLS_VERIFY", "HOMEASSISTANT_INSECURE_SKIP_TLS_VERIFY"))
	}

	baseURL = strings.TrimSpace(strings.TrimSuffix(baseURL, "/"))
	token = strings.TrimSpace(token)

	if baseURL == "" {
		return THomeAssistantTarget{}, fmt.Errorf("missing Home Assistant base URL; provide main target in Server.def or set HASS_BASE_URL/HOMEASSISTANT_BASE_URL")
	}
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		return THomeAssistantTarget{}, fmt.Errorf("invalid Home Assistant base URL %q; expected http:// or https://", baseURL)
	}
	if token == "" {
		return THomeAssistantTarget{}, fmt.Errorf("missing Home Assistant token; provide $MainAPIToken in Secrets.def or set HASS_TOKEN/HOMEASSISTANT_TOKEN")
	}

	return THomeAssistantTarget{BaseURL: baseURL, Token: token, InsecureSkipTLS: insecureSkipTLS, StatesPath: "/api/states"}, nil
}

func resolveDefinitionBoolReference(candidates []string, vars map[string]string) bool {
	for _, candidate := range candidates {
		resolved := resolveDefinitionReference(candidate, vars)
		if resolved == "" {
			continue
		}
		if parseBoolLike(resolved) {
			return true
		}
	}
	return false
}

func parseBoolLike(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	return normalized == "1" || normalized == "true" || normalized == "yes" || normalized == "y" || normalized == "on"
}

func parseDefinitionAssignments(content string) map[string]string {
	assignments := map[string]string{}

	for _, rawLine := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, matched := parseDefinitionAssignmentLine(line)
		if matched {
			assignments[name] = value
		}
	}

	return assignments
}

func parseDefinitionAssignmentLine(line string) (string, string, bool) {
	assignmentPattern := regexp.MustCompile(`^\$([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.+?)\s*;\s*$`)
	matches := assignmentPattern.FindStringSubmatch(strings.TrimSpace(line))
	if matches == nil {
		return "", "", false
	}
	name := matches[1]
	value := unquoteShellValue(strings.TrimSpace(matches[2]))
	return name, value, true
}

func parseServerAssignmentsWithAvailability(serverContent string, hostUp func(string) bool) map[string]string {
	assignments := map[string]string{}
	ifPattern := regexp.MustCompile(`^if\s+is\s+up\s+"([^"]+)"\s+then$`)
	elifPattern := regexp.MustCompile(`^elif\s+is\s+up\s+"([^"]+)"\s+then$`)

	inConditional := false
	branchSelected := false
	branchApplies := false

	for _, rawLine := range strings.Split(strings.ReplaceAll(serverContent, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if matches := ifPattern.FindStringSubmatch(line); matches != nil {
			inConditional = true
			branchSelected = false
			branchApplies = false
			host := strings.TrimSpace(matches[1])
			if host != "" && hostUp(host) {
				branchSelected = true
				branchApplies = true
			}
			continue
		}

		if matches := elifPattern.FindStringSubmatch(line); matches != nil {
			if !inConditional {
				continue
			}
			branchApplies = false
			if !branchSelected {
				host := strings.TrimSpace(matches[1])
				if host != "" && hostUp(host) {
					branchSelected = true
					branchApplies = true
				}
			}
			continue
		}

		if line == "else" {
			if inConditional && !branchSelected {
				branchSelected = true
				branchApplies = true
			} else {
				branchApplies = false
			}
			continue
		}

		if line == "end;" {
			if inConditional {
				inConditional = false
				branchSelected = false
				branchApplies = false
			}
			continue
		}

		name, value, matched := parseDefinitionAssignmentLine(line)
		if !matched {
			continue
		}
		if !inConditional || branchApplies {
			assignments[name] = value
		}
	}

	return assignments
}

func isServerHostUp(host string) bool {
	trimmedHost := strings.TrimSpace(host)
	if trimmedHost == "" {
		return false
	}

	targets := []string{}
	if strings.Contains(trimmedHost, ":") {
		targets = append(targets, trimmedHost)
	} else {
		targets = append(targets, net.JoinHostPort(trimmedHost, "8123"))
		targets = append(targets, net.JoinHostPort(trimmedHost, "443"))
		targets = append(targets, net.JoinHostPort(trimmedHost, "80"))
	}

	for _, target := range targets {
		conn, err := net.DialTimeout("tcp", target, 1500*time.Millisecond)
		if err != nil {
			continue
		}
		_ = conn.Close()
		return true
	}

	if strings.Contains(trimmedHost, ":") {
		return false
	}

	// Fall back to one ICMP probe for hosts that are reachable on LAN but do not expose these TCP ports.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ping", "-c", "1", trimmedHost)
	if err := cmd.Run(); err == nil {
		return true
	}

	return false
}

func parseMainTargetExpression(serverContent string) string {
	mainPattern := regexp.MustCompile(`^main\s+[A-Za-z_][A-Za-z0-9_]*\s+(.+?)\s*;\s*$`)
	for _, rawLine := range strings.Split(strings.ReplaceAll(serverContent, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(rawLine)
		matches := mainPattern.FindStringSubmatch(line)
		if matches != nil {
			return strings.TrimSpace(matches[1])
		}
	}
	return ""
}

func resolveDefinitionReference(expression string, vars map[string]string) string {
	value := strings.TrimSpace(unquoteShellValue(expression))
	for depth := 0; depth < 8; depth++ {
		if !strings.HasPrefix(value, "$") {
			return value
		}
		name := strings.TrimPrefix(value, "$")
		nextValue, exists := vars[name]
		if !exists {
			return ""
		}
		value = strings.TrimSpace(unquoteShellValue(nextValue))
	}
	return ""
}

func firstNonEmptyEnv(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

func toHomeAssistantEntityID(fullName string) string {
	trimmed := strings.TrimSpace(fullName)
	if trimmed == "" {
		return ""
	}

	if rawPattern := regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\.\[([^\]]+)\]$`); rawPattern.MatchString(trimmed) {
		matches := rawPattern.FindStringSubmatch(trimmed)
		if matches != nil {
			return fmt.Sprintf("%s.%s", matches[1], sanitizeObjectID(matches[2]))
		}
	}

	dotIdx := strings.Index(trimmed, ".")
	if dotIdx <= 0 || dotIdx >= len(trimmed)-1 {
		return ""
	}

	domain := trimmed[:dotIdx]
	object := trimmed[dotIdx+1:]
	object = strings.ReplaceAll(object, "/", "_")
	object = strings.ReplaceAll(object, ":", "_")
	object = strings.Trim(object, "_")
	object = sanitizeObjectID(object)
	if object == "" {
		return ""
	}

	return fmt.Sprintf("%s.%s", domain, object)
}

func sanitizeObjectID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	builder := strings.Builder{}
	lastUnderscore := false
	for _, r := range value {
		isAlnum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlnum {
			builder.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			builder.WriteRune('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(builder.String(), "_")
}

func checkEntityExists(client *http.Client, target THomeAssistantTarget, entityID string) (bool, error) {
	entityPath := url.PathEscape(entityID)
	statePath := strings.TrimSpace(target.StatesPath)
	if statePath == "" {
		statePath = "/api/states"
	}
	statePath = "/" + strings.TrimPrefix(strings.TrimSuffix(statePath, "/"), "/")
	endpoint := fmt.Sprintf("%s%s/%s", target.BaseURL, statePath, entityPath)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+target.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return true, nil
	}
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	return false, fmt.Errorf("api returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}

func fetchAllEntityIDs(client *http.Client, target THomeAssistantTarget) (map[string]bool, error) {
	statePath := strings.TrimSpace(target.StatesPath)
	if statePath == "" {
		statePath = "/api/states"
	}
	statePath = "/" + strings.TrimPrefix(strings.TrimSuffix(statePath, "/"), "/")
	endpoints := []string{statePath, statePath + "/"}
	errors := []string{}

	for _, path := range endpoints {
		endpoint := target.BaseURL + path
		req, err := http.NewRequest(http.MethodGet, endpoint, nil)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		req.Header.Set("Authorization", "Bearer "+target.Token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
		resp.Body.Close()
		if readErr != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", path, readErr))
			continue
		}

		if resp.StatusCode != http.StatusOK {
			snippet := strings.TrimSpace(string(body))
			if len(snippet) > 200 {
				snippet = snippet[:200]
			}
			errors = append(errors, fmt.Sprintf("%s: api returned %d: %s", path, resp.StatusCode, snippet))
			continue
		}

		entityIDs, parseErr := extractEntityIDsFromStatesPayload(body)
		if parseErr != nil {
			errors = append(errors, fmt.Sprintf("%s: invalid states payload: %v", path, parseErr))
			continue
		}

		return entityIDs, nil
	}

	return nil, fmt.Errorf(strings.Join(errors, "; "))
}

func extractEntityIDsFromStatesPayload(payload []byte) (map[string]bool, error) {
	states := []struct {
		EntityID string `json:"entity_id"`
	}{}
	if err := json.Unmarshal(payload, &states); err != nil {
		return nil, err
	}
	entityIDs := map[string]bool{}
	for _, state := range states {
		trimmed := strings.TrimSpace(state.EntityID)
		if trimmed != "" {
			entityIDs[trimmed] = true
		}
	}
	return entityIDs, nil
}
