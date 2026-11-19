/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Documentation (the architecture chapter)
 *
 * The handover document's chapter on the architecture every house shares: its parts, the gateways
 * and integration kinds, how the parts are deployed, and the definitions and the generator: what
 * they are and how to install and use the generator. The same for every house, and in line with
 * Architecture.md; the generator's inner workings are left to the repository.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Vienna, Austria, in collaboration with Claude.ai
 *
 * Version of: 19.11.2026
 *
 */

package main

import (
	"fmt"
	"strings"
)

// TDocumentationItem is one entry of a described list: its name and what it is.
type TDocumentationItem struct {
	Name        string
	Description string
}

// documentationRepository is where the code and the architecture's own description live.
const documentationRepository = "https://github.com/erikproper/HomeAssistantDSL"

// documentationGateways are the gateways known so far: the programs that connect a radio, a bus or
// a cloud service to the message bus.
var documentationGateways = []TDocumentationItem{
	{"Zigbee2MQTT", "the Zigbee radio: lights, plugs, wall switches, buttons, and sensors (doors, windows, motion, water, temperature)."},
	{"Z-Wave JS UI", "the Z-Wave radio: switches, dimmers, thermostats, and sensors."},
	{"EMS-ESP", "the heating's EMS bus: the boiler's temperatures, its state, and its settings."},
	{"FHEM", "older radios that Home Assistant no longer serves: FS20 (through a CUL or FHZ stick) and DuoFern covers."},
	{"Home Assistant \\enquote{ha2mqtt}", "a second Home Assistant instance for cloud and network services: weather stations (Netatmo), solar panels (Enphase Envoy), covers (Somfy), cars (Volvo), the router (FRITZ!), household appliances, printers, and the like."},
	{"mqtt\\_commandline", "scripts on a machine, shown as switches, buttons, and sensors -- for instance a picture frame's slideshow."},
	{"cpu and ping", "small reporters for the machines themselves: whether they are up, their load, and their temperature (see \\ref{sec:architecture-cpu-ping})."},
	{"Devices on MQTT directly", "devices with MQTT of their own, such as Shelly plugs or an ESPHome gateway to the electricity meter."},
}

// documentationIntegrationKinds are the kinds of integration, by how much of their MQTT side is the
// house's own (Architecture.md, "The five integration kinds"), plus the governed devices.
var documentationIntegrationKinds = []TDocumentationItem{
	{"Discovered", "the gateway announces its devices itself (MQTT discovery: Zigbee2MQTT, Z-Wave JS UI, EMS-ESP). The coordinator collects these announcements, and the definitions say which of the devices are used, and where."},
	{"Controlled", "the definitions alone say which devices exist, and the house's own programs publish them: the machines (cpu and ping), scripts (mqtt\\_commandline), and FHEM."},
	{"Bridged", "the devices live in another Home Assistant instance (ha2mqtt); automations there put their states on the bus, and commands go back the same way."},
	{"Local", "the main instance talks to the device itself, without the bus: media players, cameras, the weather forecast, a phone."},
	{"Raw", "a device with MQTT of its own but no announcements; the definitions describe its topics."},
	{"Governed", "devices that don't report their state (FS20): the governor keeps them in the state last asked for."},
}

