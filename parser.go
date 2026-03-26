/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Parser
 *
 * This component provides a CDL2-inspired structural parser for definition files, built on TDefTokenizer.
 * It produces a TParsedFile tree of TNode records used by the interpretation debug report.
 * Separately, ParseEntitiesAndFillAdministration processes entity lines for semantic analysis.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 25.03.2026
 *
 */

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// --- Node and file types ---

type TNodeKind int

const (
	NodeComment   TNodeKind = iota
	NodeStatement           // a line or set of lines ending with ';'
	NodeBlock               // a block header ending with ':' containing a child Sequence
)

// Structural keyword tokens.
// StatementEndToken is the universal statement terminator; end; is EndToken + StatementEndToken.
const (
	EndToken          = "end"
	StatementEndToken = ";"
	ElseToken         = "else"
	ElifToken         = "elif"
	IfToken           = "if"
	IsToken           = "is"
	UpToken           = "up"
	ThenToken         = "then"
	CommentPrefix     = "#"
	BlockHeaderSuffix = ":"
)

type TNode struct {
	Kind       TNodeKind
	Text       string
	SourceLine int
	Children   []TNode
}

type TParsedFile struct {
	Name           string
	Path           string
	Nodes          []TNode
	ParseSucceeded bool
	Errors         []string
}

// --- TDefinitionParser: structural parser built on TDefTokenizer ---
//
// CDL2 conventions (same as bibtex_check):
//   A() && B()  — A then B (sequence)
//   A() || B()  — A or else B (alternative)
//   ForcedX()   — X is required; reports an error on failure
//   Xety()      — X or empty (optional)

type TDefinitionParser struct {
	TDefTokenizer
	parsed *TParsedFile
}

// ParseDefinitionFile parses a .def file and returns its structural tree.
func ParseDefinitionFile(path string) (TParsedFile, error) {
	parsed := TParsedFile{
		Name:           filepath.Base(path),
		Path:           path,
		ParseSucceeded: true,
	}
	p := &TDefinitionParser{parsed: &parsed}
	if !p.OpenFile(path) {
		parsed.ParseSucceeded = false
		parsed.Errors = append(parsed.Errors, fmt.Sprintf("cannot open file %s", path))
		return parsed, nil
	}
	p.atLineStart = true
	p.NextToken(TokenKindWord) // prime the lookahead
	p.Sequence(&parsed.Nodes, false)
	p.CloseFile()
	parsed.Errors = append(parsed.Errors, p.errors...)
	if len(parsed.Errors) > 0 {
		parsed.ParseSucceeded = false
	}
	return parsed, nil
}

// --- error reporting and small helpers ---

// ReportParsingError records a parse error with source location.
func (p *TDefinitionParser) ReportParsingError(format string, args ...any) bool {
	msg := fmt.Sprintf("%s:%d: %s", p.filePath, p.linePosition, fmt.Sprintf(format, args...))
	p.parsed.Errors = append(p.parsed.Errors, msg)
	return true
}

// AppendNode appends node to *nodes.
func (p *TDefinitionParser) AppendNode(nodes *[]TNode, node TNode) bool {
	*nodes = append(*nodes, node)
	return true
}

// SetNodeText assigns the text field of a node.
func (p *TDefinitionParser) SetNodeText(node *TNode, text string) bool {
	node.Text = text
	return true
}

// --- grammar rules ---

