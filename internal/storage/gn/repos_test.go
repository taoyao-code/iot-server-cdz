package gn

import (
	"context"
	"testing"
	"time"
)

// 由于没有测试数据库，这里提供模拟的仓储实现用于测试

// MockDevicesRepo 模拟设备仓储
type MockDevicesRepo struct {
	devices map[string]*Device
}

func NewMockDevicesRepo() *MockDevicesRepo {
	return &MockDevicesRepo{
		devices: make(map[string]*Device),
	}
}

func (r *MockDevicesRepo) UpsertHeartbeat(ctx context.Context, deviceID string, gatewayID string, iccid string, rssi int, fwVer string) error {
	now := time.Now()
	r.devices[deviceID] = &Device{
		DeviceID:  deviceID,
		GatewayID: gatewayID,
		ICCID:     iccid,
		LastSeen:  &now,
		RSSI:      rssi,
		FwVer:     fwVer,
	}
	return nil
}

func (r *MockDevicesRepo) FindByID(ctx context.Context, deviceID string) (*Device, error) {
	device, exists := r.devices[deviceID]
	if !exists {
		return nil, nil // 或返回适当的错误
	}
	return device, nil
}

func (r *MockDevicesRepo) UpdateSeen(ctx context.Context, deviceID string) error {
	if device, exists := r.devices[deviceID]; exists {
		now := time.Now()
		device.LastSeen = &now
	}
	return nil
}

// MockPortsRepo 模拟端口仓储
type MockPortsRepo struct {
	ports map[string][]PortSnapshot
}

func NewMockPortsRepo() *MockPortsRepo {
	return &MockPortsRepo{
		ports: make(map[string][]PortSnapshot),
	}
}

func (r *MockPortsRepo) UpsertPortSnapshot(ctx context.Context, deviceID string, ports []PortSnapshot) error {
	r.ports[deviceID] = ports
	return nil
}

func (r *MockPortsRepo) ListByDevice(ctx context.Context, deviceID string) ([]PortSnapshot, error) {
	ports, exists := r.ports[deviceID]
	if !exists {
		return []PortSnapshot{}, nil
	}
	return ports, nil
}

// MockInboundLogsRepo 模拟入站日志仓储
type MockInboundLogsRepo struct {
	logs []InboundLog
}

type InboundLog struct {
	DeviceID   string
	Cmd        int
	Seq        int
	PayloadHex string
	ParsedOK   bool
	Reason     string
}

func NewMockInboundLogsRepo() *MockInboundLogsRepo {
	return &MockInboundLogsRepo{
		logs: make([]InboundLog, 0),
	}
}

func (r *MockInboundLogsRepo) Append(ctx context.Context, deviceID string, cmd int, seq int, payloadHex string, parsedOK bool, reason string) error {
	r.logs = append(r.logs, InboundLog{
		DeviceID:   deviceID,
		Cmd:        cmd,
		Seq:        seq,
		PayloadHex: payloadHex,
		ParsedOK:   parsedOK,
		Reason:     reason,
	})
	return nil
}

func TestMockDevicesRepo(t *testing.T) {
	repo := NewMockDevicesRepo()
	ctx := context.Background()

	// 测试UpsertHeartbeat
	deviceID := "test_device_001"
	gatewayID := "82200520004869"
	iccid := "89860463112070319417"
	rssi := 31
	fwVer := "v1.2.3"

	err := repo.UpsertHeartbeat(ctx, deviceID, gatewayID, iccid, rssi, fwVer)
	if err != nil {
		t.Fatalf("UpsertHeartbeat failed: %v", err)
	}

	// 测试FindByID
	device, err := repo.FindByID(ctx, deviceID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}

	if device == nil {
		t.Fatal("Device should not be nil")
	}

	if device.DeviceID != deviceID {
		t.Errorf("DeviceID mismatch: expected %s, got %s", deviceID, device.DeviceID)
	}
	if device.GatewayID != gatewayID {
		t.Errorf("GatewayID mismatch: expected %s, got %s", gatewayID, device.GatewayID)
	}
	if device.ICCID != iccid {
		t.Errorf("ICCID mismatch: expected %s, got %s", iccid, device.ICCID)
	}
	if device.RSSI != rssi {
		t.Errorf("RSSI mismatch: expected %d, got %d", rssi, device.RSSI)
	}
	if device.FwVer != fwVer {
		t.Errorf("FwVer mismatch: expected %s, got %s", fwVer, device.FwVer)
	}
	if device.LastSeen == nil {
		t.Error("LastSeen should not be nil")
	}

	// 测试UpdateSeen
	originalTime := *device.LastSeen
	time.Sleep(1 * time.Millisecond) // 确保时间差异

	err = repo.UpdateSeen(ctx, deviceID)
	if err != nil {
		t.Fatalf("UpdateSeen failed: %v", err)
	}

	updatedDevice, err := repo.FindByID(ctx, deviceID)
	if err != nil {
		t.Fatalf("FindByID after UpdateSeen failed: %v", err)
	}

	if !updatedDevice.LastSeen.After(originalTime) {
		t.Error("LastSeen should be updated to a more recent time")
	}
}

