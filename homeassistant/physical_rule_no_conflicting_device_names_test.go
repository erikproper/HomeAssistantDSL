package main

import (
	"strings"
	"testing"
)

// TestValidateNoConflictingDeviceNamesFlagsCrossKindDuplicate is a regression test for a real
// incident found live 2026-09-10: the same DeviceID ("host.frame") declared under two different
// integration kinds went undetected until the second declaration's own capabilities silently went
// missing.
func TestValidateNoConflictingDeviceNamesFlagsCrossKindDuplicate(t *testing.T) {
	hostDevicesByID := map[string]THostDevice{"host.oops": {}}
	importedDevicesByID := map[string]TImportedDevice{"host.oops": {}}

	warnings := validateNoConflictingDeviceNames(hostDevicesByID, nil, nil, importedDevicesByID, nil, nil, nil)
	if len(warnings) != 1 || !strings.Contains(warnings[0], `"host.oops"`) {
		t.Fatalf("warnings = %v, want exactly one flagging host.oops", warnings)
	}
}

// TestValidateNoConflictingDeviceNamesExemptsHostsCommandlineSharing confirms the one deliberate
// exception (a commandline device sharing identity with an already-positioned hosts device of the
// same id, e.g. host.frame's own picture-frame daemon) is never flagged.
func TestValidateNoConflictingDeviceNamesExemptsHostsCommandlineSharing(t *testing.T) {
	hostDevicesByID := map[string]THostDevice{"host.frame": {}}
	commandlineDevicesByID := map[string]TCommandlineDevice{"host.frame": {}}

	warnings := validateNoConflictingDeviceNames(hostDevicesByID, nil, nil, nil, commandlineDevicesByID, nil, nil)
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none -- hosts+commandline sharing the same id is deliberate", warnings)
	}
}

// TestValidateNoConflictingDeviceNamesFlagsHostsCommandlinePlusThirdKind confirms the exemption is
// exact -- a third kind ALSO declaring the same id is still flagged, even though hosts+commandline
// alone would not be.
func TestValidateNoConflictingDeviceNamesFlagsHostsCommandlinePlusThirdKind(t *testing.T) {
	hostDevicesByID := map[string]THostDevice{"host.frame": {}}
	commandlineDevicesByID := map[string]TCommandlineDevice{"host.frame": {}}
	importedDevicesByID := map[string]TImportedDevice{"host.frame": {}}

	warnings := validateNoConflictingDeviceNames(hostDevicesByID, nil, nil, importedDevicesByID, commandlineDevicesByID, nil, nil)
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly one -- a third kind breaks the hosts+commandline exemption", warnings)
	}
}

// TestValidateNoConflictingDeviceNamesNoFalsePositive confirms distinct device ids across every
// kind never trigger a warning.
func TestValidateNoConflictingDeviceNamesNoFalsePositive(t *testing.T) {
	hostDevicesByID := map[string]THostDevice{"host.a": {}}
	discoveryGatewaysByID := map[string]TDiscoveryGatewayDevice{"discovery.b": {}}
	hassBridgeDevicesByID := map[string]THassBridgeDevice{"hass.c": {}}
	importedDevicesByID := map[string]TImportedDevice{"import.d": {}}
	commandlineDevicesByID := map[string]TCommandlineDevice{"appliance.e": {}}

	warnings := validateNoConflictingDeviceNames(hostDevicesByID, discoveryGatewaysByID, hassBridgeDevicesByID, importedDevicesByID, commandlineDevicesByID, nil, nil)
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}
}

// TestCollectHostsDevicesByIDWarnsOnDuplicate is a focused test for the within-kind duplicate
// warning added alongside the cross-kind check above (the collector itself used to silently drop
// a duplicate with no feedback at all).
func TestCollectHostsDevicesByIDWarnsOnDuplicate(t *testing.T) {
	definitionDir := writePhysicalDef(t, `physical layer with:
  integration hosts with:
    device host.oops oops cpu;
    device host.oops oops cpu;
  end;
end;
`)
	_, warnings := collectHostsDevicesByID(definitionDir)
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "host.oops") && strings.Contains(w, "more than once") {
			found = true
		}
	}
	if !found {
		t.Errorf("warnings = %v, want one flagging host.oops declared more than once", warnings)
	}
}
