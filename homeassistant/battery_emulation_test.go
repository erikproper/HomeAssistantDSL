package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseBatteryEmulationLine(t *testing.T) {
	instance, emulation, ok, warnings := parseBatteryEmulationLine("battery level emulation as unique_id thermostats_x_battery_emulation lasting 90 days since 31.10.2026 with alert at 5%;")
	if !ok || len(warnings) > 0 || instance != "" {
		t.Fatalf("not parsed: %v %v", ok, warnings)
	}
	if emulation.RootID != "thermostats_x_battery_emulation" || emulation.Days != 90 || emulation.Since != "2026-10-31" || emulation.AlertPercent != 5 {
		t.Errorf("emulation %+v", emulation)
	}
	_, emulation, ok, warnings = parseBatteryEmulationLine("battery level emulation with alert at 10%;")
	if !ok || len(warnings) > 0 || emulation.Days != batteryEmulationDefaultDays || emulation.Since != "" || emulation.AlertPercent != 10 {
		t.Errorf("defaults: %+v %v %v", emulation, ok, warnings)
	}
	instance, _, ok, warnings = parseBatteryEmulationLine("battery level emulation on ha2mqtt;")
	if !ok || len(warnings) > 0 || instance != "ha2mqtt" {
		t.Errorf("instance: %q %v %v", instance, ok, warnings)
	}
}

func TestParseBatteryEmulationLineReportsErrors(t *testing.T) {
	for line, want := range map[string]string{
		"battery level emulation lasting ninety days with alert at 5%;": "is not the number of days",
		"battery level emulation since 31.13.2026 with alert at 5%;":    "no date",
		"battery level emulation with alert at 5;":                      `expected "%"`,
		"battery level emulation with alert at 100%;":                   "no alert level",
		"battery level emulation lasting 90 days;":                      `expected "with"`,
	} {
		_, _, ok, warnings := parseBatteryEmulationLine(line)
		if ok || len(warnings) == 0 || !strings.Contains(warnings[0], want) {
			t.Errorf("%q: ok %v, warnings %v; want %q", line, ok, warnings, want)
		}
	}
	for _, line := range []string{"capability sensor.battery_level 0x1_battery_zigbee2mqtt;", "battery_level;"} {
		if isBatteryEmulationStatement(line) {
			t.Errorf("%q taken as an emulation statement", line)
		}
	}
}

func TestStripBatteryEmulationStatementsKeepsLineCount(t *testing.T) {
	content := "device thermostats.x with:\n  battery level emulation with alert at 5%;\n  capability sensor.battery_level_raw 0x1_battery_zigbee2mqtt;\nend;"
	stripped := stripBatteryEmulationStatements(content)
	if strings.Count(stripped, "\n") != strings.Count(content, "\n") || strings.Contains(stripped, "emulation") || !strings.Contains(stripped, "battery_level_raw") {
		t.Errorf("stripped:\n%s", stripped)
	}
}

func TestWriteRemoteInstanceSkeletonIncludesExtraDomains(t *testing.T) {
	dir := t.TempDir()
	if err := writeRemoteInstanceSkeleton(dir, batteryEmulationInstanceDomains...); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "configuration.yaml"))
	for _, domain := range append([]string{"automation"}, batteryEmulationInstanceDomains...) {
		if !strings.Contains(string(data), domain+": !include integrations/"+domain+".yaml") {
			t.Errorf("configuration.yaml has no %s include:\n%s", domain, data)
		}
		if _, err := os.Stat(filepath.Join(dir, "integrations", domain+".yaml")); err != nil {
			t.Errorf("no integrations/%s.yaml", domain)
		}
	}
	plain := t.TempDir()
	if err := writeRemoteInstanceSkeleton(plain); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(plain, "configuration.yaml")); strings.Contains(string(data), "template:") {
		t.Errorf("a plain instance got the emulation's includes:\n%s", data)
	}
}
