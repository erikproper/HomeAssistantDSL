package main

import "testing"

func TestParseRoutingKeywords(t *testing.T) {
	cases := []struct {
		name           string
		line           string
		wantRest       string
		wantCloud      bool
		wantSelfImport bool
	}{
		{
			name:     "no keywords, bare device line",
			line:     "device host.x host cpu;",
			wantRest: "device host.x host cpu ;",
		},
		{
			name:      "cloud only",
			line:      "device host.x host cpu cloud;",
			wantRest:  "device host.x host cpu ;",
			wantCloud: true,
		},
		{
			name:           "cloud and import",
			line:           "device host.x host cpu cloud import;",
			wantRest:       "device host.x host cpu ;",
			wantCloud:      true,
			wantSelfImport: true,
		},
		{
			name:           "import and cloud, reverse order",
			line:           "device host.x host cpu import cloud;",
			wantRest:       "device host.x host cpu ;",
			wantCloud:      true,
			wantSelfImport: true,
		},
		{
			name:           "with: suffix",
			line:           "device host.x host home_assistant cloud import with:",
			wantRest:       "device host.x host home_assistant with:",
			wantCloud:      true,
			wantSelfImport: true,
		},
		{
			name:           "import alone, no cloud -- still parsed here, meaninglessness is the caller's concern",
			line:           "device host.x host cpu import;",
			wantRest:       "device host.x host cpu ;",
			wantSelfImport: true,
		},
		{
			name:     "unrecognised suffix left untouched",
			line:     "device host.x host cpu",
			wantRest: "device host.x host cpu",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rest, cloud, selfImport := parseRoutingKeywords(c.line)
			if rest != c.wantRest || cloud != c.wantCloud || selfImport != c.wantSelfImport {
				t.Errorf("parseRoutingKeywords(%q) = (%q, %v, %v), want (%q, %v, %v)",
					c.line, rest, cloud, selfImport, c.wantRest, c.wantCloud, c.wantSelfImport)
			}
		})
	}
}
