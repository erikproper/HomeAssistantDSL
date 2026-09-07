** TODO 

1. Existence checking again. Entities that are provided on the main instance. Do we check their existence as well? And make suggestions based on their device assignments?
Basically treat these as kind 3 ones, but always originating from the main HA instance.
Even though more and more entities will be pushed "under" the MQTT bus, we know that certain domains (media_player, weather, etc) cannot move there yet. So, we will need to rely on integrations that are directly linked to the conceptual layer within the main HA instance (like all entities used to be).

1b: See point about sources and their names.

2. EP: Hardware migration in Vienna.
- Mo 1 HASS backup; copy backup to MacMini
- Mo 2 HASS on green
- Mo 3 Backup Samsung + photos/frame to AppleSSD on frame
- Tu 4 Pi4 as P-S-1 for Vienna 
    Restore from AppleSSD (also photos!)
- Tu 5 Pi3 as P-S-2 for Vienna
    Copy photos back to P-S-2
- We 6 Setup P-S-1 for Vienna:
    { zigbee, samba, mqtt, smtpproxy, ... }
- ?? 7 Setup P-S-2 for Vienna:
    { picture frame, HA,... }

https://www.reichelt.at/at/de/shop/produkt/raspberry_pi_-_usb_3_0_256_gb-422300
https://shop.funk24.net/Raspberry-Pi-Flash-Drive-USB-3.0-Stick-256-GB
https://shop.funk24.net/Raspberry-Pi-Flash-Drive-USB-3.0-Stick-128-GB

Unifiy port/script mapping

3. Stick migration for Pi3 and PiB:
- Use stick on Pi3 in Vienna
- If this works, order three Raspberry sticks
- Copy the pi3 stick in vienna to one of these sticks
- Migrate frame.junglinster to one of these sticks on fedora
- Migrate protocol-server-2.junglinster to one of these sticks (see below!!)

4. EP: Fritz's have CPU temperature, plus other things + fix compute tabs + check suggestions.
Vienna: Open. Blocked until we have the new hardware deployment

5. EP to rename device names, hiding the integration part of the name.
Also revisit the names of integrations.
EP will provide name mappings
Claude to execute this on the current def files

6. 
  device infrastructural:eriks-macbook-pro-2 from import.eriks-macbook-pro-2 with:
    entity sensor.infrastructural:eriks-macbook-pro-2/cpu/load        from entity cpu/load;
    entity sensor.infrastructural:eriks-macbook-pro-2/cpu/temperature from entity cpu/temperature;
  end;

to be:

  device infrastructural:eriks-macbook-pro-2 from import.eriks-macbook-pro-2 with:
    entity sensor.infrastructural:eriks-macbook-pro-2/cpu/load        from sensor.cpu/load;
    entity sensor.infrastructural:eriks-macbook-pro-2/cpu/temperature from sensor.cpu/temperature;
  end;

where the second sensor should match the original type as provided on the MQTT bus.

So, we could have:
  entity binary_sensor.XXX from switch.YYY;
  entity binary_sensor.ZZZ from sensor.KKK with condition "$ | float > NNN"
making the local "coercion" explicit.

7. Also ... make the "device" declaration act like a space.
So:
      device infrastructural:netatmo from hass.living_room_shower_room with:
        entity sensor.physical:netatmo/co2                  from entity sensor.co2;
        entity sensor.physical:netatmo/humidity             from entity sensor.humidity;
        entity sensor.physical:netatmo/temperature          from entity sensor.temperature;
        entity sensor.infrastructural:netatmo/battery_level from entity sensor.battery_level;
      end;

should become:

      device infrastructural:netatmo from hass.living_room_shower_room with:
        entity sensor.physical:co2                  from entity sensor.co2;
        entity sensor.physical:humidity             from entity sensor.humidity;
        entity sensor.physical:temperature          from entity sensor.temperature;
        entity sensor.infrastructural:battery_level from entity sensor.battery_level;
      end;


8. MQTT (local) broker in container on p-s-1 @JL and frame @VIE

9. Zigbee2MQTT (legacy-to-conceptual passthrough, so entities migrate gradually) first @VIE 

10. Also make the adjustments and battery_alert values something for the device level. Check of discovery messages can handle such functions. 

11. Z-Wave (migrated last -- richest device/capability modelling).
Well. Potentially we can do this one after Zigbee2MQTT @ Vie,
as it contains e.g. blinds as well.

