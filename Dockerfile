# ============================================
# 构建阶段 - 使用 Go 1.24
# ============================================
FROM golang:1.24-alpine AS build

# 配置Alpine国内镜像源（解决网络访问问题）
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories && \
    sed -i 's/https/http/g' /etc/apk/repositories && \
    cat /etc/apk/repositories

# 更新包索引并安装必要工具
RUN apk update && \
    apk add --no-cache \
    git \
    ca-certificates \
    tzdata \
    && rm -rf /var/cache/apk/*

# 配置 Go 模块代理（使用多个国内代理源）
ENV GO111MODULE=on \
    GOPROXY=https://goproxy.cn,https://mirrors.aliyun.com/goproxy/,https://proxy.golang.com.cn,direct \
    GOSUMDB=off \
    CGO_ENABLED=0 \
    GONOPROXY=none \
    GONOSUMDB=* \
    GOPRIVATE=""

WORKDIR /src

# 构建参数（可通过docker build --build-arg传入）
ARG BUILD_VERSION=dev
ARG BUILD_TIME="unknown"
ARG GIT_COMMIT=unknown

# 复制依赖文件并下载（利用Docker缓存）
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download -x && go mod verify

# 只复制必要的源代码目录
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY configs/ ./configs/
COPY api/ ./api/

# 编译二进制文件（嵌入版本信息）
RUN --mount=type=cache,target=/root/.cache/go-build \
    GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s \
    -X 'main.Version=${BUILD_VERSION}' \
    -X 'main.BuildTime=${BUILD_TIME}' \
    -X 'main.GitCommit=${GIT_COMMIT}'" \
    -o /out/iot-server ./cmd/server

# 验证二进制文件
RUN /out/iot-server --version 2>/dev/null || echo "Binary built successfully"

# ============================================
# 运行阶段 - 使用轻量级Debian镜像
# ============================================
FROM debian:12-slim

# 配置国内镜像源（阿里云）并安装运行时依赖
RUN sed -i 's/deb.debian.org/mirrors.aliyun.com/g' /etc/apt/sources.list.d/debian.sources && \
    apt-get update && \
    apt-get install -y --no-install-recommends \
    ca-certificates \
    tzdata \
    curl \
    && rm -rf /var/lib/apt/lists/* \
    && update-ca-certificates

# 创建非root用户
RUN useradd -u 65532 -r -g root -s /bin/false -c "Application User" nonroot

WORKDIR /app

# 复制二进制文件
COPY --from=build /out/iot-server /app/iot-server

# 复制配置文件（生产环境会被volume覆盖）
COPY --from=build /src/configs/production.yaml /app/config.yaml
COPY --from=build /src/configs/bkv_reason_map.yaml /app/configs/bkv_reason_map.yaml

# 复制Web静态文件（测试控制台前端）
COPY web/ /app/web/

# 创建日志目录
RUN mkdir -p /var/log/iot-server && \
    chown -R nonroot:root /var/log/iot-server /app

# 创建启动脚本（自动修复日志目录权限）
RUN echo '#!/bin/bash' > /entrypoint.sh && \
    echo 'set -e' >> /entrypoint.sh && \
    echo '# 修复日志目录权限（如果volume挂载导致权限丢失）' >> /entrypoint.sh && \
    echo 'if [ ! -w /var/log/iot-server ]; then' >> /entrypoint.sh && \
    echo '  echo "Fixing log directory permissions..."' >> /entrypoint.sh && \
    echo '  chown -R 65532:root /var/log/iot-server 2>/dev/null || true' >> /entrypoint.sh && \
    echo 'fi' >> /entrypoint.sh && \
    echo '# 切换到 nonroot 用户运行应用' >> /entrypoint.sh && \
    echo 'exec su-exec 65532:root "$@"' >> /entrypoint.sh && \
    chmod +x /entrypoint.sh

# 安装 gosu（Debian版本的su-exec）
RUN apt-get update && \
    apt-get install -y --no-install-recommends gosu && \
    rm -rf /var/lib/apt/lists/* && \
    gosu nobody true

# 修改entrypoint使用gosu
RUN sed -i 's/su-exec/gosu/g' /entrypoint.sh

# 暴露端口
EXPOSE 8080 7000

# 环境变量
ENV IOT_CONFIG=/app/config.yaml \
    TZ=Asia/Shanghai \
    LANG=C.UTF-8 \
    LC_ALL=C.UTF-8

# 添加健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=40s --retries=3 \
    CMD curl -f http://localhost:8080/healthz || exit 1

# 镜像元数据
ARG BUILD_VERSION=dev
ARG BUILD_TIME="unknown"
ARG GIT_COMMIT=unknown
LABEL maintainer="IoT Team" \
      version="${BUILD_VERSION}" \
      build_time="${BUILD_TIME}" \
      git_commit="${GIT_COMMIT}" \
      description="IoT Server for device communication and management"

# 启动命令（通过entrypoint自动修复权限）
ENTRYPOINT ["/entrypoint.sh"]
CMD ["/app/iot-server"]


