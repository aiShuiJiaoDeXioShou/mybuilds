package store

import (
	"mybuilds/internal/protocol"
	"time"
)

type NodeActor struct{ ID, CredentialID string }
type NodeInput struct {
	Name     string
	Labels   []string
	Capacity int
}
type NodeCreated struct {
	Node  NodeView `json:"node"`
	Token string   `json:"token"`
}
type NodeView struct {
	ID                string               `json:"id"`
	Name              string               `json:"name"`
	State             string               `json:"state"`
	OS                string               `json:"os"`
	Arch              string               `json:"arch"`
	Labels            []string             `json:"labels"`
	MaxCapacity       int                  `json:"max_capacity"`
	LocalCapacity     int                  `json:"local_capacity"`
	EffectiveCapacity int                  `json:"effective_capacity"`
	Running           int                  `json:"running"`
	Healthy           bool                 `json:"healthy"`
	Quarantined       bool                 `json:"quarantined"`
	SessionActive     bool                 `json:"session_active"`
	Tools             []protocol.ToolCheck `json:"tools"`
	LastHeartbeat     *time.Time           `json:"last_heartbeat,omitempty"`
	CreatedAt         time.Time            `json:"created_at"`
}
type NodeFilter struct{ Page Page }
type LeasePolicy struct {
	Concurrency         int
	Heartbeat, Duration time.Duration
}
type LogCommit struct {
	Ref               protocol.LeaseRef
	Seq, Offset, Size int64
	Digest, StorageID string
	RecordCount       int
}
type LogCommitted struct {
	Ack       protocol.LogAck
	StorageID string `json:"-"`
	Created   bool   `json:"-"`
}
type LogStored struct {
	BuildID     string    `json:"build_id"`
	AttemptID   string    `json:"attempt_id"`
	Seq         int64     `json:"seq"`
	Offset      int64     `json:"offset"`
	Size        int64     `json:"size"`
	Digest      string    `json:"digest"`
	StorageID   string    `json:"-"`
	RecordCount int       `json:"record_count"`
	CreatedAt   time.Time `json:"created_at"`
}
type ArtifactCommit struct {
	Declaration protocol.ArtifactDeclaration
	StorageID   string
	// 仅由服务端对稳定stage原XML解析产生，不接受HTTP请求中的自报结果。
	VerifiedJUnit *protocol.JUnitResult `json:"-"`
}
type ArtifactCommitted struct {
	View      protocol.ArtifactView
	StorageID string `json:"-"`
	Created   bool   `json:"-"`
}
type ArtifactStored struct {
	View      protocol.ArtifactView
	StorageID string `json:"-"`
}

type nodeRecord struct {
	ID                   string `gorm:"primaryKey;size:36"`
	Name                 string `gorm:"not null;uniqueIndex;size:64"`
	State                string `gorm:"not null"`
	LabelsJSON           string `gorm:"not null"`
	MaxCapacity          int    `gorm:"not null;check:max_capacity >= 1 AND max_capacity <= 32"`
	SessionID            *string
	CreatedAt, UpdatedAt time.Time
}

func (nodeRecord) TableName() string { return "nodes" }

type nodeCredentialRecord struct {
	ID        string     `gorm:"primaryKey;size:36"`
	NodeID    string     `gorm:"not null;index;size:36"`
	Node      nodeRecord `gorm:"foreignKey:NodeID;constraint:OnDelete:RESTRICT"`
	Digest    string     `gorm:"not null;uniqueIndex;size:64"`
	CreatedAt time.Time
	RevokedAt *time.Time
}

func (nodeCredentialRecord) TableName() string { return "node_credentials" }

type nodeSessionRecord struct {
	ID                       string               `gorm:"primaryKey;size:36"`
	NodeID                   string               `gorm:"not null;index;size:36"`
	Node                     nodeRecord           `gorm:"foreignKey:NodeID;constraint:OnDelete:RESTRICT"`
	CredentialID             string               `gorm:"not null;size:36"`
	Credential               nodeCredentialRecord `gorm:"foreignKey:CredentialID;constraint:OnDelete:RESTRICT"`
	OS, Arch, ToolsJSON      string
	Capacity                 int
	HeartbeatNS, LeaseNS     int64
	CreatedAt, LastHeartbeat time.Time
}

func (nodeSessionRecord) TableName() string { return "node_sessions" }

