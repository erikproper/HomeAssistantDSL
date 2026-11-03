/*
 *
 * Module:    HomeAssistant
 * Package:   Main
 * Component: DeployRun
 *
 * PROJECT.md 30, step 2 (2026-11-03): "homeassistant -deploy [-n] Definitions/Main.def" carries
 * out the house's deploy.yaml (deploy_plan.go) -- what the per-house deploy.d/ steps did, in the
 * same order and with the same commands:
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
 * "-n" prints every command instead of running it (and decides no reload/restart). A failed
 * command fails its step: the step's instance is then neither reloaded nor restarted, and the
 * deploy goes on with the next step, as before. The broker password is never printed.
 *
 * The registry repair and the reload check still pipe tools/ha_registry_sync.py and
 * tools/ha_reload_errors.py into the HA add-on over SSH; talking to HA's websocket API instead is
 * the next step.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 03.11.2026
 *
 */

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// deployConfigEntries are the parts of an instance's generated tree that go into its configuration.
var deployConfigEntries = []string{"configuration.yaml", "automation", "entities", "customization", "integrations", "script"}

// coordinatorStateFiles are what the running coordinator keeps in its coordinator/ folder -- never
// deleted by the push (Architecture.md §6.11).
var coordinatorStateFiles = []string{"entity_existence.json", "discovery_existence.json", "discovery_topics.json", "import_existence.json", "live_device_info.json", "discovery_passthrough_devices.json"}

// metaReloadAutomation is the reload automation's file, relative to an instance's tree.
const metaReloadAutomation = "automation/infrastructural/automation.coordinator_meta_reload.yaml"

// tDeployer carries out one deploy plan.
type tDeployer struct {
	plan       TDeployPlan
	houseDir   string
	dryRun     bool
	stepFailed bool
	generator  string // the plan's sources, "$HOME" resolved
	integrated string
}

// runDeploy carries out houseDir's deploy.yaml.
func runDeploy(houseDir string, dryRun bool) error {
	data, err := os.ReadFile(filepath.Join(houseDir, "deploy.yaml"))
	if err != nil {
		return fmt.Errorf("deploy: %v -- generate first", err)
	}
	var plan TDeployPlan
	if err := yaml.Unmarshal(data, &plan); err != nil {
		return fmt.Errorf("deploy.yaml: %v", err)
	}
	d := &tDeployer{plan: plan, houseDir: houseDir, dryRun: dryRun, generator: os.ExpandEnv(plan.Sources.Generator), integrated: os.ExpandEnv(plan.Sources.Integrations)}

	d.deployCoordinator()
	var mainMeta string
	var mainMetaTime int64
	for _, instance := range plan.Instances {
		meta, at := d.deployInstance(instance)
		if instance.Name == "main" {
			mainMeta, mainMetaTime = meta, at
		}
	}
	d.deployHosts()
	d.deployContainers()
	d.deployCommandline()
	d.checkReload(mainMeta, mainMetaTime)
	return nil
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

// deployInstance is deploy.d/01 (an HA OS instance) and 02 (a container): its configuration, then
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
		// Keep HA's entity registry in step with the generator (PROJECT.md item 48), before the reload.
		d.run(d.generator+"/tools/ha_registry_sync.py", "", "ssh", append(d.ssh(instance.Machine), "sudo python3 - /home/"+user+"/"+instance.Machine+"/registry_manifest.json "+instance.Config+"/.generator_registry_manifest.json --apply")...)
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

// deployHosts is deploy.d/03: the cpu report script -- this house's own secrets only (PROJECT.md
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

// deployContainers is deploy.d/04: update.containers to every container host.
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

// checkReload is deploy.d/06 (PROJECT.md 19h.3): wait until HA main has finished its reload or
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
	// Until the reload automation is idle again (a reload takes ~45 s) and an entity of the YAML
	// mqtt config is there (after a restart, HA answers before MQTT is set up); the first 15 s let
	// the reload start.
	wait := `T=$(sudo cat /run/s6/container_environment/SUPERVISOR_TOKEN); for i in $(seq 1 33); do
      a=$(curl -s -H "Authorization: Bearer $T" http://supervisor/core/api/states/automation.coordinator_meta_reload)
      p=$(curl -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer $T" http://supervisor/core/api/states/` + probe + `)
      case "$a" in *"\"current\":0"*) [ "$p" = 200 ] && exit 0;; esac
      sleep 5
    done; exit 1`
	fmt.Println("+ sleep 15")
	if !d.dryRun {
		time.Sleep(15 * time.Second)
	}
	failed := d.stepFailed
	if !d.remote(main.Machine, wait) {
		d.stepFailed = failed
		fmt.Println("  WARNING: HA main did not finish loading within ~3 minutes -- reload errors not checked")
		return
	}
	// ha_reload_errors.py builds on ha_registry_sync.py's helpers, without its main.
	syncTool, err1 := os.ReadFile(d.generator + "/tools/ha_registry_sync.py")
	errorsTool, err2 := os.ReadFile(d.generator + "/tools/ha_reload_errors.py")
	if err1 != nil || err2 != nil {
		fmt.Println("  WARNING: tools/ha_registry_sync.py or tools/ha_reload_errors.py not found -- reload errors not checked")
		return
	}
	script := string(syncTool)
	if cut := strings.Index(script, "if __name__ == \"__main__\":"); cut >= 0 {
		script = script[:cut]
	}
	script += string(errorsTool)
	command := append(d.ssh(main.Machine), "sudo python3 - "+strconv.FormatInt(at-10, 10))
	fmt.Println("+ " + shellJoin(append([]string{"ssh"}, command...)) + " < ha_registry_sync.py (helpers) + ha_reload_errors.py")
	if d.dryRun {
		return
	}
	cmd := exec.Command("ssh", command...)
	cmd.Stdin, cmd.Stderr = strings.NewReader(script), os.Stderr
	output, err := cmd.Output()
	switch {
	case err != nil:
		fmt.Printf("  WARNING: the reload check failed: %v\n", err)
	case strings.TrimSpace(string(output)) != "":
		fmt.Println("  ERROR: Home Assistant's reload reported errors -- entities may be missing:")
		for _, line := range strings.Split(strings.TrimRight(string(output), "\n"), "\n") {
			fmt.Println("    " + line)
		}
	default:
		fmt.Println("  Home Assistant reloaded without errors")
	}
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
