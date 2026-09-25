package main

import (
	"strings"
	"testing"
)

// TestNamingSpacePathInjectsDeviceLeafPathOnlyForExplicitSphereSpecs is a focused unit test for
// PROJECT.md item 5's "device declaration acts like a space" sugar (namingSpacePath/
// specHasExplicitSpherePath, Conceptual_DeviceEntities.go): an explicit-sphere spec
// ("sensor.physical:co2") gets the device's own leaf path appended as one more segment, while a
// bare spec ("sensor.status") -- no ':' at all, resolved via the domain's default sphere -- is left
// untouched, since it carries no location semantics the DSL author asked for in the first place.
func TestNamingSpacePathInjectsDeviceLeafPathOnlyForExplicitSphereSpecs(t *testing.T) {
	base := []string{"social:shower_room"}

	got := namingSpacePath("sensor.physical:co2", base, "netatmo")
	want := []string{"social:shower_room", "netatmo"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("explicit-sphere spec: namingSpacePath = %v, want %v", got, want)
	}

	if got := namingSpacePath("sensor.status", base, "netatmo"); strings.Join(got, "|") != strings.Join(base, "|") {
		t.Errorf("bare spec: namingSpacePath = %v, want it unchanged (%v)", got, base)
	}

	if got := namingSpacePath("sensor.physical:co2", base, ""); strings.Join(got, "|") != strings.Join(base, "|") {
		t.Errorf("empty deviceNamePath: namingSpacePath = %v, want it unchanged (%v)", got, base)
	}

	// namingSpacePath must never mutate base's own backing array -- base is reused across the
	// three calls above precisely to catch that.
	if strings.Join(base, "|") != "social:shower_room" {
		t.Errorf("base was mutated: %v", base)
	}
}

// TestNamingSpacePathOmitsDeviceLeafPathWhenItEqualsTheEntitysOwnDomain is a regression test for
// a real gap found live 2026-09-10 (Vienna's vacuum): when the device's own leaf path IS the
// entity's own domain -- "device sphere:vacuum with: entity vacuum.social: from ...; end;" -- the
// leaf shouldn't be injected at all, since it would just repeat the domain a second time in the
// entity's own path, purely redundant with the domain prefix ("vacuum.social_..._vacuum" ->
// "vacuum.social_..."). A device named after something OTHER than its own domain (e.g.
// "picture_frame" for a "switch") is unaffected -- that name carries real information the domain
// prefix alone doesn't.
func TestNamingSpacePathOmitsDeviceLeafPathWhenItEqualsTheEntitysOwnDomain(t *testing.T) {
	base := []string{"social:living_room"}

	if got := namingSpacePath("vacuum.social:", base, "vacuum"); strings.Join(got, "|") != strings.Join(base, "|") {
		t.Errorf("device leaf == entity domain: namingSpacePath = %v, want it unchanged (%v)", got, base)
	}

	// Sanity check the unaffected case still works: a device leaf that ISN'T the entity's own
	// domain still gets injected normally.
	got := namingSpacePath("switch.social:", base, "picture_frame")
	want := []string{"social:living_room", "picture_frame"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("device leaf != entity domain: namingSpacePath = %v, want %v", got, want)
	}
}

