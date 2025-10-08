# 构建阶段 - 使用最新稳定版 Go
FROM golang:alpine AS build

# 配置国内镜像源加速（阿里云）
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories && \
    apk add --no-cache git ca-certificates tzdata

# 配置 Go 模块代理（使用 goproxy.cn 加速）
ENV GO111MODULE=on \
    GOPROXY=https://goproxy.cn,direct \
    GOSUMDB=sum.golang.org

WORKDIR /src

# 复制依赖文件并下载
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# 只复制必要的源代码目录
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY configs/ ./configs/

# 更新依赖（确保 go.mod 与代码同步）
RUN go mod tidy

# 构建参数
ARG BUILD_VERSION=dev
ARG BUILD_TIME
ARG GIT_COMMIT

# 编译二进制文件（添加版本信息）
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.Version=${BUILD_VERSION} -X main.BuildTime=${BUILD_TIME} -X main.GitCommit=${GIT_COMMIT}" \
    -o /out/iot-server ./cmd/server

# 运行阶段 - 使用 Debian（国内访问稳定）
FROM debian:12-slim

# 配置国内镜像源（阿里云）
RUN sed -i 's/deb.debian.org/mirrors.aliyun.com/g' /etc/apt/sources.list.d/debian.sources && \
    apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates tzdata curl && \
    rm -rf /var/lib/apt/lists/* && \
    useradd -u 65532 -r -s /bin/false nonroot

WORKDIR /app

# 复制二进制文件
COPY --from=build /out/iot-server /app/iot-server

# 复制配置文件（生产环境会被volume覆盖）
COPY --from=build /src/configs/production.yaml /app/config.yaml
COPY --from=build /src/configs/bkv_reason_map.yaml /app/configs/bkv_reason_map.yaml

# 创建日志目录
RUN mkdir -p /var/log/iot-server && \
    chown -R nonroot:nonroot /var/log/iot-server /app

# 切换到非root用户
USER nonroot

# 暴露端口
EXPOSE 8080 7000

# 环境变量
ENV IOT_CONFIG=/app/config.yaml
ENV TZ=Asia/Shanghai

# 启动命令
ENTRYPOINT ["/app/iot-server"]


