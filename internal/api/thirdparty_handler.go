package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
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
	logger     *zap.Logger
}

// NewThirdPartyHandler 创建第三方API处理器
func NewThirdPartyHandler(
	repo *pgstorage.Repository,
	sess session.SessionManager,
	outboundQ *redisstorage.OutboundQueue,
	eventQueue *thirdparty.EventQueue,
	logger *zap.Logger,
) *ThirdPartyHandler {
	return &ThirdPartyHandler{
		repo:       repo,
		sess:       sess,
		outboundQ:  outboundQ,
		eventQueue: eventQueue,
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
	PortNo      int `json:"port_no" binding:"required,min=1"`           // 端口号
	ChargeMode  int `json:"charge_mode" binding:"required,min=1,max=4"` // 充电模式：1=按时长,2=按电量,3=按功率,4=充满自停
	Amount      int `json:"amount" binding:"required,min=1"`            // 金额（分）
	Duration    int `json:"duration"`                                   // 时长（分钟）
	Power       int `json:"power"`                                      // 功率（瓦）
	PricePerKwh int `json:"price_per_kwh"`                              // 电价（分/度）
	ServiceFee  int `json:"service_fee"`                                // 服务费率（千分比）
}

// StartCharge 启动充电
// @Summary 启动充电
// @Description 第三方平台调用此接口启动设备充电
// @Tags 第三方API - 充电控制
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "设备物理ID"
// @Param request body StartChargeRequest true "充电参数"
// @Success 200 {object} StandardResponse "成功"
// @Failure 400 {object} StandardResponse "参数错误"
// @Failure 500 {object} StandardResponse "服务器错误"
// @Router /api/v1/third/devices/{id}/charge [post]
func (h *ThirdPartyHandler) StartCharge(c *gin.Context) {
	ctx := c.Request.Context()
	devicePhyID := c.Param("id")
	requestID := c.GetString("request_id")

	// 解析请求体
	var req StartChargeRequest
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
			Code:      500,
			Message:   "failed to get device",
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// 2. 检查设备在线状态（可选）
	isOnline := h.sess.IsOnline(devicePhyID, time.Now())
	if !isOnline {
		h.logger.Warn("device offline", zap.String("device_phy_id", devicePhyID))
		// 注意：这里不阻止充电指令下发，因为设备可能稍后上线
	}

	// 3. 生成订单号
	orderNo := fmt.Sprintf("THD%d%03d", time.Now().Unix(), req.PortNo)

	// 4. 创建订单记录（简化版本，实际应使用CardService）
	// 这里直接使用SQL插入订单
	insertOrderSQL := `
		INSERT INTO orders (device_id, order_no, card_no, amount, status, port_no, created_at)
		VALUES ($1, $2, 'THIRD_PARTY_API', $3, 'pending', $4, NOW())
	`
	_, err = h.repo.Pool.Exec(ctx, insertOrderSQL, devID, orderNo, req.Amount, req.PortNo)
	if err != nil {
		h.logger.Error("failed to create order", zap.Error(err))
		c.JSON(http.StatusInternalServerError, StandardResponse{
			Code:      500,
			Message:   "failed to create order",
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// 5. 构造并下发充电指令（BKV 0x0B下行）
	// 这里需要构造BKV充电指令的payload
	if h.outboundQ != nil {
		// 构造简化的充电指令消息
		msgID := uint32(time.Now().Unix() % 65536)
		cmdData := h.encodeChargeCommand(orderNo, uint8(req.ChargeMode), uint32(req.Amount), uint32(req.Duration), uint16(req.Power), uint32(req.PricePerKwh), uint16(req.ServiceFee))

		err = h.outboundQ.Enqueue(ctx, &redisstorage.OutboundMessage{
			ID:        fmt.Sprintf("api_%d", msgID),
			DeviceID:  devID,
			PhyID:     devicePhyID,
			Command:   cmdData,
			Priority:  5,
			MaxRetry:  3,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Timeout:   5000,
		})
		if err != nil {
			h.logger.Error("failed to push charge command", zap.Error(err))
			// 不返回错误，订单已创建，可稍后重试
		} else {
			h.logger.Info("charge command pushed", zap.String("order_no", orderNo))
		}
	}

	// 6. 返回成功响应
	c.JSON(http.StatusOK, StandardResponse{
		Code:    0,
		Message: "charge command sent successfully",
		Data: map[string]interface{}{
			"device_id": devicePhyID,
			"order_no":  orderNo,
			"port_no":   req.PortNo,
			"amount":    req.Amount,
			"online":    isOnline,
		},
		RequestID: requestID,
		Timestamp: time.Now().Unix(),
	})
}

// StopChargeRequest 停止充电请求
type StopChargeRequest struct {
	PortNo int `json:"port_no" binding:"required,min=1"` // 端口号
}

// StopCharge 停止充电
// @Summary 停止充电
// @Description 第三方平台调用此接口停止设备充电
// @Tags 第三方API - 充电控制
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "设备物理ID"
// @Param request body StopChargeRequest true "停止充电参数"
// @Success 200 {object} StandardResponse "成功"
// @Failure 404 {object} StandardResponse "订单不存在"
// @Failure 500 {object} StandardResponse "服务器错误"
// @Router /api/v1/third/devices/{id}/stop [post]
func (h *ThirdPartyHandler) StopCharge(c *gin.Context) {
	ctx := c.Request.Context()
	devicePhyID := c.Param("id")
	requestID := c.GetString("request_id")

	// 解析请求体
	var req StopChargeRequest
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

	h.logger.Info("stop charge requested",
		zap.String("device_phy_id", devicePhyID),
		zap.Int("port_no", req.PortNo))

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

	// 2. 查询当前活动的订单
	var orderNo string
	queryOrderSQL := `
		SELECT order_no FROM orders 
		WHERE device_id = $1 AND port_no = $2 AND status IN ('pending', 'charging')
		ORDER BY created_at DESC LIMIT 1
	`
	err = h.repo.Pool.QueryRow(ctx, queryOrderSQL, devID, req.PortNo).Scan(&orderNo)
	if err != nil {
		h.logger.Warn("no active order found", zap.Error(err))
		c.JSON(http.StatusNotFound, StandardResponse{
			Code:      404,
			Message:   "no active charging session found",
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// 3. 下发停止充电指令（BKV 0x0015控制设备）
	if h.outboundQ != nil {
		msgID := uint32(time.Now().Unix() % 65536)
		// 构造停止充电指令（简化版本）
		stopData := []byte{byte(req.PortNo), 0x00} // 端口号 + 停止命令

		err = h.outboundQ.Enqueue(ctx, &redisstorage.OutboundMessage{
			ID:        fmt.Sprintf("api_%d", msgID),
			DeviceID:  devID,
			PhyID:     devicePhyID,
			Command:   stopData,
			Priority:  8, // 停止命令优先级高
			MaxRetry:  3,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Timeout:   5000,
		})
		if err != nil {
			h.logger.Error("failed to push stop command", zap.Error(err))
		} else {
			h.logger.Info("stop command pushed", zap.String("order_no", orderNo))
		}
	}

	// 4. 更新订单状态为停止中
	updateOrderSQL := `UPDATE orders SET status = 'stopping', updated_at = NOW() WHERE order_no = $1`
	_, err = h.repo.Pool.Exec(ctx, updateOrderSQL, orderNo)
	if err != nil {
		h.logger.Error("failed to update order status", zap.Error(err))
	}

	// 5. 返回成功响应
	c.JSON(http.StatusOK, StandardResponse{
		Code:    0,
		Message: "stop command sent successfully",
		Data: map[string]interface{}{
			"device_id": devicePhyID,
			"order_no":  orderNo,
			"port_no":   req.PortNo,
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
// @Param id path string true "设备物理ID"
// @Success 200 {object} StandardResponse "成功"
// @Failure 404 {object} StandardResponse "设备不存在"
// @Failure 500 {object} StandardResponse "服务器错误"
// @Router /api/v1/third/devices/{id} [get]
func (h *ThirdPartyHandler) GetDevice(c *gin.Context) {
	ctx := c.Request.Context()
	devicePhyID := c.Param("id")
	requestID := c.GetString("request_id")

	h.logger.Info("get device requested", zap.String("device_phy_id", devicePhyID))

	// 1. 从数据库获取设备信息
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

	// 2. 查询设备基本信息
	var lastSeenAt time.Time
	var createdAt time.Time
	queryDeviceSQL := `SELECT created_at, last_seen_at FROM devices WHERE id = $1`
	err = h.repo.Pool.QueryRow(ctx, queryDeviceSQL, devID).Scan(&createdAt, &lastSeenAt)
	if err != nil {
		h.logger.Error("failed to query device", zap.Error(err))
		c.JSON(http.StatusNotFound, StandardResponse{
			Code:      404,
			Message:   "device not found",
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// 3. 检查设备在线状态
	isOnline := h.sess.IsOnline(devicePhyID, time.Now())

	// 4. 查询当前活动订单（如果有）
	var activeOrderNo *string
	var activePortNo *int
	queryActiveOrderSQL := `
		SELECT order_no, port_no FROM orders 
		WHERE device_id = $1 AND status IN ('pending', 'charging')
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

	c.JSON(http.StatusOK, StandardResponse{
		Code:      0,
		Message:   "success",
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
// @Param id path string true "订单号"
// @Success 200 {object} StandardResponse "成功"
// @Failure 404 {object} StandardResponse "订单不存在"
// @Failure 500 {object} StandardResponse "服务器错误"
// @Router /api/v1/third/orders/{id} [get]
func (h *ThirdPartyHandler) GetOrder(c *gin.Context) {
	ctx := c.Request.Context()
	orderNo := c.Param("id")
	requestID := c.GetString("request_id")

	h.logger.Info("get order requested", zap.String("order_no", orderNo))

	// 查询订单详情
	var deviceID int64
	var cardNo string
	var amount int
	var status string
	var portNo *int
	var duration *int
	var energy *int
	var createdAt time.Time
	var updatedAt time.Time

	querySQL := `
		SELECT device_id, card_no, amount, status, port_no, duration, energy, created_at, updated_at
		FROM orders 
		WHERE order_no = $1
	`
	err := h.repo.Pool.QueryRow(ctx, querySQL, orderNo).Scan(
		&deviceID, &cardNo, &amount, &status, &portNo, &duration, &energy, &createdAt, &updatedAt)
	if err != nil {
		h.logger.Warn("order not found", zap.String("order_no", orderNo), zap.Error(err))
		c.JSON(http.StatusNotFound, StandardResponse{
			Code:      404,
			Message:   "order not found",
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// 构造响应数据
	orderData := map[string]interface{}{
		"order_no":   orderNo,
		"device_id":  deviceID,
		"card_no":    cardNo,
		"amount":     float64(amount) / 100.0, // 转换为元
		"status":     status,
		"created_at": createdAt.Unix(),
		"updated_at": updatedAt.Unix(),
	}

	if portNo != nil {
		orderData["port_no"] = *portNo
	}
	if duration != nil {
		orderData["duration"] = *duration
	}
	if energy != nil {
		orderData["energy_kwh"] = float64(*energy) / 100.0 // 转换为kWh
	}

	c.JSON(http.StatusOK, StandardResponse{
		Code:      0,
		Message:   "success",
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
	status := c.Query("status")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	h.logger.Info("list orders requested",
		zap.String("device_id", devicePhyID),
		zap.String("status", status),
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

	if status != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, status)
		argIdx++
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
		SELECT order_no, device_id, card_no, amount, status, port_no, duration, energy, created_at, updated_at
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
		var cardNo string
		var amount int
		var status string
		var portNo *int
		var duration *int
		var energy *int
		var createdAt time.Time
		var updatedAt time.Time

		err := rows.Scan(&orderNo, &deviceID, &cardNo, &amount, &status, &portNo, &duration, &energy, &createdAt, &updatedAt)
		if err != nil {
			h.logger.Error("failed to scan order", zap.Error(err))
			continue
		}

		orderData := map[string]interface{}{
			"order_no":   orderNo,
			"device_id":  deviceID,
			"card_no":    cardNo,
			"amount":     float64(amount) / 100.0,
			"status":     status,
			"created_at": createdAt.Unix(),
			"updated_at": updatedAt.Unix(),
		}

		if portNo != nil {
			orderData["port_no"] = *portNo
		}
		if duration != nil {
			orderData["duration"] = *duration
		}
		if energy != nil {
			orderData["energy_kwh"] = float64(*energy) / 100.0
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
// @Param id path string true "设备物理ID"
// @Param request body SetParamsRequest true "参数列表"
// @Success 200 {object} StandardResponse "成功"
// @Failure 400 {object} StandardResponse "参数错误"
// @Failure 500 {object} StandardResponse "服务器错误"
// @Router /api/v1/third/devices/{id}/params [post]
func (h *ThirdPartyHandler) SetParams(c *gin.Context) {
	ctx := c.Request.Context()
	devicePhyID := c.Param("id")
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
			Priority:  6,
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
// @Param id path string true "设备物理ID"
// @Param request body TriggerOTARequest true "OTA升级参数"
// @Success 200 {object} StandardResponse "成功"
// @Failure 400 {object} StandardResponse "参数错误"
// @Failure 500 {object} StandardResponse "服务器错误"
// @Router /api/v1/third/devices/{id}/ota [post]
func (h *ThirdPartyHandler) TriggerOTA(c *gin.Context) {
	ctx := c.Request.Context()
	devicePhyID := c.Param("id")
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
			Priority:  7, // OTA命令优先级较高
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
