package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/emergency"
	"github.com/gastownhall/gascity/internal/events"
)

func TestEmergencyReaderCLIListShowAck(t *testing.T) {
	clearGCEnv(t)
	clearInheritedCityRoutingEnv(t)
	configureIsolatedRuntimeEnv(t)
	t.Setenv("GC_ALIAS", "rig/operator")

	cityDir := t.TempDir()
	writeEmergencyReaderCity(t, cityDir)
	now := time.Date(2026, 4, 30, 16, 19, 1, 0, time.UTC)
	older := emergencyReaderTestRecord("20260430T160701Z-7e3f9c12", emergency.SeverityCritical, "rig/agent-a", "bd update failed", now.Add(-12*time.Minute))
	older.RefBead = "ga-51t"
	newer := emergencyReaderTestRecord("20260430T161701Z-9aa0011b", emergency.SeverityError, "rig/agent-b", "mail send failed", now.Add(-2*time.Minute))
	for _, rec := range []emergency.Record{older, newer} {
		if _, err := emergency.WriteSpool(cityDir, rec); err != nil {
			t.Fatalf("WriteSpool(%s): %v", rec.ID, err)
		}
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"--city", cityDir,
		"emergency", "list",
		"--format", "hook-injection",
		"--limit", "1",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gc emergency list = %d; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "<system-reminder>") || !strings.Contains(stdout.String(), newer.ID) {
		t.Fatalf("hook output missing reminder/newer id:\nstdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "1 older entries suppressed") || strings.Contains(stdout.String(), older.ID) {
		t.Fatalf("hook output did not suppress older entry correctly:\n%s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"--city", cityDir, "emergency", "show", older.ID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gc emergency show = %d; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	var shown emergency.Record
	if err := json.Unmarshal(stdout.Bytes(), &shown); err != nil {
		t.Fatalf("show output is not record JSON: %v\n%s", err, stdout.String())
	}
	if shown.ID != older.ID || shown.RefBead != "ga-51t" {
		t.Fatalf("shown = %+v, want older record", shown)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"--city", cityDir, "emergency", "ack", older.ID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gc emergency ack = %d; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if strings.TrimSpace(stdout.String()) != "acked: "+older.ID {
		t.Fatalf("ack stdout = %q", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(emergency.SpoolDir(cityDir), "processed", older.ID+".json")); err != nil {
		t.Fatalf("acked record not moved to processed: %v", err)
	}
	eventData, err := os.ReadFile(filepath.Join(cityDir, ".gc", "events.jsonl"))
	if err != nil {
		t.Fatalf("read events.jsonl: %v", err)
	}
	var ackEvent events.Event
	lines := strings.Split(strings.TrimSpace(string(eventData)), "\n")
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &ackEvent); err != nil {
		t.Fatalf("unmarshal ack event: %v", err)
	}
	if ackEvent.Type != events.EmergencyAcked || ackEvent.Actor != "rig/operator" || ackEvent.Subject != older.ID {
		t.Fatalf("ack event = %+v, want emergency.acked from operator", ackEvent)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"--city", cityDir, "emergency", "ack", older.ID}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gc emergency re-ack = %d; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "already acked: "+older.ID) {
		t.Fatalf("re-ack stdout = %q, want already acked", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"--city", cityDir, "emergency", "list", "--severity", "critical"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("gc emergency list after ack = %d; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), older.ID) {
		t.Fatalf("acked record appeared in default list:\n%s", stdout.String())
	}
}

func TestEmergencyReaderCLIRejectsTraversal(t *testing.T) {
	clearGCEnv(t)
	clearInheritedCityRoutingEnv(t)
	configureIsolatedRuntimeEnv(t)

	cityDir := t.TempDir()
	writeEmergencyReaderCity(t, cityDir)

	for _, args := range [][]string{
		{"--city", cityDir, "emergency", "ack", "../../etc/passwd"},
		{"--city", cityDir, "emergency", "show", "/tmp/x"},
	} {
		var stdout, stderr bytes.Buffer
		code := run(args, &stdout, &stderr)
		if code != 2 {
			t.Fatalf("run(%v) code = %d, want 2; stdout=%q stderr=%q", args, code, stdout.String(), stderr.String())
		}
		if !strings.Contains(stderr.String(), "invalid id format") {
			t.Fatalf("run(%v) stderr = %q, want invalid id", args, stderr.String())
		}
	}
}

func writeEmergencyReaderCity(t *testing.T, cityDir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(cityDir, "city.toml"), []byte("[city]\nname = \"test\"\n"), 0o644); err != nil {
		t.Fatalf("write city.toml: %v", err)
	}
	if err := os.Mkdir(filepath.Join(cityDir, ".gc"), 0o700); err != nil {
		t.Fatalf("mkdir .gc: %v", err)
	}
}

func emergencyReaderTestRecord(id, severity, actor, message string, createdAt time.Time) emergency.Record {
	return emergency.Record{
		ID:        id,
		Severity:  severity,
		Actor:     actor,
		Message:   message,
		CreatedAt: createdAt.UTC(),
	}
}
