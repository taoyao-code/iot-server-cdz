package gn

import (
	"encoding/hex"
	"testing"
)

func TestParseTLVs_Basic(t *testing.T) {
	// 简单的TLV数据: tag=0x4A, len=1, value=0x01
	data := []byte{0x4A, 0x01, 0x01}

	tlvs, err := ParseTLVs(data)
	if err != nil {
		t.Fatalf("ParseTLVs failed: %v", err)
	}

	if len(tlvs) != 1 {
		t.Fatalf("Expected 1 TLV, got %d", len(tlvs))
	}

	tlv := tlvs[0]
	if tlv.Tag != 0x4A {
		t.Errorf("Expected tag 0x4A, got 0x%02X", tlv.Tag)
	}
	if tlv.Length != 1 {
		t.Errorf("Expected length 1, got %d", tlv.Length)
	}
	if tlv.GetUint8() != 0x01 {
		t.Errorf("Expected value 0x01, got 0x%02X", tlv.GetUint8())
	}
}

func TestParseTLVs_Multiple(t *testing.T) {
	// 多个TLV: 0x4A 01 01, 0x3E 02 FFFF, 0x07 01 25
	data := []byte{
		0x4A, 0x01, 0x01, // 插座序号=1
		0x3E, 0x02, 0xFF, 0xFF, // 软件版本=FFFF
		0x07, 0x01, 0x25, // 温度=37
	}

	tlvs, err := ParseTLVs(data)
	if err != nil {
		t.Fatalf("ParseTLVs failed: %v", err)
	}

	if len(tlvs) != 3 {
		t.Fatalf("Expected 3 TLVs, got %d", len(tlvs))
	}

	// 验证第一个TLV
	tlv1 := tlvs[0]
	if tlv1.Tag != TagSocketNumber || tlv1.GetUint8() != 1 {
		t.Errorf("TLV1: expected socket number 1, got tag=0x%02X, value=%d",
			tlv1.Tag, tlv1.GetUint8())
	}

	// 验证第二个TLV
	tlv2 := tlvs[1]
	if tlv2.Tag != TagSoftwareVer || tlv2.GetUint16() != 0xFFFF {
		t.Errorf("TLV2: expected software version 0xFFFF, got tag=0x%02X, value=0x%04X",
			tlv2.Tag, tlv2.GetUint16())
	}

	// 验证第三个TLV
	tlv3 := tlvs[2]
	if tlv3.Tag != TagTemperature || tlv3.GetUint8() != 0x25 {
		t.Errorf("TLV3: expected temperature 37, got tag=0x%02X, value=%d",
			tlv3.Tag, tlv3.GetUint8())
	}
}

func TestTLVList_FindByTag(t *testing.T) {
	tlvs := TLVList{
		NewTLVUint8(TagSocketNumber, 1),
		NewTLVUint16(TagSoftwareVer, 0xFFFF),
		NewTLVUint8(TagTemperature, 37),
	}

	// 查找存在的标签
	tlv := tlvs.FindByTag(TagSoftwareVer)
	if tlv == nil {
		t.Error("Expected to find software version TLV")
	} else if tlv.GetUint16() != 0xFFFF {
		t.Errorf("Expected software version 0xFFFF, got 0x%04X", tlv.GetUint16())
	}

	// 查找不存在的标签
	tlv = tlvs.FindByTag(0xFF)
	if tlv != nil {
		t.Error("Expected nil for non-existent tag")
	}
}

func TestTLV_EncodeDecodeRoundTrip(t *testing.T) {
	original := TLVList{
		NewTLVUint8(TagSocketNumber, 1),
		NewTLVUint16(TagVoltage, 2275),
		NewTLVString(0x99, "test"),
	}

	// 编码
	encoded := EncodeTLVs(original)
	t.Logf("Encoded TLVs: %s", hex.EncodeToString(encoded))

	// 解码
	decoded, err := ParseTLVs(encoded)
	if err != nil {
		t.Fatalf("ParseTLVs failed: %v", err)
	}

	// 验证
	if len(decoded) != len(original) {
		t.Fatalf("Expected %d TLVs, got %d", len(original), len(decoded))
	}

	for i := range original {
		if decoded[i].Tag != original[i].Tag {
			t.Errorf("TLV %d: tag mismatch, expected 0x%02X, got 0x%02X",
				i, original[i].Tag, decoded[i].Tag)
		}
		if decoded[i].Length != original[i].Length {
			t.Errorf("TLV %d: length mismatch, expected %d, got %d",
				i, original[i].Length, decoded[i].Length)
		}
		if hex.EncodeToString(decoded[i].Value) != hex.EncodeToString(original[i].Value) {
			t.Errorf("TLV %d: value mismatch, expected %s, got %s",
				i, hex.EncodeToString(original[i].Value), hex.EncodeToString(decoded[i].Value))
		}
	}
}

