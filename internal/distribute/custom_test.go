package distribute

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mybuilds/internal/process"
	"mybuilds/internal/protocol"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCustomPublisherActualCommandOnceAndHTTPReceipt(t *testing.T) {
	var requests atomic.Int32
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.Write([]byte("receipt-1")) }))
	defer endpoint.Close()
	workspace := t.TempDir()
	source := filepath.Join(t.TempDir(), "package.bin")
	data := []byte{0, 1, 255, 10}
	sum := sha256.Sum256(data)
	if err := os.WriteFile(source, data, 0600); err != nil {
		t.Fatal(err)
	}
	binary, _ := os.Executable()
	options := CustomOptions{Workspace: workspace, DataDir: t.TempDir(), WorkingDir: ".", ResultFile: "result.json", Argv: []string{binary, "-test.run=^TestCustomPublisherCommandHelper$", "--", "upload"}, Environment: map[string]string{"CUSTOM_ENDPOINT": endpoint.URL}, Params: map[string]string{"channel": "internal"}, AppIdentifier: "org.example.custom", VersionName: "1.2.3", Number: 17, ArtifactID: "11111111-1111-4111-8111-111111111111", ArtifactPath: source, ArtifactSize: 4, ArtifactSHA256: hex.EncodeToString(sum[:])}
	options.CommandDigest, _ = protocol.CustomCommandDigest(options.Argv, options.QueryArgv, options.WorkingDir, options.ResultFile, options.AppIdentifier, options.VersionName, options.ArtifactID, options.ArtifactSHA256, options.Number, options.ArtifactSize)
	prepared, err := PrepareCustom(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	grant := protocol.PublishGrant{IntentID: "22222222-2222-4222-8222-222222222222", Action: "custom_upload", AppIdentifier: options.AppIdentifier, VersionName: options.VersionName, VersionCode: 17, ArtifactID: options.ArtifactID, ArtifactSize: 4, ArtifactSHA256: options.ArtifactSHA256, ReportIDs: []string{}, Custom: &protocol.CustomPublishAuthorization{CommandDigest: options.CommandDigest, ResultSchemaVersion: 1}}
	grant.AuthorizationDigest, _ = protocol.PublishGrantDigest(grant)
	receipt, err := UploadCustom(context.Background(), prepared, grant, nil)
	if err != nil || receipt.Status != "uploaded" || !receipt.Started || !receipt.StopConfirmed || receipt.Remote.Custom == nil || receipt.Remote.Custom.RemoteID != "receipt-1" {
		t.Fatalf("actual publisher: %v %+v", err, receipt)
	}
	if _, err := UploadCustom(context.Background(), prepared, grant, nil); err == nil {
		t.Fatal("reexecuted command")
	}
	if requests.Load() != 1 {
		t.Fatal("duplicate irreversible request", requests.Load())
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatal("removed original artifact")
	}
	if err := prepared.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "result.json")); !os.IsNotExist(err) {
		t.Fatal("owned result remained")
	}
}

// 真子进程读取私有输入，向自有接收端发送一次，并创建受限结果。
func TestCustomPublisherCommandHelper(t *testing.T) {
	if os.Getenv("MYBUILDS_PUBLISH_INPUT") == "" {
		return
	}
	data, err := os.ReadFile(os.Getenv("MYBUILDS_PUBLISH_INPUT"))
	if err != nil {
		os.Exit(2)
	}
	var in customInput
	if json.Unmarshal(data, &in) != nil {
		os.Exit(3)
	}
	if os.Getenv("CUSTOM_HOLD") == "yes" {
		time.Sleep(5 * time.Second)
	}
	method := http.MethodPost
	if os.Args[len(os.Args)-1] == "query" {
		if in.Artifact.Path != "" {
			os.Exit(4)
		}
		method = http.MethodGet
	} else {
		body, err := os.ReadFile(in.Artifact.Path)
		if err != nil || int64(len(body)) != in.Artifact.Size {
			os.Exit(4)
		}
	}
	request, err := http.NewRequest(method, os.Getenv("CUSTOM_ENDPOINT"), nil)
	if err != nil {
		os.Exit(5)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		os.Exit(6)
	}
	response.Body.Close()
	out := customResult{Schema: 1, IntentID: in.IntentID, AuthorizationDigest: in.AuthorizationDigest, AppIdentifier: in.AppIdentifier, ArtifactID: in.Artifact.ID, ArtifactSHA256: in.Artifact.SHA256, VersionName: in.VersionName, VersionCode: in.VersionCode, Status: "uploaded", EvidenceCode: "remote_receipt", RemoteID: "receipt-1", ActionConfirmed: true}
	if leak := os.Getenv("CUSTOM_RESULT_LEAK"); leak != "" {
		out.RemoteID = leak
	}
	result, _ := json.Marshal(out)
	if os.WriteFile(os.Getenv("MYBUILDS_PUBLISH_RESULT"), result, 0600) != nil {
		os.Exit(7)
	}
	os.Exit(0)
}

