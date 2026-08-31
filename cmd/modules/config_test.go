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
}

func TestParseConfig_AnomalyThreshold_Missing(t *testing.T) {
	// PingAnomalyThresholdMs is now required — omitting it must return an error.
	yaml := `
Schedule:
    Ping: 5 Minutes
    Speedtest: OFF
    Archiving: 7 Days
Addresses:
    Local:
        IPv4: 127.0.0.1
`
	_, err := ParseConfig([]byte(yaml))
	if err == nil {
		t.Fatal("expected error when PingAnomalyThresholdMs is missing, got nil")
	}
}

func TestParseConfig_AnomalyThreshold_Explicit(t *testing.T) {
	// An explicit value in config must be preserved.
	yaml := `
Schedule:
    Ping: 5 Minutes
    Speedtest: OFF
    Archiving: 7 Days
    PingAnomalyThresholdMs: 5000
Addresses:
    Local:
        IPv4: 127.0.0.1
`
	cfg, err := ParseConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Schedule.PingAnomalyThresholdMs != 5000 {
		t.Errorf("anomaly threshold: want 5000, got %d", cfg.Schedule.PingAnomalyThresholdMs)
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
