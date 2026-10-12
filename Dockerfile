# syntax=docker/dockerfile:1

# Denova container image: bundles the embedded web frontend, bundled ripgrep,
# skills and the startup config into a single runtime based on Node.js 22
# (plugin and game backends require Node >= 22).
ARG GO_VERSION=1.26
ARG NODE_VERSION=22

FROM --platform=$BUILDPLATFORM node:${NODE_VERSION}-bookworm-slim AS web
WORKDIR /src/web
RUN npm install --global pnpm@11.15.1
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
COPY web/patches ./patches
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-bookworm AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ENV CGO_ENABLED=0 \
    GOTOOLCHAIN=auto
WORKDIR /src
COPY go.mod go.sum ./
COPY agent/go.mod agent/go.sum ./agent/
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
COPY --from=web /src/web/dist internal/webfs/dist
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -tags embedweb -trimpath \
    -ldflags "-s -w -X denova/internal/buildinfo.Version=${VERSION}" \
    -o /out/denova ./cmd/denova
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath \
    -ldflags "-s -w -X denova/internal/buildinfo.Version=${VERSION}" \
    -o /out/denova-updater ./cmd/denova-updater
RUN --mount=type=cache,target=/go/pkg/mod \
    go run ./scripts/ripgrep-assets \
    -target "$([ "$TARGETARCH" = "arm64" ] && echo linux-arm64 || echo linux-x64)" \
    -destination /out

FROM node:${NODE_VERSION}-bookworm-slim AS runtime
ENV DENOVA_DIR=/data/.denova \
    TZ=Asia/Shanghai
WORKDIR /app
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/denova ./denova
COPY --from=build /out/denova-updater ./denova-updater
COPY --from=build /out/tools ./tools
COPY --from=build /out/licenses ./licenses
COPY skills ./skills
COPY config.toml CHANGELOG.md ./
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod 0755 /usr/local/bin/docker-entrypoint.sh /app/denova /app/denova-updater /app/tools/rg
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["--port", "8080", "--no-open"]
