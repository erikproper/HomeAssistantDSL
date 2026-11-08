/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: ParserFHEM
 *
 * The grammar of Physical.def's "integration fhem" block (PROJECT.md 37,
 * claude-notes/plans/governed-integration.md): the FHEM installations, named by their host, with
 * their serial devices, and the FS20 devices they serve.
 *
 *   FHEMBody:        FHEMStatement*
 *   FHEMStatement:   Installation | FHEMDevice
 *   Installation:    "installation" Word "with" ":" SerialDevice* "end" ";"
 *   SerialDevice:    Word String ";"                    -- its kind (fhemSerialKinds) and full path
 *   FHEMDevice:      "device" DeviceID "with" ":" DeviceStatement* "end" ";"
 *   DeviceStatement: FS20Statement | Capability
 *   FS20Statement:   "fs20" Word String String "heard" "by" Word ( "," Word )* ";"
 *                                                       -- its kind, house code, address, installations
 *   Capability:      "capability" Word "." Word ";"
 *   DeviceID:        Word "." Word
 *
 * Comments ("#" to the end of the line) are taken out first.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 08.11.2026
 *
 */

package main

import (
	"strings"
)

// TFHEMParser embeds TDefParser.
type TFHEMParser struct {
	TDefParser    // an "integration fhem" block's body
	Installations []TFHEMInstallation
	Devices       []TFHEMDevice
}

// --- top level ---

// parseFHEMIntegrationBody parses an "integration fhem" block's body; warnings say what is wrong.
func parseFHEMIntegrationBody(bodyLines []string) ([]TFHEMInstallation, []TFHEMDevice, []string) {
	var lines []string
	for _, line := range bodyLines {
		lines = append(lines, stripLineComment(line))
	}
	p := &TFHEMParser{}
	p.Initialise(strings.Join(lines, "\n"), OriginFHEM)
	p.FHEMBody()
	return p.Installations, p.Devices, p.Warnings
}

// --- grammar ---

// FHEMBody: FHEMStatement*
func (p *TFHEMParser) FHEMBody() bool {
	for !p.AtEndOfTokens() {
		p.skipping = false
		if !p.FHEMStatement() {
			p.SkipToNextStatement()
		}
	}
	return true
}

// FHEMStatement: Installation | FHEMDevice
func (p *TFHEMParser) FHEMStatement() bool {
	switch {
	// An FHEM installation.
	case p.ThisWordIs("installation"):
		return p.Installation()

	// A device it serves.
	case p.ThisWordIs("device"):
		return p.FHEMDevice()

	default:
		return p.Failed(ErrorExpectedGot, MessageFHEMStatement, p.ThisTokenText())
	}
}

// Installation: "installation" Word "with" ":" SerialDevice* "end" ";"
func (p *TFHEMParser) Installation() bool {
	var installation TFHEMInstallation
	if !(p.ForcedThisWordWas("installation") &&
		/**/ p.ForcedThisWord(&installation.Host, MessageHostName) &&
		/**/ p.ForcedThisWordWas("with") &&
		/**/ p.ForcedThisPunctuationWas(':')) {
		return false
	}
	for !p.ThisWordIs("end") {
		if !p.SerialDevice(&installation) {
			return false
		}
	}
	return p.ForcedThisWordWas("end") &&
		/**/ p.ForcedThisPunctuationWas(';') &&
		/**/ p.AppendInstallation(installation)
}

// SerialDevice: Word String ";"
func (p *TFHEMParser) SerialDevice(installation *TFHEMInstallation) bool {
	var serial TFHEMSerial
	return p.ForcedThisWord(&serial.Kind, MessageSerialKind) &&
		/**/ p.KnownSerialKind(serial.Kind) &&
		/**/ p.ForcedThisString(&serial.Path, MessageSerialPath) &&
		/**/ p.ForcedThisPunctuationWas(';') &&
		/**/ p.AppendSerial(installation, serial)
}

// FHEMDevice: "device" DeviceID "with" ":" DeviceStatement* "end" ";"
func (p *TFHEMParser) FHEMDevice() bool {
	var device TFHEMDevice
	if !(p.ForcedThisWordWas("device") &&
		/**/ p.DeviceID(&device.DeviceID) &&
		/**/ p.ForcedThisWordWas("with") &&
		/**/ p.ForcedThisPunctuationWas(':')) {
		return false
	}
	for !p.ThisWordIs("end") {
		if !p.DeviceStatement(&device) {
			return false
		}
	}
	return p.ForcedThisWordWas("end") &&
		/**/ p.ForcedThisPunctuationWas(';') &&
		/**/ p.AppendDevice(device)
}

// DeviceStatement: FS20Statement | Capability
func (p *TFHEMParser) DeviceStatement(device *TFHEMDevice) bool {
	switch {
	// Its FS20 address.
	case p.ThisWordIs("fs20"):
		return p.FS20Statement(device)

	// A capability.
	case p.ThisWordIs("capability"):
		return p.Capability(device)

	default:
		return p.Failed(ErrorExpectedGot, MessageFHEMDeviceStatement, p.ThisTokenText())
	}
}

