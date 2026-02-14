# ── Stage 1: Build Admin GUI ────────────────────────
FROM node:20-alpine AS web-builder

WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci --ignore-scripts
COPY web/ .
RUN npm run build          # outputs to /web/dist/

# ── Stage 2: Build Go Server ────────────────────────
FROM golang:1.25-alpine AS go-builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Embed the pre-built GUI into the Go binary
COPY --from=web-builder /web/dist/ /build/web/dist/
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /registry ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /healthcheck ./cmd/healthcheck

# ── Stage 3: Runtime ────────────────────────────────
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -u 1001 registry

COPY --from=go-builder /registry /registry
COPY --from=go-builder /healthcheck /healthcheck

USER 1001
EXPOSE 8090

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s CMD ["/healthcheck"]
ENTRYPOINT ["/registry"]
