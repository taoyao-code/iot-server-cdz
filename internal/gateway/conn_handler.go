package gateway

import (
	"context"
	"fmt"
	"time"

	cfgpkg "github.com/taoyao-code/iot-server/internal/config"
	"github.com/taoyao-code/iot-server/internal/metrics"
	"github.com/taoyao-code/iot-server/internal/protocol/adapter"
	"github.com/taoyao-code/iot-server/internal/protocol/ap3000"
	"github.com/taoyao-code/iot-server/internal/protocol/bkv"
	"github.com/taoyao-code/iot-server/internal/session"
	"github.com/taoyao-code/iot-server/internal/tcpserver"
	"go.uber.org/zap"
)

// NewConnHandler 构建 TCP 连接处理器，完成协议识别、会话绑定与指标上报。
// 通过 getHandlers 延迟获取 AP3000 处理集合，以便在 DB 初始化后赋值。
// P0完成: 支持接口类型以兼容内存和Redis会话管理器
func NewConnHandler(
	protocols cfgpkg.ProtocolsConfig,
	sess session.SessionManager,
	policy session.WeightedPolicy,
	appm *metrics.AppMetrics,
	getAP3000Handlers func() *ap3000.Handlers,
	getBKVHandlers func() *bkv.Handlers,
) func(*tcpserver.ConnContext) {
	return func(cc *tcpserver.ConnContext) {
		var adapters []adapter.Adapter
		var apAdapter *ap3000.Adapter
		if protocols.EnableAP3000 {
			apAdapter = ap3000.NewAdapter()
			adapters = append(adapters, apAdapter)
		}
		var bkvAdapter *bkv.Adapter
		if protocols.EnableBKV {
			bkvAdapter = bkv.NewAdapter()
			if cc.Server() != nil && cc.Server().GetLogger() != nil {
				bkvAdapter.SetLogger(cc.Server().GetLogger())
			}
			// 设置checksum错误指标回调
			if appm != nil {
				bkvAdapter.SetChecksumErrorFunc(func() {
					appm.ProtocolChecksumErrorTotal.WithLabelValues("bkv").Inc()
				})
			}
			adapters = append(adapters, bkvAdapter)
		}

		var boundPhy string
		bindIfNeeded := func(phy string) {
			if boundPhy != phy {
				boundPhy = phy
				sess.Bind(phy, cc)
			}
		}

		// AP3000 路由
		if apAdapter != nil {
			type apRoute struct {
				cmd     byte
				handler func(*ap3000.Handlers, context.Context, *ap3000.Frame) error
				withHB  bool
			}
			apRoutes := []apRoute{
				{0x20, func(h *ap3000.Handlers, ctx context.Context, f *ap3000.Frame) error { return h.HandleRegister(ctx, f) }, true},
				{0x21, func(h *ap3000.Handlers, ctx context.Context, f *ap3000.Frame) error { return h.HandleHeartbeat(ctx, f) }, true},
				{0x22, func(h *ap3000.Handlers, ctx context.Context, f *ap3000.Frame) error { return h.HandleGeneric(ctx, f) }, false},
				{0x12, func(h *ap3000.Handlers, ctx context.Context, f *ap3000.Frame) error { return h.HandleGeneric(ctx, f) }, false},
				{0x82, func(h *ap3000.Handlers, ctx context.Context, f *ap3000.Frame) error { return h.Handle82Ack(ctx, f) }, false},
				{0x03, func(h *ap3000.Handlers, ctx context.Context, f *ap3000.Frame) error { return h.Handle03(ctx, f) }, false},
				{0x06, func(h *ap3000.Handlers, ctx context.Context, f *ap3000.Frame) error { return h.Handle06(ctx, f) }, false},
			}
			for _, route := range apRoutes {
				sub := route
				apAdapter.Register(sub.cmd, func(f *ap3000.Frame) error {
					handlerSet := getAP3000Handlers()
					if handlerSet == nil {
						return nil
					}
					if sub.withHB {
						bindIfNeeded(f.PhyID)
						sess.OnHeartbeat(f.PhyID, time.Now())
					}
					if appm != nil {
						if sub.withHB {
							appm.HeartbeatTotal.Inc()
							appm.OnlineGauge.Set(float64(sess.OnlineCountWeighted(time.Now(), policy)))
						}
						appm.AP3000RouteTotal.WithLabelValues(fmt.Sprintf("%02X", f.Cmd)).Inc()
					}
					return sub.handler(handlerSet, context.Background(), f)
				})
			}
		}

		// BKV 路由 - 完整注册所有BKV协议命令
		if bkvAdapter != nil {
			wrapBKVHandler := func(handlerFunc func(context.Context, *bkv.Frame) error) func(*bkv.Frame) error {
				return func(f *bkv.Frame) error {
					log := cc.Server()
					if log != nil && log.GetLogger() != nil {
						log.GetLogger().Info("BKV frame received",
							zap.String("cmd", fmt.Sprintf("0x%04X", f.Cmd)),
							zap.String("gateway_id", f.GatewayID),
							zap.Uint32("msg_id", f.MsgID),
							zap.Int("data_len", len(f.Data)),
							zap.String("remote_addr", cc.RemoteAddr().String()),
						)
					}
					if appm != nil {
						appm.BKVRouteTotal.WithLabelValues(fmt.Sprintf("%04X", f.Cmd)).Inc()
					}
					bindIfNeeded(f.GatewayID)
					bh := getBKVHandlers()
					if bh == nil {
						return nil
					}
					err := handlerFunc(context.Background(), f)
					if err != nil && log != nil && log.GetLogger() != nil {
						log.GetLogger().Error("BKV handler error",
							zap.String("cmd", fmt.Sprintf("0x%04X", f.Cmd)),
							zap.String("gateway_id", f.GatewayID),
							zap.Error(err),
						)
					}
					return err
				}
			}

			type route struct {
				cmd     uint16
				handler func(context.Context, *bkv.Frame) error
			}

			routes := []route{
				{0x0000, func(ctx context.Context, f *bkv.Frame) error {
					sess.OnHeartbeat(f.GatewayID, time.Now())
					if appm != nil {
						appm.HeartbeatTotal.Inc()
						appm.OnlineGauge.Set(float64(sess.OnlineCountWeighted(time.Now(), policy)))
					}
					return getBKVHandlers().HandleHeartbeat(ctx, f)
				}},
				{0x1000, func(ctx context.Context, f *bkv.Frame) error { return getBKVHandlers().HandleBKVStatus(ctx, f) }},
				{0x0015, func(ctx context.Context, f *bkv.Frame) error { return getBKVHandlers().HandleControl(ctx, f) }},
				{0x0005, func(ctx context.Context, f *bkv.Frame) error { return getBKVHandlers().HandleNetworkList(ctx, f) }},
				{0x0008, func(ctx context.Context, f *bkv.Frame) error { return getBKVHandlers().HandleNetworkRefresh(ctx, f) }},
				{0x0009, func(ctx context.Context, f *bkv.Frame) error { return getBKVHandlers().HandleNetworkAddNode(ctx, f) }},
				{0x000A, func(ctx context.Context, f *bkv.Frame) error { return getBKVHandlers().HandleNetworkDeleteNode(ctx, f) }},
			}

			extraRoutes := []func(){
				func() {
					bkvAdapter.Register(0x0007, wrapBKVHandler(func(ctx context.Context, f *bkv.Frame) error {
						if len(f.Data) >= 3 && len(f.Data) <= 10 {
							return getBKVHandlers().HandleOTAResponse(ctx, f)
						}
						if len(f.Data) >= 4 {
							return getBKVHandlers().HandleOTAProgress(ctx, f)
						}
						return getBKVHandlers().HandleGeneric(ctx, f)
					}))
				},
			}

			for _, r := range routes {
				bkvAdapter.Register(r.cmd, wrapBKVHandler(r.handler))
			}
			for _, register := range extraRoutes {
				register()
			}

			// 其余命令保持原来的专用 handler 注册方式...
		}

		mux := tcpserver.NewMux(adapters...)
		mux.SetServer(cc.Server()) // 设置server引用以支持日志
		mux.BindToConn(cc)

		go func() {
			<-cc.Done()
			if boundPhy != "" {
				sess.UnbindByPhy(boundPhy)
				sess.OnTCPClosed(boundPhy, time.Now())
				if appm != nil && appm.SessionOfflineTotal != nil {
					appm.SessionOfflineTotal.WithLabelValues("tcp").Inc()
				}
			}
		}()
	}
}
