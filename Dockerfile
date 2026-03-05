# Stage 1: Build
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /collector ./cmd/collector

# Stage 2: Run (minimal image with CA certs for HTTPS)
FROM alpine:3.19
RUN apk --no-cache add ca-certificates && \
    adduser -D -u 1000 collector
USER collector
COPY --from=builder /collector /collector
ENTRYPOINT ["/collector"]
