package distribute

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"mybuilds/internal/process"
	"mybuilds/internal/protocol"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

//go:embed fastlane/Gemfile fastlane/Gemfile.lock fastlane/Fastfile fastlane/*.rb
var toolSource embed.FS
var ErrCleanup = errors.New("publish_cleanup_failed")
var errTools = errors.New("publish_tools_unverified")
var errUnknown = errors.New("publish_unknown")

const outputLimit = 64 << 10

var toolNames = []string{"Gemfile", "Gemfile.lock", "Fastfile", "channel.rb", "google_play.rb", "apple.rb"}

type prepared struct {
	mu                                                     sync.Mutex
	workDir, toolDir, gemDir, credentialCopy, artifactCopy string
	identity                                               os.FileInfo
	lockDigest                                             string
	closed                                                 bool
	unsafe                                                 bool
	used                                                   map[string]bool
}

func newPrepared(ctx context.Context, toolDir, dataDir string) (*prepared, error) {
	if ctx.Err() != nil {
		return nil, errTools
	}
	h := sha256.New()
	for _, n := range toolNames {
		want, e := toolSource.ReadFile("fastlane/" + n)
		if e != nil {
			return nil, errTools
		}
		got, e := readMaterial(ctx, filepath.Join(toolDir, n), 2<<20, false)
		if e != nil || !bytes.Equal(want, got) {
			return nil, errTools
		}
		h.Write(got)
	}
	dir, e := os.MkdirTemp(dataDir, "publish-")
	if e != nil {
		return nil, errTools
	}
	info, e := os.Lstat(dir)
	if e != nil {
		os.Remove(dir)
		return nil, errTools
	}
	p := &prepared{workDir: dir, toolDir: filepath.Join(dir, "tools"), gemDir: filepath.Join(toolDir, "gems"), identity: info, lockDigest: hex.EncodeToString(h.Sum(nil)), used: map[string]bool{}}
	if os.Mkdir(p.toolDir, 0700) != nil {
		p.Close()
		return nil, errTools
	}
	for _, n := range toolNames {
		data, _ := toolSource.ReadFile("fastlane/" + n)
		if os.WriteFile(filepath.Join(p.toolDir, n), data, 0600) != nil {
			p.Close()
			return nil, errTools
		}
	}
	if os.Mkdir(filepath.Join(dir, "home"), 0700) != nil {
		p.Close()
		return nil, errTools
	}
	return p, nil
}
func (p *prepared) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.unsafe {
		return ErrCleanup
	}
	if p.closed {
		return nil
	}
	info, e := os.Lstat(p.workDir)
	if e != nil || info.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, p.identity) {
		return ErrCleanup
	}
	if os.RemoveAll(p.workDir) != nil {
		return ErrCleanup
	}
	p.closed = true
	return nil
}
func (p *prepared) write(name string, data []byte) (string, error) {
	path := filepath.Join(p.workDir, name)
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return "", errMaterial
	}
	_, e = f.Write(data)
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil || ce != nil {
		return "", errMaterial
	}
	return path, nil
}

type limitedOutput struct{ data bytes.Buffer }

func (w *limitedOutput) Write(b []byte) (int, error) {
	if w.data.Len()+len(b) > outputLimit {
		return 0, errUnknown
	}
	return w.data.Write(b)
}
func (p *prepared) environment(job string) []string {
	values := map[string]string{"HOME": filepath.Join(p.workDir, "home"), "TMPDIR": p.workDir, "LANG": "en_US.UTF-8", "BUNDLE_GEMFILE": filepath.Join(p.toolDir, "Gemfile"), "BUNDLE_PATH": p.gemDir, "BUNDLE_FROZEN": "true", "FASTLANE_SKIP_UPDATE_CHECK": "1", "FASTLANE_HIDE_CHANGELOG": "1", "FASTLANE_DISABLE_COLORS": "1", "CI": "1", "MYBUILDS_PUBLISH_TOOLS": p.toolDir, "MYBUILDS_PUBLISH_JOB": job}
	for _, k := range []string{"PATH", "JAVA_HOME", "DEVELOPER_DIR"} {
		if v := os.Getenv(k); v != "" {
			values[k] = v
		}
	}
	env := make([]string, 0, len(values))
	for k, v := range values {
		env = append(env, k+"="+v)
	}
	return env
}

type toolJob struct {
	VersionName string                     `json:"version_name"`
	Number      int64                      `json:"number"`
	Previous    []protocol.PublishReceipt  `json:"previous"`
	Submit      bool                       `json:"submit"`
	Automatic   bool                       `json:"automatic"`
	Operation   string                     `json:"operation"`
	Credential  string                     `json:"credential"`
	Artifact    string                     `json:"artifact,omitempty"`
	Result      string                     `json:"result"`
	Token       string                     `json:"token,omitempty"`
	Grant       *protocol.PublishGrant     `json:"grant,omitempty"`
	Query       *protocol.PublishQueryTask `json:"query,omitempty"`
	App         string                     `json:"app"`
}
type appleTransportCommand struct {
	Args   []string `json:"args"`
	KeyDir string   `json:"key_dir"`
}
type toolResult struct {
	Transport *appleTransportCommand              `json:"transport,omitempty"`
	Next      *protocol.ApplePublishAuthorization `json:"next,omitempty"`
	Status    string                              `json:"status"`
	Reason    string                              `json:"reason"`
	Remote    protocol.PublishRemoteEvidence      `json:"remote"`
	Matches   []protocol.PublishMatch             `json:"matches"`
}

