FROM golang:1.23-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/qh8z ./cmd/qh8z

FROM scratch
COPY --from=build /out/qh8z /qh8z
EXPOSE 8080
ENTRYPOINT ["/qh8z"]
