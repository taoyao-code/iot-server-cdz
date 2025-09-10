package ap3000

import (
	"context"
)

// repoAPI 抽象便于单测替换
type repoAPI interface {
	EnsureDevice(ctx context.Context, phyID string) (int64, error)
	InsertCmdLog(ctx context.Context, deviceID int64, msgID int, cmd int, direction int16, payload []byte, success bool) error
	UpsertPortState(ctx context.Context, deviceID int64, portNo int, status int, powerW *int) error
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
