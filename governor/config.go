/*
 *
 * Module:    Governor
 * Package:   Main
 * Component: Config
 *
 * The governor's configuration, written by the generator ("integration governed" in Physical.def,
 * PROJECT.md 37, claude-notes/plans/governed-integration.md): devices.yaml -- the governed
 * entities, their topics, defaults, enforcement and the paths (radios) they are reached through --
 * and secrets.yaml, the main broker, in the coordinator's shape. The radio channels' budgets are
 * hard-wired here (defaultChannels); devices.yaml may override them.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 08.11.2026
 *
 */

package main

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// TDevicesFile is devices.yaml.
type TDevicesFile struct {
	Status   string               `yaml:"status"` // the governor's liveness topic
	Channels map[string]TChannel  `yaml:"channels,omitempty"`
	Entities []TGovernedEntityDef `yaml:"entities"`
}

// TChannel is a radio channel's budget: the share of time it may send (its duty cycle), what one
// frame costs, and how much may be sent at once.
type TChannel struct {
	DutyCycle    float64 `yaml:"duty_cycle"`
	FrameSeconds float64 `yaml:"frame_seconds"`
	BurstSeconds float64 `yaml:"burst_seconds"`
	// CopiesWithinMS: the same message from the device again this soon is a copy -- another radio
	// hearing the same press -- not a new press (Erik, 2026-11-08; a device may override it).
	CopiesWithinMS int `yaml:"copies_within_ms"`
}

// defaultChannels are the radio channels the governor knows. FS20 (868 MHz) may send 1 % of the
// time; a frame takes about 0.22 s.
var defaultChannels = map[string]TChannel{
	"fs20": {DutyCycle: 0.01, FrameSeconds: 0.22, BurstSeconds: 3, CopiesWithinMS: 500},
}

// TGovernedEntityDef is one governed entity: a capability of a governed device.
type TGovernedEntityDef struct {
	Device       string     `yaml:"device"`
	Label        string     `yaml:"label"`
	Kind         string     `yaml:"kind"`  // switch or light
	Topic        string     `yaml:"topic"` // its state, retained; commands on <topic>/set
	Default      string     `yaml:"default"`
	OnLevel      int        `yaml:"on_level,omitempty"` // a light's level when switched on without one
	EnforceEvery Duration   `yaml:"enforce_every"`
	Channel      string     `yaml:"channel"`
	Paths        []TPathDef `yaml:"paths"`
}

// TPathDef is one way an entity's device is reached: the topic a command goes to, the topic what
// the radio received comes back on, how values are written there, and how many radios send each
// command (each costs the channel a frame).
type TPathDef struct {
	Set      string `yaml:"set"`
	Received string `yaml:"received,omitempty"`
	Encoding string `yaml:"encoding"` // fs20_switch or fs20_dimmer
	Copies   int    `yaml:"copies"`
	// CopiesWithinMS overrides the channel's: the device's own interval for copies.
	CopiesWithinMS int `yaml:"copies_within_ms,omitempty"`
}

// Duration is a time.Duration written as HH:MM:SS, as the defs write it.
type Duration struct{ time.Duration }

// UnmarshalYAML reads HH:MM:SS.
func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	var h, m, s int
	if _, err := fmt.Sscanf(node.Value, "%d:%d:%d", &h, &m, &s); err != nil {
		return fmt.Errorf("%q: expected HH:MM:SS", node.Value)
	}
	d.Duration = time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(s)*time.Second
	return nil
}

// MarshalYAML writes HH:MM:SS.
func (d Duration) MarshalYAML() (any, error) {
	total := int(d.Duration / time.Second)
	return fmt.Sprintf("%02d:%02d:%02d", total/3600, total/60%60, total%60), nil
}

// TBrokerSecrets is the main broker, secrets.yaml's "mqtt:".
type TBrokerSecrets struct {
	Server   string `yaml:"server"`
	Port     string `yaml:"port"`
	Login    string `yaml:"login"`
	Password string `yaml:"password"`
	TLS      bool   `yaml:"tls"`
}

// TSecretsFile is secrets.yaml.
type TSecretsFile struct {
	MQTT TBrokerSecrets `yaml:"mqtt"`
}

// loadYAML reads path into target.
func loadYAML(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, target); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// checkDevicesFile reports what is wrong in devices: an unknown kind, channel or encoding, an
// entity without paths.
func checkDevicesFile(devices TDevicesFile) []string {
	var problems []string
	for _, e := range devices.Entities {
		name := e.Device + " " + e.Label
		if _, known := kinds[e.Kind]; !known {
			problems = append(problems, fmt.Sprintf("%s: unknown kind %q", name, e.Kind))
		}
		if _, known := channelOf(devices, e.Channel); !known {
			problems = append(problems, fmt.Sprintf("%s: unknown radio channel %q", name, e.Channel))
		}
		if len(e.Paths) == 0 {
			problems = append(problems, fmt.Sprintf("%s: no paths", name))
		}
		for _, p := range e.Paths {
			if _, known := encodings[p.Encoding]; !known {
				problems = append(problems, fmt.Sprintf("%s: unknown encoding %q", name, p.Encoding))
			}
		}
	}
	return problems
}

// channelOf returns channel's budget: devices.yaml's, else the hard-wired one.
func channelOf(devices TDevicesFile, channel string) (TChannel, bool) {
	if c, found := devices.Channels[channel]; found {
		return c, true
	}
	c, found := defaultChannels[channel]
	return c, found
}
