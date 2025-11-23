package bkv

import (
	"context"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/taoyao-code/iot-server/internal/coremodel"
	"github.com/taoyao-code/iot-server/internal/driverapi"
	"go.uber.org/zap"
)

// CommandSource 实现 driverapi.CommandSource，将核心命令映射为BKV协议下行帧。
type CommandSource struct {
	outbound OutboundSender
	log      *zap.Logger
	msgSeq   uint32
}

var _ driverapi.CommandSource = (*CommandSource)(nil)

// NewCommandSource 创建 BKV CommandSource。
func NewCommandSource(outbound OutboundSender, log *zap.Logger) *CommandSource {
	return &CommandSource{
		outbound: outbound,
		log:      log,
	}
}

// SendCoreCommand implements driverapi.CommandSource.
func (c *CommandSource) SendCoreCommand(ctx context.Context, cmd *coremodel.CoreCommand) error {
	if cmd == nil {
		return nil
	}
	if c.outbound == nil {
		return fmt.Errorf("bkv command source: outbound sender not configured")
	}

	switch cmd.Type {
	case coremodel.CommandStartCharge:
		return c.handleStartCharge(ctx, cmd)
	case coremodel.CommandStopCharge:
		return c.handleStopCharge(ctx, cmd)
	case coremodel.CommandCancelSession:
		return c.handleCancelSession(ctx, cmd)
	case coremodel.CommandQueryPortStatus:
		return c.handleQueryPortStatus(ctx, cmd)
	case coremodel.CommandSetParams:
		return c.handleSetParams(ctx, cmd)
	case coremodel.CommandTriggerOTA:
		return c.handleTriggerOTA(ctx, cmd)
	case coremodel.CommandConfigureNetwork:
		return c.handleConfigureNetwork(ctx, cmd)
	default:
		if c.log != nil {
			c.log.Warn("bkv command source: unsupported command type",
				zap.String("type", string(cmd.Type)))
		}
		return fmt.Errorf("unsupported core command: %s", cmd.Type)
	}
}

func (c *CommandSource) handleStartCharge(ctx context.Context, cmd *coremodel.CoreCommand) error {
	payload := cmd.StartCharge
	if payload == nil {
		return fmt.Errorf("start charge payload is required")
	}

	deviceID := strings.TrimSpace(string(cmd.DeviceID))
	if deviceID == "" {
		return fmt.Errorf("device id is required")
	}

	biz, err := parseBusinessNo(cmd.BusinessNo)
	if err != nil {
		return err
	}

	port := uint8(MapPort(int(cmd.PortNo)))
	durationMin := toDurationMinute(payload.TargetDurationSec)
	mode := toModeCode(payload)

	inner := EncodeStartControlPayload(0, port, mode, durationMin, biz)
	data := WrapControlPayload(inner)
	msgID := c.nextMsgID()

	if err := c.outbound.SendDownlink(deviceID, 0x0015, msgID, data); err != nil {
		return fmt.Errorf("send downlink failed: %w", err)
	}

	if c.log != nil {
		c.log.Info("bkv command source: start charge dispatched",
			zap.String("device_id", deviceID),
			zap.Uint8("port", port),
			zap.Uint16("business_no", biz),
			zap.Uint8("mode", mode),
			zap.Uint16("duration_min", durationMin))
	}

	return nil
}

func (c *CommandSource) handleStopCharge(ctx context.Context, cmd *coremodel.CoreCommand) error {
	deviceID := strings.TrimSpace(string(cmd.DeviceID))
	if deviceID == "" {
		return fmt.Errorf("device id is required")
	}

	biz, err := parseBusinessNo(cmd.BusinessNo)
	if err != nil {
		return err
	}

	port := uint8(MapPort(int(cmd.PortNo)))
	inner := EncodeStopControlPayload(0, port, biz)
	data := WrapControlPayload(inner)
	msgID := c.nextMsgID()

	if err := c.outbound.SendDownlink(deviceID, 0x0015, msgID, data); err != nil {
		return fmt.Errorf("send downlink failed: %w", err)
	}
	if c.log != nil {
		c.log.Info("bkv command source: stop charge dispatched",
			zap.String("device_id", deviceID),
			zap.Uint8("port", port),
			zap.Uint16("business_no", biz))
	}
	return nil
}

