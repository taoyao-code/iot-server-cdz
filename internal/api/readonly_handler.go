package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/taoyao-code/iot-server/internal/session"
	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
	"go.uber.org/zap"
)

// ReadOnlyHandler 只读API处理器
type ReadOnlyHandler struct {
	repo   *pgstorage.Repository
	sess   session.SessionManager
	policy session.WeightedPolicy
	logger *zap.Logger
}

// NewReadOnlyHandler 创建只读API处理器
func NewReadOnlyHandler(
	repo *pgstorage.Repository,
	sess session.SessionManager,
	policy session.WeightedPolicy,
	logger *zap.Logger,
) *ReadOnlyHandler {
	return &ReadOnlyHandler{
		repo:   repo,
		sess:   sess,
		policy: policy,
		logger: logger,
	}
}

// ListDevices 查询设备列表
// @Summary 查询设备列表
// @Description 分页查询所有设备
// @Tags 内部API - 设备管理
// @Produce json
// @Security ApiKeyAuth
// @Param limit query int false "每页数量(默认100)"
// @Param offset query int false "偏移量(默认0)"
// @Success 200 {object} map[string]interface{} "成功"
// @Router /api/devices [get]
func (h *ReadOnlyHandler) ListDevices(c *gin.Context) {
	limit := 100
	offset := 0
	if v := c.Query("limit"); v != "" {
		if vv, e := strconv.Atoi(v); e == nil {
			limit = vv
		}
	}
	if v := c.Query("offset"); v != "" {
		if vv, e := strconv.Atoi(v); e == nil {
			offset = vv
		}
	}

	list, err := h.repo.ListDevices(c.Request.Context(), limit, offset)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"devices": list})
}

// ListDevicePorts 查询设备端口
// @Summary 查询设备端口状态
// @Description 根据设备物理ID查询所有端口状态
// @Tags 内部API - 设备管理
// @Produce json
// @Security ApiKeyAuth
// @Param device_id path string true "设备物理ID"
// @Success 200 {object} map[string]interface{} "成功"
// @Router /api/devices/{device_id}/ports [get]
func (h *ReadOnlyHandler) ListDevicePorts(c *gin.Context) {
	phy := c.Param("device_id")
	ctx := c.Request.Context()

	ports, err := h.repo.ListPortsByPhyID(ctx, phy)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	// 一致性视图：基于 Session + orders + ports 标记是否存在明显不一致
	now := time.Now()
	online := h.sess.IsOnline(phy, now)

	// 查询活跃订单（仅pending/confirmed/charging）
	const activeOrderSQL = `
		SELECT o.order_no, o.status, o.port_no
		FROM orders o
		JOIN devices d ON o.device_id = d.id
		WHERE d.phy_id = $1 AND o.status IN (0,1,2)
	`
	rows, err := h.repo.Pool.Query(ctx, activeOrderSQL, phy)
	if err != nil {
		// 回退为简单视图
		c.JSON(200, gin.H{"phyId": phy, "ports": ports})
		return
	}
	defer rows.Close()

	type ord struct {
		status int
		port   int
	}
	var activeOrders []ord
	for rows.Next() {
		var (
			orderNo string
			status  int
			portNo  int
		)
		if err := rows.Scan(&orderNo, &status, &portNo); err != nil {
			continue
		}
		activeOrders = append(activeOrders, ord{status: status, port: portNo})
	}

	// 端口是否有任何处于充电状态
	portCharging := false
	for _, p := range ports {
		if isBKVChargingStatus(p.Status) {
			portCharging = true
			break
		}
	}
	hasActiveOrder := len(activeOrders) > 0

	consistencyStatus := "ok"
	inconsistencyReason := ""

	if !online && hasActiveOrder {
		consistencyStatus = "inconsistent"
		inconsistencyReason = "device_offline_but_active_order"
	} else if online && portCharging && !hasActiveOrder {
		consistencyStatus = "inconsistent"
		inconsistencyReason = "port_charging_without_active_order"
	} else if online && hasActiveOrder && !portCharging {
		consistencyStatus = "inconsistent"
		inconsistencyReason = "active_order_but_ports_not_charging"
	}

	resp := gin.H{
		"phy_id":             phy,
		"ports":              ports,
		"online":             online,
		"consistency_status": consistencyStatus,
	}
	if inconsistencyReason != "" {
		resp["inconsistency_reason"] = inconsistencyReason
	}

	c.JSON(200, resp)
}

// GetDeviceParams 查询设备参数
// @Summary 查询设备参数
// @Description 查询设备的所有参数设置记录
// @Tags 内部API - 参数管理
// @Produce json
// @Security ApiKeyAuth
// @Param device_id path string true "设备物理ID"
// @Success 200 {object} map[string]interface{} "成功"
// @Router /api/devices/{device_id}/params [get]
func (h *ReadOnlyHandler) GetDeviceParams(c *gin.Context) {
	phy := c.Param("device_id")

	// 获取设备ID
	device, err := h.repo.GetDeviceByPhyID(c.Request.Context(), phy)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}

	// 查询参数
	params, err := h.repo.ListDeviceParams(c.Request.Context(), device.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query failed"})
		return
	}

	// 转换为API响应格式
	type paramResponse struct {
		ParamID     int        `json:"param_id"`
		Value       string     `json:"value"`
		MsgID       int        `json:"msg_id"`
		Status      string     `json:"status"`
		CreatedAt   time.Time  `json:"created_at"`
		ConfirmedAt *time.Time `json:"confirmed_at,omitempty"`
		ErrorMsg    *string    `json:"error_msg,omitempty"`
	}

	// 初始化为空数组，避免JSON序列化为null
	response := []paramResponse{}
	for _, p := range params {
		status := "pending"
		switch p.Status {
		case 1:
			status = "confirmed"
		case 2:
			status = "failed"
		}

		response = append(response, paramResponse{
			ParamID:     p.ParamID,
			Value:       string(p.ParamValue),
			MsgID:       p.MsgID,
			Status:      status,
			CreatedAt:   p.CreatedAt,
			ConfirmedAt: p.ConfirmedAt,
			ErrorMsg:    p.ErrorMsg,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"phy_id":    phy,
		"device_id": device.ID,
		"params":    response,
	})
}

