package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"

	"github.com/taoyao-code/iot-server/internal/protocol/gn"
)

// SimpleExample 简单的GN协议示例
func SimpleExample() {
	fmt.Println("=== GN协议简单示例 ===")

	// 1. 创建路由器和默认处理器
	fmt.Println("\n1. 创建协议处理器...")
	handler := gn.NewDefaultHandler()
	router := gn.NewRouter(handler)

	// 2. 测试帧解析
	fmt.Println("\n2. 测试帧格式解析...")

	// 使用文档中的真实心跳示例
	heartbeatHex := "fcff0018000000000000008220052000486920200730164545a7fcee"
	heartbeatData, err := hex.DecodeString(heartbeatHex)
	if err != nil {
		log.Fatalf("解析十六进制失败: %v", err)
	}

	frame, err := gn.ParseFrame(heartbeatData)
	if err != nil {
		fmt.Printf("帧解析失败: %v\n", err)
	} else {
		fmt.Printf("✓ 帧解析成功:\n")
		fmt.Printf("  命令: 0x%04X\n", frame.Command)
		fmt.Printf("  序列号: 0x%08X\n", frame.Sequence)
		fmt.Printf("  网关ID: %s\n", frame.GetGatewayIDHex())
		fmt.Printf("  载荷: %s\n", hex.EncodeToString(frame.Payload))
		fmt.Printf("  方向: %s\n", map[bool]string{true: "下行", false: "上行"}[frame.IsDownlink()])
	}

	// 3. 测试路由
	fmt.Println("\n3. 测试消息路由...")
	ctx := context.Background()
	err = router.Route(ctx, heartbeatData)
	if err != nil {
		fmt.Printf("路由处理失败: %v\n", err)
	} else {
		fmt.Println("✓ 消息路由成功")
	}

	// 4. 测试TLV解析
	fmt.Println("\n4. 测试TLV解析...")

	// 创建一个简单的TLV数据
	tlvs := gn.TLVList{
		gn.NewTLVUint8(gn.TagSocketNumber, 1), // 插座编号
		gn.NewTLVUint8(gn.TagTemperature, 37), // 温度
		gn.NewTLVUint16(gn.TagVoltage, 2275),  // 电压 227.5V
		gn.NewTLVString(0x99, "test_data"),    // 测试字符串
	}

	encoded := gn.EncodeTLVs(tlvs)
	fmt.Printf("编码的TLV数据: %s\n", hex.EncodeToString(encoded))

	decoded, err := gn.ParseTLVs(encoded)
	if err != nil {
		fmt.Printf("TLV解析失败: %v\n", err)
	} else {
		fmt.Printf("✓ TLV解析成功，解析出 %d 个字段:\n", len(decoded))
		for i, tlv := range decoded {
			fmt.Printf("  字段%d: 标签=0x%02X, 长度=%d, 值=%s\n",
				i+1, tlv.Tag, tlv.Length, hex.EncodeToString(tlv.Value))
		}
	}

	// 5. 测试帧编码
	fmt.Println("\n5. 测试帧编码...")

	gwid, _ := hex.DecodeString("82200520004869")
	payload := []byte("test_response")

	// 创建一个下行响应帧
	responseFrame, err := gn.NewFrame(gn.CmdHeartbeat, 0x12345678, gwid, payload, true)
	if err != nil {
		fmt.Printf("创建帧失败: %v\n", err)
	} else {
		encodedFrame, err := responseFrame.Encode()
		if err != nil {
			fmt.Printf("编码帧失败: %v\n", err)
		} else {
			fmt.Printf("✓ 帧编码成功: %s\n", hex.EncodeToString(encodedFrame))

			// 验证往返编解码
			decodedFrame, err := gn.ParseFrame(encodedFrame)
			if err != nil {
				fmt.Printf("往返解码失败: %v\n", err)
			} else {
				fmt.Printf("✓ 往返编解码验证成功\n")
				fmt.Printf("  原始载荷: %s\n", hex.EncodeToString(payload))
				fmt.Printf("  解码载荷: %s\n", hex.EncodeToString(decodedFrame.Payload))
			}
		}
	}

	// 6. 测试心跳解析
	fmt.Println("\n6. 测试心跳解析...")

	// 构造心跳载荷：ICCID + 固件版本 + RSSI
	heartbeatPayload := []byte{
		0x38, 0x39, 0x38, 0x36, 0x30, 0x34, 0x36, 0x33, 0x31, // ICCID前9字节
		0x31, 0x32, 0x30, 0x37, 0x30, 0x33, 0x31, 0x39, 0x34, // ICCID后9字节
		0x63, 0x56, 0x2e, 0x31, 0x72, 0x34, 0x36, // 固件版本 "cV.1r46"
		0x1f, // RSSI = 31
	}

	iccid, rssi, fwVer, err := gn.ParseHeartbeat(heartbeatPayload)
	if err != nil {
		fmt.Printf("心跳解析失败: %v\n", err)
	} else {
		fmt.Printf("✓ 心跳解析成功:\n")
		fmt.Printf("  ICCID: %s\n", iccid)
		fmt.Printf("  RSSI: %d\n", rssi)
		fmt.Printf("  固件版本: %s\n", fwVer)
	}

	// 7. 测试时间同步构建
	fmt.Println("\n7. 测试时间同步...")

	timeSyncPayload := gn.BuildTimeSync()
	fmt.Printf("时间同步载荷: %s\n", string(timeSyncPayload))

	fmt.Println("\n=== 示例完成 ===")
}

func main() {
	SimpleExample()
}
