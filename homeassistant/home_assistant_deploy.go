/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: HomeAssistantDeploy
 *
 * PROJECT.md 30: the Home Assistant instances' part of the deploy (deploy_plan.go, deploy_run.go):
 * each instance's configuration -- an HA OS one ("home_assistant main: vienna via ssh ...;")
 * through its SSH add-on, with the registry repair; a container one into ${container_ha_config} on
 * its hosts device -- then its reload or restart (MetaAction, PublishMetaAction), and at the end
 * the wait for HA main's reload, reporting its errors (PROJECT.md 19h.3). The registry repair and
 * the reload check talk to HA's API directly (home_assistant_registry.go).
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 04.11.2026
 *
 */

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// cutInstanceEndpoints splits a "home_assistant <qualifier>: <name> [via ssh <endpoint>] [at
// <url>];" declaration's fields into the name, its SSH endpoint (nil without one) and the URL of
// its API ("" without one: the registry repair and the reload check need it); ok is false for any
// other shape.
func cutInstanceEndpoints(fields []string) (name string, ssh *TSSHEndpoint, url string, ok bool) {
	if n := len(fields); n >= 3 && fields[n-2] == "at" && instanceURLPattern.MatchString(fields[n-1]) {
		url, fields = fields[n-1], fields[:n-2]
	}
	switch {
	case len(fields) == 1:
		return fields[0], nil, url, true
	case len(fields) == 4 && fields[1] == "via" && fields[2] == "ssh":
		if endpoint, valid := parseSSHEndpoint(fields[3]); valid {
			return fields[0], &endpoint, url, true
		}
	}
	return "", nil, "", false
}

var instanceURLPattern = regexp.MustCompile(`^https?://[^\s/]+/?$`)

// TDeployInstance is one Home Assistant instance's configuration push.
type TDeployInstance struct {
	Name    string `yaml:"name"`
	Machine string `yaml:"machine"`
	Kind    string `yaml:"kind"` // "ha_os" or "container"
	Files   string `yaml:"files"`
	Config  string `yaml:"config"`
	URL     string `yaml:"url,omitempty"` // its API, for the registry repair and the reload check
}

// defaultContainerHAConfig is a container instance's configuration folder, ${container_ha_config}
// overriding it.
const defaultContainerHAConfig = "/var/lib/home-automation/homeassistant"

// planInstancesDeploy is the Home Assistant instances' part of the plan, main first, as the
// deploy has always gone: an instance with its own "via ssh" runs on HA OS, one without as a
// container on its hosts device.
func (p *tDeployPlanner) planInstancesDeploy(instances map[string]THomeAssistantInstance) {
	qualifiers := make([]string, 0, len(instances))
	for qualifier := range instances {
		qualifiers = append(qualifiers, qualifier)
	}
	sort.Slice(qualifiers, func(i, j int) bool {
		if (qualifiers[i] == "main") != (qualifiers[j] == "main") {
			return qualifiers[i] == "main"
		}
		return qualifiers[i] < qualifiers[j]
	})
	for _, qualifier := range qualifiers {
		instance := instances[qualifier]
		name := resolveDefinitionReference(instance.Name, p.settings)
		deploy := TDeployInstance{Name: qualifier, Machine: name, Files: "hass/" + name, URL: instance.URL}
		if instance.SSH != nil {
			p.plan.Machines[name] = *instance.SSH
			deploy.Kind, deploy.Config = "ha_os", "/config"
		} else {
			p.machine(name, "Home Assistant instance "+qualifier)
			deploy.Kind, deploy.Config = "container", settingOr(p.settings, "container_ha_config", defaultContainerHAConfig)
		}
		p.plan.Instances = append(p.plan.Instances, deploy)
	}
}

// deployConfigEntries are the parts of an instance's generated tree that go into its configuration.
var deployConfigEntries = []string{"configuration.yaml", "automation", "entities", "customization", "integrations", "script"}

// metaReloadAutomation is the reload automation's file, relative to an instance's tree.
const metaReloadAutomation = "automation/infrastructural/automation.coordinator_meta_reload.yaml"

