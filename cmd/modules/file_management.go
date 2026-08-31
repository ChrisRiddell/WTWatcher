package modules

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ─── JSON data shapes ──────────────────────────────────────────────────────

// MetricsFile is the top-level structure of metrics.json.
// It is keyed by UTC date ("yyyy-MM-dd"), mapping to time slots within that day.
type MetricsFile map[string]DayData

// DayData maps a UTC time string ("HH:mm:ssZ") to measurement results for that specific moment.
type DayData map[string]TimeSlot

// TimeSlot holds optional latency and speedtest data for a given timestamp.
type TimeSlot struct {
	Latency   OrderedLatency   `json:"latency,omitempty"`
	Speedtest []SpeedtestEntry `json:"speedtest,omitempty"`
}

// latencyEntry pairs an address/target name with its collected ping measurements.
type latencyEntry struct {
	Name    string
	Entries []LatencyEntry
}

// OrderedLatency is an ordered collection of named latency entries.
// Standard Go maps do not preserve key insertion order when serialized with encoding/json
// (Go maps sort keys alphabetically). To maintain the visual ordering defined by the user
// in config.yml, OrderedLatency implements custom JSON marshaling and unmarshaling
// that serializes to and deserializes from a standard JSON object while preserving slice order.
type OrderedLatency []latencyEntry

// Get returns the ping measurements for target name, or nil if not present.
func (ol OrderedLatency) Get(name string) []LatencyEntry {
	for i := range ol {
		if ol[i].Name == name {
			return ol[i].Entries
		}
	}
	return nil
}

// MarshalJSON emits a JSON object {"target1": [...], "target2": [...]} maintaining
// the slice's exact element order.
func (ol OrderedLatency) MarshalJSON() ([]byte, error) {
	if len(ol) == 0 {
		return nil, nil
	}
	// Manually construct the JSON object byte stream to preserve target order.
	var buf []byte
	buf = append(buf, '{')
	for i, e := range ol {
		if i > 0 {
			buf = append(buf, ',')
		}
		key, err := json.Marshal(e.Name)
		if err != nil {
			return nil, err
		}
		val, err := json.Marshal(e.Entries)
		if err != nil {
			return nil, err
		}
		buf = append(buf, key...)
		buf = append(buf, ':')
		buf = append(buf, val...)
	}
	buf = append(buf, '}')
	return buf, nil
}

// UnmarshalJSON reads a JSON object into an OrderedLatency slice, preserving the exact
// order of keys as they appear in the JSON document via json.Decoder token stream.
func (ol *OrderedLatency) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))

	// Consume the opening '{' delimiter of the JSON object.
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return fmt.Errorf("OrderedLatency: expected '{', got %v", tok)
	}

	var result OrderedLatency
	for dec.More() {
		// Read the property key (target name).
		keyTok, err := dec.Token()
		if err != nil {
			return err
		}
		name, ok := keyTok.(string)
		if !ok {
			return fmt.Errorf("OrderedLatency: expected string key, got %T", keyTok)
		}
		// Decode the array of LatencyEntry measurements.
		var entries []LatencyEntry
		if err := dec.Decode(&entries); err != nil {
			return fmt.Errorf("OrderedLatency[%s]: %w", name, err)
		}
		result = append(result, latencyEntry{Name: name, Entries: entries})
	}

	// Consume the closing '}' delimiter.
	if _, err := dec.Token(); err != nil {
		return err
	}
	*ol = result
	return nil
}

// LatencyEntry holds a single ping measurement result.
// IsAnomaly is true when the raw RTT was determined to be an outlier by the IQR spike filter;
// RawAverage preserves the unfiltered mean in-memory for logging and console output without writing to metrics JSON.
type LatencyEntry struct {
	Average    float64 `json:"average"`
	RawAverage float64 `json:"-"`
	PacketLoss float64 `json:"packet_loss,omitempty"`
	Protocol   string  `json:"protocol"`
	IsAnomaly  bool    `json:"is_anomaly,omitempty"`
}

// SpeedtestEntry holds download and upload speed measurements in Megabits per second (Mbps).
type SpeedtestEntry struct {
	Download float64 `json:"download"`
	Upload   float64 `json:"upload"`
}

// ─── FileManager ───────────────────────────────────────────────────────────

// FileManager provides thread-safe access to metrics.json and handles automated
// archival of historical data into per-day files in archiveDir.
type FileManager struct {
	mu          sync.Mutex
	metricsPath string
	archiveDir  string
	logger      *Logger
}

