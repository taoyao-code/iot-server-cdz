package thirdparty

import (
	"testing"
)

func TestNewEvent(t *testing.T) {
	// 测试事件创建
	data := map[string]interface{}{
		"test_key": "test_value",
	}

	event := NewEvent(EventDeviceHeartbeat, "device-001", data)

	if event == nil {
		t.Fatal("event should not be nil")
	}

	if event.EventType != EventDeviceHeartbeat {
		t.Errorf("event type = %v, want %v", event.EventType, EventDeviceHeartbeat)
	}

	if event.DevicePhyID != "device-001" {
		t.Errorf("device phy id = %v, want device-001", event.DevicePhyID)
	}

	if event.EventID == "" {
		t.Error("event id should not be empty")
	}

	if event.Nonce == "" {
		t.Error("nonce should not be empty")
	}

	if event.Timestamp == 0 {
		t.Error("timestamp should not be zero")
	}
}

func TestDeviceRegisteredData_ToMap(t *testing.T) {
	data := &DeviceRegisteredData{
		ICCID:        "1234567890",
		IMEI:         "0987654321",
		DeviceType:   "charger",
		Firmware:     "v1.0.0",
		PortCount:    4,
		RegisteredAt: 1234567890,
	}

	m := data.ToMap()

	if m["iccid"] != "1234567890" {
		t.Errorf("iccid = %v, want 1234567890", m["iccid"])
	}

	if m["firmware"] != "v1.0.0" {
		t.Errorf("firmware = %v, want v1.0.0", m["firmware"])
	}

	if m["port_count"] != 4 {
		t.Errorf("port_count = %v, want 4", m["port_count"])
	}
}

func TestAllEventTypes(t *testing.T) {
	// 测试所有事件类型都已定义
	eventTypes := []EventType{
		EventDeviceRegistered,
		EventDeviceHeartbeat,
		EventChargingStarted,
		EventChargingEnded,
		EventDeviceAlarm,
		EventSocketStateChanged,
		EventOTAProgressUpdate,
	}

	if len(eventTypes) != 7 {
		t.Errorf("expected 7 event types, got %d", len(eventTypes))
	}

	// 确保所有事件类型都不为空
	for i, et := range eventTypes {
		if et == "" {
			t.Errorf("event type at index %d is empty", i)
		}
	}
}