12. Zigbee2MQTT (legacy-to-conceptual passthrough, so entities migrate gradually) first @JL

   1.8. Overkiz-based SOMFY cover control: homeassistant@protocols-server-2 -> MQTT.
      Note (2026-08-28): this is also where 1.1's command-automation half
      (command_<fully_qualified_entity_name>_<command>.yaml, deferred there -- see memory:
      project_entity_existence_inquiry_design) needs its open design question resolved: which
      commands a capability/domain supports, and how one maps to a remote service call on the
      bridged instance. Covers (open/close/stop) are the first real commandable domain due here;
      until then everything bridged is sensor-shaped (read-only). 1.5 (Volvo) may bring the first
      buttons/locks even earlier, if that lands first.
   
         - State reporting: one trigger/action pair per entity, one YAML file per entity --
        filename `reporting_<fully_qualified_entity_name>.yaml`, alias
        `reporting/<fully_qualified_entity_name_with_slashes>`.
      
      - Commands (e.g. light_on): one file per entity per command --
        filename `command_<fully_qualified_entity_name>_<command>.yaml`, alias
        `command/<fully_qualified_entity_name_with_slashes>/<command>`.

        Check if exports of commands would still work.

13. Ensure we have now all the integrations we need.
Then also re-enable the checking of locally (on main HA) assumed entities via the inquiry process. So, using the three values approach, triggering the inquiry of assumed to exist local entities (on main) via the coordinator. 

14. Logical layer + available nuances in relation to "via device"  (netatmo radio module via main module) and aggregation.
- A used or exported (logical or physical) device must always have a _node capability. When such a device has a _battery_level capability, then it must also have a _battery_alert capability. 
- complement device DDD with:
    from DDDx:
      CCCx [as CCCy];
    end;
  end;
  with the usual abbreviations when there is only one.
  Semantics: with this definition at the logical layer, physical device DDD is now complemented with the listed capabilities from the other physical devices. 
  The "as CCCy" indicates an optional rename, if a name clash
  would occur.
  The conceptual layer "sees" the extended version, but this does not influence the device map! So if CCCx/CCCy is "used" to materialise an entity E, then E still "belongs" to physical device DDDx.
- virtual device DDD with:


   - Note (2026-09-06): future step -- massage today's raw "physical:" sphere usage in Spaces.def
     out during the logical layer step itself, not as a separate migration first. The concrete
     case driving this: aggregating several same-kind physical sensors in one room (e.g. multiple
     temperature sensors) has no representation above raw "physical:"-sphere entities today; with
     the logical layer, a device's own capability entity can instead be declared to *collect* into
     a space-level aggregate conceptual entity:
         device infrastructural:netatmo from hass.office_corridor with:
           entity sensor.co2      collect sensor.co2;
           entity sensor.humidity collect sensor.humidity;
         end;
     Here the right-hand `sensor.co2`/`sensor.humidity` are SPACE-level conceptual entities
     (defined once at the enclosing space, not per-device) -- each device capability declared with
     `collect` becomes one of potentially several contributing sources feeding that single
     space-level aggregate, instead of each device getting its own separate physical-sphere entity
     as today. "physical" may well stay part of the eventual naming for these device-level source
     entities (`sensor.physical:...`) even after this lands -- the point of this step is removing
     the raw, ungrouped multiplicity at the conceptual layer, not necessarily the sphere name
     itself.
     Aggregation itself (combining collected values into the one space-level entity) should be
     implemented as a generator-authored YAML template sensor on the MAIN HA instance, not as
     coordinator-side logic -- letting the coordinator do this work would mean re-implementing
     HA's own templating/aggregation engine for no benefit, when the main instance already has it
     for free.

   - Note (2026-09-01): enforce that every device entry, for any integration, provides a node
     definition -- currently only "home_assistant" bridge devices (native + imports) can lack one
     (a "node" capability is optional there; "hosts" devices always get one unconditionally, and
     "discovery" gateway devices have no per-device node concept at all). Decided so far: this
     should be a HARD error (aborts generation), not just a warning -- but not yet implemented,
     since Junglinster's real Physical.def currently has 11 devices without one (hass.envoy + its
     10 inverter sub-devices), which would break generation immediately. Needs more thought before
     building: where to enforce it (Physical.def collection time, so it catches every declared
     device regardless of whether it's ever positioned in Spaces.def, vs. today's
     positioning-time-only check), and what to do about the 11 real devices first.
     Note (2026-09-06): see the "Logical-layer device combining" note below (smaller pending
     items) for a real case (hass.fritz_box/host.fritz_box) suggesting this hard-required-node
     decision may need revisiting once logical devices can "lend" a node capability from a
     sibling physical device -- possibly only logical devices (and exported devices) need the
     hard requirement, not every physical one.

15. Installation-level status binary_sensors (meta sphere): one (discovery-created) binary_sensor per HA
      instance signalling (1) a configuration problem on that instance, and (2) updates available
      for it -- purely passive/informational (dashboard-visible), deliberately decoupled from item
      2's reload/restart meta-command mechanism rather than gating it synchronously (see item 2's
      own 2026-09-05 scope decision). Not designed yet:
   - (1) config-problem signal needs a per-instance automation to actually call `check_config` (or
     equivalent) and publish its own retained status, which the coordinator turns into a discovery
     binary_sensor per installation -- the same "per-instance status becomes a per-installation
     MQTT discovery entity" shape kind-2/kind-3 existence status already uses, just for a different
     underlying fact.
   - (2) updates-available has no single source across install types: HAOS's own `update.*`
     entities (`update.home_assistant_core_update`/`_supervisor_update`/`_os_update`) are the
     natural source where Supervisor is present, but a plain Container install (e.g.
     protocols-server-2, if still bare Container) has no update entity to poll at all -- needs its
     own per-install-type sourcing story before this can be built uniformly.

