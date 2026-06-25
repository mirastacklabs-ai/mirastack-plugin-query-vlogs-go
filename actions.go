package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	mirastack "github.com/mirastacklabs-ai/mirastack-agents-sdk-go"
	"github.com/mirastacklabs-ai/mirastack-agents-sdk-go/datetimeutils"
)

// isValidVLogsTimeParam rejects empty, whitespace-only, bare "-" and bare "+"
// values that would cause VictoriaLogs to return parse errors.
func isValidVLogsTimeParam(v string) bool {
	v = strings.TrimSpace(v)
	return v != "" && v != "-" && v != "+"
}

// resolveStartEnd returns start/end strings, preferring engine-parsed TimeRange.
// When falling back to raw params, invalid values are cleared to empty string
// so the VictoriaLogs API can apply its own server-side defaults.
func resolveStartEnd(params map[string]string, tr *mirastack.TimeRange) (start, end string) {
	if tr != nil && tr.StartEpochMs > 0 {
		return datetimeutils.FormatRFC3339(tr.StartEpochMs), datetimeutils.FormatRFC3339(tr.EndEpochMs)
	}
	start = params["start"]
	end = params["end"]
	if !isValidVLogsTimeParam(start) {
		start = ""
	}
	if !isValidVLogsTimeParam(end) {
		end = ""
	}
	return start, end
}

// Action handlers for the query_vlogs plugin.
// Each action maps to a VictoriaLogs LogsQL API endpoint.

func (p *QueryVLogsPlugin) actionQuery(ctx context.Context, params map[string]string, tr *mirastack.TimeRange) (string, error) {
	query := params["query"]
	if query == "" {
		return "", fmt.Errorf("query parameter is required for query action")
	}
	limit := params["limit"]
	if limit == "" {
		limit = "100"
	}
	start, end := resolveStartEnd(params, tr)
	return p.client.Query(ctx, query, start, end, limit)
}

func (p *QueryVLogsPlugin) actionHits(ctx context.Context, params map[string]string, tr *mirastack.TimeRange) (string, error) {
	query := params["query"]
	if query == "" {
		query = "*"
	}
	step := params["step"]
	if step == "" {
		step = "5m"
	}
	start, end := resolveStartEnd(params, tr)
	return p.client.Hits(ctx, query, start, end, step, params["field"])
}

func (p *QueryVLogsPlugin) actionFieldNames(ctx context.Context, params map[string]string, tr *mirastack.TimeRange) (string, error) {
	query := params["query"]
	if query == "" {
		query = "*"
	}
	start, end := resolveStartEnd(params, tr)
	return p.client.FieldNames(ctx, query, start, end)
}

func (p *QueryVLogsPlugin) actionFieldValues(ctx context.Context, params map[string]string, tr *mirastack.TimeRange) (string, error) {
	field := params["field"]
	if field == "" {
		return "", fmt.Errorf("field parameter is required for field_values action")
	}
	query := params["query"]
	if query == "" {
		query = "*"
	}
	limit := params["limit"]
	if limit == "" {
		limit = "100"
	}
	start, end := resolveStartEnd(params, tr)
	return p.client.FieldValues(ctx, query, field, start, end, limit)
}

func (p *QueryVLogsPlugin) actionStreams(ctx context.Context, params map[string]string, tr *mirastack.TimeRange) (string, error) {
	query := params["query"]
	if query == "" {
		query = "*"
	}
	limit := params["limit"]
	if limit == "" {
		limit = "100"
	}
	start, end := resolveStartEnd(params, tr)
	return p.client.Streams(ctx, query, start, end, limit)
}

func (p *QueryVLogsPlugin) actionStats(ctx context.Context, params map[string]string, tr *mirastack.TimeRange) (string, error) {
	query := params["query"]
	if query == "" {
		return "", fmt.Errorf("query parameter is required for stats action (use LogsQL with | stats pipe)")
	}
	start, end := resolveStartEnd(params, tr)
	return p.client.StatsQuery(ctx, query, start, end)
}

