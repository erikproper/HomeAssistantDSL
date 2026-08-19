/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Physical
 *
 * Generic parsing of Physical.def's "integration <name> [on <host>] with: ... end;"
 * blocks (physical/PSM layer device identification, see Architecture.md §9.1/§9.2). This
 * file only recognises the shared wrapper syntax and dispatches each block's body to the
 * integration-specific parser registered for its name -- e.g. integration_hosts.go for
 * "integration hosts with: ...". Adding a future integration (netatmo, ems, ...) means
 * adding its own integration_<name>.go and registering it in integrationBodyParsers; this
 * file itself should not need to change.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 18.08.2026
 *
 */

package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// TIntegrationBlock is one "integration <name> [on <host>] with: ... end;" block from
// Physical.def, with its header parsed and body lines extracted but not yet interpreted.
// Interpreting BodyLines is entirely up to the integration-specific parser registered for
// Name in integrationBodyParsers.
type TIntegrationBlock struct {
	Name      string
	Host      string // optional "on <host>" qualifier; empty if not given
	BodyLines []string
	StartLine int // 1-indexed line number of the "integration ... with:" header, for diagnostics
}

// integrationBodyParsers maps an "integration <name>" name to the function that turns its
// body lines into generated output. Each integration owns its own local body syntax; only
// the generic "integration <name> [on <host>] with: ... end;" wrapper is parsed here.
var integrationBodyParsers = map[string]func(bodyLines []string, outputRoot string) error{
	"hosts": generateHostsIntegrationOutputs,
}

// parseIntegrationBlocks scans Physical.def content for top-level "integration <name>
// [on <host>] with: ... end;" blocks. Nested "with:"/"end;" pairs inside a block's body
// (e.g. a per-device capability block) are tracked by depth and do not close the block
// early -- the block only closes on the "end;" that returns depth to zero.
func parseIntegrationBlocks(physicalContent string) ([]TIntegrationBlock, []string) {
	var blocks []TIntegrationBlock
	var warnings []string

	headerPattern := regexp.MustCompile(`^integration\s+(\S+)(?:\s+on\s+(\S+))?\s+with:\s*$`)

	lines := strings.Split(strings.ReplaceAll(physicalContent, "\r\n", "\n"), "\n")

	inBlock := false
	depth := 0
	var current TIntegrationBlock

	for lineIdx, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if commentIdx := strings.Index(line, "#"); commentIdx >= 0 {
			line = strings.TrimSpace(line[:commentIdx])
		}
		if line == "" {
			continue
		}

		if !inBlock {
			if matches := headerPattern.FindStringSubmatch(line); matches != nil {
				current = TIntegrationBlock{Name: matches[1], Host: matches[2], StartLine: lineIdx + 1}
				inBlock = true
				depth = 0
			}
			continue
		}

		if strings.HasSuffix(line, "with:") {
			depth++
			current.BodyLines = append(current.BodyLines, rawLine)
			continue
		}
		if line == "end;" {
			if depth == 0 {
				blocks = append(blocks, current)
				inBlock = false
				continue
			}
			depth--
			current.BodyLines = append(current.BodyLines, rawLine)
			continue
		}

		current.BodyLines = append(current.BodyLines, rawLine)
	}

	if inBlock {
		warnings = append(warnings, fmt.Sprintf("Physical.def:%d: \"integration %s\" block is missing its closing \"end;\"", current.StartLine, current.Name))
	}

	return blocks, warnings
}

// generatePhysicalIntegrationOutputs reads Physical.def (if present), parses its
// "integration ... with: ... end;" blocks generically, and dispatches each block's body
// to the parser registered for its name. An unrecognised integration name is reported and
// skipped rather than treated as an error, so Physical.def can keep growing ahead of the
// generator's support for it.
func generatePhysicalIntegrationOutputs(definitionDir, outputRoot string) error {
	physicalContent, err := readOptionalFile(filepath.Join(definitionDir, "Physical.def"))
	if err != nil {
		return err
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
			fmt.Printf("[physical] Physical.def:%d: no parser registered for integration %q; skipping\n", block.StartLine, block.Name)
			continue
		}
		if err := handler(block.BodyLines, outputRoot); err != nil {
			return err
		}
	}
	return nil
}
