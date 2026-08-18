FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/qh8z ./cmd/qh8z

FROM scratch
COPY --from=build /out/qh8z /qh8z
EXPOSE 8080
USER 65532:65532
ENTRYPOINT ["/qh8z"]
