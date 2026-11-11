package hamqtt

import (
	"reflect"
	"testing"
)

// TestDroppedFields: an unknown field is reported, written out; origin, object_id and "~" aren't.
func TestDroppedFields(t *testing.T) {
	payload := map[string]interface{}{
		"~": "x", "cmd_t": "~/set", "stat_t": "~/state", "uniq_id": "b", "origin": map[string]interface{}{}, "object_id": "b",
	}
	if got, want := DroppedFields(payload, "button"), []string{"state_topic"}; !reflect.DeepEqual(got, want) {
		t.Errorf("DroppedFields = %v, want %v", got, want)
	}
	if got := DroppedFields(map[string]interface{}{"stat_t": "x", "uniq_id": "s"}, "sensor"); len(got) != 0 {
		t.Errorf("DroppedFields(sensor) = %v, want none", got)
	}
}

// TestWrapValueTemplateGuarded: a template Zigbee2MQTT guards itself is unwrapped before the wrap,
// which then guards it the same way, keeping the last state when the key is missing.
func TestWrapValueTemplateGuarded(t *testing.T) {
	got := WrapValueTemplate(`{% if "power_b" in value_json %}{{ value_json["power_b"] }}{% endif %}`, "'on' if (($ | int(0)) > 50) else 'off'")
	want := `{% if "power_b" in value_json %}{{ 'on' if (((value_json["power_b"]) | int(0)) > 50) else 'off' }}{% else %}{{ this.state }}{% endif %}`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	if got := WrapValueTemplate(`{{ value_json["power"] }}`, "$ + 1"); got != `{% if "power" in value_json %}{{ (value_json["power"]) + 1 }}{% else %}{{ this.state }}{% endif %}` {
		t.Errorf("plain: %s", got)
	}
}