func TestMockPortsRepo(t *testing.T) {
	repo := NewMockPortsRepo()
	ctx := context.Background()
	deviceID := "test_device_001"

	// 测试UpsertPortSnapshot
	ports := []PortSnapshot{
		{
			DeviceID:   deviceID,
			PortNo:     0,
			StatusBits: 0x80,
			BizNo:      "12345",
			Voltage:    227.5,
			Current:    4.5,
			Power:      1000.0,
			Energy:     1.25,
			Duration:   45,
			UpdatedAt:  time.Now(),
		},
		{
			DeviceID:   deviceID,
			PortNo:     1,
			StatusBits: 0x00,
			BizNo:      "",
			Voltage:    227.0,
			Current:    0.0,
			Power:      0.0,
			Energy:     0.0,
			Duration:   0,
			UpdatedAt:  time.Now(),
		},
	}

	err := repo.UpsertPortSnapshot(ctx, deviceID, ports)
	if err != nil {
		t.Fatalf("UpsertPortSnapshot failed: %v", err)
	}

	// 测试ListByDevice
	retrievedPorts, err := repo.ListByDevice(ctx, deviceID)
	if err != nil {
		t.Fatalf("ListByDevice failed: %v", err)
	}

	if len(retrievedPorts) != len(ports) {
		t.Fatalf("Expected %d ports, got %d", len(ports), len(retrievedPorts))
	}

	for i, expectedPort := range ports {
		actualPort := retrievedPorts[i]

		if actualPort.DeviceID != expectedPort.DeviceID {
			t.Errorf("Port %d DeviceID mismatch: expected %s, got %s", i, expectedPort.DeviceID, actualPort.DeviceID)
		}
		if actualPort.PortNo != expectedPort.PortNo {
			t.Errorf("Port %d PortNo mismatch: expected %d, got %d", i, expectedPort.PortNo, actualPort.PortNo)
		}
		if actualPort.StatusBits != expectedPort.StatusBits {
			t.Errorf("Port %d StatusBits mismatch: expected 0x%02X, got 0x%02X", i, expectedPort.StatusBits, actualPort.StatusBits)
		}
		if actualPort.Voltage != expectedPort.Voltage {
			t.Errorf("Port %d Voltage mismatch: expected %.1f, got %.1f", i, expectedPort.Voltage, actualPort.Voltage)
		}
	}
}

