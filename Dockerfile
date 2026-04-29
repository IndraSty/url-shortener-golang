# ============================================================
# Stage 1: Builder
# Uses the full Go toolchain to compile the binary.
# ============================================================
FROM golang:1.26-alpine AS builder

# Install git and ca-certificates (needed for go get and HTTPS)
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /build

# Copy dependency files first — Docker layer cache means this only
# re-runs when go.mod or go.sum changes, not on every code change.
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build — CGO disabled for a fully static binary (no libc dependency)
# ldflags: strip debug info and symbol table to reduce binary size
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o /build/url-shortener \
    ./cmd/api/main.go

# ============================================================
# Stage 2: Final image
# Minimal scratch-based image — no shell, no package manager.
# ============================================================
FROM scratch

# Copy TLS certificates from builder (needed for HTTPS to Neon, Upstash, QStash)
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy the compiled binary
COPY --from=builder /build/url-shortener /url-shortener

# Copy migrations — the binary runs migrations on startup
COPY --from=builder /build/migrations /migrations

# Run as non-root for security
# scratch images don't have useradd, so we use a numeric UID
USER 65534:65534

EXPOSE 8080

ENTRYPOINT ["/url-shortener"]