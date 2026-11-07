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
 * Version of: 07.11.2026
 *
 */

package main

// What is parsed.
const (
	OriginBatteryEmulation = "battery level emulation"
	OriginRule             = "rule %q"
)

// Errors.
const (
	ErrorExpectedGot        = "expected %s, got %s"
	ErrorTrailingText       = "unexpected trailing input %s"
	ErrorNotA               = "%q is not %s"
	ErrorNoDate             = "%s is no date"
	ErrorUndeclaredTemplate = "references undeclared \"jinja ${%s}(...)\" -- add a \"jinja ${%s}(...) = \"...\";\" line to Settings.def"
	ErrorTemplateArguments  = "calls \"jinja ${%s}(...)\" with %d argument(s), want %d"
)

// What was found or expected.
const (
	MessageEndOfStatement     = "the end of the statement"
	MessageUnterminatedString = "a string without its closing quote"

	MessageActionVerb        = "an action (turn, toggle, dim, brighten, press, stop, close, open, set, delay, start, pause or return_to_base)"
	MessageAlertPercentage   = "the alert's percentage (1 to 99)"
	MessageArgument          = "an argument"
	MessageBrightnessStep    = "the step in brightness (0 to 100)"
	MessageCondition         = "a condition"
	MessageDataField         = "the name of a data field"
	MessageDate              = "a date, dd.mm.yyyy"
	MessageDaysACharge       = "the number of days a charge lasts"
	MessageDeviceID          = "a device id"
	MessageDomain            = "an entity's domain"
	MessageDuration          = "a duration, HH:MM:SS"
	MessageEmulatingInstance = "the emulating Home Assistant instance"
	MessageEventType         = "an event type"
	MessageFrozenRoot        = "the frozen unique_id root"
	MessageJinjaClause       = "a quoted Jinja expression or \"(\""
	MessageJinjaSource       = "the source of the template"
	MessageJinjaTemplate     = "a quoted Jinja template"
	MessageNameRest          = "the rest of the name"
	MessageOnOrOff           = "\"on\" or \"off\""
	MessagePathSegment       = "a path segment"
	MessageQuotedValue       = "a quoted value"
	MessageReference         = "a reference (a capability label or an entity_id) or \"jinja\""
	MessageSphere            = "an entity's sphere"
	MessageState             = "a state"
	MessageStateValues       = "\"available\", a quoted value or \"in [...]\""
	MessageTemplateName      = "the name of a Jinja template"
	MessageTimeCondition     = "\"after\", \"before\" or \"weekday\""
	MessageTimeOfDay         = "a time of day, HH:MM[:SS]"
	MessageTriggerAttribute  = "an attribute of the trigger"
	MessageTriggerPredicate  = "\"turns\", \"changes\" or \"is\""
	MessageValue             = "a value"
	MessageWeekday           = "a weekday"
)
