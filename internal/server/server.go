// Package server 提供鉴权控制面，只持久化队列，不执行流水线。
package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"mybuilds/internal/config"
	"mybuilds/internal/store"
	"mybuilds/internal/version"
)

type Server struct {
	store  *store.Store
	config config.ServerConfig
}

func New(st *store.Store, cfg config.ServerConfig) *Server { return &Server{store: st, config: cfg} }

type StatusDTO struct {
	Version     string `json:"version"`
	Concurrency int    `json:"concurrency"`
	Projects    int64  `json:"projects"`
	Queued      int64  `json:"queued"`
	Skipped     int64  `json:"skipped"`
	Running     int64  `json:"running"`
	Nodes       int64  `json:"nodes"`
}

func (s *Server) Handler() http.Handler { return http.HandlerFunc(s.handle) }
func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// 所有入口先检查运行权与真实身份，不能借未知路径绕过身份边界。
	if err := s.store.CheckLock(r.Context()); err != nil {
		writeError(w, err)
		return
	}
	authorization := r.Header.Values("Authorization")
	if len(authorization) != 1 || !strings.HasPrefix(authorization[0], "Bearer ") || strings.ContainsAny(strings.TrimPrefix(authorization[0], "Bearer "), " \t\r\n") {
		writeError(w, store.ErrUnauthorized)
		return
	}
	actor, err := s.store.Authenticate(r.Context(), strings.TrimPrefix(authorization[0], "Bearer "))
	if err != nil {
		writeError(w, err)
		return
	}
	if r.URL.Path == "/api/status" && r.Method == http.MethodGet {
		if _, err := readQuery(r); err != nil {
			writeError(w, err)
			return
		}
		status, err := s.store.Status(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, 200, StatusDTO{Version: version.Version, Concurrency: s.config.Concurrency, Projects: status.Projects, Queued: status.Queued, Skipped: status.Skipped})
		return
	}
	if s.buildRoutes(w, r, actor) {
		return
	}
	s.management(w, r, actor)
}

// ListenAndServe 取消时关闭连接，失锁时停止接入；不恢复旧数据库 session。
func (s *Server) ListenAndServe(ctx context.Context) error {
	if err := s.store.CheckLock(ctx); err != nil {
		return err
	}
	live, cancel := context.WithCancel(ctx)
	defer cancel()
	service := &http.Server{Addr: s.config.Listen, Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 40 * time.Second, IdleTimeout: 60 * time.Second, BaseContext: func(net.Listener) context.Context { return live }}
	done := make(chan error, 1)
	go func() { done <- service.ListenAndServe() }()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	var result error
	for {
		select {
		case err := <-done:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return errors.New("server_listen_failed")
		case <-live.Done():
			result = nil
		case <-ticker.C:
			if err := s.store.CheckLock(live); err == nil {
				continue
			} else {
				if live.Err() == nil || errors.Is(err, store.ErrLockLost) {
					result = err
				}
			}
		}
		cancel()
		shutdown, stop := context.WithTimeout(context.Background(), 5*time.Second)
		err := service.Shutdown(shutdown)
		stop()
		if err != nil {
			_ = service.Close()
			if result == nil {
				result = errors.New("server_shutdown_failed")
			}
		}
		<-done
		return result
	}
}
