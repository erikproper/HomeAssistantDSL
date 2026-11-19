/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Documentation
 *
 * The house's handover document, as LaTeX for the report class, into the house's Documentation
 * folder when it exists: body.tex (a chapter per section, each opening with the author's own
 * <section>.tex from that folder when there is one) and house.sty (the name macros). The author's
 * main.tex there sets the layout, loads house.sty and inputs body.tex; when it is there, pdflatex
 * typesets it into main.pdf, which is copied, as <installation>.pdf, into the folder the setting
 * ${documentation} names, if set. Text refers to entities and devices only through the
 * macros, so a renamed or removed one is an error in the LaTeX run (Architecture.md, "The
 * handover document").
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Vienna, Austria, in collaboration with Claude.ai
 *
 * Version of: 19.11.2026
 *
 */

package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// TDocumentationSection is one section of the handover document: its title, the name of the
// author's own fragment (<file>.tex), what the generator writes after it, and whether it is an
// appendix.
type TDocumentationSection struct {
	Title     string
	File      string
	Generated func(d *TDocumentation) string
	Appendix  bool
}

// TDocumentationEntity is what the document knows of one entity.
type TDocumentationEntity struct {
	Key          string // the DSL's absolute form, e.g. "switch.social/apartment/kitchen/nespresso"
	EntityID     string
	FriendlyName string
	Device       string // the device it belongs to, "" when not known
}

// TDocumentation holds what the document is written from.
type TDocumentation struct {
	Spaces          []string                          // the spaces holding entities, in definition order
	EntitiesBySpace map[string][]TDocumentationEntity // space -> its entities, in definition order
	DeviceMetadata  map[string]map[string]string      // device id -> metadata field -> value
}

// documentationSections is the document's agreed section structure; the appendices come last.
var documentationSections = []TDocumentationSection{
	{Title: "First steps", File: "first-steps"},
	{Title: "The house at a glance", File: "at-a-glance"},
	{Title: "Access", File: "access"},
	{Title: "How it fits together", File: "structure"},
	{Title: "Everyday tasks", File: "tasks"},
	{Title: "When something is down", File: "troubleshooting"},
	{Title: "Contacts and suppliers", File: "contacts"},
	{Title: "Rooms and devices", File: "rooms", Generated: documentationRooms, Appendix: true},
}

// documentationStyle is house.sty's fixed part: the lookup behind every name macro.
const documentationStyle = `\NeedsTeXFormat{LaTeX2e}
\ProvidesPackage{house}[Generated from the house's definitions; don't edit]
\RequirePackage{url}
\def\UrlBreaks{\do\_\do\.\do\/\do\:}
\urlstyle{tt}

% \HouseLookup{kind}{key}: what the definitions say for key; an unknown key -- renamed or removed --
% stops the run, so the text gets updated.
\newcommand{\HouseLookup}[2]{%
    \ifcsname house@#1@#2\endcsname
        \csname house@#1@#2\endcsname
    \else
        \PackageError{house}{Unknown #1 '\detokenize{#2}'}{It was renamed or removed: update the text.}%
    \fi
}
\newcommand{\EntityName}[1]{\HouseLookup{entity}{#1}}
\newcommand{\FriendlyName}[1]{\HouseLookup{name}{#1}}
\newcommand{\DeviceName}[1]{\HouseLookup{device}{#1}}
\newcommand{\DeviceOf}[1]{\HouseLookup{deviceof}{#1}}
\newcommand{\ManufacturerOf}[1]{\HouseLookup{manufacturer}{#1}}
\newcommand{\ModelOf}[1]{\HouseLookup{model}{#1}}
\newcommand{\HouseFragmentMissing}[1]{\textit{(No text in #1.tex yet.)}}

`

// --- top level ---

// writeHouseDocumentation writes body.tex and house.sty into houseRoot's Documentation folder;
// without that folder, the house has no document and nothing is written.
func writeHouseDocumentation(houseRoot, definitionDir string, admin *TAdministrationState) error {
	dir := filepath.Join(houseRoot, "Documentation")
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return nil
	}
	d := newDocumentation(definitionDir, admin)
	if err := os.WriteFile(filepath.Join(dir, "house.sty"), []byte(d.Style()), 0o644); err != nil {
		return fmt.Errorf("writing house.sty: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "body.tex"), []byte(d.Body()), 0o644); err != nil {
		return fmt.Errorf("writing body.tex: %w", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "main.tex")); err != nil {
		return nil
	}
	lines, typeset := typesetHouseDocumentation(dir)
	settings := parseDefinitionAssignments(readCombinedSettingsContent(definitionDir))
	if target := settingOr(settings, "documentation", ""); typeset && target != "" {
		installation := settingOr(settings, "installation", filepath.Base(houseRoot))
		lines = append(lines, copyHouseDocumentation(dir, os.ExpandEnv(target), installation))
	}
	for _, line := range lines {
		fmt.Printf("[documentation] %s\n", line)
	}
	return nil
}

// copyHouseDocumentation copies dir's main.pdf into the folder target, as <installation>.pdf, and
// says where it went.
func copyHouseDocumentation(dir, target, installation string) string {
	pdf, err := os.ReadFile(filepath.Join(dir, "main.pdf"))
	if err != nil {
		return "main.pdf not copied: " + err.Error()
	}
	if info, err := os.Stat(target); err != nil || !info.IsDir() {
		return "main.pdf not copied: ${documentation} (" + homeRelative(target) + ") is not a folder"
	}
	destination := filepath.Join(target, installation+".pdf")
	if err := os.WriteFile(destination, pdf, 0o644); err != nil {
		return "main.pdf not copied: " + err.Error()
	}
	return "copied to " + homeRelative(destination)
}

// documentationRuns is how often pdflatex runs at most: until the table of contents and the
// references are stable.
const documentationRuns = 3

// documentationRunTimeout bounds one pdflatex run.
const documentationRunTimeout = 2 * time.Minute

// documentationPages finds the page count in pdflatex's log.
var documentationPages = regexp.MustCompile(`Output written on main\.pdf \((\d+) pages?`)

// typesetHouseDocumentation runs pdflatex on dir's main.tex, again while its .aux or .toc still
// changes, and says how it went: the page count, or the LaTeX errors; typeset is whether main.pdf
// came out without errors. An error doesn't stop the generation; the text is then updated.
func typesetHouseDocumentation(dir string) (lines []string, typeset bool) {
	pdflatex, err := exec.LookPath("pdflatex")
	if err != nil {
		return []string{"main.tex not typeset: no pdflatex on the PATH"}, false
	}
	var log []byte
	for run := 0; run < documentationRuns; run++ {
		before := documentationState(dir)
		ctx, cancel := context.WithTimeout(context.Background(), documentationRunTimeout)
		cmd := exec.CommandContext(ctx, pdflatex, "-interaction=nonstopmode", "-file-line-error", "main.tex")
		cmd.Dir = dir
		// Whole error lines in the log, not wrapped at 79 characters.
		cmd.Env = append(os.Environ(), "max_print_line=10000")
		// A LaTeX error exits non-zero; the log says what it was.
		_ = cmd.Run()
		timedOut := ctx.Err() == context.DeadlineExceeded
		cancel()
		if timedOut {
			return []string{"main.tex: pdflatex didn't finish within " + documentationRunTimeout.String()}, false
		}
		if log, err = os.ReadFile(filepath.Join(dir, "main.log")); err != nil {
			return []string{"main.tex: no main.log: " + err.Error()}, false
		}
		if bytes.Equal(before, documentationState(dir)) {
			break
		}
	}
	if errors := documentationErrors(log); len(errors) > 0 {
		return errors, false
	}
	if m := documentationPages.FindSubmatch(log); m != nil {
		return []string{fmt.Sprintf("main.pdf: %s pages", m[1])}, true
	}
	return []string{"main.tex typeset, but no main.pdf written (see main.log)"}, false
}

// documentationState is dir's .aux and .toc together: a further run is needed while it changes.
func documentationState(dir string) []byte {
	var state []byte
	for _, name := range []string{"main.aux", "main.toc"} {
		content, _ := os.ReadFile(filepath.Join(dir, name))
		state = append(append(state, content...), 0)
	}
	return state
}

// documentationErrors are the distinct errors in pdflatex's log (with -file-line-error, the lines
// "<file>:<line>: <message>").
func documentationErrors(log []byte) []string {
	var errors []string
	seen := map[string]bool{}
	for _, line := range strings.Split(string(log), "\n") {
		file, rest, found := strings.Cut(line, ".tex:")
		if !found || strings.ContainsAny(file, " ") {
			continue
		}
		number, message, found := strings.Cut(rest, ": ")
		if !found || strings.Trim(number, "0123456789") != "" || number == "" {
			continue
		}
		entry := fmt.Sprintf("%s.tex:%s: %s", filepath.Base(file), number, message)
		if !seen[entry] {
			seen[entry] = true
			errors = append(errors, entry)
		}
	}
	return errors
}

// newDocumentation collects the spaces, their entities and the devices' metadata.
func newDocumentation(definitionDir string, admin *TAdministrationState) *TDocumentation {
	d := &TDocumentation{EntitiesBySpace: map[string][]TDocumentationEntity{}, DeviceMetadata: map[string]map[string]string{}}
	seen := map[string]bool{}
	for _, space := range admin.SpaceOrder {
		for _, record := range admin.EntityRecordsBySpace[space] {
			entity := documentationEntity(record, admin)
			if entity.EntityID == "" || seen[entity.Key] {
				continue
			}
			seen[entity.Key] = true
			if len(d.EntitiesBySpace[space]) == 0 {
				d.Spaces = append(d.Spaces, space)
			}
			d.EntitiesBySpace[space] = append(d.EntitiesBySpace[space], entity)
		}
	}
	gateways, _ := collectDiscoveryGatewaysByID(definitionDir)
	for id, device := range gateways {
		d.addMetadata(id, device.Metadata)
	}
	hosts, _ := collectHostsDevicesByID(definitionDir)
	for id, device := range hosts {
		d.addMetadata(id, device.ConstantAttributes)
	}
	return d
}

// documentationEntity is record as the document names it, with the device it belongs to.
func documentationEntity(record TEntityRecord, admin *TAdministrationState) TDocumentationEntity {
	identity := record.Identity
	key := identity.Domain + "." + friendlyEntityName(identity)
	entity := TDocumentationEntity{Key: key, EntityID: toHomeAssistantEntityID(key), FriendlyName: friendlyEntityName(identity)}
	if device, _, found := strings.Cut(record.SourceKey, "!"); found {
		entity.Device = device
	} else if link, ok := admin.DiscoveryEntityLinks[entity.EntityID]; ok {
		entity.Device = link.GatewayDeviceID
	}
	return entity
}

func (d *TDocumentation) addMetadata(deviceID string, metadata map[string]THostConstantAttribute) {
	fields := map[string]string{}
	for name, attribute := range metadata {
		fields[name] = attribute.Value
	}
	d.DeviceMetadata[deviceID] = fields
}

// --- house.sty ---

// Style is house.sty: the lookup macros, then a definition per entity and device.
func (d *TDocumentation) Style() string {
	var b strings.Builder
	b.WriteString(documentationStyle)
	define := func(kind, key, value string) {
		fmt.Fprintf(&b, "\\@namedef{house@%s@%s}{%s}\n", kind, key, value)
	}
	devices := map[string]bool{}
	for _, space := range d.Spaces {
		for _, entity := range d.EntitiesBySpace[space] {
			define("entity", entity.Key, latexPath(entity.EntityID))
			define("name", entity.Key, latexPath(entity.FriendlyName))
			if entity.Device != "" {
				devices[entity.Device] = true
				define("deviceof", entity.Key, latexPath(entity.Device))
				d.defineMetadata(define, entity.Key, entity.Device)
			}
		}
	}
	for _, device := range sortedKeys(devices) {
		define("device", device, latexPath(device))
		d.defineMetadata(define, device, device)
	}
	return b.String()
}

// defineMetadata defines key's manufacturer and model, those of device, as far as known.
func (d *TDocumentation) defineMetadata(define func(kind, key, value string), key, device string) {
	for _, field := range []string{"manufacturer", "model"} {
		if value := d.DeviceMetadata[device][field]; value != "" {
			define(field, key, latexText(value))
		}
	}
}

// --- body.tex ---

// Body is body.tex: every section, the author's fragment first, then the generated part; the
// appendices after \appendix.
func (d *TDocumentation) Body() string {
	var b strings.Builder
	b.WriteString("% Generated from the house's definitions; don't edit. Your own text goes in <section>.tex.\n")
	inAppendix := false
	for _, section := range documentationSections {
		if section.Appendix && !inAppendix {
			b.WriteString("\n\\appendix\n")
			inAppendix = true
		}
		fmt.Fprintf(&b, "\n\\chapter{%s}\\label{chap:%s}\n\n", section.Title, section.File)
		if section.Generated == nil {
			fmt.Fprintf(&b, "\\InputIfFileExists{%s.tex}{}{\\HouseFragmentMissing{%s}}\n", section.File, section.File)
			continue
		}
		fmt.Fprintf(&b, "\\InputIfFileExists{%s.tex}{}{}\n", section.File)
		b.WriteString(section.Generated(d))
	}
	return b.String()
}

// documentationRooms lists, per space, its entities with their entity_ids and devices.
func documentationRooms(d *TDocumentation) string {
	var b strings.Builder
	for _, space := range d.Spaces {
		fmt.Fprintf(&b, "\n\\section{%s}\n\n\\begin{itemize}\n", latexText(space))
		for _, entity := range d.EntitiesBySpace[space] {
			fmt.Fprintf(&b, "    \\item \\FriendlyName{%s}: \\EntityName{%s}", entity.Key, entity.Key)
			if entity.Device != "" {
				fmt.Fprintf(&b, ", of \\DeviceOf{%s}", entity.Key)
				var made []string
				for _, field := range []struct{ name, macro string }{{"manufacturer", "ManufacturerOf"}, {"model", "ModelOf"}} {
					if d.DeviceMetadata[entity.Device][field.name] != "" {
						made = append(made, fmt.Sprintf("\\%s{%s}", field.macro, entity.Key))
					}
				}
				if len(made) > 0 {
					fmt.Fprintf(&b, " (%s)", strings.Join(made, " "))
				}
			}
			b.WriteString("\n")
		}
		b.WriteString("\\end{itemize}\n")
	}
	return b.String()
}

// --- small helpers ---

// latexPath sets a built-in name in typewriter type, breakable at its separators.
func latexPath(name string) string {
	return "\\path{" + name + "}"
}

// latexEscapes are the characters plain LaTeX text can't hold as they are.
var latexEscapes = strings.NewReplacer(
	`\`, `\textbackslash{}`, `&`, `\&`, `%`, `\%`, `$`, `\$`, `#`, `\#`, `_`, `\_`,
	`{`, `\{`, `}`, `\}`, `~`, `\textasciitilde{}`, `^`, `\textasciicircum{}`,
)

// latexText is value as plain LaTeX text.
func latexText(value string) string {
	return latexEscapes.Replace(value)
}
