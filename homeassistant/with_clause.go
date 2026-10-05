/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: WithClause
 *
 * The generic "with clause" body-statement dispatcher (2026-09-27) -- the missing half of
 * Background/Arch2.md's own WithClause(f)/GroupClause(f) sketch. `scanGroupClauseBlocks`
 * (defined.go) already formalizes that sketch at the WRAPPER-HEADER level (an "integration X
 * with: ... end;" block's own header+body extraction, shared by every layer/integration
 * kind) -- but every construct's own BODY, once extracted, has always hand-rolled its own
 * "try each recognised single-statement shape, in order" dispatch, once for the multi-line
 * block form (one call per body line) and AGAIN for the "<header> with <single-statement>;"
 * one-liner shorthand (one call on the folded text) -- two copies of the same list of shapes,
 * per consumer, kept in sync only by hand. `integration_hassbridge_parser.go`'s own
 * `applyCapabilityWithBodyLine` closure is the closest real precedent (shared between its own
 * block+one-liner dispatch already) but scoped to one file, one accumulator type
 * (THassBridgeCapability) -- this file generalizes that EXACT idea across every consumer in
 * the DSL, parameterized over the accumulator type via a Go generic (this codebase's first
 * use of one -- kept deliberately small and self-contained, not a broader style shift).
 *
 * Using it: define the accumulator type's own statement list ONCE, as
 * []TWithClauseStatement[T], then call ApplyWithClauseBodyLine for every block body line AND
 * (on the SAME list) once more for a folded one-liner's own single statement text -- a new
 * statement kind added to the list automatically works in both forms, by construction. This
 * doesn't touch scanGroupClauseBlocks/TGroupClauseBlock at all (that's still the right tool for
 * a construct's own HEADER+block-body EXTRACTION); this is purely about what happens to each
 * extracted body line/one-liner statement next.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 27.09.2026
 *
 */

package main

import "regexp"

// TWithClauseStatement is one recognised single-statement body form for a generic with-clause
// consumer, parameterised over the accumulator type T (e.g. TLogicalCapability,
// THassBridgeCapability, TLocalCapability, TLogicalDevice, ...). Pattern must match the WHOLE
// line, including its own trailing ";". Apply receives the pattern's own FindStringSubmatch
// result (index 0 is the whole match) and the current accumulator value, returning the updated
// value.
type TWithClauseStatement[T any] struct {
	Pattern *regexp.Regexp
	Apply   func(acc T, matches []string) T
}

// ApplyWithClauseBodyLine tries each statement in statements, in order, returning
// (updated, true) for the first one whose Pattern matches line, or (acc, false) if none do.
// This is the ONE dispatch a with-clause consumer needs, called identically from both its own
// multi-line block-body loop (once per body line) and its one-liner fold (once, on the folded
// single-statement text) -- see this file's own header comment for why that equivalence is the
// entire point.
func ApplyWithClauseBodyLine[T any](acc T, line string, statements []TWithClauseStatement[T]) (T, bool) {
	for _, stmt := range statements {
		if matches := stmt.Pattern.FindStringSubmatch(line); matches != nil {
			return stmt.Apply(acc, matches), true
		}
	}
	return acc, false
}
