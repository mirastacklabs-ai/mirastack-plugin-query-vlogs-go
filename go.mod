module github.com/mirastacklabs-ai/mirastack-plugin-query-vlogs

go 1.23.0

require (
	github.com/mirastacklabs-ai/mirastack-agents-sdk-go v0.1.0
	go.uber.org/zap v1.27.0
)

require (
	github.com/google/uuid v1.6.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/net v0.35.0 // indirect
	golang.org/x/sys v0.30.0 // indirect
	golang.org/x/text v0.22.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250218202821-56aae31c358a // indirect
	google.golang.org/grpc v1.72.1 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
)

replace github.com/mirastacklabs-ai/mirastack-agents-sdk-go => ../../../sdk/oss/agent-sdk/mirastack-agents-sdk-go
