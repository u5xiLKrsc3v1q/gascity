package emergency

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gastownhall/gascity/internal/citylayout"
	"github.com/gastownhall/gascity/internal/events"
)

const (
	// StatusOpen identifies an unacked emergency record.
	StatusOpen = "open"
	// StatusAcked identifies an emergency record moved to processed/.
	StatusAcked = "acked"
)

var (
	// ErrInvalidID reports an emergency id that cannot map to a spool filename.
	ErrInvalidID = errors.New("invalid emergency id")
	// ErrNotFound reports a valid emergency id with no open or processed record.
	ErrNotFound = errors.New("emergency record not found")
)

// Entry is a spool record plus reader-side state.
type Entry struct {
	Record  Record
	Status  string
	Path    string
	AckedAt time.Time
	Size    int64
}

// ListOptions filters emergency spool entries.
type ListOptions struct {
	IncludeAcked   bool
	Since          time.Time
	Severities     []string
	ActorSubstring string
	Limit          int
	Now            time.Time
}

// ListResult contains filtered emergency entries and pre-limit summary data.
type ListResult struct {
	Entries        []Entry
	Total          int
	Open           int
	Acked          int
	Suppressed     int
	OldestOpen     time.Time
	TotalSizeBytes int64
}

// AckResult describes the outcome of acknowledging an emergency record.
type AckResult struct {
	Entry        Entry
	AlreadyAcked bool
}

// PruneOptions configures emergency spool pruning.
type PruneOptions struct {
	Now          time.Time
	ProcessedTTL time.Duration
	DedupeTTL    time.Duration
}

// PruneResult reports how many emergency spool artifacts were deleted.
type PruneResult struct {
	Processed     int
	DedupeMarkers int
}

// ListRecords returns emergency spool records matching opts.
func ListRecords(cityPath string, opts ListOptions) (ListResult, error) {
	if strings.TrimSpace(cityPath) == "" {
		return ListResult{}, fmt.Errorf("city path is required")
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}
	severities, err := severityFilter(opts.Severities)
	if err != nil {
		return ListResult{}, err
	}

	var all []Entry
	openEntries, err := readEntriesFromDir(SpoolDir(cityPath), StatusOpen)
	if err != nil {
		return ListResult{}, err
	}
	all = append(all, openEntries...)
	if opts.IncludeAcked {
		ackedEntries, err := readEntriesFromDir(filepath.Join(SpoolDir(cityPath), "processed"), StatusAcked)
		if err != nil {
			return ListResult{}, err
		}
		all = append(all, ackedEntries...)
	}

	actorNeedle := strings.ToLower(strings.TrimSpace(opts.ActorSubstring))
	filtered := make([]Entry, 0, len(all))
	var result ListResult
	for _, entry := range all {
		if !opts.Since.IsZero() && entry.Record.CreatedAt.Before(opts.Since) {
			continue
		}
		if len(severities) > 0 && !severities[entry.Record.Severity] {
			continue
		}
		if actorNeedle != "" && !strings.Contains(strings.ToLower(entry.Record.Actor), actorNeedle) {
			continue
		}
		result.Total++
		result.TotalSizeBytes += entry.Size
		switch entry.Status {
		case StatusOpen:
			result.Open++
			if result.OldestOpen.IsZero() || entry.Record.CreatedAt.Before(result.OldestOpen) {
				result.OldestOpen = entry.Record.CreatedAt
			}
		case StatusAcked:
			result.Acked++
		}
		filtered = append(filtered, entry)
	}
	sortEntries(filtered)
	if opts.Limit > 0 && len(filtered) > opts.Limit {
		result.Suppressed = len(filtered) - opts.Limit
		filtered = filtered[:opts.Limit]
	}
	result.Entries = filtered
	return result, nil
}

// ShowRecord returns one open or processed emergency record by id.
func ShowRecord(cityPath, id string) (Entry, error) {
	id = strings.TrimSpace(id)
	if !ValidRecordID(id) {
		return Entry{}, fmt.Errorf("%w: %q", ErrInvalidID, id)
	}
	openPath := filepath.Join(SpoolDir(cityPath), id+".json")
	entry, err := readEntryAtPath(openPath, StatusOpen)
	if err == nil {
		return entry, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return Entry{}, err
	}
	processedPath := filepath.Join(SpoolDir(cityPath), "processed", id+".json")
	entry, err = readEntryAtPath(processedPath, StatusAcked)
	if err == nil {
		return entry, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return Entry{}, fmt.Errorf("%w: %q", ErrNotFound, id)
	}
	return Entry{}, err
}

