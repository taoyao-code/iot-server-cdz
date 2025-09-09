FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/iot-server ./cmd/server

FROM gcr.io/distroless/base-debian12:nonroot
WORKDIR /app
COPY --from=build /out/iot-server /app/iot-server
COPY configs/example.yaml /app/config.yaml
USER nonroot:nonroot
EXPOSE 8080 7000
ENV IOT_CONFIG=/app/config.yaml
ENTRYPOINT ["/app/iot-server"]


