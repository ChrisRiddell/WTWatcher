package modules

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ─── raw YAML shapes ───────────────────────────────────────────────────────

// rawConfig represents the unvalidated, raw structure decoded directly from config.yml.
type rawConfig struct {
	Schedule rawSchedule `yaml:"Schedule"`
	Ping     rawPing     `yaml:"Ping"`
	// Addresses is intentionally unmarshaled as a yaml.Node AST instead of a Go map.
	// In Go, map iteration order is randomized by design; using yaml.Node allows us
	// to iterate over the YAML MappingNode entries in the exact order the user authored them.
	Addresses yaml.Node `yaml:"Addresses"`
}

// rawSchedule holds unparsed interval strings.
type rawSchedule struct {
	Ping        string `yaml:"Ping"`
	Speedtest   string `yaml:"Speedtest"`
	Archiving   string `yaml:"Archiving"`
	LogRotation string `yaml:"LogRotation"`
}

// rawPing holds unparsed ping probe parameters and the anomaly threshold.
type rawPing struct {
	Count                  int    `yaml:"Count"`
	PingCount              int    `yaml:"PingCount"`
	Timeout                string `yaml:"Timeout"`
	PingTimeout            string `yaml:"PingTimeout"`
	Retries                *int   `yaml:"Retries"`
	PingRetries            *int   `yaml:"PingRetries"`
	AnomalyThresholdMs     int64  `yaml:"AnomalyThresholdMs"`
	PingAnomalyThresholdMs int64  `yaml:"PingAnomalyThresholdMs"`
}

// rawAddress holds string values for an individual target under the Addresses section.
type rawAddress struct {
	IPv4     string `yaml:"IPv4"`
	IPv6     string `yaml:"IPv6"`
	Domain   string `yaml:"Domain"`
	Protocol string `yaml:"Protocol"`
}

// ─── parsed / validated shapes ─────────────────────────────────────────────

// Config is the validated, fully parsed configuration ready for runtime use.
type Config struct {
	Schedule  Schedule
	Ping      Ping
	Addresses []Address
}

// Schedule holds validated interval durations converted into seconds for easy timer arithmetic.
type Schedule struct {
	PingSeconds        int64
	SpeedtestSeconds   int64
	ArchivingSeconds   int64
	LogRotationSeconds int64
}

// Ping holds validated probe parameters and anomaly filtering settings for ICMP ping.
type Ping struct {
	Count          int
	TimeoutSeconds int64
	Retries        int
	// PingAnomalyThresholdMs is the upper bound for realistic per-packet RTT in milliseconds.
	// Any sample above Q3 + 1.5×IQR that also exceeds this ceiling is treated as an OS/scheduler
	// spike (e.g. CPU contention, sleep/wake, GC stall) and marked as an anomaly. Default is 2000ms.
	PingAnomalyThresholdMs int64
}

// Address represents a validated monitoring target with typed IP addresses or domain name.
type Address struct {
	Name     string
	IPv4     *netip.Addr
	IPv6     *netip.Addr
	Domain   string
	Protocol string // Target protocol for domains: "IPv4", "IPv6", or "Both"
}

// ─── public API ────────────────────────────────────────────────────────────

// LoadConfig reads the YAML configuration file from disk, parses, and validates its contents.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file %q: %w", path, err)
	}
	return ParseConfig(data)
}

// ParseConfig validates raw YAML bytes and returns a typed *Config.
// This is exported so unit tests can test configuration parsing in-memory without filesystem access.
func ParseConfig(data []byte) (*Config, error) {
	var raw rawConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("yaml parse: %w", err)
	}

	sched, err := parseSchedule(raw.Schedule)
	if err != nil {
		return nil, err
	}

	ping, err := parsePing(raw.Ping)
	if err != nil {
		return nil, err
	}

	addrs, err := parseAddresses(&raw.Addresses)
	if err != nil {
		return nil, err
	}

	return &Config{Schedule: sched, Ping: ping, Addresses: addrs}, nil
}

// ─── internal helpers ──────────────────────────────────────────────────────

// parseSchedule validates each interval string and converts durations to whole seconds.
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
	logRotation, err := parseLogRotationInterval(r.LogRotation, "Schedule.LogRotation")
	if err != nil {
		return Schedule{}, err
	}

	return Schedule{
		PingSeconds:        ping,
		SpeedtestSeconds:   speedtest,
		ArchivingSeconds:   archiving,
		LogRotationSeconds: logRotation,
	}, nil
}

// parsePing validates probe parameters and anomaly filtering thresholds.
func parsePing(r rawPing) (Ping, error) {
	count := r.PingCount
	if count == 0 {
		count = r.Count
	}
	if count <= 0 {
		return Ping{}, fmt.Errorf("Ping.PingCount: must be a positive number of packets (e.g. 4)")
	}

	timeoutStr := r.PingTimeout
	if timeoutStr == "" {
		timeoutStr = r.Timeout
	}
	if timeoutStr == "" {
		return Ping{}, fmt.Errorf("Ping.PingTimeout: duration string is required (e.g. \"10 Seconds\")")
	}
	timeoutSec, err := parseInterval(timeoutStr, "Ping.PingTimeout")
	if err != nil {
		return Ping{}, err
	}
	if timeoutSec <= 0 {
		return Ping{}, fmt.Errorf("Ping.PingTimeout: must be a positive duration (e.g. \"10 Seconds\")")
	}

	retriesPtr := r.PingRetries
	if retriesPtr == nil {
		retriesPtr = r.Retries
	}
	if retriesPtr == nil {
		return Ping{}, fmt.Errorf("Ping.PingRetries: must be specified (e.g. 2, or 0 to disable retries)")
	}
	if *retriesPtr < 0 {
		return Ping{}, fmt.Errorf("Ping.PingRetries: must be a non-negative integer (e.g. 2)")
	}

	anomalyThresholdMs := r.PingAnomalyThresholdMs
	if anomalyThresholdMs == 0 {
		anomalyThresholdMs = r.AnomalyThresholdMs
	}
	if anomalyThresholdMs <= 0 {
		return Ping{}, fmt.Errorf("Ping.PingAnomalyThresholdMs: must be a positive number of milliseconds (e.g. 2000)")
	}

	return Ping{
		Count:                  count,
		TimeoutSeconds:         timeoutSec,
		Retries:                *retriesPtr,
		PingAnomalyThresholdMs: anomalyThresholdMs,
	}, nil
}

