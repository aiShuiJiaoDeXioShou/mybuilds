package agent

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
)

// 仅节点实际HTTP消费者使用；不继承宿主代理或放宽TLS验证。
type agentHTTP struct {
	client          *http.Client
	transport       *http.Transport
	endpoint, token string
	timeout         time.Duration
	paused          atomic.Bool
}

func newAgentHTTP(cfg config.AgentConfig) (*agentHTTP, error) {
	endpoint, err := url.Parse(cfg.Server)
	if err != nil || endpoint.Host == "" || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.ForceQuery || endpoint.Fragment != "" || endpoint.Opaque != "" {
		return nil, failure("configuration_error")
	}
	ip := net.ParseIP(endpoint.Hostname())
	if endpoint.Scheme != "https" && !(endpoint.Scheme == "http" && (endpoint.Hostname() == "localhost" || ip != nil && ip.IsLoopback())) {
		return nil, failure("configuration_error")
	}
	if len(cfg.RuntimeToken) < 32 || len(cfg.RuntimeToken) > 4096 {
		return nil, failure("configuration_error")
	}
	for _, c := range cfg.RuntimeToken {
		if c < 33 || c > 126 {
			return nil, failure("configuration_error")
		}
	}
	if cfg.HeartbeatInterval < time.Second || cfg.HeartbeatInterval > 30*time.Second {
		return nil, failure("configuration_error")
	}
	transport := &http.Transport{DialContext: (&net.Dialer{Timeout: cfg.HeartbeatInterval, KeepAlive: 30 * time.Second}).DialContext, TLSHandshakeTimeout: cfg.HeartbeatInterval, ResponseHeaderTimeout: cfg.HeartbeatInterval, IdleConnTimeout: 30 * time.Second, MaxIdleConns: 8, MaxIdleConnsPerHost: 4}
	if endpoint.Scheme == "https" {
		roots, err := config.TLSRoots(cfg.CAFile)
		if err != nil {
			return nil, failure("tls_configuration_error")
		}
		transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	}
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &agentHTTP{client: client, transport: transport, endpoint: strings.TrimRight(endpoint.String(), "/"), token: cfg.RuntimeToken, timeout: cfg.HeartbeatInterval}, nil
}
func (client *agentHTTP) close() { client.transport.CloseIdleConnections() }
func (client *agentHTTP) post(parent context.Context, path string, input, output any) error {
	return client.request(parent, http.MethodPost, path, input, output)
}
func (client *agentHTTP) get(parent context.Context, path string, output any) error {
	return client.request(parent, http.MethodGet, path, nil, output)
}
func (client *agentHTTP) request(parent context.Context, method, path string, input, output any) error {
	var data []byte
	if method == http.MethodPost {
		var err error
		data, err = json.Marshal(input)
		if err != nil || len(data) > 1<<20 {
			return failure("invalid_request")
		}
	}
	ctx, cancel := context.WithTimeout(parent, client.timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, client.endpoint+path, bytes.NewReader(data))
	if err != nil {
		return failure("invalid_request")
	}
	request.Header.Set("Authorization", "Bearer "+client.token)
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.client.Do(request)
	if err != nil {
		return client.requestError(parent, err)
	}
	defer response.Body.Close()
	limit := 1 << 20
	if strings.HasPrefix(path, "/api/agent/publishes/") || strings.HasPrefix(path, "/api/agent/publish-queries/") {
		limit = 64 << 10
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, int64(limit)+1))
	if err != nil {
		return client.requestError(parent, err)
	}
	if len(body) > limit {
		return failure("invalid_response")
	}
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		return failure("redirect_denied")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var safe struct {
			Error struct{ Code, Message string } `json:"error"`
		}
		if json.Unmarshal(body, &safe) == nil && safeNodeError(safe.Error.Code) {
			return failure(safe.Error.Code)
		}
		return failure("request_failed")
	}
	if response.StatusCode == http.StatusNoContent && (path == "/api/agent/claim" || path == "/api/agent/publish-queries/claim") && len(body) == 0 {
		return nil
	}
	if output == nil {
		if (path == "/api/agent/resources" || path == "/api/agent/stop-confirmation") && (response.StatusCode != http.StatusNoContent || len(body) != 0) {
			return failure("invalid_response")
		}
		if response.StatusCode != http.StatusNoContent && len(bytes.TrimSpace(body)) != 0 {
			return failure("invalid_response")
		}
		return nil
	}
	deletion := path == "/api/agent/deletions" || strings.HasPrefix(path, "/api/agent/deletions?") || strings.HasPrefix(path, "/api/agent/deletions/")
	if deletion && (response.StatusCode != http.StatusOK || len(body) > 32*1024) {
		return failure("invalid_response")
	}
	// 终态和删除回执不得用重复字段后值覆盖原事实；null也不能代表空事项。
	if path == "/api/agent/terminal-receipt" || deletion || strings.HasPrefix(path, "/api/agent/publishes/") || strings.HasPrefix(path, "/api/agent/publish-queries/") {
		tokens := json.NewDecoder(bytes.NewReader(body))
		count := 0
		if journalJSONValue(tokens, 0, &count) != nil {
			return failure("invalid_response")
		}
		if _, err := tokens.Token(); err != io.EOF {
			return failure("invalid_response")
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return failure("invalid_response")
	}
	if path == "/api/agent/claim" {
		grant, ok := output.(*protocol.LeaseGrant)
		if !ok || grant.Task == nil {
			return failure("invalid_response")
		}
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return failure("invalid_response")
	}
	if deletion && !validDeletionResponse(body, output) {
		return failure("invalid_response")
	}
	return nil
}

