# MIRASTACK Plugin: Query VLogs

Go plugin for querying **VictoriaLogs** using LogsQL from MIRASTACK workflows. Part of the core observability plugin suite.

The `v` prefix denotes Victoria-specific. Enterprise versions for other log backends follow the same plugin contract: `query-elogs` (Elasticsearch), `query-llogs` (Loki), etc.

## Capabilities

| Action | Description |
|--------|-------------|
| `query` | Execute LogsQL query (returns NDJSON) |
| `hits` | Time series of log hit counts |
| `field_names` | List indexed field names |
| `field_values` | List values for a specific field |
| `streams` | List log streams |
| `stats` | Server-side stats aggregation via LogsQL pipes |

## Configuration

The engine pushes configuration via `ConfigUpdated()`:

| Key | Description |
|-----|-------------|
| `logs_url` | VictoriaLogs base URL (e.g., `http://victorialogs:9428`) |

## Example Workflow Step

```yaml
- id: find-errors
  type: plugin
  plugin: query_vlogs
  params:
    action: query
    query: "service_name:api-gateway AND level:error"
    start: "-1h"
    end: "now"
    limit: "50"
```

## Development

```bash
go build -o mirastack-plugin-query-vlogs .
```

## Requirements

- Go 1.23+
- mirastack-sdk-go

## License

AGPL v3 — see [LICENSE](LICENSE).
