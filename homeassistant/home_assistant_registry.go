/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: HomeAssistantRegistry
 *
 * PROJECT.md 30 (2026-11-03): the deploy's registry repair and reload check, in Go -- until now
 * tools/ha_registry_sync.py and tools/ha_reload_errors.py, piped over SSH into the HA add-on. The
 * deploy talks to Home Assistant's websocket and REST API itself, through an SSH tunnel over the
 * instance's own endpoint ("home_assistant main: vienna via ssh ...;") to its core
 * (homeassistant:8123 inside HA OS), with a long-lived access token from the defs,
 * ${<instance>_ha_token} (e.g. ${main_ha_token} in the house's Settings.def) -- never written into
 * deploy.yaml.
 *
 * The registry repair keeps HA's entity registry in step with the generator (PROJECT.md item 48):
 * it compares the instance's registry_manifest.json (per generated entity: the unique_id HA
 * registers it under, the entity_id it should have) with the registry, then renames every entry
 * whose entity_id differs -- the same entry, so its settings and history stay -- and removes every
 * entry this generator created on an earlier deploy but no longer generates: a YAML-platform entry
 * always, an MQTT one once nothing provides it anymore. It only touches entries it can prove are
 * the generator's own, never forces a rename onto a name held by anything else, and sets the
 * sensors' display options (PROJECT.md 24). A native integration's entity (PROJECT.md 30,
 * 2026-11-04) is found by its frozen ID (platform and unique_id), or else its "as local" entity_id;
 * then its ID is written into Physical.def (writeNativeIDs). The previous manifest stays on the instance,
 * /config/.generator_registry_manifest.json, saved after a successful run. This is the only
 * registry change the system makes by itself (Erik, 2026-10-26); see Architecture.md, "Home
 * Assistant's entity identities, and the registry repair at deploy".
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 04.11.2026
 *
 */

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// haCoreAddress is Home Assistant's core as an HA OS add-on sees it.
const haCoreAddress = "homeassistant:8123"

// registryStatePath is where an instance keeps the manifest of its previous deploy.
const registryStatePath = "/config/.generator_registry_manifest.json"

// registryYAMLPlatforms are the YAML platforms the generator writes entities for; an entry of one
// of these without a config entry is YAML-defined, and only those can be the generator's own.
var registryYAMLPlatforms = map[string]bool{"template": true, "group": true, "command_line": true}

// registryMQTTKinds are the manifest's lists of MQTT entities.
var registryMQTTKinds = []string{"mqtt", "imported", "hassbridge", "pending_removal"}

// tHAConnection is a connection to one instance's API, through an SSH tunnel.
type tHAConnection struct {
	tunnel *exec.Cmd
	base   string // "http://127.0.0.1:<port>"
	token  string
	ws     *websocket.Conn
	nextID int
}

// connectHomeAssistant opens a tunnel over sshArgs to the instance's core and logs in to its
// websocket with token.
func connectHomeAssistant(sshArgs []string, token string) (*tHAConnection, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	tunnel := exec.Command("ssh", append([]string{"-N", "-o", "ExitOnForwardFailure=yes", "-L", fmt.Sprintf("127.0.0.1:%d:%s", port, haCoreAddress)}, sshArgs...)...)
	tunnel.Stderr = os.Stderr
	if err := tunnel.Start(); err != nil {
		return nil, err
	}
	c := &tHAConnection{tunnel: tunnel, base: fmt.Sprintf("http://127.0.0.1:%d", port), token: token, nextID: 1}
	for attempt := 0; ; attempt++ {
		ws, _, dialErr := websocket.DefaultDialer.Dial(fmt.Sprintf("ws://127.0.0.1:%d/api/websocket", port), nil)
		if dialErr == nil {
			c.ws = ws
			break
		}
		if attempt == 40 {
			c.Close()
			return nil, fmt.Errorf("no websocket through the tunnel: %v", dialErr)
		}
		time.Sleep(250 * time.Millisecond)
	}
	var greeting map[string]any
	if err := c.ws.ReadJSON(&greeting); err != nil || greeting["type"] != "auth_required" {
		c.Close()
		return nil, fmt.Errorf("unexpected websocket greeting: %v", err)
	}
	if err := c.ws.WriteJSON(map[string]any{"type": "auth", "access_token": token}); err != nil {
		c.Close()
		return nil, err
	}
	var auth map[string]any
	if err := c.ws.ReadJSON(&auth); err != nil || auth["type"] != "auth_ok" {
		c.Close()
		return nil, fmt.Errorf("websocket authentication failed (is the access token right?)")
	}
	return c, nil
}

