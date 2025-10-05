package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
)

// Week 6: 组网管理API路由

// RegisterNetworkRoutes 注册组网管理路由
func RegisterNetworkRoutes(r *gin.Engine, repo *pgstorage.Repository) {
	network := r.Group("/api/gateway/:gateway_id/sockets")
	{
		network.GET("", listGatewaySockets(repo))                // 查询网关所有插座
		network.GET("/:socket_no", getGatewaySocket(repo))       // 查询单个插座
		network.DELETE("/:socket_no", deleteGatewaySocket(repo)) // 删除插座
	}
}

// listGatewaySockets 查询网关所有插座
func listGatewaySockets(repo *pgstorage.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		gatewayID := c.Param("gateway_id")
		if gatewayID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "gateway_id is required"})
			return
		}

		sockets, err := repo.GetGatewaySockets(c.Request.Context(), gatewayID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query sockets", "detail": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"gateway_id": gatewayID,
			"count":      len(sockets),
			"sockets":    sockets,
		})
	}
}

// getGatewaySocket 查询单个插座
func getGatewaySocket(repo *pgstorage.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		gatewayID := c.Param("gateway_id")
		socketNoStr := c.Param("socket_no")

		socketNo, err := strconv.Atoi(socketNoStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid socket_no"})
			return
		}

		socket, err := repo.GetGatewaySocket(c.Request.Context(), gatewayID, socketNo)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "socket not found"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"gateway_id": gatewayID,
			"socket":     socket,
		})
	}
}

// deleteGatewaySocket 删除插座
func deleteGatewaySocket(repo *pgstorage.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		gatewayID := c.Param("gateway_id")
		socketNoStr := c.Param("socket_no")

		socketNo, err := strconv.Atoi(socketNoStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid socket_no"})
			return
		}

		// 验证插座是否存在
		_, err = repo.GetGatewaySocket(c.Request.Context(), gatewayID, socketNo)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "socket not found"})
			return
		}

		// 删除插座
		if err := repo.DeleteGatewaySocket(c.Request.Context(), gatewayID, socketNo); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete socket", "detail": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message":    "socket deleted successfully",
			"gateway_id": gatewayID,
			"socket_no":  socketNo,
		})
	}
}

// RefreshSocketsRequest 刷新插座列表请求
type RefreshSocketsRequest struct {
	// 可选：强制刷新标志
	Force bool `json:"force"`
}

// AddSocketRequest 添加插座请求
type AddSocketRequest struct {
	SocketNo  int    `json:"socket_no" binding:"required,min=1,max=250"`
	SocketMAC string `json:"socket_mac" binding:"required"`
	Channel   int    `json:"channel" binding:"required,min=1,max=15"`
}
