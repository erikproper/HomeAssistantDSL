/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: PhysicalGenerator
 *
 * Orchestrates the physical layer's generated output: collects the physical layer's
 * content (layers.go), parses its "integration ... with: ... end;" blocks
 * (Physical_Parser.go), and dispatches each block's body to the generator registered
 * for its name (Physical_Storage.go).
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 19.08.2026
 *
 */

package main

import (
	"fmt"
	"strings"
)

// generatePhysicalIntegrationOutputs collects the physical layer's content (see
// layers.go; today just Physical.def, merged across any "physical layer with: ... end;"
// chunks it contains), parses its "integration ... with: ... end;" blocks generically, and
// dispatches each block's body to the generator registered for its name. An unrecognised
// integration name is reported and skipped rather than treated as an error, so the
// physical layer can keep growing ahead of the generator's support for it.
func generatePhysicalIntegrationOutputs(definitionDir, outputRoot string) error {
	physicalContent, layerWarnings := collectLayerContent(definitionDir, []string{"Physical.def"}, LayerPhysical)
	for _, w := range layerWarnings {
		fmt.Printf("[physical] %s\n", w)
	}
	if strings.TrimSpace(physicalContent) == "" {
		return nil
	}

	blocks, warnings := parseIntegrationBlocks(physicalContent)
	for _, w := range warnings {
		fmt.Printf("[physical] %s\n", w)
	}

	for _, block := range blocks {
		handler, known := integrationBodyParsers[block.Name]
		if !known {
			fmt.Printf("[physical] integration %q (line %d): no parser registered; skipping\n", block.Name, block.StartLine)
			continue
		}
		if err := handler(block.BodyLines, outputRoot); err != nil {
			return err
		}
	}
	return nil
}
