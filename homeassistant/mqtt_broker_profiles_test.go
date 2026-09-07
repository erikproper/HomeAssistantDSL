package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDefFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write %s: %v", name, err)
		}
	}
	return dir
}

func TestResolveMQTTBrokerProfilesParsesMultipleNamedProfiles(t *testing.T) {
	dir := writeDefFiles(t, map[string]string{
		"Physical.def": `physical layer with:
  mqtt main:
    server   ${main_mqtt_server};
    login    ${main_mqtt_login};
    password ${main_mqtt_password};
    port     ${main_mqtt_port};
  end;

  mqtt cloud_client:
    server   mqtt.erikproper.eu;
    login    ${cloud_mqtt_login};
    password ${cloud_mqtt_password};
    port     8883;
    tls      true;
  end;

  mqtt cloud_coordinator:
    server          mqtt.erikproper.eu;
    login           coordinator;
    password        coordinator-secret;
    port            8883;
    tls             true;
    coordinator_only true;
  end;
end;
`,
		"Settings.def": `${main_mqtt_server} = "junglinster";
${main_mqtt_login} = "junglinster-user";
${main_mqtt_password} = "junglinster-pass";
${main_mqtt_port} = "1883";
${cloud_mqtt_login} = "client";
${cloud_mqtt_password} = "cloud-pass";
`,
	})

	profiles, warnings := resolveMQTTBrokerProfiles(dir)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(profiles) != 3 {
		t.Fatalf("got %d profiles, want 3: %+v", len(profiles), profiles)
	}

	main, ok := profiles["main"]
	if !ok {
		t.Fatalf("expected a \"main\" profile, got %v", profiles)
	}
	if main.Server != "junglinster" || main.Login != "junglinster-user" || main.Password != "junglinster-pass" || main.Port != "1883" || main.TLS || main.CoordinatorOnly {
		t.Errorf("main = %+v, want junglinster/junglinster-user/junglinster-pass/1883/TLS=false/CoordinatorOnly=false", main)
	}

	client, ok := profiles["cloud_client"]
	if !ok {
		t.Fatalf("expected a \"cloud_client\" profile, got %v", profiles)
	}
	if client.Server != "mqtt.erikproper.eu" || client.Login != "client" || client.Password != "cloud-pass" || client.Port != "8883" || !client.TLS || client.CoordinatorOnly {
		t.Errorf("cloud_client = %+v, want mqtt.erikproper.eu/client/cloud-pass/8883/TLS=true/CoordinatorOnly=false", client)
	}

	coordinator, ok := profiles["cloud_coordinator"]
	if !ok {
		t.Fatalf("expected a \"cloud_coordinator\" profile, got %v", profiles)
	}
	if !coordinator.TLS || !coordinator.CoordinatorOnly {
		t.Errorf("cloud_coordinator = %+v, want TLS=true/CoordinatorOnly=true", coordinator)
	}
}

func TestResolveMQTTBrokerSecretsStillReturnsMainOnly(t *testing.T) {
	dir := writeDefFiles(t, map[string]string{
		"Physical.def": `physical layer with:
  mqtt main:
    server   ${main_mqtt_server};
    login    ${main_mqtt_login};
    password ${main_mqtt_password};
    port     ${main_mqtt_port};
  end;

  mqtt cloud_client:
    server mqtt.erikproper.eu;
    login  client;
    password secret;
    port   8883;
    tls    true;
  end;
end;
`,
		"Settings.def": `${main_mqtt_server} = "junglinster";
${main_mqtt_login} = "u";
${main_mqtt_password} = "p";
${main_mqtt_port} = "1883";
`,
	})

	secrets, ok := resolveMQTTBrokerSecrets(dir)
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if secrets.Server != "junglinster" || secrets.TLS {
		t.Errorf("secrets = %+v, want the \"main\" profile only, TLS=false", secrets)
	}
}

