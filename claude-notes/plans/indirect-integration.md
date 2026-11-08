# FHEM, radio signals and the "indirect" integration (PROJECT.md 37)

Started 2026-11-08 from Erik's discussion (recorded in PROJECT.md 37). Planning; nothing decided
beyond what that discussion says. "Indirect" is a working name.

## The distinction

1. **Devices with a reliable two-way connection** (a real light or switch that reports its own
   state). FHEM's syntax for these differs per platform behind it, so FHEM gets sub-integrations,
   in the shape of the Shelly layer: a statement inside a device implies its topics and metadata
   from a family table (claude-notes/plans/shelly-layer.md).
2. **Devices we only reach by radio** -- FS20 first, but also IR and Bluetooth -- with at most an
   indirect report of their state, from some other device. For these:
   - an **outgoing** radio signal is a **button** capability with a value (on, off, 50%);
   - an **incoming** radio signal is an **event** capability;
   - a new **indirect integration** (Physical layer) defines the device itself: its metadata, and
     its state, controlled through those buttons and, where possible, corrected from sensors or
     events. The buttons, sensors and events may come from any integration.

## The indirect integration

- **Optimistic state**: the device's state is the one last asked for.
- **Enforced every X**: that assumed state is sent again, through its buttons, every X.
- **State confirmation**: a sensor or event of another device that observes the real state (e.g.
  a plug's power). It updates the indirect device's internal shadow state; a discrepancy with the
  desired state leads at once to enforcing the desired state.
- **Its own application**, next to the coordinator: it is operational, not coordinative (PROJECT.md
  37's 2026-09-22 decision), so it is held to a higher reliability bar and runs in its own
  container. The coordinator can be down without it noticing, and the other way around.

## Example, as Erik describes it

The FHEM integration defines a button that makes every known FHEM installation listen on its topic
and send the FS20 signal; the indirect integration builds a switch on it. A sketch only -- every
keyword here is open:

```
integration fhem with:
  device radio.living_room_lamp with:      # an FS20 address, nothing more
    fs20 "1234" "01";                       # house code, device address
    capability button.on with value "on";
    capability button.off with value "off";
  end;
end;

integration indirect with:
  device lights.living_room_lamp with:
    manufacturer: "ELV";
    model:        "FS20 ST-4";
    capability switch.switch
      on by button.on from radio.living_room_lamp
      off by button.off from radio.living_room_lamp
      enforced every 00:30:00
      confirmed by sensor.power from switches.living_room_lamp_plug above 5;
  end;
end;
```

Home Assistant then sees one ordinary switch, `switch.<position>`, whose state is the indirect
application's -- not the buttons.

## To reflect on (Erik)

- **Throttle groups per radio**: the application limits how many requests it sends to one radio.
  For FS20 this is also a legal limit: 868 MHz is a 1 % duty-cycle band, which FHEM's CUL tracks as
  credit. A group would be per radio (per CUL stick), shared by every device sent through it, and
  enforcement must queue behind it rather than flood it.

## Open questions

1. A name for "indirect".
2. A button "with a value": one button per value (`button.on`, `button.off`, `button.dim_50`), or
   one sending capability that takes the value as a parameter? Home Assistant's own buttons take
   none.
3. How the indirect application gets its definitions: from the generator, as the coordinator does
   (a YAML file it reads), and how Home Assistant talks to it (MQTT, as for every other kind).
4. Confirmation from an event rather than a sensor: which event types mean which state.
5. Where the shadow state lives across a restart of the application (retained on the broker, as
   the coordinator's shadows are?).
6. FHEM itself: how it is reached (FHEM's own MQTT, or a small relay), and the ESPHome question in
   item 37 (radios on ESPHome may change how FS20 is reached at all).
7. What "known FHEM installations" means: several FHEM instances (houses, sticks) listening on one
   topic, each sending -- and so how throttle groups and duplicates work across them.
