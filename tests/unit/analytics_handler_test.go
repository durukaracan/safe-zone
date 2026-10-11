package unit

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"thyris-sz/internal/analytics"
	"thyris-sz/internal/handlers"
	"thyris-sz/internal/models"
)

func fixedNow() time.Time { return reportNow }

func TestAnalyticsHandler_RejectsOtherMethods(t *testing.T) {
	h := handlers.NewDashboardAnalyticsHandler(func(time.Time) ([]models.AnalyticsHourly, error) {
		t.Fatal("the data source must not be called")
		return nil, nil
	}, fixedNow)

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodPost, "/dashboard/analytics", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestAnalyticsHandler_RejectsAnUnknownRange(t *testing.T) {
	h := handlers.NewDashboardAnalyticsHandler(func(time.Time) ([]models.AnalyticsHourly, error) {
		t.Fatal("the data source must not be called")
		return nil, nil
	}, fixedNow)

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/dashboard/analytics?range=1y", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestAnalyticsHandler_ReturnsTheReportAndAsksForTheRightWindow(t *testing.T) {
	var askedSince time.Time
	h := handlers.NewDashboardAnalyticsHandler(func(since time.Time) ([]models.AnalyticsHourly, error) {
		askedSince = since
		return []models.AnalyticsHourly{
			reportRow(time.Date(2026, 10, 9, 14, 0, 0, 0, time.UTC), models.AnalyticsKindRequest, "", 4, 1),
			reportRow(time.Date(2026, 10, 9, 14, 0, 0, 0, time.UTC), models.AnalyticsKindPII, "EMAIL", 2, 0),
		}, nil
	}, fixedNow)

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/dashboard/analytics?range=24h", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("unexpected content type %q", ct)
	}

	wantSince := mustRange(t, "24h").Start(reportNow)
	if !askedSince.Equal(wantSince) {
		t.Fatalf("expected the data source to be asked from %v, got %v", wantSince, askedSince)
	}

	var report analytics.Report
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatalf("the answer is not valid JSON: %v", err)
	}
	if report.Range != "24h" || report.Bucket != "hour" || len(report.Requests) != 24 {
		t.Fatalf("unexpected report header: %+v", report)
	}
	if report.Totals.Requests != 4 || report.Totals.Blocked != 1 || report.Totals.Allowed != 3 {
		t.Fatalf("unexpected totals: %+v", report.Totals)
	}
	if len(report.PII) != 1 || report.PII[0].Name != "EMAIL" || report.PII[0].Total != 2 {
		t.Fatalf("unexpected PII list: %+v", report.PII)
	}
}

func TestAnalyticsHandler_UsesSevenDaysByDefault(t *testing.T) {
	h := handlers.NewDashboardAnalyticsHandler(func(time.Time) ([]models.AnalyticsHourly, error) {
		return nil, nil
	}, fixedNow)

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/dashboard/analytics", nil))

	var report analytics.Report
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatalf("the answer is not valid JSON: %v", err)
	}
	if report.Range != "7d" || len(report.Requests) != 168 {
		t.Fatalf("expected the 7d default, got %s with %d points", report.Range, len(report.Requests))
	}
}

func TestAnalyticsHandler_HidesDatabaseErrors(t *testing.T) {
	h := handlers.NewDashboardAnalyticsHandler(func(time.Time) ([]models.AnalyticsHourly, error) {
		return nil, errors.New("pq: password authentication failed for user postgres")
	}, fixedNow)

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/dashboard/analytics", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if body := rec.Body.String(); body != "Failed to load analytics\n" {
		t.Fatalf("the error must not leak details, got %q", body)
	}
}
