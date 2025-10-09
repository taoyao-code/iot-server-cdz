package gn

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	gnStorage "github.com/taoyao-code/iot-server/internal/storage/gn"
)

// BusinessHandler GN协议业务处理器
type BusinessHandler struct {
	repos  *gnStorage.PostgresRepos
	worker *Worker
}

// NewBusinessHandler 创建业务处理器
func NewBusinessHandler(repos *gnStorage.PostgresRepos, worker *Worker) *BusinessHandler {
	return &BusinessHandler{
		repos:  repos,
		worker: worker,
	}
}

// HandleHeartbeat 处理A1心跳消息
func (h *BusinessHandler) HandleHeartbeat(ctx context.Context, frame *Frame, gwid string, payload []byte) error {
	log.Printf("GN: Processing heartbeat from gateway %s", gwid)

	// 解析心跳载荷
	iccid, rssi, fwVer, err := ParseHeartbeat(payload)
	if err != nil {
		log.Printf("GN: Failed to parse heartbeat: %v", err)
		// 记录解析失败的日志
		h.repos.Inbound.Append(ctx, gwid, int(frame.Command), int(frame.Sequence),
			hex.EncodeToString(payload), false, fmt.Sprintf("parse_error: %v", err))
		return err
	}

	// 更新设备心跳信息
	err = h.repos.Devices.UpsertHeartbeat(ctx, gwid, gwid, iccid, rssi, fwVer)
	if err != nil {
		log.Printf("GN: Failed to update device heartbeat: %v", err)
		return err
	}

	// 记录成功处理的日志
	err = h.repos.Inbound.Append(ctx, gwid, int(frame.Command), int(frame.Sequence),
		hex.EncodeToString(payload), true, "heartbeat_processed")
	if err != nil {
		log.Printf("GN: Failed to log inbound message: %v", err)
	}

	// 构建并发送时间同步响应
	timeSyncPayload := BuildTimeSync()
	responseFrame, err := NewFrame(CmdHeartbeat, frame.Sequence, frame.GatewayID, timeSyncPayload, true)
	if err != nil {
		log.Printf("GN: Failed to create time sync response: %v", err)
		return err
	}

	// 入队时间同步响应
	_, err = h.worker.EnqueueMessage(ctx, gwid, int(responseFrame.Command), int(responseFrame.Sequence), timeSyncPayload)
	if err != nil {
		log.Printf("GN: Failed to enqueue time sync response: %v", err)
		return err
	}

	log.Printf("GN: Heartbeat processed successfully for gateway %s (ICCID: %s, RSSI: %d, FwVer: %s)",
		gwid, iccid, rssi, fwVer)

	return nil
}

// HandleStatusReport 处理A3插座状态上报
func (h *BusinessHandler) HandleStatusReport(ctx context.Context, frame *Frame, gwid string, payload []byte) error {
	log.Printf("GN: Processing status report from gateway %s", gwid)

	// 解析插座状态
	sockets, err := ParseSocketStatus(payload)
	if err != nil {
		log.Printf("GN: Failed to parse socket status: %v", err)
		h.repos.Inbound.Append(ctx, gwid, int(frame.Command), int(frame.Sequence),
			hex.EncodeToString(payload), false, fmt.Sprintf("parse_error: %v", err))
		return err
	}

	// 批量更新端口状态
	for _, socket := range sockets {
		ports := make([]gnStorage.PortSnapshot, len(socket.Ports))
		for i, port := range socket.Ports {
			ports[i] = gnStorage.PortSnapshot{
				DeviceID:   gwid,
				PortNo:     port.Number,
				StatusBits: port.StatusBits,
				BizNo:      fmt.Sprintf("%d", port.BizNo),
				Voltage:    port.Voltage,
				Current:    port.Current,
				Power:      port.Power,
				Energy:     port.Energy,
				Duration:   port.Duration,
				UpdatedAt:  time.Now(),
			}
		}

		err = h.repos.Ports.UpsertPortSnapshot(ctx, gwid, ports)
		if err != nil {
			log.Printf("GN: Failed to update port snapshots for socket %d: %v", socket.Number, err)
			continue
		}
	}

	// 记录成功处理的日志
	err = h.repos.Inbound.Append(ctx, gwid, int(frame.Command), int(frame.Sequence),
		hex.EncodeToString(payload), true, fmt.Sprintf("status_report_processed: %d sockets", len(sockets)))
	if err != nil {
		log.Printf("GN: Failed to log inbound message: %v", err)
	}

	// 更新设备最后见时间
	h.repos.Devices.UpdateSeen(ctx, gwid)

	log.Printf("GN: Status report processed successfully for gateway %s (%d sockets)",
		gwid, len(sockets))

	return nil
}

