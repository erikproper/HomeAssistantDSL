Parser:
- Two level
- "node" NodeForPlatform
- WithClause(f): "with", (":",f; GroupClause(f)) 
- NodeForPlatform:  "myname" WithClause(MyWithClause)

entities, devices, and integrations, (and areas) are given concepts (and labels).
Follow homeassistant's definitions, but enrich their interpretation

Physical:
- Integrations hiding protocols
Logical:
- Devices with proto entities; they don't become real entities until they are put in a space.
Conceptual:
- Entities that may also refer to devices, plus device structure
- Devices are infrastructural things
- Entities are what matters to the people who live in the house and maintain the infra

Note: devices have a namespace: <name>[@<integration>]
Names of devices can be used on multiple integrations.
In that case, all proto-entities are united by default, but ... warn when overlap 
If we want to refer to a specific device defined on integration we can use the @
We can also define aggregated devices. In that case, we can handle ambiguities of names.

Update indexes in fuse boxes

NOTE: each device in the cpu integration should imply a device for the ping integration.

Step 0: Ping integration
Step 1: Architecture and experiment with MQTT based on old strategy
	 Logical layer nodes from integrations:
	- integration ping with:
		begin
			junglinster;
			xanadu;
			sonos....;
			...
		 end;
		- each creates of potentially defined nodes and entities
			- binary_sensor.$host/reachable from state topic, and a rule for the availability server based on messages => available and 2 minutes silence => not available
			- Do so via two implied lists
			- Imply nodes
			- And ... dependencies on hosts (where does ping run?) and containers.
			  In the ping case this would be nonsense of course.
	- integration cpu with
		begin
			junglinster;
			xanadu;
			...
		end;
			Each again ...
			- sensor.cpu/load => topic
			- sensor.cpu/temperature => state topic
			imply  nodes
	Aggregated nodes (derived at main when needed):
	WELL ... aggregated integration ... with "complementing"
		- node junglinster with
			- node: cpu.junglinster
			- node: ping.junglinster
			- available: change to available cpu.junglinster
			- unavalable: change to not junglinster_ping.reachabl
	Conceptual node:
	- entities infrastructural:rack/junglinster for: node junglinster
	- refers to name/value
Step 2: Weather, Netatmo, Bridge to Vienna
Step 3: Clarify the layers, entities, nodes, and the link to devices
Step 4: Also cross check areas versus spaces ..
