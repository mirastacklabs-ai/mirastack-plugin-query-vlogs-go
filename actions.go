package main

import (
	"context"
	"fmt"

	mirastack "github.com/mirastacklabs-ai/mirastack-agents-sdk-go"
	"github.com/mirastacklabs-ai/mirastack-agents-sdk-go/datetimeutils"
)

// resolveStartEnd returns start/end strings, preferring engine-parsed TimeRange.
func resolveStartEnd(params map[string]string, tr *mirastack.TimeRange) (start, end string) {
	if tr != nil && tr.StartEpochMs > 0 {
		return datetimeutils.FormatRFC3339(tr.StartEpochMs), datetimeutils.FormatRFC3339(tr.EndEpochMs)
	}
	return params["start"], params["end"]
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
