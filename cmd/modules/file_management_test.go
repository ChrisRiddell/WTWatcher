package modules

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ─── helpers ────────────────────────────────────────────────────────────────

func newTestFM(t *testing.T) (*FileManager, string) {
	t.Helper()
	dir := t.TempDir()
	metricsPath := filepath.Join(dir, "public", "metrics.json")
	archiveDir := filepath.Join(dir, "archive")
	fm, err := NewFileManager(metricsPath, archiveDir, nil)
	if err != nil {
		t.Fatalf("NewFileManager: %v", err)
	}
	return fm, dir
}

// ─── tests ──────────────────────────────────────────────────────────────────

func TestFileManager_AddLatency(t *testing.T) {
	fm, dir := newTestFM(t)

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
	entries := slot.Latency.Get("Gateway")
	if entries == nil || len(entries) != 1 {
		t.Fatalf("expected 1 gateway entry, got %v", slot.Latency)
	}
	if entries[0].Average != 1.5 {
		t.Errorf("average: want 1.5, got %v", entries[0].Average)
	}

	// Verify an anomaly entry ignores RawAverage during serialization.
	entryAnomaly := LatencyEntry{Average: 3.12, RawAverage: 2150.5, Protocol: "IPv4", IsAnomaly: true}
	if err := fm.AddLatency(ts, "Youtube", entryAnomaly); err != nil {
		t.Fatalf("AddLatency anomaly: %v", err)
	}
	rawBytes, err := os.ReadFile(filepath.Join(dir, "public", "metrics.json"))
	if err != nil {
		t.Fatalf("read raw metrics: %v", err)
	}
	if strings.Contains(string(rawBytes), "raw_average") || strings.Contains(string(rawBytes), "RawAverage") {
		t.Errorf("metrics.json should not contain raw_average: %s", string(rawBytes))
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
	if len(day["00:00:00Z"].Latency.Get("Youtube")) != 2 {
		t.Errorf("expected 2 Youtube entries (v4+v6), got %d", len(day["00:00:00Z"].Latency.Get("Youtube")))
	}
	if len(day["03:00:00Z"].Speedtest) != 1 {
		t.Errorf("expected 1 speedtest entry, got %d", len(day["03:00:00Z"].Speedtest))
	}
}

func TestFileManager_LatencyOrder(t *testing.T) {
	// Add targets in config order (Gateway → Cloudflare DNS → Youtube) and
	// confirm that the JSON emits them in that exact order, not alphabetically.
	fm, _ := newTestFM(t)
	ts := time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC)

	for _, name := range []string{"Gateway", "Cloudflare DNS", "Youtube"} {
		if err := fm.AddLatency(ts, name, LatencyEntry{Average: 1.0, Protocol: "IPv4"}); err != nil {
			t.Fatalf("AddLatency(%s): %v", name, err)
		}
	}

	data, err := fm.ReadMetrics()
	if err != nil {
		t.Fatalf("ReadMetrics: %v", err)
	}

	latency := data["2026-04-26"]["00:00:00Z"].Latency
	if len(latency) != 3 {
		t.Fatalf("want 3 latency entries, got %d", len(latency))
	}

	wantOrder := []string{"Gateway", "Cloudflare DNS", "Youtube"}
	for i, want := range wantOrder {
		if latency[i].Name != want {
			t.Errorf("latency[%d]: want %q, got %q", i, want, latency[i].Name)
		}
	}

	// Also confirm the JSON text has the keys in the right order.
	raw, err := json.Marshal(latency)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	text := string(raw)
	gatewayIdx := strings.Index(text, `"Gateway"`)
	cfIdx := strings.Index(text, `"Cloudflare DNS"`)
	youtubeIdx := strings.Index(text, `"Youtube"`)
	if !(gatewayIdx < cfIdx && cfIdx < youtubeIdx) {
		t.Errorf("JSON key order wrong: Gateway=%d Cloudflare DNS=%d Youtube=%d\nJSON: %s",
			gatewayIdx, cfIdx, youtubeIdx, text)
	}
}