func TestCustomPreparationRefusesOldResultAndEscapes(t *testing.T) {
	workspace := t.TempDir()
	source := filepath.Join(t.TempDir(), "artifact")
	os.WriteFile(source, []byte("x"), 0600)
	options := CustomOptions{Workspace: workspace, WorkingDir: ".", ResultFile: "result", Argv: []string{"sh"}, ArtifactPath: source, ArtifactSize: 1, AppIdentifier: "org.example.custom", Number: 1, ArtifactSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte("x")))}
	os.WriteFile(filepath.Join(workspace, "result"), []byte("private old receipt"), 0600)
	if _, err := PrepareCustom(context.Background(), options); err == nil {
		t.Fatal("accepted old result")
	}
	os.Remove(filepath.Join(workspace, "result"))
	options.WorkingDir = "../escape"
	if _, err := PrepareCustom(context.Background(), options); err == nil {
		t.Fatal("escaped workspace")
	}
}

func TestCustomQueryActualMetadataOnlyAndOneRead(t *testing.T) {
	var gets, posts atomic.Int32
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			gets.Add(1)
		} else {
			posts.Add(1)
		}
		w.Write([]byte("own receipt"))
	}))
	defer endpoint.Close()
	binary, _ := os.Executable()
	workspace := t.TempDir()
	c := &protocol.CustomQueryContext{AuthorizationDigest: strings.Repeat("a", 64), ArtifactID: "11111111-1111-4111-8111-111111111111", ArtifactSHA256: strings.Repeat("b", 64), ArtifactSize: 7, Number: 17, VersionName: "1.2.3", ReportIDs: []string{}}
	task := protocol.PublishQueryTask{ID: "22222222-2222-4222-8222-222222222222", Nonce: "33333333-3333-4333-8333-333333333333", Kind: "query", Store: "custom", AppIdentifier: "org.example.custom", IntentID: "44444444-4444-4444-8444-444444444444", ExpiresAt: time.Now().Add(30 * time.Second), Custom: c}
	options := CustomOptions{Workspace: workspace, WorkingDir: ".", ResultFile: "result.json", QueryArgv: []string{binary, "-test.run=^TestCustomPublisherCommandHelper$", "--", "query"}, Environment: map[string]string{"CUSTOM_ENDPOINT": endpoint.URL}, AppIdentifier: task.AppIdentifier, VersionName: c.VersionName, Number: c.Number, ArtifactID: c.ArtifactID, ArtifactSize: c.ArtifactSize, ArtifactSHA256: c.ArtifactSHA256}
	result, e := QueryCustom(context.Background(), options, task)
	if e != nil || result.Custom == nil || result.Custom.AuthorizationDigest != c.AuthorizationDigest || result.Reason != "" || len(result.Matches) != 1 {
		t.Fatalf("metadata query: %v %+v", e, result)
	}
	if gets.Load() != 1 || posts.Load() != 0 {
		t.Fatal("query mutation", gets.Load(), posts.Load())
	}
	if _, e := os.Stat(filepath.Join(workspace, "result.json")); !os.IsNotExist(e) {
		t.Fatal("owned result remained")
	}
}

