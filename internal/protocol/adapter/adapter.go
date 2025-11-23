package adapter

// Adapter 统一协议适配器接口：用于网关复用器绑定。
// 要求：
//   - Sniff 用于首帧初判，决定是否由当前适配器接管连接；
//   - ProcessBytes 处理来自连接的原始字节流（内部负责半包/粘包），
//     并将解析出的协议帧转换为中间件核心的状态更新和事件。
type Adapter interface {
	Sniff(prefix []byte) bool
	ProcessBytes(p []byte) error
}
