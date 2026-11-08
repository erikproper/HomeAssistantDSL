/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationGoverned
 *
 * PROJECT.md 37 (claude-notes/plans/governed-integration.md): Physical.def's "integration governed"
 * block (its grammar: parser_governed.go) declares the governed devices -- devices that do not
 * report their state, reached through a radio's button (integration_fhem.go), whose state the
 * governor (../governor) keeps:
 *
 *   integration governed with:
 *     device lights.test_fs20_switch with:
 *       capability binary_sensor.node;
 *       capability switch.switch
 *         by doing button.set from radio.test_fs20_switch
 *         enforced every 00:05:00
 *         default off;
 *     end;
 *   end;
 *
 * The generator writes the governor's governor/devices.yaml (each entity, its topics, default,
 * enforcement and paths -- for an FHEM device its command and received topics, its encoding, and
 * how many installations send it) and governor/secrets.yaml (the main broker). For Home Assistant
 * a governed device is a raw-MQTT device (integration_mqtt.go) on the governor's own topics:
 * governed/<device>/<label> retained, commands on .../set, and its node the governor's liveness,
 * governed/status -- so positioning, frozen ids and the YAML entities stay the generic machinery's.
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
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// TGovernedDevice is a "device <id> with: ... end;" of the block.
type TGovernedDevice struct {
	DeviceID     string
	Metadata     map[string]string
	Capabilities []TGovernedCapability
}

// TGovernedCapability is a capability of a governed device; Button is empty for its node.
type TGovernedCapability struct {
	Domain, Label string
	Button        string // "<domain>.<label>" of the radio device's button
	From          string // the radio device
	EnforceEvery  string // HH:MM:SS
	Default       string
	OnLevel       int
	AssignedID    string
}

// governorStatusTopic is the governor's liveness, "online" or "offline".
const governorStatusTopic = "governed/status"

// governedTopic is where the governor keeps an entity's state; its "/set" takes commands.
func governedTopic(deviceID, label string) string {
	return "governed/" + deviceID + "/" + label
}

// TGovernedKind is what a governed domain is to the governor: its kind, the states a default may
// be, and the FS20 kinds that can carry it with their encoding.
type TGovernedKind struct {
	Kind     string
	States   []string
	Encoding map[string]string // FS20 kind -> encoding
}

// governedKinds are the domains a governed capability may have.
var governedKinds = map[string]TGovernedKind{
	"switch": {Kind: "switch", States: []string{"on", "off"}, Encoding: map[string]string{"switch": "fs20_switch"}},
}

// checkGovernedIntegration reports what is wrong in devices, given the FHEM devices they are
// reached through.
func checkGovernedIntegration(devices []TGovernedDevice, fhemDevices []TFHEMDevice) []string {
	radios := map[string]TFHEMDevice{}
	for _, d := range fhemDevices {
		radios[d.DeviceID] = d
	}
	var warnings []string
	for _, device := range devices {
		for _, c := range device.Capabilities {
			name := fmt.Sprintf("device %q capability %s.%s", device.DeviceID, c.Domain, c.Label)
			if c.Button == "" {
				if c.Domain+"."+c.Label != "binary_sensor.node" {
					warnings = append(warnings, name+": only its node may be written without \"by doing ...\"")
				}
				continue
			}
			kind, known := governedKinds[c.Domain]
			if !known {
				warnings = append(warnings, fmt.Sprintf("%s: a governed capability is a %s", name, strings.Join(sortedKeys(governedKinds), " or ")))
				continue
			}
			if c.Default != "" && !slices.Contains(kind.States, c.Default) {
				warnings = append(warnings, fmt.Sprintf("%s: default %q is not one of %v", name, c.Default, kind.States))
			}
			if c.EnforceEvery == "" {
				warnings = append(warnings, name+": no \"enforced every HH:MM:SS\"")
			} else if _, err := parseDurationHMS(c.EnforceEvery); err != nil {
				warnings = append(warnings, fmt.Sprintf("%s: %v", name, err))
			}
			radio, found := radios[c.From]
			switch {
			case !found:
				warnings = append(warnings, fmt.Sprintf("%s: %q is no device of \"integration fhem\"", name, c.From))
			case !slices.Contains(radio.Capabilities, c.Button):
				warnings = append(warnings, fmt.Sprintf("%s: %q has no %s", name, c.From, c.Button))
			case radio.FS20 != nil && kind.Encoding[radio.FS20.Kind] == "":
				warnings = append(warnings, fmt.Sprintf("%s: an FS20 %s cannot carry a %s", name, radio.FS20.Kind, c.Domain))
			}
		}
	}
	return warnings
}

// parseDurationHMS checks HH:MM:SS.
func parseDurationHMS(text string) (int, error) {
	var h, m, s int
	if n, err := fmt.Sscanf(text, "%d:%d:%d", &h, &m, &s); n != 3 || err != nil || m > 59 || s > 59 {
		return 0, fmt.Errorf("%q is no duration HH:MM:SS", text)
	}
	return h*3600 + m*60 + s, nil
}

