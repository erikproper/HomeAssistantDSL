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
 *   home_assistant main: vienna via ssh hassio@vienna.homelinux.org:1967;   an HA OS instance
 *   home_assistant ha2mqtt: protocols-server-1;     a container, on the hosts device of that name
 *   coordinator on protocols-server-1;              where the coordinator runs (the hub)
 *   deploy via ssh erikp@vienna.homelinux.org:1970; on a hosts device: how a Mac reaches it
 *   deploy as user erikp;                           on a hosts device: LAN only, through the hub
 *
 * and the settings ${integrations_dir} (cpu/, ping/, containers/, mqtt_commandline/, coordinator/)
 * and ${coordinator_source} (the generator and coordinator source), with defaults by convention.
 *
 * Conventions: an instance with its own "via ssh" runs on HA OS with the SSH add-on (its files
 * go to ~/<name>, then into /config); one without runs as a container on its hosts device, in
 * ${container_ha_config}. The cpu report script goes to every "cpu" host with a "deploy" line --
 * through the hub for a LAN host, directly for a "via cloud" one (a Mac has none: its ~/cpu is
 * the Nextcloud folder itself). update.containers goes to every machine running a container
 * instance; a commandline device's host gets the source it builds from.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 03.11.2026
 *
 */

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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

// cutInstanceSSH splits a "home_assistant <qualifier>: <name> [via ssh <endpoint>];" declaration's
// fields into the name and its SSH endpoint (nil without one); ok is false for any other shape.
func cutInstanceSSH(fields []string) (name string, ssh *TSSHEndpoint, ok bool) {
	switch {
	case len(fields) == 1:
		return fields[0], nil, true
	case len(fields) == 4 && fields[1] == "via" && fields[2] == "ssh":
		if endpoint, valid := parseSSHEndpoint(fields[3]); valid {
			return fields[0], &endpoint, true
		}
	}
	return "", nil, false
}

// THostDeploy is a hosts device's "deploy via ssh <endpoint>;" or "deploy as user <user>;".
type THostDeploy struct {
	SSH  *TSSHEndpoint // reached directly from the Mac
	User string        // reached through the hub, as <User>@<hostname>
}

var (
	hostDeployViaSSHPattern = regexp.MustCompile(`^deploy\s+via\s+ssh\s+(\S+)\s*;$`)
	hostDeployAsPattern     = regexp.MustCompile(`^deploy\s+as\s+user\s+([A-Za-z0-9_.-]+)\s*;$`)
)

// parseHostDeployLine recognises a hosts device's "deploy ..." line; warning is set for a
// malformed one.
func parseHostDeployLine(line string) (deploy THostDeploy, warning string, ok bool) {
	if m := hostDeployViaSSHPattern.FindStringSubmatch(line); m != nil {
		endpoint, valid := parseSSHEndpoint(m[1])
		if !valid {
			return THostDeploy{}, fmt.Sprintf("%q: expected \"deploy via ssh <user>@<host>[:<port>];\"", line), true
		}
		return THostDeploy{SSH: &endpoint}, "", true
	}
	if m := hostDeployAsPattern.FindStringSubmatch(line); m != nil {
		return THostDeploy{User: m[1]}, "", true
	}
	return THostDeploy{}, "", false
}

var coordinatorOnPattern = regexp.MustCompile(`^coordinator\s+on\s+(\S+)\s*;$`)

// collectCoordinatorMachine returns the hostname of Physical.def's "coordinator on <hostname>;".
func collectCoordinatorMachine(physicalContent string) string {
	for _, line := range strings.Split(physicalContent, "\n") {
		if m := coordinatorOnPattern.FindStringSubmatch(stripLineComment(line)); m != nil {
			return m[1]
		}
	}
	return ""
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
	CPU          TDeployCPU              `yaml:"cpu"`
	Ping         string                  `yaml:"ping,omitempty"`
}

// TDeploySources are the folders the deploy copies from, "$HOME" kept as written.
type TDeploySources struct {
	Generator    string `yaml:"generator"`
	Integrations string `yaml:"integrations"`
}

// TDeployInstance is one Home Assistant instance's configuration push.
type TDeployInstance struct {
	Name    string `yaml:"name"`
	Machine string `yaml:"machine"`
	Kind    string `yaml:"kind"` // "ha_os" or "container"
	Files   string `yaml:"files"`
	Config  string `yaml:"config"`
}

// TDeployCommandline is a commandline device and the machine it runs on.
type TDeployCommandline struct {
	Device  string `yaml:"device"`
	Machine string `yaml:"machine"`
}

// TDeployCPU is where the cpu report script goes: from the hub to its LAN hosts, and directly to
// "via cloud" machines.
type TDeployCPU struct {
	Hub    string           `yaml:"hub,omitempty"`
	ViaHub []TDeployHubHost `yaml:"via_hub,omitempty"`
	Direct []string         `yaml:"direct,omitempty"`
}

