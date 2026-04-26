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
