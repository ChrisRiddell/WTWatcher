package modules

import (
	"testing"
)

func TestParseConfig_Defaults(t *testing.T) {
	yaml := `
Schedule:
    Ping: 15 Minutes
    Speedtest: 3 Hours
    Archiving: 14 Days
    LogRotation: 14 Days

Ping:
    Count: 4
    Timeout: 10 Seconds
    Retries: 2
    AnomalyThresholdMs: 2000

Speedtest:
    ServerID: AUTO

Addresses:
    Gateway:
        IPv4: 192.168.1.1
    Cloudflare DNS:
        IPv6: 2606:4700:4700::1111
    Youtube:
        Domain: youtube.com
        Protocol: Both
`
	cfg, err := ParseConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Schedule.PingSeconds != 15*60 {
		t.Errorf("ping: want 900s, got %d", cfg.Schedule.PingSeconds)
	}
	if cfg.Schedule.SpeedtestSeconds != 3*3600 {
		t.Errorf("speedtest: want 10800s, got %d", cfg.Schedule.SpeedtestSeconds)
	}
	if cfg.Schedule.ArchivingSeconds != 14*86400 {
		t.Errorf("archiving: want 1209600s, got %d", cfg.Schedule.ArchivingSeconds)
	}
	if cfg.Schedule.LogRotationSeconds != 14*86400 {
		t.Errorf("log rotation: want 1209600s, got %d", cfg.Schedule.LogRotationSeconds)
	}
	if cfg.Ping.Count != 4 {
		t.Errorf("ping count: want 4, got %d", cfg.Ping.Count)
	}
	if cfg.Ping.TimeoutSeconds != 10 {
		t.Errorf("ping timeout: want 10s, got %d", cfg.Ping.TimeoutSeconds)
	}
	if cfg.Ping.Retries != 2 {
		t.Errorf("ping retries: want 2, got %d", cfg.Ping.Retries)
	}
	if cfg.Ping.AnomalyThresholdMs != 2000 {
		t.Errorf("ping anomaly threshold: want 2000, got %d", cfg.Ping.AnomalyThresholdMs)
	}
	if cfg.Speedtest.ServerID != "AUTO" {
		t.Errorf("speedtest server id: want AUTO, got %q", cfg.Speedtest.ServerID)
	}
	if len(cfg.Addresses) != 3 {
		t.Errorf("addresses: want 3, got %d", len(cfg.Addresses))
	}
}

