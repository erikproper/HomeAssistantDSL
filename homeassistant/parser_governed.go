/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: ParserGoverned
 *
 * The grammar of Physical.def's "integration governed" block (PROJECT.md 37,
 * claude-notes/plans/governed-integration.md): the governed devices and the entities the
 * governor keeps for them.
 *
 *   GovernedBody:       GovernedDevice*
 *   GovernedDevice:     "device" DeviceID "with" ":" GovernedStatement* "end" ";"
 *   GovernedStatement:  Metadata | GovernedCapability
 *   Metadata:           Word ":" String ";"               -- manufacturer, model, ...
 *   GovernedCapability: "capability" Word "." Word ( Doing Clause* )? AssignedIDety ";"
 *                                                         -- without Doing: its node
 *   Doing:              "by" "doing" Word "." Word "from" DeviceID
 *   Clause:             "enforced" "every" Duration | "default" Word | "on" "at" Number "%"
 *   AssignedIDety:      ( "as" "unique_id" Word ( "." Word )* )?
 *   Duration:           Word ":" Word ":" Word            -- HH:MM:SS
 *   DeviceID:           Word "." Word
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

// TGovernedParser embeds TDefParser.
type TGovernedParser struct {
	TDefParser // an "integration governed" block's body
	Devices    []TGovernedDevice
}

// --- top level ---

// parseGovernedIntegrationBody parses an "integration governed" block's body; warnings say what
// is wrong.
func parseGovernedIntegrationBody(bodyLines []string) ([]TGovernedDevice, []string) {
	var lines []string
	for _, line := range bodyLines {
		lines = append(lines, stripLineComment(line))
	}
	p := &TGovernedParser{}
	p.Initialise(strings.Join(lines, "\n"), OriginGoverned)
	p.GovernedBody()
	return p.Devices, p.Warnings
}

// --- grammar ---

// GovernedBody: GovernedDevice*
func (p *TGovernedParser) GovernedBody() bool {
	for !p.AtEndOfTokens() {
		p.skipping = false
		if !p.GovernedDevice() {
			p.SkipToNextDevice()
		}
	}
	return true
}

// GovernedDevice: "device" DeviceID "with" ":" GovernedStatement* "end" ";"
func (p *TGovernedParser) GovernedDevice() bool {
	device := TGovernedDevice{Metadata: map[string]string{}}
	if !(p.ForcedThisWordWas("device") &&
		/**/ p.DeviceID(&device.DeviceID) &&
		/**/ p.ForcedThisWordWas("with") &&
		/**/ p.ForcedThisPunctuationWas(':')) {
		return false
	}
	for !p.ThisWordIs("end") {
		if !p.GovernedStatement(&device) {
			return false
		}
	}
	return p.ForcedThisWordWas("end") &&
		/**/ p.ForcedThisPunctuationWas(';') &&
		/**/ p.AppendDevice(device)
}

// GovernedStatement: Metadata | GovernedCapability
func (p *TGovernedParser) GovernedStatement(device *TGovernedDevice) bool {
	switch {
	// A capability.
	case p.ThisWordIs("capability"):
		return p.GovernedCapability(device)

	// A metadata field.
	default:
		return p.Metadata(device)
	}
}

// Metadata: Word ":" String ";"
func (p *TGovernedParser) Metadata(device *TGovernedDevice) bool {
	var field, value string
	return p.ForcedThisWord(&field, MessageGovernedStatement) &&
		/**/ p.KnownMetadataField(field) &&
		/**/ p.ForcedThisPunctuationWas(':') &&
		/**/ p.ForcedThisString(&value, MessageQuotedValue) &&
		/**/ p.ForcedThisPunctuationWas(';') &&
		/**/ p.SetMetadata(device, field, value)
}

// GovernedCapability: "capability" Word "." Word ( Doing Clause* )? AssignedIDety ";"
func (p *TGovernedParser) GovernedCapability(device *TGovernedDevice) bool {
	var capability TGovernedCapability
	if !(p.ForcedThisWordWas("capability") &&
		/**/ p.ForcedThisWord(&capability.Domain, MessageDomain) &&
		/**/ p.ForcedThisPunctuationWas('.') &&
		/**/ p.ForcedThisWord(&capability.Label, MessageCapabilityLabel)) {
		return false
	}
	if p.ThisWordIs("by") {
		if !p.Doing(&capability) {
			return false
		}
		for !p.ThisWordIs("as") && !p.ThisPunctuationIs(';') {
			if !p.Clause(&capability) {
				return false
			}
		}
	}
	return p.AssignedIDety(&capability.AssignedID) &&
		/**/ p.ForcedThisPunctuationWas(';') &&
		/**/ p.AppendCapability(device, capability)
}

