package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	mirastack "github.com/mirastacklabs-ai/mirastack-agents-sdk-go"
)

func TestActionQuery_PreservesBalancedQuotedLogSQL(t *testing.T) {
	var capturedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	p := &QueryVLogsPlugin{client: NewVLogsClient(srv.URL)} // engine=nil
	_, err := p.actionQuery(context.Background(), map[string]string{
		"query": `service_name:"payments"`,
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedQuery != `service_name:"payments"` {
		t.Fatalf("expected preserved LogSQL query, got %q", capturedQuery)
	}
}

func TestActionQuery_TrimsOuterWhitespace(t *testing.T) {
	var capturedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	p := &QueryVLogsPlugin{client: NewVLogsClient(srv.URL)}
	_, err := p.actionQuery(context.Background(), map[string]string{
		"query": `  service_name:"payments"  `,
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedQuery != `service_name:"payments"` {
		t.Fatalf("expected trimmed LogSQL query, got %q", capturedQuery)
	}
}

func TestActionQuery_RemovesTrailingSemicolonOutsideQuotes(t *testing.T) {
	var capturedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	p := &QueryVLogsPlugin{client: NewVLogsClient(srv.URL)}
	_, err := p.actionQuery(context.Background(), map[string]string{
		"query": `service_name:"payments";`,
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedQuery != `service_name:"payments"` {
		t.Fatalf("expected trailing semicolon to be removed, got %q", capturedQuery)
	}
}

func TestActionHits_AdaptiveDefaultStepForLargeRange(t *testing.T) {
	var capturedStep string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedStep = r.URL.Query().Get("step")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":[]}`))
	}))
	defer srv.Close()

	p := &QueryVLogsPlugin{client: NewVLogsClient(srv.URL)}
	_, err := p.actionHits(context.Background(), map[string]string{
		"query": `service_name:"payments"`,
	}, &mirastack.TimeRange{StartEpochMs: 1700000000000, EndEpochMs: 1700172800000}) // 48h
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedStep != "5m" {
		t.Fatalf("expected adaptive step 5m for 48h range, got %q", capturedStep)
	}
}

func TestActionStats_RetriesOnceOn422TooManyPoints(t *testing.T) {
	var (
		mu    sync.Mutex
		steps []string
		calls int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		steps = append(steps, r.URL.Query().Get("step"))
		currentCall := calls
		mu.Unlock()

		if currentCall == 1 {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte("too many points for selected step"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"result":[]}}`))
	}))
	defer srv.Close()

	p := &QueryVLogsPlugin{client: NewVLogsClient(srv.URL)}
	_, err := p.actionStats(context.Background(), map[string]string{
		"query": `service_name:"payments" | stats count()`,
		"step":  "30s",
	}, &mirastack.TimeRange{StartEpochMs: 1700000000000, EndEpochMs: 1700003600000})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if calls != 2 {
		t.Fatalf("expected exactly 2 backend calls (retry once), got %d", calls)
	}
	if len(steps) != 2 || steps[0] != "30s" || steps[1] != "1m" {
		t.Fatalf("expected retry step progression [30s,1m], got %v", steps)
	}
}

func TestActionStats_PreservesStatsPipe(t *testing.T) {
	var capturedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"result":[]}}`))
	}))
	defer srv.Close()

	p := &QueryVLogsPlugin{client: NewVLogsClient(srv.URL)}
	q := `_msg:error | stats count() by (service)`
	_, err := p.actionStats(context.Background(), map[string]string{
		"query": q,
	}, &mirastack.TimeRange{StartEpochMs: 1700000000000, EndEpochMs: 1700003600000})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedQuery != q {
		t.Fatalf("expected stats query to be preserved, got %q", capturedQuery)
	}
}
