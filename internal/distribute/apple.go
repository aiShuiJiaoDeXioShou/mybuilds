package distribute

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"mybuilds/internal/process"
	"mybuilds/internal/protocol"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type AppleOptions struct {
	BundleDir, DataDir, CredentialFile             string
	AppIdentifier, VersionName, DistributionTeamID string
	Number                                         int64
	ArtifactPath, ArtifactSHA256                   string
	ArtifactSize                                   int64
}
type PreparedApple struct {
	*prepared
	options AppleOptions
	AppID   string
}

// plistValues只读取实际主app的标量；不执行用户脚本或从issuer猜签名Team。
func plistValues(data []byte) (map[string]string, error) {
	d := xml.NewDecoder(strings.NewReader(string(data)))
	out := map[string]string{}
	seen := map[string]bool{}
	depth := 0
	key := ""
	rootDict := false
	for {
		token, e := d.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, errArtifact
		}
		switch element := token.(type) {
		case xml.StartElement:
			depth++
			if depth == 2 && element.Name.Local == "dict" {
				if rootDict {
					return nil, errArtifact
				}
				rootDict = true
			}
			if depth != 3 {
				continue
			}
			if element.Name.Local == "key" {
				if key != "" || d.DecodeElement(&key, &element) != nil || seen[key] {
					return nil, errArtifact
				}
				seen[key] = true
				depth--
				continue
			}
			if key == "" {
				return nil, errArtifact
			}
			switch element.Name.Local {
			case "string", "integer", "date":
				var value string
				if d.DecodeElement(&value, &element) != nil {
					return nil, errArtifact
				}
				out[key] = value
				depth--
			case "true", "false":
				out[key] = element.Name.Local
			}
			key = ""
		case xml.EndElement:
			depth--
		}
	}
	if !rootDict || depth != 0 || key != "" {
		return nil, errArtifact
	}
	return out, nil
}
func (p *prepared) validateIPA(ctx context.Context, in AppleOptions) error {
	if runtime.GOOS != "darwin" || zipStructure(p.artifactCopy) != nil {
		return errArtifact
	}
	z, e := zip.OpenReader(p.artifactCopy)
	if e != nil {
		return errArtifact
	}
	defer z.Close()
	root := filepath.Join(p.workDir, "ipa")
	if os.Mkdir(root, 0700) != nil {
		return errArtifact
	}
	mainApps := map[string]bool{}
	for _, f := range z.File {
		parts := strings.Split(f.Name, "/")
		if len(parts) >= 2 && parts[0] == "Payload" && strings.HasSuffix(parts[1], ".app") {
			mainApps[parts[1]] = true
		}
		target := filepath.Join(root, filepath.FromSlash(f.Name))
		if f.FileInfo().IsDir() {
			if os.MkdirAll(target, 0700) != nil {
				return errArtifact
			}
			continue
		}
		if os.MkdirAll(filepath.Dir(target), 0700) != nil {
			return errArtifact
		}
		to, e := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return errArtifact
		}
		from, e := f.Open()
		if e != nil {
			to.Close()
			return errArtifact
		}
		n, e := io.Copy(to, io.LimitReader(contextReader{ctx, from}, int64(f.UncompressedSize64)+1))
		from.Close()
		ce := to.Close()
		if e != nil || ce != nil || n != int64(f.UncompressedSize64) {
			return errArtifact
		}
		if f.Mode()&0111 != 0 {
			if os.Chmod(target, 0700) != nil {
				return errArtifact
			}
		}
	}
	if len(mainApps) != 1 {
		return errArtifact
	}
	var app string
	for name := range mainApps {
		app = filepath.Join(root, "Payload", name)
	}
	plist, e := p.tool(ctx, "/usr/bin/plutil", "-convert", "xml1", "-o", "-", filepath.Join(app, "Info.plist"))
	if e != nil {
		return errArtifact
	}
	v, e := plistValues(plist)
	if e != nil || v["CFBundleIdentifier"] != in.AppIdentifier || v["CFBundleShortVersionString"] != in.VersionName || v["CFBundleVersion"] != strconv.FormatInt(in.Number, 10) {
		return errArtifact
	}
	if _, e = p.tool(ctx, "/usr/bin/codesign", "--verify", "--deep", "--strict", "-R=anchor apple generic", app); e != nil {
		return errArtifact
	}
	entitlements, e := p.tool(ctx, "/usr/bin/codesign", "-d", "--entitlements", ":-", app)
	if e != nil {
		return errArtifact
	}
	index := strings.Index(string(entitlements), "<?xml")
	if index < 0 {
		return errArtifact
	}
	v, e = plistValues(entitlements[index:])
	if e != nil || in.DistributionTeamID == "" || v["com.apple.developer.team-identifier"] != in.DistributionTeamID || v["application-identifier"] != in.DistributionTeamID+"."+in.AppIdentifier || v["get-task-allow"] == "true" {
		return errArtifact
	}
	return nil
}
func PrepareApple(ctx context.Context, in AppleOptions) (*PreparedApple, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("apple_platform_unsupported")
	}
	if !appPattern.MatchString(in.AppIdentifier) || in.VersionName == "" || in.Number < 1 || in.DistributionTeamID == "" {
		return nil, errors.New("apple_app_invalid")
	}
	p, e := newPrepared(ctx, in.BundleDir, in.DataDir)
	if e != nil {
		return nil, e
	}
	fail := func(err error) (*PreparedApple, error) {
		if p.Close() != nil {
			return nil, ErrCleanup
		}
		return nil, err
	}
	data, e := readMaterial(ctx, in.CredentialFile, 64<<10, true)
	if e != nil {
		return fail(errors.New("apple_credentials_invalid"))
	}
	if _, e = appleCredential(data); e != nil {
		return fail(errors.New("apple_credentials_invalid"))
	}
	p.credentialCopy, e = p.write("credential.json", data)
	if e != nil {
		return fail(e)
	}
	if e = p.copyArtifact(ctx, in.ArtifactPath, in.ArtifactSHA256, in.ArtifactSize, ".ipa"); e != nil {
		return fail(e)
	}
	if e = p.validateIPA(ctx, in); e != nil {
		return fail(e)
	}
	out, _, e := p.run(ctx, "apple_preflight", in.AppIdentifier, nil, nil, nil)
	if e != nil || out.Status != "ready" || out.Remote.Apple == nil || out.Remote.Apple.AppID == "" {
		return fail(errors.New("apple_credentials_invalid"))
	}
	return &PreparedApple{p, in, out.Remote.Apple.AppID}, nil
}

