package main

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/taoyao-code/iot-server/internal/protocol/bkv"
)

var errNeedMoreData = errors.New("need more data")

// networkNode 组网插座配置
type networkNode struct {
	Slot uint8   // 插座编号 1-250
	MAC  [6]byte // 插座MAC原始字节
}

// 配置结构体（保持最小化，仅监听地址和基础组网/控制参数）
type serverConfig struct {
	Addr         string
	AutoControl  bool
	PowerControl bool          // 是否自动下发按功率充电命令(0x17)
	NetChannel   uint8         // 组网信道(1-15)
	NetNodes     []networkNode // 需要下发的插座列表（对所有网关生效）
}

func main() {
	cfg := loadConfig()

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Printf("BKV 最小协议测试服务启动，监听地址: %s", cfg.Addr)

	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		log.Fatalf("监听失败: %v", err)
	}
	defer ln.Close()

	log.Printf("自动下发充电控制命令: %v", cfg.AutoControl)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("接受连接失败: %v", err)
			continue
		}
		go handleConn(conn, cfg)
	}
}

// loadConfig 加载最小配置，优先环境变量，其次命令行参数
func loadConfig() serverConfig {
	defaultAddr := getEnv("BKV_TEST_ADDR", ":7065")
	// 默认开启自动控制（无需显式设置环境变量或命令行参数）
	autoFromEnv := parseBoolEnv("BKV_TEST_AUTO_CONTROL", true)
	powerFromEnv := parseBoolEnv("BKV_TEST_POWER_CONTROL", false)

	addrFlag := flag.String("addr", defaultAddr, "TCP 监听地址，例如 :7065")
	autoFlag := flag.Bool("auto-control", autoFromEnv, "收到心跳后自动下发一次开始充电命令")
	powerFlag := flag.Bool("power-control", powerFromEnv, "收到心跳后自动下发一次按功率充电命令")
	netConfigFlag := flag.String("net-config", "network_config.json", "组网配置文件路径(JSON)，例如 ./network_config.json")
	flag.Parse()

	netChannel, netNodes := loadNetworkConfig(*netConfigFlag)

	return serverConfig{
		Addr:         *addrFlag,
		AutoControl:  *autoFlag,
		PowerControl: *powerFlag,
		NetChannel:   netChannel,
		NetNodes:     netNodes,
	}
}

func getEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func parseBoolEnv(key string, def bool) bool {
	val, ok := os.LookupEnv(key)
	if !ok || val == "" {
		return def
	}
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return def
	}
}

// parseMAC 支持纯HEX或带分隔符的MAC字符串
func parseMAC(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errors.New("empty mac")
	}

	// 去掉常见分隔符
	s = strings.ReplaceAll(s, ":", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ToLower(s)

	if len(s) != 12 {
		return nil, errors.New("mac hex 长度必须为12")
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, err
	}
	if len(b) != 6 {
		return nil, errors.New("mac 字节长度必须为6")
	}
	return b, nil
}

// networkConfigFile 组网配置文件格式
type networkConfigFile struct {
	Channel uint8          `json:"channel"` // 信道(1-15)
	Sockets []socketConfig `json:"sockets"` // 插座列表
}

// socketConfig 单个插座配置
type socketConfig struct {
	Slot uint8  `json:"slot"` // 插座编号 1-250
	MAC  string `json:"mac"`  // 插座MAC，例如 "854121800889"
}

// loadNetworkConfig 从JSON文件加载组网配置（信道 + 插座列表）
// 示例文件内容：
//
//	{
//	  "channel": 4,
//	  "sockets": [
//	    { "slot": 1, "mac": "854121800889" }
//	  ]
//	}
func loadNetworkConfig(path string) (uint8, []networkNode) {
	path = strings.TrimSpace(path)
	if path == "" {
		return 0, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("未找到组网配置文件 %s，跳过组网逻辑: %v", path, err)
		return 0, nil
	}

	var fileCfg networkConfigFile
	if err := json.Unmarshal(data, &fileCfg); err != nil {
		log.Printf("解析组网配置文件失败(%s): %v", path, err)
		return 0, nil
	}

	// 信道检查
	channel := fileCfg.Channel
	if channel < 1 || channel > 15 {
		if channel != 0 {
			log.Printf("组网配置文件 %s 中信道无效(%d)，使用默认4", path, channel)
		}
		channel = 4
	}

	var nodes []networkNode
	for _, s := range fileCfg.Sockets {
		if s.Slot < 1 || s.Slot > 250 {
			log.Printf("忽略无效插座号: %d (必须在1-250)", s.Slot)
			continue
		}

		macBytes, err := parseMAC(s.MAC)
		if err != nil {
			log.Printf("忽略插座%d的无效MAC(%q): %v", s.Slot, s.MAC, err)
			continue
		}

		var macArr [6]byte
		copy(macArr[:], macBytes)

		nodes = append(nodes, networkNode{
			Slot: s.Slot,
			MAC:  macArr,
		})
	}

	if len(nodes) == 0 {
		log.Printf("组网配置文件 %s 中没有有效插座配置，跳过组网下发", path)
		return channel, nil
	}

	log.Printf("已加载组网配置: 信道=%d, 插座数量=%d", channel, len(nodes))
	return channel, nodes
}

