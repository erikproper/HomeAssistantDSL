/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationFHEM
 *
 * PROJECT.md 37 (claude-notes/plans/governed-integration.md): Physical.def's "integration fhem"
 * block (its grammar: parser_fhem.go) declares the FHEM installations, named by their host, their
 * serial devices (radio sticks), and the FS20 devices they serve:
 *
 *   integration fhem with:
 *     installation protocols-server-2 with:
 *       cul "/dev/serial/by-id/usb-busware.de_CUL868-if00";
 *     end;
 *     device radio.hallway_lamp with:
 *       fs20 switch "1234" "02" heard by protocols-server-2;
 *       capability button.set;
 *     end;
 *   end;
 *
 * The generator writes each installation's whole FHEM configuration, fhem/<host>/fhem.cfg (Erik's
 * old way, approach A: FHEM reached through its own MQTT), and its broker password apart, in
 * fhem/<host>/secrets; the deploy (integration_fhem_deploy.go) installs both. FHEM's MQTT2_CLIENT
 * connects to the main broker -- MQTT2 needs no Perl module beyond FHEM's own -- and per device:
 *
 *   - a command on fhem/fs20/<device>/set (from the governed application, never retained) is sent
 *     by FHEM's FS20 module: on/off, or for a dimmer 0..100 as the nearest of its 16 dim levels;
 *   - what FHEM's radio receives for the device -- a remote, another stick's frame, a sensor, and
 *     the echo of its own sends -- is published on fhem/fs20/<device>, retained for a sensor only.
 *     The governed application de-doubles what several installations hear (the plan, question 7).
 *
 * Resending is not FHEM's: the configuration has no refresh timer (Erik, 2026-11-08). Each
 * installation's own liveness is fhem/installation/<host>, "online" (retained, after each connect)
 * or "offline" (its last will).
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
	"slices"
	"strings"
)

// TFHEMInstallation is an "installation <host> with: ... end;": an FHEM installation and its
// serial devices.
type TFHEMInstallation struct {
	Host    string
	Serials []TFHEMSerial
}

// TFHEMSerial is a serial device of an installation: its kind (fhemSerialKinds) and its full
// path, as FHEM takes it ("COM3" on Windows).
type TFHEMSerial struct {
	Kind string
	Path string
}

// TFHEMDevice is a "device <id> with: ... end;" of the block.
type TFHEMDevice struct {
	DeviceID     string
	FS20         *TFS20Address
	Capabilities []string // "<domain>.<label>"
	// CopiesWithinMS: "copies within <n> ms;", the same message from the device again this soon is
	// a copy (another radio heard it), not a press -- 0: the radio channel's (the governor's).
	CopiesWithinMS int
}

// TFS20Address is "fs20 <kind> "<house code>" "<address>" heard by <host>, ...;".
type TFS20Address struct {
	Kind      string
	HouseCode string
	Address   string
	HeardBy   []string
}

// TFHEMSerialKind is what a serial device kind is to FHEM: the line defining it, and the radio
// channels it serves.
type TFHEMSerialKind struct {
	Define   func(name, path string) string
	Channels []string
}

// fhemSerialKinds are the serial devices an installation may have. A CUL's FHT id 0000 keeps it
// from sending FHT messages; FS20 needs none.
var fhemSerialKinds = map[string]TFHEMSerialKind{
	"cul": {Define: func(name, path string) string { return "define " + name + " CUL " + path + "@9600 0000" }, Channels: []string{"fs20"}},
	"fhz": {Define: func(name, path string) string { return "define " + name + " FHZ " + path }, Channels: []string{"fs20"}},
}

// TFS20Kind is an FS20 kind (the old FS20.Create's four): the capability it must have, and how
// what FHEM receives for it is published.
type TFS20Kind struct {
	Capability string // "<domain>.<label>"; a label "*" is any
	Takes      string // what its button takes: "on/off", "level", or "" (it only reports)
	Retained   bool   // what is received is a state, not an event
}

// fs20Kinds are the FS20 kinds a device may be.
var fs20Kinds = map[string]TFS20Kind{
	"switch": {Capability: "button.set", Takes: "on/off"},
	"dimmer": {Capability: "button.set", Takes: "level"},
	// A remote's presses as FHEM reports them -- on, off, and a held button's dimup/dimdown --
	// each its own event type: what a press means is the rules' (Erik, 2026-11-08), as for the
	// Philips dimmer remote. Unlike the old setup, no eventMap folds dim-up into on.
	"transmitter": {Capability: "event.button"},
	"sensor":      {Capability: "binary_sensor.*", Retained: true},
}

// fs20DimLevels are the dim levels an FS20 dimmer knows (FHEM's dim06% .. dim100%).
var fs20DimLevels = []int{6, 12, 18, 25, 31, 37, 43, 50, 56, 62, 68, 75, 81, 87, 93, 100}

