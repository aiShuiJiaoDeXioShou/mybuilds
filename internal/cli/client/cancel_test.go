package client

import (
	"context"
	"encoding/json"
	"mybuilds/internal/config"
	"mybuilds/internal/store"
	"os"
	"strings"
	"testing"
	"time"
)

func TestActualRemoteQueuedCancelAndConfirmationFlags(t *testing.T) {
	st, address := realRemoteAPI(t)
	ctx := context.Background()
	admin, e := st.Authenticate(ctx, os.Getenv("MYBUILDS_CLIENT_TOKEN"))
	if e != nil {
		t.Fatal(e)
	}
	p, e := st.CreateProject(ctx, admin, store.ProjectInput{Name: "app", Repository: "https://example.org/repo.git", AllowedNodes: []string{"worker"}, DefaultNode: "worker"})
	if e != nil {
		t.Fatal(e)
	}
	batch, e := st.Enqueue(ctx, store.EnqueueInput{Actor: admin, ProjectID: p.ID, ProjectVersion: p.PolicyVersion, Key: "cancel", RequestDigest: strings.Repeat("a", 64), SHA: strings.Repeat("b", 40), Branch: "main", Source: "repo", File: "mybuilds.yml", SourceDigest: strings.Repeat("c", 64), Builds: []store.PreparedBuild{{Name: "compile", Status: "queued", PostBudgetNS: int64(2 * time.Minute), Snapshot: store.BuildSnapshot{Definition: config.Build{Steps: []config.Step{{Kind: "run", Name: "shell", Run: ":"}}}, Condition: "ready", AllowedNodes: p.AllowedNodes, DefaultNode: p.DefaultNode}, Steps: []store.StepProgress{{Phase: "ordinary", Index: 1, Name: "shell", Kind: "run", Condition: "ready", Status: "pending"}}}}})
	if e != nil {
		t.Fatal(e)
	}
	id := batch.Builds[0].ID
	out, e := executeRemote(t, "--server-url", address, "build", "cancel", id, "--json")
	var view store.BuildView
	if e != nil || json.Unmarshal([]byte(out), &view) != nil || view.Status != "cancelled" || !view.CancelRequested || view.AttemptID != "" {
		t.Fatal(out, e)
	}
	for _, args := range [][]string{{"build", "cancel", id, "--retry"}, {"build", "confirm-stopped", id}, {"build", "confirm-stopped", id, "--note", "PRIVATE"}, {"build", "confirm-stopped", id, "--force"}, {"build", "retry", id}} {
		out, e := executeRemote(t, append([]string{"--server-url", address}, args...)...)
		if e == nil || strings.Contains(out, "PRIVATE") || strings.Contains(e.Error(), "PRIVATE") {
			t.Fatal("非法停止命令", args, out, e)
		}
	}
}
