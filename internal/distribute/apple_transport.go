package distribute

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"mybuilds/internal/process"
	"mybuilds/internal/protocol"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"
)

var transportCorrelation = regexp.MustCompile(`(?:RequestUUID|requestUUID|Delivery UUID)\s*[:=]\s*([0-9a-fA-F-]{36})`)

// uploadBinary只执行fastlane实际生成并再次核对的固定argv，不使用PTY/SDK重试。
func (p *PreparedApple) uploadBinary(ctx context.Context, g protocol.PublishGrant, onStart func(process.StartInfo) error) (toolResult, process.Result, error) {
	out, preparation, e := p.run(ctx, "apple_transport_prepare", g.AppIdentifier, &g, nil, nil)
	if e != nil || out.Status != "ready" || out.Transport == nil || out.Remote.Apple == nil || out.Remote.Apple.AppID != p.AppID {
		if e == nil {
			e = errUnknown
		}
		return toolResult{}, preparation, e
	}
	data, e := readMaterial(ctx, p.credentialCopy, 64<<10, true)
	if e != nil {
		return toolResult{}, process.Result{}, e
	}
	credential, e := appleCredential(data)
	if e != nil {
		return toolResult{}, process.Result{}, e
	}
	command := out.Transport
	expected := []string{"altool", "--upload-app", "--apiKey", credential.KeyID, "--apiIssuer", credential.IssuerID, "-t", "ios", "-f", p.artifactCopy, "-k", "100000"}
	rel, e := filepath.Rel(p.workDir, command.KeyDir)
	info, se := os.Lstat(command.KeyDir)
	if e != nil || rel == "." || strings.HasPrefix(rel, "..") || se != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0700 || !reflect.DeepEqual(command.Args, expected) {
		return toolResult{}, process.Result{}, errTools
	}
	key, e := readMaterial(ctx, filepath.Join(command.KeyDir, "AuthKey_"+credential.KeyID+".p8"), 64<<10, true)
	if e != nil || string(key) != credential.Key {
		return toolResult{}, process.Result{}, errTools
	}
	var stdout, stderr limitedOutput
	env := append(p.environment(""), "API_PRIVATE_KEYS_DIR="+command.KeyDir)
	result := process.Run(ctx, process.Command{Path: "/usr/bin/xcrun", Args: command.Args, Dir: p.workDir, Env: env, OnStart: onStart}, &stdout, &stderr)
	if result.CleanupFailed {
		p.mu.Lock()
		p.unsafe = true
		p.mu.Unlock()
		return toolResult{}, result, ErrCleanup
	}
	response := append(append([]byte{}, stdout.data.Bytes()...), stderr.data.Bytes()...)
	return appleTransportResult(g, p.AppID, result, response), result, nil
}

func appleTransportResult(g protocol.PublishGrant, appID string, result process.Result, response []byte) toolResult {
	out := toolResult{Status: "unknown", Reason: "transport_unconfirmed", Matches: []protocol.PublishMatch{}}
	matches := transportCorrelation.FindAllSubmatch(response, -1)
	if !result.Started || result.CleanupFailed || result.ExitCode != 0 || result.Reason != "" || !strings.Contains(string(response), "UPLOAD SUCCEEDED with no errors") || len(matches) != 1 || !uuidPattern.MatchString(string(matches[0][1])) {
		return out
	}
	digest := sha256.Sum256(response)
	out.Status = "confirmed"
	out.Reason = ""
	out.Remote.Apple = &protocol.AppleRemoteEvidence{AppID: appID, TransportID: string(matches[0][1]), ResponseSHA256: hex.EncodeToString(digest[:]), RequestSHA256: g.Apple.RequestSHA256, ActionConfirmed: true}
	now := time.Now().UTC()
	out.Remote.Apple.UploadedAt = &now
	return out
}
