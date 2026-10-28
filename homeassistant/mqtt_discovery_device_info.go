/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: MQTTDiscoveryDeviceInfo
 *
 * PROJECT.md 19b (2026-10-26): every discovery device's own metadata -- manufacturer, model,
 * model_id, hw_version, sw_version, configuration_url -- captured into Physical.def. The
 * coordinator publishes what each gateway's discovery messages report as one retained aggregate
 * (house_event_bus_coordinator/discovery_device_info.go); generate fetches it (cloud broker first,
 * then the house's own -- verifyLocalBrokerInstallation makes sure that is really this house's),
 * caches it in Definitions/.cache/discovery_device_info.json for when no
 * broker is reachable, and writes it into each device's block in Physical.def:
 *
 *   device sensors.bedroom_frient with:
 *     identifiers "zigbee2mqtt_0x0015bc001a02b332";
 *     manufacturer: "Develco";
 *     model:        "Motion sensor";
 *     ...
 *
 * A "forced" value is the author's: it keeps its value, and a different discovered one is noted
 * next to it ("# discovery: \"...\""), so it stays visible what discovery provides.
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
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// discoveryMetadataFields are the captured metadata fields, in the order they're written.
var discoveryMetadataFields = []string{"manufacturer", "model", "model_id", "hw_version", "sw_version", "serial_number", "configuration_url", "via_device"}

// discoveryDeviceInfoStatusTopic mirrors house_event_bus_coordinator/discovery_device_info.go's.
func discoveryDeviceInfoStatusTopic() string {
	return "discovery_device_info/state"
}

func discoveryDeviceInfoCachePath(definitionDir string) string {
	return filepath.Join(definitionDir, ".cache", "discovery_device_info.json")
}

// fetchRetainedJSON reads topic's retained message from the broker in secrets into out.
func fetchRetainedJSON(secrets TMQTTBrokerSecrets, topic, clientID string, out interface{}) error {
	payload, err := fetchRetained(secrets, topic, clientID)
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, out)
}

// fetchRetained reads topic's retained message from the broker in secrets.
func fetchRetained(secrets TMQTTBrokerSecrets, topic, clientID string) ([]byte, error) {
	if err, known := brokerKnownUnreachable(secrets); known {
		return nil, fmt.Errorf("broker %s:%s already known unreachable this run: %w", secrets.Server, secrets.Port, err)
	}
	opts := mqtt.NewClientOptions()
	scheme := "tcp"
	if secrets.TLS {
		scheme = "ssl"
	}
	opts.AddBroker(fmt.Sprintf("%s://%s:%s", scheme, secrets.Server, secrets.Port))
	opts.SetClientID(clientID)
	opts.SetUsername(secrets.Login)
	opts.SetPassword(secrets.Password)
	opts.SetConnectTimeout(fetchDiscoveryExistenceTimeout)
	opts.SetAutoReconnect(false)
	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(fetchDiscoveryExistenceTimeout) || token.Error() != nil {
		err := fmt.Errorf("connecting to MQTT broker %s:%s: timed out", secrets.Server, secrets.Port)
		if tokenErr := token.Error(); tokenErr != nil {
			err = fmt.Errorf("connecting to MQTT broker %s:%s: %w", secrets.Server, secrets.Port, tokenErr)
		}
		markBrokerUnreachable(secrets, err)
		return nil, err
	}
	defer client.Disconnect(250)
	received := make(chan []byte, 1)
	subToken := client.Subscribe(topic, 0, func(_ mqtt.Client, msg mqtt.Message) {
		select {
		case received <- msg.Payload():
		default:
		}
	})
	if !subToken.WaitTimeout(fetchDiscoveryExistenceTimeout) || subToken.Error() != nil {
		return nil, fmt.Errorf("subscribing to %s: %v", topic, subToken.Error())
	}
	select {
	case payload := <-received:
		return payload, nil
	case <-time.After(fetchDiscoveryExistenceTimeout):
		return nil, fmt.Errorf("timed out waiting for %s", topic)
	}
}

