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

func TestLookupDefaultSphere(t *testing.T) {
	testCases := []struct {
		object            string
		expectedSphere    string
		expectSphereFound bool
	}{
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
