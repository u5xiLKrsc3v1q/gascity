package emergency

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/events"
)

func TestNewRecordBuildsStableIDAndFields(t *testing.T) {
	now := time.Date(2026, 4, 30, 16, 7, 1, 0, time.FixedZone("PDT", -7*60*60))
	rec, err := NewRecord(RecordOptions{
		Severity:   SeverityCritical,
		Actor:      "gascity/agent-a",
		Message:    "bd update failed",
		RefBead:    "ga-51t",
		SourcePath: "/city/rig",
		SourcePID:  1234,
		Hostname:   "host-a",
		Metadata:   map[string]string{"trigger": "dolt-down"},
		Now:        func() time.Time { return now },
		Random:     bytes.NewReader([]byte{0x7e, 0x3f, 0x9c, 0x12}),
	})
	if err != nil {
		t.Fatalf("NewRecord: %v", err)
	}

	if rec.ID != "20260430T230701Z-7e3f9c12" {
		t.Fatalf("ID = %q, want UTC compact timestamp plus random suffix", rec.ID)
	}
	if rec.CreatedAt.Location() != time.UTC || !rec.CreatedAt.Equal(now.UTC()) {
		t.Fatalf("CreatedAt = %v, want UTC %v", rec.CreatedAt, now.UTC())
	}
	if rec.Severity != SeverityCritical || rec.Actor != "gascity/agent-a" || rec.Message != "bd update failed" {
		t.Fatalf("record fields = %+v", rec)
	}
	if rec.RefBead != "ga-51t" || rec.SourcePath != "/city/rig" || rec.SourcePID != 1234 || rec.Hostname != "host-a" {
		t.Fatalf("record source fields = %+v", rec)
	}
	if rec.Metadata["trigger"] != "dolt-down" {
		t.Fatalf("metadata = %#v, want trigger=dolt-down", rec.Metadata)
	}
}

func TestNewRecordRejectsInvalidSeverityAndOversizeMessage(t *testing.T) {
	if _, err := NewRecord(RecordOptions{
		Severity: "fatal",
		Actor:    "human",
		Message:  "bad",
	}); err == nil || !strings.Contains(err.Error(), "severity") {
		t.Fatalf("invalid severity error = %v, want severity error", err)
	}

	if _, err := NewRecord(RecordOptions{
		Severity: SeverityError,
		Actor:    "human",
		Message:  strings.Repeat("x", MaxMessageBytes+1),
	}); err == nil || !strings.Contains(err.Error(), "4 KiB") {
		t.Fatalf("oversize message error = %v, want 4 KiB cap error", err)
	}
}

func TestWriteSpoolAtomicJSONPermissions(t *testing.T) {
	cityPath := t.TempDir()
	rec := Record{
		ID:        "20260430T160701Z-7e3f9c12",
		Severity:  SeverityError,
		Actor:     "human",
		Message:   "dolt down",
		CreatedAt: time.Date(2026, 4, 30, 16, 7, 1, 0, time.UTC),
	}

	path, err := WriteSpool(cityPath, rec)
	if err != nil {
		t.Fatalf("WriteSpool: %v", err)
	}
	wantPath := filepath.Join(cityPath, ".gc", "emergency", rec.ID+".json")
	if path != wantPath {
		t.Fatalf("path = %q, want %q", path, wantPath)
	}
	if fi, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatalf("stat spool dir: %v", err)
	} else if fi.Mode().Perm() != 0o700 {
		t.Fatalf("spool dir mode = %o, want 0700", fi.Mode().Perm())
	}
	if fi, err := os.Stat(path); err != nil {
		t.Fatalf("stat spool file: %v", err)
	} else if fi.Mode().Perm() != 0o600 {
		t.Fatalf("spool file mode = %o, want 0600", fi.Mode().Perm())
	}
	tmpMatches, err := filepath.Glob(filepath.Join(filepath.Dir(path), "*.tmp"))
	if err != nil {
		t.Fatalf("glob tmp files: %v", err)
	}
	if len(tmpMatches) != 0 {
		t.Fatalf("tmp files left behind: %v", tmpMatches)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read spool: %v", err)
	}
	var got Record
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal spool: %v", err)
	}
	if got.ID != rec.ID || got.Message != rec.Message {
		t.Fatalf("spool record = %+v, want %+v", got, rec)
	}
}