func TestParseConfig_InvalidIP(t *testing.T) {
	yaml := `
Schedule:
    Ping: 15 Minutes
    Speedtest: 3 Hours
    Archiving: 14 Days
    LogRotation: 14 Days
Ping:
    Count: 4
    Timeout: 10 Seconds
    Retries: 2
    AnomalyThresholdMs: 2000
Addresses:
    Bad:
        IPv4: not-an-ip
`
	_, err := ParseConfig([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for invalid IP, got nil")
	}
}

func TestParseConfig_InvalidProtocol(t *testing.T) {
	yaml := `
Schedule:
    Ping: 15 Minutes
    Speedtest: 3 Hours
    Archiving: 14 Days
    LogRotation: 14 Days
Ping:
    Count: 4
    Timeout: 10 Seconds
    Retries: 2
    AnomalyThresholdMs: 2000
Addresses:
    Site:
        Domain: example.com
        Protocol: UDP
`
	_, err := ParseConfig([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for invalid protocol, got nil")
	}
}

func TestParseConfig_InvalidInterval(t *testing.T) {
	yaml := `
Schedule:
    Ping: 15 Weeks
    Speedtest: 3 Hours
    Archiving: 14 Days
    LogRotation: 14 Days
Ping:
    Count: 4
    Timeout: 10 Seconds
    Retries: 2
    AnomalyThresholdMs: 2000
Addresses: {}
`
	_, err := ParseConfig([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for invalid interval unit, got nil")
	}
}

func TestParseConfig_InvalidDomain(t *testing.T) {
	yaml := `
Schedule:
    Ping: 1 Minutes
    Speedtest: 1 Hours
    Archiving: 1 Days
    LogRotation: 14 Days
Ping:
    Count: 4
    Timeout: 10 Seconds
    Retries: 2
    AnomalyThresholdMs: 2000
Addresses:
    Bad:
        Domain: not a domain
`
	_, err := ParseConfig([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for invalid domain, got nil")
	}
}

func TestParseConfig_IPv6Address(t *testing.T) {
	yaml := `
Schedule:
    Ping: 5 Minutes
    Speedtest: 1 Hours
    Archiving: 7 Days
    LogRotation: 14 Days
Ping:
    Count: 4
    Timeout: 10 Seconds
    Retries: 2
    AnomalyThresholdMs: 2000
Addresses:
    CloudflareDNS:
        IPv6: 2606:4700:4700::1111
`
	cfg, err := ParseConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Addresses) != 1 {
		t.Fatalf("want 1 address, got %d", len(cfg.Addresses))
	}
	a := cfg.Addresses[0]
	if a.IPv6 == nil {
		t.Fatal("expected IPv6 to be set")
	}
	if !a.IPv6.Is6() {
		t.Errorf("expected IPv6 address")
	}
}

func TestParseConfig_MissingIPAndDomain(t *testing.T) {
	yaml := `
Schedule:
    Ping: 5 Minutes
    Speedtest: 1 Hours
    Archiving: 7 Days
    LogRotation: 14 Days
Ping:
    Count: 4
    Timeout: 10 Seconds
    Retries: 2
    AnomalyThresholdMs: 2000
Addresses:
    Empty: {}
`
	_, err := ParseConfig([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for address missing IP and Domain")
	}
}

func TestParseConfig_OffInterval(t *testing.T) {
	yaml := `
Schedule:
    Ping: 1 Minutes
    Speedtest: OFF
    Archiving: 14 Days
    LogRotation: OFF
Ping:
    Count: 4
    Timeout: 10 Seconds
    Retries: 2
    AnomalyThresholdMs: 2000
Addresses:
    Local:
        IPv4: 127.0.0.1
`
	cfg, err := ParseConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Schedule.SpeedtestSeconds != 0 {
		t.Errorf("speedtest: want 0, got %d", cfg.Schedule.SpeedtestSeconds)
	}
	if cfg.Schedule.LogRotationSeconds != 0 {
		t.Errorf("log rotation: want 0, got %d", cfg.Schedule.LogRotationSeconds)
	}
}

func TestParseConfig_AddressOrder(t *testing.T) {
	// Addresses are intentionally out of alphabetical order to prove that
	// document order is preserved. Alphabetical would be: Cloudflare DNS,
	// Gateway, Youtube. Config order is: Gateway, Cloudflare DNS, Youtube.
	yaml := `
Schedule:
    Ping: 1 Minutes
    Speedtest: OFF
    Archiving: 1 Days
    LogRotation: 14 Days

Ping:
    Count: 4
    Timeout: 10 Seconds
    Retries: 2
    AnomalyThresholdMs: 2000

Addresses:
    Gateway:
        IPv4: 192.168.1.1
    Cloudflare DNS:
        IPv4: 1.1.1.1
    Youtube:
        Domain: youtube.com
        Protocol: IPv4
`
	cfg, err := ParseConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Addresses) != 3 {
		t.Fatalf("want 3 addresses, got %d", len(cfg.Addresses))
	}
	want := []string{"Gateway", "Cloudflare DNS", "Youtube"}
	for i, name := range want {
		if cfg.Addresses[i].Name != name {
			t.Errorf("address[%d]: want %q, got %q", i, name, cfg.Addresses[i].Name)
		}
	}
}

func TestParseConfig_PingValidation(t *testing.T) {
	baseValid := `
Schedule:
    Ping: 5 Minutes
    Speedtest: OFF
    Archiving: 7 Days
    LogRotation: 14 Days
Addresses:
    Local:
        IPv4: 127.0.0.1
`

	tests := []struct {
		name      string
		pingBlock string
		wantErr   bool
	}{
		{
			name:      "missing Ping section",
			pingBlock: "",
			wantErr:   true,
		},
		{
			name: "missing Count",
			pingBlock: `
Ping:
    Timeout: 10 Seconds
    Retries: 2
    AnomalyThresholdMs: 2000
`,
			wantErr: true,
		},
		{
			name: "zero Count",
			pingBlock: `
Ping:
    Count: 0
    Timeout: 10 Seconds
    Retries: 2
    AnomalyThresholdMs: 2000
`,
			wantErr: true,
		},
		{
			name: "negative Count",
			pingBlock: `
Ping:
    Count: -1
    Timeout: 10 Seconds
    Retries: 2
    AnomalyThresholdMs: 2000
`,
			wantErr: true,
		},
		{
			name: "missing Timeout",
			pingBlock: `
Ping:
    Count: 4
    Retries: 2
    AnomalyThresholdMs: 2000
`,
			wantErr: true,
		},
		{
			name: "invalid Timeout unit",
			pingBlock: `
Ping:
    Count: 4
    Timeout: 10 Years
    Retries: 2
    AnomalyThresholdMs: 2000
`,
			wantErr: true,
		},
		{
			name: "disallowed Timeout unit Minutes",
			pingBlock: `
Ping:
    Count: 4
    Timeout: 5 Minutes
    Retries: 2
    AnomalyThresholdMs: 2000
`,
			wantErr: true,
		},
		{
			name: "disallowed Timeout unit Hours",
			pingBlock: `
Ping:
    Count: 4
    Timeout: 1 Hours
    Retries: 2
    AnomalyThresholdMs: 2000
`,
			wantErr: true,
		},
		{
			name: "disallowed Timeout OFF",
			pingBlock: `
Ping:
    Count: 4
    Timeout: OFF
    Retries: 2
    AnomalyThresholdMs: 2000
`,
			wantErr: true,
		},
		{
			name: "disallowed decimal Timeout",
			pingBlock: `
Ping:
    Count: 4
    Timeout: 0.5 Seconds
    Retries: 2
    AnomalyThresholdMs: 2000
`,
			wantErr: true,
		},
		{
			name: "zero Timeout",
			pingBlock: `
Ping:
    Count: 4
    Timeout: 0 Seconds
    Retries: 2
    AnomalyThresholdMs: 2000
`,
			wantErr: true,
		},
		{
			name: "singular Second Timeout is valid",
			pingBlock: `
Ping:
    Count: 4
    Timeout: 1 Second
    Retries: 2
    AnomalyThresholdMs: 2000
`,
			wantErr: false,
		},
		{
			name: "missing Retries",
			pingBlock: `
Ping:
    Count: 4
    Timeout: 10 Seconds
    AnomalyThresholdMs: 2000
`,
			wantErr: true,
		},
		{
			name: "negative Retries",
			pingBlock: `
Ping:
    Count: 4
    Timeout: 10 Seconds
    Retries: -1
    AnomalyThresholdMs: 2000
`,
			wantErr: true,
		},
		{
			name: "zero Retries is valid",
			pingBlock: `
Ping:
    Count: 4
    Timeout: 10 Seconds
    Retries: 0
    AnomalyThresholdMs: 2000
`,
			wantErr: false,
		},
		{
			name: "missing AnomalyThresholdMs",
			pingBlock: `
Ping:
    Count: 4
    Timeout: 10 Seconds
    Retries: 2
`,
			wantErr: true,
		},
		{
			name: "zero AnomalyThresholdMs",
			pingBlock: `
Ping:
    Count: 4
    Timeout: 10 Seconds
    Retries: 2
    AnomalyThresholdMs: 0
`,
			wantErr: true,
		},
		{
			name: "primary field names (Count, Timeout, Retries, AnomalyThresholdMs)",
			pingBlock: `
Ping:
    Count: 5
    Timeout: 15 Seconds
    Retries: 3
    AnomalyThresholdMs: 3000
`,
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw := baseValid + tc.pingBlock
			cfg, err := ParseConfig([]byte(raw))
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tc.wantErr && tc.name == "primary field names (Count, Timeout, Retries, AnomalyThresholdMs)" {
				if cfg.Ping.Count != 5 {
					t.Errorf("Count: want 5, got %d", cfg.Ping.Count)
				}
				if cfg.Ping.TimeoutSeconds != 15 {
					t.Errorf("TimeoutSeconds: want 15, got %d", cfg.Ping.TimeoutSeconds)
				}
				if cfg.Ping.Retries != 3 {
					t.Errorf("Retries: want 3, got %d", cfg.Ping.Retries)
				}
				if cfg.Ping.AnomalyThresholdMs != 3000 {
					t.Errorf("AnomalyThresholdMs: want 3000, got %d", cfg.Ping.AnomalyThresholdMs)
				}
			}
		})
	}
}

func TestParseConfig_LogRotationValidation(t *testing.T) {
	base := `
Ping:
    Count: 4
    Timeout: 10 Seconds
    Retries: 2
    AnomalyThresholdMs: 2000
Addresses:
    Local:
        IPv4: 127.0.0.1
`
	tests := []struct {
		name       string
		schedBlock string
		wantErr    bool
		wantSec    int64
	}{
		{
			name: "14 Days",
			schedBlock: `
Schedule:
    Ping: 5 Minutes
    Speedtest: OFF
    Archiving: 7 Days
    LogRotation: 14 Days
`,
			wantErr: false,
			wantSec: 14 * 86400,
		},
		{
			name: "1 Day singular",
			schedBlock: `
Schedule:
    Ping: 5 Minutes
    Speedtest: OFF
    Archiving: 7 Days
    LogRotation: 1 Day
`,
			wantErr: false,
			wantSec: 86400,
		},
		{
			name: "disallowed 1 Month singular",
			schedBlock: `
Schedule:
    Ping: 5 Minutes
    Speedtest: OFF
    Archiving: 7 Days
    LogRotation: 1 Month
`,
			wantErr: true,
		},
		{
			name: "disallowed 3 Months plural",
			schedBlock: `
Schedule:
    Ping: 5 Minutes
    Speedtest: OFF
    Archiving: 7 Days
    LogRotation: 3 Months
`,
			wantErr: true,
		},
		{
			name: "disallowed decimal Days",
			schedBlock: `
Schedule:
    Ping: 5 Minutes
    Speedtest: OFF
    Archiving: 7 Days
    LogRotation: 0.5 Days
`,
			wantErr: true,
		},
		{
			name: "disallowed zero Days",
			schedBlock: `
Schedule:
    Ping: 5 Minutes
    Speedtest: OFF
    Archiving: 7 Days
    LogRotation: 0 Days
`,
			wantErr: true,
		},
		{
			name: "disallowed negative Days",
			schedBlock: `
Schedule:
    Ping: 5 Minutes
    Speedtest: OFF
    Archiving: 7 Days
    LogRotation: -7 Days
`,
			wantErr: true,
		},
		{
			name: "OFF uppercase",
			schedBlock: `
Schedule:
    Ping: 5 Minutes
    Speedtest: OFF
    Archiving: 7 Days
    LogRotation: OFF
`,
			wantErr: false,
			wantSec: 0,
		},
		{
			name: "off lowercase",
			schedBlock: `
Schedule:
    Ping: 5 Minutes
    Speedtest: OFF
    Archiving: 7 Days
    LogRotation: off
`,
			wantErr: false,
			wantSec: 0,
		},
		{
			name: "disallowed Minutes unit",
			schedBlock: `
Schedule:
    Ping: 5 Minutes
    Speedtest: OFF
    Archiving: 7 Days
    LogRotation: 15 Minutes
`,
			wantErr: true,
		},
		{
			name: "disallowed Hours unit",
			schedBlock: `
Schedule:
    Ping: 5 Minutes
    Speedtest: OFF
    Archiving: 7 Days
    LogRotation: 2 Hours
`,
			wantErr: true,
		},
		{
			name: "missing LogRotation field",
			schedBlock: `
Schedule:
    Ping: 5 Minutes
    Speedtest: OFF
    Archiving: 7 Days
`,
			wantErr: true,
		},
		{
			name: "invalid number",
			schedBlock: `
Schedule:
    Ping: 5 Minutes
    Speedtest: OFF
    Archiving: 7 Days
    LogRotation: abc Days
`,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw := tc.schedBlock + base
			cfg, err := ParseConfig([]byte(raw))
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if cfg.Schedule.LogRotationSeconds != tc.wantSec {
					t.Errorf("LogRotationSeconds: want %d, got %d", tc.wantSec, cfg.Schedule.LogRotationSeconds)
				}
			}
		})
	}
}

