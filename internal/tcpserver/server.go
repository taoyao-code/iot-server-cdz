package tcpserver

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	cfgpkg "github.com/taoyao-code/iot-server/internal/config"
	"go.uber.org/zap"
)

// Server TCP 网关最小实现
type Server struct {
	cfg         cfgpkg.TCPConfig
	ln          net.Listener
	wg          sync.WaitGroup
	stopC       chan struct{}
	handler     func([]byte)
	connHandler func(*ConnContext)
	// 可选指标回调
	onAccept    func()
	onRecvBytes func(n int)
	nextConnID  uint64

	// Week2: 限流和熔断
	connLimiter *ConnectionLimiter
	rateLimiter *RateLimiter
	breaker     *CircuitBreaker
	logger      *zap.Logger
}

// New 创建 TCP 网关
func New(cfg cfgpkg.TCPConfig) *Server {
	return &Server{cfg: cfg, stopC: make(chan struct{})}
}

// SetHandler 设置上行报文处理回调（raw bytes）
func (s *Server) SetHandler(h func([]byte)) { s.handler = h }

// SetConnHandler 设置连接级回调（提供写能力与生命周期）
func (s *Server) SetConnHandler(h func(*ConnContext)) { s.connHandler = h }

// SetMetricsCallbacks 设置指标回调
func (s *Server) SetMetricsCallbacks(onAccept func(), onRecvBytes func(int)) {
	s.onAccept, s.onRecvBytes = onAccept, onRecvBytes
}

// SetLogger 设置日志器
func (s *Server) SetLogger(logger *zap.Logger) {
	s.logger = logger
}

// GetLogger 获取日志器
func (s *Server) GetLogger() *zap.Logger {
	return s.logger
}

// GetCurrentTime 获取当前时间 (用于协议识别超时检测)
func (s *Server) GetCurrentTime() time.Time {
	return time.Now()
}

// EnableLimiting 启用限流和熔断（Week2）
func (s *Server) EnableLimiting(maxConn int, ratePerSec int, rateBurst int, breakerThreshold int, breakerTimeout time.Duration) {
	s.connLimiter = NewConnectionLimiter(maxConn, 5*time.Second)
	s.rateLimiter = NewRateLimiter(ratePerSec, rateBurst)
	s.breaker = NewCircuitBreaker(breakerThreshold, breakerTimeout)

	// 设置熔断器状态变化回调
	s.breaker.SetStateChangeCallback(func(from, to State) {
		if s.logger != nil {
			s.logger.Warn("circuit breaker state changed",
				zap.String("from", from.String()),
				zap.String("to", to.String()),
			)
		}
	})
}

// ActiveConnections 获取当前活跃连接数
func (s *Server) ActiveConnections() int {
	if s.connLimiter != nil {
		return s.connLimiter.Current()
	}
	return 0
}

// MaxConnections 获取最大连接数
func (s *Server) MaxConnections() int {
	if s.connLimiter != nil {
		return s.connLimiter.MaxConnections()
	}
	return 0
}

// GetLimiterStats 获取限流器统计
func (s *Server) GetLimiterStats() *LimiterStats {
	if s.connLimiter != nil {
		stats := s.connLimiter.Stats()
		return &stats
	}
	return nil
}

// GetRateLimiterStats 获取速率限流器统计
func (s *Server) GetRateLimiterStats() *RateLimiterStats {
	if s.rateLimiter != nil {
		stats := s.rateLimiter.Stats()
		return &stats
	}
	return nil
}

// GetCircuitBreakerStats 获取熔断器统计
func (s *Server) GetCircuitBreakerStats() *CircuitBreakerStats {
	if s.breaker != nil {
		stats := s.breaker.Stats()
		return &stats
	}
	return nil
}

