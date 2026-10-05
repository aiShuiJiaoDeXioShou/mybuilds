package store

import (
	"mybuilds/internal/protocol"
	"reflect"
	"testing"
	"time"

	"mybuilds/internal/config"
)

func TestRetryTransactionRollbackPreservesNumberAndEvidence(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		p, id := cancelledQueued(t, s)
		queueBuild(t, s, p, "blocker", "build", false)
		if err := s.writer.Exec("CREATE UNIQUE INDEX retry008_fixture_failure ON builds(name) WHERE status = 'queued'").Error; err != nil {
			t.Fatal(err)
		}
		before := recoveryRows(t, s)
		current, _ := s.GetProject(testContext, p.Name)
		var batches, requests, audits int64
		s.db.Model(&batchRecord{}).Count(&batches)
		s.db.Model(&requestRecord{}).Count(&requests)
		s.db.Model(&auditRecord{}).Count(&audits)
		if _, err := s.Retry(testContext, localAdmin, RetryInput{BuildID: id, Key: "rollback"}); err != ErrConflict {
			t.Fatal("真实约束失败", err)
		}
		if !reflect.DeepEqual(before, recoveryRows(t, s)) {
			t.Fatal("回滚留下新构建")
		}
		after, _ := s.GetProject(testContext, p.Name)
		if after.NextNumber != current.NextNumber {
			t.Fatal("回滚消耗编号")
		}
		for _, item := range []struct {
			model    any
			expected int64
		}{{&batchRecord{}, batches}, {&requestRecord{}, requests}, {&auditRecord{}, audits}} {
			var count int64
			s.db.Model(item.model).Count(&count)
			if count != item.expected {
				t.Fatal("部分事务提交")
			}
		}
	})
}
func TestRetryUploadPermissionPrecedesSkippedCondition(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		p, err := s.CreateProject(testContext, localAdmin, projectInput("upload-app"))
		if err != nil {
			t.Fatal(err)
		}
		input := enqueueInput(p, "original-upload")
		input.AllowUpload = true
		input.Builds[0].Snapshot.Definition.Steps[0] = config.Step{Kind: "upload", Name: "upload", Target: "google_play", AppIdentifier: "com.example.app", File: "app.aab", Credentials: "${DECLARED_SECRET}", When: &config.When{Params: map[string]string{"version": "never"}}}
		input.Builds[0].Steps[0].Kind = "upload"
		input.Builds[0].Steps[0].Name = "upload"
		input.Builds[0].Steps[0].Condition = "skipped"
		input.Builds[0].Steps[0].Status = "skipped"
		out, err := s.Enqueue(testContext, input)
		if err != nil {
			t.Fatal(err)
		}
		id := out.Builds[0].ID
		if _, err = s.Cancel(testContext, localAdmin, id); err != nil {
			t.Fatal(err)
		}
		token, _ := s.CreateToken(testContext, localAdmin, "trigger")
		trigger, _ := s.Authenticate(testContext, token.Token)
		if _, err = s.Retry(testContext, trigger, RetryInput{BuildID: id, Key: "trigger", AllowUpload: true}); err != ErrForbidden {
			t.Fatal("falsewhen越过admin", err)
		}
		if _, err = s.Retry(testContext, localAdmin, RetryInput{BuildID: id, Key: "admin-no-allow"}); err != ErrForbidden {
			t.Fatal("falsewhen越过显式许可", err)
		}
		if _, err = s.Retry(testContext, localAdmin, RetryInput{BuildID: id, Key: "admin-allow", AllowUpload: true}); err != nil {
			t.Fatal("合法许可后false条件仍应跳过", err)
		}
	})
}

func TestRetryAttemptWithoutActualStopEvidenceIsNotKnownStopped(t *testing.T) {
	for _, status := range []string{"failed", "interrupted"} {
		t.Run(status, func(t *testing.T) {
			stores(t, func(t *testing.T, s *Store, opt Options) {
				_, g, _ := claimed(t, s)
				// 真实Claim已有attempt，checkout尚未任何step intent；手改终态不能凭空证明其进程停止。
				if err := s.writer.Model(&buildRecord{}).Where("id = ?", g.Ref.BuildID).Updates(map[string]any{"status": status, "stop_unconfirmed": false, "reason": "checkout_error"}).Error; err != nil {
					t.Fatal(err)
				}
				before := recoveryRows(t, s)
				if _, err := s.Retry(testContext, localAdmin, RetryInput{BuildID: g.Ref.BuildID, Key: "unknown-checkout"}); err != ErrStopUnconfirmed {
					t.Fatal("只凭终态/未intent错误推断停止", err)
				}
				if !reflect.DeepEqual(before, recoveryRows(t, s)) {
					t.Fatal("未知停止仍创建新记录")
				}
			})
		})
	}
}

func TestRetryActualConfirmedCheckoutFailureWithZeroStepActions(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, opt Options) {
		a, g, _ := claimed(t, s)
		progress := protocol.ExecutionProgress{Kind: "build_finished", Status: "failed", Reason: "checkout_error", PostPhase: "none", StopConfirmed: true, RemainingPostBudgetNS: int64(2 * time.Minute), ArtifactSteps: []protocol.ArtifactExpectation{}}
		terminal := event(g.Ref, 1, progress)
		accept(t, s, a, terminal)
		var receipt executionReceiptRecord
		s.db.First(&receipt, "build_id = ? AND seq = ?", g.Ref.BuildID, terminal.Seq)
		if receipt.Kind != "build_finished" || !receipt.StopKnown {
			t.Fatal("真实checkout终态未确认")
		}
		if _, err := s.Retry(testContext, localAdmin, RetryInput{BuildID: g.Ref.BuildID, Key: "checkout-confirmed"}); err != nil {
			t.Fatal("真实零动作已确认失败不能重试", err)
		}
	})
}
