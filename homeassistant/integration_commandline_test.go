/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationCommandlineTest
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 06.09.2026
 *
 */

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectCommandlineDevicesByIDParsesAllThreeKinds(t *testing.T) {
	definitionDir := writePhysicalDef(t, `physical layer with:
  integration commandline with:
    device host.frame frame with:
      switch.slideshow: "check_slideshow" "start_slideshow" "stop_slideshow";
      sensor.uptime:    "uptime_seconds";
      button.reboot:    "reboot_frame";
    end;
  end;
end;
`)

	byID, warnings := collectCommandlineDevicesByID(definitionDir)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	device, ok := byID["host.frame"]
	if !ok {
		t.Fatalf("expected host.frame to be collected; got %v", byID)
	}
	if device.Host != "frame" {
		t.Errorf("Host = %q, want \"frame\"", device.Host)
	}

	sw, ok := device.Capabilities["slideshow"]
	if !ok || sw.Kind != "switch" || sw.StatusScript != "check_slideshow" || sw.OnScript != "start_slideshow" || sw.OffScript != "stop_slideshow" {
		t.Errorf("Capabilities[slideshow] = %+v, want switch with the three slideshow scripts", sw)
	}
	sensor, ok := device.Capabilities["uptime"]
	if !ok || sensor.Kind != "sensor" || sensor.StatusScript != "uptime_seconds" {
		t.Errorf("Capabilities[uptime] = %+v, want sensor with status_script \"uptime_seconds\"", sensor)
	}
	button, ok := device.Capabilities["reboot"]
	if !ok || button.Kind != "button" || button.PressScript != "reboot_frame" {
		t.Errorf("Capabilities[reboot] = %+v, want button with press_script \"reboot_frame\"", button)
	}
}

// TestCollectCommandlineDevicesByIDWarnsOnWrongScriptCount is the regression guard for a
// mis-authored switch/sensor/button line -- e.g. a switch declared with only one script instead
// of the required three (status/on/off) -- which must warn and be dropped, not silently produce a
// capability with two empty script fields the daemon would then invoke as empty commands.
func TestCollectCommandlineDevicesByIDWarnsOnWrongScriptCount(t *testing.T) {
	definitionDir := writePhysicalDef(t, `physical layer with:
  integration commandline with:
    device host.frame frame with:
      switch.slideshow: "check_slideshow";
    end;
  end;
end;
`)

	byID, warnings := collectCommandlineDevicesByID(definitionDir)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "wants 3 quoted script(s), got 1") {
		t.Fatalf("expected exactly one wrong-script-count warning, got: %v", warnings)
	}
	if device := byID["host.frame"]; len(device.Capabilities) != 0 {
		t.Errorf("Capabilities = %+v, want the malformed switch capability dropped, not partially recorded", device.Capabilities)
	}
}

func TestGenerateCommandlineIntegrationOutputsWritesHostAndCoordinatorFiles(t *testing.T) {
	outputRoot := t.TempDir()
	devices, warnings := parseCommandlineIntegrationBody(strings.Split(`device host.frame frame with:
  switch.slideshow: "check_slideshow" "start_slideshow" "stop_slideshow";
  button.reboot:    "reboot_frame";
end;`, "\n"))
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	ctx := TPhysicalGenerationContext{OutputRoot: outputRoot}
	if err := generateCommandlineIntegrationOutputs(strings.Split(`device host.frame frame with:
  switch.slideshow: "check_slideshow" "start_slideshow" "stop_slideshow";
  button.reboot:    "reboot_frame";
end;`, "\n"), ctx); err != nil {
		t.Fatalf("generateCommandlineIntegrationOutputs: %v", err)
	}
	_ = devices

	hostFile, err := os.ReadFile(filepath.Join(outputRoot, "commandline", "frame.yaml"))
	if err != nil {
		t.Fatalf("reading commandline/frame.yaml: %v", err)
	}
	hostContent := string(hostFile)
	for _, want := range []string{`host: "frame"`, "switches:", "slideshow:", `status_script: "check_slideshow"`, `on_script: "start_slideshow"`, `off_script: "stop_slideshow"`, "buttons:", "reboot:", `press_script: "reboot_frame"`} {
		if !strings.Contains(hostContent, want) {
			t.Errorf("commandline/frame.yaml missing %q; got:\n%s", want, hostContent)
		}
	}
	if strings.Contains(hostContent, "sensors:") {
		t.Errorf("commandline/frame.yaml should have no \"sensors:\" section (no sensor capability declared); got:\n%s", hostContent)
	}

	coordFile, err := os.ReadFile(filepath.Join(outputRoot, "coordinator", "commandline.yaml"))
	if err != nil {
		t.Fatalf("reading coordinator/commandline.yaml: %v", err)
	}
	coordContent := string(coordFile)
	for _, want := range []string{"host.frame:", `host: "frame"`, "switches:", "slideshow: {}", "buttons:", "reboot: {}"} {
		if !strings.Contains(coordContent, want) {
			t.Errorf("coordinator/commandline.yaml missing %q; got:\n%s", want, coordContent)
		}
	}
	if strings.Contains(coordContent, "check_slideshow") {
		t.Errorf("coordinator/commandline.yaml must never carry script content; got:\n%s", coordContent)
	}
}

// TestGenerateCommandlineIntegrationOutputsUsesPositionedEntityID confirms a capability positioned
// in Spaces.def (DeviceConceptualLinks populated the same way a real "entity <spec> from
// <device-id> entity <capability>;" line would) gets its resolved entity_id/name written into
// coordinator/commandline.yaml, instead of being left for the coordinator to auto-derive.
func TestGenerateCommandlineIntegrationOutputsUsesPositionedEntityID(t *testing.T) {
	outputRoot := t.TempDir()
	admin := newAdministrationState()
	admin.DeviceConceptualLinks["host.frame"] = TDeviceConceptualLink{
		AttributeEntityIDs: map[string]TDeviceAttributeLink{
			"slideshow": {
				EntityID: "switch.social_apartment_living_room_picture_frame",
				Identity: TEntityIdentity{Domain: "switch", Sphere: "social", Path: "apartment/living_room/picture_frame"},
			},
		},
	}

	ctx := TPhysicalGenerationContext{OutputRoot: outputRoot, Admin: admin}
	if err := generateCommandlineIntegrationOutputs(strings.Split(`device host.frame frame with:
  switch.slideshow: "check_slideshow" "start_slideshow" "stop_slideshow";
end;`, "\n"), ctx); err != nil {
		t.Fatalf("generateCommandlineIntegrationOutputs: %v", err)
	}

	coordFile, err := os.ReadFile(filepath.Join(outputRoot, "coordinator", "commandline.yaml"))
	if err != nil {
		t.Fatalf("reading coordinator/commandline.yaml: %v", err)
	}
	coordContent := string(coordFile)
	for _, want := range []string{
		`entity_id: "switch.social_apartment_living_room_picture_frame"`,
		`name: "social/apartment/living_room/picture_frame"`,
	} {
		if !strings.Contains(coordContent, want) {
			t.Errorf("coordinator/commandline.yaml missing %q; got:\n%s", want, coordContent)
		}
	}
}
