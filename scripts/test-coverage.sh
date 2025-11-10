#!/bin/bash
# 测试覆盖率报告生成脚本

set -e

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  IoT Server - 测试覆盖率报告"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# 检查是否设置了测试数据库
if [ -z "$TEST_DATABASE_URL" ]; then
    echo "⚠️  未设置 TEST_DATABASE_URL 环境变量"
    echo ""
    echo "📦 尝试使用 Docker Compose 启动测试环境..."
    
    if command -v docker-compose &> /dev/null; then
        # 启动测试服务
        docker-compose -f docker-compose.test.yml up -d
        
        echo "⏳ 等待PostgreSQL就绪..."
        sleep 5
        
        # 设置环境变量
        export TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5433/iot_test?sslmode=disable"
        export REDIS_URL="redis://localhost:6380/0"
        
        echo "✅ 测试环境已就绪"
        echo ""
        
        # 设置清理标志
        CLEANUP_DOCKER=true
    else
        echo "❌ Docker Compose 不可用"
        echo "   部分测试将被跳过（需要数据库的测试）"
        echo ""
    fi
fi

# 创建覆盖率输出目录
mkdir -p coverage

echo "📊 运行单元测试并生成覆盖率报告..."
echo ""

# 运行所有测试，生成覆盖率报告
go test ./internal/... -coverprofile=coverage/coverage.out -covermode=atomic -v 2>&1 | tee coverage/test.log

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  覆盖率统计"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# 生成总体覆盖率
go tool cover -func=coverage/coverage.out | tail -n 1

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  核心模块覆盖率"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# 按包统计覆盖率
go tool cover -func=coverage/coverage.out | grep -E "internal/(storage|api|protocol|session|outbound|thirdparty)" | \
    awk -F'/' '{pkg=$2"/"$3; sub(/\.go.*/, "", pkg)} {coverage[pkg]+=$NF; count[pkg]++} END {for (p in coverage) printf "%-40s %6.1f%%\n", p, coverage[p]/count[p]}' | \
    sort -k2 -rn

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  生成 HTML 报告"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# 生成HTML覆盖率报告
go tool cover -html=coverage/coverage.out -o coverage/coverage.html

echo "✅ HTML 报告已生成: coverage/coverage.html"
echo ""
echo "🔍 查看详细报告："
echo "   open coverage/coverage.html    (macOS)"
echo "   xdg-open coverage/coverage.html (Linux)"
echo ""
echo "📁 覆盖率文件："
echo "   coverage/coverage.out  - 覆盖率数据"
echo "   coverage/coverage.html - HTML 报告"
echo "   coverage/test.log      - 测试日志"
echo ""

# 清理Docker环境
if [ "$CLEANUP_DOCKER" = "true" ]; then
    echo "🧹 清理测试环境..."
    docker-compose -f docker-compose.test.yml down
    echo "✅ 测试环境已清理"
    echo ""
fi

