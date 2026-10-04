package store

import (
	"github.com/google/uuid"
	"mybuilds/internal/protocol"
	"testing"
	"time"
)

func healthyReport() protocol.NodeReport {
	return protocol.NodeReport{OS: "linux", Arch: "amd64", Capacity: 2, Tools: []protocol.ToolCheck{{Name: "shell", Status: "passed"}, {Name: "git", Status: "passed", Version: "2.43.0"}, {Name: "node_journal", Status: "passed"}, {Name: "ios_signing", Status: "skipped", Reason: "unsupported"}}}
}
func openSession(t *testing.T, s *Store, name string, capacity int) (NodeActor, protocol.SessionGrant) {
	t.Helper()
	c, err := s.CreateNode(testContext, localAdmin, NodeInput{Name: name, Capacity: capacity})
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.AuthenticateNode(testContext, c.Token)
	if err != nil {
		t.Fatal(err)
	}
	policy := LeasePolicy{Concurrency: 2, Heartbeat: time.Second, Duration: 10 * time.Second}
	grant, err := s.OpenNodeSession(testContext, a, protocol.SessionRequest{SessionID: uuid.NewString(), Report: healthyReport(), HeartbeatNS: int64(policy.Heartbeat), LeaseNS: int64(policy.Duration)}, policy)
	if err != nil {
		t.Fatal(err)
	}
	return a, grant
}
func TestNodeSessionPolicyAndIsolation(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, grant := openSession(t, s, "linux", 1)
		policy := LeasePolicy{Concurrency: 2, Heartbeat: time.Second, Duration: 10 * time.Second}
		request := protocol.SessionRequest{SessionID: grant.SessionID, Report: healthyReport(), HeartbeatNS: int64(policy.Heartbeat), LeaseNS: int64(policy.Duration)}
		same, err := s.OpenNodeSession(testContext, a, request, policy)
		if err != nil || same != grant || same.NodeName != "linux" {
			t.Fatal("session replay", err)
		}
		view, err := s.GetNode(testContext, localAdmin, "linux")
		if err != nil || !view.Healthy || !view.SessionActive || view.EffectiveCapacity != 1 {
			t.Fatal("health", view, err)
		}
		request.SessionID = uuid.NewString()
		if _, err = s.OpenNodeSession(testContext, a, request, policy); err != ErrSessionConflict {
			t.Fatal("live replaced", err)
		}
		if _, err = s.Heartbeat(testContext, a, protocol.HeartbeatRequest{SessionID: request.SessionID, Report: healthyReport()}, policy); err != ErrSessionExpired {
			t.Fatal("other session heartbeat", err)
		}
		old := time.Now().UTC().Add(-21 * time.Second)
		if err = s.writer.Model(&nodeSessionRecord{}).Where("id = ?", grant.SessionID).Update("last_heartbeat", old).Error; err != nil {
			t.Fatal(err)
		}
		view, err = s.GetNode(testContext, localAdmin, "linux")
		if err != nil || view.Healthy || view.SessionActive {
			t.Fatal("stale health", err)
		}
		// 即使心跳已超期，running或停止保护仍禁止替换。
		p, err := s.CreateProject(testContext, localAdmin, projectInput("app"))
		if err != nil {
			t.Fatal(err)
		}
		enq, err := s.Enqueue(testContext, enqueueInput(p, "session-guard"))
		if err != nil {
			t.Fatal(err)
		}
		id := enq.Builds[0].ID
		if err = s.writer.Model(&buildRecord{}).Where("id = ?", id).Updates(map[string]any{"node_id": a.ID, "status": "running"}).Error; err != nil {
			t.Fatal(err)
		}
		if _, err = s.OpenNodeSession(testContext, a, request, policy); err != ErrSessionConflict {
			t.Fatal("running bypass", err)
		}
		s.writer.Model(&buildRecord{}).Where("id = ?", id).Updates(map[string]any{"status": "interrupted", "stop_unconfirmed": true})
		if _, err = s.OpenNodeSession(testContext, a, request, policy); err != ErrSessionConflict {
			t.Fatal("guard bypass", err)
		}
		if err = s.DeleteNode(testContext, localAdmin, "linux"); err != ErrConflict {
			t.Fatal("guard delete", err)
		}
		s.writer.Model(&buildRecord{}).Where("id = ?", id).Update("stop_unconfirmed", false)
		replaced, err := s.OpenNodeSession(testContext, a, request, policy)
		if err != nil || replaced.SessionID != request.SessionID {
			t.Fatal("stale replace", err)
		}
		request.SessionID = grant.SessionID
		if _, err = s.OpenNodeSession(testContext, a, request, policy); err != ErrSessionConflict {
			t.Fatal("old resurrected", err)
		}
		if _, err = s.Heartbeat(testContext, a, protocol.HeartbeatRequest{SessionID: grant.SessionID, Report: healthyReport()}, policy); err != ErrSessionExpired {
			t.Fatal("old heartbeat", err)
		}
		if err = s.SetNodeState(testContext, localAdmin, "linux", "draining"); err != nil {
			t.Fatal(err)
		}
		if _, err = s.Heartbeat(testContext, a, protocol.HeartbeatRequest{SessionID: replaced.SessionID, Report: healthyReport()}, policy); err != nil {
			t.Fatal("drain heartbeat", err)
		}
		if err = s.SetNodeState(testContext, localAdmin, "linux", "disabled"); err != nil {
			t.Fatal(err)
		}
		if _, err = s.Heartbeat(testContext, a, protocol.HeartbeatRequest{SessionID: replaced.SessionID, Report: healthyReport()}, policy); err != ErrNodeUnauthorized {
			t.Fatal("disabled heartbeat", err)
		}
		if err = s.SetNodeState(testContext, localAdmin, "linux", "enabled"); err != nil {
			t.Fatal(err)
		}
		other, _ := openSession(t, s, "other", 1)
		if _, err = s.Heartbeat(testContext, other, protocol.HeartbeatRequest{SessionID: replaced.SessionID, Report: healthyReport()}, policy); err != ErrSessionExpired {
			t.Fatal("other node heartbeat", err)
		}
		if err = s.RevokeNodeToken(testContext, localAdmin, "linux"); err != nil {
			t.Fatal(err)
		}
		if _, err = s.Heartbeat(testContext, a, protocol.HeartbeatRequest{SessionID: replaced.SessionID, Report: healthyReport()}, policy); err != ErrNodeUnauthorized {
			t.Fatal("cached actor bypass", err)
		}
	})
}