var fs20HexCode = regexp.MustCompile(`^[0-9A-Fa-f]+$`)
var fs20ELVCode = regexp.MustCompile(`^[1-4]+$`)

// isFS20Code reports whether code is an FS20 code of digits hex digits, or of twice as many
// digits 1 to 4 (ELV's notation).
func isFS20Code(code string, digits int) bool {
	return len(code) == digits && fs20HexCode.MatchString(code) ||
		len(code) == 2*digits && fs20ELVCode.MatchString(code)
}

// fs20Topic is where FHEM publishes what it receives for device; its "/set" is where it takes
// commands.
func fs20Topic(deviceID string) string {
	return "fhem/fs20/" + deviceID
}

// fhemInstallationTopic is an installation's liveness, "online" or "offline".
func fhemInstallationTopic(host string) string {
	return "fhem/installation/" + host
}

// checkFHEMIntegration reports what is wrong between the block's installations and devices.
func checkFHEMIntegration(installations []TFHEMInstallation, devices []TFHEMDevice) []string {
	var warnings []string
	channels := map[string]map[string]bool{}
	for _, installation := range installations {
		if channels[installation.Host] != nil {
			warnings = append(warnings, fmt.Sprintf("installation %q: declared twice", installation.Host))
		}
		channels[installation.Host] = map[string]bool{}
		for _, serial := range installation.Serials {
			for _, channel := range fhemSerialKinds[serial.Kind].Channels {
				channels[installation.Host][channel] = true
			}
		}
	}
	for _, device := range devices {
		if device.FS20 == nil {
			warnings = append(warnings, fmt.Sprintf("device %q: no \"fs20 ...;\" statement", device.DeviceID))
			continue
		}
		for _, host := range device.FS20.HeardBy {
			switch {
			case channels[host] == nil:
				warnings = append(warnings, fmt.Sprintf("device %q: heard by %q, which is no installation of this block", device.DeviceID, host))
			case !channels[host]["fs20"]:
				warnings = append(warnings, fmt.Sprintf("device %q: heard by %q, which has no FS20 radio", device.DeviceID, host))
			}
		}
		kind := fs20Kinds[device.FS20.Kind]
		if len(device.Capabilities) != 1 || !capabilityMatches(device.Capabilities[0], kind.Capability) {
			warnings = append(warnings, fmt.Sprintf("device %q: an FS20 %s has exactly one capability, %s -- got %v", device.DeviceID, device.FS20.Kind, kind.Capability, device.Capabilities))
		}
	}
	return warnings
}

// capabilityMatches reports whether capability is pattern, whose label may be "*".
func capabilityMatches(capability, pattern string) bool {
	if domain, found := strings.CutSuffix(pattern, ".*"); found {
		return strings.HasPrefix(capability, domain+".")
	}
	return capability == pattern
}

// fhemConfig is installation's whole fhem.cfg: the global settings, its serial devices, the
// MQTT2_CLIENT to broker, and per device it hears its FS20 definition, its command listener and
// its publisher. The broker password is not in it (fhem/<host>/secrets).
func fhemConfig(installation TFHEMInstallation, devices []TFHEMDevice, broker TMQTTBrokerSecrets) string {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	b.WriteString(strings.ReplaceAll(generatorHeader, "###", "#"))
	line("# FHEM's configuration on %s (PROJECT.md 37). FHEM saves nothing back into it (autosave 0).", installation.Host)
	line("")
	line("attr global logfile ./log/fhem-%%Y-%%m.log")
	line("attr global modpath .")
	line("attr global statefile ./log/fhem.save")
	line("attr global verbose 3")
	line("attr global motd none")
	line("attr global autosave 0")
	line("define telnetPort telnet 7072")
	line("define WEB FHEMWEB 8083")
	line("define Logfile FileLog ./log/fhem-%%Y-%%m.log Logfile")

	line("")
	line("# Its serial devices.")
	fs20Radio := ""
	counts := map[string]int{}
	for _, serial := range installation.Serials {
		counts[serial.Kind]++
		name := fmt.Sprintf("%s_%d", serial.Kind, counts[serial.Kind])
		line("%s", fhemSerialKinds[serial.Kind].Define(name, serial.Path))
		if fs20Radio == "" && slices.Contains(fhemSerialKinds[serial.Kind].Channels, "fs20") {
			fs20Radio = name
		}
	}

	var heard []TFHEMDevice
	var subscriptions []string
	for _, device := range devices {
		if device.FS20 != nil && slices.Contains(device.FS20.HeardBy, installation.Host) {
			heard = append(heard, device)
			if fs20Kinds[device.FS20.Kind].Takes != "" {
				subscriptions = append(subscriptions, fs20Topic(device.DeviceID)+"/set")
			}
		}
	}
	if len(subscriptions) == 0 {
		subscriptions = []string{fhemInstallationTopic(installation.Host) + "/set"} // nothing; "#" would be all
	}

	line("")
	line("# The main broker.")
	line("define mqtt MQTT2_CLIENT %s:%s", broker.Server, broker.Port)
	line("attr mqtt username %s", broker.Login)
	line("attr mqtt clientId fhem_%s", installation.Host)
	line("attr mqtt autocreate no")
	line("attr mqtt subscriptions %s", strings.Join(subscriptions, " "))
	line("attr mqtt lwt %s offline", fhemInstallationTopic(installation.Host))
	line("attr mqtt lwtRetain 1")
	line("attr mqtt msgAfterConnect -r %s online", fhemInstallationTopic(installation.Host))
	if broker.TLS {
		line("attr mqtt SSL 1")
	}

	for _, device := range heard {
		writeFS20Device(line, device, fs20Radio)
	}
	return b.String()
}

