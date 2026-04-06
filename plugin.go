package main

import (
	"context"
	"fmt"

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
		Name:         "query_vlogs",
		Version:      "0.1.0",
		Description:  "Search and analyze logs from VictoriaLogs using LogsQL. Supports log search, hit count time series, field discovery, field value enumeration, stream listing, and server-side stats aggregation.",
		Permissions:  []mirastack.Permission{mirastack.PermissionRead},
		DevOpsStages: []mirastack.DevOpsStage{mirastack.StageObserve},
		Actions: []mirastack.Action{
			{
				ID:          "query",
				Description: "Search log entries using LogsQL expressions",
				Permission:  mirastack.PermissionRead,
				Stages:      []mirastack.DevOpsStage{mirastack.StageObserve},
				InputParams: []mirastack.ParamSchema{
					{Name: "query", Type: "string", Required: true, Description: "LogsQL query expression"},
					{Name: "limit", Type: "string", Required: false, Description: "Maximum number of log entries to return (default: 100)"},
				},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Log entries in VictoriaLogs NDJSON format"},
				},
			},
			{
				ID:          "hits",
				Description: "Get log hit count time series (histogram)",
				Permission:  mirastack.PermissionRead,
				Stages:      []mirastack.DevOpsStage{mirastack.StageObserve},
				InputParams: []mirastack.ParamSchema{
					{Name: "query", Type: "string", Required: false, Description: "LogsQL filter expression"},
					{Name: "step", Type: "string", Required: false, Description: "Time bucket step (e.g., 1m, 5m, 1h)"},
					{Name: "field", Type: "string", Required: false, Description: "Field to group hits by"},
				},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Hit count time series"},
				},
			},
			{
				ID:          "field_names",
				Description: "List all field names present in logs",
				Permission:  mirastack.PermissionRead,
				Stages:      []mirastack.DevOpsStage{mirastack.StageObserve},
				InputParams: []mirastack.ParamSchema{
					{Name: "query", Type: "string", Required: false, Description: "LogsQL filter to scope fields"},
				},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Array of field names"},
				},
			},
			{
				ID:          "field_values",
				Description: "List values for a specific log field",
				Permission:  mirastack.PermissionRead,
				Stages:      []mirastack.DevOpsStage{mirastack.StageObserve},
				InputParams: []mirastack.ParamSchema{
					{Name: "field", Type: "string", Required: true, Description: "Field name to get values for"},
					{Name: "query", Type: "string", Required: false, Description: "LogsQL filter to scope values"},
				},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Array of field values"},
				},
			},
			{
				ID:          "streams",
				Description: "List log streams (unique label combinations)",
				Permission:  mirastack.PermissionRead,
				Stages:      []mirastack.DevOpsStage{mirastack.StageObserve},
				InputParams: []mirastack.ParamSchema{
					{Name: "query", Type: "string", Required: false, Description: "LogsQL filter to scope streams"},
				},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Array of log streams"},
				},
			},
			{
				ID:          "stats",
				Description: "Aggregate server-side statistics from logs",
				Permission:  mirastack.PermissionRead,
				Stages:      []mirastack.DevOpsStage{mirastack.StageObserve},
				InputParams: []mirastack.ParamSchema{
					{Name: "query", Type: "string", Required: true, Description: "LogsQL stats expression"},
				},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Aggregated statistics result"},
				},
			},
		},
		Intents: []mirastack.IntentPattern{
			{Pattern: "search logs", Description: "Search log entries", Priority: 10},
			{Pattern: "find errors in logs", Description: "Search for error-level log entries", Priority: 9},
			{Pattern: "log field values", Description: "List field values from logs", Priority: 5},
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

	resp, _ := mirastack.RespondMap(map[string]any{"result": result})
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
	if url, ok := config["logs_url"]; ok && url != "" {
		p.client = NewVLogsClient(url)
		if p.logger != nil {
			p.logger.Info("VictoriaLogs client updated", zap.String("url", url))
		}
	}
}
