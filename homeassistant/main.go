/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Main
 *
 * CLI entry point: accepts a path to a Main.def file and runs generation, or (with
 * "-would_define <entity_id>") checks whether a given entity_id would be defined/assumed by
 * this DSL without generating anything (would_define.go), or (with
 * "-derived_migration_candidates") reports real battery_alert migration candidates for
 * plans/derived-capability-mechanism.md's rollout (derived_migration_helper.go), or (with
 * "-bootstrap_stable_ids <mapping.json>") runs PROJECT.md item 7's one-time stable-id bootstrap
 * (bootstrap_stable_ids.go).
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
	"path/filepath"
	"strings"
)

func main() {
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error determining working directory: %v\n", err)
		os.Exit(1)
	}

	wouldDefine := flag.String("would_define", "", "check whether the given HA entity_id would be defined/assumed by this DSL, without generating any output")
	derivedMigrationCandidates := flag.Bool("derived_migration_candidates", false, "report real battery_alert migration candidates (plans/derived-capability-mechanism.md), without generating any output or editing any file")
	bootstrapStableIDs := flag.String("bootstrap_stable_ids", "", "PROJECT.md item 7's one-time stable-id bootstrap: assign \"as <id>;\" to every live discovery capability from the given {entity_id: unique_id} registry mapping JSON file, run once per house")
	format := flag.Bool("format", false, "PROJECT.md 22c: format every .def file in the given .def's folder (def_format.go), without generating any output")
	flag.Parse()

	args := flag.Args()
	if len(args) != 1 || !strings.HasSuffix(args[0], ".def") {
		fmt.Fprintf(os.Stderr, "usage: homeassistant [-format] [-would_define <entity_id>] [-derived_migration_candidates] [-bootstrap_stable_ids <mapping.json>] <path/to/Main.def>\n")
		os.Exit(1)
	}

	if *format {
		if err := runFormatDefs(filepath.Dir(args[0])); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		return
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

	if *derivedMigrationCandidates {
		if err := runBatteryAlertMigrationReport(root, args[0]); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		return
	}

	if *bootstrapStableIDs != "" {
		if err := runBootstrapStableIDs(root, args[0], *bootstrapStableIDs); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := runGenerationFromDefFile(root, args[0]); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
