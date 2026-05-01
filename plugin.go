package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/mirastacklabs-ai/mirastack-agents-sdk-go"
	"go.uber.org/zap"
)

// QueryVLogsPlugin queries VictoriaLogs using the LogsQL API.
// The "v" prefix denotes Victoria-specific. Enterprise versions for other log backends
// (Elasticsearch, Loki, etc.) will follow the same plugin contract with a different prefix.
type QueryVLogsPlugin struct {
	client *VLogsClient
	engine *mirastack.EngineContext
	logger *zap.Logger
}

// SetEngineContext injects the engine callback context (pull model config).
func (p *QueryVLogsPlugin) SetEngineContext(ec *mirastack.EngineContext) {
	p.engine = ec
}

func (p *QueryVLogsPlugin) Info() *mirastack.PluginInfo {
	return &mirastack.PluginInfo{
		Name:    "query_vlogs",
		Version: "0.2.0",
		Description: "Search and analyze logs from VictoriaLogs using LogsQL. " +
			"Use this plugin to search log entries, build hit-count histograms, discover fields " +
			"and their values, list log streams, and compute server-side aggregations. " +
			"Start with field_names for schema discovery, query for keyword search, and stats for aggregation.",
		Permissions:  []mirastack.Permission{mirastack.PermissionRead, mirastack.PermissionAdmin},
		DevOpsStages: []mirastack.DevOpsStage{mirastack.StageObserve, mirastack.StageOperate},
		Actions: []mirastack.Action{
			{
				ID: "query",
				Description: "Search log entries using LogsQL expressions. " +
					"Use this for keyword search, error filtering, or pattern matching in logs. " +
					"Returns raw log lines in NDJSON format.",
				Permission: mirastack.PermissionRead,
				Stages:     []mirastack.DevOpsStage{mirastack.StageObserve},
				Intents: []mirastack.IntentPattern{
					{Pattern: "search logs", Description: "Search log entries with LogsQL", Priority: 10},
					{Pattern: "find errors in logs", Description: "Search for error-level log entries", Priority: 9},
					{Pattern: "grep logs for", Description: "Search logs matching a pattern", Priority: 8},
					{Pattern: "log entries containing", Description: "Find logs containing specific text", Priority: 7},
				},
				InputParams: []mirastack.ParamSchema{
					{Name: "query", Type: "string", Required: true, Description: "LogsQL query expression (e.g. '_msg:error AND service:payment')"},
					{Name: "limit", Type: "string", Required: false, Description: "Maximum number of log entries to return (default: 100)"},
				},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Log entries in VictoriaLogs NDJSON format"},
				},
			},
			{
				ID: "hits",
				Description: "Get log hit count as a time series histogram. " +
					"Use this to see log volume over time, identify spikes in errors, " +
					"or compare event frequency across time buckets.",
				Permission: mirastack.PermissionRead,
				Stages:     []mirastack.DevOpsStage{mirastack.StageObserve},
				Intents: []mirastack.IntentPattern{
					{Pattern: "log volume over time", Description: "Show log hit counts as histogram", Priority: 9},
					{Pattern: "error spike", Description: "Detect spikes in error log volume", Priority: 8},
					{Pattern: "log frequency", Description: "Show log event frequency over time", Priority: 7},
				},
				InputParams: []mirastack.ParamSchema{
					{Name: "query", Type: "string", Required: false, Description: "LogsQL filter expression (default: * for all logs)"},
					{Name: "step", Type: "string", Required: false, Description: "Time bucket step (e.g., 1m, 5m, 1h). Defaults to 5m."},
					{Name: "field", Type: "string", Required: false, Description: "Field to group hits by"},
				},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Hit count time series with timestamps and counts"},
				},
			},
			{
				ID: "field_names",
				Description: "List all field names present in log entries. " +
					"Use this for schema discovery to understand what structured fields are available. " +
					"Scoping with a LogsQL filter narrows results to relevant logs.",
				Permission: mirastack.PermissionRead,
				Stages:     []mirastack.DevOpsStage{mirastack.StageObserve},
				Intents: []mirastack.IntentPattern{
					{Pattern: "log fields", Description: "List available log field names", Priority: 9},
					{Pattern: "what log fields exist", Description: "Discover log schema fields", Priority: 8},
					{Pattern: "log schema", Description: "Show log entry structure", Priority: 7},
				},
				InputParams: []mirastack.ParamSchema{
					{Name: "query", Type: "string", Required: false, Description: "LogsQL filter to scope field discovery"},
				},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Array of field names"},
				},
			},
			{
				ID: "field_values",
				Description: "List values observed for a specific log field. " +
					"Use this to find unique values for service names, error codes, or status fields. " +
					"Helpful before building specific log queries.",
				Permission: mirastack.PermissionRead,
				Stages:     []mirastack.DevOpsStage{mirastack.StageObserve},
				Intents: []mirastack.IntentPattern{
					{Pattern: "log field values", Description: "List values for a log field", Priority: 9},
					{Pattern: "unique values in logs", Description: "Find unique field values in logs", Priority: 8},
					{Pattern: "which log services", Description: "Find service names from log fields", Priority: 7},
				},
				InputParams: []mirastack.ParamSchema{
					{Name: "field", Type: "string", Required: true, Description: "Field name to get values for (e.g., 'service', 'level', 'status')"},
					{Name: "query", Type: "string", Required: false, Description: "LogsQL filter to scope values"},
				},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Array of field values"},
				},
			},
			{
				ID: "streams",
				Description: "List log streams — unique label combinations identifying log sources. " +
					"Use this to find which applications, environments, or nodes are producing logs. " +
					"Streams represent the highest-level grouping in VictoriaLogs.",
				Permission: mirastack.PermissionRead,
				Stages:     []mirastack.DevOpsStage{mirastack.StageObserve},
				Intents: []mirastack.IntentPattern{
					{Pattern: "log streams", Description: "List log stream sources", Priority: 9},
					{Pattern: "which apps produce logs", Description: "Find applications generating logs", Priority: 8},
					{Pattern: "log sources", Description: "List sources of log data", Priority: 7},
				},
				InputParams: []mirastack.ParamSchema{
					{Name: "query", Type: "string", Required: false, Description: "LogsQL filter to scope streams"},
				},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Array of log streams with label sets"},
				},
			},
			{
				ID: "stats",
				Description: "Compute server-side aggregate statistics from logs using LogsQL stats pipes. " +
					"Use this for counting, grouping, averaging, and other aggregations. " +
					"Much faster than client-side aggregation of raw log entries.",
				Permission: mirastack.PermissionRead,
				Stages:     []mirastack.DevOpsStage{mirastack.StageObserve},
				Intents: []mirastack.IntentPattern{
					{Pattern: "log statistics", Description: "Compute aggregate statistics from logs", Priority: 9},
					{Pattern: "count log events", Description: "Count log entries matching criteria", Priority: 8},
					{Pattern: "aggregate logs", Description: "Run server-side log aggregation", Priority: 7},
				},
				InputParams: []mirastack.ParamSchema{
					{Name: "query", Type: "string", Required: true, Description: "LogsQL stats expression (e.g. '_msg:error | stats count() by (service)')"},
				},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Aggregated statistics result"},
				},
			},
			{
				ID: "delete_stream",
				Description: "Delete log entries matching a LogsQL filter expression. " +
					"This is a destructive ADMIN operation that permanently removes matching logs. " +
					"Use only for data cleanup or compliance-driven log purging. Requires approval.",
				Permission: mirastack.PermissionAdmin,
				Stages:     []mirastack.DevOpsStage{mirastack.StageOperate},
				Intents: []mirastack.IntentPattern{
					{Pattern: "delete logs", Description: "Delete log entries from storage", Priority: 9},
					{Pattern: "purge log stream", Description: "Purge a log stream permanently", Priority: 8},
					{Pattern: "remove log entries", Description: "Remove specific log data", Priority: 7},
				},
				InputParams: []mirastack.ParamSchema{
					{Name: "match", Type: "string", Required: true, Description: "LogsQL filter expression selecting logs to delete (e.g. '_stream:{service=\"old-app\"}')"},
				},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Deletion confirmation"},
				},
			},
		},
		Intents: []mirastack.IntentPattern{
			{Pattern: "search logs", Description: "Search log entries", Priority: 10},
			{Pattern: "find errors in logs", Description: "Search for error-level log entries", Priority: 9},
			{Pattern: "log field values", Description: "List field values from logs", Priority: 5},
			{Pattern: "logsql", Description: "Query using LogsQL syntax", Priority: 8},
			{Pattern: "log volume", Description: "Check log event volume and trends", Priority: 6},
			{Pattern: "application logs", Description: "View application log entries", Priority: 6},
			{Pattern: "log aggregation", Description: "Aggregate and summarize log data", Priority: 6},
		},
		PromptTemplates: []mirastack.PromptTemplate{
			{
				Name:        "query_vlogs_guide",
				Description: "Best practices for using VictoriaLogs query tools",
				Content: `You have access to VictoriaLogs log search tools. Follow these guidelines:

1. DISCOVERY FIRST: Use field_names to discover available fields. Use streams to find log sources.
2. LOGSQL BASICS: Use _msg:keyword for message search, field:value for exact match, _msg:~"regex" for regex.
3. BOOLEAN OPS: Combine with AND, OR, NOT. Example: _msg:error AND service:payment NOT _msg:timeout
4. TIME SCOPING: Engine provides time range automatically. Narrow scope for performance.
5. HITS for TRENDS: Use hits action to see log volume distribution before diving into raw entries.
6. STATS for AGGREGATION: Use LogsQL stats pipe for server-side counts and aggregation.
   Example: "_msg:error | stats count() by (service)" counts errors per service.
7. FIELD VALUES: Use field_values to enumerate possible values before filtering.
8. LIMIT results when exploring: start with limit=20, increase if needed.
9. COMMON PATTERNS:
   - Error search: _msg:error AND service:"my-app"
   - HTTP 5xx: status:~"5[0-9]{2}"
   - Slow requests: duration:>1000
   - Error count by service: _msg:error | stats count() by (service)`,
			},
		},
		ConfigParams: []mirastack.ConfigParam{
			{Key: "logs_url", Type: "string", Required: true, Description: "VictoriaLogs base URL (e.g. http://victorialogs:9428)"},
		},
	}
}

