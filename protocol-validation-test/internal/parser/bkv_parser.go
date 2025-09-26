package parser

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
)

// DefaultBKVParser 默认BKV协议解析器实现
type DefaultBKVParser struct {
	tlvParser TLVParser
}

// NewDefaultBKVParser 创建默认BKV解析器
func NewDefaultBKVParser(tlvParser TLVParser) *DefaultBKVParser {
	return &DefaultBKVParser{
		tlvParser: tlvParser,
	}
}

// ParseBKV 解析BKV载荷
func (p *DefaultBKVParser) ParseBKV(data []byte) (*BKVPayload, error) {
	payload := &BKVPayload{
		Fields: make(map[uint8][]byte),
	}

	// 最小BKV载荷长度检查 (至少包含命令码)
	if len(data) < 2 {
		return nil, fmt.Errorf("BKV payload too short: %d bytes", len(data))
	}

	// 解析BKV命令码 (前2字节)
	payload.Command = binary.BigEndian.Uint16(data[0:2])

	// 如果有更多数据，继续解析
	if len(data) > 2 {
		// 尝试解析TLV结构
		tlvData := data[2:]
		fields, err := p.tlvParser.ParseTLV(tlvData)
		if err != nil {
			return nil, fmt.Errorf("failed to parse TLV in BKV payload: %w", err)
		}
		payload.Fields = fields

		// 提取常用字段
		p.extractCommonFields(payload)
	}

	return payload, nil
}

// extractCommonFields 提取常用字段
func (p *DefaultBKVParser) extractCommonFields(payload *BKVPayload) {
	// 提取序列号 (0x02)
	if sequenceBytes, exists := payload.Fields[0x02]; exists && len(sequenceBytes) == 8 {
		payload.Sequence = binary.BigEndian.Uint64(sequenceBytes)
	}

	// 提取网关ID (0x03)
	if gatewayBytes, exists := payload.Fields[0x03]; exists {
		payload.GatewayID = hex.EncodeToString(gatewayBytes)
		payload.GatewayID = strings.ToUpper(payload.GatewayID)
	}

	// 提取插座号 (0x4A)
	if socketBytes, exists := payload.Fields[0x4A]; exists && len(socketBytes) == 1 {
		payload.SocketNo = socketBytes[0]
	}

	// 提取插孔号 (0x08)
	if portBytes, exists := payload.Fields[0x08]; exists && len(portBytes) == 1 {
		payload.PortNo = portBytes[0]
	}
}

