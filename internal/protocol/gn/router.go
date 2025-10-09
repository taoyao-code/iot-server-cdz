package gn

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"
)

// GN协议命令常量
const (
	CmdHeartbeat    = 0x0000 // A1 心跳/时间同步
	CmdStatusReport = 0x1000 // A3 插座状态上报
	CmdStatusQuery  = 0x1001 // A4 查询插座状态
	CmdControl      = 0x2000 // C1 控制指令
	CmdControlEnd   = 0x2001 // C2 结束上报
	CmdParamSet     = 0x3000 // E2 参数设置
	CmdParamQuery   = 0x3001 // E3 参数查询
	CmdException    = 0x4000 // F1 异常上报
)

// Handler 定义GN协议消息处理器接口
type Handler interface {
	HandleHeartbeat(ctx context.Context, frame *Frame, gwid string, payload []byte) error
	HandleStatusReport(ctx context.Context, frame *Frame, gwid string, payload []byte) error
	HandleStatusQuery(ctx context.Context, frame *Frame, gwid string, payload []byte) error
	HandleControl(ctx context.Context, frame *Frame, gwid string, payload []byte) error
	HandleControlEnd(ctx context.Context, frame *Frame, gwid string, payload []byte) error
	HandleParamSet(ctx context.Context, frame *Frame, gwid string, payload []byte) error
	HandleParamQuery(ctx context.Context, frame *Frame, gwid string, payload []byte) error
	HandleException(ctx context.Context, frame *Frame, gwid string, payload []byte) error
}

// Router GN协议路由器
type Router struct {
	handler Handler
}

// NewRouter 创建新的路由器
func NewRouter(handler Handler) *Router {
	return &Router{
		handler: handler,
	}
}

// Route 路由帧到对应的处理器
func (r *Router) Route(ctx context.Context, data []byte) error {
	// 解析帧
	frame, err := ParseFrame(data)
	if err != nil {
		return fmt.Errorf("parse frame failed: %w", err)
	}

	gwid := frame.GetGatewayIDHex()

	// 根据命令路由
	switch frame.Command {
	case CmdHeartbeat:
		return r.handler.HandleHeartbeat(ctx, frame, gwid, frame.Payload)
	case CmdStatusReport:
		return r.handler.HandleStatusReport(ctx, frame, gwid, frame.Payload)
	case CmdStatusQuery:
		return r.handler.HandleStatusQuery(ctx, frame, gwid, frame.Payload)
	case CmdControl:
		return r.handler.HandleControl(ctx, frame, gwid, frame.Payload)
	case CmdControlEnd:
		return r.handler.HandleControlEnd(ctx, frame, gwid, frame.Payload)
	case CmdParamSet:
		return r.handler.HandleParamSet(ctx, frame, gwid, frame.Payload)
	case CmdParamQuery:
		return r.handler.HandleParamQuery(ctx, frame, gwid, frame.Payload)
	case CmdException:
		return r.handler.HandleException(ctx, frame, gwid, frame.Payload)
	default:
		return fmt.Errorf("unknown command: 0x%04X", frame.Command)
	}
}

// DefaultHandler 默认处理器实现（用于测试和开发）
type DefaultHandler struct{}

// NewDefaultHandler 创建默认处理器
func NewDefaultHandler() *DefaultHandler {
	return &DefaultHandler{}
}

func (h *DefaultHandler) HandleHeartbeat(ctx context.Context, frame *Frame, gwid string, payload []byte) error {
	fmt.Printf("GN Heartbeat: gwid=%s, seq=0x%08X, payload=%s\n",
		gwid, frame.Sequence, hex.EncodeToString(payload))
	return nil
}

func (h *DefaultHandler) HandleStatusReport(ctx context.Context, frame *Frame, gwid string, payload []byte) error {
	fmt.Printf("GN Status Report: gwid=%s, seq=0x%08X, payload_len=%d\n",
		gwid, frame.Sequence, len(payload))
	return nil
}

func (h *DefaultHandler) HandleStatusQuery(ctx context.Context, frame *Frame, gwid string, payload []byte) error {
	fmt.Printf("GN Status Query: gwid=%s, seq=0x%08X\n",
		gwid, frame.Sequence)
	return nil
}

func (h *DefaultHandler) HandleControl(ctx context.Context, frame *Frame, gwid string, payload []byte) error {
	fmt.Printf("GN Control: gwid=%s, seq=0x%08X, payload=%s\n",
		gwid, frame.Sequence, hex.EncodeToString(payload))
	return nil
}

func (h *DefaultHandler) HandleControlEnd(ctx context.Context, frame *Frame, gwid string, payload []byte) error {
	fmt.Printf("GN Control End: gwid=%s, seq=0x%08X, payload=%s\n",
		gwid, frame.Sequence, hex.EncodeToString(payload))
	return nil
}

func (h *DefaultHandler) HandleParamSet(ctx context.Context, frame *Frame, gwid string, payload []byte) error {
	fmt.Printf("GN Param Set: gwid=%s, seq=0x%08X, payload=%s\n",
		gwid, frame.Sequence, hex.EncodeToString(payload))
	return nil
}

