/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: ParserMQTTCapability
 *
 * The grammar of the capability and "shelly" statements in Physical.def's "integration mqtt" block
 * (integration_mqtt.go, integration_mqtt_shelly.go):
 *
 *   MQTTCapability: "capability" Word "." Labelled Topicety AssignedIDety ";"
 *   ShellyStatement: "shelly" String ";"
 *   Topicety:       ( "topic" String DomainDetails )?  -- without: implied by the device's layer
 *   Labelled:       Word ( "/" Word )*
 *   DomainDetails:  "on" String                   -- binary_sensor
 *                   | "command" String            -- switch
 *                   | Unitety Divisorety          -- sensor
 *   Unitety:        ( "unit" String )?
 *   Divisorety:     ( "divided" "by" Number )?
 *   AssignedIDety:  ( "as" "unique_id" Word ( "." Word )* )?
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 07.11.2026
 *
 */

package main

import "math"

// TMQTTCapabilityParser embeds TDefParser.
type TMQTTCapabilityParser struct {
	TDefParser // the statement being parsed
	Domain     string
	Label      string
	AssignedID string
	Topic      TRawTopicCapability
	ShellyID   string
}

// --- top level ---

// parseMQTTCapabilityLine parses line as a capability statement. ok is false when it isn't a
// correct one; warnings say what is wrong with it.
func parseMQTTCapabilityLine(line string) (p *TMQTTCapabilityParser, ok bool) {
	p = &TMQTTCapabilityParser{}
	p.Initialise(line, OriginMQTTCapability)
	return p, p.MQTTCapability()
}

// parseShellyLine parses line as a "shelly" statement; ok is false when it isn't a correct one.
func parseShellyLine(line string) (p *TMQTTCapabilityParser, ok bool) {
	p = &TMQTTCapabilityParser{}
	p.Initialise(line, OriginShelly)
	return p, p.ShellyStatement()
}

// --- grammar ---

// MQTTCapability: "capability" Word "." Labelled Topicety AssignedIDety ";"
func (p *TMQTTCapabilityParser) MQTTCapability() bool {
	return p.ForcedThisWordWas("capability") &&
		/**/ p.ForcedThisWord(&p.Domain, MessageDomain) &&
		/**/ p.ForcedThisPunctuationWas('.') &&
		/**/ p.Labelled() &&
		/**/ p.Topicety() &&
		/**/ p.AssignedIDety() &&
		/**/ p.ForcedThisPunctuationWas(';') &&
		/**/ p.ForcedEndOfStatement()
}

// ShellyStatement: "shelly" String ";"
func (p *TMQTTCapabilityParser) ShellyStatement() bool {
	return p.ForcedThisWordWas("shelly") &&
		/**/ p.ForcedThisString(&p.ShellyID, MessageShellyID) &&
		/**/ p.ForcedThisPunctuationWas(';') &&
		/**/ p.ForcedEndOfStatement()
}

// Topicety: ( "topic" String DomainDetails )?; without it, the topic is implied (Topic.Implied).
func (p *TMQTTCapabilityParser) Topicety() bool {
	switch {
	case p.ThisWordWas("topic"):
		return p.ForcedThisString(&p.Topic.Topic, MessageMQTTTopic) &&
			/**/ p.DomainDetails()
	default:
		p.Topic.Implied = true
		return true
	}
}

// Labelled: Word ( "/" Word )*
func (p *TMQTTCapabilityParser) Labelled() bool {
	var segment string
	if !p.ForcedThisWord(&p.Label, MessageCapabilityLabel) {
		return false
	}
	for p.ThisPunctuationWas('/') {
		if !p.ForcedThisWord(&segment, MessageCapabilityLabel) {
			return false
		}
		p.Label += "/" + segment
	}
	return true
}

// DomainDetails: what a capability of its domain needs besides its topic.
func (p *TMQTTCapabilityParser) DomainDetails() bool {
	switch p.Domain {
	// "on" for that value, "off" for any other.
	case "binary_sensor":
		return p.ForcedThisWordWas("on") &&
			/**/ p.ForcedThisString(&p.Topic.On, MessageOnValue)

	// The topic that takes the commands.
	case "switch":
		return p.ForcedThisWordWas("command") &&
			/**/ p.ForcedThisString(&p.Topic.Command, MessageMQTTTopic)

	// The value as it is, or converted.
	case "sensor":
		return p.Unitety() &&
			/**/ p.Divisorety()

	default:
		return p.Failed(ErrorNotA, p.Domain, MessageMQTTDomain)
	}
}

// Unitety: ( "unit" String )?
func (p *TMQTTCapabilityParser) Unitety() bool {
	return p.ThisWordWas("unit") &&
		/**/ p.ForcedThisString(&p.Topic.Unit, MessageUnit) ||
		true
}

// Divisorety: ( "divided" "by" Number )?
func (p *TMQTTCapabilityParser) Divisorety() bool {
	return p.ThisWordWas("divided") &&
		/**/ p.ForcedThisWordWas("by") &&
		/**/ p.ForcedNumber(&p.Topic.DividedBy, 1, math.MaxInt, MessageDivisor) ||
		true
}

// AssignedIDety: ( "as" "unique_id" Word ( "." Word )* )?
func (p *TMQTTCapabilityParser) AssignedIDety() bool {
	var segment string
	if p.ThisWordWas("as") &&
		/**/ p.ForcedThisWordWas("unique_id") &&
		/**/ p.ForcedThisWord(&p.AssignedID, MessageFrozenRoot) {
		for p.ThisPunctuationWas('.') &&
			/**/ p.ForcedThisWord(&segment, MessageFrozenRoot) {
			p.AssignedID += "." + segment
		}
	}
	return true
}
