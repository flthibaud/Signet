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

WORKDIR /app

COPY --from=go-builder /app/bin/api ./api
COPY migrations ./migrations

EXPOSE 8000

ENTRYPOINT ["./api"]