// RequestSHA256由真正已核对AppID/原包生成，供Agent在申请授权之前冻结。
func (p *PreparedApple) RequestSHA256(a protocol.ApplePublishAuthorization) (string, error) {
	var body any
	version := a.AppStoreVersionID
	build := a.BuildID
	submission := a.ReviewSubmissionID
	switch a.Action {
	case "upload_binary":
		body = map[string]any{"app": p.options.AppIdentifier, "version": p.options.VersionName, "number": p.options.Number, "size": p.options.ArtifactSize, "sha256": p.options.ArtifactSHA256}
	case "select_build":
		body = map[string]any{"data": map[string]any{"type": "builds", "id": build}}
	case "set_release_policy":
		policy := "MANUAL"
		if a.AutomaticRelease {
			policy = "AFTER_APPROVAL"
		}
		body = map[string]any{"data": map[string]any{"type": "appStoreVersions", "id": version, "attributes": map[string]any{"releaseType": policy}}}
	case "create_review":
		body = map[string]any{"data": map[string]any{"type": "reviewSubmissions", "attributes": map[string]any{"platform": "IOS"}, "relationships": map[string]any{"app": map[string]any{"data": map[string]any{"type": "apps", "id": p.AppID}}}}}
	case "add_review_item":
		body = map[string]any{"data": map[string]any{"type": "reviewSubmissionItems", "relationships": map[string]any{"reviewSubmission": map[string]any{"data": map[string]any{"type": "reviewSubmissions", "id": submission}}, "appStoreVersion": map[string]any{"data": map[string]any{"type": "appStoreVersions", "id": version}}}}}
	case "submit_review":
		body = map[string]any{"data": map[string]any{"type": "reviewSubmissions", "id": submission, "attributes": map[string]any{"submitted": true}}}
	default:
		return "", errors.New("apple_action_invalid")
	}
	data, _ := json.Marshal(body)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
func UploadApple(ctx context.Context, p *PreparedApple, g protocol.PublishGrant, onStart func(process.StartInfo) error) (protocol.PublishReceipt, error) {
	var zero protocol.PublishReceipt
	if p == nil || p.prepared == nil || g.Apple == nil {
		return zero, errArtifact
	}
	d, _ := protocol.PublishGrantDigest(g)
	expected, e := p.RequestSHA256(*g.Apple)
	if e != nil || d != g.AuthorizationDigest || g.Apple.RequestSHA256 != expected || g.Action != g.Apple.Action || g.AppIdentifier != p.options.AppIdentifier || g.VersionCode != p.options.Number || g.VersionName != p.options.VersionName || g.ArtifactSHA256 != p.options.ArtifactSHA256 || g.ArtifactSize != p.options.ArtifactSize || g.IntentID == "" {
		return zero, errors.New("apple_action_invalid")
	}
	if g.Apple.Action != "upload_binary" && (!g.Apple.SubmitForReview || (g.Apple.AutomaticRelease && !g.Apple.SubmitForReview)) {
		return zero, errors.New("apple_action_invalid")
	}
	if p.verifyArtifact(ctx, g.ArtifactSHA256, g.ArtifactSize) != nil {
		return zero, errArtifact
	}
	if !p.once(g.IntentID) {
		return zero, errUnknown
	}
	var out toolResult
	var result process.Result
	if g.Apple.Action == "upload_binary" {
		out, result, e = p.uploadBinary(ctx, g, onStart)
	} else {
		out, result, e = p.run(ctx, "apple_upload", g.AppIdentifier, &g, nil, onStart)
	}
	r := receipt(g, result, out)
	if r.Status != "confirmed" && e == nil {
		e = errUnknown
	}
	return r, e
}
func QueryApple(ctx context.Context, in AppleOptions, task protocol.PublishQueryTask) (response protocol.PublishQueryResult, returnErr error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if runtime.GOOS != "darwin" {
		return protocol.PublishQueryResult{}, errors.New("apple_platform_unsupported")
	}
	if (task.Kind != "doctor" && task.Kind != "query") || task.Store != "app_store" || task.AppIdentifier != in.AppIdentifier || !appPattern.MatchString(in.AppIdentifier) || time.Now().After(task.ExpiresAt) {
		return protocol.PublishQueryResult{}, errors.New("apple_app_invalid")
	}
	p, e := newPrepared(ctx, in.BundleDir, in.DataDir)
	if e != nil {
		return protocol.PublishQueryResult{}, e
	}
	defer func() {
		if ce := p.Close(); ce != nil {
			returnErr = ce
		}
	}()
	data, e := readMaterial(ctx, in.CredentialFile, 64<<10, true)
	if e != nil {
		return protocol.PublishQueryResult{}, e
	}
	if _, e = appleCredential(data); e != nil {
		return protocol.PublishQueryResult{}, e
	}
	p.credentialCopy, e = p.write("credential.json", data)
	if e != nil {
		return protocol.PublishQueryResult{}, e
	}
	operation := "apple_query"
	if task.Kind == "doctor" {
		if _, te := p.tool(ctx, "/usr/bin/xcrun", "--find", "altool"); te != nil {
			return protocol.PublishQueryResult{}, errTools
		}
		operation = "apple_preflight"
	}
	out, _, e := p.run(ctx, operation, in.AppIdentifier, nil, &task, nil)
	result := queryResult(task, out, p.lockDigest)
	if result.Matches == nil {
		result.Matches = []protocol.PublishMatch{}
	}
	if task.Kind == "doctor" && out.Status == "ready" {
		result.DoctorChecks = []protocol.ToolCheck{{Name: "fastlane", Status: "passed", Version: "2.240.1"}, {Name: "apple_transport", Status: "passed"}, {Name: "app_store_credentials", Status: "passed"}, {Name: "app_store_application", Status: "passed"}}
	}
	return result, e
}

// NextAction只读已确认原链并规划下一次请求，绝不授予执行权或重做旧动作。
func (p *PreparedApple) NextAction(ctx context.Context, previous []protocol.PublishReceipt, submit, automatic bool) (*protocol.ApplePublishAuthorization, error) {
	if p == nil || p.prepared == nil || automatic && !submit || len(previous) > 6 {
		return nil, errors.New("apple_action_invalid")
	}
	seen := map[string]bool{}
	for index, r := range previous {
		if (index == 0 && r.MutationStage != "upload_binary") || seen[r.MutationStage] || r.IntentID == "" || r.Ref != previous[0].Ref {
			return nil, errUnknown
		}
		seen[r.MutationStage] = true
		digest, e := protocol.PublishReceiptDigest(r)
		if e != nil || r.Digest != digest || r.Status != "confirmed" || r.CleanupFailed || !r.StopConfirmed || r.Remote.Apple == nil || r.Remote.Apple.AppID != p.AppID || !r.Remote.Apple.ActionConfirmed || !digestPattern.MatchString(r.Remote.Apple.RequestSHA256) || !digestPattern.MatchString(r.Remote.Apple.ResponseSHA256) {
			return nil, errUnknown
		}
	}
	if len(previous) == 0 {
		a := &protocol.ApplePublishAuthorization{Action: "upload_binary", SubmitForReview: submit, AutomaticRelease: automatic}
		a.RequestSHA256, _ = p.RequestSHA256(*a)
		return a, nil
	}
	if !submit {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	job := toolJob{Operation: "apple_next", Credential: p.credentialCopy, Result: "", Token: filepath.Join(p.workDir, "token.json"), App: p.options.AppIdentifier, VersionName: p.options.VersionName, Number: p.options.Number, Previous: previous, Submit: submit, Automatic: automatic}
	out, _, e := p.invoke(ctx, job, nil)
	if e != nil {
		return nil, e
	}
	if out.Status != "ready" {
		if out.Reason == "apple_processing_timeout" {
			return nil, errors.New("apple_processing_timeout")
		}
		return nil, errors.New("apple_precondition_failed")
	}
	if out.Next == nil {
		return nil, nil
	}
	if out.Next.Action == "upload_binary" {
		return nil, errUnknown
	}
	out.Next.SubmitForReview = submit
	out.Next.AutomaticRelease = automatic
	out.Next.RequestSHA256, e = p.RequestSHA256(*out.Next)
	return out.Next, e
}
