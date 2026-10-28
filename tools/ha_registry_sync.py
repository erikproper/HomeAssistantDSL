#!/usr/bin/env python3
"""Keep Home Assistant's entity registry in step with the generator (PROJECT.md item 48).

Runs inside the HA SSH add-on (standard library only), as root, so it can read the Supervisor
token. Compares the generator's registry_manifest.json (per generated entity: the unique_id HA
registers it under, the entity_id it should have) with HA's own entity registry, then:

  * renames every registered entity whose entity_id differs from the manifest's -- the same
    registry entry keeps its settings and history, so a rename never leaves an orphan behind;
  * removes every entry this generator created on an earlier deploy but no longer generates -- so a
    removed entity never lingers as an orphan either. YAML-platform entries (template, group,
    automation, script, input_*) always; MQTT entries
    (Zigbee, imported or bridged) only once nothing provides them anymore (no state, or a restored one):
    the coordinator retracts their discovery itself and HA then deletes them, except a disabled
    one, which HA leaves behind -- that's the case this catches.

Only ever touches entries it can prove are the generator's own: a YAML-platform entry (no config
entry) whose unique_id is in the current or the previous manifest, or an MQTT entry whose
unique_id is a frozen stable_id in the manifest. A rename whose target entity_id is held by
anything else is skipped and reported, never forced.

This is the registry REPAIR of the deploy, and the only registry change the system makes by itself
(Erik, 2026-10-26): driven solely by the generator's own table (the manifest), never ad hoc; any
other registry change is an explicit one-off migration. It depends on Home Assistant's websocket
registry API and on how Home Assistant keeps entity_ids per unique_id -- see Architecture.md,
"Home Assistant's entity identities, and the registry repair at deploy".

Usage: ha_registry_sync.py <manifest> <state-file> [--apply]
Without --apply it only reports what it would do. With --apply it also saves the manifest as
<state-file>, the "previous manifest" of the next run.
"""
import base64
import json
import os
import socket
import struct
import sys

# The YAML platforms the generator writes entities for. An entry of one of these without a config
# entry is YAML-defined; only those can be the generator's own.
YAML_PLATFORMS = {"template", "group", "command_line"}


# --- websocket (minimal, standard library only) ---

class HomeAssistant:
    def __init__(self, host="supervisor", port=80, path="/core/websocket"):
        self.sock = socket.create_connection((host, port), timeout=60)
        key = base64.b64encode(os.urandom(16)).decode()
        self.sock.sendall((f"GET {path} HTTP/1.1\r\nHost: {host}\r\nUpgrade: websocket\r\n"
                           f"Connection: Upgrade\r\nSec-WebSocket-Key: {key}\r\n"
                           f"Sec-WebSocket-Version: 13\r\n\r\n").encode())
        response = b""
        while b"\r\n\r\n" not in response:
            response += self.sock.recv(1)
        if b" 101 " not in response.split(b"\r\n")[0]:
            raise SystemExit("websocket upgrade failed: " + response.decode(errors="replace"))
        self.next_id = 1
        if self._receive()["type"] != "auth_required":
            raise SystemExit("unexpected websocket greeting")
        token = open("/run/s6/container_environment/SUPERVISOR_TOKEN").read().strip()
        self._send({"type": "auth", "access_token": token})
        if self._receive()["type"] != "auth_ok":
            raise SystemExit("websocket authentication failed")

    def call(self, command):
        command = dict(command, id=self.next_id)
        self.next_id += 1
        self._send(command)
        while True:
            reply = self._receive()
            if reply.get("id") == command["id"] and reply.get("type") == "result":
                return reply

    def _recv_exact(self, n):
        data = b""
        while len(data) < n:
            chunk = self.sock.recv(n - len(data))
            if not chunk:
                raise SystemExit("websocket closed")
            data += chunk
        return data

    def _send(self, obj):
        payload = json.dumps(obj).encode()
        header = bytes([0x81])
        mask = os.urandom(4)
        if len(payload) < 126:
            header += bytes([0x80 | len(payload)])
        elif len(payload) < 65536:
            header += bytes([0x80 | 126]) + struct.pack(">H", len(payload))
        else:
            header += bytes([0x80 | 127]) + struct.pack(">Q", len(payload))
        self.sock.sendall(header + mask + bytes(b ^ mask[i % 4] for i, b in enumerate(payload)))

    def _receive(self):
        message = b""
        while True:
            first, second = self._recv_exact(2)
            length = second & 0x7F
            if length == 126:
                length = struct.unpack(">H", self._recv_exact(2))[0]
            elif length == 127:
                length = struct.unpack(">Q", self._recv_exact(8))[0]
            message += self._recv_exact(length)
            if first & 0x80:
                return json.loads(message)