// actionSearch builds a LogsQL query from structured params (service, level, keywords)
// and executes it. This removes the need for the LLM to generate raw LogsQL syntax.
// Workflow: discover available fields → build query with proper quoting → execute.
func (p *QueryVLogsPlugin) actionSearch(ctx context.Context, params map[string]string, tr *mirastack.TimeRange) (string, error) {
	service := strings.TrimSpace(params["service"])
	level := strings.TrimSpace(params["level"])
	keywords := strings.TrimSpace(params["keywords"])

	if service == "" && level == "" && keywords == "" {
		return "", fmt.Errorf("at least one of service, level, or keywords is required for search action")
	}

	start, end := resolveStartEnd(params, tr)

	// Step 1: Discover available fields to determine which field holds service/level info.
	fieldNamesRaw, err := p.client.FieldNames(ctx, "*", start, end)
	if err != nil {
		return "", fmt.Errorf("field discovery failed: %w", err)
	}

	availableFields := parseFieldNames(fieldNamesRaw)
	serviceField := resolveFieldName(availableFields, []string{
		"service", "service_name", "app", "application",
		"resource.attributes.service.name", "ServiceName",
		"kubernetes_container_name", "k8s.deployment.name",
		"data_stream.dataset",
	})
	levelField := resolveFieldName(availableFields, []string{
		"level", "severity", "severity_text", "log_level",
		"loglevel", "Level", "severity_number",
	})

	// Step 2: Build LogsQL query with proper syntax and quoting.
	var clauses []string

	if service != "" && serviceField != "" {
		clauses = append(clauses, fmt.Sprintf("%s:%q", serviceField, service))
	} else if service != "" {
		clauses = append(clauses, fmt.Sprintf("_msg:%q", service))
	}

	if level != "" && levelField != "" {
		clauses = append(clauses, fmt.Sprintf("%s:%q", levelField, level))
	} else if level != "" {
		clauses = append(clauses, fmt.Sprintf("_msg:%s", level))
	}

	if keywords != "" {
		for _, kw := range strings.Fields(keywords) {
			clauses = append(clauses, fmt.Sprintf("_msg:%q", kw))
		}
	}

	query := strings.Join(clauses, " AND ")
	if query == "" {
		query = "*"
	}

	limit := params["limit"]
	if limit == "" {
		limit = "100"
	}

	// Step 3: Execute the properly-built query.
	result, err := p.client.Query(ctx, query, start, end, limit)
	if err != nil {
		return "", fmt.Errorf("search query execution failed (query=%s): %w", query, err)
	}

	// Return both the constructed query and the results for transparency.
	return fmt.Sprintf(`{"logsql_query":%q,"service_field":%q,"level_field":%q,"available_fields":%q,"result":%s,"result_count":%d}`,
		query, serviceField, levelField, strings.Join(availableFields, ","), result, countNDJSONLines(result)), nil
}

// parseFieldNames extracts field names from VictoriaLogs field_names JSON response.
// VictoriaLogs returns either:
//   - {"values":[{"value":"fieldname","hits":N}, ...]}  (structured)
//   - ["field1","field2",...] (simple array)
//   - one field name per line (NDJSON)
func parseFieldNames(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	// Try structured format: {"values":[{"value":"...","hits":N},...]}
	type fieldEntry struct {
		Value string `json:"value"`
	}
	type fieldResponse struct {
		Values []fieldEntry `json:"values"`
	}
	var structured fieldResponse
	if err := json.Unmarshal([]byte(raw), &structured); err == nil && len(structured.Values) > 0 {
		fields := make([]string, 0, len(structured.Values))
		for _, e := range structured.Values {
			if e.Value != "" {
				fields = append(fields, e.Value)
			}
		}
		return fields
	}

	// Try simple JSON array: ["field1","field2",...]
	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err == nil && len(arr) > 0 {
		return arr
	}

	// Fallback: line-separated values
	var fields []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "[" || line == "]" {
			continue
		}
		line = strings.Trim(line, `",[]`)
		if line != "" {
			fields = append(fields, line)
		}
	}
	return fields
}

// resolveFieldName picks the first match from candidates that exists in available fields.
func resolveFieldName(available []string, candidates []string) string {
	avSet := make(map[string]struct{}, len(available))
	for _, f := range available {
		avSet[strings.ToLower(f)] = struct{}{}
	}
	for _, c := range candidates {
		if _, ok := avSet[strings.ToLower(c)]; ok {
			return c
		}
	}
	return ""
}

// countNDJSONLines counts non-empty lines (each is a log entry).
func countNDJSONLines(raw string) int {
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

func (p *QueryVLogsPlugin) actionDeleteStream(ctx context.Context, params map[string]string, tr *mirastack.TimeRange) (string, error) {
	match := params["match"]
	if match == "" {
		return "", fmt.Errorf("match parameter is required for delete_stream")
	}
	start, end := resolveStartEnd(params, tr)
	if err := p.client.DeleteStream(ctx, match, start, end); err != nil {
		return "", err
	}
	return fmt.Sprintf(`{"status":"success","deleted":"%s"}`, match), nil
}