// Sequence: (Comment | Conditional | BlockOrStatement | 'end' ';')*
//
// When untilEnd is true, Sequence reads until 'end;' and consumes it.
// When untilEnd is false, Sequence reads until EOS (top-level call).
func (p *TDefinitionParser) Sequence(nodes *[]TNode, untilEnd bool) bool {
	for {
		if p.ThisTokenIs(TokenKindEOS) {
			if untilEnd {
				return p.ReportParsingError("unexpected end of file while parsing block") && false
			}
			return true
		}

		// Comment → NodeComment
		if p.Comment(nodes) {
			continue
		}

		// 'end;' closes the innermost block (or reports an error at top level).
		if p.ThisToken() == EndToken {
			p.NextToken(TokenKindWord)
			if p.ThisTokenIs(TokenKindSemicolon) {
				p.NextToken(TokenKindWord)
				if untilEnd {
					return true
				}
				p.ReportParsingError("unexpected 'end;' at top level")
			} else {
				p.ReportParsingError("expected ';' after 'end'")
			}
			continue
		}

		// Structural conditional 'if is up ...' (and fallback for other 'if ...' forms).
		if p.Conditional(nodes) {
			continue
		}

		// Generic block header or statement.
		if !p.BlockOrStatement(nodes) {
			if !p.ThisTokenIs(TokenKindEOS) {
				p.ReportParsingError("unexpected token %q", p.ThisToken())
				p.NextToken(TokenKindWord)
			}
		}
	}
}

// ConditionalBody: (Comment | BlockOrStatement)* — stops at 'end', 'elif', or 'else'.
func (p *TDefinitionParser) ConditionalBody(nodes *[]TNode) bool {
	for {
		if p.ThisTokenIs(TokenKindEOS) {
			return true
		}
		tok := p.ThisToken()
		if tok == EndToken || tok == ElifToken || tok == ElseToken {
			return true
		}
		if p.Comment(nodes) {
			continue
		}
		p.BlockOrStatement(nodes)
	}
}

// Conditional handles all 'if'-starting constructs.
// The structural form 'if is up "hostname" then ... elif ... else ... end;' creates a NodeBlock.
// Any other 'if ...' form (e.g. macro-body 'if ${x} is provided then') becomes a NodeStatement.
func (p *TDefinitionParser) Conditional(nodes *[]TNode) bool {
	if p.ThisToken() != IfToken {
		return false
	}
	sourceLine := p.linePosition
	parts := []string{IfToken}
	p.NextToken(TokenKindWord)

	// Detect the structural form: 'if is up ...'
	if p.ThisToken() == IsToken {
		parts = append(parts, IsToken)
		p.NextToken(TokenKindWord)
		if p.ThisToken() == UpToken {
			return p.finishStructuralConditional(nodes, sourceLine, parts)
		}
		// Not the structural form; fall through and collect as a Statement below.
	}

	// Non-structural 'if ...': collect the rest of the line as a NodeStatement.
	return p.collectControlStatement(nodes, sourceLine, parts)
}

// finishStructuralConditional completes parsing of 'if is up "hostname" then ... end;'.
// parts already contains ["if", "is"]; current token is "up".
func (p *TDefinitionParser) finishStructuralConditional(nodes *[]TNode, sourceLine int, parts []string) bool {
	// Consume 'up' and everything up to (and including) 'then'.
	for !p.ThisTokenIs(TokenKindEOS) {
		tok := p.ThisToken()
		parts = append(parts, tok)
		p.NextToken(TokenKindPath)
		if tok == ThenToken {
			break
		}
	}
	node := TNode{Kind: NodeBlock, SourceLine: sourceLine, Text: strings.Join(parts, " ")}
	p.ConditionalBody(&node.Children)

	// elif branches
	for p.ThisToken() == ElifToken {
		branchLine := p.linePosition
		branchParts := []string{p.ThisToken()}
		p.NextToken(TokenKindPath)
		for !p.ThisTokenIs(TokenKindEOS) {
			tok := p.ThisToken()
			branchParts = append(branchParts, tok)
			p.NextToken(TokenKindPath)
			if tok == ThenToken {
				break
			}
		}
		branch := TNode{Kind: NodeBlock, SourceLine: branchLine, Text: strings.Join(branchParts, " ")}
		p.ConditionalBody(&branch.Children)
		node.Children = append(node.Children, branch)
	}

	// else branch
	if p.ThisToken() == ElseToken {
		elseLine := p.linePosition
		p.NextToken(TokenKindWord)
		branch := TNode{Kind: NodeBlock, SourceLine: elseLine, Text: ElseToken}
		p.ConditionalBody(&branch.Children)
		node.Children = append(node.Children, branch)
	}

	// Consume 'end;'
	if p.ThisToken() == EndToken {
		p.NextToken(TokenKindWord)
		if p.ThisTokenIs(TokenKindSemicolon) {
			p.NextToken(TokenKindWord)
		} else {
			p.ReportParsingError("expected ';' after 'end' in conditional")
		}
	} else {
		p.ReportParsingError("expected 'end;' to close conditional block")
	}

	return p.AppendNode(nodes, node)
}

