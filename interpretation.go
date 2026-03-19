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
	definitionFiles, err := filepath.Glob(filepath.Join(root, "New", house, "Definitions", "*.def"))
	if err != nil {
		return err
	}
	sort.Strings(definitionFiles)

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

	interpretationPath := filepath.Join(root, "New", house, "interpretation.txt")
	return os.WriteFile(interpretationPath, []byte(builder.String()), 0o644)
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