# --- planning ---

def load_json(path, default):
    try:
        return json.load(open(path))
    except FileNotFoundError:
        return default


def domain_of(entity_id):
    return entity_id.split(".", 1)[0]


def manifest_key(item):
    return (domain_of(item["entity_id"]), item["unique_id"])


def is_yaml_entry(entry):
    return entry["platform"] in YAML_PLATFORMS and not entry.get("config_entry_id")


MQTT_KINDS = ("mqtt", "imported", "hassbridge", "pending_removal")


def stale_mqtt_keys(manifest, previous):
    """The mqtt entries generated before -- or still pending removal -- but no longer."""
    current = {manifest_key(m) for kind in MQTT_KINDS for m in manifest.get(kind, [])}
    earlier = {manifest_key(m) for kind in MQTT_KINDS for m in previous.get(kind, [])}
    return earlier - current


def pending_removals(registry, manifest, previous, live_entity_ids):
    """The no longer generated mqtt entries that were still live, so not removed this time: kept in
    the saved state ("pending_removal") so the next run removes them (2026-10-28, found when a
    bridged entity moved to "local": the sync runs before HA's reload, so a dropped YAML mqtt
    entity is always still live then, and was forgotten -- an orphan for good)."""
    stale = stale_mqtt_keys(manifest, previous)
    pending = []
    for entry in registry:
        key = (domain_of(entry["entity_id"]), entry["unique_id"])
        if entry["platform"] == "mqtt" and key in stale and entry["entity_id"] in live_entity_ids:
            pending.append({"unique_id": entry["unique_id"], "entity_id": entry["entity_id"]})
    return sorted(pending, key=lambda p: p["entity_id"])


def plan(registry, manifest, previous, live_entity_ids=frozenset()):
    """Returns (renames, removals, reports). renames: [(entry, new_entity_id)], removals: [entry]."""
    reports = []
    by_entity_id = {e["entity_id"]: e for e in registry}
    yaml_by_unique_id, mqtt_by_unique_id = {}, {}
    # HA's registry is keyed on (domain, platform, unique_id): one unique_id may well appear in two
    # domains (e.g. a Zigbee button's "event" and "sensor" entity), so the domain is part of the key.
    for entry in registry:
        key = (domain_of(entry["entity_id"]), entry["unique_id"])
        if is_yaml_entry(entry):
            yaml_by_unique_id.setdefault(key, []).append(entry)
        elif entry["platform"] == "mqtt":
            mqtt_by_unique_id.setdefault(key, []).append(entry)

    current_yaml = {manifest_key(m) for m in manifest["yaml"]}
    previous_yaml = {manifest_key(m) for m in previous.get("yaml", [])}
    stale_yaml = previous_yaml - current_yaml

    # Removals: entries generated before but no longer -- never anything else.
    removals = [e for key in sorted(stale_yaml) for e in yaml_by_unique_id.get(key, [])]
    # Generated automations, scripts and input_* helpers: matched on (platform, unique_id), YAML only.
    other_platforms = {o["platform"] for o in manifest.get("other", []) + previous.get("other", [])}
    current_other = {(o["platform"], o["unique_id"]) for o in manifest.get("other", [])}
    previous_other = {(o["platform"], o["unique_id"]) for o in previous.get("other", [])}
    for entry in registry:
        key = (entry["platform"], entry["unique_id"])
        if entry["platform"] in other_platforms and not entry.get("config_entry_id") \
                and key in previous_other and key not in current_other:
            removals.append(entry)

    for key in sorted(stale_mqtt_keys(manifest, previous)):
        for entry in mqtt_by_unique_id.get(key, []):
            if entry["entity_id"] not in live_entity_ids:
                removals.append(entry)
    removed = {e["entity_id"] for e in removals}

    renames = []
    for kind, index in (("yaml", yaml_by_unique_id), ("mqtt", mqtt_by_unique_id), ("imported", mqtt_by_unique_id),
                        ("hassbridge", mqtt_by_unique_id)):
        for item in manifest.get(kind, []):
            entries = index.get(manifest_key(item), [])
            if len(entries) != 1:
                if len(entries) > 1:
                    reports.append(f"skip: unique_id {item['unique_id']} has {len(entries)} registry entries")
                continue  # not registered yet: HA will register it under the right name itself
            entry = entries[0]
            if entry["entity_id"] != item["entity_id"]:
                renames.append((entry, item["entity_id"]))

    # Native-integration entities (PROJECT.md 17j): an entity the generator only refers to, whose
    # unique_id it doesn't know, is identified by its source in the defs ("<device-id>!<capability>").
    # When the entity_id expected for a source changed since the previous deploy, the entry still
    # holding the previous one is renamed. Never removed: it belongs to its integration.
    # The first time (PROJECT.md 21, 2026-10-28), when the defs name the entity_id its integration
    # gave it ("seed") and no previous deploy renamed it yet, that entity is renamed.
    previous_native = {n["source"]: n["entity_id"] for n in previous.get("native", [])}
    for item in manifest.get("native", []):
        holder = by_entity_id.get(item["entity_id"])
        if holder is not None and not is_yaml_entry(holder) and holder["platform"] != "mqtt":
            continue  # already under its expected name
        old = previous_native.get(item["source"])
        if old is None or old == item["entity_id"]:
            old = item.get("seed")
        if not old or old == item["entity_id"]:
            continue
        entry = by_entity_id.get(old)
        if entry is None:
            if item["entity_id"] not in by_entity_id:
                reports.append(f"skip: native {item['source']}: neither {item['entity_id']} nor its previous {old} is registered")
            continue
        if is_yaml_entry(entry) or entry["platform"] == "mqtt":
            reports.append(f"skip: native {item['source']}: {old} is a {entry['platform']} entry, not a native integration's")
            continue
        renames.append((entry, item["entity_id"]))

    # A rename's target must be free, freed by a removal, or held by an entry that itself moves.
    moving = {entry["entity_id"] for entry, _ in renames}
    feasible = []
    for entry, target in renames:
        holder = by_entity_id.get(target)
        if holder is None or target in removed or target in moving:
            feasible.append((entry, target))
        else:
            reports.append(f"skip: rename {entry['entity_id']} -> {target}: target is held by "
                           f"{holder['platform']} entry {holder['unique_id']!r}, not the generator's own")
    return feasible, removals, reports