// collectControlStatement collects tokens already in parts plus any remaining tokens
// until ';', ':', or a control-line terminator ('then', 'do'), then appends a NodeStatement.
func (p *TDefinitionParser) collectControlStatement(nodes *[]TNode, sourceLine int, parts []string) bool {
	for {
		switch {
		case p.ThisTokenIs(TokenKindEOS), p.ThisTokenIs(TokenKindColon), p.ThisTokenIs(TokenKindSemicolon):
			// End of statement (degenerate: no terminator found, or unexpected delimiter).
			// Let BlockOrStatement handle it on the next iteration.
			return p.AppendNode(nodes, TNode{Kind: NodeStatement, SourceLine: sourceLine,
				Text: strings.Join(parts, " ")})
		case p.ThisTokenIs(TokenKindComment):
			p.NextToken(TokenKindWord)
		default:
			tok := p.ThisToken()
			parts = append(parts, tok)
			p.NextToken(TokenKindPath)
			if tok == ThenToken || tok == "do" {
				return p.AppendNode(nodes, TNode{Kind: NodeStatement, SourceLine: sourceLine,
					Text: strings.Join(parts, " ")})
			}
		}
	}
}

// Comment: TokenKindComment → NodeComment
func (p *TDefinitionParser) Comment(nodes *[]TNode) bool {
	if !p.ThisTokenIs(TokenKindComment) {
		return false
	}
	node := TNode{Kind: NodeComment, SourceLine: p.linePosition, Text: p.ThisToken()}
	p.NextToken(TokenKindWord)
	return p.AppendNode(nodes, node)
}

// BlockOrStatement collects a sequence of tokens terminated by ':' (block) or ';' (statement).
// Control-line terminators 'then' and 'do' also end a statement without consuming a ';'.
// Standalone 'else' on its own is also a statement.
func (p *TDefinitionParser) BlockOrStatement(nodes *[]TNode) bool {
	if p.ThisTokenIs(TokenKindEOS) || p.ThisTokenIs(TokenKindComment) {
		return false
	}
	if p.ThisToken() == EndToken {
		return false // 'end;' is handled by Sequence
	}

	sourceLine := p.linePosition
	parts := []string{}

	for {
		switch {
		case p.ThisTokenIs(TokenKindEOS):
			if len(parts) > 0 {
				p.ReportParsingError("unexpected end of file in statement or block header")
				p.AppendNode(nodes, TNode{Kind: NodeStatement, SourceLine: sourceLine,
					Text: strings.Join(parts, " ")})
			}
			return len(parts) > 0

		case p.ThisTokenIs(TokenKindColon):
			// ':' terminates a block header only when preceded by one of the DSL's three
			// block-opening patterns: 'with', ')', '}' (macro definitions), or 'secrets'.
			// Any other preceding token means ':' is statement-internal (e.g. 'light on: x;',
			// 'call macro :relative_path;', or a relative-path positional argument).
			last := ""
			if len(parts) > 0 {
				last = parts[len(parts)-1]
			}
			if last == "with" || last == ")" || last == "}" || last == "secrets" {
				p.NextToken(TokenKindWord)
				headerText := strings.Join(parts, " ") + ":"
				node := TNode{Kind: NodeBlock, SourceLine: sourceLine, Text: headerText}
				p.Sequence(&node.Children, true)
				return p.AppendNode(nodes, node)
			}
			// Statement-internal ':' — append and continue collecting.
			parts = append(parts, ":")
			p.NextToken(TokenKindPath)

		case p.ThisTokenIs(TokenKindSemicolon):
			// Statement.
			p.NextToken(TokenKindWord)
			return p.AppendNode(nodes, TNode{Kind: NodeStatement, SourceLine: sourceLine,
				Text: strings.Join(parts, " ") + ";"})

		case p.ThisTokenIs(TokenKindComment):
			// Inline comment mid-statement: skip.
			p.NextToken(TokenKindWord)

		default:
			tok := p.ThisToken()

			// Standalone 'else' is a one-word statement.
			if tok == ElseToken && len(parts) == 0 {
				p.NextToken(TokenKindWord)
				return p.AppendNode(nodes, TNode{Kind: NodeStatement, SourceLine: sourceLine,
					Text: ElseToken})
			}

			parts = append(parts, tok)

			// Control-line terminators end a statement without a ';'.
			if tok == ThenToken || tok == "do" {
				p.NextToken(TokenKindWord)
				return p.AppendNode(nodes, TNode{Kind: NodeStatement, SourceLine: sourceLine,
					Text: strings.Join(parts, " ")})
			}

			// Use TokenKindPath so entity paths (letters + . / : [ ] + ${...}) are collected greedily.
			p.NextToken(TokenKindPath)
		}
	}
}

