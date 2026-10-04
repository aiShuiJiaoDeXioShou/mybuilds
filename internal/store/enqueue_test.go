package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"math"
	"mybuilds/internal/config"
	"strings"
	"sync"
	"testing"
	"time"
)

func enqueueInput(p Project, key string) EnqueueInput {
	value := "secret-param-marker"
	b := config.Build{Params: map[string]config.Parameter{"version": {Default: &value}}, Steps: []config.Step{{Kind: "run", Name: "compile", Run: "echo secret-script-marker"}}, Post: &config.Post{Always: []config.Step{{Kind: "run", Name: "cleanup", Run: "echo secret-post-marker"}}}}
	return EnqueueInput{Actor: localAdmin, ProjectID: p.ID, ProjectVersion: p.PolicyVersion, Key: key, RequestDigest: strings.Repeat("a", 64), SHA: strings.Repeat("b", 40), Branch: "main", Source: "repo", File: "mybuilds.yml", SourceDigest: strings.Repeat("c", 64), Builds: []PreparedBuild{{Name: "android", Status: "queued", Snapshot: BuildSnapshot{Definition: b, Params: map[string]string{"version": value}, Facts: map[string]string{"project": "forged", "build.number": "999", "node.name": "forged"}, Condition: "ready", Reasons: []string{"条件已满足"}, AllowedNodes: p.AllowedNodes, DefaultNode: p.DefaultNode}, PostBudgetNS: int64(2 * time.Minute), Steps: []StepProgress{{Phase: "ordinary", Index: 1, Name: "compile", Kind: "run", Condition: "ready", Status: "pending"}, {Phase: "always", Index: 1, Name: "cleanup", Kind: "run", Condition: "ready", Status: "pending"}}}}}
}
func TestEnqueueConcurrentIdempotency(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		p, err := s.CreateProject(testContext, localAdmin, projectInput("app"))
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		var mu sync.Mutex
		ids := map[string]bool{}
		numbers := map[int64]bool{}
		firstResponses := 0
		run := func(same bool) {
			t.Helper()
			errs := make(chan error, 20)
			for i := 0; i < 20; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					key := "same"
					if !same {
						key = fmt.Sprintf("different-%d", i)
					}
					result, e := s.Enqueue(testContext, enqueueInput(p, key))
					if e != nil {
						errs <- e
						return
					}
					mu.Lock()
					if !result.Replayed {
						firstResponses++
					}
					ids[result.ID] = true
					numbers[*result.Builds[0].Number] = true
					mu.Unlock()
				}(i)
			}
			wg.Wait()
			close(errs)
			for e := range errs {
				t.Fatalf("enqueue: %v", e)
			}
		}
		run(true)
		if len(ids) != 1 || len(numbers) != 1 || firstResponses != 1 {
			t.Fatal("same key duplicated")
		}
		run(false)
		if len(ids) != 21 || len(numbers) != 21 || firstResponses != 21 {
			t.Fatalf("different key lost: %d/%d", len(ids), len(numbers))
		}
		current, _ := s.GetProject(testContext, p.Name)
		if current.NextNumber != 22 {
			t.Fatal("counter incorrect")
		}
		found, err := s.FindRequest(testContext, localAdmin, "same", strings.Repeat("a", 64))
		if err != nil || found == nil || *found.Builds[0].Number != 1 {
			t.Fatal("find original")
		}
		if _, err = s.FindRequest(testContext, localAdmin, "same", strings.Repeat("d", 64)); !errors.Is(err, ErrConflict) {
			t.Fatal("digest conflict")
		}
		created, err := s.CreateToken(testContext, localAdmin, "trigger")
		if err != nil {
			t.Fatal(err)
		}
		actor, _ := s.Authenticate(testContext, created.Token)
		other := enqueueInput(current, "same")
		other.Actor = actor
		result, err := s.Enqueue(testContext, other)
		if err != nil || *result.Builds[0].Number != 22 {
			t.Fatal("cross identity scope")
		}
	})
}
func TestEnqueueSkippedRollbackAndFacts(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		p, err := s.CreateProject(testContext, localAdmin, projectInput("app"))
		if err != nil {
			t.Fatal(err)
		}
		input := enqueueInput(p, "mixed")
		skipped := input.Builds[0]
		skipped.Name = "ios"
		skipped.Status = "skipped"
		skipped.Snapshot.Condition = "skipped"
		input.Builds = append(input.Builds, skipped)
		result, err := s.Enqueue(testContext, input)
		if err != nil || len(result.Builds) != 2 || result.Builds[1].Number != nil || *result.Builds[0].Number != 1 {
			t.Fatalf("mixed: %v %v", result, err)
		}
		var row buildRecord
		if err = s.db.First(&row, "id = ?", result.Builds[0].ID).Error; err != nil {
			t.Fatal(err)
		}
		var snapshot BuildSnapshot
		if err = json.Unmarshal([]byte(row.SnapshotJSON), &snapshot); err != nil {
			t.Fatal(err)
		}
		if snapshot.Facts["project"] != p.Name || snapshot.Facts["build.number"] != "1" || snapshot.Facts["build.id"] != row.ID || snapshot.Facts["node.name"] != "" {
			t.Fatalf("spoofed facts: %v", snapshot.Facts)
		}
		if snapshot.Params["version"] != "secret-param-marker" || snapshot.Definition.Steps[0].Run != "echo secret-script-marker" {
			t.Fatal("internal snapshot lost")
		}
		input = enqueueInput(p, "all-skipped")
		input.Builds[0].Status = "skipped"
		input.Builds[0].Snapshot.Condition = "skipped"
		skippedResult, err := s.Enqueue(testContext, input)
		if err != nil || skippedResult.Builds[0].Number != nil {
			t.Fatal("skipped numbered")
		}
		p, _ = s.GetProject(testContext, p.Name)
		if p.NextNumber != 2 {
			t.Fatal("skipped consumed number")
		}
		bad := enqueueInput(p, "bad")
		bad.Builds = append(bad.Builds, bad.Builds[0])
		if _, err = s.Enqueue(testContext, bad); !errors.Is(err, ErrInvalid) {
			t.Fatalf("bad batch: %v", err)
		}
		// 真实 DB 约束让第二项插入失败，验证首项、计数和幂等全部回滚。
		if err = s.db.Exec("CREATE UNIQUE INDEX store006_test_one_name ON builds(name) WHERE status = 'queued'").Error; err != nil {
			t.Fatal(err)
		}
		failed := enqueueInput(p, "db-failure")
		failed.Builds[0].Name = "unique-first"
		second := failed.Builds[0]
		second.Name = "android"
		failed.Builds = append(failed.Builds, second)
		if _, err = s.Enqueue(testContext, failed); !errors.Is(err, ErrConflict) {
			t.Fatalf("DB rollback: %v", err)
		}
		current, _ := s.GetProject(testContext, p.Name)
		if current.NextNumber != 2 {
			t.Fatal("failed transaction consumed number")
		}
		found, err := s.FindRequest(testContext, localAdmin, "db-failure", failed.RequestDigest)
		if err != nil || found != nil {
			t.Fatal("failed transaction saved key")
		}
		if err = s.db.Exec("DROP INDEX store006_test_one_name").Error; err != nil {
			t.Fatal(err)
		}
		if err = s.DeleteProject(testContext, localAdmin, p.Name); !errors.Is(err, ErrConflict) {
			t.Fatal("history deleted")
		}
		status, _ := s.Status(testContext)
		if status.Projects != 1 || status.Queued != 1 || status.Skipped != 2 {
			t.Fatalf("counts: %+v", status)
		}
	})
}
func TestEnqueueAuthorizationVersionAndOverflow(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		input := projectInput("overflow")
		input.BuildNumberStart = math.MaxInt64
		p, err := s.CreateProject(testContext, localAdmin, input)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.Enqueue(testContext, enqueueInput(p, "overflow")); !errors.Is(err, ErrConflict) {
			t.Fatalf("overflow: %v", err)
		}
		p, err = s.CreateProject(testContext, localAdmin, projectInput("app"))
		if err != nil {
			t.Fatal(err)
		}
		stale := enqueueInput(p, "stale")
		stale.ProjectVersion = 2
		if _, err = s.Enqueue(testContext, stale); !errors.Is(err, ErrConflict) {
			t.Fatal("stale version")
		}
		created, _ := s.CreateToken(testContext, localAdmin, "trigger")
		actor, _ := s.Authenticate(testContext, created.Token)
		forbidden := enqueueInput(p, "upload")
		forbidden.Actor = actor
		forbidden.HasUpload = true
		forbidden.AllowUpload = true
		if _, err = s.Enqueue(testContext, forbidden); !errors.Is(err, ErrForbidden) {
			t.Fatal("trigger upload")
		}
		if err = s.RevokeToken(testContext, localAdmin, actor.ID); err != nil {
			t.Fatal(err)
		}
		revoked := enqueueInput(p, "revoked")
		revoked.Actor = actor
		if _, err = s.Enqueue(testContext, revoked); !errors.Is(err, ErrUnauthorized) {
			t.Fatal("revoked enqueue")
		}
		repeated := enqueueInput(p, "steps")
		repeated.Builds[0].Steps = append(repeated.Builds[0].Steps, repeated.Builds[0].Steps[0])
		if _, err = s.Enqueue(testContext, repeated); !errors.Is(err, ErrInvalid) {
			t.Fatal("duplicate progress")
		}
		if err = s.write(testContext, func(tx *gorm.DB) error {
			return tx.Create(&buildRecord{ID: "constraint", BatchID: "missing", ProjectID: p.ID, Name: "bad"}).Error
		}); !errors.Is(err, ErrConflict) {
			t.Fatal("build FK unenforced")
		}
	})
}
