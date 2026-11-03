FROM node:22-alpine AS frontend-builder

RUN corepack enable

WORKDIR /app/frontend

COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN corepack pnpm install --frozen-lockfile

COPY frontend/ ./
RUN pnpm build

# Stage 2: Build Go binary
FROM golang:1.26-alpine AS go-builder

ARG VERSION=dev

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

COPY --from=frontend-builder /app/frontend/build ./frontend/build

RUN CGO_ENABLED=0 GOOS=linux go build \
	-ldflags="-X main.version=${VERSION}" \
	-o /app/bin/api ./cmd/api

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

# Run as an unprivileged user.
RUN adduser -D -H -u 10001 signet

WORKDIR /app

COPY --from=go-builder /app/bin/api ./api

USER signet

EXPOSE 8000

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
	CMD wget -qO- "http://127.0.0.1:${PORT:-8000}/v1/readiness" >/dev/null || exit 1

ENTRYPOINT ["./api"]
