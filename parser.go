/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Parser
 *
 * This component provides a CDL1-inspired parser for normalized definition files and builds a structural interpretation tree.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 21.03.2026
 *
 */

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type TNodeKind int

const (
	NodeComment TNodeKind = iota
	NodeStatement
	NodeBlock
)

// Structural keyword tokens.
// StatementEndToken is the universal statement terminator; end; is EndToken+StatementEndToken.
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

type TDefinitionParser struct {
	filePath string
	lines    []string
	index    int
	parsed   *TParsedFile
}

func ParseDefinitionFile(path string) (TParsedFile, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return TParsedFile{}, err
	}

	parsed := TParsedFile{
		Name:           filepath.Base(path),
		Path:           path,
		ParseSucceeded: true,
	}
	parser := newDefinitionParser(string(content), path, &parsed)
	parser.Sequence(&parsed.Nodes, false)
	if len(parsed.Errors) > 0 {
		parsed.ParseSucceeded = false
	}
	return parsed, nil
}

func newDefinitionParser(content, filePath string, parsed *TParsedFile) *TDefinitionParser {
	normalizedContent := strings.ReplaceAll(content, "\r\n", "\n")
	return &TDefinitionParser{
		filePath: filePath,
		lines:    strings.Split(normalizedContent, "\n"),
		index:    0,
		parsed:   parsed,
	}
}

// --- error reporting and recovery ---

func (p *TDefinitionParser) ReportParsingError(format string, args ...any) bool {
	lineNo := p.CurrentLineNumber()
	message := fmt.Sprintf("%s:%d: %s", p.filePath, lineNo, fmt.Sprintf(format, args...))
	p.parsed.Errors = append(p.parsed.Errors, message)
	return true
}

// --- primitive token-matching functions ---

func (p *TDefinitionParser) HasCurrentLine() bool {
	return p.index < len(p.lines)
}

func (p *TDefinitionParser) CurrentLine() string {
	if !p.HasCurrentLine() {
		return ""
	}
	return strings.TrimSpace(p.lines[p.index])
}

func (p *TDefinitionParser) CurrentLineNumber() int {
	if p.index < len(p.lines) {
		return p.index + 1
	}
	if len(p.lines) == 0 {
		return 1
	}
	return len(p.lines)
}

func (p *TDefinitionParser) Advance() bool {
	if p.HasCurrentLine() {
		p.index++
	}
	return true
}

func (p *TDefinitionParser) ThisEmptyLineWas() bool {
	return p.HasCurrentLine() && p.CurrentLine() == ""
}

func (p *TDefinitionParser) ThisEndWas() bool {
	return p.HasCurrentLine() && p.thisLineIs(EndToken+StatementEndToken)
}

func (p *TDefinitionParser) ThisCommentWas(text *string) bool {
	if p.HasCurrentLine() && strings.HasPrefix(p.CurrentLine(), CommentPrefix) {
		*text = p.CurrentLine()
		return true
	}
	return false
}

// --- token helpers ---

// CurrentTokens returns the whitespace-normalised tokens of the current line,
// with any trailing inline comment stripped first. Multiple spaces between tokens are collapsed.
func (p *TDefinitionParser) CurrentTokens() []string {
	return strings.Fields(stripTrailingInlineComment(p.CurrentLine()))
}

// tokenAt returns the n-th token (0-based) of the current line, or "" if out of range.
func (p *TDefinitionParser) tokenAt(n int) string {
	tokens := p.CurrentTokens()
	if n < 0 || n >= len(tokens) {
		return ""
	}
	return tokens[n]
}

// thisLineIs returns true if the current line consists of exactly the given tokens.
func (p *TDefinitionParser) thisLineIs(expected ...string) bool {
	tokens := p.CurrentTokens()
	if len(tokens) != len(expected) {
		return false
	}
	for i, t := range expected {
		if tokens[i] != t {
			return false
		}
	}
	return true
}

// thisLineStartsWith returns true if the current line's tokens begin with the given sequence.
// Any amount of whitespace between tokens in the source is accepted.
func (p *TDefinitionParser) thisLineStartsWith(prefix ...string) bool {
	tokens := p.CurrentTokens()
	if len(tokens) < len(prefix) {
		return false
	}
	for i, t := range prefix {
		if tokens[i] != t {
			return false
		}
	}
	return true
}

// --- line classification helpers ---

// isSpecialToken returns true for structural keywords and non-content lines that are never statements or block headers.
func (p *TDefinitionParser) isSpecialToken(line string) bool {
	return line == "" || line == EndToken+StatementEndToken || strings.HasPrefix(line, CommentPrefix)
}

// isBlockHeader returns true when the line introduces a block: it ends with ':'.
func (p *TDefinitionParser) isBlockHeader(line string) bool {
	return strings.HasSuffix(line, BlockHeaderSuffix)
}

func isControlLine(line string) bool {
	return line == ElseToken || strings.HasSuffix(line, " "+ThenToken) || strings.HasSuffix(line, " do")
}