// Close ends the websocket and the tunnel.
func (c *tHAConnection) Close() {
	if c.ws != nil {
		c.ws.Close()
	}
	if c.tunnel.Process != nil {
		c.tunnel.Process.Kill()
		c.tunnel.Wait()
	}
}

// call sends a websocket command and returns its result message.
func (c *tHAConnection) call(command map[string]any) (map[string]any, error) {
	id := c.nextID
	c.nextID++
	command["id"] = id
	if err := c.ws.WriteJSON(command); err != nil {
		return nil, err
	}
	for {
		var reply map[string]any
		if err := c.ws.ReadJSON(&reply); err != nil {
			return nil, err
		}
		if replyID, _ := reply["id"].(float64); int(replyID) == id && reply["type"] == "result" {
			return reply, nil
		}
	}
}

// state returns an entity's state over the REST API, and its HTTP status.
func (c *tHAConnection) state(entityID string) (map[string]any, int) {
	request, _ := http.NewRequest("GET", c.base+"/api/states/"+entityID, nil)
	request.Header.Set("Authorization", "Bearer "+c.token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, 0
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	var state map[string]any
	json.Unmarshal(body, &state)
	return state, response.StatusCode
}

// registryEntry is one entry of HA's entity registry, as config/entity_registry/list gives it.
type registryEntry struct {
	EntityID      string `json:"entity_id"`
	UniqueID      string `json:"unique_id"`
	Platform      string `json:"platform"`
	ConfigEntryID string `json:"config_entry_id"`
}

func (e registryEntry) domain() string { return entityDomain(e.EntityID) }
func (e registryEntry) isYAML() bool {
	return registryYAMLPlatforms[e.Platform] && e.ConfigEntryID == ""
}
func (e registryEntry) key() registryKey { return registryKey{e.domain(), e.UniqueID} }

// registryKey is how HA keys its registry -- (domain, unique_id): one unique_id may well appear in
// two domains (a Zigbee button's "event" and "sensor" entity).
type registryKey struct{ Domain, UniqueID string }

// registryItem is one manifest entry; Source/Seed only for a native one, Platform for an "other".
type registryItem struct {
	UniqueID  string `json:"unique_id,omitempty"`
	EntityID  string `json:"entity_id,omitempty"`
	Platform  string `json:"platform,omitempty"`
	Source    string `json:"source,omitempty"`
	Seed      string `json:"seed,omitempty"`
	Precision *int   `json:"precision,omitempty"`
	Unit      string `json:"unit,omitempty"`
}

func (i registryItem) key() registryKey { return registryKey{entityDomain(i.EntityID), i.UniqueID} }

// tRegistryManifest is registry_manifest.json: lists of items by kind ("yaml", "mqtt", "imported",
// "hassbridge", "pending_removal", "other", "native", "display"), other fields kept as they are.
type tRegistryManifest map[string]json.RawMessage

func (m tRegistryManifest) items(kind string) []registryItem {
	var items []registryItem
	if raw, ok := m[kind]; ok {
		json.Unmarshal(raw, &items)
	}
	return items
}

func entityDomain(entityID string) string {
	domain, _, _ := strings.Cut(entityID, ".")
	return domain
}

// registryRename is a planned rename.
type registryRename struct {
	Entry  registryEntry
	Target string
}

// staleMQTTKeys are the MQTT entries generated before -- or still pending removal -- but no longer.
func staleMQTTKeys(manifest, previous tRegistryManifest) map[registryKey]bool {
	current := map[registryKey]bool{}
	for _, kind := range registryMQTTKinds {
		for _, item := range manifest.items(kind) {
			current[item.key()] = true
		}
	}
	stale := map[registryKey]bool{}
	for _, kind := range registryMQTTKinds {
		for _, item := range previous.items(kind) {
			if !current[item.key()] {
				stale[item.key()] = true
			}
		}
	}
	return stale
}

// pendingRemovals are the no longer generated MQTT entries that were still live, so not removed
// this time: kept in the saved state so the next run removes them (2026-10-28: the repair runs
// before HA's reload, so a dropped YAML mqtt entity is always still live then).
func pendingRemovals(registry []registryEntry, manifest, previous tRegistryManifest, live map[string]bool) []registryItem {
	stale := staleMQTTKeys(manifest, previous)
	var pending []registryItem
	for _, entry := range registry {
		if entry.Platform == "mqtt" && stale[entry.key()] && live[entry.EntityID] {
			pending = append(pending, registryItem{UniqueID: entry.UniqueID, EntityID: entry.EntityID})
		}
	}
	sort.Slice(pending, func(i, j int) bool { return pending[i].EntityID < pending[j].EntityID })
	return pending
}

// planRegistryRepair returns the renames and removals that bring registry in step with manifest,
// and a report for everything it leaves alone.
func planRegistryRepair(registry []registryEntry, manifest, previous tRegistryManifest, live map[string]bool) (renames []registryRename, removals []registryEntry, reports []string) {
	byEntityID := map[string]registryEntry{}
	yamlByKey, mqttByKey := map[registryKey][]registryEntry{}, map[registryKey][]registryEntry{}
	for _, entry := range registry {
		byEntityID[entry.EntityID] = entry
		switch {
		case entry.isYAML():
			yamlByKey[entry.key()] = append(yamlByKey[entry.key()], entry)
		case entry.Platform == "mqtt":
			mqttByKey[entry.key()] = append(mqttByKey[entry.key()], entry)
		}
	}

	// Removals: entries generated before but no longer -- never anything else.
	currentYAML := map[registryKey]bool{}
	for _, item := range manifest.items("yaml") {
		currentYAML[item.key()] = true
	}
	var staleYAML []registryKey
	for _, item := range previous.items("yaml") {
		if !currentYAML[item.key()] {
			staleYAML = append(staleYAML, item.key())
		}
	}
	sortRegistryKeys(staleYAML)
	for _, key := range staleYAML {
		removals = append(removals, yamlByKey[key]...)
	}
	// Generated automations, scripts and input_* helpers: matched on (platform, unique_id), YAML only.
	otherPlatforms, currentOther, previousOther := map[string]bool{}, map[[2]string]bool{}, map[[2]string]bool{}
	for _, item := range manifest.items("other") {
		otherPlatforms[item.Platform], currentOther[[2]string{item.Platform, item.UniqueID}] = true, true
	}
	for _, item := range previous.items("other") {
		otherPlatforms[item.Platform], previousOther[[2]string{item.Platform, item.UniqueID}] = true, true
	}
	for _, entry := range registry {
		key := [2]string{entry.Platform, entry.UniqueID}
		if otherPlatforms[entry.Platform] && entry.ConfigEntryID == "" && previousOther[key] && !currentOther[key] {
			removals = append(removals, entry)
		}
	}
	var staleMQTT []registryKey
	for key := range staleMQTTKeys(manifest, previous) {
		staleMQTT = append(staleMQTT, key)
	}
	sortRegistryKeys(staleMQTT)
	for _, key := range staleMQTT {
		for _, entry := range mqttByKey[key] {
			if !live[entry.EntityID] {
				removals = append(removals, entry)
			}
		}
	}
	removed := map[string]bool{}
	for _, entry := range removals {
		removed[entry.EntityID] = true
	}

	for _, kind := range []string{"yaml", "mqtt", "imported", "hassbridge"} {
		index := mqttByKey
		if kind == "yaml" {
			index = yamlByKey
		}
		for _, item := range manifest.items(kind) {
			entries := index[item.key()]
			if len(entries) != 1 {
				if len(entries) > 1 {
					reports = append(reports, fmt.Sprintf("skip: unique_id %s has %d registry entries", item.UniqueID, len(entries)))
				}
				continue // not registered yet: HA registers it under the right name itself
			}
			if entries[0].EntityID != item.EntityID {
				renames = append(renames, registryRename{entries[0], item.EntityID})
			}
		}
	}

	// Native-integration entities (PROJECT.md 17j, 30): found by their frozen ID (platform and
	// unique_id), or else by their "as local" entity_id (seed), and renamed to their expected
	// entity_id. Never removed.
	nativeByID := byNativeID(registry)
	for _, item := range manifest.items("native") {
		entry, found, report := nativeEntry(item, byEntityID, nativeByID)
		if report != "" {
			reports = append(reports, report)
		}
		if found && entry.EntityID != item.EntityID {
			renames = append(renames, registryRename{entry, item.EntityID})
		}
	}

	// A rename's target must be free, freed by a removal, or held by an entry that itself moves.
	moving := map[string]bool{}
	for _, rename := range renames {
		moving[rename.Entry.EntityID] = true
	}
	var feasible []registryRename
	for _, rename := range renames {
		holder, held := byEntityID[rename.Target]
		if !held || removed[rename.Target] || moving[rename.Target] {
			feasible = append(feasible, rename)
			continue
		}
		reports = append(reports, fmt.Sprintf("skip: rename %s -> %s: target is held by %s entry %q, not the generator's own", rename.Entry.EntityID, rename.Target, holder.Platform, holder.UniqueID))
	}
	return feasible, removals, reports
}

// isNative reports whether entry is a native integration's, neither generated YAML nor MQTT.
func (e registryEntry) isNative() bool { return !e.isYAML() && e.Platform != "mqtt" }

// nativeKey is how HA identifies a native entity: its domain, platform and unique_id.
type nativeKey struct{ Domain, Platform, UniqueID string }

// byNativeID indexes registry's native entries by their nativeKey.
func byNativeID(registry []registryEntry) map[nativeKey]registryEntry {
	index := map[nativeKey]registryEntry{}
	for _, entry := range registry {
		if entry.isNative() {
			index[nativeKey{entry.domain(), entry.Platform, entry.UniqueID}] = entry
		}
	}
	return index
}

// nativeEntry returns the registry entry of a native manifest item: by its frozen ID, else by its
// "as local" entity_id (seed), else the native entry already holding its expected entity_id. The
// report says why none was found, or why the one found isn't a native integration's.
func nativeEntry(item registryItem, byEntityID map[string]registryEntry, byID map[nativeKey]registryEntry) (registryEntry, bool, string) {
	if item.Platform != "" {
		entry, found := byID[nativeKey{entityDomain(item.EntityID), item.Platform, item.UniqueID}]
		if !found {
			return entry, false, fmt.Sprintf("skip: native %s: %s %q isn't registered", item.Source, item.Platform, item.UniqueID)
		}
		return entry, true, ""
	}
	if item.Seed != "" {
		if entry, found := byEntityID[item.Seed]; found {
			if !entry.isNative() {
				return entry, false, fmt.Sprintf("skip: native %s: %s is a %s entry, not a native integration's", item.Source, item.Seed, entry.Platform)
			}
			return entry, true, ""
		}
	}
	if entry, found := byEntityID[item.EntityID]; found && entry.isNative() {
		return entry, true, ""
	}
	if item.Seed != "" {
		return registryEntry{}, false, fmt.Sprintf("skip: native %s: neither %s nor %s is registered", item.Source, item.Seed, item.EntityID)
	}
	return registryEntry{}, false, fmt.Sprintf("skip: native %s: %s isn't registered -- name its entity with \"as local <entity_id>\"", item.Source, item.EntityID)
}

// nativeIDs are the frozen IDs to write into Physical.def (PROJECT.md 30): for every native item
// without one, its entry's platform and unique_id, keyed by its source.
func nativeIDs(registry []registryEntry, manifest tRegistryManifest) (map[string]TNativeID, []string) {
	byEntityID := map[string]registryEntry{}
	for _, entry := range registry {
		byEntityID[entry.EntityID] = entry
	}
	ids, reports := map[string]TNativeID{}, []string(nil)
	for _, item := range manifest.items("native") {
		if item.Platform != "" {
			continue
		}
		entry, found, _ := nativeEntry(item, byEntityID, nil)
		if !found {
			continue // reported by planRegistryRepair
		}
		if entry.UniqueID == "" || strings.ContainsAny(entry.UniqueID, "\"\n") || !nativePlatformPattern.MatchString(entry.Platform) {
			reports = append(reports, fmt.Sprintf("skip: native %s: %s's platform %q and unique_id %q can't be written as \"as unique_id <platform> \\\"<unique_id>\\\"\"", item.Source, entry.EntityID, entry.Platform, entry.UniqueID))
			continue
		}
		ids[item.Source] = TNativeID{entry.Platform, entry.UniqueID}
	}
	return ids, reports
}

var nativePlatformPattern = regexp.MustCompile(`^[a-z0-9_]+$`)

func sortRegistryKeys(keys []registryKey) {
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Domain != keys[j].Domain {
			return keys[i].Domain < keys[j].Domain
		}
		return keys[i].UniqueID < keys[j].UniqueID
	})
}