// documentationArchitecture is the architecture chapter's text.
func documentationArchitecture(d *TDocumentation) string {
	var b strings.Builder
	b.WriteString(`
This chapter describes the architecture that every house shares: its parts, how they are
deployed, and how the house is changed; the next chapter describes how this house puts it to
work. The code, and a fuller description of the architecture, are at
\url{` + documentationRepository + `}.

\section{The parts}

\begin{description}
    \item[The main instance:] The Home Assistant instance the residents use: its dashboards, its
        automations, its app. It talks to no radio itself; it gets almost everything from the
        message bus, and sends its commands back over it.
    \item[The message bus:] Each house has an MQTT broker (Mosquitto). Every other part connects
        to it, and they talk to each other only through it; so a part can be replaced, moved to
        another machine, or restarted without the others noticing more than a short gap.
    \item[The bridge between sites:] A broker in the cloud connects the houses. Through it, a house
        shares selected devices with the other (for instance the weather station of the one house
        in the other), and machines that travel (laptops) report wherever they are. No house
        depends on it: without it, each house keeps working, only without what the other shares.
    \item[The coordinator:] A program next to the broker that knows the definitions. It puts the
        gateways' announcements in the form the definitions say, keeps track of what is reachable
        (when a gateway is down, its devices are shown as unavailable), keeps the last known state
        of every device on the bus, and carries the shared devices to and from the other house.
    \item[The governor:] A program for devices that never say what state they are in (FS20 radio
        switches and dimmers). It remembers the state last asked for, sends it again every so
        often, and keeps within the radio's legal air-time budget (1\,\% of the time).
    \item[The gateways:] The programs that connect a radio, a bus, or a cloud service to the
        message bus. Those known so far:
`)
	b.WriteString(documentationDescribedList(documentationGateways, "        "))
	b.WriteString(`\end{description}

\section{Kinds of integration}

How a device reaches the main instance depends on how much of its MQTT side is the house's own:
`)
	b.WriteString(documentationDescribedList(documentationIntegrationKinds, ""))
	b.WriteString(`Whatever the kind, the main instance sees the same kind of entities, named after where they
are in the house rather than after the radio or service behind them. Replacing a device -- a plug
by another make, a Zigbee sensor by a Z-Wave one -- keeps its name and its history.

\section{Deployment}

\begin{description}
    \item[The main instance] runs on a dedicated machine with Home Assistant OS (for instance a
        Home Assistant Green), and nothing else: no broker, no radios. Home Assistant OS is updated
        from its own settings, and makes its own backups; the machine can be replaced and restored
        from such a backup.
    \item[The other key parts] run on the house's hub, a Linux machine named
        \path{protocols-server-1} (machines are named after their role): the broker, Zigbee2MQTT,
        Z-Wave JS UI, and ha2mqtt each in a container (Podman), with the radio sticks plugged into
        that machine; the coordinator and the governor as services of the machine itself. A script
        (\path{update.containers}) updates the containers; every day, each container's data is
        backed up on the hub and copied to a backup machine.
    \item[FHEM] runs natively (not in a container) on small machines of its own, near the radios it
        serves.
\end{description}

\subsection{cpu and ping}\label{sec:architecture-cpu-ping}

Every machine that can run a script reports itself regularly (\path{cpu/report}): its load,
its temperature, and what it is. Devices that can't (televisions, speakers, gateways, the router)
are pinged from the hub instead (\path{ping/report}). Either way, each has a \enquote{node} entity
saying whether it is reachable, and what depends on it follows: when a gateway or its machine is
down, its devices show as unavailable rather than as wrong values.

\section{The definitions and the generator}

\subsection{The house is modelled, not programmed}

The source of truth about a house is a set of definitions: which devices it has, where each of
them is, what they are called, and which rules connect them. A program, the generator, turns them
into the configuration of every part: Home Assistant's entities, automations and dashboard lists,
the coordinator's, the gateways' and the machines' own. A change made by hand in Home Assistant's
own settings -- an entity's name, an automation -- is therefore lost at the next deployment; the
change belongs in the definitions.

The definitions describe a house in three layers:
\begin{description}
    \item[Physical:] the devices as they are, with the gateway or service each of them comes from,
        and the machines, with how they are reached.
    \item[Logical:] devices composed from others, and properties derived from them (for instance
        \enquote{consumes power} from a plug's power reading).
    \item[Conceptual:] the house as its residents see it: its spaces (house, floors, rooms), the
        entities in them, and the rules (\enquote{when this button is pressed, toggle that light}).
\end{description}
Each house has a folder of its own with its definitions (\path{Definitions}), two scripts
(\path{generate} and \path{deploy}), what the last generation wrote, and this document
(\path{Documentation}); definitions the houses share are in a folder next to them
(\path{Shared}). The next chapter says where this house's folder is.

\subsection{Installing the generator}\label{sec:architecture-generator}

The generator, the coordinator, and the governor are one code base, written in Go, at
\url{` + documentationRepository + `}; its \path{README.md} and
\path{Architecture.md} describe the definitions' language and the generator's workings in full.
Generating and deploying need a Mac (or another Unix machine) with:
\begin{itemize}
    \item Go (\url{https://go.dev}) and git;
    \item the code, cloned to \path{$HOME/go/src/homeassistant}: deploying builds the coordinator
        and the governor from there;
    \item the generator itself, installed from it: \path{cd $HOME/go/src/homeassistant/homeassistant}
        and \path{go install .}, which puts the program \path{homeassistant} in \path{$HOME/go/bin}
        (to be on the shell's \path{PATH});
    \item for deploying, SSH access to the house's machines (appendix~\ref{chap:passwords});
    \item for this document, a LaTeX installation (\path{pdflatex}).
\end{itemize}
After an update of the code (\path{git pull}), the generator is installed again the same way.

\subsection{Making a change}

A change -- a new device, a renamed room, a new rule -- is made in the house's definitions, and
then generated and deployed: \path{bash ./generate} and \path{bash ./deploy} in the house's
folder. \path{generate} checks the definitions and reports what it finds wrong; \path{deploy}
carries the result to the machines (\path{bash ./deploy -n} only shows what it would do,
\path{bash ./deploy -only <part>} deploys one part). Generating also writes this document anew,
so the names in it are always the current ones.
`)
	return b.String()
}

// documentationDescribedList is items as a LaTeX description list, each line indented by indent.
func documentationDescribedList(items []TDocumentationItem, indent string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\\begin{description}\n", indent)
	for _, item := range items {
		fmt.Fprintf(&b, "%s    \\item[%s:] %s\n", indent, item.Name, item.Description)
	}
	fmt.Fprintf(&b, "%s\\end{description}\n", indent)
	return b.String()
}
