package distribute

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"mybuilds/internal/process"
	"mybuilds/internal/protocol"
	"strings"
	"time"
)

type GooglePlayOptions struct {
	BundleDir, Bundletool, DataDir, CredentialFile      string
	AppIdentifier, VersionName, UploadCertificateSHA256 string
	Number                                              int64
	ArtifactPath, ArtifactSHA256                        string
	ArtifactSize                                        int64
}
type PreparedGooglePlay struct {
	*prepared
	options GooglePlayOptions
}

func PrepareGooglePlay(ctx context.Context, in GooglePlayOptions) (*PreparedGooglePlay, error) {
	if !appPattern.MatchString(in.AppIdentifier) || in.VersionName == "" || in.Number < 1 || in.Number > 2100000000 {
		return nil, errors.New("play_app_invalid")
	}
	p, e := newPrepared(ctx, in.BundleDir, in.DataDir)
	if e != nil {
		return nil, e
	}
	fail := func(err error) (*PreparedGooglePlay, error) {
		if p.Close() != nil {
			return nil, ErrCleanup
		}
		return nil, err
	}
	data, e := readMaterial(ctx, in.CredentialFile, 1<<20, true)
	if e != nil {
		return fail(errors.New("play_credentials_invalid"))
	}
	if _, e = googleCredential(data); e != nil {
		return fail(errors.New("play_credentials_invalid"))
	}
	p.credentialCopy, e = p.write("credential.json", data)
	if e != nil {
		return fail(e)
	}
	if e = p.copyArtifact(ctx, in.ArtifactPath, in.ArtifactSHA256, in.ArtifactSize, ".aab"); e != nil {
		return fail(e)
	}
	if e = p.validateAAB(ctx, in); e != nil {
		return fail(e)
	}
	out, _, e := p.run(ctx, "play_preflight", in.AppIdentifier, nil, nil, nil)
	if e != nil || out.Status != "ready" {
		return fail(errors.New("play_credentials_invalid"))
	}
	return &PreparedGooglePlay{p, in}, nil
}
func UploadGooglePlay(ctx context.Context, p *PreparedGooglePlay, g protocol.PublishGrant, onStart func(process.StartInfo) error) (protocol.PublishReceipt, error) {
	var zero protocol.PublishReceipt
	if p == nil || p.prepared == nil {
		return zero, errArtifact
	}
	d, _ := protocol.PublishGrantDigest(g)
	if d != g.AuthorizationDigest || g.Apple != nil || g.Action != "upload" || g.AppIdentifier != p.options.AppIdentifier || g.VersionCode != p.options.Number || g.VersionName != p.options.VersionName || g.ArtifactSHA256 != p.options.ArtifactSHA256 || g.ArtifactSize != p.options.ArtifactSize || g.IntentID == "" || g.ReleaseName == "" || g.Track == "" || (g.ReleaseStatus != "draft" && g.ReleaseStatus != "completed") || g.ChangesNotSentForReview {
		return zero, errors.New("play_app_invalid")
	}
	if p.verifyArtifact(ctx, g.ArtifactSHA256, g.ArtifactSize) != nil {
		return zero, errArtifact
	}
	if !p.once("google_upload") {
		return zero, errUnknown
	}
	out, result, e := p.run(ctx, "play_upload", g.AppIdentifier, &g, nil, onStart)
	r := googleReceipt(g, result, out)
	if r.Status != "uploaded" && e == nil {
		e = errUnknown
	}
	return r, e
}
func QueryGooglePlay(ctx context.Context, in GooglePlayOptions, task protocol.PublishQueryTask) (response protocol.PublishQueryResult, returnErr error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if (task.Kind != "doctor" && task.Kind != "query") || task.Store != "google_play" || task.AppIdentifier != in.AppIdentifier || !appPattern.MatchString(in.AppIdentifier) || time.Now().After(task.ExpiresAt) {
		return protocol.PublishQueryResult{}, errors.New("play_app_invalid")
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
	data, e := readMaterial(ctx, in.CredentialFile, 1<<20, true)
	if e != nil {
		return protocol.PublishQueryResult{}, e
	}
	if _, e = googleCredential(data); e != nil {
		return protocol.PublishQueryResult{}, e
	}
	p.credentialCopy, e = p.write("credential.json", data)
	if e != nil {
		return protocol.PublishQueryResult{}, e
	}
	operation := "play_query"
	if task.Kind == "doctor" {
		version, ve := p.tool(ctx, "java", "-jar", in.Bundletool, "version")
		jar, je := readMaterial(ctx, in.Bundletool, 64<<20, false)
		digest := sha256.Sum256(jar)
		if ve != nil || je != nil || strings.TrimSpace(string(version)) != "1.18.3" || hex.EncodeToString(digest[:]) != "a099cfa1543f55593bc2ed16a70a7c67fe54b1747bb7301f37fdfd6d91028e29" {
			return protocol.PublishQueryResult{}, errTools
		}
		operation = "play_preflight"
	}
	out, _, e := p.run(ctx, operation, in.AppIdentifier, nil, &task, nil)
	result := queryResult(task, out, p.lockDigest)
	if result.Matches == nil {
		result.Matches = []protocol.PublishMatch{}
	}
	if task.Kind == "doctor" && out.Status == "ready" {
		result.DoctorChecks = []protocol.ToolCheck{{Name: "fastlane", Status: "passed", Version: "2.240.1"}, {Name: "bundletool", Status: "passed", Version: "1.18.3"}, {Name: "google_play_credentials", Status: "passed"}, {Name: "google_play_application", Status: "passed"}}
	}
	return result, e
}

func googleReceipt(g protocol.PublishGrant, result process.Result, out toolResult) protocol.PublishReceipt {
	r := receipt(g, result, out)
	if r.Status == "confirmed" {
		r.Status = "uploaded"
		r.Digest, _ = protocol.PublishReceiptDigest(r)
	}
	return r
}
