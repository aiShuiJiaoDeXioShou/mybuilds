package client

import (
	"context"
	"encoding/json"
	"mybuilds/internal/store"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectCustomManualBindingActualHTTP(t *testing.T) {
	st, api, _, grant := clientEvidenceFixture(t, "run")
	evidence := filepath.Join(t.TempDir(), "verification.json")
	raw := `{"source":"manual_attested","evidence_code":"ownership_attested","note":"private administrator observation","evidence_sha256":"` + strings.Repeat("a", 64) + `"}`
	if e := os.WriteFile(evidence, []byte(raw), 0600); e != nil {
		t.Fatal(e)
	}
	output, e := executeRemote(t, "--server-url", api.URL, "project", "app", "bind", "app", "--store", "custom", "--app-id", "org.example.owned", "--node", grant.Ref.NodeID, "--verification-file", evidence, "--json")
	var view store.ApplicationView
	if e != nil || json.Unmarshal([]byte(output), &view) != nil || view.Status != "verified" || view.VerificationSource != "manual_attested" || strings.Contains(output, "private administrator") {
		t.Fatal("manual binding actual", e, output)
	}
	p, e := st.GetProject(context.Background(), "app")
	if e != nil {
		t.Fatal(e)
	}
	list, e := st.ListApplications(context.Background(), p.ID)
	if e != nil || len(list) != 1 {
		t.Fatal(e)
	}
	if _, e = executeRemote(t, "--server-url", api.URL, "project", "app", "bind", "app", "--store", "app_store", "--app-id", "org.example.apple", "--node", grant.Ref.NodeID, "--verification-file", evidence, "--credentials-env", "APPLE"); e == nil {
		t.Fatal("manual bypassed Apple")
	}
}

func TestProjectProfileFlagsProduceTypedBindingsBeforeHTTP(t *testing.T) {
	cmd := newRemoteProjectCommand()
	init, _, e := cmd.Find([]string{"init"})
	if e != nil {
		t.Fatal(e)
	}
	if e = init.ParseFlags([]string{"--repo", "/owned/repo", "--nodes", "worker", "--framework", "flutter", "--platform", "ios,android"}); e != nil {
		t.Fatal(e)
	}
	input, e := remoteProjectInput(init, "owned")
	if e != nil || input.Settings.Pipeline.Builds["android"].Profile != "flutter-android" || input.Settings.Pipeline.Builds["ios"].Profile != "flutter-ios" {
		t.Fatal("typed bind", e)
	}
	for _, args := range [][]string{{"--framework", "flutter"}, {"--platform", "android,android"}, {"--platform", ""}, {"--framework", "bad", "--platform", "android"}, {"--platform", "android", "--file", "mybuilds.yml"}} {
		root := newRemoteProjectCommand()
		child, _, _ := root.Find([]string{"init"})
		all := append([]string{"--repo", "/owned/repo", "--nodes", "worker"}, args...)
		if e := child.ParseFlags(all); e != nil {
			t.Fatal(e)
		}
		if _, e := remoteProjectInput(child, "owned"); e == nil {
			t.Fatal("accepted ambiguous binding", args)
		}
	}
}
