# DialCatz Den — one small image: the Go binary with the admin panel embedded, on distroless (no shell),
# running as non-root. Builds from a plain clone of this repository (Dokploy / Coolify "from Git", docker compose
# build), nothing prebuilt needed. The version comes from the VERSION file.
# Multi-arch: docker buildx build --platform linux/amd64,linux/arm64 -t ghcr.io/catzoidhub/dialcatz-den:<v> --push .

# 1. Admin panel (Vue) → internal/web/dist
FROM oven/bun:1.3 AS admin
WORKDIR /src/admin
COPY admin/package.json admin/bun.lock ./
RUN bun install --frozen-lockfile
COPY admin/ ./
RUN bun run build

# 2. Server binary (pure Go: modernc SQLite, no cgo)
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS server
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=admin /src/internal/web/dist ./internal/web/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
      -ldflags "-s -w -X github.com/catzoidhub/DialCatz-Den/internal/config.Version=$(cat VERSION)" \
      -o /out/dialcatz-den ./cmd/dialcatz-den \
 && mkdir -p /out/data

# 3. Runtime
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=server /out/dialcatz-den /dialcatz-den
COPY --from=server --chown=65532:65532 /out/data /data
ENV DIALCATZ_DATA_DIR=/data DIALCATZ_ADDR=:8080
VOLUME /data
EXPOSE 8080
USER 65532:65532
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s CMD ["/dialcatz-den", "healthcheck"]
ENTRYPOINT ["/dialcatz-den"]