// parseInterval parses human-friendly duration strings like "10 Seconds", "15 Minutes", "3 Hours", "14 Days"
// into seconds. If the value is "OFF" (case-insensitive), it returns 0 (disabling the task).
func parseInterval(s, field string) (int64, error) {
	s = strings.TrimSpace(s)
	if strings.EqualFold(s, "off") {
		return 0, nil
	}
	parts := strings.Fields(s)
	if len(parts) != 2 {
		return 0, fmt.Errorf("%s: invalid interval %q (expected \"<N> Seconds|Minutes|Hours|Days\")", field, s)
	}
	n, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s: invalid number %q", field, parts[0])
	}
	switch strings.ToLower(parts[1]) {
	case "second", "seconds":
		return n, nil
	case "minute", "minutes":
		return n * 60, nil
	case "hour", "hours":
		return n * 3600, nil
	case "day", "days":
		return n * 86400, nil
	default:
		return 0, fmt.Errorf("%s: unknown unit %q (use Seconds, Minutes, Hours, or Days)", field, parts[1])
	}
}

// parseLogRotationInterval parses duration strings specifically for Schedule.LogRotation.
// It supports "OFF" (returns 0), "Days", and "Months" (calculated as 30 days per month).
func parseLogRotationInterval(s, field string) (int64, error) {
	s = strings.TrimSpace(s)
	if strings.EqualFold(s, "off") {
		return 0, nil
	}
	parts := strings.Fields(s)
	if len(parts) != 2 {
		return 0, fmt.Errorf("%s: invalid interval %q (expected \"<N> Days|Months\" or \"OFF\")", field, s)
	}
	n, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s: invalid number %q", field, parts[0])
	}
	switch strings.ToLower(parts[1]) {
	case "day", "days":
		return n * 86400, nil
	case "month", "months":
		return n * 30 * 86400, nil
	default:
		return 0, fmt.Errorf("%s: unknown unit %q (use Days, Months, or OFF)", field, parts[1])
	}
}

// parseAddresses walks the yaml.Node AST of the Addresses mapping in document order,
// ensuring the ordered slice of Address objects matches the exact sequence in config.yml.
func parseAddresses(node *yaml.Node) ([]Address, error) {
	// Resolve YAML anchor or alias nodes if present.
	n := node
	if n.Kind == yaml.AliasNode {
		n = n.Alias
	}

	// An empty or null Addresses block is permitted (e.g. empty target list).
	if n.Kind == 0 || n.Tag == "!!null" || len(n.Content) == 0 {
		return nil, nil
	}

	if n.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("Addresses: expected a YAML mapping, got kind %v", n.Kind)
	}

	// In yaml.v3, MappingNode.Content contains interleaved key and value nodes: [key0, val0, key1, val1, ...]
	addrs := make([]Address, 0, len(n.Content)/2)
	for i := 0; i+1 < len(n.Content); i += 2 {
		keyNode := n.Content[i]
		valNode := n.Content[i+1]

		name := keyNode.Value

		var raw rawAddress
		if err := valNode.Decode(&raw); err != nil {
			return nil, fmt.Errorf("Addresses.%s: %w", name, err)
		}

		a, err := parseAddress(name, raw)
		if err != nil {
			return nil, err
		}
		addrs = append(addrs, a)
	}
	return addrs, nil
}

// parseAddress validates individual target fields: parsing and verifying IP address formats,
// validating domains, and verifying protocol choices ("IPv4", "IPv6", "Both").
func parseAddress(name string, r rawAddress) (Address, error) {
	a := Address{Name: name}

	hasIP := false
	if r.IPv4 != "" {
		addr, err := netip.ParseAddr(strings.TrimSpace(r.IPv4))
		if err != nil || !addr.Is4() {
			if err == nil {
				err = errors.New("not an IPv4 address")
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
				err = errors.New("not an IPv6 address")
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

		// Protocol option determines whether to ping IPv4, IPv6, or both DNS records.
		proto := strings.TrimSpace(r.Protocol)
		if proto == "" {
			proto = "IPv4" // Default to IPv4 if unspecified.
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

// validateDomain performs an offline syntactic validation of domain strings to avoid network lookups during config load.
func validateDomain(domain string) error {
	d := strings.TrimSpace(domain)
	if d == "" {
		return errors.New("domain must not be empty")
	}
	// Check for invalid whitespace within the domain name.
	if strings.Contains(d, " ") {
		return fmt.Errorf("invalid domain %q: contains spaces", d)
	}
	labels := strings.Split(d, ".")
	if len(labels) < 2 {
		return fmt.Errorf("invalid domain %q: no dots", d)
	}
	for _, label := range labels {
		if label == "" {
			return fmt.Errorf("invalid domain %q: empty label", d)
		}
	}
	// Ensure an IP address wasn't mistakenly entered in the Domain field.
	if _, err := netip.ParseAddr(d); err == nil {
		return fmt.Errorf("invalid domain %q: looks like an IP address", d)
	}
	return nil
}
