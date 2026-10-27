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
	"strings"
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

// localBrokerInstallationTopic is where every coordinator tells, retained on its own local broker,
// which installation that broker belongs to (house_event_bus_coordinator/main.go).
const localBrokerInstallationTopic = "coordinator/installation"

// verifyLocalBrokerInstallation checks, once per run, that the house's own broker -- reached by
// host name, which both houses share (protocols-server-1.fritz.box) -- is really this house's:
// generated from the other house's network, that name reaches THAT house's broker (found live
// 2026-10-26: generating Junglinster from Vienna fetched Vienna's device metadata). If its
// coordinator says another installation, the broker counts as unreachable for this run, so every
// fetch falls back to the cloud broker or its cache instead of taking another house's data. A
// broker whose coordinator doesn't tell yet passes.
func verifyLocalBrokerInstallation(ctx TPhysicalGenerationContext) {
	if !ctx.HasMQTTSecrets || ctx.Installation == "" {
		return
	}
	payload, err := fetchRetained(ctx.MQTTSecrets, localBrokerInstallationTopic, "homeassistant-generator-installation-check")
	if err != nil {
		return
	}
	if owner := strings.TrimSpace(string(payload)); owner != "" && owner != ctx.Installation {
		markBrokerUnreachable(ctx.MQTTSecrets, fmt.Errorf("%s:%s is %s's broker, not %s's (this Mac is on another house's network)", ctx.MQTTSecrets.Server, ctx.MQTTSecrets.Port, owner, ctx.Installation))
		fmt.Printf("[physical] local broker %s:%s belongs to %s, not %s -- using the cloud broker and caches instead\n", ctx.MQTTSecrets.Server, ctx.MQTTSecrets.Port, owner, ctx.Installation)
	}
}
