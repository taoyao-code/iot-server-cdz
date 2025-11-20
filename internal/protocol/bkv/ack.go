package bkv

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

func encodeBKVField(tag, value byte) []byte {
	return []byte{0x03, 0x01, tag, value}
}

// encodeBKVAck 构造BKV子协议ACK载荷
// 结构：04 01 01 [cmd] 0a 01 02 [frameSeq:8] 09 01 03 [gateway:7] + fields...
func encodeBKVAck(cmd uint16, frameSeq uint64, gatewayID string, fields ...[]byte) ([]byte, error) {
	gatewayBytes, err := hex.DecodeString(gatewayID)
	if err != nil {
		return nil, fmt.Errorf("invalid gateway id %q: %w", gatewayID, err)
	}

	// BKV文档固定7字节网关ID，不足左侧补0，超出截断
	switch {
	case len(gatewayBytes) < 7:
		padded := make([]byte, 7)
		copy(padded[7-len(gatewayBytes):], gatewayBytes)
		gatewayBytes = padded
	case len(gatewayBytes) > 7:
		gatewayBytes = gatewayBytes[:7]
	}

	buf := bytes.NewBuffer(make([]byte, 0, 32))
	buf.Write([]byte{0x04, 0x01, 0x01})
	var cmdBytes [2]byte
	binary.BigEndian.PutUint16(cmdBytes[:], cmd)
	buf.Write(cmdBytes[:])

	buf.Write([]byte{0x0a, 0x01, 0x02})
	var seqBytes [8]byte
	binary.BigEndian.PutUint64(seqBytes[:], frameSeq)
	buf.Write(seqBytes[:])

	buf.Write([]byte{0x09, 0x01, 0x03})
	buf.Write(gatewayBytes)

	for _, field := range fields {
		if len(field) == 0 {
			continue
		}
		buf.Write(field)
	}

	return buf.Bytes(), nil
}

// EncodeBKVStatusAck 构造0x1017插座状态上报的应答载荷
func EncodeBKVStatusAck(payload *BKVPayload, success bool) ([]byte, error) {
	if payload == nil {
		return nil, fmt.Errorf("nil payload")
	}

	status := byte(0x00)
	if success {
		status = 0x01
	}
	return encodeBKVAck(payload.Cmd, payload.FrameSeq, payload.GatewayID, encodeBKVField(0x0F, status))
}

// EncodeBKVChargingEndAck 构造0x1004充电结束应答
func EncodeBKVChargingEndAck(payload *BKVPayload, socketNo, portNo *int, success bool) ([]byte, error) {
	if payload == nil {
		return nil, fmt.Errorf("nil payload")
	}

	fields := make([][]byte, 0, 3)
	status := byte(0x00)
	if success {
		status = 0x01
	}
	fields = append(fields, encodeBKVField(0x0F, status))
	if socketNo != nil && *socketNo >= 0 {
		fields = append(fields, encodeBKVField(0x4A, byte(*socketNo)))
	}
	if portNo != nil && *portNo >= 0 {
		fields = append(fields, encodeBKVField(0x08, byte(*portNo)))
	}

	return encodeBKVAck(payload.Cmd, payload.FrameSeq, payload.GatewayID, fields...)
}

// EncodeBKVExceptionAck 构造0x1010异常事件应答
func EncodeBKVExceptionAck(payload *BKVPayload, socketNo *int, success bool) ([]byte, error) {
	if payload == nil {
		return nil, fmt.Errorf("nil payload")
	}

	fields := make([][]byte, 0, 2)
	if socketNo != nil && *socketNo >= 0 {
		fields = append(fields, encodeBKVField(0x4A, byte(*socketNo)))
	}
	status := byte(0x00)
	if success {
		status = 0x01
	}
	fields = append(fields, encodeBKVField(0x0F, status))

	return encodeBKVAck(payload.Cmd, payload.FrameSeq, payload.GatewayID, fields...)
}
