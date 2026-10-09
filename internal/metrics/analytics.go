package metrics

import (
	"sync"
	"time"
	"unicode/utf8"

	"thyris-sz/internal/models"
)

// maxAnalyticsNameLength matches the size of the analytics_hourly.name
// column. A longer name would make the whole database write fail, so names
// are cut to this length before they are counted.
const maxAnalyticsNameLength = 100

// AnalyticsKey identifies one counter: one hour, one kind of thing, one name.
// It mirrors the primary key of the analytics_hourly table.
type AnalyticsKey struct {
	BucketHour time.Time
	Kind       string
	Name       string
}

// AnalyticsCount is the value kept for one AnalyticsKey.
type AnalyticsCount struct {
	Total   int64
	Blocked int64
}

// AnalyticsEvent is what the detection code reports for one finished
// request. It carries only counts and names, never request text, request
// ids or IP addresses.
type AnalyticsEvent struct {
	Time    time.Time
	Blocked bool
	// PIICounts is the number of detections per PII type, for example
	// {"EMAIL": 2}.
	PIICounts map[string]int
	// FailedGuardrails lists guardrails that rejected the request. The
	// caller must pass only names that exist in the guardrails table, so a
	// client cannot grow the counters by sending made-up names.
	FailedGuardrails []string
}

// analyticsBuffer holds counters that have not been written to the database
// yet. It has its own lock so recording analytics never waits for the
// dashboard summary lock.
var analyticsBuffer = struct {
	sync.Mutex
	counts map[AnalyticsKey]AnalyticsCount
}{counts: make(map[AnalyticsKey]AnalyticsCount)}

// RecordAnalytics adds one finished request to the in-memory hourly
// counters. It does no I/O, so it is cheap enough for the /detect path.
func RecordAnalytics(e AnalyticsEvent) {
	hour := e.Time.UTC().Truncate(time.Hour)

	analyticsBuffer.Lock()
	defer analyticsBuffer.Unlock()

	addCount(AnalyticsKey{hour, models.AnalyticsKindRequest, ""}, 1, e.Blocked)

	for piiType, n := range e.PIICounts {
		if n <= 0 {
			continue
		}
		addCount(AnalyticsKey{hour, models.AnalyticsKindPII, clipName(piiType)}, int64(n), e.Blocked)
	}

	for _, name := range e.FailedGuardrails {
		addCount(AnalyticsKey{hour, models.AnalyticsKindGuardrail, clipName(name)}, 1, e.Blocked)
	}
}

// TakeAnalytics returns every counter collected so far and empties the
// buffer. The caller owns the returned map. Taking everything in one short
// locked step keeps /detect from waiting while the caller writes to the
// database.
func TakeAnalytics() map[AnalyticsKey]AnalyticsCount {
	analyticsBuffer.Lock()
	defer analyticsBuffer.Unlock()

	taken := analyticsBuffer.counts
	analyticsBuffer.counts = make(map[AnalyticsKey]AnalyticsCount)
	return taken
}

// addCount must be called with analyticsBuffer locked.
func addCount(key AnalyticsKey, n int64, blocked bool) {
	c := analyticsBuffer.counts[key]
	c.Total += n
	if blocked {
		c.Blocked += n
	}
	analyticsBuffer.counts[key] = c
}

// clipName cuts a name to maxAnalyticsNameLength bytes without splitting a
// multi-byte character.
func clipName(name string) string {
	if len(name) <= maxAnalyticsNameLength {
		return name
	}
	cut := maxAnalyticsNameLength
	for cut > 0 && !utf8.RuneStart(name[cut]) {
		cut--
	}
	return name[:cut]
}
