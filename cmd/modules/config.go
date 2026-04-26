package modules

import (
	"fmt"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ─── raw YAML shapes ───────────────────────────────────────────────────────

type rawConfig struct {
	Schedule  rawSchedule             `yaml:"Schedule"`
	Addresses map[string]rawAddress   `yaml:"Addresses"`
}

type rawSchedule struct {
	Ping      string `yaml:"Ping"`
	Speedtest string `yaml:"Speedtest"`
	Archiving string `yaml:"Archiving"`
}

type rawAddress struct {
	IPv4     string `yaml:"IPv4"`
	IPv6     string `yaml:"IPv6"`
	Domain   string `yaml:"Domain"`
	Protocol string `yaml:"Protocol"`
}

// ─── parsed / validated shapes ─────────────────────────────────────────────

// Config is the validated, ready-to-use configuration.
type Config struct {
	Schedule  Schedule
	Addresses []Address
}

// Schedule holds interval durations in seconds.
type Schedule struct {
	PingSeconds      int64
	SpeedtestSeconds int64
	ArchivingSeconds int64
}

// Address represents a single monitoring target.
type Address struct {
	Name     string
	IPv4     *netip.Addr
	IPv6     *netip.Addr
	Domain   string
	Protocol string // "IPv4", "IPv6", "Both", or "" (unused for raw IPs)
}

// ─── public API ────────────────────────────────────────────────────────────

// LoadConfig reads and validates the YAML file at path.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file %q: %w", path, err)
	}
	return ParseConfig(data)
}

// ParseConfig validates raw YAML bytes and returns a *Config. Exported so
// tests can call it without needing a real file.
func ParseConfig(data []byte) (*Config, error) {
	var raw rawConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("yaml parse: %w", err)
	}

	sched, err := parseSchedule(raw.Schedule)
	if err != nil {
		return nil, err
	}

	addrs, err := parseAddresses(raw.Addresses)
	if err != nil {
		return nil, err
	}

	return &Config{Schedule: sched, Addresses: addrs}, nil
}

// ─── internal helpers ──────────────────────────────────────────────────────

func parseSchedule(r rawSchedule) (Schedule, error) {
	ping, err := parseInterval(r.Ping, "Schedule.Ping")
	if err != nil {
		return Schedule{}, err
	}
	speedtest, err := parseInterval(r.Speedtest, "Schedule.Speedtest")
	if err != nil {
		return Schedule{}, err
	}
	archiving, err := parseInterval(r.Archiving, "Schedule.Archiving")
	if err != nil {
		return Schedule{}, err
	}
	return Schedule{
		PingSeconds:      ping,
		SpeedtestSeconds: speedtest,
		ArchivingSeconds: archiving,
	}, nil
}

// parseInterval converts strings like "15 Minutes", "3 Hours", "14 Days"
// into a whole number of seconds. It returns 0 if the string is "OFF".
func parseInterval(s, field string) (int64, error) {
	s = strings.TrimSpace(s)
	if strings.ToUpper(s) == "OFF" {
		return 0, nil
	}
	parts := strings.Fields(s)
	if len(parts) != 2 {
		return 0, fmt.Errorf("%s: invalid interval %q (expected \"<N> Minutes|Hours|Days\")", field, s)
	}
	n, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s: invalid number %q", field, parts[0])
	}
	switch strings.ToLower(parts[1]) {
	case "minute", "minutes":
		return n * 60, nil
	case "hour", "hours":
		return n * 3600, nil
	case "day", "days":
		return n * 86400, nil
	default:
		return 0, fmt.Errorf("%s: unknown unit %q (use Minutes, Hours, or Days)", field, parts[1])
	}
}

func parseAddresses(raw map[string]rawAddress) ([]Address, error) {
	addrs := make([]Address, 0, len(raw))
	for name, r := range raw {
		a, err := parseAddress(name, r)
		if err != nil {
			return nil, err
		}
		addrs = append(addrs, a)
	}
	return addrs, nil
}

func parseAddress(name string, r rawAddress) (Address, error) {
	a := Address{Name: name}

	hasIP := false
	if r.IPv4 != "" {
		addr, err := netip.ParseAddr(strings.TrimSpace(r.IPv4))
		if err != nil || !addr.Is4() {
			if err == nil {
				err = fmt.Errorf("not an IPv4 address")
			}
			return Address{}, fmt.Errorf("Addresses.%s: invalid IPv4 address %q: %w", name, r.IPv4, err)
		}
		a.IPv4 = &addr
		hasIP = true
	}
	if r.IPv6 != "" {
		addr, err := netip.ParseAddr(strings.TrimSpace(r.IPv6))
		if err != nil || !addr.Is6() {
			if err == nil {
				err = fmt.Errorf("not an IPv6 address")
			}
			return Address{}, fmt.Errorf("Addresses.%s: invalid IPv6 address %q: %w", name, r.IPv6, err)
		}
		a.IPv6 = &addr
		hasIP = true
	}

	if r.Domain != "" {
		if err := validateDomain(r.Domain); err != nil {
			return Address{}, fmt.Errorf("Addresses.%s: %w", name, err)
		}
		a.Domain = strings.TrimSpace(r.Domain)

		// Protocol is only relevant for domain targets.
		proto := strings.TrimSpace(r.Protocol)
		if proto == "" {
			proto = "IPv4" // sensible default
		}
		switch proto {
		case "IPv4", "IPv6", "Both":
			a.Protocol = proto
		default:
			return Address{}, fmt.Errorf("Addresses.%s: invalid Protocol %q (use IPv4, IPv6, or Both)", name, proto)
		}
	} else if !hasIP {
		return Address{}, fmt.Errorf("Addresses.%s: must specify either IPv4, IPv6, or Domain", name)
	}

	return a, nil
}

// validateDomain performs a lightweight structural check on a domain name.
func validateDomain(domain string) error {
	d := strings.TrimSpace(domain)
	if d == "" {
		return fmt.Errorf("domain must not be empty")
	}
	// net.LookupHost is not called here so tests stay offline; instead we do a
	// simple label-based structural check.
	if strings.Contains(d, " ") {
		return fmt.Errorf("invalid domain %q: contains spaces", d)
	}
	for _, label := range strings.Split(d, ".") {
		if label == "" {
			return fmt.Errorf("invalid domain %q: empty label", d)
		}
	}
	// Quick sanity: must have at least one dot.
	if !strings.Contains(d, ".") {
		return fmt.Errorf("invalid domain %q: no dots", d)
	}
	// Use net.LookupCNAME offline parser indirectly: just try parsing as host.
	if net.ParseIP(d) != nil {
		return fmt.Errorf("invalid domain %q: looks like an IP address", d)
	}
	return nil
}