func (p *prepared) run(ctx context.Context, operation, app string, grant *protocol.PublishGrant, query *protocol.PublishQueryTask, onStart func(process.StartInfo) error) (toolResult, process.Result, error) {
	return p.invoke(ctx, toolJob{Operation: operation, Credential: p.credentialCopy, Artifact: p.artifactCopy, Token: filepath.Join(p.workDir, "token.json"), Grant: grant, Query: query, App: app, Previous: []protocol.PublishReceipt{}}, onStart)
}
func (p *prepared) invoke(ctx context.Context, job toolJob, onStart func(process.StartInfo) error) (toolResult, process.Result, error) {
	var out toolResult
	var result process.Result
	if ctx.Err() != nil {
		return out, result, errUnknown
	}
	path, e := exec.LookPath("bundle")
	if e != nil {
		return out, result, errTools
	}
	nonce, e := os.MkdirTemp(p.workDir, "command-")
	if e != nil {
		return out, result, errMaterial
	}
	defer func() {
		if !result.CleanupFailed {
			os.RemoveAll(nonce)
		}
	}()
	if os.Mkdir(filepath.Join(nonce, "fastlane"), 0700) != nil {
		return out, result, errTools
	}
	fastfile, _ := toolSource.ReadFile("fastlane/Fastfile")
	if os.WriteFile(filepath.Join(nonce, "fastlane", "Fastfile"), fastfile, 0600) != nil {
		return out, result, errTools
	}
	target := filepath.Join(nonce, "result.json")
	job.Result = target
	data, _ := json.Marshal(job)
	jobPath := filepath.Join(nonce, "job.json")
	if os.WriteFile(jobPath, data, 0600) != nil {
		return out, result, errMaterial
	}
	var stdout, stderr limitedOutput
	result = process.Run(ctx, process.Command{Path: path, Args: []string{"exec", "fastlane", "mybuilds_channel"}, Dir: nonce, Env: p.environment(jobPath), OnStart: onStart}, &stdout, &stderr)
	if result.CleanupFailed {
		p.mu.Lock()
		p.unsafe = true
		p.mu.Unlock()
		return out, result, ErrCleanup
	}
	if result.ExitCode != 0 || result.Reason != "" {
		return out, result, errUnknown
	}
	data, e = readMaterial(ctx, target, outputLimit, true)
	if e != nil || strictJSON(data, &out) != nil {
		return out, result, errUnknown
	}
	return out, result, nil
}
func (p *prepared) once(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.unsafe || p.used[id] {
		return false
	}
	p.used[id] = true
	return true
}
func receipt(grant protocol.PublishGrant, result process.Result, out toolResult) protocol.PublishReceipt {
	r := protocol.PublishReceipt{IntentID: grant.IntentID, AuthorizationDigest: grant.AuthorizationDigest, Ref: grant.Ref, Status: "unknown", EvidenceCode: "result_unconfirmed", MutationStage: grant.Action, Started: result.Started, StopConfirmed: !result.CleanupFailed, CleanupFailed: result.CleanupFailed, Remote: out.Remote}
	confirmed := out.Status == "confirmed"
	if grant.Apple == nil {
		confirmed = confirmed && out.Remote.BundleAccepted && out.Remote.TrackAccepted && out.Remote.CommitAccepted && out.Remote.BundleSHA256 == grant.ArtifactSHA256 && out.Remote.VersionCode == grant.VersionCode && out.Remote.ReleaseName == grant.ReleaseName && out.Remote.Track == grant.Track
	} else {
		confirmed = confirmed && out.Remote.Apple != nil && out.Remote.Apple.ActionConfirmed && out.Remote.Apple.RequestSHA256 == grant.Apple.RequestSHA256 && digestPattern.MatchString(out.Remote.Apple.ResponseSHA256)
		if grant.Apple.Action == "upload_binary" {
			confirmed = confirmed && out.Remote.Apple != nil && uuidPattern.MatchString(out.Remote.Apple.TransportID)
		}
	}
	if result.Started && !result.CleanupFailed && result.ExitCode == 0 && result.Reason == "" && confirmed {
		r.Status = "confirmed"
		r.EvidenceCode = "remote_receipt"
	}
	r.Digest, _ = protocol.PublishReceiptDigest(r)
	return r
}
func queryResult(task protocol.PublishQueryTask, out toolResult, lock string) protocol.PublishQueryResult {
	return protocol.PublishQueryResult{ID: task.ID, Nonce: task.Nonce, Kind: task.Kind, BindingID: task.BindingID, IntentID: task.IntentID, NodeID: task.NodeID, SessionID: task.SessionID, ObservedAt: time.Now().UTC(), ObservedLifecycle: out.Status, Matches: out.Matches, ToolLockDigest: lock, Reason: out.Reason}
}
