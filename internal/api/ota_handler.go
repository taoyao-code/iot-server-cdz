package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
	"go.uber.org/zap"
)

// OTAHandler OTA升级API处理器
type OTAHandler struct {
	repo   *pgstorage.Repository
	logger *zap.Logger
}

// NewOTAHandler 创建OTA Handler
func NewOTAHandler(repo *pgstorage.Repository, logger *zap.Logger) *OTAHandler {
	return &OTAHandler{repo: repo, logger: logger}
}

// CreateOTATaskRequest OTA任务创建请求
type CreateOTATaskRequest struct {
	TargetType      int    `json:"target_type" binding:"required,min=1,max=2"`
	TargetSocketNo  *int   `json:"target_socket_no"`
	FirmwareVersion string `json:"firmware_version" binding:"required"`
	FTPServer       string `json:"ftp_server" binding:"required"`
	FTPPort         int    `json:"ftp_port" binding:"required"`
	FileName        string `json:"file_name" binding:"required"`
	FileSize        *int64 `json:"file_size"`
}

// CreateOTATask 创建OTA任务
// @Summary 创建OTA升级任务
// @Tags OTA管理
// @Accept json
// @Produce json
// @Param device_id path int true "设备ID"
// @Param request body CreateOTATaskRequest true "OTA任务参数"
// @Success 201 {object} map[string]interface{}
// @Router /api/devices/{device_id}/ota [post]
func (h *OTAHandler) CreateOTATask(c *gin.Context) {
	deviceIDStr := c.Param("device_id")
	deviceID, err := strconv.ParseInt(deviceIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid device_id"})
		return
	}

	var req CreateOTATaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if req.TargetType == 2 && req.TargetSocketNo == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target_socket_no required"})
		return
	}

	task := &pgstorage.OTATask{
		DeviceID:        deviceID,
		TargetType:      req.TargetType,
		TargetSocketNo:  req.TargetSocketNo,
		FirmwareVersion: req.FirmwareVersion,
		FTPServer:       req.FTPServer,
		FTPPort:         req.FTPPort,
		FileName:        req.FileName,
		FileSize:        req.FileSize,
		Status:          0,
	}

	taskID, err := h.repo.CreateOTATask(c.Request.Context(), task)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "success", "task_id": taskID})
}

// GetOTATask 查询OTA任务
// @Summary 查询OTA任务详情
// @Tags OTA管理
// @Param task_id path int true "任务ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/devices/{device_id}/ota/{task_id} [get]
func (h *OTAHandler) GetOTATask(c *gin.Context) {
	taskIDStr := c.Param("task_id")
	taskID, err := strconv.ParseInt(taskIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task_id"})
		return
	}

	task, err := h.repo.GetOTATask(c.Request.Context(), taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"task": task})
}

// ListOTATasks 查询OTA任务列表
// @Summary 查询设备OTA任务列表
// @Tags OTA管理
// @Param device_id path int true "设备ID"
// @Param limit query int false "数量限制"
// @Success 200 {object} map[string]interface{}
// @Router /api/devices/{device_id}/ota [get]
func (h *OTAHandler) ListOTATasks(c *gin.Context) {
	deviceIDStr := c.Param("device_id")
	deviceID, err := strconv.ParseInt(deviceIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid device_id"})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	tasks, err := h.repo.GetDeviceOTATasks(c.Request.Context(), deviceID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"device_id": deviceID, "tasks": tasks})
}
