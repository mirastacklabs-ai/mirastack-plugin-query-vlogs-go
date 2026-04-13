package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	mirastack "github.com/mirastacklabs-ai/mirastack-agents-sdk-go"
	"github.com/mirastacklabs-ai/mirastack-agents-sdk-go/datetimeutils"
)

// ── isValidVLogsTimeParam ────────────────────────────────────────────────────

func TestIsValidVLogsTimeParam(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"", false},
		{"-", false},
		{"+", false},
		{"  ", false},
		{" - ", false},
		{" + ", false},
		{"-1h", true},
		{"now", true},
		{"2026-04-10T00:00:00Z", true},
		{"1743379200", true},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("input=%q", tc.input), func(t *testing.T) {
			got := isValidVLogsTimeParam(tc.input)
			if got != tc.valid {
				t.Errorf("isValidVLogsTimeParam(%q) = %v, want %v", tc.input, got, tc.valid)
			}
		})
	}
}

// ── resolveStartEnd ──────────────────────────────────────────────────────────

func TestResolveStartEnd_PrefersTimeRange(t *testing.T) {
	tr := &mirastack.TimeRange{
		StartEpochMs: 1743379200000,
		EndEpochMs:   1743382800000,
	}
	start, end := resolveStartEnd(map[string]string{"start": "raw", "end": "raw"}, tr)

	expectedStart := datetimeutils.FormatRFC3339(tr.StartEpochMs)
	expectedEnd := datetimeutils.FormatRFC3339(tr.EndEpochMs)

	if start != expectedStart {
		t.Errorf("start: expected %s, got %s", expectedStart, start)
	}
	if end != expectedEnd {
		t.Errorf("end: expected %s, got %s", expectedEnd, end)
	}
}

func TestResolveStartEnd_FallbackRejectsInvalidDash(t *testing.T) {
	start, end := resolveStartEnd(map[string]string{"start": "-", "end": "+"}, nil)
	if start != "" {
		t.Errorf("expected empty start for bare \"-\", got %q", start)
	}
	if end != "" {
		t.Errorf("expected empty end for bare \"+\", got %q", end)
	}
}

func TestResolveStartEnd_FallbackAcceptsValid(t *testing.T) {
	start, end := resolveStartEnd(map[string]string{"start": "-1h", "end": "now"}, nil)
	if start != "-1h" {
		t.Errorf("expected start=-1h, got %q", start)
	}
	if end != "now" {
		t.Errorf("expected end=now, got %q", end)
	}
}

func TestResolveStartEnd_NilTimeRangeEmptyParams(t *testing.T) {
	start, end := resolveStartEnd(map[string]string{}, nil)
	if start != "" {
		t.Errorf("expected empty start, got %q", start)
	}
	if end != "" {
		t.Errorf("expected empty end, got %q", end)
	}
}

// ── actionQuery ──────────────────────────────────────────────────────────────

func TestActionQuery_UsesTimeRange(t *testing.T) {
	var capturedStart, capturedEnd string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedStart = r.URL.Query().Get("start")
		capturedEnd = r.URL.Query().Get("end")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"msg":"test"}`))
	}))
	defer srv.Close()

	p := &QueryVLogsPlugin{client: NewVLogsClient(srv.URL)}
	tr := &mirastack.TimeRange{
		StartEpochMs: 1743379200000,
		EndEpochMs:   1743382800000,
	}

	_, err := p.actionQuery(context.Background(), map[string]string{
		"query": "*",
	}, tr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedStart := datetimeutils.FormatRFC3339(tr.StartEpochMs)
	if capturedStart != expectedStart {
		t.Errorf("expected start=%s, got %s", expectedStart, capturedStart)
	}
	expectedEnd := datetimeutils.FormatRFC3339(tr.EndEpochMs)
	if capturedEnd != expectedEnd {
		t.Errorf("expected end=%s, got %s", expectedEnd, capturedEnd)
	}
}

func TestActionQuery_FallbackRejectsInvalidDash(t *testing.T) {
	var capturedStart string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedStart = r.URL.Query().Get("start")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"msg":"ok"}`))
	}))
	defer srv.Close()

	p := &QueryVLogsPlugin{client: NewVLogsClient(srv.URL)}

	_, err := p.actionQuery(context.Background(), map[string]string{
		"query": "*",
		"start": "-",
		"end":   "-",
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Invalid "-" should be cleared; VLogsClient skips empty start in query params
	if capturedStart == "-" {
		t.Error("start=\"-\" should have been rejected")
	}
}

func TestActionQuery_MissingQueryReturnsError(t *testing.T) {
	p := &QueryVLogsPlugin{client: NewVLogsClient("http://localhost:0")}

	_, err := p.actionQuery(context.Background(), map[string]string{}, nil)
	if err == nil {
		t.Fatal("expected error for missing query")
	}
}

// ── actionStats ──────────────────────────────────────────────────────────────

func TestActionStats_UsesTimeRange(t *testing.T) {
	var capturedStart, capturedEnd string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedStart = r.URL.Query().Get("start")
		capturedEnd = r.URL.Query().Get("end")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	p := &QueryVLogsPlugin{client: NewVLogsClient(srv.URL)}
	tr := &mirastack.TimeRange{StartEpochMs: 1743379200000, EndEpochMs: 1743382800000}

	_, err := p.actionStats(context.Background(), map[string]string{
		"query": "* | stats count()",
	}, tr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedStart != datetimeutils.FormatRFC3339(tr.StartEpochMs) {
		t.Errorf("expected RFC3339 start, got %q", capturedStart)
	}
	if capturedEnd != datetimeutils.FormatRFC3339(tr.EndEpochMs) {
		t.Errorf("expected RFC3339 end, got %q", capturedEnd)
	}
}
