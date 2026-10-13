package analytics

import (
	"sort"
	"time"

	"thyris-sz/internal/models"
)

// How the points of a range are grouped.
const (
	BucketHour = "hour"
	BucketDay  = "day" // a day starts at 00:00 UTC
)

// DefaultRange is used when the request names no range.
const DefaultRange = "7d"

// TopN is how many entries the "PII by category" and "most triggered
// guardrails" lists contain at most.
const TopN = 10

// Range describes one of the time windows the dashboard can ask for.
// Short ranges are grouped by hour and long ranges by day, so every range
// has between 24 and 168 points: enough to see a trend, few enough to read.
type Range struct {
	Key    string
	Bucket string
	Points int
}

var ranges = map[string]Range{
	"24h": {Key: "24h", Bucket: BucketHour, Points: 24},
	"7d":  {Key: "7d", Bucket: BucketHour, Points: 168},
	"30d": {Key: "30d", Bucket: BucketDay, Points: 30},
	"90d": {Key: "90d", Bucket: BucketDay, Points: 90},
}

// ParseRange turns the "range" query value into a Range. An empty value
// means DefaultRange. The second result is false for an unknown value.
func ParseRange(key string) (Range, bool) {
	if key == "" {
		key = DefaultRange
	}
	r, ok := ranges[key]
	return r, ok
}

func (r Range) step() time.Duration {
	if r.Bucket == BucketDay {
		return 24 * time.Hour
	}
	return time.Hour
}

// Start is the beginning of the first point of the window that ends with
// the point containing now. The last point is the current, unfinished hour
// or day.
func (r Range) Start(now time.Time) time.Time {
	step := r.step()
	current := now.UTC().Truncate(step)
	return current.Add(-time.Duration(r.Points-1) * step)
}

// Point is one value of the requests-over-time series.
type Point struct {
	Time    time.Time `json:"time"`
	Total   int64     `json:"total"`
	Blocked int64     `json:"blocked"`
}

// NamedCount is one entry of a "top" list.
type NamedCount struct {
	Name  string `json:"name"`
	Total int64  `json:"total"`
}

// Totals are the sums over the whole window.
type Totals struct {
	Requests int64 `json:"requests"`
	Blocked  int64 `json:"blocked"`
	Allowed  int64 `json:"allowed"`
}

// Report is the answer of GET /dashboard/analytics. It holds only counts and
// names, never request text, request ids or IP addresses.
type Report struct {
	Range      string       `json:"range"`
	Bucket     string       `json:"bucket"`
	From       time.Time    `json:"from"`
	To         time.Time    `json:"to"`
	Totals     Totals       `json:"totals"`
	Requests   []Point      `json:"requests"`
	PII        []NamedCount `json:"pii"`
	Guardrails []NamedCount `json:"guardrails"`
}

// BuildReport groups hourly rows into the points of rng and fills the hours
// or days without any row with zeros, so the time axis has no gaps. Rows
// outside the window are ignored. The lists in the result are never nil, so
// they are always encoded as JSON arrays.
func BuildReport(rows []models.AnalyticsHourly, rng Range, now time.Time) Report {
	step := rng.step()
	start := rng.Start(now)
	end := start.Add(time.Duration(rng.Points) * step) // exclusive

	points := make([]Point, rng.Points)
	for i := range points {
		points[i].Time = start.Add(time.Duration(i) * step)
	}

	pii := make(map[string]int64)
	guardrails := make(map[string]int64)
	var totals Totals

	for _, row := range rows {
		hour := row.BucketHour.UTC()
		if hour.Before(start) || !hour.Before(end) {
			continue
		}

		switch row.Kind {
		case models.AnalyticsKindRequest:
			idx := int(hour.Truncate(step).Sub(start) / step)
			points[idx].Total += row.Total
			points[idx].Blocked += row.Blocked
			totals.Requests += row.Total
			totals.Blocked += row.Blocked
		case models.AnalyticsKindPII:
			pii[row.Name] += row.Total
		case models.AnalyticsKindGuardrail:
			guardrails[row.Name] += row.Total
		}
	}
	totals.Allowed = totals.Requests - totals.Blocked

	return Report{
		Range:      rng.Key,
		Bucket:     rng.Bucket,
		From:       start,
		To:         end,
		Totals:     totals,
		Requests:   points,
		PII:        topN(pii),
		Guardrails: topN(guardrails),
	}
}

// topN returns the TopN largest entries, biggest first. Equal totals are
// ordered by name so the answer is the same on every call.
func topN(counts map[string]int64) []NamedCount {
	list := make([]NamedCount, 0, len(counts))
	for name, total := range counts {
		list = append(list, NamedCount{Name: name, Total: total})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Total != list[j].Total {
			return list[i].Total > list[j].Total
		}
		return list[i].Name < list[j].Name
	})
	if len(list) > TopN {
		list = list[:TopN]
	}
	return list
}
