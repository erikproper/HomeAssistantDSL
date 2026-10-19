---
name: project-rule-grammar-no-semicolon-between-clauses-2026-10-15
description: "DONE 2026-10-15: the rule grammar's \"when\"/\"if\" clauses no longer end in \";\" -- Pascal-style, the clause ends wherever the next clause's own keyword (\"if\"/\"then\") begins. Only the final \"then\" action(s) keep their \";\". Both houses' Conceptual.def mass-rewritten; both regenerate byte-identical to before."
metadata:
  node_type: memory
  type: project
  originSessionId: 90d6291d-7e36-4477-8954-091d4a9d14e2
  modified: 2026-10-15T16:14:44.801Z
---

**The change**: user flagged real rule text as wrong style --
```
when event.physical:door/moes/1 is 2_single or
     ...
     event.physical:/apartment/hallway/bedroom/door is 1_single;
then toggle light.social:main;
```
should be (no `;` before `then`):
```
when event.physical:door/moes/1 is 2_single or
     ...
     event.physical:/apartment/hallway/bedroom/door is 1_single
then toggle light.social:main;
```
Confirmed scope with the user: forbid the `;` there (not just tolerate), and mechanically rewrite
every existing "when"/"if" clause across both houses' Conceptual.def, not just new rules.

**Two separate places needed the fix, not one** -- a real trap worth remembering:
1. `rule_parser.go`'s `WhenStatement`/`IfStatement` -- swapped `ForcedThisPunctuationWas(';')` for
   a new `ForcedEndOfInput()` (checks `AtEndOfTokens()`).
2. The outer line-joiner that decides which physical lines to glue into one logical line BEFORE
   the grammar ever sees them -- and there turned out to be TWO of these, not one:
   - `parser.go`'s own rule-body joiner (inside the `currentRule != nil` branch).
   - `defined.go`'s `joinWrappedStatementContinuations`/`normalizeDefinitionWhitespace` -- a
     UNIVERSAL pass every `.def` file flows through at file-read time, BEFORE parser.go's own
     loop ever sees a line. Fixing only (1) and parser.go's joiner looked right in isolated Go
     tests (which call `ParseEntitiesAndFillAdministration` directly on a string, bypassing this
     universal pass entirely) but broke on the real files: this earlier pass still used the OLD
     "not yet ';'/':'-terminated, keep joining" rule, so a now-semicolon-less "when ... 1_single"
     line got wrongly glued to the next "then:" line before parser.go ever got a chance.
   Fixed by extracting a shared `lineIsCompleteStatement`/`endsWithChainConnective` (defined.go):
   a "when "/"if "-prefixed line is complete once it stops dangling on a trailing "or"/"and"
   connective; everything else keeps the original ";"/":"-terminated rule. Both joiners now call
   the same helper.

**How to apply**: if a future grammar change to this DSL changes where a statement/clause legally
ends, remember there may be MORE than one place line-joining happens -- check for
`normalizeDefinitionWhitespace`/`joinWrappedStatementContinuations` (defined.go) specifically,
since it's a silent, universal preprocessing pass that isolated Go-level parser tests never
exercise (they skip straight to `ParseEntitiesAndFillAdministration` on a raw string). A test
passing in isolation is not proof the real `./generate` pipeline will behave the same -- always
verify against a real house's `./generate` (clean, zero warnings) before declaring a grammar
change done, not just `go test`.

**Verification**: `go test ./...` green; both Vienna and Junglinster's `./generate` clean (zero
warnings); both houses' generated `hass/<house>/` output byte-identical to before the change
(confirmed via `diff -rq` against a pre-change copy) -- proof this was a pure syntax change with
zero semantic effect on the generated automations.
