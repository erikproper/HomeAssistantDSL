/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationHostsDeploy
 *
 * PROJECT.md 30: the hosts integration's part of the deploy (deploy_plan.go, deploy_run.go). A hosts
 * device says how it is reached: "deploy via ssh <user>@<host>[:<port>];" (from the Mac) or
 * "deploy as user <user>;" (LAN only, through the hub). The cpu report script -- this house's own
 * secrets only -- goes to the hub and on to its LAN hosts, directly to a "via cloud" machine (a Mac
 * has none: its ~/cpu is the Nextcloud folder itself); ping goes to the hub.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 03.11.2026
 *
 */

package main

import (
	"fmt"
	"regexp"
)

// THostDeploy is a hosts device's "deploy via ssh <endpoint>;" or "deploy as user <user>;".
type THostDeploy struct {
	SSH  *TSSHEndpoint // reached directly from the Mac
	User string        // reached through the hub, as <User>@<hostname>
}

var (
	hostDeployViaSSHPattern = regexp.MustCompile(`^deploy\s+via\s+ssh\s+(\S+)\s*;$`)
	hostDeployAsPattern     = regexp.MustCompile(`^deploy\s+as\s+user\s+([A-Za-z0-9_.-]+)\s*;$`)
)

// parseHostDeployLine recognises a hosts device's "deploy ..." line; warning is set for a
// malformed one.
func parseHostDeployLine(line string) (deploy THostDeploy, warning string, ok bool) {
	if m := hostDeployViaSSHPattern.FindStringSubmatch(line); m != nil {
		endpoint, valid := parseSSHEndpoint(m[1])
		if !valid {
			return THostDeploy{}, fmt.Sprintf("%q: expected \"deploy via ssh <user>@<host>[:<port>];\"", line), true
		}
		return THostDeploy{SSH: &endpoint}, "", true
	}
	if m := hostDeployAsPattern.FindStringSubmatch(line); m != nil {
		return THostDeploy{User: m[1]}, "", true
	}
	return THostDeploy{}, "", false
}

// TDeployCPU is where the cpu report script goes: from the hub to its LAN hosts, and directly to
// "via cloud" machines.
type TDeployCPU struct {
	Hub    string           `yaml:"hub,omitempty"`
	ViaHub []TDeployHubHost `yaml:"via_hub,omitempty"`
	Direct []string         `yaml:"direct,omitempty"`
}

// TDeployHubHost is a LAN host the hub reaches as <User>@<Host>.
type TDeployHubHost struct {
	Host string `yaml:"host"`
	User string `yaml:"user"`
}

// planHostsMachines adds every hosts device with "deploy via ssh" to the plan's machines -- first,
// so the other parts find the machines they run on.
func (p *tDeployPlanner) planHostsMachines(hosts map[string]THostDevice) {
	for _, id := range sortedKeys(hosts) {
		if d := hosts[id]; d.Deploy.SSH != nil {
			p.plan.Machines[d.HostName] = *d.Deploy.SSH
		}
	}
}

// planHostsDeploy is the hosts integration's part of the plan: ping and the cpu script's
// distribution on the coordinator's machine (the hub), and the cpu script's targets -- every "cpu"
// host with a "deploy" line, through the hub for a LAN host, directly for a "via cloud" one.
func (p *tDeployPlanner) planHostsDeploy(hosts map[string]THostDevice) {
	p.plan.Ping, p.plan.CPU.Hub = p.plan.Coordinator, p.plan.Coordinator
	for _, id := range sortedKeys(hosts) {
		d := hosts[id]
		if d.IntegrationType != "cpu" {
			continue
		}
		switch {
		case d.Cloud && d.Deploy.SSH != nil:
			p.plan.CPU.Direct = append(p.plan.CPU.Direct, d.HostName)
		case d.Cloud:
			// A Mac: its ~/cpu is the Nextcloud folder itself.
		case d.Deploy.SSH != nil:
			p.plan.CPU.ViaHub = append(p.plan.CPU.ViaHub, TDeployHubHost{Host: d.HostName, User: d.Deploy.SSH.User})
		case d.Deploy.User != "":
			p.plan.CPU.ViaHub = append(p.plan.CPU.ViaHub, TDeployHubHost{Host: d.HostName, User: d.Deploy.User})
		default:
			p.warnings = append(p.warnings, fmt.Sprintf("deploy: cpu host %q (%s) has no \"deploy\" line -- it doesn't get the cpu report script (\"deploy as user <user>;\" for a LAN host the hub reaches)", d.HostName, id))
		}
	}
}

// deployHosts is the old deploy.d/03: the cpu report script -- this house's own secrets only (PROJECT.md
// 30) -- to the hub and on to its LAN hosts, directly to a "via cloud" machine; ping to the hub.
func (d *tDeployer) deployHosts() {
	if d.plan.CPU.Hub == "" && len(d.plan.CPU.Direct) == 0 {
		return
	}
	d.step("Deploying the hosts integration related files")
	cpu := d.integrated + "/cpu"
	own := "secrets." + d.plan.Installation
	d.run("", "", "rsync", "-rltv", "cpu/secrets.cloud_client", cpu)
	d.run("", "", "rsync", "-rltv", "cpu/"+own, cpu)
	if hub := d.plan.CPU.Hub; hub != "" {
		shell, target := d.rsyncSSH(hub)
		d.run("", "", "rsync", "-rltv", "--delete", "--exclude", "secrets.*", "-e", shell, cpu, target+".")
		d.run("", "", "rsync", "-rltv", "-e", shell, cpu+"/secrets.cloud_client", cpu+"/"+own, target+"cpu/")
		d.remote(hub, `for f in cpu/secrets.*; do case "$f" in cpu/secrets.cloud_client|cpu/`+own+`) ;; *) rm -f "$f";; esac; done`)
		for _, host := range d.plan.CPU.ViaHub {
			d.remote(hub, fmt.Sprintf(`if ping -4 -c 1 -W 2 %[1]s > /dev/null 2>&1; then echo "Updating host/cpu integration for %[1]s"; rsync -arv --delete --exclude "com.erikproper.cpu-report*.plist" --delete-excluded ~/cpu %[2]s@%[1]s:.; else echo "Skipping %[1]s: not responding to ping"; fi`, host.Host, host.User))
		}
	}
	for _, machine := range d.plan.CPU.Direct {
		// Only the cloud client's secrets: a "via cloud" machine reports through the cloud broker. Any
		// other secrets already there are left alone -- excluded, so neither sent nor deleted (no
		// "--delete-excluded": macOS's own rsync ignores a protect filter).
		shell, target := d.rsyncSSH(machine)
		d.run("", "", "rsync", "-arv", "--delete", "--exclude", "secrets.*", "--exclude", "com.erikproper.cpu-report*.plist", "-e", shell, cpu, target+".")
		d.run("", "", "rsync", "-rltv", "-e", shell, cpu+"/secrets.cloud_client", target+"cpu/")
	}
	if d.plan.Ping != "" {
		shell, target := d.rsyncSSH(d.plan.Ping)
		d.run("", "", "rsync", "-rltv", "--exclude", "hosts", "--exclude", "secrets", d.integrated+"/ping/", "./ping")
		d.run("", "", "rsync", "-rltv", "--delete", "-e", shell, "./ping", target+".")
	}
}
