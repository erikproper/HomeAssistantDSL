/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: DeployPlan
 *
 * PROJECT.md 30 (2026-11-03, agreed with Erik): the deploy, until now per house in deploy.d/ with
 * host names, ports and paths written into each step, follows from the defs. The generator writes
 * deploy.yaml in the house folder -- which files go to which machine, which instance gets its
 * configuration how, which hosts get the cpu report script -- and "homeassistant -deploy" (step 2)
 * carries it out. The defs say:
 *
 *   home_assistant main: vienna via ssh hassio@vienna.homelinux.org:1967
 *     at https://home-vienna.erikproper.eu;         an HA OS instance, and its API's URL
 *   home_assistant ha2mqtt: protocols-server-1;     a container, on the hosts device of that name
 *   coordinator on protocols-server-1;              where the coordinator runs (the hub)
 *   deploy via ssh erikp@vienna.homelinux.org:1970; on a hosts device: how a Mac reaches it
 *   deploy as user erikp;                           on a hosts device: LAN only, through the hub
 *
 * and the settings ${integrations_dir} (cpu/, ping/, containers/, mqtt_commandline/, coordinator/)
 * and ${coordinator_source} (the generator and coordinator source), with defaults by convention.
 *
 * Each integration plans its own part, in its own file: coordinator_deploy.go,
 * home_assistant_deploy.go (an instance with its own "via ssh" runs on HA OS, one without as a
 * container on its hosts device), integration_hosts_deploy.go (the machines, the cpu report
 * script, ping), containers_deploy.go, integration_commandline_deploy.go, integration_fhem_deploy.go. This file holds what
 * they share: the machines, the settings, and the order the parts are planned in.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 08.11.2026
 *
 */

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// TSSHEndpoint is "<user>@<host>[:<port>]", how the deploying Mac reaches a machine.
type TSSHEndpoint struct {
	User string `yaml:"user"`
	Host string `yaml:"host"`
	Port int    `yaml:"port,omitempty"`
}

var sshEndpointPattern = regexp.MustCompile(`^([A-Za-z0-9_.-]+)@([A-Za-z0-9_.-]+)(?::(\d+))?$`)

// parseSSHEndpoint parses "<user>@<host>[:<port>]".
func parseSSHEndpoint(text string) (TSSHEndpoint, bool) {
	m := sshEndpointPattern.FindStringSubmatch(text)
	if m == nil {
		return TSSHEndpoint{}, false
	}
	port, _ := strconv.Atoi(m[3])
	return TSSHEndpoint{User: m[1], Host: m[2], Port: port}, true
}

// TDeployPlan is deploy.yaml (this file's header comment).
type TDeployPlan struct {
	Installation string                  `yaml:"installation"`
	Sources      TDeploySources          `yaml:"sources"`
	Machines     map[string]TSSHEndpoint `yaml:"machines"`
	Coordinator  string                  `yaml:"coordinator,omitempty"`
	Instances    []TDeployInstance       `yaml:"instances,omitempty"`
	Containers   []string                `yaml:"containers,omitempty"`
	Commandline  []TDeployCommandline    `yaml:"commandline,omitempty"`
	FHEM         []TDeployFHEM           `yaml:"fhem,omitempty"`
	CPU          TDeployCPU              `yaml:"cpu"`
	Ping         string                  `yaml:"ping,omitempty"`
}

// TDeploySources are the folders the deploy copies from, "$HOME" kept as written.
type TDeploySources struct {
	Generator    string `yaml:"generator"`
	Integrations string `yaml:"integrations"`
}

// tDeployPlanner builds a deploy plan, each integration adding its own part.
type tDeployPlanner struct {
	plan     TDeployPlan
	settings map[string]string
	warnings []string
}

// machine returns name, warning when the plan has no way to reach it (what runs there).
func (p *tDeployPlanner) machine(name, what string) string {
	if _, known := p.plan.Machines[name]; !known {
		p.warnings = append(p.warnings, fmt.Sprintf("deploy: %s runs on %q, which has no way to be reached -- give its hosts device \"deploy via ssh <user>@<host>[:<port>];\"", what, name))
	}
	return name
}

// buildDeployPlan returns the house's deploy plan, and a warning for everything the defs leave
// undecided (a machine without a way to reach it, a LAN cpu host without a "deploy" line). The
// hosts devices' machines come first, so every other part finds the machines it runs on.
func buildDeployPlan(outputRoot, physicalContent string, settings map[string]string, installation string, instances map[string]THomeAssistantInstance, hosts map[string]THostDevice, commandline map[string]TCommandlineDevice) (TDeployPlan, []string) {
	p := &tDeployPlanner{settings: settings, plan: TDeployPlan{
		Installation: installation,
		Sources: TDeploySources{
			Generator:    settingOr(settings, "coordinator_source", "$HOME/go/src/homeassistant"),
			Integrations: settingOr(settings, "integrations_dir", homeRelative(filepath.Join(filepath.Dir(outputRoot), "Integrations"))),
		},
		Machines: map[string]TSSHEndpoint{},
	}}
	p.planHostsMachines(hosts)
	p.planCoordinatorDeploy(physicalContent)
	p.planInstancesDeploy(instances)
	p.planContainersDeploy()
	p.planCommandlineDeploy(commandline)
	p.planFHEMDeploy(physicalContent)
	p.planHostsDeploy(hosts)
	return p.plan, p.warnings
}

// settingOr returns the setting name's value, or fallback when it isn't set.
func settingOr(settings map[string]string, name, fallback string) string {
	if value := resolveDefinitionReference("${"+name+"}", settings); value != "" {
		return value
	}
	return fallback
}

// homeRelative writes path with this Mac's home folder as "$HOME" (CLAUDE.md: never a
// machine-specific absolute path).
func homeRelative(path string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(path, home+"/") {
		return "$HOME" + strings.TrimPrefix(path, home)
	}
	return path
}

// generateDeployPlan writes the house's deploy.yaml and reports what the defs leave undecided.
func generateDeployPlan(outputRoot, definitionDir, physicalContent, installation string, instances map[string]THomeAssistantInstance, hosts map[string]THostDevice) []string {
	commandline, _ := collectCommandlineDevicesByID(definitionDir)
	plan, warnings := buildDeployPlan(outputRoot, physicalContent, parseDefinitionAssignments(readCombinedSettingsContent(definitionDir)), installation, instances, hosts, commandline)
	var data strings.Builder
	encoder := yaml.NewEncoder(&data)
	encoder.SetIndent(2) // Erik: 2 is the standard indentation
	if err := encoder.Encode(plan); err != nil {
		return append(warnings, fmt.Sprintf("deploy.yaml: %v", err))
	}
	if err := os.WriteFile(filepath.Join(outputRoot, "deploy.yaml"), []byte(generatorHeader+data.String()), 0o644); err != nil {
		return append(warnings, fmt.Sprintf("deploy.yaml: %v", err))
	}
	return warnings
}