func TestWriteSpoolDoesNotOverwriteExistingRecord(t *testing.T) {
	cityPath := t.TempDir()
	rec := Record{
		ID:        "20260430T160701Z-7e3f9c12",
		Severity:  SeverityError,
		Actor:     "human",
		Message:   "original",
		CreatedAt: time.Date(2026, 4, 30, 16, 7, 1, 0, time.UTC),
	}
	path, err := WriteSpool(cityPath, rec)
	if err != nil {
		t.Fatalf("WriteSpool original: %v", err)
	}

	duplicate := rec
	duplicate.Message = "replacement"
	if _, err := WriteSpool(cityPath, duplicate); err == nil || !strings.Contains(err.Error(), "record already exists") {
		t.Fatalf("WriteSpool duplicate error = %v, want record already exists", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read original spool: %v", err)
	}
	if strings.Contains(string(data), "replacement") {
		t.Fatalf("duplicate write replaced existing spool record: %s", data)
	}
}

func TestRecordSignaledMirrorsTypedPayload(t *testing.T) {
	rec := Record{
		ID:         "20260430T160701Z-7e3f9c12",
		Severity:   SeverityCritical,
		Actor:      "gascity/agent-a",
		Message:    "bd update failed",
		RefBead:    "ga-51t",
		SourcePath: "/city/rig",
		SourcePID:  1234,
		Hostname:   "host-a",
		CreatedAt:  time.Date(2026, 4, 30, 16, 7, 1, 0, time.UTC),
		Metadata:   map[string]string{"trigger": "dolt-down"},
	}
	ep := events.NewFake()

	if err := RecordSignaled(ep, rec); err != nil {
		t.Fatalf("RecordSignaled: %v", err)
	}
	evts, err := ep.List(events.Filter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(evts) != 1 {
		t.Fatalf("events = %d, want 1", len(evts))
	}
	if evts[0].Type != events.EmergencySignaled || evts[0].Actor != rec.Actor || evts[0].Subject != rec.ID {
		t.Fatalf("event = %+v, want emergency.signaled from record actor and id", evts[0])
	}
	var payload Record
	if err := json.Unmarshal(evts[0].Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.ID != rec.ID || payload.Metadata["trigger"] != "dolt-down" {
		t.Fatalf("payload = %+v, want record payload", payload)
	}
}

func TestNotifyDeduperAllowsFirstAndSuppressesSecond(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 4, 30, 16, 7, 1, 0, time.UTC)
	key := NotifyDedupeKey(SeverityCritical, "dolt connection refused")

	first, err := MarkNotifyDedupe(dir, key, now, 5*time.Minute)
	if err != nil {
		t.Fatalf("first MarkNotifyDedupe: %v", err)
	}
	if !first.Fire {
		t.Fatalf("first dedupe result = %+v, want fire", first)
	}

	second, err := MarkNotifyDedupe(dir, key, now.Add(12*time.Second), 5*time.Minute)
	if err != nil {
		t.Fatalf("second MarkNotifyDedupe: %v", err)
	}
	if second.Fire {
		t.Fatalf("second dedupe result = %+v, want suppress", second)
	}
	if second.Age < 12*time.Second || second.KeyPrefix == "" {
		t.Fatalf("second dedupe details = %+v, want age and key prefix", second)
	}
}

func TestListRecordsFiltersSortsAndRendersHookInjection(t *testing.T) {
	cityPath := t.TempDir()
	now := time.Date(2026, 4, 30, 16, 19, 1, 0, time.UTC)
	older := emergencyTestRecord("20260430T160701Z-7e3f9c12", SeverityCritical, "rig/agent-a", "bd update failed", now.Add(-12*time.Minute))
	older.RefBead = "ga-51t"
	newer := emergencyTestRecord("20260430T161701Z-9aa0011b", SeverityError, "rig/agent-b", "mail send failed", now.Add(-2*time.Minute))
	processed := emergencyTestRecord("20260429T101512Z-aa3300dd", SeverityWarn, "rig/agent-a", "already handled", now.Add(-24*time.Hour))

	for _, rec := range []Record{older, newer, processed} {
		if _, err := WriteSpool(cityPath, rec); err != nil {
			t.Fatalf("WriteSpool(%s): %v", rec.ID, err)
		}
	}
	if _, err := AckRecord(cityPath, processed.ID, now.Add(-23*time.Hour)); err != nil {
		t.Fatalf("AckRecord processed fixture: %v", err)
	}

	result, err := ListRecords(cityPath, ListOptions{Now: now, Limit: 1})
	if err != nil {
		t.Fatalf("ListRecords: %v", err)
	}
	if result.Total != 2 || result.Open != 2 || result.Acked != 0 || result.Suppressed != 1 {
		t.Fatalf("summary = %+v, want total/open=2 acked=0 suppressed=1", result)
	}
	if len(result.Entries) != 1 || result.Entries[0].Record.ID != newer.ID || result.Entries[0].Status != StatusOpen {
		t.Fatalf("entries = %+v, want newest open entry only", result.Entries)
	}
	if result.OldestOpen.IsZero() || !result.OldestOpen.Equal(older.CreatedAt) {
		t.Fatalf("OldestOpen = %v, want %v", result.OldestOpen, older.CreatedAt)
	}

	hook := RenderHookInjection(result, now)
	for _, want := range []string{
		"<system-reminder>",
		"You have 2 unacked emergency signal(s) (oldest 12m ago).",
		"Showing the 1 most recent; 1 older entries suppressed.",
		newer.ID + "  [error]",
		"Run `gc emergency ack <id>` once handled.",
		"</system-reminder>",
	} {
		if !strings.Contains(hook, want) {
			t.Fatalf("hook output missing %q:\n%s", want, hook)
		}
	}
	if strings.Contains(hook, older.ID) {
		t.Fatalf("hook output included suppressed older entry:\n%s", hook)
	}

	filtered, err := ListRecords(cityPath, ListOptions{
		Now:            now,
		IncludeAcked:   true,
		Severities:     []string{SeverityWarn},
		ActorSubstring: "AGENT-A",
	})
	if err != nil {
		t.Fatalf("ListRecords filtered: %v", err)
	}
	if len(filtered.Entries) != 1 || filtered.Entries[0].Record.ID != processed.ID || filtered.Entries[0].Status != StatusAcked {
		t.Fatalf("filtered entries = %+v, want acked agent warning", filtered.Entries)
	}
}

func TestShowAndAckRecordSafetyAndIdempotence(t *testing.T) {
	cityPath := t.TempDir()
	now := time.Date(2026, 4, 30, 16, 19, 1, 0, time.UTC)
	rec := emergencyTestRecord("20260430T160701Z-7e3f9c12", SeverityCritical, "rig/agent-a", "dolt down", now.Add(-12*time.Minute))
	if _, err := WriteSpool(cityPath, rec); err != nil {
		t.Fatalf("WriteSpool: %v", err)
	}

	shown, err := ShowRecord(cityPath, rec.ID)
	if err != nil {
		t.Fatalf("ShowRecord open: %v", err)
	}
	if shown.Status != StatusOpen || shown.Record.ID != rec.ID {
		t.Fatalf("shown = %+v, want open record", shown)
	}

	acked, err := AckRecord(cityPath, rec.ID, now)
	if err != nil {
		t.Fatalf("AckRecord: %v", err)
	}
	if acked.AlreadyAcked || acked.Entry.Status != StatusAcked {
		t.Fatalf("ack result = %+v, want newly acked", acked)
	}
	if _, err := os.Stat(filepath.Join(SpoolDir(cityPath), rec.ID+".json")); !os.IsNotExist(err) {
		t.Fatalf("open spool path still exists or unexpected err: %v", err)
	}
	if _, err := os.Stat(filepath.Join(SpoolDir(cityPath), "processed", rec.ID+".json")); err != nil {
		t.Fatalf("processed spool path missing: %v", err)
	}

	reack, err := AckRecord(cityPath, rec.ID, now.Add(4*time.Minute))
	if err != nil {
		t.Fatalf("AckRecord re-ack: %v", err)
	}
	if !reack.AlreadyAcked || reack.Entry.Status != StatusAcked {
		t.Fatalf("reack result = %+v, want idempotent acked", reack)
	}

	for _, badID := range []string{"../../etc/passwd", "/tmp/x", ".."} {
		if _, err := AckRecord(cityPath, badID, now); !errors.Is(err, ErrInvalidID) {
			t.Fatalf("AckRecord(%q) err = %v, want ErrInvalidID", badID, err)
		}
		if _, err := ShowRecord(cityPath, badID); !errors.Is(err, ErrInvalidID) {
			t.Fatalf("ShowRecord(%q) err = %v, want ErrInvalidID", badID, err)
		}
	}
	if _, err := AckRecord(cityPath, "20260430T160701Z-aaaaaaaa", now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("AckRecord missing err = %v, want ErrNotFound", err)
	}

	symlinkID := "20260430T160701Z-bbbbbbbb"
	if err := os.Symlink("/etc/passwd", filepath.Join(SpoolDir(cityPath), symlinkID+".json")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	if _, err := ShowRecord(cityPath, symlinkID); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("ShowRecord symlink err = %v, want symlink refusal", err)
	}
}

func TestPruneProcessedRecordsAndNotifyDedupeMarkers(t *testing.T) {
	cityPath := t.TempDir()
	now := time.Date(2026, 4, 30, 16, 19, 1, 0, time.UTC)
	oldProcessed := emergencyTestRecord("20260420T160701Z-7e3f9c12", SeverityError, "rig/agent-a", "old", now.Add(-10*24*time.Hour))
	newProcessed := emergencyTestRecord("20260429T160701Z-9aa0011b", SeverityError, "rig/agent-a", "new", now.Add(-24*time.Hour))
	open := emergencyTestRecord("20260421T160701Z-c11a0033", SeverityCritical, "rig/agent-a", "still open", now.Add(-9*24*time.Hour))

	for _, rec := range []Record{oldProcessed, newProcessed, open} {
		if _, err := WriteSpool(cityPath, rec); err != nil {
			t.Fatalf("WriteSpool(%s): %v", rec.ID, err)
		}
	}
	if _, err := AckRecord(cityPath, oldProcessed.ID, now.Add(-9*24*time.Hour)); err != nil {
		t.Fatalf("Ack old: %v", err)
	}
	if _, err := AckRecord(cityPath, newProcessed.ID, now.Add(-23*time.Hour)); err != nil {
		t.Fatalf("Ack new: %v", err)
	}

	dedupeDir := filepath.Join(SpoolDir(cityPath), notifyDedupeDirName)
	if err := os.MkdirAll(dedupeDir, 0o700); err != nil {
		t.Fatalf("MkdirAll dedupe: %v", err)
	}
	oldMarker := filepath.Join(dedupeDir, "critical-old")
	newMarker := filepath.Join(dedupeDir, "critical-new")
	for _, marker := range []string{oldMarker, newMarker} {
		if err := os.WriteFile(marker, nil, 0o600); err != nil {
			t.Fatalf("WriteFile marker: %v", err)
		}
	}
	if err := os.Chtimes(oldMarker, now.Add(-48*time.Hour), now.Add(-48*time.Hour)); err != nil {
		t.Fatalf("Chtimes old marker: %v", err)
	}
	if err := os.Chtimes(newMarker, now.Add(-2*time.Hour), now.Add(-2*time.Hour)); err != nil {
		t.Fatalf("Chtimes new marker: %v", err)
	}

	pruned, err := Prune(cityPath, PruneOptions{
		Now:          now,
		ProcessedTTL: 7 * 24 * time.Hour,
		DedupeTTL:    24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if pruned.Processed != 1 || pruned.DedupeMarkers != 1 {
		t.Fatalf("pruned = %+v, want one processed and one marker", pruned)
	}
	if _, err := os.Stat(filepath.Join(SpoolDir(cityPath), "processed", oldProcessed.ID+".json")); !os.IsNotExist(err) {
		t.Fatalf("old processed still exists or unexpected err: %v", err)
	}
	for _, path := range []string{
		filepath.Join(SpoolDir(cityPath), "processed", newProcessed.ID+".json"),
		filepath.Join(SpoolDir(cityPath), open.ID+".json"),
		newMarker,
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to remain: %v", path, err)
		}
	}
	if _, err := os.Stat(oldMarker); !os.IsNotExist(err) {
		t.Fatalf("old marker still exists or unexpected err: %v", err)
	}
}

func emergencyTestRecord(id, severity, actor, message string, createdAt time.Time) Record {
	return Record{
		ID:        id,
		Severity:  severity,
		Actor:     actor,
		Message:   message,
		CreatedAt: createdAt.UTC(),
	}
}
