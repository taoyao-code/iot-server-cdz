package bkv

import "context"

// repoAPI 抽象（与 ap3000 对齐一部分能力）
type repoAPI interface {
	EnsureDevice(ctx context.Context, phyID string) (int64, error)
	InsertCmdLog(ctx context.Context, deviceID int64, msgID int, cmd int, direction int16, payload []byte, success bool) error
	UpsertPortState(ctx context.Context, deviceID int64, portNo int, status int, powerW *int) error
	SettleOrder(ctx context.Context, deviceID int64, portNo int, orderHex string, durationSec int, kwh01 int, reason int) error
}

// Handlers BKV 最小处理器集合（心跳/状态->端口快照；通用日志）
// Reason 可选：用于结束原因映射（BKV→平台统一码）
type Handlers struct {
	Repo   repoAPI
	Reason *ReasonMap
}

// HandleHeartbeat 最小心跳处理（BKV 未定义字段，这里仅记录日志与端口示例占位）
func (h *Handlers) HandleHeartbeat(ctx context.Context, f *Frame) error {
	if h == nil || h.Repo == nil {
		return nil
	}
	devID, err := h.Repo.EnsureDevice(ctx, "BKV-UNKNOWN")
	if err != nil {
		return err
	}
	// 占位：无端口/功率细节，直接日志
	return h.Repo.InsertCmdLog(ctx, devID, 0, int(f.Cmd), 0, f.Data, true)
}

// HandleStatus 示例：将首字节当作端口或状态占位（后续按真实协议完善）
func (h *Handlers) HandleStatus(ctx context.Context, f *Frame) error {
	if h == nil || h.Repo == nil {
		return nil
	}
	devID, err := h.Repo.EnsureDevice(ctx, "BKV-UNKNOWN")
	if err != nil {
		return err
	}
	// 占位解析：data[0]=port, data[1]=status（若缺失则使用默认）
	port := 1
	status := 1
	if len(f.Data) >= 1 {
		port = int(f.Data[0])
	}
	if len(f.Data) >= 2 {
		status = int(f.Data[1])
	}
	_ = h.Repo.UpsertPortState(ctx, devID, port, status, nil)
	return h.Repo.InsertCmdLog(ctx, devID, 0, int(f.Cmd), 0, f.Data, true)
}

// HandleSettle 结算占位：从 data 提取原因码，映射为平台码后落库（其他字段占位）
// 约定占位：data[0]=reason（BKV），端口/订单/时长/电量暂为空占位
func (h *Handlers) HandleSettle(ctx context.Context, f *Frame) error {
	if h == nil || h.Repo == nil {
		return nil
	}
	devID, err := h.Repo.EnsureDevice(ctx, "BKV-UNKNOWN")
	if err != nil {
		return err
	}
	reason := 0
	if len(f.Data) > 0 {
		reason = int(f.Data[0])
	}
	if h.Reason != nil {
		if v, ok := h.Reason.Translate(reason); ok {
			reason = v
		}
	}
	// 占位字段：port=1, orderHex="BKV-UNKNOWN", duration/kwh=0
	_ = h.Repo.SettleOrder(ctx, devID, 1, "BKV-UNKNOWN", 0, 0, reason)
	return h.Repo.InsertCmdLog(ctx, devID, 0, int(f.Cmd), 0, f.Data, true)
}