func (p *QueryVLogsPlugin) Schema() *mirastack.PluginSchema {
	info := p.Info()
	return &mirastack.PluginSchema{
		Actions: info.Actions,
	}
}

func (p *QueryVLogsPlugin) Execute(ctx context.Context, req *mirastack.ExecuteRequest) (*mirastack.ExecuteResponse, error) {
	if p.logger == nil {
		p.logger, _ = zap.NewProduction()
	}

	// Pull config from engine if client is not yet initialized (cached 15s in SDK).
	if p.client == nil && p.engine != nil {
		if config, err := p.engine.GetConfig(ctx); err == nil {
			p.applyConfig(config)
		}
	}

	action := req.ActionID
	if action == "" {
		action = req.Params["action"]
	}
	if action == "" {
		resp, _ := mirastack.RespondError("action parameter is required")
		resp.Logs = []string{"missing required parameter: action"}
		return resp, nil
	}

	result, err := p.dispatch(ctx, action, req.Params, req.TimeRange)
	if err != nil {
		resp, _ := mirastack.RespondError(err.Error())
		resp.Logs = []string{fmt.Sprintf("action %s failed: %v", action, err)}
		return resp, nil
	}

	resp, _ := mirastack.RespondJSON(enrichLogsOutput(action, result))
	resp.Logs = []string{fmt.Sprintf("action %s completed", action)}
	return resp, nil
}

