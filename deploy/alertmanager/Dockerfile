FROM node:24-alpine AS ui
RUN apk add --no-cache git
RUN git clone --depth 1 --branch v0.33.1 https://github.com/prometheus/alertmanager.git /src
WORKDIR /src/ui/app
RUN npm ci --no-audit --no-fund && npm run build

FROM golang:1.26.6-alpine AS build
COPY --from=ui /src /src
WORKDIR /src
RUN go get \
    golang.org/x/crypto@v0.55.0 \
    golang.org/x/mod@v0.40.0 \
    golang.org/x/net@v0.58.0 \
    golang.org/x/text@v0.41.0 \
    google.golang.org/grpc@v1.82.1
RUN CGO_ENABLED=0 GOOS=linux go build \
    -tags netgo \
    -trimpath \
    -ldflags="-s -w -X github.com/prometheus/common/version.Version=0.33.1-qh8z-patched" \
    -o /out/alertmanager ./cmd/alertmanager
RUN CGO_ENABLED=0 GOOS=linux go build \
    -tags netgo \
    -trimpath \
    -ldflags="-s -w -X github.com/prometheus/common/version.Version=0.33.1-qh8z-patched" \
    -o /out/amtool ./cmd/amtool

FROM alpine:3.24
RUN apk add --no-cache ca-certificates tzdata \
    && mkdir -p /etc/alertmanager /alertmanager \
    && chown -R nobody:nobody /etc/alertmanager /alertmanager \
    && chmod g+w /alertmanager
COPY --from=build /out/alertmanager /bin/alertmanager
COPY --from=build /out/amtool /bin/amtool
COPY --from=build /src/examples/ha/alertmanager.yml /etc/alertmanager/alertmanager.yml
USER nobody
EXPOSE 9093
VOLUME ["/alertmanager"]
WORKDIR /alertmanager
ENTRYPOINT ["/bin/alertmanager"]
CMD ["--config.file=/etc/alertmanager/alertmanager.yml", "--storage.path=/alertmanager"]