// handleConn 处理单个设备连接，负责拆帧、日志和最小应答
func handleConn(conn net.Conn, cfg serverConfig) {
	defer conn.Close()

	remote := conn.RemoteAddr().String()
	log.Printf("新连接: %s", remote)
	defer log.Printf("连接关闭: %s", remote)

	buf := make([]byte, 4096)
	var pending []byte
	sentControl := false      // 是否已下发按时/按量控制命令(0x07)
	sentPowerControl := false // 是否已下发按功率控制命令(0x17)
	netSent := false          // 是否已下发组网刷新命令
	netAcked := false         // 是否已收到组网刷新ACK
	querySent := false        // 是否已发送查询插座状态命令(0x1D)

	for {
		n, err := conn.Read(buf)
		if err != nil {
			log.Printf("读取失败(%s): %v", remote, err)
			return
		}
		if n == 0 {
			continue
		}

		pending = append(pending, buf[:n]...)

		for {
			frameBytes, rest, err := nextFrame(pending)
			if err != nil {
				if errors.Is(err, errNeedMoreData) {
					// 需要更多数据，跳出内层循环等待下一次读取
					break
				}
				// 协议错误时尝试跳过一个字节重新同步
				log.Printf("解析原始帧失败(%s): %v，尝试丢弃1字节重新同步", remote, err)
				if len(pending) > 0 {
					pending = pending[1:]
				} else {
					pending = nil
				}
				break
			}

			pending = rest

			// 使用现有解析器解析帧
			frame, err := bkv.Parse(frameBytes)
			if err != nil {
				log.Printf("BKV 解析失败(%s): %v", remote, err)
				continue
			}

			logFrame("RX", remote, frameBytes, frame)

			// 若配置了组网节点，在首次心跳后下发一次2.2.5组网“刷新列表”命令
			if !netSent && len(cfg.NetNodes) > 0 && frame.IsHeartbeat() {
				netCmd := buildNetworkRefresh(frame, cfg.NetChannel, cfg.NetNodes)
				if len(netCmd) > 0 {
					if _, err := conn.Write(netCmd); err != nil {
						log.Printf("发送组网刷新命令失败(%s): %v", remote, err)
						return
					}
					if netFrame, err := bkv.Parse(netCmd); err == nil {
						logFrame("TX", remote, netCmd, netFrame)
					} else {
						log.Printf("TX(%s) 组网帧解析失败: %v raw=%s", remote, err, hex.EncodeToString(netCmd))
					}
				}
				netSent = true
			}

			// 识别设备对2.2.5组网刷新的ACK
			if frame.Cmd == 0x0005 && frame.IsUplink() && isNetworkRefreshAck(frame) {
				netAcked = true
				log.Printf("收到组网刷新ACK(%s)", remote)

				// 在组网成功后，平台主动下发一次“查询插座状态”命令 (0x0015 子命令0x1D)
				if !querySent {
					queryCmd := buildQuerySocketStatus(frame, cfg.NetNodes)
					if len(queryCmd) > 0 {
						if _, err := conn.Write(queryCmd); err != nil {
							log.Printf("发送查询插座状态命令失败(%s): %v", remote, err)
							return
						}
						if qFrame, err := bkv.Parse(queryCmd); err == nil {
							logFrame("TX", remote, queryCmd, qFrame)
						} else {
							log.Printf("TX(%s) 查询插座状态帧解析失败: %v raw=%s", remote, err, hex.EncodeToString(queryCmd))
						}
					}
					querySent = true
				}
			}

			// 识别设备对查询插座状态(0x0015 子命令0x1D)的回复，仅做解析日志
			if frame.Cmd == 0x0015 && frame.IsUplink() && isSocketStatusQueryResponse(frame) {
				logSocketStatusQueryResponse(remote, frame)
			}

			// 若启用自动控制，在组网完成后首次心跳时下发一次开始充电命令
			if cfg.AutoControl && !sentControl && frame.IsHeartbeat() && (len(cfg.NetNodes) == 0 || netAcked) {
				ctrl := buildStartChargeCommand(frame, cfg.NetNodes)
				if _, err := conn.Write(ctrl); err != nil {
					log.Printf("发送开始充电命令失败(%s): %v", remote, err)
					return
				}
				if ctrlFrame, err := bkv.Parse(ctrl); err == nil {
					logFrame("TX", remote, ctrl, ctrlFrame)
				} else {
					log.Printf("TX(%s) 控制帧解析失败: %v raw=%s", remote, err, hex.EncodeToString(ctrl))
				}
				sentControl = true
			}

			// 若启用按功率控制，在组网完成后首次心跳时下发一次按功率充电命令 (cmd=0x0015, 子命令0x17)
			if cfg.PowerControl && !sentPowerControl && frame.IsHeartbeat() && (len(cfg.NetNodes) == 0 || netAcked) {
				powerCmd := buildPowerLevelChargeCommand(frame, cfg.NetNodes)
				if len(powerCmd) > 0 {
					if _, err := conn.Write(powerCmd); err != nil {
						log.Printf("发送按功率充电命令失败(%s): %v", remote, err)
						return
					}
					if pFrame, err := bkv.Parse(powerCmd); err == nil {
						logFrame("TX", remote, powerCmd, pFrame)
					} else {
						log.Printf("TX(%s) 按功率控制帧解析失败: %v raw=%s", remote, err, hex.EncodeToString(powerCmd))
					}
					sentPowerControl = true
				}
			}

			// 根据协议文档实现最小应答逻辑
			// 其中包含对按功率充电结束上报(0x18)的解析
			if frame.Cmd == 0x0015 && frame.IsUplink() && len(frame.Data) >= 3 && frame.Data[2] == 0x18 {
				logPowerLevelChargingEnd(remote, frame)
			}

			replies := buildReplies(frame)
			for _, reply := range replies {
				if _, err := conn.Write(reply); err != nil {
					log.Printf("发送应答失败(%s): %v", remote, err)
					return
				}

				if replyFrame, err := bkv.Parse(reply); err == nil {
					logFrame("TX", remote, reply, replyFrame)
				} else {
					log.Printf("TX(%s) 原始帧解析失败: %v raw=%s", remote, err, hex.EncodeToString(reply))
				}
			}
		}
	}
}

