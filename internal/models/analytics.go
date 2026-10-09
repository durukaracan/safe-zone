package models

import "time"

// Kinds of rows in analytics_hourly. Kept as constants so a typo can never
// create an inconsistent value in the table.
const (
	// AnalyticsKindRequest counts every /detect request. Name is empty.
	AnalyticsKindRequest = "REQUEST"

	// AnalyticsKindPII counts detections per PII type. Name is the type,
	// for example "EMAIL".
	AnalyticsKindPII = "PII"

	// AnalyticsKindGuardrail counts how often a guardrail failed. Name is
	// the guardrail name.
	AnalyticsKindGuardrail = "GUARDRAIL"
)

// AnalyticsHourly is one counter for one hour. It is a summary, not a log:
// it holds only counts and names, never request text, request ids or IP
// addresses.
//
// The three key columns together are the primary key, so there is exactly
// one row per (hour, kind, name). New events add to the existing row
// instead of creating a new one. Because bucket_hour is the first key
// column, range queries such as "the last 7 days" can use the key index.
//
// It deliberately does not use gorm.Model: rows are counters, not records
// that are soft-deleted. Old rows are removed for good by the retention job.
type AnalyticsHourly struct {
	// BucketHour is the start of the hour (UTC), for example 14:00.
	BucketHour time.Time `gorm:"primaryKey;not null" json:"bucket_hour"`
	Kind       string    `gorm:"primaryKey;size:20;not null" json:"kind"`
	// Name is empty for REQUEST rows. Its size matches the limit used for
	// guardrail names in log lines.
	Name string `gorm:"primaryKey;size:100;not null;default:''" json:"name"`

	// Total is how many times it happened in that hour.
	Total int64 `gorm:"not null;default:0" json:"total"`
	// Blocked is how many of those requests were blocked.
	Blocked int64 `gorm:"not null;default:0" json:"blocked"`
}

// TableName keeps the table name singular and readable. GORM's default
// would pluralize "hourly" into "analytics_hourlies".
func (AnalyticsHourly) TableName() string {
	return "analytics_hourly"
}