func (h *DefaultHandler) HandleParamQuery(ctx context.Context, frame *Frame, gwid string, payload []byte) error {
	fmt.Printf("GN Param Query: gwid=%s, seq=0x%08X\n",
		gwid, frame.Sequence)
	return nil
}

func (h *DefaultHandler) HandleException(ctx context.Context, frame *Frame, gwid string, payload []byte) error {
	fmt.Printf("GN Exception: gwid=%s, seq=0x%08X, payload=%s\n",
		gwid, frame.Sequence, hex.EncodeToString(payload))
	return nil
}

// ParseHeartbeat 解析心跳载荷
func ParseHeartbeat(payload []byte) (iccid string, rssi int, fwVer string, err error) {
	if len(payload) < 20 {
		return "", 0, "", fmt.Errorf("heartbeat payload too short: %d bytes", len(payload))
	}

	// 根据文档示例解析
	// ICCID (18字节的BCD编码) + 固件版本 + RSSI
	iccidBytes := payload[:18]
	iccid = hex.EncodeToString(iccidBytes)

	// 查找RSSI (通常在末尾)
	if len(payload) > 18 {
		rssi = int(payload[len(payload)-1])
	}

	// 固件版本在ICCID和RSSI之间
	if len(payload) > 19 {
		fwVerBytes := payload[18 : len(payload)-1]
		fwVer = string(fwVerBytes)
	}

	return iccid, rssi, fwVer, nil
}

// BuildTimeSync 构建时间同步响应载荷
func BuildTimeSync() []byte {
	now := time.Now()
	// 格式: YYYYMMDDHHMMSS (14字节)
	timeStr := now.Format("20060102150405")
	return []byte(timeStr)
}

// ParseSocketStatus 解析插座状态报告
func ParseSocketStatus(payload []byte) ([]SocketInfo, error) {
	// 解析TLV格式的插座状态
	tlvs, err := ParseTLVs(payload)
	if err != nil {
		return nil, fmt.Errorf("parse TLVs failed: %w", err)
	}

	var sockets []SocketInfo
	var currentSocket *SocketInfo

	for _, tlv := range tlvs {
		switch tlv.Tag {
		case TagSocketNumber:
			// 新的插座开始
			if currentSocket != nil {
				sockets = append(sockets, *currentSocket)
			}
			currentSocket = &SocketInfo{
				Number: int(tlv.GetUint8()),
				Ports:  make([]PortInfo, 0),
			}
		case TagSoftwareVer:
			if currentSocket != nil {
				currentSocket.SoftwareVer = tlv.GetUint16()
			}
		case TagTemperature:
			if currentSocket != nil {
				currentSocket.Temperature = int(tlv.GetUint8())
			}
		case TagRSSI:
			if currentSocket != nil {
				currentSocket.RSSI = int(tlv.GetUint8())
			}
		case TagSocketAttr:
			// 插孔属性，需要进一步解析TLV
			portTLVs, err := ParseTLVs(tlv.Value)
			if err != nil {
				continue
			}

			port := PortInfo{}
			for _, pTLV := range portTLVs {
				switch pTLV.Tag {
				case TagPortNumber:
					port.Number = int(pTLV.GetUint8())
				case TagPortStatus:
					port.StatusBits = int(pTLV.GetUint8())
				case TagBusinessNumber:
					port.BizNo = pTLV.GetUint16()
				case TagVoltage:
					port.Voltage = float64(pTLV.GetUint16()) / 10.0
				case TagPower:
					port.Power = float64(pTLV.GetUint16()) / 10.0
				case TagCurrent:
					port.Current = float64(pTLV.GetUint16()) / 1000.0
				case TagEnergy:
					port.Energy = float64(pTLV.GetUint16()) / 100.0
				case TagDuration:
					port.Duration = int(pTLV.GetUint16())
				}
			}

			if currentSocket != nil {
				currentSocket.Ports = append(currentSocket.Ports, port)
			}
		}
	}

	// 添加最后一个插座
	if currentSocket != nil {
		sockets = append(sockets, *currentSocket)
	}

	return sockets, nil
}

// SocketInfo 插座信息
type SocketInfo struct {
	Number      int        `json:"number"`
	SoftwareVer uint16     `json:"software_ver"`
	Temperature int        `json:"temperature"`
	RSSI        int        `json:"rssi"`
	Ports       []PortInfo `json:"ports"`
}

// PortInfo 端口信息
type PortInfo struct {
	Number     int     `json:"number"`
	StatusBits int     `json:"status_bits"`
	BizNo      uint16  `json:"biz_no"`
	Voltage    float64 `json:"voltage"`  // V
	Power      float64 `json:"power"`    // W
	Current    float64 `json:"current"`  // A
	Energy     float64 `json:"energy"`   // kWh
	Duration   int     `json:"duration"` // minutes
}

// BuildStatusQuery 构建状态查询命令载荷
func BuildStatusQuery(socketNum uint8) []byte {
	// 简单的查询命令，可能只需要插座编号
	return []byte{socketNum}
}
