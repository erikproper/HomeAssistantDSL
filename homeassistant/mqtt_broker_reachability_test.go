package main

import (
	"errors"
	"testing"
)

// TestBrokerKnownUnreachableStartsFalse confirms a broker never marked unreachable reports as such.
// Uses a unique Server value per test to avoid cross-test contamination of the shared package-level
// cache (tests in this file run in the same process/package).
func TestBrokerKnownUnreachableStartsFalse(t *testing.T) {
	secrets := TMQTTBrokerSecrets{Server: "test-broker-never-marked.example", Port: "1883"}
	if _, known := brokerKnownUnreachable(secrets); known {
		t.Errorf("expected a never-marked broker to report unknown")
	}
}

// TestMarkBrokerUnreachableIsRemembered is the core regression coverage for the real gap found live
// 2026-09-20: a single unreachable broker made ./generate pay a full connection timeout separately
// for each of ~74 discovery gateways -- once marked, later callers for the SAME broker must see the
// cached failure instead of needing their own connection attempt.
func TestMarkBrokerUnreachableIsRemembered(t *testing.T) {
	secrets := TMQTTBrokerSecrets{Server: "test-broker-marked.example", Port: "1883"}
	wantErr := errors.New("connection refused")
	markBrokerUnreachable(secrets, wantErr)

	err, known := brokerKnownUnreachable(secrets)
	if !known {
		t.Fatalf("expected the broker to be known unreachable after marking")
	}
	if !errors.Is(err, wantErr) && err.Error() != wantErr.Error() {
		t.Errorf("brokerKnownUnreachable error = %v, want %v", err, wantErr)
	}
}

// TestBrokerKeyDistinguishesDifferentBrokers confirms marking one broker unreachable doesn't affect
// a genuinely different one (different server, port, login, or TLS setting).
func TestBrokerKeyDistinguishesDifferentBrokers(t *testing.T) {
	marked := TMQTTBrokerSecrets{Server: "test-broker-a.example", Port: "1883", Login: "alice", TLS: false}
	other := TMQTTBrokerSecrets{Server: "test-broker-a.example", Port: "1883", Login: "bob", TLS: false}

	markBrokerUnreachable(marked, errors.New("down"))

	if _, known := brokerKnownUnreachable(other); known {
		t.Errorf("expected a broker with a different login to be unaffected by marking a different one")
	}
	if _, known := brokerKnownUnreachable(marked); !known {
		t.Errorf("expected the originally-marked broker to still be known unreachable")
	}
}
