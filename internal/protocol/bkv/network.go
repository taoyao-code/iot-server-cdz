package bkv

import (
	"fmt"
	"time"

	"github.com/taoyao-code/iot-server/internal/storage/models"
)

// Week 6: 组网管理协议（cmd=0x08/0x09/0x0A）

// NetworkAck 组网相关指令的通用ACK（cmd=0x0005，子命令=0x08/0x09/0x0A）
// 文档示例：下发刷新列表后，设备回复 0x0005，payload: [0x08][0x01] (1=OK,0=失败)
type NetworkAck struct {
	SubCmd uint8 // 0x08 刷新列表, 0x09 添加, 0x0A 删除
	Result uint8 // 1=OK, 0=失败
}

// ParseNetworkAck 解析组网ACK（刷新/添加/删除），长度必须≥2
func ParseNetworkAck(data []byte) (*NetworkAck, error) {
	// 文档示例：payload 为 [len_hi][len_lo][sub_cmd][result]
	if len(data) < 4 {
		return nil, fmt.Errorf("network ack too short: %d", len(data))
	}
	declLen := int(data[0])<<8 | int(data[1])
	if declLen != len(data)-2 {
		return nil, fmt.Errorf("network ack bad length: decl=%d actual=%d", declLen, len(data)-2)
	}
	return &NetworkAck{
		SubCmd: data[2],
		Result: data[3],
	}, nil
}

// SocketEntry 组网刷新列表中的单个插座信息（1+6+4+1+1+1=14字节）
type SocketEntry struct {
	SocketNo uint8
	MAC      [6]byte
	UID      [4]byte
	Channel  uint8
	RSSI     int8
	Status   uint8 // 0=离线 1=在线
}

// SocketEntryToModel 转换为存储模型（gateway_sockets）
func (e SocketEntry) SocketEntryToModel(gatewayID string, seenAt time.Time) *models.GatewaySocket {
	macStr := fmt.Sprintf("%02X%02X%02X%02X%02X%02X", e.MAC[0], e.MAC[1], e.MAC[2], e.MAC[3], e.MAC[4], e.MAC[5])
	uidStr := fmt.Sprintf("%02X%02X%02X%02X", e.UID[0], e.UID[1], e.UID[2], e.UID[3])
	ch := int32(e.Channel)
	status := int32(e.Status)
	rssi := int32(e.RSSI)

	return &models.GatewaySocket{
		GatewayID:      gatewayID,
		SocketNo:       int32(e.SocketNo),
		SocketMAC:      macStr,
		SocketUID:      &uidStr,
		Channel:        &ch,
		Status:         &status,
		SignalStrength: &rssi,
		LastSeenAt:     &seenAt,
	}
}

// ParseNetworkRefreshList 解析刷新列表（0x0005 子命令0x08的“列表”报文：count + entries）
// 注意：有的设备仅回 ACK；只有收到列表时才能建立 UID-MAC 映射
func ParseNetworkRefreshList(data []byte) ([]SocketEntry, error) {
	if len(data) < 1 {
		return nil, fmt.Errorf("refresh list too short: %d", len(data))
	}
	count := int(data[0])
	expected := 1 + count*14
	if len(data) != expected {
		return nil, fmt.Errorf("refresh list length mismatch: count=%d expected=%d actual=%d", count, expected, len(data))
	}
	entries := make([]SocketEntry, 0, count)
	pos := 1
	for i := 0; i < count; i++ {
		if pos+14 > len(data) {
			return nil, fmt.Errorf("refresh list truncated at entry %d", i)
		}
		var e SocketEntry
		e.SocketNo = data[pos]
		copy(e.MAC[:], data[pos+1:pos+7])
		copy(e.UID[:], data[pos+7:pos+11])
		e.Channel = data[pos+11]
		e.RSSI = int8(data[pos+12])
		e.Status = data[pos+13]
		entries = append(entries, e)
		pos += 14
	}
	return entries, nil
}
