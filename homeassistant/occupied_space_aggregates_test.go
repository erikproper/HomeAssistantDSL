package main

import "testing"

// TestOccupiedSpaceAggregate: an entity positioned at the aggregate's own name blocks it -- it isn't
// generated, and it is reported with its other members; the generator's own aggregate registration
// is no positioning.
func TestOccupiedSpaceAggregate(t *testing.T) {
	admin := newAdministrationState()
	admin.EntityRecordsBySpace["social/house/office"] = []TEntityRecord{
		{Name: "binary_sensor.social/house/office/door", Provenance: "Conceptual.def:12"},
		{Name: "binary_sensor.social/house/office/left/door", Provenance: "Conceptual.def:13"},
		{Name: "binary_sensor.social/house/office/window", Provenance: "derived binary_sensor subdomain aggregate"},
	}
	spaceAggregateMembers, positionedEntityIDs = nil, nil
	defer func() { spaceAggregateMembers, positionedEntityIDs = nil, nil }()
	if recordSpaceAggregate(admin, "binary_sensor.social_house_office_door", []string{"binary_sensor.social_house_office_left_door"}) {
		t.Error("an aggregate whose name is positioned may be generated")
	}
	if !recordSpaceAggregate(admin, "binary_sensor.social_house_office_window", []string{"binary_sensor.social_house_office_a_window"}) {
		t.Error("the generator's own aggregate registration counts as positioned")
	}
	occupied := occupiedSpaceAggregates()
	others, taken := occupied["binary_sensor.social_house_office_door"]
	if !taken || len(others) != 1 || others[0] != "binary_sensor.social_house_office_left_door" {
		t.Errorf("occupied = %v", occupied)
	}
	if _, taken := occupied["binary_sensor.social_house_office_window"]; taken {
		t.Errorf("window reported as occupied: %v", occupied)
	}
}

// TestSingletonSpaceAggregate: an aggregate of one member positioned in its own space is reported;
// one whose single member sits in a deeper space, and a heating leakage evidence group, are not.
func TestSingletonSpaceAggregate(t *testing.T) {
	admin := newAdministrationState()
	admin.EntityRecordsBySpace["social/house/office"] = []TEntityRecord{
		{Name: "sensor.social/house/office/wall/temperature", Provenance: "Conceptual.def:12"},
		{Name: "binary_sensor.social/house/office/window", Provenance: "Conceptual.def:13"},
	}
	spaceAggregateMembers, positionedEntityIDs = nil, nil
	defer func() { spaceAggregateMembers, positionedEntityIDs = nil, nil }()
	recordSpaceAggregate(admin, "sensor.social_house_office_temperature", []string{"sensor.social_house_office_wall_temperature"})
	recordSpaceAggregate(admin, "sensor.social_house_temperature", []string{"sensor.social_house_office_wall_temperature"})
	recordSpaceAggregate(admin, "binary_sensor.physical_house_office_heating_leakage_evidence", []string{"binary_sensor.social_house_office_window"})
	singletons := singletonSpaceAggregates()
	if len(singletons) != 1 || singletons["sensor.social_house_office_temperature"] != "sensor.social_house_office_wall_temperature" {
		t.Errorf("singletons = %v", singletons)
	}
}
