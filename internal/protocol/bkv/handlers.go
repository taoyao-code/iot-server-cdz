package bkv

import "context"

// repoAPI 抽象（与 ap3000 对齐一部分能力）
type repoAPI interface {
	EnsureDevice(ctx context.Context, phyID string) (int64, error)
	InsertCmdLog(ctx context.Context, deviceID int64, msgID int, cmd int, direction int16, payload []byte, success bool) error
	UpsertPortState(ctx context.Context, deviceID int64, portNo int, status int, powerW *int) error
	SettleOrder(ctx context.Context, deviceID int64, portNo int, orderHex string, durationSec int, kwh01 int, reason int) error
	AckOutboundByMsgID(ctx context.Context, deviceID int64, msgID int, ok bool, errCode *int) error
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

// HandleAck 占位：解析 msgID 与 code，并按 code==0 视为成功，调用 AckOutboundByMsgID
// 约定占位：data[0..1]=msgID LE，data[2]=code
func (h *Handlers) HandleAck(ctx context.Context, f *Frame) error {
	if h == nil || h.Repo == nil {
		return nil
	}
	devID, err := h.Repo.EnsureDevice(ctx, "BKV-UNKNOWN")
	if err != nil {
		return err
	}
	var msgID int
	var code int
	if len(f.Data) >= 2 {
		msgID = int(uint16(f.Data[0]) | uint16(f.Data[1])<<8)
	}
	if len(f.Data) >= 3 {
		code = int(f.Data[2])
	}
	ok := code == 0
	var ecode *int
	if !ok {
		ecode = &code
	}
	_ = h.Repo.AckOutboundByMsgID(ctx, devID, msgID, ok, ecode)
	return h.Repo.InsertCmdLog(ctx, devID, msgID, int(f.Cmd), 0, f.Data, ok)
}

// HandleControl 占位：控制类指令（如启动/停止），此处仅记录日志
func (h *Handlers) HandleControl(ctx context.Context, f *Frame) error {
	if h == nil || h.Repo == nil {
		return nil
	}
	devID, err := h.Repo.EnsureDevice(ctx, "BKV-UNKNOWN")
	if err != nil {
		return err
	}
	return h.Repo.InsertCmdLog(ctx, devID, 0, int(f.Cmd), 0, f.Data, true)
}

// HandleParam 占位：参数读/写/回读最小路径（83/84/85），此处仅记录
func (h *Handlers) HandleParam(ctx context.Context, f *Frame) error {
	if h == nil || h.Repo == nil {
		return nil
	}
	devID, err := h.Repo.EnsureDevice(ctx, "BKV-UNKNOWN")
	if err != nil {
		return err
	}
	return h.Repo.InsertCmdLog(ctx, devID, 0, int(f.Cmd), 0, f.Data, true)
}
