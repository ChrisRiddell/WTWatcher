package modules

import (
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"time"
)

// rawSpeedtest matches the JSON shape emitted by the speedtest CLI.
type rawSpeedtest struct {
	Download struct {
		Bandwidth int64 `json:"bandwidth"` // bytes/s
	} `json:"download"`
	Upload struct {
		Bandwidth int64 `json:"bandwidth"` // bytes/s
	} `json:"upload"`
}

// RunSpeedtest executes the speedtest CLI, parses its JSON output, and saves
// the result into fm at the given UTC timestamp ts.
func RunSpeedtest(fm *FileManager, logger *Logger, ts time.Time) {
	fmt.Printf("[speedtest] starting speedtest run at %s\n", ts.Format("15:04:05Z"))
	entry, err := execSpeedtest()
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

// execSpeedtest is a thin wrapper around the CLI so tests can mock it.
var execSpeedtest = func() (SpeedtestEntry, error) {
	out, err := exec.Command(
		"speedtest",
		"--accept-license",
		"--accept-gdpr",
		"--format=json",
	).Output()
	if err != nil {
		return SpeedtestEntry{}, fmt.Errorf("speedtest command: %w", err)
	}
	return parseSpeedtestOutput(out)
}

// parseSpeedtestOutput converts raw CLI JSON into a SpeedtestEntry.
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

// bpsToMbps converts bytes-per-second to Megabits-per-second, rounded to 2dp.
func bpsToMbps(bps int64) float64 {
	mbps := float64(bps) * 8 / 1_000_000
	return math.Round(mbps*100) / 100
}