func TestParseConfig_ArchivingValidation(t *testing.T) {
	base := `
Ping:
    Count: 4
    Timeout: 10 Seconds
    Retries: 2
    AnomalyThresholdMs: 2000
Addresses:
    Local:
        IPv4: 127.0.0.1
`
	tests := []struct {
		name       string
		schedBlock string
		wantErr    bool
		wantSec    int64
	}{
		{
			name: "14 Days",
			schedBlock: `
Schedule:
    Ping: 5 Minutes
    Speedtest: OFF
    Archiving: 14 Days
    LogRotation: 14 Days
`,
			wantErr: false,
			wantSec: 14 * 86400,
		},
		{
			name: "1 Day singular",
			schedBlock: `
Schedule:
    Ping: 5 Minutes
    Speedtest: OFF
    Archiving: 1 Day
    LogRotation: 14 Days
`,
			wantErr: false,
			wantSec: 86400,
		},
		{
			name: "disallowed 1 Month singular",
			schedBlock: `
Schedule:
    Ping: 5 Minutes
    Speedtest: OFF
    Archiving: 1 Month
    LogRotation: 14 Days
`,
			wantErr: true,
		},
		{
			name: "disallowed 3 Months plural",
			schedBlock: `
Schedule:
    Ping: 5 Minutes
    Speedtest: OFF
    Archiving: 3 Months
    LogRotation: 14 Days
`,
			wantErr: true,
		},
		{
			name: "disallowed decimal Days",
			schedBlock: `
Schedule:
    Ping: 5 Minutes
    Speedtest: OFF
    Archiving: 0.5 Days
    LogRotation: 14 Days
`,
			wantErr: true,
		},
		{
			name: "disallowed OFF uppercase",
			schedBlock: `
Schedule:
    Ping: 5 Minutes
    Speedtest: OFF
    Archiving: OFF
    LogRotation: 14 Days
`,
			wantErr: true,
		},
		{
			name: "disallowed off lowercase",
			schedBlock: `
Schedule:
    Ping: 5 Minutes
    Speedtest: OFF
    Archiving: off
    LogRotation: 14 Days
`,
			wantErr: true,
		},
		{
			name: "disallowed Minutes unit",
			schedBlock: `
Schedule:
    Ping: 5 Minutes
    Speedtest: OFF
    Archiving: 15 Minutes
    LogRotation: 14 Days
`,
			wantErr: true,
		},
		{
			name: "disallowed Hours unit",
			schedBlock: `
Schedule:
    Ping: 5 Minutes
    Speedtest: OFF
    Archiving: 2 Hours
    LogRotation: 14 Days
`,
			wantErr: true,
		},
		{
			name: "missing Archiving field",
			schedBlock: `
Schedule:
    Ping: 5 Minutes
    Speedtest: OFF
    LogRotation: 14 Days
`,
			wantErr: true,
		},
		{
			name: "invalid number",
			schedBlock: `
Schedule:
    Ping: 5 Minutes
    Speedtest: OFF
    Archiving: abc Days
    LogRotation: 14 Days
`,
			wantErr: true,
		},
		{
			name: "zero days",
			schedBlock: `
Schedule:
    Ping: 5 Minutes
    Speedtest: OFF
    Archiving: 0 Days
    LogRotation: 14 Days
`,
			wantErr: true,
		},
		{
			name: "negative days",
			schedBlock: `
Schedule:
    Ping: 5 Minutes
    Speedtest: OFF
    Archiving: -5 Days
    LogRotation: 14 Days
`,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw := tc.schedBlock + base
			cfg, err := ParseConfig([]byte(raw))
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if cfg.Schedule.ArchivingSeconds != tc.wantSec {
					t.Errorf("ArchivingSeconds: want %d, got %d", tc.wantSec, cfg.Schedule.ArchivingSeconds)
				}
			}
		})
	}
}

