package api

import (
	"context"
	"encoding/json"
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

// portBusyError 表示端口已被占用（存在活跃订单）
type portBusyError struct {
	orderNo        string
	portStatus     int
	portStatusText string
}

func (e *portBusyError) Error() string { return "port busy" }

// portInconsistentError 表示端口状态与订单状态不一致（例如端口charging但无活跃订单）
type portInconsistentError struct {
	portStatus int
}

func (e *portInconsistentError) Error() string { return "port state inconsistent" }

// portFaultError 表示端口处于故障状态
type portFaultError struct {
	portStatus int
}

func (e *portFaultError) Error() string { return "port in fault state" }

// mapPortStatusText 将端口状态枚举映射为可读文案（保持与历史实现一致）
func mapPortStatusText(status int) string {
	switch status {
	case 0:
		return "free"
	case 1:
		return "occupied"
	case 2:
		return "charging"
	case 3:
		return "fault"
	default:
		return fmt.Sprintf("unknown(%d)", status)
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
	Duration        int    `json:"duration"`                                   // 时长（分钟）- 兼容旧版
	Power           int    `json:"power"`                                      // 功率（瓦）
	PricePerKwh     int    `json:"price_per_kwh"`                              // 电价（分/度）
	ServiceFee      int    `json:"service_fee"`                                // 服务费率（千分比）
}

// GetDuration 获取时长（优先使用 duration_minutes）
func (r *StartChargeRequest) GetDuration() int {
	if r.DurationMinutes > 0 {
		return r.DurationMinutes
	}
	return r.Duration
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

	// 1. 验证设备存在（使用 CoreRepo 作为核心存储）
	device, err := h.core.EnsureDevice(ctx, devicePhyID)
	if err != nil {
		h.logger.Error("failed to ensure device via core repo", zap.Error(err))
		c.JSON(http.StatusInternalServerError, StandardResponse{
			Code: 500,
			// EN: failed to get device
			Message:   "获取设备失败",
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}
	devID := device.ID

	// 2. P0-1修复: 强制检查设备在线状态
	isOnline := h.sess.IsOnline(devicePhyID, time.Now())
	if !isOnline {
		h.logger.Warn("device offline, rejecting order creation",
			zap.String("device_phy_id", devicePhyID))
		c.JSON(http.StatusServiceUnavailable, StandardResponse{
			Code: 503,
			// EN: device is offline, cannot create order
			Message: "设备离线，无法创建订单",
			Data: map[string]interface{}{
				"device_id": devicePhyID,
				"status":    "offline",
			},
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// 2.1 解析 socket_uid 对应的映射，获取 socket_no
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

	// 3. 清理超时的pending订单（超过5分钟自动取消），使用 CoreRepo 实现
	if cleaner, ok := h.core.(interface {
		CleanupPendingOrders(ctx context.Context, deviceID int64, before time.Time) (int64, error)
	}); ok {
		before := time.Now().Add(-5 * time.Minute)
		cleaned, err := cleaner.CleanupPendingOrders(ctx, devID, before)
		if err != nil {
			h.logger.Warn("failed to cleanup stale pending orders via core repo",
				zap.String("device_phy_id", devicePhyID),
				zap.Error(err))
		} else if cleaned > 0 {
			h.logger.Info("cleaned up stale pending orders",
				zap.String("device_phy_id", devicePhyID),
				zap.Int64("count", cleaned))
		}
	}

	// 3.5. P1-4修复: 验证端口状态一致性
	isConsistent, portStatus, err := h.verifyPortStatus(ctx, devID, req.PortNo)
	if err != nil {
		h.logger.Warn("P1-4: failed to verify port status, continuing anyway",
			zap.String("device_phy_id", devicePhyID),
			zap.Int("port_no", req.PortNo),
			zap.Error(err))
		// 端口状态查询失败不阻塞创单，记录告警即可
	} else if !isConsistent {
		h.logger.Warn("P1-4: port status mismatch detected",
			zap.String("device_phy_id", devicePhyID),
			zap.Int("port_no", req.PortNo),
			zap.Int("db_status", portStatus),
			zap.String("action", "rejecting order creation"))
		c.JSON(http.StatusConflict, StandardResponse{
			Code: 40901, // PORT_STATE_MISMATCH
			// EN: port state mismatch, port may be in use
			Message: "端口状态不一致，端口可能正在使用中",
			Data: map[string]interface{}{
				"port_no":    req.PortNo,
				"status":     portStatus,
				"error_code": "PORT_STATE_MISMATCH",
			},
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// 4. P1-3修复: 使用 CoreRepo 的事务 + 行锁检查端口并创建订单
	// 5. 生成订单号并派生业务号（BKV要求）
	orderNo := fmt.Sprintf("THD%d%03d", time.Now().Unix(), req.PortNo)
	businessNo := deriveBusinessNo(orderNo)

	// 6. 在同一事务中完成活跃订单检查、端口锁定与订单创建
	err = h.core.WithTx(ctx, func(repo storage.CoreRepo) error {
		// 扩展接口：需要锁定订单和端口的能力
		lockRepo, ok := repo.(interface {
			LockActiveOrderForPort(ctx context.Context, deviceID int64, portNo int32) (*models.Order, bool, error)
			LockOrCreatePort(ctx context.Context, deviceID int64, portNo int32) (*models.Port, error)
		})
		if !ok {
			return fmt.Errorf("core repo does not support locking operations")
		}

		// 4.1 锁定活跃订单，防止跨表状态不一致
		existingOrder, exists, err := lockRepo.LockActiveOrderForPort(ctx, devID, int32(req.PortNo))
		if err != nil {
			h.logger.Error("failed to check port via core repo", zap.Error(err))
			return err
		}
		if exists && existingOrder != nil {
			// 端口已被占用，查询真实的端口状态（在同一事务中读取端口快照）
			var actualPortStatus int
			if port, err := repo.GetPort(ctx, devID, int32(req.PortNo)); err == nil {
				actualPortStatus = int(port.Status)
			}
			portStatusText := mapPortStatusText(actualPortStatus)

			h.logger.Warn("port already in use",
				zap.String("device_phy_id", devicePhyID),
				zap.Int("port_no", req.PortNo),
				zap.String("existing_order", existingOrder.OrderNo),
				zap.Int("actual_port_status", actualPortStatus),
				zap.String("port_status_text", portStatusText))

			return &portBusyError{
				orderNo:        existingOrder.OrderNo,
				portStatus:     actualPortStatus,
				portStatusText: portStatusText,
			}
		}

		// 4.2 行锁定端口记录，若不存在则初始化
		port, err := lockRepo.LockOrCreatePort(ctx, devID, int32(req.PortNo))
		if err != nil {
			h.logger.Error("failed to lock or create port via core repo", zap.Error(err))
			return err
		}
		lockedPortStatus := int(port.Status)

		// 4.3 验证端口状态是否可用（保持与历史业务枚举语义一致）
		if lockedPortStatus == 2 {
			// 端口状态为charging但没有活跃订单，数据不一致
			h.logger.Error("P1-3: port state mismatch - charging status without active order",
				zap.String("device_phy_id", devicePhyID),
				zap.Int("port_no", req.PortNo),
				zap.Int("port_status", lockedPortStatus))
			return &portInconsistentError{portStatus: lockedPortStatus}
		}
		if lockedPortStatus == 3 {
			// 端口故障
			h.logger.Warn("port is in fault state",
				zap.String("device_phy_id", devicePhyID),
				zap.Int("port_no", req.PortNo))
			return &portFaultError{portStatus: lockedPortStatus}
		}

		// 6. 创建订单记录
		amountCent := int64(req.Amount)
		order := &models.Order{
			DeviceID:   devID,
			PortNo:     int32(req.PortNo),
			OrderNo:    orderNo,
			BusinessNo: int32(businessNo),
			Status:     0,
			ChargeMode: int32(req.ChargeMode),
			AmountCent: &amountCent,
		}

		if err := repo.CreateOrder(ctx, order); err != nil {
			h.logger.Error("failed to create order via core repo", zap.Error(err))
			return err
		}

		return nil
	})
	if err != nil {
		switch e := err.(type) {
		case *portBusyError:
			c.JSON(http.StatusConflict, StandardResponse{
				Code: 409,
				// EN: port is busy
				Message: "端口正在使用中",
				Data: map[string]interface{}{
					"current_order": e.orderNo,
					"port_status":   e.portStatusText,
				},
				RequestID: requestID,
				Timestamp: time.Now().Unix(),
			})
			return
		case *portInconsistentError:
			c.JSON(http.StatusConflict, StandardResponse{
				Code: 40903, // PORT_STATE_INCONSISTENT
				// EN: port state inconsistent, please retry
				Message: "端口状态不一致，请重试",
				Data: map[string]interface{}{
					"port_no":     req.PortNo,
					"port_status": e.portStatus,
					"error_code":  "PORT_STATE_INCONSISTENT",
				},
				RequestID: requestID,
				Timestamp: time.Now().Unix(),
			})
			return
		case *portFaultError:
			c.JSON(http.StatusServiceUnavailable, StandardResponse{
				Code: 503,
				// EN: port is in fault state
				Message: "端口故障",
				Data: map[string]interface{}{
					"port_no": req.PortNo,
					"status":  "fault",
				},
				RequestID: requestID,
				Timestamp: time.Now().Unix(),
			})
			return
		default:
			h.logger.Error("failed to create order in transaction", zap.Error(err))
			c.JSON(http.StatusInternalServerError, StandardResponse{
				Code: 500,
				// EN: database error
				Message:   "数据库错误",
				RequestID: requestID,
				Timestamp: time.Now().Unix(),
			})
			return
		}
	}

	h.logger.Info("P1-3: order created with port locked",
		zap.String("order_no", orderNo),
		zap.Int64("device_id", devID),
		zap.Int("port_no", req.PortNo),
		zap.String("socket_uid", req.SocketUID),
		zap.Int("socket_no", socketNo))

	if err := h.dispatchStartChargeCommand(ctx, devicePhyID, devID, socketNo, &req, orderNo, businessNo); err != nil {
		h.logger.Error("failed to dispatch start command",
			zap.Error(err),
			zap.String("order_no", orderNo),
			zap.String("device_phy_id", devicePhyID),
			zap.String("socket_uid", req.SocketUID),
			zap.Int("socket_no", socketNo))

		c.JSON(http.StatusInternalServerError, StandardResponse{
			Code: 500,
			// EN: charge command enqueue failed
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

	// 主动查询插座状态（0x001D），避免仅依赖周期性0x94
	_ = h.enqueueSocketStatusQuery(ctx, devID, devicePhyID, socketNo)

	// 新增：等待设备状态就绪验证
	if err := h.waitForDeviceReady(ctx, devID, devicePhyID, req.PortNo, orderNo, requestID); err != nil {
		// 设备未就绪，返回具体错误信息
		h.logger.Warn("device not ready for charging",
			zap.String("device_phy_id", devicePhyID),
			zap.Int("port_no", req.PortNo),
			zap.String("order_no", orderNo),
			zap.Error(err))

		c.JSON(http.StatusServiceUnavailable, StandardResponse{
			Code: 50301, // DEVICE_NOT_READY
			// EN: device is not ready for charging
			Message: "设备未就绪，无法开始充电",
			Data: map[string]interface{}{
				"device_id":    devicePhyID,
				"port_no":      req.PortNo,
				"order_no":     orderNo,
				"error_code":   "DEVICE_NOT_READY",
				"error_detail": err.Error(),
			},
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// 9. 返回成功响应
	c.JSON(http.StatusOK, StandardResponse{
		Code: 0,
		// EN: charge command sent successfully
		Message: "充电指令发送成功",
		Data: map[string]interface{}{
			"device_id":   devicePhyID,
			"order_no":    orderNo,
			"business_no": int(businessNo),
			"port_no":     req.PortNo,
			"amount":      req.Amount,
			"online":      isOnline,
		},
		RequestID: requestID,
		Timestamp: time.Now().Unix(),
	})
}

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
	SocketUID string `json:"socket_uid" binding:"required"`    // 插座 UID（必填）
	PortNo    *int   `json:"port_no" binding:"required,min=0"` // 端口号：0=A端口, 1=B端口, ...（必填，使用指针避免0值validation问题）
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
	// 尝试从上下文中获取test_session_id（内部测试控制台会注入）
	var testSessionID *string
	if v := ctx.Value("test_session_id"); v != nil {
		if s, ok := v.(string); ok && s != "" {
			testSessionID = &s
		}
	}

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

	// 1. 验证设备存在
	devID, err := h.repo.EnsureDevice(ctx, devicePhyID)
	if err != nil {
		h.logger.Error("failed to get device", zap.Error(err))
		c.JSON(http.StatusInternalServerError, StandardResponse{
			Code: 500,
			// EN: failed to get device
			Message:   "获取设备失败",
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// 1.1 解析 socket_uid 获取 socket_no
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

	// 2. 查询当前活动的订单 - P1-5修复: 支持charging状态
	var orderNo string
	var businessNo int64
	var orderStatus int
	var isFallbackMode bool // 标记是否为降级模式（无订单但端口在充电）

	queryOrderSQL := `
		SELECT order_no, business_no, status FROM orders
		WHERE device_id = $1 AND port_no = $2 AND status IN ($3, $4, $5)
		ORDER BY created_at DESC LIMIT 1
	`
	err = h.repo.Pool.QueryRow(ctx, queryOrderSQL, devID, *req.PortNo,
		OrderStatusPending, OrderStatusConfirmed, OrderStatusCharging).Scan(&orderNo, &businessNo, &orderStatus)
	// 降级逻辑：找不到订单时，检查端口实际状态
	if err != nil {
		h.logger.Warn("no active order found, checking port status for fallback", zap.Error(err))

		// 查询端口实际状态
		var portStatus int
		portQuery := `SELECT status FROM ports WHERE device_id = $1 AND port_no = $2`
		portErr := h.repo.Pool.QueryRow(ctx, portQuery, devID, *req.PortNo).Scan(&portStatus)

		if portErr != nil {
			// 端口记录也不存在
			h.logger.Error("port not found", zap.Error(portErr))
			c.JSON(http.StatusNotFound, StandardResponse{
				Code: 404,
				// EN: no active charging session and port not found
				Message:   "未找到活动的充电会话，且端口不存在",
				RequestID: requestID,
				Timestamp: time.Now().Unix(),
			})
			return
		}

		// 检查端口是否真的在充电（bit7=0x80）
		isActuallyCharging := (portStatus & 0x80) != 0

		if !isActuallyCharging {
			// 端口未在充电，无需停止
			h.logger.Info("port not charging, no action needed",
				zap.Int("port_status", portStatus))
			c.JSON(http.StatusNotFound, StandardResponse{
				Code: 404,
				// EN: no active charging session
				Message: "未找到活动的充电会话",
				Data: map[string]interface{}{
					"port_status":     portStatus,
					"is_charging":     false,
					"fallback_reason": "port not in charging state",
				},
				RequestID: requestID,
				Timestamp: time.Now().Unix(),
			})
			return
		}

		// 端口确实在充电，但无订单记录 → 强制停止（状态修复）
		// 一致性审计: 添加统一的状态字段便于追踪
		h.logger.Warn("consistency: fallback stop triggered - port charging without order",
			// 标准一致性字段
			zap.String("source", "api_stop_charge"),
			zap.String("scenario", "fallback_stop_without_order"),
			zap.String("expected_state", "port_has_active_order"),
			zap.String("actual_state", "port_charging_no_order"),
			// 业务上下文
			zap.String("device_phy_id", devicePhyID),
			zap.Int("port_no", *req.PortNo),
			zap.Int("port_status", portStatus),
			zap.String("port_status_hex", fmt.Sprintf("0x%02x", portStatus)),
		)

		// 生成临时订单号用于日志追踪和异常记录
		orderNo = fmt.Sprintf("FALLBACK_%d_%d", time.Now().Unix(), *req.PortNo)
		businessNo = 0 // 无业务单号
		isFallbackMode = true

		// 写入异常订单记录用于审计（不影响设备停止逻辑）
		failureReason := "fallback_stop_without_order"
		if err := h.repo.InsertFallbackOrder(ctx, devID, *req.PortNo, orderNo, failureReason, testSessionID); err != nil {
			h.logger.Error("failed to insert fallback order",
				zap.String("order_no", orderNo),
				zap.Error(err))
			// 不阻断后续停止指令发送
		}

		// 发送停止指令（复用后面的逻辑）
		// 注意：不更新订单状态（因为订单不存在），直接发送硬件指令
		goto sendStopCommand
	}

	// P1-5修复: 使用CAS更新为stopping中间态
	{
		updateOrderSQL := `
			UPDATE orders
			SET status = $1, updated_at = NOW()
			WHERE order_no = $2 AND status IN ($3, $4, $5)
		`
		result, updateErr := h.repo.Pool.Exec(ctx, updateOrderSQL, OrderStatusStopping, orderNo,
			OrderStatusPending, OrderStatusConfirmed, OrderStatusCharging)
		if updateErr != nil {
			h.logger.Error("failed to update order to stopping", zap.Error(updateErr))
			c.JSON(http.StatusInternalServerError, StandardResponse{
				Code: 500,
				// EN: failed to stop order
				Message:   "停止订单失败",
				RequestID: requestID,
				Timestamp: time.Now().Unix(),
			})
			return
		}
		if result.RowsAffected() == 0 {
			h.logger.Warn("order status changed, cannot stop",
				zap.String("order_no", orderNo))
			c.JSON(http.StatusConflict, StandardResponse{
				Code: 409,
				// EN: order status has changed, cannot stop
				Message:   "订单状态已变更，无法停止",
				RequestID: requestID,
				Timestamp: time.Now().Unix(),
			})
			return
		}
	}

sendStopCommand:
	biz := uint16(businessNo)
	if biz == 0 {
		biz = deriveBusinessNo(orderNo)
	}

	stopCommandSent, dispatchErr := h.dispatchStopChargeCommand(ctx, devicePhyID, devID, socketNo, *req.PortNo, orderNo, biz)
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

	// 可选同步查询：在降级模式下，为了尽量拿到“真实端口状态”，主动发送一次查询插座状态命令(0x001D)，
	// 并在短时间窗口内轮询 ports 表，观察状态是否发生变化。
	if isFallbackMode {
		if err := h.syncPortStatusAfterStop(ctx, devID, devicePhyID, socketNo, *req.PortNo, requestID); err != nil {
			h.logger.Warn("sync port status after fallback stop failed",
				zap.String("device_phy_id", devicePhyID),
				zap.Int("port_no", *req.PortNo),
				zap.Error(err))
		}
	}

	// 4. 返回成功响应
	responseData := map[string]interface{}{
		"device_id":    devicePhyID,
		"port_no":      req.PortNo,
		"business_no":  int(biz),
		"command_sent": stopCommandSent,
	}

	var message string
	if isFallbackMode {
		// 降级模式：无订单但强制停止
		message = "检测到端口状态异常（充电中但无订单记录），已发送强制停止指令"
		responseData["fallback_mode"] = true
		responseData["fallback_order"] = orderNo
		responseData["note"] = "这是状态修复操作，已向设备发送停止指令，端口将恢复空闲状态"
	} else {
		// 正常模式：有订单
		message = "停止指令已发送，订单将在30秒内停止"
		responseData["order_no"] = orderNo
		responseData["status"] = "stopping"
		responseData["note"] = "订单将在30秒后自动变为stopped,或收到设备ACK后立即停止"
	}

	// 将最新端口状态快照附加到响应中，方便调用方直接看到“停止后”的状态视图。
	var latestPortStatus int
	const qPort = `SELECT status FROM ports WHERE device_id = $1 AND port_no = $2`
	if err := h.repo.Pool.QueryRow(ctx, qPort, devID, *req.PortNo).Scan(&latestPortStatus); err == nil {
		isCharging := isBKVChargingStatus(latestPortStatus)
		responseData["port_status"] = latestPortStatus
		responseData["is_charging"] = isCharging
		// state_converged 仅表示“从DB视角看端口不再显示充电”，不代表设备物理层一定已经停止。
		responseData["state_converged"] = !isCharging
	}

	c.JSON(http.StatusOK, StandardResponse{
		Code:      0,
		Message:   message,
		Data:      responseData,
		RequestID: requestID,
		Timestamp: time.Now().Unix(),
	})
}

// syncPortStatusAfterStop 在 fallback 停止后，主动查询一次插座状态并等待短时间窗口内的状态收敛
//
// 设计约束（KISS/YAGNI）：
//   - 不引入复杂的命令-响应配对机制，而是复用已有的 0x001D 查询插座状态能力；
//   - 下发查询命令后，通过轮询 ports 表观测状态变化（最多约3秒），避免长期卡在陈旧快照；
//   - 仅在 fallback 模式下启用，作为“提高确定性”的增强，而不是硬依赖。
func (h *ThirdPartyHandler) syncPortStatusAfterStop(
	ctx context.Context,
	deviceID int64,
	devicePhyID string,
	socketNo int,
	portNo int,
	requestID string,
) error {
	// 1. 读取当前端口状态作为基准
	var initialStatus int
	const qPort = `SELECT status FROM ports WHERE device_id = $1 AND port_no = $2`
	if err := h.repo.Pool.QueryRow(ctx, qPort, deviceID, portNo).Scan(&initialStatus); err != nil {
		// 端口不存在时不强制报错，交由上层一致性任务处理
		return nil
	}

	// 2. 下发一次查询插座状态命令(0x001D)，复用 StartCharge 中的实现约定。
	if err := h.enqueueSocketStatusQuery(ctx, deviceID, devicePhyID, socketNo); err != nil {
		h.logger.Warn("failed to enqueue socket status query after fallback stop",
			zap.String("device_phy_id", devicePhyID),
			zap.Int("port_no", portNo),
			zap.Error(err))
	}

	// 3. 在短时间窗口内轮询 ports 表，观察状态是否有变化
	deadline := time.Now().Add(3 * time.Second)
	pollInterval := 200 * time.Millisecond

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		time.Sleep(pollInterval)

		var currentStatus int
		if err := h.repo.Pool.QueryRow(ctx, qPort, deviceID, portNo).Scan(&currentStatus); err != nil {
			// 查询失败时继续下一轮，避免在这里中断
			continue
		}

		if currentStatus != initialStatus {
			h.logger.Info("port status changed after fallback stop",
				zap.String("device_phy_id", devicePhyID),
				zap.Int("port_no", portNo),
				zap.Int("old_status", initialStatus),
				zap.Int("new_status", currentStatus),
				zap.String("request_id", requestID))
			break
		}
	}

	return nil
}

// CancelOrderRequest P0修复: 取消订单请求
type CancelOrderRequest struct {
	OrderNo string `json:"order_no" binding:"required"`
	Reason  string `json:"reason"`
}

// CancelOrder P0修复: 取消订单
// @Summary 取消订单
// @Description 取消pending或confirmed状态的订单,charging状态订单必须先停止充电
// @Tags 订单管理
// @Accept json
// @Produce json
// @Param order_id path string true "订单号"
// @Param request body CancelOrderRequest true "取消订单参数"
// @Success 200 {object} StandardResponse
// @Failure 400 {object} StandardResponse "订单状态不允许取消"
// @Failure 404 {object} StandardResponse "订单不存在"
// @Router /api/v1/third/orders/{order_id}/cancel [post]
func (h *ThirdPartyHandler) CancelOrder(c *gin.Context) {
	ctx := c.Request.Context()
	requestID := c.GetString("request_id")
	orderNo := c.Param("order_id")

	h.logger.Info("cancel order requested",
		zap.String("order_no", orderNo))

	// 1. 查询订单当前状态
	var orderStatus int
	var deviceID int64
	var portNo int
	queryOrderSQL := `
		SELECT status, device_id, port_no 
		FROM orders 
		WHERE order_no = $1
	`
	err := h.repo.Pool.QueryRow(ctx, queryOrderSQL, orderNo).Scan(&orderStatus, &deviceID, &portNo)
	if err != nil {
		h.logger.Warn("order not found", zap.String("order_no", orderNo), zap.Error(err))
		c.JSON(http.StatusNotFound, StandardResponse{
			Code: 404,
			// EN: order does not exist
			Message:   "订单不存在",
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// 2. P0修复: charging状态订单不允许直接取消
	if orderStatus == OrderStatusCharging {
		h.logger.Warn("cannot cancel charging order",
			zap.String("order_no", orderNo),
			zap.Int("status", orderStatus))
		c.JSON(http.StatusBadRequest, StandardResponse{
			Code:    40001, // ORDER_IS_CHARGING
			Message: "charging状态订单无法直接取消,请先调用停止充电接口",
			Data: map[string]interface{}{
				"order_no":    orderNo,
				"status":      orderStatus,
				"status_name": "charging",
				"error_code":  "ORDER_IS_CHARGING",
			},
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// 3. 检查订单是否可取消(pending=0, confirmed=1)
	if orderStatus != OrderStatusPending && orderStatus != OrderStatusConfirmed {
		h.logger.Warn("order status not cancellable",
			zap.String("order_no", orderNo),
			zap.Int("status", orderStatus))
		c.JSON(http.StatusBadRequest, StandardResponse{
			Code:    400,
			Message: fmt.Sprintf("订单状态%d不允许取消", orderStatus),
			Data: map[string]interface{}{
				"order_no": orderNo,
				"status":   orderStatus,
			},
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// 4. 更新订单状态为cancelling(8)
	updateSQL := `UPDATE orders SET status = $1, updated_at = NOW() WHERE order_no = $2`
	_, err = h.repo.Pool.Exec(ctx, updateSQL, OrderStatusCancelling, orderNo)
	if err != nil {
		h.logger.Error("failed to update order status", zap.Error(err))
		c.JSON(http.StatusInternalServerError, StandardResponse{
			Code: 500,
			// EN: failed to cancel order
			Message:   "取消订单失败",
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// 5. 下发取消指令到设备(如果需要)
	// TODO: 根据业务需求决定是否需要通知设备

	// 6. 返回成功响应
	h.logger.Info("order cancelled successfully",
		zap.String("order_no", orderNo),
		zap.Int("original_status", orderStatus))

	c.JSON(http.StatusOK, StandardResponse{
		Code: 0,
		// EN: cancel command sent, order will be cancelled in 30 seconds
		Message: "取消指令已发送，订单将在30秒内取消",
		Data: map[string]interface{}{
			"order_no": orderNo,
			"status":   "cancelling",
			"note":     "订单将在30秒后自动变为cancelled,或收到设备ACK后立即取消",
		},
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
	devID, err := h.repo.EnsureDevice(ctx, devicePhyID)
	if err != nil {
		h.logger.Error("failed to get device", zap.Error(err))
		c.JSON(http.StatusInternalServerError, StandardResponse{
			Code: 500,
			// EN: failed to get device
			Message:   "获取设备失败",
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// 2. 查询设备基本信息
	var lastSeenAt time.Time
	var createdAt time.Time
	queryDeviceSQL := `SELECT created_at, last_seen_at FROM devices WHERE id = $1`
	err = h.repo.Pool.QueryRow(ctx, queryDeviceSQL, devID).Scan(&createdAt, &lastSeenAt)
	if err != nil {
		h.logger.Error("failed to query device", zap.Error(err))
		c.JSON(http.StatusNotFound, StandardResponse{
			Code: 404,
			// EN: device not found
			Message:   "设备不存在",
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// 3. 检查设备在线状态
	isOnline := h.sess.IsOnline(devicePhyID, time.Now())

	// 4. 查询当前活动订单（如果有）- 仅查询真正的活跃状态
	var activeOrderNo *string
	var activePortNo *int
	queryActiveOrderSQL := `
		SELECT order_no, port_no FROM orders
		WHERE device_id = $1 AND status IN (0, 1, 2)
		ORDER BY created_at DESC LIMIT 1
	`
	err = h.repo.Pool.QueryRow(ctx, queryActiveOrderSQL, devID).Scan(&activeOrderNo, &activePortNo)
	if err != nil {
		// 没有活动订单，忽略错误
		activeOrderNo = nil
	}

	// 5. 返回设备详情
	deviceData := map[string]interface{}{
		"device_id":     devicePhyID,
		"device_db_id":  devID,
		"online":        isOnline,
		"status":        getDeviceStatus(isOnline, activeOrderNo),
		"last_seen_at":  lastSeenAt.Unix(),
		"registered_at": createdAt.Unix(),
	}

	if activeOrderNo != nil {
		deviceData["active_order"] = map[string]interface{}{
			"order_no": *activeOrderNo,
			"port_no":  *activePortNo,
		}
	}

	// 一致性视图: 设备在线状态 / 活跃订单 / 端口状态之间是否一致
	consistencyStatus, inconsistencyReason := h.evaluateDeviceConsistency(ctx, devID, devicePhyID, isOnline, activeOrderNo)
	if consistencyStatus != "" {
		deviceData["consistency_status"] = consistencyStatus
		if inconsistencyReason != "" {
			deviceData["inconsistency_reason"] = inconsistencyReason
		}
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

// GetOrder 查询订单详情
// @Summary 查询订单详情
// @Description 根据订单号查询订单的详细信息和实时进度
// @Tags 第三方API - 订单管理
// @Produce json
// @Security ApiKeyAuth
// @Param order_id path string true "订单号"
// @Success 200 {object} StandardResponse "成功"
// @Failure 404 {object} StandardResponse "订单不存在"
// @Failure 500 {object} StandardResponse "服务器错误"
// @Router /api/v1/third/orders/{order_id} [get]
func (h *ThirdPartyHandler) GetOrder(c *gin.Context) {
	ctx := c.Request.Context()
	orderNo := c.Param("order_id")
	requestID := c.GetString("request_id")

	h.logger.Info("get order requested", zap.String("order_no", orderNo))

	// 查询订单详情
	var deviceID int64
	var amount *int64
	var status int
	var portNo int
	var startTime *time.Time
	var endTime *time.Time
	var kwh *int64
	var createdAt time.Time
	var updatedAt time.Time

	querySQL := `
		SELECT device_id, amount_cent, status, port_no, start_time, end_time, kwh_0p01, created_at, updated_at
		FROM orders 
		WHERE order_no = $1
	`
	err := h.repo.Pool.QueryRow(ctx, querySQL, orderNo).Scan(
		&deviceID, &amount, &status, &portNo, &startTime, &endTime, &kwh, &createdAt, &updatedAt)
	if err != nil {
		h.logger.Warn("order not found", zap.String("order_no", orderNo), zap.Error(err))
		c.JSON(http.StatusNotFound, StandardResponse{
			Code: 404,
			// EN: order not found
			Message:   "订单不存在",
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// 构造响应数据
	orderData := map[string]interface{}{
		"order_no":   orderNo,
		"device_id":  deviceID,
		"port_no":    portNo,
		"status":     getOrderStatusString(status),
		"created_at": createdAt.Unix(),
		"updated_at": updatedAt.Unix(),
	}

	if amount != nil {
		orderData["amount"] = float64(*amount) / 100.0 // 转换为元
	}
	if startTime != nil {
		orderData["start_time"] = startTime.Unix()
	}
	if endTime != nil {
		orderData["end_time"] = endTime.Unix()
	}
	if kwh != nil {
		orderData["energy_kwh"] = float64(*kwh) / 100.0 // 转换为kWh
	}

	// 一致性视图: 检查订单状态与端口/会话是否一致
	consistencyStatus, inconsistencyReason := h.evaluateOrderConsistency(ctx, deviceID, portNo, status)
	if consistencyStatus != "" {
		orderData["consistency_status"] = consistencyStatus
		if inconsistencyReason != "" {
			orderData["inconsistency_reason"] = inconsistencyReason
		}
	}

	c.JSON(http.StatusOK, StandardResponse{
		Code: 0,
		// EN: success
		Message:   "成功",
		Data:      orderData,
		RequestID: requestID,
		Timestamp: time.Now().Unix(),
	})
}

// ListOrders 订单列表（分页）
// @Summary 订单列表查询
// @Description 查询订单列表,支持按设备ID、状态筛选和分页
// @Tags 第三方API - 订单管理
// @Produce json
// @Security ApiKeyAuth
// @Param device_id query string false "设备物理ID"
// @Param status query string false "订单状态:pending/charging/completed"
// @Param page query int false "页码(默认1)"
// @Param page_size query int false "每页数量(默认20,最大100)"
// @Success 200 {object} StandardResponse "成功"
// @Failure 400 {object} StandardResponse "参数错误"
// @Failure 500 {object} StandardResponse "服务器错误"
// @Router /api/v1/third/orders [get]
func (h *ThirdPartyHandler) ListOrders(c *gin.Context) {
	ctx := c.Request.Context()
	requestID := c.GetString("request_id")

	// 解析查询参数
	devicePhyID := c.Query("device_id")
	statusParam := strings.TrimSpace(c.Query("status"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	var statusCodes []int
	if statusParam != "" {
		codes, err := parseOrderStatusFilter(statusParam)
		if err != nil {
			h.logger.Warn("invalid status filter",
				zap.String("status", statusParam),
				zap.Error(err))
			c.JSON(http.StatusBadRequest, StandardResponse{
				Code:      400,
				Message:   fmt.Sprintf("invalid status filter: %s", statusParam),
				RequestID: requestID,
				Timestamp: time.Now().Unix(),
			})
			return
		}
		statusCodes = codes
	}

	h.logger.Info("list orders requested",
		zap.String("device_id", devicePhyID),
		zap.String("status", statusParam),
		zap.Int("page", page),
		zap.Int("page_size", pageSize))

	// 构造查询条件
	whereClauses := []string{}
	args := []interface{}{}
	argIdx := 1

	if devicePhyID != "" {
		// 先获取设备ID
		devID, err := h.repo.EnsureDevice(ctx, devicePhyID)
		if err == nil {
			whereClauses = append(whereClauses, fmt.Sprintf("device_id = $%d", argIdx))
			args = append(args, devID)
			argIdx++
		}
	}

	if len(statusCodes) > 0 {
		if len(statusCodes) == 1 {
			whereClauses = append(whereClauses, fmt.Sprintf("status = $%d", argIdx))
			args = append(args, statusCodes[0])
			argIdx++
		} else {
			placeholders := make([]string, 0, len(statusCodes))
			for _, code := range statusCodes {
				placeholders = append(placeholders, fmt.Sprintf("$%d", argIdx))
				args = append(args, code)
				argIdx++
			}
			whereClauses = append(whereClauses, fmt.Sprintf("status IN (%s)", strings.Join(placeholders, ",")))
		}
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = "WHERE " + whereClauses[0]
		for i := 1; i < len(whereClauses); i++ {
			whereSQL += " AND " + whereClauses[i]
		}
	}

	// 查询总数
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM orders %s", whereSQL)
	var total int
	err := h.repo.Pool.QueryRow(ctx, countSQL, args...).Scan(&total)
	if err != nil {
		h.logger.Error("failed to count orders", zap.Error(err))
		total = 0
	}

	// 查询订单列表
	offset := (page - 1) * pageSize
	args = append(args, pageSize, offset)
	querySQL := fmt.Sprintf(`
		SELECT order_no, device_id, amount_cent, status, port_no, start_time, end_time, kwh_0p01, created_at, updated_at
		FROM orders 
		%s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereSQL, argIdx, argIdx+1)

	rows, err := h.repo.Pool.Query(ctx, querySQL, args...)
	if err != nil {
		h.logger.Error("failed to query orders", zap.Error(err))
		c.JSON(http.StatusInternalServerError, StandardResponse{
			Code:      500,
			Message:   "failed to query orders",
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}
	defer rows.Close()

	orders := []map[string]interface{}{}
	for rows.Next() {
		var orderNo string
		var deviceID int64
		var amount *int64
		var status int
		var portNo int
		var startTime *time.Time
		var endTime *time.Time
		var kwh *int64
		var createdAt time.Time
		var updatedAt time.Time

		err := rows.Scan(&orderNo, &deviceID, &amount, &status, &portNo, &startTime, &endTime, &kwh, &createdAt, &updatedAt)
		if err != nil {
			h.logger.Error("failed to scan order", zap.Error(err))
			continue
		}

		orderData := map[string]interface{}{
			"order_no":   orderNo,
			"device_id":  deviceID,
			"port_no":    portNo,
			"status":     getOrderStatusString(status),
			"created_at": createdAt.Unix(),
			"updated_at": updatedAt.Unix(),
		}

		if amount != nil {
			orderData["amount"] = float64(*amount) / 100.0
		}
		if startTime != nil {
			orderData["start_time"] = startTime.Unix()
		}
		if endTime != nil {
			orderData["end_time"] = endTime.Unix()
		}
		if kwh != nil {
			orderData["energy_kwh"] = float64(*kwh) / 100.0
		}

		orders = append(orders, orderData)
	}

	c.JSON(http.StatusOK, StandardResponse{
		Code:    0,
		Message: "success",
		Data: map[string]interface{}{
			"orders":    orders,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
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

// SetParams 设置参数
// @Summary 设置设备参数
// @Description 批量设置设备运行参数
// @Tags 第三方API - 设备管理
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param device_id path string true "设备物理ID"
// @Param request body SetParamsRequest true "参数列表"
// @Success 200 {object} StandardResponse "成功"
// @Failure 400 {object} StandardResponse "参数错误"
// @Failure 500 {object} StandardResponse "服务器错误"
// @Router /api/v1/third/devices/{device_id}/params [post]
func (h *ThirdPartyHandler) SetParams(c *gin.Context) {
	ctx := c.Request.Context()
	devicePhyID := c.Param("device_id")
	requestID := c.GetString("request_id")

	// 解析请求体
	var req SetParamsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Error("invalid request body", zap.Error(err))
		c.JSON(http.StatusBadRequest, StandardResponse{
			Code:      400,
			Message:   fmt.Sprintf("invalid request: %v", err),
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	h.logger.Info("set params requested",
		zap.String("device_phy_id", devicePhyID),
		zap.Int("param_count", len(req.Params)))

	if h.driverCmd == nil {
		c.JSON(http.StatusServiceUnavailable, StandardResponse{
			Code:      503,
			Message:   "command dispatcher unavailable",
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	params := make([]coremodel.SetParamItem, 0, len(req.Params))
	for _, p := range req.Params {
		params = append(params, coremodel.SetParamItem{
			ID:    int32(p.ID),
			Value: p.Value,
		})
	}

	cmd := &coremodel.CoreCommand{
		Type:      coremodel.CommandSetParams,
		CommandID: fmt.Sprintf("setparams:%s:%d", devicePhyID, time.Now().UnixNano()),
		DeviceID:  coremodel.DeviceID(devicePhyID),
		IssuedAt:  time.Now(),
		SetParams: &coremodel.SetParamsPayload{Params: params},
	}

	if err := h.driverCmd.SendCoreCommand(ctx, cmd); err != nil {
		h.logger.Error("failed to dispatch set params command", zap.Error(err))
		c.JSON(http.StatusInternalServerError, StandardResponse{
			Code:      500,
			Message:   "failed to send param command",
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// 3. 返回成功响应
	c.JSON(http.StatusOK, StandardResponse{
		Code:    0,
		Message: "param command sent successfully",
		Data: map[string]interface{}{
			"device_id":   devicePhyID,
			"param_count": len(req.Params),
		},
		RequestID: requestID,
		Timestamp: time.Now().Unix(),
	})
}

// TriggerOTARequest 触发OTA升级请求
type TriggerOTARequest struct {
	FirmwareURL  string `json:"firmware_url" binding:"required"` // 固件下载URL
	Version      string `json:"version" binding:"required"`      // 固件版本
	MD5          string `json:"md5" binding:"required,len=32"`   // 固件MD5校验
	Size         int    `json:"size" binding:"required,min=1"`   // 固件大小（字节）
	TargetType   int    `json:"target_type" binding:"required"`  // 目标类型：1=网关,2=插座
	TargetSocket int    `json:"target_socket"`                   // 目标插座号（target_type=2时必填）
}

// TriggerOTA 触发OTA升级
// @Summary 触发OTA升级
// @Description 下发固件升级指令到设备
// @Tags 第三方API - OTA管理
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param device_id path string true "设备物理ID"
// @Param request body TriggerOTARequest true "OTA升级参数"
// @Success 200 {object} StandardResponse "成功"
// @Failure 400 {object} StandardResponse "参数错误"
// @Failure 500 {object} StandardResponse "服务器错误"
// @Router /api/v1/third/devices/{device_id}/ota [post]
func (h *ThirdPartyHandler) TriggerOTA(c *gin.Context) {
	ctx := c.Request.Context()
	devicePhyID := c.Param("device_id")
	requestID := c.GetString("request_id")

	// 解析请求体
	var req TriggerOTARequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Error("invalid request body", zap.Error(err))
		c.JSON(http.StatusBadRequest, StandardResponse{
			Code:      400,
			Message:   fmt.Sprintf("invalid request: %v", err),
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	h.logger.Info("trigger ota requested",
		zap.String("device_phy_id", devicePhyID),
		zap.String("version", req.Version),
		zap.Int("target_type", req.TargetType))

	if h.driverCmd == nil {
		c.JSON(http.StatusServiceUnavailable, StandardResponse{
			Code:      503,
			Message:   "command dispatcher unavailable",
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	socket := int32(req.TargetSocket)
	cmd := &coremodel.CoreCommand{
		Type:      coremodel.CommandTriggerOTA,
		CommandID: fmt.Sprintf("ota:%s:%d", devicePhyID, time.Now().UnixNano()),
		DeviceID:  coremodel.DeviceID(devicePhyID),
		IssuedAt:  time.Now(),
		TriggerOTA: &coremodel.TriggerOTAPayload{
			TargetType:   int32(req.TargetType),
			TargetSocket: &socket,
			FirmwareURL:  req.FirmwareURL,
			Version:      req.Version,
			MD5:          req.MD5,
			Size:         int32(req.Size),
		},
	}

	if err := h.driverCmd.SendCoreCommand(ctx, cmd); err != nil {
		h.logger.Error("failed to dispatch ota command", zap.Error(err))
		c.JSON(http.StatusInternalServerError, StandardResponse{
			Code:      500,
			Message:   "failed to send ota command",
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// 4. 返回成功响应
	c.JSON(http.StatusOK, StandardResponse{
		Code:    0,
		Message: "ota command sent successfully",
		Data: map[string]interface{}{
			"device_id":   devicePhyID,
			"version":     req.Version,
			"target_type": req.TargetType,
		},
		RequestID: requestID,
		Timestamp: time.Now().Unix(),
	})
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

// getOrderStatusString 将订单状态码转换为字符串
func getOrderStatusString(status int) string {
	switch status {
	case 0:
		return "pending"
	case 1:
		return "confirmed"
	case 2:
		return "charging"
	case 3:
		return "completed"
	case 4:
		return "failed"
	case 5:
		return "cancelled"
	case 6:
		return "refunded"
	case 7:
		return "settled"
	case 8:
		return "cancelling"
	case 9:
		return "stopping"
	case 10:
		return "interrupted"
	default:
		return "unknown"
	}
}

// enqueueSocketStatusQuery 发送一次查询插座状态命令（0x001D）
//
// 说明：
//   - 目前仅用于 StartCharge/StopCharge 等 API 内部自愈场景；
//   - 使用 Redis 出站队列，交由统一的 worker 写入设备；
//   - 不关心具体响应，仅依赖 HandleSocketStateResponse 将结果写回 ports 表。
func (h *ThirdPartyHandler) enqueueSocketStatusQuery(
	ctx context.Context,
	deviceID int64,
	devicePhyID string,
	socketNo int,
) error {
	return h.dispatchQueryPortStatusCommand(ctx, deviceID, devicePhyID, socketNo)
}

// parseOrderStatusFilter 将查询参数中的订单状态过滤值解析为内部状态码切片
// 支持两种形式：
// 1) 纯数字: "0","1","2"... → 直接转换为对应状态码
// 2) 文本枚举: "pending","charging","completed","failed","cancelled","refunded","settled","cancelling","stopping","interrupted"
func parseOrderStatusFilter(s string) ([]int, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return nil, nil
	}

	// 数字形式: 直接解析为单个状态码
	if code, err := strconv.Atoi(s); err == nil {
		return []int{code}, nil
	}

	switch s {
	case "pending":
		return []int{OrderStatusPending}, nil
	case "confirmed":
		return []int{OrderStatusConfirmed}, nil
	case "charging":
		return []int{OrderStatusCharging}, nil
	case "completed":
		// 包含 completed(3) 和 settled(7)
		return []int{OrderStatusCompleted, 7}, nil
	case "failed":
		// 包含 failed(4) 和 refunded(6)
		return []int{4, 6}, nil
	case "cancelled":
		return []int{OrderStatusCancelled}, nil
	case "refunded":
		return []int{6}, nil
	case "settled":
		return []int{7}, nil
	case "cancelling":
		return []int{OrderStatusCancelling}, nil
	case "stopping":
		return []int{OrderStatusStopping}, nil
	case "interrupted":
		return []int{OrderStatusInterrupted}, nil
	default:
		return nil, fmt.Errorf("unsupported status value: %s", s)
	}
}

// evaluateDeviceConsistency 评估设备的状态一致性（会话/订单/端口快照）
// 返回: consistency_status("ok"/"inconsistent") 以及可选的不一致原因
func (h *ThirdPartyHandler) evaluateDeviceConsistency(ctx context.Context, deviceID int64, devicePhyID string, isOnline bool, activeOrderNo *string) (string, string) {
	// 读取端口快照
	ports, err := h.repo.ListPortsByPhyID(ctx, devicePhyID)
	if err != nil {
		h.logger.Warn("failed to list ports for consistency check",
			zap.String("device_phy_id", devicePhyID),
			zap.Error(err))
		// 查询失败时不强行给出不一致结论
		return "", ""
	}

	// 查询活跃订单（仅包含真正的活跃状态：pending/confirmed/charging）
	// 不包含过渡状态（cancelling/stopping/interrupted），它们应该在30-60秒内流转到终态
	const activeOrderSQL = `
		SELECT order_no, status, port_no
		FROM orders
		WHERE device_id = $1 AND status IN (0,1,2)
	`
	rows, err := h.repo.Pool.Query(ctx, activeOrderSQL, deviceID)
	if err != nil {
		h.logger.Warn("failed to query active orders for consistency check",
			zap.Int64("device_id", deviceID),
			zap.Error(err))
		return "", ""
	}
	defer rows.Close()

	type ord struct {
		no     string
		status int
		port   int
	}
	var activeOrders []ord
	for rows.Next() {
		var o ord
		if err := rows.Scan(&o.no, &o.status, &o.port); err != nil {
			continue
		}
		activeOrders = append(activeOrders, o)
	}

	// 构造端口充电视图
	portCharging := false
	for _, p := range ports {
		if isBKVChargingStatus(p.Status) {
			portCharging = true
			break
		}
	}

	hasActiveOrder := len(activeOrders) > 0

	// 规则1: 设备离线但存在活跃订单
	if !isOnline && hasActiveOrder {
		return "inconsistent", "device_offline_but_active_order"
	}

	// 规则2: 设备在线且端口显示在充电，但没有任何活跃订单
	if isOnline && portCharging && !hasActiveOrder {
		return "inconsistent", "port_charging_without_active_order"
	}

	// 规则3: 设备在线且存在活跃订单，但所有端口都不在充电状态
	if isOnline && hasActiveOrder && !portCharging {
		return "inconsistent", "active_order_but_ports_not_charging"
	}

	// 规则4: 检查过渡状态订单（stopping/cancelling/interrupted）是否长时间未流转
	// 这些状态应该在30-60秒内完成，如果端口仍在充电则说明存在严重不一致
	const transitionOrderSQL = `
		SELECT order_no, status, port_no, updated_at
		FROM orders
		WHERE device_id = $1 AND status IN (8,9,10)
	`
	transitionRows, err := h.repo.Pool.Query(ctx, transitionOrderSQL, deviceID)
	if err == nil {
		defer transitionRows.Close()
		for transitionRows.Next() {
			var orderNo string
			var status int
			var portNo int
			var updatedAt time.Time
			if err := transitionRows.Scan(&orderNo, &status, &portNo, &updatedAt); err != nil {
				continue
			}

			// 按状态选择超时时间窗口:
			// - cancelling(8)/stopping(9): 30秒内视为正常过渡，不标记不一致
			// - interrupted(10): 60秒内视为短暂中断，交由后台任务处理
			var transitionTimeout time.Duration
			switch status {
			case 8, 9:
				transitionTimeout = 30 * time.Second
			case 10:
				transitionTimeout = 60 * time.Second
			default:
				// 理论上不会到这里，兜底给一个较大的窗口
				transitionTimeout = 60 * time.Second
			}

			// 未超过过渡超时时间窗口时，不视为不一致，交由 OrderMonitor/PortStatusSyncer 收敛
			if time.Since(updatedAt) < transitionTimeout {
				continue
			}

			// 检查对应端口是否仍在充电
			for _, p := range ports {
				if p.PortNo == portNo && isBKVChargingStatus(p.Status) {
					// 过渡状态订单 + 端口仍在充电 = 严重不一致
					return "inconsistent", fmt.Sprintf("transition_order_%d_but_port_charging", status)
				}
			}
		}
	}

	return "ok", ""
}

// evaluateOrderConsistency 评估单个订单与端口/设备会话的状态一致性
func (h *ThirdPartyHandler) evaluateOrderConsistency(ctx context.Context, deviceID int64, portNo int, status int) (string, string) {
	// 获取设备phy_id
	const devSQL = `SELECT phy_id FROM devices WHERE id=$1`
	var phyID string
	if err := h.repo.Pool.QueryRow(ctx, devSQL, deviceID).Scan(&phyID); err != nil || phyID == "" {
		return "", ""
	}

	// 会话在线状态
	isOnline := h.sess.IsOnline(phyID, time.Now())

	// 端口快照
	const portSQL = `SELECT status FROM ports WHERE device_id=$1 AND port_no=$2`
	var portStatus int
	if err := h.repo.Pool.QueryRow(ctx, portSQL, deviceID, portNo).Scan(&portStatus); err != nil {
		// 端口不存在时，不做一致性判断
		return "", ""
	}

	isPortCharging := isBKVChargingStatus(portStatus)
	isOrderActive := status == OrderStatusCharging || status == OrderStatusPending ||
		status == OrderStatusConfirmed || status == OrderStatusCancelling ||
		status == OrderStatusStopping || status == OrderStatusInterrupted
	isOrderFinal := status == OrderStatusCompleted || status == OrderStatusCancelled ||
		status == OrderStatusFailed || status == 7 // settled

	// 规则1: 订单仍处于活跃/中间态，但设备已离线
	if isOrderActive && !isOnline {
		return "inconsistent", "order_active_but_device_offline"
	}

	// 规则2: 订单活跃/中间态，但端口并不处于充电状态
	if isOrderActive && !isPortCharging {
		return "inconsistent", "order_active_but_port_not_charging"
	}

	// 规则3: 订单已终态，但端口仍处于充电状态
	if isOrderFinal && isPortCharging {
		return "inconsistent", "order_final_but_port_charging"
	}

	return "ok", ""
}

// isBKVChargingStatus 判断端口状态位图是否表示充电中
// 当前实现与端口状态同步器 PortStatusSyncer 和 BKV TLV 中 PortStatus.IsCharging 保持一致：
// 使用 bit7(0x80) 作为“充电中”标志，bit0(0x01) 作为“在线”标志。
func isBKVChargingStatus(status int) bool {
	return status&0x80 != 0
}

// ===== P1-4修复: 端口状态同步验证 =====

// verifyPortStatus P1-4: 验证端口状态与订单状态一致
// 返回: (isConsistent bool, portStatus int, err error)
func (h *ThirdPartyHandler) verifyPortStatus(ctx context.Context, deviceID int64, portNo int) (bool, int, error) {
	// 优先通过 CoreRepo (GORM) 读取端口快照，避免在核心路径中直接拼接 SQL。
	if h.core != nil {
		port, err := h.core.GetPort(ctx, deviceID, int32(portNo))
		if err != nil {
			// 端口不存在或查询失败
			return false, -1, err
		}
		dbPortStatus := int(port.Status)

		// 验证端口状态：charging(2)表示端口被占用，free(0)或occupied(1)表示可用
		if dbPortStatus == 2 {
			h.logger.Warn("P1-4: port status indicates charging",
				zap.Int64("device_id", deviceID),
				zap.Int("port_no", portNo),
				zap.Int("status", dbPortStatus))
			return false, dbPortStatus, nil
		}

		return true, dbPortStatus, nil
	}

	// 回退路径：直接使用 pgxpool 查询（兼容旧实现）
	var dbPortStatus int
	queryPortSQL := `
SELECT status FROM ports 
WHERE device_id = $1 AND port_no = $2
`
	err := h.repo.Pool.QueryRow(ctx, queryPortSQL, deviceID, portNo).Scan(&dbPortStatus)
	if err != nil {
		// 端口不存在或查询失败
		return false, -1, err
	}

	// TODO P1-4: 这里应该下发0x1012命令同步查询设备实时端口状态
	// 由于0x1012需要同步等待响应(5秒超时)，需要实现命令-响应配对机制
	// 当前仅验证数据库状态，实际部署时需要补充实时查询

	// 验证端口状态：charging(2)表示端口被占用，free(0)或occupied(1)表示可用
	if dbPortStatus == 2 {
		h.logger.Warn("P1-4: port status indicates charging",
			zap.Int64("device_id", deviceID),
			zap.Int("port_no", portNo),
			zap.Int("status", dbPortStatus))
		return false, dbPortStatus, nil
	}

	return true, dbPortStatus, nil
}

// syncPortStatusPeriodic P1-4: 定期同步所有在线设备的端口状态
// 应该在后台goroutine中每5分钟调用一次
func (h *ThirdPartyHandler) syncPortStatusPeriodic(ctx context.Context) error {
	// TODO P1-4: 实现定期同步逻辑
	// 1. 查询所有在线设备
	// 2. 对每个设备下发0x1012查询所有端口状态
	// 3. 比对数据库状态，记录不一致情况
	// 4. 触发告警或自动修正

	h.logger.Debug("P1-4: periodic port status sync (not fully implemented)")
	return nil
}

// GetOrderEvents P1-7完善: 查询订单的所有事件（兜底接口）
// @Summary 查询订单事件
// @Description 查询订单的所有事件列表，按序列号排序。用于事件推送失败时的兜底查询。
// @Tags 第三方API - 订单管理
// @Produce json
// @Security ApiKeyAuth
// @Param order_id path string true "订单号"
// @Success 200 {object} StandardResponse "成功"
// @Failure 404 {object} StandardResponse "订单不存在"
// @Failure 500 {object} StandardResponse "服务器错误"
// @Router /api/v1/third/orders/{order_id}/events [get]
func (h *ThirdPartyHandler) GetOrderEvents(c *gin.Context) {
	ctx := c.Request.Context()
	orderNo := c.Param("order_id")
	requestID := c.GetString("request_id")

	h.logger.Info("get order events requested",
		zap.String("order_no", orderNo),
		zap.String("request_id", requestID))

	// 查询订单事件
	events, err := h.repo.GetOrderEvents(ctx, orderNo)
	if err != nil {
		h.logger.Error("failed to get order events",
			zap.String("order_no", orderNo),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, StandardResponse{
			Code:      500,
			Message:   "failed to get order events",
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// 如果没有事件，返回空数组而非404
	if len(events) == 0 {
		c.JSON(http.StatusOK, StandardResponse{
			Code:    0,
			Message: "success",
			Data: map[string]interface{}{
				"order_no":     orderNo,
				"events":       []interface{}{},
				"total_events": 0,
			},
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// 构造响应
	eventList := make([]map[string]interface{}, 0, len(events))
	for _, e := range events {
		eventMap := map[string]interface{}{
			"event_id":    e.ID,
			"event_type":  e.EventType,
			"sequence_no": e.SequenceNo,
			"status":      e.Status, // 0=待推送, 1=已推送, 2=失败
			"retry_count": e.RetryCount,
			"created_at":  e.CreatedAt.Unix(),
		}

		// 可选字段
		if e.PushedAt != nil {
			eventMap["pushed_at"] = e.PushedAt.Unix()
		}
		if e.ErrorMessage != nil {
			eventMap["error_message"] = *e.ErrorMessage
		}

		// 解析事件数据
		var eventData map[string]interface{}
		if err := json.Unmarshal(e.EventData, &eventData); err == nil {
			eventMap["data"] = eventData
		}

		eventList = append(eventList, eventMap)
	}

	c.JSON(http.StatusOK, StandardResponse{
		Code:    0,
		Message: "success",
		Data: map[string]interface{}{
			"order_no":     orderNo,
			"events":       eventList,
			"total_events": len(events),
		},
		RequestID: requestID,
		Timestamp: time.Now().Unix(),
	})
}

// waitForDeviceReady 等待设备就绪状态验证
// 在发送充电命令后，轮询检查设备状态是否从空载转为可充电状态
func (h *ThirdPartyHandler) waitForDeviceReady(ctx context.Context, deviceID int64, devicePhyID string, portNo int, orderNo string, requestID string) error {
	// 最大等待时间5秒，轮询间隔500ms
	maxWait := 5 * time.Second
	pollInterval := 500 * time.Millisecond
	deadline := time.Now().Add(maxWait)

	h.logger.Info("waiting for device ready state",
		zap.String("device_phy_id", devicePhyID),
		zap.Int("port_no", portNo),
		zap.String("order_no", orderNo),
		zap.Duration("max_wait", maxWait))

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// 查询当前端口状态
		var portStatus int
		querySQL := `SELECT status FROM ports WHERE device_id = $1 AND port_no = $2`
		err := h.repo.Pool.QueryRow(ctx, querySQL, deviceID, portNo).Scan(&portStatus)
		if err != nil {
			h.logger.Warn("failed to query port status during ready check",
				zap.String("device_phy_id", devicePhyID),
				zap.Int("port_no", portNo),
				zap.Error(err))
			// 查询失败时继续等待，不立即返回错误
			time.Sleep(pollInterval)
			continue
		}

		// 分析状态位 - 与 BKV PortStatus/端口状态同步器保持一致：
		//   bit7(0x80): 充电中
		//   bit3(0x08): 空载/空闲
		//   bit0(0x01): 在线
		isOnline := (portStatus & 0x01) != 0   // bit0=1表示在线
		isCharging := (portStatus & 0x80) != 0 // bit7=1表示充电中
		isNoLoad := (portStatus & 0x08) != 0   // bit3=1表示空载/空闲

		h.logger.Debug("device state check",
			zap.String("device_phy_id", devicePhyID),
			zap.Int("port_no", portNo),
			zap.Int("port_status", portStatus),
			zap.String("port_status_hex", fmt.Sprintf("0x%02x", portStatus)),
			zap.Bool("is_online", isOnline),
			zap.Bool("is_charging", isCharging),
			zap.Bool("is_no_load", isNoLoad))

		// 设备就绪条件：
		// 1. 必须在线
		// 2. 不能处于空载状态（空载表示设备拒绝充电）
		// 3. 如果已经开始充电，则视为就绪
		if !isOnline {
			return fmt.Errorf("device offline (status=0x%02x)", portStatus)
		}

		if isCharging {
			// 设备已经开始充电，视为就绪
			h.logger.Info("device ready - charging started",
				zap.String("device_phy_id", devicePhyID),
				zap.Int("port_no", portNo),
				zap.String("order_no", orderNo))
			return nil
		}

		if isNoLoad {
			// 设备处于空载状态，继续等待
			h.logger.Debug("device in no-load state, waiting",
				zap.String("device_phy_id", devicePhyID),
				zap.Int("port_no", portNo),
				zap.Int("port_status", portStatus))
			time.Sleep(pollInterval)
			continue
		}

		// 设备在线且不在空载状态，视为就绪
		h.logger.Info("device ready - online and not no-load",
			zap.String("device_phy_id", devicePhyID),
			zap.Int("port_no", portNo),
			zap.String("order_no", orderNo),
			zap.Int("port_status", portStatus))
		return nil
	}

	// 超时：设备仍未就绪 - 获取最终状态
	var finalStatus int
	finalQuerySQL := `SELECT status FROM ports WHERE device_id = $1 AND port_no = $2`
	_ = h.repo.Pool.QueryRow(ctx, finalQuerySQL, deviceID, portNo).Scan(&finalStatus)
	return fmt.Errorf("device not ready after %v (final_status=0x%02x)", maxWait, finalStatus)
}
