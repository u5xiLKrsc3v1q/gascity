package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/doctor"
	"github.com/gastownhall/gascity/internal/emergency"
)

func TestEmergencySpoolDoctorCheckWarnsAndFixPrunes(t *testing.T) {
	cityPath := t.TempDir()
	now := time.Date(2026, 4, 30, 16, 19, 1, 0, time.UTC)
	open := emergencyDoctorTestRecord("20260430T160701Z-7e3f9c12", emergency.SeverityCritical, "rig/agent-a", "still open", now.Add(-12*time.Minute))
	oldProcessed := emergencyDoctorTestRecord("20260420T160701Z-9aa0011b", emergency.SeverityError, "rig/agent-a", "old", now.Add(-10*24*time.Hour))
	newProcessed := emergencyDoctorTestRecord("20260429T160701Z-c11a0033", emergency.SeverityWarn, "rig/agent-a", "new", now.Add(-24*time.Hour))
	for _, rec := range []emergency.Record{open, oldProcessed, newProcessed} {
		if _, err := emergency.WriteSpool(cityPath, rec); err != nil {
			t.Fatalf("WriteSpool(%s): %v", rec.ID, err)
		}
	}
	if _, err := emergency.AckRecord(cityPath, oldProcessed.ID, now.Add(-9*24*time.Hour)); err != nil {
		t.Fatalf("Ack old: %v", err)
	}
	if _, err := emergency.AckRecord(cityPath, newProcessed.ID, now.Add(-23*time.Hour)); err != nil {
		t.Fatalf("Ack new: %v", err)
	}
	dedupeDir := filepath.Join(emergency.SpoolDir(cityPath), ".notify-dedupe")
	if err := os.MkdirAll(dedupeDir, 0o700); err != nil {
		t.Fatalf("MkdirAll dedupe: %v", err)
	}
	oldMarker := filepath.Join(dedupeDir, "critical-old")
	if err := os.WriteFile(oldMarker, nil, 0o600); err != nil {
		t.Fatalf("WriteFile marker: %v", err)
	}
	if err := os.Chtimes(oldMarker, now.Add(-48*time.Hour), now.Add(-48*time.Hour)); err != nil {
		t.Fatalf("Chtimes marker: %v", err)
	}

	check := newEmergencySpoolCheck(cityPath, 7*24*time.Hour, func() time.Time { return now })
	result := check.Run(&doctor.CheckContext{CityPath: cityPath})
	if result.Status != doctor.StatusWarning {
		t.Fatalf("status = %v, want warning; result=%+v", result.Status, result)
	}
	for _, want := range []string{"1 unacked entries", "oldest 12m", "2 processed"} {
		if !strings.Contains(result.Message, want) {
			t.Fatalf("message = %q, want %q", result.Message, want)
		}
	}
	if !check.CanFix() {
		t.Fatal("emergency spool check should be fixable")
	}
	if err := check.Fix(&doctor.CheckContext{CityPath: cityPath}); err != nil {
		t.Fatalf("Fix: %v", err)
	}
	if _, err := os.Stat(filepath.Join(emergency.SpoolDir(cityPath), "processed", oldProcessed.ID+".json")); !os.IsNotExist(err) {
		t.Fatalf("old processed record still exists or unexpected err: %v", err)
	}
	if _, err := os.Stat(oldMarker); !os.IsNotExist(err) {
		t.Fatalf("old dedupe marker still exists or unexpected err: %v", err)
	}
	if _, err := os.Stat(filepath.Join(emergency.SpoolDir(cityPath), "processed", newProcessed.ID+".json")); err != nil {
		t.Fatalf("new processed record should remain: %v", err)
	}
	if _, err := os.Stat(filepath.Join(emergency.SpoolDir(cityPath), open.ID+".json")); err != nil {
		t.Fatalf("open record should remain: %v", err)
	}
}

func emergencyDoctorTestRecord(id, severity, actor, message string, createdAt time.Time) emergency.Record {
	return emergency.Record{
		ID:        id,
		Severity:  severity,
		Actor:     actor,
		Message:   message,
		CreatedAt: createdAt.UTC(),
	}
}
