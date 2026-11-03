package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildDeployPlan: the plan follows from the defs' deploy declarations (PROJECT.md 30) --
// machines from "via ssh", instances by kind, the cpu script via the hub or directly, and a
// warning for a LAN cpu host nothing says how to reach.
func TestBuildDeployPlan(t *testing.T) {
	physical := "home_assistant main: vienna via ssh hassio@h.example:1967;\nhome_assistant ha2mqtt: ps1;\ncoordinator on ps1;\n"
	instances := collectHomeAssistantInstances(physical)
	hosts, warnings := parseHostsIntegrationBody([]string{
		"device node.ps1 host ps1 cpu with:", "deploy via ssh erikp@h.example:1970;", "end;",
		"device node.ps2 host ps2 cpu with deploy as user erikp;",
		"device node.mqtt host mqtt cpu via cloud with deploy via ssh erikp@mqtt.example;",
		"device node.mini host mini cpu via cloud;",
		"device node.nas host nas cpu;",
	})
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	byID := map[string]THostDevice{}
	for _, d := range hosts {
		byID[d.DeviceID] = d
	}
	plan, planWarnings := buildDeployPlan("/x/House", physical, map[string]string{}, "vienna", instances, byID, map[string]TCommandlineDevice{"appliance.frame": {Host: "ps1"}})
	if len(plan.Instances) != 2 || plan.Instances[0].Name != "main" || plan.Instances[0].Kind != "ha_os" || plan.Instances[1].Kind != "container" || plan.Instances[1].Machine != "ps1" {
		t.Errorf("instances = %+v", plan.Instances)
	}
	if plan.Machines["vienna"].Port != 1967 || plan.Machines["ps1"].User != "erikp" || plan.Machines["mqtt"].Host != "mqtt.example" {
		t.Errorf("machines = %+v", plan.Machines)
	}
	if plan.Coordinator != "ps1" || plan.CPU.Hub != "ps1" || len(plan.CPU.ViaHub) != 2 || len(plan.CPU.Direct) != 1 || plan.CPU.Direct[0] != "mqtt" {
		t.Errorf("coordinator %q, cpu = %+v", plan.Coordinator, plan.CPU)
	}
	if len(plan.Containers) != 1 || len(plan.Commandline) != 1 || len(planWarnings) != 1 {
		t.Errorf("containers %v, commandline %v, warnings %v (want one: nas)", plan.Containers, plan.Commandline, planWarnings)
	}
}

// TestRunDeployDryRun: "-deploy -n" prints the plan's commands without running any, and never
// the broker password.
func TestRunDeployDryRun(t *testing.T) {
	dir := t.TempDir()
	plan := "installation: x\nsources:\n  generator: /src\n  integrations: /int\nmachines:\n  hub:\n    user: erikp\n    host: h.example\n    port: 1970\n  main:\n    user: hassio\n    host: h.example\n    port: 1967\ncoordinator: hub\ninstances:\n  - name: main\n    machine: main\n    kind: ha_os\n    files: hass/main\n    config: /config\ncpu:\n  hub: hub\nping: hub\n"
	mustWrite := func(path, content string) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite(filepath.Join(dir, "deploy.yaml"), plan)
	mustWrite(filepath.Join(dir, "coordinator", "secrets.yaml"), "brokers:\n  cloud_coordinator:\n    server: mqtt.example\n    port: 8883\n    login: c\n    password: s3cret\n")
	output := captureStdout(t, func() {
		if err := runDeploy(dir, true, nil); err != nil {
			t.Fatal(err)
		}
	})
	for _, want := range []string{"bash ./coordinator/deploy", "x/meta/reload/main/request", "-P '****'", "erikp@h.example:cpu/"} {
		if !strings.Contains(output, want) {
			t.Errorf("dry run lacks %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "s3cret") {
		t.Errorf("dry run prints the broker password")
	}
}

// TestRunDeployOnly: "-only" deploys just the parts named, and rejects a name no part has.
func TestRunDeployOnly(t *testing.T) {
	dir := t.TempDir()
	plan := "installation: x\nsources:\n  generator: /src\n  integrations: /int\nmachines:\n  hub:\n    user: erikp\n    host: h.example\ncoordinator: hub\ncpu:\n  hub: hub\nping: hub\ncontainers:\n  - hub\n"
	if err := os.WriteFile(filepath.Join(dir, "deploy.yaml"), []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	output := captureStdout(t, func() {
		if err := runDeploy(dir, true, []string{"containers"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(output, "update.containers") || strings.Contains(output, "coordinator/deploy") || strings.Contains(output, "/cpu") {
		t.Errorf("-only containers deployed something else:\n%s", output)
	}
	if err := runDeploy(dir, true, []string{"nonsense"}); err == nil {
		t.Errorf("-only nonsense was accepted")
	}
}
