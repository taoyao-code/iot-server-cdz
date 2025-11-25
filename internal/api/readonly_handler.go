package api

import (
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

	type ord struct {
		status int
		port   int
	}

	// 端口是否有任何处于充电状态
	portCharging := false
	for _, p := range ports {
		if isBKVChargingStatus(p.Status) {
			portCharging = true
			break
		}
	}

	c.JSON(200, gin.H{"active_orders": portCharging})
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
func (h *ReadOnlyHandler) GetOrder(c *gin.Context) {
}

// Ready 快速就绪检查
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
