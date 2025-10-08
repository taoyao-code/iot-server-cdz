# 构建阶段
FROM golang:1.22-alpine AS build

# 安装构建依赖
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /src

# 复制依赖文件并下载
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# 复制源代码
COPY . .

# 构建参数
ARG BUILD_VERSION=dev
ARG BUILD_TIME
ARG GIT_COMMIT

# 编译二进制文件（添加版本信息）
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.Version=${BUILD_VERSION} -X main.BuildTime=${BUILD_TIME} -X main.GitCommit=${GIT_COMMIT}" \
    -o /out/iot-server ./cmd/server

# 运行阶段
FROM gcr.io/distroless/base-debian12:nonroot

# 复制时区信息
COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

WORKDIR /app

# 复制二进制文件
COPY --from=build /out/iot-server /app/iot-server

# 复制配置文件（生产环境会被volume覆盖）
COPY configs/production.yaml /app/config.yaml
COPY configs/bkv_reason_map.yaml /app/configs/bkv_reason_map.yaml

# 创建日志目录
USER root
RUN mkdir -p /var/log/iot-server && chown nonroot:nonroot /var/log/iot-server
USER nonroot:nonroot

# 暴露端口
EXPOSE 8080 7000

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=40s --retries=3 \
  CMD ["/app/iot-server", "healthcheck"]

# 环境变量
ENV IOT_CONFIG=/app/config.yaml
ENV TZ=Asia/Shanghai

# 启动命令
ENTRYPOINT ["/app/iot-server"]