// TestNamingSpacePathOmitsDeviceLeafPathWhenItEqualsTheLastSegmentOfACompoundLeaf generalises the
// domain-suppression rule above to a compound deviceNamePath (2026-09-15, Vienna's hallway ceiling
// light): "device infrastructural:main/light from ... with: entity light.social:main from ...;
// end;" suppresses injection for the light entity itself (domain "light" == deviceNamePath's own
// last "/" segment "light"), so it resolves to plain ".../main" rather than the redundant
// ".../main/light" -- while a DIFFERENT domain (e.g. the auto-implied "node" capability, domain
// "binary_sensor") is unaffected and still gets the full "main/light" injected, landing on
// ".../main/light/node". This is what lets a device leaf carry a disambiguating discriminator
// ("light") that only shows up where something without its own domain-based redundancy actually
// needs the context, without forcing every capability of the device to repeat it.
func TestNamingSpacePathOmitsDeviceLeafPathWhenItEqualsTheLastSegmentOfACompoundLeaf(t *testing.T) {
	base := []string{"social:apartment/hallway"}

	if got := namingSpacePath("light.social:main", base, "main/light"); strings.Join(got, "|") != strings.Join(base, "|") {
		t.Errorf("domain == last segment of compound leaf: namingSpacePath = %v, want it unchanged (%v)", got, base)
	}

	got := namingSpacePath("binary_sensor.infrastructural:node", base, "main/light")
	want := []string{"social:apartment/hallway", "main/light"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("domain != last segment of compound leaf: namingSpacePath = %v, want %v", got, want)
	}
}

// TestNamingSpacePathKeepsClippedDeviceLeafWhenSpecHasNoLeafOfItsOwn is a regression test for a
// real bug found live 2026-09-24 (Junglinster's "backups/switch" discovery device, positioned as
// "infrastructural:backups/switch" specifically to avoid colliding with the sibling "backups"
// hosts-kind device's own bare "infrastructural:backups" -- same shape as imac/xanadu's own
// ".../switch" compound leaves). Its "entity switch.social: from core;" line (empty leaf -- the
// device's own compound name is the ONLY source of identity, unlike "switch.social:imac" or
// "light.social:main" where the spec's own explicit leaf already supplies one) resolved to the
// bare, collapsed "switch.social_house_storage_room" -- confirmed live, colliding with (and
// silently favouring, whichever happened to register last) any other unqualified switch in that
// space, instead of the intended "switch.social_house_storage_room_backups". The domain-tail-match
// suppression (originally added for the "vacuum.social:"/"light.social:main" cases, where full
// suppression is correct) was firing unconditionally, discarding "backups" along with the
// redundant "switch" tail instead of keeping it. Fixed: suppression only discards the WHOLE
// deviceNamePath when spec's own leaf is non-empty (that other identity source exists) or nothing
// would remain after clipping (deviceNamePath IS the domain outright, the original "vacuum" case,
// unaffected by this fix) -- otherwise the CLIPPED deviceNamePath (matching tail segment removed,
// not the whole thing) is folded in, so a bare "switch.social:" still gets its device's own real
// name, just without the domain-redundant tail.
func TestNamingSpacePathKeepsClippedDeviceLeafWhenSpecHasNoLeafOfItsOwn(t *testing.T) {
	base := []string{"social:house/storage_room"}

	// Empty spec leaf, compound deviceNamePath whose tail matches the domain: the clipped
	// prefix ("backups") must still be folded in -- this is the actual bug/fix under test.
	got := namingSpacePath("switch.social:", base, "backups/switch")
	want := []string{"social:house/storage_room", "backups"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("empty spec leaf, compound deviceNamePath: namingSpacePath(%q, _, %q) = %v, want %v", "switch.social:", "backups/switch", got, want)
	}

	// Empty spec leaf, deviceNamePath IS the domain outright (no slash at all, e.g. a device
	// literally named "vacuum"): nothing remains after clipping, so full suppression (the
	// pre-existing, already-correct "vacuum.social:" precedent) must be unchanged.
	if got := namingSpacePath("vacuum.social:", base, "vacuum"); strings.Join(got, "|") != strings.Join(base, "|") {
		t.Errorf("empty spec leaf, deviceNamePath == domain: namingSpacePath = %v, want it unchanged (%v)", got, base)
	}

	// Non-empty spec leaf (the pre-existing imac/main precedent, TestNamingSpacePathOmits...
	// above) must still fully suppress -- confirming this fix didn't regress the case it's
	// deliberately NOT changing.
	if got := namingSpacePath("switch.social:imac", base, "imac/switch"); strings.Join(got, "|") != strings.Join(base, "|") {
		t.Errorf("non-empty spec leaf, compound deviceNamePath: namingSpacePath = %v, want it unchanged (%v)", got, base)
	}
}

