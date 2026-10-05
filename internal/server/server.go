// Package server 提供鉴权、调度与中央证据，不执行仓库脚本。
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
	profiles          map[string]config.LoadedProfile
	profileError      error
	store             *store.Store
	config            config.ServerConfig
	evidenceReadOwner string
	evidenceReadError error
}

func New(st *store.Store, cfg config.ServerConfig) *Server {
	owner, err := st.RegisterEvidenceReadOwner(context.Background())
	profiles, profileErr := config.LoadBuildProfiles(cfg.BuildProfiles, builtinProfiles())
	return &Server{store: st, config: cfg, evidenceReadOwner: owner, evidenceReadError: err, profiles: profiles, profileError: profileErr}
}

type StatusDTO struct {
	Version      string `json:"version"`
	Concurrency  int    `json:"concurrency"`
	Projects     int64  `json:"projects"`
	Queued       int64  `json:"queued"`
	Skipped      int64  `json:"skipped"`
	Running      int64  `json:"running"`
	Nodes        int64  `json:"nodes"`
	Interrupted  int64  `json:"interrupted"`
	HealthyNodes int64  `json:"healthy_nodes"`
}

func (s *Server) Handler() http.Handler { return http.HandlerFunc(s.handle) }
func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	limit := 30 * time.Second
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(parts) == 4 && parts[0] == "api" && parts[1] == "agent" && parts[2] == "artifacts" && r.Method == http.MethodPut {
		limit = 2 * time.Minute
	}
	if len(parts) == 4 && parts[0] == "api" && parts[1] == "artifacts" && r.Method == http.MethodGet {
		limit = 10 * time.Minute
	}
	if len(parts) == 4 && parts[0] == "api" && parts[1] == "builds" && parts[3] == "log" && r.Method == http.MethodGet && r.URL.Query().Get("follow") == "1" {
		limit = 15 * time.Minute
	}
	ctx, cancel := context.WithTimeout(r.Context(), limit)
	defer cancel()
	r = r.WithContext(ctx)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// 所有入口先检查运行权与真实身份，不能借未知路径绕过身份边界。
	if err := s.store.CheckLock(r.Context()); err != nil {
		writeError(w, err)
		return
	}
	if s.profileError != nil {
		writeError(w, errPipeline)
		return
	}
	if s.evidenceReadError != nil {
		writeError(w, s.evidenceReadError)
		return
	}
	if len(parts) == 2 && parts[0] == "hook" {
		s.receiveWebhook(w, r, parts[1])
		return
	}
	authorization := r.Header.Values("Authorization")
	if len(authorization) != 1 || !strings.HasPrefix(authorization[0], "Bearer ") || strings.ContainsAny(strings.TrimPrefix(authorization[0], "Bearer "), " \t\r\n") {
		writeError(w, store.ErrUnauthorized)
		return
	}
	token := strings.TrimPrefix(authorization[0], "Bearer ")
	if strings.HasPrefix(r.URL.Path, "/api/agent/") {
		actor, err := s.store.AuthenticateNode(r.Context(), token)
		if err != nil {
			writeError(w, err)
			return
		}
		s.agentRoutes(w, r, actor)
		return
	}
	actor, err := s.store.Authenticate(r.Context(), token)
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
		writeJSON(w, 200, s.statusView(status))
		return
	}
	if r.URL.Path == "/api/doctor" {
		s.controlDoctor(w, r, actor)
		return
	}
	if s.approvalRoutes(w, r, actor) {
		return
	}
	if s.publishRoutes(w, r, actor) {
		return
	}
	if s.hookRoutes(w, r, actor) {
		return
	}
	if s.nodeRoutes(w, r, actor) {
		return
	}
	if s.retentionRoutes(w, r, actor) {
		return
	}
	if s.buildRoutes(w, r, actor) {
		return
	}
	s.management(w, r, actor)
}

// ListenAndServe 取消时关闭连接，失锁时停止接入；不恢复旧数据库 session。
func (s *Server) ListenAndServe(ctx context.Context) error {
	if s.profileError != nil {
		return errPipeline
	}
	if err := s.store.CheckLock(ctx); err != nil {
		return err
	}
	if s.evidenceReadError != nil {
		return s.evidenceReadError
	}
	recovery, finishRecovery := context.WithTimeout(ctx, 30*time.Second)
	err := s.store.Recover(recovery)
	finishRecovery()
	if err != nil {
		return err
	}
	if err := s.store.SyncGlobalRetention(ctx, s.config.Retention); err != nil {
		return err
	}
	live, cancel := context.WithCancel(ctx)
	defer cancel()
	service := &http.Server{Addr: s.config.Listen, Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 40 * time.Second, IdleTimeout: 60 * time.Second, BaseContext: func(net.Listener) context.Context { return live }}
	done := make(chan error, 1)
	go func() { done <- service.ListenAndServe() }()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	retentionTicker := time.NewTicker(60 * time.Second)
	defer retentionTicker.Stop()
	hookTicker := time.NewTicker(time.Second)
	defer hookTicker.Stop()
	var hookDone chan error
	var retentionDone chan error
	defer func() {
		cancel()
		if retentionDone != nil {
			<-retentionDone
		}
		if hookDone != nil {
			<-hookDone
		}
	}()
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
		case <-hookTicker.C:
			if hookDone != nil {
				continue
			}
			hookDone = make(chan error, 1)
			finished := hookDone
			go func() { finished <- s.closeDueWebhookWindows(live) }()
			continue
		case err := <-hookDone:
			hookDone = nil
			if !errors.Is(err, store.ErrLockLost) {
				continue
			}
			result = err
		case <-retentionTicker.C:
			if retentionDone != nil {
				continue
			}
			retentionDone = make(chan error, 1)
			finished := retentionDone
			// 有界文件IO不阻塞原200ms运行权/租约核对，退出时等待本轮确已停止。
			go func() {
				round, stop := context.WithTimeout(live, 30*time.Second)
				defer stop()
				err := s.advanceCentralRetention(round, "", 100)
				if !errors.Is(err, store.ErrLockLost) && round.Err() == nil {
					_, finalErr := s.store.FinalizeRetention(round, "", 100)
					if err == nil || errors.Is(finalErr, store.ErrLockLost) {
						err = finalErr
					}
				}
				finished <- err
			}()
			continue
		case err := <-retentionDone:
			retentionDone = nil
			if !errors.Is(err, store.ErrLockLost) {
				continue
			}
			result = err
		case <-ticker.C:
			if err := s.store.CheckLock(live); err == nil {
				if err = s.store.ExpireLeases(live); err == nil {
					continue
				} else if live.Err() == nil || errors.Is(err, store.ErrLockLost) {
					result = err
				}
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

func (s *Server) statusView(status store.QueueStatus) StatusDTO {
	return StatusDTO{Version: version.Version, Concurrency: s.config.Concurrency, Projects: status.Projects, Queued: status.Queued, Skipped: status.Skipped, Running: status.Running, Interrupted: status.Interrupted, Nodes: status.Nodes, HealthyNodes: status.HealthyNodes}
}
