If ~/.claude/checkpoints/ has a checkpoint for this project, read it first to recover context.

# Claude Code Instructions — Home Assistant DSL

This project is a Pascal/fil-style DSL that generates Home Assistant YAML (scripts, automations, template sensors/switches). The direct template is the mailfilters codebase at `../mailfilters`.

---

## File Header Comment Style

Every `.go` file starts with a block comment in this exact format:

```go
/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: <ComponentName>
 *
 * <One sentence describing what this component does.>
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: DD.MM.YYYY
 *
 */
```

---

## Type Naming: `T` Prefix

All Go types and structs are prefixed with uppercase `T`:

```go
type TParser struct { ... }
type TServer struct { ... }
type TConditionKind int
type TConditionGroup []TCondition
```

Never use unprefixed names (`Parser`, `Server`) or lowercase-t (`tParser`).

---

## CDL1-Inspired Parser Style

The parser is a recursive-descent, bool-returning parser modelled on CDL1.
See: https://en.wikipedia.org/wiki/Compiler_Description_Language

### Core rules

| Pattern | Meaning |
|---------|---------|
| `A() && B()` | A then B (sequence) |
| `A() \|\| B()` | A or else B (alternative) |
| `ForcedX()` | X is required; reports an error and skips on failure |
| `Xety()` | X or empty (optional) |

### Parse functions return `bool`

- `true` = the rule matched and was consumed
- `false` = the rule did not match (no input consumed)
- Never return `error`; errors are recorded on the filter base / parse state and execution continues

### Data is accumulated via pointers or struct fields, not return values

```go
// Correct
func (p *TParser) ScriptLogin(script *TScript) bool {
    step := TScriptStep{Kind: StepLogin}
    return p.ThisKeywordWas("login") &&
        p.ForcedThisIdentWas(&step.Server) &&
        p.ForcedThisSemicolonWas() &&
        p.AppendStep(script, step)
}

// Wrong — do not return data
func (p *TParser) ParseLogin() (TScriptStep, error) { ... }
```

### Forced variants follow the standard error-and-skip pattern

```go
func (p *TParser) ForcedThisKeywordWas(keyword string) bool {
    return p.ThisKeywordWas(keyword) ||
        p.ReportParsingError("expected %q, got %q", keyword, p.current.Value) &&
            p.SkipToEndOrNextBlock()
}
```

### Grammar rule → method naming

Grammar productions map directly to method names. A production comment above each parser method documents the concrete syntax:

```go
// ScriptLogin: "login" Ident ";"
func (p *TParser) ScriptLogin(script *TScript) bool { ... }

// Options: "options" ":" OptionPropertiesEty "end" ";"?
func (p *TParser) Options() bool { ... }
```

---

## Inline Comment Style

Comments explain *why* or *what*, especially for non-obvious logic. Style seen in `lua.go`:

- Short sentence per case in a switch, explaining intent:
  ```go
  case StepLogin:
      // Emit an IMAP connection block, inlining all credentials from the server definition.
  ```
- For tracking variables, explain the purpose and consumers:
  ```go
  // Track the server named in the most recent StepSelect; used by "run all rules"
  // (without an explicit server) to know which folder rules to emit.
  currentServer := ""
  ```
- For wrapper helpers, one line is enough:
  ```go
  // The "w" function is a convenient wrapper around fmt.Fprintf to write formatted lines to the file.
  w := func(format string, args ...interface{}) {
      fmt.Fprintf(f, format, args...)
  }
  ```

---

## Section Separator Comments

Group related functions with a single-line banner comment:

```go
// --- error reporting and recovery ---

// --- primitive token-matching functions ---

// --- Forced versions: report error and skip on failure, returning false to abort the chain ---

// --- top level ---
```

---

## Small Helper Convention

Tiny bool-returning helpers exist to allow side-effects inside `&&` chains:

```go
func (p *TParser) SetCondKind(cond *TCondition, kind TConditionKind) bool {
    cond.Kind = kind
    return true
}

func (p *TParser) AppendStep(script *TScript, step TScriptStep) bool {
    script.Steps = append(script.Steps, step)
    return true
}
```

Always return `true` (they signal "succeeded").

---

## General Go Style

- `iota` for enum-like constants; the type name follows the `T` prefix rule:
  ```go
  type TConditionKind int
  const (
      CondFrom TConditionKind = iota
      CondSubject
      ...
  )
  ```
- Field comments on the same line when short:
  ```go
  FastmailJSON string // base name of the linked .json mailrules file
  SourceFile   string // absolute path of the .fil file this block was parsed from
  ```
- Avoid over-engineering: no extra abstractions, no error-wrapping for internal helpers, no feature flags.