type attemptRecord struct {
	ID             string               `gorm:"primaryKey;size:36"`
	BuildID        string               `gorm:"not null;uniqueIndex;size:36"`
	Build          buildRecord          `gorm:"foreignKey:BuildID;constraint:OnDelete:RESTRICT"`
	NodeID         string               `gorm:"not null;index;size:36"`
	Node           nodeRecord           `gorm:"foreignKey:NodeID;constraint:OnDelete:RESTRICT"`
	SessionID      string               `gorm:"not null;uniqueIndex:session_claim;size:36"`
	Session        nodeSessionRecord    `gorm:"foreignKey:SessionID;constraint:OnDelete:RESTRICT"`
	CredentialID   string               `gorm:"not null;size:36"`
	Credential     nodeCredentialRecord `gorm:"foreignKey:CredentialID;constraint:OnDelete:RESTRICT"`
	ClaimKey       string               `gorm:"not null;uniqueIndex:session_claim;size:36"`
	LeaseID        string               `gorm:"not null;uniqueIndex;size:36"`
	Epoch          int64                `gorm:"not null;check:epoch > 0"`
	LeaseExpiresAt time.Time
	CreatedAt      time.Time
}

func (attemptRecord) TableName() string { return "attempts" }

type executionReceiptRecord struct {
	Kind      string        `gorm:"not null;default:''"`
	StopKnown bool          `gorm:"not null;default:false"`
	ID        string        `gorm:"primaryKey;size:36"`
	BuildID   string        `gorm:"not null;uniqueIndex:event_sequence;size:36"`
	Build     buildRecord   `gorm:"foreignKey:BuildID;constraint:OnDelete:RESTRICT"`
	AttemptID string        `gorm:"not null;uniqueIndex:event_sequence;size:36"`
	Attempt   attemptRecord `gorm:"foreignKey:AttemptID;constraint:OnDelete:RESTRICT"`
	Seq       int64         `gorm:"not null;uniqueIndex:event_sequence;check:seq > 0"`
	Digest    string
	CreatedAt time.Time
}

func (executionReceiptRecord) TableName() string { return "execution_receipts" }

type stopConfirmationRecord struct {
	ID                                                      string        `gorm:"primaryKey;size:36"`
	BuildID                                                 string        `gorm:"not null;size:36"`
	Build                                                   buildRecord   `gorm:"foreignKey:BuildID;constraint:OnDelete:RESTRICT"`
	AttemptID                                               string        `gorm:"not null;uniqueIndex;size:36"`
	Attempt                                                 attemptRecord `gorm:"foreignKey:AttemptID;constraint:OnDelete:RESTRICT"`
	NodeID, SessionID, LeaseID, ActorID, EvidenceCode, Note string
	Epoch                                                   int64
	CreatedAt                                               time.Time
}

func (stopConfirmationRecord) TableName() string { return "stop_confirmations" }

type logChunkRecord struct {
	ID                string        `gorm:"primaryKey;size:36"`
	BuildID           string        `gorm:"not null;uniqueIndex:log_sequence;size:36"`
	Build             buildRecord   `gorm:"foreignKey:BuildID;constraint:OnDelete:RESTRICT"`
	AttemptID         string        `gorm:"not null;uniqueIndex:log_sequence;size:36"`
	Attempt           attemptRecord `gorm:"foreignKey:AttemptID;constraint:OnDelete:RESTRICT"`
	Seq               int64         `gorm:"not null;uniqueIndex:log_sequence;check:seq > 0"`
	Offset, Size      int64
	Digest, StorageID string
	RecordCount       int
	CreatedAt         time.Time
}

func (logChunkRecord) TableName() string { return "log_chunks" }

type artifactRecord struct {
	Purpose                              string        `gorm:"not null;default:''"`
	ReportRevision                       int64         `gorm:"not null;default:0"`
	ReportKey                            string        `gorm:"not null;default:''"`
	VerifiedJUnitJSON                    string        `gorm:"column:verified_junit_json;not null;default:''"`
	ID                                   string        `gorm:"primaryKey;size:36"`
	BuildID                              string        `gorm:"not null;index;size:36"`
	Build                                buildRecord   `gorm:"foreignKey:BuildID;constraint:OnDelete:RESTRICT"`
	AttemptID                            string        `gorm:"not null;uniqueIndex:artifact_sequence;size:36"`
	Attempt                              attemptRecord `gorm:"foreignKey:AttemptID;constraint:OnDelete:RESTRICT"`
	Seq                                  int64         `gorm:"not null;uniqueIndex:artifact_sequence;check:seq > 0"`
	Phase, Step, Name, SHA256, StorageID string
	Index                                int
	Size                                 int64
	CreatedAt                            time.Time
}

func (artifactRecord) TableName() string { return "artifacts" }
