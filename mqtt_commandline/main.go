/*
 *
 * Module:    MqttCommandline
 * Package:   Main
 * Component: Main
 *
 * The daemon PROJECT.md item 1 describes: a generic, long-running service for script-backed HA
 * entities (switch/sensor/button), reading a generator-authored YAML config
 * (homeassistant/integration_commandline_generator.go's <host>.yaml) describing which scripts back
 * which entity. Architecturally closer to the coordinator (its own persistent broker connection,
 * MQTT Last Will for liveness) than to Integrations/cpu/report's fire-and-exit cron/launchd shape --
 * see PROJECT.md item 1's own liveness-design rationale.
 *
 * Wire shape (mirrors hosts/<host>/...): commandline/<host>/node/state (retained LWT liveness,
 * "true" once connected, "false" on any disconnect -- broker-detected via the Will on an ungraceful
 * one, published explicitly on a graceful shutdown); commandline/<host>/<entity>/state (retained,
 * a status_script's trimmed stdout, verbatim -- no daemon-side interpretation of what the value
 * means); commandline/<host>/<entity>/set (switch command -- "1"/"0", matching the already-deployed
 * check_slideshow-style scripts' own existing convention verbatim, so no existing script needs
 * rewriting for this daemon to use it -- runs on_script/off_script, then immediately re-runs
 * status_script and republishes state, "state reflects reality, never assumed"); commandline/<host>/
 * <entity>/press (button command, no state at all, matching MQTT button semantics).
 *
 * The coordinator (house_event_bus_coordinator/discoverycommandline.go) never subscribes to any of
 * these topics itself -- HA's own switch/button discovery config points its command_topic directly
 * at this daemon's own topic, so this daemon is the only thing that ever needs to see a command.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 06.09.2026
 *
 */

package main

import (
	"bufio"
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"gopkg.in/yaml.v3"
)

// TSwitchConfig is one declared switch's three scripts, in the order
// integration_commandline_generator.go writes them.
type TSwitchConfig struct {
	StatusScript string `yaml:"status_script"`
	OnScript     string `yaml:"on_script"`
	OffScript    string `yaml:"off_script"`
}

// TSensorConfig is one declared sensor's single status script.
type TSensorConfig struct {
	StatusScript string `yaml:"status_script"`
}

// TButtonConfig is one declared button's single press script.
type TButtonConfig struct {
	PressScript string `yaml:"press_script"`
}

// TCommandlineConfig is the generator-authored <host>.yaml this daemon reads at startup --
// mirrors integration_commandline_generator.go's generateCommandlineHostFile output exactly.
type TCommandlineConfig struct {
	Host     string                   `yaml:"host"`
	Switches map[string]TSwitchConfig `yaml:"switches"`
	Sensors  map[string]TSensorConfig `yaml:"sensors"`
	Buttons  map[string]TButtonConfig `yaml:"buttons"`
}

// loadConfig reads and parses a generator-produced <host>.yaml file.
func loadConfig(path string) (TCommandlineConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return TCommandlineConfig{}, fmt.Errorf("cannot read %s: %w", path, err)
	}
	var cfg TCommandlineConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return TCommandlineConfig{}, fmt.Errorf("cannot parse %s: %w", path, err)
	}
	if cfg.Host == "" {
		return TCommandlineConfig{}, fmt.Errorf("%s: no \"host:\" set", path)
	}
	return cfg, nil
}

// TSecrets is one MQTT broker profile's connection values, read from the plain "key=value"
// shell-sourceable format generateMQTTShellSecretsFile writes (homeassistant/
// integration_hosts_generator.go) -- the same file shape Integrations/ping/report and
// Integrations/cpu/report already read via "source ~/cpu/secrets".
type TSecrets struct {
	Server   string
	Login    string
	Password string
	Port     string
	TLS      bool
}

