---
name: project-living-room-bt-proxy-plan
description: "DONE 2026-10-15: the BT dongle moved from protocols-server-1@Junglinster to frame@Junglinster (the Ubuntu-cloned picture-frame host), running denvera/bt-proxy as a podman quadlet, paired with HA's bluetooth integration on ha2mqtt."
metadata:
  node_type: memory
  type: project
  originSessionId: 90d6291d-7e36-4477-8954-091d4a9d14e2
  modified: 2026-10-15T11:34:01.374Z
---

**Done**: frame@Junglinster (a Raspberry Pi, Ubuntu 26.04 aarch64, cloned from frame@Vienna's own
image) now runs a dedicated Bluetooth proxy for HA, using the USB Broadcom BCM20702A0 dongle
(`hci0`, btusb driver) that was pulled off protocols-server-1@Junglinster once its onboard Intel
adapter was confirmed sufficient there. frame's own onboard Pi Bluetooth (`hci1`,
`hci_uart_bcm`) is left alone — the dongle (`hci0`) was already the default controller, so no
`--adapter` override was needed.

**Software**: [denvera/bt-proxy](https://github.com/denvera/bt-proxy) — an ESPHome-native-API-
compatible Bluetooth proxy for any Linux host with BlueZ (not real ESPHome firmware, which is
ESP32-only via `esp32_ble`). Speaks the same wire protocol HA's `esphome` integration expects, so
HA discovers/pairs it exactly like a real ESP32 proxy. This is also what "protocols-server-1-bt-
proxy" (the stale ESPHome entry cleaned up earlier this session) actually was — a prior, now-
defunct instance of this same project, not a test ESP32.

**Deployment**: podman quadlet at `/etc/containers/systemd/bt-proxy.container` on frame (bluez
package + `bluetooth.service` had to be installed/enabled first — frame didn't have either).
`Network=host` (required for mDNS discovery) + `PodmanArgs=--privileged` + `Volume=/var/run/dbus:
/var/run/dbus`, image `ghcr.io/denvera/bt-proxy`, `Exec=--name frame-bt-proxy --friendly-name
"Junglinster Frame BT Proxy"`. Confirmed live: paired with HA's bluetooth integration on ha2mqtt.

**How to apply**: this is now the established pattern for giving HA a remote/relocatable BLE
adapter without real ESP32 hardware or USB-over-IP — reuse `denvera/bt-proxy` (same podman-quadlet
shape) for any future "put a Bluetooth adapter somewhere else on the network" need, rather than
re-deriving it or defaulting to usbip.
