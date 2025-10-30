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

		// BKV 路由 - 完整注册所有BKV协议命令
		if bkvAdapter != nil {
			// 通用Handler包装器：添加会话绑定和指标上报
			wrapBKVHandler := func(handlerFunc func(context.Context, *bkv.Frame) error) func(*bkv.Frame) error {
				return func(f *bkv.Frame) error {
					// 记录BKV帧信息
					if cc.Server() != nil && cc.Server().GetLogger() != nil {
						cc.Server().GetLogger().Info("BKV frame received",
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

					// 执行Handler并记录结果
					err := handlerFunc(context.Background(), f)
					if err != nil && cc.Server() != nil && cc.Server().GetLogger() != nil {
						cc.Server().GetLogger().Error("BKV handler error",
							zap.String("cmd", fmt.Sprintf("0x%04X", f.Cmd)),
							zap.String("gateway_id", f.GatewayID),
							zap.Error(err),
						)
					}
					return err
				}
			}

			// 基础命令
			bkvAdapter.Register(0x0000, wrapBKVHandler(func(ctx context.Context, f *bkv.Frame) error {
				// 记录心跳到Redis会话，修正 online 判断
				sess.OnHeartbeat(f.GatewayID, time.Now())
				if appm != nil {
					appm.HeartbeatTotal.Inc()
					appm.OnlineGauge.Set(float64(sess.OnlineCountWeighted(time.Now(), policy)))
				}
				return getBKVHandlers().HandleHeartbeat(ctx, f)
			})) // 心跳

			// BKV子协议
			bkvAdapter.Register(0x1000, wrapBKVHandler(func(ctx context.Context, f *bkv.Frame) error {
				return getBKVHandlers().HandleBKVStatus(ctx, f)
			})) // BKV状态上报

			// 控制命令
			bkvAdapter.Register(0x0015, wrapBKVHandler(func(ctx context.Context, f *bkv.Frame) error {
				return getBKVHandlers().HandleControl(ctx, f)
			})) // 控制设备

			// 网络管理
			bkvAdapter.Register(0x0005, wrapBKVHandler(func(ctx context.Context, f *bkv.Frame) error {
				return getBKVHandlers().HandleGeneric(ctx, f)
			})) // 网络节点列表
			bkvAdapter.Register(0x0008, wrapBKVHandler(func(ctx context.Context, f *bkv.Frame) error {
				return getBKVHandlers().HandleNetworkRefresh(ctx, f)
			})) // 刷新插座列表
			bkvAdapter.Register(0x0009, wrapBKVHandler(func(ctx context.Context, f *bkv.Frame) error {
				return getBKVHandlers().HandleNetworkAddNode(ctx, f)
			})) // 添加插座
			bkvAdapter.Register(0x000A, wrapBKVHandler(func(ctx context.Context, f *bkv.Frame) error {
				return getBKVHandlers().HandleNetworkDeleteNode(ctx, f)
			})) // 删除插座

			// OTA升级
			bkvAdapter.Register(0x0007, wrapBKVHandler(func(ctx context.Context, f *bkv.Frame) error {
				// 根据数据长度判断是响应还是进度
				if len(f.Data) >= 3 && len(f.Data) <= 10 {
					return getBKVHandlers().HandleOTAResponse(ctx, f)
				} else if len(f.Data) >= 4 {
					return getBKVHandlers().HandleOTAProgress(ctx, f)
				}
				return getBKVHandlers().HandleGeneric(ctx, f)
			})) // OTA升级

			// 刷卡充电相关
			bkvAdapter.Register(0x000B, wrapBKVHandler(func(ctx context.Context, f *bkv.Frame) error {
				return getBKVHandlers().HandleCardSwipe(ctx, f)
			})) // 刷卡上报/下发充电
			bkvAdapter.Register(0x000C, wrapBKVHandler(func(ctx context.Context, f *bkv.Frame) error {
				return getBKVHandlers().HandleChargeEnd(ctx, f)
			})) // 充电结束上报
			bkvAdapter.Register(0x000F, wrapBKVHandler(func(ctx context.Context, f *bkv.Frame) error {
				return getBKVHandlers().HandleOrderConfirm(ctx, f)
			})) // 订单确认
			bkvAdapter.Register(0x001A, wrapBKVHandler(func(ctx context.Context, f *bkv.Frame) error {
				return getBKVHandlers().HandleBalanceQuery(ctx, f)
			})) // 余额查询

			// 按功率充电
			bkvAdapter.Register(0x0017, wrapBKVHandler(func(ctx context.Context, f *bkv.Frame) error {
				return getBKVHandlers().HandleControl(ctx, f) // 复用控制处理器
			})) // 按功率分档充电命令
			bkvAdapter.Register(0x0018, wrapBKVHandler(func(ctx context.Context, f *bkv.Frame) error {
				return getBKVHandlers().HandlePowerLevelEnd(ctx, f)
			})) // 按功率充电结束
			bkvAdapter.Register(0x0019, wrapBKVHandler(func(ctx context.Context, f *bkv.Frame) error {
				return getBKVHandlers().HandleServiceFeeEnd(ctx, f)
			})) // 服务费充电结束

			// 参数管理
			bkvAdapter.Register(0x0001, wrapBKVHandler(func(ctx context.Context, f *bkv.Frame) error {
				return getBKVHandlers().HandleParamReadResponse(ctx, f)
			})) // 批量读取参数
			bkvAdapter.Register(0x0002, wrapBKVHandler(func(ctx context.Context, f *bkv.Frame) error {
				return getBKVHandlers().HandleParamWriteResponse(ctx, f)
			})) // 批量写入参数
			bkvAdapter.Register(0x0003, wrapBKVHandler(func(ctx context.Context, f *bkv.Frame) error {
				return getBKVHandlers().HandleParamSyncResponse(ctx, f)
			})) // 参数同步
			bkvAdapter.Register(0x0004, wrapBKVHandler(func(ctx context.Context, f *bkv.Frame) error {
				return getBKVHandlers().HandleParamResetResponse(ctx, f)
			})) // 参数重置

			// 查询插座状态
			socketStateHandler := wrapBKVHandler(func(ctx context.Context, f *bkv.Frame) error {
				return getBKVHandlers().HandleSocketStateResponse(ctx, f)
			})
			bkvAdapter.Register(0x000D, socketStateHandler)
			bkvAdapter.Register(0x000E, socketStateHandler)
			bkvAdapter.Register(0x001D, socketStateHandler)

			// 语音配置
			bkvAdapter.Register(0x001B, wrapBKVHandler(func(ctx context.Context, f *bkv.Frame) error {
				return getBKVHandlers().HandleVoiceConfigResponse(ctx, f)
			})) // 语音配置

			// BKV子协议扩展
			bkvAdapter.Register(0x1017, wrapBKVHandler(func(ctx context.Context, f *bkv.Frame) error {
				// 插座状态上报也视作心跳来源，保持在线状态新鲜
				sess.OnHeartbeat(f.GatewayID, time.Now())
				if appm != nil {
					appm.HeartbeatTotal.Inc()
					appm.OnlineGauge.Set(float64(sess.OnlineCountWeighted(time.Now(), policy)))
				}
				return getBKVHandlers().HandleHeartbeat(ctx, f) // 插座状态上报复用心跳处理
			})) // 插座状态上报
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