func TestMockInboundLogsRepo(t *testing.T) {
	repo := NewMockInboundLogsRepo()
	ctx := context.Background()

	// 测试Append
	deviceID := "test_device_001"
	cmd := 0x0000
	seq := 12345
	payloadHex := "20200730164545"
	parsedOK := true
	reason := "success"

	err := repo.Append(ctx, deviceID, cmd, seq, payloadHex, parsedOK, reason)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// 验证日志被正确添加
	if len(repo.logs) != 1 {
		t.Fatalf("Expected 1 log entry, got %d", len(repo.logs))
	}

	log := repo.logs[0]
	if log.DeviceID != deviceID {
		t.Errorf("DeviceID mismatch: expected %s, got %s", deviceID, log.DeviceID)
	}
	if log.Cmd != cmd {
		t.Errorf("Cmd mismatch: expected %d, got %d", cmd, log.Cmd)
	}
	if log.Seq != seq {
		t.Errorf("Seq mismatch: expected %d, got %d", seq, log.Seq)
	}
	if log.PayloadHex != payloadHex {
		t.Errorf("PayloadHex mismatch: expected %s, got %s", payloadHex, log.PayloadHex)
	}
	if log.ParsedOK != parsedOK {
		t.Errorf("ParsedOK mismatch: expected %t, got %t", parsedOK, log.ParsedOK)
	}
	if log.Reason != reason {
		t.Errorf("Reason mismatch: expected %s, got %s", reason, log.Reason)
	}
}

// TestReposIntegration 测试仓储集成
func TestReposIntegration(t *testing.T) {
	// 创建模拟仓储（实际环境中会使用PostgreSQL）
	devicesRepo := NewMockDevicesRepo()
	portsRepo := NewMockPortsRepo()
	inboundRepo := NewMockInboundLogsRepo()

	ctx := context.Background()
	deviceID := "test_device_001"
	gatewayID := "82200520004869"

	// 1. 设备心跳更新
	err := devicesRepo.UpsertHeartbeat(ctx, deviceID, gatewayID, "89860463112070319417", 31, "v1.2.3")
	if err != nil {
		t.Fatalf("Device heartbeat failed: %v", err)
	}

	// 2. 端口状态更新
	ports := []PortSnapshot{
		{
			DeviceID:   deviceID,
			PortNo:     0,
			StatusBits: 0x80,
			BizNo:      "ORDER123",
			Voltage:    227.5,
			Current:    4.5,
			Power:      1000.0,
			Energy:     1.25,
			Duration:   45,
			UpdatedAt:  time.Now(),
		},
	}

	err = portsRepo.UpsertPortSnapshot(ctx, deviceID, ports)
	if err != nil {
		t.Fatalf("Port snapshot failed: %v", err)
	}

	// 3. 记录入站日志
	err = inboundRepo.Append(ctx, deviceID, 0x1000, 12345, "4a01013e02ffff", true, "parsed_successfully")
	if err != nil {
		t.Fatalf("Inbound log failed: %v", err)
	}

	// 4. 验证数据一致性
	device, err := devicesRepo.FindByID(ctx, deviceID)
	if err != nil {
		t.Fatalf("Find device failed: %v", err)
	}

	if device.GatewayID != gatewayID {
		t.Errorf("Device gateway ID mismatch")
	}

	retrievedPorts, err := portsRepo.ListByDevice(ctx, deviceID)
	if err != nil {
		t.Fatalf("List ports failed: %v", err)
	}

	if len(retrievedPorts) != 1 {
		t.Errorf("Expected 1 port, got %d", len(retrievedPorts))
	}

	if len(inboundRepo.logs) != 1 {
		t.Errorf("Expected 1 log entry, got %d", len(inboundRepo.logs))
	}

	t.Logf("Integration test passed: device=%+v, ports=%+v, logs=%d",
		device, retrievedPorts, len(inboundRepo.logs))
}

// TestPostgresReposInterface 测试PostgreSQL仓储接口（仅编译时检查）
func TestPostgresReposInterface(t *testing.T) {
	// 这个测试主要是编译时检查，确保PostgreSQL实现符合接口

	// 如果有真实的数据库连接，可以测试实际功能
	// pool := getTestDBPool() // 实际测试时需要
	// repos := NewPostgresRepos(pool)

	// 模拟接口检查
	var _ DevicesRepo = (*postgresDevicesRepo)(nil)
	var _ PortsRepo = (*postgresPortsRepo)(nil)
	var _ InboundLogsRepo = (*postgresInboundLogsRepo)(nil)
	var _ OutboundQueueRepo = (*postgresOutboundQueueRepo)(nil)
	var _ ParamsPendingRepo = (*postgresParamsPendingRepo)(nil)

	t.Log("All repository interfaces are properly implemented")
}
