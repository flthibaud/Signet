# Stage 1: Build frontend
FROM node:22-alpine AS frontend-builder

RUN corepack enable

WORKDIR /app/frontend

COPY frontend/package.json frontend/pnpm-lock.yaml ./
# pnpm version is pinned via the "packageManager" field in package.json
RUN corepack pnpm install --frozen-lockfile

COPY frontend/ ./
RUN pnpm build

# Stage 2: Build Go binary
FROM golang:1.25-alpine AS go-builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Copy built frontend assets for go:embed
COPY --from=frontend-builder /app/frontend/build ./frontend/build

RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/api ./cmd/api

# Stage 3: Final image
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

# Run as an unprivileged user. -H skips the home directory: the app writes
# nothing to disk, so the image can also run with a read-only root filesystem
# (see docker-compose.yml).
RUN adduser -D -H -u 10001 signet

WORKDIR /app

# Owned by root, readable/executable by everyone: the runtime user must not be
# able to overwrite its own binary or the migrations.
COPY --from=go-builder /app/bin/api ./api
COPY migrations ./migrations

USER signet

EXPOSE 8000

# Readiness, not liveness: an instance that cannot reach Postgres is not healthy
# even though the process is fine. The shell form is what expands PORT at
# runtime. A GET (not --spider's HEAD) because the router registers GET only.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
	CMD wget -qO- "http://127.0.0.1:${PORT:-8000}/v1/readiness" >/dev/null || exit 1

ENTRYPOINT ["./api"]
