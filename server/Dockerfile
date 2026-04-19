# syntax=docker/dockerfile:1

# ── Build stage ───────────────────────────────────────────────────────────────
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build-base
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go run github.com/swaggo/swag/cmd/swag@latest init -d .,internal -g cmd/api/main.go

FROM build-base AS build-dev
RUN CGO_ENABLED=0 go build -mod=mod -o /out/server ./cmd/api

FROM build-base AS build-prod
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -mod=mod -ldflags="-s -w" -o /out/server ./cmd/api

# ── Grab the migrate binary for the target platform ──────────────────────────
FROM --platform=$TARGETPLATFORM migrate/migrate:v4.18.3 AS migrate-tool

# ── Runtime base ─────────────────────────────────────────────────────────────
FROM alpine:3.20 AS runtime-base
RUN addgroup -g 65532 nonroot && adduser -u 65532 -G nonroot -S -H nonroot
RUN apk add --no-cache ca-certificates tzdata bash wget
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
VOLUME ["/data", "/sandbox"]
EXPOSE 8080

# ── Dev runtime: hot-reload with CompileDaemon ───────────────────────────────
FROM golang:1.25-alpine AS runtime-dev
WORKDIR /app
RUN apk add --no-cache ca-certificates tzdata bash wget
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
COPY go.mod go.sum ./
RUN go mod download
RUN go install github.com/githubnemo/CompileDaemon@latest
EXPOSE 8080
ENTRYPOINT ["/entrypoint.sh", "CompileDaemon", "-build=go build -mod=mod -o /tmp/server ./cmd/api", "-command=/tmp/server", "-graceful-kill=true", "-polling=true"]

# ── Production runtime ───────────────────────────────────────────────────────
FROM runtime-base AS runtime-prod
COPY --from=migrate-tool /usr/local/bin/migrate /usr/local/bin/migrate
COPY --from=build-prod /out/server /server
COPY migrations /migrations
ENTRYPOINT ["/entrypoint.sh", "/server"]
