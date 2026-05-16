package main

import (
	"fmt"
	"time"

	"github.com/gastownhall/gascity/internal/doctor"
	"github.com/gastownhall/gascity/internal/emergency"
)

const (
	defaultEmergencyProcessedTTL = 7 * 24 * time.Hour
	defaultEmergencyDedupeTTL    = 24 * time.Hour
)

type emergencySpoolCheck struct {
	cityPath     string
	processedTTL time.Duration
	now          func() time.Time
}

func newEmergencySpoolCheck(cityPath string, processedTTL time.Duration, now func() time.Time) *emergencySpoolCheck {
	if processedTTL <= 0 {
		processedTTL = defaultEmergencyProcessedTTL
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &emergencySpoolCheck{cityPath: cityPath, processedTTL: processedTTL, now: now}
}

func (c *emergencySpoolCheck) Name() string { return "emergency-spool" }

func (c *emergencySpoolCheck) Run(_ *doctor.CheckContext) *doctor.CheckResult {
	now := c.now().UTC()
	result := &doctor.CheckResult{Name: c.Name()}
	list, err := emergency.ListRecords(c.cityPath, emergency.ListOptions{
		IncludeAcked: true,
		Now:          now,
	})
	if err != nil {
		result.Status = doctor.StatusError
		result.Message = fmt.Sprintf("reading emergency spool: %v", err)
		return result
	}
	processedOldest := oldestProcessed(list)
	processedSummary := fmt.Sprintf("%d processed", list.Acked)
	if !processedOldest.IsZero() {
		processedSummary = fmt.Sprintf("%d processed (oldest %s)", list.Acked, emergency.FormatAge(now, processedOldest))
	}
	expiredProcessed := expiredProcessedCount(list, now, c.processedTTL)
	if list.Open > 0 || expiredProcessed > 0 {
		result.Status = doctor.StatusWarning
		if list.Open > 0 {
			result.Message = fmt.Sprintf("%d unacked entries (oldest %s); %s; spool %d bytes",
				list.Open,
				emergency.FormatAge(now, list.OldestOpen),
				processedSummary,
				list.TotalSizeBytes,
			)
		} else {
			result.Message = fmt.Sprintf("0 unacked entries; %s; %d expired processed entries eligible for pruning; spool %d bytes",
				processedSummary,
				expiredProcessed,
				list.TotalSizeBytes,
			)
		}
		result.FixHint = "run `gc emergency list` and `gc emergency ack <id>` for handled entries; `gc doctor --fix` prunes expired processed entries"
		return result
	}
	result.Status = doctor.StatusOK
	result.Message = fmt.Sprintf("0 unacked entries; %s; spool %d bytes", processedSummary, list.TotalSizeBytes)
	return result
}

func (c *emergencySpoolCheck) CanFix() bool { return true }

func (c *emergencySpoolCheck) Fix(_ *doctor.CheckContext) error {
	_, err := emergency.Prune(c.cityPath, emergency.PruneOptions{
		Now:          c.now().UTC(),
		ProcessedTTL: c.processedTTL,
		DedupeTTL:    defaultEmergencyDedupeTTL,
	})
	return err
}

func oldestProcessed(result emergency.ListResult) time.Time {
	var oldest time.Time
	for _, entry := range result.Entries {
		if entry.Status != emergency.StatusAcked {
			continue
		}
		if oldest.IsZero() || entry.Record.CreatedAt.Before(oldest) {
			oldest = entry.Record.CreatedAt
		}
	}
	return oldest
}

func expiredProcessedCount(result emergency.ListResult, now time.Time, ttl time.Duration) int {
	if ttl <= 0 {
		return 0
	}
	var count int
	for _, entry := range result.Entries {
		if entry.Status != emergency.StatusAcked {
			continue
		}
		if now.Sub(entry.Record.CreatedAt) >= ttl {
			count++
		}
	}
	return count
}
