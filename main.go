/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Main
 *
 * CLI entry point: parses command-line arguments and dispatches to the appropriate runner.
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

	if err := runCLI(root, os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func runCLI(root string, args []string) error {
	args = applyDebugOptionArgs(args)
	if DebugEnabled {
		if err := consolidateLegacyDebugReports(root, HouseNames); err != nil {
			return err
		}
	}

	if len(args) == 0 {
		return runInterpretation(root, HouseNames)
	}

	command := strings.ToLower(strings.TrimSpace(args[0]))
	switch command {
	case "interpret", "parse":
		houses, err := resolveRequestedHouses(args[1:])
		if err != nil {
			return err
		}
		return runInterpretation(root, houses)
	case "expand":
		houses, err := resolveRequestedHouses(args[1:])
		if err != nil {
			return err
		}
		return runExpansion(root, houses)
	case "check":
		houses, err := resolveRequestedHouses(args[1:])
		if err != nil {
			return err
		}
		return runDefinedCheck(root, houses)
	case "generate", "gen":
		// Local mode: homeassistant generate Definitions/Main.def
		if len(args) > 1 && strings.HasSuffix(args[1], ".def") {
			return runGenerationFromDefFile(root, args[1])
		}
		// Legacy mode: homeassistant generate [Vienna|Junglinster]
		houses, err := resolveRequestedHouses(args[1:])
		if err != nil {
			return err
		}
		return runGeneration(root, houses)
	default:
		return fmt.Errorf("unknown command %q (allowed: interpret, expand, check, generate)", command)
	}
}

func resolveRequestedHouses(args []string) ([]string, error) {
	if len(args) == 0 {
		return append([]string{}, HouseNames...), nil
	}

	knownByLower := map[string]string{}
	for _, houseName := range HouseNames {
		knownByLower[strings.ToLower(houseName)] = houseName
	}

	resolvedHouses := []string{}
	seen := map[string]bool{}
	for _, rawArg := range args {
		normalizedArg := strings.ToLower(strings.TrimSpace(rawArg))
		houseName, ok := knownByLower[normalizedArg]
		if !ok {
			return nil, fmt.Errorf("unknown house %q (allowed: %s)", rawArg, strings.Join(HouseNames, ", "))
		}
		if !seen[houseName] {
			seen[houseName] = true
			resolvedHouses = append(resolvedHouses, houseName)
		}
	}
	return resolvedHouses, nil
}
