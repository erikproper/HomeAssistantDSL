/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Debug
 *
 * This component centralizes development-time debug reports and related CLI debug options.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 22.03.2026
 *
 */

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	DebugReportInterpretation = "Interpretation"
	DebugReportExpansion      = "Expansion"
	DebugReportCollections    = "Collections"
	DebugReportDefined        = "Defined"
	DebugIndexFileName        = "DebugReports.txt"
)

var DebugReportNames = []string{
	DebugReportInterpretation,
	DebugReportExpansion,
	DebugReportCollections,
	DebugReportDefined,
}

// DebugEnabled controls whether development reports are written.
// Defaults to false; pass --debug on the command line to enable.
var DebugEnabled = false

func applyDebugOptionArgs(args []string) []string {
	filtered := []string{}
	debugEnabled := DebugEnabled

	for _, rawArg := range args {
		normalizedArg := strings.ToLower(strings.TrimSpace(rawArg))
		switch normalizedArg {
		case "-debug", "--debug":
			debugEnabled = true
		case "-no-debug", "--no-debug", "-nodebug", "--nodebug":
			debugEnabled = false
		default:
			filtered = append(filtered, rawArg)
		}
	}

	DebugEnabled = debugEnabled
	return filtered
}

func debugReportPath(root, house, reportName string) string {
	return filepath.Join(debugHouseRoot(root, house), debugReportFileName(reportName))
}

func writeDebugReport(root, house, reportName, content string) (string, error) {
	if !DebugEnabled {
		return "", nil
	}

	reportPath := debugReportPath(root, house, reportName)
	if err := os.MkdirAll(filepath.Dir(reportPath), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(reportPath, []byte(content), 0o644); err != nil {
		return "", err
	}
	if err := writeDebugIndex(root, house); err != nil {
		return "", err
	}

	return reportPath, nil
}

func debugHouseRoot(root, house string) string {
	return filepath.Join(root, "New", house)
}

func debugReportFileName(reportName string) string {
	if reportName == DebugReportInterpretation {
		return "Interpretation.txt"
	}
	return reportName + ".txt"
}

func oldRootDebugReportPath(root, house, reportName string) string {
	return filepath.Join(root, fmt.Sprintf("%s.%s.txt", reportName, house))
}

func legacyDebugReportPath(root, house, reportName string) string {
	if reportName == DebugReportInterpretation {
		return filepath.Join(root, "New", house, "interpretation.txt")
	}
	return filepath.Join(root, "New", house, "Definitions", reportName+".txt")
}

func consolidateLegacyDebugReports(root string, houses []string) error {
	oldRootIndexPath := filepath.Join(root, DebugIndexFileName)
	if removeErr := os.Remove(oldRootIndexPath); removeErr != nil && !os.IsNotExist(removeErr) {
		return removeErr
	}

	for _, house := range houses {
		for _, reportName := range DebugReportNames {
			targetPath := debugReportPath(root, house, reportName)
			candidates := []string{targetPath, oldRootDebugReportPath(root, house, reportName), legacyDebugReportPath(root, house, reportName)}

			newestPath := ""
			var newestModTime time.Time
			for _, candidate := range candidates {
				info, statErr := os.Stat(candidate)
				if statErr != nil {
					if os.IsNotExist(statErr) {
						continue
					}
					return statErr
				}
				if newestPath == "" || info.ModTime().After(newestModTime) {
					newestPath = candidate
					newestModTime = info.ModTime()
				}
			}

			if newestPath != "" && newestPath != targetPath {
				content, readErr := os.ReadFile(newestPath)
				if readErr != nil {
					return readErr
				}
				if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
					return err
				}
				if writeErr := os.WriteFile(targetPath, content, 0o644); writeErr != nil {
					return writeErr
				}
			}

			for _, candidate := range candidates {
				if candidate == targetPath {
					continue
				}
				if removeErr := os.Remove(candidate); removeErr != nil && !os.IsNotExist(removeErr) {
					return removeErr
				}
			}
		}
		if err := writeDebugIndex(root, house); err != nil {
			return err
		}
	}

	return nil
}

func writeDebugIndex(root, house string) error {
	houseRoot := debugHouseRoot(root, house)
	if err := os.MkdirAll(houseRoot, 0o755); err != nil {
		return err
	}
	indexPath := filepath.Join(houseRoot, DebugIndexFileName)
	builder := strings.Builder{}
	builder.WriteString("=== DEBUG REPORT INDEX ===\n\n")
	builder.WriteString(fmt.Sprintf("Updated: %s\n", time.Now().Format(time.RFC3339)))
	builder.WriteString(fmt.Sprintf("Debug enabled: %t\n", DebugEnabled))
	builder.WriteString(fmt.Sprintf("House root: %s\n\n", houseRoot))
	builder.WriteString("Report files:\n\n")

	availablePaths := []string{}
	for _, reportName := range DebugReportNames {
		reportPath := debugReportPath(root, house, reportName)
		if info, err := os.Stat(reportPath); err == nil {
			availablePaths = append(availablePaths, fmt.Sprintf("- %s (%d bytes)", reportPath, info.Size()))
		}
	}
	sort.Strings(availablePaths)
	if len(availablePaths) == 0 {
		builder.WriteString("- (none)\n")
	} else {
		for _, line := range availablePaths {
			builder.WriteString(line)
			builder.WriteString("\n")
		}
	}

	return os.WriteFile(indexPath, []byte(builder.String()), 0o644)
}
