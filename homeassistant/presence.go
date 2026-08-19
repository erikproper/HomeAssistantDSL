/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Presence
 *
 * Post-generation entity reference checks, run automatically as part of "generate":
 *
 *   Check [1] — referential integrity (always, offline):
 *     Every entity ID referenced in the generated YAML must be in the "declared" set:
 *       • entities declared in Entities.def (both explicitly defined and assumed-from-HA)
 *       • entities implied by generation (template lights/switches, groups, automations, scripts)
 *     Violations are printed as warnings; generation still completes.
 *
 *   Check [2] — online availability (when HA is reachable):
 *     Entities marked as assumed-from-HA (no definition or import in the DSL) are verified
 *     against a live Home Assistant instance.  Skipped silently when HA cannot be reached.
 *
 *   Check [3] — bridge entity availability (when each bridge is reachable):
 *     For every REST bridge declared in Bridges.def, the remote entity IDs referenced by
 *     "import rest" directives are verified against that bridge's live HA instance.
 *     Each bridge is checked independently; unreachable bridges are reported and skipped.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 27.04.2026
 *
 */

package main

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// entityRefPatterns extracts entity ID strings from generated YAML.
// Three classes are matched:
//   - Scalar entity_id: value on the same line as the key
//   - YAML list items whose text looks like a HA entity ID (domain.object_id)
//   - Jinja template string literals passed to states/is_state/state_attr/etc.
var entityRefPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?m)^\s*entity_id:\s*([a-z_][a-z0-9_]*\.[a-z0-9_]+)\s*$`),
	regexp.MustCompile(`(?m)^\s*-\s*([a-z_][a-z0-9_]*\.[a-z0-9_]+)\s*$`),
	regexp.MustCompile(`'([a-z_][a-z0-9_]*\.[a-z0-9_]+)'`),
}

// entityIDLike validates that a string looks like a HA entity ID (domain.object_id).
var entityIDLike = regexp.MustCompile(`^[a-z_][a-z0-9_]*\.[a-z0-9_]+$`)

// runPostGenerationChecks performs both check [1] and check [2] after YAML generation
// completes.  Both checks are best-effort: warnings are printed but generation is not
// aborted.
func runPostGenerationChecks(definitionDir, outputDir, label string, admin *TAdministrationState) {
	// Check [1]: referential integrity.
	fmt.Printf("%s: checking entity references ...\n", label)
	unknowns := checkEntityReferences(outputDir, admin)
	if len(unknowns) > 0 {
		fmt.Printf("%s: [presence] %d unaccounted entity reference(s) in generated YAML\n", label, len(unknowns))
		for _, id := range unknowns {
			fmt.Printf("%s:   - %s\n", label, id)
		}
	}

	// Check [2]: online availability of assumed entities (only when HA is reachable).
	checkAssumedEntitiesOnline(definitionDir, label, admin)

	// Check [3]: bridge entity availability (one check per REST bridge, when reachable).
	checkBridgeEntitiesOnline(label, admin)
}

// checkEntityReferences builds the declared entity ID set (DSL entities + generated file
// names), scans the generated YAML for all references, and returns every reference not
// present in the declared set.
func checkEntityReferences(outputDir string, admin *TAdministrationState) []string {
	declared := buildDeclaredEntityIDSet(outputDir, admin)
	referenced, err := scanYAMLEntityReferences(outputDir)
	if err != nil {
		return nil
	}

	var unknowns []string
	for id := range referenced {
		if !declared[id] {
			unknowns = append(unknowns, id)
		}
	}
	sort.Strings(unknowns)
	return unknowns
}

// buildDeclaredEntityIDSet returns the full set of HA entity IDs that are "known":
//   - every entity declared in the DSL (EntityRecordsBySpace), whether explicitly defined
//     or assumed to be provided by a HA integration
//   - every implied entity produced by the generator (derived from output file names
//     in the entities/, automation/, and script/ subdirectories)
func buildDeclaredEntityIDSet(outputDir string, admin *TAdministrationState) map[string]bool {
	declared := map[string]bool{}

	// DSL-declared entities (both defined and assumed).
	// For adjustment sensors, also declare the integration-supplied _raw source entity.
	for _, records := range admin.EntityRecordsBySpace {
		for _, rec := range records {
			id := toHomeAssistantEntityID(rec.Name)
			if id == "" {
				continue
			}
			declared[id] = true
			if rec.Identity.Domain == "sensor" && rec.AdjustmentOffset != "" {
				declared[id+"_raw"] = true
			}
		}
	}

	// Implied entities: derived from generated file names (template lights/switches,
	// binary sensor groups, sensor groups, automations, scripts, customisations, etc.).
	entitySubdirs := map[string]bool{
		"entities":      true,
		"automation":    true,
		"script":        true,
		"customization": true,
	}
	_ = filepath.Walk(outputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(info.Name(), ".yaml") {
			return err
		}
		rel, relErr := filepath.Rel(outputDir, path)
		if relErr != nil {
			return nil
		}
		topDir := strings.SplitN(rel, string(filepath.Separator), 2)[0]
		if entitySubdirs[topDir] {
			base := strings.TrimSuffix(info.Name(), ".yaml")
			if entityIDLike.MatchString(base) {
				declared[base] = true
			}
		}
		return nil
	})

	return declared
}

// checkAssumedEntitiesOnline attempts to reach the Home Assistant instance configured
// for definitionDir.  If reachable, it verifies that every entity in EntityRecordsBySpace
// that is assumed (not explicitly defined or imported) actually exists on the instance.
// Silently skipped when no HA target is configured; prints a warning when configured but
// unreachable; always prints a summary when the check runs.
func checkAssumedEntitiesOnline(definitionDir, label string, admin *TAdministrationState) {
	target, err := resolveHomeAssistantTarget(definitionDir)
	if err != nil {
		return // no target configured — skip silently
	}

	fmt.Printf("%s: checking assumed entities on the main instance (%s) ...\n", label, target.BaseURL)

	httpTransport := http.DefaultTransport.(*http.Transport).Clone()
	if target.InsecureSkipTLS {
		httpTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	client := &http.Client{Timeout: 8 * time.Second, Transport: httpTransport}

	availableIDs, fetchErr := fetchAllEntityIDs(client, target)
	if fetchErr != nil {
		fmt.Printf("%s: [main] %s not reachable — skipping entity check\n", label, target.BaseURL)
		return
	}

	// Collect assumed (external) entity IDs.
	assumedByID := map[string]string{} // entity_id -> full DSL name
	for _, records := range admin.EntityRecordsBySpace {
		for _, rec := range records {
			if rec.HasDefinitionOrImport || rec.NoCollect {
				continue
			}
			id := toHomeAssistantEntityID(rec.Name)
			if id != "" {
				assumedByID[id] = rec.Name
			}
		}
	}

	var missing []string
	for id := range assumedByID {
		if !availableIDs[id] {
			missing = append(missing, id)
		}
	}

	if len(missing) == 0 {
		fmt.Printf("%s: [main] %d assumed entities verified on %s\n", label, len(assumedByID), target.BaseURL)
		return
	}

	sort.Strings(missing)
	fmt.Printf("%s: [main] %d of %d assumed entities not found on %s\n", label, len(missing), len(assumedByID), target.BaseURL)
	for _, id := range missing {
		fmt.Printf("%s:   - %s\n", label, id)
	}
}

// scanYAMLEntityReferences walks all YAML files under outputDir and collects every
// entity ID string referenced in entity_id fields, entities lists, and Jinja templates.
func scanYAMLEntityReferences(outputDir string) (map[string]bool, error) {
	refs := map[string]bool{}
	err := filepath.Walk(outputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(info.Name(), ".yaml") {
			return err
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("reading %s: %w", path, readErr)
		}
		for _, id := range extractEntityIDsFromYAML(string(content)) {
			refs[id] = true
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scanning %s: %w", outputDir, err)
	}
	return refs, nil
}

// checkBridgeEntitiesOnline verifies each REST bridge declared in Bridges.def.
// For every bridge that is reachable, the remote entity IDs referenced by "import rest"
// directives are checked against that bridge's live HA instance.  Each bridge is checked
// independently; unreachable bridges are reported and skipped.  Multiple bridges are
// handled in bridge-name order for deterministic output.
func checkBridgeEntitiesOnline(label string, admin *TAdministrationState) {
	if len(admin.RestImports) == 0 || len(admin.BridgeTargets) == 0 {
		return
	}

	// Group remote entity IDs by bridge name.
	byBridge := map[string][]string{}
	for _, imp := range admin.RestImports {
		byBridge[imp.BridgeName] = append(byBridge[imp.BridgeName], imp.RemoteEntityID)
	}

	bridgeNames := make([]string, 0, len(byBridge))
	for name := range byBridge {
		bridgeNames = append(bridgeNames, name)
	}
	sort.Strings(bridgeNames)

	for _, bridgeName := range bridgeNames {
		remoteIDs := byBridge[bridgeName]
		target, exists := admin.BridgeTargets[bridgeName]
		if !exists {
			fmt.Printf("%s: [bridge/%s] bridge not declared in Bridges.def — skipping\n", label, bridgeName)
			continue
		}

		fmt.Printf("%s: checking assumed bridge entities on the %s instance (%s) ...\n", label, bridgeName, target.BaseURL)

		httpTransport := http.DefaultTransport.(*http.Transport).Clone()
		if target.InsecureSkipTLS {
			httpTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		}
		client := &http.Client{Timeout: 8 * time.Second, Transport: httpTransport}

		availableIDs, fetchErr := fetchAllEntityIDs(client, target)
		if fetchErr != nil {
			fmt.Printf("%s: [bridge/%s] %s not reachable — skipping entity check\n", label, bridgeName, target.BaseURL)
			continue
		}

		var missing []string
		for _, id := range remoteIDs {
			if !availableIDs[id] {
				missing = append(missing, id)
			}
		}

		if len(missing) == 0 {
			fmt.Printf("%s: [bridge/%s] %d bridge entities verified on %s\n", label, bridgeName, len(remoteIDs), target.BaseURL)
			continue
		}

		sort.Strings(missing)
		fmt.Printf("%s: [bridge/%s] %d of %d bridge entities not found on %s\n", label, bridgeName, len(missing), len(remoteIDs), target.BaseURL)
		for _, id := range missing {
			fmt.Printf("%s:   - %s\n", label, id)
		}
	}
}

// extractEntityIDsFromYAML applies all entityRefPatterns against the YAML content
// and returns every distinct entity ID string found.
func extractEntityIDsFromYAML(content string) []string {
	seen := map[string]bool{}
	for _, pattern := range entityRefPatterns {
		for _, match := range pattern.FindAllStringSubmatch(content, -1) {
			if len(match) >= 2 {
				id := strings.TrimSpace(match[1])
				if entityIDLike.MatchString(id) {
					seen[id] = true
				}
			}
		}
	}
	result := make([]string, 0, len(seen))
	for id := range seen {
		result = append(result, id)
	}
	return result
}
