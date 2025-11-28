# Gateway Module - 协议网关

> **导航**: [← 返回根目录](../../CLAUDE.md)
> **路径**: `internal/gateway/`
> **最后更新**: 2025-11-28

---

## 📋 模块职责

Gateway 模块是 TCP 连接的协议路由层：

- **连接接入**: 接受 TCP 连接
- **协议识别**: 根据端口或首帧识别协议类型
- **路由分发**: 将连接路由到对应协议处理器
- **会话注册**: 绑定连接到 Session Manager

---

## 📂 文件结构

```
gateway/
└── conn_handler.go    # 连接处理器
```

---

## 🔑 核心逻辑

### 协议路由

```go
type ConnHandler struct {
    sessionMgr session.Manager
    handlers   map[string]ProtocolHandler
}

func (ch *ConnHandler) Handle(conn net.Conn) error {
    // 1. 读取首帧识别协议
    protocol := detectProtocol(conn)

    // 2. 获取协议处理器
    handler := ch.handlers[protocol]

    // 3. 注册会话
    phyID := handler.ExtractPhyID(conn)
    ch.sessionMgr.Bind(phyID, conn)

    // 4. 处理连接
    return handler.Handle(conn)
}
```

### 协议识别

```go
func detectProtocol(conn net.Conn) string {
    // 方式1: 根据端口
    addr := conn.LocalAddr().(*net.TCPAddr)
    switch addr.Port {
    case 7011:
        return "ap3000"
    case 7012:
        return "bkv"
    case 7013:
        return "gn"
    }

    // 方式2: 读取首帧
    buf := make([]byte, 4)
    conn.Read(buf)
    if buf[0] == 0x68 {
        return "ap3000"
    }
    return "unknown"
}
```

---

## 🔗 相关文档

- [TCP Server](../tcpserver/CLAUDE.md)
- [Protocol Module](../protocol/CLAUDE.md)
- [Session Module](../session/CLAUDE.md)

---

**最后更新**: 2025-11-28