// AckRecord moves one open emergency record to processed/ idempotently.
func AckRecord(cityPath, id string, now time.Time) (AckResult, error) {
	id = strings.TrimSpace(id)
	if !ValidRecordID(id) {
		return AckResult{}, fmt.Errorf("%w: %q", ErrInvalidID, id)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}

	spoolDir := SpoolDir(cityPath)
	processedDir := filepath.Join(spoolDir, "processed")
	processedPath := filepath.Join(processedDir, id+".json")
	if entry, err := readEntryAtPath(processedPath, StatusAcked); err == nil {
		return AckResult{Entry: entry, AlreadyAcked: true}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return AckResult{}, err
	}

	openPath := filepath.Join(spoolDir, id+".json")
	openEntry, err := readEntryAtPath(openPath, StatusOpen)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if entry, processedErr := readEntryAtPath(processedPath, StatusAcked); processedErr == nil {
				return AckResult{Entry: entry, AlreadyAcked: true}, nil
			}
			return AckResult{}, fmt.Errorf("%w: %q", ErrNotFound, id)
		}
		return AckResult{}, err
	}

	if err := os.MkdirAll(processedDir, 0o700); err != nil {
		return AckResult{}, fmt.Errorf("creating processed emergency dir: %w", err)
	}
	if err := os.Chmod(processedDir, 0o700); err != nil {
		return AckResult{}, fmt.Errorf("setting processed emergency dir permissions: %w", err)
	}
	if err := os.Link(openPath, processedPath); err != nil {
		if os.IsExist(err) {
			entry, readErr := readEntryAtPath(processedPath, StatusAcked)
			if readErr != nil {
				return AckResult{}, readErr
			}
			_ = os.Remove(openPath)
			return AckResult{Entry: entry, AlreadyAcked: true}, nil
		}
		return AckResult{}, fmt.Errorf("moving emergency record to processed: %w", err)
	}
	if err := os.Remove(openPath); err != nil {
		return AckResult{}, fmt.Errorf("removing open emergency record after ack: %w", err)
	}
	_ = os.Chtimes(processedPath, now, now)
	openEntry.Status = StatusAcked
	openEntry.Path = processedPath
	openEntry.AckedAt = now
	return AckResult{Entry: openEntry}, nil
}

// RecordAckedBy records an emergency.acked event for rec using actor.
func RecordAckedBy(recorder events.Recorder, rec Record, actor string) error {
	return recordEmergencyEventWithActor(recorder, events.EmergencyAcked, rec, actor)
}

// RecordAckedToCityLog mirrors an emergency.acked event to city events.jsonl.
func RecordAckedToCityLog(cityPath string, rec Record, actor string, stderr io.Writer) error {
	eventPath := citylayout.RuntimePath(cityPath, "events.jsonl")
	provider, err := events.NewFileRecorder(eventPath, stderr)
	if err != nil {
		return fmt.Errorf("opening events.jsonl: %w", err)
	}
	defer provider.Close() //nolint:errcheck // best-effort close
	return RecordAckedBy(provider, rec, actor)
}

// RenderHookInjection renders unacked records as a startup hook reminder.
func RenderHookInjection(result ListResult, now time.Time) string {
	if result.Open == 0 {
		return ""
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var b strings.Builder
	oldest := result.OldestOpen
	if oldest.IsZero() {
		for _, entry := range result.Entries {
			if entry.Status == StatusOpen && (oldest.IsZero() || entry.Record.CreatedAt.Before(oldest)) {
				oldest = entry.Record.CreatedAt
			}
		}
	}
	fmt.Fprintln(&b, "<system-reminder>")
	fmt.Fprintf(&b, "You have %d unacked emergency signal(s) (oldest %s ago).\n", result.Open, FormatAge(now, oldest))
	if result.Suppressed > 0 {
		fmt.Fprintf(&b, "Showing the %d most recent; %d older entries suppressed.\n", len(result.Entries), result.Suppressed)
	}
	fmt.Fprintln(&b)
	for _, entry := range result.Entries {
		if entry.Status != StatusOpen {
			continue
		}
		rec := entry.Record
		fmt.Fprintf(&b, "  %s  [%s]  %s  %s\n", rec.ID, rec.Severity, FormatAge(now, rec.CreatedAt), rec.Actor)
		fmt.Fprintf(&b, "    %s\n", truncateForHook(rec.Message, 200))
		ref := strings.TrimSpace(rec.RefBead)
		if ref == "" {
			ref = "(none)"
		}
		fmt.Fprintf(&b, "    ref: %s\n\n", ref)
	}
	fmt.Fprintln(&b, "Run `gc emergency ack <id>` once handled. `gc emergency list` for full detail. Acked entries surface in `gc emergency list --all`.")
	fmt.Fprintln(&b, "</system-reminder>")
	return b.String()
}

// FormatAge returns a compact age string for emergency CLI surfaces.
func FormatAge(now, then time.Time) string {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if then.IsZero() {
		return "0m"
	}
	d := now.Sub(then)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d/time.Hour))
	default:
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	}
}

