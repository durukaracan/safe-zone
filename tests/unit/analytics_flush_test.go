package unit

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"thyris-sz/internal/analytics"
	"thyris-sz/internal/metrics"
	"thyris-sz/internal/models"
)

// recordingWriter is a fake database: it remembers what it was asked to
// write and can be told to fail.
type recordingWriter struct {
	calls [][]models.AnalyticsHourly
	err   error
}

func (w *recordingWriter) write(rows []models.AnalyticsHourly) error {
	if w.err != nil {
		return w.err
	}
	w.calls = append(w.calls, rows)
	return nil
}

func TestFlushOnce_WritesCountersAndEmptiesTheBuffer(t *testing.T) {
	metrics.TakeAnalytics()

	at := time.Date(2026, 10, 9, 14, 0, 0, 0, time.UTC)
	metrics.RecordAnalytics(metrics.AnalyticsEvent{
		Time:      at,
		Blocked:   true,
		PIICounts: map[string]int{"EMAIL": 2},
	})

	w := &recordingWriter{}
	if err := analytics.FlushOnce(w.write); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(w.calls) != 1 {
		t.Fatalf("expected one write, got %d", len(w.calls))
	}
	// One REQUEST row and one PII row.
	if len(w.calls[0]) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(w.calls[0]))
	}
	for _, row := range w.calls[0] {
		if !row.BucketHour.Equal(at) {
			t.Fatalf("unexpected bucket hour %v", row.BucketHour)
		}
		if row.Kind == models.AnalyticsKindPII && (row.Name != "EMAIL" || row.Total != 2 || row.Blocked != 2) {
			t.Fatalf("unexpected PII row: %+v", row)
		}
		if row.Kind == models.AnalyticsKindRequest && (row.Name != "" || row.Total != 1 || row.Blocked != 1) {
			t.Fatalf("unexpected REQUEST row: %+v", row)
		}
	}

	if left := metrics.TakeAnalytics(); len(left) != 0 {
		t.Fatalf("buffer should be empty after a successful flush, got %d counters", len(left))
	}
}

func TestFlushOnce_NothingToWriteDoesNotCallTheDatabase(t *testing.T) {
	metrics.TakeAnalytics()

	w := &recordingWriter{}
	if err := analytics.FlushOnce(w.write); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(w.calls) != 0 {
		t.Fatalf("expected no write for an empty buffer, got %d", len(w.calls))
	}
}

func TestFlushOnce_FailedWriteKeepsTheCountsForTheNextFlush(t *testing.T) {
	metrics.TakeAnalytics()

	at := time.Date(2026, 10, 9, 14, 0, 0, 0, time.UTC)
	metrics.RecordAnalytics(metrics.AnalyticsEvent{Time: at, Blocked: true})

	failing := &recordingWriter{err: errors.New("database is down")}
	if err := analytics.FlushOnce(failing.write); err == nil {
		t.Fatal("expected the write error to be returned")
	}

	// A new request arrives while the database is still down.
	metrics.RecordAnalytics(metrics.AnalyticsEvent{Time: at, Blocked: false})

	working := &recordingWriter{}
	if err := analytics.FlushOnce(working.write); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(working.calls) != 1 || len(working.calls[0]) != 1 {
		t.Fatalf("expected one write with one row, got %+v", working.calls)
	}

	row := working.calls[0][0]
	if row.Total != 2 || row.Blocked != 1 {
		t.Fatalf("expected the old and new counts added together (total=2 blocked=1), got %+v", row)
	}
}

func TestRun_FlushesOneLastTimeWhenStopped(t *testing.T) {
	metrics.TakeAnalytics()

	metrics.RecordAnalytics(metrics.AnalyticsEvent{Time: time.Now()})

	w := &recordingWriter{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		// A long interval, so only the shutdown flush can write.
		analytics.Run(ctx, time.Hour, w.write)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after the context was cancelled")
	}

	if len(w.calls) != 1 {
		t.Fatalf("expected exactly one write on shutdown, got %d", len(w.calls))
	}
}

func TestCleanupOnce_KeepsExactly90Days(t *testing.T) {
	var gotCutoff time.Time
	del := func(cutoff time.Time) (int64, error) {
		gotCutoff = cutoff
		return 7, nil
	}

	// 17:37 in UTC+3 is 14:37 UTC. 90 days earlier is 2026-07-11, and the
	// cut-off is rounded down to the start of that hour.
	istanbul := time.FixedZone("UTC+3", 3*60*60)
	now := time.Date(2026, 10, 9, 17, 37, 0, 0, istanbul)

	removed, err := analytics.CleanupOnce(now, del)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if removed != 7 {
		t.Fatalf("expected the removed count to be passed on, got %d", removed)
	}

	want := time.Date(2026, 7, 11, 14, 0, 0, 0, time.UTC)
	if !gotCutoff.Equal(want) {
		t.Fatalf("expected cut-off %v, got %v", want, gotCutoff)
	}
}

func TestCleanupOnce_ReturnsTheDeleteError(t *testing.T) {
	del := func(time.Time) (int64, error) { return 0, errors.New("database is down") }

	if _, err := analytics.CleanupOnce(time.Now(), del); err == nil {
		t.Fatal("expected the delete error to be returned")
	}
}

func TestRunCleanup_RunsAtStartAndKeepsGoingAfterAnError(t *testing.T) {
	var calls atomic.Int32
	del := func(time.Time) (int64, error) {
		calls.Add(1)
		return 0, errors.New("database is down")
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		analytics.RunCleanup(ctx, 10*time.Millisecond, del)
	}()

	// The first run happens immediately; later runs follow the ticker even
	// though every run fails.
	deadline := time.After(2 * time.Second)
	for calls.Load() < 3 {
		select {
		case <-deadline:
			t.Fatalf("expected at least 3 cleanup runs, got %d", calls.Load())
		case <-time.After(5 * time.Millisecond):
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunCleanup did not stop after the context was cancelled")
	}
}
