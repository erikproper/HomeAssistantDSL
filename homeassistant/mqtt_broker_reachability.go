/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: MQTTBrokerReachability
 *
 * A tiny per-process circuit breaker shared by every "connect as a one-shot client, fetch one
 * retained status payload, disconnect" fetch function (mqtt_discovery_existence.go,
 * mqtt_entity_existence.go, mqtt_import_existence.go, discovery_passthrough_suggestions.go).
 *
 * Real gap found live 2026-09-20: each of those functions is called once PER gateway/instance --
 * fetchDiscoveryExistenceFromBroker alone runs once for every declared discovery gateway (74 of
 * them on Vienna). When the broker is genuinely unreachable (checked from off-network against a
 * LAN-only hostname, in the incident that surfaced this), every single one of those calls
 * independently pays the FULL connection timeout for the exact same doomed TCP connect, even
 * though the very first failure already answers the question for every later one on the same
 * broker. This cache lets the second and later callers skip straight to their own cache-fallback
 * path instead of repeating an already-known-doomed connection attempt.
 *
 * Deliberately scoped to CONNECTION-level failures only (DNS/TCP/auth) -- never a later
 * subscribe-timeout or "no retained message published yet" timeout on an otherwise-successful
 * connection, since those say nothing about the broker's own reachability and are frequently
 * genuinely per-gateway (a gateway the coordinator simply hasn't gotten to yet).
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 20.09.2026
 *
 */

package main

import (
	"fmt"
	"sync"
)

var (
	brokerReachabilityMu  sync.Mutex
	brokerUnreachableErrs = map[string]error{}
)

// brokerKey identifies a broker by its own connection secrets -- two fetches with the exact same
// server/port/login/tls are the exact same broker, regardless of which gateway/instance/topic each
// one is actually fetching. Password is deliberately excluded (never needed to disambiguate a real
// broker, and keeping secrets out of a map key used only for equality is one less place a stray
// debug print could leak one).
func brokerKey(secrets TMQTTBrokerSecrets) string {
	return fmt.Sprintf("%s:%s:%s:%t", secrets.Server, secrets.Port, secrets.Login, secrets.TLS)
}

// brokerKnownUnreachable reports whether secrets' broker already failed a connection attempt
// earlier in this same process run -- callers should skip straight to their own cache fallback
// when this returns true, rather than repeat an already-known-doomed connection attempt.
func brokerKnownUnreachable(secrets TMQTTBrokerSecrets) (error, bool) {
	brokerReachabilityMu.Lock()
	defer brokerReachabilityMu.Unlock()
	err, known := brokerUnreachableErrs[brokerKey(secrets)]
	return err, known
}

// markBrokerUnreachable records that secrets' broker failed to connect this run -- call only for a
// genuine connection-level failure, see this file's own header comment for why that distinction
// matters.
func markBrokerUnreachable(secrets TMQTTBrokerSecrets, err error) {
	brokerReachabilityMu.Lock()
	defer brokerReachabilityMu.Unlock()
	brokerUnreachableErrs[brokerKey(secrets)] = err
}
