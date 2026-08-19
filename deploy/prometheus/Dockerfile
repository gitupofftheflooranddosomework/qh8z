FROM node:22-alpine AS ui
RUN apk add --no-cache bash git
RUN corepack enable
RUN git clone --depth 1 --branch v3.13.2 https://github.com/prometheus/prometheus.git /src
WORKDIR /src/web/ui
RUN pnpm install --frozen-lockfile
RUN cd react-app && pnpm install --frozen-lockfile
RUN CI="" pnpm run build

FROM golang:1.26.6-alpine AS build
RUN apk add --no-cache bash coreutils findutils gzip
COPY --from=ui /src /src
WORKDIR /src
RUN bash scripts/compress_assets.sh
RUN go get \
    golang.org/x/crypto@v0.52.0 \
    golang.org/x/net@v0.56.0 \
    golang.org/x/text@v0.39.0 \
    google.golang.org/grpc@v1.82.1
RUN CGO_ENABLED=0 GOOS=linux go build \
    -tags netgo,builtinassets \
    -trimpath \
    -ldflags="-s -w -X github.com/prometheus/common/version.Version=3.13.2-qh8z-patched" \
    -o /out/prometheus ./cmd/prometheus
RUN CGO_ENABLED=0 GOOS=linux go build \
    -tags netgo,builtinassets \
    -trimpath \
    -ldflags="-s -w -X github.com/prometheus/common/version.Version=3.13.2-qh8z-patched" \
    -o /out/promtool ./cmd/promtool

FROM alpine:3.24
RUN apk add --no-cache ca-certificates tzdata \
    && mkdir -p /etc/prometheus /prometheus \
    && chown -R nobody:nobody /etc/prometheus /prometheus \
    && chmod g+w /prometheus
COPY --from=build /out/prometheus /bin/prometheus
COPY --from=build /out/promtool /bin/promtool
COPY --from=build /src/documentation/examples/prometheus.yml /etc/prometheus/prometheus.yml
USER nobody
EXPOSE 9090
VOLUME ["/prometheus"]
WORKDIR /prometheus
ENTRYPOINT ["/bin/prometheus"]
CMD ["--config.file=/etc/prometheus/prometheus.yml", "--storage.tsdb.path=/prometheus"]
