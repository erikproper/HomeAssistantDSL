# FHEM, radio signals and the "governed" integration (PROJECT.md 37)

Started 2026-11-08 from Erik's discussion (recorded in PROJECT.md 37). Planning.
Decided (Erik, 2026-11-08): the integration is called **governed** (it was "indirect" in the
discussion); sending a signal is written "on by doing <button>", so it reads as an action.

## The distinction

1. **Devices with a reliable two-way connection** (a real light or switch that reports its own
   state). FHEM's syntax for these differs per platform behind it, so FHEM gets sub-integrations,
   in the shape of the Shelly layer: a statement inside a device implies its topics and metadata
   from a family table (claude-notes/plans/shelly-layer.md).
2. **Devices we only reach by radio** -- FS20 first, but also IR and Bluetooth -- with at most an
   indirect report of their state, from some other device. For these:
   - an **outgoing** radio signal is a **button** capability with a value (on, off, 50%);
   - an **incoming** radio signal is an **event** capability;
   - a new **governed integration** (Physical layer) defines the device itself: its metadata, and
     its state, controlled through those buttons and, where possible, corrected from sensors or
     events. The buttons, sensors and events may come from any integration.

## FS20 facts (FHEM's own module, 10_FS20.pm, read 2026-11-08)

- **Actuators send nothing back.** FHEM only ever transmits in `FS20_Set`; there is no
  acknowledgement or status handling, and the module notes "The FS20ST ... does not respond to
  sendstate". The FHEM wiki: FS20 has no return channel ("fehlender Rückkanal"). Switching an
  actuator by hand on the device is not seen at all.
- **Transmitters are seen.** A remote or wall switch (S4, S20, ...) sends the same frames as FHEM
  does; `FS20_Parse` updates the state of every defined device with the same housecode and
  button. So FHEM sees a *transition someone asked for* on the radio -- not whether the actuator
  received it.
- **Dim levels**: 16 steps, `dim06%` .. `dim100%` (6, 12, 18, 25, 31, 37, 43, 50, 56, 62, 68, 75,
  81, 87, 93, 100), `off` for 0, `on` back to the previous level. FHEM itself does not round:
  `set <dev> dim 40` fails, only the 16 values are accepted. Also `toggle`, `dimup`, `dimdown`,
  `on-for-timer`/`off-for-timer <s>` (rounded up to one of 112 durations, at most 15360 s).
- **Duty cycle**: 868 MHz is a 1 % band; a CUL has `credit10ms`, and refuses to send ("LOVF") when
  it runs out. One frame takes about 0.22 s.

## The governed integration

- **Optimistic state**: the device's state is the one last asked for.
- **Enforced every X**: that assumed state is sent again, through its buttons, every X.
- **State confirmation**: a sensor or event of another device that observes the real state (e.g.
  a plug's power). It updates the governed device's internal shadow state; a discrepancy with the
  desired state leads at once to enforcing the desired state.
- **Its own application**, next to the coordinator: it is operational, not coordinative (PROJECT.md
  37's 2026-09-22 decision), so it is held to a higher reliability bar and runs in its own
  container. The coordinator can be down without it noticing, and the other way around.

- **A transition seen on the radio** (a remote or wall switch sending to the same address, as FHEM
  reports it) changes the *desired* state: someone asked for it. It is not a confirmation that the
  device follows, and must not be enforced back to the old state.

## Example, as Erik describes it

The FHEM integration defines a button that makes every known FHEM installation listen on its topic
and send the FS20 signal; the governed integration builds a switch on it. A sketch only -- every
keyword here is open:

```
integration fhem with:
  device radio.living_room_lamp with:      # an FS20 address, nothing more
    fs20 "1234" "01";                       # house code, device address
    capability button.on with value "on";
    capability button.off with value "off";
  end;
end;

integration governed with:
  device lights.living_room_lamp with:
    manufacturer: "ELV";
    model:        "FS20 ST-4";
    capability switch.switch
      on by doing button.on from radio.living_room_lamp
      off by doing button.off from radio.living_room_lamp
      enforced every 00:30:00
      confirmed by sensor.power from switches.living_room_lamp_plug above 5;
  end;
end;
```

Home Assistant then sees one ordinary switch, `switch.<position>`, whose state is the governed
application's -- not the buttons.

## To reflect on (Erik)

- **Throttle groups per radio**: the governed application limits how many requests it sends to one radio.
  For FS20 this is also a legal limit: 868 MHz is a 1 % duty-cycle band, which FHEM's CUL tracks as
  credit. A group would be per radio (per CUL stick), shared by every device sent through it, and
  enforcement must queue behind it rather than flood it.

## Open questions

1. (decided: "governed")
2. (decided, Erik 2026-11-08: a value as parameter, not a button per value) A sending capability
   takes the value; the FS20 layer maps a percentage to the nearest of its 16 dim levels (0: off),
   as Erik's old implementation did in FHEM (its scripts: $HOME/SmartLiving/Old, once synced to
   this Mac -- check their rounding against the 16 levels). Still open: its keyword and how it shows in Home
   Assistant, if at all.
3. How the governed application gets its definitions: from the generator, as the coordinator does
   (a YAML file it reads), and how Home Assistant talks to it (MQTT, as for every other kind).
4. Confirmation from an event rather than a sensor: which event types mean which state.
5. Where the shadow state lives across a restart of the application (retained on the broker, as
   the coordinator's shadows are?).
6. FHEM itself: how it is reached (FHEM's own MQTT, or a small relay), and the ESPHome question in
   item 37 (radios on ESPHome may change how FS20 is reached at all).
7. What "known FHEM installations" means: several FHEM instances (houses, sticks) listening on one
   topic, each sending -- and so how throttle groups and duplicates work across them.
