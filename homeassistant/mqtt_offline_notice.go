/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: MQTTOfflineNotice
 *
 * A single, run-wide "working from cached data" notice, shared by every live-MQTT-with-cache-
 * fallback fetch this generator makes (entity existence per instance, discovery existence,
 * passthrough devices, import existence per remote installation -- mqtt_entity_existence.go,
 * mqtt_discovery_existence.go, mqtt_import_existence.go, discovery_passthrough_suggestions.go).
 * Before this existed, each of those fetches printed its own two-line blow-by-blow (a "cloud
 * broker attempt failed ... trying local" line, then a "live fetch failed ... using cached copy"
 * line, each carrying the raw Go network error) -- with up to six separate resources fetched in
 * one generate run, a genuinely offline run produced a wall of a dozen near-identical lines (real
 * complaint, 2026-09-25: "just say we will be working with an old cache ... but no further
 * messages about the lack of a connection"). Once the user already knows connectivity is down,
 * repeating that fact per resource adds nothing.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 25.09.2026
 *
 */

package main

import (
	"fmt"
	"sync"
)

var offlineCacheNoticeOnce sync.Once

// printOfflineCacheNoticeOnce prints the one-line "working from cached data" notice the first
// time any fetch in this generator run falls back to its local cache, and is a silent no-op on
// every subsequent call -- callers no longer need to carry or print the underlying fetch error
// themselves for this case (a hard failure with no cache to fall back to still returns and prints
// its own real error, same as before; this only replaces the "we're offline but have a cache"
// path).
func printOfflineCacheNoticeOnce() {
	offlineCacheNoticeOnce.Do(func() {
		fmt.Println("[physical] no live MQTT connection -- working from cached data; some entity-existence/discovery info may be stale until connectivity returns")
	})
}