// --- helper used by interpretation.go ---

// stripTrailingInlineComment removes an inline comment (" #...") from a trimmed line.
func stripTrailingInlineComment(line string) string {
	trimmedLine := strings.TrimSpace(line)
	commentIndex := strings.Index(trimmedLine, " #")
	if commentIndex >= 0 {
		return strings.TrimSpace(trimmedLine[:commentIndex])
	}
	return trimmedLine
}

// --- semantic types and processing (unchanged) ---

type TExpansionParseResult struct {
	Administration   *TAdministrationState
	InvocationCount  int
	ValidInvocations int
	TypeErrors       int
}

// ParseEntitiesAndFillAdministration parses entity lines, applies strict macro validation,
// performs aggressive macro expansion, and records open/close entity/space events into administration.
func ParseEntitiesAndFillAdministration(entityLines []string, entitiesPath string, ctx *TMacroExpansionContext, report *strings.Builder) (TExpansionParseResult, error) {
	administration := NewAdministrationState()
	onSpaceClosed := func(_ string) {
		// Space-close hooks are centralized in administration; aggregate derivation stays a separate pass.
	}
	invocationCount := 0
	validInvocations := 0
	typeErrors := 0

	// processInvocation parses, validates, expands, and registers one macro invocation text.
	// It is used both for top-level invocations and for inline "entity … with <invocation>;" bodies.
	processInvocation := func(invText string, lineNum int) error {
		invocationCount++
		report.WriteString(fmt.Sprintf("Line %d (in %s):\n", lineNum, formatSpacePath(administration.SpacePath)))
		report.WriteString(fmt.Sprintf("  Invocation: %s\n", invText))
		invocation, parseErr := ctx.ParseMacroInvocation(invText)
		if parseErr != nil {
			return fmt.Errorf("strict macro validation failed in %s at line %d: failed to parse invocation %q: %w", entitiesPath, lineNum, invText, parseErr)
		}
		if strictErr := ctx.ValidateInvocationStrict(invocation); strictErr != nil {
			return fmt.Errorf("strict macro validation failed in %s at line %d for invocation %q: %w", entitiesPath, lineNum, invText, strictErr)
		}
		validInvocations++
		report.WriteString("  Status: OK (all parameters valid)\n")
		callChain := fmt.Sprintf("%s:%d → %s %s", filepath.Base(entitiesPath), lineNum, invocation.Name, invocation.Target)
		expandedRecords, expandErr := collectExpandedEntityRecords(ctx, invocation, administration.SpacePath, false, callChain)
		if expandErr != nil {
			return fmt.Errorf("strict macro validation failed in %s at line %d while expanding invocation %q: %w", entitiesPath, lineNum, invText, expandErr)
		}
		for _, expandedRecord := range expandedRecords {
			administration.EnsureSpaceRegistered(expandedRecord.SpacePath, SpaceKindRegular)
			expandedSpaceName := formatNestedSpaceName(expandedRecord.SpacePath)
			isExternal := !expandedRecord.Record.HasDefinitionOrImport
			externalEntry := ""
			if isExternal {
				externalEntry = expandedRecord.Record.Name
			}
			administration.RegisterEntityClosure(TPendingEntityCollection{
				SpaceName:      expandedSpaceName,
				Entry:          expandedRecord.Record.Name,
				ExternalEntry:  externalEntry,
				Record:         expandedRecord.Record,
				HasExternalRef: isExternal,
			})
		}
		if len(invocation.Parameters) > 0 {
			report.WriteString("  Parameters:\n")
			for pkey, pval := range invocation.Parameters {
				report.WriteString(fmt.Sprintf("    %s = %q\n", pkey, pval))
			}
		}
		report.WriteString("\n")
		return nil
	}

	for i := 0; i < len(entityLines); i++ {
		trimmed := strings.TrimSpace(entityLines[i])
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if candidateMacroName, isCreateInvocation := extractCallInvocationMacroName(trimmed); isCreateInvocation {
			if _, exists := ctx.Macros[candidateMacroName]; !exists {
				return TExpansionParseResult{}, fmt.Errorf("strict macro validation failed in %s at line %d: unknown macro %q in invocation %q", entitiesPath, i+1, candidateMacroName, trimmed)
			}
		}

		if entityDecl, ok := extractEntityDeclaration(trimmed); ok {
			// A second ':' in an entity spec is legacy sub-domain notation; '/' must be used instead.
			if hasSecondColonSeparator(entityDecl.Specification) {
				fmt.Fprintf(os.Stderr, "[WARNING] %s line %d: entity specification %q uses a second ':' sub-domain separator (legacy); use '/' instead\n",
					entitiesPath, i+1, entityDecl.Specification)
			}

			administration.EnsureSpaceRegistered(administration.SpacePath, SpaceKindRegular)

			spaceName := administration.CurrentSpaceName()
			fullName := normalizeEntityFullName(entityDecl.Specification, administration.SpacePath)
			hasDefOrImport, optionKeys := analyzeEntityDefinitionContext(entityLines, i)
			hasConfigOptions := len(optionKeys) > 0
			entry := fmt.Sprintf("%s (line %d)", fullName, i+1)
			if entityDecl.NoCollect {
				entry += " [no_collect]"
			}
			externalEntry := ""
			hasExternalRef := false
			if !hasDefOrImport {
				hasExternalRef = true
				externalEntry = fmt.Sprintf("%s (line %d)", fullName, i+1)
				if hasConfigOptions {
					externalEntry += fmt.Sprintf(" [config options: %s]", strings.Join(optionKeys, ", "))
				}
			}

			record := TEntityRecord{
				Name:                  fullName,
				Identity:              extractEntityIdentity(fullName),
				NoCollect:             entityDecl.NoCollect,
				HasDefinitionOrImport: hasDefOrImport,
				Provenance:            fmt.Sprintf("%s:%d", filepath.Base(entitiesPath), i+1),
			}

			if strings.HasSuffix(trimmed, " with:") {
				administration.PendingEntityCollections = append(administration.PendingEntityCollections, TPendingEntityCollection{
					SpaceName:      spaceName,
					Entry:          entry,
					ExternalEntry:  externalEntry,
					Record:         record,
					ExpectedDepth:  len(administration.OpenBlocks) + 1,
					HasExternalRef: hasExternalRef,
				})
			} else {
				administration.RegisterEntityClosure(TPendingEntityCollection{
					SpaceName:      spaceName,
					Entry:          entry,
					ExternalEntry:  externalEntry,
					Record:         record,
					HasExternalRef: hasExternalRef,
				})
				// "entity … with <invocation>;" — process the inline body as a macro invocation.
				if withIdx := strings.Index(trimmed, " with "); withIdx >= 0 {
					inlineStmt := strings.TrimSpace(trimmed[withIdx+len(" with "):])
					if isMacroInvocation(inlineStmt, ctx.Macros) {
						if err := processInvocation(inlineStmt, i+1); err != nil {
							return TExpansionParseResult{}, err
						}
					}
				}
			}
		}

		if isMacroInvocation(trimmed, ctx.Macros) {
			// Collect multi-line "with:" body into a single invocation text if needed.
			invocationText := trimmed
			if strings.HasSuffix(trimmed, "with:") {
				j := i + 1
				for ; j < len(entityLines); j++ {
					nextTrimmed := strings.TrimSpace(entityLines[j])
					invocationText += "\n" + nextTrimmed
					if nextTrimmed == "end;" {
						break
					}
				}
				if j < len(entityLines) {
					i = j
				}
			}
			if err := processInvocation(invocationText, i+1); err != nil {
				return TExpansionParseResult{}, err
			}
			continue
		}

		if spaceKind, spaceName, ok := parseSpaceHeader(trimmed); ok {
			administration.OpenSpace(spaceKind, spaceName)
			continue
		}

		if strings.HasSuffix(trimmed, "with:") {
			administration.OpenOtherBlock()
			continue
		}

		// Handle "space off: ..." statements
		if strings.HasPrefix(trimmed, "space off:") {
			spaceName := administration.CurrentSpaceName()
			itemsStr := strings.TrimPrefix(trimmed, "space off:")
			itemsStr = strings.TrimSpace(itemsStr)
			itemsStr = strings.TrimSuffix(itemsStr, ";")
			items := parseSpaceCollectionItems(itemsStr)
			extensionalItems := []string{}
			for _, item := range items {
				// Aggregation tokens (@light, @media, @all) are stored as-is for later expansion.
				if strings.HasPrefix(item, "@") {
					extensionalItems = append(extensionalItems, item)
					continue
				}
				normalized := normalizeEntityFullName(item, strings.Split(spaceName, "/"))
				if strings.ContainsAny(normalized, "$:{}[]*") || normalized == "" {
					fmt.Fprintf(os.Stderr, "[SERIOUS WARNING] Space %q: intensional or unresolved entity reference %q (normalized as %q) in space off; only extensional references are allowed and will be stored.\n", spaceName, item, normalized)
					continue
				}
				extensionalItems = append(extensionalItems, normalized)
			}
			administration.RecordSpaceOff(spaceName, extensionalItems)
			continue
		}

		// Handle "light on: ..." statements
		if strings.HasPrefix(trimmed, "light on:") {
			// Only record SpaceOnByName for the current space block, not for nested/child spaces
			// This prevents parent 'light on:' from propagating to children
			spaceName := administration.CurrentSpaceName()
			lightsStr := strings.TrimPrefix(trimmed, "light on:")
			lightsStr = strings.TrimSpace(lightsStr)
			lightsStr = strings.TrimSuffix(lightsStr, ";")
			lights := parseSpaceCollectionItems(lightsStr)
			extensionalLights := []string{}
			for _, light := range lights {
				lightRef := strings.TrimSpace(light)
				// Aggregation tokens (@light, @media, @all) are stored as-is for later expansion.
				if strings.HasPrefix(lightRef, "@") {
					extensionalLights = append(extensionalLights, lightRef)
					continue
				}
				// Prepend "light." when the reference has no domain prefix.
				if !strings.Contains(lightRef, ".") {
					lightRef = "light." + lightRef
				}
				normalized := normalizeEntityFullName(lightRef, strings.Split(spaceName, "/"))
				if strings.ContainsAny(normalized, "$:{}[]*") || normalized == "" {
					fmt.Fprintf(os.Stderr, "[SERIOUS WARNING] Space %q: intensional or unresolved entity reference %q (normalized as %q) in light on; only extensional references are allowed and will be stored.\n", spaceName, light, normalized)
					continue
				}
				extensionalLights = append(extensionalLights, normalized)
			}
			// Only set SpaceOnByName if inside a direct space block (not inherited)
			if len(administration.SpacePath) > 0 {
				administration.RecordSpaceOn(spaceName, extensionalLights)
			}
			continue
		}

		// Handle "light off: ..." statements — records which lights to turn off for the aggregate entity.
		if strings.HasPrefix(trimmed, "light off:") {
			spaceName := administration.CurrentSpaceName()
			lightsStr := strings.TrimSpace(strings.TrimPrefix(trimmed, "light off:"))
			lightsStr = strings.TrimSuffix(lightsStr, ";")
			lights := parseSpaceCollectionItems(lightsStr)
			offLights := []string{}
			for _, light := range lights {
				lightRef := strings.TrimSpace(light)
				if strings.HasPrefix(lightRef, "@") {
					offLights = append(offLights, lightRef)
					continue
				}
				if !strings.Contains(lightRef, ".") {
					lightRef = "light." + lightRef
				}
				normalized := normalizeEntityFullName(lightRef, strings.Split(spaceName, "/"))
				if strings.ContainsAny(normalized, "$:{}[]*") || normalized == "" {
					continue
				}
				offLights = append(offLights, normalized)
			}
			if len(administration.SpacePath) > 0 {
				administration.RecordLightOff(spaceName, offLights)
			}
			continue
		}

		// Handle "heating leak: ..." statements — records which entities (doors/windows) must be
		// closed before a radiator can safely run.  Generates a leakage_evidence sensor per space.
		if strings.HasPrefix(trimmed, "heating leak:") {
			spaceName := administration.CurrentSpaceName()
			refsStr := strings.TrimSpace(strings.TrimPrefix(trimmed, "heating leak:"))
			refsStr = strings.TrimSuffix(refsStr, ";")
			normalizedRefs := []string{}
			for _, ref := range parseSpaceCollectionItems(refsStr) {
				ref = strings.TrimSpace(ref)
				if ref == "" {
					continue
				}
				normalized := normalizeEntityFullName(ref, strings.Split(spaceName, "/"))
				if strings.ContainsAny(normalized, "$:{}[]*") || normalized == "" {
					fmt.Fprintf(os.Stderr, "[WARNING] Space %q: unresolvable heating leak reference %q\n", spaceName, ref)
					continue
				}
				normalizedRefs = append(normalizedRefs, normalized)
			}
			administration.RecordHeatingLeak(spaceName, normalizedRefs)
			continue
		}

		if trimmed == EndToken+StatementEndToken {
			administration.HandleEndToken(onSpaceClosed)
		}
	}

	// For every space that has heating leak entries, auto-register a leakage_evidence sensor.
	// The entity lives in the physical domain at the same path as the social space.
	for spaceName, leaks := range administration.HeatingLeaksByName {
		if len(leaks) == 0 {
			continue
		}
		// Derive the path component (strip the leading sphere name, e.g. "social/kitchen" → "kitchen").
		contextPath := spaceName
		if idx := strings.Index(spaceName, "/"); idx >= 0 {
			contextPath = spaceName[idx+1:]
		} else if isKnownSphere(spaceName) {
			contextPath = ""
		}
		var entityName string
		if contextPath == "" {
			entityName = "binary_sensor.physical/heating/leakage_evidence"
		} else {
			entityName = fmt.Sprintf("binary_sensor.physical/%s/heating/leakage_evidence", contextPath)
		}
		administration.EnsureSpaceRegistered(administration.SpacePath, SpaceKindRegular)
		administration.RegisterEntityClosure(TPendingEntityCollection{
			SpaceName: spaceName,
			Entry:     fmt.Sprintf("%s (heating leak)", entityName),
			Record: TEntityRecord{
				Name:                  entityName,
				Identity:              extractEntityIdentity(entityName),
				NoCollect:             false,
				HasDefinitionOrImport: true,
				Provenance:            "heating_leak:" + spaceName,
			},
			HasExternalRef: false,
		})
	}

	RunPostParseChecks(administration)

	return TExpansionParseResult{
		Administration:   administration,
		InvocationCount:  invocationCount,
		ValidInvocations: validInvocations,
		TypeErrors:       typeErrors,
	}, nil
}
