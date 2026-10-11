package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strings"
	"testing"
	"time"
)

// These tests need a running stack (see tests/README.md). The ones that read
// /dashboard/analytics sign in as a dashboard user, so they only run when
// TSZ_TEST_ADMIN_EMAIL and TSZ_TEST_ADMIN_PASSWORD are set. The session
// check needs no credentials and always runs.

type analyticsPoint struct {
	Time    string `json:"time"`
	Total   int64  `json:"total"`
	Blocked int64  `json:"blocked"`
}

type analyticsNamed struct {
	Name  string `json:"name"`
	Total int64  `json:"total"`
}

type analyticsReport struct {
	Range  string `json:"range"`
	Bucket string `json:"bucket"`
	Totals struct {
		Requests int64 `json:"requests"`
		Blocked  int64 `json:"blocked"`
		Allowed  int64 `json:"allowed"`
	} `json:"totals"`
	Requests   []analyticsPoint `json:"requests"`
	PII        []analyticsNamed `json:"pii"`
	Guardrails []analyticsNamed `json:"guardrails"`
}

// analyticsFlushWait is a little longer than analytics.FlushInterval (30s).
const analyticsFlushWait = 60 * time.Second

func analyticsClient(t *testing.T) *http.Client {
	t.Helper()

	email := strings.TrimSpace(os.Getenv("TSZ_TEST_ADMIN_EMAIL"))
	password := os.Getenv("TSZ_TEST_ADMIN_PASSWORD")
	if email == "" || password == "" {
		t.Skip("TSZ_TEST_ADMIN_EMAIL / TSZ_TEST_ADMIN_PASSWORD not set; skipping dashboard session test")
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	client := &http.Client{Jar: jar, Timeout: 15 * time.Second}

	payload, _ := json.Marshal(map[string]string{"email": email, "password": password})
	req, err := http.NewRequest(http.MethodPost, baseURL()+"/auth/login", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	applyTestAuthHeaders(req)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login returned %d, expected 200", resp.StatusCode)
	}
	return client
}

func getAnalytics(t *testing.T, client *http.Client, query string) (int, []byte) {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, baseURL()+"/dashboard/analytics"+query, nil)
	if err != nil {
		t.Fatalf("analytics request: %v", err)
	}
	applyTestAuthHeaders(req)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /dashboard/analytics failed: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read analytics body: %v", err)
	}
	return resp.StatusCode, body
}

func loadReport(t *testing.T, client *http.Client, rng string) (analyticsReport, []byte) {
	t.Helper()

	status, body := getAnalytics(t, client, "?range="+rng)
	if status != http.StatusOK {
		t.Fatalf("range=%s: expected 200, got %d: %s", rng, status, body)
	}
	var report analyticsReport
	if err := json.Unmarshal(body, &report); err != nil {
		t.Fatalf("range=%s: invalid JSON: %v", rng, err)
	}
	return report, body
}

func piiTotal(items []analyticsNamed, name string) int64 {
	for _, item := range items {
		if item.Name == name {
			return item.Total
		}
	}
	return 0
}

func TestAnalytics_RequiresASession(t *testing.T) {
	// A plain client with no cookie and no API credentials.
	resp, err := http.Get(baseURL() + "/dashboard/analytics")
	if err != nil {
		t.Fatalf("GET /dashboard/analytics failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without a session, got %d", resp.StatusCode)
	}
}

func TestAnalytics_RangesHaveTheDocumentedShape(t *testing.T) {
	client := analyticsClient(t)

	cases := []struct {
		rng    string
		bucket string
		points int
	}{
		{"24h", "hour", 24},
		{"7d", "hour", 168},
		{"30d", "day", 30},
		{"90d", "day", 90},
	}
	for _, c := range cases {
		report, _ := loadReport(t, client, c.rng)
		if report.Range != c.rng || report.Bucket != c.bucket {
			t.Errorf("range=%s: got range=%q bucket=%q, want bucket %q", c.rng, report.Range, report.Bucket, c.bucket)
		}
		if len(report.Requests) != c.points {
			t.Errorf("range=%s: got %d points, want %d", c.rng, len(report.Requests), c.points)
		}
		if report.Totals.Allowed != report.Totals.Requests-report.Totals.Blocked {
			t.Errorf("range=%s: allowed %d != requests %d - blocked %d",
				c.rng, report.Totals.Allowed, report.Totals.Requests, report.Totals.Blocked)
		}
	}

	// No range means the same as 7d.
	status, body := getAnalytics(t, client, "")
	if status != http.StatusOK {
		t.Fatalf("default range: expected 200, got %d", status)
	}
	var def analyticsReport
	if err := json.Unmarshal(body, &def); err != nil || def.Range != "7d" {
		t.Errorf("default range should be 7d, got %q (err=%v)", def.Range, err)
	}

	if status, _ := getAnalytics(t, client, "?range=1y"); status != http.StatusBadRequest {
		t.Errorf("unknown range: expected 400, got %d", status)
	}
}

func TestAnalytics_CountsDetectRequestsAfterTheFlush(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for the 30 second flush; skipped with -short")
	}
	client := analyticsClient(t)

	before, _ := loadReport(t, client, "24h")

	// Unique text so the test can prove it is never echoed back by analytics.
	secretEmail := fmt.Sprintf("analytics-it-%d@example.com", time.Now().UnixNano())
	rid := fmt.Sprintf("analytics-it-rid-%d", time.Now().UnixNano())
	const sent = 3

	for i := 0; i < sent; i++ {
		body := fmt.Sprintf(`{"text": "Contact me at %s", "rid": "%s-%d"}`, secretEmail, rid, i)
		resp, err := postJSONWithAuth(http.DefaultClient, baseURL()+"/detect", strings.NewReader(body))
		if err != nil {
			t.Fatalf("POST /detect failed: %v", err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized {
			t.Fatalf("/detect returned 401: set TSZ_TEST_ADMIN_KEY (the stack's ADMIN_API_KEY) or TSZ_TEST_BEARER_TOKEN")
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 from /detect, got %d", resp.StatusCode)
		}
	}

	// Other tests may send requests at the same time, so only "at least" holds.
	deadline := time.Now().Add(analyticsFlushWait)
	var after analyticsReport
	var raw []byte
	for {
		after, raw = loadReport(t, client, "24h")
		if after.Totals.Requests >= before.Totals.Requests+sent {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("after %s the requests total is %d, expected at least %d",
				analyticsFlushWait, after.Totals.Requests, before.Totals.Requests+sent)
		}
		time.Sleep(3 * time.Second)
	}

	if got, want := piiTotal(after.PII, "EMAIL"), piiTotal(before.PII, "EMAIL")+sent; got < want {
		t.Errorf("EMAIL detections: got %d, want at least %d", got, want)
	}

	// The report may hold counts and names only, never what was sent.
	for _, secret := range []string{secretEmail, rid} {
		if bytes.Contains(raw, []byte(secret)) {
			t.Errorf("analytics response contains request data %q", secret)
		}
	}
}
