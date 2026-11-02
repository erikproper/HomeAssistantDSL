package main

import (
	"reflect"
	"testing"
)

// TestSensorSubdomainMembers: a space's mean is over its own sensors and its direct sub-spaces'
// aggregates, so a room with three sensors weighs as much as one with one; a space between them
// (upward_hallway) takes the place of its own sub-spaces, and a no_collect or infrastructural
// sensor stays out (PROJECT.md 28).
func TestSensorSubdomainMembers(t *testing.T) {
	admin := newAdministrationState()
	admin.SpaceOrder = []string{"social/house", "social/house/office", "social/house/kitchen", "social/house/upward_hallway", "social/house/upward_hallway/first_floor", "social/terrace"}
	sensor := func(sphere, path string, noCollect bool) TEntityRecord {
		return TEntityRecord{Name: "sensor." + sphere + ":" + path, Identity: TEntityIdentity{Domain: "sensor", Sphere: sphere, Path: path}, NoCollect: noCollect}
	}
	admin.EntityRecordsBySpace = map[string][]TEntityRecord{
		"social/house/office":                     {sensor("social", "house/office/desk/temperature", false), sensor("social", "house/office/door/temperature", false), sensor("social", "house/office/plug/temperature", true)},
		"social/house/kitchen":                    {sensor("social", "house/kitchen/table/temperature", false), sensor("infrastructural", "house/kitchen/server/temperature", false)},
		"social/house/upward_hallway/first_floor": {sensor("social", "house/upward_hallway/first_floor/radiator/temperature", false)},
		"social/terrace":                          {sensor("social", "terrace/railing/temperature", false)},
	}
	cases := map[string][]string{
		"social/house/office":         {"sensor.social_house_office_desk_temperature", "sensor.social_house_office_door_temperature"},
		"social/house":                {"sensor.social_house_kitchen_temperature", "sensor.social_house_office_temperature", "sensor.social_house_upward_hallway_temperature"},
		"social/house/upward_hallway": {"sensor.social_house_upward_hallway_first_floor_temperature"},
		"social":                      {"sensor.social_house_temperature", "sensor.social_terrace_temperature"},
	}
	for space, want := range cases {
		if got := sensorSubdomainMembers(space, "temperature", admin); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: members %v, want %v", space, got, want)
		}
	}
}
