package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/taoyao-code/iot-server/internal/protocol/bkv"
)

func TestParseMACVariants(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []byte
	}{
		{"plain", "854121800889", []byte{0x85, 0x41, 0x21, 0x80, 0x08, 0x89}},
		{"colonSeparated", "85:41:21:80:08:89", []byte{0x85, 0x41, 0x21, 0x80, 0x08, 0x89}},
		{"dashSeparated", "85-41-21-80-08-89", []byte{0x85, 0x41, 0x21, 0x80, 0x08, 0x89}},
		{"dotSeparated", "8541.2180.0889", []byte{0x85, 0x41, 0x21, 0x80, 0x08, 0x89}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseMAC(tc.in)
			if err != nil {
				t.Fatalf("parseMAC returned error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("len mismatch: got %d want %d", len(got), len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("byte %d mismatch: got 0x%02x want 0x%02x", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestLoadNetworkConfigWithGeneratedFile(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	script := filepath.Join(wd, "generate_configs.sh")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("script not found: %v", err)
	}

	tmp := t.TempDir()
	cmd := exec.Command("bash", script, "1")
	cmd.Dir = tmp
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generate_configs.sh failed: %v output=%s", err, out)
	}

	configPath := filepath.Join(tmp, "network_config_gateway_001.json")
	channel, nodes := loadNetworkConfig(configPath)

	if channel != 3 { // CHANNELS 数组第一项
		t.Fatalf("channel mismatch: got %d want 3", channel)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if nodes[0].Slot != 1 {
		t.Fatalf("slot mismatch: got %d want 1", nodes[0].Slot)
	}
	wantMAC := [6]byte{0x85, 0x41, 0x21, 0x80, 0x08, 0x89}
	if nodes[0].MAC != wantMAC {
		t.Fatalf("mac mismatch: got %x want %x", nodes[0].MAC, wantMAC)
	}
}

func TestBuildNetworkRefreshFrame(t *testing.T) {
	frame := &bkv.Frame{GatewayID: "82241218000382"}
	nodes := []networkNode{
		{Slot: 1, MAC: [6]byte{0x85, 0x41, 0x21, 0x80, 0x08, 0x89}},
	}
	raw := buildNetworkRefresh(frame, 6, nodes)
	if len(raw) == 0 {
		t.Fatal("buildNetworkRefresh returned empty slice")
	}
	parsed, err := bkv.Parse(raw)
	if err != nil {
		t.Fatalf("bkv.Parse failed: %v", err)
	}
	if parsed.Cmd != 0x0005 {
		t.Fatalf("cmd mismatch: got 0x%04x", parsed.Cmd)
	}
	if parsed.Direction != 0x00 {
		t.Fatalf("direction mismatch: got 0x%02x", parsed.Direction)
	}
	if parsed.Data[2] != 0x08 {
		t.Fatalf("subcmd mismatch: got 0x%02x", parsed.Data[2])
	}
	if parsed.Data[3] != 6 {
		t.Fatalf("channel mismatch: got %d", parsed.Data[3])
	}
	if parsed.Data[4] != 1 {
		t.Fatalf("slot mismatch: got %d", parsed.Data[4])
	}
	if got := parsed.Data[5:11]; string(got) != string([]byte{0x85, 0x41, 0x21, 0x80, 0x08, 0x89}) {
		t.Fatalf("mac mismatch: got %x", got)
	}
}

func TestBuildQuerySocketStatus(t *testing.T) {
	frame := &bkv.Frame{GatewayID: "82241218000382"}
	nodes := []networkNode{{Slot: 2}}
	raw := buildQuerySocketStatus(frame, nodes)
	parsed, err := bkv.Parse(raw)
	if err != nil {
		t.Fatalf("bkv.Parse failed: %v", err)
	}
	if parsed.Cmd != 0x0015 {
		t.Fatalf("cmd mismatch: 0x%04x", parsed.Cmd)
	}
	if parsed.Data[2] != 0x1D {
		t.Fatalf("subcmd mismatch: got 0x%02x", parsed.Data[2])
	}
	if parsed.Data[3] != 0x02 {
		t.Fatalf("socket mismatch: got %d", parsed.Data[3])
	}
}

func TestBuildStartChargeCommand(t *testing.T) {
	frame := &bkv.Frame{GatewayID: "82241218000382"}
	nodes := []networkNode{{Slot: 3}}
	raw := buildStartChargeCommand(frame, nodes)
	parsed, err := bkv.Parse(raw)
	if err != nil {
		t.Fatalf("bkv.Parse failed: %v", err)
	}
	data := parsed.Data
	if data[2] != 0x07 {
		t.Fatalf("subcmd mismatch: got 0x%02x", data[2])
	}
	if data[3] != 0x03 {
		t.Fatalf("socket mismatch: got %d", data[3])
	}
	if data[4] != 0x00 {
		t.Fatalf("port mismatch: got %d", data[4])
	}
	if data[5] != 0x01 {
		t.Fatalf("switch flag mismatch: got %d", data[5])
	}
	if data[6] != 0x01 {
		t.Fatalf("mode mismatch: got %d", data[6])
	}
}

func TestNextFrameWithMultipleFrames(t *testing.T) {
	frame1 := bkv.BuildUplink(0x0000, 1, "82241218000382", []byte{0x00, 0x02, 0x0c, 0x01, 0x01})
	frame2 := bkv.BuildUplink(0x0015, 2, "82241218000382", []byte{0x00, 0x02, 0x0c, 0x01, 0x01})
	buf := append(frame1, frame2...)

	first, rest, err := nextFrame(buf)
	if err != nil {
		t.Fatalf("nextFrame failed: %v", err)
	}
	if len(first) != len(frame1) {
		t.Fatalf("first frame length mismatch")
	}
	if len(rest) != len(frame2) {
		t.Fatalf("rest length mismatch")
	}
	if string(rest) != string(frame2) {
		t.Fatalf("rest content mismatch")
	}
}

func TestEncodeTimestampBCD(t *testing.T) {
	ts := time.Date(2025, 11, 26, 8, 45, 13, 0, time.UTC)
	got := encodeTimestampBCD(ts)
	want := []byte{0x20, 0x25, 0x11, 0x26, 0x08, 0x45, 0x13}
	if len(got) != len(want) {
		t.Fatalf("length mismatch: got %d want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("byte %d mismatch: got 0x%02x want 0x%02x", i, got[i], want[i])
		}
	}
}

func TestIsNetworkRefreshAck(t *testing.T) {
	ack := &bkv.Frame{
		Data: []byte{0x00, 0x01, 0x08, 0x01},
	}
	if !isNetworkRefreshAck(ack) {
		t.Fatalf("expected true for valid ack")
	}

	invalid := &bkv.Frame{
		Data: []byte{0x00, 0x01, 0x09, 0x00},
	}
	if isNetworkRefreshAck(invalid) {
		t.Fatalf("expected false for invalid ack")
	}
}
