package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/taoyao-code/iot-server/internal/metrics"
	"github.com/taoyao-code/iot-server/internal/outbound"
	"github.com/taoyao-code/iot-server/internal/protocol/bkv"
	"github.com/taoyao-code/iot-server/internal/session"
	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
	redisstorage "github.com/taoyao-code/iot-server/internal/storage/redis"
	"github.com/taoyao-code/iot-server/internal/thirdparty"
	"go.uber.org/zap"
)

// ThirdPartyHandler 第三方API处理器
type ThirdPartyHandler struct {
	repo       *pgstorage.Repository
	sess       session.SessionManager
	outboundQ  *redisstorage.OutboundQueue
	eventQueue *thirdparty.EventQueue
	metrics    *metrics.AppMetrics // 一致性监控指标
	logger     *zap.Logger
}

// NewThirdPartyHandler 创建第三方API处理器
func NewThirdPartyHandler(
	repo *pgstorage.Repository,
	sess session.SessionManager,
	outboundQ *redisstorage.OutboundQueue,
	eventQueue *thirdparty.EventQueue,
	metrics *metrics.AppMetrics,
	logger *zap.Logger,
) *ThirdPartyHandler {
	return &ThirdPartyHandler{
		repo:       repo,
		sess:       sess,
		outboundQ:  outboundQ,
		eventQueue: eventQueue,
		metrics:    metrics,
		logger:     logger,
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
	PortNo          int `json:"port_no" binding:"min=0"`                    // 端口号：0=A端口, 1=B端口, ...（移除required，因为0是有效值）
	ChargeMode      int `json:"charge_mode" binding:"required,min=1,max=4"` // 充电模式：1=按时长,2=按电量,3=按功率,4=充满自停
	Amount          int `json:"amount" binding:"required,min=1"`            // 金额（分）
	DurationMinutes int `json:"duration_minutes"`                           // 时长（分钟）- 推荐使用
	Duration        int `json:"duration"`                                   // 时长（分钟）- 兼容旧版
	Power           int `json:"power"`                                      // 功率（瓦）
	PricePerKwh     int `json:"price_per_kwh"`                              // 电价（分/度）
	ServiceFee      int `json:"service_fee"`                                // 服务费率（千分比）
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
		zap.Int("amount", req.Amount))

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

	// 3. 清理超时的pending订单（超过5分钟自动取消）
	cleanupSQL := `
		UPDATE orders 
		SET status = 3, updated_at = NOW()
		WHERE device_id = $1 AND status = 0 
		  AND created_at < NOW() - INTERVAL '5 minutes'
	`
	cleanupResult, _ := h.repo.Pool.Exec(ctx, cleanupSQL, devID)
	if cleanupResult.RowsAffected() > 0 {
		h.logger.Info("cleaned up stale pending orders",
			zap.String("device_phy_id", devicePhyID),
			zap.Int64("count", cleanupResult.RowsAffected()))
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

	// 4. P1-3修复: 使用事务+行锁检查端口并创建订单
	tx, err := h.repo.Pool.Begin(ctx)
	if err != nil {
		h.logger.Error("failed to begin transaction", zap.Error(err))
		c.JSON(http.StatusInternalServerError, StandardResponse{
			Code: 500,
			// EN: database error
			Message:   "数据库错误",
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}
	defer tx.Rollback(ctx)

	// 4.1. 同时锁定orders和ports表（P1-3完整方案：防止跨表状态不一致）
	// 🔥 关键修复: 使用SKIP LOCKED快速失败，锁定所有活跃订单
	// 注意：这里需要包含过渡状态（8,9,10），因为正在stopping/cancelling的订单仍然占用端口
	// 应该等待过渡状态完成后才能创建新订单
	var existingOrderNo string
	checkPortSQL := `
		SELECT order_no FROM orders
		WHERE device_id = $1 AND port_no = $2
		  AND status IN (0, 1, 2, 8, 9, 10)  -- pending, confirmed, charging, cancelling, stopping, interrupted
		ORDER BY created_at DESC
		FOR UPDATE SKIP LOCKED
	`
	rows, err := tx.Query(ctx, checkPortSQL, devID, req.PortNo)
	if err != nil {
		tx.Rollback(ctx)
		h.logger.Error("failed to check port", zap.Error(err))
		c.JSON(http.StatusInternalServerError, StandardResponse{
			Code: 500,
			// EN: database error
			Message:   "数据库错误",
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}
	defer rows.Close()

	// 检查是否有活跃订单
	if rows.Next() {
		if err := rows.Scan(&existingOrderNo); err == nil {
			// 端口已被占用，查询真实的端口状态
			var actualPortStatus int
			portStatusQuery := `SELECT status FROM ports WHERE device_id = $1 AND port_no = $2`
			portStatusErr := h.repo.Pool.QueryRow(ctx, portStatusQuery, devID, req.PortNo).Scan(&actualPortStatus)

			portStatusText := "unknown"
			if portStatusErr == nil {
				// 转换端口状态为可读文本
				switch actualPortStatus {
				case 0:
					portStatusText = "free"
				case 1:
					portStatusText = "occupied"
				case 2:
					portStatusText = "charging"
				case 3:
					portStatusText = "fault"
				default:
					portStatusText = fmt.Sprintf("unknown(%d)", actualPortStatus)
				}
			}

			tx.Rollback(ctx)
			h.logger.Warn("port already in use",
				zap.String("device_phy_id", devicePhyID),
				zap.Int("port_no", req.PortNo),
				zap.String("existing_order", existingOrderNo),
				zap.Int("actual_port_status", actualPortStatus),
				zap.String("port_status_text", portStatusText))
			c.JSON(http.StatusConflict, StandardResponse{
				Code: 409,
				// EN: port is busy
				Message: "端口正在使用中",
				Data: map[string]interface{}{
					"current_order": existingOrderNo,
					"port_status":   portStatusText,
				},
				RequestID: requestID,
				Timestamp: time.Now().Unix(),
			})
			return
		}
	}

	// 4.2. P1-3: 同时锁定ports表，防止端口状态被其他事务修改
	lockPortSQL := `
		SELECT status FROM ports
		WHERE device_id = $1 AND port_no = $2
		FOR UPDATE
	`
	var lockedPortStatus int
	err = tx.QueryRow(ctx, lockPortSQL, devID, req.PortNo).Scan(&lockedPortStatus)
	if err != nil {
		// 端口不存在，需要先初始化
		initPortSQL := `
			INSERT INTO ports (device_id, port_no, status, updated_at)
			VALUES ($1, $2, 0, NOW())
			ON CONFLICT (device_id, port_no) DO NOTHING
		`
		_, _ = tx.Exec(ctx, initPortSQL, devID, req.PortNo)
		lockedPortStatus = 0
	}

	// 4.3. P1-3: 验证端口状态是否可用
	if lockedPortStatus == 2 {
		// 端口状态为charging但没有活跃订单，数据不一致
		tx.Rollback(ctx)
		h.logger.Error("P1-3: port state mismatch - charging status without active order",
			zap.String("device_phy_id", devicePhyID),
			zap.Int("port_no", req.PortNo),
			zap.Int("port_status", lockedPortStatus))
		c.JSON(http.StatusConflict, StandardResponse{
			Code: 40903, // PORT_STATE_INCONSISTENT
			// EN: port state inconsistent, please retry
			Message: "端口状态不一致，请重试",
			Data: map[string]interface{}{
				"port_no":     req.PortNo,
				"port_status": lockedPortStatus,
				"error_code":  "PORT_STATE_INCONSISTENT",
			},
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	} else if lockedPortStatus == 3 {
		// 端口故障
		tx.Rollback(ctx)
		h.logger.Warn("port is in fault state",
			zap.String("device_phy_id", devicePhyID),
			zap.Int("port_no", req.PortNo))
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
	}

	// 5. 生成订单号并派生业务号（BKV要求）
	orderNo := fmt.Sprintf("THD%d%03d", time.Now().Unix(), req.PortNo)
	businessNo := deriveBusinessNo(orderNo)

	// 6. 在同一事务中创建订单记录
	insertOrderSQL := `
		INSERT INTO orders (device_id, order_no, business_no, amount_cent, status, port_no, charge_mode, created_at)
		VALUES ($1, $2, $3, $4, 0, $5, $6, NOW())
	`
	_, err = tx.Exec(ctx, insertOrderSQL, devID, orderNo, businessNo, req.Amount, req.PortNo, req.ChargeMode)
	if err != nil {
		tx.Rollback(ctx)
		h.logger.Error("failed to create order", zap.Error(err))
		c.JSON(http.StatusInternalServerError, StandardResponse{
			Code: 500,
			// EN: failed to create order
			Message:   "创建订单失败",
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// 7. 提交事务
	if err := tx.Commit(ctx); err != nil {
		h.logger.Error("failed to commit transaction", zap.Error(err))
		c.JSON(http.StatusInternalServerError, StandardResponse{
			Code: 500,
			// EN: database error
			Message:   "数据库错误",
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	h.logger.Info("P1-3: order created with port locked",
		zap.String("order_no", orderNo),
		zap.Int64("device_id", devID),
		zap.Int("port_no", req.PortNo))

	// 8. 构造并下发充电指令（BKV 0x0015下行）
	// 按协议 2.2.8：外层BKV命令0x0015，内层控制命令0x07
	if h.outboundQ != nil {
		msgID := uint32(time.Now().Unix() % 65536)
		mapped := uint8(mapPort(req.PortNo))
		biz := businessNo
		// 🔧 修复：使用 GetDuration() 获取时长参数
		durationMin := uint16(req.GetDuration())

		// 🔥 插座号设置（硬件层核心参数）
		//
		// 【重要】插座号决定了目标设备，必须根据设备类型选择：
		//
		// 1. 单机版设备（如82241218000382）：
		//    - 仅支持插座号 0（默认/唯一插座）
		//    - 设备内有2个物理插孔（A孔/B孔），通过portNo区分
		//    - 如果使用插座号1/2，设备会返回ACK失败（result=00）
		//
		// 2. 组网版设备（待对接）：
		//    - 支持插座号 1-250（多个独立插座通过网关管理）
		//    - 每个插座有独立MAC地址和UID编号
		//    - 需要先通过2.2.5/2.2.6命令下发网络节点列表
		//
		// 【协议依据】设备对接指引-组网设备2024(1).txt：
		//   - 2.2.8 控制命令格式：[长度][0x07][插座号][插孔号][开关][模式][时长][业务号]
		//   - 单机版协议示例中插座号始终为0
		//
		// 【测试验证】生产环境（2025-10-31）：
		//   - 插座号=1 → 设备ACK: 00 01 00（失败）
		//   - 插座号=2 → 设备ACK: 00 02 00（失败）
		//   - 插座号=0 → 设备ACK: 01 00 00（成功）✅
		socketNo := uint8(0)

		// 构造内层payload（命令0x07 + 参数）
		innerPayload := h.encodeStartControlPayload(socketNo, mapped, uint8(req.ChargeMode), durationMin, biz) // 【关键修复】根据组网设备协议2.2.8，长度字段=参数字节数（不含0x07命令字节）
		// 协议示例: 0008 07 02 00 01 01 00f0 0000
		//          ^^^^ 长度=8 (后面8字节参数，不含07)
		// 格式：[参数长度(2字节)] + [07命令] + [参数]
		paramLen := len(innerPayload) - 1 // 去掉0x07命令字节
		payload := make([]byte, 2+len(innerPayload))
		payload[0] = byte(paramLen >> 8) // 参数长度高字节
		payload[1] = byte(paramLen)      // 参数长度低字节
		copy(payload[2:], innerPayload)  // 完整内层payload(含0x07)

		h.logger.Info("DEBUG: payload生成", zap.Int("inner_len", len(innerPayload)), zap.Int("total_len", len(payload)), zap.String("payload_hex", fmt.Sprintf("%x", payload)))
		// 构造外层BKV帧（命令0x0015）
		frame := bkv.Build(0x0015, msgID, devicePhyID, payload)
		h.logger.Info("DEBUG: BKV帧生成", zap.Int("frame_len", len(frame)), zap.String("frame_hex", fmt.Sprintf("%x", frame)))

		err = h.outboundQ.Enqueue(ctx, &redisstorage.OutboundMessage{
			ID:        fmt.Sprintf("api_%d", msgID),
			DeviceID:  devID,
			PhyID:     devicePhyID,
			Command:   frame,
			Priority:  outbound.PriorityHigh, // P1-6: 启动充电=高优先级
			MaxRetry:  3,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Timeout:   5000,
		})
		if err != nil {
			h.logger.Error("failed to push charge command",
				zap.Error(err),
				zap.String("order_no", orderNo),
				zap.String("device_phy_id", devicePhyID))

			// 一致性修复: 命令入队失败时返回错误，确保原子性语义
			// 符合规范要求: "从客户端视角看要么都成功，要么都失败"
			// 虽然订单已创建，但 OrderMonitor 会在超时后自动清理 pending 订单
			c.JSON(http.StatusInternalServerError, StandardResponse{
				Code: 500,
				// EN: charge command enqueue failed
				Message: "充电命令入队失败，请稍后重试",
				Data: map[string]interface{}{
					"order_no":   orderNo,
					"device_id":  devicePhyID,
					"reason":     "queue_enqueue_failed",
					"retry_hint": "pending订单将在5分钟后自动清理，请稍后重试",
				},
				RequestID: requestID,
				Timestamp: time.Now().Unix(),
			})
			return
		}

		h.logger.Info("charge command pushed",
			zap.String("order_no", orderNo),
			zap.String("device_phy_id", devicePhyID),
			zap.Int("port_no", req.PortNo))

		// 主动查询插座状态（0x001D），避免仅依赖周期性0x94
		_ = h.enqueueSocketStatusQuery(ctx, devID, devicePhyID, 0)
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

// StopChargeRequest 停止充电请求
type StopChargeRequest struct {
	PortNo *int `json:"port_no" binding:"required,min=0"` // 端口号：0=A端口, 1=B端口, ...（必填，使用指针避免0值validation问题）
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

	// 3. 下发停止充电指令（BKV 0x0015控制设备）
	stopCommandSent := false
	if h.outboundQ != nil {
		msgID := uint32(time.Now().Unix() % 65536)
		// 构造停止充电控制负载：socketNo=0, port 映射, switch=0
		innerStopData := h.encodeStopControlPayload(uint8(0), uint8(mapPort(*req.PortNo)), biz)

		// 【关键修复】长度=参数字节数（不含0x07）
		stopParamLen := len(innerStopData) - 1
		stopData := make([]byte, 2+len(innerStopData))
		stopData[0] = byte(stopParamLen >> 8)
		stopData[1] = byte(stopParamLen)
		copy(stopData[2:], innerStopData)

		err = h.outboundQ.Enqueue(ctx, &redisstorage.OutboundMessage{
			ID:        fmt.Sprintf("api_%d", msgID),
			DeviceID:  devID,
			PhyID:     devicePhyID,
			Command:   bkv.Build(0x0015, msgID, devicePhyID, stopData),
			Priority:  outbound.PriorityEmergency, // P1-6: 停止充电=紧急优先级
			MaxRetry:  3,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Timeout:   5000,
		})
		if err != nil {
			h.logger.Error("failed to push stop command", zap.Error(err))
		} else {
			h.logger.Info("stop command pushed", zap.String("order_no", orderNo))
			stopCommandSent = true
		}
	}

	// 可选同步查询：在降级模式下，为了尽量拿到“真实端口状态”，主动发送一次查询插座状态命令(0x001D)，
	// 并在短时间窗口内轮询 ports 表，观察状态是否发生变化。
	if isFallbackMode {
		if err := h.syncPortStatusAfterStop(ctx, devID, devicePhyID, *req.PortNo, requestID); err != nil {
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

	// 2. 下发一次查询插座状态命令(0x001D)，复用 StartCharge 中的实现约定：
	//   - 单机版设备使用 socketNo=0；
	//   - 组网设备可按插座号扩展，这里先与现有行为保持一致。
	if h.outboundQ != nil {
		if err := h.enqueueSocketStatusQuery(ctx, deviceID, devicePhyID, 0 /*socketNo*/); err != nil {
			// 查询命令发送失败不影响停止本身，仅记录告警
			h.logger.Warn("failed to enqueue socket status query after fallback stop",
				zap.String("device_phy_id", devicePhyID),
				zap.Int("port_no", portNo),
				zap.Error(err))
		}
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

	// 1. 验证设备存在
	_, err := h.repo.EnsureDevice(ctx, devicePhyID)
	if err != nil {
		h.logger.Error("failed to get device", zap.Error(err))
		c.JSON(http.StatusInternalServerError, StandardResponse{
			Code:      500,
			Message:   "failed to get device",
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// 2. 下发参数写入指令（BKV 0x0002）
	if h.outboundQ != nil {
		msgID := uint32(time.Now().Unix() % 65536)

		// 构造参数写入指令payload
		// 格式：参数个数(1字节) + [参数ID(1字节) + 参数值长度(1字节) + 参数值(N字节)]...
		paramData := []byte{byte(len(req.Params))}
		for _, p := range req.Params {
			paramValue := []byte(p.Value)
			paramData = append(paramData, byte(p.ID), byte(len(paramValue)))
			paramData = append(paramData, paramValue...)
		}

		// 获取设备ID（前面已验证过）
		devID, _ := h.repo.EnsureDevice(ctx, devicePhyID)
		err = h.outboundQ.Enqueue(ctx, &redisstorage.OutboundMessage{
			ID:        fmt.Sprintf("api_%d", msgID),
			DeviceID:  devID,
			PhyID:     devicePhyID,
			Command:   paramData,
			Priority:  outbound.PriorityNormal, // P1-6: 参数设置=普通优先级
			MaxRetry:  3,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Timeout:   5000,
		})
		if err != nil {
			h.logger.Error("failed to push param write command", zap.Error(err))
			c.JSON(http.StatusInternalServerError, StandardResponse{
				Code:      500,
				Message:   "failed to send param command",
				RequestID: requestID,
				Timestamp: time.Now().Unix(),
			})
			return
		}
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

	// 1. 验证设备存在
	devID, err := h.repo.EnsureDevice(ctx, devicePhyID)
	if err != nil {
		h.logger.Error("failed to get device", zap.Error(err))
		c.JSON(http.StatusInternalServerError, StandardResponse{
			Code:      500,
			Message:   "failed to get device",
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// 2. 创建OTA任务记录（如果有ota_tasks表）
	// 这里简化处理，直接下发OTA指令

	// 3. 下发OTA升级指令（BKV 0x0007）
	if h.outboundQ != nil {
		msgID := uint32(time.Now().Unix() % 65536)

		// 构造OTA指令payload（简化版）
		// 实际格式需要根据BKV协议规范
		otaData := []byte{
			byte(req.TargetType),   // 目标类型
			byte(req.TargetSocket), // 目标插座号
		}
		// 追加URL、版本等信息（简化处理）
		otaData = append(otaData, []byte(req.FirmwareURL)...)

		err = h.outboundQ.Enqueue(ctx, &redisstorage.OutboundMessage{
			ID:        fmt.Sprintf("api_%d", msgID),
			DeviceID:  devID,
			PhyID:     devicePhyID,
			Command:   otaData,
			Priority:  outbound.PriorityLow, // P1-6: OTA升级=低优先级
			MaxRetry:  3,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Timeout:   5000,
		})
		if err != nil {
			h.logger.Error("failed to push ota command", zap.Error(err))
			c.JSON(http.StatusInternalServerError, StandardResponse{
				Code:      500,
				Message:   "failed to send ota command",
				RequestID: requestID,
				Timestamp: time.Now().Unix(),
			})
			return
		}
	}

	// 4. 返回成功响应
	c.JSON(http.StatusOK, StandardResponse{
		Code:    0,
		Message: "ota command sent successfully",
		Data: map[string]interface{}{
			"device_id":    devicePhyID,
			"device_db_id": devID,
			"version":      req.Version,
			"target_type":  req.TargetType,
		},
		RequestID: requestID,
		Timestamp: time.Now().Unix(),
	})
}

// ===== 辅助函数 =====

// encodeChargeCommand 编码充电指令（简化版本）
// 实际应使用 internal/protocol/bkv/card.go 中的 ChargeCommand.Encode()
func (h *ThirdPartyHandler) encodeChargeCommand(orderNo string, chargeMode uint8, amount, duration uint32, power uint16, pricePerKwh uint32, serviceFee uint16) []byte {
	// 这里返回简化的payload
	// 实际应该使用完整的BKV编码
	data := make([]byte, 0, 64)

	// 订单号（16字节，定长）
	orderBytes := make([]byte, 16)
	copy(orderBytes, orderNo)
	data = append(data, orderBytes...)

	// 充电模式（1字节）
	data = append(data, chargeMode)

	// 金额（4字节）
	data = append(data, byte(amount>>24), byte(amount>>16), byte(amount>>8), byte(amount))

	// 时长（4字节）
	data = append(data, byte(duration>>24), byte(duration>>16), byte(duration>>8), byte(duration))

	// 功率（2字节）
	data = append(data, byte(power>>8), byte(power))

	// 电价（4字节）
	data = append(data, byte(pricePerKwh>>24), byte(pricePerKwh>>16), byte(pricePerKwh>>8), byte(pricePerKwh))

	// 服务费率（2字节）
	data = append(data, byte(serviceFee>>8), byte(serviceFee))

	return data
}

// mapPort 将业务端口号(1/2)映射为协议端口(0=A,1=B)
func mapPort(port int) int {
	if port < 0 {
		return 0
	}
	return port
}

// encodeStartControlPayload 构造0x0015开始充电控制负载
// 格式：[0x07][插座1B][插孔1B][开关1B][模式1B][时长2B][业务号2B]
// 参考：docs/协议/BKV设备对接总结.md 2.1节
func (h *ThirdPartyHandler) encodeStartControlPayload(socketNo uint8, port uint8, mode uint8, durationMin uint16, businessNo uint16) []byte {
	// 0x0015控制命令：按协议格式
	buf := make([]byte, 9)
	buf[0] = 0x07                   // BKV子命令：0x07=控制命令
	buf[1] = socketNo               // 插座号
	buf[2] = port                   // 插孔号 (0=A孔, 1=B孔)
	buf[3] = 0x01                   // 开关：1=开启, 0=关闭
	buf[4] = mode                   // 充电模式：1=按时长,0=按电量
	buf[5] = byte(durationMin >> 8) // 时长高字节
	buf[6] = byte(durationMin)      // 时长低字节
	buf[7] = byte(businessNo >> 8)  // 业务号高字节
	buf[8] = byte(businessNo)       // 业务号低字节
	return buf
}

// encodeStopControlPayload 构造0x0015停止充电控制负载
func (h *ThirdPartyHandler) encodeStopControlPayload(socketNo uint8, port uint8, businessNo uint16) []byte {
	// 0x0015停止命令：开关=0表示关闭
	// 格式：[0x07][插座1B][插孔1B][开关1B][模式1B][时长2B][业务号2B]
	buf := make([]byte, 9)
	buf[0] = 0x07     // BKV子命令：0x07=控制命令
	buf[1] = socketNo // 插座号
	buf[2] = port     // 插孔号
	buf[3] = 0x00     // 开关：0=关闭
	buf[4] = 0x01     // 模式（停止时无意义，填1）
	buf[5] = 0x00     // 时长高字节
	buf[6] = 0x00     // 时长低字节
	buf[7] = byte(businessNo >> 8)
	buf[8] = byte(businessNo)
	return buf
}

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
	if h.outboundQ == nil {
		return nil
	}

	msgID := uint32(time.Now().Unix() % 65536)

	// 构造查询插座命令 payload：仅包含插座号
	qInnerPayload := bkv.EncodeQuerySocketCommand(&bkv.QuerySocketCommand{SocketNo: uint8(socketNo)})

	// 查询命令长度=参数字节数（无子命令字节）
	qParamLen := len(qInnerPayload)
	qPayload := make([]byte, 2+len(qInnerPayload))
	qPayload[0] = byte(qParamLen >> 8)
	qPayload[1] = byte(qParamLen)
	copy(qPayload[2:], qInnerPayload)

	frame := bkv.Build(0x001D, msgID, devicePhyID, qPayload)

	return h.outboundQ.Enqueue(ctx, &redisstorage.OutboundMessage{
		ID:        fmt.Sprintf("api_%d", msgID),
		DeviceID:  deviceID,
		PhyID:     devicePhyID,
		Command:   frame,
		Priority:  outbound.PriorityHigh,
		MaxRetry:  2,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Timeout:   3000,
	})
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
	// 查询数据库中的端口状态
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
