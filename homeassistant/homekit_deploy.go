/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: HomeKit (deploy)
 *
 * The deploy's half of PROJECT.md 62 (homekit.go): after main's reload, each bridge in homekit.json
 * gets exactly its entities, through the bridge's own options flow in Home Assistant (the steps
 * its settings dialog takes: bridge mode, "include", the entities' domains; the entities; the
 * device triggers, kept as they are). The bridge is found by its config entry's title,
 * "<name>:<port>". Its current filter is what the flow shows as defaults; when it already
 * matches, the flow is cancelled and nothing changes. With -n the flow is always cancelled.
 *
 * Home Assistant restarts the bridge after a change; the iPhones keep the pairing, and an
 * accessory that stays keeps its name and room in Apple Home.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Vienna, Austria, in collaboration with Claude.ai
 *
 * Version of: 21.11.2026
 *
 */

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// applyHomekit sets main's HomeKit bridges to homekit.json (this file's header comment).
func (d *tDeployer) applyHomekit() {
	var main *TDeployInstance
	for i := range d.plan.Instances {
		if d.plan.Instances[i].Name == "main" && d.plan.Instances[i].Kind == "ha_os" {
			main = &d.plan.Instances[i]
		}
	}
	if main == nil {
		return
	}
	data, err := os.ReadFile(filepath.Join(d.houseDir, main.Files, homekitManifestFile))
	if os.IsNotExist(err) {
		return // External.def has no HomeKit bridge
	}
	d.step("Setting what the HomeKit bridges expose")
	var bridges []THomekitManifestBridge
	if err == nil {
		err = json.Unmarshal(data, &bridges)
	}
	if err != nil {
		fmt.Printf("  ERROR: %s: %v\n", homekitManifestFile, err)
		d.stepFailed = true
		return
	}
	token := d.instanceToken(*main)
	if token == "" || main.URL == "" {
		fmt.Printf("  WARNING: no ${%s_ha_token} in the defs, or no \"at <url>\" on home_assistant %s -- HomeKit bridges not set\n", main.Name, main.Name)
		return
	}
	ha := &tHAConnection{base: strings.TrimSuffix(main.URL, "/"), token: token}
	for _, bridge := range bridges {
		fmt.Printf("+ HomeKit bridge %q (port %d): %d entities, through its options flow (%s)\n", bridge.Name, bridge.Port, len(bridge.Entities), main.URL)
		report, err := ha.setHomekitBridge(bridge, d.dryRun)
		if err != nil {
			fmt.Printf("  ERROR: %v\n", err)
			d.stepFailed = true
			continue
		}
		fmt.Println("  " + report)
	}
}

// setHomekitBridge runs bridge's options flow (this file's header comment); the report says what
// changed, or would.
func (c *tHAConnection) setHomekitBridge(bridge THomekitManifestBridge, dryRun bool) (string, error) {
	var entries []struct {
		EntryID string `json:"entry_id"`
		Title   string `json:"title"`
		Source  string `json:"source"`
	}
	if err := c.rest("GET", "/api/config/config_entries/entry?domain=homekit", nil, &entries); err != nil {
		return "", err
	}
	title := fmt.Sprintf("%s:%d", bridge.Name, bridge.Port)
	entryID := ""
	for _, entry := range entries {
		if entry.Title == title {
			if entry.Source == "import" {
				return "", fmt.Errorf("bridge %s is set up from YAML, which this deploy doesn't write", title)
			}
			entryID = entry.EntryID
		}
	}
	if entryID == "" {
		return "", fmt.Errorf("no HomeKit bridge %s in Home Assistant -- make it in Settings > Devices & services first", title)
	}

	var step tHAFlowStep
	if err := c.rest("POST", "/api/config/config_entries/options/flow", map[string]any{"handler": entryID}, &step); err != nil {
		return "", err
	}
	cancel := func() { c.rest("DELETE", "/api/config/config_entries/options/flow/"+step.FlowID, nil, nil) }
	if step.StepID != "init" {
		cancel()
		return "", fmt.Errorf("its options flow began with step %q, expected \"init\"", step.StepID)
	}
	mode, _ := step.defaultOf("include_exclude_mode").(string)
	homekitMode, _ := step.defaultOf("mode").(string)
	currentDomains := stringsOf(step.defaultOf("domains"))
	if homekitMode != "bridge" {
		cancel()
		return "", fmt.Errorf("it is in %q mode, not a bridge", homekitMode)
	}
	domains := homekitDomains(bridge.Entities)
	if err := c.flowStep(&step, map[string]any{"mode": "bridge", "include_exclude_mode": "include", "domains": domains}, "include"); err != nil {
		cancel()
		return "", err
	}
	currentEntities := stringsOf(step.defaultOf("entities"))
	if homekitFilterUnchanged(mode, currentDomains, currentEntities, bridge.Entities) {
		cancel()
		return "unchanged", nil
	}
	report := fmt.Sprintf("was %s, domains %v (in %v: %v) -> exactly %v", mode, currentDomains, domains, currentEntities, bridge.Entities)
	if dryRun {
		cancel()
		return "would change: " + report, nil
	}
	if err := c.flowStep(&step, map[string]any{"entities": bridge.Entities}, "bridged_device_triggers"); err != nil {
		cancel()
		return "", err
	}
	if err := c.flowStep(&step, map[string]any{}, ""); err != nil {
		cancel()
		return "", err
	}
	if step.Type != "create_entry" {
		cancel()
		return "", fmt.Errorf("its options flow ended in %q, not saved", step.Type)
	}
	return "changed: " + report, nil
}

