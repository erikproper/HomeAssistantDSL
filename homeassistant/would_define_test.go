package main

import (
	"strings"
	"testing"
)

func TestDescribeWouldDefineForDiscoveryImpliedEntity(t *testing.T) {
	admin := newAdministrationState()
	decl := TDevicePositioningDeclaration{Spec: "infrastructural:appletv-office", DeviceID: "host.appletv-office"}
	hostDevicesByID := map[string]THostDevice{
		"host.appletv-office": {DeviceID: "host.appletv-office", HostName: "appletv-office", IntegrationType: "ping"},
	}
	if warnings := registerDevicePositioning(admin, decl, hostDevicesByID, nil, nil, "Spaces.def", 1); len(warnings) != 0 {
		t.Fatalf("unexpected warnings setting up the fixture: %v", warnings)
	}

	message, found := describeWouldDefine(admin, "binary_sensor.infrastructural_appletv_office_node")
	if !found {
		t.Fatalf("expected the discovery-implied node entity to be found")
	}
	if !containsAll(message, "yes", "discovery-implied") {
		t.Errorf("message = %q, want it to say yes/discovery-implied", message)
	}
}

func TestDescribeWouldDefineForUnknownEntity(t *testing.T) {
	admin := newAdministrationState()
	message, found := describeWouldDefine(admin, "sensor.nothing_registers_this")
	if found {
		t.Errorf("expected an unregistered entity id to not be found, got message %q", message)
	}
	if !containsAll(message, "no") {
		t.Errorf("message = %q, want it to say no", message)
	}
}

func containsAll(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
