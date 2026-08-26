/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Defined
 *
 * Shared utilities for resolving Home Assistant connection targets, fetching entity state lists,
 * parsing Bridges.def / Settings.def configuration files, and the
 * generic "group clause" grammar (QualifiedElementsGroup/ForcedElementsGroup/
 * ForceElementsSequence, Background/Arch2.md) shared by every "<header> [with]: ... end;"
 * construct parsed outside the main Entities.def grammar.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 23.03.2026
 *
 */

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type THomeAssistantTarget struct {
	BaseURL         string
	Token           string
	InsecureSkipTLS bool
	StatesPath      string
}

type TBridgeRestDefinition struct {
	BridgeName    string
	EndpointExpr  string
	TokenExpr     string
	InsecureTLS   bool
	ResolvedURL   string
	ResolvedToken string
}

// TMQTTBrokerSecrets holds one named MQTT broker profile's connection secrets, resolved from a
// Physical.def "mqtt <name>: server ...; login ...; password ...; port ...; [tls true;] end;"
// block (resolveMQTTBrokerProfiles). TLS is false unless the block explicitly declares
// "tls true;" -- every existing house's local broker stays plaintext by default, matching
// current behaviour exactly.
type TMQTTBrokerSecrets struct {
	Server   string
	Login    string
	Password string
	Port     string
	TLS      bool
	// CoordinatorOnly marks a profile as holding broad-access credentials meant for the
	// coordinator process itself, never for leaf host ping/cpu report scripts -- e.g. a cloud
	// broker's coordinator account, as opposed to the narrowly-scoped client account every leaf
	// device shares. generateHostsIntegrationOutputs excludes any profile with this set from the
	// per-broker secrets files it writes into cpu/, so a laptop never ends up with a copy of the
	// coordinator's own credentials on disk. Declared via "coordinator_only true;".
	CoordinatorOnly bool
}

var mqttProfileHeaderPattern = regexp.MustCompile(`^mqtt\s+(\S+):\s*$`)
var mqttProfileFieldPattern = regexp.MustCompile(`^(server|login|password|port|tls|coordinator_only)\s+(\S+);\s*$`)

// resolveMQTTBrokerProfiles scans Physical.def for every "mqtt <name>: ... end;" block and
// resolves each declared field (server/login/password/port/tls) via resolveDefinitionReference
// against Settings.def's variables -- a field's value may be a literal or a "${var}" reference,
// exactly as already written in the block (e.g. "server ${main_mqtt_server};"); nothing about the
// profile name itself is assumed or required to match a variable-naming convention. Returns every
// profile found, keyed by its declared name (e.g. "main", "cloud"), plus any warnings for
// unrecognised lines.
func resolveMQTTBrokerProfiles(definitionDir string) (map[string]TMQTTBrokerSecrets, []string) {
	physicalContent, _, warnings := collectLayerContent(definitionDir, []string{"Physical.def"}, LayerPhysical)
	settingsContent := readCombinedSettingsContent(definitionDir)
	vars := parseDefinitionAssignments(settingsContent)

	blocks, blockWarnings := scanGroupClauseBlocks(splitLines(physicalContent), "Physical.def", mqttProfileHeaderPattern)
	warnings = append(warnings, blockWarnings...)

	profiles := map[string]TMQTTBrokerSecrets{}
	for _, block := range blocks {
		name := block.HeaderMatch[1]
		var secrets TMQTTBrokerSecrets
		for _, rawLine := range block.BodyLines {
			line := strings.TrimSpace(rawLine)
			if commentIdx := strings.Index(line, "#"); commentIdx >= 0 {
				line = strings.TrimSpace(line[:commentIdx])
			}
			if line == "" {
				continue
			}
			matches := mqttProfileFieldPattern.FindStringSubmatch(line)
			if matches == nil {
				warnings = append(warnings, fmt.Sprintf("Physical.def: unrecognised line inside \"mqtt %s\" block: %q", name, line))
				continue
			}
			value := resolveDefinitionReference(matches[2], vars)
			switch matches[1] {
			case "server":
				secrets.Server = value
			case "login":
				secrets.Login = value
			case "password":
				secrets.Password = value
			case "port":
				secrets.Port = value
			case "tls":
				secrets.TLS = value == "true"
			case "coordinator_only":
				secrets.CoordinatorOnly = value == "true"
			}
		}
		profiles[name] = secrets
	}
	return profiles, warnings
}