// TestNamingSpacePathSkipsDeviceLeafForDoubleColonForm is a regression test for the real design
// gap the user surfaced live 2026-09-19 (Vienna's wind_speed/wind_direction): "device declaration
// acts like a space" (namingSpacePath) always folds the device's own leaf into an explicit-sphere
// spec's name, with no way to say "attach to the enclosing space, but not this particular device's
// own internal/infrastructural leaf" -- exactly the case for a real physical measurement that
// should read as a plain space-level entity (sensor.social_terrace_wind_speed), not one carrying
// an internal device name the DSL author never intended to expose
// (sensor.social_terrace_netatmo_windmeter_wind_speed). Fixed via a new "sphere::path" (double
// colon, empty leaf-override segment) form: hasDeviceLeafOverride detects it and namingSpacePath
// returns spacePath unextended, exactly as if deviceNamePath had been "" -- normalizeEntityFullName
// needs no change of its own, since it already resolves a lone leftover colon in the path portion
// to an empty (trimmed) leading segment. See project_device_leaf_naming_conceptual_mismatch_gap.md;
// the sibling "sphere:leaf:path" explicit-override form (a DIFFERENT replacement leaf, not just
// "none") that note also describes was implemented 2026-09-21, see
// TestNamingSpacePathReplacesDeviceLeafForExplicitOverrideForm below.
func TestNamingSpacePathSkipsDeviceLeafForDoubleColonForm(t *testing.T) {
	base := []string{"social:terrace"}

	if got := namingSpacePath("sensor.social::wind_speed", base, "netatmo_windmeter"); strings.Join(got, "|") != strings.Join(base, "|") {
		t.Errorf("double-colon form: namingSpacePath = %v, want it unchanged (%v)", got, base)
	}

	if full := normalizeEntityFullName("sensor.social::wind_speed", namingSpacePath("sensor.social::wind_speed", base, "netatmo_windmeter")); full != "sensor.social/terrace/wind_speed" {
		t.Errorf("double-colon form full name = %q, want %q", full, "sensor.social/terrace/wind_speed")
	}

	// The ordinary single-colon form is unaffected: the device leaf still gets folded in.
	got := namingSpacePath("sensor.social:wind_speed", base, "netatmo_windmeter")
	want := []string{"social:terrace", "netatmo_windmeter"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("single-colon form: namingSpacePath = %v, want %v", got, want)
	}
}

// TestNamingSpacePathReplacesDeviceLeafForExplicitOverrideForm is the regression test for the
// 2026-09-21 case (environment.weather's own "pressure" capability, a ROOT-level device with no
// enclosing space of its own): the "sphere:leaf:path" explicit-replacement-leaf form -- a DIFFERENT
// leaf than "none" (TestNamingSpacePathSkipsDeviceLeafForDoubleColonForm's own double-colon case)
// -- lets the DSL author name an alternate location word in place of the device's own internal
// name. namingSpacePath itself doesn't need to know the replacement value at all: it only needs to
// suppress deviceNamePath injection (exactly as the double-colon form already does), and
// normalizeEntityFullName's own pre-existing "every remaining colon becomes a '/' separator"
// handling produces "terrace/pressure" from spec's own remainder unaided.
func TestNamingSpacePathReplacesDeviceLeafForExplicitOverrideForm(t *testing.T) {
	var empty []string

	if got := namingSpacePath("sensor.social:terrace:pressure", empty, "weather"); strings.Join(got, "|") != strings.Join(empty, "|") {
		t.Errorf("explicit-leaf-override form: namingSpacePath = %v, want it unchanged (%v) -- deviceNamePath must be suppressed", got, empty)
	}

	if full := normalizeEntityFullName("sensor.social:terrace:pressure", namingSpacePath("sensor.social:terrace:pressure", empty, "weather")); full != "sensor.social/terrace/pressure" {
		t.Errorf("explicit-leaf-override form full name = %q, want %q", full, "sensor.social/terrace/pressure")
	}
}

