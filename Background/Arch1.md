Home Automation Architecture Notes
**Date:** 2026-08-05

## Context

The migration from a Raspberry Pi 4 to a Home Assistant Green has triggered a broader rethink of the home automation architecture.

The objective is no longer simply to migrate hardware, but to evolve towards a modular architecture where:

- Home Assistant focuses on automation and user interaction.
- Radio protocols are owned by dedicated services.
- Legacy systems remain available where they provide value.
- Cloud services are isolated from the core automation platform.
- MQTT serves as a generic integration layer where appropriate.

---

# Current Migration Status

## New Z-Wave Environment

The new Z-Wave network currently consists of:

- Home Assistant Green
- Raspberry Pi running Docker
- Z-Wave JS UI
- Aeotec ZWA-2 controller
- Two Aeotec ZW117 Range Extenders

Validated:

- Docker installation works.
- Docker restart policy is `unless-stopped`.
- Pi reboot automatically restores Z-Wave JS UI.
- Home Assistant reconnects automatically.
- Z-Wave JS UI communicates correctly with the ZWA-2.

---

# Mesh Validation

A realistic RF test was performed.

```
Basement
    |
 ZWA-2
    |
1st floor
    |
ZW117 repeater
    |
2nd floor
    |
ZW117 repeater
    |
Test Fibaro module
```

The test Fibaro successfully performed:

1. Exclusion
2. Inclusion
3. Exclusion
4. Inclusion

while remaining installed on the second floor.

This demonstrates that the temporary mesh is already sufficiently functional for migration.

---

# Node ID Behaviour

Observed:

```
First inclusion  -> Node ID 4
Second inclusion -> Node ID 5
```

The controller appears to allocate monotonically increasing Node IDs rather than immediately reusing released IDs.

This still needs further validation but is useful when planning the migration and maintaining external administration.

---

# Planned Migration Strategy

Current idea:

1. Keep the old Gen5 network operational.
2. Build an initial backbone using the ZW117 repeaters.
3. Migrate remote mains-powered devices first.
4. Allow migrated Fibaro modules to become the new repeaters.
5. Migrate controller-near devices afterwards.

Advantages:

- The new mesh continuously improves.
- The old mesh remains usable long enough.
- RF coverage should remain acceptable during migration.

---

# Long-Term Architecture

## Separation of Responsibilities

Rather than one large Home Assistant installation owning everything, responsibilities become separated.

```
                    Home Assistant Green
                  ------------------------
                  Automation
                  Dashboards
                  User Interface

                           |

        -----------------------------------------

        |                    |                  |

     Radio Pi             FHEM Pi          Other services
```

---

# Radio Pi

The Radio Pi becomes responsible for hardware-oriented protocols.

Possible services:

- Z-Wave JS UI
- Zigbee2MQTT
- Bluetooth
- RTL_433
- other radio services

The physical radios remain independent from Home Assistant itself.

---

# FHEM

FHEM remains valuable despite its age.

Its role changes from "automation platform" to "integration platform".

Responsibilities may include:

- FS20
- legacy hardware
- cloud services
- experiments
- protocol conversion
- MQTT publishing

Conceptually:

```
Hardware / Cloud

        |

      FHEM

        |

      MQTT

        |

Home Assistant
```

This allows Home Assistant to consume normalised information without needing to understand every vendor protocol.

---

# MQTT versus Native APIs

The discussion reinforced that there is no single integration mechanism suitable for everything.

## Z-Wave

Preferred:

```
Home Assistant
      |
WebSocket
      |
Z-Wave JS UI
```

Reasons:

- network management
- inclusion/exclusion
- interviews
- firmware updates
- configuration parameters
- associations

MQTT would lose much of this richness.

---

## Zigbee

Zigbee2MQTT deliberately exposes MQTT.

This provides excellent interoperability with:

- Home Assistant
- FHEM
- Node-RED
- custom software

MQTT is therefore appropriate here.

---

## Matter

Matter currently belongs much closer to Home Assistant.

Unlike Z-Wave, it is not simply another radio protocol.

Expected direction:

```
Matter Devices

        |

Home Assistant
```

rather than inserting MQTT in between.

---

# Cloud Services

## Netatmo

Current issue:

- one Netatmo account
- two houses
- concurrent logins are problematic

Current workaround:

REST retrieval from one location.

Possible future solution:

```
Netatmo

    |

Gateway (FHEM or dedicated service)

    |

MQTT

    |

Both Home Assistant instances
```

Only one service authenticates to Netatmo.

Everything else consumes MQTT.

The same concept can later be reused for other awkward cloud services.

---

# Multi-House Design

Current houses:

- Junglinster
- Vienna

Each house should remain autonomous.

Only selected information should be exchanged.

```
Junglinster

     |

 MQTT Bridge

     |

Vienna
```

Examples of shared information:

- weather
- energy
- security state
- occupancy (if desired)

The bridge should exchange semantic information rather than implementation details.

---

# Vienna

Possible future architecture:

```
Home Assistant Green

    |

Automation

---------------------------

Pi4

- Photo frame
- MQTT bridge
- Zigbee (future)
- Other lightweight services
```

The Pi4 becomes a small infrastructure server rather than simply a display.

---

# Docker

Running services in Docker provides significant flexibility.

Advantages:

- easy migration between Pis
- easy backup
- reproducible deployments
- service isolation

Moving Z-Wave JS UI to another Pi should largely consist of:

- copying the compose directory
- copying the persistent storage
- plugging in the ZWA-2
- starting Docker

---

# General Design Principles

The discussion converged towards several architectural principles.

## 1. Separate concerns

Different systems should own different responsibilities.

Examples:

- Home Assistant → automation
- Radio server → hardware protocols
- FHEM → legacy and cloud integration

---

## 2. Use native APIs where they are strongest

Examples:

- Z-Wave JS → WebSocket
- ESPHome → native API
- Matter → native integration

Avoid replacing rich APIs with MQTT if functionality would be lost.

---

## 3. Use MQTT as a semantic integration layer

MQTT works best for:

- interoperability
- decoupling
- gateways
- cloud integrations
- legacy systems

---

## 4. Let each platform do what it does best

Home Assistant should not necessarily own every protocol.

Likewise, FHEM should not necessarily own automation.

Each platform can specialise.

---

# Overall Vision

```
                    Home Assistant Green
               (automation and dashboards)

                         |

         +---------------+---------------+

         |                               |

    WebSocket                         MQTT

         |                               |

   Z-Wave JS UI                     FHEM Gateway

         |                               |

      Z-Wave               FS20 / Netatmo / Legacy

                         |

                    MQTT Bridge

                         |

                Second Home Assistant
                     (Vienna)
```

The architecture evolves from a monolithic Home Assistant installation towards a collection of specialised services communicating through well-defined interfaces.

This should improve maintainability, portability and long-term flexibility while allowing each technology to be used in the way it was designed.