func TestParseConfig_SpeedtestValidation(t *testing.T) {
	// baseValid provides a complete, valid config minus the Speedtest block so each
	// sub-test can inject its own Speedtest section.
	baseValid := `
Schedule:
    Ping: 5 Minutes
    Speedtest: OFF
    Archiving: 7 Days
    LogRotation: 14 Days
Ping:
    Count: 4
    Timeout: 10 Seconds
    Retries: 2
    AnomalyThresholdMs: 2000
Addresses:
    Local:
        IPv4: 127.0.0.1
`

	tests := []struct {
		name           string
		speedtestBlock string
		wantErr        bool
		wantServerID   string
	}{
		{
			name: "AUTO is accepted and normalised to AUTO",
			speedtestBlock: `
Speedtest:
    ServerID: AUTO
`,
			wantErr:      false,
			wantServerID: "AUTO",
		},
		{
			name: "lowercase auto is accepted and normalised to AUTO",
			speedtestBlock: `
Speedtest:
    ServerID: auto
`,
			wantErr:      false,
			wantServerID: "AUTO",
		},
		{
			name: "mixed-case Auto is accepted and normalised to AUTO",
			speedtestBlock: `
Speedtest:
    ServerID: Auto
`,
			wantErr:      false,
			wantServerID: "AUTO",
		},
		{
			name:           "missing Speedtest section defaults to AUTO",
			speedtestBlock: "",
			wantErr:        false,
			wantServerID:   "AUTO",
		},
		{
			name: "valid numeric server ID is accepted",
			speedtestBlock: `
Speedtest:
    ServerID: 12345
`,
			wantErr:      false,
			wantServerID: "12345",
		},
		{
			name: "quoted numeric server ID is accepted",
			speedtestBlock: `
Speedtest:
    ServerID: "54321"
`,
			wantErr:      false,
			wantServerID: "54321",
		},
		{
			name: "non-numeric server ID is rejected",
			speedtestBlock: `
Speedtest:
    ServerID: london-1
`,
			wantErr: true,
		},
		{
			name: "zero server ID is rejected",
			speedtestBlock: `
Speedtest:
    ServerID: 0
`,
			wantErr: true,
		},
		{
			name: "negative server ID is rejected",
			speedtestBlock: `
Speedtest:
    ServerID: -1
`,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw := baseValid + tc.speedtestBlock
			cfg, err := ParseConfig([]byte(raw))
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tc.wantErr && cfg.Speedtest.ServerID != tc.wantServerID {
				t.Errorf("ServerID: want %q, got %q", tc.wantServerID, cfg.Speedtest.ServerID)
			}
			if !tc.wantErr && tc.wantServerID != "AUTO" {
				// Numeric IDs must not activate auto selection.
				if cfg.Speedtest.IsAuto() {
					t.Errorf("IsAuto() should be false for numeric ServerID %q", tc.wantServerID)
				}
			}
		})
	}
}