func TestParseTLVs_RealSocketStatus(t *testing.T) {
	// 构造一个符合GN协议的插孔属性TLV数据
	// 基于文档中的解析: 0x8=插孔号, 0x9=状态, 0xA=业务号, 0x95=电压等
	tlvs := TLVList{
		NewTLVUint8(TagPortNumber, 0),      // 插孔号=0
		NewTLVUint8(TagPortStatus, 0x80),   // 状态=0x80
		NewTLVUint16(TagBusinessNumber, 0), // 业务号=0
		NewTLVUint16(TagVoltage, 0x08E3),   // 电压=2275 (227.5V)
		NewTLVUint16(TagPower, 0),          // 功率=0
		NewTLVUint16(TagCurrent, 0),        // 电流=0
		NewTLVUint16(TagEnergy, 0),         // 用电量=0
		NewTLVUint16(TagDuration, 0),       // 时间=0
	}

	// 编码
	encoded := EncodeTLVs(tlvs)
	t.Logf("Encoded socket TLVs: %s", hex.EncodeToString(encoded))

	// 解码
	decoded, err := ParseTLVs(encoded)
	if err != nil {
		t.Fatalf("ParseTLVs failed: %v", err)
	}

	t.Logf("Parsed %d TLVs from socket status", len(decoded))

	// 验证关键字段
	portNumberTLV := decoded.FindByTag(TagPortNumber)
	if portNumberTLV == nil || portNumberTLV.GetUint8() != 0 {
		t.Error("Expected port number 0")
	}

	voltageTLV := decoded.FindByTag(TagVoltage)
	if voltageTLV == nil || voltageTLV.GetUint16() != 0x08E3 {
		t.Error("Expected voltage 0x08E3")
	} else {
		voltage := voltageTLV.GetUint16()
		t.Logf("Voltage: %d (%.1fV)", voltage, float64(voltage)/10.0)
	}
}

func TestParseTLVs_InvalidLength(t *testing.T) {
	// TLV声明长度为5，但实际数据只有2字节
	data := []byte{0x4A, 0x05, 0x01, 0x02}

	_, err := ParseTLVs(data)
	if err == nil {
		t.Error("Expected error for invalid TLV length")
	}
}

func TestTLV_GetMethods(t *testing.T) {
	// 测试各种数据类型的getter方法
	tests := []struct {
		name      string
		tlv       TLV
		uint8Val  uint8
		uint16Val uint16
		stringVal string
	}{
		{
			name:      "uint8",
			tlv:       NewTLVUint8(0x01, 123),
			uint8Val:  123,
			uint16Val: 0, // 只有1字节，GetUint16返回0
			stringVal: string([]byte{123}),
		},
		{
			name:      "uint16",
			tlv:       NewTLVUint16(0x02, 0x1234),
			uint8Val:  0x12, // 第一个字节
			uint16Val: 0x1234,
			stringVal: "\x12\x34", // 二进制字符串
		},
		{
			name:      "string",
			tlv:       NewTLVString(0x03, "hello"),
			uint8Val:  'h',    // 第一个字符的ASCII
			uint16Val: 0x6865, // "he" 的大端序值
			stringVal: "hello",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.tlv.GetUint8() != test.uint8Val {
				t.Errorf("GetUint8(): expected %d, got %d", test.uint8Val, test.tlv.GetUint8())
			}
			if test.tlv.GetUint16() != test.uint16Val {
				t.Errorf("GetUint16(): expected %d, got %d", test.uint16Val, test.tlv.GetUint16())
			}
			if test.tlv.GetString() != test.stringVal {
				t.Errorf("GetString(): expected %q, got %q", test.stringVal, test.tlv.GetString())
			}
		})
	}
}