func TestNodeSessionReportAndPolicyBoundaries(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		actor, grant := openSession(t, s, "linux", 1)
		policy := LeasePolicy{Concurrency: 2, Heartbeat: time.Second, Duration: 10 * time.Second}
		in := protocol.SessionRequest{SessionID: grant.SessionID, Report: healthyReport(), HeartbeatNS: int64(policy.Heartbeat), LeaseNS: int64(policy.Duration)}
		for _, bad := range []LeasePolicy{{Concurrency: 0, Heartbeat: time.Second, Duration: 10 * time.Second}, {Concurrency: -1, Heartbeat: time.Second, Duration: 10 * time.Second}, {Concurrency: 1, Heartbeat: 0, Duration: 10 * time.Second}, {Concurrency: 1, Heartbeat: time.Second, Duration: 9 * time.Second}, {Concurrency: 1, Heartbeat: 30 * time.Second, Duration: 121 * time.Second}} {
			if _, err := s.OpenNodeSession(testContext, actor, in, bad); err != ErrInvalid {
				t.Fatal("invalid policy", err)
			}
		}
		in.LeaseNS++
		if _, err := s.OpenNodeSession(testContext, actor, in, policy); err != ErrInvalid {
			t.Fatal("policy mismatch", err)
		}
		for _, report := range []protocol.NodeReport{{OS: "linux", Arch: "amd64", Capacity: 0}, {OS: "linux", Arch: "amd64", Capacity: 1, Tools: []protocol.ToolCheck{{Name: "shell", Status: "passed", Reason: "secret-marker"}}}, {OS: "linux", Arch: "amd64", Capacity: 1, Tools: []protocol.ToolCheck{{Name: "shell", Status: "passed"}, {Name: "shell", Status: "passed"}}}, {OS: "darwin", Arch: "arm64", Capacity: 1, Tools: []protocol.ToolCheck{{Name: "ios_signing", Status: "passed"}}}, {OS: "linux", Arch: "amd64", Capacity: 1, Tools: []protocol.ToolCheck{{Name: "git", Status: "passed", Version: "secret-marker"}}}} {
			if _, err := s.Heartbeat(testContext, actor, protocol.HeartbeatRequest{SessionID: grant.SessionID, Report: report}, policy); err != ErrInvalid {
				t.Fatal("bad report", err)
			}
		}
		p, err := s.CreateProject(testContext, localAdmin, projectInput("heartbeat"))
		if err != nil {
			t.Fatal(err)
		}
		enq, err := s.Enqueue(testContext, enqueueInput(p, "heartbeat"))
		if err != nil {
			t.Fatal(err)
		}
		expiry := time.Now().UTC().Add(time.Minute)
		if err = s.writer.Model(&buildRecord{}).Where("id = ?", enq.Builds[0].ID).Updates(map[string]any{"node_id": actor.ID, "lease_expires_at": expiry}).Error; err != nil {
			t.Fatal(err)
		}
		stale := time.Now().UTC().Add(-3 * time.Second)
		if err = s.writer.Model(&nodeSessionRecord{}).Where("id = ?", grant.SessionID).Update("last_heartbeat", stale).Error; err != nil {
			t.Fatal(err)
		}
		view, err := s.GetNode(testContext, localAdmin, "linux")
		if err != nil || view.Healthy || !view.SessionActive {
			t.Fatal("health window", view, err)
		}
		if _, err = s.Heartbeat(testContext, actor, protocol.HeartbeatRequest{SessionID: grant.SessionID, Report: healthyReport()}, policy); err != nil {
			t.Fatal(err)
		}
		var row buildRecord
		if err = s.db.First(&row, "id = ?", enq.Builds[0].ID).Error; err != nil || row.LeaseExpiresAt == nil || !row.LeaseExpiresAt.Equal(expiry) {
			t.Fatal("heartbeat renewed task", err)
		}
		status, err := s.Status(testContext)
		if err != nil || status.HealthyNodes != 1 {
			t.Fatal("healthy status", status, err)
		}
	})
}