// fetchRetainedAll reads every retained message matching filter (a wildcard topic) from the broker
// in secrets, keyed by topic: what arrives until the broker has been quiet for a moment.
func fetchRetainedAll(secrets TMQTTBrokerSecrets, filter, clientID string) (map[string][]byte, error) {
	if err, known := brokerKnownUnreachable(secrets); known {
		return nil, fmt.Errorf("broker %s:%s already known unreachable this run: %w", secrets.Server, secrets.Port, err)
	}
	opts := mqtt.NewClientOptions()
	scheme := "tcp"
	if secrets.TLS {
		scheme = "ssl"
	}
	opts.AddBroker(fmt.Sprintf("%s://%s:%s", scheme, secrets.Server, secrets.Port))
	opts.SetClientID(clientID)
	opts.SetUsername(secrets.Login)
	opts.SetPassword(secrets.Password)
	opts.SetConnectTimeout(fetchDiscoveryExistenceTimeout)
	opts.SetAutoReconnect(false)
	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(fetchDiscoveryExistenceTimeout) || token.Error() != nil {
		err := fmt.Errorf("connecting to MQTT broker %s:%s: timed out", secrets.Server, secrets.Port)
		if tokenErr := token.Error(); tokenErr != nil {
			err = fmt.Errorf("connecting to MQTT broker %s:%s: %w", secrets.Server, secrets.Port, tokenErr)
		}
		markBrokerUnreachable(secrets, err)
		return nil, err
	}
	defer client.Disconnect(250)
	var mu sync.Mutex
	messages := map[string][]byte{}
	arrived := make(chan struct{}, 1)
	subToken := client.Subscribe(filter, 0, func(_ mqtt.Client, msg mqtt.Message) {
		if !msg.Retained() {
			return
		}
		mu.Lock()
		messages[msg.Topic()] = append([]byte(nil), msg.Payload()...)
		mu.Unlock()
		select {
		case arrived <- struct{}{}:
		default:
		}
	})
	if !subToken.WaitTimeout(fetchDiscoveryExistenceTimeout) || subToken.Error() != nil {
		return nil, fmt.Errorf("subscribing to %s: %v", filter, subToken.Error())
	}
	quiet := time.NewTimer(fetchRetainedQuiet)
	defer quiet.Stop()
	for {
		select {
		case <-arrived:
			quiet.Reset(fetchRetainedQuiet)
		case <-quiet.C:
			mu.Lock()
			defer mu.Unlock()
			if len(messages) == 0 {
				return nil, fmt.Errorf("no retained messages on %s", filter)
			}
			return messages, nil
		}
	}
}

// fetchRetainedQuiet is how long fetchRetainedAll waits after the last retained message.
const fetchRetainedQuiet = 1500 * time.Millisecond

// fetchDiscoveryDeviceInfo returns the discovered metadata per gateway, from a broker when one
// answers (refreshing the cache), else from the cache.
func fetchDiscoveryDeviceInfo(definitionDir string, ctx TPhysicalGenerationContext) (map[string]map[string]string, error) {
	var info map[string]map[string]string
	fetched := false
	if cloudSecrets, ok := coordinatorOnlyBrokerSecrets(ctx); ok {
		fetched = fetchRetainedJSON(cloudSecrets, ctx.Installation+"/"+discoveryDeviceInfoStatusTopic(), "homeassistant-generator-device-info", &info) == nil
	}
	if !fetched {
		fetched = fetchRetainedJSON(ctx.MQTTSecrets, discoveryDeviceInfoStatusTopic(), "homeassistant-generator-device-info", &info) == nil
	}
	cachePath := discoveryDeviceInfoCachePath(definitionDir)
	if fetched {
		if data, err := json.Marshal(info); err == nil {
			_ = os.MkdirAll(filepath.Dir(cachePath), 0o755)
			_ = os.WriteFile(cachePath, data, 0o644)
		}
		return info, nil
	}
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, fmt.Errorf("no broker answered and no cache at %s yet", cachePath)
	}
	return info, json.Unmarshal(data, &info)
}