// TestDisplaySuffixForHassBridgeCapability is a regression test for a real gap found live
// 2026-09-10 (Vienna's vacuum): the coordinator's discovery "name" field used to always append
// the raw Physical.def capability key (e.g. "roomba"), leaking a physical-layer implementation
// detail into the conceptual layer's own friendly name even when the DSL author explicitly said
// "nothing more specific than the device itself" (an empty trailing path, "vacuum.social: from
// roomba;"). The ordinary non-empty-path case (DisplaySuffix == the capability key) is already
// exercised by this codebase's many other hassbridge positioning tests; this test covers only the
// two new empty-path branches: the device's own leaf already equals the domain (suffix omitted
// entirely -- the device's own display name already conveys it), and where it doesn't (domain
// used instead).
func TestDisplaySuffixForHassBridgeCapability(t *testing.T) {
	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.roomba": {
			DeviceID:  "hass.roomba",
			Instances: []string{"protocols-server-2"},
			Capabilities: map[string]THassBridgeCapability{
				"roomba": {Domain: "vacuum", Sources: map[string]string{"protocols-server-2": "vacuum.roomba"}},
			},
		},
		"hass.gadget": {
			DeviceID:  "hass.gadget",
			Instances: []string{"protocols-server-2"},
			Capabilities: map[string]THassBridgeCapability{
				"main": {Domain: "vacuum", Sources: map[string]string{"protocols-server-2": "vacuum.gadget"}},
			},
		},
	}

	const dsl = `space social:living_room with:
  device hass.roomba as vacuum with:
    entity vacuum.social: from roomba;
  end;
  device hass.gadget as gadget with:
    entity vacuum.social: from main;
  end;
end;`

	result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", &TMacroExpansionContext{}, &strings.Builder{}, nil, nil, hassBridgeDevicesByID, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Empty path, device leaf ("vacuum") == domain ("vacuum") -> suffix omitted entirely.
	roombaLink := result.Administration.DeviceConceptualLinks["hass.roomba"]
	if got := roombaLink.AttributeEntityIDs["roomba"].DisplaySuffix; got != "" {
		t.Errorf("device leaf == domain: DisplaySuffix = %q, want \"\" (omitted)", got)
	}

	// Empty path, device leaf ("gadget") != domain ("vacuum") -> domain used as the suffix.
	gadgetLink := result.Administration.DeviceConceptualLinks["hass.gadget"]
	if got := gadgetLink.AttributeEntityIDs["main"].DisplaySuffix; got != "vacuum" {
		t.Errorf("device leaf != domain: DisplaySuffix = %q, want \"vacuum\"", got)
	}
}

