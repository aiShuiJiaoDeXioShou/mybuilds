package store

import (
	"mybuilds/internal/protocol"
	"time"
)

// 审批记录独立保存旧Ref；新的lease不能重写已确认的原归属。
type approvalRecord struct {
	ID                                                  string        `gorm:"primaryKey;size:36"`
	BuildID                                             string        `gorm:"not null;size:36;uniqueIndex:approval_build_index;uniqueIndex:approval_build_revision"`
	Build                                               buildRecord   `gorm:"foreignKey:BuildID;constraint:OnDelete:RESTRICT"`
	AttemptID                                           string        `gorm:"not null;size:36"`
	Attempt                                             attemptRecord `gorm:"foreignKey:AttemptID;constraint:OnDelete:RESTRICT"`
	NodeID                                              string        `gorm:"not null;size:36;index"`
	Node                                                nodeRecord    `gorm:"foreignKey:NodeID;constraint:OnDelete:RESTRICT"`
	OrdinaryIndex                                       int           `gorm:"not null;uniqueIndex:approval_build_index"`
	Revision                                            int64         `gorm:"not null;uniqueIndex:approval_build_revision;check:revision > 0"`
	StepName                                            string
	ResumeRefJSON                                       string `gorm:"not null;default:''"`
	ResourceID, OwnershipDigest                         string
	CheckpointRefJSON                                   string `gorm:"not null"`
	EventSeq                                            int64
	EventDigest, CheckpointDigest, SnapshotDigest       string
	WorkspaceID, ResultID                               string
	NextOrdinaryIndex                                   int
	RemainingBudgetNS                                   *int64
	RemainingPostBudgetNS                               int64
	LastLogSeq, LastLogOffset, LastArtifactSeq          int64
	CheckpointJSON, LedgerJSON                          string
	StopConfirmed, CleanupFailed, SystemResourcesClosed bool
	State                                               string `gorm:"not null;index"`
	Decision, DecisionActor, Note, DecisionDigest       string
	DecisionActorID                                     *string         `gorm:"size:36"`
	Actor                                               *identityRecord `gorm:"foreignKey:DecisionActorID;constraint:OnDelete:RESTRICT"`
	DecidedAt                                           *time.Time
	CreatedAt                                           time.Time
}

func (approvalRecord) TableName() string { return "approvals" }

type ApprovalFilter struct {
	ProjectID, State string
	Page             Page
}
type ApprovalDecision struct {
	ApprovalID       string `json:"approval_id"`
	Revision         int64  `json:"revision"`
	CheckpointDigest string `json:"checkpoint_digest"`
	Decision         string `json:"decision"`
	Note             string `json:"note"`
}

// 公开审批仅显示安全证据与真实决定，不输出快照、命令、秘密或节点目录。
type ApprovalView struct {
	ID                    string                   `json:"id"`
	BuildID               string                   `json:"build_id"`
	ProjectID             string                   `json:"project_id"`
	Project               string                   `json:"project"`
	BuildName             string                   `json:"build_name"`
	Number                int64                    `json:"number"`
	Step                  string                   `json:"step"`
	Index                 int                      `json:"index"`
	Revision              int64                    `json:"revision"`
	CheckpointDigest      string                   `json:"checkpoint_digest"`
	State                 string                   `json:"state"`
	NodeName              string                   `json:"node_name"`
	ArtifactIDs           []string                 `json:"artifact_ids"`
	Reports               *protocol.ReportEvidence `json:"reports,omitempty"`
	ReportSealDigest      string                   `json:"report_seal_digest,omitempty"`
	RemainingBudgetNS     *int64                   `json:"remaining_budget_ns"`
	RemainingPostBudgetNS int64                    `json:"remaining_post_budget_ns"`
	Decision              string                   `json:"decision,omitempty"`
	DecisionActor         string                   `json:"decision_actor,omitempty"`
	DecidedAt             *time.Time               `json:"decided_at,omitempty"`
	NoteDigest            string                   `json:"note_digest,omitempty"`
	CreatedAt             time.Time                `json:"created_at"`
}
