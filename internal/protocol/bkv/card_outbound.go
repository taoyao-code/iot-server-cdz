package bkv

import (
	"fmt"
)

// Week5: 刷卡充电下行消息发送

// sendChargeCommand 发送充电指令 (0x0B下行)
func (h *Handlers) sendChargeCommand(gatewayID string, msgID uint32, cmd *ChargeCommand) error {
	if h.Outbound == nil {
		return fmt.Errorf("outbound sender not configured")
	}

	// 编码充电指令
	data := EncodeChargeCommand(cmd)

	// 发送下行消息
	return h.Outbound.SendDownlink(gatewayID, 0x0B, msgID, data)
}

// sendOrderConfirmReply 发送订单确认回复 (0x0F下行)
func (h *Handlers) sendOrderConfirmReply(gatewayID string, msgID uint32, orderNo string, result uint8) error {
	if h.Outbound == nil {
		return fmt.Errorf("outbound sender not configured")
	}

	// 构造确认回复数据
	// 格式：订单号(16字节BCD) + 结果(1字节)
	data := make([]byte, 17)

	// 订单号BCD编码
	orderBytes := stringToBCD(orderNo, 16)
	copy(data[0:16], orderBytes)

	// 结果
	data[16] = result

	// 发送下行消息
	return h.Outbound.SendDownlink(gatewayID, 0x0F, msgID, data)
}

// sendChargeEndReply 发送充电结束确认 (0x0C下行)
func (h *Handlers) sendChargeEndReply(gatewayID string, msgID uint32, orderNo string, result uint8) error {
	if h.Outbound == nil {
		return fmt.Errorf("outbound sender not configured")
	}

	// 构造结束确认数据
	// 格式：订单号(16字节BCD) + 结果(1字节)
	data := make([]byte, 17)

	// 订单号BCD编码
	orderBytes := stringToBCD(orderNo, 16)
	copy(data[0:16], orderBytes)

	// 结果
	data[16] = result

	// 发送下行消息
	return h.Outbound.SendDownlink(gatewayID, 0x0C, msgID, data)
}

// sendBalanceResponse 发送余额响应 (0x1A下行)
func (h *Handlers) sendBalanceResponse(gatewayID string, msgID uint32, resp *BalanceResponse) error {
	if h.Outbound == nil {
		return fmt.Errorf("outbound sender not configured")
	}

	// 编码余额响应
	data := EncodeBalanceResponse(resp)

	// 发送下行消息
	return h.Outbound.SendDownlink(gatewayID, 0x1A, msgID, data)
}
