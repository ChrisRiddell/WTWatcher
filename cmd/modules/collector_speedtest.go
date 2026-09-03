package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"strings"
	"time"
)

// rawSpeedtest reflects the relevant subsets of the JSON payload produced by the official Ookla speedtest CLI.
type rawSpeedtest struct {
	Download struct {
		Bandwidth int64 `json:"bandwidth"` // Bandwidth in bytes per second (B/s).
	} `json:"download"`
	Upload struct {
		Bandwidth int64 `json:"bandwidth"` // Bandwidth in bytes per second (B/s).
	} `json:"upload"`
}

// RunSpeedtest executes the speedtest CLI subprocess, parses the measurement output,
// logs throughput statistics, and persists the entry into the metrics data store.
func RunSpeedtest(ctx context.Context, speedtestCfg Speedtest, fm *FileManager, logger *Logger, ts time.Time) {
	if ctx.Err() != nil {
		logger.Warn("speedtest run cancelled", "error", ctx.Err())
		return
	}
	fmt.Printf("[speedtest] starting speedtest run at %s\n", formatConsoleTime(ts))
	entry, err := execSpeedtest(ctx, speedtestCfg.ServerID)
	if err != nil {
		logger.Error("speedtest failed", "error", err)
		fmt.Printf("[speedtest] FAILED: %v\n", err)
		return
	}
	if err := fm.AddSpeedtest(ts, entry); err != nil {
		logger.Error("save speedtest failed", "error", err)
		fmt.Printf("[speedtest] FAILED to save: %v\n", err)
		return
	}
	logger.Info("speedtest ok",
		"download_mbps", entry.Download,
		"upload_mbps", entry.Upload)
	fmt.Printf("[speedtest] down=%.2f Mbps  up=%.2f Mbps\n", entry.Download, entry.Upload)
	fmt.Println("[speedtest] run complete")
}

// buildSpeedtestArgs constructs the argument list for invoking the Ookla speedtest CLI.
// When serverID is non-empty and not set to "AUTO" (case-insensitive), it appends "--server-id" and the ID.
func buildSpeedtestArgs(serverID string) []string {
	args := []string{
		"--accept-license",
		"--accept-gdpr",
		"--format=json",
	}
	trimmed := strings.TrimSpace(serverID)
	if trimmed != "" && !strings.EqualFold(trimmed, "auto") {
		args = append(args, "--server-id", trimmed)
	}
	return args
}

// execSpeedtest wraps the external Ookla CLI invocation. Storing it in a function variable
// allows unit tests to stub/mock execution without requiring the real speedtest binary installed.
var execSpeedtest = func(ctx context.Context, serverID string) (SpeedtestEntry, error) {
	args := buildSpeedtestArgs(serverID)
	cmd := exec.CommandContext(ctx, "speedtest", args...)
	out, err := cmd.Output()
	if err != nil {
		// If command failed with an exit error, extract stderr output to provide a descriptive error message.
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			return SpeedtestEntry{}, fmt.Errorf("speedtest command: %w (stderr: %s)", err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return SpeedtestEntry{}, fmt.Errorf("speedtest command: %w", err)
	}
	return parseSpeedtestOutput(out)
}

// parseSpeedtestOutput deserializes CLI JSON output and converts raw byte rates into Megabits per second.
func parseSpeedtestOutput(data []byte) (SpeedtestEntry, error) {
	var raw rawSpeedtest
	if err := json.Unmarshal(data, &raw); err != nil {
		return SpeedtestEntry{}, fmt.Errorf("parse speedtest JSON: %w", err)
	}
	return SpeedtestEntry{
		Download: bpsToMbps(raw.Download.Bandwidth),
		Upload:   bpsToMbps(raw.Upload.Bandwidth),
	}, nil
}

// bpsToMbps converts raw byte rates (bytes/second) to Megabits per second (Mbps), rounded to 2 decimal places.
// Formula: (Bytes/sec * 8 bits/Byte) / 1,000,000 bits/Megabit
func bpsToMbps(bps int64) float64 {
	mbps := float64(bps) * 8 / 1_000_000
	return math.Round(mbps*100) / 100
}