// FS20Statement: "fs20" Word String String "heard" "by" Word ( "," Word )* ";"
func (p *TFHEMParser) FS20Statement(device *TFHEMDevice) bool {
	fs20 := &TFS20Address{}
	if !(p.ForcedThisWordWas("fs20") &&
		/**/ p.ForcedThisWord(&fs20.Kind, MessageFS20Kind) &&
		/**/ p.KnownFS20Kind(fs20.Kind) &&
		/**/ p.ForcedThisString(&fs20.HouseCode, MessageFS20HouseCode) &&
		/**/ p.ValidFS20Code(fs20.HouseCode, 4, MessageFS20HouseCode) &&
		/**/ p.ForcedThisString(&fs20.Address, MessageFS20Address) &&
		/**/ p.ValidFS20Code(fs20.Address, 2, MessageFS20Address) &&
		/**/ p.ForcedThisWordWas("heard") &&
		/**/ p.ForcedThisWordWas("by")) {
		return false
	}
	for {
		var host string
		if !p.ForcedThisWord(&host, MessageHostName) {
			return false
		}
		fs20.HeardBy = append(fs20.HeardBy, host)
		if !p.ThisPunctuationWas(',') {
			break
		}
	}
	return p.ForcedThisPunctuationWas(';') &&
		/**/ p.SetFS20(device, fs20)
}

// Capability: "capability" Word "." Word ";"
func (p *TFHEMParser) Capability(device *TFHEMDevice) bool {
	var domain, label string
	return p.ForcedThisWordWas("capability") &&
		/**/ p.ForcedThisWord(&domain, MessageDomain) &&
		/**/ p.ForcedThisPunctuationWas('.') &&
		/**/ p.ForcedThisWord(&label, MessageCapabilityLabel) &&
		/**/ p.ForcedThisPunctuationWas(';') &&
		/**/ p.AppendCapability(device, domain+"."+label)
}

// DeviceID: Word "." Word
func (p *TFHEMParser) DeviceID(target *string) bool {
	var category, name string
	return p.ForcedThisWord(&category, MessageDeviceID) &&
		/**/ p.ForcedThisPunctuationWas('.') &&
		/**/ p.ForcedThisWord(&name, MessageDeviceID) &&
		/**/ p.SetText(target, category+"."+name)
}

// --- checks ---

// KnownSerialKind fails at a serial device kind fhemSerialKinds doesn't have.
func (p *TFHEMParser) KnownSerialKind(kind string) bool {
	_, known := fhemSerialKinds[kind]
	return known ||
		p.Failed(ErrorNotA, kind, MessageSerialKind)
}

// KnownFS20Kind fails at an FS20 kind fs20Kinds doesn't have.
func (p *TFHEMParser) KnownFS20Kind(kind string) bool {
	_, known := fs20Kinds[kind]
	return known ||
		p.Failed(ErrorNotA, kind, MessageFS20Kind)
}

// ValidFS20Code fails at an FS20 code that is neither hexadecimal (digits hex digits, as FHEM
// writes it) nor ELV's notation (twice as many digits 1 to 4).
func (p *TFHEMParser) ValidFS20Code(code string, digits int, what string) bool {
	return isFS20Code(code, digits) ||
		p.Failed(ErrorNotA, code, what)
}

// --- small helpers ---

// SkipToNextStatement skips what is left of a statement that failed, up to the next
// "installation" or "device".
func (p *TFHEMParser) SkipToNextStatement() bool {
	p.NextToken()
	for !p.AtEndOfTokens() && !p.ThisWordIs("installation") && !p.ThisWordIs("device") {
		p.NextToken()
	}
	return true
}

// SetText sets *target to value.
func (p *TFHEMParser) SetText(target *string, value string) bool {
	*target = value
	return true
}

// AppendSerial adds a serial device to installation.
func (p *TFHEMParser) AppendSerial(installation *TFHEMInstallation, serial TFHEMSerial) bool {
	installation.Serials = append(installation.Serials, serial)
	return true
}

// AppendInstallation adds a parsed installation.
func (p *TFHEMParser) AppendInstallation(installation TFHEMInstallation) bool {
	p.Installations = append(p.Installations, installation)
	return true
}

// SetFS20 gives device its FS20 address, failing at a second one.
func (p *TFHEMParser) SetFS20(device *TFHEMDevice, fs20 *TFS20Address) bool {
	if device.FS20 != nil {
		return p.Failed(ErrorSecondOf, MessageFS20Statement)
	}
	device.FS20 = fs20
	return true
}

// AppendCapability adds a capability to device.
func (p *TFHEMParser) AppendCapability(device *TFHEMDevice, capability string) bool {
	device.Capabilities = append(device.Capabilities, capability)
	return true
}

// AppendDevice adds a parsed device.
func (p *TFHEMParser) AppendDevice(device TFHEMDevice) bool {
	p.Devices = append(p.Devices, device)
	return true
}