func TestParseConfig_SchedulePingValidation(t *testing.T) {
	base := `
Ping:
    Count: 4
    Timeout: 10 Seconds
    Retries: 2
    AnomalyThresholdMs: 2000
Addresses:
    Local:
        IPv4: 127.0.0.1
`
	tests := []struct {
		name       string
		schedBlock string
		wantErr    bool
		wantSec    int64
	}{
		{
			name: "15 Minutes",
			schedBlock: `
Schedule:
    Ping: 15 Minutes
    Speedtest: OFF
    Archiving: 14 Days
    LogRotation: 14 Days
`,
			wantErr: false,
			wantSec: 15 * 60,
		},
		{
			name: "1 Minute singular",
			schedBlock: `
Schedule:
    Ping: 1 Minute
    Speedtest: OFF
    Archiving: 14 Days
    LogRotation: 14 Days
`,
			wantErr: false,
			wantSec: 60,
		},
		{
			name: "2 Hours",
			schedBlock: `
Schedule:
    Ping: 2 Hours
    Speedtest: OFF
    Archiving: 14 Days
    LogRotation: 14 Days
`,
			wantErr: false,
			wantSec: 2 * 3600,
		},
		{
			name: "1 Hour singular",
			schedBlock: `
Schedule:
    Ping: 1 Hour
    Speedtest: OFF
    Archiving: 14 Days
    LogRotation: 14 Days
`,
			wantErr: false,
			wantSec: 3600,
		},
		{
			name: "disallowed Seconds unit",
			schedBlock: `
Schedule:
    Ping: 30 Seconds
    Speedtest: OFF
    Archiving: 14 Days
    LogRotation: 14 Days
`,
			wantErr: true,
		},
		{
			name: "disallowed Days unit",
			schedBlock: `
Schedule:
    Ping: 1 Days
    Speedtest: OFF
    Archiving: 14 Days
    LogRotation: 14 Days
`,
			wantErr: true,
		},
		{
			name: "disallowed OFF",
			schedBlock: `
Schedule:
    Ping: OFF
    Speedtest: OFF
    Archiving: 14 Days
    LogRotation: 14 Days
`,
			wantErr: true,
		},
		{
			name: "disallowed decimal Minutes",
			schedBlock: `
Schedule:
    Ping: 0.5 Minutes
    Speedtest: OFF
    Archiving: 14 Days
    LogRotation: 14 Days
`,
			wantErr: true,
		},
		{
			name: "disallowed decimal Hours",
			schedBlock: `
Schedule:
    Ping: 0.5 Hours
    Speedtest: OFF
    Archiving: 14 Days
    LogRotation: 14 Days
`,
			wantErr: true,
		},
		{
			name: "disallowed zero Minutes",
			schedBlock: `
Schedule:
    Ping: 0 Minutes
    Speedtest: OFF
    Archiving: 14 Days
    LogRotation: 14 Days
`,
			wantErr: true,
		},
		{
			name: "disallowed negative Minutes",
			schedBlock: `
Schedule:
    Ping: -5 Minutes
    Speedtest: OFF
    Archiving: 14 Days
    LogRotation: 14 Days
`,
			wantErr: true,
		},
		{
			name: "missing Ping field",
			schedBlock: `
Schedule:
    Speedtest: OFF
    Archiving: 14 Days
    LogRotation: 14 Days
`,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw := tc.schedBlock + base
			cfg, err := ParseConfig([]byte(raw))
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if cfg.Schedule.PingSeconds != tc.wantSec {
					t.Errorf("PingSeconds: want %d, got %d", tc.wantSec, cfg.Schedule.PingSeconds)
				}
			}
		})
	}
}

