package main

import "testing"

// TestMetadataValue: a JSON field (string or number), or the whole message; nothing usable is no value.
func TestMetadataValue(t *testing.T) {
	for _, c := range []struct {
		payload, key, want string
		ok                 bool
	}{
		{`{"version":"3.8.4","hostname":"ems-esp"}`, "version", "3.8.4", true},
		{`{"value":"11.24.3","time":1}`, "value", "11.24.3", true},
		{`{"build":42}`, "build", "42", true},
		{`{"version":""}`, "version", "", false},
		{`not json`, "version", "", false},
		{` 2.14.2 `, "", "2.14.2", true},
	} {
		if got, ok := metadataValue([]byte(c.payload), c.key); got != c.want || ok != c.ok {
			t.Errorf("metadataValue(%q, %q) = %q, %v; want %q, %v", c.payload, c.key, got, ok, c.want, c.ok)
		}
	}
}
