/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: DeployRun
 *
 * PROJECT.md 30, step 2 (2026-11-03): "homeassistant -deploy [-n] [-only <parts>] Definitions/Main.def"
 * carries out the house's deploy.yaml (deploy_plan.go) -- what the per-house deploy.d/ steps did,
 * in the same order and with the same commands, each integration's part in its own file:
 *
 *   1. the coordinator: its source and the house's coordinator/ to its machine, built and
 *      restarted there (Integrations/coordinator/deploy);
 *   2. each Home Assistant instance's configuration (main first), the registry repair for an
 *      HA OS one, then its reload or restart (MetaAction, PublishMetaAction);
 *   3. the cpu report script (this house's secrets only) to the hub, from there to its LAN hosts,
 *      and directly to a "via cloud" one; ping to the hub;
 *   4. update.containers to every container host;
 *   5. each commandline machine: the source it builds from, and its commandline/;
 *   6. waiting for HA main's reload, and reporting its errors.
 *
 * "-only" deploys just the parts named, comma-separated, like sourcing one deploy.d/ step used to:
 * coordinator, an instance (main, ha2mqtt, ...; home_assistant for all), hosts, containers,
 * commandline -- the reload check runs when main is among them.
 *
 * "-n" prints every command instead of running it (and decides no reload/restart); the registry
 * repair then reads the registry and reports what it would do, changing nothing. A failed
 * command fails its step: the step's instance is then neither reloaded nor restarted, and the
 * deploy goes on with the next step, as before. The broker password is never printed.
 *
 * The registry repair and the reload check talk to HA's websocket and REST API at the instance's
 * own URL, "at <url>" in its declaration (home_assistant_registry.go).
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 04.11.2026
 *
 */

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// tDeployer carries out one deploy plan.
type tDeployer struct {
	plan         TDeployPlan
	houseDir     string
	dryRun       bool
	only         map[string]bool // the parts "-only" names; nil for all
	stepFailed   bool
	generator    string // the plan's sources, "$HOME" resolved
	integrated   string
	mainMeta     string // main's reload or restart, for the reload check
	mainMetaTime int64
}

// deployParts are the parts "-only" may name, besides the instances' own names.
var deployParts = []string{"coordinator", "home_assistant", "hosts", "containers", "commandline"}

// runDeploy carries out houseDir's deploy.yaml, only the parts named in only when it isn't empty.
func runDeploy(houseDir string, dryRun bool, only []string) error {
	data, err := os.ReadFile(filepath.Join(houseDir, "deploy.yaml"))
	if err != nil {
		return fmt.Errorf("deploy: %v -- generate first", err)
	}
	var plan TDeployPlan
	if err := yaml.Unmarshal(data, &plan); err != nil {
		return fmt.Errorf("deploy.yaml: %v", err)
	}
	d := &tDeployer{plan: plan, houseDir: houseDir, dryRun: dryRun, generator: os.ExpandEnv(plan.Sources.Generator), integrated: os.ExpandEnv(plan.Sources.Integrations)}
	if len(only) > 0 {
		valid := append([]string{}, deployParts...)
		for _, instance := range plan.Instances {
			valid = append(valid, instance.Name)
		}
		d.only = map[string]bool{}
		for _, part := range only {
			if !slices.Contains(valid, part) {
				return fmt.Errorf("deploy: -only %q: not a part of this deploy -- one of %s", part, strings.Join(valid, ", "))
			}
			d.only[part] = true
		}
	}

	if d.selected("coordinator") {
		d.deployCoordinator()
	}
	d.deployInstances()
	if d.selected("hosts") {
		d.deployHosts()
	}
	if d.selected("containers") {
		d.deployContainers()
	}
	if d.selected("commandline") {
		d.deployCommandline()
	}
	if d.selected("main") || d.selected("home_assistant") {
		d.checkReload(d.mainMeta, d.mainMetaTime)
	}
	return nil
}

// selected reports whether part is deployed in this run.
func (d *tDeployer) selected(part string) bool {
	return d.only == nil || d.only[part]
}

// step starts a deploy step.
func (d *tDeployer) step(title string) {
	d.stepFailed = false
	fmt.Printf("\n****\n*\n* %s\n*\n****\n\n", title)
}

// run runs a command (prints it with -n), its stdin from stdin when not "". shown replaces the
// printed form when not "" (a command carrying a secret). A failure fails the step.
func (d *tDeployer) run(stdin, shown string, name string, args ...string) bool {
	if shown == "" {
		shown = shellJoin(append([]string{name}, args...))
	}
	if stdin != "" {
		shown += " < " + stdin
	}
	fmt.Println("+ " + shown)
	if d.dryRun {
		return true
	}
	cmd := exec.Command(name, args...)
	cmd.Dir, cmd.Stdout, cmd.Stderr = d.houseDir, os.Stdout, os.Stderr
	if stdin != "" {
		file, err := os.Open(stdin)
		if err != nil {
			fmt.Printf("  ERROR: %v\n", err)
			d.stepFailed = true
			return false
		}
		defer file.Close()
		cmd.Stdin = file
	}
	if err := cmd.Run(); err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		d.stepFailed = true
		return false
	}
	return true
}

// ssh returns the ssh arguments reaching machine.
func (d *tDeployer) ssh(machine string) []string {
	m := d.plan.Machines[machine]
	if m.Port != 0 {
		return []string{"-p", strconv.Itoa(m.Port), m.User + "@" + m.Host}
	}
	return []string{m.User + "@" + m.Host}
}

// rsyncSSH returns rsync's "-e" value and the "<user>@<host>:" prefix reaching machine.
func (d *tDeployer) rsyncSSH(machine string) (shell, target string) {
	m := d.plan.Machines[machine]
	shell = "ssh"
	if m.Port != 0 {
		shell = "ssh -p " + strconv.Itoa(m.Port)
	}
	return shell, m.User + "@" + m.Host + ":"
}

// remote runs command on machine.
func (d *tDeployer) remote(machine, command string) bool {
	return d.run("", "", "ssh", append(d.ssh(machine), command)...)
}

// shellJoin writes args as one shell command line, quoting where needed.
func shellJoin(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		if arg != "" && !strings.ContainsAny(arg, " \t\n\"'$*?;|&<>(){}[]#~\\`") {
			quoted[i] = arg
			continue
		}
		quoted[i] = "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
	}
	return strings.Join(quoted, " ")
}