func TestParseConfig_ScheduleSpeedtestValidation(t *testing.T) {
	base := `
Ping:
    Count: 4
    Timeout: 10 Seconds
    Retries: 2
    AnomalyThresholdMs: 2000
Addresses:
    Local:
        IPv4: 127.0.0.1
`
	tests := []struct {
		name       string
		schedBlock string
		wantErr    bool
		wantSec    int64
	}{
		{
			name: "OFF uppercase",
			schedBlock: `
Schedule:
    Ping: 15 Minutes
    Speedtest: OFF
    Archiving: 14 Days
    LogRotation: 14 Days
`,
			wantErr: false,
			wantSec: 0,
		},
		{
			name: "off lowercase",
			schedBlock: `
Schedule:
    Ping: 15 Minutes
    Speedtest: off
    Archiving: 14 Days
    LogRotation: 14 Days
`,
			wantErr: false,
			wantSec: 0,
		},
		{
			name: "1 Hours",
			schedBlock: `
Schedule:
    Ping: 15 Minutes
    Speedtest: 1 Hours
    Archiving: 14 Days
    LogRotation: 14 Days
`,
			wantErr: false,
			wantSec: 3600,
		},
		{
			name: "1 Hour singular",
			schedBlock: `
Schedule:
    Ping: 15 Minutes
    Speedtest: 1 Hour
    Archiving: 14 Days
    LogRotation: 14 Days
`,
			wantErr: false,
			wantSec: 3600,
		},
		{
			name: "3 Hours",
			schedBlock: `
Schedule:
    Ping: 15 Minutes
    Speedtest: 3 Hours
    Archiving: 14 Days
    LogRotation: 14 Days
`,
			wantErr: false,
			wantSec: 3 * 3600,
		},
		{
			name: "disallowed Minutes unit (e.g. 5 Minutes)",
			schedBlock: `
Schedule:
    Ping: 15 Minutes
    Speedtest: 5 Minutes
    Archiving: 14 Days
    LogRotation: 14 Days
`,
			wantErr: true,
		},
		{
			name: "disallowed decimal Hours (e.g. 0.5 Hours)",
			schedBlock: `
Schedule:
    Ping: 15 Minutes
    Speedtest: 0.5 Hours
    Archiving: 14 Days
    LogRotation: 14 Days
`,
			wantErr: true,
		},
		{
			name: "disallowed Seconds unit",
			schedBlock: `
Schedule:
    Ping: 15 Minutes
    Speedtest: 30 Seconds
    Archiving: 14 Days
    LogRotation: 14 Days
`,
			wantErr: true,
		},
		{
			name: "disallowed Days unit",
			schedBlock: `
Schedule:
    Ping: 15 Minutes
    Speedtest: 1 Days
    Archiving: 14 Days
    LogRotation: 14 Days
`,
			wantErr: true,
		},
		{
			name: "disallowed zero Hours",
			schedBlock: `
Schedule:
    Ping: 15 Minutes
    Speedtest: 0 Hours
    Archiving: 14 Days
    LogRotation: 14 Days
`,
			wantErr: true,
		},
		{
			name: "disallowed negative Hours",
			schedBlock: `
Schedule:
    Ping: 15 Minutes
    Speedtest: -1 Hours
    Archiving: 14 Days
    LogRotation: 14 Days
`,
			wantErr: true,
		},
		{
			name: "missing Speedtest field",
			schedBlock: `
Schedule:
    Ping: 15 Minutes
    Archiving: 14 Days
    LogRotation: 14 Days
`,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw := tc.schedBlock + base
			cfg, err := ParseConfig([]byte(raw))
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if cfg.Schedule.SpeedtestSeconds != tc.wantSec {
					t.Errorf("SpeedtestSeconds: want %d, got %d", tc.wantSec, cfg.Schedule.SpeedtestSeconds)
				}
			}
		})
	}
}

