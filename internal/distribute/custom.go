package distribute

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"mybuilds/internal/process"
	"mybuilds/internal/protocol"
)

var errCustomInput = errors.New("custom_input_invalid")
var errCustomResult = errors.New("custom_result_invalid")
var customEnvName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var customStates = []string{"uploaded", "processing", "submitted", "published", "failed", "unknown"}

type CustomOptions struct {
	Workspace, DataDir, WorkingDir, ResultFile             string            `json:"-"`
	Argv, QueryArgv                                        []string          `json:"-"`
	Environment                                            map[string]string `json:"-"`
	Params                                                 map[string]string `json:"-"`
	CommandDigest                                          string            `json:"-"`
	SecretValues                                           []string          `json:"-"`
	Credentials, ArtifactPath                              string            `json:"-"`
	AppIdentifier, VersionName, ArtifactID, ArtifactSHA256 string
	Number, ArtifactSize                                   int64
}
type PreparedCustom struct {
	mu                                            sync.Mutex
	options                                       CustomOptions
	workDir, resultPath, privateDir, artifactCopy string
	resultParent                                  string
	dirIdentity, parentIdentity, resultIdentity   os.FileInfo
	files                                         map[string]os.FileInfo
	used, closed, unsafe                          bool
}
type customArtifactInput struct {
	ID     string `json:"id"`
	Path   string `json:"path,omitempty"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}
type customInput struct {
	Schema              int                 `json:"schema"`
	IntentID            string              `json:"intent_id"`
	AuthorizationDigest string              `json:"authorization_digest"`
	Ref                 protocol.LeaseRef   `json:"ref"`
	AppIdentifier       string              `json:"app_identifier"`
	VersionName         string              `json:"version_name"`
	VersionCode         int64               `json:"version_code"`
	Artifact            customArtifactInput `json:"artifact"`
	ReportSealDigest    string              `json:"report_seal_digest,omitempty"`
	ReportIDs           []string            `json:"report_ids"`
	Params              map[string]string   `json:"params"`
	Channel             string              `json:"channel,omitempty"`
}
type customResult struct {
	Schema              int    `json:"schema"`
	IntentID            string `json:"intent_id"`
	AuthorizationDigest string `json:"authorization_digest"`
	AppIdentifier       string `json:"app_identifier"`
	ArtifactID          string `json:"artifact_id"`
	ArtifactSHA256      string `json:"artifact_sha256"`
	VersionName         string `json:"version_name"`
	VersionCode         int64  `json:"version_code"`
	Status              string `json:"status"`
	EvidenceCode        string `json:"evidence_code"`
	RemoteID            string `json:"remote_id"`
	ActionConfirmed     bool   `json:"action_confirmed"`
	RequestSHA256       string `json:"request_sha256,omitempty"`
	ResponseSHA256      string `json:"response_sha256,omitempty"`
}

// PrepareCustom只准备受限文件，不授权或执行任何发布命令。
func PrepareCustom(ctx context.Context, in CustomOptions) (*PreparedCustom, error) {
	return prepareCustom(ctx, in, true)
}
func prepareCustom(ctx context.Context, in CustomOptions, upload bool) (p *PreparedCustom, err error) {
	if ctx.Err() != nil || !customCommand(in.Argv) || !customRelative(in.WorkingDir, true) || !customRelative(in.ResultFile, false) || !appPattern.MatchString(in.AppIdentifier) || in.Number < 1 || in.ArtifactSize < 1 || in.ArtifactSize > 1<<30 || !digestPattern.MatchString(in.ArtifactSHA256) {
		return nil, errCustomInput
	}
	workspace, e := filepath.EvalSymlinks(in.Workspace)
	if e != nil {
		return nil, errCustomInput
	}
	workspace, e = filepath.Abs(workspace)
	if e != nil {
		return nil, errCustomInput
	}
	dir := filepath.Join(workspace, filepath.FromSlash(in.WorkingDir))
	actual, e := filepath.EvalSymlinks(dir)
	if e != nil || !customWithin(workspace, actual) {
		return nil, errCustomInput
	}
	info, e := os.Stat(actual)
	if e != nil || !info.IsDir() {
		return nil, errCustomInput
	}
	result := filepath.Join(actual, filepath.FromSlash(in.ResultFile))
	parent := filepath.Dir(result)
	canonical, e := filepath.EvalSymlinks(parent)
	if e != nil || canonical != parent || !customWithin(workspace, canonical) {
		return nil, errCustomInput
	}
	parentInfo, e := os.Lstat(parent)
	if e != nil || !parentInfo.IsDir() || parentInfo.Mode()&os.ModeSymlink != 0 {
		return nil, errCustomInput
	}
	if _, e := os.Lstat(result); !errors.Is(e, os.ErrNotExist) {
		return nil, errCustomInput
	}
	for name, value := range in.Environment {
		if !customEnvName.MatchString(name) || strings.HasPrefix(name, "MYBUILDS_") || strings.ContainsRune(value, 0) {
			return nil, errCustomInput
		}
	}
	if strings.ContainsRune(in.Credentials, 0) {
		return nil, errCustomInput
	}
	private, e := process.TemporaryDirectory(workspace, "mybuilds-custom-")
	if e != nil {
		return nil, errCustomInput
	}
	identity, e := os.Lstat(private)
	if e != nil {
		return nil, ErrCleanup
	}
	p = &PreparedCustom{options: in, workDir: actual, resultPath: result, resultParent: parent, privateDir: private, dirIdentity: identity, parentIdentity: parentInfo, files: map[string]os.FileInfo{}, artifactCopy: filepath.Join(private, "artifact.bin")}
	defer func() {
		if err != nil {
			if p.Close() != nil {
				err = ErrCleanup
			}
			p = nil
		}
	}()
	if upload {
		if err = p.copyArtifact(ctx); err != nil {
			return p, err
		}
	}
	// 显式复制数据，避免调用方并发修改本次命令或参数。
	p.options.Argv = slices.Clone(in.Argv)
	p.options.QueryArgv = slices.Clone(in.QueryArgv)
	p.options.Environment = cloneCustomMap(in.Environment)
	p.options.Params = cloneCustomMap(in.Params)
	p.options.SecretValues = slices.Clone(in.SecretValues)
	return p, nil
}
func cloneCustomMap(in map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		out[k] = v
	}
	return out
}
func customWithin(root, p string) bool {
	rel, e := filepath.Rel(root, p)
	return e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
func customRelative(value string, empty bool) bool {
	if value == "" {
		return empty
	}
	if filepath.IsAbs(value) || strings.ContainsAny(value, "\\\x00\r\n") || strings.Contains(value, "{{") || filepath.ToSlash(filepath.Clean(value)) != value {
		return false
	}
	for _, p := range strings.Split(value, "/") {
		if p == ".." || p == "" {
			return false
		}
	}
	return empty || value != "."
}
func customCommand(argv []string) bool {
	if len(argv) == 0 || len(argv) > 128 || strings.TrimSpace(argv[0]) == "" {
		return false
	}
	total := 0
	for _, arg := range argv {
		total += len(arg)
		if len(arg) > 4096 || strings.ContainsRune(arg, 0) || !utf8.ValidString(arg) || total > 64<<10 {
			return false
		}
	}
	return true
}
func (p *PreparedCustom) copyArtifact(ctx context.Context) error {
	source, e := openMaterial(p.options.ArtifactPath, false)
	if e != nil {
		return errCustomInput
	}
	defer source.Close()
	before, e := source.Stat()
	if e != nil || before.Size() != p.options.ArtifactSize {
		return errCustomInput
	}
	dest, e := os.OpenFile(p.artifactCopy, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return errCustomInput
	}
	identity, e := dest.Stat()
	if e != nil {
		dest.Close()
		return ErrCleanup
	}
	p.files[p.artifactCopy] = identity
	h := sha256.New()
	buffer := make([]byte, 64<<10)
	var copied int64
	for {
		if ctx.Err() != nil {
			dest.Close()
			return errCustomInput
		}
		n, rerr := source.Read(buffer)
		if n > 0 {
			copied += int64(n)
			if copied > p.options.ArtifactSize {
				dest.Close()
				return errCustomInput
			}
			if _, e := dest.Write(buffer[:n]); e != nil {
				dest.Close()
				return errCustomInput
			}
			h.Write(buffer[:n])
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			dest.Close()
			return errCustomInput
		}
	}
	syncErr := dest.Sync()
	closeErr := dest.Close()
	after, e := source.Stat()
	leaf, le := os.Lstat(p.options.ArtifactPath)
	if syncErr != nil || closeErr != nil || e != nil || le != nil || !leaf.Mode().IsRegular() || !os.SameFile(before, leaf) || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) || copied != p.options.ArtifactSize || hex.EncodeToString(h.Sum(nil)) != p.options.ArtifactSHA256 || ctx.Err() != nil {
		return errCustomInput
	}
	return nil
}
func (p *PreparedCustom) writeInput(in customInput) (string, error) {
	data, e := json.Marshal(in)
	if e != nil || len(data) > 64<<10 {
		return "", errCustomInput
	}
	name := filepath.Join(p.privateDir, "input.json")
	f, e := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return "", errCustomInput
	}
	info, e := f.Stat()
	if e != nil {
		f.Close()
		return "", ErrCleanup
	}
	p.files[name] = info
	_, e = f.Write(data)
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil || ce != nil {
		return "", errCustomInput
	}
	return name, nil
}
func (p *PreparedCustom) unchanged() bool {
	parent, e := os.Lstat(p.resultParent)
	private, e2 := os.Lstat(p.privateDir)
	return e == nil && e2 == nil && parent.Mode()&os.ModeSymlink == 0 && private.Mode()&os.ModeSymlink == 0 && os.SameFile(parent, p.parentIdentity) && os.SameFile(private, p.dirIdentity)
}

// UploadCustom一次调用原process.Run；任何不完整授权后证据保持unknown。
func UploadCustom(ctx context.Context, p *PreparedCustom, grant protocol.PublishGrant, onStart func(process.StartInfo) error) (receipt protocol.PublishReceipt, err error) {
	receipt = protocol.PublishReceipt{IntentID: grant.IntentID, AuthorizationDigest: grant.AuthorizationDigest, Ref: grant.Ref, Status: "unknown", EvidenceCode: "result_unconfirmed", MutationStage: "custom_upload", StopConfirmed: true}
	defer func() { receipt.Digest, _ = protocol.PublishReceiptDigest(receipt) }()
	if p == nil {
		return receipt, errCustomInput
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	digest, _ := protocol.PublishGrantDigest(grant)
	if digest != grant.AuthorizationDigest || grant.Action != "custom_upload" || grant.Custom == nil || grant.Custom.ResultSchemaVersion != 1 || !digestPattern.MatchString(p.options.CommandDigest) || grant.Custom.CommandDigest != p.options.CommandDigest || grant.Apple != nil || grant.AppIdentifier != p.options.AppIdentifier || grant.VersionName != p.options.VersionName || grant.VersionCode != p.options.Number || grant.ArtifactID != p.options.ArtifactID || grant.ArtifactSize != p.options.ArtifactSize || grant.ArtifactSHA256 != p.options.ArtifactSHA256 || grant.ReportIDs == nil {
		return receipt, errCustomInput
	}
	input := customInput{Schema: 1, IntentID: grant.IntentID, AuthorizationDigest: grant.AuthorizationDigest, Ref: grant.Ref, AppIdentifier: grant.AppIdentifier, VersionName: grant.VersionName, VersionCode: grant.VersionCode, Artifact: customArtifactInput{ID: grant.ArtifactID, Path: p.artifactCopy, Size: grant.ArtifactSize, SHA256: grant.ArtifactSHA256}, ReportSealDigest: grant.ReportSealDigest, ReportIDs: slices.Clone(grant.ReportIDs), Params: p.options.Params, Channel: p.options.Params["channel"]}
	result, run, e := p.runCustom(ctx, input, onStart)
	receipt.Started, receipt.StopConfirmed, receipt.CleanupFailed = run.Started, !run.CleanupFailed, run.CleanupFailed
	if errors.Is(e, ErrCleanup) {
		receipt.CleanupFailed = true
		receipt.StopConfirmed = false
	}
	if e != nil {
		return receipt, e
	}
	if !customResultMatches(result, grant) {
		return receipt, errCustomResult
	}
	receipt.Status, receipt.EvidenceCode = result.Status, result.EvidenceCode
	receipt.Remote.Custom = &protocol.CustomPublishEvidence{RemoteID: result.RemoteID, Lifecycle: result.Status, RequestSHA256: result.RequestSHA256, ResponseSHA256: result.ResponseSHA256, ActionConfirmed: result.ActionConfirmed}
	if receipt.Status == "unknown" {
		return receipt, errCustomResult
	}
	return receipt, nil
}

// 查询只执行原query_argv一次，输入没有artifact.path，不读取或下载原产物。
func QueryCustom(ctx context.Context, in CustomOptions, task protocol.PublishQueryTask) (response protocol.PublishQueryResult, returnErr error) {
	response = protocol.PublishQueryResult{ID: task.ID, Nonce: task.Nonce, Kind: task.Kind, BindingID: task.BindingID, IntentID: task.IntentID, NodeID: task.NodeID, SessionID: task.SessionID, ObservedAt: time.Now().UTC(), Matches: []protocol.PublishMatch{}, Reason: "observation_insufficient"}
	c := task.Custom
	if c == nil || task.Store != "custom" || task.Kind != "query" || time.Now().After(task.ExpiresAt) || in.ArtifactPath != "" || in.AppIdentifier != task.AppIdentifier {
		return response, errCustomInput
	}
	in.Argv = slices.Clone(in.QueryArgv)
	p, e := prepareCustom(ctx, in, false)
	if e != nil {
		return response, e
	}
	defer func() {
		if e := p.Close(); e != nil {
			returnErr = e
		}
	}()
	input := customInput{Schema: 1, IntentID: task.IntentID, AuthorizationDigest: c.AuthorizationDigest, Ref: c.OriginalRef, AppIdentifier: task.AppIdentifier, VersionName: c.VersionName, VersionCode: c.Number, Artifact: customArtifactInput{ID: c.ArtifactID, Size: c.ArtifactSize, SHA256: c.ArtifactSHA256}, ReportSealDigest: c.ReportSealDigest, ReportIDs: slices.Clone(c.ReportIDs), Params: in.Params, Channel: in.Params["channel"]}
	p.mu.Lock()
	result, _, e := p.runCustom(ctx, input, nil)
	p.mu.Unlock()
	if e != nil {
		return response, e
	}
	expected := protocol.PublishGrant{IntentID: task.IntentID, AuthorizationDigest: c.AuthorizationDigest, AppIdentifier: task.AppIdentifier, VersionName: c.VersionName, VersionCode: c.Number, ArtifactID: c.ArtifactID, ArtifactSize: c.ArtifactSize, ArtifactSHA256: c.ArtifactSHA256}
	if !customResultMatches(result, expected) || result.Status == "unknown" {
		return response, errCustomResult
	}
	response.Reason = ""
	response.ObservedLifecycle = result.Status
	response.Custom = &protocol.CustomQueryEvidence{IntentID: result.IntentID, AuthorizationDigest: result.AuthorizationDigest, ArtifactID: result.ArtifactID, ArtifactSHA256: result.ArtifactSHA256, VersionName: result.VersionName, VersionCode: result.VersionCode, EvidenceCode: result.EvidenceCode, Remote: protocol.CustomPublishEvidence{RemoteID: result.RemoteID, Lifecycle: result.Status, RequestSHA256: result.RequestSHA256, ResponseSHA256: result.ResponseSHA256, ActionConfirmed: result.ActionConfirmed}}
	response.Matches = []protocol.PublishMatch{{VersionCodes: []int64{result.VersionCode}, Lifecycle: result.Status, Custom: &response.Custom.Remote}}
	return response, nil
}

func (p *PreparedCustom) runCustom(ctx context.Context, input customInput, onStart func(process.StartInfo) error) (result customResult, run process.Result, err error) {
	if p.used || p.closed || p.unsafe || ctx.Err() != nil || !p.unchanged() {
		return result, run, errCustomInput
	}
	p.used = true
	inputPath, e := p.writeInput(input)
	if e != nil {
		return result, run, e
	}
	env := cloneCustomMap(p.options.Environment)
	env["HOME"], env["TMPDIR"] = p.privateDir, p.privateDir
	env["MYBUILDS_PUBLISH_INPUT"], env["MYBUILDS_PUBLISH_RESULT"] = inputPath, p.resultPath
	if p.options.Credentials != "" {
		env["MYBUILDS_PUBLISH_CREDENTIAL"] = p.options.Credentials
	}
	executable, e := customExecutable(p.options.Argv[0], p.workDir, env)
	if e != nil {
		return result, run, errCustomInput
	}
	values := []string{}
	for key, value := range env {
		values = append(values, key+"="+value)
	}
	slices.Sort(values)
	output := &limitedOutput{}
	started := func(info process.StartInfo) error {
		if ctx.Err() != nil || !p.unchanged() {
			return errCustomInput
		}
		if onStart != nil {
			return onStart(info)
		}
		return nil
	}
	run = process.Run(ctx, process.Command{Path: executable, Args: slices.Clone(p.options.Argv[1:]), Dir: p.workDir, Env: values, OnStart: started}, output, output)
	if run.CleanupFailed || !p.unchanged() {
		p.unsafe = true
		return result, run, ErrCleanup
	}
	info, e := os.Lstat(p.resultPath)
	if e == nil {
		if !info.Mode().IsRegular() {
			p.unsafe = true
			return result, run, ErrCleanup
		}
		p.resultIdentity = info
	}
	data, e := readMaterial(ctx, p.resultPath, 64<<10, true)
	if e != nil {
		return result, run, errCustomResult
	}
	secrets := append(slices.Clone(p.options.SecretValues), p.options.Credentials)
	if !customJSONBounds(data) || strictJSON(data, &result) != nil {
		return result, run, errCustomResult
	}
	for _, secret := range secrets {
		if ctx.Err() != nil {
			return result, run, errCustomResult
		}
		if customContainsSecret(data, secret) || customContainsSecret(output.data.Bytes(), secret) || customDecodedSecrets(result, secret) {
			return result, run, errCustomResult
		}
	}
	if run.Reason != "" || run.ExitCode != 0 || !run.Started {
		return result, run, errCustomResult
	}
	return result, run, nil
}
func customExecutable(name, workspace string, env map[string]string) (string, error) {
	if filepath.IsAbs(name) {
		return exec.LookPath(name)
	}
	if strings.ContainsAny(name, "/\\") {
		return exec.LookPath(filepath.Join(workspace, name))
	}
	for _, dir := range filepath.SplitList(env["PATH"]) {
		if filepath.IsAbs(dir) {
			if p, e := exec.LookPath(filepath.Join(dir, name)); e == nil {
				return p, nil
			}
		}
	}
	return "", errCustomInput
}
func customContainsSecret(data []byte, secret string) bool {
	return secret != "" && bytes.Contains(data, []byte(secret))
}
func customDecodedSecrets(result customResult, secret string) bool {
	if secret == "" {
		return false
	}
	for _, v := range []string{result.IntentID, result.AuthorizationDigest, result.AppIdentifier, result.ArtifactID, result.ArtifactSHA256, result.VersionName, result.Status, result.EvidenceCode, result.RemoteID, result.RequestSHA256, result.ResponseSHA256} {
		if strings.Contains(v, secret) {
			return true
		}
	}
	return false
}
func customResultMatches(result customResult, grant protocol.PublishGrant) bool {
	if result.Schema != 1 || result.IntentID != grant.IntentID || result.AuthorizationDigest != grant.AuthorizationDigest || result.AppIdentifier != grant.AppIdentifier || result.ArtifactID != grant.ArtifactID || result.ArtifactSHA256 != grant.ArtifactSHA256 || result.VersionName != grant.VersionName || result.VersionCode != grant.VersionCode || !slices.Contains(customStates, result.Status) || len(result.RemoteID) > 128 || strings.ContainsAny(result.RemoteID, "\x00\r\n") || result.RequestSHA256 != "" && !digestPattern.MatchString(result.RequestSHA256) || result.ResponseSHA256 != "" && !digestPattern.MatchString(result.ResponseSHA256) {
		return false
	}
	for _, r := range result.RemoteID {
		if unicode.IsControl(r) {
			return false
		}
	}
	if !utf8.ValidString(result.RemoteID) {
		return false
	}
	if result.Status == "failed" {
		return slices.Contains([]string{"confirmed_not_sent", "remote_rejected"}, result.EvidenceCode) && !result.ActionConfirmed
	}
	if result.Status == "unknown" {
		return result.EvidenceCode == "result_unconfirmed" && !result.ActionConfirmed
	}
	return result.RemoteID != "" && result.ActionConfirmed && slices.Contains([]string{"remote_receipt", "remote_state"}, result.EvidenceCode)
}
func customJSONBounds(data []byte) bool {
	if len(data) > 64<<10 || !utf8.Valid(data) {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	count := 0
	var visit func(int) bool
	visit = func(depth int) bool {
		count++
		if count > 4096 || depth > 16 {
			return false
		}
		token, e := decoder.Token()
		if e != nil || token == nil {
			return false
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return true
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for decoder.More() {
				k, e := decoder.Token()
				key, ok := k.(string)
				if e != nil || !ok || seen[key] {
					return false
				}
				seen[key] = true
				if !visit(depth + 1) {
					return false
				}
			}
			end, e := decoder.Token()
			return e == nil && end == json.Delim('}')
		case '[':
			for decoder.More() {
				if !visit(depth + 1) {
					return false
				}
			}
			end, e := decoder.Token()
			return e == nil && end == json.Delim(']')
		}
		return false
	}
	if !visit(0) {
		return false
	}
	_, e := decoder.Token()
	return e == io.EOF
}

func (p *PreparedCustom) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	if p.unsafe || !p.unchanged() {
		return ErrCleanup
	}
	deadline := time.Now().Add(15 * time.Second)
	if p.resultIdentity != nil {
		current, e := os.Lstat(p.resultPath)
		if e != nil || !current.Mode().IsRegular() || !os.SameFile(current, p.resultIdentity) || os.Remove(p.resultPath) != nil {
			return ErrCleanup
		}
	}
	for name, identity := range p.files {
		if time.Now().After(deadline) {
			return ErrCleanup
		}
		current, e := os.Lstat(name)
		if e != nil || !current.Mode().IsRegular() || !os.SameFile(current, identity) || os.Remove(name) != nil {
			return ErrCleanup
		}
	}
	if os.Remove(p.privateDir) != nil {
		return ErrCleanup
	}
	p.closed = true
	return nil
}