// loadSecrets reads and parses a generator-produced secrets.<host> file.
func loadSecrets(path string) (TSecrets, error) {
	file, err := os.Open(path)
	if err != nil {
		return TSecrets{}, fmt.Errorf("cannot read %s: %w", path, err)
	}
	defer file.Close()

	var secrets TSecrets
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		switch key {
		case "mqtt_server":
			secrets.Server = value
		case "mqtt_login":
			secrets.Login = value
		case "mqtt_password":
			secrets.Password = value
		case "mqtt_port":
			secrets.Port = value
		case "mqtt_tls":
			secrets.TLS = value == "1"
		}
	}
	if err := scanner.Err(); err != nil {
		return TSecrets{}, fmt.Errorf("reading %s: %w", path, err)
	}
	if secrets.Server == "" || secrets.Port == "" {
		return TSecrets{}, fmt.Errorf("%s: mqtt_server/mqtt_port not set", path)
	}
	return secrets, nil
}

// shellPath is a var, not a const, purely so tests can point it at a nonexistent binary to
// exercise the "shell itself cannot run at all" error path.
var shellPath = "sh"

// runStatusScript runs a status_script through "sh -c" (ordinary shell word-splitting/quoting/
// expansion, per PROJECT.md item 1's own design -- lets one generic script be reused
// parametrically) and returns its trimmed stdout. A non-zero exit is NOT treated as a failure
// here: a status script may legitimately signal its own "off"/"not running" state via a non-zero
// exit alongside its own stdout value (confirmed live 2026-09-06: the already-deployed
// check_slideshow prints "1" AND exits 1 when the slideshow isn't running) -- this daemon has no
// per-script knowledge of which convention a given status script uses, so it always trusts stdout
// and only ever surfaces an error for a genuine failure to execute the command at all (e.g. "sh"
// itself missing). This leniency is deliberately NOT shared with runCommandScript below: an
// on/off/press script's exit code IS a meaningful, unconfused failure signal (no equivalent
// "meaningful non-zero" convention is known for that class of script), and treating it the same
// lenient way would silently swallow a real command failure.
func runStatusScript(command string) (string, error) {
	out, err := exec.Command(shellPath, "-c", command).Output()
	if err != nil {
		if _, isExitError := err.(*exec.ExitError); isExitError {
			return strings.TrimSpace(string(out)), nil
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// runCommandScript runs an on_script/off_script/press_script through "sh -c" -- unlike
// runStatusScript, a non-zero exit here IS treated as a real failure (see that function's own doc
// comment for why the two differ). The script's own stdout is irrelevant for an action script, so
// only an error is returned.
func runCommandScript(command string) error {
	return exec.Command(shellPath, "-c", command).Run()
}

func nodeTopic(host string) string          { return "commandline/" + host + "/node/state" }
func stateTopic(host, name string) string   { return "commandline/" + host + "/" + name + "/state" }
func commandTopic(host, name string) string { return "commandline/" + host + "/" + name + "/set" }
func pressTopic(host, name string) string   { return "commandline/" + host + "/" + name + "/press" }

// publishRetained publishes payload to topic, retained, waiting for broker acknowledgement.
func publishRetained(client mqtt.Client, topic, payload string) error {
	token := client.Publish(topic, 0, true, payload)
	if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
		if err := token.Error(); err != nil {
			return fmt.Errorf("publishing %s: %w", topic, err)
		}
		return fmt.Errorf("publishing %s: timed out", topic)
	}
	return nil
}

// publishStatus runs statusScript and republishes its trimmed stdout, verbatim, to topic --
// shared by sensors and switches (a switch's own status_script is what its state topic reports,
// same as a sensor's).
func publishStatus(client mqtt.Client, host, name, statusScript, topic string) {
	out, err := runStatusScript(statusScript)
	if err != nil {
		fmt.Printf("[mqtt_commandline] %s/%s: status script failed: %v\n", host, name, err)
		return
	}
	if err := publishRetained(client, topic, out); err != nil {
		fmt.Printf("[mqtt_commandline] %v\n", err)
	}
}

// handleSwitchCommand runs on_script/off_script for a "1"/"0" command payload (matching the
// already-deployed check_slideshow-style scripts' own convention verbatim). On success, it
// publishes the COMMANDED (target) state immediately, rather than re-running status_script to
// confirm it -- status_script can be slow (e.g. check_slideshow's own "sudo xrandr" call), and
// waiting on it here would leave HA's switch UI sitting in a pending state for no good reason. This
// is deliberately optimistic: if the command didn't actually take effect, the target state
// published here is wrong until statusPollInterval's next periodic poll re-runs status_script and
// corrects it -- "eventually reflects reality," not "reflects reality immediately," a relaxation
// from mqtt_relay.go's own stricter principle that's fine here since the correction window is
// short and bounded. On failure, nothing is published at all -- the retained state is left exactly
// as it was, for the same periodic poll to resolve. An unrecognised payload is logged and ignored,
// not treated as either on or off.
func handleSwitchCommand(client mqtt.Client, host, name string, cfg TSwitchConfig) mqtt.MessageHandler {
	return func(_ mqtt.Client, msg mqtt.Message) {
		payload := strings.TrimSpace(string(msg.Payload()))
		var script string
		switch payload {
		case "1":
			script = cfg.OnScript
		case "0":
			script = cfg.OffScript
		default:
			fmt.Printf("[mqtt_commandline] %s/%s: unrecognised command payload %q; ignored\n", host, name, payload)
			return
		}
		if err := runCommandScript(script); err != nil {
			fmt.Printf("[mqtt_commandline] %s/%s: command script failed: %v\n", host, name, err)
			return
		}
		if err := publishRetained(client, stateTopic(host, name), payload); err != nil {
			fmt.Printf("[mqtt_commandline] %v\n", err)
		}
	}
}

// handleButtonCommand runs pressScript on every message received on the press topic -- no state
// to report either way, matching HA's own MQTT button semantics.
func handleButtonCommand(host, name, pressScript string) mqtt.MessageHandler {
	return func(_ mqtt.Client, _ mqtt.Message) {
		if err := runCommandScript(pressScript); err != nil {
			fmt.Printf("[mqtt_commandline] %s/%s: press script failed: %v\n", host, name, err)
		}
	}
}

// statusPollInterval is how often every switch's and sensor's status_script is re-run and
// republished, independent of any command -- keeps a sensor's value fresh (it has no other trigger
// to report on) and self-heals a switch's retained state against any drift (e.g. the underlying
// state changed by a means other than this daemon's own on/off command). No interval is specified
// anywhere in PROJECT.md item 1's own design; this is a daemon-level implementation choice, not a
// DSL-configurable value -- revisit if a real capability ever needs a different cadence.
const statusPollInterval = 60 * time.Second

// subscribeAndPublishInitial subscribes every declared switch's command topic and button's press
// topic, and publishes every switch's and sensor's current status once -- shared by the initial
// connect and every reconnect (MQTT subscriptions and retained publishes both need redoing after a
// reconnect; paho does not remember subscriptions across a new network-level connection here since
// CleanSession is left at its library default).
func subscribeAndPublishInitial(client mqtt.Client, cfg TCommandlineConfig) {
	if err := publishRetained(client, nodeTopic(cfg.Host), "true"); err != nil {
		fmt.Printf("[mqtt_commandline] %v\n", err)
	}
	for name, sw := range cfg.Switches {
		name, sw := name, sw
		if token := client.Subscribe(commandTopic(cfg.Host, name), 0, handleSwitchCommand(client, cfg.Host, name, sw)); token.Wait() && token.Error() != nil {
			fmt.Printf("[mqtt_commandline] subscribing to %s: %v\n", commandTopic(cfg.Host, name), token.Error())
		}
		publishStatus(client, cfg.Host, name, sw.StatusScript, stateTopic(cfg.Host, name))
	}
	for name, b := range cfg.Buttons {
		name, b := name, b
		if token := client.Subscribe(pressTopic(cfg.Host, name), 0, handleButtonCommand(cfg.Host, name, b.PressScript)); token.Wait() && token.Error() != nil {
			fmt.Printf("[mqtt_commandline] subscribing to %s: %v\n", pressTopic(cfg.Host, name), token.Error())
		}
	}
	for name, s := range cfg.Sensors {
		publishStatus(client, cfg.Host, name, s.StatusScript, stateTopic(cfg.Host, name))
	}
}

// pollStatusForever re-publishes every switch's and sensor's status every statusPollInterval,
// until stop is closed.
func pollStatusForever(client mqtt.Client, cfg TCommandlineConfig, stop <-chan struct{}) {
	ticker := time.NewTicker(statusPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			for name, sw := range cfg.Switches {
				publishStatus(client, cfg.Host, name, sw.StatusScript, stateTopic(cfg.Host, name))
			}
			for name, s := range cfg.Sensors {
				publishStatus(client, cfg.Host, name, s.StatusScript, stateTopic(cfg.Host, name))
			}
		}
	}
}

func main() {
	configPath := flag.String("config", "", "path to the generator-authored <host>.yaml config")
	secretsPath := flag.String("secrets", "", "path to the generator-authored secrets.<host> file")
	flag.Parse()
	if *configPath == "" || *secretsPath == "" {
		fmt.Fprintf(os.Stderr, "usage: mqtt_commandline -config <path/to/host.yaml> -secrets <path/to/secrets.host>\n")
		os.Exit(1)
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	secrets, err := loadSecrets(*secretsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	opts := mqtt.NewClientOptions()
	scheme := "tcp"
	if secrets.TLS {
		scheme = "ssl"
		opts.SetTLSConfig(&tls.Config{})
	}
	opts.AddBroker(fmt.Sprintf("%s://%s:%s", scheme, secrets.Server, secrets.Port))
	opts.SetClientID("mqtt_commandline-" + cfg.Host)
	opts.SetUsername(secrets.Login)
	opts.SetPassword(secrets.Password)
	opts.SetAutoReconnect(true)
	opts.SetConnectTimeout(10 * time.Second)
	// LWT: the broker publishes this the moment it detects this daemon's connection has died
	// (crash, network partition) without a clean disconnect -- see this file's own header comment
	// for why this daemon gets the coordinator's own class of liveness mechanism rather than
	// cpu/report's ping-based one.
	opts.SetWill(nodeTopic(cfg.Host), "false", 0, true)
	opts.SetOnConnectHandler(func(client mqtt.Client) {
		fmt.Printf("[mqtt_commandline] connected; publishing initial state for %q\n", cfg.Host)
		subscribeAndPublishInitial(client, cfg)
	})

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
		if err := token.Error(); err != nil {
			fmt.Fprintf(os.Stderr, "error: cannot connect to MQTT broker %s:%s: %v\n", secrets.Server, secrets.Port, err)
		} else {
			fmt.Fprintf(os.Stderr, "error: cannot connect to MQTT broker %s:%s: timed out\n", secrets.Server, secrets.Port)
		}
		os.Exit(1)
	}

	stop := make(chan struct{})
	go pollStatusForever(client, cfg, stop)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	close(stop)
	// Graceful shutdown: publish "false" explicitly rather than relying solely on the broker's own
	// Will delivery, which some brokers delay -- a clean disconnect should report unavailable
	// immediately, the same "state reflects reality" principle used throughout this daemon.
	if err := publishRetained(client, nodeTopic(cfg.Host), "false"); err != nil {
		fmt.Printf("[mqtt_commandline] %v\n", err)
	}
	client.Disconnect(250)
}
