package main

import (
	"strings"
	"testing"
)

// defStatements is a def's content as the parser reads it -- statements joined, whitespace collapsed
// -- for tests that check what a writer wrote, not how it is laid out (PROJECT.md 22c).
func defStatements(content string) string {
	lines := strings.Split(joinWrappedStatementContinuations(content), "\n")
	for i, line := range lines {
		lines[i] = strings.Join(strings.Fields(line), " ")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// TestFormatDefContent: nesting indentation, a long statement broken before its clause keywords
// (never after a ":"), comments and rule clauses untouched, the same meaning, and idempotent.
func TestFormatDefContent(t *testing.T) {
	in := `physical layer with:
integration discovery with:
      # a comment stays as written
  device sensors.a with:
capability binary_sensor.node bridge_0x00124b0029e8c409_connection_state_zigbee2mqtt as unique_id node_zigbee2mqtt_bridge_bridge_0x00124b0029e8c409_connection_state_zigbee2mqtt;   # trailing
  end;
  device switches.b with entity light.social: from light_with_a_rather_long_source_name_that_goes_on_and_on;
end;
end;`
	want := `physical layer with:
  integration discovery with:
      # a comment stays as written
    device sensors.a with:
      capability binary_sensor.node bridge_0x00124b0029e8c409_connection_state_zigbee2mqtt
        as unique_id
          node_zigbee2mqtt_bridge_bridge_0x00124b0029e8c409_connection_state_zigbee2mqtt;   # trailing
    end;
    device switches.b with entity light.social: from light_with_a_rather_long_source_name_that_goes_on_and_on;
  end;
end;`
	got := formatDefContent(in)
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if again := formatDefContent(got); again != got {
		t.Errorf("not idempotent:\n%s", again)
	}
	if defStatements(stripComments(got)) != defStatements(stripComments(in)) {
		t.Errorf("meaning changed")
	}
}

func stripComments(content string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = stripLineComment(line)
	}
	return strings.Join(lines, "\n")
}

// TestFormatDefAlignsSettings: the "=" of consecutive settings lines aligned; a blank line starts
// a new run.
func TestFormatDefAlignsSettings(t *testing.T) {
	in := "${a} = \"1\";\n${longer_name}   =   \"2\";\n\n${x} = \"3\";"
	want := "${a}           = \"1\";\n${longer_name} = \"2\";\n\n${x} = \"3\";"
	if got := formatDefContent(in); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

// TestFormatDefFoldsSingleMemberBlocks: a block holding one statement gets its one-line form where
// the grammar has one (PROJECT.md 30); otherwise, or with a comment, it stays a block. An "else:"
// is written at its "then if"'s level and folds without taking the shared "end;".
func TestFormatDefFoldsSingleMemberBlocks(t *testing.T) {
	in := "device buttons.a with:\n  entity sensor.infrastructural:battery_level;\nend;\n" +
		"device buttons.b with:\n  entity light.social:main from light not aggregated;\nend;\n" +
		"derived capability binary_sensor.n as unique_id x with:\n  condition node.tv;\nend;\n" +
		"then:\n  dim light.social:main with 5%;\nend;\n" +
		"device buttons.c as leaf door with:\n  entity sensor.infrastructural:battery_level;\nend;\n" +
		"device buttons.d with:\n  entity sensor.infrastructural:battery_level; # kept\nend;\n" +
		"for binary_sensor.*/door:\n  icon: \"mdi:door-open\";\nend;\n" +
		"device node.tv host sony-tv ping with:\n  manufacturer: \"Sony\";\nend;\n" +
		"absorb from node.chime:\n  capability binary_sensor.node;\nend;\n" +
		"space social:x with:\n  device buttons.e with:\n    entity light.social:;\n  end;\nend;\n" +
		"rule social:cover with:\n  then if cover.social: is opening:\n    stop cover.social:;\n    else:\n      open cover.social:;\n    end;\nend;"
	want := "device buttons.a with entity sensor.infrastructural:battery_level;\n" +
		"device buttons.b with entity light.social:main from light not aggregated;\n" +
		"derived capability binary_sensor.n as unique_id x with condition node.tv;\n" +
		"then dim light.social:main with 5%;\n" +
		"device buttons.c as leaf door with entity sensor.infrastructural:battery_level;\n" +
		"device buttons.d with:\n  entity sensor.infrastructural:battery_level; # kept\nend;\n" +
		"for binary_sensor.*/door with icon: \"mdi:door-open\";\n" +
		"device node.tv host sony-tv ping with manufacturer: \"Sony\";\n" +
		"absorb from node.chime capability binary_sensor.node;\n" +
		"space social:x with:\n  device buttons.e with entity light.social:;\nend;\n" +
		"rule social:cover with:\n  then if cover.social: is opening:\n    stop cover.social:;\n  else open cover.social:;\n  end;\nend;"
	if got := formatDefContent(in); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if again := formatDefContent(want); again != want {
		t.Errorf("not idempotent:\n%s", again)
	}
}

// TestFormatDefBreaksLongBlockHeader: a "... with:" header longer than the line width is broken
// before its clause keywords like a finished statement, "with:" staying on its last line.
func TestFormatDefBreaksLongBlockHeader(t *testing.T) {
	in := "capability device_tracker.position device_tracker.volvo_xc40_location as unique_id hassbridge_device_tracker_position with:\n  attribute latitude: device_tracker.volvo_xc40_location!latitude;\nend;"
	want := "capability device_tracker.position device_tracker.volvo_xc40_location\n  as unique_id hassbridge_device_tracker_position with:\n  attribute latitude: device_tracker.volvo_xc40_location!latitude;\nend;"
	if got := formatDefContent(in); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if again := formatDefContent(want); again != want {
		t.Errorf("not idempotent:\n%s", again)
	}
}

// TestFormatDefBreaksLongCondition: a too-long condition is broken after "condition", one
// top-level operand per line ending in its operator; parentheses stay whole.
func TestFormatDefBreaksLongCondition(t *testing.T) {
	in := "device appliance.sonos with:\n  derived capability binary_sensor.node as unique_id x\n" +
		"      with condition physical appliance.sonos_playbar and node.sonos_playbar and (node.left or node.right) and node.subwoofer;\n" +
		"  condition (physical appliance.washing_machine and node.candy_washing_machine_ping) or not switch.switch;\nend;"
	want := "device appliance.sonos with:\n  derived capability binary_sensor.node as unique_id x\n    with condition\n" +
		"      physical appliance.sonos_playbar and\n      node.sonos_playbar and\n      (node.left or node.right) and\n      node.subwoofer;\n" +
		"  condition\n    (physical appliance.washing_machine and node.candy_washing_machine_ping) or\n    not switch.switch;\nend;"
	if got := formatDefContent(in); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if again := formatDefContent(want); again != want {
		t.Errorf("not idempotent:\n%s", again)
	}
}
