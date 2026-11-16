# Compile on the native builder and cross-compile for the requested target.
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /out/local-dns-drp .

# Scratch keeps the runtime minimal and avoids emulation for multi-platform builds.
FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/local-dns-drp /usr/local/bin/local-dns-drp
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/local-dns-drp"]
