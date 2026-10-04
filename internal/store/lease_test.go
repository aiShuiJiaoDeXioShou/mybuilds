package store

import (
	"fmt"
	"github.com/google/uuid"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"sync"
	"testing"
	"time"
)

func leaseReport() protocol.NodeReport {
	r := healthyReport()
	r.Tools = append(r.Tools, protocol.ToolCheck{Name: "java", Status: "passed", Version: "17.0.20.1"}, protocol.ToolCheck{Name: "android_aapt2", Status: "passed", Version: "2.19"}, protocol.ToolCheck{Name: "android_apksigner", Status: "passed", Version: "0.9"})
	return r
}
func leaseFixture(t *testing.T, s *Store) (NodeActor, protocol.SessionGrant, Project, LeasePolicy) {
	t.Helper()
	actor, session := openSession(t, s, "linux", 2)
	p, err := s.CreateProject(testContext, localAdmin, projectInput("app"))
	if err != nil {
		t.Fatal(err)
	}
	return actor, session, p, LeasePolicy{Concurrency: 2, Heartbeat: time.Second, Duration: 10 * time.Second}
}
func queueBuild(t *testing.T, s *Store, p Project, key, name string, runner bool) string {
	t.Helper()
	in := enqueueInput(p, key)
	in.Builds[0].Name = name
	if runner {
		in.Builds[0].Snapshot.Definition.Runner = &config.Runner{Platform: "android"}
	}
	out, err := s.Enqueue(testContext, in)
	if err != nil {
		t.Fatal(err)
	}
	return out.Builds[0].ID
}
func TestLeaseClaimConcurrentAndReplay(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, session, p, policy := leaseFixture(t, s)
		b, other := openSession(t, s, "linux2", 1)
		if err := s.writer.Model(&projectRecord{}).Where("id = ?", p.ID).Update("nodes_json", `["linux","linux2"]`).Error; err != nil {
			t.Fatal(err)
		}
		p.AllowedNodes = []string{"linux", "linux2"}
		for _, pair := range []struct {
			a NodeActor
			s string
		}{{a, session.SessionID}, {b, other.SessionID}} {
			if _, err := s.Heartbeat(testContext, pair.a, protocol.HeartbeatRequest{SessionID: pair.s, Report: leaseReport()}, policy); err != nil {
				t.Fatal(err)
			}
		}
		for i := 0; i < 20; i++ {
			queueBuild(t, s, p, fmt.Sprintf("job-%d", i), fmt.Sprintf("build-%d", i), true)
		}
		type result struct {
			actor   NodeActor
			request protocol.ClaimRequest
			grant   *protocol.LeaseGrant
			err     error
		}
		results := make(chan result, 20)
		var wg sync.WaitGroup
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				actor, sid := a, session.SessionID
				if i%2 != 0 {
					actor, sid = b, other.SessionID
				}
				request := protocol.ClaimRequest{SessionID: sid, ClaimKey: uuid.NewString()}
				grant, err := s.Claim(testContext, actor, request, policy)
				results <- result{actor, request, grant, err}
			}(i)
		}
		wg.Wait()
		close(results)
		seen := map[string]bool{}
		count := 0
		for result := range results {
			if result.err != nil {
				t.Fatal(result.err)
			}
			if result.grant == nil {
				continue
			}
			count++
			grant := result.grant
			if seen[grant.Ref.BuildID] {
				t.Fatal("duplicate claim")
			}
			seen[grant.Ref.BuildID] = true
			replay, err := s.Claim(testContext, result.actor, result.request, policy)
			if err != nil || replay == nil || replay.Ref != grant.Ref {
				t.Fatal("claim key replay", err)
			}
			if grant.Task == nil || grant.Task.SHA != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" || grant.Task.Number < 1 || grant.TTLNS <= 0 {
				t.Fatal("frozen task")
			}
			if err = s.CheckExecution(testContext, result.actor, grant.Ref); err != nil {
				t.Fatal(err)
			}
			wrong := grant.Ref
			wrong.Epoch++
			if err = s.CheckExecution(testContext, result.actor, wrong); err != ErrLeaseInvalid {
				t.Fatal("epoch accepted", err)
			}
		}
		if count != 2 {
			t.Fatalf("capacity claims: %d", count)
		}
		status, err := s.Status(testContext)
		if err != nil || status.Running != 2 || status.Queued != 18 {
			t.Fatal("counts", status, err)
		}
	})
}
func TestLeaseNameGuardExpiryAndState(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		actor, session, p, policy := leaseFixture(t, s)
		for i := 0; i < 3; i++ {
			queueBuild(t, s, p, fmt.Sprintf("same-%d", i), "android", false)
		}
		request := protocol.ClaimRequest{SessionID: session.SessionID, ClaimKey: uuid.NewString()}
		grant, err := s.Claim(testContext, actor, request, policy)
		if err != nil || grant == nil {
			t.Fatal("claim", err)
		}
		if next, err := s.Claim(testContext, actor, protocol.ClaimRequest{SessionID: session.SessionID, ClaimKey: uuid.NewString()}, policy); err != nil || next != nil {
			t.Fatal("same name raced", err)
		}
		if err = s.SetNodeState(testContext, localAdmin, "linux", "draining"); err != nil {
			t.Fatal(err)
		}
		if _, err = s.Renew(testContext, actor, grant.Ref, policy); err != nil {
			t.Fatal("drain renew", err)
		}
		if err = s.SetNodeState(testContext, localAdmin, "linux", "disabled"); err != nil {
			t.Fatal(err)
		}
		if _, err = s.Renew(testContext, actor, grant.Ref, policy); err != ErrNodeUnauthorized {
			t.Fatal("disabled renewed", err)
		}
		if err = s.SetNodeState(testContext, localAdmin, "linux", "enabled"); err != nil {
			t.Fatal(err)
		}
		// 使用真实UTC边界，已经等于到期的租约不得延长。
		now := time.Now().UTC()
		if err = s.writer.Model(&buildRecord{}).Where("id = ?", grant.Ref.BuildID).Update("lease_expires_at", now).Error; err != nil {
			t.Fatal(err)
		}
		if _, err = s.Renew(testContext, actor, grant.Ref, policy); err != ErrLeaseExpired {
			t.Fatal("expired renewed", err)
		}
		if _, err = s.Claim(testContext, actor, request, policy); err != ErrLeaseExpired {
			t.Fatal("claim replay revived", err)
		}
	})
}
func TestLeaseAuthorizationCapabilityAndLabels(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		actor, session, p, policy := leaseFixture(t, s)
		id := queueBuild(t, s, p, "default", "default", false)
		if err := s.writer.Model(&projectRecord{}).Where("id = ?", p.ID).Update("nodes_json", `["other"]`).Error; err != nil {
			t.Fatal(err)
		}
		request := func() protocol.ClaimRequest {
			return protocol.ClaimRequest{SessionID: session.SessionID, ClaimKey: uuid.NewString()}
		}
		if grant, err := s.Claim(testContext, actor, request(), policy); err != nil || grant != nil {
			t.Fatal("authorization expanded", err)
		}
		var row buildRecord
		s.db.First(&row, "id = ?", id)
		if row.Reason != "node_unauthorized" {
			t.Fatal("queue reason", row.Reason)
		}
		if err := s.writer.Model(&projectRecord{}).Where("id = ?", p.ID).Update("nodes_json", `["linux"]`).Error; err != nil {
			t.Fatal(err)
		}
		// 固定快照的AllowedNodes仍是linux，即使当前允许新增，旧快照也不能扩大。
		in := enqueueInput(p, "android")
		in.Builds[0].Name = "android"
		in.Builds[0].Snapshot.Definition.Runner = &config.Runner{Platform: "android", Labels: []string{"sdk*"}}
		out, err := s.Enqueue(testContext, in)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.Heartbeat(testContext, actor, protocol.HeartbeatRequest{SessionID: session.SessionID, Report: leaseReport()}, policy); err != nil {
			t.Fatal(err)
		}
		if err = s.writer.Model(&nodeRecord{}).Where("id = ?", actor.ID).Update("labels_json", `["sdk35"]`).Error; err != nil {
			t.Fatal(err)
		}
		grant, err := s.Claim(testContext, actor, request(), policy)
		if err != nil || grant == nil || grant.Ref.BuildID != id {
			t.Fatal("default", err)
		}
		if grant, err = s.Claim(testContext, actor, request(), policy); err != nil || grant != nil {
			t.Fatal("labels treated as glob", err)
		}
		row = buildRecord{}
		if err = s.db.First(&row, "id = ?", out.Builds[0].ID).Error; err != nil {
			t.Fatal(err)
		}
		if row.Reason != "capability_mismatch" {
			t.Fatal("label reason", row.Reason)
		}
		if err = s.writer.Model(&nodeRecord{}).Where("id = ?", actor.ID).Update("labels_json", `["sdk*"]`).Error; err != nil {
			t.Fatal(err)
		}
		if grant, err = s.Claim(testContext, actor, request(), policy); err != nil || grant == nil {
			t.Fatal("exact label", err)
		}
	})
}

