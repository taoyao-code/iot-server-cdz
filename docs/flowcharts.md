# 逻辑流程图总览

本文汇总核心链路的 Mermaid 流程图，覆盖启动、设备接入、第三方/刷卡充电、下行队列与事件推送。

## 服务启动流程

```mermaid
flowchart TD
    A[main] --> B[加载配置]
    B --> C[初始化日志]
    C --> D[bootstrap.Run]
    D --> E[指标+就绪探针]
    E --> F[Redis 客户端]
    F --> G[Redis Session 管理器]
    G --> H[DB 迁移+CoreRepo]
    H --> I[事件队列/去重/推送器]
    I --> J[Redis OutboundQueue + 适配器]
    J --> K[DriverCore]
    K --> L[协议处理器 AP3000/BKV]
    L --> M[HTTP 服务(API/健康/控制台)]
    M --> N[Workers: Outbound/Event/订单监控/端口同步/事件推送]
    N --> O[TCP 服务监听]
    O --> P[等待信号 优雅关闭+Redis 清理]
```

## TCP 接入与会话/心跳

```mermaid
flowchart TD
    A[TCP 连接建立] --> B[ConnHandler 协议探测]
    B -->|AP3000| C[ap3000.Adapter 路由]
    B -->|BKV| D[bkv.Adapter 路由]
    C --> E[Bind Session + 心跳 + 指标]
    D --> E
    E --> F[Session Redis 记录 last_seen/conn_id]
    F --> G[在线/加权在线统计]
    D --> H[BKV 命令分发 0x0000/0x1017/0x1000/0x0015...]
    H --> I[bkv.Handlers -> CoreEvents(DriverCore)]
    I --> J[CoreRepo 落库 devices/ports/orders]
    J --> K[第三方推送队列(可选)]
```

## 第三方启动充电链路

```mermaid
flowchart TD
    A[HTTP POST /third/devices/{phy}/charge] --> B[core.EnsureDevice]
    B --> C[Session.IsOnline 检查]
    C --> D[事务锁端口+创建订单 order_no/business_no]
    D --> E[driverCmd.SendCoreCommand -> BKV CommandSource]
    E --> F[编码 0x0015 启动命令 -> OutboundAdapter]
    F --> G[入 Redis OutboundQueue]
    G --> H[RedisWorker 取队列]
    H --> I[Session.GetConn 校验在线]
    I --> J[TCP 写下行帧 0x0015]
    J --> K[设备上报 控制结果/心跳/状态/进度]
    K --> L[bkv.Handlers -> CoreEvents -> DriverCore]
    L --> M[CoreRepo 更新订单/端口]
    M --> N[EventQueue 推送(可选)]
```

## 刷卡充电链路（设备侧发起）

```mermaid
flowchart TD
    A[设备 0x000B 刷卡上行] --> B[HandleCardSwipe -> CoreEvent SessionStarted(card_swipe)]
    B --> C[DriverCore 落库/推送]
    C --> D[CardService 生成充电指令(可选)]
    D --> E[0x000B 下发充电命令 -> OutboundQueue -> Worker -> TCP]
    E --> F[设备 0x000F 订单确认]
    F --> G[HandleOrderConfirm -> CoreEvent SessionStarted(order_confirm)]
    G --> H[充电中: 0x1017/0x1000 状态/进度]
    H --> I[DriverCore 更新订单/端口]
    I --> J[0x000C/0x1000 结束上报]
    J --> K[CoreEvent SessionEnded -> 结算订单/端口空闲/推送]
```

## 下行队列与 ACK

```mermaid
flowchart TD
    A[下行命令来源: 协议ACK/第三方API等] --> B[OutboundAdapter 构帧 bkv.Build]
    B --> C[EnsureDevice 获取 device_id]
    C --> D[入 Redis SortedSet OutboundQueue(优先级/超时)]
    D --> E[RedisWorker 轮询出队]
    E --> F[Session.GetConn 校验在线/心跳]
    F --> G[TCP 写命令]
    G --> H[等待 ACK/超时]
    H --> I[MarkSuccess 或 MarkFailed/重试/死信]
    I --> J[MarkSuccessByPhyMsgID 清理处理中索引(ACK 场景)]
```

## 事件推送链路

```mermaid
flowchart TD
    A[DriverCore 生成 CoreEvent] --> B[CoreRepo 落库一致性视图]
    B --> C[EventQueue(启用时) 入 Redis]
    C --> D[Event Worker 推送 Webhook(去重可选)]
```
