#!/bin/bash
# BKV 网关组网配置生成脚本
# 用法: ./generate_configs.sh <网关数量>

if [ $# -eq 0 ]; then
    echo "用法: $0 <网关数量>"
    echo "示例: $0 5"
    exit 1
fi

GATEWAY_COUNT=$1
CHANNELS=(3 6 9 12 15 4 7 10 13 5)  # 保守方案的信道序列

echo "正在生成 ${GATEWAY_COUNT} 个网关的配置文件..."

for i in $(seq 1 $GATEWAY_COUNT); do
    GATEWAY_ID=$(printf "gateway_%03d" $i)
    CHANNEL_INDEX=$(( (i - 1) % 10 ))
    CHANNEL=${CHANNELS[$CHANNEL_INDEX]}

    CONFIG_FILE="network_config_${GATEWAY_ID}.json"

    cat > "$CONFIG_FILE" <<EOF
{
  "channel": ${CHANNEL},
  "sockets": [
    {
      "uid": 30101501140$(printf "%04d" $((2400 + i))),
      "mac": "85412180$(printf "%04x" $((0x0889 + i - 1)))"
    }
  ]
}
EOF

    echo "✓ 已生成: ${CONFIG_FILE} (信道: ${CHANNEL})"
done

echo ""
echo "========== 配置摘要 =========="
for i in $(seq 1 $GATEWAY_COUNT); do
    GATEWAY_ID=$(printf "gateway_%03d" $i)
    CHANNEL_INDEX=$(( (i - 1) % 10 ))
    CHANNEL=${CHANNELS[$CHANNEL_INDEX]}
    printf "网关 %s -> 信道 %2d\n" "$GATEWAY_ID" "$CHANNEL"
done
echo "=============================="