// captureDiscoveryMetadata writes the discovered metadata into Physical.def (see this file's header
// comment), and reports what it changed.
func captureDiscoveryMetadata(definitionDir string, gateways map[string]TDiscoveryGatewayDevice, hostDevices map[string]THostDevice, bridgeDevices map[string]THassBridgeDevice, ctx TPhysicalGenerationContext) []string {
	if !ctx.HasMQTTSecrets || len(gateways)+len(hostDevices)+len(bridgeDevices) == 0 {
		return nil
	}
	info, err := fetchDiscoveryDeviceInfo(definitionDir, ctx)
	if err != nil {
		return []string{fmt.Sprintf("device metadata: %v -- Physical.def left as it is", err)}
	}
	path := filepath.Join(definitionDir, "Physical.def")
	lines, err := readDefStatementLines(path) // one line per statement (PROJECT.md 22c)
	if err != nil {
		return []string{fmt.Sprintf("device metadata: %v", err)}
	}
	updated, changed := writeDiscoveryMetadata(strings.Join(lines, "\n"), gateways, info)
	updated, hostsChanged := writeHostMetadata(updated, hostDevices, info)
	changed += hostsChanged
	updated, bridgeChanged := writeBridgeMetadata(updated, bridgeDevices, info)
	changed += bridgeChanged
	if changed == 0 {
		return nil
	}
	if err := writeDefFormatted(path, strings.Split(updated, "\n")); err != nil {
		return []string{fmt.Sprintf("device metadata: %v", err)}
	}
	return []string{fmt.Sprintf("device metadata: updated %d device(s) in Physical.def", changed)}
}

// discoveryDeviceFlags are a discovery device's own device-level lines that may sit among its
// metadata (integration_discovery_parser.go), compared without a trailing comment.
var discoveryDeviceFlags = map[string]bool{
	"ignore other capabilities;": true,
	"ignore own availability;":   true, // PROJECT.md 22b
	"is retained;":               true, // PROJECT.md 20
	"is not retained;":           true,
}

var (
	metadataDeviceHeaderPattern = regexp.MustCompile(`^(\s*)device\s+([A-Za-z_][A-Za-z0-9_]*\.[A-Za-z_][A-Za-z0-9_]*)\s+with:\s*$`)
	metadataIdentifiersPattern  = regexp.MustCompile(`^\s*identifiers\s+"`)
)

// writeDiscoveryMetadata returns content with each gateway's metadata block rewritten from info,
// and the number of devices whose block changed. A device's metadata lines sit together, right
// after its identifiers, in discoveryMetadataFields order, aligned.
func writeDiscoveryMetadata(content string, gateways map[string]TDiscoveryGatewayDevice, info map[string]map[string]string) (string, int) {
	lines := strings.Split(content, "\n")
	var out []string
	changed := 0
	for i := 0; i < len(lines); i++ {
		header := metadataDeviceHeaderPattern.FindStringSubmatch(lines[i])
		out = append(out, lines[i])
		if header == nil {
			continue
		}
		deviceID := header[2]
		gateway, isGateway := gateways[deviceID]
		discovered := info[deviceID]
		if !isGateway || len(discovered) == 0 {
			continue
		}
		// The device's own leading lines, before its capabilities: identifiers, metadata, and
		// device-level flags ("ignore other capabilities;") -- every metadata line among them is
		// collected, so a flag written in between never leaves a second block behind (found live
		// 2026-10-27, with a flag right after the identifiers).
		j := i + 1
		var identifiers, oldBlock, flags []string
		indent := header[1] + "  "
		for ; j < len(lines); j++ {
			trimmed := strings.TrimSpace(lines[j])
			if metadataIdentifiersPattern.MatchString(lines[j]) {
				identifiers = append(identifiers, lines[j])
				indent = lines[j][:len(lines[j])-len(strings.TrimLeft(lines[j], " \t"))]
				continue
			}
			if m := discoveryMetadataPattern.FindStringSubmatch(quoteDeviceReference(trimmed)); m != nil && knownConstantDeviceAttributes[m[1]] {
				oldBlock = append(oldBlock, lines[j])
				continue
			}
			if discoveryDeviceFlags[stripLineComment(trimmed)] {
				flags = append(flags, lines[j])
				continue
			}
			break
		}
		block := metadataBlock(gateway.Metadata, discovered, indent)
		if strings.Join(block, "\n") != strings.Join(oldBlock, "\n") {
			changed++
		}
		out = append(out, identifiers...)
		out = append(out, block...)
		out = append(out, flags...)
		i = j - 1
	}
	return strings.Join(out, "\n"), changed
}