func (c *CommandSource) handleCancelSession(ctx context.Context, cmd *coremodel.CoreCommand) error {
	// 复用停止命令编码，但记录取消原因便于诊断
	if cmd.CancelSession != nil && c.log != nil {
		biz := ""
		if cmd.BusinessNo != nil {
			biz = string(*cmd.BusinessNo)
		}
		c.log.Info("bkv command source: cancel session requested",
			zap.String("device_id", string(cmd.DeviceID)),
			zap.Int("port_no", int(cmd.PortNo)),
			zap.String("business_no", biz),
			zap.String("reason", cmd.CancelSession.Reason))
	}
	return c.handleStopCharge(ctx, cmd)
}

func (c *CommandSource) handleQueryPortStatus(ctx context.Context, cmd *coremodel.CoreCommand) error {
	deviceID := strings.TrimSpace(string(cmd.DeviceID))
	if deviceID == "" {
		return fmt.Errorf("device id is required")
	}

	socket := uint8(0)
	if cmd.QueryPortStatus != nil && cmd.QueryPortStatus.SocketNo != nil {
		if sn := *cmd.QueryPortStatus.SocketNo; sn >= 0 {
			socket = uint8(sn)
		}
	}

	payload := EncodeQueryPortStatusPayload(socket)
	msgID := c.nextMsgID()

	if err := c.outbound.SendDownlink(deviceID, 0x0015, msgID, payload); err != nil {
		return fmt.Errorf("send downlink failed: %w", err)
	}
	if c.log != nil {
		c.log.Info("bkv command source: query port status dispatched",
			zap.String("device_id", deviceID),
			zap.Uint8("socket_no", socket))
	}
	return nil
}

func (c *CommandSource) handleSetParams(ctx context.Context, cmd *coremodel.CoreCommand) error {
	if cmd.SetParams == nil || len(cmd.SetParams.Params) == 0 {
		return fmt.Errorf("set params payload required")
	}
	deviceID := strings.TrimSpace(string(cmd.DeviceID))
	if deviceID == "" {
		return fmt.Errorf("device id is required")
	}

	payload := encodeSetParamsPayload(cmd.SetParams.Params)
	msgID := c.nextMsgID()

	if err := c.outbound.SendDownlink(deviceID, 0x0002, msgID, payload); err != nil {
		return fmt.Errorf("send downlink failed: %w", err)
	}
	if c.log != nil {
		c.log.Info("bkv command source: set params dispatched",
			zap.String("device_id", deviceID),
			zap.Int("param_count", len(cmd.SetParams.Params)))
	}
	return nil
}

func (c *CommandSource) handleTriggerOTA(ctx context.Context, cmd *coremodel.CoreCommand) error {
	payload := cmd.TriggerOTA
	if payload == nil {
		return fmt.Errorf("trigger OTA payload required")
	}
	deviceID := strings.TrimSpace(string(cmd.DeviceID))
	if deviceID == "" {
		return fmt.Errorf("device id is required")
	}
	data := encodeOTAPayload(payload)
	msgID := c.nextMsgID()

	if err := c.outbound.SendDownlink(deviceID, 0x0007, msgID, data); err != nil {
		return fmt.Errorf("send downlink failed: %w", err)
	}
	if c.log != nil {
		c.log.Info("bkv command source: ota dispatched",
			zap.String("device_id", deviceID),
			zap.String("version", payload.Version))
	}
	return nil
}

func (c *CommandSource) handleConfigureNetwork(ctx context.Context, cmd *coremodel.CoreCommand) error {
	payload := cmd.ConfigureNetwork
	if payload == nil {
		return fmt.Errorf("configure network payload required")
	}

	deviceID := strings.TrimSpace(string(cmd.DeviceID))
	if deviceID == "" {
		return fmt.Errorf("device id is required")
	}

	data, err := encodeConfigureNetworkPayload(payload)
	if err != nil {
		return err
	}

	msgID := c.nextMsgID()
	if err := c.outbound.SendDownlink(deviceID, 0x0005, msgID, data); err != nil {
		return fmt.Errorf("send downlink failed: %w", err)
	}
	if c.log != nil {
		c.log.Info("bkv command source: configure network dispatched",
			zap.String("device_id", deviceID),
			zap.Int("node_count", len(payload.Nodes)))
	}
	return nil
}

