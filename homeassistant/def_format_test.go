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
capability binary_sensor.node bridge_0x00124b0029e8c409_connection_state_zigbee2mqtt as node_zigbee2mqtt_bridge_bridge_0x00124b0029e8c409_connection_state_zigbee2mqtt;   # trailing
  end;
  device switches.b with entity light.social: from light_with_a_rather_long_source_name_that_goes_on_and_on;
end;
end;`
	want := `physical layer with:
  integration discovery with:
      # a comment stays as written
    device sensors.a with:
      capability binary_sensor.node bridge_0x00124b0029e8c409_connection_state_zigbee2mqtt
          as node_zigbee2mqtt_bridge_bridge_0x00124b0029e8c409_connection_state_zigbee2mqtt;   # trailing
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
