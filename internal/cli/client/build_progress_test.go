package client

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"mybuilds/internal/store"
)

func TestActualRemoteBuildRunningEvidenceViews(t *testing.T) {
	st, address := realRemoteAPI(t)
	ctx := context.Background()
	admin, e := st.Authenticate(ctx, os.Getenv("MYBUILDS_CLIENT_TOKEN"))
	if e != nil {
		t.Fatal(e)
	}
	node, e := st.CreateNode(ctx, admin, store.NodeInput{Name: "worker", Capacity: 1})
	if e != nil {
		t.Fatal(e)
	}
	actor, e := st.AuthenticateNode(ctx, node.Token)
	if e != nil {
		t.Fatal(e)
	}
	policy := store.LeasePolicy{Concurrency: 1, Heartbeat: 5 * time.Second, Duration: 30 * time.Second}
	session, e := st.OpenNodeSession(ctx, actor, protocol.SessionRequest{SessionID: uuid.NewString(), HeartbeatNS: int64(policy.Heartbeat), LeaseNS: int64(policy.Duration), Report: protocol.NodeReport{OS: "linux", Arch: "amd64", Capacity: 1, Tools: []protocol.ToolCheck{{Name: "shell", Status: "passed"}, {Name: "git", Status: "passed"}, {Name: "node_journal", Status: "passed"}}}}, policy)
	if e != nil {
		t.Fatal(e)
	}
	p, e := st.CreateProject(ctx, admin, store.ProjectInput{Name: "app", Repository: "https://example.org/repo.git", AllowedNodes: []string{"worker"}, DefaultNode: "worker"})
	if e != nil {
		t.Fatal(e)
	}
	budget := int64(time.Minute)
	_, e = st.Enqueue(ctx, store.EnqueueInput{Actor: admin, ProjectID: p.ID, ProjectVersion: p.PolicyVersion, Key: "view", RequestDigest: strings.Repeat("a", 64), SHA: strings.Repeat("b", 40), Branch: "main", Source: "repo", File: "mybuilds.yml", SourceDigest: strings.Repeat("c", 64), Builds: []store.PreparedBuild{{Name: "compile", Status: "queued", InitialBudgetNS: &budget, PostBudgetNS: int64(2 * time.Minute), Snapshot: store.BuildSnapshot{Definition: config.Build{Timeout: "1m", Params: map[string]config.Parameter{"version": {Required: true}}, Steps: []config.Step{{Name: "shell", Kind: "run", Run: "PRIVATE_SCRIPT"}}}, Params: map[string]string{"version": "PRIVATE_VALUE"}, Condition: "ready", AllowedNodes: p.AllowedNodes, DefaultNode: p.DefaultNode}, Steps: []store.StepProgress{{Phase: "ordinary", Index: 1, Name: "shell", Kind: "run", Condition: "ready", Status: "pending"}}}}})
	if e != nil {
		t.Fatal(e)
	}
	grant, e := st.Claim(ctx, actor, protocol.ClaimRequest{SessionID: session.SessionID, ClaimKey: uuid.NewString()}, policy)
	if e != nil || grant == nil {
		t.Fatal(e)
	}
	out, e := executeRemote(t, "--server-url", address, "build", "show", grant.Ref.BuildID, "--json")
	if e != nil {
		t.Fatal(e)
	}
	var view store.BuildView
	if e = json.Unmarshal([]byte(out), &view); e != nil || view.Status != "running" || view.NodeName != "worker" || view.AttemptID != grant.Ref.AttemptID || view.LeaseEpoch != grant.Ref.Epoch || view.RemainingBudgetNS == nil || *view.RemainingBudgetNS != budget {
		t.Fatal(out, e)
	}
	if strings.Contains(out, "PRIVATE_") || strings.Contains(out, node.Token) {
		t.Fatal("安全视图泄露", out)
	}
	out, e = executeRemote(t, "--server-url", address, "build", "show", grant.Ref.BuildID)
	if e != nil {
		t.Fatal(e)
	}
	for _, key := range []string{"node_name", "session_id", "attempt_id", "lease_id", "lease_epoch", "cancel_requested", "stop_unconfirmed", "remaining_post_budget_ns", "intent=false", "started=false", "stop_confirmed=false", "cleanup_failed=false"} {
		if !strings.Contains(out, key) {
			t.Fatal("文本缺执行证据", key, out)
		}
	}
	out, e = executeRemote(t, "--server-url", address, "status", "--json")
	if e != nil || !strings.Contains(out, `"running":1`) || !strings.Contains(out, `"healthy_nodes":1`) {
		t.Fatal(out, e)
	}
}
