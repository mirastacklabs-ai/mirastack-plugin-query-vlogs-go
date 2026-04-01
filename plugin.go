package main

import (
	"context"
	"fmt"

	"github.com/mirastacklabs-ai/mirastack-sdk-go"
	"go.uber.org/zap"
)

// QueryVLogsPlugin queries VictoriaLogs using the LogsQL API.
// The "v" prefix denotes Victoria-specific. Enterprise versions for other log backends
// (Elasticsearch, Loki, etc.) will follow the same plugin contract with a different prefix.
type QueryVLogsPlugin struct {
	client *VLogsClient
	logger *zap.Logger
}

func (p *QueryVLogsPlugin) Info() *mirastack.PluginInfo {
	return &mirastack.PluginInfo{
		Name:         "query_vlogs",
		Version:      "0.1.0",
		Description:  "Search and analyze logs from VictoriaLogs using LogsQL. Supports log search, hit count time series, field discovery, field value enumeration, stream listing, and server-side stats aggregation.",
		Permissions:  []mirastack.Permission{mirastack.PermissionRead},
		DevOpsStages: []mirastack.DevOpsStage{mirastack.StageObserve},
		Intents: []mirastack.IntentPattern{
			{Pattern: "search logs", Description: "Search log entries", Priority: 10},
			{Pattern: "find errors in logs", Description: "Search for error-level log entries", Priority: 9},
			{Pattern: "log field values", Description: "List field values from logs", Priority: 5},
		},
	}
}

func (p *QueryVLogsPlugin) Schema() *mirastack.PluginSchema {
	return &mirastack.PluginSchema{
		InputParams: []mirastack.ParamSchema{
			{Name: "action", Type: "string", Required: true, Description: "One of: query, hits, field_names, field_values, streams, stats"},
			{Name: "query", Type: "string", Required: false, Description: "LogsQL query expression (e.g., 'service_name:api-gateway AND level:error')"},
			{Name: "start", Type: "string", Required: false, Description: "Start time (RFC3339 or relative like -1h)"},
			{Name: "end", Type: "string", Required: false, Description: "End time (RFC3339 or 'now')"},
			{Name: "limit", Type: "string", Required: false, Description: "Maximum number of log entries to return (default: 100)"},
			{Name: "field", Type: "string", Required: false, Description: "Field name for field_values action; also used with hits for grouping"},
			{Name: "step", Type: "string", Required: false, Description: "Time bucket step for hits action (e.g., 1m, 5m, 1h)"},
		},
		OutputParams: []mirastack.ParamSchema{
			{Name: "result", Type: "json", Required: true, Description: "Query result in VictoriaLogs NDJSON response format"},
		},
	}
}

func (p *QueryVLogsPlugin) Execute(ctx context.Context, req *mirastack.ExecuteRequest) (*mirastack.ExecuteResponse, error) {
	if p.logger == nil {
		p.logger, _ = zap.NewProduction()
	}

	action := req.Params["action"]
	if action == "" {
		return &mirastack.ExecuteResponse{
			Output: map[string]string{"error": "action parameter is required"},
			Logs:   []string{"missing required parameter: action"},
		}, nil
	}

	result, err := p.dispatch(ctx, action, req.Params)
	if err != nil {
		return &mirastack.ExecuteResponse{
			Output: map[string]string{"error": err.Error()},
			Logs:   []string{fmt.Sprintf("action %s failed: %v", action, err)},
		}, nil
	}

	return &mirastack.ExecuteResponse{
		Output: map[string]string{"result": result},
		Logs:   []string{fmt.Sprintf("action %s completed", action)},
	}, nil
}

func (p *QueryVLogsPlugin) dispatch(ctx context.Context, action string, params map[string]string) (string, error) {
	if p.client == nil {
		return "", fmt.Errorf("plugin not configured: logs_url not set")
	}

	switch action {
	case "query":
		return p.actionQuery(ctx, params)
	case "hits":
		return p.actionHits(ctx, params)
	case "field_names":
		return p.actionFieldNames(ctx, params)
	case "field_values":
		return p.actionFieldValues(ctx, params)
	case "streams":
		return p.actionStreams(ctx, params)
	case "stats":
		return p.actionStats(ctx, params)
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

func (p *QueryVLogsPlugin) HealthCheck(ctx context.Context) error {
	if p.client == nil {
		return fmt.Errorf("not configured")
	}
	_, err := p.client.FieldNames(ctx, "*", "", "")
	return err
}

func (p *QueryVLogsPlugin) ConfigUpdated(_ context.Context, config map[string]string) error {
	if url, ok := config["logs_url"]; ok && url != "" {
		p.client = NewVLogsClient(url)
		if p.logger != nil {
			p.logger.Info("VictoriaLogs client updated", zap.String("url", url))
		}
	}
	return nil
}
