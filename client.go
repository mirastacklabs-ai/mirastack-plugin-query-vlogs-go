package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// VLogsClient is an HTTP client for the VictoriaLogs LogsQL API.
// Endpoints:
//   - /select/logsql/query        — Search logs
//   - /select/logsql/hits         — Time series of log hit counts
//   - /select/logsql/field_names  — List available field names
//   - /select/logsql/field_values — Get values for a specific field
//   - /select/logsql/streams      — List log streams
//   - /select/logsql/stats_query  — Server-side stats aggregation
type VLogsClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewVLogsClient creates a client for a VictoriaLogs instance.
func NewVLogsClient(baseURL string) *VLogsClient {
	return &VLogsClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// Query executes a LogsQL search query. Returns NDJSON lines.
func (c *VLogsClient) Query(ctx context.Context, query, start, end, limit string) (string, error) {
	params := url.Values{"query": {query}}
	if start != "" {
		params.Set("start", start)
	}
	if end != "" {
		params.Set("end", end)
	}
	if limit != "" {
		params.Set("limit", limit)
	}
	return c.get(ctx, "/select/logsql/query", params)
}

// Hits returns time series of log hit counts.
func (c *VLogsClient) Hits(ctx context.Context, query, start, end, step, fields string) (string, error) {
	params := url.Values{"query": {query}}
	if start != "" {
		params.Set("start", start)
	}
	if end != "" {
		params.Set("end", end)
	}
	if step != "" {
		params.Set("step", step)
	}
	if fields != "" {
		params.Set("fields", fields)
	}
	return c.get(ctx, "/select/logsql/hits", params)
}

// FieldNames returns available field names for logs matching the query.
func (c *VLogsClient) FieldNames(ctx context.Context, query, start, end string) (string, error) {
	params := url.Values{"query": {query}}
	if start != "" {
		params.Set("start", start)
	}
	if end != "" {
		params.Set("end", end)
	}
	return c.get(ctx, "/select/logsql/field_names", params)
}

// FieldValues returns values for a specific field.
func (c *VLogsClient) FieldValues(ctx context.Context, query, field, start, end, limit string) (string, error) {
	params := url.Values{"query": {query}}
	if field != "" {
		params.Set("field", field)
	}
	if start != "" {
		params.Set("start", start)
	}
	if end != "" {
		params.Set("end", end)
	}
	if limit != "" {
		params.Set("limit", limit)
	}
	return c.get(ctx, "/select/logsql/field_values", params)
}

// Streams returns log streams matching the query.
func (c *VLogsClient) Streams(ctx context.Context, query, start, end, limit string) (string, error) {
	params := url.Values{"query": {query}}
	if start != "" {
		params.Set("start", start)
	}
	if end != "" {
		params.Set("end", end)
	}
	if limit != "" {
		params.Set("limit", limit)
	}
	return c.get(ctx, "/select/logsql/streams", params)
}

// StatsQuery executes a LogsQL stats aggregation query.
func (c *VLogsClient) StatsQuery(ctx context.Context, query, start, end string) (string, error) {
	params := url.Values{"query": {query}}
	if start != "" {
		params.Set("start", start)
	}
	if end != "" {
		params.Set("end", end)
	}
	return c.get(ctx, "/select/logsql/stats_query", params)
}

// DeleteStream deletes log entries matching the provided stream selector.
// VictoriaLogs endpoint: POST /delete with query parameter.
func (c *VLogsClient) DeleteStream(ctx context.Context, match, start, end string) error {
	params := url.Values{"query": {match}}
	if start != "" {
		params.Set("start", start)
	}
	if end != "" {
		params.Set("end", end)
	}
	u := c.baseURL + "/delete?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("VictoriaLogs delete error (HTTP %d): %s", resp.StatusCode, truncate(string(body), 512))
	}
	return nil
}

// get performs a GET request and returns the raw response body as a string.
func (c *VLogsClient) get(ctx context.Context, path string, params url.Values) (string, error) {
	u := c.baseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("VictoriaLogs API error (HTTP %d): %s", resp.StatusCode, truncate(string(body), 512))
	}

	return string(body), nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