15b. Check aggregation of sensors. If one is down, what do we do with data?

16. Check old todo/plan part below the "--------" below

17. Code cleaning (dead code, superseded generator logic)

18. Architectural review, code review and documenting
   - Note (2026-09-05): GO_CONVENTIONS.md §7/§8 specifies a three-layer parser architecture
     (character stream -> tokeniser -> recursive-descent, CDL1 bool-returning style, no `error`
     returns from parse functions). The DSL frontend (Physical.def/Spaces.def parsing) does not
     follow this -- it's line-based `regexp.MustCompile`/`FindStringSubmatch` matching throughout,
     confirmed spanning at least 15 non-test files including the generic layer (`Physical_Parser.go`,
     `layers.go`, `expander.go`), not just the per-integration parsers. Flagged by the user ahead of
     the code-review step above; expected to be remedied, but deliberately NOT attempted piecemeal
     inside unrelated feature work (e.g. the 2026-09-05 import-grammar unification, item 4a below,
     stayed in the existing regexp style rather than migrating just the one file it touched) --
     needs its own dedicated pass given the scope (the whole DSL frontend, not one file).

18b: Test if the meta call to reset works.

19. FHEM/FS20 integration

20. Publish

21. CHeck for more of the existing integrations, like fritzbox, ems-esp, etc.

---------------

# Raspberry Pi fleet storage migration strategy

Covers: the storage medium decision for the Pi 3 fleet (2x Junglinster, 1x Vienna), the
original Model B+ ("the old Pi 2"), and the specific plan for migrating `protocols-server-2`
off its microSD + external SSD "bunny hop" onto a single USB flash stick.

## 1. Storage medium decision (Pi 3 fleet)

- **Chosen medium: the official Raspberry Pi USB flash drive** (128GB), one per board.
- **Why not an SSD:** Pi 3B/3B+ only has USB 2.0, and on the 3B+ it's shared internally
  with onboard Ethernet through one controller — realistic throughput tops out around
  35–40MB/s regardless of what's attached. An SSD's main selling point (speed) is wasted;
  it only adds cost, bulk, an enclosure/cable, and potential power-draw issues on the Pi's
  modest 5V rail.
- **Why not plain/cheap microSD:** weakest option for a server-type role — prone to
  corruption from unclean power loss and wear from sustained small writes (logs, journal,
  container state).
