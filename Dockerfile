# MIRASTACK Plugin — Query VLogs Go (multi-arch: linux/amd64, linux/arm64)
#
# Build:
#   docker buildx build --platform linux/amd64,linux/arm64 \
#     -f agents/oss/mirastack-plugin-query-vlogs-go/Dockerfile .

FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

# Copy plugin module
COPY agents/oss/mirastack-plugin-query-vlogs-go/go.mod agents/oss/mirastack-plugin-query-vlogs-go/go.sum* agents/oss/mirastack-plugin-query-vlogs-go/
WORKDIR /src/agents/oss/mirastack-plugin-query-vlogs-go
RUN go mod download

WORKDIR /src
COPY agents/oss/mirastack-plugin-query-vlogs-go/ agents/oss/mirastack-plugin-query-vlogs-go/

WORKDIR /src/agents/oss/mirastack-plugin-query-vlogs-go
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -ldflags "-s -w" -o /mirastack-plugin-query-vlogs .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /mirastack-plugin-query-vlogs /usr/local/bin/mirastack-plugin-query-vlogs
EXPOSE 50051
ENTRYPOINT ["mirastack-plugin-query-vlogs"]
