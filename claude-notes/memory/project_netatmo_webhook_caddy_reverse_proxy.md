---
name: project-netatmo-webhook-caddy-reverse-proxy
description: "DONE (2026-08-26): protocols-server-2's HA instance gets a public HTTPS endpoint (junglinster.homelinux.org, via FritzBox port-forward + a Caddy Podman Quadlet + Let's Encrypt) so Netatmo's integration can register a push webhook instead of polling -- no Nabu Casa subscription needed."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-08-26T21:18:16.041Z
---

Netatmo's HA integration wants a public HTTPS:443 endpoint to register a push webhook (otherwise
it falls back to slow polling, ~10 min). The user already has two Nabu Casa Cloud subscriptions
(one per main instance per house) and didn't want a third just for protocols-server-2 (p-s-2) --
Netatmo is deliberately kept off the main "Home Assistant Green" instance to keep it clean of
non-core integrations (the user's own framing).

**Solution: self-hosted reverse proxy, zero subscription cost.** p-s-2's FritzBox already has a
dyndns name (`junglinster.homelinux.org`) and a real (non-CGNAT) public IP. Forwarded external
443 -> p-s-2's LAN IP:443, terminated by a **Caddy** container on p-s-2 itself (automatic
Let's-Encrypt, no certbot/deploy-hooks needed), reverse-proxying to the HA container.

**p-s-2 specifics (Fedora + Podman + Quadlet)**, worth remembering for any future container work
on this host:
- Containers are managed via **Podman Quadlet** (`.container` files under
  `/etc/containers/systemd/`, auto-generating systemd units) -- HA's own is
  `/etc/containers/systemd/homeassistant.container`, **rootful** (only visible via `sudo podman
  ps`, not the regular user's), `Network=host`, `Volume=...:Z` SELinux labels on bind mounts.
  Firewall is `firewalld` (not `ufw`) -- `firewall-cmd --permanent --add-port=<n>/tcp --reload`.
- Quadlet units are **not** `systemctl enable`d the normal way -- "Failed to enable unit:
  ...transient or generated" is expected; the `[Install]` section is handled automatically by the
  generator at boot. Just `systemctl daemon-reload` + `systemctl start <name>.service`.
- Added `caddy.container` (same directory, same conventions: rootful, `Network=host`,
  `/var/lib/home-automation/caddy/{Caddyfile,data,config}` mirroring HA's own
  `/var/lib/home-automation/homeassistant` volume tree). Caddyfile is a two-line
  `junglinster.homelinux.org { reverse_proxy 127.0.0.1:8123 }` -- `127.0.0.1` works because both
  containers share the host's network namespace (`Network=host`).
- Caddy's `http-01` ACME challenge failed once (port 80 forward issue, never diagnosed further --
  non-blocking) but it transparently fell back to `tls-alpn-01` (port 443 only) and obtained the
  cert successfully anyway. Worth checking if the HTTP->HTTPS auto-redirect ever matters later.

**Real gotcha, cost real back-and-forth**: tried making the *generator* write an `http:` block
(`use_x_forwarded_for`/`trusted_proxies`) into p-s-2's generated `configuration.yaml`, since
hand-editing a generator-owned file is fragile (overwritten every regeneration) -- built a whole
`reverse_proxied` DSL keyword + tests for it, then discovered live that Home Assistant has
**migrated the `http:` integration to a UI-managed config entry** in recent versions: any `http:`
block in `configuration.yaml` is silently ignored (removed outright from 2027.2.0). **Reverted
the whole DSL feature** (`homeassistant/defined.go`'s `THomeAssistantInstance`,
`remote_instance_automations.go`'s `writeRemoteInstanceSkeleton`, both back to their pre-change
shape) -- this setting genuinely can't be generator-owned any more. The actual fix: Settings ->
System -> Network in HA's own UI, "Trust X-Forwarded-For" = on, trusted proxies =
`127.0.0.1/32` + `::1/128`. **Lesson for next time**: before building generator support for *any*
`configuration.yaml` top-level key, check whether that HA integration has since moved to a
UI-managed config entry -- this migration has been happening piecemeal across HA integrations for
a while (http, mqtt, and others), and YAML support silently degrades to a no-op rather than an
error.
