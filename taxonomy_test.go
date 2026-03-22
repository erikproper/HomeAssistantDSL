/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Taxonomy/Test
 *
 * This component verifies canonical taxonomy mappings for device class and aggregation domain metadata.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 20.03.2026
 *
 */

package main

import "testing"

func TestLookupDeviceClass(t *testing.T) {
	testCases := []struct {
		object            string
		expectedClass     string
		expectClassExists bool
	}{
		{object: "battery_alert", expectedClass: "battery", expectClassExists: true},
		{object: "battery_level", expectedClass: "battery", expectClassExists: true},
		{object: "noise", expectedClass: "signal_strength", expectClassExists: true},
		{object: "unknown", expectedClass: "", expectClassExists: false},
	}

	for _, testCase := range testCases {
		actualClass, classExists := lookupDeviceClass(testCase.object)
		if classExists != testCase.expectClassExists {
			t.Fatalf("lookupDeviceClass(%q) existence mismatch: got %v, expected %v", testCase.object, classExists, testCase.expectClassExists)
		}
		if actualClass != testCase.expectedClass {
			t.Fatalf("lookupDeviceClass(%q) value mismatch: got %q, expected %q", testCase.object, actualClass, testCase.expectedClass)
		}
	}
}

func TestLookupAggregatedDomain(t *testing.T) {
	testCases := []struct {
		object             string
		expectedDomain     string
		expectDomainExists bool
	}{
		{object: "light", expectedDomain: "light", expectDomainExists: true},
		{object: "media_player", expectedDomain: "switch", expectDomainExists: true},
		{object: "temperature", expectedDomain: "sensor", expectDomainExists: true},
		{object: "unknown", expectedDomain: "", expectDomainExists: false},
	}

	for _, testCase := range testCases {
		actualDomain, domainExists := lookupAggregatedDomain(testCase.object)
		if domainExists != testCase.expectDomainExists {
			t.Fatalf("lookupAggregatedDomain(%q) existence mismatch: got %v, expected %v", testCase.object, domainExists, testCase.expectDomainExists)
		}
		if actualDomain != testCase.expectedDomain {
			t.Fatalf("lookupAggregatedDomain(%q) value mismatch: got %q, expected %q", testCase.object, actualDomain, testCase.expectedDomain)
		}
	}
}

func TestLookupIcon(t *testing.T) {
	ResetGlobalVariables()
	t.Cleanup(ResetGlobalVariables)

	testCases := []struct {
		object          string
		expectedIcon    string
		expectIconFound bool
	}{
		{object: "motion", expectedIcon: "mdi:motion-sensor", expectIconFound: true},
		{object: "water", expectedIcon: "mdi:water-off", expectIconFound: true},
		{object: "unknown", expectedIcon: "", expectIconFound: false},
	}

	for _, testCase := range testCases {
		actualIcon, iconFound := lookupIcon(testCase.object)
		if iconFound != testCase.expectIconFound {
			t.Fatalf("lookupIcon(%q) existence mismatch: got %v, expected %v", testCase.object, iconFound, testCase.expectIconFound)
		}
		if actualIcon != testCase.expectedIcon {
			t.Fatalf("lookupIcon(%q) value mismatch: got %q, expected %q", testCase.object, actualIcon, testCase.expectedIcon)
		}
	}

	GlobalVariables["PressureIcon"] = "mdi:test-tube"
	actualIcon, iconFound := lookupIcon("pressure")
	if !iconFound {
		t.Fatalf("expected override-backed icon lookup for pressure")
	}
	if actualIcon != "mdi:test-tube" {
		t.Fatalf("lookupIcon(%q) override mismatch: got %q, expected %q", "pressure", actualIcon, "mdi:test-tube")
	}
}

func TestLookupDefaultSphere(t *testing.T) {
	testCases := []struct {
		object            string
		expectedSphere    string
		expectSphereFound bool
	}{
		{object: "node_alert", expectedSphere: "infrastructural", expectSphereFound: true},
		{object: "door", expectedSphere: "social", expectSphereFound: true},
		{object: "co2", expectedSphere: "", expectSphereFound: false},
	}

	for _, testCase := range testCases {
		actualSphere, sphereFound := lookupDefaultSphere(testCase.object)
		if sphereFound != testCase.expectSphereFound {
			t.Fatalf("lookupDefaultSphere(%q) existence mismatch: got %v, expected %v", testCase.object, sphereFound, testCase.expectSphereFound)
		}
		if actualSphere != testCase.expectedSphere {
			t.Fatalf("lookupDefaultSphere(%q) value mismatch: got %q, expected %q", testCase.object, actualSphere, testCase.expectedSphere)
		}
	}
}