func (p *TDefinitionParser) ThisStatementWas(text *string, consumedLines *int) bool {
	if !p.HasCurrentLine() {
		return false
	}

	startIndex := p.index
	parts := []string{}

	for cursor := startIndex; cursor < len(p.lines); cursor++ {
		line := strings.TrimSpace(p.lines[cursor])
		lineForSyntax := stripTrailingInlineComment(line)

		if cursor == startIndex && isControlLine(lineForSyntax) {
			*text = lineForSyntax
			*consumedLines = 1
			return true
		}

		if lineForSyntax == "" {
			if cursor == startIndex {
				return false
			}
			break
		}

		if cursor == startIndex {
			if p.isSpecialToken(lineForSyntax) || p.isBlockHeader(lineForSyntax) {
				return false
			}
		} else if p.isSpecialToken(lineForSyntax) || p.isBlockHeader(lineForSyntax) {
			break
		}

		parts = append(parts, lineForSyntax)
		if strings.HasSuffix(lineForSyntax, StatementEndToken) {
			*text = strings.Join(parts, " ")
			*consumedLines = cursor - startIndex + 1
			return true
		}
	}

	return false
}

func (p *TDefinitionParser) ThisHeaderWas(text *string) bool {
	line := p.CurrentLine()
	lineForSyntax := stripTrailingInlineComment(line)
	if p.isSpecialToken(lineForSyntax) || !p.isBlockHeader(lineForSyntax) {
		return false
	}
	*text = line
	return true
}

func stripTrailingInlineComment(line string) string {
	trimmedLine := strings.TrimSpace(line)
	commentIndex := strings.Index(trimmedLine, " #")
	if commentIndex >= 0 {
		return strings.TrimSpace(trimmedLine[:commentIndex])
	}
	return trimmedLine
}

// --- small helpers ---

func (p *TDefinitionParser) AppendNode(nodes *[]TNode, node TNode) bool {
	*nodes = append(*nodes, node)
	return true
}

// --- grammar rules ---

func (p *TDefinitionParser) Sequence(nodes *[]TNode, untilEnd bool) bool {
	for p.HasCurrentLine() {
		if p.ThisEmptyLineWas() {
			p.Advance()
			continue
		}

		if untilEnd && p.ThisEndWas() {
			p.Advance()
			return true
		}

		if p.Comment(nodes) || p.Conditional(nodes) || p.Block(nodes) || p.Statement(nodes) {
			continue
		}

		if !untilEnd && p.ThisEndWas() {
			p.ReportParsingError("unexpected %s at top level", EndToken+StatementEndToken)
			p.Advance()
			continue
		}

		p.ReportParsingError("unexpected line %q", p.CurrentLine())
		p.Advance()
	}

	if untilEnd {
		return p.ReportParsingError("unexpected end of file while parsing block")
	}
	return true
}

// ConditionalBody reads nodes until it sees end;, elif, or else — which belong to the enclosing conditional.
func (p *TDefinitionParser) ConditionalBody(nodes *[]TNode) bool {
	for p.HasCurrentLine() {
		if p.ThisEmptyLineWas() {
			p.Advance()
			continue
		}
		if p.ThisEndWas() || p.thisLineStartsWith(ElifToken) || p.thisLineIs(ElseToken) {
			return true
		}
		if p.Comment(nodes) || p.Conditional(nodes) || p.Block(nodes) || p.Statement(nodes) {
			continue
		}
		p.ReportParsingError("unexpected line %q in conditional branch", p.CurrentLine())
		p.Advance()
	}
	return true
}

// Conditional: IfToken IsToken UpToken "\"..." ThenToken ConditionalBody
//
//	(ElifToken IsToken UpToken "\"..." ThenToken ConditionalBody)*
//	(ElseToken ConditionalBody)? end;
//
// Any spacing between keywords is accepted. Macro-style if-expressions remain statements.
func (p *TDefinitionParser) Conditional(nodes *[]TNode) bool {
	if !p.thisLineStartsWith(IfToken, IsToken, UpToken) {
		return false
	}
	node := TNode{Kind: NodeBlock, SourceLine: p.CurrentLineNumber(), Text: p.CurrentLine()}
	p.Advance()
	p.ConditionalBody(&node.Children)

	for p.HasCurrentLine() && p.thisLineStartsWith(ElifToken, IsToken, UpToken) {
		branch := TNode{Kind: NodeBlock, SourceLine: p.CurrentLineNumber(), Text: p.CurrentLine()}
		p.Advance()
		p.ConditionalBody(&branch.Children)
		node.Children = append(node.Children, branch)
	}

	if p.HasCurrentLine() && p.thisLineIs(ElseToken) {
		branch := TNode{Kind: NodeBlock, SourceLine: p.CurrentLineNumber(), Text: p.CurrentLine()}
		p.Advance()
		p.ConditionalBody(&branch.Children)
		node.Children = append(node.Children, branch)
	}

	if p.ThisEndWas() {
		p.Advance()
	} else {
		p.ReportParsingError("expected %s to close if-block", EndToken+StatementEndToken)
	}

	return p.AppendNode(nodes, node)
}

