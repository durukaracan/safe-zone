package unit

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"thyris-sz/internal/analytics"
	"thyris-sz/internal/models"
)

// reportNow is 14:37 UTC on 2026-10-09: inside the 14:00 hour, and a Friday.
var reportNow = time.Date(2026, 10, 9, 14, 37, 0, 0, time.UTC)

func reportRow(hour time.Time, kind, name string, total, blocked int64) models.AnalyticsHourly {
	return models.AnalyticsHourly{BucketHour: hour, Kind: kind, Name: name, Total: total, Blocked: blocked}
}

func mustRange(t *testing.T, key string) analytics.Range {
	t.Helper()
	rng, ok := analytics.ParseRange(key)
	if !ok {
		t.Fatalf("range %q should be valid", key)
	}
	return rng
}

func TestParseRange_DefaultsAndRejects(t *testing.T) {
	if rng, ok := analytics.ParseRange(""); !ok || rng.Key != "7d" {
		t.Fatalf("an empty range should mean 7d, got %+v ok=%v", rng, ok)
	}
	if _, ok := analytics.ParseRange("1y"); ok {
		t.Fatal("an unknown range must be rejected")
	}
}

func TestParseRange_GroupsShortRangesByHourAndLongOnesByDay(t *testing.T) {
	cases := map[string]struct {
		bucket string
		points int
	}{
		"24h": {"hour", 24},
		"7d":  {"hour", 168},
		"30d": {"day", 30},
		"90d": {"day", 90},
	}
	for key, want := range cases {
		rng := mustRange(t, key)
		if rng.Bucket != want.bucket || rng.Points != want.points {
			t.Fatalf("%s: expected %s x %d, got %s x %d", key, want.bucket, want.points, rng.Bucket, rng.Points)
		}
	}
}

func TestBuildReport_FillsEmptyHoursWithZeros(t *testing.T) {
	rows := []models.AnalyticsHourly{
		reportRow(time.Date(2026, 10, 9, 13, 0, 0, 0, time.UTC), models.AnalyticsKindRequest, "", 5, 1),
		reportRow(time.Date(2026, 10, 9, 9, 0, 0, 0, time.UTC), models.AnalyticsKindRequest, "", 2, 0),
	}

	report := analytics.BuildReport(rows, mustRange(t, "24h"), reportNow)

	if len(report.Requests) != 24 {
		t.Fatalf("expected 24 points, got %d", len(report.Requests))
	}
	first, last := report.Requests[0], report.Requests[23]
	if want := time.Date(2026, 10, 8, 15, 0, 0, 0, time.UTC); !first.Time.Equal(want) {
		t.Fatalf("expected the first point at %v, got %v", want, first.Time)
	}
	if want := time.Date(2026, 10, 9, 14, 0, 0, 0, time.UTC); !last.Time.Equal(want) {
		t.Fatalf("expected the last point at the current hour %v, got %v", want, last.Time)
	}

	for i, p := range report.Requests {
		if i > 0 && p.Time.Sub(report.Requests[i-1].Time) != time.Hour {
			t.Fatalf("point %d is not one hour after the previous one", i)
		}
	}

	byHour := map[int]analytics.Point{}
	for _, p := range report.Requests {
		byHour[p.Time.Hour()] = p
	}
	if p := byHour[13]; p.Total != 5 || p.Blocked != 1 {
		t.Fatalf("13:00 should be total=5 blocked=1, got %+v", p)
	}
	if p := byHour[9]; p.Total != 2 || p.Blocked != 0 {
		t.Fatalf("09:00 should be total=2 blocked=0, got %+v", p)
	}
	if p := byHour[10]; p.Total != 0 || p.Blocked != 0 {
		t.Fatalf("10:00 had no row and should be zero, got %+v", p)
	}

	if report.Totals.Requests != 7 || report.Totals.Blocked != 1 || report.Totals.Allowed != 6 {
		t.Fatalf("unexpected totals: %+v", report.Totals)
	}
}

