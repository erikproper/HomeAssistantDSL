/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: CapabilityIcon
 *
 * PROJECT.md 30 (2026-11-03, Erik: "ALL integrations should have that with icon option"): a
 * capability line of any integration may end in "with icon: "<icon>";", the icon of the entities
 * positioned from it, ahead of a Defaults.def rule (resolveCapabilityDefaults). A "${...}" setting
 * may stand inside the quotes. It replaces a house's own "defaults:" block in Physical.def, used
 * for one entity only (Junglinster's reservoir).
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 03.11.2026
 *
 */

package main

import (
	"regexp"
	"strings"
)

// capabilityIconClausePattern is a capability line's trailing "with icon: "<icon>";".
var capabilityIconClausePattern = regexp.MustCompile(`^(.*\S)\s+with\s+icon:\s*"([^"]*)"\s*;\s*$`)

// cutCapabilityIcon returns line without its trailing "with icon: "<icon>"" (still ending in ";"),
// and the icon; line unchanged and "" when it has none.
func cutCapabilityIcon(line string) (rest, icon string) {
	m := capabilityIconClausePattern.FindStringSubmatch(line)
	if m == nil {
		return line, ""
	}
	return m[1] + ";", m[2]
}

// iconClauseSettingPattern finds a "with icon: "..."" clause, or a block's "icon: "..."" line,
// whose quotes hold a "${...}" setting.
var iconClauseSettingPattern = regexp.MustCompile(`((?:^|\s)icon:\s*")([^"]*\$\{[^"]*)(")`)

// resolveCapabilityIconSettings resolves the "${...}" settings inside every icon of Physical.def's
// content (collectLayerContent), so every integration's parser reads the icon itself.
func resolveCapabilityIconSettings(content string, settings map[string]string) string {
	if !strings.Contains(content, "${") {
		return content
	}
	return iconClauseSettingPattern.ReplaceAllStringFunc(content, func(clause string) string {
		m := iconClauseSettingPattern.FindStringSubmatch(clause)
		return m[1] + substituteSettingsVariables(m[2], settings) + m[3]
	})
}

// capabilityIconSettings returns the settings an icon may refer to.
func capabilityIconSettings(definitionDir string) map[string]string {
	return parseDefinitionAssignments(readCombinedSettingsContent(definitionDir))
}

// capabilityLabelPattern captures the label of a capability line, of any integration.
// The domain is optional: a hosts capability is "capability <name>: <entity>;".
var capabilityLabelPattern = regexp.MustCompile(`^(?:(?:internal|inverted)\s+){0,2}(?:derived\s+)?capability\s+(?:[A-Za-z_][A-Za-z0-9_]*\.)?([A-Za-z_][A-Za-z0-9_/]*):?(?:\s|;|$)`)

// capabilityDevicePattern captures the device id of a "device <id> ..." header.
var capabilityDevicePattern = regexp.MustCompile(`^device\s+(\S+)`)

// cutCapabilityIcons returns an integration block's body lines without their capabilities' "with
// icon: ..." (cutCapabilityIcon), for parsers that don't read it themselves, and the icons by
// "<device-id>!<label>".
func cutCapabilityIcons(bodyLines []string) ([]string, map[string]string) {
	icons := map[string]string{}
	out := make([]string, len(bodyLines))
	deviceID := ""
	for i, line := range bodyLines {
		out[i] = line
		code := stripLineComment(line)
		if m := capabilityDevicePattern.FindStringSubmatch(code); m != nil {
			deviceID = m[1]
			continue
		}
		m := capabilityLabelPattern.FindStringSubmatch(code)
		if m == nil {
			continue
		}
		if rest, icon := cutCapabilityIcon(code); icon != "" {
			out[i] = rest
			icons[deviceID+"!"+m[1]] = icon
		}
	}
	return out, icons
}