// Prune deletes expired processed records and notify-dedupe markers.
func Prune(cityPath string, opts PruneOptions) (PruneResult, error) {
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}
	if opts.ProcessedTTL <= 0 {
		opts.ProcessedTTL = 7 * 24 * time.Hour
	}
	if opts.DedupeTTL <= 0 {
		opts.DedupeTTL = 24 * time.Hour
	}
	var result PruneResult
	processedDir := filepath.Join(SpoolDir(cityPath), "processed")
	processedEntries, err := readEntriesFromDir(processedDir, StatusAcked)
	if err != nil {
		return result, err
	}
	for _, entry := range processedEntries {
		if opts.Now.Sub(entry.Record.CreatedAt) < opts.ProcessedTTL {
			continue
		}
		if err := os.Remove(entry.Path); err != nil && !os.IsNotExist(err) {
			return result, fmt.Errorf("pruning processed emergency %q: %w", entry.Record.ID, err)
		}
		result.Processed++
	}

	dedupeDir := filepath.Join(SpoolDir(cityPath), notifyDedupeDirName)
	markers, err := os.ReadDir(dedupeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return result, fmt.Errorf("reading notify dedupe dir: %w", err)
	}
	for _, marker := range markers {
		if marker.IsDir() {
			continue
		}
		path := filepath.Join(dedupeDir, marker.Name())
		info, err := safeLstat(path)
		if err != nil {
			return result, err
		}
		if opts.Now.Sub(info.ModTime()) < opts.DedupeTTL {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return result, fmt.Errorf("pruning notify dedupe marker %q: %w", marker.Name(), err)
		}
		result.DedupeMarkers++
	}
	return result, nil
}

func readEntriesFromDir(dir, status string) ([]Entry, error) {
	items, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading emergency dir %q: %w", dir, err)
	}
	entries := make([]Entry, 0, len(items))
	for _, item := range items {
		if item.IsDir() || !strings.HasSuffix(item.Name(), ".json") {
			continue
		}
		entry, err := readEntryAtPath(filepath.Join(dir, item.Name()), status)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func readEntryAtPath(path, status string) (Entry, error) {
	info, err := safeLstat(path)
	if err != nil {
		return Entry{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Entry{}, fmt.Errorf("reading emergency record %q: %w", path, err)
	}
	var rec Record
	if err := json.Unmarshal(data, &rec); err != nil {
		return Entry{}, fmt.Errorf("decoding emergency record %q: %w", path, err)
	}
	if !ValidRecordID(rec.ID) {
		return Entry{}, fmt.Errorf("emergency record %q has invalid id %q", path, rec.ID)
	}
	if filepath.Base(path) != rec.ID+".json" {
		return Entry{}, fmt.Errorf("emergency record filename %q does not match id %q", filepath.Base(path), rec.ID)
	}
	entry := Entry{
		Record: rec,
		Status: status,
		Path:   path,
		Size:   info.Size(),
	}
	if status == StatusAcked {
		entry.AckedAt = info.ModTime()
	}
	return entry, nil
}

func safeLstat(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("refusing emergency symlink %q", path)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("refusing non-regular emergency record %q", path)
	}
	return info, nil
}

func severityFilter(values []string) (map[string]bool, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := make(map[string]bool, len(values))
	for _, raw := range values {
		severity := strings.TrimSpace(raw)
		if !ValidSeverity(severity) {
			return nil, fmt.Errorf("severity must be one of info, warn, error, critical")
		}
		out[severity] = true
	}
	return out, nil
}

func sortEntries(entries []Entry) {
	sort.SliceStable(entries, func(i, j int) bool {
		left := entries[i].Record.CreatedAt
		right := entries[j].Record.CreatedAt
		if left.Equal(right) {
			return entries[i].Record.ID > entries[j].Record.ID
		}
		return left.After(right)
	})
}

func truncateForHook(message string, limit int) string {
	msg := strings.Join(strings.Fields(message), " ")
	if limit <= 0 || len(msg) <= limit {
		return msg
	}
	if limit <= 3 {
		return msg[:limit]
	}
	return msg[:limit-3] + "..."
}
