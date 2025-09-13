package api

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/taoyao-code/iot-server/internal/session"
	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
)

// RegisterReadOnlyRoutes registers read-only management APIs on the provided router.
func RegisterReadOnlyRoutes(r *gin.Engine, repo *pgstorage.Repository, sess *session.Manager, policy session.WeightedPolicy) {
	if r == nil || repo == nil || sess == nil {
		return
	}

	r.GET("/api/devices", func(c *gin.Context) {
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

	r.GET("/api/devices/:phyId/ports", func(c *gin.Context) {
		phy := c.Param("phyId")
		ports, err := repo.ListPortsByPhyID(c.Request.Context(), phy)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"phyId": phy, "ports": ports})
	})

	r.GET("/api/sessions/:phyId", func(c *gin.Context) {
		phy := c.Param("phyId")
		online := sess.IsOnlineWeighted(phy, time.Now(), policy)
		var lastSeen *time.Time
		if d, e := repo.GetDeviceByPhyID(c.Request.Context(), phy); e == nil {
			lastSeen = d.LastSeenAt
		}
		c.JSON(200, gin.H{"phyId": phy, "online": online, "lastSeenAt": lastSeen})
	})

	r.GET("/api/orders/:id", func(c *gin.Context) {
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

	r.GET("/api/devices/:phyId/orders", func(c *gin.Context) {
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