// writeFS20Device writes device's FS20 definition, its command listener (when it takes
// commands) and its publisher.
func writeFS20Device(line func(string, ...any), device TFHEMDevice, radio string) {
	id, kind := device.DeviceID, fs20Kinds[device.FS20.Kind]
	quoted := regexp.QuoteMeta(id)
	line("")
	line("# %s: FS20 %s %s %s.", id, device.FS20.Kind, device.FS20.HouseCode, device.FS20.Address)
	line("define %s FS20 %s %s", id, device.FS20.HouseCode, device.FS20.Address)
	if radio != "" {
		line("attr %s IODev %s", id, radio)
	}
	switch kind.Takes {
	case "on/off":
		line("define %s.command MQTT2_DEVICE", id)
		line("attr %s.command IODev mqtt", id)
		line(`attr %s.command readingList %s/set:(on|off) { fhem("set %s $EVENT");; {} }`, id, regexp.QuoteMeta(fs20Topic(id)), id)
	case "level":
		line("define %s.command MQTT2_DEVICE", id)
		line("attr %s.command IODev mqtt", id)
		line(`attr %s.command readingList %s/set:.* { fhem("set %s " . %s);; {} }`, id, regexp.QuoteMeta(fs20Topic(id)), id, fs20DimExpression())
	}
	retain := ""
	if kind.Retained {
		retain = "-r "
	}
	line("define %s.publish notify %s:.* set mqtt publish %s%s $EVENT", id, quoted, retain, fs20Topic(id))
}

// fs20DimExpression is the Perl expression (";" doubled, as fhem.cfg wants it) turning $EVENT --
// "on", "off" or 0..100 -- into an FS20 command: on, off, or the nearest dim level ("off" below
// half the lowest). Anything else is sent as "off" -- never an unknown command.
func fs20DimExpression() string {
	levels := make([]string, len(fs20DimLevels))
	for i, level := range fs20DimLevels {
		levels[i] = fmt.Sprint(level)
	}
	return `do { my $v = $EVENT;; ` +
		`$v =~ /^(on|off)$/ ? $v : $v !~ /^\d+$/ || $v < ` + fmt.Sprint(fs20DimLevels[0]/2) + ` ? "off" : ` +
		`sprintf("dim%02d%%", (sort { abs($a - $v) <=> abs($b - $v) } (` + strings.Join(levels, ", ") + `))[0]) }`
}

// generateFHEMIntegrationOutputs is "integration fhem"'s entry in integrationBodyParsers: per
// installation its fhem/<host>/fhem.cfg and fhem/<host>/secrets.
func generateFHEMIntegrationOutputs(bodyLines []string, ctx TPhysicalGenerationContext) error {
	installations, devices, warnings := parseFHEMIntegrationBody(bodyLines)
	warnings = append(warnings, checkFHEMIntegration(installations, devices)...)
	for _, w := range warnings {
		fmt.Printf("[integration fhem] %s\n", w)
	}
	if !ctx.HasMQTTSecrets {
		fmt.Printf("[integration fhem] no main broker -- no FHEM configuration written\n")
		return nil
	}
	for _, installation := range installations {
		dir := filepath.Join(ctx.OutputRoot, "fhem", installation.Host)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "fhem.cfg"), []byte(fhemConfig(installation, devices, ctx.MQTTSecrets)), 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "secrets"), []byte(ctx.MQTTSecrets.Password+"\n"), 0o600); err != nil {
			return err
		}
	}
	return nil
}

// collectFHEMInstallations returns the hosts of Physical.def's FHEM installations, for the
// deploy plan.
func collectFHEMInstallations(physicalContent string) []string {
	blocks, _ := parseIntegrationBlocks(physicalContent, nil)
	var hosts []string
	for _, block := range blocks {
		if block.Name != "fhem" {
			continue
		}
		installations, _, _ := parseFHEMIntegrationBody(block.BodyLines)
		for _, installation := range installations {
			hosts = append(hosts, installation.Host)
		}
	}
	return hosts
}