// Comment: '#' .*
func (p *TDefinitionParser) Comment(nodes *[]TNode) bool {
	text := ""
	node := TNode{Kind: NodeComment, SourceLine: p.CurrentLineNumber()}
	return p.ThisCommentWas(&text) &&
		p.SetNodeText(&node, text) &&
		p.Advance() &&
		p.AppendNode(nodes, node)
}

// Statement: <line ending with ';'>
func (p *TDefinitionParser) Statement(nodes *[]TNode) bool {
	text := ""
	consumedLines := 0
	node := TNode{Kind: NodeStatement, SourceLine: p.CurrentLineNumber()}
	if !p.ThisStatementWas(&text, &consumedLines) {
		return false
	}
	if !p.SetNodeText(&node, text) {
		return false
	}
	for i := 0; i < consumedLines; i++ {
		p.Advance()
	}
	return p.AppendNode(nodes, node)
}

// Block: Header Sequence end;
func (p *TDefinitionParser) Block(nodes *[]TNode) bool {
	header := ""
	node := TNode{Kind: NodeBlock, SourceLine: p.CurrentLineNumber()}

	if !p.ThisHeaderWas(&header) {
		return false
	}

	if !p.SetNodeText(&node, header) || !p.Advance() {
		return false
	}

	// Colon-style blocks (e.g., "bridges:", "choose x:", "space ... with:").
	if p.isBlockHeader(stripTrailingInlineComment(header)) {
		return p.Sequence(&node.Children, true) && p.AppendNode(nodes, node)
	}

	return false
}

func (p *TDefinitionParser) SetNodeText(node *TNode, text string) bool {
	node.Text = text
	return true
}

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

	for i := 0; i < len(entityLines); i++ {
		trimmed := strings.TrimSpace(entityLines[i])
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if candidateMacroName, isCreateInvocation := extractCreateInvocationMacroName(trimmed); isCreateInvocation {
			if _, exists := ctx.Macros[candidateMacroName]; !exists {
				return TExpansionParseResult{}, fmt.Errorf("strict macro validation failed in %s at line %d: unknown macro %q in invocation %q", entitiesPath, i+1, candidateMacroName, trimmed)
			}
		}

		if entityDecl, ok := extractEntityDeclaration(trimmed); ok {
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
			}
		}

		if isMacroInvocation(trimmed, ctx.Macros) {
			invocationCount++
			report.WriteString(fmt.Sprintf("Line %d (in %s):\n", i+1, formatSpacePath(administration.SpacePath)))
			report.WriteString(fmt.Sprintf("  Invocation: %s\n", trimmed))

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

			invocation, parseErr := ctx.ParseMacroInvocation(invocationText)
			if parseErr != nil {
				return TExpansionParseResult{}, fmt.Errorf("strict macro validation failed in %s at line %d: failed to parse invocation %q: %w", entitiesPath, i+1, trimmed, parseErr)
			}

			if strictErr := ctx.ValidateInvocationStrict(invocation); strictErr != nil {
				return TExpansionParseResult{}, fmt.Errorf("strict macro validation failed in %s at line %d for invocation %q: %w", entitiesPath, i+1, trimmed, strictErr)
			}
			validInvocations++
			report.WriteString("  Status: OK (all parameters valid)\n")

			expandedRecords, expandErr := collectExpandedEntityRecords(ctx, invocation, administration.SpacePath, false)
			if expandErr != nil {
				return TExpansionParseResult{}, fmt.Errorf("strict macro validation failed in %s at line %d while expanding invocation %q: %w", entitiesPath, i+1, trimmed, expandErr)
			}
			for _, expandedRecord := range expandedRecords {
				administration.EnsureSpaceRegistered(expandedRecord.SpacePath, SpaceKindRegular)
				administration.AppendEntityRecord(formatNestedSpaceName(expandedRecord.SpacePath), expandedRecord.Record)
			}

			if len(invocation.Parameters) > 0 {
				report.WriteString("  Parameters:\n")
				for pkey, pval := range invocation.Parameters {
					report.WriteString(fmt.Sprintf("    %s = %q\n", pkey, pval))
				}
			}

			report.WriteString("\n")
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
			administration.RecordSpaceOff(spaceName, items)
			continue
		}

		// Handle "light on: ..." statements
		if strings.HasPrefix(trimmed, "light on:") {
			spaceName := administration.CurrentSpaceName()
			lightsStr := strings.TrimPrefix(trimmed, "light on:")
			lightsStr = strings.TrimSpace(lightsStr)
			lightsStr = strings.TrimSuffix(lightsStr, ";")
			lights := parseSpaceCollectionItems(lightsStr)
			administration.RecordSpaceOn(spaceName, lights)
			continue
		}

		if trimmed == EndToken+StatementEndToken {
			administration.HandleEndToken(onSpaceClosed)
		}
	}

	return TExpansionParseResult{
		Administration:   administration,
		InvocationCount:  invocationCount,
		ValidInvocations: validInvocations,
		TypeErrors:       typeErrors,
	}, nil
}
