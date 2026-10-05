package main

import "testing"

// TestDeviceDisplayNameNoTrailingSlashForEmptyPath is the regression test for a real bug found
// live 2026-09-28 (Junglinster's own "utility.somfy_kitchen", a device with no "as" leaf of its
// own): appending "/" + path unconditionally left a dangling trailing slash ("house/kitchen/")
// whenever path was empty, which a caller combining this with its own per-capability suffix (the
// hassbridge relay's own "display_suffix") then doubled up into a visible "house/kitchen//cover".
func TestDeviceDisplayNameNoTrailingSlashForEmptyPath(t *testing.T) {
	got := deviceDisplayName("social/house/kitchen", "")
	want := "house/kitchen"
	if got != want {
		t.Errorf("deviceDisplayName(%q, \"\") = %q, want %q (no trailing slash)", "social/house/kitchen", got, want)
	}
}

// TestDeviceDisplayNameWithPath confirms the ordinary (non-empty path) case is unaffected by the
// trailing-slash fix above.
func TestDeviceDisplayNameWithPath(t *testing.T) {
	got := deviceDisplayName("social/house/kitchen", "nespresso")
	want := "house/kitchen/nespresso"
	if got != want {
		t.Errorf("deviceDisplayName(%q, %q) = %q, want %q", "social/house/kitchen", "nespresso", got, want)
	}
}

// TestDeviceDisplayNameRootSpaceEmptyPath confirms the "root"/"" space shortcut also never
// produces a stray leading/trailing slash when path is empty.
func TestDeviceDisplayNameRootSpaceEmptyPath(t *testing.T) {
	if got := deviceDisplayName("root", ""); got != "" {
		t.Errorf(`deviceDisplayName("root", "") = %q, want ""`, got)
	}
}

// TestOpenSpaceAreaNameStripsSphereColonPrefix is the regression test for a real, WIDESPREAD bug
// found live 2026-09-28 (first noticed on Junglinster's own kitchen netatmo device, then confirmed
// to affect essentially every "as area" space in BOTH houses whose sphere isn't "infrastructural"
// -- "social" being the overwhelming common case): OpenSpace's own area-name computation passed
// the WHOLE "sphere:leaf" spec (e.g. "social:kitchen") into deviceSpecLeafPath, whose own
// "multi-sphere" verbatim-preserving branch (isMultiSphereDeviceNamePath) is designed for a
// DEVICE's own "as" clause, where a single non-"infrastructural" sphere token is deliberately a
// distinct shape -- reused here for a SPACE spec (which has no such special case, always exactly
// one ordinary sphere:leaf pair), it returned the leaf argument UNCHANGED, colon and all, so the
// resulting suggested_area read "social/house/social:kitchen" instead of "social/house/kitchen".
func TestOpenSpaceAreaNameStripsSphereColonPrefix(t *testing.T) {
	admin := newAdministrationState()
	admin.OpenSpace("space", "social:house", false)
	admin.OpenSpace("space", "social:kitchen", true)

	got := admin.CurrentArea()
	want := "social/house/kitchen"
	if got != want {
		t.Errorf("CurrentArea() = %q, want %q (no embedded \"social:\" colon-prefix left over)", got, want)
	}
}

// TestResolveDeviceEntityFullNameStripsDomainTailEvenWithNoDeviceLeaf is the regression test for
// the user's own correction, 2026-09-29: the domain-matching-tail strip
// (stripEntityOwnTrailingDomainSegment) must apply UNCONDITIONALLY, not only when the entity's own
// device has an "as" leaf. A prior, more conservative version of this function skipped the strip
// whenever deviceNamePath was empty, to protect a real regression
// (TestGenerateHassBridgeEntityReportingAutomationsWritesAttributesFile's own roomba vacuum case,
// updated the same day to match this new, unconditional behaviour) -- the user's own correction:
// that gating was wrong. The real, live trigger: Junglinster's own "utility.somfy_kitchen" (no "as"
// clause, likely never will have one) declaring "entity cover.social:cover;" must resolve to
// "cover.social_house_kitchen", not "..._cover".
func TestResolveDeviceEntityFullNameStripsDomainTailEvenWithNoDeviceLeaf(t *testing.T) {
	got := toHomeAssistantEntityID(resolveDeviceEntityFullName("cover.social:cover", []string{"house", "kitchen"}, ""))
	want := "cover.social_house_kitchen"
	if got != want {
		t.Errorf("resolveDeviceEntityFullName(%q, ..., \"\") entity_id = %q, want %q", "cover.social:cover", got, want)
	}
}

// TestResolveDeviceEntityFullNameKeepsGenuineMultiSegmentPathWithNoDeviceLeaf confirms the
// unconditional strip above only ever removes a TRAILING segment that exactly equals the domain --
// a genuine multi-segment path (no such trailing duplicate) is completely unaffected, with or
// without a device leaf.
func TestResolveDeviceEntityFullNameKeepsGenuineMultiSegmentPathWithNoDeviceLeaf(t *testing.T) {
	got := toHomeAssistantEntityID(resolveDeviceEntityFullName("number.social:cover/my_position", []string{"house", "kitchen"}, ""))
	want := "number.social_house_kitchen_cover_my_position"
	if got != want {
		t.Errorf("resolveDeviceEntityFullName(%q, ..., \"\") entity_id = %q, want %q", "number.social:cover/my_position", got, want)
	}
}
