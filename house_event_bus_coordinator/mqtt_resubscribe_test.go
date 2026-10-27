package main

import (
	"reflect"
	"sync"
	"testing"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// renewClient records what a reconnect's subscription renewal sends.
type renewClient struct {
	mqtt.Client
	mu       sync.Mutex
	filters  map[string]byte
	callback mqtt.MessageHandler
	renewals int
}

func (c *renewClient) Subscribe(string, byte, mqtt.MessageHandler) mqtt.Token { return fakeToken{} }

func (c *renewClient) SubscribeMultiple(filters map[string]byte, callback mqtt.MessageHandler) mqtt.Token {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.filters, c.callback, c.renewals = filters, callback, c.renewals+1
	return fakeToken{}
}

// TestSubscriptionRegistryRenewsOnReconnect: after a reconnect, a client is subscribed again to
// every topic it subscribed to before, with a nil handler so paho keeps the routes it has -- the
// Junglinster coordinator stayed deaf to the cloud broker for hours after a reconnect without
// this (2026-10-26). A client that never subscribed sends nothing.
func TestSubscriptionRegistryRenewsOnReconnect(t *testing.T) {
	client, other := &renewClient{}, &renewClient{}
	handler := func(mqtt.Client, mqtt.Message) {}
	subscribe(client, "hosts/#", 0, handler)
	subscribe(client, "junglinster/meta/+/+/request", 1, handler)

	subscriptionRegistry.renew(client)
	subscriptionRegistry.renew(other)

	want := map[string]byte{"hosts/#": 0, "junglinster/meta/+/+/request": 1}
	if !reflect.DeepEqual(client.filters, want) {
		t.Errorf("renewed filters = %v, want %v", client.filters, want)
	}
	if client.callback != nil {
		t.Errorf("renewal passed a handler, want nil (keep the routes paho already has)")
	}
	if other.renewals != 0 {
		t.Errorf("a client without subscriptions renewed %d time(s), want 0", other.renewals)
	}
}