func TestLoadConfig_RootConfigFile(t *testing.T) {
	cfg, err := LoadConfig("../../config.yml")
	if err != nil {
		t.Fatalf("failed to load root config.yml: %v", err)
	}
	if cfg.Schedule.PingSeconds != 15*60 {
		t.Errorf("PingSeconds: want 900, got %d", cfg.Schedule.PingSeconds)
	}
	if cfg.Schedule.SpeedtestSeconds != 0 {
		t.Errorf("SpeedtestSeconds: want 0 (OFF), got %d", cfg.Schedule.SpeedtestSeconds)
	}
	if cfg.Schedule.ArchivingSeconds != 14*86400 {
		t.Errorf("ArchivingSeconds: want 1209600, got %d", cfg.Schedule.ArchivingSeconds)
	}
	if cfg.Schedule.LogRotationSeconds != 7*86400 {
		t.Errorf("LogRotationSeconds: want 604800, got %d", cfg.Schedule.LogRotationSeconds)
	}
	if cfg.Ping.Count != 4 {
		t.Errorf("Ping.Count: want 4, got %d", cfg.Ping.Count)
	}
	if cfg.Ping.TimeoutSeconds != 10 {
		t.Errorf("Ping.TimeoutSeconds: want 10, got %d", cfg.Ping.TimeoutSeconds)
	}
	if cfg.Ping.Retries != 2 {
		t.Errorf("Ping.Retries: want 2, got %d", cfg.Ping.Retries)
	}
	if cfg.Ping.AnomalyThresholdMs != 2000 {
		t.Errorf("Ping.AnomalyThresholdMs: want 2000, got %d", cfg.Ping.AnomalyThresholdMs)
	}
	if cfg.Speedtest.ServerID != "AUTO" {
		t.Errorf("Speedtest.ServerID: want AUTO, got %q", cfg.Speedtest.ServerID)
	}
}
