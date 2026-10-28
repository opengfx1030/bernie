# bernie container image: the static binary on a minimal base.
#
# Build from the repository root:
#   docker build -f deploy/docker/Dockerfile -t bernie .
# Multi-arch (the build stage always runs natively and cross-compiles):
#   docker buildx build --platform linux/amd64,linux/arm64 -f deploy/docker/Dockerfile .
#
# Run (see docs/install.md and deploy/compose/ for the full set of options):
#   docker run --rm -p 127.0.0.1:7860:7860 -e BERNIE_DASH_TOKEN=... \
#     --device /dev/kfd --device /dev/dri --group-add <render-gid> --group-add <video-gid> \
#     --security-opt seccomp=unconfined --security-opt label=disable \
#     -v "$PWD/data:/data" -v /sys:/sys:ro ghcr.io/opengfx1030/bernie

FROM --platform=$BUILDPLATFORM golang:1.26 AS build
ARG TARGETOS=linux
ARG TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath \
      -ldflags "-s -w -X github.com/opengfx1030/bernie/internal/cli.Version=${VERSION}" \
      -o /out/bernie ./cmd/bernie \
 && mkdir -p /out/data/config /out/data/state /out/data/data /out/data/cache

# distroless/static: no shell, no package manager, about 2 MB. It adds what a
# bare scratch image lacks and bernie needs: CA certificates (HTTPS to Hugging
# Face and GitHub), tzdata, and a non-root user (uid 65532) with a passwd entry.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/bernie /bernie
COPY --from=build --chown=65532:65532 /out/data /data
ENV HOME=/data \
    BERNIE_CONFIG_DIR=/data/config \
    BERNIE_STATE_DIR=/data/state \
    BERNIE_DATA_DIR=/data/data \
    BERNIE_CACHE_DIR=/data/cache
LABEL org.opencontainers.image.title="bernie" \
      org.opencontainers.image.description="Run vLLM on AMD ROCm GPUs: CLI and web dashboard" \
      org.opencontainers.image.source="https://github.com/opengfx1030/bernie" \
      org.opencontainers.image.licenses="Apache-2.0"
VOLUME ["/data"]
EXPOSE 7860
# Non-loopback listen address: the dashboard needs a token. Set BERNIE_DASH_TOKEN,
# or read the random one it prints at start (`docker logs`).
ENTRYPOINT ["/bernie"]
CMD ["dash", "--listen", "0.0.0.0:7860"]
