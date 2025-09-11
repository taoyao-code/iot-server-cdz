package ap3000

import (
	"context"
)

// repoAPI 抽象便于单测替换
type repoAPI interface {
	EnsureDevice(ctx context.Context, phyID string) (int64, error)
	InsertCmdLog(ctx context.Context, deviceID int64, msgID int, cmd int, direction int16, payload []byte, success bool) error
	UpsertPortState(ctx context.Context, deviceID int64, portNo int, status int, powerW *int) error
	UpsertOrderProgress(ctx context.Context, deviceID int64, portNo int, orderHex string, durationSec int, kwh01 int, status int, powerW01 *int) error
	SettleOrder(ctx context.Context, deviceID int64, portNo int, orderHex string, durationSec int, kwh01 int, reason int) error
	AckOutboundByMsgID(ctx context.Context, deviceID int64, msgID int, ok bool, errCode *int) error
}

// Handlers 最小处理器集合（示例：记录心跳/注册与指令日志）
type Handlers struct {
	Repo repoAPI
}

func (h *Handlers) HandleRegister(ctx context.Context, f *Frame) error {
	if h == nil || h.Repo == nil {
		return nil
	}
	devID, err := h.Repo.EnsureDevice(ctx, f.PhyID)
	if err != nil {
		return err
	}
	// 尝试解析端口状态并更新端口快照
	if ps, derr := Decode20or21(f.Data); derr == nil {
		_ = h.Repo.UpsertPortState(ctx, devID, ps.Port, ps.Status, ps.PowerW)
	}
	return h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), 0, f.Data, true)
}

func (h *Handlers) HandleHeartbeat(ctx context.Context, f *Frame) error {
	return h.HandleRegister(ctx, f)
}

func (h *Handlers) HandleGeneric(ctx context.Context, f *Frame) error {
	if h == nil || h.Repo == nil {
		return nil
	}
	devID, err := h.Repo.EnsureDevice(ctx, f.PhyID)
	if err != nil {
		return err
	}
	return h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), 0, f.Data, true)
}

// Handle03 结算消费信息上传：落库并完成订单
func (h *Handlers) Handle03(ctx context.Context, f *Frame) error {
	if h == nil || h.Repo == nil {
		return nil
	}
	devID, err := h.Repo.EnsureDevice(ctx, f.PhyID)
	if err != nil {
		return err
	}
	s, derr := Decode03(f.Data)
	if derr == nil && s != nil {
		_ = h.Repo.SettleOrder(ctx, devID, s.Port, s.OrderHex, s.DurationSec, s.Kwh01, s.Reason)
	}
	return h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), 0, f.Data, derr == nil)
}

// Handle06 功率心跳：更新订单进度与端口状态
func (h *Handlers) Handle06(ctx context.Context, f *Frame) error {
	if h == nil || h.Repo == nil {
		return nil
	}
	devID, err := h.Repo.EnsureDevice(ctx, f.PhyID)
	if err != nil {
		return err
	}
	p, derr := Decode06(f.Data)
	if derr == nil && p != nil {
		_ = h.Repo.UpsertOrderProgress(ctx, devID, p.Port, p.OrderHex, p.DurationSec, p.Kwh01, p.Status, &p.PowerW01)
		_ = h.Repo.UpsertPortState(ctx, devID, p.Port, p.Status, nil)
	}
	return h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), 0, f.Data, derr == nil)
}

// Handle82Ack 处理 82 设备应答：根据 MsgID 标记下行任务完成/失败
func (h *Handlers) Handle82Ack(ctx context.Context, f *Frame) error {
	if h == nil || h.Repo == nil {
		return nil
	}
	devID, err := h.Repo.EnsureDevice(ctx, f.PhyID)
	if err != nil {
		return err
	}
	code, derr := Decode82Ack(f.Data)
	ok := derr == nil && code == 0
	var ecode *int
	if !ok {
		ecode = &code
	}
	_ = h.Repo.AckOutboundByMsgID(ctx, devID, int(f.MsgID), ok, ecode)
	return h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), 0, f.Data, ok)
}
