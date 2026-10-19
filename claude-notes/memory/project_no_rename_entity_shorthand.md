---
name: project-no-rename-entity-shorthand
description: "DONE 2026-09-19: two new 'no rename' shorthands -- 'entity <spec>;' inside a device block (capability auto-inferred from spec's own explicit path) and '<domain>.<label>;' inside a Logical.def absorb block (capability auto-inferred from the label) -- both only when the inferred name matches exactly; a rename still needs the explicit form."
metadata: 
  node_type: memory
  type: project
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-18T20:25:43.028Z
---

Two syntactic simplifications, both following the same principle: when the desired local name is
IDENTICAL to the underlying capability name, the capability reference can be dropped entirely and
auto-inferred; a genuine rename (local name differs from the capability) still requires the
existing explicit form. Both are pure textual expansion added ALONGSIDE the existing shorthand,
never replacing it -- no existing `.def` content needed to change for either to start working.

**1. Conceptual layer, inside a `device <spec> from <device-id> with: ... end;` block.**
`entity <spec> [with no_collect];` (no `from <capability>` at all) expands to the existing
`entity <spec> from <capability> [with no_collect];` shape, inferring `<capability>` as
`deviceSpecLeafPath(spec)` (the explicit path after the sphere's `:`, e.g. "co2" from
"sensor.physical:co2"). New pattern `forDeviceBareEntityPattern` + function
`expandForDeviceBareEntityLine` (`Conceptual_DeviceCapabilityEntities.go`), tried as a fallback in
parser.go's main loop when the existing `expandForDeviceShorthandLine` doesn't match. Returns no
match when the spec has no explicit path at all (e.g. "switch.social:", relying entirely on
deviceNamePath injection) -- nothing to infer from, so the explicit "from <capability>;" form is
still required there.

**2. Logical layer, inside a `capabilities from <device-id>: ... end;` absorb block.**
`<domain>.<label>;` (no `: <value>` at all) is equivalent to `<domain>.<label>: <label>;` --
inferring the absorbed capability as the label itself. New pattern
`logicalAbsorbBareCapabilityPattern` in `integration_logical_parser.go`, checked in the `inAbsorb`
state before the existing `logicalCapabilityPattern` (colon+value) check.

Both fully tested (unit-level expansion tests, plus one end-to-end
`ParseEntitiesAndFillAdministration`-based test per shorthand confirming identical registration to
the explicit form). `go build`/`vet`/`test` clean.

**How to apply:** available for use in both houses' `.def` files immediately -- no retroactive
sweep of Vienna's already-migrated declarations was done or requested; this is a forward-looking
convenience, not a required cleanup. If asked to "simplify" a batch of existing declarations later,
this is the mechanism to use, but don't do it unprompted.
