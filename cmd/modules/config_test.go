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
    PingCount: 4
    PingTimeout: 10 Seconds
    PingRetries: 2
    PingAnomalyThresholdMs: 2000

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
	if cfg.Ping.PingAnomalyThresholdMs != 2000 {
		t.Errorf("ping anomaly threshold: want 2000, got %d", cfg.Ping.PingAnomalyThresholdMs)
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
    PingCount: 4
    PingTimeout: 10 Seconds
    PingRetries: 2
    PingAnomalyThresholdMs: 2000
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
    PingCount: 4
    PingTimeout: 10 Seconds
    PingRetries: 2
    PingAnomalyThresholdMs: 2000
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
    PingCount: 4
    PingTimeout: 10 Seconds
    PingRetries: 2
    PingAnomalyThresholdMs: 2000
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
    PingCount: 4
    PingTimeout: 10 Seconds
    PingRetries: 2
    PingAnomalyThresholdMs: 2000
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
    PingCount: 4
    PingTimeout: 10 Seconds
    PingRetries: 2
    PingAnomalyThresholdMs: 2000
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
    PingCount: 4
    PingTimeout: 10 Seconds
    PingRetries: 2
    PingAnomalyThresholdMs: 2000
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
    PingCount: 4
    PingTimeout: 10 Seconds
    PingRetries: 2
    PingAnomalyThresholdMs: 2000
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
    PingCount: 4
    PingTimeout: 10 Seconds
    PingRetries: 2
    PingAnomalyThresholdMs: 2000

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
			name: "missing PingCount",
			pingBlock: `
Ping:
    PingTimeout: 10 Seconds
    PingRetries: 2
    PingAnomalyThresholdMs: 2000
`,
			wantErr: true,
		},
		{
			name: "zero PingCount",
			pingBlock: `
Ping:
    PingCount: 0
    PingTimeout: 10 Seconds
    PingRetries: 2
    PingAnomalyThresholdMs: 2000
`,
			wantErr: true,
		},
		{
			name: "negative PingCount",
			pingBlock: `
Ping:
    PingCount: -1
    PingTimeout: 10 Seconds
    PingRetries: 2
    PingAnomalyThresholdMs: 2000
`,
			wantErr: true,
		},
		{
			name: "missing PingTimeout",
			pingBlock: `
Ping:
    PingCount: 4
    PingRetries: 2
    PingAnomalyThresholdMs: 2000
`,
			wantErr: true,
		},
		{
			name: "invalid PingTimeout unit",
			pingBlock: `
Ping:
    PingCount: 4
    PingTimeout: 10 Years
    PingRetries: 2
    PingAnomalyThresholdMs: 2000
`,
			wantErr: true,
		},
		{
			name: "missing PingRetries",
			pingBlock: `
Ping:
    PingCount: 4
    PingTimeout: 10 Seconds
    PingAnomalyThresholdMs: 2000
`,
			wantErr: true,
		},
		{
			name: "negative PingRetries",
			pingBlock: `
Ping:
    PingCount: 4
    PingTimeout: 10 Seconds
    PingRetries: -1
    PingAnomalyThresholdMs: 2000
`,
			wantErr: true,
		},
		{
			name: "zero PingRetries is valid",
			pingBlock: `
Ping:
    PingCount: 4
    PingTimeout: 10 Seconds
    PingRetries: 0
    PingAnomalyThresholdMs: 2000
`,
			wantErr: false,
		},
		{
			name: "missing PingAnomalyThresholdMs",
			pingBlock: `
Ping:
    PingCount: 4
    PingTimeout: 10 Seconds
    PingRetries: 2
`,
			wantErr: true,
		},
		{
			name: "zero PingAnomalyThresholdMs",
			pingBlock: `
Ping:
    PingCount: 4
    PingTimeout: 10 Seconds
    PingRetries: 2
    PingAnomalyThresholdMs: 0
`,
			wantErr: true,
		},
		{
			name: "alias field names (Count, Timeout, Retries, AnomalyThresholdMs)",
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
			if !tc.wantErr && tc.name == "alias field names (Count, Timeout, Retries, AnomalyThresholdMs)" {
				if cfg.Ping.Count != 5 {
					t.Errorf("Count: want 5, got %d", cfg.Ping.Count)
				}
				if cfg.Ping.TimeoutSeconds != 15 {
					t.Errorf("TimeoutSeconds: want 15, got %d", cfg.Ping.TimeoutSeconds)
				}
				if cfg.Ping.Retries != 3 {
					t.Errorf("Retries: want 3, got %d", cfg.Ping.Retries)
				}
				if cfg.Ping.PingAnomalyThresholdMs != 3000 {
					t.Errorf("PingAnomalyThresholdMs: want 3000, got %d", cfg.Ping.PingAnomalyThresholdMs)
				}
			}
		})
	}
}

func TestParseConfig_LogRotationValidation(t *testing.T) {
	base := `
Ping:
    PingCount: 4
    PingTimeout: 10 Seconds
    PingRetries: 2
    PingAnomalyThresholdMs: 2000
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
			name: "1 Month singular",
			schedBlock: `
Schedule:
    Ping: 5 Minutes
    Speedtest: OFF
    Archiving: 7 Days
    LogRotation: 1 Month
`,
			wantErr: false,
			wantSec: 30 * 86400,
		},
		{
			name: "3 Months plural",
			schedBlock: `
Schedule:
    Ping: 5 Minutes
    Speedtest: OFF
    Archiving: 7 Days
    LogRotation: 3 Months
`,
			wantErr: false,
			wantSec: 90 * 86400,
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