// TestDeviceWithBlockOmitsRedundantDeviceLeafPath is PROJECT.md item 5's own worked example (a
// Netatmo device positioned under a room): capability specs inside the device's own "with:" block
// that omit its leaf path ("netatmo/") must still resolve to the SAME entity ids the old,
// fully-spelled-out "sensor.physical:netatmo/co2" style produced (see this repo's real
// Junglinster/Vienna Spaces.def, migrated off that old style alongside this feature) -- asserted
// directly against the expected ids here, since writing BOTH styles in one DSL sample would now
// double the leaf path for the old one (deliberately: the whole point is that repeating it is no
// longer necessary, so the old style must migrate rather than coexist as an equally-valid
// alternative going forward).
func TestDeviceWithBlockOmitsRedundantDeviceLeafPath(t *testing.T) {
	const dsl = `space social:shower_room with:
  device hass.living_room_shower_room as netatmo with:
    entity sensor.physical:co2                  from sensor.co2;
    entity sensor.physical:humidity             from sensor.humidity;
    entity sensor.physical:temperature          from sensor.temperature;
    entity sensor.infrastructural:battery_level from sensor.battery_level;
  end;
end;`

	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.living_room_shower_room": {DeviceID: "hass.living_room_shower_room", Instances: []string{"protocols-server-2"}, Capabilities: map[string]THassBridgeCapability{
			"node":          {Domain: "binary_sensor", Sources: map[string]string{"protocols-server-2": "sensor.netatmo is available"}},
			"co2":           {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.netatmo_co2"}},
			"humidity":      {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.netatmo_humidity"}},
			"temperature":   {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.netatmo_temperature"}},
			"battery_level": {Domain: "sensor", Sources: map[string]string{"protocols-server-2": "sensor.netatmo_battery_level"}},
		}},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, nil, nil, hassBridgeDevicesByID, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	link := result.Administration.DeviceConceptualLinks["hass.living_room_shower_room"]
	want := map[string]string{
		"co2":           "sensor.physical_shower_room_netatmo_co2",
		"humidity":      "sensor.physical_shower_room_netatmo_humidity",
		"temperature":   "sensor.physical_shower_room_netatmo_temperature",
		"battery_level": "sensor.infrastructural_shower_room_netatmo_battery_level",
	}
	for attr, wantID := range want {
		got, ok := link.AttributeEntityIDs[attr]
		if !ok || got.EntityID != wantID {
			t.Errorf("AttributeEntityIDs[%q] = %+v, ok=%v, want EntityID %s", attr, got, ok, wantID)
		}
	}
}

// TestDeferredCapabilityLinkResolvesAgainstItsOwnDeclaredSpace is the regression test for
// PROJECT.md item 6's real bug, found live 2026-09-07: Vienna's "entity
// switch.social:picture_frame from host.frame slideshow;", positioned inside a nested space
// but BEFORE host.frame's own "device ... from host.frame;" positioning appeared later in the same
// file, was deferred (registerDeviceCapabilityEntityLink's own retry-after-full-parse mechanism,
// parser.go's pendingCapabilityLinks) and then, on retry, resolved against whatever
// administration.SpacePath happened to be AFTER the whole file was read (back at "root", both
// spaces closed) instead of the space the reference line actually sat in -- producing
// "switch.social_picture_frame" instead of the wanted
// "switch.social_apartment_living_room_picture_frame". Fixed by snapshotting
// administration.SpacePath into pendingCapabilityLink at defer time and restoring it around each
// retry call (parser.go).
func TestDeferredCapabilityLinkResolvesAgainstItsOwnDeclaredSpace(t *testing.T) {
	const dsl = `space social:apartment with:
  space social:living_room with:
    entity switch.social:picture_frame from hass.frame slideshow;
  end;
end;
device hass.frame as frame with:
end;`

	hassBridgeDevicesByID := map[string]THassBridgeDevice{
		"hass.frame": {DeviceID: "hass.frame", Instances: []string{"protocols-server-2"}, Capabilities: map[string]THassBridgeCapability{
			"node":      {Domain: "binary_sensor", Sources: map[string]string{"protocols-server-2": "sensor.frame is available"}},
			"slideshow": {Domain: "switch", Sources: map[string]string{"protocols-server-2": "switch.frame_slideshow"}},
		}},
	}

	var report strings.Builder
	result, err := ParseEntitiesAndFillAdministration(strings.Split(dsl, "\n"), nil, "test.def", &TMacroExpansionContext{}, &report, nil, nil, hassBridgeDevicesByID, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	link := result.Administration.DeviceConceptualLinks["hass.frame"]
	slideshow, ok := link.AttributeEntityIDs["slideshow"]
	if !ok || slideshow.EntityID != "switch.social_apartment_living_room_picture_frame" {
		t.Errorf("slideshow (deferred capability link) = %+v, ok=%v, want EntityID switch.social_apartment_living_room_picture_frame", slideshow, ok)
	}
}