func TestBuildReport_GroupsLongRangesByUTCDay(t *testing.T) {
	rows := []models.AnalyticsHourly{
		reportRow(time.Date(2026, 10, 9, 1, 0, 0, 0, time.UTC), models.AnalyticsKindRequest, "", 4, 1),
		reportRow(time.Date(2026, 10, 9, 13, 0, 0, 0, time.UTC), models.AnalyticsKindRequest, "", 6, 2),
		reportRow(time.Date(2026, 10, 8, 23, 0, 0, 0, time.UTC), models.AnalyticsKindRequest, "", 3, 0),
	}

	report := analytics.BuildReport(rows, mustRange(t, "30d"), reportNow)

	if report.Bucket != "day" || len(report.Requests) != 30 {
		t.Fatalf("expected 30 daily points, got %s x %d", report.Bucket, len(report.Requests))
	}
	if want := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC); !report.Requests[0].Time.Equal(want) {
		t.Fatalf("expected the first day %v, got %v", want, report.Requests[0].Time)
	}

	today := report.Requests[29]
	if want := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC); !today.Time.Equal(want) {
		t.Fatalf("expected the last day %v, got %v", want, today.Time)
	}
	if today.Total != 10 || today.Blocked != 3 {
		t.Fatalf("today should add its two rows (total=10 blocked=3), got %+v", today)
	}
	// 23:00 UTC on the 8th belongs to the 8th, not to the 9th.
	if yesterday := report.Requests[28]; yesterday.Total != 3 {
		t.Fatalf("yesterday should be total=3, got %+v", yesterday)
	}
}

func TestBuildReport_IgnoresRowsOutsideTheWindow(t *testing.T) {
	rows := []models.AnalyticsHourly{
		// One hour before the 24h window starts (it starts at 15:00 yesterday).
		reportRow(time.Date(2026, 10, 8, 14, 0, 0, 0, time.UTC), models.AnalyticsKindRequest, "", 99, 99),
		// The next hour, which has not started yet.
		reportRow(time.Date(2026, 10, 9, 15, 0, 0, 0, time.UTC), models.AnalyticsKindRequest, "", 99, 99),
		reportRow(time.Date(2026, 10, 9, 14, 0, 0, 0, time.UTC), models.AnalyticsKindRequest, "", 1, 0),
	}

	report := analytics.BuildReport(rows, mustRange(t, "24h"), reportNow)

	if report.Totals.Requests != 1 {
		t.Fatalf("only the row inside the window counts, got totals %+v", report.Totals)
	}
}

func TestBuildReport_TopListsAreSortedAndCapped(t *testing.T) {
	hour := time.Date(2026, 10, 9, 14, 0, 0, 0, time.UTC)
	rows := []models.AnalyticsHourly{
		reportRow(hour, models.AnalyticsKindPII, "TCKN", 3, 0),
		reportRow(hour, models.AnalyticsKindPII, "EMAIL", 9, 0),
		reportRow(hour.Add(-time.Hour), models.AnalyticsKindPII, "EMAIL", 1, 0),
		// Same total as TCKN: ordered by name.
		reportRow(hour, models.AnalyticsKindPII, "IBAN", 3, 0),
	}
	for i := 0; i < 12; i++ {
		name := "GUARD_" + string(rune('A'+i))
		rows = append(rows, reportRow(hour, models.AnalyticsKindGuardrail, name, int64(i+1), 0))
	}

	report := analytics.BuildReport(rows, mustRange(t, "24h"), reportNow)

	if len(report.PII) != 3 {
		t.Fatalf("expected 3 PII entries, got %d", len(report.PII))
	}
	if got := report.PII[0]; got.Name != "EMAIL" || got.Total != 10 {
		t.Fatalf("EMAIL should lead with the rows of both hours added (10), got %+v", got)
	}
	if report.PII[1].Name != "IBAN" || report.PII[2].Name != "TCKN" {
		t.Fatalf("equal totals should be ordered by name, got %+v", report.PII)
	}

	if len(report.Guardrails) != analytics.TopN {
		t.Fatalf("expected the guardrail list to be cut to %d, got %d", analytics.TopN, len(report.Guardrails))
	}
	if report.Guardrails[0].Name != "GUARD_L" || report.Guardrails[0].Total != 12 {
		t.Fatalf("the biggest guardrail should come first, got %+v", report.Guardrails[0])
	}
}

func TestBuildReport_EmptyListsEncodeAsArrays(t *testing.T) {
	report := analytics.BuildReport(nil, mustRange(t, "24h"), reportNow)

	body, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{`"pii":[]`, `"guardrails":[]`} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("expected %s in %s", want, body)
		}
	}
	if len(report.Requests) != 24 {
		t.Fatalf("an empty database still gives a full zero-filled series, got %d points", len(report.Requests))
	}
}
