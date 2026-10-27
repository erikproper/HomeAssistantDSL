package main

import (
	"strings"
	"testing"
)

// TestWriteHostMetadata: a one-liner becomes a block once it has metadata, a block's metadata is
// rewritten right after its header with forced values kept, and other lines stay.
func TestWriteHostMetadata(t *testing.T) {
	content := `integration hosts with:
    device node.pi pi cpu;
    device node.mini mini cpu cloud import with:
      suggested_area: "office";
      model: "Mac" forced;
      sw_version: "macOS 25";
    end;
    device node.tv sony_tv ping;
end;`
	hosts := map[string]THostDevice{
		"node.pi":   {DeviceID: "node.pi", HostName: "pi", IntegrationType: "cpu"},
		"node.mini": {DeviceID: "node.mini", HostName: "mini", IntegrationType: "cpu", ConstantAttributes: map[string]THostConstantAttribute{"model": {Value: "Mac", Forced: true}, "sw_version": {Value: "macOS 25"}}},
		"node.tv":   {DeviceID: "node.tv", HostName: "sony_tv", IntegrationType: "ping"},
	}
	info := map[string]map[string]string{
		"node.pi":   {"model": "Raspberry Pi 4", "serial_number": "123"},
		"node.mini": {"model": "Mac16,10", "sw_version": "macOS 26.1"},
	}
	want := `integration hosts with:
    device node.pi pi cpu with:
      model:         "Raspberry Pi 4";
      serial_number: "123";
    end;
    device node.mini mini cpu cloud import with:
      model:      "Mac" forced;   # discovery: "Mac16,10"
      sw_version: "macOS 26.1";
      suggested_area: "office";
    end;
    device node.tv sony_tv ping;
end;`
	got, changed := writeHostMetadata(content, hosts, info)
	if got != want {
		t.Errorf("writeHostMetadata:\n%s\nwant:\n%s", got, want)
	}
	if changed != 2 {
		t.Errorf("changed = %d, want 2", changed)
	}
	if again, changedAgain := writeHostMetadata(want, hosts, info); again != want || changedAgain != 0 {
		t.Errorf("not stable: changed %d\n%s", changedAgain, again)
	}
}

// TestWriteBridgeMetadata: a bridge device's metadata may follow its capabilities and nested
// blocks; it's rewritten in its place, a value nothing reports stays, via_device is captured.
func TestWriteBridgeMetadata(t *testing.T) {
	content := `  integration home_assistant ha2mqtt with:
    device hass.shutter with:
      capability cover.core cover.front as x;
      capability number.my_position number.front_my with:
        model: "inside a nested block";
      end;
      manufacturer: "Somfy";
      model: "RollerShutter";
    end;
  end;`
	bridge := map[string]THassBridgeDevice{"hass.shutter": {DeviceID: "hass.shutter", ConstantAttributes: map[string]THostConstantAttribute{
		"manufacturer": {Value: "Somfy"}, "model": {Value: "RollerShutter"},
	}}}
	info := map[string]map[string]string{"hass.shutter": {"sw_version": "2.1", "via_device": "hass.tahoma"}}
	want := `  integration home_assistant ha2mqtt with:
    device hass.shutter with:
      capability cover.core cover.front as x;
      capability number.my_position number.front_my with:
        model: "inside a nested block";
      end;
      manufacturer: "Somfy";
      model:        "RollerShutter";
      sw_version:   "2.1";
      via_device:   hass.tahoma;
    end;
  end;`
	got, changed := writeBridgeMetadata(content, bridge, info)
	// A device reference is written unquoted, and read back as one (Erik, 2026-10-27).
	if m := discoveryMetadataPattern.FindStringSubmatch(quoteDeviceReference("via_device:   hass.tahoma;")); m == nil || m[2] != "hass.tahoma" {
		t.Errorf("an unquoted via_device isn't read back: %q", m)
	}
	if got != want || changed != 1 {
		t.Errorf("writeBridgeMetadata (changed %d):\n%s\nwant:\n%s", changed, got, want)
	}
	// An exported device's header too (found live 2026-10-27: Junglinster's Vienna Netatmos).
	exported := strings.Replace(content, "device hass.shutter with:", "device hass.shutter export with:", 1)
	if got, changed := writeBridgeMetadata(exported, bridge, info); changed != 1 || !strings.Contains(got, `via_device:   hass.tahoma;`) {
		t.Errorf("an exported device's metadata isn't written (changed %d):\n%s", changed, got)
	}
}
