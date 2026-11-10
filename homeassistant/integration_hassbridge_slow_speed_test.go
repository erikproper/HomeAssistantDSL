package main

import (
	"strings"
	"testing"
)

// TestSlowSpeedWithoutSource: "slow speed;" (Home Assistant 2026.11, Overkiz's low-speed entity
// deprecated) makes the shadow its cover's own entity at speed "slow", with the cover's position;
// "slow speed <source>;" keeps its own source, without a speed.
func TestSlowSpeedWithoutSource(t *testing.T) {
	devices, warnings := parseHassBridgeIntegrationBody([]string{
		"device covers.shower_room with:",
		"  capability cover.cover cover.shower_room with:",
		"    derived value cover.shower_room!current_position;",
		"    slow speed;",
		"  end;",
		"  capability cover.other cover.other with:",
		"    slow speed cover.other_low_speed;",
		"  end;",
		"end;",
	}, "ha2mqtt")
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	slow := devices[0].Capabilities["slow/cover"]
	if slow.Source != "cover.shower_room" || slow.Speed != "slow" || slow.Position != "cover.shower_room!current_position" {
		t.Errorf("slow/cover = %+v", slow)
	}
	if other := devices[0].Capabilities["slow/other"]; other.Source != "cover.other_low_speed" || other.Speed != "" {
		t.Errorf("slow/other = %+v", other)
	}
}

// TestSlowSpeedFrozenIDLine: the frozen id of a slow-speed shadow is found with and without a
// source on its line.
func TestSlowSpeedFrozenIDLine(t *testing.T) {
	for line, stripped := range map[string]string{
		"    slow speed as unique_id hassbridge_cover_x_slow;":                   "    slow speed",
		"    slow speed cover.x_low_speed as unique_id hassbridge_cover_x_slow;": "    slow speed cover.x_low_speed",
	} {
		m := slowSpeedAssignedIDPattern.FindStringSubmatch(line)
		if m == nil || m[1] != stripped || m[2] != "hassbridge_cover_x_slow" {
			t.Errorf("%q: %q", line, m)
		}
	}
}

// TestSlowSpeedCommands: a slow shadow's open and set position go to the cover's own entity with
// "speed: slow"; its stop takes no speed.
func TestSlowSpeedCommands(t *testing.T) {
	identity := TEntityIdentity{}
	var open, setPosition, stop commandSpec
	for _, c := range domainCommands["cover"] {
		switch c.Name {
		case "open":
			open = c
		case "set_position":
			setPosition = c
		case "stop":
			stop = c
		}
	}
	if body := entityCommandAutomationBody(identity, open, "cover.shower_room", "t", "slow"); !strings.Contains(body, "    target:\n      entity_id: cover.shower_room\n    data:\n      speed: slow\n") {
		t.Errorf("open:\n%s", body)
	}
	if body := entityCommandAutomationBody(identity, setPosition, "cover.shower_room", "t", "slow"); !strings.Contains(body, "      position: \"{{ trigger.payload }}\"\n      speed: slow\n") {
		t.Errorf("set_position:\n%s", body)
	}
	if body := entityCommandAutomationBody(identity, stop, "cover.shower_room", "t", "slow"); strings.Contains(body, "speed") || !strings.Contains(body, "    entity_id: cover.shower_room\n") {
		t.Errorf("stop:\n%s", body)
	}
}
