package main

import "testing"

func TestParseRoutingKeywords(t *testing.T) {
	cases := []struct {
		name         string
		line         string
		wantRest     string
		wantViaCloud bool
	}{
		{
			name:     "no keyword, bare device line",
			line:     "device node.x host x cpu;",
			wantRest: "device node.x host x cpu;",
		},
		{
			name:         "via cloud",
			line:         "device node.x host x cpu via cloud;",
			wantRest:     "device node.x host x cpu ;",
			wantViaCloud: true,
		},
		{
			name:         "with: suffix",
			line:         "device node.x host x cpu via cloud with:",
			wantRest:     "device node.x host x cpu with:",
			wantViaCloud: true,
		},
		{
			name:     "the retired \"cloud import\" is not recognised",
			line:     "device node.x host x cpu cloud import;",
			wantRest: "device node.x host x cpu cloud import;",
		},
		{
			name:     "unrecognised suffix left untouched",
			line:     "device node.x host x cpu",
			wantRest: "device node.x host x cpu",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rest, viaCloud := parseRoutingKeywords(c.line)
			if rest != c.wantRest || viaCloud != c.wantViaCloud {
				t.Errorf("parseRoutingKeywords(%q) = (%q, %v), want (%q, %v)", c.line, rest, viaCloud, c.wantRest, c.wantViaCloud)
			}
		})
	}
}