// NewFileManager initializes a FileManager, ensuring target directories exist and
// metrics.json is initialized with a valid empty JSON object if not present.
func NewFileManager(metricsPath, archiveDir string, logger *Logger) (*FileManager, error) {
	if err := os.MkdirAll(filepath.Dir(metricsPath), 0o755); err != nil {
		return nil, fmt.Errorf("create metrics directory: %w", err)
	}
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return nil, fmt.Errorf("create archive directory: %w", err)
	}

	fm := &FileManager{metricsPath: metricsPath, archiveDir: archiveDir, logger: logger}

	// Initialize metrics.json with an empty JSON object if file is missing.
	if _, err := os.Stat(metricsPath); errors.Is(err, os.ErrNotExist) {
		if err := fm.writeRaw(MetricsFile{}); err != nil {
			return nil, fmt.Errorf("initialise metrics file: %w", err)
		}
	}
	return fm, nil
}

// ReadMetrics returns a thread-safe snapshot of the current metrics.json content.
func (fm *FileManager) ReadMetrics() (MetricsFile, error) {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	return fm.readRaw()
}

// AddLatency records a latency result under the given UTC timestamp, creating date/time slots
// as needed while maintaining target insertion order.
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

	// Append to existing entry for this target name or add a new entry to the ordered list.
	found := false
	for i := range slot.Latency {
		if slot.Latency[i].Name == name {
			slot.Latency[i].Entries = append(slot.Latency[i].Entries, entry)
			found = true
			break
		}
	}
	if !found {
		slot.Latency = append(slot.Latency, latencyEntry{Name: name, Entries: []LatencyEntry{entry}})
	}
	data[dateKey][timeKey] = slot

	return fm.writeRaw(data)
}

// AddSpeedtest records a speedtest measurement under the given UTC timestamp bucket.
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

// Archive scans metrics.json for date entries older than retainSeconds, moves them into
// individual archive files (archive/yyyy-mm-dd.json), and deletes them from active metrics.json.
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
			continue // Skip unrecognized date formats.
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

// readRaw reads and decodes metricsPath. If JSON parsing fails due to external file corruption,
// it creates a .bak backup and recovers with an empty dataset to prevent total application failure.
func (fm *FileManager) readRaw() (MetricsFile, error) {
	raw, err := os.ReadFile(fm.metricsPath)
	if err != nil {
		return nil, fmt.Errorf("read metrics: %w", err)
	}
	var mf MetricsFile
	if err := json.Unmarshal(raw, &mf); err != nil {
		// Attempt self-recovery: preserve corrupted file for inspection and reset active file.
		backup := fm.metricsPath + ".bak"
		if renameErr := os.Rename(fm.metricsPath, backup); renameErr != nil {
			return nil, fmt.Errorf("metrics.json corrupt (%w) and failed to rename to %s: %v", err, backup, renameErr)
		}
		mf = MetricsFile{}
		if writeErr := fm.writeRaw(mf); writeErr != nil {
			return nil, fmt.Errorf("metrics.json corrupt (backed up to %s), failed to write empty file: %w", backup, writeErr)
		}
		return mf, fmt.Errorf("metrics.json corrupt (backed up to %s): %w", backup, err)
	}
	if mf == nil {
		mf = MetricsFile{}
	}
	return mf, nil
}

// writeRaw formats and writes data to disk atomically using a temporary file and rename.
// This guarantees that partially-written or interrupted files never corrupt metrics.json.
func (fm *FileManager) writeRaw(mf MetricsFile) error {
	b, err := json.MarshalIndent(mf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metrics: %w", err)
	}
	// Atomic write: write to .tmp file first, then atomically rename over the target.
	tmp := fm.metricsPath + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return fmt.Errorf("write temp metrics: %w", err)
	}
	return os.Rename(tmp, fm.metricsPath)
}

// archiveDay merges archived dayData into archiveDir/yyyy-mm-dd.json atomically.
func (fm *FileManager) archiveDay(dateKey string, dayData DayData) error {
	destPath := filepath.Join(fm.archiveDir, dateKey+".json")

	// Merge with existing archived entries for that day if the file already exists.
	existing := DayData{}
	if raw, err := os.ReadFile(destPath); err == nil {
		if unmarshalErr := json.Unmarshal(raw, &existing); unmarshalErr != nil {
			if fm.logger != nil {
				fm.logger.Warn("archive file corrupt — overwriting with fresh data",
					"date", dateKey, "path", destPath, "error", unmarshalErr)
			}
			existing = DayData{}
		}
	}
	maps.Copy(existing, dayData)

	b, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal archive %s: %w", dateKey, err)
	}

	// Write atomically via temporary file.
	tmp := destPath + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return fmt.Errorf("write temp archive %s: %w", dateKey, err)
	}
	return os.Rename(tmp, destPath)
}

// formatKeys converts a UTC time into standard date ("yyyy-MM-dd") and time ("HH:mm:ssZ") dictionary keys.
func formatKeys(ts time.Time) (dateKey, timeKey string) {
	utc := ts.UTC()
	dateKey = utc.Format("2006-01-02")
	timeKey = utc.Format("15:04:05") + "Z"
	return
}