// Doing: "by" "doing" Word "." Word "from" DeviceID
func (p *TGovernedParser) Doing(capability *TGovernedCapability) bool {
	var domain, label string
	return p.ForcedThisWordWas("by") &&
		/**/ p.ForcedThisWordWas("doing") &&
		/**/ p.ForcedThisWord(&domain, MessageDomain) &&
		/**/ p.ForcedThisPunctuationWas('.') &&
		/**/ p.ForcedThisWord(&label, MessageCapabilityLabel) &&
		/**/ p.SetText(&capability.Button, domain+"."+label) &&
		/**/ p.ForcedThisWordWas("from") &&
		/**/ p.DeviceID(&capability.From)
}

// Clause: "enforced" "every" Duration | "default" Word | "on" "at" Number "%"
func (p *TGovernedParser) Clause(capability *TGovernedCapability) bool {
	switch {
	// How often its state is sent again.
	case p.ThisWordWas("enforced"):
		return p.ForcedThisWordWas("every") &&
			/**/ p.Duration(&capability.EnforceEvery)

	// Its state when the broker has none.
	case p.ThisWordWas("default"):
		return p.ForcedThisWord(&capability.Default, MessageState)

	// A light's level when it is switched on without one.
	case p.ThisWordWas("on"):
		return p.ForcedThisWordWas("at") &&
			/**/ p.ForcedNumber(&capability.OnLevel, 1, 100, MessageOnLevel) &&
			/**/ p.ForcedThisPunctuationWas('%')

	default:
		return p.Failed(ErrorExpectedGot, MessageGovernedClause, p.ThisTokenText())
	}
}

// AssignedIDety: ( "as" "unique_id" Word ( "." Word )* )?
func (p *TGovernedParser) AssignedIDety(target *string) bool {
	var segment string
	if p.ThisWordWas("as") &&
		/**/ p.ForcedThisWordWas("unique_id") &&
		/**/ p.ForcedThisWord(target, MessageFrozenRoot) {
		for p.ThisPunctuationWas('.') &&
			/**/ p.ForcedThisWord(&segment, MessageFrozenRoot) {
			*target += "." + segment
		}
	}
	return true
}

// Duration: Word ":" Word ":" Word, into *target as written (HH:MM:SS).
func (p *TGovernedParser) Duration(target *string) bool {
	var hours, minutes, seconds string
	return p.ForcedThisWord(&hours, MessageDuration) &&
		/**/ p.ForcedThisPunctuationWas(':') &&
		/**/ p.ForcedThisWord(&minutes, MessageDuration) &&
		/**/ p.ForcedThisPunctuationWas(':') &&
		/**/ p.ForcedThisWord(&seconds, MessageDuration) &&
		/**/ p.SetText(target, hours+":"+minutes+":"+seconds)
}

// DeviceID: Word "." Word
func (p *TGovernedParser) DeviceID(target *string) bool {
	var category, name string
	return p.ForcedThisWord(&category, MessageDeviceID) &&
		/**/ p.ForcedThisPunctuationWas('.') &&
		/**/ p.ForcedThisWord(&name, MessageDeviceID) &&
		/**/ p.SetText(target, category+"."+name)
}

// --- checks ---

// KnownMetadataField fails at a word that is no metadata field.
func (p *TGovernedParser) KnownMetadataField(field string) bool {
	return knownConstantDeviceAttributes[field] ||
		p.Failed(ErrorNotA, field, MessageGovernedStatement)
}

// --- small helpers ---

// SkipToNextDevice skips what is left of a device that failed, up to the next "device".
func (p *TGovernedParser) SkipToNextDevice() bool {
	p.NextToken()
	for !p.AtEndOfTokens() && !p.ThisWordIs("device") {
		p.NextToken()
	}
	return true
}

// SetText sets *target to value.
func (p *TGovernedParser) SetText(target *string, value string) bool {
	*target = value
	return true
}

// SetMetadata gives device a metadata field.
func (p *TGovernedParser) SetMetadata(device *TGovernedDevice, field, value string) bool {
	device.Metadata[field] = value
	return true
}

// AppendCapability adds a capability to device.
func (p *TGovernedParser) AppendCapability(device *TGovernedDevice, capability TGovernedCapability) bool {
	device.Capabilities = append(device.Capabilities, capability)
	return true
}

// AppendDevice adds a parsed device.
func (p *TGovernedParser) AppendDevice(device TGovernedDevice) bool {
	p.Devices = append(p.Devices, device)
	return true
}