// resolveMQTTBrokerSecrets resolves the house's "main" MQTT broker connection secrets --
// backward-compatible convenience wrapper over resolveMQTTBrokerProfiles for callers that only
// ever cared about the one house-wide broker. Returns ok=false if "main" isn't declared, or all
// four connection fields are empty -- not every house has an MQTT broker configured yet (e.g.
// Vienna today), and callers should treat that as "nothing to generate," not an error.
func resolveMQTTBrokerSecrets(definitionDir string) (TMQTTBrokerSecrets, bool) {
	profiles, _ := resolveMQTTBrokerProfiles(definitionDir)
	secrets, found := profiles["main"]
	if !found || (secrets.Server == "" && secrets.Login == "" && secrets.Password == "" && secrets.Port == "") {
		return TMQTTBrokerSecrets{}, false
	}
	return secrets, true
}

// resolveMQTTDiscoveryPhysicalPrefix resolves ${mqtt_discovery_physical} from Settings.def --
// the MQTT Discovery topic prefix external "physical gateway" devices (e.g. EMS-ESP) publish
// their own native HA discovery under, instead of the default "homeassistant" prefix HA itself
// subscribes to. This is what lets the coordinator observe and selectively relay a gateway's
// auto-discovered entities (the "discovery" integration, integration_discovery_*.go) without HA
// picking up the gateway's raw, unpositioned discovery directly. "" if not configured -- not
// every house has a "discovery" integration yet.
func resolveMQTTDiscoveryPhysicalPrefix(definitionDir string) string {
	settingsContent := readCombinedSettingsContent(definitionDir)
	vars := parseDefinitionAssignments(settingsContent)
	return resolveDefinitionReference("${mqtt_discovery_physical}", vars)
}

// resolveMQTTDiscoveryConceptualPrefix resolves ${mqtt_discovery_conceptual} from Settings.def --
// the MQTT Discovery topic prefix the coordinator itself publishes our own conceptual-layer
// entities under (mirrors resolveMQTTDiscoveryPhysicalPrefix's role for the *source* side).
// Falls back to "homeassistant" -- HA's own default discovery prefix -- when unset, so this is
// never "" the way MQTTDiscoveryPhysicalPrefix legitimately can be for a house with no
// "discovery" integration; every house's coordinator always publishes conceptual discovery.
func resolveMQTTDiscoveryConceptualPrefix(definitionDir string) string {
	settingsContent := readCombinedSettingsContent(definitionDir)
	vars := parseDefinitionAssignments(settingsContent)
	if prefix := resolveDefinitionReference("${mqtt_discovery_conceptual}", vars); prefix != "" {
		return prefix
	}
	return "homeassistant"
}

// resolveInstallationName resolves ${installation} from Settings.def -- this house's own short
// name (e.g. "junglinster", "vienna"), used to qualify state/discovery topics a coordinator
// publishes onto a shared cloud broker (mqtt_relay.go, house_event_bus_coordinator side) so two
// installations sharing that broker's namespace never collide. "" if not configured -- not every
// house has adopted the cloud broker "cloud"/"local"/"import" mechanism yet.
func resolveInstallationName(definitionDir string) string {
	settingsContent := readCombinedSettingsContent(definitionDir)
	vars := parseDefinitionAssignments(settingsContent)
	return resolveDefinitionReference("${installation}", vars)
}

// --- shared file and string utilities ---

// readOptionalFile reads the file at path and returns its contents as a string.
// If the file does not exist, it returns an empty string and no error.
func readOptionalFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

