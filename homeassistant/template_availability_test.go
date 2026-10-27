package main

import "testing"

// TestTemplateAvailability: a derived value needs all its sources; an expression that tests
// availability itself gets none; an aggregate needs any member, or with all every member.
func TestTemplateAvailability(t *testing.T) {
	expr := "(states('sensor.a') | float(0)) - (states('sensor.b') | float(0)) + (states('sensor.a') | float(0))"
	if got, want := derivedValueAvailability(expr), "{{ has_value('sensor.a') and has_value('sensor.b') }}"; got != want {
		t.Errorf("derivedValueAvailability = %q, want %q", got, want)
	}
	if got := derivedValueAvailability("states('sensor.a') not in ['unavailable', 'unknown']"); got != "" {
		t.Errorf("an availability test got availability %q", got)
	}
	if got := derivedValueAvailability("now().hour > 6"); got != "" {
		t.Errorf("an expression without sources got availability %q", got)
	}
	members := []string{"light.a", "light.b"}
	if got, want := aggregateAvailability(members, false), "{{ ['light.a', 'light.b'] | select('has_value') | list | count > 0 }}"; got != want {
		t.Errorf("any = %q, want %q", got, want)
	}
	if got, want := aggregateAvailability(members, true), "{{ ['light.a', 'light.b'] | reject('has_value') | list | count == 0 }}"; got != want {
		t.Errorf("all = %q, want %q", got, want)
	}
}
