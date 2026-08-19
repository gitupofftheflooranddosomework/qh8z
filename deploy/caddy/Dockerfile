FROM golang:1.25.13-alpine AS build
WORKDIR /src
RUN go mod init qh8z/caddy-build \
    && go get github.com/caddyserver/caddy/v2/cmd/caddy@v2.11.4 \
    && go get golang.org/x/net@v0.56.0 \
        golang.org/x/text@v0.39.0 \
        google.golang.org/grpc@v1.82.1
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/caddy github.com/caddyserver/caddy/v2/cmd/caddy

FROM alpine:3.24
RUN apk add --no-cache ca-certificates mailcap
RUN mkdir -p /config/caddy /data/caddy /etc/caddy /usr/share/caddy \
    && chmod 1777 /config/caddy /data/caddy
COPY --from=build /out/caddy /usr/bin/caddy
ENV XDG_CONFIG_HOME=/config
ENV XDG_DATA_HOME=/data
EXPOSE 80 443 443/udp 2019
WORKDIR /srv
CMD ["caddy", "run", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile"]
