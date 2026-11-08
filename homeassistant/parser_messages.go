/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: ParserMessages
 *
 * The messages of the definition parsers (GO_CONVENTIONS.md §8): Origin… names what is parsed,
 * Error… is a message with its details as placeholders, and Message… names what was expected.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Vienna, Austria, in collaboration with Claude.ai
 *
 * Version of: 08.11.2026
 *
 */

package main

// What is parsed.
const (
	OriginBatteryEmulation = "battery level emulation"
	OriginExternal         = "External.def"
	OriginFHEM             = "integration fhem"
	OriginGoverned         = "integration governed"
	OriginMQTTCapability   = "mqtt capability"
	OriginRule             = "rule %q"
	OriginShelly           = "shelly statement"
)

// Errors.
const (
	ErrorExpectedGot        = "expected %s, got %s"
	ErrorTrailingText       = "unexpected trailing input %s"
	ErrorNotA               = "%q is not %s"
	ErrorSecondOf           = "a second %s"
	ErrorNoDate             = "%s is no date"
	ErrorUndeclaredTemplate = "references undeclared \"jinja ${%s}(...)\" -- add a \"jinja ${%s}(...) = \"...\";\" line to Settings.def"
	ErrorTemplateArguments  = "calls \"jinja ${%s}(...)\" with %d argument(s), want %d"
)

// What was found or expected.
const (
	MessageEndOfStatement     = "the end of the statement"
	MessageUnterminatedString = "a string without its closing quote"

	MessageActionVerb          = "an action (turn, toggle, dim, brighten, press, stop, close, open, set, delay, start, pause or return_to_base)"
	MessageAlertPercentage     = "the alert's percentage (1 to 99)"
	MessageArgument            = "an argument"
	MessageBrightnessStep      = "the step in brightness (0 to 100)"
	MessageCapabilityLabel     = "a capability label"
	MessageCondition           = "a condition"
	MessageDataField           = "the name of a data field"
	MessageDate                = "a date, dd.mm.yyyy"
	MessageDaysACharge         = "the number of days a charge lasts"
	MessageDetailLevel         = "a detail level"
	MessageDeviceID            = "a device id"
	MessageDivisor             = "the number to divide by"
	MessageDomain              = "an entity's domain"
	MessageDuration            = "a duration, HH:MM:SS"
	MessageEmulatingInstance   = "the emulating Home Assistant instance"
	MessageEventType           = "an event type"
	MessageFHEMDeviceStatement = "fs20, capability or \"end\""
	MessageFHEMStatement       = "installation or device"
	MessageFS20Address         = "the FS20 address, quoted: 2 hex digits or 4 digits 1-4"
	MessageFS20HouseCode       = "the FS20 house code, quoted: 4 hex digits or 8 digits 1-4"
	MessageFS20Kind            = "an FS20 kind (switch, dimmer, transmitter or sensor)"
	MessageFS20Statement       = "fs20 statement"
	MessageFrozenRoot          = "the frozen unique_id root"
	MessageGovernedClause      = "enforced, default, on or the end of the capability"
	MessageGovernedStatement   = "a metadata field (manufacturer, model, ...) or capability"
	MessageHostName            = "a host name"
	MessageJSONField           = "the quoted name of a JSON field"
	MessageJinjaClause         = "a quoted Jinja expression or \"(\""
	MessageJinjaSource         = "the source of the template"
	MessageJinjaTemplate       = "a quoted Jinja template"
	MessageListOption          = "clean_prefix, clean_postfix, detail_level or \"end\""
	MessageListPattern         = "an entity pattern, e.g. binary_sensor.*/node"
	MessageListTitle           = "the list's quoted title"
	MessageMQTTDomain          = "a domain that can read a topic (binary_sensor, switch, sensor or event)"
	MessageMQTTTopic           = "a quoted MQTT topic"
	MessageNamePart            = "a part of a name"
	MessageNameRest            = "the rest of the name"
	MessageOnLevel             = "the level it is switched on at (1 to 100)"
	MessageOnOrOff             = "\"on\" or \"off\""
	MessageOnValue             = "the quoted value that means on"
	MessagePathSegment         = "a path segment"
	MessageQuotedValue         = "a quoted value"
	MessageReference           = "a reference (a capability label or an entity_id) or \"jinja\""
	MessageSerialKind          = "a serial device kind (cul or fhz)"
	MessageSerialPath          = "the serial device's quoted full path"
	MessageShellyID            = "the Shelly's quoted id, e.g. \"shellyplug-6CC3D8\""
	MessageSphere              = "an entity's sphere"
	MessageState               = "a state"
	MessageStateValues         = "\"available\", a quoted value or \"in [...]\""
	MessageTemplateName        = "the name of a Jinja template"
	MessageTimeCondition       = "\"after\", \"before\" or \"weekday\""
	MessageTimeOfDay           = "a time of day, HH:MM[:SS]"
	MessageTriggerAttribute    = "an attribute of the trigger"
	MessageTriggerPredicate    = "\"turns\", \"changes\" or \"is\""
	MessageUnit                = "a quoted unit"
	MessageValue               = "a value"
	MessageWeekday             = "a weekday"
)