func TestRenewRechecksOriginalExpiryAfterRealDatabaseDelay(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		actor, session, p, policy := leaseFixture(t, s)
		queueBuild(t, s, p, "slow-renew", "android", false)
		grant, err := s.Claim(testContext, actor, protocol.ClaimRequest{SessionID: session.SessionID, ClaimKey: uuid.NewString()}, policy)
		if err != nil || grant == nil {
			t.Fatal(err)
		}
		original := time.Now().UTC().Add(100 * time.Millisecond)
		if err = s.writer.Model(&buildRecord{}).Where("id = ?", grant.Ref.BuildID).Update("lease_expires_at", original).Error; err != nil {
			t.Fatal(err)
		}
		// 数据库自己的真实触发器延迟写操作，验证事务末尾而非入口的旧到期检查；没有Go测试hook。
		if opt.Driver == "postgres" {
			if err = s.writer.Exec(`CREATE FUNCTION slow_lease_update() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_sleep(0.2); RETURN NEW; END $$`).Error; err != nil {
				t.Fatal(err)
			}
			if err = s.writer.Exec(`CREATE TRIGGER slow_lease_update BEFORE UPDATE OF lease_expires_at ON builds FOR EACH ROW EXECUTE FUNCTION slow_lease_update()`).Error; err != nil {
				t.Fatal(err)
			}
		} else {
			if err = s.writer.Exec(`CREATE TRIGGER slow_lease_update AFTER UPDATE OF lease_expires_at ON builds BEGIN SELECT sum(x) FROM (WITH RECURSIVE delay(x) AS (VALUES(0) UNION ALL SELECT x+1 FROM delay WHERE x<1000000) SELECT x FROM delay); END`).Error; err != nil {
				t.Fatal(err)
			}
		}
		start := time.Now()
		if _, err = s.Renew(testContext, actor, grant.Ref, policy); err != ErrLeaseExpired {
			t.Fatal("old expiry bypassed", err)
		}
		if time.Since(start) < 100*time.Millisecond {
			t.Fatal("fixture did not cross expiry")
		}
		var row buildRecord
		if err = s.db.First(&row, "id = ?", grant.Ref.BuildID).Error; err != nil || row.LeaseExpiresAt == nil || !row.LeaseExpiresAt.Equal(original) {
			t.Fatal("failed renew persisted", err)
		}
	})
}

