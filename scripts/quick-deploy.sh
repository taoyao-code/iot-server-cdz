#!/bin/bash
# 快速部署脚本 - 直接替换二进制文件，无需重新构建Docker镜像
# 使用: ./scripts/quick-deploy.sh
# 或: make quick-deploy

set -e

# ============ 配置区域（根据实际情况修改）============
SERVER="${SERVER:-182.43.177.92}"
SERVER_USER="${SERVER_USER:-root}"
SERVER_PATH="${SERVER_PATH:-/dataDisk/wwwroot/iot/iot-server/iot-server}"
CONTAINER_NAME="${CONTAINER_NAME:-iot-server-prod}"
# ====================================================

echo "🚀 快速部署开始..."

# 1. 本地编译Linux版本
echo "📦 编译Linux版本..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/iot-server-linux ./cmd/server
echo "✅ 编译完成"

# 2. 上传到服务器
echo "📤 上传到服务器..."
scp bin/iot-server-linux $SERVER_USER@$SERVER:$SERVER_PATH/bin/iot-server-new

# 3. SSH到服务器执行热替换
echo "🔄 执行热替换..."
ssh $SERVER_USER@$SERVER bash << ENDSSH
set -e
cd $SERVER_PATH

# 停止容器
echo "停止服务..."
docker stop $CONTAINER_NAME

# 替换二进制文件
echo "替换二进制..."
docker cp bin/iot-server-new $CONTAINER_NAME:/app/iot-server

# 启动容器
echo "启动服务..."
docker start $CONTAINER_NAME

# 查看日志
echo "查看启动日志..."
timeout 5 docker logs -f $CONTAINER_NAME --tail 20 || true

echo "✅ 部署完成！"
ENDSSH

echo ""
echo "✅ 快速部署完成！"
echo "🔍 验证部署："
echo "   curl http://$SERVER:7055/healthz"

# 验证
sleep 2
if curl -s http://$SERVER:7055/healthz | grep -q "ok"; then
    echo "✅ 服务正常运行"
else
    echo "⚠️  服务可能未正常启动，请检查日志"
fi

