package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
)

// Week 7: OTA升级API路由

// RegisterOTARoutes 注册OTA升级路由
func RegisterOTARoutes(r *gin.Engine, repo *pgstorage.Repository) {
	ota := r.Group("/api/devices/:device_id/ota")
	{
		ota.POST("", createOTATask(repo))      // 创建OTA任务
		ota.GET("/:task_id", getOTATask(repo)) // 查询OTA任务
		ota.GET("", listOTATasks(repo))        // 查询设备OTA任务列表
	}
}

// CreateOTATaskRequest OTA任务创建请求
type CreateOTATaskRequest struct {
	TargetType      int    `json:"target_type" binding:"required,min=1,max=2"` // 1=DTU, 2=插座
	TargetSocketNo  *int   `json:"target_socket_no"`                           // 插座编号
	FirmwareVersion string `json:"firmware_version" binding:"required"`
	FTPServer       string `json:"ftp_server" binding:"required"`
	FTPPort         int    `json:"ftp_port" binding:"required"`
	FileName        string `json:"file_name" binding:"required"`
	FileSize        *int64 `json:"file_size"`
}

// createOTATask 创建OTA任务
func createOTATask(repo *pgstorage.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		deviceIDStr := c.Param("device_id")
		deviceID, err := strconv.ParseInt(deviceIDStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid device_id"})
			return
		}

		var req CreateOTATaskRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "detail": err.Error()})
			return
		}

		// 验证参数
		if req.TargetType == 2 && req.TargetSocketNo == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "target_socket_no is required for socket upgrade"})
			return
		}

		// 创建任务
		task := &pgstorage.OTATask{
			DeviceID:        deviceID,
			TargetType:      req.TargetType,
			TargetSocketNo:  req.TargetSocketNo,
			FirmwareVersion: req.FirmwareVersion,
			FTPServer:       req.FTPServer,
			FTPPort:         req.FTPPort,
			FileName:        req.FileName,
			FileSize:        req.FileSize,
			Status:          0, // 待发送
		}

		taskID, err := repo.CreateOTATask(c.Request.Context(), task)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create OTA task", "detail": err.Error()})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"message": "OTA task created successfully",
			"task_id": taskID,
		})
	}
}

// getOTATask 查询OTA任务
func getOTATask(repo *pgstorage.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		taskIDStr := c.Param("task_id")
		taskID, err := strconv.ParseInt(taskIDStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task_id"})
			return
		}

		task, err := repo.GetOTATask(c.Request.Context(), taskID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "OTA task not found"})
			return
		}

		// 添加状态描述
		statusDesc := getOTAStatusDescription(task.Status)

		c.JSON(http.StatusOK, gin.H{
			"task":        task,
			"status_desc": statusDesc,
		})
	}
}

// listOTATasks 查询设备OTA任务列表
func listOTATasks(repo *pgstorage.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		deviceIDStr := c.Param("device_id")
		deviceID, err := strconv.ParseInt(deviceIDStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid device_id"})
			return
		}

		// 查询参数
		limitStr := c.DefaultQuery("limit", "10")
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit <= 0 {
			limit = 10
		}
		if limit > 100 {
			limit = 100
		}

		tasks, err := repo.GetDeviceOTATasks(c.Request.Context(), deviceID, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query OTA tasks", "detail": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"device_id": deviceID,
			"count":     len(tasks),
			"tasks":     tasks,
		})
	}
}

// getOTAStatusDescription 获取OTA状态描述
func getOTAStatusDescription(status int) string {
	switch status {
	case 0:
		return "待发送"
	case 1:
		return "已下发"
	case 2:
		return "升级中"
	case 3:
		return "成功"
	case 4:
		return "失败"
	default:
		return "未知"
	}
}