- **Why the official RPi stick specifically:** genuinely USB 3.0 internally (wasted on a
  Pi 3's USB2 port, but indicates a decent controller/NAND), advertises resilience to
  power-loss events and SMART health reporting, and draws modest power — good fit for
  boxes that are hard to reach physically (especially the Vienna unit).
- A spare USB3 stick already on hand is a reasonable pilot/test unit before ordering the
  fleet: a USB3-generation drive, even bottlenecked to USB2 speeds, generally still has a
  meaningfully better controller/NAND than genuine USB2-era sticks, because small-file
  random I/O (what actually matters for rootfs duty) is dominated by controller quality,
  not the interface's rated sequential speed.
- A short "does it boot and run" pilot confirms functionality, not long-term endurance —
  wear-related failures typically only show up after months of sustained writes.

## 2. USB-boot capability varies by board — check before assuming

| Board | Native USB boot? | Notes |
|---|---|---|
| Pi 3B / 3B+ | Yes, via one-time OTP bit or EEPROM `BOOT_ORDER` | Confirmed already **enabled** on `protocols-server-2`. |
| Pi 2 (v1.1, BCM2836) | No | Boot ROM predates USB-MSD boot support entirely. |
| Pi 2 (v1.2, BCM2837) | Yes | Same silicon as Pi 3B — check via `cat /proc/cpuinfo` (`Hardware` line: BCM2836 vs BCM2837). |
| Original Model B/B+, Pi Zero (BCM2835, ARMv6) | **No, ever** | No OTP/EEPROM workaround exists — boot ROM code simply doesn't support it. Confirmed this is what "the old Pi 2" actually is (`cat /proc/cpuinfo` → `Hardware: BCM2835`, `Model: Raspberry Pi Model B Plus Rev 1.2`, single core). |

## 3. The old "Pi 2" is actually an original Model B+ — separate plan

- **Role:** FS20 protocol experiments, running one radio. Light workload.
- **OS:** Raspberry Pi OS — but must be the **Legacy, 32-bit** image, not the current
  default. Fedora is not an option at all (Fedora's 32-bit ARM support always required
  ARMv7+, excluding this ARMv6 chip from day one; Fedora dropped 32-bit ARM entirely after
  Fedora 36, and the chip can't run 64-bit Fedora either since it's 32-bit-only silicon).
  Mainline Raspberry Pi OS has also since narrowed to Zero W/2B onward, hence Legacy.
- **Storage:** a regular flash stick for rootfs is fine for this workload.
- **Permanent constraint:** this board can never boot purely from USB — a small SD card
  carrying the boot partition (`bootcode.bin`/`start.elf`/kernel/`cmdline.txt` with
  `root=` pointing at the stick) is required forever, not as a stepping stone. Same shape
  of hybrid as `protocols-server-2`, but for a hardware reason rather than a device/
  enclosure quirk.

## 4. `protocols-server-2`: current setup and diagnosis

**Current boot chain** (confirmed via `df` and `lsusb`):

- microSD `mmcblk0p1` → EFI System Partition (`/boot/efi`, FAT)
- microSD `mmcblk0p2` → `/boot` (kernel, initramfs, GRUB)
- External 1TB Samsung Portable SSD T3 → root filesystem, inside LVM
  (`/dev/mapper/systemVG-LVRoot`)

This is a full **U-Boot → UEFI → GRUB** chain, not the traditional Raspberry Pi
firmware-loads-kernel-directly process — meaning *all* boot-critical pieces (RPi
firmware, U-Boot, ESP, `/boot`) currently live on the SD card; the SSD only ever holds
root.

**Diagnosis so far:**

- USB boot mode is already enabled on this board.
- The SSD previously had its own boot partition; it didn't work, and the space was
  reclaimed for cache. This rules out "never attempted."
- `lsusb -t` shows the SSD bound to `usb-storage`, **not** `uas` — rules out the common
  UAS/boot-ROM-incompatibility explanation.
- Remaining live hypotheses: stale EEPROM/bootloader firmware (fixable via
  `rpi-eeprom-update`, needs a temporary Raspberry Pi OS boot since Fedora doesn't ship
  the tool), or something specific to the SSD/enclosure's own power-up/init timing that a
  bare flash stick simply won't exhibit.
- **Actual data footprint is tiny:** ~31GB used (30G root + 644M `/boot` + 45M ESP) out of
  929G — comfortably fits a 128GB stick with room to spare. Rules out a raw block-level
  `dd` of the SSD (1TB source won't fit on a 128GB destination) — migration must be a
  proper rebuild/copy, not a block clone.

## 5. Migration plan for `protocols-server-2`

### Step 1 — Build and validate a test stick

1. On a spare/other Pi 3, write a fresh Fedora image to a 128GB Raspberry Pi stick with
   the full self-contained layout (ESP + `/boot` + LVM root all on the one device).
2. Grow the root filesystem to fill the stick.
3. Move that stick to `protocols-server-2`'s actual hardware. Remove the microSD **and**
   the SSD entirely. Attempt to boot from the stick alone.

This isolates the real question — can *this specific board* natively boot a bare USB
stick — from anything about the existing SSD/enclosure. If it boots, the SSD's original
problem was device-specific, and the EEPROM-update path can be skipped entirely.

### Step 2 — Do **not** transplant the old root filesystem onto the stick

Initially considered: boot back on the old microSD+SSD pair, mount the new stick, and
`rsync -aAX` the SSD's root across (excluding `/etc/fstab`, `/boot`, `/boot/efi`).
**Rejected in favour of the approach below** — a straight rootfs transplant risks kernel/
initramfs version mismatches, stale BLS boot entries (keyed by machine-id + kernel
version), and GRUB/UUID references tied to the old device layout — all fixable, but fiddly
and easy to get subtly wrong.

### Step 3 — Selective migration onto the clean, already-tested install (preferred)

Keep the stick's own working OS/boot layer untouched. Boot the old pair, mount the stick's
root partition only (not its `/boot`/`/boot/efi`), and copy across just the things that
are actually stateful/customised:

- [ ] Hostname
- [ ] Go toolchain (if manually installed, e.g. `/usr/local/go`)
- [ ] Podman/Quadlet unit files (`/etc/containers/systemd/` or equivalent)
- [ ] Container **persistent data**, not just the unit files (e.g.
      `/var/lib/home-automation/zigbee` — the zigbee2mqtt config/state/database)
- [ ] crontab / systemd timers (`/var/spool/cron/*`, `/etc/cron.d/*`)
- [ ] Network configuration — static IP / NetworkManager profiles (check **before** the
      swap; a fresh image defaulting to DHCP on a box you can't easily reach is the
      classic way this goes wrong)
- [ ] udev rules (`/etc/udev/rules.d/` — e.g. the stable `/dev/zigbee`-style symlink used
      by the container's device passthrough)
- [ ] SSH host keys (`/etc/ssh/ssh_host_*_key*`) — judgement call: keep for continuity, or
      let it regenerate and clear the resulting `known_hosts` warnings elsewhere
- [ ] `/etc/systemd/system/*.d/` overrides, `sysctl.d` tuning
- [ ] **firewalld** — check `firewall-cmd --list-all-zones` on the old box for open
      ports/services (MQTT, zigbee2mqtt frontend, etc.), then re-apply the equivalent
      `firewall-cmd --permanent --add-port=...` / `--add-service=...` commands on the new
      system rather than copying `/etc/firewalld/zones/*.xml` directly — zones can be
      bound to a specific interface name, which may differ on the new install and cause
      the copied rule to silently do nothing
- [ ] SELinux customisations, if any
- [ ] Any additional local users/groups and their `authorized_keys`

**Finding what's actually been customised**, rather than relying on memory:

```bash
rpm -Va                                   # files differing from package defaults
find /etc -newer /etc/os-release -type f  # rough view of what's been touched since install
```

Neither is exhaustive, but together they're a useful cross-check against the list above.

### Step 4 — Cut over

Once the checklist items are copied and verified, boot `protocols-server-2` from the
stick alone. Keep the old microSD + SSD pair as a fallback until the new setup has proven
itself in normal operation for a while.

---------------
Smaller pending items, not gating the above:
- Real bug found live 2026-09-07, worked around not fixed: the "entity <spec> from <device-id>
  entity <capability>;" construct's deferred-retry path (parser.go's pendingCapabilityLinks, for a
  reference reached before its device's own "device <spec> from <device-id>;" positioning) doesn't
  capture administration.SpacePath at defer time -- the retry (after the whole file is read)
  resolves localSpec's intensional "sphere:path" form against whatever SpacePath happens to be
  current then (typically root), not the space the reference line actually sits in. Confirmed live:
  Vienna's "entity switch.social:picture_frame from host.frame entity slideshow;", positioned
  before host.frame's own positioning line, resolved to "switch.social_picture_frame" instead of
  "switch.social_apartment_living_room_picture_frame". Affects every device kind using this
  construct (hosts/hassbridge/imported/commandline alike), not just commandline -- just never
  hit before since every existing real Spaces.def happened to position a device before referencing
  its capabilities. Worked around for Vienna by reordering (the reference now sits after its
  device's positioning); the real fix is capturing SpacePath in pendingCapabilityLink/
  pendingSourceLink and restoring it before each deferred retry call.
- Cross-house import naming decoupling + shorthand grammar + kind-4 existence tracking (2026-09-07):
  built and unit-tested locally (generator + coordinator, `go vet`/`go test -race` clean across all
  three packages), NOT yet deployed or verified live. Three changes, requested together: (1) the
  export side's cloud stable identity is now `exportStableID(qualifier, deviceID, capability)` =
  `"<qualifier>_<deviceID-with-dots-as-underscores>_<capability>"` (discoveryhassbridge.go),
  decoupled from the exporting house's own local entity naming -- previously the cloud `unique_id`
  was derived from the local entity_id, so repositioning a device locally on the exporting side
  silently broke every importer. (2) Physical.def's import grammar now also accepts a shorthand
  form, `<domain>.<capability>;` (domain read for DSL-author readability only, discarded), alongside
  the existing explicit `<capability>: <domain>.<remote-entity-ref>;` form -- the coordinator
  resolves a shorthand capability purely from the topic's own stable-id segment
  (matchImportedCapabilityByStableID, discoveryimport.go), and always republishes it locally under
  the *declared* domain regardless of what domain the exporter actually used (silent coercion, by
  design -- never warned about). (3) Kind-4 existence tracking, mirroring kind-2's own three-state
  model exactly: house_event_bus_coordinator/import_existence.go tracks only shorthand-declared
  capabilities, publishes to `import_existence/<remoteInstallation>/existence/state`;
  homeassistant/mqtt_import_existence.go fetches it generate-time and hard-fails on a confirmed
  known-not-to-exist verdict, wired into Physical_Generator.go right after
  generateImportedDeviceFile. Both houses' `deploy.d/0{2,4}_deploy.coordinator` already exclude the
  new `import_existence.json` runtime-state file from the rsync `--delete`. Remaining: regenerate
  both real houses and diff `coordinator/*.yaml` output before any live deploy; consider migrating
  one real declaration (e.g. Junglinster's own Vienna Netatmo imports) to the shorthand form as live
  verification -- propose to the user rather than doing unilaterally.
- Logical-layer device combining ("aggregate" a new device vs "absorb" into an existing master
  device, e.g. smarty's Zigbee switch into host.smarty) -- gated on steps 3 and 4 both landing.
- Logical-layer availability composition, noted 2026-08-31 once the physical-layer coordinator
  work landed a device's node-topic-gated + self-check availability (`buildAvailabilityFields`,
  discovery.go): that two-factor AND is the right amount of refinement for a single physical-layer
  device, but "aggregate"/"absorb" combine devices whose availability chains can be deeper and
  don't compose by simple ANDing alone. Two concrete cases to design against once step 6 starts:
  (a) a compute host running an integration (e.g. protocols-server-2) has its OWN liveness,
  independent of any device it reports on -- an entity can be "available" per its own device/node
  signals while the host reporting it is itself down; this is the still-open "stacked/dependent
  availability" reminder already flagged in splendid-zooming-lighthouse.md's out-of-scope section.
  (b) Netatmo's IP-connected base modules add a layer of their own: an outdoor/rain/wind module's
  true availability depends on ITS radio link to the base module AND the base module's own IP/WiFi
  link to Netatmo's cloud AND (a) above -- a chain, not a flat set, and today's per-entity "node"
  capability only captures the nearest link in it. Not designed; revisit once step 6 (Logical
  layer) is actually being built, alongside aggregate/absorb themselves.
  Note (2026-09-05), from the cross-house import "node" liveness fix (item 4a): whatever
  availability chain the EXPORTING installation ends up composing internally for a device (its own
  liveness, a base module's link, a radio link, etc.) must stay entirely its own concern -- the
  cloud catalogue it publishes should keep exposing exactly one boolean per device (today's "node"
  capability), never the individual links that fed it. From an IMPORTER's own perspective there are
  only ever two independent availability dependencies to reason about: (1) the remote device's own
  reported available/not-available state (whatever composed that, on the exporter's side), and (2)
  this house's own connection to the cloud broker. Neither should ever surface any of the
  exporting installation's own internal dependency structure into the importer's device map --
  revisit this constraint explicitly once the composed-availability design above is actually built,
  to make sure the exporter-side composition still collapses to one clean signal before crossing
  the cloud-broker boundary.
  Note (2026-09-06), a second concrete case found live: `hass.fritz_box` (a `home_assistant`
  bridge device, reports `sensor.gb_received`, no native node capability of its own) and
  `host.fritz_box` (a `hosts`/ping device, ping-based node) are two separate Physical.def
  declarations describing the exact same real-world router -- the DSL has no way to say so today,
  so `hass.fritz_box` correctly gets flagged as having no node capability, even though the
  physical unit it represents unquestionably has one via its sibling declaration. Interim
  workaround (in use now): fake a node capability on `hass.fritz_box` via the same "is available"
  sugar already used elsewhere (`binary_sensor.node: <already-reported entity> is available;`).
  The real fix is a logical device's ability to "lend" a node capability from whichever
  constituent physical device actually has one, rather than requiring every physical device to
  carry its own -- directly relevant to item 14's own "should every device be hard-required to
  have a node capability" question (revisit that decision once lending exists; it may turn out
  only *logical* devices need the hard requirement, not every physical one). Also matters for
  `export`: an exported device (physical or logical) needs exactly one node capability for the
  cloud catalogue's "one boolean per device" contract to hold, but a physical device on its own
  may have none and would need to borrow one from a sibling the same way -- e.g. a `hosts`-kind
  ping device lending its liveness to a `home_assistant`-bridge device representing the same
  physical unit.
- DONE (2026-09-01): the raw/extensional "domain.[raw-id]" Spaces.def entity-reference syntax is
  removed -- no real Spaces.def/Physical.def in either house still used it (confirmed by audit),
  so `TEntityIdentity.IsRaw`/`RawName`, the bracket-detection branch in `extractEntityIdentity`
  (expander.go), the bracket-passthrough regex in `toHomeAssistantEntityID` (defined.go), and the
  three generator.go/administration.go/would_define.go call sites that special-cased it were all
  deleted outright, not just deprecated. `sun/solar_elevation` (still declared bare in both
  houses' Spaces.def, sourced from a hand-authored HA template outside generator control) remains
  the one real case that would need this syntax if the DSL ever took it over -- unaddressed, not
  gating anything.
- Future refinement (2026-08-23, not yet designed): replace the `.def` files with an integrated
  database representing the coordinator+transformer's actual current understanding of the
  conceptual and physical layers -- physical-layer content populated/kept live from what the
  integrations themselves discover (MQTT discovery, device_attr(), live-reported metadata),
  rather than hand-declared and re-generated from static text each `./configure` run -- while
  still allowing reconfiguration of key components (e.g. which `home_assistant <qualifier>`
  instances exist) through it. Large, cross-cutting shift from the current
  Definitions-directory-as-source-of-truth model; needs its own design pass once the current MQTT
  migration (steps 1-4 above) has landed and the shape of "what the coordinator already knows
  live" is clearer. Reinforced by 2026-08-24's protocols-server-2 rollout: compile-time
  (generator-authored automations, `.def`-driven expected-entity lists) and run-time (coordinator's
  live MQTT state) are still two separate worlds today, stitched together only by redeploys and
  manual "reload YAML" steps on each HA instance -- a merged compile-time/run-time coordinator,
  backed by that same database and possibly its own web UI, would remove that whole class of
  "did you redeploy/reload yet" friction rather than just papering over it with better tooling.
- Rename the `homeassistant_instances/...` MQTT topic tree (bootstrap/bridge protocol,
  discoveryhassbridge.go) to `homeassistant.instances/...`, matching this
  project's own dot-separated discovery-prefix convention (`homeassistant.physical`/
  `homeassistant.conceptual`) -- purely a naming inconsistency introduced 2026-08-23, not yet
  fixed. Safe/mechanical now that both the coordinator code and every instance's automations are
  generator-owned; just needs a coordinated regenerate+redeploy across every instance at once.
- Volvo integration's cloud auth is broken on the "main" (Junglinster) HA incarnation (401
  Unauthorized, confirmed 2026-08-24 via the assumed-entities warning system flagging 18 missing
  `sensor.social_cars_xc40_*`/etc. entities) -- needs re-authenticating in HA's own UI; not a code
  issue.
- Deploy-script lessons from protocols-server-2's 2026-08-23/24 rollout, worth remembering before
  Vienna gets its own deploy.d scripts: (1) a symlink created via `ssh ... ln -sf <target> <dir>/`
  must use a target relative to the link's own location, not the host's absolute path -- the link
  is later read from *inside* the HA container, where the host's absolute path doesn't exist, and
  HA's own error reporting mislabels the resulting failure as "File not found:
  /config/configuration.yaml" regardless of which file actually broke, which cost a long debugging
  session to trace. (2) current Home Assistant's `mqtt.publish` service no longer accepts
  `payload_template` (schema-validation error "extra keys not allowed") -- template the `payload:`
  field directly instead; both are now fixed in `homeassistant/remote_instance_automations.go` and
  `deploy.d/0{1,2}_deploy.hass.*`.
- `isCoordinatorOwnedTopic`'s legacy `host_`/`discovery_` topic-shape branch
  (house_event_bus_coordinator/discoverycleanup.go) exists only to catch pre-migration discovery
  topics during the one-time cutover to identity-based topic keys (`.../coordinator/<stable-id>/config`).
  Once each deployed house's coordinator has run once and its retirement log confirms the old
  topics are gone -- Junglinster first, Vienna once it's actually deployed -- remove that branch;
  it'll be permanently dead code by then.

---

See below for the full narrative behind each TODO item above; Architecture.md for the target
design (canonical event bus, coordinator, three modelling layers); README.md for current
DSL/usage reference. As each TODO item is finished, its write-up below should be trimmed down to
a one-line DONE note (or removed, if nothing durable needs keeping beyond what's already in
code/README.md/Architecture.md) -- the goal is for this section to end up essentially empty once
Phase 3 is done, and for the TODO list above to stay the only thing anyone needs to read at a
glance.

** Roadmap detail

Phase 1 — Infrastructure: DONE
  protocols-server-1 exists; Podman/Quadlets working; Z-Wave JS UI running and validated.

  Z-Wave migration specifics (in progress):
  - New environment: Home Assistant Green; a Raspberry Pi running Podman; Z-Wave JS UI;
    Aeotec ZWA-2 controller; two Aeotec ZW117 range extenders.
  - Mesh validation: a Fibaro module was excluded/included/excluded/included successfully
    while physically remaining on the second floor, through Basement (ZWA-2) -> 1st floor
    repeater -> 2nd floor repeater -> device -- confirms the temporary mesh is already
    functional enough to migrate onto.
  - Node ID behaviour: observed monotonically increasing allocation (first inclusion ->
    Node ID 4, second -> Node ID 5) rather than immediate reuse of released IDs. Not fully
    validated; migration plan deliberately avoids depending on Node ID preservation.
    Possible allocation strategies to watch for: first unused ID, highest known ID + 1,
    highest ever assigned ID + 1.
  - Migration order: keep the old Gen5 mesh operational -> build an initial new backbone
    from the ZW117 repeaters -> migrate remote mains-powered devices first -> let migrated
    Fibaro modules become new repeaters, strengthening the mesh as migration proceeds ->
    migrate controller-near devices last. Keeps RF coverage acceptable throughout and
    doesn't require every device to be physically reachable from the new controller
    during migration.

Phase 2 — Stabilise the existing DSL/generator
  Test Vienna, test Junglinster, stabilise the compiler against both houses' live
  configuration. The initial bulk pass (2026-08-09 -- 08-18) was a genuine prerequisite
  for starting Phase 3 -- designing the new architecture against infrastructure that
  was still mid-change would have meant designing against a moving target. Beyond that
  bulk pass, Phase 2 continues in parallel with Phase 3 rather than as a separate
  blocking gate: each integration tackled in Phase 3 (ping/cpu, Netatmo, EMS, Zigbee2MQTT, Z-Wave) surfaces and stabilises its own corner of the existing
  DSL/generator as it's iterated on, so the two phases run together per-integration
  from here on.

Phase 3 — New architecture, incremental learning steps
  Each step tests one new architectural capability before the next is attempted, and
  grows the coordinator's aggregation vocabulary gradually rather than all at once.

  Timeline note (2026-08-18): travelling over the next few weeks, so the bridge role
  (federation across MQTT brokers, §5/§6.5 in Architecture.md) will not be touched
  until September. Physical-level DSL preparation therefore focuses first and only on
  step 0 (ping + cpu) below, since that step needs no cross-broker bridging at all.
  Update (2026-08-22): step 0 is done, delivered while still travelling -- confirms the
  no-cross-broker-bridging framing held.
  Update (2026-08-23): steps 1 and 2 swapped -- protocols-server-2/Netatmo work now takes
  priority over the EMS heater (see the "** TODO" list at the top of this file for the current,
  detailed breakdown). Current focus is step 1's first sub-step, weather forecast bridging.

  0. Ping + CPU integration. DONE (2026-08-22). Treated as one conceptual integration
     with two data-collection scripts (a pinger; an OS-dependent CPU load/temperature
     collector). Every cpu device implies a corresponding ping device. The actual DSL
     shape that shipped differs from the original sketch below (an "integration <name>
     [on <host>] with devices: ..." / Integrations.def syntax was never built) -- what
     exists instead, and is now documented in README.md's "Devices (the hosts
     integration)" and "Lists.def syntax" sections:
     - Physical.def's "integration hosts with: device <id> <host> <type> with: ...;
       end; end;" declares each host's type (cpu/home_assistant/ping), capability
       entity mappings (optionally grouped, e.g. "cpu/load: sensor.processor_use;"),
       and constant device-map overrides ("model: \"...\" [forced];").
     - Spaces.def's "entity device.<spec> from <device-id> with:
       variable_device_attributes;" positions a device in the conceptual space tree,
       implying discovery-implied node/attribute entities -- created by the
       coordinator's own HA MQTT Discovery publish, not generator-authored YAML.
     - "space <spec> as area with:" marks a space as an HA Area feeding suggested_area.
     - house_event_bus_coordinator/ is no longer a stub: connects to the MQTT broker,
       publishes discovery configs (retained), subscribes to live-reported device
       metadata, merges it with Physical.def's static/forced constants (forced-DSL >
       live > default-DSL precedence), and republishes a device's discovery config
       live when new data arrives -- confirmed working end to end. Deployed as a
       systemd service on protocols-server-1 (Junglinster), staged through xanadu.
     - Integrations/cpu/report reports CPU load normalized to a percentage of core
       count (not raw Unix load average) plus device metadata, cross-platform.
     - Space-level aggregates (temperature, humidity, ...) and generator-authored
       customization files both exclude infrastructural-sphere / discovery-implied
       entities, so a device's cpu/temperature never gets folded into the room it
       physically sits in, and never gets a redundant customization file duplicating
       what the coordinator's own discovery payload already sets.

     Still open, deferred to step 1 (Netatmo) or later: per-host metadata aggregation
     with other integrations at the same physical location (e.g. combining a netatmo
     device's readings with the host device it shares a room with) would need a
     "complement" operator, not just aggregation -- not designed yet.

  1. Netatmo and other cloud services, via the secondary HA instance (protocols-server-2,
     integration-adapter role) -- DONE, see the "** TODO" list's 1.2a-1.2d above for the
     actual mechanism (MQTT-based hassbridge export/import, not the REST bridge originally
     sketched here). 1.3a (below) removed the REST bridge mechanism entirely, now that
     nothing depends on it: `Bridges.def` and its `${junglinster_instance}`/
     `${junglinster_api_token}` Settings.def variables deleted from Vienna's real
     Definitions/ (2026-08-31), the only house that ever used them.

  2. EMS heater device. Not started -- design/syntax not yet worked out. Control of picture frame
     via MQTT (the commandline integration) is DONE -- see the "** TODO" list's item 1 above.

  3. Zigbee2MQTT. Exercises the legacy-to-conceptual passthrough capability described in
     Architecture.md §6.4, so entities can be switched over to the new architecture
     gradually rather than in one cutover.

  4. Z-Wave. Migrated last, since it needs the most native-API-aware handling and the
     richest device/capability modelling.

Phase 4 — Full platform migration & cleanup
  Once a platform has fully moved through Phase 3's incremental steps, retire the
  now-superseded generator logic for it from the existing app rather than carrying both
  paths indefinitely.

Phase 5 — Publish (not started)
  Architecture.md should, at some stage, be ready for others to read, alongside an
  up-to-date README.md, on GitHub. Explicitly deferred until the architecture above has
  actually been built and proven out, not before.
