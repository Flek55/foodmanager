# syntax=docker/dockerfile:1
#
# Production image: one Go binary + the built frontend, on distroless
# (no shell, no package manager, runs as non-root). ~25 MB.
#
#   docker build --build-arg VERSION=$(git rev-parse --short HEAD) -t foodmanager .

# ---- 1. Frontend ----
FROM node:24-alpine AS web
WORKDIR /src/frontend
# Dependencies first: this layer is cached until package-lock.json changes.
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

# ---- 2. Backend ----
FROM golang:1.26-alpine AS server
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
ARG VERSION=dev
# CGO off: a fully static binary that runs on distroless/static.
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/server ./cmd/server

# ---- 3. Runtime ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=server /out/server /app/server
COPY --from=web /src/frontend/dist /app/web
ENV APP_ENV=prod \
    HTTP_ADDR=:8080 \
    STATIC_DIR=/app/web
EXPOSE 8080
USER nonroot
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD ["/app/server", "-healthcheck"]
ENTRYPOINT ["/app/server"]
