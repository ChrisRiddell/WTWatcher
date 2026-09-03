package modules

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"net/netip"
	"os"
	"slices"
	"strings"
	"time"

	probing "github.com/prometheus-community/pro-bing"
)

// RunPing iterates over all targets in Config, resolves their network endpoints,
// measures ICMP round-trip latency and packet loss, checks for statistical anomalies,
// and saves the results to the FileManager.
func RunPing(ctx context.Context, cfg *Config, fm *FileManager, logger *Logger, ts time.Time) {
	fmt.Printf("[ping] starting ping run at %s\n", formatConsoleTime(ts))
	for _, addr := range cfg.Addresses {
		if ctx.Err() != nil {
			logger.Warn("ping run cancelled", "error", ctx.Err())
			return
		}
		targets, err := resolveTargets(addr)
		if err != nil {
			logger.Warn("ping: could not resolve target",
				"name", addr.Name, "error", err)
			fmt.Printf("[ping] %-20s WARN: %v\n", addr.Name, err)
			continue
		}
		for _, t := range targets {
			if ctx.Err() != nil {
				logger.Warn("ping run cancelled", "error", ctx.Err())
				return
			}
			entry, err := pingWithRetry(ctx, t.host, t.proto, cfg.Ping)
			if err != nil {
				logger.Error("ping failed",
					"name", addr.Name, "host", t.host, "proto", t.proto, "error", err)
				fmt.Printf("[ping] %-20s %-5s FAILED: %v\n", addr.Name, t.proto, err)
				continue
			}
			if entry.IsAnomaly {
				logger.Warn("ping anomaly detected — spike filtered",
					"name", addr.Name, "host", t.host,
					"unfiltered_avg_ms", entry.RawAverage,
					"filtered_avg_ms", entry.Average,
					"proto", entry.Protocol)
				fmt.Printf("[ping] %-20s %-5s ANOMALY unfiltered avg=%.2f ms -> filtered avg=%.2f ms  loss=%.0f%%\n",
					addr.Name, entry.Protocol, entry.RawAverage, entry.Average, entry.PacketLoss)
			}
			if err := fm.AddLatency(ts, addr.Name, entry); err != nil {
				logger.Error("save latency failed",
					"name", addr.Name, "error", err)
			} else if !entry.IsAnomaly {
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

// pingTarget pairs a concrete IP or hostname with its IP family ("IPv4" or "IPv6").
type pingTarget struct {
	host  string
	proto string // "IPv4" or "IPv6"
}

// resolveTargets expands an Address configuration entry into one or more concrete ping targets.
// If the target is an explicit IP address, it returns that IP.
// If the target is a domain name, it resolves the required protocol family or both A and AAAA records.
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

	// Resolve domain target based on configured protocol mode.
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

// resolveBoth performs DNS lookups for a domain and picks one IPv4 address and one IPv6 address.
func resolveBoth(domain string) ([]pingTarget, error) {
	ips, err := net.LookupHost(domain)
	if err != nil {
		return nil, fmt.Errorf("DNS lookup %q: %w", domain, err)
	}
	seen := map[string]bool{}
	var targets []pingTarget
	for _, ip := range ips {
		addr, err := netip.ParseAddr(ip)
		if err != nil {
			continue
		}
		proto := "IPv4"
		if addr.Is6() {
			proto = "IPv6"
		}
		// Capture at most one IP per protocol family for consistent latency tracking.
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

// pingWithRetry executes doPing up to cfg.Retries additional times if errors occur.
func pingWithRetry(ctx context.Context, host, proto string, pingCfg Ping) (LatencyEntry, error) {
	var lastErr error
	for i := 0; i <= pingCfg.Retries; i++ {
		if ctx.Err() != nil {
			return LatencyEntry{}, ctx.Err()
		}
		entry, err := doPing(ctx, host, proto, pingCfg)
		if err == nil {
			return entry, nil
		}
		lastErr = err
	}
	return LatencyEntry{}, lastErr
}

// doPing determines network routing type ("ip4" or "ip6") and socket privilege mode before executing runPinger.
func doPing(ctx context.Context, host, proto string, pingCfg Ping) (LatencyEntry, error) {
	network := "ip4"
	if proto == "IPv6" {
		network = "ip6"
	}

	// Check if running as root (UID 0).
	// On Linux/macOS, non-root users cannot open raw ICMP sockets without special capabilities,
	// so we default to unprivileged UDP-based ICMP when not running as root.
	privileged := os.Getuid() == 0

	return runPinger(ctx, host, network, proto, privileged, pingCfg)
}

// runPinger initializes and executes a pro-bing pinger instance.
// If privileged raw socket mode fails with a permission error, it automatically retries
// using unprivileged ICMP datagram sockets.
func runPinger(ctx context.Context, host, network, proto string, privileged bool, pingCfg Ping) (LatencyEntry, error) {
	timeout := time.Duration(pingCfg.TimeoutSeconds) * time.Second
	pCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	pinger, err := probing.NewPinger(host)
	if err != nil {
		return LatencyEntry{}, fmt.Errorf("create pinger for %q: %w", host, err)
	}
	pinger.SetNetwork(network)
	pinger.Count = pingCfg.Count
	pinger.Timeout = timeout
	pinger.SetPrivileged(privileged)

	done := make(chan error, 1)
	go func() { done <- pinger.Run() }()

	select {
	case err := <-done:
		if err != nil {
			// On permission denial (e.g. non-root on Darwin or missing CAP_NET_RAW on Linux),
			// transparently retry using unprivileged UDP ICMP.
			if privileged && isPermissionError(err) {
				return runPinger(ctx, host, network, proto, false, pingCfg)
			}
			return LatencyEntry{}, fmt.Errorf("ping %q: %w", host, err)
		}
	case <-pCtx.Done():
		pinger.Stop()
		// Wait for the running pinger goroutine to terminate cleanly.
		<-done
		return LatencyEntry{}, fmt.Errorf("ping %q: timeout or cancelled: %w", host, pCtx.Err())
	}

	stats := pinger.Statistics()
	rawAvg := roundTo2(stats.AvgRtt.Seconds() * 1000)

	entry := LatencyEntry{
		Average:  rawAvg,
		Protocol: proto,
	}
	if stats.PacketLoss > 0 {
		entry.PacketLoss = roundTo2(stats.PacketLoss)
	}

	// ── Anomaly detection ────────────────────────────────────────────────
	// Check for temporary spikes caused by OS scheduling delays, system sleep,
	// or garbage collection pauses rather than true network latency.
	// We apply Tukey's Interquartile Range (IQR) fence to per-packet RTTs.
	// If outliers exceeding anomalyThresholdMs are detected, replace the average
	// with the clean sample average, retain the unfiltered raw average, and flag IsAnomaly.
	filtered := filterAnomalyRTTs(stats.Rtts, pingCfg.AnomalyThresholdMs)
	if filtered != nil {
		var sum float64
		for _, rtt := range filtered {
			sum += rtt.Seconds() * 1000
		}
		filteredAvg := roundTo2(sum / float64(len(filtered)))
		entry.Average = filteredAvg
		entry.RawAverage = rawAvg
		entry.IsAnomaly = true
	}

	return entry, nil
}

// filterAnomalyRTTs applies Tukey's boxplot fence outlier detection to a slice of packet RTTs:
// 1. Sorts RTTs and calculates 25th percentile (Q1) and 75th percentile (Q3).
// 2. Computes the Interquartile Range: IQR = Q3 - Q1.
// 3. Sets upper outlier fence: Fence = max(Q3 + 1.5 * IQR, anomalyThresholdMs).
// 4. Returns non-outlier RTTs if at least 2 clean samples remain and at least one outlier was filtered out.
func filterAnomalyRTTs(rtts []time.Duration, anomalyThresholdMs int64) []time.Duration {
	if len(rtts) < 2 {
		return nil
	}

	threshold := time.Duration(anomalyThresholdMs) * time.Millisecond

	sorted := slices.Clone(rtts)
	slices.Sort(sorted)

	q1 := percentile(sorted, 25)
	q3 := percentile(sorted, 75)
	iqr := q3 - q1
	fence := max(q3+time.Duration(1.5*float64(iqr)), threshold)

	var good []time.Duration
	for _, rtt := range sorted {
		if rtt <= fence {
			good = append(good, rtt)
		}
	}

	if len(good) < 2 || len(good) == len(sorted) {
		// Not enough clean samples or no outliers were eliminated.
		return nil
	}
	return good
}

// percentile computes the p-th percentile (0..100) from a sorted slice of durations
// using linear interpolation between adjacent ranks.
func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	rank := p / 100 * float64(len(sorted)-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	if lo == hi {
		return sorted[lo]
	}
	frac := rank - float64(lo)
	return sorted[lo] + time.Duration(frac*float64(sorted[hi]-sorted[lo]))
}

// isPermissionError reports whether an error originates from an OS socket permission denial.
func isPermissionError(err error) bool {
	return errors.Is(err, os.ErrPermission) ||
		(err != nil && (strings.Contains(err.Error(), "operation not permitted") ||
			strings.Contains(err.Error(), "permission denied")))
}

// roundTo2 rounds a floating-point number to two decimal places.
func roundTo2(f float64) float64 {
	return math.Round(f*100) / 100
}
