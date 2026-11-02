/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: ConceptualUnpositionedReport
 *
 * PROJECT.md 25d: suggestions at two levels. The other suggestion files list what the integrations
 * offer and Physical.def doesn't declare (to be positioned as capabilities); this one lists what
 * Physical.def and Logical.def declare and Conceptual.def doesn't position (to be positioned at the
 * conceptual layer): suggestions/conceptual.txt, per device, its unpositioned capabilities --
 * devices that are positioned first, then those never positioned at all.
 *
 * Not listed: "hidden" capabilities (they only feed a derived one), and capabilities another
 * device absorbs (positioned through that device). A capability that something else uses -- a
 * derived capability's source, a request's or node's condition, an "is switched by", a
 * "dependency on" (its node) -- is listed
 * with what uses it: it works without a position, and may not need one.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 02.11.2026
 *
 */

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// TUnpositionedDevice is one device with declared but unpositioned capabilities.
type TUnpositionedDevice struct {
	DeviceID     string
	Positioned   bool // some capability of the device is positioned (absorbed ones included)
	Capabilities []TUnpositionedCapability
}

// TUnpositionedCapability is one unpositioned capability, and what uses it anyway.
type TUnpositionedCapability struct {
	Name   string   // "<domain>.<capability>"
	UsedBy []string // e.g. "binary_sensor.consumes", "request.hallway_light_toggle", sorted
	label  string   // the capability's name
	leaf   string   // a discovered capability's raw leaf, which some registrations key on
}

// collectUnpositionedCapabilities lists, per declared device, the capabilities that may be
// positioned; writeUnpositionedCapabilitiesReport drops those that are, once every registration
// (export and firmware updates included) has run.
func collectUnpositionedCapabilities(admin *TAdministrationState, discoveryGatewaysByID map[string]TDiscoveryGatewayDevice, hassBridgeDevicesByID map[string]THassBridgeDevice, importedDevicesByID map[string]TImportedDevice, hostDevicesByID map[string]THostDevice, commandlineDevicesByID map[string]TCommandlineDevice, localDevicesByID map[string]TLocalDevice, logicalDevicesByID map[string]TLogicalDevice) []TUnpositionedDevice {
	declared := map[string]map[string]bool{}
	add := func(deviceID, capability string) {
		if declared[deviceID] == nil {
			declared[deviceID] = map[string]bool{}
		}
		declared[deviceID][capability] = true
	}
	absorbed := map[string]bool{}          // "<device-id>!<capability>"
	usedBy := map[string]map[string]bool{} // "<device-id>!<capability>" -> users
	use := func(deviceID, capability, user string) {
		key := deviceID + "!" + bareCapabilityName(capability)
		if usedBy[key] == nil {
			usedBy[key] = map[string]bool{}
		}
		usedBy[key][user] = true
	}
	for id, gateway := range discoveryGatewaysByID {
		for name, capability := range gateway.Capabilities {
			if !capability.Hidden {
				add(id, name)
			}
			for _, source := range []string{capability.DerivedFromCapability, capability.AvailabilityOf} {
				if source != "" {
					use(id, source, capability.Domain+"."+name)
				}
			}
		}
	}
	for id, device := range hassBridgeDevicesByID {
		for name, capability := range device.Capabilities {
			add(id, name)
			if capability.DerivedFromCapability != "" {
				use(id, capability.DerivedFromCapability, capability.Domain+"."+name)
			}
		}
	}
	for id, device := range importedDevicesByID {
		for name, capability := range device.Capabilities {
			add(id, name)
			if capability.DerivedFromCapability != "" {
				use(id, capability.DerivedFromCapability, capability.Domain+"."+name)
			}
		}
	}
	for id, device := range hostDevicesByID {
		if mat, known := MaterializationForIntegrationType(device.IntegrationType); known {
			add(id, "node")
			for _, name := range mat.AttributeNames(device) {
				add(id, name)
			}
		}
	}
	for id, device := range commandlineDevicesByID {
		add(id, "node")
		for name := range device.Capabilities {
			add(id, name)
		}
	}
	for id, device := range localDevicesByID {
		for name, capability := range device.Capabilities {
			add(id, name)
			for _, source := range []string{capability.DerivedFromCapability, capability.AvailabilityOf} {
				if source != "" {
					use(id, source, capability.Domain+"."+name)
				}
			}
		}
	}
	for id, device := range logicalDevicesByID {
		for name, capability := range device.Capabilities {
			add(id, name)
			if capability.AbsorbedFromDeviceID != "" {
				absorbed[capability.AbsorbedFromDeviceID+"!"+capability.AbsorbedFromCapability] = true
			}
			user := id
			if !strings.HasPrefix(id, "request.") {
				user = id + " " + capability.Domain + "." + name
			}
			for _, expr := range []*TConditionExpr{capability.DerivedCondition, capability.DerivedValue} {
				conditionCapabilityRefs(expr, id, func(deviceID, capability string) { use(deviceID, capability, user) })
			}
		}
		for label, fact := range device.SwitchedBy {
			use(fact.SourceDeviceID, fact.SourceLabel, id+" "+label)
		}
		for _, dependency := range device.DependsOn {
			use(dependency, "node", id+" (dependency)")
		}
	}

	var devices []TUnpositionedDevice
	for id, capabilities := range declared {
		device := TUnpositionedDevice{DeviceID: id}
		for name := range capabilities {
			if absorbed[id+"!"+name] {
				device.Positioned = true
				continue
			}
			domain, _ := lookupCapabilityDomain(name, id, discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, hostDevicesByID, commandlineDevicesByID, localDevicesByID, logicalDevicesByID)
			capability := TUnpositionedCapability{Name: domain + "." + name, label: name, leaf: discoveryGatewaysByID[id].Capabilities[name].Leaf}
			for user := range usedBy[id+"!"+name] {
				capability.UsedBy = append(capability.UsedBy, user)
			}
			sort.Strings(capability.UsedBy)
			device.Capabilities = append(device.Capabilities, capability)
		}
		if len(device.Capabilities) > 0 || device.Positioned {
			sort.Slice(device.Capabilities, func(i, j int) bool { return device.Capabilities[i].Name < device.Capabilities[j].Name })
			devices = append(devices, device)
		}
	}
	sort.Slice(devices, func(i, j int) bool { return devices[i].DeviceID < devices[j].DeviceID })
	return devices
}

