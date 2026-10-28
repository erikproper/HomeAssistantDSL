package main

import (
	"testing"
	"time"
)

// TestInstanceLiveness: a ping at start; no pong -> offline; the coordinator's own ping is no sign
// of life; any message from the instance (here the pong) -> online.
func TestInstanceLiveness(t *testing.T) {
	client := &fakeClient{}
	l := &TInstanceLiveness{client: client, instance: "ha2mqtt", quietTime: time.Hour, pongTime: 20 * time.Millisecond}
	if err := l.start(); err != nil {
		t.Fatal(err)
	}
	published := func() map[string]string {
		got := map[string]string{}
		for _, p := range client.publishedSnapshot() {
			got[p.topic] = string(p.payload)
		}
		return got
	}
	if published()[instancePingTopic("ha2mqtt")] != "ping" {
		t.Fatalf("no ping at start: %v", published())
	}
	time.Sleep(60 * time.Millisecond)
	if published()[instanceStatusTopic("ha2mqtt")] != instanceOffline {
		t.Fatalf("no pong: want offline, got %v", published())
	}
	l.observe(instancePingTopic("ha2mqtt"), "ping", false)
	if published()[instanceStatusTopic("ha2mqtt")] != instanceOffline {
		t.Errorf("the coordinator's own ping counted as a sign of life")
	}
	l.observe(instancePingTopic("ha2mqtt"), "pong", false)
	if published()[instanceStatusTopic("ha2mqtt")] != instanceOnline {
		t.Errorf("pong: want online, got %v", published())
	}
}
