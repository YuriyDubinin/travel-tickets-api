# syntax=docker/dockerfile:1

# ---- Builder ----
# Run the builder on the native build platform (fast); cross-compile to the
# requested target platform below.
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

RUN apk add --no-cache git

WORKDIR /build

# Download dependencies first to leverage Docker layer caching.
COPY go.mod go.sum ./
RUN go mod download

# Cross-compile the statically-linked binary for the target platform. TARGETOS /
# TARGETARCH are provided automatically by BuildKit from --platform, so the Go
# toolchain runs natively while emitting a binary for the target architecture.
COPY . .
ARG TARGETOS TARGETARCH
RUN mkdir -p /app \
 && CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -ldflags="-s -w" -o /app/api ./cmd/api

# ---- Runtime ----
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata wget \
 && adduser -D -u 1000 app

# Binary and migrations from the builder stage.
COPY --from=builder /app/api /app/api
COPY --from=builder /build/migrations /app/migrations

WORKDIR /app
USER app

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -q -O /dev/null http://localhost:8080/api/ping || exit 1

CMD ["/app/api"]
