package bkv

import (
	"context"
	"fmt"
)

// repoAPI 抽象（与 ap3000 对齐一部分能力）
type repoAPI interface {
	EnsureDevice(ctx context.Context, phyID string) (int64, error)
	InsertCmdLog(ctx context.Context, deviceID int64, msgID int, cmd int, direction int16, payload []byte, success bool) error
	UpsertPortState(ctx context.Context, deviceID int64, portNo int, status int, powerW *int) error
	SettleOrder(ctx context.Context, deviceID int64, portNo int, orderHex string, durationSec int, kwh01 int, reason int) error
	AckOutboundByMsgID(ctx context.Context, deviceID int64, msgID int, ok bool, errCode *int) error
}

// Handlers BKV 协议处理器集合
type Handlers struct {
	Repo   repoAPI
	Reason *ReasonMap
}

// HandleHeartbeat 处理心跳帧 (cmd=0x0000 或 BKV cmd=0x1017)
func (h *Handlers) HandleHeartbeat(ctx context.Context, f *Frame) error {
	if h == nil || h.Repo == nil {
		return nil
	}

	// 使用网关ID作为设备标识
	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		devicePhyID = "BKV-UNKNOWN"
	}

	devID, err := h.Repo.EnsureDevice(ctx, devicePhyID)
	if err != nil {
		return err
	}

	// 记录心跳日志
	success := true
	return h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), getDirection(f.IsUplink()), f.Data, success)
}

// HandleBKVStatus 处理BKV插座状态上报 (cmd=0x1000 with BKV payload)
func (h *Handlers) HandleBKVStatus(ctx context.Context, f *Frame) error {
	if h == nil || h.Repo == nil {
		return nil
	}

	// 获取BKV载荷
	payload, err := f.GetBKVPayload()
	if err != nil {
		return fmt.Errorf("failed to parse BKV payload: %w", err)
	}

	// 使用BKV载荷中的网关ID
	devicePhyID := payload.GatewayID
	if devicePhyID == "" {
		devicePhyID = f.GatewayID
	}
	if devicePhyID == "" {
		devicePhyID = "BKV-UNKNOWN"
	}

	devID, err := h.Repo.EnsureDevice(ctx, devicePhyID)
	if err != nil {
		return err
	}

	// 记录命令日志
	if err := h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), getDirection(f.IsUplink()), f.Data, true); err != nil {
		return err
	}

	// 如果是状态上报，尝试解析并更新端口状态
	if payload.IsStatusReport() {
		return h.handleSocketStatusUpdate(ctx, devID, payload)
	}

	return nil
}

// handleSocketStatusUpdate 处理插座状态更新
func (h *Handlers) handleSocketStatusUpdate(ctx context.Context, deviceID int64, payload *BKVPayload) error {
	// 尝试解析插座状态 (这里使用简化的解析，因为完整的TLV解析比较复杂)
	// 从TLV字段中提取基本信息
	var portAStatus, portBStatus int = 0, 0
	var portAPower, portBPower *int

	// 简化的字段解析
	for _, field := range payload.Fields {
		switch field.Tag {
		case 0x03:
			// 插座相关字段，暂时使用默认状态
		case 0x00:
			if len(field.Value) >= 3 && field.Value[1] == 0x09 {
				// 插座状态字段
				portAStatus = int(field.Value[2])
			}
		}
	}

	// 更新端口A状态
	if err := h.Repo.UpsertPortState(ctx, deviceID, 0, portAStatus, portAPower); err != nil {
		return fmt.Errorf("failed to update port A state: %w", err)
	}

	// 更新端口B状态
	if err := h.Repo.UpsertPortState(ctx, deviceID, 1, portBStatus, portBPower); err != nil {
		return fmt.Errorf("failed to update port B state: %w", err)
	}

	return nil
}

// HandleControl 处理控制指令 (cmd=0x0015)
func (h *Handlers) HandleControl(ctx context.Context, f *Frame) error {
	if h == nil || h.Repo == nil {
		return nil
	}

	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		devicePhyID = "BKV-UNKNOWN"
	}

	devID, err := h.Repo.EnsureDevice(ctx, devicePhyID)
	if err != nil {
		return err
	}

	// 记录控制指令日志
	success := true
	return h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), getDirection(f.IsUplink()), f.Data, success)
}

// HandleGeneric 通用处理器，记录所有其他指令
func (h *Handlers) HandleGeneric(ctx context.Context, f *Frame) error {
	if h == nil || h.Repo == nil {
		return nil
	}

	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		devicePhyID = "BKV-UNKNOWN"
	}

	devID, err := h.Repo.EnsureDevice(ctx, devicePhyID)
	if err != nil {
		return err
	}

	// 记录通用指令日志
	success := true
	return h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), getDirection(f.IsUplink()), f.Data, success)
}

// getDirection 获取数据方向标识
func getDirection(isUplink bool) int16 {
	if isUplink {
		return 1 // 上行
	}
	return 0 // 下行
}

// HandleParam 处理参数读写指令 (占位实现)
func (h *Handlers) HandleParam(ctx context.Context, f *Frame) error {
	if h == nil || h.Repo == nil {
		return nil
	}

	devicePhyID := f.GatewayID
	if devicePhyID == "" {
		devicePhyID = "BKV-UNKNOWN"
	}

	devID, err := h.Repo.EnsureDevice(ctx, devicePhyID)
	if err != nil {
		return err
	}

	success := true
	if f.Cmd == 0x83 || f.Cmd == 0x84 {
		// 参数写入
		success = len(f.Data) > 0
	}
	if f.Cmd == 0x85 {
		// 参数回读，有数据则成功
		success = len(f.Data) > 0
	}

	return h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), getDirection(f.IsUplink()), f.Data, success)
}
