//go:build darwin || linux

package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
)

func TestPublishEvidenceActualPrivateFileAndReplacement(t *testing.T) {
	lock, journal := recoveryJournalFile(t)
	parent, err := lock.resultParent()
	if err != nil {
		t.Fatal(err)
	}
	result := filepath.Join(parent, uuid.NewString())
	if err = os.Mkdir(result, 0700); err != nil {
		t.Fatal(err)
	}
	journal.state.ResultDir = result
	e := &taskExecution{journal: journal, cfg: config.AgentConfig{DataDir: lock.root.Name()}}
	id, err := e.publisherCandidate(2)
	if err != nil || e.savePublishEvidence(id) != nil {
		t.Fatal("原候选证据未持久化", err)
	}
	file := filepath.Join(result, "publishes", id+".json")
	info, err := os.Lstat(file)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("非私有原证据", err)
	}
	grant := protocol.PublishGrant{IntentID: id, Ref: *journal.state.Ref, Action: "upload", AppIdentifier: "com.example.app", ArtifactID: uuid.NewString(), ArtifactSHA256: strings.Repeat("a", 64), ArtifactSize: 20, VersionName: "1.2.3", VersionCode: 1, ReportIDs: []string{}}
	grant.AuthorizationDigest, _ = protocol.PublishGrantDigest(grant)
	if err = e.publisherGrant(grant); err != nil || e.savePublishEvidence(id) != nil {
		t.Fatal("once原授权证据", err)
	}
	if err = e.publisherGrant(grant); err == nil {
		t.Fatal("第二可执行授权被接受")
	}
	data, err := os.ReadFile(file)
	var checkpoint publishCheckpoint
	if err != nil || json.Unmarshal(data, &checkpoint) != nil || !checkpoint.Once || checkpoint.Grant == nil || checkpoint.Grant.AuthorizationDigest != grant.AuthorizationDigest {
		t.Fatal("原授权证据不完整", err)
	}
	foreign := filepath.Join(t.TempDir(), "foreign")
	if err = os.WriteFile(foreign, []byte("FOREIGN_FILE_UNCHANGED"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(foreign, file); err != nil {
		t.Fatal(err)
	}
	if err = e.savePublishEvidence(id); err == nil {
		t.Fatal("未知替换被覆盖")
	}
	actual, err := os.ReadFile(foreign)
	if err != nil || string(actual) != "FOREIGN_FILE_UNCHANGED" {
		t.Fatal("周边文件被修改", err)
	}
}

func TestPublishRecoveryExactCheckpointAndManifest(t *testing.T) {
	_, j := recoveryJournalFile(t)
	id := uuid.NewString()
	g := protocol.PublishGrant{IntentID: id, Ref: *j.state.Ref, Action: "upload"}
	g.AuthorizationDigest, _ = protocol.PublishGrantDigest(g)
	j.state.Publishes = []publishCheckpoint{{IntentID: id, Index: 2, Once: true, Grant: &g}}
	j.state.PendingEvent.Progress.PublishIntents = []protocol.PublishExpectation{{IntentID: id, Status: "unknown"}}
	if err := validatePublisherJournal(j.state); err != nil {
		t.Fatal("原未知意图被遗忘", err)
	}
	j.state.PendingEvent.Progress.PublishIntents = nil
	if err := validatePublisherJournal(j.state); err == nil {
		t.Fatal("原已授意图从终态消失")
	}
	j.state.PendingEvent.Progress.PublishIntents = []protocol.PublishExpectation{{IntentID: id, Status: "unknown"}}
	g.Ref.AttemptID = uuid.NewString()
	g.AuthorizationDigest, _ = protocol.PublishGrantDigest(g)
	if err := validatePublisherJournal(j.state); err == nil {
		t.Fatal("跨attempt证据混入")
	}
}
