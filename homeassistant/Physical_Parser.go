/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: PhysicalParser
 *
 * Generic parsing of Physical.def's "integration <name> [on <host>] with: ... end;"
 * blocks (physical/PSM layer device identification, see Architecture.md §9.1/§9.2). This
 * file only recognises the shared wrapper syntax; it does not interpret block bodies or
 * know what integrations exist (see Physical_Storage.go) or produce output (see
 * Physical_Generator.go).
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 19.08.2026
 *
 */

package main

import "regexp"

// TIntegrationBlock is one "integration <name> [on <host>] with: ... end;" block from the
// physical layer, with its header parsed and body lines extracted but not yet interpreted.
// Interpreting BodyLines is entirely up to the integration-specific parser registered for
// Name in integrationBodyParsers (Physical_Storage.go).
type TIntegrationBlock struct {
	Name      string
	Host      string // optional "on <host>" qualifier; empty if not given
	BodyLines []string
	StartLine int // 1-indexed line number (within the scanned body) of the "integration ... with:" header, for diagnostics
}

var integrationHeaderPattern = regexp.MustCompile(`^integration\s+(\S+)(?:\s+on\s+(\S+))?\s+with:\s*$`)

// parseIntegrationBlocks scans physical-layer content for top-level "integration <name>
// [on <host>] with: ... end;" blocks, via the shared group-clause scanner (defined.go).
// mergedLineNos is collectLayerContent's own line-number mapping for physicalContent (its
// "physical layer with: ... end;" wrapper already stripped) -- required to translate StartLine
// back to a real Physical.def line number (translateContentLineNo, defined.go); pass nil to skip
// translation (StartLine then reports a position within physicalContent instead, only correct if
// the caller already knows physicalContent's lines line up 1:1 with the source file).
func parseIntegrationBlocks(physicalContent string, mergedLineNos []int) ([]TIntegrationBlock, []string) {
	matches, warnings := scanGroupClauseBlocks(splitLines(physicalContent), "physical layer", integrationHeaderPattern)

	var blocks []TIntegrationBlock
	for _, m := range matches {
		startLine := m.StartLine
		if mergedLineNos != nil {
			startLine = translateContentLineNo(mergedLineNos, startLine)
		}
		blocks = append(blocks, TIntegrationBlock{
			Name:      m.HeaderMatch[1],
			Host:      m.HeaderMatch[2],
			BodyLines: m.BodyLines,
			StartLine: startLine,
		})
	}
	return blocks, warnings
}
