package ap3000

import (
	"context"
	"time"

	"github.com/taoyao-code/iot-server/internal/metrics"
)

// repoAPI 抽象便于单测替换
type repoAPI interface {
	EnsureDevice(ctx context.Context, phyID string) (int64, error)
	InsertCmdLog(ctx context.Context, deviceID int64, msgID int, cmd int, direction int16, payload []byte, success bool) error
	UpsertPortState(ctx context.Context, deviceID int64, portNo int, status int, powerW *int) error
	AckOutboundByMsgID(ctx context.Context, deviceID int64, msgID int, ok bool, errCode *int) error
}

// Handlers 最小处理器集合（示例：记录心跳/注册与指令日志）
type Handlers struct {
	Repo repoAPI
	// 可选：第三方推送
	Pusher interface {
		SendJSON(ctx context.Context, endpoint string, payload any) (int, []byte, error)
	}
	PushURL string
	// 可选：指标
	Metrics *metrics.AppMetrics
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
	success := derr == nil && s != nil
	if success {
		// 异步第三方推送（可选）
		if h.Pusher != nil && h.PushURL != "" {
			payload := map[string]any{
				"event":       "charge.settlement",
				"devicePhyId": f.PhyID,
				"timestamp":   time.Now().Unix(),
				"nonce":       f.PhyID,
				"data": map[string]any{
					"port":        s.Port,
					"order":       s.OrderHex,
					"durationSec": s.DurationSec,
					"kwh01":       s.Kwh01,
					"reason":      s.Reason,
				},
			}
			go func() { _, _, _ = h.Pusher.SendJSON(context.Background(), h.PushURL, payload) }()
		}
	}
	return h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), 0, f.Data, success)
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
	// 指标：82 成功/失败计数
	if h.Metrics != nil && h.Metrics.AP3000Ack82Total != nil {
		if ok {
			h.Metrics.AP3000Ack82Total.WithLabelValues("ok").Inc()
		} else {
			h.Metrics.AP3000Ack82Total.WithLabelValues("err").Inc()
		}
	}
	return h.Repo.InsertCmdLog(ctx, devID, int(f.MsgID), int(f.Cmd), 0, f.Data, ok)
}