// Start 监听并接受连接（非阻塞，内部 goroutine）
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		return err
	}
	s.ln = ln

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			// Week2: 速率限流检查
			if s.rateLimiter != nil && !s.rateLimiter.Allow() {
				time.Sleep(10 * time.Millisecond)
				continue
			}

			conn, err := s.ln.Accept()
			if err != nil {
				select {
				case <-s.stopC:
					return
				default:
				}
				// 短暂错误等待后重试
				time.Sleep(50 * time.Millisecond)
				continue
			}

			// ✅ 优化1: 启用TCP Keepalive,防止NAT超时
			if tcpConn, ok := conn.(*net.TCPConn); ok {
				_ = tcpConn.SetKeepAlive(true)
				_ = tcpConn.SetKeepAlivePeriod(60 * time.Second)
				if s.logger != nil {
					s.logger.Debug("TCP keepalive enabled",
						zap.String("remote_addr", conn.RemoteAddr().String()),
						zap.Duration("period", 60*time.Second),
					)
				}
			}

			// 记录TCP连接
			if s.logger != nil {
				s.logger.Info("TCP connection accepted",
					zap.String("remote_addr", conn.RemoteAddr().String()),
					zap.String("local_addr", conn.LocalAddr().String()),
				)
			}

			if s.onAccept != nil {
				s.onAccept()
			}

			// Week2: 连接数限流检查
			if s.connLimiter != nil {
				if err := s.connLimiter.Acquire(context.Background()); err != nil {
					conn.Close()
					if s.logger != nil {
						s.logger.Warn("connection rejected by limiter", zap.Error(err))
					}
					continue
				}
			}

			// Week2: 熔断器检查
			if s.breaker != nil {
				err := s.breaker.Call(func() error {
					// 处理连接
					s.handleConnWithProtection(conn)
					return nil
				})

				if err == ErrCircuitOpen || err == ErrTooManyRequests {
					conn.Close()
					if s.connLimiter != nil {
						s.connLimiter.Release()
					}
					if s.logger != nil {
						s.logger.Warn("connection rejected by circuit breaker", zap.Error(err))
					}
					continue
				}
			} else {
				// 无熔断器，直接处理
				s.handleConnWithProtection(conn)
			}
		}
	}()
	return nil
}

// handleConnWithProtection 带保护的连接处理
func (s *Server) handleConnWithProtection(conn net.Conn) {
	if s.connHandler != nil {
		// 新接口：连接上下文 + 读写分离
		s.wg.Add(1)
		go func(c net.Conn) {
			defer s.wg.Done()
			defer func() {
				// 记录连接关闭
				if s.logger != nil {
					s.logger.Info("TCP connection closed",
						zap.String("remote_addr", c.RemoteAddr().String()),
					)
				}
				c.Close()
			}()
			defer func() {
				if s.connLimiter != nil {
					s.connLimiter.Release()
				}
			}()
			defer func() {
				if r := recover(); r != nil {
					if s.logger != nil {
						s.logger.Error("panic in handleConn",
							zap.Any("panic", r),
							zap.String("remote_addr", c.RemoteAddr().String()),
						)
					}
					// 记录为失败，影响熔断器
					if s.breaker != nil {
						s.breaker.afterCall(fmt.Errorf("panic: %v", r))
					}
				}
			}()

			ctx := newConnContext(s, c)
			// 允许上层为该连接安装专属 onRead/绑定会话等
			s.connHandler(ctx)
			ctx.run()
		}(conn)
		return
	}

	// 兼容旧接口（raw bytes handler）
	if s.handler != nil {
		s.wg.Add(1)
		go func(c net.Conn) {
			defer s.wg.Done()
			defer c.Close()
			defer func() {
				if s.connLimiter != nil {
					s.connLimiter.Release()
				}
			}()
			defer func() {
				if r := recover(); r != nil {
					if s.logger != nil {
						s.logger.Error("panic in handler", zap.Any("panic", r))
					}
				}
			}()

			_ = c.SetReadDeadline(time.Now().Add(s.cfg.ReadTimeout))
			_ = c.SetWriteDeadline(time.Now().Add(s.cfg.WriteTimeout))
			buf := make([]byte, 4096)
			for {
				n, err := c.Read(buf)
				if n > 0 {
					if s.onRecvBytes != nil {
						s.onRecvBytes(n)
					}
					s.handler(buf[:n])
				}
				if err != nil {
					if ne, ok := err.(net.Error); ok && ne.Timeout() {
						continue
					}
					if err == io.EOF {
						return
					}
					return
				}
			}
		}(conn)
		return
	}

	// 无handler，直接关闭连接
	conn.Close()
	if s.connLimiter != nil {
		s.connLimiter.Release()
	}
}

// Shutdown 优雅关闭监听并等待连接退出
func (s *Server) Shutdown(ctx context.Context) error {
	close(s.stopC)
	if s.ln != nil {
		_ = s.ln.Close()
	}
	ch := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(ch)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-ch:
		return nil
	}
}
