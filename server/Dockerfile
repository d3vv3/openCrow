# syntax=docker/dockerfile:1

FROM golang:1.23-alpine AS build
WORKDIR /app

COPY . .
RUN go mod tidy
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=mod -o /out/server ./cmd/api

FROM alpine:3.20
RUN addgroup -g 65532 nonroot && adduser -u 65532 -G nonroot -D nonroot
RUN apk add --no-cache bash python3 py3-pip nodejs npm tzdata
COPY --from=build /out/server /server
VOLUME ["/data"]
EXPOSE 8080
# USER nonroot:nonroot
ENTRYPOINT ["/server"]
