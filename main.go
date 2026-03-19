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
 * Version of: 18.03.2026
 *
 */

package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var THouseNames = []string{"Vienna", "Junglinster"}

type TAssignment struct {
	Name  string
	Value string
}

type TServerBranch struct {
	Label       string
	Condition   string
	Assignments []TAssignment
	Otherwise   bool
}

type TMacro struct {
	Name      string
	Signature string
	Body      []string
	Source    string
}

type TMigrator struct {
	Root string
}

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
	default:
		// Backward compatibility: go run . Vienna Junglinster
		houses, err := resolveRequestedHouses(args)
		if err != nil {
			return err
		}
		return runMigration(root, houses)
	}
}

func runMigration(root string, houses []string) error {
	migrator := TMigrator{Root: root}
	for _, house := range houses {
		if err := migrator.MigrateHouse(house); err != nil {
			return fmt.Errorf("error migrating %s: %w", house, err)
		}
		fmt.Printf("migrated %s\n", house)
	}
	return nil
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

func (m *TMigrator) MigrateHouse(house string) error {
	definitionDir := filepath.Join(m.Root, "New", house, "Definitions")
	if err := os.MkdirAll(definitionDir, 0o755); err != nil {
		return err
	}

	settingsContent, err := m.BuildSettings(house)
	if err != nil {
		return err
	}
	serverContent, err := m.BuildServer(house)
	if err != nil {
		return err
	}
	bridgesContent, err := m.BuildBridges(house)
	if err != nil {
		return err
	}
	entitiesContent, err := m.BuildEntitiesDefinition(house)
	if err != nil {
		return err
	}
	listsContent, err := m.BuildListsDefinition(house)
	if err != nil {
		return err
	}
	macrosContent, err := m.BuildMacros(house)
	if err != nil {
		return err
	}

	outputs := map[string]string{
		"Settings.def": settingsContent,
		"Server.def":   serverContent,
		"Bridges.def":  bridgesContent,
		"Entities.def": entitiesContent,
		"Lists.def":    listsContent,
		"Macros.def":   macrosContent,
	}

	for fileName, content := range outputs {
		if err := os.WriteFile(filepath.Join(definitionDir, fileName), []byte(content), 0o644); err != nil {
			return err
		}
	}

	return nil
}

func (m *TMigrator) BuildSettings(house string) (string, error) {
	settingsDir := filepath.Join(m.Root, "Old", house, "Settings")
	assignments := []TAssignment{}
	for _, fileName := range []string{"general.def", "local.def"} {
		content, err := readOptionalFile(filepath.Join(settingsDir, fileName))
		if err != nil {
			return "", err
		}
		assignments = append(assignments, parseAssignments(content)...)
	}

	legacyModuleSettings, err := readOptionalFile(filepath.Join(m.Root, "Old", house, "Modules", "Settings.1.hass"))
	if err != nil {
		return "", err
	}
	assignments = appendMissingAssignments(assignments, filterAssignments(parseAssignments(legacyModuleSettings), isLegacyIconSetting))

	secretAssignments, err := readOptionalFile(filepath.Join(settingsDir, "secrets.def"))
	if err != nil {
		return "", err
	}
	secretNames := orderedAssignmentNames(parseAssignments(secretAssignments))

	builder := strings.Builder{}
	writeGeneratedHeader(&builder, house, "Settings.def", filepath.Join("Old", house, "Settings"))
	builder.WriteString("# Domain settings are retained here; secrets remain external.\n\n")
	builder.WriteString("settings:\n")
	for _, assignment := range assignments {
		fmt.Fprintf(&builder, "  %s %q;\n", assignment.Name, assignment.Value)
	}
	if len(assignments) == 0 {
		builder.WriteString("  # No non-secret settings were present in the legacy source.\n")
	}
	builder.WriteString("end;\n")

	if len(secretNames) > 0 {
		builder.WriteString("\n# External secret bindings detected in legacy Settings/secrets.def:\n")
		for _, secretName := range secretNames {
			fmt.Fprintf(&builder, "# - %s\n", secretName)
		}
	}

	return builder.String(), nil
}

func (m *TMigrator) BuildServer(house string) (string, error) {
	serverPath := filepath.Join(m.Root, "Old", house, "Definitions", "00_server.def")
	content, err := os.ReadFile(serverPath)
	if err != nil {
		return "", err
	}

	preBranchAssignments := []TAssignment{}
	postBranchAssignments := []TAssignment{}
	branches := []TServerBranch{}
	currentBranchIndex := -1
	insideBranching := false
	seenConditional := false

	ifPattern := regexp.MustCompile(`^if\s+ServerIsUp\s+\$([A-Za-z_][A-Za-z0-9_]*)\s*;\s*then$`)
	elifPattern := regexp.MustCompile(`^elif\s+ServerIsUp\s+\$([A-Za-z_][A-Za-z0-9_]*)\s*;\s*then$`)

	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		trimmedLine := strings.TrimSpace(scanner.Text())
		if trimmedLine == "" || strings.HasPrefix(trimmedLine, "#") {
			continue
		}

		if matches := ifPattern.FindStringSubmatch(trimmedLine); matches != nil {
			seenConditional = true
			insideBranching = true
			branches = append(branches, TServerBranch{Label: sanitizeName(matches[1]), Condition: matches[1]})
			currentBranchIndex = len(branches) - 1
			continue
		}
		if matches := elifPattern.FindStringSubmatch(trimmedLine); matches != nil {
			branches = append(branches, TServerBranch{Label: sanitizeName(matches[1]), Condition: matches[1]})
			currentBranchIndex = len(branches) - 1
			continue
		}
		if trimmedLine == "else" {
			branches = append(branches, TServerBranch{Label: "otherwise", Otherwise: true})
			currentBranchIndex = len(branches) - 1
			continue
		}
		if trimmedLine == "fi" {
			insideBranching = false
			currentBranchIndex = -1
			continue
		}

		assignment, matched := parseAssignmentLine(trimmedLine)
		if !matched {
			continue
		}

		if insideBranching && currentBranchIndex >= 0 {
			branches[currentBranchIndex].Assignments = append(branches[currentBranchIndex].Assignments, assignment)
			continue
		}

		if !seenConditional {
			preBranchAssignments = append(preBranchAssignments, assignment)
		} else {
			postBranchAssignments = append(postBranchAssignments, assignment)
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	derivedExternalNames := []string{}
	resultVariables := []string{}
	seenResultVariables := map[string]bool{}
	for _, branch := range branches {
		for _, assignment := range branch.Assignments {
			if !seenResultVariables[assignment.Name] {
				seenResultVariables[assignment.Name] = true
				resultVariables = append(resultVariables, assignment.Name)
			}
			derivedExternalNames = appendUnique(derivedExternalNames, fmt.Sprintf("%s__%s", assignment.Name, branch.Label))
		}
	}

	builder := strings.Builder{}
	writeGeneratedHeader(&builder, house, "Server.def", filepath.Join("Old", house, "Definitions", "00_server.def"))
	builder.WriteString("# The conditional routing logic is preserved here, but concrete deployment values stay external.\n\n")
	builder.WriteString("servers:\n")
	for _, assignment := range preBranchAssignments {
		fmt.Fprintf(&builder, "  external %s;\n", assignment.Name)
	}
	for _, externalName := range derivedExternalNames {
		fmt.Fprintf(&builder, "  external %s;\n", externalName)
	}
	for _, assignment := range postBranchAssignments {
		if looksSensitive(assignment.Name) {
			fmt.Fprintf(&builder, "  secret %s;\n", assignment.Name)
		} else {
			fmt.Fprintf(&builder, "  external %s;\n", assignment.Name)
		}
	}
	if len(preBranchAssignments) > 0 || len(derivedExternalNames) > 0 || len(postBranchAssignments) > 0 {
		builder.WriteString("\n")
	}
	for _, resultVariable := range resultVariables {
		fmt.Fprintf(&builder, "  choose %s:\n", resultVariable)
		for _, branch := range branches {
			for _, assignment := range branch.Assignments {
				if assignment.Name != resultVariable {
					continue
				}
				targetName := fmt.Sprintf("%s__%s", assignment.Name, branch.Label)
				if branch.Otherwise {
					fmt.Fprintf(&builder, "    otherwise %s;\n", targetName)
				} else {
					fmt.Fprintf(&builder, "    when server_is_up %s then %s;\n", branch.Condition, targetName)
				}
			}
		}
		builder.WriteString("  end;\n\n")
	}
	builder.WriteString("end;\n")

	return builder.String(), nil
}

func (m *TMigrator) BuildBridges(house string) (string, error) {
	bridgePath := filepath.Join(m.Root, "Old", house, "Definitions", "01_bridges.def")
	content, err := os.ReadFile(bridgePath)
	if err != nil {
		return "", err
	}

	secretNames := []string{}
	statementLines := []string{}

	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		trimmedLine := strings.TrimSpace(scanner.Text())
		if trimmedLine == "" {
			continue
		}
		if strings.HasPrefix(trimmedLine, "#") {
			statementLines = append(statementLines, "  "+trimmedLine)
			continue
		}

		fields := splitShellFields(trimmedLine)
		if len(fields) == 0 {
			continue
		}

		switch fields[0] {
		case "bridge":
			if len(fields) >= 5 && fields[1] == "rest" {
				secretName := fmt.Sprintf("%s_authorization", sanitizeName(fields[2]))
				secretNames = appendUnique(secretNames, secretName)
				statementLines = append(statementLines,
					fmt.Sprintf("  bridge rest %s %s authorization %s;", fields[2], fields[3], secretName),
				)
			} else {
				statementLines = append(statementLines, fmt.Sprintf("  %s;", strings.Join(fields, " ")))
			}
		case "main":
			statementLines = append(statementLines, fmt.Sprintf("  %s;", strings.Join(fields, " ")))
		default:
			statementLines = append(statementLines, fmt.Sprintf("  %s;", strings.Join(fields, " ")))
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	builder := strings.Builder{}
	writeGeneratedHeader(&builder, house, "Bridges.def", filepath.Join("Old", house, "Definitions", "01_bridges.def"))
	builder.WriteString("# Secret-bearing bridge parameters are externalized into named bindings.\n\n")
	builder.WriteString("bridges:\n")
	for _, secretName := range secretNames {
		fmt.Fprintf(&builder, "  secret %s;\n", secretName)
	}
	if len(secretNames) > 0 && len(statementLines) > 0 {
		builder.WriteString("\n")
	}
	for _, statementLine := range statementLines {
		builder.WriteString(statementLine)
		builder.WriteString("\n")
	}
	builder.WriteString("end;\n")

	return builder.String(), nil
}

func (m *TMigrator) BuildVerbatimDefinition(house, fileName, label string) (string, error) {
	definitionPath := filepath.Join(m.Root, "Old", house, "Definitions", fileName)
	content, err := os.ReadFile(definitionPath)
	if err != nil {
		return "", err
	}

	builder := strings.Builder{}
	writeGeneratedHeader(&builder, house, label+".def", filepath.Join("Old", house, "Definitions", fileName))
	builder.WriteString("# This file intentionally stays close to the legacy DSL while the parser grammar is still being fixed.\n\n")
	builder.WriteString(strings.TrimRight(string(content), "\n"))
	builder.WriteString("\n")
	return builder.String(), nil
}

func (m *TMigrator) BuildEntitiesDefinition(house string) (string, error) {
	definitionPath := filepath.Join(m.Root, "Old", house, "Definitions", "02_entities.def")
	content, err := os.ReadFile(definitionPath)
	if err != nil {
		return "", err
	}

	builder := strings.Builder{}
	writeGeneratedHeader(&builder, house, "Entities.def", filepath.Join("Old", house, "Definitions", "02_entities.def"))
	builder.WriteString("# This file keeps the legacy entity structure, but normalizes blocks using with:/end; and statement ';'.\n\n")
	builder.WriteString(transformEntitiesDefinition(string(content)))
	return builder.String(), nil
}

func (m *TMigrator) BuildListsDefinition(house string) (string, error) {
	definitionPath := filepath.Join(m.Root, "Old", house, "Definitions", "03_lists.def")
	content, err := os.ReadFile(definitionPath)
	if err != nil {
		return "", err
	}

	builder := strings.Builder{}
	writeGeneratedHeader(&builder, house, "Lists.def", filepath.Join("Old", house, "Definitions", "03_lists.def"))
	builder.WriteString("# This file keeps the legacy list selections, but normalizes them into explicit begin/end blocks.\n\n")
	builder.WriteString(transformListsDefinition(string(content)))
	return builder.String(), nil
}

func (m *TMigrator) BuildMacros(house string) (string, error) {
	curatedMacrosPath := filepath.Join(m.Root, "New", house, "Definitions", "Macros.def")
	curatedMacrosContent, err := readOptionalFile(curatedMacrosPath)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(curatedMacrosContent) != "" {
		return curatedMacrosContent, nil
	}

	macroFiles, err := filepath.Glob(filepath.Join(m.Root, "Old", house, "Macros", "*.def"))
	if err != nil {
		return "", err
	}
	sort.Strings(macroFiles)

	macros := []TMacro{}
	for _, macroPath := range macroFiles {
		content, err := os.ReadFile(macroPath)
		if err != nil {
			return "", err
		}
		macro, err := parseMacro(string(content), filepath.Base(macroPath))
		if err != nil {
			return "", err
		}
		macros = append(macros, macro)
	}

	builder := strings.Builder{}
	writeGeneratedHeader(&builder, house, "Macros.def", filepath.Join("Old", house, "Macros"))
	builder.WriteString("# Macros are collapsed into one file, but still form a prelude that should be read before other definitions.\n\n")
	builder.WriteString("macros:\n")
	for index, macro := range macros {
		parameterNames := inferMacroParameterNames(macro)
		fmt.Fprintf(&builder, "  %s\n", formatMacroHeader(macro.Name, parameterNames))
		fmt.Fprintf(&builder, "    # Source: %s\n", macro.Source)
		if signatureComment := normalizeMacroSignatureComment(macro.Signature); signatureComment != "" {
			fmt.Fprintf(&builder, "    %s\n", signatureComment)
		}
		normalizedBodyLines := normalizeMacroBody(macro.Body, parameterNames)
		for _, bodyLine := range normalizedBodyLines {
			if strings.TrimSpace(bodyLine) == "" {
				builder.WriteString("\n")
				continue
			}
			builder.WriteString("    ")
			builder.WriteString(strings.TrimRight(bodyLine, " \t"))
			builder.WriteString("\n")
		}
		builder.WriteString("  end;\n")
		if index < len(macros)-1 {
			builder.WriteString("\n")
		}
	}
	builder.WriteString("end;\n")

	return builder.String(), nil
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
		emit(top.BaseIndent+1, "end;")
		blockStack = blockStack[:len(blockStack)-1]
		contentIndent = top.BaseIndent
		return true
	}
	openBlock := func(kind, header string) {
		baseIndent := contentIndent
		emit(baseIndent, header)
		emit(baseIndent+1, "begin")
		blockStack = append(blockStack, TMacroBlock{Kind: kind, BaseIndent: baseIndent})
		contentIndent = baseIndent + 2
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
			openBlock("space", "space "+trimTrailingPunctuation(spaceName))
			continue
		}

		switch trimmedLine {
		case "else":
			if len(blockStack) > 0 && blockStack[len(blockStack)-1].Kind == "if" {
				top := blockStack[len(blockStack)-1]
				emit(top.BaseIndent+1, "end;")
				emit(top.BaseIndent, "else")
				emit(top.BaseIndent+1, "begin")
				contentIndent = top.BaseIndent + 2
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

func formatMacroHeader(macroName string, parameterNames []string) string {
	if len(parameterNames) == 0 {
		return fmt.Sprintf("macro %s:", macroName)
	}
	parameters := []string{}
	for _, parameterName := range parameterNames {
		trimmedParameterName := strings.TrimSpace(parameterName)
		if trimmedParameterName == "" {
			continue
		}
		parameters = append(parameters, "$"+trimmedParameterName)
	}
	return fmt.Sprintf("macro %s (%s):", macroName, strings.Join(parameters, " "))
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
			// so blank separators do not end up inside generated begin/end bodies.
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
					outputLines = append(outputLines, strings.Repeat(" ", statementIndent+2)+"group "+groupValues+";")
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

	return strings.TrimRight(strings.Join(outputLines, "\n"), "\n") + "\n"
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
		outputLines = append(outputLines, strings.Repeat(" ", listHeaderIndent+2)+"end;")
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
			outputLines = append(outputLines, strings.Repeat(" ", listHeaderIndent)+trimTrailingPunctuation(trimmedLine))
			outputLines = append(outputLines, strings.Repeat(" ", listHeaderIndent+2)+"begin")
			insideList = true
			continue
		}

		if insideList && indentWidth <= listHeaderIndent && !strings.HasPrefix(trimmedLine, "#") {
			closeList()
		}
		flushBlankLines()

		if strings.HasPrefix(trimmedLine, "#") {
			if insideList {
				outputLines = append(outputLines, strings.Repeat(" ", listHeaderIndent+4)+trimmedLine)
			} else {
				outputLines = append(outputLines, trimmedLine)
			}
			continue
		}
		if insideList {
			outputLines = append(outputLines, strings.Repeat(" ", listHeaderIndent+4)+trimTrailingPunctuation(trimmedLine)+";")
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
		trimmedLine = "value " + strings.TrimPrefix(trimmedLine, "define value ")
	}
	if strings.HasPrefix(trimmedLine, "defining value ") {
		trimmedLine = "value " + strings.TrimPrefix(trimmedLine, "defining value ")
	}
	if strings.HasPrefix(trimmedLine, "define satisfies ") {
		trimmedLine = "condition " + strings.TrimPrefix(trimmedLine, "define satisfies ")
	}
	if strings.HasPrefix(trimmedLine, "define adjust ") {
		trimmedLine = "adjustment " + strings.TrimPrefix(trimmedLine, "define adjust ")
	}
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
		trimmedLine = "definition " + strings.TrimPrefix(trimmedLine, "define ")
	}
	if strings.HasPrefix(trimmedLine, "provides device_node ") {
		trimmedLine = "providing node " + strings.TrimPrefix(trimmedLine, "provides device_node ")
	}
	if strings.HasPrefix(trimmedLine, "declare entity ") {
		trimmedLine = "entity " + strings.TrimPrefix(trimmedLine, "declare entity ")
	}
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
