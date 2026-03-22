/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Interpretation
 *
 * This component renders parser output into a human-readable interpretation text file per house.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 18.03.2026
 *
 */

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func runInterpretation(root string, houses []string) error {
	for _, house := range houses {
		if err := interpretHouse(root, house); err != nil {
			return fmt.Errorf("error interpreting %s: %w", house, err)
		}
		fmt.Printf("interpreted %s\n", house)
	}
	return nil
}

func interpretHouse(root, house string) error {
	definitionDir := filepath.Join(root, "New", house, "Definitions")
	definitionFiles, err := resolveDefinitionLoadOrder(definitionDir)
	if err != nil {
		return err
	}

	parsedFiles := []TParsedFile{}
	for _, definitionFile := range definitionFiles {
		parsedFile, parseErr := ParseDefinitionFile(definitionFile)
		if parseErr != nil {
			return parseErr
		}
		parsedFiles = append(parsedFiles, parsedFile)
	}

	builder := strings.Builder{}
	fmt.Fprintf(&builder, "House: %s\n", house)
	fmt.Fprintf(&builder, "Definitions folder: New/%s/Definitions\n\n", house)

	for _, parsedFile := range parsedFiles {
		fmt.Fprintf(&builder, "File: %s\n", parsedFile.Name)
		if parsedFile.ParseSucceeded {
			builder.WriteString("Status: ok\n")
		} else {
			builder.WriteString("Status: errors\n")
		}

		if len(parsedFile.Errors) > 0 {
			builder.WriteString("Errors:\n")
			for _, parseError := range parsedFile.Errors {
				fmt.Fprintf(&builder, "  - %s\n", parseError)
			}
		}

		builder.WriteString("Interpretation:\n")
		writeNodeInterpretation(&builder, parsedFile.Nodes, 2)
		builder.WriteString("\n")
	}

	interpretationReportPath, err := writeDebugReport(root, house, DebugReportInterpretation, builder.String())
	if err != nil {
		return err
	}
	if interpretationReportPath != "" {
		fmt.Printf("  interpretation report: %s\n", interpretationReportPath)
	}
	return nil
}

func resolveDefinitionLoadOrder(definitionDir string) ([]string, error) {
	mainPath := filepath.Join(definitionDir, "Main.def")
	mainContent, err := readOptionalFile(mainPath)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(mainContent) == "" {
		fallbackFiles, fallbackErr := filepath.Glob(filepath.Join(definitionDir, "*.def"))
		if fallbackErr != nil {
			return nil, fallbackErr
		}
		sort.Strings(fallbackFiles)
		return fallbackFiles, nil
	}

	orderedFiles := []string{mainPath}
	seenFiles := map[string]bool{"Main.def": true}
	mainLines := strings.Split(strings.ReplaceAll(mainContent, "\r\n", "\n"), "\n")

	for lineIndex, mainLine := range mainLines {
		trimmedLine := strings.TrimSpace(mainLine)
		if trimmedLine == "" || strings.HasPrefix(trimmedLine, "#") {
			continue
		}

		syntaxLine := stripTrailingInlineComment(trimmedLine)
		syntaxLine = trimTrailingPunctuation(syntaxLine)
		if !strings.HasPrefix(syntaxLine, "include ") {
			continue
		}

		includedName := strings.TrimSpace(strings.TrimPrefix(syntaxLine, "include "))
		if includedName == "" {
			return nil, fmt.Errorf("%s:%d: include target is empty", mainPath, lineIndex+1)
		}
		if !strings.Contains(includedName, ".") {
			includedName += ".def"
		}

		if seenFiles[includedName] {
			continue
		}

		includedPath := filepath.Join(definitionDir, includedName)
		if _, statErr := os.Stat(includedPath); statErr != nil {
			if os.IsNotExist(statErr) {
				return nil, fmt.Errorf("%s:%d: included file not found: %s", mainPath, lineIndex+1, includedName)
			}
			return nil, statErr
		}

		orderedFiles = append(orderedFiles, includedPath)
		seenFiles[includedName] = true
	}

	allDefinitionFiles, err := filepath.Glob(filepath.Join(definitionDir, "*.def"))
	if err != nil {
		return nil, err
	}
	sort.Strings(allDefinitionFiles)
	for _, definitionPath := range allDefinitionFiles {
		definitionName := filepath.Base(definitionPath)
		if seenFiles[definitionName] {
			continue
		}
		orderedFiles = append(orderedFiles, definitionPath)
	}

	return orderedFiles, nil
}

func writeNodeInterpretation(builder *strings.Builder, nodes []TNode, indent int) {
	prefix := strings.Repeat(" ", indent)
	for _, node := range nodes {
		switch node.Kind {
		case NodeComment:
			fmt.Fprintf(builder, "%sCOMMENT %s\n", prefix, node.Text)
		case NodeStatement:
			fmt.Fprintf(builder, "%sSTATEMENT %s\n", prefix, node.Text)
		case NodeBlock:
			fmt.Fprintf(builder, "%sBLOCK %s\n", prefix, node.Text)
			writeNodeInterpretation(builder, node.Children, indent+2)
			fmt.Fprintf(builder, "%sEND BLOCK\n", prefix)
		}
	}
}
