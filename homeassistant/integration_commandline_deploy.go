/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationCommandlineDeploy
 *
 * PROJECT.md 30: the commandline integration's part of the deploy (deploy_plan.go, deploy_run.go):
 * each commandline device's machine gets the source it builds mqtt_commandline from and the house's
 * commandline/ (its service file filled in), and builds and restarts it there.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 03.11.2026
 *
 */

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TDeployCommandline is a commandline device and the machine it runs on.
type TDeployCommandline struct {
	Device  string `yaml:"device"`
	Machine string `yaml:"machine"`
}

// planCommandlineDeploy is the commandline integration's part of the plan: each device's machine.
func (p *tDeployPlanner) planCommandlineDeploy(commandline map[string]TCommandlineDevice) {
	for _, id := range sortedKeys(commandline) {
		host := commandline[id].Host
		p.plan.Commandline = append(p.plan.Commandline, TDeployCommandline{Device: id, Machine: p.machine(host, "commandline device "+id)})
	}
}

// deployCommandline is deploy.d/05: each commandline machine gets the source it builds from and
// the house's commandline/ (its service file filled in), built and restarted there.
func (d *tDeployer) deployCommandline() {
	done := map[string]bool{}
	for _, device := range d.plan.Commandline {
		machine := device.Machine
		if done[machine] {
			continue
		}
		done[machine] = true
		d.step("Deploying the commandline integration (" + machine + ") related files")
		shell, target := d.rsyncSSH(machine)
		d.run("", "", "rsync", "-rltv", "-e", shell, "--exclude", ".git", d.generator, target+"go/src")
		d.run("", "", "rsync", "-rltv", d.integrated+"/mqtt_commandline/", "./commandline")
		d.fillServiceFile("commandline/mqtt_commandline.service", map[string]string{"@@CMDLINE_USER@@": d.plan.Machines[machine].User, "@@CMDLINE_HOST@@": machine})
		d.run("", "", "rsync", "-rltv", "--delete", "-e", shell, "./commandline", target+".")
		d.remote(machine, "bash ./commandline/deploy.frame")
	}
}

// fillServiceFile replaces the placeholders in the house's copy of a service file.
func (d *tDeployer) fillServiceFile(path string, values map[string]string) {
	fmt.Printf("+ fill in %s: %v\n", path, values)
	if d.dryRun {
		return
	}
	full := filepath.Join(d.houseDir, path)
	data, err := os.ReadFile(full)
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		d.stepFailed = true
		return
	}
	text := string(data)
	for placeholder, value := range values {
		text = strings.ReplaceAll(text, placeholder, value)
	}
	if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		d.stepFailed = true
	}
}
