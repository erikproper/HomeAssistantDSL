#!/usr/bin/env python3
"""Extract, from Home Assistant's own code, the fields each MQTT domain accepts (PROJECT.md 19c).

Home Assistant's MQTT discovery validates a payload with the domain's YAML schema extended with
REMOVE_EXTRA (e.g. mqtt/binary_sensor.py: DISCOVERY_SCHEMA = PLATFORM_SCHEMA_MODERN.extend({},
extra=REMOVE_EXTRA)): a field the schema doesn't know is dropped silently -- the same field makes a
YAML "mqtt:" entity invalid. The generator writes YAML entities from relayed discovery configs, so
it must drop exactly those fields: this tool reads them from the installed Home Assistant itself,
for the version it runs, as hamqtt/ha_mqtt_fields.json (embedded into the generator and the coordinator):

  {"ha_version": ..., "domains": {domain: [field, ...]}, "light_schemas": {"basic"|"json"|
   "template": [...]}, "abbreviations": {...}, "device_abbreviations": {...},
   "origin_abbreviations": {...}}

Run it with a Python holding the house's Home Assistant version -- e.g. inside Home Assistant's own
container image, with the table written to standard output:
  podman run --rm -i --network none --entrypoint python3 \
    ghcr.io/home-assistant/home-assistant:<version> - --stdout < ha_mqtt_fields.py > ../hamqtt/ha_mqtt_fields.json
-- again after a Home Assistant upgrade that changes components/mqtt.
Usage: ha_mqtt_fields.py [--stdout]
"""
import importlib
import json
import os
import pkgutil
import sys

from homeassistant.const import __version__ as ha_version

try:
    import probatio as vol  # Home Assistant's schema library since 2026 (installed as "voluptuous")
except ImportError:
    import voluptuous as vol
import homeassistant.components.mqtt as mqtt_package
from homeassistant.components.mqtt import abbreviations

# Modules of the MQTT component that aren't entity domains.
NOT_DOMAINS = {
    "abbreviations", "addon", "async_client", "client", "config", "config_flow",
    "config_integration", "const", "debug_info", "device_automation", "device_trigger",
    "diagnostics", "discovery", "entity", "models", "repairs", "schemas", "subscription", "tag",
    "trigger", "util",
}


def schema_fields(schema):
    """The field names a (possibly composed) voluptuous schema accepts."""
    if isinstance(schema, vol.All):
        fields = set()
        for validator in schema.validators:
            fields |= schema_fields(validator)
        return fields
    if isinstance(schema, vol.Schema):
        schema = schema.schema
    if isinstance(schema, dict):
        return {str(key.schema if isinstance(key, vol.Marker) else key) for key in schema}
    return set()


def main():
    domains = {}
    for info in pkgutil.iter_modules(mqtt_package.__path__):
        name = info.name
        if name in NOT_DOMAINS or name == "light":
            continue
        module = importlib.import_module(f"homeassistant.components.mqtt.{name}")
        schema = getattr(module, "PLATFORM_SCHEMA_MODERN", None)
        if schema is None:
            continue
        fields = schema_fields(schema)
        if fields:
            domains[name] = sorted(fields)

    light_schemas = {}
    for kind in ("basic", "json", "template"):
        module = importlib.import_module(f"homeassistant.components.mqtt.light.schema_{kind}")
        schema = getattr(module, f"PLATFORM_SCHEMA_MODERN_{kind.upper()}", None)
        if schema is None:
            sys.exit(f"light schema_{kind}: no PLATFORM_SCHEMA_MODERN_{kind.upper()} -- update this tool")
        light_schemas[kind] = sorted(schema_fields(schema))

    table = {
        "ha_version": ha_version,
        "domains": domains,
        "light_schemas": light_schemas,
        "abbreviations": dict(abbreviations.ABBREVIATIONS),
        "device_abbreviations": dict(abbreviations.DEVICE_ABBREVIATIONS),
        "origin_abbreviations": dict(abbreviations.ORIGIN_ABBREVIATIONS),
    }
    if "--stdout" in sys.argv:
        json.dump(table, sys.stdout, indent=1, sort_keys=True)
        sys.stdout.write("\n")
        return
    path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "hamqtt", "ha_mqtt_fields.json")
    with open(path, "w") as out:
        json.dump(table, out, indent=1, sort_keys=True)
        out.write("\n")
    print(f"{path}: {len(domains)} domains, light schemas {sorted(light_schemas)}")


if __name__ == "__main__":
    main()