func TestGlobalConcurrencyAboveNodeCapacityBound(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		actor, session, p, policy := leaseFixture(t, s)
		policy.Concurrency = 33
		if _, err := s.OpenNodeSession(testContext, actor, protocol.SessionRequest{SessionID: session.SessionID, Report: healthyReport(), HeartbeatNS: int64(policy.Heartbeat), LeaseNS: int64(policy.Duration)}, policy); err != nil {
			t.Fatal("global concurrency rejected", err)
		}
		for i := 0; i < 3; i++ {
			queueBuild(t, s, p, fmt.Sprintf("large-global-%d", i), fmt.Sprintf("build-%d", i), false)
		}
		count := 0
		for i := 0; i < 3; i++ {
			grant, err := s.Claim(testContext, actor, protocol.ClaimRequest{SessionID: session.SessionID, ClaimKey: uuid.NewString()}, policy)
			if err != nil {
				t.Fatal(err)
			}
			if grant != nil {
				count++
			}
		}
		if count != 2 {
			t.Fatal("node capacity changed by global", count)
		}
	})
}

func TestConcurrentSameNameClaimsAndFrozenAuthorizationIntersection(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		actor, session, p, policy := leaseFixture(t, s)
		policy.Concurrency = 33
		if err := s.writer.Model(&nodeRecord{}).Where("id = ?", actor.ID).Update("max_capacity", 32).Error; err != nil {
			t.Fatal(err)
		}
		report := healthyReport()
		report.Capacity = 32
		if _, err := s.Heartbeat(testContext, actor, protocol.HeartbeatRequest{SessionID: session.SessionID, Report: report}, policy); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 20; i++ {
			queueBuild(t, s, p, fmt.Sprintf("same-concurrent-%d", i), "android", false)
		}
		grants := make(chan *protocol.LeaseGrant, 20)
		errs := make(chan error, 20)
		var wg sync.WaitGroup
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				grant, err := s.Claim(testContext, actor, protocol.ClaimRequest{SessionID: session.SessionID, ClaimKey: uuid.NewString()}, policy)
				if err != nil {
					errs <- err
				}
				grants <- grant
			}()
		}
		wg.Wait()
		close(grants)
		close(errs)
		for err := range errs {
			t.Fatal(err)
		}
		count := 0
		for grant := range grants {
			if grant != nil {
				count++
			}
		}
		if count != 1 {
			t.Fatal("same name duplicated", count)
		}
	})
	stores(t, func(t *testing.T, s *Store, opt Options) {
		_, _, p, policy := leaseFixture(t, s)
		other, session := openSession(t, s, "other", 1)
		queueBuild(t, s, p, "frozen-nodes", "android", true)
		if err := s.writer.Model(&projectRecord{}).Where("id = ?", p.ID).Update("nodes_json", `["linux","other"]`).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := s.Heartbeat(testContext, other, protocol.HeartbeatRequest{SessionID: session.SessionID, Report: leaseReport()}, policy); err != nil {
			t.Fatal(err)
		}
		grant, err := s.Claim(testContext, other, protocol.ClaimRequest{SessionID: session.SessionID, ClaimKey: uuid.NewString()}, policy)
		if err != nil || grant != nil {
			t.Fatal("current authorization expanded old snapshot", err)
		}
	})
}
