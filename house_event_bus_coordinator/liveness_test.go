package main

import "testing"

// TestLivenessKeepsTheGatewaysOwnVerdictRetained: after our Offline, the gateway's own live Online
// (not retained by it) is published retained, and so is a Last Will's Offline after our Online;
// our own message coming back changes nothing.
func TestLivenessKeepsTheGatewaysOwnVerdictRetained(t *testing.T) {
	client := &fakeClient{}
	l := newLiveness(client, TLivenessSpec{Name: "plug", Traffic: "plug/#", Verdict: "plug/online", Online: "true", Offline: "false"})
	l.status = "false"
	l.observe("plug/online", "true", false)
	l.observe("plug/online", "true", false)
	l.observe("plug/online", "false", false)
	l.observe("plug/online", "false", true)
	got := client.publishedSnapshot()
	if len(got) != 2 || got[0].topic != "plug/online" || string(got[0].payload) != "true" || string(got[1].payload) != "false" {
		t.Errorf("published %v", got)
	}
}
