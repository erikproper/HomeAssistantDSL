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
