#!/bin/bash
# 快速部署脚本 - 编译并替换线上二进制文件
#
# 使用方式:
#   make quick-deploy                              # 使用默认配置
#   SERVER=test.com make quick-deploy              # 自定义服务器
#
# 环境变量:
#   SERVER          目标服务器地址 (默认: 182.43.177.92)
#   SERVER_USER     SSH 用户名 (默认: root)
#   SSH_KEY         SSH 密钥路径 (默认: ~/.ssh/id_rsa)
#   SERVER_PATH     服务器项目路径 (默认: /dataDisk/wwwroot/iot/iot-server/iot-server)
#   CONTAINER_NAME  Docker 容器名 (默认: iot-server-prod)

set -e

# 配置
SERVER="${SERVER:-182.43.177.92}"
SERVER_USER="${SERVER_USER:-root}"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_rsa}"
SERVER_PATH="${SERVER_PATH:-/dataDisk/wwwroot/iot/iot-server/iot-server}"
CONTAINER_NAME="${CONTAINER_NAME:-iot-server-prod}"

# 显示配置
echo "📋 部署配置:"
echo "   服务器: $SERVER_USER@$SERVER"
echo "   容器: $CONTAINER_NAME"
echo "   SSH密钥: $SSH_KEY"
echo ""

# 前置检查
if [ ! -f "$SSH_KEY" ]; then
    echo "❌ SSH 密钥不存在: $SSH_KEY"
    exit 1
fi

# 步骤 1: 编译
echo "🚀 [1/4] 编译 Linux 版本..."
if ! CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/iot-server-linux ./cmd/server; then
    echo "❌ 编译失败"
    exit 1
fi
echo "✅ 编译完成"

# 步骤 2: 上传
echo ""
echo "📤 [2/4] 上传到服务器..."
if ! scp -i "$SSH_KEY" bin/iot-server-linux $SERVER_USER@$SERVER:$SERVER_PATH/bin/iot-server-new; then
    echo "❌ 上传失败，请检查网络或 SSH 配置"
    exit 1
fi
echo "✅ 上传完成"

# 步骤 3: 替换
echo ""
echo "🔄 [3/4] 停止容器..."
if ! ssh -i "$SSH_KEY" $SERVER_USER@$SERVER "docker stop $CONTAINER_NAME"; then
    echo "❌ 停止容器失败"
    exit 1
fi

echo "🔄 [3/4] 替换二进制..."
if ! ssh -i "$SSH_KEY" $SERVER_USER@$SERVER "cd $SERVER_PATH && docker cp bin/iot-server-new $CONTAINER_NAME:/app/iot-server"; then
    echo "❌ 替换失败，正在恢复容器..."
    ssh -i "$SSH_KEY" $SERVER_USER@$SERVER "docker start $CONTAINER_NAME"
    exit 1
fi

echo "🔄 [3/4] 启动容器..."
if ! ssh -i "$SSH_KEY" $SERVER_USER@$SERVER "docker start $CONTAINER_NAME"; then
    echo "❌ 启动容器失败"
    exit 1
fi
echo "✅ 替换完成"

# 步骤 4: 完成
echo ""
echo "✅ [4/4] 部署完成"
echo ""
echo "💡 验证部署: curl http://$SERVER:7055/healthz"

