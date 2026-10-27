/*
 *
 * Module:    HouseEventBusCoordinator
 * Package:   Main
 * Component: DiscoveryDeviceInfo
 *
 * PROJECT.md 19b (2026-10-26): every declared discovery gateway's own device metadata --
 * manufacturer, model, model_id, hw_version, sw_version, configuration_url -- as its raw discovery
 * messages report it (the "device"/"dev" block), collected while relaying and published as one
 * retained aggregate, discovery_device_info/state ({gateway-id: {field: value}}), locally and on the
 * cloud broker under the installation's prefix -- the same pattern as discovery_existence/state.
 * The generator fetches it at generate time and writes it into Physical.def: metadata changes
 * rarely, so it no longer needs discovery to carry it (item 19).
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 26.10.2026
 *
 */

package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"homeassistant/hamqtt"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// DiscoveryDeviceInfoDebounceDelay batches the startup replay of every retained discovery message
// into one aggregate publish.
const DiscoveryDeviceInfoDebounceDelay = 3 * time.Second

// deviceInfoMetadataFields are the device-block fields captured as metadata.
var deviceInfoMetadataFields = []string{"manufacturer", "model", "model_id", "hw_version", "sw_version", "configuration_url", "serial_number"}

// deviceInfoFieldKeys maps each device-block key a gateway may use -- full, or MQTT discovery's
// abbreviation (EMS-ESP publishes "mf", "mdl", ...) per Home Assistant's own table -- to the
// metadata field it holds.
var deviceInfoFieldKeys = func() map[string]string {
	keys := map[string]string{}
	for _, field := range deviceInfoMetadataFields {
		keys[field] = field
	}
	for abbreviation, field := range hamqtt.Fields.DeviceAbbreviations {
		if _, metadata := keys[field]; metadata {
			keys[abbreviation] = field
		}
	}
	return keys
}()

// RecordFields merges a hosts device's live-reported metadata (TLiveDeviceInfoStore's snapshot,
// from its own ".../device/state" topic) into deviceID's, and reports whether anything changed --
// so generate captures it into Physical.def like a gateway's (PROJECT.md 19e, 2026-10-27).
func (t *TDiscoveryDeviceInfoTracker) RecordFields(deviceID string, fields map[string]string) bool {
	block := map[string]interface{}{}
	for key, value := range fields {
		block[key] = value
	}
	return t.Record(deviceID, map[string]interface{}{"device": block})
}

// TDiscoveryDeviceInfoTracker holds every gateway's metadata seen so far.
type TDiscoveryDeviceInfoTracker struct {
	mu            sync.Mutex
	byGateway     map[string]map[string]string
	debounceTimer *time.Timer
}

// discoveryDeviceInfo is the coordinator's one tracker, fed by subscribeDiscoveryBridge.
var discoveryDeviceInfo = &TDiscoveryDeviceInfoTracker{byGateway: map[string]map[string]string{}}

// Record merges the metadata in rawPayload's device block into gatewayID's, and reports whether
// anything changed. A gateway's sub-devices may publish a block with identifiers only (EMS-ESP's
// boiler, mixer, ...), which adds nothing.
func (t *TDiscoveryDeviceInfoTracker) Record(gatewayID string, rawPayload map[string]interface{}) bool {
	block, _ := rawPayload["device"].(map[string]interface{})
	if block == nil {
		block, _ = rawPayload["dev"].(map[string]interface{})
	}
	if block == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	changed := false
	for key, value := range block {
		field, known := deviceInfoFieldKeys[key]
		text, isText := value.(string)
		if !known || !isText || text == "" {
			continue
		}
		if t.byGateway[gatewayID] == nil {
			t.byGateway[gatewayID] = map[string]string{}
		}
		if t.byGateway[gatewayID][field] != text {
			t.byGateway[gatewayID][field] = text
			changed = true
		}
	}
	return changed
}

// discoveryDeviceInfoStatusTopic mirrors homeassistant/mqtt_discovery_device_info.go's copy.
func discoveryDeviceInfoStatusTopic() string {
	return "discovery_device_info/state"
}

// Publish publishes the aggregate, retained, locally and (with cloudClient) on the cloud broker.
func (t *TDiscoveryDeviceInfoTracker) Publish(mainClient, cloudClient mqtt.Client, ownInstallation string) error {
	t.mu.Lock()
	data, err := json.Marshal(t.byGateway)
	t.mu.Unlock()
	if err != nil {
		return fmt.Errorf("marshalling discovery device info: %w", err)
	}
	topic := discoveryDeviceInfoStatusTopic()
	if err := publishRetained(mainClient, topic, data); err != nil {
		return fmt.Errorf("publishing %s (local): %w", topic, err)
	}
	if cloudClient != nil {
		if err := publishRetained(cloudClient, ownInstallation+"/"+topic, data); err != nil {
			return fmt.Errorf("publishing %s (cloud): %w", ownInstallation+"/"+topic, err)
		}
	}
	return nil
}

