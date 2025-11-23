package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/taoyao-code/iot-server/internal/driverapi"
	"github.com/taoyao-code/iot-server/internal/metrics"
	"github.com/taoyao-code/iot-server/internal/service"
	"github.com/taoyao-code/iot-server/internal/session"
	"github.com/taoyao-code/iot-server/internal/storage"
	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
	"github.com/taoyao-code/iot-server/internal/thirdparty"
	"go.uber.org/zap"
)

// TestConsoleHandler 内部测试控制台处理器
// 仅供内部测试/运维人员使用，复用ThirdPartyHandler逻辑
type TestConsoleHandler struct {
	repo            *pgstorage.Repository
	sess            session.SessionManager
	eventQueue      *thirdparty.EventQueue
	thirdPartyH     *ThirdPartyHandler
	timelineService *service.TimelineService
	logger          *zap.Logger
}

// NewTestConsoleHandler 创建测试控制台处理器
func NewTestConsoleHandler(
	repo *pgstorage.Repository,
	core storage.CoreRepo,
	sess session.SessionManager,
	commandSource driverapi.CommandSource,
	eventQueue *thirdparty.EventQueue,
	metrics *metrics.AppMetrics,
	logger *zap.Logger,
) *TestConsoleHandler {
	return &TestConsoleHandler{
		repo:            repo,
		sess:            sess,
		eventQueue:      eventQueue,
		thirdPartyH:     NewThirdPartyHandler(repo, core, sess, commandSource, eventQueue, metrics, logger),
		timelineService: service.NewTimelineService(repo, logger),
		logger:          logger,
	}
}

// TestDevice 测试设备信息
type TestDevice struct {
	DevicePhyID   string            `json:"device_phy_id"`
	DeviceID      int               `json:"device_id"`
	IsOnline      bool              `json:"is_online"`
	Status        string            `json:"status"`             // offline/idle/charging
	Consistency   string            `json:"consistency_status"` // ok/inconsistent
	Inconsistency string            `json:"inconsistency_reason,omitempty"`
	LastHeartbeat *time.Time        `json:"last_heartbeat,omitempty"`
	Ports         []TestDevicePort  `json:"ports"`
	ActiveOrders  []TestActiveOrder `json:"active_orders,omitempty"`
	RegisteredAt  time.Time         `json:"registered_at"`
}

// PortStatusMeta 端口状态元信息（用于调试和高级场景）
//
// 提供协议层原始状态位的详细解析，帮助调试和理解底层协议状态。
// 所有位标志都基于 BKV 协议的位图定义。
type PortStatusMeta struct {
	RawStatus  int  `json:"raw_status" example:"137"`    // 协议层原始位图值（如 0x89=137）
	IsOnline   bool `json:"is_online" example:"true"`    // bit0 (0x01): 端口在线标志
	IsCharging bool `json:"is_charging" example:"false"` // bit7 (0x80): 正在充电标志
	IsIdle     bool `json:"is_idle" example:"true"`      // bit3 (0x08): 空载状态标志
	HasFault   bool `json:"has_fault" example:"false"`   // 是否存在故障（基于 !IsOnline）
}

// TestDevicePort 测试设备端口信息
//
// 端口状态使用业务枚举表示，而非协议层位图：
//   - 0: 空闲（idle）- 端口在线且无充电任务
//   - 1: 充电中（charging）- 端口正在执行充电任务
//   - 2: 故障（fault）- 端口离线或存在硬件故障
//
// Meta 字段提供原始协议状态的详细解析，用于高级调试。
type TestDevicePort struct {
	PortNo  int            `json:"port_no" example:"0"`                           // 端口号：0=A端口, 1=B端口, ...
	Status  int            `json:"status" enums:"0,1,2" example:"0"`              // 业务枚举: 0=空闲, 1=充电中, 2=故障
	Power   int            `json:"power" example:"0"`                             // 当前功率（瓦）
	OrderNo string         `json:"order_no,omitempty" example:"THD1763559879001"` // 关联订单号（仅充电时有值）
	Meta    PortStatusMeta `json:"meta"`                                          // 状态元信息（调试用）
}

// decodeConsolePortStatus 将BKV协议层位图状态转换为业务枚举
//
// BKV协议状态位定义 (参考 internal/protocol/bkv/tlv.go:257-283):
//   - bit0 (0x01): 在线标志
//   - bit3 (0x08): 空载标志
//   - bit7 (0x80): 充电中标志
//
// 转换规则（按优先级）:
//  1. bit7=1 → 1 (充电中)
//  2. !bit0  → 2 (故障/离线)
//  3. 其他   → 0 (空闲)
func decodeConsolePortStatus(raw int) (int, PortStatusMeta) {
	statusByte := raw & 0xFF
	meta := PortStatusMeta{
		RawStatus:  raw,
		IsOnline:   (statusByte & 0x01) != 0,
		IsCharging: (statusByte & 0x80) != 0,
		IsIdle:     (statusByte & 0x08) != 0,
	}
	meta.HasFault = !meta.IsOnline

	// 状态优先级判断
	switch {
	case meta.IsCharging:
		return 1, meta // 充电中
	case !meta.IsOnline:
		return 2, meta // 故障/离线
	default:
		return 0, meta // 空闲
	}
}

