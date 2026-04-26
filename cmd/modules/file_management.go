package modules

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ─── JSON data shapes ──────────────────────────────────────────────────────

// MetricsFile is the root map: date → time → payload.
type MetricsFile map[string]DayData

// DayData maps a UTC time string to one or more data payloads.
type DayData map[string]TimeSlot

// TimeSlot holds optional latency and speedtest data for a given timestamp.
type TimeSlot struct {
	Latency   map[string][]LatencyEntry `json:"latency,omitempty"`
	Speedtest []SpeedtestEntry          `json:"speedtest,omitempty"`
}

// LatencyEntry is one ping result for a named address.
type LatencyEntry struct {
	Average    float64 `json:"average"`
	PacketLoss float64 `json:"packet_loss,omitempty"`
	Protocol   string  `json:"protocol"`
}

// SpeedtestEntry holds a single speedtest result.
type SpeedtestEntry struct {
	Download float64 `json:"download"`
	Upload   float64 `json:"upload"`
}

// ─── FileManager ───────────────────────────────────────────────────────────

// FileManager provides thread-safe read/write access to metrics.json and
// supports archiving old data into per-day files.
type FileManager struct {
	mu          sync.Mutex
	metricsPath string
	archiveDir  string
}

// NewFileManager creates a FileManager. The directory for metricsPath is
// created if it does not exist. An empty metrics.json is created if it does
// not already exist.
func NewFileManager(metricsPath, archiveDir string) (*FileManager, error) {
	if err := os.MkdirAll(filepath.Dir(metricsPath), 0o755); err != nil {
		return nil, fmt.Errorf("create metrics directory: %w", err)
	}
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return nil, fmt.Errorf("create archive directory: %w", err)
	}

	fm := &FileManager{metricsPath: metricsPath, archiveDir: archiveDir}

	// Ensure the file exists with a valid empty map.
	if _, err := os.Stat(metricsPath); os.IsNotExist(err) {
		if err2 := fm.writeRaw(MetricsFile{}); err2 != nil {
			return nil, fmt.Errorf("initialise metrics file: %w", err2)
		}
	}
	return fm, nil
}

// ReadMetrics returns the current contents of metrics.json.
func (fm *FileManager) ReadMetrics() (MetricsFile, error) {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	return fm.readRaw()
}

// AddLatency appends a latency result to the correct timestamp bucket.
func (fm *FileManager) AddLatency(ts time.Time, name string, entry LatencyEntry) error {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	data, err := fm.readRaw()
	if err != nil {
		return err
	}

	dateKey, timeKey := formatKeys(ts)
	if data[dateKey] == nil {
		data[dateKey] = DayData{}
	}
	slot := data[dateKey][timeKey]
	if slot.Latency == nil {
		slot.Latency = map[string][]LatencyEntry{}
	}
	slot.Latency[name] = append(slot.Latency[name], entry)
	data[dateKey][timeKey] = slot

	return fm.writeRaw(data)
}

// AddSpeedtest appends a speedtest result to the correct timestamp bucket.
func (fm *FileManager) AddSpeedtest(ts time.Time, entry SpeedtestEntry) error {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	data, err := fm.readRaw()
	if err != nil {
		return err
	}

	dateKey, timeKey := formatKeys(ts)
	if data[dateKey] == nil {
		data[dateKey] = DayData{}
	}
	slot := data[dateKey][timeKey]
	slot.Speedtest = append(slot.Speedtest, entry)
	data[dateKey][timeKey] = slot

	return fm.writeRaw(data)
}

// Archive moves entries older than retainSeconds into individual yyyy-mm-dd.json
// files inside archiveDir.
func (fm *FileManager) Archive(retainSeconds int64) error {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	data, err := fm.readRaw()
	if err != nil {
		return err
	}

	cutoff := time.Now().UTC().Add(-time.Duration(retainSeconds) * time.Second)

	for dateKey, dayData := range data {
		t, err := time.Parse("2006-01-02", dateKey)
		if err != nil {
			continue // skip malformed keys
		}
		if t.Before(cutoff) {
			if err := fm.archiveDay(dateKey, dayData); err != nil {
				return err
			}
			delete(data, dateKey)
		}
	}

	return fm.writeRaw(data)
}

// ─── internal helpers ──────────────────────────────────────────────────────

func (fm *FileManager) readRaw() (MetricsFile, error) {
	raw, err := os.ReadFile(fm.metricsPath)
	if err != nil {
		return nil, fmt.Errorf("read metrics: %w", err)
	}
	var mf MetricsFile
	if err := json.Unmarshal(raw, &mf); err != nil {
		// Attempt recovery: back up corrupt file and start fresh.
		backup := fm.metricsPath + ".bak"
		_ = os.Rename(fm.metricsPath, backup)
		mf = MetricsFile{}
		_ = fm.writeRaw(mf)
		return mf, fmt.Errorf("metrics.json corrupt (backed up to %s): %w", backup, err)
	}
	if mf == nil {
		mf = MetricsFile{}
	}
	return mf, nil
}

func (fm *FileManager) writeRaw(mf MetricsFile) error {
	b, err := json.MarshalIndent(mf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metrics: %w", err)
	}
	// Write atomically via temp file + rename.
	tmp := fm.metricsPath + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return fmt.Errorf("write temp metrics: %w", err)
	}
	return os.Rename(tmp, fm.metricsPath)
}

func (fm *FileManager) archiveDay(dateKey string, dayData DayData) error {
	destPath := filepath.Join(fm.archiveDir, dateKey+".json")

	// Merge with existing archive for that day if it already exists.
	existing := DayData{}
	if raw, err := os.ReadFile(destPath); err == nil {
		_ = json.Unmarshal(raw, &existing)
	}
	for k, v := range dayData {
		existing[k] = v
	}

	b, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal archive %s: %w", dateKey, err)
	}
	return os.WriteFile(destPath, b, 0o644)
}

func formatKeys(ts time.Time) (dateKey, timeKey string) {
	utc := ts.UTC()
	dateKey = utc.Format("2006-01-02")
	timeKey = utc.Format("15:04:05") + "Z"
	return
}
