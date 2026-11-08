/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: IntegrationFHEMDeploy
 *
 * PROJECT.md 37: the FHEM integration's part of the deploy (deploy_plan.go, deploy_run.go). Each
 * FHEM installation ("installation <host> with: ... end;" in "integration fhem", integration_fhem.go)
 * runs on the hosts device of that name, which says how it is reached ("deploy via ssh ...;"). Its
 * fhem/<host>/fhem.cfg goes to ~/fhem there, with the shared Integrations/fhem/deploy, which
 * installs it, restarts FHEM, and gives FHEM the broker password from fhem/<host>/secrets -- on
 * stdin, never on a command line.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 08.11.2026
 *
 */

package main

import (
	"path/filepath"
)

// TDeployFHEM is an FHEM installation: its machine and the house folder of its files.
type TDeployFHEM struct {
	Machine string `yaml:"machine"`
	Files   string `yaml:"files"`
}

// planFHEMDeploy is the FHEM integration's part of the plan: each installation's machine.
func (p *tDeployPlanner) planFHEMDeploy(physicalContent string) {
	for _, host := range collectFHEMInstallations(physicalContent) {
		p.plan.FHEM = append(p.plan.FHEM, TDeployFHEM{Machine: p.machine(host, "FHEM installation "+host), Files: filepath.Join("fhem", host)})
	}
}

// deployFHEM installs each FHEM installation's configuration and restarts it.
func (d *tDeployer) deployFHEM() {
	for _, installation := range d.plan.FHEM {
		d.step("Deploying the FHEM installation on " + installation.Machine)
		shell, target := d.rsyncSSH(installation.Machine)
		d.run("", "", "rsync", "-rltv", "-e", shell, installation.Files+"/fhem.cfg", d.integrated+"/fhem/deploy", target+"fhem/")
		if d.stepFailed {
			continue
		}
		d.run(filepath.Join(d.houseDir, installation.Files, "secrets"), "", "ssh", append(d.ssh(installation.Machine), "bash ./fhem/deploy")...)
	}
}