// registryDisplayUpdate is a sensor's new display options (PROJECT.md 24).
type registryDisplayUpdate struct {
	EntityID string
	Options  map[string]any
}

// displayUpdates are the sensors whose display options (precision, display unit) differ from what
// the defaults give them, every other key of those options kept. A display unit is only set for a
// sensor with a device class whose shown unit differs (Home Assistant converts it).
func (c *tHAConnection) displayUpdates(manifest tRegistryManifest, attributes map[string]map[string]any) []registryDisplayUpdate {
	var updates []registryDisplayUpdate
	for _, item := range manifest.items("display") {
		attrs, present := attributes[item.EntityID]
		if !present {
			continue // not there (yet): the next deploy
		}
		reply, err := c.call(map[string]any{"type": "config/entity_registry/get", "entity_id": item.EntityID})
		if err != nil || reply["success"] != true {
			continue // no registry entry (an entity without unique_id): nothing to set
		}
		current := map[string]any{}
		if result, ok := reply["result"].(map[string]any); ok {
			if options, ok := result["options"].(map[string]any); ok {
				if sensor, ok := options["sensor"].(map[string]any); ok {
					current = sensor
				}
			}
		}
		wanted := map[string]any{}
		for k, v := range current {
			wanted[k] = v
		}
		if item.Precision != nil {
			wanted["display_precision"] = float64(*item.Precision)
		}
		shown, _ := attrs["unit_of_measurement"].(string)
		if item.Unit != "" && attrs["device_class"] != nil && shown != "" && shown != item.Unit {
			wanted["unit_of_measurement"] = item.Unit
		}
		if !jsonEqual(current, wanted) {
			updates = append(updates, registryDisplayUpdate{item.EntityID, wanted})
		}
	}
	return updates
}