func TestFileManager_TaskStaging(t *testing.T) {
	fm, dir := newTestFM(t)
	metricsPath := filepath.Join(dir, "public", "metrics.json")
	tmpPath := filepath.Join(dir, "public", "metrics.json.tmp")

	// 1. Record initial baseline data.
	initialTS := time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC)
	if err := fm.AddLatency(initialTS, "Gateway", LatencyEntry{Average: 1.0, Protocol: "IPv4"}); err != nil {
		t.Fatalf("AddLatency baseline: %v", err)
	}

	// 2. Begin a task session.
	if err := fm.StartTask(); err != nil {
		t.Fatalf("StartTask: %v", err)
	}

	// Staging .tmp file must exist right after StartTask.
	if _, err := os.Stat(tmpPath); err != nil {
		t.Fatalf("expected .tmp file after StartTask: %v", err)
	}

	// 3. Add a new latency measurement during the task.
	taskTS := time.Date(2026, 4, 26, 0, 15, 0, 0, time.UTC)
	if err := fm.AddLatency(taskTS, "Gateway", LatencyEntry{Average: 2.5, Protocol: "IPv4"}); err != nil {
		t.Fatalf("AddLatency in task: %v", err)
	}

	// Verify that the active metrics.json file has NOT been modified yet.
	committedData, err := fm.ReadMetrics()
	if err != nil {
		t.Fatalf("ReadMetrics during task: %v", err)
	}
	if _, exists := committedData["2026-04-26"]["00:15:00Z"]; exists {
		t.Fatal("active metrics.json should NOT contain task data before CommitTask")
	}

	// Verify that the staging .tmp file DOES contain the new measurement.
	tmpBytes, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatalf("read .tmp file: %v", err)
	}
	var stagedData MetricsFile
	if err := json.Unmarshal(tmpBytes, &stagedData); err != nil {
		t.Fatalf("unmarshal .tmp file: %v", err)
	}
	if entries := stagedData["2026-04-26"]["00:15:00Z"].Latency.Get("Gateway"); len(entries) != 1 || entries[0].Average != 2.5 {
		t.Fatalf("staged data missing or incorrect in .tmp: %v", stagedData)
	}

	// 4. Add a second target to simulate a multi-target probe cycle.
	if err := fm.AddLatency(taskTS, "Cloudflare DNS", LatencyEntry{Average: 12.0, Protocol: "IPv4"}); err != nil {
		t.Fatalf("AddLatency second target in task: %v", err)
	}

	// Active file still must not contain the time slot.
	committedData, err = fm.ReadMetrics()
	if err != nil {
		t.Fatalf("ReadMetrics during task: %v", err)
	}
	if _, exists := committedData["2026-04-26"]["00:15:00Z"]; exists {
		t.Fatal("active metrics.json should still NOT contain task data before CommitTask")
	}

	// 5. Commit the task.
	if err := fm.CommitTask(); err != nil {
		t.Fatalf("CommitTask: %v", err)
	}

	// Staging .tmp file must be gone (renamed over metrics.json).
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Fatalf("expected .tmp file to be removed after CommitTask, stat err: %v", err)
	}

	// Active metrics.json must now contain both targets for the new time slot.
	finalData, err := fm.ReadMetrics()
	if err != nil {
		t.Fatalf("ReadMetrics after commit: %v", err)
	}
	slot, exists := finalData["2026-04-26"]["00:15:00Z"]
	if !exists {
		t.Fatal("expected 00:15:00Z to exist in metrics.json after CommitTask")
	}
	if len(slot.Latency) != 2 {
		t.Fatalf("expected 2 latency targets in committed slot, got %d", len(slot.Latency))
	}

	// Confirm on-disk file content matches.
	diskBytes, err := os.ReadFile(metricsPath)
	if err != nil {
		t.Fatalf("read metrics.json: %v", err)
	}
	if !strings.Contains(string(diskBytes), "Cloudflare DNS") {
		t.Fatal("metrics.json does not contain Cloudflare DNS")
	}
}

func TestFileManager_TaskAbort(t *testing.T) {
	fm, dir := newTestFM(t)
	tmpPath := filepath.Join(dir, "public", "metrics.json.tmp")

	// 1. Record initial baseline data.
	initialTS := time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC)
	if err := fm.AddLatency(initialTS, "Gateway", LatencyEntry{Average: 1.0, Protocol: "IPv4"}); err != nil {
		t.Fatalf("AddLatency baseline: %v", err)
	}

	// 2. Begin a task session.
	if err := fm.StartTask(); err != nil {
		t.Fatalf("StartTask: %v", err)
	}

	// 3. Add partial measurement.
	taskTS := time.Date(2026, 4, 26, 0, 15, 0, 0, time.UTC)
	if err := fm.AddLatency(taskTS, "Gateway", LatencyEntry{Average: 99.9, Protocol: "IPv4"}); err != nil {
		t.Fatalf("AddLatency in task: %v", err)
	}

	if _, err := os.Stat(tmpPath); err != nil {
		t.Fatalf("expected .tmp file to exist: %v", err)
	}

	// 4. Abort the task (simulating cancellation or timeout).
	fm.AbortTask()

	// .tmp file must be removed.
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Fatalf("expected .tmp file to be deleted after AbortTask, got: %v", err)
	}

	// Active metrics.json must remain completely untouched.
	data, err := fm.ReadMetrics()
	if err != nil {
		t.Fatalf("ReadMetrics: %v", err)
	}
	if _, exists := data["2026-04-26"]["00:15:00Z"]; exists {
		t.Fatal("metrics.json should NOT contain data from aborted task")
	}
}

func TestFileManager_StaleTmpCleanedOnStartup(t *testing.T) {
	dir := t.TempDir()
	metricsPath := filepath.Join(dir, "public", "metrics.json")
	tmpPath := filepath.Join(dir, "public", "metrics.json.tmp")

	if err := os.MkdirAll(filepath.Dir(metricsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	// Simulate an ungraceful crash leaving behind a stale .tmp file.
	if err := os.WriteFile(tmpPath, []byte(`{"stale": true}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Initializing NewFileManager should clean up the stale file.
	_, err := NewFileManager(metricsPath, filepath.Join(dir, "archive"), nil)
	if err != nil {
		t.Fatalf("NewFileManager: %v", err)
	}

	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Fatalf("stale .tmp file was not cleaned up on startup: %v", err)
	}
}