func (c *CommandSource) nextMsgID() uint32 {
	seq := atomic.AddUint32(&c.msgSeq, 1)
	if seq == 0 {
		return uint32(time.Now().Unix() % 65535)
	}
	return seq % 65535
}

func parseBusinessNo(bn *coremodel.BusinessNo) (uint16, error) {
	if bn == nil {
		return 0, fmt.Errorf("business no is required")
	}
	str := strings.TrimSpace(string(*bn))
	if str == "" {
		return 0, fmt.Errorf("business no is required")
	}

	base := 10
	if strings.HasPrefix(str, "0x") || strings.HasPrefix(str, "0X") {
		base = 16
		str = str[2:]
	}

	val, err := strconv.ParseUint(str, base, 16)
	if err != nil {
		return 0, fmt.Errorf("invalid business no: %s", str)
	}
	if val == 0 {
		val = 1
	}
	return uint16(val), nil
}

func toDurationMinute(secPtr *int32) uint16 {
	if secPtr == nil || *secPtr <= 0 {
		return 1
	}
	min := *secPtr / 60
	if min <= 0 {
		min = 1
	}
	if min > 0xFFFF {
		return 0xFFFF
	}
	return uint16(min)
}

func toModeCode(payload *coremodel.StartChargePayload) uint8 {
	if payload == nil {
		return 1
	}
	if payload.ModeCode != nil && *payload.ModeCode > 0 {
		code := *payload.ModeCode
		if code > 0 && code < 256 {
			return uint8(code)
		}
	}

	mode := strings.ToLower(payload.Mode)
	switch mode {
	case "duration", "time", "mode_1":
		return 1
	case "energy", "mode_2":
		return 2
	case "power", "mode_3":
		return 3
	case "full", "mode_4":
		return 4
	}

	if payload.Mode != "" {
		if val, err := strconv.Atoi(payload.Mode); err == nil && val > 0 && val < 256 {
			return uint8(val)
		}
	}

	return 1
}

func encodeSetParamsPayload(items []coremodel.SetParamItem) []byte {
	buf := make([]byte, 0, len(items)*4+1)
	buf = append(buf, byte(len(items)))
	for _, p := range items {
		valueBytes := []byte(p.Value)
		buf = append(buf, byte(p.ID), byte(len(valueBytes)))
		buf = append(buf, valueBytes...)
	}
	return buf
}

func encodeOTAPayload(payload *coremodel.TriggerOTAPayload) []byte {
	data := []byte{byte(payload.TargetType)}
	socket := uint8(0)
	if payload.TargetSocket != nil && *payload.TargetSocket >= 0 {
		socket = uint8(*payload.TargetSocket)
	}
	data = append(data, socket)
	if payload.FirmwareURL != "" {
		data = append(data, []byte(payload.FirmwareURL)...)
	}
	return data
}

func encodeConfigureNetworkPayload(payload *coremodel.ConfigureNetworkPayload) ([]byte, error) {
	inner := make([]byte, 2+len(payload.Nodes)*7)
	inner[0] = 0x08
	inner[1] = byte(payload.Channel)
	pos := 2
	for _, node := range payload.Nodes {
		inner[pos] = byte(node.SocketNo)
		pos++
		macBytes, err := hex.DecodeString(node.SocketMAC)
		if err != nil || len(macBytes) != 6 {
			return nil, fmt.Errorf("invalid socket mac: %s", node.SocketMAC)
		}
		copy(inner[pos:], macBytes)
		pos += 6
	}

	payloadWithLen := make([]byte, 2+len(inner))
	payloadWithLen[0] = byte(len(inner) >> 8)
	payloadWithLen[1] = byte(len(inner))
	copy(payloadWithLen[2:], inner)
	return payloadWithLen, nil
}