func jsonEqual(a, b any) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}

// applyRegistryRepair removes and renames -- the renames in two phases, so a set of renames that
// trade names (a -> b, b -> a) never collides -- and sets the display options (a failure there is
// a warning, never a failed deploy).
func (c *tHAConnection) applyRegistryRepair(renames []registryRename, removals []registryEntry, displays []registryDisplayUpdate) (errors, warnings []string) {
	for _, display := range displays {
		if reply, err := c.call(map[string]any{"type": "config/entity_registry/update", "entity_id": display.EntityID, "options_domain": "sensor", "options": display.Options}); err != nil || reply["success"] != true {
			warnings = append(warnings, fmt.Sprintf("display options of %s: %v", display.EntityID, replyError(reply, err)))
		}
	}
	for _, entry := range removals {
		if reply, err := c.call(map[string]any{"type": "config/entity_registry/remove", "entity_id": entry.EntityID}); err != nil || reply["success"] != true {
			errors = append(errors, fmt.Sprintf("remove %s: %v", entry.EntityID, replyError(reply, err)))
		}
	}
	type parked struct{ temporary, target string }
	var parkedRenames []parked
	for n, rename := range renames {
		temporary := fmt.Sprintf("%s.registry_sync_%d_%d", rename.Entry.domain(), os.Getpid(), n)
		if reply, err := c.call(map[string]any{"type": "config/entity_registry/update", "entity_id": rename.Entry.EntityID, "new_entity_id": temporary}); err != nil || reply["success"] != true {
			errors = append(errors, fmt.Sprintf("rename %s -> %s: %v", rename.Entry.EntityID, rename.Target, replyError(reply, err)))
			continue
		}
		parkedRenames = append(parkedRenames, parked{temporary, rename.Target})
	}
	for _, p := range parkedRenames {
		if reply, err := c.call(map[string]any{"type": "config/entity_registry/update", "entity_id": p.temporary, "new_entity_id": p.target}); err != nil || reply["success"] != true {
			errors = append(errors, fmt.Sprintf("rename %s -> %s: %v (parked; the next run retries)", p.temporary, p.target, replyError(reply, err)))
		}
	}
	return errors, warnings
}

