package agent

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
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
	data, err := json.Marshal(input)
	if err != nil || len(data) > 1<<20 {
		return failure("invalid_request")
	}
	ctx, cancel := context.WithTimeout(parent, client.timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.endpoint+path, bytes.NewReader(data))
	if err != nil {
		return failure("invalid_request")
	}
	request.Header.Set("Authorization", "Bearer "+client.token)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.client.Do(request)
	if err != nil {
		if parent.Err() != nil {
			return failure("cancelled")
		}
		return failure("network_error")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil {
		return failure("network_error")
	}
	if len(body) > 1<<20 {
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
	if response.StatusCode == http.StatusNoContent && path == "/api/agent/claim" && len(body) == 0 {
		return nil
	}
	if output == nil {
		if response.StatusCode != http.StatusNoContent && len(bytes.TrimSpace(body)) != 0 {
			return failure("invalid_response")
		}
		return nil
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
	return nil
}
func safeNodeError(code string) bool {
	switch code {
	case "node_unauthorized", "session_conflict", "session_expired", "lease_invalid", "lease_expired", "event_conflict", "sequence_invalid", "budget_invalid", "stop_unconfirmed", "artifact_conflict", "log_conflict", "control_lock_lost", "invalid_request", "request_too_large", "forbidden", "not_found", "unauthorized", "conflict":
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
		if !ok || safe.Code != "network_error" || ctx.Err() != nil {
			return err
		}
		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return failure("network_error")
		case <-timer.C:
		}
	}
}