// 仅核当前三个管理响应的精确必填字段，不为其它接口引入新的解码框架。
func validDeletionResponse(data []byte, output any) bool {
	fields := func(raw []byte, names ...string) bool {
		var values map[string]json.RawMessage
		if json.Unmarshal(raw, &values) != nil || len(values) != len(names) {
			return false
		}
		for _, name := range names {
			if _, ok := values[name]; !ok {
				return false
			}
		}
		return true
	}
	switch out := output.(type) {
	case *[]protocol.NodeDeletion:
		if out == nil || *out == nil || len(*out) > 10 {
			return false
		}
		var items []json.RawMessage
		if json.Unmarshal(data, &items) != nil || len(items) != len(*out) {
			return false
		}
		for i, item := range *out {
			if !validNodeDeletion(item) || !fields(items[i], "id", "resource_id", "build_id", "attempt_id", "ownership_digest", "has_workspace", "has_results") {
				return false
			}
		}
		return true
	case *protocol.DeletionAuthority:
		return out != nil && exactUUID(out.ID) && exactUUID(out.NodeID) && exactUUID(out.ResourceID) && exactUUID(out.Nonce) && safeDigest(out.OwnershipDigest) && !out.ExpiresAt.IsZero() && fields(data, "id", "node_id", "resource_id", "ownership_digest", "nonce", "expires_at")
	case *protocol.NodeDeletionReceipt:
		return out != nil && exactUUID(out.ID) && out.Seq > 0 && safeDigest(out.Digest) && fields(data, "id", "seq", "digest")
	}
	return false
}
func safeNodeError(code string) bool {
	switch code {
	case "node_unauthorized", "session_conflict", "session_expired", "lease_invalid", "lease_expired", "event_conflict", "sequence_invalid", "budget_invalid", "stop_unconfirmed", "artifact_conflict", "log_conflict", "control_lock_lost", "invalid_request", "request_too_large", "forbidden", "not_found", "unauthorized", "conflict", "retention_retired", "retention_protected", "retention_readers_active", "retention_ownership_unknown", "retention_receipt_conflict", "retention_object_invalid", "retention_limit", "retention_timeout", "retention_cancelled", "retention_io_error":
		return true
	}
	return false
}

// 同一消息在单次有界网络预算内重试，保留claim key、事件摘要和日志序号。
func (client *agentHTTP) retryPost(parent context.Context, path string, input, output any) error {
	ctx, cancel := context.WithTimeout(parent, client.timeout)
	defer cancel()
	for {
		err := client.post(ctx, path, input, output)
		if err == nil {
			return nil
		}
		safe, ok := err.(*Error)
		if parent.Err() != nil {
			return failure("cancelled")
		}
		if ctx.Err() != nil && ok && (safe.Code == "cancelled" || safe.Code == "network_error") {
			return failure("network_error")
		}
		if !ok || safe.Code != "network_error" {
			return err
		}
		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			if parent.Err() != nil {
				return failure("cancelled")
			}
			return failure("network_error")
		case <-timer.C:
		}
	}
}

// 只在真实执行Authority内等待网络恢复，每轮HTTP期限不延长原租约。
func (client *agentHTTP) retryExecutionPost(ctx context.Context, path string, input, output any) error {
	for {
		err := client.retryPost(ctx, path, input, output)
		if !temporaryNetwork(err) || ctx.Err() != nil {
			return err
		}
		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return failure("cancelled")
		case <-timer.C:
		}
	}
}
func temporaryNetwork(err error) bool {
	safe, ok := err.(*Error)
	return ok && safe.Code == "network_error"
}
func (client *agentHTTP) requestError(parent context.Context, err error) error {
	if parent.Err() != nil {
		return failure("cancelled")
	}
	var certificate *tls.CertificateVerificationError
	var record tls.RecordHeaderError
	if errors.As(err, &certificate) || errors.As(err, &record) {
		return failure("tls_configuration_error")
	}
	var transport *url.Error
	if errors.As(err, &transport) {
		err = transport.Err
	}
	var network net.Error
	if errors.As(err, &network) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) {
		client.paused.Store(true)
		return failure("network_error")
	}
	return failure("request_failed")
}

func safeDigest(value string) bool {
	data, e := hex.DecodeString(value)
	return e == nil && len(data) == 32 && hex.EncodeToString(data) == value
}
func validNodeDeletion(item protocol.NodeDeletion) bool {
	return exactUUID(item.ID) && exactUUID(item.ResourceID) && exactUUID(item.BuildID) && exactUUID(item.AttemptID) && safeDigest(item.OwnershipDigest) && item.HasWorkspace
}