// metadataBlock renders one device's metadata lines: its forced values kept (with a differing
// discovered value noted), every other field from discovery.
func metadataBlock(current map[string]THostConstantAttribute, discovered map[string]string, indent string) []string {
	width := 0
	for _, field := range discoveryMetadataFields {
		if current[field].Forced || discovered[field] != "" || current[field].Value != "" {
			width = max(width, len(field)+1)
		}
	}
	var block []string
	for _, field := range discoveryMetadataFields {
		label := fmt.Sprintf("%-*s", width, field+":")
		// A device reference is written as one, unquoted ("via_device: utility.envoy;").
		value := func(v string) string {
			if field == "via_device" {
				return v
			}
			return fmt.Sprintf("%q", v)
		}
		if attribute, has := current[field]; has && attribute.Forced {
			line := fmt.Sprintf("%s%s %s forced;", indent, label, value(attribute.Value))
			if found := discovered[field]; found != "" && found != attribute.Value {
				line += fmt.Sprintf("   # discovery: %s", value(found))
			}
			block = append(block, line)
			continue
		}
		if found := discovered[field]; found != "" {
			block = append(block, fmt.Sprintf("%s%s %s;", indent, label, value(found)))
			continue
		}
		// Nothing reported for it: a value already there stays (2026-10-27).
		if attribute, has := current[field]; has && attribute.Value != "" {
			block = append(block, fmt.Sprintf("%s%s %s;", indent, label, value(attribute.Value)))
		}
	}
	return block
}

// metadataHostDevicePattern matches a hosts device's declaration, a one-liner or a block's header:
// "device <id> <host> <type> [cloud] [import];" / "... with:".
var metadataHostDevicePattern = regexp.MustCompile(`^(\s*)device\s+(\S+)\s+(\S+)\s+(\S+)((?:\s+(?:cloud|import))*)\s*(;|with:)\s*$`)

