/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Defined
 *
 * Shared utilities for resolving Home Assistant connection targets, fetching entity state lists,
 * and parsing Server.def / Bridges.def / Settings.def / Secrets.def configuration files.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 23.03.2026
 *
 */

package main

import (
	"context"
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
	"strings"
	"time"
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

// trimTrailingPunctuation removes a trailing semicolon from s, if present.
func trimTrailingPunctuation(s string) string {
	return strings.TrimSuffix(strings.TrimSpace(s), ";")
}

func resolveBridgeTargets(definitionDir string) (map[string]THomeAssistantTarget, error) {
	serverPath := filepath.Join(definitionDir, "Server.def")
	secretsPath := filepath.Join(definitionDir, "Secrets.def")
	bridgesPath := filepath.Join(definitionDir, "Bridges.def")

	serverContent, _ := readOptionalFile(serverPath)
	settingsContent := readCombinedSettingsContent(definitionDir)
	secretsContent, _ := readOptionalFile(secretsPath)
	bridgesContent, _ := readOptionalFile(bridgesPath)

	vars := parseServerAssignmentsWithDefined(serverContent, isServerHostUp)
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

func resolveHomeAssistantTarget(definitionDir string) (THomeAssistantTarget, error) {
	baseURL := ""
	token := ""
	insecureSkipTLS := false

	serverPath := filepath.Join(definitionDir, "Server.def")
	secretsPath := filepath.Join(definitionDir, "Secrets.def")

	serverContent, _ := readOptionalFile(serverPath)
	settingsContent := readCombinedSettingsContent(definitionDir)
	secretsContent, _ := readOptionalFile(secretsPath)

	vars := parseServerAssignmentsWithDefined(serverContent, isServerHostUp)
	for name, value := range parseDefinitionAssignments(settingsContent) {
		vars[name] = value
	}
	for name, value := range parseDefinitionAssignments(secretsContent) {
		vars[name] = value
	}

	mainTargetExpr := parseMainTargetExpression(serverContent)
	baseURL = resolveDefinitionReference(mainTargetExpr, vars)
	token = resolveDefinitionReference("${main_api_token}", vars)
	insecureSkipTLS = resolveDefinitionBoolReference([]string{"${main_api_tls_insecure}", "${main_api_tls_skip_verify}", "${main_api_insecure_tls}"}, vars)

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
		return THomeAssistantTarget{}, fmt.Errorf("missing Home Assistant token; provide ${main_api_token} in Secrets.def or set HASS_TOKEN/HOMEASSISTANT_TOKEN")
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
	assignmentPattern := regexp.MustCompile(`^\$\{([A-Za-z_][A-Za-z0-9_]*)\}\s*=\s*(.+?)\s*;\s*$`)
	matches := assignmentPattern.FindStringSubmatch(strings.TrimSpace(line))
	if matches == nil {
		return "", "", false
	}
	name := matches[1]
	value := unquoteShellValue(strings.TrimSpace(matches[2]))
	return name, value, true
}

func parseServerAssignmentsWithDefined(serverContent string, hostUp func(string) bool) map[string]string {
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

// readCombinedSettingsContent reads the shared Settings.def followed by the house-local
// Settings.def (which may override shared variables such as $Workdays).
func readCombinedSettingsContent(definitionDir string) string {
	sharedPath := filepath.Join(definitionDir, "../../Shared/Definitions/Settings.def")
	localPath := filepath.Join(definitionDir, "Settings.def")
	shared, _ := readOptionalFile(sharedPath)
	local, _ := readOptionalFile(localPath)
	return shared + "\n" + local
}

