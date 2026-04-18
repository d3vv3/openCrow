# syntax=docker/dockerfile:1

# Build stage: Alpine is fine since CGO_ENABLED=0 produces a static binary
FROM golang:1.23-alpine AS build-base
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

FROM build-base AS build-dev
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=mod -o /out/server ./cmd/api

FROM build-base AS build-prod
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=mod -ldflags="-s -w" -o /out/server ./cmd/api

# Runtime base: minimal Alpine
FROM alpine:3.20 AS runtime-base
RUN addgroup -g 65532 nonroot && adduser -u 65532 -G nonroot -S -H nonroot
RUN apk add --no-cache ca-certificates tzdata bash wget
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
VOLUME ["/data", "/sandbox"]
EXPOSE 8080

# Dev runtime: hot-reload with CompileDaemon
FROM golang:1.23-alpine AS runtime-dev
WORKDIR /app
RUN apk add --no-cache ca-certificates tzdata bash wget
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
COPY go.mod go.sum ./
RUN go mod download
RUN go install github.com/githubnemo/CompileDaemon@latest
EXPOSE 8080
ENTRYPOINT ["/entrypoint.sh", "CompileDaemon", "-build=go build -mod=mod -o /tmp/server ./cmd/api", "-command=/tmp/server", "-graceful-kill=true", "-polling=true"]

FROM runtime-base AS runtime-prod
COPY --from=build-prod /out/server /server
ENTRYPOINT ["/entrypoint.sh", "/server"]
