package modules

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	probing "github.com/prometheus-community/pro-bing"
)

const (
	pingCount   = 4
	pingTimeout = 10 * time.Second
	pingRetries = 2
)

// RunPing pings all configured addresses and writes results to fm.
// ts is the UTC timestamp used as the JSON key.
func RunPing(cfg *Config, fm *FileManager, logger *Logger, ts time.Time) {
	fmt.Printf("[ping] starting ping run at %s\n", ts.Format("15:04:05Z"))
	for _, addr := range cfg.Addresses {
		targets, err := resolveTargets(addr)
		if err != nil {
			logger.Warn("ping: could not resolve target",
				"name", addr.Name, "error", err)
			fmt.Printf("[ping] %-20s WARN: %v\n", addr.Name, err)
			continue
		}
		for _, t := range targets {
			entry, err := pingWithRetry(t.host, t.proto, pingRetries)
			if err != nil {
				logger.Error("ping failed",
					"name", addr.Name, "host", t.host, "proto", t.proto, "error", err)
				fmt.Printf("[ping] %-20s %-5s FAILED: %v\n", addr.Name, t.proto, err)
				continue
			}
			if err := fm.AddLatency(ts, addr.Name, entry); err != nil {
				logger.Error("save latency failed",
					"name", addr.Name, "error", err)
			} else {
				logger.Info("ping ok",
					"name", addr.Name, "host", t.host,
					"avg_ms", entry.Average, "proto", entry.Protocol)
				fmt.Printf("[ping] %-20s %-5s avg=%.2f ms  loss=%.0f%%\n",
					addr.Name, entry.Protocol, entry.Average, entry.PacketLoss)
			}
		}
	}
	fmt.Println("[ping] run complete")
}

// ─── internal ──────────────────────────────────────────────────────────────

type pingTarget struct {
	host  string
	proto string // "IPv4" or "IPv6"
}

// resolveTargets expands an Address into one or more pingTarget values.
// For domain addresses with Protocol=Both we resolve both A and AAAA records.
func resolveTargets(a Address) ([]pingTarget, error) {
	if a.Domain == "" {
		var targets []pingTarget
		if a.IPv6 != nil {
			targets = append(targets, pingTarget{host: a.IPv6.String(), proto: "IPv6"})
		}
		if a.IPv4 != nil {
			targets = append(targets, pingTarget{host: a.IPv4.String(), proto: "IPv4"})
		}
		return targets, nil
	}

	// Domain target
	switch a.Protocol {
	case "IPv4":
		return []pingTarget{{host: a.Domain, proto: "IPv4"}}, nil
	case "IPv6":
		return []pingTarget{{host: a.Domain, proto: "IPv6"}}, nil
	case "Both":
		return resolveBoth(a.Domain)
	default:
		return []pingTarget{{host: a.Domain, proto: "IPv4"}}, nil
	}
}

func resolveBoth(domain string) ([]pingTarget, error) {
	ips, err := net.LookupHost(domain)
	if err != nil {
		return nil, fmt.Errorf("DNS lookup %q: %w", domain, err)
	}
	seen := map[string]bool{}
	var targets []pingTarget
	for _, ip := range ips {
		addr, err := net.ResolveIPAddr("ip", ip)
		if err != nil {
			continue
		}
		parsed := net.ParseIP(addr.String())
		if parsed == nil {
			continue
		}
		proto := "IPv4"
		if parsed.To4() == nil {
			proto = "IPv6"
		}
		if !seen[proto] {
			seen[proto] = true
			targets = append(targets, pingTarget{host: ip, proto: proto})
		}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("no usable IPs for %q", domain)
	}
	return targets, nil
}

func pingWithRetry(host, proto string, retries int) (LatencyEntry, error) {
	var lastErr error
	for i := 0; i <= retries; i++ {
		entry, err := doPing(host, proto)
		if err == nil {
			return entry, nil
		}
		lastErr = err
	}
	return LatencyEntry{}, lastErr
}

func doPing(host, proto string) (LatencyEntry, error) {
	network := "ip4"
	if proto == "IPv6" {
		network = "ip6"
	}

	// Determine whether we can use privileged raw-socket mode.
	// On macOS without root, raw sockets are denied; fall back to unprivileged
	// (UDP-based ICMP) which works without special permissions.
	privileged := os.Getuid() == 0

	return runPinger(host, network, proto, privileged)
}

func runPinger(host, network, proto string, privileged bool) (LatencyEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()

	pinger, err := probing.NewPinger(host)
	if err != nil {
		return LatencyEntry{}, fmt.Errorf("create pinger for %q: %w", host, err)
	}
	pinger.SetNetwork(network)
	pinger.Count = pingCount
	pinger.Timeout = pingTimeout
	pinger.SetPrivileged(privileged)

	done := make(chan error, 1)
	go func() { done <- pinger.Run() }()

	select {
	case err := <-done:
		if err != nil {
			// If we tried privileged mode and got a permission error, retry
			// immediately in unprivileged mode.
			if privileged && isPermissionError(err) {
				return runPinger(host, network, proto, false)
			}
			return LatencyEntry{}, fmt.Errorf("ping %q: %w", host, err)
		}
	case <-ctx.Done():
		pinger.Stop()
		return LatencyEntry{}, fmt.Errorf("ping %q: timeout", host)
	}

	stats := pinger.Statistics()
	entry := LatencyEntry{
		Average:  roundTo2(stats.AvgRtt.Seconds() * 1000),
		Protocol: proto,
	}
	if stats.PacketLoss > 0 {
		entry.PacketLoss = roundTo2(stats.PacketLoss)
	}
	return entry, nil
}

// isPermissionError reports whether err looks like an OS permission denial.
func isPermissionError(err error) bool {
	return errors.Is(err, os.ErrPermission) ||
		(err != nil && (containsStr(err.Error(), "operation not permitted") ||
			containsStr(err.Error(), "permission denied")))
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}()
}

func roundTo2(f float64) float64 {
	return float64(int(f*100+0.5)) / 100
}