// ValidateBKV 验证BKV载荷
func (p *DefaultBKVParser) ValidateBKV(payload *BKVPayload) error {
	errors := make([]string, 0)

	// 验证命令码
	if !p.isValidBKVCommand(payload.Command) {
		errors = append(errors, fmt.Sprintf("Invalid BKV command: 0x%04X", payload.Command))
	}

	// 根据命令类型进行特定验证
	switch payload.Command {
	case BKVCmdStatusReport:
		if err := p.validateStatusReport(payload); err != nil {
			errors = append(errors, err.Error())
		}
	case BKVCmdControlDevice:
		if err := p.validateControlDevice(payload); err != nil {
			errors = append(errors, err.Error())
		}
	case BKVCmdChargingEnd:
		if err := p.validateChargingEnd(payload); err != nil {
			errors = append(errors, err.Error())
		}
	case BKVCmdCardCharging:
		if err := p.validateCardCharging(payload); err != nil {
			errors = append(errors, err.Error())
		}
	case BKVCmdBalanceQuery:
		if err := p.validateBalanceQuery(payload); err != nil {
			errors = append(errors, err.Error())
		}
	case BKVCmdParamSet:
		if err := p.validateParamSet(payload); err != nil {
			errors = append(errors, err.Error())
		}
	case BKVCmdParamQuery:
		if err := p.validateParamQuery(payload); err != nil {
			errors = append(errors, err.Error())
		}
	case BKVCmdExceptionEvent:
		if err := p.validateExceptionEvent(payload); err != nil {
			errors = append(errors, err.Error())
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("BKV validation failed: %s", strings.Join(errors, ", "))
	}

	return nil
}

// isValidBKVCommand 检查是否为有效的BKV命令
func (p *DefaultBKVParser) isValidBKVCommand(command uint16) bool {
	validCommands := []uint16{
		BKVCmdStatusReport,
		BKVCmdControlDevice,
		BKVCmdChargingEnd,
		BKVCmdCardCharging,
		BKVCmdBalanceQuery,
		BKVCmdParamSet,
		BKVCmdParamQuery,
		BKVCmdExceptionEvent,
	}

	for _, validCmd := range validCommands {
		if command == validCmd {
			return true
		}
	}
	return false
}

// validateStatusReport 验证状态上报
func (p *DefaultBKVParser) validateStatusReport(payload *BKVPayload) error {
	// 必须包含网关ID
	if payload.GatewayID == "" {
		return fmt.Errorf("status report missing gateway ID")
	}

	// 检查插座状态字段 (0x94)
	if !p.tlvParser.HasField(payload.Fields, 0x94) {
		return fmt.Errorf("status report missing socket status field")
	}

	return nil
}

// validateControlDevice 验证设备控制
func (p *DefaultBKVParser) validateControlDevice(payload *BKVPayload) error {
	// 必须包含插座号
	if payload.SocketNo == 0 {
		return fmt.Errorf("control device missing socket number")
	}

	// 检查控制开关字段 (0x13)
	if !p.tlvParser.HasField(payload.Fields, 0x13) {
		return fmt.Errorf("control device missing switch field")
	}

	return nil
}

// validateChargingEnd 验证充电结束
func (p *DefaultBKVParser) validateChargingEnd(payload *BKVPayload) error {
	// 必须包含插座号
	if payload.SocketNo == 0 {
		return fmt.Errorf("charging end missing socket number")
	}

	// 检查业务号字段 (0x0A)
	if !p.tlvParser.HasField(payload.Fields, 0x0A) {
		return fmt.Errorf("charging end missing business ID field")
	}

	return nil
}

// validateCardCharging 验证刷卡充电
func (p *DefaultBKVParser) validateCardCharging(payload *BKVPayload) error {
	// 必须包含插座号
	if payload.SocketNo == 0 {
		return fmt.Errorf("card charging missing socket number")
	}

	// 检查业务号字段 (0x0A)
	if !p.tlvParser.HasField(payload.Fields, 0x0A) {
		return fmt.Errorf("card charging missing business ID field")
	}

	return nil
}

// validateBalanceQuery 验证余额查询
func (p *DefaultBKVParser) validateBalanceQuery(payload *BKVPayload) error {
	// 必须包含插座号
	if payload.SocketNo == 0 {
		return fmt.Errorf("balance query missing socket number")
	}

	return nil
}

// validateParamSet 验证参数设置
func (p *DefaultBKVParser) validateParamSet(payload *BKVPayload) error {
	// 必须包含插座号
	if payload.SocketNo == 0 {
		return fmt.Errorf("param set missing socket number")
	}

	return nil
}

// validateParamQuery 验证参数查询
func (p *DefaultBKVParser) validateParamQuery(payload *BKVPayload) error {
	// 必须包含插座号
	if payload.SocketNo == 0 {
		return fmt.Errorf("param query missing socket number")
	}

	return nil
}

// validateExceptionEvent 验证异常事件
func (p *DefaultBKVParser) validateExceptionEvent(payload *BKVPayload) error {
	// 必须包含插座号
	if payload.SocketNo == 0 {
		return fmt.Errorf("exception event missing socket number")
	}

	// 检查异常事件原因字段 (0x54)
	if !p.tlvParser.HasField(payload.Fields, 0x54) {
		return fmt.Errorf("exception event missing event reason field")
	}

	return nil
}

// GetBKVType 获取BKV类型
func (p *DefaultBKVParser) GetBKVType(payload *BKVPayload) string {
	switch payload.Command {
	case BKVCmdStatusReport:
		return "status_report"
	case BKVCmdControlDevice:
		return "control_device"
	case BKVCmdChargingEnd:
		return "charging_end"
	case BKVCmdCardCharging:
		return "card_charging"
	case BKVCmdBalanceQuery:
		return "balance_query"
	case BKVCmdParamSet:
		return "param_set"
	case BKVCmdParamQuery:
		return "param_query"
	case BKVCmdExceptionEvent:
		return "exception_event"
	default:
		return "unknown"
	}
}

// GetFieldValue 获取BKV字段值
func (p *DefaultBKVParser) GetFieldValue(payload *BKVPayload, tag uint8) ([]byte, bool) {
	return p.tlvParser.GetFieldValue(payload.Fields, tag)
}

// GetFieldAsUint8 获取字段值作为uint8
func (p *DefaultBKVParser) GetFieldAsUint8(payload *BKVPayload, tag uint8) (uint8, error) {
	return p.tlvParser.GetFieldAsUint8(payload.Fields, tag)
}

// GetFieldAsUint16 获取字段值作为uint16
func (p *DefaultBKVParser) GetFieldAsUint16(payload *BKVPayload, tag uint8) (uint16, error) {
	return p.tlvParser.GetFieldAsUint16(payload.Fields, tag)
}

// GetFieldAsUint32 获取字段值作为uint32
func (p *DefaultBKVParser) GetFieldAsUint32(payload *BKVPayload, tag uint8) (uint32, error) {
	return p.tlvParser.GetFieldAsUint32(payload.Fields, tag)
}

// GetFieldAsString 获取字段值作为字符串
func (p *DefaultBKVParser) GetFieldAsString(payload *BKVPayload, tag uint8) (string, error) {
	return p.tlvParser.GetFieldAsString(payload.Fields, tag)
}

// HasField 检查字段是否存在
func (p *DefaultBKVParser) HasField(payload *BKVPayload, tag uint8) bool {
	return p.tlvParser.HasField(payload.Fields, tag)
}