func TestGenerateCPUBrokerSecretsFilesExcludesCoordinatorOnly(t *testing.T) {
	outputRoot := t.TempDir()
	profiles := map[string]TMQTTBrokerSecrets{
		"main":              {Server: "junglinster", Login: "u", Password: "p", Port: "1883"},
		"cloud_client":      {Server: "mqtt.erikproper.eu", Login: "client", Password: "cp", Port: "8883", TLS: true},
		"cloud_coordinator": {Server: "mqtt.erikproper.eu", Login: "coordinator", Password: "coordp", Port: "8883", TLS: true, CoordinatorOnly: true},
	}

	if err := generateCPUBrokerSecretsFiles(outputRoot, profiles, ""); err != nil {
		t.Fatalf("generateCPUBrokerSecretsFiles error: %v", err)
	}

	defaultContent, err := os.ReadFile(filepath.Join(outputRoot, "cpu", "secrets.default"))
	if err != nil {
		t.Fatalf("expected cpu/secrets.default (from \"main\"): %v", err)
	}
	if !strings.Contains(string(defaultContent), "mqtt_server=junglinster") || !strings.Contains(string(defaultContent), "mqtt_tls=0") {
		t.Errorf("secrets.default content = %q, missing expected fields", defaultContent)
	}

	clientContent, err := os.ReadFile(filepath.Join(outputRoot, "cpu", "secrets.cloud_client"))
	if err != nil {
		t.Fatalf("expected cpu/secrets.cloud_client: %v", err)
	}
	if !strings.Contains(string(clientContent), "mqtt_tls=1") {
		t.Errorf("secrets.cloud_client content = %q, want mqtt_tls=1", clientContent)
	}

	if _, err := os.Stat(filepath.Join(outputRoot, "cpu", "secrets.cloud_coordinator")); err == nil {
		t.Errorf("expected NO cpu/secrets.cloud_coordinator -- CoordinatorOnly profiles must never be written into the leaf-facing cpu/ output")
	}

	if _, err := os.Stat(filepath.Join(outputRoot, "cpu", "secrets.main")); err == nil {
		t.Errorf("expected \"main\" to be written as secrets.default, not secrets.main")
	}
}

// TestGenerateCPUBrokerSecretsFilesWritesInstallationAlias covers the 2026-09-02 addition: a
// second copy of every non-"main" leaf-usable profile, written under "secrets.<installation>" --
// identical content to "secrets.<profile-name>" -- so the report/deploy scripts can address "the
// cloud profile for this installation" generically, without knowing the literal profile name.
func TestGenerateCPUBrokerSecretsFilesWritesInstallationAlias(t *testing.T) {
	outputRoot := t.TempDir()
	profiles := map[string]TMQTTBrokerSecrets{
		"main":         {Server: "junglinster", Login: "u", Password: "p", Port: "1883"},
		"cloud_client": {Server: "mqtt.erikproper.eu", Login: "client", Password: "cp", Port: "8883", TLS: true},
	}

	if err := generateCPUBrokerSecretsFiles(outputRoot, profiles, "junglinster"); err != nil {
		t.Fatalf("generateCPUBrokerSecretsFiles error: %v", err)
	}

	clientContent, err := os.ReadFile(filepath.Join(outputRoot, "cpu", "secrets.cloud_client"))
	if err != nil {
		t.Fatalf("expected cpu/secrets.cloud_client: %v", err)
	}
	aliasContent, err := os.ReadFile(filepath.Join(outputRoot, "cpu", "secrets.junglinster"))
	if err != nil {
		t.Fatalf("expected cpu/secrets.junglinster (installation alias of secrets.cloud_client): %v", err)
	}
	if string(aliasContent) != string(clientContent) {
		t.Errorf("secrets.junglinster = %q, want identical to secrets.cloud_client = %q", aliasContent, clientContent)
	}
}

// TestGenerateCPUBrokerSecretsFilesNoInstallationAliasWhenUnset confirms no alias file is written
// at all when installation is "" (matching "main"'s own report-time behaviour of never needing
// installation-qualification).
func TestGenerateCPUBrokerSecretsFilesNoInstallationAliasWhenUnset(t *testing.T) {
	outputRoot := t.TempDir()
	profiles := map[string]TMQTTBrokerSecrets{
		"cloud_client": {Server: "mqtt.erikproper.eu", Login: "client", Password: "cp", Port: "8883", TLS: true},
	}

	if err := generateCPUBrokerSecretsFiles(outputRoot, profiles, ""); err != nil {
		t.Fatalf("generateCPUBrokerSecretsFiles error: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(outputRoot, "cpu"))
	if err != nil {
		t.Fatalf("reading cpu dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "secrets.cloud_client" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("cpu dir entries = %v, want exactly [secrets.cloud_client]", names)
	}
}
