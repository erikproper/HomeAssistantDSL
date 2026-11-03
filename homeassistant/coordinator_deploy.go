/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: CoordinatorDeploy
 *
 * PROJECT.md 30: the coordinator's part of the deploy (deploy_plan.go, deploy_run.go). Physical.def
 * says where it runs, "coordinator on <hostname>;" -- the hub, where ping and the cpu script's
 * distribution run too. Its source and the house's coordinator/ go there, and the shared
 * Integrations/coordinator/deploy builds and restarts it.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 03.11.2026
 *
 */

package main

import (
	"regexp"
	"strings"
)

var coordinatorOnPattern = regexp.MustCompile(`^coordinator\s+on\s+(\S+)\s*;$`)

// collectCoordinatorMachine returns the hostname of Physical.def's "coordinator on <hostname>;".
func collectCoordinatorMachine(physicalContent string) string {
	for _, line := range strings.Split(physicalContent, "\n") {
		if m := coordinatorOnPattern.FindStringSubmatch(stripLineComment(line)); m != nil {
			return m[1]
		}
	}
	return ""
}

// planCoordinatorDeploy is the coordinator's part of the plan.
func (p *tDeployPlanner) planCoordinatorDeploy(physicalContent string) {
	p.plan.Coordinator = collectCoordinatorMachine(physicalContent)
	if p.plan.Coordinator == "" {
		p.warnings = append(p.warnings, "deploy: no \"coordinator on <hostname>;\" in Physical.def -- the coordinator, ping and the cpu script's hub are not deployed")
		return
	}
	p.machine(p.plan.Coordinator, "the coordinator")
}

// coordinatorStateFiles are what the running coordinator keeps in its coordinator/ folder -- never
// deleted by the push (Architecture.md §6.11).
var coordinatorStateFiles = []string{"entity_existence.json", "discovery_existence.json", "discovery_topics.json", "import_existence.json", "live_device_info.json", "discovery_passthrough_devices.json"}

// deployCoordinator is deploy.d/00: the coordinator's source and the house's coordinator/ to its
// machine, built and restarted there.
func (d *tDeployer) deployCoordinator() {
	if d.plan.Coordinator == "" {
		return
	}
	d.step("Deploying (and compiling) the coordinator and its associated files")
	shell, target := d.rsyncSSH(d.plan.Coordinator)
	d.run("", "", "rsync", "-rltv", "--delete", "-e", shell, "--exclude", ".git", d.generator, target+"go/src")
	d.run("", "", "rsync", "-rltv", "--exclude", "devices.yaml", "--exclude", "secrets.yaml", d.integrated+"/coordinator/", "coordinator")
	args := []string{"-rltv", "--delete"}
	for _, file := range coordinatorStateFiles {
		args = append(args, "--exclude", file)
	}
	d.run("", "", "rsync", append(args, "-e", shell, "coordinator", target+".")...)
	d.remote(d.plan.Coordinator, "bash ./coordinator/deploy")
}
