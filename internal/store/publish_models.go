package store

import (
	"mybuilds/internal/protocol"
	"time"
)

// 材料引用仅留在私有绑定；公开DTO不含凭据内容或路径。
type BindApplicationInput struct {
	Custom                  *protocol.CustomBindingEvidence `json:"custom,omitempty"`
	ProjectID               string                          `json:"project_id"`
	NodeID                  string                          `json:"node_id"`
	Store                   string                          `json:"store"`
	AppIdentifier           string                          `json:"app_identifier"`
	CredentialRef           string                          `json:"credential_ref"`
	UploadCertificateSHA256 string                          `json:"upload_certificate_sha256,omitempty"`
	AllowedTracks           []string                        `json:"allowed_tracks"`
}
type ApplicationView struct {
	VerificationSource      string     `json:"verification_source,omitempty"`
	ID                      string     `json:"id"`
	ProjectID               string     `json:"project_id"`
	NodeID                  string     `json:"node_id"`
	Store                   string     `json:"store"`
	AppIdentifier           string     `json:"app_identifier"`
	Status                  string     `json:"status"`
	AllowedTracks           []string   `json:"allowed_tracks"`
	UploadCertificateSHA256 string     `json:"upload_certificate_sha256,omitempty"`
	VerifiedAt              *time.Time `json:"verified_at,omitempty"`
}
type PublishView struct {
	ID                   string                         `json:"id"`
	ProjectID            string                         `json:"project_id"`
	BuildID              string                         `json:"build_id"`
	BuildName            string                         `json:"build_name"`
	Number               int64                          `json:"number"`
	SHA                  string                         `json:"sha"`
	AttemptID            string                         `json:"attempt_id"`
	NodeID               string                         `json:"node_id"`
	Store                string                         `json:"store"`
	AppIdentifier        string                         `json:"app_identifier"`
	Action               string                         `json:"action"`
	Track                string                         `json:"track,omitempty"`
	VersionName          string                         `json:"version_name"`
	VersionCode          int64                          `json:"version_code"`
	ArtifactID           string                         `json:"artifact_id"`
	ArtifactSize         int64                          `json:"artifact_size"`
	ArtifactSHA256       string                         `json:"artifact_sha256"`
	ReportSealDigest     string                         `json:"report_seal_digest,omitempty"`
	ReportIDs            []string                       `json:"report_ids"`
	Status               string                         `json:"status"`
	EvidenceCode         string                         `json:"evidence_code,omitempty"`
	Remote               protocol.PublishRemoteEvidence `json:"remote"`
	ApplicationProtected bool                           `json:"application_protected"`
	IntentDigest         string                         `json:"intent_digest"`
	GrantedAt            time.Time                      `json:"granted_at"`
	UpdatedAt            time.Time                      `json:"updated_at"`
}
type NodePublishState struct {
	IntentID      string            `json:"intent_id"`
	Ref           protocol.LeaseRef `json:"ref"`
	StepClosed    bool              `json:"step_closed"`
	Authorized    bool              `json:"authorized"`
	Status        string            `json:"status,omitempty"`
	ReceiptDigest string            `json:"receipt_digest,omitempty"`
}
type PublishQueryView struct {
	ID                string                  `json:"id"`
	IntentID          string                  `json:"intent_id,omitempty"`
	Kind              string                  `json:"kind"`
	Status            string                  `json:"status"`
	RequestedAt       time.Time               `json:"requested_at"`
	CompletedAt       *time.Time              `json:"completed_at,omitempty"`
	Reason            string                  `json:"reason,omitempty"`
	ObservedLifecycle string                  `json:"observed_lifecycle,omitempty"`
	Matches           []protocol.PublishMatch `json:"matches"`
}
type ConfirmPublishInput struct {
	IntentID             string                         `json:"intent_id"`
	Key                  string                         `json:"key"`
	ExpectedIntentDigest string                         `json:"expected_intent_digest"`
	Outcome              string                         `json:"outcome"`
	EvidenceCode         string                         `json:"evidence_code"`
	Note                 string                         `json:"note"`
	EvidenceSHA256       string                         `json:"evidence_sha256"`
	RemoteEvidence       protocol.PublishRemoteEvidence `json:"remote_evidence"`
}
type applicationRecord struct {
	ID                                                         string        `gorm:"primaryKey;size:36"`
	ProjectID                                                  string        `gorm:"not null;size:36"`
	Project                                                    projectRecord `gorm:"foreignKey:ProjectID;constraint:OnDelete:RESTRICT"`
	NodeID                                                     string        `gorm:"not null;size:36"`
	Node                                                       nodeRecord    `gorm:"foreignKey:NodeID;constraint:OnDelete:RESTRICT"`
	Store                                                      string        `gorm:"not null;uniqueIndex:application_identity;size:32"`
	AppIdentifier                                              string        `gorm:"not null;uniqueIndex:application_identity;size:255"`
	CredentialRef, UploadCertificateSHA256, TracksJSON, Status string
	VerificationSource, CustomEvidenceJSON                     string
	VerifiedAt                                                 *time.Time
	CreatedAt, UpdatedAt                                       time.Time
}

