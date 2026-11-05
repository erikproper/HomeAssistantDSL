package main

import (
	"strings"
	"testing"
)

func TestParseChargeEstimateLine(t *testing.T) {
	decl, ok, warnings := parseChargeEstimateLine("derived capability sensor.battery_level by charge lasting 90 days since 31.10.2026;")
	if !ok || len(warnings) > 0 {
		t.Fatalf("not parsed: %v", warnings)
	}
	if decl.Domain != "sensor" || decl.Label != "battery_level" || decl.Days != 90 || decl.Since != "2026-10-31" {
		t.Errorf("declaration %+v", decl)
	}
}

func TestParseChargeEstimateLineLeavesOtherDerivedForms(t *testing.T) {
	for _, line := range []string{
		`derived capability binary_sensor.battery_alert from sensor.battery_level via "$ < 5";`,
		"derived capability binary_sensor.node with condition climate.climate is available;",
		"capability sensor.battery_level 0x1_battery_zigbee2mqtt;",
	} {
		if _, ok, warnings := parseChargeEstimateLine(line); ok || len(warnings) > 0 {
			t.Errorf("%q taken as a charge estimate (%v)", line, warnings)
		}
	}
}

func TestParseChargeEstimateLineReportsErrors(t *testing.T) {
	for line, want := range map[string]string{
		"derived capability sensor.battery_level by charge lasting ninety days since 31.10.2026;": "no number of days",
		"derived capability sensor.battery_level by charge lasting 90 days since 31.13.2026;":     "no date",
		"derived capability sensor.battery_level by charge lasting 90 days from 31.10.2026;":      `expected "since"`,
	} {
		_, ok, warnings := parseChargeEstimateLine(line)
		if ok || len(warnings) == 0 || !strings.Contains(warnings[0], want) {
			t.Errorf("%q: ok %v, warnings %v; want %q", line, ok, warnings, want)
		}
	}
}

func TestChargeLevelExpression(t *testing.T) {
	got := chargeLevelExpression("input_datetime.x_batteries_replaced", 90, "2026-10-31")
	want := "[100, [0, (100 * (90 - (now().date() - strptime((states('input_datetime.x_batteries_replaced') if has_value('input_datetime.x_batteries_replaced') else '2026-10-31'), '%Y-%m-%d').date()).days) / 90) | round(0) | int] | max] | min"
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}