// unquoteShellValue strips surrounding double or single quotes from a value string.
func unquoteShellValue(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func resolveBridgeTargets(definitionDir string) (map[string]THomeAssistantTarget, error) {
	bridgesPath := filepath.Join(definitionDir, "Bridges.def")

	// Settings.def (local, per-house) now also holds real secrets -- see .gitignore in
	// the SmartLiving repo -- so there's no separate Secrets.def to read anymore.
	settingsContent := readCombinedSettingsContent(definitionDir)
	bridgesContent, _ := readOptionalFile(bridgesPath)

	vars := parseDefinitionAssignments(settingsContent)

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
			InsecureSkipTLS: resolveDefinitionBoolReference([]string{"${main_api_tls_insecure}", "${main_api_tls_skip_verify}", "${main_api_insecure_tls}"}, vars),
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
	if !strings.HasPrefix(value, "${") {
		return value
	}
	end := strings.Index(value, "}")
	if end < 2 {
		return value
	}
	name := value[2:end]
	resolvedVar, exists := vars[name]
	if !exists {
		return ""
	}
	return strings.TrimSpace(resolvedVar) + value[end+1:]
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

// resolveMainIncarnationName reads Physical.def (plus combined Settings.def, which also
// holds real secrets, for ${var} resolution) and returns the name declared by
// "home_assistant main: <name> <url>;", e.g. "junglinster". Returns "" if Physical.def
// doesn't exist yet or has no such directive — callers should fall back to the
// pre-instance-aware behaviour in that case, so houses that haven't adopted Physical.def
// yet keep generating exactly as before.
func resolveMainIncarnationName(definitionDir string) string {
	// Warnings from this extraction are intentionally dropped here: generatePhysicalIntegrationOutputs
	// re-collects the same physical layer content later in the pipeline and reports them there,
	// so surfacing them again from this early, output-path-only read would just be noise.
	physicalContent, _, _ := collectLayerContent(definitionDir, []string{"Physical.def"}, LayerPhysical)
	if strings.TrimSpace(physicalContent) == "" {
		return ""
	}
	settingsContent := readCombinedSettingsContent(definitionDir)

	vars := parseDefinitionAssignments(settingsContent)

	nameExpr, _ := parseHomeAssistantMainDirective(physicalContent)
	if nameExpr == "" {
		return ""
	}
	return resolveDefinitionReference(nameExpr, vars)
}

// parseHomeAssistantMainDirective extracts the two whitespace-separated tokens (name, url
// expressions) from a "home_assistant main: <name> <url>;" line in Physical.def.
func parseHomeAssistantMainDirective(physicalContent string) (nameExpr, urlExpr string) {
	pattern := regexp.MustCompile(`^home_assistant\s+main:\s*(.+?)\s*;\s*$`)
	for _, rawLine := range strings.Split(strings.ReplaceAll(physicalContent, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(rawLine)
		matches := pattern.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		fields := strings.Fields(matches[1])
		if len(fields) == 0 {
			continue
		}
		nameExpr = fields[0]
		if len(fields) > 1 {
			urlExpr = fields[1]
		}
		return nameExpr, urlExpr
	}
	return "", ""
}

// collectAssumedEntityIDs returns every HA entity_id the DSL references but never defines or
// imports itself -- i.e. assumed to already exist on some HA instance. Shared by
// generateAssumedEntitiesFile (Physical_Generator.go) and, previously, presence.go's
// now-removed REST-based checkAssumedEntitiesOnline -- same computation, relocated rather than
// duplicated. Sorted for deterministic generated output.
func collectAssumedEntityIDs(definitionDir string, admin *TAdministrationState) []string {
	assumedByID := map[string]bool{}
	for _, records := range admin.EntityRecordsBySpace {
		for _, rec := range records {
			if rec.HasDefinitionOrImport || rec.NoCollect || rec.DiscoveryImplied {
				continue
			}
			if id := toHomeAssistantEntityID(rec.Name); id != "" {
				assumedByID[id] = true
			}
		}
	}
	// Physical.def's "integration hosts" home_assistant-type devices reference entities (e.g.
	// sensor.processor_use) that are assumed to already exist locally, the same way -- these
	// aren't declared in Spaces.def (no space/entity linkage exists for them), so they'd
	// otherwise go unaccounted for entirely.
	for id := range homeAssistantCapabilityEntityIDs(definitionDir) {
		assumedByID[id] = true
	}

	ids := make([]string, 0, len(assumedByID))
	for id := range assumedByID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// THomeAssistantInstance is one "home_assistant <qualifier>: <name> <url>;" declaration in
// Physical.def -- Name/URL still as their raw ${...} expressions, unresolved.
type THomeAssistantInstance struct {
	Name string
	URL  string
}

// collectHomeAssistantInstances generalises parseHomeAssistantMainDirective to every qualifier,
// not just "main" -- e.g. "home_assistant protocols-server-2: protocols-server-2;" names a
// secondary instance the same way "home_assistant main: junglinster;" names the main one. The url
// token is optional (nothing requires it any more now that the REST-based presence check is gone
// -- see presence.go's removed checkAssumedEntitiesOnline; the coordinator's replacement routes
// entirely by instance name over MQTT), kept only as an informational field when given. Keyed by
// qualifier (not by Name, which callers must resolveDefinitionReference themselves). This is
// naming/declaration only -- it doesn't imply any entity-bridging capability for the named
// instance, just gives it a stable key other generator output (e.g.
// coordinator/assumed_entities.yaml) can group by.
func collectHomeAssistantInstances(physicalContent string) map[string]THomeAssistantInstance {
	instances := map[string]THomeAssistantInstance{}
	pattern := regexp.MustCompile(`^home_assistant\s+(\S+):\s*(.+?)\s*;\s*$`)
	for _, rawLine := range strings.Split(strings.ReplaceAll(physicalContent, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(rawLine)
		matches := pattern.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		fields := strings.Fields(matches[2])
		if len(fields) == 0 {
			continue
		}
		instance := THomeAssistantInstance{Name: fields[0]}
		if len(fields) > 1 {
			instance.URL = fields[1]
		}
		instances[matches[1]] = instance
	}
	return instances
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

// settingsAssignmentAttemptPattern recognises a line that was clearly *intended* as a
// "${name} = value" assignment (starts with "${...} ="), even if it doesn't fully match
// parseDefinitionAssignmentLine's stricter grammar (e.g. a missing trailing ";") -- used only to
// decide whether an unmatched line deserves a warning, not to extract anything from it.
var settingsAssignmentAttemptPattern = regexp.MustCompile(`^\$\{[A-Za-z_][A-Za-z0-9_]*\}\s*=`)

// warnedSettingsLines de-dupes parseDefinitionAssignments' own warning: it's called repeatedly,
// once per resolver that needs settings (resolveMQTTBrokerSecrets, resolveMQTTDiscoveryPhysicalPrefix,
// ...), typically re-parsing the exact same Settings.def content each time -- without this, one
// malformed line would warn once per resolver invoked that run, not once overall.
var warnedSettingsLines = map[string]bool{}

// parseDefinitionAssignments parses every "${name} = value;" line in content into a name->value
// map, silently skipping blank lines and "#" comments. Any other non-empty line that looks like
// an assignment attempt (starts "${...} =") but doesn't match the full grammar -- most commonly a
// missing trailing ";", which is easy to type and produces no other symptom -- gets a warning
// printed directly (this is called repeatedly, once per resolver that needs settings, so a
// warning here is the only place guaranteed to fire regardless of which resolver first hits the
// malformed line; warnedSettingsLines keeps that to once per line, not once per resolver call).
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
			continue
		}
		if settingsAssignmentAttemptPattern.MatchString(line) && !warnedSettingsLines[line] {
			warnedSettingsLines[line] = true
			fmt.Fprintf(os.Stderr, "[settings] %q looks like a \"${name} = value;\" assignment but wasn't recognised -- missing trailing \";\"?\n", line)
		}
	}

	return assignments
}

func parseDefinitionAssignmentLine(line string) (string, string, bool) {
	assignmentPattern := regexp.MustCompile(`^\$\{([A-Za-z_][A-Za-z0-9_]*)\}\s*=\s*(.+?)\s*;\s*$`)
	matches := assignmentPattern.FindStringSubmatch(strings.TrimSpace(line))
	if matches == nil {
		return "", "", false
	}
	name := matches[1]
	value := unquoteShellValue(strings.TrimSpace(matches[2]))
	return name, value, true
}

func resolveDefinitionReference(expression string, vars map[string]string) string {
	value := strings.TrimSpace(unquoteShellValue(expression))
	for depth := 0; depth < 8; depth++ {
		if !strings.HasPrefix(value, "${") {
			return value
		}
		end := strings.Index(value, "}")
		if end < 2 {
			return value
		}
		name := value[2:end]
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

	return nil, fmt.Errorf("%s", strings.Join(errors, "; "))
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

// readCombinedSettingsContent reads the shared Settings.def followed by the house-local
// Settings.def (which may override shared variables such as $Workdays).
func readCombinedSettingsContent(definitionDir string) string {
	sharedPath := filepath.Join(definitionDir, "../../Shared/Definitions/Settings.def")
	localPath := filepath.Join(definitionDir, "Settings.def")
	shared, _ := readOptionalFile(sharedPath)
	local, _ := readOptionalFile(localPath)
	return shared + "\n" + local
}

// --- group clause grammar ---
//
// Shared primitive implementing the DSL's generic "with clause" / "group clause" grammar
// (Background/Arch2.md's WithClause(f)/GroupClause(f) sketch, formalised as):
//
//   QualifiedElementsGroup(GroupToken, f): MaybeToken(GroupToken) && ForcedElementsGroup(f)
//   ForcedElementsGroup(f):    (MaybeToken(ColonToken) && ForceElementsSequence(f)) || f()
//   ForceElementsSequence(f):  MaybeToken(EndToken) || (f() && ForceElementsSequence(f))
//
// i.e. an optional qualifying keyword ("with", or "main" for the mqtt/home_assistant
// declarations), then either a ":"-introduced sequence of elements terminated by "end;", or
// (no colon) a single bare element. This is the single shared scanner behind every such
// construct parsed outside the main Entities.def grammar -- integration blocks
// (physical.go), layer blocks (layers.go), and any future one -- so that nested-group
// tracking is written and tested once, as genuine recursion, not reinvented per construct
// with an ad hoc depth counter.
//
// QualifiedElementsGroup itself is not implemented here: callers already match a header
// line (including its trailing "with:"/"<word>:") via their own regex before calling
// scanGroupClauseBlocks, so by the time these functions run, the qualifying keyword and the
// colon-or-not decision are already known (see hadColon below).

const endToken = "end;"

// TGroupClauseBlock is one "<header> with: ... end;" match: the header regex's full
// submatch set (index 0 is the whole matched line; 1.. are capture groups), the group-clause
// body lines between the header and its matching "end;" (nested groups' own headers and
// closing "end;" lines are included verbatim, since from the caller's point of view a
// nested group is just more body text -- interpreting it is up to whatever per-context
// parser consumes BodyLines next), and the header's source line for diagnostics. BodyLineNos
// runs parallel to BodyLines: BodyLineNos[i] is BodyLines[i]'s original 1-indexed line number
// in SourceFile, surviving the blank-line/comment stripping newLineCursor does while
// scanning -- callers that report diagnostics against a body line must use BodyLineNos[i],
// not the body's own local index, or the reported line drifts from the source file.
type TGroupClauseBlock struct {
	HeaderMatch []string
	BodyLines   []string
	BodyLineNos []int
	SourceFile  string
	StartLine   int
}

// TLineCursor is a minimal token cursor over pre-cleaned (trimmed, comment-stripped,
// blank-line-free) content lines -- one cleaned line is one token at this grammar's
// granularity. Original 1-indexed source line numbers are preserved alongside each token
// for diagnostics.
type TLineCursor struct {
	lines   []string
	lineNos []int
	pos     int
}

func newLineCursor(rawLines []string) *TLineCursor {
	cur := &TLineCursor{}
	for idx, rawLine := range rawLines {
		line := strings.TrimSpace(rawLine)
		if commentIdx := strings.Index(line, "#"); commentIdx >= 0 {
			line = strings.TrimSpace(line[:commentIdx])
		}
		if line == "" {
			continue
		}
		cur.lines = append(cur.lines, line)
		cur.lineNos = append(cur.lineNos, idx+1)
	}
	return cur
}

func (c *TLineCursor) AtEnd() bool { return c.pos >= len(c.lines) }

func (c *TLineCursor) ThisLine() string {
	if c.AtEnd() {
		return ""
	}
	return c.lines[c.pos]
}

func (c *TLineCursor) ThisLineNumber() int {
	if c.AtEnd() {
		return -1
	}
	return c.lineNos[c.pos]
}

// Advance returns the current line and moves the cursor past it.
func (c *TLineCursor) Advance() string {
	line := c.ThisLine()
	c.pos++
	return line
}

// forceElementsSequence: MaybeToken(EndToken) || (f() && ForceElementsSequence(f)).
// Repeatedly applies element until "end;" is found and consumed. If appendEndTo is
// non-nil, the consumed "end;" is appended to it -- used when this sequence is itself
// nested inside another element's body (its "end;" is that outer body's text too); the
// outermost call in scanGroupClauseBlocks passes nil so the block-terminating "end;" isn't
// recorded as part of the block's own content.
func forceElementsSequence(cur *TLineCursor, element func(*TLineCursor) bool, appendEndTo *[]string, appendEndLineNos *[]int) bool {
	if cur.ThisLine() == endToken {
		lineNo := cur.ThisLineNumber()
		line := cur.Advance()
		if appendEndTo != nil {
			*appendEndTo = append(*appendEndTo, line)
			*appendEndLineNos = append(*appendEndLineNos, lineNo)
		}
		return true
	}
	if !element(cur) {
		return false
	}
	return forceElementsSequence(cur, element, appendEndTo, appendEndLineNos)
}

// forcedElementsGroup: (MaybeToken(ColonToken) && ForceElementsSequence(f)) || f().
// hadColon stands in for "MaybeToken(ColonToken)": callers already know whether the header
// line ended in ":" (a sequence follows) or not (a single bare element follows) from their
// own header-matching regex, so there's no separate colon token left on the cursor to
// re-check here.
func forcedElementsGroup(cur *TLineCursor, hadColon bool, element func(*TLineCursor) bool, appendEndTo *[]string, appendEndLineNos *[]int) bool {
	if hadColon {
		return forceElementsSequence(cur, element, appendEndTo, appendEndLineNos)
	}
	return element(cur)
}

// genericBodyElement returns a ForcedElement (the "f" of the grammar above) that appends
// every line it consumes to body, treating a nested "<header>:" line as an opaque element
// whose own sub-body -- including its own closing "end;" -- is consumed and appended
// recursively. This is what makes nested "with:"/"end;" pairs (e.g. a per-device
// capability block inside an "integration hosts with:" block) not close the outer group
// early, without an explicit depth counter: the recursion IS the depth tracking.
func genericBodyElement(body *[]string, lineNos *[]int) func(cur *TLineCursor) bool {
	var element func(cur *TLineCursor) bool
	element = func(cur *TLineCursor) bool {
		if cur.AtEnd() {
			return false
		}
		line := cur.ThisLine()
		lineNo := cur.ThisLineNumber()
		*body = append(*body, cur.Advance())
		*lineNos = append(*lineNos, lineNo)
		if strings.HasSuffix(line, ":") {
			return forceElementsSequence(cur, element, body, lineNos)
		}
		return true
	}
	return element
}

// scanGroupClauseBlocks finds every line in rawLines matched by headerPattern (which must
// itself require the line to end in "with:" or another bare ":"-terminated qualifying
// keyword, e.g. "main:"), and extracts each match's group-clause body via
// forcedElementsGroup/forceElementsSequence/genericBodyElement above. Lines outside any
// header match are skipped, so this can scan a whole file for scattered top-level blocks
// or a single already-extracted body for nested ones -- the same primitive either way.
func scanGroupClauseBlocks(rawLines []string, sourceFile string, headerPattern *regexp.Regexp) ([]TGroupClauseBlock, []string) {
	var blocks []TGroupClauseBlock
	var warnings []string

	cur := newLineCursor(rawLines)

	for !cur.AtEnd() {
		line := cur.ThisLine()
		matches := headerPattern.FindStringSubmatch(line)
		if matches == nil {
			cur.Advance()
			continue
		}
		startLine := cur.ThisLineNumber()
		cur.Advance()

		var body []string
		var bodyLineNos []int
		hadColon := strings.HasSuffix(line, ":")
		if !forcedElementsGroup(cur, hadColon, genericBodyElement(&body, &bodyLineNos), nil, nil) {
			warnings = append(warnings, fmt.Sprintf("%s:%d: block starting %q is missing its closing \"end;\"", sourceFile, startLine, matches[0]))
			break
		}

		blocks = append(blocks, TGroupClauseBlock{HeaderMatch: matches, BodyLines: body, BodyLineNos: bodyLineNos, SourceFile: sourceFile, StartLine: startLine})
	}

	return blocks, warnings
}

// translateContentLineNo maps a line number computed within a layer-extracted content string
// (e.g. scanGroupClauseBlocks's StartLine, when it's given content collectLayerContent already
// merged/stripped the "<layer> with: ... end;" wrapper from) back to the original source file's
// line number, via collectLayerContent's own returned mergedLineNos (mergedLineNos[i] is merged
// content line i+1's original file line). Without this translation, a StartLine computed by
// re-splitting and re-scanning collectLayerContent's *output* -- as parseIntegrationBlocks and
// collectHassBridgeDevicesByID both do, since Physical.def's own "integration ... with: ... end;"
// blocks are nested one level inside the outer "physical layer with: ... end;" wrapper
// collectLayerContent already stripped -- reports a position within that already-shifted content,
// not a real Physical.def line number (this is what produced a materially wrong line number in a
// real warning; see PROJECT.md/this commit). Falls back to contentLineNo unchanged if the mapping
// doesn't cover it (shouldn't normally happen).
func translateContentLineNo(mergedLineNos []int, contentLineNo int) int {
	if contentLineNo-1 >= 0 && contentLineNo-1 < len(mergedLineNos) {
		return mergedLineNos[contentLineNo-1]
	}
	return contentLineNo
}

// splitLines is a small helper for callers that have raw file content rather than an
// already-split []string.
func splitLines(content string) []string {
	return strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
}
