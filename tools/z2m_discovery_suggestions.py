#!/usr/bin/env python3
"""
z2m_discovery_suggestions.py -- turns suggestions/discovery.txt's own generic, "sensor.:
<leaf>; # not recognized" placeholder lines into ready-to-paste Physical.def capability lines,
using the raw Zigbee2MQTT discovery config's own real domain and a Zigbee2MQTT-specific "strip the
device's own identifier prefix and the _zigbee2mqtt suffix" label convention.

Deliberately NOT built into the Go generator (homeassistant/mqtt_discovery_existence.go's own
recognizeDiscoveryCapability): that function has to stay platform-agnostic (a "discovery" gateway
could be EMS-ESP, Z-Wave, anything else self-announcing over MQTT discovery, each with its own
leaf-naming convention) -- this script's own stripping/labelling rule is real, but ONLY true for
Zigbee2MQTT's own "<device-identifier>_<attribute>_zigbee2mqtt" unique_id convention. Since both
houses are (for now) exclusively migrating Zigbee2MQTT devices, it's a reusable dev-tool script
instead, not a permanent code path.

Usage:
    1. Run ./generate once so suggestions/discovery.txt is fresh.
    2. Capture a live discovery dump (needs the house's own coordinator SSH/broker credentials):
         ssh -p <port> <user>@<host> \\
           "mosquitto_sub -h localhost -p 1883 -u <login> -P <password> \\
            -t 'homeassistant.physical/+/+/+/config' -v" > /tmp/z2m_dump.txt
    3. Run this script:
         python3 tools/z2m_discovery_suggestions.py \\
           --suggestions <House>/suggestions/discovery.txt \\
           --physical-def <House>/Definitions/Physical.def \\
           --dump /tmp/z2m_dump.txt
    4. Review the printed lines, paste each gateway's own block into its existing
       "device discovery.<id> with: ... end;" block in Physical.def (never a bare "sensor.: ...;"
       line -- every line printed here already carries its own real domain and label, so
       Physical.def should never contain a "# not recognized" comment at all).
    5. Re-run ./generate, diff, and only then deploy -- same discipline as every other migration.

This script only ever READS Physical.def and suggestions/discovery.txt; it never writes to either.
"""

import argparse
import json
import re
import sys
from collections import OrderedDict

DISCOVERY_TOPIC_RE = re.compile(r"homeassistant\.physical/([^/]+)/")
GATEWAY_HEADER_RE = re.compile(r"device (discovery\.[A-Za-z0-9_./-]+) with:")
IDENTIFIERS_RE = re.compile(r'identifiers\s+"zigbee2mqtt_([^"]+)"')
SUGGESTION_LINE_RE = re.compile(r"\s*[a-z_]+\.:\s*(\S+);")


def load_leaf_domains(dump_path):
    """Reads a raw `mosquitto_sub -v` capture of homeassistant.physical/+/+/+/config messages and
    returns {unique_id: raw_domain}. Last message wins if a unique_id is somehow seen twice."""
    leaf_domain = {}
    with open(dump_path) as f:
        for line in f:
            line = line.rstrip("\n")
            if not line.strip():
                continue
            parts = line.split(" ", 1)
            if len(parts) != 2:
                continue
            topic, payload = parts
            m = DISCOVERY_TOPIC_RE.match(topic)
            if not m:
                continue
            domain = m.group(1)
            try:
                data = json.loads(payload)
            except ValueError:
                continue
            uid = data.get("unique_id") or data.get("uniq_id")
            if uid:
                leaf_domain[uid] = domain
    return leaf_domain


def load_gateway_prefixes(physical_def_path):
    """Reads Physical.def and returns {gatewayID: identifier-prefix}, e.g.
    {"discovery.hallway_aqara_windoor": "0x00158d0003d4e964"} -- the prefix every one of that
    gateway's own leaf unique_ids starts with, per Zigbee2MQTT's own "<prefix>_<attr>_zigbee2mqtt"
    convention, needed to strip it back off when deriving a label."""
    prefixes = {}
    current_gateway = None
    with open(physical_def_path) as f:
        for line in f:
            header = GATEWAY_HEADER_RE.search(line)
            if header:
                current_gateway = header.group(1)
                continue
            ident = IDENTIFIERS_RE.search(line)
            if ident and current_gateway:
                prefixes[current_gateway] = ident.group(1)
            if re.match(r"\s*end;\s*$", line):
                current_gateway = None
    return prefixes


def load_suggestions(suggestions_path):
    """Reads suggestions/discovery.txt and returns an ordered {gatewayID: [leaf, ...]} -- every
    leaf is already the bare value (post the domain-prefix bug fix, mqtt_discovery_existence.go,
    2026-09-16); this script doesn't care what placeholder domain/comment the line carried."""
    gateways = OrderedDict()
    current_gateway = None
    with open(suggestions_path) as f:
        for line in f:
            header = GATEWAY_HEADER_RE.match(line.strip())
            if header:
                current_gateway = header.group(1)
                gateways[current_gateway] = []
                continue
            if line.strip() == "end;":
                current_gateway = None
                continue
            m = SUGGESTION_LINE_RE.match(line)
            if m and current_gateway is not None:
                gateways[current_gateway].append(m.group(1))
    return gateways


def derive_label(leaf, prefix):
    """Strips prefix + "_zigbee2mqtt" off leaf -- Zigbee2MQTT's own bare attribute name, e.g.
    "0x00158d0003d4e964_linkquality_zigbee2mqtt" with prefix "0x00158d0003d4e964" -> "linkquality".
    Falls back to the leaf itself (rare -- a gateway with no "identifiers" line found) rather than
    guessing wrong."""
    label = leaf
    if prefix and label.startswith(prefix + "_"):
        label = label[len(prefix) + 1 :]
    return re.sub(r"_zigbee2mqtt$", "", label)


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--suggestions", required=True, help="path to suggestions/discovery.txt")
    parser.add_argument("--physical-def", required=True, help="path to the house's own Definitions/Physical.def")
    parser.add_argument("--dump", required=True, help="path to a raw 'mosquitto_sub -v' capture of homeassistant.physical/+/+/+/config")
    parser.add_argument("--gateway", help="only print this one gateway (default: all)")
    args = parser.parse_args()

    leaf_domain = load_leaf_domains(args.dump)
    gateway_prefixes = load_gateway_prefixes(args.physical_def)
    suggestions = load_suggestions(args.suggestions)

    if not suggestions:
        print("No suggestions found -- nothing to do.", file=sys.stderr)
        return

    for gateway, leaves in suggestions.items():
        if args.gateway and gateway != args.gateway:
            continue
        if not leaves:
            continue
        prefix = gateway_prefixes.get(gateway, "")
        print(f"=== {gateway} ===")
        if not prefix:
            print("  # WARNING: no \"identifiers\" line found for this gateway in Physical.def --")
            print("  # labels below could not be stripped of their own device-id prefix.")
        for leaf in leaves:
            domain = leaf_domain.get(leaf)
            if domain is None:
                print(f"  # WARNING: {leaf!r} not seen in --dump -- can't confirm its real domain, skipped")
                continue
            label = derive_label(leaf, prefix)
            print(f"  {domain}.{label}: {leaf};")
        print()


if __name__ == "__main__":
    main()
