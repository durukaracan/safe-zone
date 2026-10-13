// Package analytics moves the hourly counters kept in memory by the metrics
// package into the database.
package analytics

import (
	"context"
	"log"
	"time"

	"thyris-sz/internal/metrics"
	"thyris-sz/internal/models"
)

// FlushInterval is how often the counters are written to the database. A
// crash loses at most this much counting; a normal shutdown loses none,
// because Run flushes one last time.
const FlushInterval = 30 * time.Second

// RetentionDays is how long hourly counters are kept. Older rows are
// deleted for good by RunCleanup.
const RetentionDays = 90

// CleanupInterval is how often old rows are looked for. The job also runs
// once when the application starts.
const CleanupInterval = 24 * time.Hour

// WriteFunc writes counter rows to the database. It must be all-or-nothing:
// when it returns an error, none of the rows may have been stored.
type WriteFunc func(rows []models.AnalyticsHourly) error

// FlushOnce takes everything counted so far and writes it with write. If
// the write fails, the counts are put back in the buffer, so they are tried
// again at the next flush.
func FlushOnce(write WriteFunc) error {
	counts := metrics.TakeAnalytics()
	if len(counts) == 0 {
		return nil
	}

	rows := make([]models.AnalyticsHourly, 0, len(counts))
	for key, c := range counts {
		rows = append(rows, models.AnalyticsHourly{
			BucketHour: key.BucketHour,
			Kind:       key.Kind,
			Name:       key.Name,
			Total:      c.Total,
			Blocked:    c.Blocked,
		})
	}

	if err := write(rows); err != nil {
		metrics.RestoreAnalytics(counts)
		return err
	}
	return nil
}

// Run flushes every interval until ctx is cancelled, then flushes one last
// time and returns. A failed flush is logged and retried on the next tick;
// it never stops the loop.
func Run(ctx context.Context, interval time.Duration, write WriteFunc) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := FlushOnce(write); err != nil {
				log.Printf("[Analytics] could not write hourly counters, will retry: %v", err)
			}
		case <-ctx.Done():
			if err := FlushOnce(write); err != nil {
				log.Printf("[Analytics] could not write hourly counters on shutdown: %v", err)
			}
			return
		}
	}
}

// DeleteFunc permanently removes counters whose hour started before cutoff
// and returns how many rows it removed.
type DeleteFunc func(cutoff time.Time) (int64, error)

// CleanupOnce deletes counters older than RetentionDays. The cut-off is
// "now minus RetentionDays", rounded down to the start of an hour, so whole
// hourly rows are kept or removed, never half of one.
func CleanupOnce(now time.Time, del DeleteFunc) (int64, error) {
	cutoff := now.UTC().AddDate(0, 0, -RetentionDays).Truncate(time.Hour)
	return del(cutoff)
}

// RunCleanup runs CleanupOnce right away and then every interval, until ctx
// is cancelled. A failed run is logged and tried again at the next tick; it
// never stops the loop.
func RunCleanup(ctx context.Context, interval time.Duration, del DeleteFunc) {
	cleanup := func() {
		removed, err := CleanupOnce(time.Now(), del)
		if err != nil {
			log.Printf("[Analytics] could not delete old hourly counters, will retry: %v", err)
			return
		}
		if removed > 0 {
			log.Printf("[Analytics] deleted %d hourly counter rows older than %d days", removed, RetentionDays)
		}
	}

	cleanup()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cleanup()
		case <-ctx.Done():
			return
		}
	}
}
