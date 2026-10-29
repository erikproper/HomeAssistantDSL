package main

import (
	"strings"
	"testing"
)

// TestIntegrationNodeReporting: an "integration <name> is available" node (PROJECT.md 25) reports
// from its two template edges, not from a state trigger on a non-entity.
func TestIntegrationNodeReporting(t *testing.T) {
	source := "integration overkiz is available"
	if name, ok := integrationSourceName(source); !ok || name != "overkiz" {
		t.Fatalf("integrationSourceName = %q, %v", name, ok)
	}
	if _, ok := integrationSourceName("cover.kitchen is available"); ok {
		t.Errorf("an entity source isn't an integration source")
	}
	expr := sourceToJinja2(source)
	if !strings.Contains(expr, "integration_entities('overkiz')") {
		t.Errorf("expr = %s", expr)
	}
	body := entityReportingAutomationBody(TEntityIdentity{}, []string{bareEntityFromSource(source)}, "t", expr, "")
	if strings.Contains(body, "platform: state") || strings.Count(body, "platform: template") != 2 {
		t.Errorf("want two template triggers and no state trigger:\n%s", body)
	}
}
