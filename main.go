/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Main
 *
 * CLI entry point: accepts a path to a Main.def file and runs generation.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 24.03.2026
 *
 */

package main

import (
	"fmt"
	"os"
	"strings"
)

var HouseNames = []string{"Vienna", "Junglinster"}

func main() {
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error determining working directory: %v\n", err)
		os.Exit(1)
	}

	args := applyDebugOptionArgs(os.Args[1:])
	if DebugEnabled {
		if err := consolidateLegacyDebugReports(root, HouseNames); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
	}

	if len(args) != 1 || !strings.HasSuffix(args[0], ".def") {
		fmt.Fprintf(os.Stderr, "usage: homeassistant <path/to/Main.def>\n")
		os.Exit(1)
	}

	if err := runGenerationFromDefFile(root, args[0]); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