// deployInstances deploys every instance selected (deployInstance), remembering main's meta
// action for the reload check.
func (d *tDeployer) deployInstances() {
	for _, instance := range d.plan.Instances {
		if !d.selected(instance.Name) && !d.selected("home_assistant") {
			continue
		}
		meta, at := d.deployInstance(instance)
		if instance.Name == "main" {
			d.mainMeta, d.mainMetaTime = meta, at
		}
	}
}

// deployInstance is the old deploy.d/01 (an HA OS instance) and 02 (a container): its configuration, then
// its reload or restart. Returns the meta action and when it was asked for.
func (d *tDeployer) deployInstance(instance TDeployInstance) (string, int64) {
	d.step("Deploying homeassistant " + instance.Name + "'s YAML files")
	shell, target := d.rsyncSSH(instance.Machine)
	user := d.plan.Machines[instance.Machine].User
	switch instance.Kind {
	case "ha_os":
		meta := d.metaAction(instance, instance.Config+"/"+metaReloadAutomation)
		d.run("", "", "rsync", "-rltv", "--delete", "-e", shell, instance.Files+"/", target+instance.Machine)
		for _, entry := range deployConfigEntries {
			d.remote(instance.Machine, "sudo rsync -arv --delete /home/"+user+"/"+instance.Machine+"/"+entry+" /root/config")
		}
		for _, entry := range []string{"automation", "script"} {
			d.remote(instance.Machine, "sudo ln -sf ../"+entry+"s.yaml "+instance.Config+"/"+entry+"/")
		}
		// Keep HA's entity registry in step with the generator (PROJECT.md item 48), before the reload
		// (home_assistant_registry.go).
		d.repairRegistry(instance)
		at := time.Now().Unix()
		d.publishMeta(meta, instance.Name)
		if d.stepFailed {
			meta = "none"
		}
		return meta, at
	case "container":
		meta := d.metaAction(instance, instance.Config+"/"+metaReloadAutomation)
		for _, entry := range deployConfigEntries {
			if _, err := os.Stat(filepath.Join(d.houseDir, instance.Files, entry)); err == nil {
				d.run("", "", "rsync", "-rltv", "-e", shell, "--rsync-path=sudo rsync", "--delete", instance.Files+"/"+entry, target+instance.Config)
			}
		}
		for _, entry := range []string{"automation", "script"} {
			d.remote(instance.Machine, "sudo ln -sf ../"+entry+"s.yaml "+instance.Config+"/"+entry+"/ 2> /dev/null; true")
		}
		d.remote(instance.Machine, "sudo chown root:root -R "+instance.Config)
		d.publishMeta(meta, instance.Name)
		return meta, 0
	}
	fmt.Printf("  WARNING: instance %q has kind %q, which the deploy doesn't know -- skipped\n", instance.Name, instance.Kind)
	return "none", 0
}

// metaAction is the reload or restart an instance gets once its configuration is in place:
// "restart" when its reload automation itself changes (reloading would remove the automation
// waiting for that very reload, found live 2026-10-26), else "reload"; "none" when the machine
// can't be reached. -n decides nothing.
func (d *tDeployer) metaAction(instance TDeployInstance, remotePath string) string {
	local, err := os.ReadFile(filepath.Join(d.houseDir, instance.Files, metaReloadAutomation))
	command := append(d.ssh(instance.Machine), "sudo cat "+remotePath+" 2> /dev/null; true")
	fmt.Println("+ " + shellJoin(append([]string{"ssh"}, command...)) + "   (reload or restart)")
	if d.dryRun {
		return "reload"
	}
	remote, sshErr := exec.Command("ssh", command...).Output()
	switch {
	case sshErr != nil:
		return "none"
	case err == nil && bytes.Equal(local, remote):
		return "reload"
	}
	return "restart"
}