func TestReportedOSArchitecturePairsAllowGenericScheduling(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		policy := LeasePolicy{Concurrency: 33, Heartbeat: time.Second, Duration: 10 * time.Second}
		pairs := []struct{ os, arch string }{{"darwin", "amd64"}, {"darwin", "arm64"}}
		for _, arch := range []string{"386", "amd64", "arm", "arm64", "loong64", "mips", "mips64", "mips64le", "mipsle", "ppc64", "ppc64le", "riscv64", "s390x"} {
			pairs = append(pairs, struct{ os, arch string }{"linux", arch})
		}
		for _, pair := range pairs {
			name := pair.os + "-" + pair.arch
			created, err := s.CreateNode(testContext, localAdmin, NodeInput{Name: name, Capacity: 1})
			if err != nil {
				t.Fatal(err)
			}
			actor, err := s.AuthenticateNode(testContext, created.Token)
			if err != nil {
				t.Fatal(err)
			}
			// 这是实际Store对已声明报告的协议回归，不把其它CPU上的工具声明当实机验收。
			report := healthyReport()
			report.OS = pair.os
			report.Arch = pair.arch
			report.Capacity = 1
			grant, err := s.OpenNodeSession(testContext, actor, protocol.SessionRequest{SessionID: uuid.NewString(), Report: report, HeartbeatNS: int64(policy.Heartbeat), LeaseNS: int64(policy.Duration)}, policy)
			if err != nil {
				t.Fatalf("valid reported pair %s: %v", name, err)
			}
			input := projectInput(name)
			input.AllowedNodes = []string{name}
			input.DefaultNode = name
			p, err := s.CreateProject(testContext, localAdmin, input)
			if err != nil {
				t.Fatal(err)
			}
			queueBuild(t, s, p, "generic-"+name, "shell", false)
			claimed, err := s.Claim(testContext, actor, protocol.ClaimRequest{SessionID: grant.SessionID, ClaimKey: uuid.NewString()}, policy)
			if err != nil || claimed == nil {
				t.Fatalf("generic scheduling %s: %v", name, err)
			}
		}
		report := healthyReport()
		report.OS = "darwin"
		report.Arch = "riscv64"
		if validNodeReport(report) {
			t.Fatal("invalid OS/arch pair accepted")
		}
	})
}
