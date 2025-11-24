package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/taoyao-code/iot-server/internal/coremodel"
	"github.com/taoyao-code/iot-server/internal/driverapi"
	"github.com/taoyao-code/iot-server/internal/metrics"
	"github.com/taoyao-code/iot-server/internal/session"
	"github.com/taoyao-code/iot-server/internal/storage"
	"github.com/taoyao-code/iot-server/internal/storage/models"
	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
	"github.com/taoyao-code/iot-server/internal/thirdparty"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ThirdPartyHandler 第三方API处理器
type ThirdPartyHandler struct {
	repo       *pgstorage.Repository
	core       storage.CoreRepo
	sess       session.SessionManager
	driverCmd  driverapi.CommandSource
	eventQueue *thirdparty.EventQueue
	metrics    *metrics.AppMetrics // 一致性监控指标
	logger     *zap.Logger
}

// NewThirdPartyHandler 创建第三方API处理器
func NewThirdPartyHandler(
	repo *pgstorage.Repository,
	core storage.CoreRepo,
	sess session.SessionManager,
	commandSource driverapi.CommandSource,
	eventQueue *thirdparty.EventQueue,
	metrics *metrics.AppMetrics,
	logger *zap.Logger,
) *ThirdPartyHandler {
	return &ThirdPartyHandler{
		repo:       repo,
		core:       core,
		sess:       sess,
		driverCmd:  commandSource,
		eventQueue: eventQueue,
		metrics:    metrics,
		logger:     logger,
	}
}

// mapPortStatusText 将端口状态枚举映射为可读文案（保持与历史实现一致）
func mapPortStatusText(status int) string {
	statusByte := status & 0xFF
	isOnline := statusByte&0x01 != 0
	isIdle := statusByte&0x08 != 0
	isCharging := statusByte&0x80 != 0

	switch {
	case isCharging:
		return "charging"
	case !isOnline:
		return "offline"
	case isIdle:
		return "free"
	default:
		return "occupied"
	}
}

// StandardResponse 标准响应格式
type StandardResponse struct {
	Code      int         `json:"code"`           // 0=成功, >0=错误码
	Message   string      `json:"message"`        // 消息
	Data      interface{} `json:"data,omitempty"` // 业务数据
	RequestID string      `json:"request_id"`     // 请求追踪ID
	Timestamp int64       `json:"timestamp"`      // 时间戳
}

// StartChargeRequest 启动充电请求
type StartChargeRequest struct {
	SocketUID       string `json:"socket_uid" binding:"required"`              // 插座 UID（必填）
	PortNo          int    `json:"port_no" binding:"min=0"`                    // 端口号：0=A端口, 1=B端口, ...（移除required，因为0是有效值）
	ChargeMode      int    `json:"charge_mode" binding:"required,min=1,max=4"` // 充电模式：1=按时长,2=按电量,3=按功率,4=充满自停
	Amount          int    `json:"amount" binding:"required,min=1"`            // 金额（分）
	DurationMinutes int    `json:"duration_minutes"`                           // 时长（分钟）- 推荐使用
	Power           int    `json:"power"`                                      // 功率（瓦）
	PricePerKwh     int    `json:"price_per_kwh"`                              // 电价（分/度）
	ServiceFee      int    `json:"service_fee"`                                // 服务费率（千分比）
	OrderNo         string `json:"order_no"`                                   // 第三方传入的订单号（可选）
	BusinessNo      int    `json:"business_no"`                                // 第三方传入的业务号（可选，0表示使用派生值）
}

// TriggerOTARequest OTA 请求体（用于测试和参数校验）
type TriggerOTARequest struct {
	FirmwareURL  string `json:"firmware_url" binding:"required"`
	Version      string `json:"version" binding:"required"`
	MD5          string `json:"md5" binding:"required"`
	Size         int    `json:"size" binding:"required"`
	TargetType   int    `json:"target_type" binding:"required"`
	TargetSocket int    `json:"target_socket"`
}

// GetDuration 获取时长（优先使用 duration_minutes）
func (r *StartChargeRequest) GetDuration() int {
	return r.DurationMinutes
}

