package unit

import (
	"strings"
	"testing"
	"time"

	"thyris-sz/internal/metrics"
	"thyris-sz/internal/models"
)

func analyticsKey(hour time.Time, kind, name string) metrics.AnalyticsKey {
	return metrics.AnalyticsKey{BucketHour: hour, Kind: kind, Name: name}
}

func TestRecordAnalytics_RoundsToTheHourInUTC(t *testing.T) {
	metrics.TakeAnalytics()

	// 17:37 in UTC+3 is 14:37 UTC, so it must land in the 14:00 UTC bucket.
	istanbul := time.FixedZone("UTC+3", 3*60*60)
	metrics.RecordAnalytics(metrics.AnalyticsEvent{
		Time: time.Date(2026, 10, 9, 17, 37, 12, 0, istanbul),
	})

	got := metrics.TakeAnalytics()
	want := analyticsKey(time.Date(2026, 10, 9, 14, 0, 0, 0, time.UTC), models.AnalyticsKindRequest, "")
	if _, ok := got[want]; !ok {
		t.Fatalf("expected a REQUEST counter for 14:00 UTC, got %v", got)
	}
	if len(got) != 1 {
		t.Fatalf("expected exactly one counter, got %d", len(got))
	}
}

func TestRecordAnalytics_CountsAllowedAndBlockedRequests(t *testing.T) {
	metrics.TakeAnalytics()

	at := time.Date(2026, 10, 9, 14, 5, 0, 0, time.UTC)
	metrics.RecordAnalytics(metrics.AnalyticsEvent{Time: at, Blocked: false})
	metrics.RecordAnalytics(metrics.AnalyticsEvent{Time: at.Add(10 * time.Minute), Blocked: true})
	metrics.RecordAnalytics(metrics.AnalyticsEvent{Time: at.Add(20 * time.Minute), Blocked: false})

	hour := time.Date(2026, 10, 9, 14, 0, 0, 0, time.UTC)
	c := metrics.TakeAnalytics()[analyticsKey(hour, models.AnalyticsKindRequest, "")]
	if c.Total != 3 || c.Blocked != 1 {
		t.Fatalf("expected total=3 blocked=1, got total=%d blocked=%d", c.Total, c.Blocked)
	}
}

func TestRecordAnalytics_SeparatesHours(t *testing.T) {
	metrics.TakeAnalytics()

	metrics.RecordAnalytics(metrics.AnalyticsEvent{Time: time.Date(2026, 10, 9, 14, 59, 0, 0, time.UTC)})
	metrics.RecordAnalytics(metrics.AnalyticsEvent{Time: time.Date(2026, 10, 9, 15, 0, 0, 0, time.UTC)})

	got := metrics.TakeAnalytics()
	if len(got) != 2 {
		t.Fatalf("expected two hourly counters, got %d", len(got))
	}
}

func TestRecordAnalytics_CountsPIITypes(t *testing.T) {
	metrics.TakeAnalytics()

	at := time.Date(2026, 10, 9, 14, 0, 0, 0, time.UTC)
	metrics.RecordAnalytics(metrics.AnalyticsEvent{
		Time:      at,
		Blocked:   true,
		PIICounts: map[string]int{"EMAIL": 2, "TCKN": 1, "PHONE": 0},
	})
	metrics.RecordAnalytics(metrics.AnalyticsEvent{
		Time:      at,
		Blocked:   false,
		PIICounts: map[string]int{"EMAIL": 3},
	})

	got := metrics.TakeAnalytics()
	email := got[analyticsKey(at, models.AnalyticsKindPII, "EMAIL")]
	if email.Total != 5 || email.Blocked != 2 {
		t.Fatalf("EMAIL: expected total=5 blocked=2, got total=%d blocked=%d", email.Total, email.Blocked)
	}
	if tckn := got[analyticsKey(at, models.AnalyticsKindPII, "TCKN")]; tckn.Total != 1 {
		t.Fatalf("TCKN: expected total=1, got %d", tckn.Total)
	}
	if _, found := got[analyticsKey(at, models.AnalyticsKindPII, "PHONE")]; found {
		t.Fatal("a PII type with zero detections must not create a counter")
	}
}

func TestRecordAnalytics_CountsFailedGuardrails(t *testing.T) {
	metrics.TakeAnalytics()

	at := time.Date(2026, 10, 9, 14, 0, 0, 0, time.UTC)
	metrics.RecordAnalytics(metrics.AnalyticsEvent{
		Time:             at,
		Blocked:          true,
		FailedGuardrails: []string{"PROMPT_INJECTION", "NUMERIC"},
	})
	metrics.RecordAnalytics(metrics.AnalyticsEvent{
		Time:             at,
		Blocked:          true,
		FailedGuardrails: []string{"PROMPT_INJECTION"},
	})

	got := metrics.TakeAnalytics()
	if c := got[analyticsKey(at, models.AnalyticsKindGuardrail, "PROMPT_INJECTION")]; c.Total != 2 || c.Blocked != 2 {
		t.Fatalf("PROMPT_INJECTION: expected total=2 blocked=2, got %+v", c)
	}
	if c := got[analyticsKey(at, models.AnalyticsKindGuardrail, "NUMERIC")]; c.Total != 1 {
		t.Fatalf("NUMERIC: expected total=1, got %+v", c)
	}
}

func TestRecordAnalytics_CutsVeryLongNames(t *testing.T) {
	metrics.TakeAnalytics()

	at := time.Date(2026, 10, 9, 14, 0, 0, 0, time.UTC)
	// With a one-byte prefix, every "ş" (two bytes) starts at an odd index,
	// so byte 100 is the middle of a character. The cut must step back.
	longName := "a" + strings.Repeat("ş", 80)
	metrics.RecordAnalytics(metrics.AnalyticsEvent{
		Time:      at,
		PIICounts: map[string]int{longName: 1},
	})

	for k := range metrics.TakeAnalytics() {
		if k.Kind != models.AnalyticsKindPII {
			continue
		}
		if len(k.Name) > 100 {
			t.Fatalf("name was not cut: %d bytes", len(k.Name))
		}
		if strings.ContainsRune(k.Name, '�') {
			t.Fatal("name was cut in the middle of a character")
		}
	}
}

func TestTakeAnalytics_EmptiesTheBuffer(t *testing.T) {
	metrics.TakeAnalytics()

	metrics.RecordAnalytics(metrics.AnalyticsEvent{Time: time.Now()})

	if first := metrics.TakeAnalytics(); len(first) == 0 {
		t.Fatal("expected counters on the first take")
	}
	if second := metrics.TakeAnalytics(); len(second) != 0 {
		t.Fatalf("expected an empty buffer on the second take, got %d counters", len(second))
	}
}