func replyError(reply map[string]any, err error) any {
	if err != nil {
		return err
	}
	return reply["error"]
}

// repairRegistry is an HA OS instance's registry repair: its registry_manifest.json against its
// registry, the previous manifest read from and saved to the instance (registryStatePath). With -n
// it reads the registry and reports what it would do -- nothing changed, nothing saved.
func (d *tDeployer) repairRegistry(instance TDeployInstance) {
	manifestPath := filepath.Join(d.houseDir, instance.Files, "registry_manifest.json")
	fmt.Printf("+ registry repair: %s against %s's registry (websocket, through an SSH tunnel)\n", manifestPath[len(d.houseDir)+1:], instance.Name)
	token := d.instanceToken(instance)
	if token == "" {
		fmt.Printf("  WARNING: no ${%s_ha_token} in the defs -- registry repair skipped\n", instance.Name)
		return
	}
	data, err := os.ReadFile(manifestPath)
	var manifest tRegistryManifest
	if err == nil {
		err = json.Unmarshal(data, &manifest)
	}
	if err != nil {
		fmt.Printf("  ERROR: manifest: %v\n", err)
		d.stepFailed = true
		return
	}
	previous := tRegistryManifest{}
	if state, err := exec.Command("ssh", append(d.ssh(instance.Machine), "sudo cat "+registryStatePath+" 2> /dev/null; true")...).Output(); err == nil && len(state) > 0 {
		json.Unmarshal(state, &previous)
	}

	ha, err := connectHomeAssistant(d.ssh(instance.Machine), token)
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		d.stepFailed = true
		return
	}
	defer ha.Close()
	registryReply, err1 := ha.call(map[string]any{"type": "config/entity_registry/list"})
	statesReply, err2 := ha.call(map[string]any{"type": "get_states"})
	if err1 != nil || err2 != nil {
		fmt.Printf("  ERROR: reading the registry: %v %v\n", err1, err2)
		d.stepFailed = true
		return
	}
	var registry []registryEntry
	remarshal(registryReply["result"], &registry)
	var states []struct {
		EntityID   string         `json:"entity_id"`
		Attributes map[string]any `json:"attributes"`
	}
	remarshal(statesReply["result"], &states)
	live, attributes := map[string]bool{}, map[string]map[string]any{}
	for _, state := range states {
		attributes[state.EntityID] = state.Attributes
		if restored, _ := state.Attributes["restored"].(bool); !restored {
			live[state.EntityID] = true
		}
	}

	renames, removals, reports := planRegistryRepair(registry, manifest, previous, live)
	note := ""
	if len(previous) == 0 {
		note = " -- no previous manifest yet, so no removals this time"
	}
	mode := "applying"
	if d.dryRun {
		mode = "dry run"
	}
	fmt.Printf("registry repair (%s): %d rename(s), %d removal(s)%s\n", mode, len(renames), len(removals), note)
	for _, rename := range renames {
		fmt.Printf("  rename %s -> %s\n", rename.Entry.EntityID, rename.Target)
	}
	for _, entry := range removals {
		fmt.Printf("  remove %s (%s, no longer generated)\n", entry.EntityID, entry.Platform)
	}
	for _, report := range reports {
		fmt.Println("  " + report)
	}
	displays := ha.displayUpdates(manifest, attributes)
	fmt.Printf("display options: %d sensor(s)\n", len(displays))
	ids, idReports := nativeIDs(registry, manifest)
	for _, report := range idReports {
		fmt.Println("  " + report)
	}
	if d.dryRun {
		for _, source := range sortedKeys(ids) {
			fmt.Printf("  would freeze native %s as %s %q in Physical.def\n", source, ids[source].Platform, ids[source].UniqueID)
		}
		return
	}
	if len(ids) > 0 {
		for _, report := range writeNativeIDs(filepath.Join(d.houseDir, "Definitions", "Physical.def"), ids) {
			fmt.Println("  " + report)
		}
		fmt.Println("  Physical.def has new frozen native IDs: generate before the next deploy")
	}
	errors, warnings := ha.applyRegistryRepair(renames, removals, displays)
	for _, warning := range warnings {
		fmt.Println("  WARNING " + warning)
	}
	for _, e := range errors {
		fmt.Println("  ERROR " + e)
	}
	if len(errors) > 0 {
		d.stepFailed = true
		return
	}
	saved := tRegistryManifest{}
	for k, v := range manifest {
		saved[k] = v
	}
	pending, _ := json.Marshal(pendingRemovals(registry, manifest, previous, live))
	if string(pending) == "null" {
		pending = []byte("[]")
	}
	saved["pending_removal"] = pending
	state, _ := json.Marshal(saved)
	save := exec.Command("ssh", append(d.ssh(instance.Machine), "sudo tee "+registryStatePath+" > /dev/null")...)
	save.Stdin = strings.NewReader(string(state))
	if err := save.Run(); err != nil {
		fmt.Printf("  ERROR: saving the manifest as %s: %v\n", registryStatePath, err)
		d.stepFailed = true
	}
}

