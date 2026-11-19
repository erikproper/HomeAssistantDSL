/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Documentation (tests)
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Vienna, Austria, in collaboration with Claude.ai
 *
 * Version of: 19.11.2026
 *
 */

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDocumentationErrors(t *testing.T) {
	log := []byte(`(./body.tex
(./first-steps.tex
./first-steps.tex:1: Package house Error: Unknown entity 'switch.social/apartment/kitchen/nope'.
See the house package documentation for explanation.
./first-steps.tex:1: Package house Error: Unknown entity 'switch.social/apartment/kitchen/nope'.
./body.tex:12: Undefined control sequence.
Overfull \hbox (12.3pt too wide) in paragraph at lines 40--41
LaTeX Font Info:    External font cmex10 loaded for size (Font) <7> on input line 3.
`)
	want := []string{
		"first-steps.tex:1: Package house Error: Unknown entity 'switch.social/apartment/kitchen/nope'.",
		"body.tex:12: Undefined control sequence.",
	}
	if got := documentationErrors(log); !reflect.DeepEqual(got, want) {
		t.Errorf("documentationErrors:\n got %q\nwant %q", got, want)
	}
}

func TestTypesetHouseDocumentationReportsUnknownName(t *testing.T) {
	if _, err := exec.LookPath("pdflatex"); err != nil {
		t.Skip("no pdflatex")
	}
	dir := t.TempDir()
	d := &TDocumentation{EntitiesBySpace: map[string][]TDocumentationEntity{}, DeviceMetadata: map[string]map[string]string{}}
	files := map[string]string{
		"main.tex":         "\\documentclass{report}\\usepackage{house}\\begin{document}\\input{body}\\end{document}\n",
		"house.sty":        d.Style(),
		"body.tex":         d.Body(),
		"introduction.tex": "\\EntityName{switch.social/nope}\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, typeset := typesetHouseDocumentation(dir)
	if typeset || len(got) != 1 || !strings.Contains(got[0], "introduction.tex:1: Package house Error: Unknown entity 'switch.social/nope'") {
		t.Errorf("typesetHouseDocumentation: got %q", got)
	}
}

func TestCopyHouseDocumentation(t *testing.T) {
	dir, target := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.pdf"), []byte("%PDF"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := copyHouseDocumentation(dir, filepath.Join(target, "missing"), "vienna"); !strings.Contains(got, "is not a folder") {
		t.Errorf("into a missing folder: got %q", got)
	}
	copyHouseDocumentation(dir, target, "vienna")
	if pdf, err := os.ReadFile(filepath.Join(target, "vienna.pdf")); err != nil || string(pdf) != "%PDF" {
		t.Errorf("vienna.pdf: %q, %v", pdf, err)
	}
}
