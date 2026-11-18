# Compile on the native builder and cross-compile for the requested target.
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY go.mod ./
COPY cmd/ ./cmd/
COPY internal/ ./internal/
# Keep the original executable path for compatibility with existing deployments.
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /out/adguard-dns-monitor ./cmd/dns-monitor

# Scratch keeps the runtime minimal and avoids emulation for multi-platform builds.
FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/adguard-dns-monitor /usr/local/bin/adguard-dns-monitor
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/adguard-dns-monitor"]
