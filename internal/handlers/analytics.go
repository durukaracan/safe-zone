package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"thyris-sz/internal/analytics"
	"thyris-sz/internal/models"
)

// AnalyticsLoader returns the hourly counters whose hour started at or after
// since.
type AnalyticsLoader func(since time.Time) ([]models.AnalyticsHourly, error)

// NewDashboardAnalyticsHandler builds GET /dashboard/analytics. The data
// source and the clock are passed in so the handler can be tested without a
// database.
//
// Optional query parameter: range=24h|7d|30d|90d (default 7d).
//
// The answer holds only counts and names. Counts that are still waiting in
// memory for the next flush (at most analytics.FlushInterval old) are not
// included.
func NewDashboardAnalyticsHandler(load AnalyticsLoader, now func() time.Time) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		rng, ok := analytics.ParseRange(r.URL.Query().Get("range"))
		if !ok {
			http.Error(w, "Invalid range", http.StatusBadRequest)
			return
		}

		current := now()
		rows, err := load(rng.Start(current))
		if err != nil {
			log.Printf("[Analytics] could not load hourly counters: %v", err)
			http.Error(w, "Failed to load analytics", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if err := json.NewEncoder(w).Encode(analytics.BuildReport(rows, rng, current)); err != nil {
			log.Printf("[Analytics] could not encode report: %v", err)
		}
	}
}