// TDeployHubHost is a LAN host the hub reaches as <User>@<Host>.
type TDeployHubHost struct {
	Host string `yaml:"host"`
	User string `yaml:"user"`
}

// defaultContainerHAConfig is a container instance's configuration folder, ${container_ha_config}
// overriding it.
const defaultContainerHAConfig = "/var/lib/home-automation/homeassistant"

// buildDeployPlan returns the house's deploy plan, and a warning for everything the defs leave
// undecided (a machine without a way to reach it, a LAN cpu host without a "deploy" line).
func buildDeployPlan(outputRoot, physicalContent string, settings map[string]string, installation string, instances map[string]THomeAssistantInstance, hosts map[string]THostDevice, commandline map[string]TCommandlineDevice) (TDeployPlan, []string) {
	var warnings []string
	plan := TDeployPlan{
		Installation: installation,
		Sources: TDeploySources{
			Generator:    settingOr(settings, "coordinator_source", "$HOME/go/src/homeassistant"),
			Integrations: settingOr(settings, "integrations_dir", homeRelative(filepath.Join(filepath.Dir(outputRoot), "Integrations"))),
		},
		Machines:    map[string]TSSHEndpoint{},
		Coordinator: collectCoordinatorMachine(physicalContent),
	}

	hostIDs := make([]string, 0, len(hosts))
	for id := range hosts {
		hostIDs = append(hostIDs, id)
	}
	sort.Strings(hostIDs)
	for _, id := range hostIDs {
		if d := hosts[id]; d.Deploy.SSH != nil {
			plan.Machines[d.HostName] = *d.Deploy.SSH
		}
	}
	machine := func(name, what string) string {
		if _, known := plan.Machines[name]; !known {
			warnings = append(warnings, fmt.Sprintf("deploy: %s runs on %q, which has no way to be reached -- give its hosts device \"deploy via ssh <user>@<host>[:<port>];\"", what, name))
		}
		return name
	}

	if plan.Coordinator == "" {
		warnings = append(warnings, "deploy: no \"coordinator on <hostname>;\" in Physical.def -- the coordinator, ping and the cpu script's hub are not deployed")
	} else {
		machine(plan.Coordinator, "the coordinator")
		plan.Ping = plan.Coordinator
		plan.CPU.Hub = plan.Coordinator
	}

	qualifiers := make([]string, 0, len(instances))
	for qualifier := range instances {
		qualifiers = append(qualifiers, qualifier)
	}
	sort.Slice(qualifiers, func(i, j int) bool { // main first, as the deploy has always gone
		if (qualifiers[i] == "main") != (qualifiers[j] == "main") {
			return qualifiers[i] == "main"
		}
		return qualifiers[i] < qualifiers[j]
	})
	for _, qualifier := range qualifiers {
		instance := instances[qualifier]
		name := resolveDefinitionReference(instance.Name, settings)
		deploy := TDeployInstance{Name: qualifier, Machine: name, Files: "hass/" + name}
		if instance.SSH != nil {
			plan.Machines[name] = *instance.SSH
			deploy.Kind, deploy.Config = "ha_os", "/config"
		} else {
			machine(name, "Home Assistant instance "+qualifier)
			deploy.Kind, deploy.Config = "container", settingOr(settings, "container_ha_config", defaultContainerHAConfig)
			plan.Containers = append(plan.Containers, name)
		}
		plan.Instances = append(plan.Instances, deploy)
	}

	commandlineIDs := make([]string, 0, len(commandline))
	for id := range commandline {
		commandlineIDs = append(commandlineIDs, id)
	}
	sort.Strings(commandlineIDs)
	for _, id := range commandlineIDs {
		host := commandline[id].Host
		plan.Commandline = append(plan.Commandline, TDeployCommandline{Device: id, Machine: machine(host, "commandline device "+id)})
	}

	for _, id := range hostIDs {
		d := hosts[id]
		if d.IntegrationType != "cpu" {
			continue
		}
		switch {
		case d.Cloud && d.Deploy.SSH != nil:
			plan.CPU.Direct = append(plan.CPU.Direct, d.HostName)
		case d.Cloud:
			// A Mac: its ~/cpu is the Nextcloud folder itself.
		case d.Deploy.SSH != nil:
			plan.CPU.ViaHub = append(plan.CPU.ViaHub, TDeployHubHost{Host: d.HostName, User: d.Deploy.SSH.User})
		case d.Deploy.User != "":
			plan.CPU.ViaHub = append(plan.CPU.ViaHub, TDeployHubHost{Host: d.HostName, User: d.Deploy.User})
		default:
			warnings = append(warnings, fmt.Sprintf("deploy: cpu host %q (%s) has no \"deploy\" line -- it doesn't get the cpu report script (\"deploy as user <user>;\" for a LAN host the hub reaches)", d.HostName, id))
		}
	}
	return plan, warnings
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
