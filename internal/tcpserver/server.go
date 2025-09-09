package tcpserver

import (
	"context"
	"net"
	"sync"
	"time"

	cfgpkg "github.com/taoyao-code/iot-server/internal/config"
)

// Server TCP 网关最小实现
type Server struct {
	cfg   cfgpkg.TCPConfig
	ln    net.Listener
	wg    sync.WaitGroup
	stopC chan struct{}
}

// New 创建 TCP 网关
func New(cfg cfgpkg.TCPConfig) *Server {
	return &Server{cfg: cfg, stopC: make(chan struct{})}
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

			s.wg.Add(1)
			go func(c net.Conn) {
				defer s.wg.Done()
				_ = c.SetReadDeadline(time.Now().Add(s.cfg.ReadTimeout))
				_ = c.SetWriteDeadline(time.Now().Add(s.cfg.WriteTimeout))
				// 最小闭环：立即关闭
				_ = c.Close()
			}(conn)
		}
	}()
	return nil
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
