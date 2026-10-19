---
name: project-ps1-junglinster-dns-search-domain-reboot-bug
description: "RESOLVED 2026-10-16 (real fix, not a workaround): fresh Ubuntu installs on this project's hosts don't apply the DHCP-advertised search domain for bare hostnames by default. Root cause + real fix: netplan dhcp4-overrides/dhcp6-overrides use-domains: true. Applied on both p-s-1@Junglinster and the new Vienna p-s-1 SSD. The earlier /etc/hosts static-entry workaround was explicitly rejected by the user (\"that is not a fix, but a hack\") and has been reverted everywhere."
metadata:
  node_type: memory
  type: project
  originSessionId: 90d6291d-7e36-4477-8954-091d4a9d14e2
  modified: 2026-10-16T11:27:27.724Z
---

**Real root cause, confirmed 2026-10-16**: Ubuntu's `systemd-networkd` (via netplan) does NOT, by
default, apply a DHCP server's advertised search domain to bare-hostname DNS resolution --
`resolvectl status` already correctly reported `DNS Domain: fritz.box` even while bare lookups
("getent hosts xanadu", "ping xanadu") failed or only resolved the FQDN form. This affects every
fresh Ubuntu install on this project's own hosts (frame.vienna/frame.junglinster,
protocols-server-1@Junglinster, the new Vienna p-s-1 SSD) -- not specific to any one box.

**The real fix**: a netplan drop-in enabling `use-domains`:
```yaml
# /etc/netplan/99-local-dns-search.yaml
network:
  version: 2
  ethernets:
    <real interface name>:        # eno1 on Junglinster's p-s-1, eth0 on Vienna's new SSD -- check
      dhcp4-overrides:             # with `ip -o link show`, don't assume eno1/eth0 from an example
        use-domains: true
      dhcp6-overrides:
        use-domains: true
```
`sudo netplan generate && sudo netplan apply`. Confirmed fully working on BOTH
protocols-server-1@Junglinster (user applied it themselves, interface `eno1`) and Vienna's new
p-s-1 SSD (interface `eth0`) -- `ping xanadu`/`ping junglinster` (bare, no `.fritz.box` suffix)
both resolve and reply correctly afterwards, with NO `/etc/hosts` override needed at all.

**The earlier `/etc/hosts` static-entry workaround was WRONG and has been reverted
everywhere** -- the user's own words: "That is not a fix, but a hack! Revert this immediately!"
Both the Junglinster entry (`192.168.178.42 junglinster`, added 2026-10-15) and the brand-new
Vienna one (`192.168.178.34 xanadu`, added minutes earlier the same day) were removed once the
real netplan fix was confirmed sufficient on its own.

**How to apply**: if a FUTURE Ubuntu host on this project (another fresh install/clone) shows
this same symptom (bare hostname fails or only resolves the FQDN form, despite
`resolvectl status` already showing the right `DNS Domain`), apply this SAME netplan drop-in
immediately -- do NOT reach for a static `/etc/hosts` entry as even a temporary measure; the user
has explicitly rejected that approach twice now.