// nextFrame 从缓冲区中提取下一帧原始数据，处理粘包/半包
func nextFrame(buf []byte) ([]byte, []byte, error) {
	if len(buf) < 4 {
		return nil, buf, errNeedMoreData
	}

	start := findFrameStart(buf)
	if start == -1 {
		return nil, nil, errors.New("未找到有效包头")
	}

	if start > 0 {
		buf = buf[start:]
	}

	if len(buf) < 4 {
		return nil, buf, errNeedMoreData
	}

	dataLen := binary.BigEndian.Uint16(buf[2:4])
	if dataLen < 2 {
		return nil, buf[1:], errors.New("长度字段无效")
	}

	totalLen := 4 + int(dataLen)
	if len(buf) < totalLen {
		return nil, buf, errNeedMoreData
	}

	frameBytes := make([]byte, totalLen)
	copy(frameBytes, buf[:totalLen])
	rest := buf[totalLen:]

	return frameBytes, rest, nil
}

// findFrameStart 查找下一个有效包头
func findFrameStart(buf []byte) int {
	for i := 0; i < len(buf)-1; i++ {
		if (buf[i] == 0xFC && buf[i+1] == 0xFE) || (buf[i] == 0xFC && buf[i+1] == 0xFF) {
			return i
		}
	}
	return -1
}