func (p *QueryVLogsPlugin) dispatch(ctx context.Context, action string, params map[string]string, tr *mirastack.TimeRange) (string, error) {
	if p.client == nil {
		return "", fmt.Errorf("plugin not configured: logs_url not set")
	}

	switch action {
	case "query":
		return p.actionQuery(ctx, params, tr)
	case "hits":
		return p.actionHits(ctx, params, tr)
	case "field_names":
		return p.actionFieldNames(ctx, params, tr)
	case "field_values":
		return p.actionFieldValues(ctx, params, tr)
	case "streams":
		return p.actionStreams(ctx, params, tr)
	case "stats":
		return p.actionStats(ctx, params, tr)
	case "delete_stream":
		return p.actionDeleteStream(ctx, params, tr)
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

func (p *QueryVLogsPlugin) HealthCheck(ctx context.Context) error {
	// Pull config from engine (cached 15s in SDK)
	if p.engine != nil {
		config, err := p.engine.GetConfig(ctx)
		if err == nil {
			p.applyConfig(config)
		}
	}
	if p.client == nil {
		return fmt.Errorf("not configured")
	}
	_, err := p.client.FieldNames(ctx, "*", "", "")
	return err
}

func (p *QueryVLogsPlugin) ConfigUpdated(_ context.Context, config map[string]string) error {
	p.applyConfig(config)
	return nil
}

func (p *QueryVLogsPlugin) applyConfig(config map[string]string) {
	url := config["logs_url"]
	if url == "" {
		url = os.Getenv("MIRASTACK_LOGS_URL")
	}
	if url != "" {
		p.client = NewVLogsClient(url)
		if p.logger != nil {
			p.logger.Info("VictoriaLogs client updated", zap.String("url", url))
		}
	}
}

// enrichLogsOutput wraps raw log query results with metadata for LLM consumption.
// enrichLogsOutput wraps raw log results with metadata for LLM consumption.
// Return type is map[string]string to honour the plugin CallPlugin contract:
// the SDK unmarshals plugin responses into map[string]string and will fail
// on any non-string JSON value. All counts and booleans are stringified here.
func enrichLogsOutput(action, raw string) map[string]string {
	out := map[string]string{
		"action": action,
		"result": raw,
	}

	const maxLen = 32000
	if len(raw) > maxLen {
		out["result"] = raw[:maxLen]
		out["truncated"] = "true"
	}

	// For query action, count NDJSON lines (each non-empty line is a log entry).
	if action == "query" || action == "streams" {
		lines := strings.Split(strings.TrimSpace(raw), "\n")
		count := 0
		for _, l := range lines {
			if strings.TrimSpace(l) != "" {
				count++
			}
		}
		out["result_count"] = strconv.Itoa(count)
	}

	// For JSON array responses, extract count.
	var parsed any
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
		switch d := parsed.(type) {
		case []any:
			out["result_count"] = strconv.Itoa(len(d))
		case map[string]any:
			if values, ok := d["values"].([]any); ok {
				out["result_count"] = strconv.Itoa(len(values))
			}
		}
	}

	return out
}
