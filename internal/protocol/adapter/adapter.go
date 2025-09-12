package adapter

// Adapter 统一协议适配器接口：用于网关复用器绑定
// 要求：
// - Sniff 用于首帧初判
// - ProcessBytes 处理来自连接的原始字节流（内部负责半包/粘包）
type Adapter interface {
	Sniff(prefix []byte) bool
	ProcessBytes(p []byte) error
}
