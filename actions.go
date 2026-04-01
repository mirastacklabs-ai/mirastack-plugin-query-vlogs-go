package main

import (
	"context"
	"fmt"
)

// Action handlers for the query_vlogs plugin.
// Each action maps to a VictoriaLogs LogsQL API endpoint.

func (p *QueryVLogsPlugin) actionQuery(ctx context.Context, params map[string]string) (string, error) {
	query := params["query"]
	if query == "" {
		return "", fmt.Errorf("query parameter is required for query action")
	}
	limit := params["limit"]
	if limit == "" {
		limit = "100"
	}
	return p.client.Query(ctx, query, params["start"], params["end"], limit)
}

func (p *QueryVLogsPlugin) actionHits(ctx context.Context, params map[string]string) (string, error) {
	query := params["query"]
	if query == "" {
		query = "*"
	}
	step := params["step"]
	if step == "" {
		step = "5m"
	}
	return p.client.Hits(ctx, query, params["start"], params["end"], step, params["field"])
}

func (p *QueryVLogsPlugin) actionFieldNames(ctx context.Context, params map[string]string) (string, error) {
	query := params["query"]
	if query == "" {
		query = "*"
	}
	return p.client.FieldNames(ctx, query, params["start"], params["end"])
}

func (p *QueryVLogsPlugin) actionFieldValues(ctx context.Context, params map[string]string) (string, error) {
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
	return p.client.FieldValues(ctx, query, field, params["start"], params["end"], limit)
}

func (p *QueryVLogsPlugin) actionStreams(ctx context.Context, params map[string]string) (string, error) {
	query := params["query"]
	if query == "" {
		query = "*"
	}
	limit := params["limit"]
	if limit == "" {
		limit = "100"
	}
	return p.client.Streams(ctx, query, params["start"], params["end"], limit)
}

func (p *QueryVLogsPlugin) actionStats(ctx context.Context, params map[string]string) (string, error) {
	query := params["query"]
	if query == "" {
		return "", fmt.Errorf("query parameter is required for stats action (use LogsQL with | stats pipe)")
	}
	return p.client.StatsQuery(ctx, query, params["start"], params["end"])
}