def apply(ha, renames, removals):
    errors = []
    for entry in removals:
        reply = ha.call({"type": "config/entity_registry/remove", "entity_id": entry["entity_id"]})
        if not reply.get("success"):
            errors.append(f"remove {entry['entity_id']}: {reply.get('error')}")
    # Two phases, so a set of renames that trade names (a -> b, b -> a) never collides.
    parked = []
    for n, (entry, target) in enumerate(renames):
        domain = entry["entity_id"].split(".", 1)[0]
        temporary = f"{domain}.registry_sync_{os.getpid()}_{n}"
        reply = ha.call({"type": "config/entity_registry/update", "entity_id": entry["entity_id"],
                         "new_entity_id": temporary})
        if reply.get("success"):
            parked.append((temporary, target))
        else:
            errors.append(f"rename {entry['entity_id']} -> {target}: {reply.get('error')}")
    for temporary, target in parked:
        reply = ha.call({"type": "config/entity_registry/update", "entity_id": temporary,
                         "new_entity_id": target})
        if not reply.get("success"):
            errors.append(f"rename {temporary} -> {target}: {reply.get('error')} (parked; the next run retries)")
    return errors


def main():
    args = [a for a in sys.argv[1:] if a != "--apply"]
    do_apply = "--apply" in sys.argv[1:]
    if len(args) != 2:
        raise SystemExit(__doc__)
    manifest_path, state_path = args
    manifest = load_json(manifest_path, None)
    if manifest is None:
        raise SystemExit(f"no manifest at {manifest_path}")
    previous = load_json(state_path, {})

    ha = HomeAssistant()
    registry = ha.call({"type": "config/entity_registry/list"})["result"]
    states = ha.call({"type": "get_states"})["result"]
    live = frozenset(s["entity_id"] for s in states if not s.get("attributes", {}).get("restored"))
    renames, removals, reports = plan(registry, manifest, previous, live)

    mode = "applying" if do_apply else "dry run"
    print(f"registry sync ({mode}): {len(renames)} rename(s), {len(removals)} removal(s)"
          + ("" if previous else " -- no previous manifest yet, so no removals this time"))
    for entry, target in renames:
        print(f"  rename {entry['entity_id']} -> {target}")
    for entry in removals:
        print(f"  remove {entry['entity_id']} ({entry['platform']}, no longer generated)")
    for report in reports:
        print(f"  {report}")

    if do_apply:
        errors = apply(ha, renames, removals)
        for error in errors:
            print(f"  ERROR {error}")
        if errors:
            raise SystemExit(1)
        saved = dict(manifest)
        saved["pending_removal"] = pending_removals(registry, manifest, previous, live)
        with open(state_path, "w") as state:
            json.dump(saved, state)


if __name__ == "__main__":
    main()