// homekitFilterUnchanged tells whether a bridge's filter -- mode, the domains and the included
// entities its options flow shows -- is already exactly want.
func homekitFilterUnchanged(mode string, domains, entities, want []string) bool {
	return mode == "include" &&
		sameStringSet(domains, homekitDomains(want)) &&
		sameStringSet(entities, want)
}

// homekitDomains is the sorted domains of entityIDs.
func homekitDomains(entityIDs []string) []string {
	var domains []string
	for _, entityID := range entityIDs {
		if domain := entityDomain(entityID); !containsString(domains, domain) {
			domains = append(domains, domain)
		}
	}
	sort.Strings(domains)
	return domains
}

// sameStringSet tells whether a and b hold the same strings.
func sameStringSet(a, b []string) bool {
	for _, s := range a {
		if !containsString(b, s) {
			return false
		}
	}
	for _, s := range b {
		if !containsString(a, s) {
			return false
		}
	}
	return true
}

// stringsOf is value as strings, when it is a list of them.
func stringsOf(value any) []string {
	list, _ := value.([]any)
	var values []string
	for _, item := range list {
		if s, ok := item.(string); ok {
			values = append(values, s)
		}
	}
	return values
}

// tHAFlowStep is one step of a Home Assistant config or options flow, as its REST API gives it.
type tHAFlowStep struct {
	FlowID     string           `json:"flow_id"`
	Type       string           `json:"type"`
	StepID     string           `json:"step_id"`
	DataSchema []map[string]any `json:"data_schema"`
	Errors     map[string]any   `json:"errors"`
}

// defaultOf is the default the step's form shows for field.
func (s tHAFlowStep) defaultOf(field string) any {
	for _, item := range s.DataSchema {
		if item["name"] == field {
			return item["default"]
		}
	}
	return nil
}

// flowStep answers step with input; the next step must be next ("" for the flow's end).
func (c *tHAConnection) flowStep(step *tHAFlowStep, input map[string]any, next string) error {
	var reply tHAFlowStep
	if err := c.rest("POST", "/api/config/config_entries/options/flow/"+step.FlowID, input, &reply); err != nil {
		return err
	}
	if len(reply.Errors) > 0 {
		return fmt.Errorf("step %q refused the input: %v", step.StepID, reply.Errors)
	}
	if next != "" && reply.StepID != next {
		return fmt.Errorf("after step %q came %q, expected %q", step.StepID, reply.StepID, next)
	}
	*step = reply
	return nil
}

// rest calls Home Assistant's REST API: body as JSON when not nil, the reply into reply when not nil.
func (c *tHAConnection) rest(method, path string, body, reply any) error {
	var payload io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(data)
	}
	request, err := http.NewRequest(method, c.base+path, payload)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	if response.StatusCode >= 300 {
		return fmt.Errorf("%s %s: %s %s", method, path, response.Status, bytes.TrimSpace(data))
	}
	if reply != nil && len(data) > 0 {
		return json.Unmarshal(data, reply)
	}
	return nil
}