// SchedulePublish publishes after DiscoveryDeviceInfoDebounceDelay without further changes.
func (t *TDiscoveryDeviceInfoTracker) SchedulePublish(mainClient, cloudClient mqtt.Client, ownInstallation string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.debounceTimer != nil {
		t.debounceTimer.Reset(DiscoveryDeviceInfoDebounceDelay)
		return
	}
	t.debounceTimer = time.AfterFunc(DiscoveryDeviceInfoDebounceDelay, func() {
		t.mu.Lock()
		t.debounceTimer = nil
		t.mu.Unlock()
		if err := t.Publish(mainClient, cloudClient, ownInstallation); err != nil {
			fmt.Printf("[discovery-device-info] %v\n", err)
		}
	})
}

// localBrokerInstallationTopic tells, retained on this house's own broker, which installation the
// broker belongs to -- both houses reach theirs by the same host name, so the generator checks it
// before trusting the local broker (homeassistant/mqtt_broker_reachability.go,
// verifyLocalBrokerInstallation).
const localBrokerInstallationTopic = "coordinator/installation"

// publishLocalBrokerInstallation publishes installation on localBrokerInstallationTopic, retained.
func publishLocalBrokerInstallation(client mqtt.Client, installation string) error {
	if installation == "" {
		return nil
	}
	return publishRetained(client, localBrokerInstallationTopic, []byte(installation))
}

// TDiscoveryRawConfigs holds every declared gateway's raw discovery configs, as the gateway
// published them -- declared in the defs or not -- keyed by "<raw domain>/<unique_id>"; published
// retained per gateway on discovery_raw_configs/<gateway-id>/state (PROJECT.md 19, 2026-10-27,
// Erik). The generator builds every YAML entity from them (hamqtt.RelayedConfig), so a capability
// declared for the first time gets its YAML on that very generate, never a discovery relay; the
// same data fills suggestions and completes the declaration in Physical.def.
type TDiscoveryRawConfigs struct {
	mu        sync.Mutex
	byGateway map[string]map[string]map[string]interface{}
	dirty     map[string]bool
	timer     *time.Timer
}

// discoveryRawConfigs is the coordinator's one collection, fed by subscribeDiscoveryBridge.
var discoveryRawConfigs = &TDiscoveryRawConfigs{byGateway: map[string]map[string]map[string]interface{}{}, dirty: map[string]bool{}}

func discoveryRawConfigsStatusTopic(gatewayID string) string {
	return "discovery_raw_configs/" + gatewayID + "/state"
}

// Record stores raw as gatewayID's config for rawDomain/uniqueID, and reports whether it changed.
func (c *TDiscoveryRawConfigs) Record(gatewayID, rawDomain, uniqueID string, raw map[string]interface{}) bool {
	data, err := json.Marshal(raw)
	if err != nil {
		return false
	}
	key := rawDomain + "/" + uniqueID
	c.mu.Lock()
	defer c.mu.Unlock()
	if previous, err := json.Marshal(c.byGateway[gatewayID][key]); err == nil && string(previous) == string(data) {
		return false
	}
	var copied map[string]interface{}
	if err := json.Unmarshal(data, &copied); err != nil {
		return false
	}
	if c.byGateway[gatewayID] == nil {
		c.byGateway[gatewayID] = map[string]map[string]interface{}{}
	}
	c.byGateway[gatewayID][key] = copied
	c.dirty[gatewayID] = true
	return true
}

// SchedulePublish publishes every changed gateway's configs, retained, locally and on the cloud
// broker, after DiscoveryDeviceInfoDebounceDelay without further changes.
func (c *TDiscoveryRawConfigs) SchedulePublish(mainClient, cloudClient mqtt.Client, ownInstallation string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.timer != nil {
		c.timer.Reset(DiscoveryDeviceInfoDebounceDelay)
		return
	}
	c.timer = time.AfterFunc(DiscoveryDeviceInfoDebounceDelay, func() {
		c.mu.Lock()
		c.timer = nil
		payloads := map[string][]byte{}
		for gatewayID := range c.dirty {
			if data, err := json.Marshal(c.byGateway[gatewayID]); err == nil {
				payloads[gatewayID] = data
			}
		}
		c.dirty = map[string]bool{}
		c.mu.Unlock()
		for gatewayID, data := range payloads {
			topic := discoveryRawConfigsStatusTopic(gatewayID)
			if err := publishRetained(mainClient, topic, data); err != nil {
				fmt.Printf("[discovery-raw-configs] publishing %s (local): %v\n", topic, err)
			}
			if cloudClient != nil {
				if err := publishRetained(cloudClient, ownInstallation+"/"+topic, data); err != nil {
					fmt.Printf("[discovery-raw-configs] publishing %s (cloud): %v\n", ownInstallation+"/"+topic, err)
				}
			}
		}
	})
}