// GetSessionStatus 查询会话状态
// @Summary 查询设备会话状态
// @Description 查询设备在线状态和最后活跃时间
// @Tags 内部API - 会话管理
// @Produce json
// @Security ApiKeyAuth
// @Param device_id path string true "设备物理ID"
// @Success 200 {object} map[string]interface{} "成功"
// @Router /api/sessions/{device_id} [get]
func (h *ReadOnlyHandler) GetSessionStatus(c *gin.Context) {
	phy := c.Param("device_id")
	online := h.sess.IsOnlineWeighted(phy, time.Now(), h.policy)
	var lastSeen *time.Time
	if d, e := h.repo.GetDeviceByPhyID(c.Request.Context(), phy); e == nil {
		lastSeen = d.LastSeenAt
	}
	c.JSON(200, gin.H{"phyId": phy, "online": online, "lastSeenAt": lastSeen})
}

// GetOrder 查询订单
// @Summary 查询订单详情
// @Description 根据订单ID查询订单详情
// @Tags 内部API - 订单管理
// @Produce json
// @Security ApiKeyAuth
// @Param order_id path int true "订单ID"
// @Success 200 {object} map[string]interface{} "成功"
// @Failure 404 {object} map[string]interface{} "订单不存在"
// @Router /api/orders/{order_id} [get]
func (h *ReadOnlyHandler) GetOrder(c *gin.Context) {
	idStr := c.Param("order_id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}
	ord, err := h.repo.GetOrderByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}

	// 一致性视图：评估订单与端口/会话的一致性
	consistencyStatus, inconsistencyReason := h.evaluateOrderConsistency(c.Request.Context(), ord.DeviceID, ord.PortNo, ord.Status)

	resp := gin.H{"order": ord}
	if consistencyStatus != "" {
		resp["consistency_status"] = consistencyStatus
	}
	if inconsistencyReason != "" {
		resp["inconsistency_reason"] = inconsistencyReason
	}

	c.JSON(200, resp)
}

// ListDeviceOrders 查询设备订单
// @Summary 查询设备订单列表
// @Description 根据设备物理ID查询订单列表
// @Tags 内部API - 订单管理
// @Produce json
// @Security ApiKeyAuth
// @Param device_id path string true "设备物理ID"
// @Param limit query int false "每页数量(默认100)"
// @Param offset query int false "偏移量(默认0)"
// @Success 200 {object} map[string]interface{} "成功"
// @Router /api/devices/{device_id}/orders [get]
func (h *ReadOnlyHandler) ListDeviceOrders(c *gin.Context) {
	phy := c.Param("device_id")
	limit := 100
	offset := 0
	if v := c.Query("limit"); v != "" {
		if vv, e := strconv.Atoi(v); e == nil {
			limit = vv
		}
	}
	if v := c.Query("offset"); v != "" {
		if vv, e := strconv.Atoi(v); e == nil {
			offset = vv
		}
	}
	list, err := h.repo.ListOrdersByPhyID(c.Request.Context(), phy, limit, offset)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	// 为每个订单附加一致性状态（轻量级，只使用 DB + Session）
	ctx := c.Request.Context()
	enriched := make([]map[string]interface{}, 0, len(list))
	for _, ord := range list {
		status, reason := h.evaluateOrderConsistency(ctx, ord.DeviceID, ord.PortNo, ord.Status)
		item := map[string]interface{}{
			"id":                 ord.ID,
			"device_id":          ord.DeviceID,
			"phy_id":             ord.PhyID,
			"port_no":            ord.PortNo,
			"order_no":           ord.OrderNo,
			"status":             ord.Status,
			"start_time":         ord.StartTime,
			"end_time":           ord.EndTime,
			"kwh_0p01":           ord.Kwh01,
			"amount_cent":        ord.AmountCent,
			"test_session":       ord.TestSessionID,
			"consistency_status": status,
		}
		if reason != "" {
			item["inconsistency_reason"] = reason
		}
		enriched = append(enriched, item)
	}

	c.JSON(200, gin.H{"orders": enriched})
}

// Ready 快速就绪检查
// @Summary 快速就绪检查
// @Description 检查服务是否就绪
// @Tags 系统
// @Produce json
// @Success 200 {object} map[string]interface{} "就绪"
// @Failure 503 {object} map[string]interface{} "未就绪"
// @Router /ready [get]
func (h *ReadOnlyHandler) Ready(c *gin.Context) {
	// 检查数据库
	ctx := c.Request.Context()
	if err := h.repo.Pool.Ping(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "not ready",
			"reason": "database not available",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":         "ready",
		"online_devices": h.sess.OnlineCount(time.Now()),
		"time":           time.Now().Format(time.RFC3339),
	})
}

// evaluateOrderConsistency 复用第三方读取中的一致性规则，对只读视图提供相同的一致性标记
func (h *ReadOnlyHandler) evaluateOrderConsistency(ctx context.Context, deviceID int64, portNo int, status int) (string, string) {
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