// capabilityPositioned reports whether deviceID's capability (by its name, or its raw leaf) has a
// conceptual entity: positioned in Conceptual.def, or registered another way (an export, a firmware
// update, a request's physical event), each recorded in ConceptualUseBySource as
// "<device-id>!<capability>[!<kind>]".
func capabilityPositioned(admin *TAdministrationState, deviceID string, capability TUnpositionedCapability, usedSources map[string]bool) bool {
	if admin.PositionedCapabilities[deviceID][capability.label] {
		return true
	}
	for _, key := range []string{capability.label, capability.leaf} {
		if key != "" && usedSources[deviceID+"!"+key] {
			return true
		}
	}
	return false
}

// writeUnpositionedCapabilitiesReport writes <outputRoot>/suggestions/conceptual.txt from the
// candidates' unpositioned capabilities, or removes it when there are none.
func writeUnpositionedCapabilitiesReport(outputRoot string, admin *TAdministrationState, candidates []TUnpositionedDevice) error {
	usedSources := map[string]bool{} // "<device-id>!<capability>", without a "!<kind>" suffix
	for key := range admin.ConceptualUseBySource {
		parts := strings.SplitN(key, "!", 3)
		if len(parts) >= 2 {
			usedSources[parts[0]+"!"+parts[1]] = true
		}
	}
	var devices []TUnpositionedDevice
	for _, candidate := range candidates {
		device := TUnpositionedDevice{DeviceID: candidate.DeviceID, Positioned: candidate.Positioned || len(admin.DevicePositioningOccurrences[candidate.DeviceID]) > 0}
		for _, capability := range candidate.Capabilities {
			if capabilityPositioned(admin, candidate.DeviceID, capability, usedSources) {
				device.Positioned = true
			} else {
				device.Capabilities = append(device.Capabilities, capability)
			}
		}
		if len(device.Capabilities) > 0 {
			devices = append(devices, device)
		}
	}
	path := filepath.Join(outputRoot, "suggestions", "conceptual.txt")
	if len(devices) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	var sb strings.Builder
	for _, positioned := range []bool{true, false} {
		if positioned {
			sb.WriteString("# Positioned devices, with capabilities Conceptual.def doesn't position:\n\n")
		} else {
			sb.WriteString("# Devices Conceptual.def never positions:\n\n")
		}
		for _, device := range devices {
			if device.Positioned != positioned {
				continue
			}
			sb.WriteString(fmt.Sprintf("device %s with:\n", device.DeviceID))
			for _, capability := range device.Capabilities {
				line := "  " + capability.Name + ";"
				if len(capability.UsedBy) > 0 {
					line += " # used by " + strings.Join(capability.UsedBy, ", ")
				}
				sb.WriteString(line + "\n")
			}
			sb.WriteString("end;\n\n")
		}
	}
	return writeYAMLFile(path, sb.String())
}

// conditionCapabilityRefs calls found with every (device, capability) expr refers to: "<capability>
// from <device>", or the own device's capability when there's no "from".
func conditionCapabilityRefs(expr *TConditionExpr, ownDeviceID string, found func(deviceID, capability string)) {
	if expr == nil {
		return
	}
	if expr.Ref != "" {
		deviceID := ownDeviceID
		if expr.FromDeviceID != "" {
			deviceID = expr.FromDeviceID
		}
		found(deviceID, expr.Ref)
	}
	for _, child := range []*TConditionExpr{expr.Left, expr.Right, expr.Child} {
		conditionCapabilityRefs(child, ownDeviceID, found)
	}
}
