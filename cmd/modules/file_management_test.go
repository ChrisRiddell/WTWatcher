package modules

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ─── helpers ────────────────────────────────────────────────────────────────

func newTestFM(t *testing.T) (*FileManager, string) {
	t.Helper()
	dir := t.TempDir()
	metricsPath := filepath.Join(dir, "public", "metrics.json")
	archiveDir := filepath.Join(dir, "archive")
	fm, err := NewFileManager(metricsPath, archiveDir)
	if err != nil {
		t.Fatalf("NewFileManager: %v", err)
	}
	return fm, dir
}

// ─── tests ──────────────────────────────────────────────────────────────────

func TestFileManager_AddLatency(t *testing.T) {
	fm, _ := newTestFM(t)

	ts := time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC)
	entry := LatencyEntry{Average: 1.5, Protocol: "IPv4"}

	if err := fm.AddLatency(ts, "Gateway", entry); err != nil {
		t.Fatalf("AddLatency: %v", err)
	}

	data, err := fm.ReadMetrics()
	if err != nil {
		t.Fatalf("ReadMetrics: %v", err)
	}

	day, ok := data["2026-04-26"]
	if !ok {
		t.Fatal("date key missing")
	}
	slot, ok := day["00:00:00Z"]
	if !ok {
		t.Fatal("time key missing")
	}
	entries, ok := slot.Latency["Gateway"]
	if !ok || len(entries) != 1 {
		t.Fatalf("expected 1 gateway entry, got %v", slot.Latency)
	}
	if entries[0].Average != 1.5 {
		t.Errorf("average: want 1.5, got %v", entries[0].Average)
	}
}

func TestFileManager_AddSpeedtest(t *testing.T) {
	fm, _ := newTestFM(t)

	ts := time.Date(2026, 4, 26, 3, 0, 0, 0, time.UTC)
	entry := SpeedtestEntry{Download: 88.47, Upload: 9.19}

	if err := fm.AddSpeedtest(ts, entry); err != nil {
		t.Fatalf("AddSpeedtest: %v", err)
	}

	data, err := fm.ReadMetrics()
	if err != nil {
		t.Fatalf("ReadMetrics: %v", err)
	}
	slot := data["2026-04-26"]["03:00:00Z"]
	if len(slot.Speedtest) != 1 {
		t.Fatalf("expected 1 speedtest entry, got %d", len(slot.Speedtest))
	}
	if slot.Speedtest[0].Download != 88.47 {
		t.Errorf("download: want 88.47, got %v", slot.Speedtest[0].Download)
	}
}

func TestFileManager_CorruptionRecovery(t *testing.T) {
	fm, dir := newTestFM(t)
	metricsPath := filepath.Join(dir, "public", "metrics.json")

	// Corrupt the file.
	if err := os.WriteFile(metricsPath, []byte("not json!!!"), 0o644); err != nil {
		t.Fatal(err)
	}

	// ReadMetrics should recover and return an empty map, not panic.
	data, err := fm.ReadMetrics()
	if err == nil {
		t.Fatal("expected error from corrupt file, got nil")
	}
	if data == nil {
		t.Fatal("expected non-nil data after recovery")
	}

	// Backup file should now exist.
	if _, err := os.Stat(metricsPath + ".bak"); err != nil {
		t.Errorf("backup file not created: %v", err)
	}
}

func TestFileManager_Archive(t *testing.T) {
	fm, dir := newTestFM(t)

	// Insert data for a date that's clearly in the past (30 days ago).
	oldDate := time.Now().UTC().AddDate(0, 0, -30)
	entry := LatencyEntry{Average: 5.0, Protocol: "IPv4"}
	if err := fm.AddLatency(oldDate, "Gateway", entry); err != nil {
		t.Fatalf("AddLatency: %v", err)
	}

	// Also insert today's data.
	now := time.Now().UTC()
	if err := fm.AddLatency(now, "Gateway", entry); err != nil {
		t.Fatalf("AddLatency today: %v", err)
	}

	// Archive data older than 14 days.
	retainSecs := int64(14 * 86400)
	if err := fm.Archive(retainSecs); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	data, err := fm.ReadMetrics()
	if err != nil {
		t.Fatalf("ReadMetrics after archive: %v", err)
	}

	// Old date should be gone from metrics.json.
	oldDateKey := oldDate.Format("2006-01-02")
	if _, ok := data[oldDateKey]; ok {
		t.Errorf("old date %s still present in metrics.json after archiving", oldDateKey)
	}

	// Archive file for old date should exist.
	archivePath := filepath.Join(dir, "archive", oldDateKey+".json")
	raw, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("archive file missing for %s: %v", oldDateKey, err)
	}
	var archived DayData
	if err := json.Unmarshal(raw, &archived); err != nil {
		t.Fatalf("unmarshal archive: %v", err)
	}
	if len(archived) == 0 {
		t.Error("archived file is empty")
	}

	// Today's data should still be in metrics.json.
	todayKey := now.Format("2006-01-02")
	if _, ok := data[todayKey]; !ok {
		t.Errorf("today's data %s missing from metrics.json", todayKey)
	}
}

func TestFileManager_MetricsJSONStructure(t *testing.T) {
	fm, _ := newTestFM(t)

	ts := time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC)

	// Add latency for Gateway (IPv4)
	if err := fm.AddLatency(ts, "Gateway", LatencyEntry{Average: 1.5, Protocol: "IPv4"}); err != nil {
		t.Fatal(err)
	}
	// Add latency for Cloudflare DNS (IPv6, with packet loss)
	if err := fm.AddLatency(ts, "Cloudflare DNS", LatencyEntry{Average: 15.5, PacketLoss: 20, Protocol: "IPv6"}); err != nil {
		t.Fatal(err)
	}
	// Add latency for Youtube (IPv4 + IPv6)
	if err := fm.AddLatency(ts, "Youtube", LatencyEntry{Average: 15.5, Protocol: "IPv4"}); err != nil {
		t.Fatal(err)
	}
	if err := fm.AddLatency(ts, "Youtube", LatencyEntry{Average: 16.5, Protocol: "IPv6"}); err != nil {
		t.Fatal(err)
	}
	// Add speedtest at 03:00
	tsST := time.Date(2026, 4, 26, 3, 0, 0, 0, time.UTC)
	if err := fm.AddSpeedtest(tsST, SpeedtestEntry{Download: 88.47, Upload: 9.19}); err != nil {
		t.Fatal(err)
	}

	data, err := fm.ReadMetrics()
	if err != nil {
		t.Fatal(err)
	}

	day := data["2026-04-26"]
	if len(day["00:00:00Z"].Latency) != 3 {
		t.Errorf("expected 3 latency hosts, got %d", len(day["00:00:00Z"].Latency))
	}
	if len(day["00:00:00Z"].Latency["Youtube"]) != 2 {
		t.Errorf("expected 2 Youtube entries (v4+v6), got %d", len(day["00:00:00Z"].Latency["Youtube"]))
	}
	if len(day["03:00:00Z"].Speedtest) != 1 {
		t.Errorf("expected 1 speedtest entry, got %d", len(day["03:00:00Z"].Speedtest))
	}
}
