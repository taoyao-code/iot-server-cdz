package ap3000

import (
	"context"

	pgrepo "github.com/taoyao-code/iot-server/internal/storage/pg"
)

// Handlers 最小处理器集合（示例：记录心跳/注册与指令日志）
type Handlers struct {
	Repo *pgrepo.Repository
}

func (h *Handlers) HandleRegister(ctx context.Context, f *Frame) error {
	if h == nil || h.Repo == nil {
		return nil
	}
	devID, err := h.Repo.EnsureDevice(ctx, f.PhyID)
	if err != nil {
		return err
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
