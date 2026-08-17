package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	mirastack "github.com/mirastacklabs-ai/mirastack-agents-sdk-go"
	"github.com/mirastacklabs-ai/mirastack-agents-sdk-go/datetimeutils"
	"github.com/mirastacklabs-ai/mirastack-agents-sdk-go/telemetrycache"
)

// isValidVLogsTimeParam rejects empty, whitespace-only, bare "-" and bare "+"
// values that would cause VictoriaLogs to return parse errors.
func isValidVLogsTimeParam(v string) bool {
	v = strings.TrimSpace(v)
	return v != "" && v != "-" && v != "+"
}

func sanitizeLogsQL(q string) string {
	q = strings.TrimSpace(q)
	if q == "" {
		return ""
	}
	if strings.HasPrefix(q, "```") && strings.HasSuffix(q, "```") {
		inner := strings.TrimSuffix(strings.TrimPrefix(q, "```"), "```")
		inner = strings.TrimSpace(inner)
		if nl := strings.IndexByte(inner, '\n'); nl >= 0 {
			first := strings.TrimSpace(inner[:nl])
			rest := strings.TrimSpace(inner[nl+1:])
			if first != "" && !strings.ContainsAny(first, " \t") {
				inner = rest
			}
		}
		q = strings.TrimSpace(inner)
	}
	q = trimTrailingSemicolonOutsideQuotes(q)
	return strings.TrimSpace(q)
}

func trimTrailingSemicolonOutsideQuotes(q string) string {
	end := len(q) - 1
	for end >= 0 {
		switch q[end] {
		case ' ', '\t', '\n', '\r':
			end--
		default:
			goto foundEnd
		}
	}
	return ""

foundEnd:
	if q[end] != ';' {
		return q
	}

	inSingle := false
	inDouble := false
	escaped := false
	for i := 0; i <= end; i++ {
		ch := q[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}
		if ch == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
	}
	if inSingle || inDouble {
		return q
	}
	return strings.TrimSpace(q[:end])
}

func protectLogsQLForCache(q string) string {
	q = strings.TrimSpace(q)
	if q == "" {
		return q
	}
	// telemetrycache in older SDK releases sanitizes with PromQL rules and can
	// strip a trailing quote from expressions like service_name:"payments".
	if strings.HasSuffix(q, `"`) || strings.HasSuffix(q, `'`) {
		return "(" + q + ")"
	}
	return q
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
	query := sanitizeLogsQL(params["query"])
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
	query := protectLogsQLForCache(sanitizeLogsQL(params["query"]))
	if query == "" {
		query = "*"
	}
	startSec, endSec := resolveLogsRangeBoundsSec(params, tr)
	step := params["step"]
	if step == "" {
		step = telemetrycache.AdaptiveStep(startSec*1000, endSec*1000)
	}
	dsID := resolveLogsDataSourceID(params)
	result, err := telemetrycache.WithStepRetry(startSec, endSec, step, func(stepForRun string) ([]byte, error) {
		return telemetrycache.HitsCached(
			ctx,
			p.engine,
			dsID,
			query,
			strings.TrimSpace(params["field"]),
			startSec,
			endSec,
			stepForRun,
			func(filteredQuery string, cStart, cEnd int64, chunkStep string) ([]byte, error) {
				return bytesFromString(p.client.Hits(
					ctx,
					filteredQuery,
					datetimeutils.FormatRFC3339(cStart*1000),
					datetimeutils.FormatRFC3339(cEnd*1000),
					chunkStep,
					params["field"],
				))
			},
		)
	})
	if err != nil {
		return "", err
	}
	return string(result), nil
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
	query := protectLogsQLForCache(sanitizeLogsQL(params["query"]))
	if query == "" {
		return "", fmt.Errorf("query parameter is required for stats action (use LogsQL with | stats pipe)")
	}
	startSec, endSec := resolveLogsRangeBoundsSec(params, tr)
	step := params["step"]
	if step == "" {
		step = telemetrycache.AdaptiveStep(startSec*1000, endSec*1000)
	}
	dsID := resolveLogsDataSourceID(params)
	result, err := telemetrycache.WithStepRetry(startSec, endSec, step, func(stepForRun string) ([]byte, error) {
		return telemetrycache.StatsRangeCached(
			ctx,
			p.engine,
			dsID,
			query,
			startSec,
			endSec,
			stepForRun,
			func(cStart, cEnd int64, chunkStep string) ([]byte, error) {
				return bytesFromString(p.client.StatsRangeQuery(
					ctx,
					query,
					datetimeutils.FormatRFC3339(cStart*1000),
					datetimeutils.FormatRFC3339(cEnd*1000),
					chunkStep,
				))
			},
		)
	})
	if err != nil {
		return "", err
	}
	return string(result), nil
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

func resolveLogsDataSourceID(params map[string]string) string {
	for _, k := range []string{"data_source_id", "datasource_id", "integration_id"} {
		if v := strings.TrimSpace(params[k]); v != "" {
			return v
		}
	}
	return "default"
}

func resolveLogsRangeBoundsSec(params map[string]string, tr *mirastack.TimeRange) (int64, int64) {
	if tr != nil && tr.StartEpochMs > 0 {
		return tr.StartEpochMs / 1000, tr.EndEpochMs / 1000
	}
	nowSec := time.Now().UTC().Unix()
	startSec, startOK := logsTimeParamToSec(params["start"], nowSec)
	endSec, endOK := logsTimeParamToSec(params["end"], nowSec)
	switch {
	case !startOK && !endOK:
		endSec = nowSec
		startSec = endSec - 3600
	case !startOK && endOK:
		startSec = endSec - 3600
	case startOK && !endOK:
		endSec = nowSec
	}
	if endSec <= startSec {
		endSec = nowSec
		startSec = endSec - 3600
	}
	return startSec, endSec
}

func logsTimeParamToSec(raw string, nowSec int64) (int64, bool) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return 0, false
	}
	if v == "now" {
		return nowSec, true
	}
	if n, err := strconv.ParseInt(v, 10, 64); err == nil {
		if n > 1_000_000_000_000 {
			return n / 1000, true
		}
		return n, true
	}
	return 0, false
}

func bytesFromString(s string, err error) ([]byte, error) {
	if err != nil {
		return nil, err
	}
	return []byte(s), nil
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