// StartCharge 启动充电
// @Summary 启动充电
// @Description 第三方平台调用此接口启动设备充电
// @Tags 第三方API - 充电控制
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param device_id path string true "设备物理ID"
// @Param request body StartChargeRequest true "充电参数"
// @Success 200 {object} StandardResponse "成功"
// @Failure 400 {object} StandardResponse "参数错误"
// @Failure 500 {object} StandardResponse "服务器错误"
// @Router /api/v1/third/devices/{device_id}/charge [post]
func (h *ThirdPartyHandler) StartCharge(c *gin.Context) {
	ctx := c.Request.Context()
	devicePhyID := c.Param("device_id")
	requestID := c.GetString("request_id")

	// 解析请求体
	var req StartChargeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Error("invalid request body", zap.Error(err))
		c.JSON(http.StatusBadRequest, StandardResponse{
			Code: 400,
			// EN: invalid request body
			Message:   fmt.Sprintf("无效的请求: %v", err),
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	h.logger.Info("start charge requested",
		zap.String("device_phy_id", devicePhyID),
		zap.Int("port_no", req.PortNo),
		zap.Int("charge_mode", req.ChargeMode),
		zap.Int("amount", req.Amount),
		zap.String("socket_uid", req.SocketUID))

	// 2.1 解析 socket_uid 对应的映射，获取 socket_no（查不到则报错，禁止 port_no 兜底）
	mapping, err := h.getSocketMappingByUID(ctx, req.SocketUID)
	if err != nil {
		status := http.StatusInternalServerError
		msg := fmt.Sprintf("查询插座映射失败: %v", err)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusBadRequest
			msg = fmt.Sprintf("未找到插座UID映射: %s", req.SocketUID)
		}
		c.JSON(status, StandardResponse{
			Code:      status,
			Message:   msg,
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	if mapping.GatewayID != "" && mapping.GatewayID != devicePhyID {
		c.JSON(http.StatusBadRequest, StandardResponse{
			Code:    400,
			Message: fmt.Sprintf("插座UID与设备不匹配: uid=%s, gateway=%s", req.SocketUID, mapping.GatewayID),
			Data: map[string]interface{}{
				"socket_uid": req.SocketUID,
				"gateway_id": mapping.GatewayID,
				"device_id":  devicePhyID,
			},
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}
	socketNo := int(mapping.SocketNo)
	if socketNo <= 0 {
		c.JSON(http.StatusBadRequest, StandardResponse{
			Code:      400,
			Message:   fmt.Sprintf("非法的插座编号: %d (uid=%s)", socketNo, req.SocketUID),
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// 4. 生成订单号并派生业务号（BKV要求）
	orderNo := req.OrderNo
	if strings.TrimSpace(orderNo) == "" {
		// 订单不存在，直接返回错误信息提示
		c.JSON(http.StatusBadRequest, StandardResponse{
			Code:    400,
			Message: "请求中缺少订单号，请提供有效订单号后重试",
			Data: map[string]interface{}{
				"order_no": orderNo,
			},
		})
		return
	}
	businessNo := uint16(req.BusinessNo)
	if businessNo == 0 {
		businessNo = deriveBusinessNo(orderNo)
	}
	if err := h.dispatchStartChargeCommand(ctx, devicePhyID, 0, socketNo, &req, orderNo, businessNo); err != nil {
		h.logger.Error("failed to dispatch start command",
			zap.Error(err),
			zap.String("order_no", orderNo),
			zap.String("device_phy_id", devicePhyID),
			zap.String("socket_uid", req.SocketUID),
			zap.Int("socket_no", socketNo))

		c.JSON(http.StatusInternalServerError, StandardResponse{
			Code:    500,
			Message: "充电命令发送失败，请稍后重试",
			Data: map[string]interface{}{
				"order_no":   orderNo,
				"device_id":  devicePhyID,
				"reason":     "command_dispatch_failed",
				"retry_hint": "pending订单将在5分钟后自动清理，请稍后重试",
			},
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	h.logger.Info("charge command dispatched",
		zap.String("order_no", orderNo),
		zap.String("device_phy_id", devicePhyID),
		zap.Int("port_no", req.PortNo),
		zap.String("socket_uid", req.SocketUID),
		zap.Int("socket_no", socketNo))

	// 9. 返回成功响应
	c.JSON(http.StatusOK, StandardResponse{
		Code:    0,
		Message: "充电指令发送成功",
		Data: map[string]interface{}{
			"device_id":   devicePhyID,
			"order_no":    orderNo,
			"business_no": int(businessNo),
			"port_no":     req.PortNo,
			"amount":      req.Amount,
		},
		RequestID: requestID,
		Timestamp: time.Now().Unix(),
	})
}

// dispatchStartChargeCommand
func (h *ThirdPartyHandler) dispatchStartChargeCommand(
	ctx context.Context,
	devicePhyID string,
	deviceID int64,
	socketNo int,
	req *StartChargeRequest,
	orderNo string,
	businessNo uint16,
) error {
	if req == nil {
		return fmt.Errorf("request required")
	}

	if h.driverCmd == nil {
		return fmt.Errorf("driver command source not configured")
	}

	durationMin := uint16(req.GetDuration())
	if durationMin == 0 {
		durationMin = 1
	}

	return h.sendStartChargeViaDriver(ctx, devicePhyID, socketNo, req.PortNo, businessNo, orderNo, req.ChargeMode, durationMin)
}

// sendStartChargeViaDriver
func (h *ThirdPartyHandler) sendStartChargeViaDriver(
	ctx context.Context,
	devicePhyID string,
	socketNo int,
	portNo int,
	businessNo uint16,
	orderNo string,
	chargeMode int,
	durationMin uint16,
) error {
	if h.driverCmd == nil {
		return fmt.Errorf("driver command source not configured")
	}
	bizStr := strconv.Itoa(int(businessNo))
	biz := coremodel.BusinessNo(bizStr)
	modeCode := int32(chargeMode)
	durationSec := int32(durationMin) * 60
	socket := int32(socketNo)

	cmd := &coremodel.CoreCommand{
		Type:      coremodel.CommandStartCharge,
		CommandID: fmt.Sprintf("start:%s:%d", orderNo, time.Now().UnixNano()),
		DeviceID:  coremodel.DeviceID(devicePhyID),
		PortNo:    coremodel.PortNo(portNo),
		SocketNo: func() *int32 {
			return &socket
		}(),
		BusinessNo: func() *coremodel.BusinessNo {
			return &biz
		}(),
		IssuedAt: time.Now(),
		StartCharge: &coremodel.StartChargePayload{
			Mode:              fmt.Sprintf("mode_%d", chargeMode),
			ModeCode:          &modeCode,
			TargetDurationSec: &durationSec,
		},
	}

	return h.driverCmd.SendCoreCommand(ctx, cmd)
}

func (h *ThirdPartyHandler) dispatchStopChargeCommand(
	ctx context.Context,
	devicePhyID string,
	deviceID int64,
	socketNo int,
	portNo int,
	orderNo string,
	businessNo uint16,
) (bool, error) {
	if h.driverCmd == nil {
		return false, fmt.Errorf("driver command source not configured")
	}
	if err := h.sendStopChargeViaDriver(ctx, devicePhyID, socketNo, portNo, businessNo, orderNo); err != nil {
		return false, err
	}
	return true, nil
}

func (h *ThirdPartyHandler) sendStopChargeViaDriver(
	ctx context.Context,
	devicePhyID string,
	socketNo int,
	portNo int,
	businessNo uint16,
	orderNo string,
) error {
	if h.driverCmd == nil {
		return fmt.Errorf("driver command source not configured")
	}
	biz := coremodel.BusinessNo(strconv.Itoa(int(businessNo)))
	socket := int32(socketNo)

	cmd := &coremodel.CoreCommand{
		Type:      coremodel.CommandStopCharge,
		CommandID: fmt.Sprintf("stop:%s:%d", orderNo, time.Now().UnixNano()),
		DeviceID:  coremodel.DeviceID(devicePhyID),
		PortNo:    coremodel.PortNo(portNo),
		SocketNo: func() *int32 {
			return &socket
		}(),
		BusinessNo: func() *coremodel.BusinessNo {
			return &biz
		}(),
		IssuedAt: time.Now(),
		StopCharge: &coremodel.StopChargePayload{
			Reason: "api_stop_charge",
		},
	}

	return h.driverCmd.SendCoreCommand(ctx, cmd)
}

func (h *ThirdPartyHandler) dispatchQueryPortStatusCommand(
	ctx context.Context,
	deviceID int64,
	devicePhyID string,
	socketNo int,
) error {
	if h.driverCmd == nil {
		return fmt.Errorf("driver command source not configured")
	}
	return h.sendQueryPortStatusViaDriver(ctx, devicePhyID, socketNo)
}

func (h *ThirdPartyHandler) sendQueryPortStatusViaDriver(
	ctx context.Context,
	devicePhyID string,
	socketNo int,
) error {
	if h.driverCmd == nil {
		return fmt.Errorf("driver command source not configured")
	}

	socket := int32(socketNo)
	cmd := &coremodel.CoreCommand{
		Type:      coremodel.CommandQueryPortStatus,
		CommandID: fmt.Sprintf("query:%s:%d", devicePhyID, time.Now().UnixNano()),
		DeviceID:  coremodel.DeviceID(devicePhyID),
		IssuedAt:  time.Now(),
		QueryPortStatus: &coremodel.QueryPortStatusPayload{
			SocketNo: &socket,
		},
	}

	return h.driverCmd.SendCoreCommand(ctx, cmd)
}

// getSocketMappingByUID 通过 socket_uid 查询插座映射。
func (h *ThirdPartyHandler) getSocketMappingByUID(ctx context.Context, socketUID string) (*models.GatewaySocket, error) {
	if h.core == nil {
		return nil, fmt.Errorf("core repo not configured")
	}
	uid := strings.TrimSpace(socketUID)
	if uid == "" {
		return nil, fmt.Errorf("socket_uid is required")
	}
	return h.core.GetGatewaySocketByUID(ctx, uid)
}

// StopChargeRequest 停止充电请求
type StopChargeRequest struct {
	SocketUID  string `json:"socket_uid" binding:"required"`    // 插座 UID（必填）
	PortNo     *int   `json:"port_no" binding:"required,min=0"` // 端口号：0=A端口, 1=B端口, ...（必填，使用指针避免0值validation问题）
	OrderNo    string `json:"order_no"`                         // 第三方传入订单号（可选）
	BusinessNo int    `json:"business_no"`                      // 第三方传入业务号（可选）
}

// StopCharge 停止充电
// @Summary 停止充电
// @Description 第三方平台调用此接口停止设备充电
// @Tags 第三方API - 充电控制
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param device_id path string true "设备物理ID"
// @Param request body StopChargeRequest true "停止充电参数"
// @Success 200 {object} StandardResponse "成功"
// @Failure 404 {object} StandardResponse "订单不存在"
// @Failure 500 {object} StandardResponse "服务器错误"
// @Router /api/v1/third/devices/{device_id}/stop [post]
func (h *ThirdPartyHandler) StopCharge(c *gin.Context) {
	ctx := c.Request.Context()
	devicePhyID := c.Param("device_id")
	requestID := c.GetString("request_id")

	// 解析请求体
	var req StopChargeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Error("invalid request body", zap.Error(err))
		c.JSON(http.StatusBadRequest, StandardResponse{
			Code: 400,
			// EN: invalid request body
			Message:   fmt.Sprintf("无效的请求: %v", err),
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	h.logger.Info("stop charge requested",
		zap.String("device_phy_id", devicePhyID),
		zap.String("socket_uid", req.SocketUID),
		zap.Int("port_no", *req.PortNo))

	// 无状态模式：不强制设备/订单落库

	// 1. 解析 socket_uid 获取 socket_no（查不到则报错，禁止 port_no 兜底）
	mapping, err := h.getSocketMappingByUID(ctx, req.SocketUID)
	if err != nil {
		status := http.StatusInternalServerError
		msg := fmt.Sprintf("查询插座映射失败: %v", err)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusBadRequest
			msg = fmt.Sprintf("未找到插座UID映射: %s", req.SocketUID)
		}
		c.JSON(status, StandardResponse{
			Code:      status,
			Message:   msg,
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}
	if mapping.GatewayID != "" && mapping.GatewayID != devicePhyID {
		c.JSON(http.StatusBadRequest, StandardResponse{
			Code:    400,
			Message: fmt.Sprintf("插座UID与设备不匹配: uid=%s, gateway=%s", req.SocketUID, mapping.GatewayID),
			Data: map[string]interface{}{
				"socket_uid": req.SocketUID,
				"gateway_id": mapping.GatewayID,
				"device_id":  devicePhyID,
			},
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}
	socketNo := int(mapping.SocketNo)
	if socketNo <= 0 {
		c.JSON(http.StatusBadRequest, StandardResponse{
			Code:      400,
			Message:   fmt.Sprintf("非法的插座编号: %d (uid=%s)", socketNo, req.SocketUID),
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// 无状态模式：优先使用第三方提供的订单/业务号，否则生成临时号
	orderNo := req.OrderNo
	if strings.TrimSpace(orderNo) == "" {
		orderNo = fmt.Sprintf("TMP_STOP_%d_%d", time.Now().Unix(), *req.PortNo)
	}
	businessNo := int64(req.BusinessNo)
	if businessNo == 0 {
		businessNo = int64(deriveBusinessNo(orderNo))
	}

	// 无状态模式：跳过订单状态更新，直接下发停止命令

	biz := uint16(businessNo)
	if biz == 0 {
		biz = deriveBusinessNo(orderNo)
	}

	stopCommandSent, dispatchErr := h.dispatchStopChargeCommand(ctx, devicePhyID, 0, socketNo, *req.PortNo, orderNo, biz)
	if dispatchErr != nil {
		h.logger.Error("failed to dispatch stop command",
			zap.Error(dispatchErr),
			zap.String("order_no", orderNo),
			zap.String("device_phy_id", devicePhyID),
			zap.String("socket_uid", req.SocketUID),
			zap.Int("socket_no", socketNo))

		c.JSON(http.StatusInternalServerError, StandardResponse{
			Code: 500,
			// EN: stop command dispatch failed
			Message: "停止命令发送失败，请稍后重试",
			Data: map[string]interface{}{
				"order_no":   orderNo,
				"device_id":  devicePhyID,
				"reason":     "command_dispatch_failed",
				"retry_hint": "若设备未响应，可重新发起停止请求",
			},
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// 4. 返回成功响应（无订单时仍返回临时订单号）
	responseData := map[string]interface{}{
		"device_id":    devicePhyID,
		"port_no":      req.PortNo,
		"business_no":  int(biz),
		"command_sent": stopCommandSent,
		"order_no":     orderNo,
		"status":       "stopping",
		"note":         "无状态停止已下发，等待设备ACK",
	}
	message := "停止指令已下发"

	c.JSON(http.StatusOK, StandardResponse{
		Code:      0,
		Message:   message,
		Data:      responseData,
		RequestID: requestID,
		Timestamp: time.Now().Unix(),
	})
}

// GetDevice 查询设备状态
// @Summary 查询设备状态
// @Description 查询设备在线状态、端口状态、活动订单等信息
// @Tags 第三方API - 设备管理
// @Produce json
// @Security ApiKeyAuth
// @Param device_id path string true "设备物理ID"
// @Success 200 {object} StandardResponse "成功"
// @Failure 404 {object} StandardResponse "设备不存在"
// @Failure 500 {object} StandardResponse "服务器错误"
// @Router /api/v1/third/devices/{device_id} [get]
func (h *ThirdPartyHandler) GetDevice(c *gin.Context) {
	ctx := c.Request.Context()
	devicePhyID := c.Param("device_id")
	requestID := c.GetString("request_id")

	h.logger.Info("get device requested", zap.String("device_phy_id", devicePhyID))

	// 1. 从数据库获取设备信息
	device, err := h.core.GetDeviceByPhyID(ctx, devicePhyID)
	if err != nil {
		h.logger.Error("failed to get device", zap.Error(err))
		c.JSON(http.StatusNotFound, StandardResponse{
			Code: 404,
			// EN: device not found
			Message:   "设备不存在",
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// 2. 检查设备在线状态
	isOnline := h.sess.IsOnline(devicePhyID, time.Now())

	// 3. 查询端口信息
	ports, err := h.repo.ListPortsByPhyID(ctx, devicePhyID)
	if err != nil {
		h.logger.Warn("failed to list ports", zap.String("device_phy_id", devicePhyID), zap.Error(err))
		ports = nil // 继续返回设备信息，即使端口查询失败
	}

	// 4. 查询活跃订单（status IN (0,1,2) = pending/charging/ending）
	activeOrders := []struct {
		OrderNo string
		PortNo  int
	}{}

	queryOrders := `
		SELECT order_no, port_no
		FROM orders
		WHERE device_id = $1 AND status IN (0, 1, 2)
		ORDER BY created_at DESC
		LIMIT 10
	`
	rows, err := h.repo.Pool.Query(ctx, queryOrders, device.ID)
	if err != nil {
		h.logger.Warn("failed to query active orders", zap.Int64("device_id", device.ID), zap.Error(err))
	} else {
		defer rows.Close()
		for rows.Next() {
			var order struct {
				OrderNo string
				PortNo  int
			}
			if err := rows.Scan(&order.OrderNo, &order.PortNo); err == nil {
				activeOrders = append(activeOrders, order)
			}
		}
	}

	// 构建端口号到订单号的映射
	orderNoByPort := make(map[int]string)
	for _, order := range activeOrders {
		orderNoByPort[order.PortNo] = order.OrderNo
	}

	// 5. 构建端口列表
	portList := []map[string]interface{}{}
	hasChargingPort := false
	for _, port := range ports {
		powerW := 0
		if port.PowerW != nil {
			powerW = *port.PowerW
		}

		// 转换端口状态：协议位图 -> 业务枚举（0=空闲,1=充电中,2=故障）
		statusEnum := 0 // 默认空闲
		isCharging := (port.Status & 0x80) != 0
		isPortOnline := (port.Status & 0x01) != 0

		if !isPortOnline {
			statusEnum = 2 // 故障
		} else if isCharging {
			statusEnum = 1 // 充电中
			hasChargingPort = true
		}

		portData := map[string]interface{}{
			"port_no": port.PortNo,
			"status":  statusEnum,
			"power":   powerW,
		}

		// 添加订单号（如果有）
		if orderNo, exists := orderNoByPort[port.PortNo]; exists {
			portData["order_no"] = orderNo
		}

		portList = append(portList, portData)
	}

	// 6. 确定设备整体状态
	deviceStatus := "idle"
	if !isOnline {
		deviceStatus = "offline"
	} else if hasChargingPort {
		deviceStatus = "charging"
	}

	// 7. 构建活跃订单列表
	activeOrderList := []map[string]interface{}{}
	for _, order := range activeOrders {
		orderData := map[string]interface{}{
			"order_no": order.OrderNo,
			"port_no":  order.PortNo,
		}
		activeOrderList = append(activeOrderList, orderData)
	}

	// 8. 返回设备详情
	deviceData := map[string]interface{}{
		"device_phy_id": devicePhyID,
		"device_id":     device.ID,
		"is_online":     isOnline,
		"status":        deviceStatus,
		"ports":         portList,
		"active_orders": activeOrderList,
		"registered_at": device.CreatedAt,
	}
	if device.LastSeenAt != nil {
		deviceData["last_seen_at"] = *device.LastSeenAt
	}

	c.JSON(http.StatusOK, StandardResponse{
		Code: 0,
		// EN: success
		Message:   "成功",
		Data:      deviceData,
		RequestID: requestID,
		Timestamp: time.Now().Unix(),
	})
}

// ListDevices 查询设备列表
// @Summary 查询设备列表
// @Description 查询所有设备的基本信息和状态
// @Tags 第三方API - 设备管理
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} StandardResponse "成功"
// @Failure 500 {object} StandardResponse "服务器错误"
// @Router /api/v1/third/devices [get]
func (h *ThirdPartyHandler) ListDevices(c *gin.Context) {
	ctx := c.Request.Context()
	requestID := c.GetString("request_id")

	h.logger.Info("list devices requested")

	// 1. 查询所有设备（使用较大的 limit）
	devices, err := h.repo.ListDevices(ctx, 1000, 0)
	if err != nil {
		h.logger.Error("failed to list devices", zap.Error(err))
		c.JSON(http.StatusInternalServerError, StandardResponse{
			Code:      500,
			Message:   "查询设备列表失败",
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// 2. 构建设备列表
	deviceList := []map[string]interface{}{}
	for _, device := range devices {
		// 检查在线状态
		isOnline := h.sess.IsOnline(device.PhyID, time.Now())

		// 查询端口信息
		ports, err := h.repo.ListPortsByPhyID(ctx, device.PhyID)
		if err != nil {
			h.logger.Warn("failed to list ports", zap.String("device_phy_id", device.PhyID), zap.Error(err))
			ports = nil
		}

		// 查询活跃订单
		activeOrders := []struct {
			OrderNo string
			PortNo  int
		}{}

		queryOrders := `
			SELECT order_no, port_no
			FROM orders
			WHERE device_id = $1 AND status IN (0, 1, 2)
			ORDER BY created_at DESC
			LIMIT 10
		`
		rows, err := h.repo.Pool.Query(ctx, queryOrders, device.ID)
		if err != nil {
			h.logger.Warn("failed to query active orders", zap.Int64("device_id", device.ID), zap.Error(err))
		} else {
			defer rows.Close()
			for rows.Next() {
				var order struct {
					OrderNo string
					PortNo  int
				}
				if err := rows.Scan(&order.OrderNo, &order.PortNo); err == nil {
					activeOrders = append(activeOrders, order)
				}
			}
		}

		// 构建端口号到订单号的映射
		orderNoByPort := make(map[int]string)
		for _, order := range activeOrders {
			orderNoByPort[order.PortNo] = order.OrderNo
		}

		// 构建端口列表
		portList := []map[string]interface{}{}
		hasChargingPort := false
		for _, port := range ports {
			powerW := 0
			if port.PowerW != nil {
				powerW = *port.PowerW
			}

			// 转换端口状态
			statusEnum := 0
			isCharging := (port.Status & 0x80) != 0
			isPortOnline := (port.Status & 0x01) != 0

			if !isPortOnline {
				statusEnum = 2
			} else if isCharging {
				statusEnum = 1
				hasChargingPort = true
			}

			portData := map[string]interface{}{
				"port_no": port.PortNo,
				"status":  statusEnum,
				"power":   powerW,
			}

			if orderNo, exists := orderNoByPort[port.PortNo]; exists {
				portData["order_no"] = orderNo
			}

			portList = append(portList, portData)
		}

		// 确定设备状态
		deviceStatus := "idle"
		if !isOnline {
			deviceStatus = "offline"
		} else if hasChargingPort {
			deviceStatus = "charging"
		}

		// 构建活跃订单列表
		activeOrderList := []map[string]interface{}{}
		for _, order := range activeOrders {
			orderData := map[string]interface{}{
				"order_no": order.OrderNo,
				"port_no":  order.PortNo,
			}
			activeOrderList = append(activeOrderList, orderData)
		}

		// 添加到设备列表
		deviceData := map[string]interface{}{
			"device_phy_id": device.PhyID,
			"device_id":     device.ID,
			"is_online":     isOnline,
			"status":        deviceStatus,
			"ports":         portList,
			"active_orders": activeOrderList,
		}
		if device.LastSeenAt != nil {
			deviceData["last_seen_at"] = *device.LastSeenAt
		}
		deviceList = append(deviceList, deviceData)
	}

	c.JSON(http.StatusOK, StandardResponse{
		Code:      0,
		Message:   "成功",
		Data:      deviceList,
		RequestID: requestID,
		Timestamp: time.Now().Unix(),
	})
}

// SetParamsRequest 设置参数请求
type SetParamsRequest struct {
	Params []ParamItem `json:"params" binding:"required,min=1"` // 参数列表
}

// ParamItem 参数项
type ParamItem struct {
	ID    int    `json:"id" binding:"required"`    // 参数ID
	Value string `json:"value" binding:"required"` // 参数值
}

// ===== 辅助函数 =====

// deriveBusinessNo 从订单号推导16位业务号
func deriveBusinessNo(orderNo string) uint16 {
	var sum uint32
	for i := 0; i < len(orderNo); i++ {
		sum = (sum*131 + uint32(orderNo[i])) & 0xFFFF
	}
	if sum == 0 {
		sum = 1
	}
	return uint16(sum)
}

// getDeviceStatus 获取设备状态描述
func getDeviceStatus(online bool, activeOrderNo *string) string {
	if !online {
		return "offline"
	}
	if activeOrderNo != nil {
		return "charging"
	}
	return "idle"
}

// isBKVChargingStatus 判断端口状态位图是否表示充电中
func isBKVChargingStatus(status int) bool {
	return status&0x80 != 0
}

// isBKVOnlineStatus 判断端口状态位图是否在线
func isBKVOnlineStatus(status int) bool {
	return status&0x01 != 0
}

// evaluateDeviceConsistency 无状态占位实现
func (h *ThirdPartyHandler) evaluateDeviceConsistency(ctx context.Context, deviceID int64, devicePhyID string, isOnline bool, activeOrderNo *string) (string, string) {
	return "", ""
}

// GetOrder 占位实现，当前无订单存储时返回404
func (h *ThirdPartyHandler) GetOrder(c *gin.Context) {
	c.JSON(http.StatusNotFound, StandardResponse{
		Code:      404,
		Message:   "订单不存在",
		RequestID: c.GetString("request_id"),
		Timestamp: time.Now().Unix(),
	})
}