// governedGatewayDevices are devices as raw-MQTT devices on the governor's topics, for Home
// Assistant.
func governedGatewayDevices(devices []TGovernedDevice) []TDiscoveryGatewayDevice {
	var gateways []TDiscoveryGatewayDevice
	for _, device := range devices {
		g := TDiscoveryGatewayDevice{DeviceID: device.DeviceID, Identifiers: []string{device.DeviceID}, Capabilities: map[string]TDiscoveryCapability{}, Raw: true, RawTopics: map[string]TRawTopicCapability{}, Retain: retainDeclared}
		for field, value := range device.Metadata {
			if g.Metadata == nil {
				g.Metadata = map[string]THostConstantAttribute{}
			}
			g.Metadata[field] = THostConstantAttribute{Value: value}
		}
		for _, c := range device.Capabilities {
			g.Capabilities[c.Label] = TDiscoveryCapability{Domain: c.Domain, Leaf: rawMQTTLeaf(device.DeviceID, c.Label), AssignedID: c.AssignedID}
			if c.Button == "" {
				g.RawTopics[c.Label] = TRawTopicCapability{Topic: governorStatusTopic, On: "online", Implied: true}
				continue
			}
			topic := governedTopic(device.DeviceID, c.Label)
			g.RawTopics[c.Label] = TRawTopicCapability{Topic: topic, Command: topic + "/set", Governed: true}
		}
		gateways = append(gateways, g)
	}
	return gateways
}

// TGovernorDevicesFile is the governor's devices.yaml (governor/config.go's TDevicesFile).
type TGovernorDevicesFile struct {
	Status   string                `yaml:"status"`
	Entities []TGovernorEntityFile `yaml:"entities"`
}

// TGovernorEntityFile is one governed entity in devices.yaml.
type TGovernorEntityFile struct {
	Device       string              `yaml:"device"`
	Label        string              `yaml:"label"`
	Kind         string              `yaml:"kind"`
	Topic        string              `yaml:"topic"`
	Default      string              `yaml:"default,omitempty"`
	EnforceEvery string              `yaml:"enforce_every"`
	Channel      string              `yaml:"channel"`
	Paths        []TGovernorPathFile `yaml:"paths"`
}

// TGovernorPathFile is one path of an entity in devices.yaml.
type TGovernorPathFile struct {
	Set      string `yaml:"set"`
	Received string `yaml:"received,omitempty"`
	Encoding string `yaml:"encoding"`
	Copies   int    `yaml:"copies"`
}

// governorDevicesFile is the governor's devices.yaml for devices, reached through fhemDevices.
func governorDevicesFile(devices []TGovernedDevice, fhemDevices []TFHEMDevice) TGovernorDevicesFile {
	radios := map[string]TFHEMDevice{}
	for _, d := range fhemDevices {
		radios[d.DeviceID] = d
	}
	file := TGovernorDevicesFile{Status: governorStatusTopic}
	for _, device := range devices {
		for _, c := range device.Capabilities {
			radio, found := radios[c.From]
			if c.Button == "" || !found || radio.FS20 == nil {
				continue
			}
			file.Entities = append(file.Entities, TGovernorEntityFile{
				Device: device.DeviceID, Label: c.Label, Kind: governedKinds[c.Domain].Kind,
				Topic: governedTopic(device.DeviceID, c.Label), Default: c.Default, EnforceEvery: c.EnforceEvery, Channel: "fs20",
				Paths: []TGovernorPathFile{{Set: fs20Topic(c.From) + "/set", Received: fs20Topic(c.From), Encoding: governedKinds[c.Domain].Encoding[radio.FS20.Kind], Copies: len(radio.FS20.HeardBy)}},
			})
		}
	}
	return file
}

// collectFHEMDevices returns the devices of Physical.def's "integration fhem" blocks.
func collectFHEMDevices(physicalContent string) []TFHEMDevice {
	blocks, _ := parseIntegrationBlocks(physicalContent, nil)
	var devices []TFHEMDevice
	for _, block := range blocks {
		if block.Name == "fhem" {
			_, blockDevices, _ := parseFHEMIntegrationBody(block.BodyLines)
			devices = append(devices, blockDevices...)
		}
	}
	return devices
}

// generateGovernedIntegrationOutputs is "integration governed"'s entry in integrationBodyParsers:
// the governor's devices.yaml and secrets.yaml. Its Home Assistant entities come from
// generateYAMLMQTTEntities, as for every raw device.
func generateGovernedIntegrationOutputs(bodyLines []string, ctx TPhysicalGenerationContext) error {
	devices, warnings := parseGovernedIntegrationBody(bodyLines)
	fhemDevices := collectFHEMDevices(ctx.PhysicalContent)
	warnings = append(warnings, checkGovernedIntegration(devices, fhemDevices)...)
	for _, w := range warnings {
		fmt.Printf("[integration governed] %s\n", w)
	}
	if !ctx.HasMQTTSecrets {
		fmt.Printf("[integration governed] no main broker -- no governor configuration written\n")
		return nil
	}
	var data strings.Builder
	encoder := yaml.NewEncoder(&data)
	encoder.SetIndent(2)
	if err := encoder.Encode(governorDevicesFile(devices, fhemDevices)); err != nil {
		return err
	}
	dir := filepath.Join(ctx.OutputRoot, "governor")
	if err := writeYAMLFile(filepath.Join(dir, "devices.yaml"), generatorHeader+data.String()); err != nil {
		return err
	}
	var secrets strings.Builder
	secrets.WriteString(generatorHeader + "mqtt:\n")
	writeMQTTSecretsFields(&secrets, "  ", ctx.MQTTSecrets)
	if err := writeYAMLFile(filepath.Join(dir, "secrets.yaml"), secrets.String()); err != nil {
		return err
	}
	return os.Chmod(filepath.Join(dir, "secrets.yaml"), 0o600)
}
