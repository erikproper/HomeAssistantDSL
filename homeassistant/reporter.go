/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: Reporter
 *
 * Centralized warning reporting for parsing and administration.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 22.03.2026
 *
 */

package main

import (
	"fmt"
)

type TReporter struct{}

var Reporter = &TReporter{}

func (r *TReporter) Warn(format string, args ...interface{}) {
	fmt.Printf("[WARNING] "+format+"\n", args...)
}
