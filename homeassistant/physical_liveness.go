/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: PhysicalLiveness
 *
 * PROJECT.md 25c (design: claude-notes/plans/gateway-liveness.md, agreed with Erik 2026-11-01): a
 * gateway device's "liveness with: ... end;" block in Physical.def says how the coordinator checks
 * that the gateway is working -- also when it hangs while still connected, which its Last Will
 * doesn't show:
 *
 *   device integration.zigbee2mqtt with:
 *     ...
 *     liveness with:
 *       traffic "zigbee2mqtt/#" quiet for 00:05:00;
 *       ignore "zigbee2mqtt/#/set", "zigbee2mqtt/#/retain";
 *       ask "zigbee2mqtt/bridge/request/health_check" with "";
 *       answer on "zigbee2mqtt/bridge/response/health_check" has data.healthy within 00:00:30;
 *       verdict on "zigbee2mqtt/bridge/state" online "{\"state\":\"online\"}" offline "{\"state\":\"offline\"}";
 *     end;
 *   end;
 *
 * "answer ... is "<payload>"" for an exact answer instead of "has <field>" (a JSON field that must
 * be true). Two unanswered questions in a row make the gateway offline.
 * The generator writes every declared block to coordinator/liveness.yaml; the coordinator
 * (house_event_bus_coordinator/liveness.go) has no gateway-specific code.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 01.11.2026
 *
 */

package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// livenessBlockHeader opens the block inside a gateway device.
const livenessBlockHeader = "liveness with:"

// livenessDefaultMisses is how many unanswered questions make a gateway offline: one lost answer
// must not make all its entities unavailable.
const livenessDefaultMisses = 2

// TLivenessDecl is one gateway's "liveness with:" block; also an entry of coordinator/liveness.yaml.
type TLivenessDecl struct {
	Device        string   `yaml:"device"`
	Traffic       string   `yaml:"traffic"`
	Ignore        []string `yaml:"ignore,omitempty"`
	Quiet         string   `yaml:"quiet"`
	Ask           string   `yaml:"ask"`
	AskPayload    string   `yaml:"ask_payload"`
	Answer        string   `yaml:"answer"`
	AnswerPayload string   `yaml:"answer_payload,omitempty"`
	AnswerField   string   `yaml:"answer_field,omitempty"`
	Within        string   `yaml:"within"`
	Verdict       string   `yaml:"verdict"`
	Online        string   `yaml:"online"`
	Offline       string   `yaml:"offline"`
	Misses        int      `yaml:"misses"`
}

// TLivenessFile is coordinator/liveness.yaml.
type TLivenessFile struct {
	Gateways []TLivenessDecl `yaml:"gateways"`
}

const livenessQuoted = `"((?:[^"\\]|\\.)*)"`
const livenessDuration = `(\d{1,2}:\d{2}:\d{2})`

var (
	livenessTrafficPattern = regexp.MustCompile(`^traffic\s+` + livenessQuoted + `\s+quiet\s+for\s+` + livenessDuration + `\s*;$`)
	livenessIgnorePattern  = regexp.MustCompile(`^ignore\s+(.+?)\s*;$`)
	livenessAskPattern     = regexp.MustCompile(`^ask\s+` + livenessQuoted + `\s+with\s+` + livenessQuoted + `\s*;$`)
	livenessAnswerPattern  = regexp.MustCompile(`^answer\s+on\s+` + livenessQuoted + `\s+(?:is\s+` + livenessQuoted + `|has\s+([A-Za-z0-9_.]+))\s+within\s+` + livenessDuration + `\s*;$`)
	livenessVerdictPattern = regexp.MustCompile(`^verdict\s+on\s+` + livenessQuoted + `\s+online\s+` + livenessQuoted + `\s+offline\s+` + livenessQuoted + `\s*;$`)
	livenessQuotedItem     = regexp.MustCompile(`^\s*` + livenessQuoted + `\s*$`)
)

// livenessUnquote undoes a quoted string's escapes (\" and \\).
func livenessUnquote(s string) string {
	if unquoted, err := strconv.Unquote(`"` + s + `"`); err == nil {
		return unquoted
	}
	return s
}

// parseLivenessBlock parses a "liveness with: ... end;" body (cur just past its header) for
// deviceID; warnings report an unknown line or a missing statement.
func parseLivenessBlock(cur *TLineCursor, deviceID string) (*TLivenessDecl, []string) {
	decl := &TLivenessDecl{Device: deviceID, Misses: livenessDefaultMisses}
	var warnings []string
	warn := func(format string, args ...interface{}) {
		warnings = append(warnings, fmt.Sprintf("Physical.def: device %q, \"%s\": ", deviceID, livenessBlockHeader)+fmt.Sprintf(format, args...))
	}
	for !cur.AtEnd() {
		line := cur.Advance()
		if line == "end;" {
			for i, value := range []string{decl.Traffic, decl.Ask, decl.Answer, decl.Verdict} {
				if value == "" {
					warn("no %q line -- the coordinator won't check this gateway", []string{"traffic", "ask", "answer", "verdict"}[i])
					return nil, warnings
				}
			}
			return decl, warnings
		}
		if m := livenessTrafficPattern.FindStringSubmatch(line); m != nil {
			decl.Traffic, decl.Quiet = livenessUnquote(m[1]), m[2]
		} else if m := livenessAskPattern.FindStringSubmatch(line); m != nil {
			decl.Ask, decl.AskPayload = livenessUnquote(m[1]), livenessUnquote(m[2])
		} else if m := livenessAnswerPattern.FindStringSubmatch(line); m != nil {
			decl.Answer, decl.Within = livenessUnquote(m[1]), m[4]
			if m[3] != "" {
				decl.AnswerField = m[3]
			} else {
				decl.AnswerPayload = livenessUnquote(m[2])
			}
		} else if m := livenessVerdictPattern.FindStringSubmatch(line); m != nil {
			decl.Verdict, decl.Online, decl.Offline = livenessUnquote(m[1]), livenessUnquote(m[2]), livenessUnquote(m[3])
		} else if m := livenessIgnorePattern.FindStringSubmatch(line); m != nil {
			for _, item := range strings.Split(m[1], ",") {
				if q := livenessQuotedItem.FindStringSubmatch(item); q != nil {
					decl.Ignore = append(decl.Ignore, livenessUnquote(q[1]))
				} else {
					warn("\"ignore\" takes quoted MQTT filters, separated by commas; ignoring %q", strings.TrimSpace(item))
				}
			}
		} else {
			warn("unrecognised line %q", line)
		}
	}
	warn("no closing \"end;\"")
	return nil, warnings
}

// writeLivenessFile writes coordinator/liveness.yaml from the gateways' blocks (none: an empty list,
// so a removed block stops being checked).
func writeLivenessFile(outputRoot string, gateways map[string]TDiscoveryGatewayDevice) error {
	file := TLivenessFile{Gateways: []TLivenessDecl{}}
	for _, gateway := range gateways {
		if gateway.Liveness != nil {
			file.Gateways = append(file.Gateways, *gateway.Liveness)
		}
	}
	sort.Slice(file.Gateways, func(i, j int) bool { return file.Gateways[i].Device < file.Gateways[j].Device })
	data, err := yaml.Marshal(file)
	if err != nil {
		return err
	}
	return writeYAMLFile(filepath.Join(outputRoot, "coordinator", "liveness.yaml"), generatorHeader+string(data))
}