// buildReplies 根据上行帧构造最小必要的下行应答
func buildReplies(frame *bkv.Frame) [][]byte {
	var replies [][]byte

	switch frame.Cmd {
	case 0x0000:
		// 2.1.1 心跳上报/回复
		replies = append(replies, buildHeartbeatReply(frame))
	case 0x0015:
		// 2.2.9 充电结束上报（按时/按电量）
		if isChargingEnd(frame) {
			replies = append(replies, buildChargingEndAck(frame))
		}
	case 0x1000:
		// 0x1000 + BKV 子协议的最小ACK集合
		payload, err := frame.GetBKVPayload()
		if err != nil {
			break
		}

		targetGW := payload.GatewayID
		if targetGW == "" {
			targetGW = frame.GatewayID
		}
		if targetGW == "" {
			break
		}

		// 2.2.3 状态上报 ACK（cmd=0x1017）
		if payload.IsStatusReport() {
			data, err := bkv.EncodeBKVStatusAck(payload, true)
			if err != nil || len(data) == 0 {
				// 状态ACK失败不影响其他ACK
			} else {
				replies = append(replies, bkv.Build(0x1000, frame.MsgID, targetGW, data))
			}
		}

		// 按电费/服务费模式 & BKV充电结束上报 (cmd=0x1004)
		if payload.IsChargingEnd() {
			var socketPtr, portPtr *int
			for _, f := range payload.Fields {
				switch f.Tag {
				case 0x4A: // 插座号
					if len(f.Value) >= 1 {
						v := int(f.Value[0])
						socketPtr = &v
					}
				case 0x08: // 插孔号
					if len(f.Value) >= 1 {
						v := int(f.Value[0])
						portPtr = &v
					}
				}
			}
			data, err := bkv.EncodeBKVChargingEndAck(payload, socketPtr, portPtr, true)
			if err == nil && len(data) > 0 {
				replies = append(replies, bkv.Build(0x1000, frame.MsgID, targetGW, data))
			}
		}

		// 异常事件上报 (cmd=0x1010)
		if payload.IsExceptionReport() {
			// 解析异常事件以获取插座号（可选）
			event, err := bkv.ParseBKVExceptionEvent(payload)
			var socketPtr *int
			if err == nil && event != nil && event.SocketNo > 0 {
				v := int(event.SocketNo)
				socketPtr = &v
			}
			data, err := bkv.EncodeBKVExceptionAck(payload, socketPtr, true)
			if err == nil && len(data) > 0 {
				replies = append(replies, bkv.Build(0x1000, frame.MsgID, targetGW, data))
			}
		}
	}

	return replies
}

// buildHeartbeatReply 构造心跳应答帧（cmd=0x0000）
func buildHeartbeatReply(frame *bkv.Frame) []byte {
	// 按文档使用当前时间生成7字节 BCD 时间戳 YYYYMMDDHHMMSS
	ts := time.Now().In(time.FixedZone("CST", 8*3600))
	data := encodeTimestampBCD(ts)

	return bkv.Build(0x0000, frame.MsgID, frame.GatewayID, data)
}

// isChargingEnd 判断是否为 2.2.9 充电结束上报帧
func isChargingEnd(frame *bkv.Frame) bool {
	data := frame.Data
	if len(data) < 3 {
		return false
	}
	// 数据格式: [长度高][长度低][子命令]...[其他字段]
	// 子命令:
	//   0x02 - 按时/按电量充电结束上报 (2.2.9)
	//   0x18 - 按功率充电结束上报 (2.2.2)
	return data[2] == 0x02 || data[2] == 0x18
}

// buildPowerLevelChargeCommand 构造一次“按功率下发充电命令” (cmd=0x0015, 子命令0x17)
// 对标 2.2.1 按功率下发充电命令，做最小化实现：单档功率 + 支付金额
func buildPowerLevelChargeCommand(frame *bkv.Frame, nodes []networkNode) []byte {
	// 默认插座号1，如配置了组网节点则使用首个节点
	socketNo := uint8(1)
	if len(nodes) > 0 && nodes[0].Slot >= 1 && nodes[0].Slot <= 250 {
		socketNo = nodes[0].Slot
	}

	const (
		portNo       = uint8(0)     // A孔
		switchOn     = uint8(0x01)  // 开
		paymentCents = uint16(100)  // 支付金额: 1元(100分)
		levelCount   = uint8(1)     // 单档
		levelPowerW  = uint16(2000) // 2000W
		levelPrice   = uint16(25)   // 0.25元/度(25分)
		levelMinutes = uint16(60)   // 60分钟
	)

	// 内层payload: [lenH][lenL][0x17][插座号][插孔号][开关][支付金额2B][档位个数][档位1(功率2+单价2+时长2)]
	innerLen := 1 + 1 + 1 + 1 + 2 + 1 + 6 // cmd+socket+port+switch+payment+count+1level
	data := make([]byte, 2+innerLen)

	data[0] = byte(innerLen >> 8)
	data[1] = byte(innerLen)
	data[2] = 0x17
	data[3] = socketNo
	data[4] = portNo
	data[5] = switchOn
	data[6] = byte(paymentCents >> 8)
	data[7] = byte(paymentCents)
	data[8] = levelCount

	// 档位1
	offset := 9
	data[offset] = byte(levelPowerW >> 8)
	data[offset+1] = byte(levelPowerW & 0xFF)
	data[offset+2] = byte(levelPrice >> 8)
	data[offset+3] = byte(levelPrice)
	data[offset+4] = byte(levelMinutes >> 8)
	data[offset+5] = byte(levelMinutes)

	msgID := uint32(time.Now().Unix() & 0xffffffff)
	return bkv.Build(0x0015, msgID, frame.GatewayID, data)
}