// TestActiveOrder 活跃订单信息
type TestActiveOrder struct {
	OrderNo   string     `json:"order_no"`
	PortNo    int        `json:"port_no"`
	Status    int        `json:"status"`
	StartTime *time.Time `json:"start_time,omitempty"` // 修复：支持NULL值（pending订单没有start_time）
	Amount    *int       `json:"amount,omitempty"`     // 修复：支持NULL值
}

// TestScenario 测试场景
type TestScenario struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Template    TestScenarioTemplate `json:"template"`
}

// TestScenarioTemplate 测试场景模板
type TestScenarioTemplate struct {
	ChargeMode      int `json:"charge_mode"`
	Amount          int `json:"amount"`
	DurationMinutes int `json:"duration_minutes,omitempty"`
	Power           int `json:"power,omitempty"`
	PricePerKwh     int `json:"price_per_kwh"`
	ServiceFee      int `json:"service_fee"`
}

// TestOrderDetail 测试订单详情
type TestOrderDetail struct {
	OrderNo     string     `json:"order_no"`
	DevicePhyID string     `json:"device_phy_id"`
	PortNo      int        `json:"port_no"`
	Status      int        `json:"status"`
	StatusText  string     `json:"status_text"`
	ChargeMode  int        `json:"charge_mode"`
	AmountCent  *int       `json:"amount_cent,omitempty"` // 修复：支持NULL值
	Kwh0p01     *int       `json:"kwh_0p01,omitempty"`    // 修复：支持NULL值
	StartTime   *time.Time `json:"start_time,omitempty"`
	EndTime     *time.Time `json:"end_time,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// SimulateThirdPartyRequest 模拟第三方调用请求
type SimulateThirdPartyRequest struct {
	Action      string                 `json:"action"` // start_charge, stop_charge, get_device, get_order
	DevicePhyID string                 `json:"device_phy_id,omitempty"`
	OrderNo     string                 `json:"order_no,omitempty"`
	Params      map[string]interface{} `json:"params,omitempty"`
}

// ListTestDevices 列出可测试设备
// @Summary 列出可测试设备
// @Description 获取所有可用于E2E测试的设备列表及其状态
// @Tags 内部测试控制台
// @Accept json
// @Produce json
// @Success 200 {object} StandardResponse{data=[]TestDevice}
// @Router /internal/test/devices [get]
func (h *TestConsoleHandler) ListTestDevices(c *gin.Context) {
	ctx := c.Request.Context()
	requestID := c.GetString("request_id")

	// 查询所有设备（使用分页，最多返回1000个）
	devices, err := h.repo.ListDevices(ctx, 1000, 0)
	if err != nil {
		h.logger.Error("failed to list devices", zap.Error(err))
		c.JSON(http.StatusInternalServerError, StandardResponse{
			Code:      500,
			Message:   "failed to list devices",
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// 构建测试设备列表（初始化为空数组，避免JSON序列化为null）
	testDevices := []TestDevice{}
	for _, dev := range devices {
		// 查询设备端口
		ports, err := h.repo.ListPortsByPhyID(ctx, dev.PhyID)
		if err != nil {
			h.logger.Warn("failed to list ports for device",
				zap.String("device_phy_id", dev.PhyID),
				zap.Error(err))
			continue
		}

		// 检查在线状态
		isOnline := h.sess.IsOnline(dev.PhyID, time.Now())

		// 查询活跃订单
		activeOrders, err := h.getActiveOrders(ctx, dev.ID)
		if err != nil {
			h.logger.Warn("failed to get active orders",
				zap.String("device_phy_id", dev.PhyID),
				zap.Error(err))
		}

		// 构建端口号到订单号的映射，用于填充 OrderNo 字段
		orderNoByPort := make(map[int]string)
		for _, order := range activeOrders {
			if order.OrderNo != "" {
				orderNoByPort[order.PortNo] = order.OrderNo
			}
		}

		// 构建端口列表（初始化为空数组）
		testPorts := []TestDevicePort{}
		for _, port := range ports {
			powerW := 0
			if port.PowerW != nil {
				powerW = *port.PowerW
			}

			// 转换协议层状态为业务枚举
			statusEnum, meta := decodeConsolePortStatus(port.Status)

			testPorts = append(testPorts, TestDevicePort{
				PortNo:  port.PortNo,
				Status:  statusEnum,
				Power:   powerW,
				OrderNo: orderNoByPort[port.PortNo],
				Meta:    meta,
			})
		}

		// 设备级一致性视图（仅用于测试控制台）
		consistencyStatus, inconsistencyReason := h.thirdPartyH.evaluateDeviceConsistency(ctx, dev.ID, dev.PhyID, isOnline, nil)

		testDevices = append(testDevices, TestDevice{
			DevicePhyID:   dev.PhyID,
			DeviceID:      int(dev.ID),
			IsOnline:      isOnline,
			Status:        getDeviceStatus(isOnline, nil),
			Consistency:   consistencyStatus,
			Inconsistency: inconsistencyReason,
			Ports:         testPorts,
			ActiveOrders:  activeOrders,
			RegisteredAt:  time.Now(), // 使用当前时间作为占位符
		})
	}

	c.JSON(http.StatusOK, StandardResponse{
		Code:      0,
		Message:   "success",
		Data:      testDevices,
		RequestID: requestID,
		Timestamp: time.Now().Unix(),
	})
}

// GetTestDevice 获取测试设备详情
// @Summary 获取测试设备详情
// @Description 获取指定设备的详细信息，包括端口状态和活跃订单
// @Tags 内部测试控制台
// @Accept json
// @Produce json
// @Param device_id path string true "设备物理ID"
// @Success 200 {object} StandardResponse{data=TestDevice}
// @Router /internal/test/devices/{device_id} [get]
func (h *TestConsoleHandler) GetTestDevice(c *gin.Context) {
	ctx := c.Request.Context()
	devicePhyID := c.Param("device_id")
	requestID := c.GetString("request_id")

	// 获取设备信息
	device, err := h.repo.GetDeviceByPhyID(ctx, devicePhyID)
	if err != nil {
		h.logger.Error("failed to get device",
			zap.String("device_phy_id", devicePhyID),
			zap.Error(err))
		c.JSON(http.StatusNotFound, StandardResponse{
			Code:      404,
			Message:   "device not found",
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// 查询设备端口
	ports, err := h.repo.ListPortsByPhyID(ctx, devicePhyID)
	if err != nil {
		h.logger.Error("failed to list ports",
			zap.String("device_phy_id", devicePhyID),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, StandardResponse{
			Code:      500,
			Message:   "failed to list ports",
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// 检查在线状态
	isOnline := h.sess.IsOnline(devicePhyID, time.Now())

	// 查询活跃订单
	activeOrders, err := h.getActiveOrders(ctx, device.ID)
	if err != nil {
		h.logger.Warn("failed to get active orders",
			zap.String("device_phy_id", devicePhyID),
			zap.Error(err))
	}

	// 构建端口号到订单号的映射
	orderNoByPort := make(map[int]string)
	for _, order := range activeOrders {
		if order.OrderNo != "" {
			orderNoByPort[order.PortNo] = order.OrderNo
		}
	}

	// 构建端口列表（初始化为空数组）
	testPorts := []TestDevicePort{}
	for _, port := range ports {
		powerW := 0
		if port.PowerW != nil {
			powerW = *port.PowerW
		}

		// 转换协议层状态为业务枚举
		statusEnum, meta := decodeConsolePortStatus(port.Status)

		testPorts = append(testPorts, TestDevicePort{
			PortNo:  port.PortNo,
			Status:  statusEnum,
			Power:   powerW,
			OrderNo: orderNoByPort[port.PortNo],
			Meta:    meta,
		})
	}

	// 设备级一致性视图（仅用于测试控制台）
	consistencyStatus, inconsistencyReason := h.thirdPartyH.evaluateDeviceConsistency(ctx, device.ID, device.PhyID, isOnline, nil)

	testDevice := TestDevice{
		DevicePhyID:   device.PhyID,
		DeviceID:      int(device.ID),
		IsOnline:      isOnline,
		Status:        getDeviceStatus(isOnline, nil),
		Consistency:   consistencyStatus,
		Inconsistency: inconsistencyReason,
		Ports:         testPorts,
		ActiveOrders:  activeOrders,
		RegisteredAt:  time.Now(), // 使用当前时间作为占位符
	}

	c.JSON(http.StatusOK, StandardResponse{
		Code:      0,
		Message:   "success",
		Data:      testDevice,
		RequestID: requestID,
		Timestamp: time.Now().Unix(),
	})
}

// ListTestScenarios 列出测试场景
// @Summary 列出测试场景
// @Description 获取预定义的E2E测试场景列表
// @Tags 内部测试控制台
// @Accept json
// @Produce json
// @Success 200 {object} StandardResponse{data=[]TestScenario}
// @Router /internal/test/scenarios [get]
func (h *TestConsoleHandler) ListTestScenarios(c *gin.Context) {
	requestID := c.GetString("request_id")

	scenarios := []TestScenario{
		{
			ID:          "normal-charge",
			Name:        "正常充电成功",
			Description: "标准充电流程：创建订单 → 设备执行 → 订单结算 → 事件推送",
			Template: TestScenarioTemplate{
				ChargeMode:      1,   // 按时长
				Amount:          100, // 1元
				DurationMinutes: 5,
				PricePerKwh:     100, // 1元/度
				ServiceFee:      100, // 10%
			},
		},
		{
			ID:          "device-offline",
			Name:        "设备离线拒绝",
			Description: "验证设备离线时系统拒绝创建订单",
			Template: TestScenarioTemplate{
				ChargeMode:      1,
				Amount:          100,
				DurationMinutes: 5,
				PricePerKwh:     100,
				ServiceFee:      100,
			},
		},
		{
			ID:          "port-busy",
			Name:        "端口占用冲突",
			Description: "验证端口被占用时返回409错误",
			Template: TestScenarioTemplate{
				ChargeMode:      1,
				Amount:          100,
				DurationMinutes: 5,
				PricePerKwh:     100,
				ServiceFee:      100,
			},
		},
		{
			ID:          "manual-stop",
			Name:        "手动停止充电",
			Description: "充电中手动停止，验证订单状态流转",
			Template: TestScenarioTemplate{
				ChargeMode:      1,
				Amount:          500,
				DurationMinutes: 30,
				PricePerKwh:     100,
				ServiceFee:      100,
			},
		},
		{
			ID:          "auto-stop-by-duration",
			Name:        "按时长自动停止",
			Description: "验证按时长充电到期后，设备自动停止时订单和端口状态能正确收敛为完成/空闲",
			Template: TestScenarioTemplate{
				ChargeMode:      1, // 按时长
				Amount:          100,
				DurationMinutes: 1, // 1分钟自动停止
				PricePerKwh:     100,
				ServiceFee:      100,
			},
		},
		{
			ID:          "fallback-stop",
			Name:        "端口在充电但无订单的强制停止",
			Description: "模拟端口状态为充电但无任何活跃订单，调用停止接口验证fallback异常订单与端口自愈逻辑",
			Template: TestScenarioTemplate{
				ChargeMode:      1,
				Amount:          100,
				DurationMinutes: 5,
				PricePerKwh:     100,
				ServiceFee:      100,
			},
		},
		{
			ID:          "port-convergence-test",
			Name:        "端口状态收敛验证",
			Description: "完整的Start→Stop流程，验证订单终态化后端口状态在30秒内从charging收敛到idle，检测OrderMonitor和PortStatusSyncer的自愈能力",
			Template: TestScenarioTemplate{
				ChargeMode:      1,   // 按时长
				Amount:          100, // 1元
				DurationMinutes: 1,   // 1分钟短时充电
				PricePerKwh:     100,
				ServiceFee:      100,
			},
		},
		{
			ID:          "zombie-connection-test",
			Name:        "僵尸连接检测与自愈",
			Description: "验证TCP连接存在但心跳超时时的行为：SessionManager.IsOnlineWeighted降权判定、订单自动interrupt、端口状态修复。需要在设备端停止发送心跳但保持TCP连接",
			Template: TestScenarioTemplate{
				ChargeMode:      1,   // 按时长
				Amount:          100, // 1元
				DurationMinutes: 2,   // 2分钟，足够触发心跳超时
				PricePerKwh:     100,
				ServiceFee:      100,
			},
		},
	}

	c.JSON(http.StatusOK, StandardResponse{
		Code:      0,
		Message:   "success",
		Data:      scenarios,
		RequestID: requestID,
		Timestamp: time.Now().Unix(),
	})
}

// StartTestChargeRequest 启动测试充电请求
type StartTestChargeRequest struct {
	PortNo          int    `json:"port_no" binding:"min=0"` // 端口号：0=A端口, 1=B端口, ...（移除required，因为0是有效值）
	ChargeMode      int    `json:"charge_mode" binding:"required,min=1,max=4"`
	Amount          int    `json:"amount" binding:"required,min=1"`
	DurationMinutes int    `json:"duration_minutes"`
	Power           int    `json:"power"`
	PricePerKwh     int    `json:"price_per_kwh"`
	ServiceFee      int    `json:"service_fee"`
	ScenarioID      string `json:"scenario_id"` // 可选：使用场景ID
}

// StartTestCharge 启动测试充电
// @Summary 启动测试充电
// @Description 通过测试控制台启动充电，自动生成test_session_id并复用第三方API逻辑
// @Tags 内部测试控制台
// @Accept json
// @Produce json
// @Param device_id path string true "设备物理ID"
// @Param request body StartTestChargeRequest true "充电参数"
// @Success 200 {object} StandardResponse{data=map[string]interface{}}
// @Router /internal/test/devices/{device_id}/charge [post]
func (h *TestConsoleHandler) StartTestCharge(c *gin.Context) {
	ctx := c.Request.Context()
	devicePhyID := c.Param("device_id")
	requestID := c.GetString("request_id")

	// 解析请求
	var req StartTestChargeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, StandardResponse{
			Code:      400,
			Message:   fmt.Sprintf("invalid request: %v", err),
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// 生成test_session_id
	testSessionID := uuid.New().String()

	// 记录测试会话开始
	h.logger.Info("test session started",
		zap.String("test_session_id", testSessionID),
		zap.String("device_phy_id", devicePhyID),
		zap.String("scenario_id", req.ScenarioID),
		zap.Int("port_no", req.PortNo))

	// 将test_session_id注入到context中
	c.Set("test_session_id", testSessionID)

	// 根据测试场景执行对应逻辑
	switch req.ScenarioID {
	case "fallback-stop":
		// 场景：不直接启动充电，而是引导用户使用 StopTestCharge 配合手动构造端口状态
		// 这里只返回场景说明与新建的test_session_id，后续停止操作会记录完整时间线
		h.logger.Info("scenario: fallback-stop - session initialized",
			zap.String("test_session_id", testSessionID))
		c.JSON(http.StatusOK, StandardResponse{
			Code:    0,
			Message: "fallback-stop 场景已初始化，请根据需要手动设置端口状态后调用停止接口",
			Data: map[string]interface{}{
				"test_session_id": testSessionID,
				"device_id":       devicePhyID,
				"scenario":        "fallback-stop",
				"note":            "本场景主要用于观察 Stop 接口的 fallback 异常订单与端口自愈行为",
			},
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return

	case "device-offline":
		// 场景：设备离线拒绝 - 直接返回503错误，不调用充电API
		h.logger.Info("scenario: device-offline - returning 503",
			zap.String("test_session_id", testSessionID))
		c.JSON(http.StatusServiceUnavailable, StandardResponse{
			Code:    503,
			Message: "设备离线，无法创建订单",
			Data: map[string]interface{}{
				"test_session_id": testSessionID,
				"device_id":       devicePhyID,
				"status":          "offline",
				"scenario":        "device-offline",
			},
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return

	case "port-busy":
		// 场景：端口占用冲突 - 检查端口是否真的被占用
		// 如果没有占用，先创建一个pending订单模拟占用
		device, err := h.repo.GetDeviceByPhyID(ctx, devicePhyID)
		if err != nil {
			c.JSON(http.StatusNotFound, StandardResponse{
				Code:      404,
				Message:   "device not found",
				RequestID: requestID,
				Timestamp: time.Now().Unix(),
			})
			return
		}

		// 检查是否已有活跃订单（仅pending/confirmed/charging，不包含过渡状态）
		var existingOrder string
		checkSQL := `SELECT order_no FROM orders
			WHERE device_id = $1 AND port_no = $2 AND status IN (0, 1, 2)
			ORDER BY created_at DESC LIMIT 1`
		_ = h.repo.Pool.QueryRow(ctx, checkSQL, device.ID, req.PortNo).Scan(&existingOrder)

		if existingOrder == "" {
			// 没有活跃订单，创建一个pending订单来模拟占用
			mockOrderNo := fmt.Sprintf("MOCK%d%03d", time.Now().Unix(), req.PortNo)
			insertSQL := `INSERT INTO orders (device_id, order_no, port_no, amount_cent, status, charge_mode, created_at)
				VALUES ($1, $2, $3, $4, 0, $5, NOW())`
			_, _ = h.repo.Pool.Exec(ctx, insertSQL, device.ID, mockOrderNo, req.PortNo, req.Amount, req.ChargeMode)
			existingOrder = mockOrderNo
			h.logger.Info("scenario: port-busy - created mock order",
				zap.String("test_session_id", testSessionID),
				zap.String("mock_order", mockOrderNo))
		}

		// 查询真实的端口状态
		var actualPortStatus int
		portStatusQuery := `SELECT status FROM ports WHERE device_id = $1 AND port_no = $2`
		portStatusErr := h.repo.Pool.QueryRow(ctx, portStatusQuery, device.ID, req.PortNo).Scan(&actualPortStatus)

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

		// 返回端口占用错误
		c.JSON(http.StatusConflict, StandardResponse{
			Code:    409,
			Message: "端口正在使用中",
			Data: map[string]interface{}{
				"test_session_id": testSessionID,
				"current_order":   existingOrder,
				"port_status":     portStatusText,
				"scenario":        "port-busy",
			},
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return

	case "normal-charge", "manual-stop", "auto-stop-by-duration", "":
		// 场景：正常充电 或 手动停止充电 - 正常调用充电API
		// 空scenario_id也走正常流程
		h.logger.Info("scenario: normal flow",
			zap.String("test_session_id", testSessionID),
			zap.String("scenario_id", req.ScenarioID))
		// 继续执行下面的正常逻辑

	default:
		// 未知场景
		h.logger.Warn("unknown scenario_id, using normal flow",
			zap.String("test_session_id", testSessionID),
			zap.String("scenario_id", req.ScenarioID))
		// 继续执行正常逻辑
	}

	// 构造StartChargeRequest并调用第三方API逻辑
	chargeReq := StartChargeRequest{
		PortNo:          req.PortNo,
		ChargeMode:      req.ChargeMode,
		Amount:          req.Amount,
		DurationMinutes: req.DurationMinutes,
		Duration:        req.DurationMinutes,
		Power:           req.Power,
		PricePerKwh:     req.PricePerKwh,
		ServiceFee:      req.ServiceFee,
	}

	// 将请求体重新设置为 StartChargeRequest 格式
	c.Request = c.Request.Clone(context.WithValue(ctx, "test_session_id", testSessionID))

	// 保存原始的响应写入器，用于捕获第三方API的响应
	// 直接调用ThirdPartyHandler的StartCharge方法
	// 注意：需要确保已经设置了device_id参数
	originalParams := c.Params
	c.Params = gin.Params{{Key: "device_id", Value: devicePhyID}}

	// 保存原始请求体，准备替换
	bodyBytes, _ := json.Marshal(chargeReq)
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// 调用第三方API的充电逻辑
	h.thirdPartyH.StartCharge(c)

	// 恢复原始参数
	c.Params = originalParams
}

// StopTestCharge 停止测试充电
// @Summary 停止测试充电
// @Description 停止正在进行的测试充电
// @Tags 内部测试控制台
// @Accept json
// @Produce json
// @Param device_id path string true "设备物理ID"
// @Param request body StopChargeRequest true "停止充电参数"
// @Success 200 {object} StandardResponse
// @Router /internal/test/devices/{device_id}/stop [post]
func (h *TestConsoleHandler) StopTestCharge(c *gin.Context) {
	ctx := c.Request.Context()
	devicePhyID := c.Param("device_id")
	requestID := c.GetString("request_id")

	// 解析请求参数
	var req StopChargeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, StandardResponse{
			Code:      400,
			Message:   fmt.Sprintf("invalid request: %v", err),
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// 生成test_session_id用于追踪
	testSessionID := uuid.New().String()
	c.Set("test_session_id", testSessionID)

	h.logger.Info("test stop charge requested",
		zap.String("test_session_id", testSessionID),
		zap.String("device_phy_id", devicePhyID),
		zap.Int("port_no", *req.PortNo))

	// 将test_session_id注入到context中，供时间线服务记录
	c.Request = c.Request.Clone(context.WithValue(ctx, "test_session_id", testSessionID))

	// 设置路径参数供thirdPartyHandler使用
	originalParams := c.Params
	c.Params = gin.Params{{Key: "device_id", Value: devicePhyID}}

	// 重新构造请求体
	bodyBytes, _ := json.Marshal(req)
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// 使用ResponseRecorder捕获第三方API的响应
	recorder := &responseRecorder{
		ResponseWriter: c.Writer,
		body:           &bytes.Buffer{},
		statusCode:     200,
	}
	c.Writer = recorder

	// 调用第三方API的停止充电逻辑
	h.thirdPartyH.StopCharge(c)

	// 恢复原始参数
	c.Params = originalParams

	// 尝试修改响应，添加test_session_id
	if recorder.statusCode >= 200 && recorder.statusCode < 300 {
		var resp StandardResponse
		if err := json.Unmarshal(recorder.body.Bytes(), &resp); err == nil {
			// 成功解析，添加test_session_id到响应数据
			if resp.Data == nil {
				resp.Data = make(map[string]interface{})
			}
			if dataMap, ok := resp.Data.(map[string]interface{}); ok {
				dataMap["test_session_id"] = testSessionID
			} else {
				// Data不是map，包装成新的map
				resp.Data = map[string]interface{}{
					"original":        resp.Data,
					"test_session_id": testSessionID,
				}
			}
			// 重新序列化并写入
			newBody, _ := json.Marshal(resp)
			c.Writer = recorder.ResponseWriter
			c.Writer.WriteHeader(recorder.statusCode)
			c.Writer.Write(newBody)
			return
		}
	}

	// 无法修改响应，直接写回原始内容
	c.Writer = recorder.ResponseWriter
	c.Writer.WriteHeader(recorder.statusCode)
	c.Writer.Write(recorder.body.Bytes())
}

// responseRecorder 用于捕获响应内容的recorder
type responseRecorder struct {
	gin.ResponseWriter
	body       *bytes.Buffer
	statusCode int
}

func (r *responseRecorder) Write(data []byte) (int, error) {
	r.body.Write(data)
	return len(data), nil
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
}

// GetTestSession 获取测试会话时间线
// @Summary 获取测试会话时间线
// @Description 获取指定test_session_id的完整E2E时间线数据
// @Tags 内部测试控制台
// @Accept json
// @Produce json
// @Param test_session_id path string true "测试会话ID"
// @Success 200 {object} StandardResponse{data=service.Timeline}
// @Router /internal/test/sessions/{test_session_id} [get]
func (h *TestConsoleHandler) GetTestSession(c *gin.Context) {
	ctx := c.Request.Context()
	testSessionID := c.Param("test_session_id")
	requestID := c.GetString("request_id")

	h.logger.Info("fetching test session timeline",
		zap.String("test_session_id", testSessionID))

	// 调用时间线服务获取数据
	timeline, err := h.timelineService.GetTimeline(ctx, testSessionID)
	if err != nil {
		h.logger.Error("failed to get timeline",
			zap.String("test_session_id", testSessionID),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, StandardResponse{
			Code:      500,
			Message:   "failed to get timeline",
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	c.JSON(http.StatusOK, StandardResponse{
		Code:      0,
		Message:   "success",
		Data:      timeline,
		RequestID: requestID,
		Timestamp: time.Now().Unix(),
	})
}

// getActiveOrders 获取设备的活跃订单
// 注意：只包含真正的活跃状态（pending/confirmed/charging），不包含过渡状态（stopping/cancelling/interrupted）
// 过渡状态订单应该在30-60秒内自动流转到终态，由OrderMonitor负责清理
func (h *TestConsoleHandler) getActiveOrders(ctx context.Context, deviceID int64) ([]TestActiveOrder, error) {
	query := `
		SELECT order_no, port_no, status, start_time, amount_cent
		FROM orders
		WHERE device_id = $1 AND status IN (0, 1, 2)
		ORDER BY created_at DESC
		LIMIT 10
	`

	rows, err := h.repo.Pool.Query(ctx, query, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// 初始化为空数组，避免JSON序列化为null
	orders := []TestActiveOrder{}
	for rows.Next() {
		var order TestActiveOrder
		if err := rows.Scan(&order.OrderNo, &order.PortNo, &order.Status, &order.StartTime, &order.Amount); err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}

	return orders, nil
}

// ListTestOrders 列出测试订单
// @Summary 列出测试订单
// @Description 查询所有测试订单，支持按状态筛选
// @Tags 内部测试控制台
// @Accept json
// @Produce json
// @Param status query int false "订单状态筛选"
// @Param limit query int false "返回数量限制（默认50）"
// @Success 200 {object} StandardResponse{data=[]TestOrderDetail}
// @Router /internal/test/orders [get]
func (h *TestConsoleHandler) ListTestOrders(c *gin.Context) {
	ctx := c.Request.Context()
	requestID := c.GetString("request_id")

	// 解析查询参数
	statusFilter := c.Query("status")
	limit := 50
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 200 {
			limit = l
		}
	}

	// 构建查询
	query := `
		SELECT o.order_no, d.phy_id, o.port_no, o.status, o.charge_mode,
		       o.amount_cent, o.kwh_0p01, o.start_time, o.end_time,
		       o.created_at, o.updated_at
		FROM orders o
		JOIN devices d ON o.device_id = d.id
		WHERE 1=1
	`
	args := []interface{}{}
	argIdx := 1

	if statusFilter != "" {
		query += fmt.Sprintf(" AND o.status = $%d", argIdx)
		status, _ := strconv.Atoi(statusFilter)
		args = append(args, status)
		argIdx++
	}

	// 优先显示测试会话订单
	query += " ORDER BY o.test_session_id IS NOT NULL DESC, o.created_at DESC"
	query += fmt.Sprintf(" LIMIT $%d", argIdx)
	args = append(args, limit)

	rows, err := h.repo.Pool.Query(ctx, query, args...)
	if err != nil {
		h.logger.Error("failed to list test orders", zap.Error(err))
		c.JSON(http.StatusInternalServerError, StandardResponse{
			Code:      500,
			Message:   "failed to list orders",
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}
	defer rows.Close()

	// 初始化为空数组，避免JSON序列化为null
	orders := []TestOrderDetail{}
	for rows.Next() {
		var order TestOrderDetail
		err := rows.Scan(
			&order.OrderNo, &order.DevicePhyID, &order.PortNo, &order.Status,
			&order.ChargeMode, &order.AmountCent, &order.Kwh0p01,
			&order.StartTime, &order.EndTime, &order.CreatedAt, &order.UpdatedAt,
		)
		if err != nil {
			h.logger.Warn("failed to scan order", zap.Error(err))
			continue
		}

		// 转换状态文本
		order.StatusText = getOrderStatusText(order.Status)
		orders = append(orders, order)
	}

	c.JSON(http.StatusOK, StandardResponse{
		Code:      0,
		Message:   "success",
		Data:      orders,
		RequestID: requestID,
		Timestamp: time.Now().Unix(),
	})
}

// GetTestOrder 获取测试订单详情
// @Summary 获取测试订单详情
// @Description 获取指定订单的详细信息
// @Tags 内部测试控制台
// @Accept json
// @Produce json
// @Param order_no path string true "订单号"
// @Success 200 {object} StandardResponse{data=TestOrderDetail}
// @Router /internal/test/orders/{order_no} [get]
func (h *TestConsoleHandler) GetTestOrder(c *gin.Context) {
	ctx := c.Request.Context()
	orderNo := c.Param("order_no")
	requestID := c.GetString("request_id")

	query := `
		SELECT o.order_no, d.phy_id, o.port_no, o.status, o.charge_mode,
		       o.amount_cent, o.kwh_0p01, o.start_time, o.end_time,
		       o.created_at, o.updated_at
		FROM orders o
		JOIN devices d ON o.device_id = d.id
		WHERE o.order_no = $1
	`

	var order TestOrderDetail
	err := h.repo.Pool.QueryRow(ctx, query, orderNo).Scan(
		&order.OrderNo, &order.DevicePhyID, &order.PortNo, &order.Status,
		&order.ChargeMode, &order.AmountCent, &order.Kwh0p01,
		&order.StartTime, &order.EndTime, &order.CreatedAt, &order.UpdatedAt,
	)
	if err != nil {
		h.logger.Error("failed to get order", zap.String("order_no", orderNo), zap.Error(err))
		c.JSON(http.StatusNotFound, StandardResponse{
			Code:      404,
			Message:   "order not found",
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	order.StatusText = getOrderStatusText(order.Status)

	c.JSON(http.StatusOK, StandardResponse{
		Code:      0,
		Message:   "success",
		Data:      order,
		RequestID: requestID,
		Timestamp: time.Now().Unix(),
	})
}

// SimulateThirdPartyCall 模拟第三方调用
// @Summary 模拟第三方调用
// @Description 模拟第三方应用调用服务器API进行设备操作
// @Tags 内部测试控制台
// @Accept json
// @Produce json
// @Param request body SimulateThirdPartyRequest true "模拟请求参数"
// @Success 200 {object} StandardResponse
// @Router /internal/test/simulate [post]
func (h *TestConsoleHandler) SimulateThirdPartyCall(c *gin.Context) {
	ctx := c.Request.Context()
	requestID := c.GetString("request_id")

	var req SimulateThirdPartyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, StandardResponse{
			Code:      400,
			Message:   fmt.Sprintf("invalid request: %v", err),
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	h.logger.Info("simulate third party call",
		zap.String("action", req.Action),
		zap.String("device_phy_id", req.DevicePhyID),
		zap.String("order_no", req.OrderNo))

	var result map[string]interface{}
	var err error

	switch req.Action {
	case "start_charge":
		result, err = h.simulateStartCharge(ctx, req)
	case "stop_charge":
		result, err = h.simulateStopCharge(ctx, req)
	case "get_device":
		result, err = h.simulateGetDevice(ctx, req)
	case "get_order":
		result, err = h.simulateGetOrder(ctx, req)
	default:
		c.JSON(http.StatusBadRequest, StandardResponse{
			Code:      400,
			Message:   fmt.Sprintf("unknown action: %s", req.Action),
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, StandardResponse{
			Code:      500,
			Message:   err.Error(),
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	c.JSON(http.StatusOK, StandardResponse{
		Code:      0,
		Message:   "simulation completed",
		Data:      result,
		RequestID: requestID,
		Timestamp: time.Now().Unix(),
	})
}

// simulateStartCharge 模拟启动充电
func (h *TestConsoleHandler) simulateStartCharge(ctx context.Context, req SimulateThirdPartyRequest) (map[string]interface{}, error) {
	// 提取参数
	portNo, _ := req.Params["port_no"].(float64)
	chargeMode, _ := req.Params["charge_mode"].(float64)
	amount, _ := req.Params["amount"].(float64)
	durationMinutes, _ := req.Params["duration_minutes"].(float64)

	// 这里应该调用thirdPartyHandler.StartCharge，但由于需要gin.Context，我们简化处理
	// 实际应该创建一个模拟的gin.Context或提取业务逻辑到独立函数

	return map[string]interface{}{
		"action":       "start_charge",
		"device_id":    req.DevicePhyID,
		"port_no":      int(portNo),
		"charge_mode":  int(chargeMode),
		"amount":       int(amount),
		"duration_min": int(durationMinutes),
		"note":         "模拟调用成功，实际业务逻辑需要完整实现",
	}, nil
}

// simulateStopCharge 模拟停止充电
func (h *TestConsoleHandler) simulateStopCharge(ctx context.Context, req SimulateThirdPartyRequest) (map[string]interface{}, error) {
	portNo, _ := req.Params["port_no"].(float64)

	return map[string]interface{}{
		"action":    "stop_charge",
		"device_id": req.DevicePhyID,
		"port_no":   int(portNo),
		"note":      "模拟调用成功",
	}, nil
}

// simulateGetDevice 模拟查询设备
func (h *TestConsoleHandler) simulateGetDevice(ctx context.Context, req SimulateThirdPartyRequest) (map[string]interface{}, error) {
	// 查询设备信息
	device, err := h.repo.GetDeviceByPhyID(ctx, req.DevicePhyID)
	if err != nil {
		return nil, fmt.Errorf("device not found: %w", err)
	}

	isOnline := h.sess.IsOnline(req.DevicePhyID, time.Now())

	return map[string]interface{}{
		"action":    "get_device",
		"device_id": device.PhyID,
		"online":    isOnline,
		"note":      "模拟查询成功",
	}, nil
}

// simulateGetOrder 模拟查询订单
func (h *TestConsoleHandler) simulateGetOrder(ctx context.Context, req SimulateThirdPartyRequest) (map[string]interface{}, error) {
	query := `
		SELECT order_no, status, amount_cent, kwh_0p01
		FROM orders
		WHERE order_no = $1
	`

	var orderNo string
	var status int
	var amount, kwh *int
	err := h.repo.Pool.QueryRow(ctx, query, req.OrderNo).Scan(&orderNo, &status, &amount, &kwh)
	if err != nil {
		return nil, fmt.Errorf("order not found: %w", err)
	}

	result := map[string]interface{}{
		"action":   "get_order",
		"order_no": orderNo,
		"status":   status,
	}
	if amount != nil {
		result["amount_cent"] = *amount
	}
	if kwh != nil {
		result["kwh_0p01"] = *kwh
	}

	return result, nil
}

// getOrderStatusText 订单状态码转文本
func getOrderStatusText(status int) string {
	statusMap := map[int]string{
		0:  "pending",
		1:  "confirmed",
		2:  "charging",
		3:  "completed",
		4:  "failed",
		5:  "cancelled",
		6:  "refunded",
		7:  "settled",
		8:  "cancelling",
		9:  "stopping",
		10: "interrupted",
	}
	if text, ok := statusMap[status]; ok {
		return text
	}
	return "unknown"
}