// publishMeta asks instance's reload or restart automation to run, through the cloud broker
// (coordinator/secrets.yaml's "brokers: cloud_coordinator"), the coordinator relaying it -- a
// deploy doesn't necessarily run from inside the house. Never after a failed step: the instance's
// files didn't all arrive.
func (d *tDeployer) publishMeta(action, instance string) {
	if action == "none" || d.stepFailed {
		fmt.Printf("  (skipping meta trigger for %q -- this step's deploy did not complete)\n", instance)
		return
	}
	data, err := os.ReadFile(filepath.Join(d.houseDir, "coordinator", "secrets.yaml"))
	var secrets struct {
		Brokers map[string]struct {
			Server   string `yaml:"server"`
			Port     any    `yaml:"port"`
			Login    string `yaml:"login"`
			Password string `yaml:"password"`
		} `yaml:"brokers"`
	}
	if err == nil {
		err = yaml.Unmarshal(data, &secrets)
	}
	broker, found := secrets.Brokers["cloud_coordinator"]
	if err != nil || !found || broker.Server == "" || d.plan.Installation == "" {
		fmt.Printf("  (skipping meta %s trigger for %q -- no cloud_coordinator broker in coordinator/secrets.yaml)\n", action, instance)
		return
	}
	topic := d.plan.Installation + "/meta/" + action + "/" + instance + "/request"
	port := fmt.Sprint(broker.Port)
	args := []string{"-h", broker.Server, "-p", port, "--tls-use-os-certs", "-u", broker.Login, "-P", broker.Password, "-t", topic, "-m", "PRESS"}
	shown := shellJoin([]string{"mosquitto_pub", "-h", broker.Server, "-p", port, "--tls-use-os-certs", "-u", broker.Login, "-P", "****", "-t", topic, "-m", "PRESS"})
	failed := d.stepFailed
	if d.run("", shown, "mosquitto_pub", args...) && !d.dryRun {
		fmt.Printf("  triggered %s for instance %q (via cloud)\n", action, instance)
	} else if d.dryRun {
		fmt.Printf("  (would trigger %s for instance %q -- decided at run time)\n", action, instance)
	} else {
		fmt.Printf("  WARNING: failed to trigger %s for instance %q (the deploy itself still succeeded)\n", action, instance)
		d.stepFailed = failed // a failed trigger isn't a failed deploy
	}
}

// checkReload is the old deploy.d/06 (PROJECT.md 19h.3): wait until HA main has finished its reload or
// restart, then report the reload's errors since it was asked for.
func (d *tDeployer) checkReload(meta string, at int64) {
	d.step("Checking Home Assistant's reload")
	var main *TDeployInstance
	for i := range d.plan.Instances {
		if d.plan.Instances[i].Name == "main" && d.plan.Instances[i].Kind == "ha_os" {
			main = &d.plan.Instances[i]
		}
	}
	probes, _ := filepath.Glob(filepath.Join(d.houseDir, "hass", "*", "entities", "mqtt", "*", "*.yaml"))
	if main != nil {
		probes, _ = filepath.Glob(filepath.Join(d.houseDir, main.Files, "entities", "mqtt", "*", "*.yaml"))
	}
	switch {
	case main == nil || meta == "none" || meta == "":
		fmt.Println("  (skipping -- HA main did not reload or restart in this deploy)")
		return
	case len(probes) == 0:
		fmt.Println("  (skipping -- no YAML mqtt entities)")
		return
	}
	probe := strings.TrimSuffix(filepath.Base(probes[0]), ".yaml")
	// The first 15 s let the reload start; then until the reload automation is idle again (a reload
	// takes ~45 s) and an entity of the YAML mqtt config is there (home_assistant_registry.go).
	fmt.Printf("+ sleep 15, then wait for %s's reload (automation.coordinator_meta_reload idle, %s there) and read its errors since then (websocket, %s)\n", main.Name, probe, main.URL)
	if d.dryRun {
		return
	}
	time.Sleep(15 * time.Second)
	token := d.instanceToken(*main)
	if token == "" || main.URL == "" {
		fmt.Printf("  WARNING: no ${%s_ha_token} in the defs, or no \"at <url>\" on home_assistant %s -- reload errors not checked\n", main.Name, main.Name)
		return
	}
	ha, err := connectHomeAssistant(main.URL, token)
	if err != nil {
		fmt.Printf("  WARNING: the reload check failed: %v\n", err)
		return
	}
	defer ha.Close()
	errors, finished := ha.waitForReload(probe, at-10)
	switch {
	case !finished:
		fmt.Println("  WARNING: HA main did not finish loading within ~3 minutes -- reload errors not checked")
	case len(errors) > 0:
		fmt.Println("  ERROR: Home Assistant's reload reported errors -- entities may be missing:")
		for _, line := range errors {
			fmt.Println("    " + line)
		}
	default:
		fmt.Println("  Home Assistant reloaded without errors")
	}
}