func (applicationRecord) TableName() string { return "application_bindings" }

type publishIntentRecord struct {
	ID                                                                                     string            `gorm:"primaryKey;size:36"`
	BindingID                                                                              string            `gorm:"not null;size:36;index"`
	Binding                                                                                applicationRecord `gorm:"foreignKey:BindingID;constraint:OnDelete:RESTRICT"`
	BuildID                                                                                string            `gorm:"not null;size:36;index"`
	Build                                                                                  buildRecord       `gorm:"foreignKey:BuildID;constraint:OnDelete:RESTRICT"`
	AttemptID                                                                              string            `gorm:"not null;uniqueIndex:publish_action;size:36"`
	Attempt                                                                                attemptRecord     `gorm:"foreignKey:AttemptID;constraint:OnDelete:RESTRICT"`
	StepIndex                                                                              int               `gorm:"not null;uniqueIndex:publish_action"`
	Action                                                                                 string            `gorm:"not null;uniqueIndex:publish_action;size:32"`
	ArtifactID                                                                             *string           `gorm:"size:36"`
	Artifact                                                                               *artifactRecord   `gorm:"foreignKey:ArtifactID;constraint:OnDelete:RESTRICT"`
	RequestDigest, GrantJSON, Status, ReceiptDigest, ReceiptJSON, EvidenceCode, RemoteJSON string
	CreatedAt, UpdatedAt                                                                   time.Time
}

func (publishIntentRecord) TableName() string { return "publish_intents" }

type applicationGuardRecord struct {
	BindingID string              `gorm:"primaryKey;size:36"`
	Binding   applicationRecord   `gorm:"foreignKey:BindingID;constraint:OnDelete:RESTRICT"`
	IntentID  string              `gorm:"not null;uniqueIndex;size:36"`
	Intent    publishIntentRecord `gorm:"foreignKey:IntentID;constraint:OnDelete:RESTRICT"`
	CreatedAt time.Time
}

func (applicationGuardRecord) TableName() string { return "application_guards" }

type publishQueryRecord struct {
	ID                                                  string               `gorm:"primaryKey;size:36"`
	BindingID                                           string               `gorm:"not null;size:36;index"`
	Binding                                             applicationRecord    `gorm:"foreignKey:BindingID;constraint:OnDelete:RESTRICT"`
	IntentID                                            *string              `gorm:"size:36;index"`
	Intent                                              *publishIntentRecord `gorm:"foreignKey:IntentID;constraint:OnDelete:RESTRICT"`
	Kind, ActorID, Status, SessionID, Nonce, ResultJSON string
	ExpiresAt, CreatedAt                                time.Time
	CompletedAt                                         *time.Time
}

func (publishQueryRecord) TableName() string { return "publish_queries" }

type publishDecisionRecord struct {
	ID                string              `gorm:"primaryKey;size:36"`
	IntentID          string              `gorm:"not null;size:36"`
	Intent            publishIntentRecord `gorm:"foreignKey:IntentID;constraint:OnDelete:RESTRICT"`
	ActorID           string              `gorm:"not null;uniqueIndex:publish_decision"`
	Key               string              `gorm:"not null;uniqueIndex:publish_decision;size:36"`
	Digest, InputJSON string
	CreatedAt         time.Time
}

func (publishDecisionRecord) TableName() string { return "publish_decisions" }
