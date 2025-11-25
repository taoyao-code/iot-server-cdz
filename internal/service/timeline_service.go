package service

import (
	"context"
	"encoding/json"
	"time"

	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
	"go.uber.org/zap"
)

// TimelineService 时间线聚合服务
type TimelineService struct {
	repo   *pgstorage.Repository
	logger *zap.Logger
}

// NewTimelineService 创建时间线服务
func NewTimelineService(repo *pgstorage.Repository, logger *zap.Logger) *TimelineService {
	return &TimelineService{
		repo:   repo,
		logger: logger,
	}
}

// Timeline E2E测试时间线
type Timeline struct {
	TestSessionID string          `json:"test_session_id"`
	DevicePhyID   string          `json:"device_phy_id,omitempty"`
	StartTime     time.Time       `json:"start_time"`
	EndTime       *time.Time      `json:"end_time,omitempty"`
	Events        []TimelineEvent `json:"events"`
	Summary       TimelineSummary `json:"summary"`
}

// TimelineEvent 时间线事件
type TimelineEvent struct {
	Timestamp   time.Time              `json:"timestamp"`
	Type        string                 `json:"type"` // http_request, db_operation, outbound_cmd, device_report, event_push
	Source      string                 `json:"source"`
	Description string                 `json:"description"`
	Data        map[string]interface{} `json:"data"`
	RawPayload  string                 `json:"raw_payload,omitempty"`
}

// TimelineSummary 时间线摘要
type TimelineSummary struct {
	TotalEvents   int                    `json:"total_events"`
	OrdersCreated int                    `json:"orders_created"`
	CommandsSent  int                    `json:"commands_sent"`
	EventsPushed  int                    `json:"events_pushed"`
	Errors        int                    `json:"errors"`
	FinalStatus   string                 `json:"final_status,omitempty"`
	Details       map[string]interface{} `json:"details,omitempty"`
}

// GetTimeline 获取test_session_id的完整时间线
func (s *TimelineService) GetTimeline(ctx context.Context, testSessionID string) (*Timeline, error) {
	timeline := &Timeline{
		TestSessionID: testSessionID,
		Events:        []TimelineEvent{},
		Summary: TimelineSummary{
			Details: make(map[string]interface{}),
		},
	}

	// 2. 查询出站命令
	cmdEvents, err := s.getOutboundCommandEvents(ctx, testSessionID)
	if err != nil {
		s.logger.Warn("failed to get outbound command events",
			zap.String("test_session_id", testSessionID),
			zap.Error(err))
	} else {
		timeline.Events = append(timeline.Events, cmdEvents...)
		timeline.Summary.CommandsSent = len(cmdEvents)
	}

	// 3. 查询指令日志
	cmdLogEvents, err := s.getCmdLogEvents(ctx, testSessionID)
	if err != nil {
		s.logger.Warn("failed to get cmd log events",
			zap.String("test_session_id", testSessionID),
			zap.Error(err))
	} else {
		timeline.Events = append(timeline.Events, cmdLogEvents...)
	}

	// 4. 查询事件推送
	eventPushEvents, err := s.getEventPushEvents(ctx, testSessionID)
	if err != nil {
		s.logger.Warn("failed to get event push events",
			zap.String("test_session_id", testSessionID),
			zap.Error(err))
	} else {
		timeline.Events = append(timeline.Events, eventPushEvents...)
		timeline.Summary.EventsPushed = len(eventPushEvents)
	}

	// 按时间排序事件
	sortEventsByTimestamp(timeline.Events)

	// 设置时间范围
	if len(timeline.Events) > 0 {
		timeline.StartTime = timeline.Events[0].Timestamp
		lastEvent := timeline.Events[len(timeline.Events)-1]
		timeline.EndTime = &lastEvent.Timestamp
	}

	// 汇总统计
	timeline.Summary.TotalEvents = len(timeline.Events)
	timeline.Summary.Errors = countEventsByType(timeline.Events, "error")

	return timeline, nil
}