// instanceToken is the instance's long-lived access token, ${<instance>_ha_token} in the defs.
func (d *tDeployer) instanceToken(instance TDeployInstance) string {
	settings := parseDefinitionAssignments(readCombinedSettingsContent(filepath.Join(d.houseDir, "Definitions")))
	return resolveDefinitionReference("${"+instance.Name+"_ha_token}", settings)
}

// remarshal decodes a websocket result into a typed value.
func remarshal(value any, into any) {
	data, _ := json.Marshal(value)
	json.Unmarshal(data, into)
}

// waitForReload waits until the instance's reload automation is idle again and probe -- an entity
// of its YAML mqtt config -- is there (after a restart, HA answers before MQTT is set up), at most
// about three minutes; then returns the reload automation's errors logged since since.
func (c *tHAConnection) waitForReload(probe string, since int64) (errors []string, finished bool) {
	for attempt := 0; attempt < 33; attempt++ {
		automation, automationStatus := c.state("automation.coordinator_meta_reload")
		_, probeStatus := c.state(probe)
		attributes, _ := automation["attributes"].(map[string]any)
		current, known := attributes["current"].(float64)
		if automationStatus == 200 && known && current == 0 && probeStatus == 200 {
			finished = true
			break
		}
		time.Sleep(5 * time.Second)
	}
	if !finished {
		return nil, false
	}
	reply, err := c.call(map[string]any{"type": "system_log/list"})
	if err != nil {
		return []string{"system_log/list: " + err.Error()}, true
	}
	var entries []struct {
		Name      string   `json:"name"`
		Timestamp float64  `json:"timestamp"`
		Message   []string `json:"message"`
	}
	remarshal(reply["result"], &entries)
	for _, entry := range entries {
		if entry.Name != "homeassistant.components.automation.coordinator_meta_reload" || entry.Timestamp < float64(since) {
			continue
		}
		// The entry gathers the messages of one kind of error; the last ones are the latest.
		messages := entry.Message
		if len(messages) > 3 {
			messages = messages[len(messages)-3:]
		}
		errors = append(errors, messages...)
	}
	return errors, true
}
