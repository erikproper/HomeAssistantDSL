/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Main
 *
 * CLI entry point: accepts a path to a Main.def file and runs generation, or (with
 * "-would_define <entity_id>") checks whether a given entity_id would be defined/assumed by
 * this DSL without generating anything (would_define.go).
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 23.08.2026
 *
 */

package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error determining working directory: %v\n", err)
		os.Exit(1)
	}

	wouldDefine := flag.String("would_define", "", "check whether the given HA entity_id would be defined/assumed by this DSL, without generating any output")
	flag.Parse()

	args := flag.Args()
	if len(args) != 1 || !strings.HasSuffix(args[0], ".def") {
		fmt.Fprintf(os.Stderr, "usage: homeassistant [-would_define <entity_id>] <path/to/Main.def>\n")
		os.Exit(1)
	}

	if *wouldDefine != "" {
		found, err := runWouldDefineCheck(root, args[0], *wouldDefine)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		if !found {
			os.Exit(1)
		}
		return
	}

	if err := runGenerationFromDefFile(root, args[0]); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
