/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Layers
 *
 * Generic parsing of the three explicit modelling-layer wrappers (Architecture.md §2/§9.1):
 *
 *   physical layer with: ... end;
 *   logical layer with: ... end;
 *   conceptual layer with: ... end;
 *
 * A layer may be specified in multiple chunks, spread across multiple .def files -- this
 * file only extracts and concatenates those chunks by layer name and source file set; it
 * does not interpret their content. Callers (generator.go) hand the concatenated body for
 * a given layer/file-set to whichever downstream parser understands that content (e.g.
 * ParseEntitiesAndFillAdministration for conceptual space/entity syntax).
 *
 * Placement rules (enforced here as warnings, not hard errors, matching this generator's
 * existing "warn and continue" style for source imperfections):
 *   - "space"/"list" declarations belong in the conceptual layer.
 *   - "mqtt"/"home_assistant"/"integration" declarations belong in the physical layer.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 19.08.2026
 *
 */

package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	LayerPhysical   = "physical"
	LayerLogical    = "logical"
	LayerConceptual = "conceptual"
)

// disallowedTopLevelPrefixes lists, per layer, the statement keywords that belong in a
// different layer and should not appear at a layer chunk's top level.
var disallowedTopLevelPrefixes = map[string][]string{
	LayerPhysical:   {"space ", "space:", "list "},
	LayerConceptual: {"mqtt ", "home_assistant ", "integration "},
}

var layerHeaderPattern = regexp.MustCompile(`^(\S+)\s+layer\s+with:\s*$`)

// TLayerBlock is one "<layer> layer with: ... end;" block, with its header parsed and body
// lines extracted but not yet interpreted.
type TLayerBlock struct {
	Layer       string
	SourceFile  string
	BodyLines   []string
	BodyLineNos []int
	StartLine   int
}

// parseLayerBlocks scans content (from sourceFile, used only for diagnostics) for top-level
// "<name> layer with: ... end;" blocks, via the shared group-clause scanner
// (groupclause.go). An unrecognised layer name (not physical/logical/conceptual) is
// reported as a warning; its block is still returned so the caller can decide whether to
// use it.
func parseLayerBlocks(content, sourceFile string) ([]TLayerBlock, []string) {
	matches, warnings := scanGroupClauseBlocks(splitLines(content), sourceFile, layerHeaderPattern)

	var blocks []TLayerBlock
	for _, m := range matches {
		layerName := m.HeaderMatch[1]
		if layerName != LayerPhysical && layerName != LayerLogical && layerName != LayerConceptual {
			warnings = append(warnings, fmt.Sprintf("%s:%d: unrecognised layer name %q (expected physical, logical, or conceptual)", sourceFile, m.StartLine, layerName))
		}
		blocks = append(blocks, TLayerBlock{
			Layer:       layerName,
			SourceFile:  sourceFile,
			BodyLines:   m.BodyLines,
			BodyLineNos: m.BodyLineNos,
			StartLine:   m.StartLine,
		})
	}
	return blocks, warnings
}

// collectLayerContent reads each file in fileNames (missing files are skipped, not an
// error) and concatenates every chunk found for layerName, across all of them, in
// file-then-position order -- this is what lets a layer be "specified in multiple chunks,
// across different def files." Returns the merged body as a single string, a parallel slice
// giving each merged line's original 1-indexed source line number (mergedLineNos[i] is
// merged-content line i+1's line number in its own SourceFile -- callers reporting
// diagnostics against a merged-content line index must look up this slice rather than using
// the index directly, since blank/comment stripping during extraction means the two no
// longer coincide), plus any warnings (unrecognised layer names, unclosed blocks, and
// content that looks like it belongs in a different layer).
func collectLayerContent(definitionDir string, fileNames []string, layerName string) (string, []int, []string) {
	var merged []string
	var mergedLineNos []int
	var warnings []string
	found := false

	for _, fileName := range fileNames {
		content, err := readOptionalFile(filepath.Join(definitionDir, fileName))
		if err != nil || strings.TrimSpace(content) == "" {
			continue
		}
		blocks, blockWarnings := parseLayerBlocks(content, fileName)
		warnings = append(warnings, blockWarnings...)
		for _, block := range blocks {
			if block.Layer != layerName {
				continue
			}
			found = true
			merged = append(merged, block.BodyLines...)
			mergedLineNos = append(mergedLineNos, block.BodyLineNos...)
			warnings = append(warnings, misplacedContentWarnings(block)...)
		}
	}

	if !found {
		warnings = append(warnings, fmt.Sprintf("no %q layer with: ... end; block found across %v", layerName, fileNames))
	}

	return strings.Join(merged, "\n"), mergedLineNos, warnings
}

// misplacedContentWarnings flags top-level lines in a layer chunk that start with a
// keyword expected in a different layer (see disallowedTopLevelPrefixes). Only depth-0
// lines are checked -- a nested block's internal content isn't re-classified.
func misplacedContentWarnings(block TLayerBlock) []string {
	disallowed, hasRule := disallowedTopLevelPrefixes[block.Layer]
	if !hasRule {
		return nil
	}

	var warnings []string
	depth := 0
	for _, rawLine := range block.BodyLines {
		line := strings.TrimSpace(rawLine)
		if commentIdx := strings.Index(line, "#"); commentIdx >= 0 {
			line = strings.TrimSpace(line[:commentIdx])
		}
		if line == "" {
			continue
		}
		if strings.HasSuffix(line, "with:") {
			if depth == 0 {
				for _, prefix := range disallowed {
					if strings.HasPrefix(line, prefix) {
						warnings = append(warnings, fmt.Sprintf("%s: %q layer chunk (from line %d) contains %q, which looks like it belongs in a different layer", block.SourceFile, block.Layer, block.StartLine, line))
					}
				}
			}
			depth++
			continue
		}
		if line == "end;" {
			if depth > 0 {
				depth--
			}
			continue
		}
		if depth == 0 {
			for _, prefix := range disallowed {
				if strings.HasPrefix(line, prefix) {
					warnings = append(warnings, fmt.Sprintf("%s: %q layer chunk (from line %d) contains %q, which looks like it belongs in a different layer", block.SourceFile, block.Layer, block.StartLine, line))
				}
			}
		}
	}
	return warnings
}
