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
			apAdapter.Register(0x20, func(f *ap3000.Frame) error {
				bindIfNeeded(f.PhyID)
				sess.OnHeartbeat(f.PhyID, time.Now())
				if appm != nil {
					appm.HeartbeatTotal.Inc()
					appm.OnlineGauge.Set(float64(sess.OnlineCountWeighted(time.Now(), policy)))
					appm.AP3000RouteTotal.WithLabelValues(fmt.Sprintf("%02X", f.Cmd)).Inc()
				}
				handlerSet := getAP3000Handlers()
				if handlerSet == nil {
					return nil
				}
				return handlerSet.HandleRegister(context.Background(), f)
			})
			apAdapter.Register(0x21, func(f *ap3000.Frame) error {
				bindIfNeeded(f.PhyID)
				sess.OnHeartbeat(f.PhyID, time.Now())
				if appm != nil {
					appm.HeartbeatTotal.Inc()
					appm.OnlineGauge.Set(float64(sess.OnlineCountWeighted(time.Now(), policy)))
					appm.AP3000RouteTotal.WithLabelValues(fmt.Sprintf("%02X", f.Cmd)).Inc()
				}
				handlerSet := getAP3000Handlers()
				if handlerSet == nil {
					return nil
				}
				return handlerSet.HandleHeartbeat(context.Background(), f)
			})
			apAdapter.Register(0x22, func(f *ap3000.Frame) error {
				if appm != nil {
					appm.AP3000RouteTotal.WithLabelValues(fmt.Sprintf("%02X", f.Cmd)).Inc()
				}
				handlerSet := getAP3000Handlers()
				if handlerSet == nil {
					return nil
				}
				return handlerSet.HandleGeneric(context.Background(), f)
			})
			apAdapter.Register(0x12, func(f *ap3000.Frame) error {
				if appm != nil {
					appm.AP3000RouteTotal.WithLabelValues(fmt.Sprintf("%02X", f.Cmd)).Inc()
				}
				handlerSet := getAP3000Handlers()
				if handlerSet == nil {
					return nil
				}
				return handlerSet.HandleGeneric(context.Background(), f)
			})
			apAdapter.Register(0x82, func(f *ap3000.Frame) error {
				if appm != nil {
					appm.AP3000RouteTotal.WithLabelValues(fmt.Sprintf("%02X", f.Cmd)).Inc()
				}
				handlerSet := getAP3000Handlers()
				if handlerSet == nil {
					return nil
				}
				return handlerSet.Handle82Ack(context.Background(), f)
			})
			apAdapter.Register(0x03, func(f *ap3000.Frame) error {
				if appm != nil {
					appm.AP3000RouteTotal.WithLabelValues(fmt.Sprintf("%02X", f.Cmd)).Inc()
				}
				handlerSet := getAP3000Handlers()
				if handlerSet == nil {
					return nil
				}
				return handlerSet.Handle03(context.Background(), f)
			})
			apAdapter.Register(0x06, func(f *ap3000.Frame) error {
				if appm != nil {
					appm.AP3000RouteTotal.WithLabelValues(fmt.Sprintf("%02X", f.Cmd)).Inc()
				}
				handlerSet := getAP3000Handlers()
				if handlerSet == nil {
					return nil
				}
				return handlerSet.Handle06(context.Background(), f)
			})
		}

		// BKV 路由 - 使用真实的BKV协议命令码
		if bkvAdapter != nil {
			// 心跳 (0x0000)
			bkvAdapter.Register(0x0000, func(f *bkv.Frame) error {
				if appm != nil {
					appm.BKVRouteTotal.WithLabelValues(fmt.Sprintf("%04X", f.Cmd)).Inc()
				}
				bindIfNeeded(f.GatewayID)
				bh := getBKVHandlers()
				if bh == nil {
					return nil
				}
				return bh.HandleHeartbeat(context.Background(), f)
			})

			// BKV子协议数据 (插座状态上报等) (0x1000)
			bkvAdapter.Register(0x1000, func(f *bkv.Frame) error {
				if appm != nil {
					appm.BKVRouteTotal.WithLabelValues(fmt.Sprintf("%04X", f.Cmd)).Inc()
				}
				bindIfNeeded(f.GatewayID)
				bh := getBKVHandlers()
				if bh == nil {
					return nil
				}
				return bh.HandleBKVStatus(context.Background(), f)
			})

			// 控制指令 (0x0015)
			bkvAdapter.Register(0x0015, func(f *bkv.Frame) error {
				if appm != nil {
					appm.BKVRouteTotal.WithLabelValues(fmt.Sprintf("%04X", f.Cmd)).Inc()
				}
				bindIfNeeded(f.GatewayID)
				bh := getBKVHandlers()
				if bh == nil {
					return nil
				}
				return bh.HandleControl(context.Background(), f)
			})

			// 网络节点列表 (0x0005)
			bkvAdapter.Register(0x0005, func(f *bkv.Frame) error {
				if appm != nil {
					appm.BKVRouteTotal.WithLabelValues(fmt.Sprintf("%04X", f.Cmd)).Inc()
				}
				bindIfNeeded(f.GatewayID)
				bh := getBKVHandlers()
				if bh == nil {
					return nil
				}
				return bh.HandleGeneric(context.Background(), f)
			})

			// OTA升级 (0x0007)
			bkvAdapter.Register(0x0007, func(f *bkv.Frame) error {
				if appm != nil {
					appm.BKVRouteTotal.WithLabelValues(fmt.Sprintf("%04X", f.Cmd)).Inc()
				}
				bindIfNeeded(f.GatewayID)
				bh := getBKVHandlers()
				if bh == nil {
					return nil
				}
				return bh.HandleGeneric(context.Background(), f)
			})

			// 参数设置/查询 (0x83, 0x84, 0x85)
			paramHandler := func(f *bkv.Frame) error {
				if appm != nil {
					appm.BKVRouteTotal.WithLabelValues(fmt.Sprintf("%04X", f.Cmd)).Inc()
				}
				bindIfNeeded(f.GatewayID)
				bh := getBKVHandlers()
				if bh == nil {
					return nil
				}
				return bh.HandleParam(context.Background(), f)
			}
			bkvAdapter.Register(0x83, paramHandler)
			bkvAdapter.Register(0x84, paramHandler)
			bkvAdapter.Register(0x85, paramHandler)
		}

		mux := tcpserver.NewMux(adapters...)
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
