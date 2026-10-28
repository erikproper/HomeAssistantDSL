/*
 *
 * Module:    HomeAssistant
 * Package:   HAMQTT
 * Component: Imported
 *
 * An imported entity's conceptual config (PROJECT.md 19h.4, 2026-10-27): an entity another
 * installation exports over the cloud broker, which the coordinator relays onto this house's own
 * imports/<entity>/state. What the exporting house says about it (its cloud catalogue: device
 * metadata, via_device, typing, payloads) is Remote; everything else is the importer's declaration.
 * Shared by the coordinator's relay and the generator's YAML entities, so the two can never differ.
 * The rules are documented at house_event_bus_coordinator/discoveryimport.go's
 * buildImportedDiscoveryBody.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 27.10.2026
 *
 */

package hamqtt

import "strings"

// ImportedStateTopic is where the coordinator relays an imported entity's state.
func ImportedStateTopic(localEntity string) string {
	return "imports/" + localEntity + "/state"
}

// ImportedUniqueID is an imported entity's unique_id: its frozen stable id, else the derived one.
func ImportedUniqueID(localEntity, stableID string) string {
	if stableID != "" {
		return stableID
	}
	return "imported_" + strings.ReplaceAll(localEntity, ".", "_")
}

// ImportedRemote is what the exporting installation's cloud catalogue says about an entity, as the
// coordinator publishes it on import_catalogue/state; ViaDevice is already the local device id.
type ImportedRemote struct {
	Manufacturer string `json:"manufacturer,omitempty"`
	Model        string `json:"model,omitempty"`
	ModelID      string `json:"model_id,omitempty"`
	HwVersion    string `json:"hw_version,omitempty"`
	SwVersion    string `json:"sw_version,omitempty"`
	SerialNumber string `json:"serial_number,omitempty"`
	ViaDevice    string `json:"via_device,omitempty"`
	DeviceClass  string `json:"device_class,omitempty"`
	Unit         string `json:"unit_of_measurement,omitempty"`
	StateClass   string `json:"state_class,omitempty"`
	Icon         string `json:"icon,omitempty"`
	PayloadOn    string `json:"payload_on,omitempty"`
	PayloadOff   string `json:"payload_off,omitempty"`
	// ExportID is the exporter's persistent identity for the capability (PROJECT.md 22): its
	// frozen universal ID, or "host_<hostname>_<attribute>" for a host; RemoteDeviceID the
	// exporter's current device id for it. The generator writes ExportID into the importer's
	// declaration ("from <export-id>") and reports a RemoteDeviceID that differs from it.
	ExportID       string `json:"export_id,omitempty"`
	RemoteDeviceID string `json:"remote_device_id,omitempty"`
}

// ImportedEntity is what an imported entity's config is built from.
type ImportedEntity struct {
	UniqueID      string
	LocalEntity   string
	LocalDeviceID string
	DeviceName    string
	Name          string // empty: "<device name>/<capability>"
	Capability    string
	SuggestedArea string
	Remote        ImportedRemote
	// NodeAvailabilityTopics: the device's own node (when it has one, and this isn't it) and every
	// "dependency on" node topic.
	NodeAvailabilityTopics []string
}

func importedDeviceBlock(localDeviceID, deviceName, suggestedArea string) map[string]interface{} {
	block := map[string]interface{}{"identifiers": []string{localDeviceID}, "name": deviceName}
	if suggestedArea != "" {
		block["suggested_area"] = suggestedArea
	}
	return block
}

// ImportedEntityConfig returns e's conceptual config. The caller adds what is its own (the
// coordinator its origin).
func ImportedEntityConfig(e ImportedEntity) map[string]interface{} {
	block := importedDeviceBlock(e.LocalDeviceID, e.DeviceName, e.SuggestedArea)
	for key, value := range map[string]string{
		"manufacturer": e.Remote.Manufacturer, "model": e.Remote.Model, "model_id": e.Remote.ModelID,
		"hw_version": e.Remote.HwVersion, "sw_version": e.Remote.SwVersion, "serial_number": e.Remote.SerialNumber,
		"via_device": e.Remote.ViaDevice,
	} {
		if value != "" {
			block[key] = value
		}
	}
	name := e.Name
	if name == "" {
		name = e.DeviceName + "/" + e.Capability
	}
	stateTopic := ImportedStateTopic(e.LocalEntity)
	body := map[string]interface{}{
		"unique_id":         e.UniqueID,
		"default_entity_id": e.LocalEntity,
		"name":              name,
		"state_topic":       stateTopic,
		"device":            block,
	}
	for key, value := range map[string]string{
		"device_class": e.Remote.DeviceClass, "unit_of_measurement": e.Remote.Unit,
		"state_class": e.Remote.StateClass, "icon": e.Remote.Icon,
	} {
		if value != "" {
			body[key] = value
		}
	}
	if strings.HasPrefix(e.LocalEntity, "binary_sensor.") {
		if e.Remote.PayloadOn != "" && e.Remote.PayloadOff != "" {
			body["payload_on"], body["payload_off"] = e.Remote.PayloadOn, e.Remote.PayloadOff
		} else {
			body["payload_on"], body["payload_off"] = StateOn, StateOff
		}
	}
	body["availability"] = ProxiedAvailability(stateTopic, e.NodeAvailabilityTopics...)
	body["availability_mode"] = "all"
	return body
}

// DerivedImportedEntity is an imported device's "derived" capability: a template over another of
// its capabilities' relayed state, needing nothing from the exporting house.
type DerivedImportedEntity struct {
	UniqueID      string
	LocalEntity   string
	LocalDeviceID string
	DeviceName    string
	Name          string // empty: "<device name>/<capability>"
	Capability    string
	SuggestedArea string
	BaseTopic     string
	Template      string
	// NodeAvailabilityTopics: as ImportedEntity's.
	NodeAvailabilityTopics []string
}

// DerivedImportedEntityConfig returns e's conceptual config.
func DerivedImportedEntityConfig(e DerivedImportedEntity) map[string]interface{} {
	name := e.Name
	if name == "" {
		name = e.DeviceName + "/" + e.Capability
	}
	body := map[string]interface{}{
		"unique_id":         e.UniqueID,
		"default_entity_id": e.LocalEntity,
		"name":              name,
		"state_topic":       e.BaseTopic,
		"value_template":    "{{ " + e.Template + " }}",
		"device":            importedDeviceBlock(e.LocalDeviceID, e.DeviceName, e.SuggestedArea),
	}
	if strings.HasPrefix(e.LocalEntity, "binary_sensor.") {
		body["payload_on"], body["payload_off"] = StateOn, StateOff
	}
	body["availability"] = ProxiedAvailability(e.BaseTopic, e.NodeAvailabilityTopics...)
	body["availability_mode"] = "all"
	return body
}
