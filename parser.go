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
 * Version of: 18.03.2026
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

// --- primitive line-matching functions ---

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

func (p *TDefinitionParser) ThisBeginWas() bool {
	return p.HasCurrentLine() && p.CurrentLine() == "begin"
}

func (p *TDefinitionParser) ThisEndWas() bool {
	return p.HasCurrentLine() && p.CurrentLine() == "end;"
}

func (p *TDefinitionParser) ThisCommentWas(text *string) bool {
	if p.HasCurrentLine() && strings.HasPrefix(p.CurrentLine(), "#") {
		*text = p.CurrentLine()
		return true
	}
	return false
}

func (p *TDefinitionParser) ThisStatementWas(text *string) bool {
	line := p.CurrentLine()
	lineForSyntax := stripTrailingInlineComment(line)
	if lineForSyntax == "" || lineForSyntax == "begin" || lineForSyntax == "end;" || strings.HasPrefix(lineForSyntax, "#") {
		return false
	}
	if strings.HasSuffix(lineForSyntax, ":") || p.NextNonCommentNonEmptyLine() == "begin" {
		return false
	}
	if p.HasCurrentLine() {
		*text = line
		return true
	}
	return false
}

func (p *TDefinitionParser) ThisHeaderWas(text *string) bool {
	line := p.CurrentLine()
	lineForSyntax := stripTrailingInlineComment(line)
	if lineForSyntax == "" || lineForSyntax == "begin" || lineForSyntax == "end;" || strings.HasPrefix(lineForSyntax, "#") {
		return false
	}
	if !strings.HasSuffix(lineForSyntax, ":") && p.NextNonCommentNonEmptyLine() != "begin" {
		return false
	}
	*text = line
	return true
}

func (p *TDefinitionParser) NextNonCommentNonEmptyLine() string {
	for lookAheadIndex := p.index + 1; lookAheadIndex < len(p.lines); lookAheadIndex++ {
		candidateLine := strings.TrimSpace(p.lines[lookAheadIndex])
		if candidateLine == "" || strings.HasPrefix(candidateLine, "#") {
			continue
		}
		return candidateLine
	}
	return ""
}

func stripTrailingInlineComment(line string) string {
	trimmedLine := strings.TrimSpace(line)
	commentIndex := strings.Index(trimmedLine, " #")
	if commentIndex >= 0 {
		return strings.TrimSpace(trimmedLine[:commentIndex])
	}
	return trimmedLine
}

// --- Forced versions: report error and skip on failure, returning false to abort the chain ---

func (p *TDefinitionParser) ForcedThisBeginWas() bool {
	return p.ThisBeginWas() || p.ReportParsingError("expected begin after block header, got %q", p.CurrentLine())
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

		if p.Comment(nodes) || p.Statement(nodes) || p.Block(nodes) {
			continue
		}

		if !untilEnd && p.ThisEndWas() {
			p.ReportParsingError("unexpected end; at top level")
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
	node := TNode{Kind: NodeStatement, SourceLine: p.CurrentLineNumber()}
	return p.ThisStatementWas(&text) &&
		p.SetNodeText(&node, text) &&
		p.Advance() &&
		p.AppendNode(nodes, node)
}

// Block: Header begin Sequence end;
func (p *TDefinitionParser) Block(nodes *[]TNode) bool {
	header := ""
	node := TNode{Kind: NodeBlock, SourceLine: p.CurrentLineNumber()}

	if !p.ThisHeaderWas(&header) {
		return false
	}

	if !p.SetNodeText(&node, header) || !p.Advance() {
		return false
	}

	// Colon-style blocks (e.g., "servers:", "bridges:", "choose x:", "space ... with:") do not use a begin line.
	if strings.HasSuffix(header, ":") {
		return p.Sequence(&node.Children, true) && p.AppendNode(nodes, node)
	}

	// Legacy begin/end style blocks (e.g., "space ..." or "declare entity ...") require begin.
	return p.ForcedThisBeginWas() &&
		p.Advance() &&
		p.Sequence(&node.Children, true) &&
		p.AppendNode(nodes, node)
}

func (p *TDefinitionParser) SetNodeText(node *TNode, text string) bool {
	node.Text = text
	return true
}
