FROM golang:1.25.13-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/qh8z ./cmd/qh8z
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/healthcheck ./cmd/healthcheck

FROM scratch
COPY --from=build /out/qh8z /qh8z
COPY --from=build /out/healthcheck /healthcheck
EXPOSE 8080
USER 65532:65532
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 CMD ["/healthcheck"]
ENTRYPOINT ["/qh8z"]
