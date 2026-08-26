/*
 *
 * Module:    HouseEventBusCoordinator
 * Package:   Main
 * Component: Secrets
 *
 * Parses the secrets.yaml file homeassistant/Physical_Generator.go generates alongside
 * devices.yaml (generateCoordinatorSecretsFile) -- the main MQTT broker's connection
 * credentials under "mqtt:", plus any other coordinator-only broker profile (e.g.
 * "cloud_coordinator") under "brokers:", both resolved from Physical.def/Settings.def. Separate
 * struct from the generator's own TMQTTBrokerSecrets (defined.go) deliberately: different Go
 * module/binary, and five string/bool fields don't warrant a shared package just to avoid
 * retyping them.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 21.08.2026
 *
 */

package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// TMQTTBrokerSecrets is one MQTT broker's connection credentials.
type TMQTTBrokerSecrets struct {
	Server   string `yaml:"server"`
	Port     string `yaml:"port"`
	Login    string `yaml:"login"`
	Password string `yaml:"password"`
	TLS      bool   `yaml:"tls"`
}

// TMQTTSecretsFile is the top-level shape of a generated secrets.yaml file. Brokers holds every
// other coordinator-only profile (e.g. "cloud_coordinator") declared "coordinator_only true;" in
// Physical.def -- main.go connects one additional mqtt.Client per entry, for cross-installation
// relay/import (mqtt_relay.go). Empty/nil when a house has no such profile declared yet.
type TMQTTSecretsFile struct {
	MQTT    TMQTTBrokerSecrets            `yaml:"mqtt"`
	Brokers map[string]TMQTTBrokerSecrets `yaml:"brokers"`
}

// loadSecretsFile reads and parses a generator-produced secrets.yaml file. Unlike
// devices.yaml's "house has no MQTT settings" case (a soft skip on the generator side), a
// missing secrets.yaml here is a hard error: the coordinator has nothing to do without a
// broker to connect to.
func loadSecretsFile(path string) (TMQTTSecretsFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return TMQTTSecretsFile{}, fmt.Errorf("cannot read %s: %w (has the house's Settings.def been given MQTT broker settings?)", path, err)
	}

	var secretsFile TMQTTSecretsFile
	if err := yaml.Unmarshal(data, &secretsFile); err != nil {
		return TMQTTSecretsFile{}, fmt.Errorf("cannot parse %s: %w", path, err)
	}
	return secretsFile, nil
}
