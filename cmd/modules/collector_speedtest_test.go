package modules

import (
	"testing"
)

func TestParseSpeedtestOutput(t *testing.T) {
	// Snippet from the spec's sample output (bandwidth in bytes/s)
	raw := []byte(`{"type":"result","timestamp":"2026-04-26T12:42:00Z","ping":{"jitter":5.885,"latency":15.085},"download":{"bandwidth":85339924,"bytes":1244743792,"elapsed":15011},"upload":{"bandwidth":11191198,"bytes":124164513,"elapsed":11508},"packetLoss":0}`)

	entry, err := parseSpeedtestOutput(raw)
	if err != nil {
		t.Fatalf("parseSpeedtestOutput: %v", err)
	}

	// 85 339 924 bytes/s * 8 / 1_000_000 = 682.719392 → 682.72 Mbps
	wantDown := 682.72
	if entry.Download != wantDown {
		t.Errorf("download: want %.2f, got %.2f", wantDown, entry.Download)
	}

	// 11 191 198 * 8 / 1_000_000 = 89.529584 → 89.53 Mbps
	wantUp := 89.53
	if entry.Upload != wantUp {
		t.Errorf("upload: want %.2f, got %.2f", wantUp, entry.Upload)
	}
}

func TestParseSpeedtestOutput_InvalidJSON(t *testing.T) {
	_, err := parseSpeedtestOutput([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestBpsToMbps(t *testing.T) {
	cases := []struct {
		bps      int64
		wantMbps float64
	}{
		{1_000_000, 8.00}, // 1 MB/s = 8 Mbps
		{125_000, 1.00},   // 125 KB/s = 1 Mbps
		{0, 0.00},
		{85_339_924, 682.72}, // from spec sample
	}
	for _, tc := range cases {
		got := bpsToMbps(tc.bps)
		if got != tc.wantMbps {
			t.Errorf("bpsToMbps(%d): want %.2f, got %.2f", tc.bps, tc.wantMbps, got)
		}
	}
}

func TestBuildSpeedtestArgs(t *testing.T) {
	tests := []struct {
		name     string
		serverID string
		wantArgs []string
	}{
		{
			name:     "AUTO produces default args without --server-id",
			serverID: "AUTO",
			wantArgs: []string{"--accept-license", "--accept-gdpr", "--format=json"},
		},
		{
			name:     "lowercase auto produces default args without --server-id",
			serverID: "auto",
			wantArgs: []string{"--accept-license", "--accept-gdpr", "--format=json"},
		},
		{
			name:     "empty serverID produces default args without --server-id",
			serverID: "",
			wantArgs: []string{"--accept-license", "--accept-gdpr", "--format=json"},
		},
		{
			name:     "whitespace-only serverID produces default args without --server-id",
			serverID: "   ",
			wantArgs: []string{"--accept-license", "--accept-gdpr", "--format=json"},
		},
		{
			name:     "numeric serverID adds --server-id flag",
			serverID: "12345",
			wantArgs: []string{"--accept-license", "--accept-gdpr", "--format=json", "--server-id", "12345"},
		},
		{
			name:     "serverID with surrounding whitespace is trimmed",
			serverID: "  54321  ",
			wantArgs: []string{"--accept-license", "--accept-gdpr", "--format=json", "--server-id", "54321"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildSpeedtestArgs(tc.serverID)
			if len(got) != len(tc.wantArgs) {
				t.Fatalf("buildSpeedtestArgs(%q): got %v, want %v", tc.serverID, got, tc.wantArgs)
			}
			for i := range got {
				if got[i] != tc.wantArgs[i] {
					t.Errorf("buildSpeedtestArgs(%q)[%d]: got %q, want %q", tc.serverID, i, got[i], tc.wantArgs[i])
				}
			}
		})
	}
}
