package agent

import (
	"context"
	"errors"
	"mybuilds/internal/config"
	"mybuilds/internal/distribute"
	"mybuilds/internal/protocol"
	"mybuilds/internal/store"
	"strings"
	"time"
)

// 管理核对使用当前节点身份、独立有限预算；不领取build、续旧lease或调用Run。
func runPublishQuery(ctx context.Context, client *agentHTTP, lock *dataLock, cfg config.AgentConfig, sessionID, nodeID string) error {
	if err := lock.Check(); err != nil {
		return err
	}
	var task protocol.PublishQueryTask
	if err := client.post(ctx, "/api/agent/publish-queries/claim", struct {
		SessionID string `json:"session_id"`
	}{sessionID}, &task); err != nil {
		return err
	}
	if task.ID == "" {
		return nil
	}
	if !exactUUID(task.ID) || !exactUUID(task.Nonce) || task.NodeID != nodeID || task.SessionID != sessionID || !exactUUID(task.BindingID) || !task.ExpiresAt.After(time.Now()) || task.ExpiresAt.After(time.Now().Add(31*time.Second)) || task.Kind != "doctor" && task.Kind != "query" || task.Store != "google_play" && task.Store != "app_store" && task.Store != "custom" {
		return failure("invalid_response")
	}
	// 管理命令同样在启动前落盘。Ref为空使未知清理阻止下次session，不能猜旧PID。
	journal, err := newJournal(lock, task.ID, sessionID)
	if err != nil {
		return err
	}
	bounded, cancel := context.WithDeadline(ctx, task.ExpiresAt)
	defer cancel()
	result := protocol.PublishQueryResult{ID: task.ID, Nonce: task.Nonce, Kind: task.Kind, BindingID: task.BindingID, IntentID: task.IntentID, NodeID: nodeID, SessionID: sessionID, ObservedAt: time.Now().UTC(), Matches: []protocol.PublishMatch{}, Reason: "play_tools_missing"}
	if task.Store == "app_store" {
		result.Reason = "apple_tools_missing"
	}
	values, err := loadSecrets(cfg)
	if task.Store == "custom" {
		if task.Kind != "query" || task.Custom == nil {
			return failure("invalid_response")
		}
		if err == nil {
			result, err = runCustomPublishQuery(bounded, cfg, task, values)
		}
		if errors.Is(err, distribute.ErrCleanup) {
			return failure("cleanup_error")
		}
		if err != nil {
			result = protocol.PublishQueryResult{ID: task.ID, Nonce: task.Nonce, Kind: task.Kind, BindingID: task.BindingID, IntentID: task.IntentID, NodeID: nodeID, SessionID: sessionID, ObservedAt: time.Now().UTC(), Matches: []protocol.PublishMatch{}, Reason: "observation_insufficient"}
		}
		if err = journal.remove(); err != nil {
			return err
		}
		var view store.PublishQueryView
		return client.retryPost(bounded, "/api/agent/publish-queries/"+task.ID+"/result", result, &view)
	}

	name := strings.TrimSuffix(strings.TrimPrefix(task.CredentialRef, "${"), "}")
	secrets := taskSecrets(cfg, config.Build{Steps: []config.Step{{Kind: "upload", Credentials: task.CredentialRef}}}, values)
	if err == nil && cfg.PublishTools != nil && secrets[name] != "" {
		if task.Store == "google_play" {
			result, err = distribute.QueryGooglePlay(bounded, distribute.GooglePlayOptions{BundleDir: cfg.PublishTools.BundleDir, Bundletool: cfg.PublishTools.Bundletool, DataDir: cfg.DataDir, CredentialFile: secrets[name], AppIdentifier: task.AppIdentifier, VersionName: task.VersionName, Number: task.VersionCode, UploadCertificateSHA256: task.UploadCertificateSHA256}, task)
		} else {
			result, err = distribute.QueryApple(bounded, distribute.AppleOptions{BundleDir: cfg.PublishTools.BundleDir, DataDir: cfg.DataDir, CredentialFile: secrets[name], AppIdentifier: task.AppIdentifier, VersionName: task.VersionName, Number: task.VersionCode}, task)
		}
		if errors.Is(err, distribute.ErrCleanup) {
			return failure("cleanup_error")
		}
		if err != nil && result.ID == "" {
			result = protocol.PublishQueryResult{ID: task.ID, Nonce: task.Nonce, Kind: task.Kind, BindingID: task.BindingID, IntentID: task.IntentID, NodeID: nodeID, SessionID: sessionID, ObservedAt: time.Now().UTC(), Matches: []protocol.PublishMatch{}, Reason: "play_unknown"}
			if task.Store == "app_store" {
				result.Reason = "apple_unknown"
			}
		}
	}
	if errors.Is(err, distribute.ErrCleanup) {
		return failure("cleanup_error")
	}
	if err = lock.Check(); err != nil {
		return err
	}
	// 已知管理子进程和独立Close停止后才解除保护；HTTP失败不代表物理进程未知。
	if err = journal.remove(); err != nil {
		return err
	}
	var view store.PublishQueryView
	return client.retryPost(bounded, "/api/agent/publish-queries/"+task.ID+"/result", result, &view)
}