// buildQuerySocketStatus 构造一次“查询插座状态”命令 (cmd=0x0015, 子命令0x1D)
// 对标 2.2.4 查询插座状态：fcff00150015001c91ee008600445945300500011D0181fcee
func buildQuerySocketStatus(frame *bkv.Frame, nodes []networkNode) []byte {
	// 默认查询插座1；如已配置组网，则优先使用首个节点的插座号
	socketNo := uint8(1)
	if len(nodes) > 0 && nodes[0].Slot >= 1 && nodes[0].Slot <= 250 {
		socketNo = nodes[0].Slot
	}

	// 内层payload: [长度高][长度低][子命令0x1D][插座号]
	data := []byte{0x00, 0x01, 0x1D, socketNo}

	msgID := uint32(time.Now().Unix() & 0xffffffff)
	return bkv.Build(0x0015, msgID, frame.GatewayID, data)
}

// isSocketStatusQueryResponse 判断是否为“查询插座状态”回复帧
// 对标 2.2.4 设备回复：data形如 0021 1D 01 ...
func isSocketStatusQueryResponse(frame *bkv.Frame) bool {
	d := frame.Data
	if len(d) < 4 {
		return false
	}
	// [长度高][长度低][子命令0x1D][插座号]...
	return d[2] == 0x1D
}

// logSocketStatusQueryResponse 解析并记录插座状态查询回复的关键字段
func logSocketStatusQueryResponse(remote string, frame *bkv.Frame) {
	d := frame.Data
	socketNo := d[3]
	rawHex := strings.ToLower(hex.EncodeToString(d))
	log.Printf("收到插座状态查询回复(%s): 插座=%d payload=%s", remote, socketNo, rawHex)
}

// logPowerLevelChargingEnd 解析并记录按功率充电结束上报(0x0015, 子命令0x18)的关键字段
func logPowerLevelChargingEnd(remote string, frame *bkv.Frame) {
	end, err := bkv.ParseBKVChargingEnd(frame.Data)
	if err != nil {
		log.Printf("解析按功率充电结束上报失败(%s): %v raw=%s", remote, err, hex.EncodeToString(frame.Data))
		return
	}

	energyKWh := float64(end.EnergyUsed) / 100.0
	amountYuan := float64(end.AmountSpent) / 100.0
	settlePowerW := float64(end.SettlingPower) / 10.0

	log.Printf("按功率充电结束(%s): 插座=%d 插孔=%d 业务号=0x%04X 总电量=%.2fkWh 结算功率=%.1fW 总金额=%.2f元 结束原因=%d",
		remote,
		end.SocketNo,
		int(end.Port),
		end.BusinessNo,
		energyKWh,
		settlePowerW,
		amountYuan,
		int(end.EndReason),
	)
}

// buildChargingEndAck 构造充电结束确认帧
// 对应 docs/协议/设备对接指引 和 issues/BKV协议数据格式详细规范.md 中的示例:
// 确认: fcff0016001500000000008600445945300500020c0101d8fcee
func buildChargingEndAck(frame *bkv.Frame) []byte {
	// 数据区结构: [长度高][长度低][子命令][结果][保留/扩展]
	// 这里保持与文档示例一致: 0002 0c 01 01
	data := []byte{0x00, 0x02, 0x0c, 0x01, 0x01}
	return bkv.Build(0x0015, frame.MsgID, frame.GatewayID, data)
}

