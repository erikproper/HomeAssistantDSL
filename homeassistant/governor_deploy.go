/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: GovernorDeploy
 *
 * PROJECT.md 37: the governor's part of the deploy (deploy_plan.go, deploy_run.go). The governor
 * runs next to the coordinator, on its machine ("coordinator on <hostname>;"), as a service of its
 * own, when Physical.def has an "integration governed" block. Its source and the house's
 * governor/ go there, and the shared Integrations/governor/deploy builds and restarts it.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 08.11.2026
 *
 */

package main

// planGovernorDeploy is the governor's part of the plan: the coordinator's machine, when there are
// governed devices.
func (p *tDeployPlanner) planGovernorDeploy(physicalContent string) {
	blocks, _ := parseIntegrationBlocks(physicalContent, nil)
	for _, block := range blocks {
		if block.Name == "governed" && p.plan.Coordinator != "" {
			p.plan.Governor = p.plan.Coordinator
			return
		}
	}
}

// deployGovernor sends the source and the house's governor/ to its machine, where it is built and
// restarted.
func (d *tDeployer) deployGovernor() {
	if d.plan.Governor == "" {
		return
	}
	d.step("Deploying (and compiling) the governor and its associated files")
	shell, target := d.rsyncSSH(d.plan.Governor)
	d.run("", "", "rsync", "-rltv", "--delete", "-e", shell, "--exclude", ".git", d.generator, target+"go/src")
	d.run("", "", "rsync", "-rltv", "--exclude", "devices.yaml", "--exclude", "secrets.yaml", d.integrated+"/governor/", "governor")
	d.run("", "", "rsync", "-rltv", "--delete", "-e", shell, "governor", target+".")
	d.remote(d.plan.Governor, "bash ./governor/deploy")
}
