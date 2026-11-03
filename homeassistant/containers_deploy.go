/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: ContainersDeploy
 *
 * PROJECT.md 30: update.containers' part of the deploy (deploy_plan.go, deploy_run.go): the script
 * that checks and updates a hub's Podman containers (PROJECT.md 29) goes to every machine running a
 * container instance.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 03.11.2026
 *
 */

package main

// planContainersDeploy is update.containers' part of the plan: every machine running a container
// instance (PROJECT.md 29).
func (p *tDeployPlanner) planContainersDeploy() {
	for _, instance := range p.plan.Instances {
		if instance.Kind == "container" {
			p.plan.Containers = append(p.plan.Containers, instance.Machine)
		}
	}
}

// deployContainers is the old deploy.d/04: update.containers to every container host.
func (d *tDeployer) deployContainers() {
	if len(d.plan.Containers) == 0 {
		return
	}
	d.step("Deploying the container update script")
	for _, machine := range d.plan.Containers {
		shell, target := d.rsyncSSH(machine)
		d.run("", "", "rsync", "-ltv", "-e", shell, d.integrated+"/containers/update.containers", target+".")
	}
}
