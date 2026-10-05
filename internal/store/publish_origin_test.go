package store

import (
	"github.com/google/uuid"
	"mybuilds/internal/protocol"
	"strings"
	"testing"
)

func TestPublishFrozenFileRejectsAdditionalAmbiguousOrForeignSource(t *testing.T) {
	for _, source := range []string{"output/app.aab", "foreign/app.aab"} {
		t.Run(source, func(t *testing.T) {
			stores(t, func(t *testing.T, s *Store, _ Options) {
				a, g, in := publishFixture(t, s)
				// 真实同producer已完成声明。额外节点声明不应使中央任选一份发布。
				id := uuid.NewString()
				var row stepRecord
				if e := s.writer.First(&row, "build_id = ? AND phase = ? AND \"index\" = ?", g.Ref.BuildID, "ordinary", 1).Error; e != nil {
					t.Fatal(e)
				}
				// 首个真实快照仅含一个ID，新增未经finished声明的文件本身先被拒。
				_, e := s.CommitArtifact(testContext, a, ArtifactCommit{Declaration: protocol.ArtifactDeclaration{Ref: g.Ref, ID: id, Seq: 2, Phase: "ordinary", Index: 1, Step: "package", Name: "app.aab", SourcePath: source, Size: 123, SHA256: strings.Repeat("b", 64)}, StorageID: uuid.NewString()})
				if e != ErrArtifactConflict {
					t.Fatalf("undeclared artifact accepted: %v", e)
				}
				grant, e := s.AuthorizePublish(testContext, a, in)
				if e != nil || grant.ArtifactID != in.ArtifactID {
					t.Fatal("legacy exact source failed", e)
				}
			})
		})
	}
}

func TestPublishFixedEvidenceRejectsRawCodeAndStage(t *testing.T) {
	stores(t, func(t *testing.T, s *Store, _ Options) {
		a, g, in := publishFixture(t, s)
		grant, e := s.AuthorizePublish(testContext, a, in)
		if e != nil {
			t.Fatal(e)
		}
		for _, field := range []string{"code", "stage"} {
			receipt := protocol.PublishReceipt{IntentID: grant.IntentID, Ref: g.Ref, AuthorizationDigest: grant.AuthorizationDigest, Status: "unknown", StopConfirmed: true, EvidenceCode: "result_unconfirmed", MutationStage: "upload"}
			if field == "code" {
				receipt.EvidenceCode = "private/raw/detail"
			} else {
				receipt.MutationStage = "private/raw/detail"
			}
			receipt.Digest, _ = protocol.PublishReceiptDigest(receipt)
			if _, e = s.RecordPublish(testContext, a, receipt); e != ErrInvalid {
				t.Fatalf("raw %s accepted: %v", field, e)
			}
		}
	})
}

func TestArtifactSourcePathIsPrivateAndRelative(t *testing.T) {
	base := protocol.ArtifactDeclaration{Ref: protocol.LeaseRef{NodeID: uuid.NewString(), SessionID: uuid.NewString(), BuildID: uuid.NewString(), AttemptID: uuid.NewString(), LeaseID: uuid.NewString(), Epoch: 1}, ID: uuid.NewString(), Seq: 1, Phase: "ordinary", Index: 1, Step: "package", Name: "app.aab", Size: 1, SHA256: strings.Repeat("a", 64)}
	for _, source := range []string{"../app.aab", "/app.aab", "a/../app.aab", "a\\app.aab", "a/other.aab"} {
		base.SourcePath = source
		if validArtifactDeclaration(base) {
			t.Fatalf("invalid source %q", source)
		}
	}
	base.SourcePath = "output/app.aab"
	if !validArtifactDeclaration(base) {
		t.Fatal("actual relative source rejected")
	}
}
