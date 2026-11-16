FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/adguard-dns-monitor .

FROM alpine:3.22
RUN apk add --no-cache ca-certificates \
    && addgroup -S -g 10001 app \
    && adduser -S -D -H -u 10001 -G app app
COPY --from=build /out/adguard-dns-monitor /usr/local/bin/adguard-dns-monitor
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/adguard-dns-monitor"]
