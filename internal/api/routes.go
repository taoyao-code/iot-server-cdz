package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/taoyao-code/iot-server/internal/api/middleware"
	"github.com/taoyao-code/iot-server/internal/session"
	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
	"go.uber.org/zap"
)

// RegisterReadOnlyRoutes 注册只读查询路由
// P0修复: 添加API认证保护
// P0完成: 支持接口类型以兼容内存和Redis会话管理器
func RegisterReadOnlyRoutes(
	r *gin.Engine,
	repo *pgstorage.Repository,
	sess session.SessionManager,
	policy session.WeightedPolicy,
	authCfg middleware.AuthConfig,
	logger *zap.Logger,
) {
	if r == nil || repo == nil || sess == nil {
		return
	}

	// 注意：健康检查路由(/health, /health/ready, /health/live)
	// 已由 health.RegisterHTTPRoutes 统一注册，此处不再重复

	// 保留简化版 /ready 路由用于快速检查
	r.GET("/ready", func(c *gin.Context) {
		// 检查数据库
		ctx := c.Request.Context()
		if err := repo.Pool.Ping(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "not ready",
				"reason": "database not available",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":         "ready",
			"online_devices": sess.OnlineCount(time.Now()),
			"time":           time.Now().Format(time.RFC3339),
		})
	})

	// API路由组（需要认证）
	api := r.Group("/api")
	if authCfg.Enabled {
		api.Use(middleware.APIKeyAuth(authCfg, logger))
		logger.Info("api authentication enabled", zap.Int("api_keys_count", len(authCfg.APIKeys)))
	} else {
		logger.Warn("api authentication disabled - only for development!")
	}

	// 设备相关
	api.GET("/devices", func(c *gin.Context) {
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
		list, err := repo.ListDevices(c.Request.Context(), limit, offset)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"devices": list})
	})

	api.GET("/devices/:phyId/ports", func(c *gin.Context) {
		phy := c.Param("phyId")
		ports, err := repo.ListPortsByPhyID(c.Request.Context(), phy)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"phyId": phy, "ports": ports})
	})

	// P0修复: 新增参数查询接口
	api.GET("/devices/:phyId/params", func(c *gin.Context) {
		phy := c.Param("phyId")

		// 获取设备ID
		device, err := repo.GetDeviceByPhyID(c.Request.Context(), phy)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
			return
		}

		// 查询参数
		params, err := repo.ListDeviceParams(c.Request.Context(), device.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "query failed"})
			return
		}

		// 转换为API响应格式
		type paramResponse struct {
			ParamID     int        `json:"param_id"`
			Value       string     `json:"value"` // 简化处理
			MsgID       int        `json:"msg_id"`
			Status      string     `json:"status"` // pending/confirmed/failed
			CreatedAt   time.Time  `json:"created_at"`
			ConfirmedAt *time.Time `json:"confirmed_at,omitempty"`
			ErrorMsg    *string    `json:"error_msg,omitempty"`
		}

		var response []paramResponse
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
	})

	// 会话相关
	api.GET("/sessions/:phyId", func(c *gin.Context) {
		phy := c.Param("phyId")
		online := sess.IsOnlineWeighted(phy, time.Now(), policy)
		var lastSeen *time.Time
		if d, e := repo.GetDeviceByPhyID(c.Request.Context(), phy); e == nil {
			lastSeen = d.LastSeenAt
		}
		c.JSON(200, gin.H{"phyId": phy, "online": online, "lastSeenAt": lastSeen})
	})

	// 订单相关
	api.GET("/orders/:id", func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			c.JSON(400, gin.H{"error": "invalid id"})
			return
		}
		ord, err := repo.GetOrderByID(c.Request.Context(), id)
		if err != nil {
			c.JSON(404, gin.H{"error": "not found"})
			return
		}
		c.JSON(200, gin.H{"order": ord})
	})

	api.GET("/devices/:phyId/orders", func(c *gin.Context) {
		phy := c.Param("phyId")
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
		list, err := repo.ListOrdersByPhyID(c.Request.Context(), phy, limit, offset)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"orders": list})
	})
}