// writeHostMetadata returns content with each hosts device's metadata rewritten from info (what
// the device reports on its own ".../device/state", PROJECT.md 19e), and the number of devices
// whose metadata changed: the same lines and rules as a gateway's (metadataBlock), right after the
// device's header; a one-liner becomes a "with: ... end;" block once it has metadata.
func writeHostMetadata(content string, hostDevices map[string]THostDevice, info map[string]map[string]string) (string, int) {
	lines := strings.Split(content, "\n")
	var out []string
	changed := 0
	for i := 0; i < len(lines); i++ {
		header := metadataHostDevicePattern.FindStringSubmatch(lines[i])
		if header == nil {
			out = append(out, lines[i])
			continue
		}
		device, isHost := hostDevices[header[2]]
		discovered := info[header[2]]
		if !isHost || device.HostName != header[3] || device.IntegrationType != header[4] || len(discovered) == 0 {
			out = append(out, lines[i])
			continue
		}
		indent := header[1] + "  "
		block := metadataBlock(device.ConstantAttributes, discovered, indent)
		if header[6] == ";" {
			if len(block) == 0 {
				out = append(out, lines[i])
				continue
			}
			out = append(out, header[1]+"device "+header[2]+" "+header[3]+" "+header[4]+header[5]+" with:")
			out = append(out, block...)
			out = append(out, header[1]+"end;")
			changed++
			continue
		}
		// A block: its metadata lines (wherever they are) go, the rendered ones follow the header.
		var oldBlock, rest []string
		j := i + 1
		for ; j < len(lines); j++ {
			trimmed := strings.TrimSpace(lines[j])
			if trimmed == "end;" {
				break
			}
			if m := discoveryMetadataPattern.FindStringSubmatch(quoteDeviceReference(trimmed)); m != nil && isDiscoveryMetadataField(m[1]) {
				oldBlock = append(oldBlock, lines[j])
				continue
			}
			rest = append(rest, lines[j])
		}
		if strings.Join(block, "\n") != strings.Join(oldBlock, "\n") {
			changed++
		}
		out = append(out, lines[i])
		out = append(out, block...)
		out = append(out, rest...)
		i = j - 1
	}
	return strings.Join(out, "\n"), changed
}

// isDiscoveryMetadataField reports whether field is one of discoveryMetadataFields.
func isDiscoveryMetadataField(field string) bool {
	for _, known := range discoveryMetadataFields {
		if field == known {
			return true
		}
	}
	return false
}

// metadataBridgeHeaderPattern matches a bridge device's block header, exported or not:
// "device <id> [export [as <name>]] with:".
var metadataBridgeHeaderPattern = regexp.MustCompile(`^(\s*)device\s+([A-Za-z_][A-Za-z0-9_]*\.[A-Za-z_][A-Za-z0-9_]*)(?:\s+export(?:\s+as\s+\S+)?)?\s+with:\s*$`)

// writeBridgeMetadata returns content with each "home_assistant" bridge device's metadata
// rewritten from info (what the coordinator learns about it, via_device included, PROJECT.md
// 19h.3), and the number of devices whose metadata changed. A bridge device's block may hold its
// metadata anywhere, after its capabilities too, and nested "... with: ... end;" blocks: the
// rendered lines (metadataBlock) take the place of the first old one, or come last.
func writeBridgeMetadata(content string, bridgeDevices map[string]THassBridgeDevice, info map[string]map[string]string) (string, int) {
	lines := strings.Split(content, "\n")
	var out []string
	changed := 0
	for i := 0; i < len(lines); i++ {
		out = append(out, lines[i])
		header := metadataBridgeHeaderPattern.FindStringSubmatch(lines[i])
		if header == nil {
			continue
		}
		device, isBridge := bridgeDevices[header[2]]
		discovered := info[header[2]]
		if !isBridge || len(discovered) == 0 {
			continue
		}
		block := metadataBlock(device.ConstantAttributes, discovered, header[1]+"  ")
		var body, oldBlock []string
		at := -1
		depth := 0
		j := i + 1
		for ; j < len(lines); j++ {
			trimmed := strings.TrimSpace(lines[j])
			if trimmed == "end;" {
				if depth == 0 {
					break
				}
				depth--
			} else if strings.HasSuffix(trimmed, "with:") {
				depth++
			}
			if m := discoveryMetadataPattern.FindStringSubmatch(quoteDeviceReference(trimmed)); depth == 0 && m != nil && isDiscoveryMetadataField(m[1]) {
				if at < 0 {
					at = len(body)
				}
				oldBlock = append(oldBlock, lines[j])
				continue
			}
			body = append(body, lines[j])
		}
		if at < 0 {
			at = len(body)
		}
		if strings.Join(block, "\n") != strings.Join(oldBlock, "\n") {
			changed++
		}
		out = append(out, body[:at]...)
		out = append(out, block...)
		out = append(out, body[at:]...)
		i = j - 1
	}
	return strings.Join(out, "\n"), changed
}