// buildNetworkRefresh 构造2.2.5“下发网络节点列表-刷新列表”命令
// 对应 docs/协议/设备对接指引-组网设备2024(1).txt 和 internal/protocol/bkv/complete_examples_test.go
func buildNetworkRefresh(frame *bkv.Frame, channel uint8, nodes []networkNode) []byte {
	if len(nodes) == 0 {
		return nil
	}
	if channel < 1 || channel > 15 {
		channel = 4
	}

	// 数据格式:
	// [长度高][长度低][子命令0x08][信道][(插座号1B + MAC6B) * N]
	innerLen := 1 + len(nodes)*(1+6)
	data := make([]byte, 2+1+1+len(nodes)*(1+6))

	data[0] = byte(innerLen >> 8)
	data[1] = byte(innerLen)
	data[2] = 0x08    // 子命令: 0x08 刷新列表
	data[3] = channel // 信道

	offset := 4
	for _, n := range nodes {
		data[offset] = n.Slot
		offset++
		copy(data[offset:offset+6], n.MAC[:])
		offset += 6
	}

	msgID := uint32(time.Now().Unix() & 0xffffffff)
	return bkv.Build(0x0005, msgID, frame.GatewayID, data)
}

// encodeTimestampBCD 按协议将时间编码为 7 字节 BCD: YYYYMMDDHHMMSS
func encodeTimestampBCD(t time.Time) []byte {
	year := t.Year()
	yy1 := year / 100
	yy2 := year % 100

	return []byte{
		toBCD(yy1),
		toBCD(yy2),
		toBCD(int(t.Month())),
		toBCD(t.Day()),
		toBCD(t.Hour()),
		toBCD(t.Minute()),
		toBCD(t.Second()),
	}
}

func toBCD(v int) byte {
	if v < 0 {
		v = 0
	}
	if v > 99 {
		v = v % 100
	}
	hi := (v / 10) & 0x0F
	lo := (v % 10) & 0x0F
	return byte(hi<<4 | lo)
}

// logFrame 打印完整上下行协议日志（包含原始HEX和关键字段）
func logFrame(direction, remote string, raw []byte, frame *bkv.Frame) {
	rawHex := strings.ToLower(hex.EncodeToString(raw))

	log.Printf("[%s] %s cmd=0x%04x msgID=%08x gateway=%s len=%d raw=%s",
		direction,
		remote,
		frame.Cmd,
		frame.MsgID,
		frame.GatewayID,
		len(raw),
		rawHex,
	)
}

// buildStartChargeCommand 构造一次最小的“开始充电”下行命令 (cmd=0x0015, 子命令0x07)
// 参考 internal/api/thirdparty_handler.go encodeStartControlPayload 及 2.2.8 协议示例
func buildStartChargeCommand(frame *bkv.Frame, nodes []networkNode) []byte {
	// 默认插座号0（单机版）；如果提供了组网节点，则优先使用首个节点的插座号
	socketNo := uint8(0)
	if len(nodes) > 0 && nodes[0].Slot > 0 {
		socketNo = nodes[0].Slot
	}

	// A孔(端口0)，按时长模式，1分钟，业务号固定为1
	const (
		portNo         = uint8(0)
		modeTime       = uint8(1)
		durationMin    = uint16(1)
		defaultBizNo   = uint16(1)
		controlCommand = uint8(0x07)
	)

	// 内层payload: [0x07][插座][插孔][开关1][模式][时长2][业务号2]
	inner := make([]byte, 9)
	inner[0] = controlCommand
	inner[1] = socketNo
	inner[2] = portNo
	inner[3] = 0x01                   // 开关: 1=开启
	inner[4] = modeTime               // 模式: 1=按时长
	inner[5] = byte(durationMin >> 8) // 时长高字节
	inner[6] = byte(durationMin)      // 时长低字节
	inner[7] = byte(defaultBizNo >> 8)
	inner[8] = byte(defaultBizNo)

	// 外层payload: [长度2B][内层payload]，长度=参数字节数（不含命令字节）
	paramLen := len(inner) - 1
	payload := make([]byte, 2+len(inner))
	payload[0] = byte(paramLen >> 8)
	payload[1] = byte(paramLen)
	copy(payload[2:], inner)

	msgID := uint32(time.Now().Unix() & 0xffffffff)
	return bkv.Build(0x0015, msgID, frame.GatewayID, payload)
}

// isNetworkRefreshAck 判断是否为2.2.5组网刷新列表的设备ACK
// 参考 internal/protocol/bkv/complete_examples_test.go::Test_Cmd0005_Network_Minimal
func isNetworkRefreshAck(frame *bkv.Frame) bool {
	d := frame.Data
	if len(d) < 4 {
		return false
	}
	// [长度高][长度低][子命令0x08][结果]
	if d[0] != 0x00 { // 示例中长度为0x0001
		return false
	}
	if d[2] != 0x08 {
		return false
	}
	// d[3]==0x01 表示成功
	return d[3] == 0x01
}
