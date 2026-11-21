# Build image version stays in sync with go.mod. Pinned to the multi-arch index
# digest of golang:1.27-alpine; refresh deliberately with:
# docker buildx imagetools inspect golang:1.27-alpine
ARG GO_IMAGE=golang:1.27-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280

# Compile on the native builder and cross-compile for the requested target.
FROM --platform=$BUILDPLATFORM ${GO_IMAGE} AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY go.mod ./
COPY cmd/ ./cmd/
COPY internal/ ./internal/
# Keep the original executable path for compatibility with existing deployments.
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/adguard-dns-monitor ./cmd/dns-monitor
# Create a world-writable /tmp so the heartbeat works even without a tmpfs mount.
RUN mkdir -p /out/tmp && chmod 1777 /out/tmp

# Scratch keeps the runtime minimal and avoids emulation for multi-platform builds.
FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/adguard-dns-monitor /usr/local/bin/adguard-dns-monitor
COPY --from=build --chmod=1777 /out/tmp /tmp
USER 10001:10001
HEALTHCHECK --interval=60s --timeout=5s --start-period=30s --retries=3 \
  CMD ["/usr/local/bin/adguard-dns-monitor", "-healthcheck"]
ENTRYPOINT ["/usr/local/bin/adguard-dns-monitor"]