func TestCustomResultStrictJSONAndExactEvidence(t *testing.T) {
	for _, raw := range []string{`{"schema":1,"schema":1}`, `{"schema":null}`, `{"schema":1} {}`, `{"Schema":1}`, `{"schema":1,"extra":"private"}`} {
		var result customResult
		if customJSONBounds([]byte(raw)) && strictJSON([]byte(raw), &result) == nil {
			t.Fatalf("accepted ambiguous result %q", raw)
		}
	}
	g := protocol.PublishGrant{IntentID: "original", AuthorizationDigest: "auth", AppIdentifier: "app", ArtifactID: "artifact", ArtifactSHA256: "sha", VersionName: "v", VersionCode: 17}
	r := customResult{Schema: 1, IntentID: g.IntentID, AuthorizationDigest: g.AuthorizationDigest, AppIdentifier: g.AppIdentifier, ArtifactID: g.ArtifactID, ArtifactSHA256: g.ArtifactSHA256, VersionName: g.VersionName, VersionCode: g.VersionCode, Status: "uploaded", EvidenceCode: "remote_receipt", RemoteID: "own", ActionConfirmed: true}
	if !customResultMatches(r, g) {
		t.Fatal("exact result rejected")
	}
	r.ArtifactID = "other"
	if customResultMatches(r, g) {
		t.Fatal("wrong artifact accepted")
	}
	r.ArtifactID = g.ArtifactID
	r.RemoteID = "decoded\x01"
	if customResultMatches(r, g) {
		t.Fatal("control accepted")
	}
	if !customDecodedSecrets(customResult{RemoteID: "declared-sensitive"}, "declared-sensitive") {
		t.Fatal("decoded secret missed")
	}
}

func TestCustomActualCancellationAndDecodedSecretStayUnknown(t *testing.T) {
	for _, mode := range []string{"cancel", "decoded-secret"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.Write([]byte("own")) }))
			defer endpoint.Close()
			source := filepath.Join(t.TempDir(), "artifact")
			if e := os.WriteFile(source, []byte("original"), 0600); e != nil {
				t.Fatal(e)
			}
			binary, _ := os.Executable()
			sum := sha256.Sum256([]byte("original"))
			options := CustomOptions{Workspace: t.TempDir(), WorkingDir: ".", ResultFile: "result.json", Argv: []string{binary, "-test.run=^TestCustomPublisherCommandHelper$", "--", "upload"}, Environment: map[string]string{"CUSTOM_ENDPOINT": endpoint.URL}, ArtifactPath: source, ArtifactSize: 8, ArtifactSHA256: hex.EncodeToString(sum[:]), ArtifactID: "11111111-1111-4111-8111-111111111111", AppIdentifier: "org.example.custom", VersionName: "1.0", Number: 1}
			if mode == "cancel" {
				options.Environment["CUSTOM_HOLD"] = "yes"
			} else {
				options.Environment["CUSTOM_RESULT_LEAK"] = "<declared-private>"
				options.SecretValues = []string{"<declared-private>"}
			}
			options.CommandDigest, _ = protocol.CustomCommandDigest(options.Argv, nil, ".", "result.json", options.AppIdentifier, options.VersionName, options.ArtifactID, options.ArtifactSHA256, 1, 8)
			prepared, e := PrepareCustom(context.Background(), options)
			if e != nil {
				t.Fatal(e)
			}
			grant := protocol.PublishGrant{IntentID: "22222222-2222-4222-8222-222222222222", Action: "custom_upload", AppIdentifier: options.AppIdentifier, VersionName: options.VersionName, VersionCode: 1, ArtifactID: options.ArtifactID, ArtifactSHA256: options.ArtifactSHA256, ArtifactSize: 8, ReportIDs: []string{}, Custom: &protocol.CustomPublishAuthorization{CommandDigest: options.CommandDigest, ResultSchemaVersion: 1}}
			grant.AuthorizationDigest, _ = protocol.PublishGrantDigest(grant)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			receipt, e := UploadCustom(ctx, prepared, grant, func(_ process.StartInfo) error {
				if mode == "cancel" {
					cancel()
				}
				return nil
			})
			if e == nil || receipt.Status != "unknown" || !receipt.Started || !receipt.StopConfirmed || receipt.CleanupFailed || receipt.Remote.Custom != nil {
				t.Fatalf("unconfirmed command promoted: %v %+v", e, receipt)
			}
			expected := int32(1)
			if mode == "cancel" {
				expected = 0
			}
			if calls.Load() != expected {
				t.Fatal("unexpected irreversible call", calls.Load())
			}
			if e = prepared.Close(); e != nil {
				t.Fatal(e)
			}
			b, e := os.ReadFile(source)
			if e != nil || string(b) != "original" {
				t.Fatal("original artifact changed")
			}
		})
	}
}
