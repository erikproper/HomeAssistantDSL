"""Report the errors of Home Assistant's own reload automation since a moment (PROJECT.md 19h.3).

One field Home Assistant doesn't know in a YAML "mqtt:" entity makes its whole MQTT reload fail
(found live 2026-10-27: 64 entities missing while the deploy reported success), so deploy step 06
checks for it once Home Assistant has finished reloading. Not runnable on its own: it is appended
to ha_registry_sync.py (minus that script's main), whose websocket client it uses, and run on the
instance itself:

  { sed '/^if __name__ == "__main__":/,$d' tools/ha_registry_sync.py; cat tools/ha_reload_errors.py; } |
    ssh <instance> sudo python3 - <since, seconds since the epoch>

Prints one line per error message logged since then, nothing when there is none.
"""

def report_reload_errors(since):
    ha = HomeAssistant()
    for entry in ha.call({"type": "system_log/list"}).get("result", []):
        if entry.get("name") != "homeassistant.components.automation.coordinator_meta_reload":
            continue
        if entry.get("timestamp", 0) < since:
            continue
        # The entry gathers the messages of one kind of error; the last ones are the latest.
        for message in entry.get("message", [])[-3:]:
            print(message)


report_reload_errors(float(sys.argv[1]))