// RecordBridgeDevice merges a "home_assistant" bridge device's live metadata (its store snapshot)
// and its resolved via_device into deviceID's, and reports whether anything changed (PROJECT.md
// 19h.3).
func (t *TDiscoveryDeviceInfoTracker) RecordBridgeDevice(deviceID string, fields map[string]string, viaDeviceID string) bool {
	changed := t.RecordFields(deviceID, fields)
	if viaDeviceID == "" {
		return changed
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.byGateway[deviceID] == nil {
		t.byGateway[deviceID] = map[string]string{}
	}
	if t.byGateway[deviceID]["via_device"] != viaDeviceID {
		t.byGateway[deviceID]["via_device"] = viaDeviceID
		changed = true
	}
	return changed
}

// THassBridgeTyping holds, per bridge entity (its local entity_id), what its source entity on the
// other instance reports of its typing -- unit, device_class, icon, learned by the existence
// inquiries -- published retained as hassbridge_typing/state, locally and on the cloud broker, for
// the generator's YAML (PROJECT.md 19h.3, 2026-10-27; the declaration still takes precedence).
type THassBridgeTyping struct {
	mu       sync.Mutex
	byEntity map[string]map[string]string
	timer    *time.Timer
}

// hassBridgeTyping is the coordinator's one collection, fed by the hassbridge relay.
var hassBridgeTyping = &THassBridgeTyping{byEntity: map[string]map[string]string{}}

func hassBridgeTypingStatusTopic() string {
	return "hassbridge_typing/state"
}

// Record stores localEntity's typing and, when it changed, schedules a publish.
func (c *THassBridgeTyping) Record(localEntity, unit, deviceClass, icon string, mainClient, cloudClient mqtt.Client, ownInstallation string) {
	typing := map[string]string{}
	for key, value := range map[string]string{"unit": unit, "device_class": deviceClass, "icon": icon} {
		if value != "" {
			typing[key] = value
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	previous, known := c.byEntity[localEntity]
	if known && fmt.Sprint(previous) == fmt.Sprint(typing) {
		return
	}
	c.byEntity[localEntity] = typing
	if c.timer != nil {
		c.timer.Reset(DiscoveryDeviceInfoDebounceDelay)
		return
	}
	c.timer = time.AfterFunc(DiscoveryDeviceInfoDebounceDelay, func() {
		c.mu.Lock()
		c.timer = nil
		data, err := json.Marshal(c.byEntity)
		c.mu.Unlock()
		if err != nil {
			fmt.Printf("[hassbridge-typing] marshalling: %v\n", err)
			return
		}
		topic := hassBridgeTypingStatusTopic()
		if err := publishRetained(mainClient, topic, data); err != nil {
			fmt.Printf("[hassbridge-typing] publishing %s (local): %v\n", topic, err)
		}
		if cloudClient != nil {
			if err := publishRetained(cloudClient, ownInstallation+"/"+topic, data); err != nil {
				fmt.Printf("[hassbridge-typing] publishing %s (cloud): %v\n", ownInstallation+"/"+topic, err)
			}
		}
	})
}

// TImportCatalogue holds, per imported entity (its local entity_id), what the exporting house's
// cloud catalogue says about it (hamqtt.ImportedRemote) -- published retained as
// import_catalogue/state, locally and on the cloud broker, for the generator's YAML (PROJECT.md
// 19h.4, 2026-10-27): an imported device's knowledge belongs to the exporting house, so it is
// cached at the importer, never written into its defs.
type TImportCatalogue struct {
	mu       sync.Mutex
	byEntity map[string]hamqtt.ImportedRemote
	timer    *time.Timer
}

// importCatalogue is the coordinator's one collection, fed by subscribeImportedDevices.
var importCatalogue = &TImportCatalogue{byEntity: map[string]hamqtt.ImportedRemote{}}

func importCatalogueStatusTopic() string {
	return "import_catalogue/state"
}

// Record stores localEntity's remote description, and reports whether it changed.
func (c *TImportCatalogue) Record(localEntity string, remote hamqtt.ImportedRemote) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if previous, known := c.byEntity[localEntity]; known && previous == remote {
		return false
	}
	c.byEntity[localEntity] = remote
	return true
}

// SchedulePublish publishes the catalogue, retained, locally and on the cloud broker, after
// DiscoveryDeviceInfoDebounceDelay without further changes.
func (c *TImportCatalogue) SchedulePublish(mainClient, cloudClient mqtt.Client, ownInstallation string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.timer != nil {
		c.timer.Reset(DiscoveryDeviceInfoDebounceDelay)
		return
	}
	c.timer = time.AfterFunc(DiscoveryDeviceInfoDebounceDelay, func() {
		c.mu.Lock()
		c.timer = nil
		data, err := json.Marshal(c.byEntity)
		c.mu.Unlock()
		if err != nil {
			fmt.Printf("[import-catalogue] marshalling: %v\n", err)
			return
		}
		topic := importCatalogueStatusTopic()
		if err := publishRetained(mainClient, topic, data); err != nil {
			fmt.Printf("[import-catalogue] publishing %s (local): %v\n", topic, err)
		}
		if cloudClient != nil {
			if err := publishRetained(cloudClient, ownInstallation+"/"+topic, data); err != nil {
				fmt.Printf("[import-catalogue] publishing %s (cloud): %v\n", ownInstallation+"/"+topic, err)
			}
		}
	})
}