// HandleStatusQuery 处理A4状态查询
func (h *BusinessHandler) HandleStatusQuery(ctx context.Context, frame *Frame, gwid string, payload []byte) error {
	log.Printf("GN: Processing status query from gateway %s", gwid)

	// 对于状态查询，我们直接确认收到
	// 实际业务可能需要返回特定的状态信息

	// 记录日志
	err := h.repos.Inbound.Append(ctx, gwid, int(frame.Command), int(frame.Sequence),
		hex.EncodeToString(payload), true, "status_query_received")
	if err != nil {
		log.Printf("GN: Failed to log inbound message: %v", err)
	}

	// ACK确认
	h.worker.AckMessage(ctx, gwid, int(frame.Sequence))

	log.Printf("GN: Status query processed for gateway %s", gwid)
	return nil
}

// HandleControl 处理C1控制指令
func (h *BusinessHandler) HandleControl(ctx context.Context, frame *Frame, gwid string, payload []byte) error {
	log.Printf("GN: Processing control command from gateway %s", gwid)

	// 记录日志
	err := h.repos.Inbound.Append(ctx, gwid, int(frame.Command), int(frame.Sequence),
		hex.EncodeToString(payload), true, "control_command_received")
	if err != nil {
		log.Printf("GN: Failed to log inbound message: %v", err)
	}

	// 对于控制指令，确认收到
	h.worker.AckMessage(ctx, gwid, int(frame.Sequence))

	log.Printf("GN: Control command processed for gateway %s", gwid)
	return nil
}

// HandleControlEnd 处理C2结束上报
func (h *BusinessHandler) HandleControlEnd(ctx context.Context, frame *Frame, gwid string, payload []byte) error {
	log.Printf("GN: Processing control end from gateway %s", gwid)

	// 记录日志
	err := h.repos.Inbound.Append(ctx, gwid, int(frame.Command), int(frame.Sequence),
		hex.EncodeToString(payload), true, "control_end_received")
	if err != nil {
		log.Printf("GN: Failed to log inbound message: %v", err)
	}

	// ACK确认
	h.worker.AckMessage(ctx, gwid, int(frame.Sequence))

	log.Printf("GN: Control end processed for gateway %s", gwid)
	return nil
}

// HandleParamSet 处理E2参数设置
func (h *BusinessHandler) HandleParamSet(ctx context.Context, frame *Frame, gwid string, payload []byte) error {
	log.Printf("GN: Processing param set from gateway %s", gwid)

	// 记录日志
	err := h.repos.Inbound.Append(ctx, gwid, int(frame.Command), int(frame.Sequence),
		hex.EncodeToString(payload), true, "param_set_received")
	if err != nil {
		log.Printf("GN: Failed to log inbound message: %v", err)
	}

	// ACK确认
	h.worker.AckMessage(ctx, gwid, int(frame.Sequence))

	log.Printf("GN: Param set processed for gateway %s", gwid)
	return nil
}

// HandleParamQuery 处理E3参数查询
func (h *BusinessHandler) HandleParamQuery(ctx context.Context, frame *Frame, gwid string, payload []byte) error {
	log.Printf("GN: Processing param query from gateway %s", gwid)

	// 记录日志
	err := h.repos.Inbound.Append(ctx, gwid, int(frame.Command), int(frame.Sequence),
		hex.EncodeToString(payload), true, "param_query_received")
	if err != nil {
		log.Printf("GN: Failed to log inbound message: %v", err)
	}

	// ACK确认
	h.worker.AckMessage(ctx, gwid, int(frame.Sequence))

	log.Printf("GN: Param query processed for gateway %s", gwid)
	return nil
}

// HandleException 处理F1异常上报
func (h *BusinessHandler) HandleException(ctx context.Context, frame *Frame, gwid string, payload []byte) error {
	log.Printf("GN: Processing exception from gateway %s", gwid)

	// 记录日志
	err := h.repos.Inbound.Append(ctx, gwid, int(frame.Command), int(frame.Sequence),
		hex.EncodeToString(payload), true, "exception_received")
	if err != nil {
		log.Printf("GN: Failed to log inbound message: %v", err)
	}

	// ACK确认
	h.worker.AckMessage(ctx, gwid, int(frame.Sequence))

	log.Printf("GN: Exception processed for gateway %s", gwid)
	return nil
}

// SendStatusQuery 发送状态查询指令
func (h *BusinessHandler) SendStatusQuery(ctx context.Context, deviceID string, socketNum uint8) error {
	payload := BuildStatusQuery(socketNum)

	// 生成序列号（实际应用中可能需要更复杂的序列号管理）
	seq := int(time.Now().Unix() & 0xFFFF)

	_, err := h.worker.EnqueueMessage(ctx, deviceID, CmdStatusQuery, seq, payload)
	if err != nil {
		return fmt.Errorf("failed to enqueue status query: %w", err)
	}

	log.Printf("GN: Status query enqueued for device %s, socket %d", deviceID, socketNum)
	return nil
}

// GetDeviceStatus 获取设备状态
func (h *BusinessHandler) GetDeviceStatus(ctx context.Context, deviceID string) (*gnStorage.Device, []gnStorage.PortSnapshot, error) {
	device, err := h.repos.Devices.FindByID(ctx, deviceID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find device: %w", err)
	}

	ports, err := h.repos.Ports.ListByDevice(ctx, deviceID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list ports: %w", err)
	}

	return device, ports, nil
}