// getOutboundCommandEvents 获取出站命令事件
func (s *TimelineService) getOutboundCommandEvents(ctx context.Context, testSessionID string) ([]TimelineEvent, error) {
	query := `
		SELECT
			id, device_phy_id, cmd, payload, priority,
			status, created_at, sent_at
		FROM outbound_queue
		WHERE test_session_id = $1
		ORDER BY created_at ASC
	`

	rows, err := s.repo.Pool.Query(ctx, query, testSessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []TimelineEvent
	for rows.Next() {
		var (
			id          int64
			devicePhyID string
			cmd         int
			payload     []byte
			priority    int
			status      int
			createdAt   time.Time
			sentAt      *time.Time
		)

		if err := rows.Scan(&id, &devicePhyID, &cmd, &payload, &priority,
			&status, &createdAt, &sentAt); err != nil {
			return nil, err
		}

		// 命令入队事件
		events = append(events, TimelineEvent{
			Timestamp:   createdAt,
			Type:        "outbound_cmd",
			Source:      "outbound_queue",
			Description: "下行命令入队",
			Data: map[string]interface{}{
				"queue_id":       id,
				"device_phy_id":  devicePhyID,
				"cmd":            cmd,
				"priority":       priority,
				"status":         status,
				"payload_length": len(payload),
			},
			RawPayload: formatHex(payload),
		})

		// 如果已发送，添加发送事件
		if sentAt != nil {
			events = append(events, TimelineEvent{
				Timestamp:   *sentAt,
				Type:        "outbound_cmd",
				Source:      "outbound_queue",
				Description: "下行命令已发送",
				Data: map[string]interface{}{
					"queue_id":      id,
					"device_phy_id": devicePhyID,
					"cmd":           cmd,
				},
			})
		}
	}

	return events, nil
}

// getCmdLogEvents 获取指令日志事件
func (s *TimelineService) getCmdLogEvents(ctx context.Context, testSessionID string) ([]TimelineEvent, error) {
	query := `
		SELECT
			device_id, port_no, cmd_type, direction,
			payload, created_at
		FROM cmd_log
		WHERE test_session_id = $1
		ORDER BY created_at ASC
		LIMIT 100
	`

	rows, err := s.repo.Pool.Query(ctx, query, testSessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []TimelineEvent
	for rows.Next() {
		var (
			deviceID  int
			portNo    int
			cmdType   string
			direction string
			payload   []byte
			createdAt time.Time
		)

		if err := rows.Scan(&deviceID, &portNo, &cmdType, &direction, &payload, &createdAt); err != nil {
			return nil, err
		}

		events = append(events, TimelineEvent{
			Timestamp:   createdAt,
			Type:        "device_report",
			Source:      "cmd_log",
			Description: "设备指令日志: " + cmdType,
			Data: map[string]interface{}{
				"device_id":      deviceID,
				"port_no":        portNo,
				"cmd_type":       cmdType,
				"direction":      direction,
				"payload_length": len(payload),
			},
			RawPayload: formatHex(payload),
		})
	}

	return events, nil
}

// getEventPushEvents 获取事件推送记录
func (s *TimelineService) getEventPushEvents(ctx context.Context, testSessionID string) ([]TimelineEvent, error) {
	query := `
		SELECT
			event_type, entity_type, entity_id,
			payload, status, created_at, sent_at
		FROM events
		WHERE test_session_id = $1
		ORDER BY created_at ASC
	`

	rows, err := s.repo.Pool.Query(ctx, query, testSessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []TimelineEvent
	for rows.Next() {
		var (
			eventType  string
			entityType string
			entityID   string
			payload    json.RawMessage
			status     int
			createdAt  time.Time
			sentAt     *time.Time
		)

		if err := rows.Scan(&eventType, &entityType, &entityID,
			&payload, &status, &createdAt, &sentAt); err != nil {
			return nil, err
		}

		// 事件创建
		events = append(events, TimelineEvent{
			Timestamp:   createdAt,
			Type:        "event_push",
			Source:      "events",
			Description: "事件生成: " + eventType,
			Data: map[string]interface{}{
				"event_type":  eventType,
				"entity_type": entityType,
				"entity_id":   entityID,
				"status":      status,
			},
			RawPayload: string(payload),
		})

		// 如果已发送
		if sentAt != nil {
			events = append(events, TimelineEvent{
				Timestamp:   *sentAt,
				Type:        "event_push",
				Source:      "events",
				Description: "事件已推送: " + eventType,
				Data: map[string]interface{}{
					"event_type": eventType,
					"status":     status,
				},
			})
		}
	}

	return events, nil
}

// 辅助函数

func sortEventsByTimestamp(events []TimelineEvent) {
	// 简单冒泡排序（实际应使用sort.Slice）
	for i := 0; i < len(events); i++ {
		for j := i + 1; j < len(events); j++ {
			if events[i].Timestamp.After(events[j].Timestamp) {
				events[i], events[j] = events[j], events[i]
			}
		}
	}
}

func countEventsByType(events []TimelineEvent, eventType string) int {
	count := 0
	for _, e := range events {
		if e.Type == eventType {
			count++
		}
	}
	return count
}

func getOrderStatusText(status interface{}) string {
	var statusInt int
	switch v := status.(type) {
	case int:
		statusInt = v
	case int64:
		statusInt = int(v)
	default:
		return "unknown"
	}

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

	if text, ok := statusMap[statusInt]; ok {
		return text
	}
	return "unknown"
}

func formatHex(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	if len(data) > 100 {
		// 只显示前100字节
		data = data[:100]
	}
	hex := ""
	for _, b := range data {
		hex += string(rune(b))
	}
	return hex
}
