package store

import (
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"time"
)

type WebhookActor struct {
	ProjectID, CredentialID, SecretFingerprint string
	PolicyVersion                              int64
}
type WebhookPolicy struct {
	ProjectID, Provider, RepositoryKey         string
	Enabled                                    bool
	CredentialID, SecretRef, SecretFingerprint string
	Generic                                    *config.GenericHookSettings
	BuildNames                                 []string
	Params                                     map[string]string
	BuildParams                                map[string]map[string]string
	QuietPeriod                                time.Duration
	AllowUpload                                bool
	PolicyVersion                              int64
}
type WebhookPolicyInput struct {
	Policy                WebhookPolicy
	Settings              config.ProjectSettings
	ExpectedPolicyVersion int64
}
type WebhookEvent struct {
	ID, ProjectID, CredentialID, WindowID string
	Event                                 protocol.WebhookEvent
	ReceivedAt                            time.Time
}
type WebhookReceipt struct {
	EventID, WindowID, Status, Reason string
	Replayed                          bool
}
type WebhookWindow struct {
	ID, ProjectID, GroupKey             string
	Generation, Revision, PolicyVersion int64
	State, Reason, Branch               string
	Policy                              WebhookPolicy
	OpenedAt, Deadline                  time.Time
	CandidateSHA, FinalSHA, BatchID     string
	BuildIDs, ReusedBuildIDs            []string
}
type WebhookBaseline struct {
	BuildID, SHA, AttemptID, ReceiptDigest string
	Seq                                    int64
	ConfirmedAt                            time.Time
}
type WebhookComparison struct {
	Name, Key string
	Baseline  *WebhookBaseline
	Changes   protocol.ChangeFacts
}
type WebhookCloseRef struct {
	WindowID                string
	Revision, PolicyVersion int64
}
type WebhookCloseInput struct {
	Ref                             WebhookCloseRef
	Actor                           WebhookActor
	SHA, Source, File, SourceDigest string
	SelectedBuilds                  []string
	Builds                          []PreparedBuild
	Comparisons                     []WebhookComparison
	HasUpload                       bool
}
type WebhookWindowResult struct {
	WindowID, State, Reason, SHA, BatchID string
	BuildIDs, ReusedBuildIDs              []string
	Replayed                              bool
}

// 持久行只保存正规化证据，不保存原body、headers或秘密。
type webhookPolicyRecord struct {
	ProjectID  string        `gorm:"primaryKey;size:36"`
	Project    projectRecord `gorm:"foreignKey:ProjectID;constraint:OnDelete:RESTRICT"`
	PolicyJSON string
}

func (webhookPolicyRecord) TableName() string { return "webhook_policies" }

type webhookEventRecord struct {
	ID                                                   string        `gorm:"primaryKey;size:36"`
	ProjectID                                            string        `gorm:"not null;size:36;index"`
	Project                                              projectRecord `gorm:"foreignKey:ProjectID;constraint:OnDelete:RESTRICT"`
	CredentialID, Provider, BodyKey, WindowID, EventJSON string
	ReceivedAt                                           time.Time
}

func (webhookEventRecord) TableName() string { return "webhook_events" }

type webhookAliasRecord struct {
	ID         string             `gorm:"primaryKey;size:36"`
	ProjectID  string             `gorm:"not null;uniqueIndex:hook_delivery"`
	Provider   string             `gorm:"not null;uniqueIndex:hook_delivery"`
	DeliveryID string             `gorm:"not null;uniqueIndex:hook_delivery"`
	EventID    string             `gorm:"not null;size:36"`
	Event      webhookEventRecord `gorm:"foreignKey:EventID;constraint:OnDelete:RESTRICT"`
	Digest     string
}

func (webhookAliasRecord) TableName() string { return "webhook_aliases" }

type webhookWindowRecord struct {
	ID                                                                                            string        `gorm:"primaryKey;size:36;index:hook_due,priority:3"`
	ProjectID                                                                                     string        `gorm:"not null;size:36"`
	Project                                                                                       projectRecord `gorm:"foreignKey:ProjectID;constraint:OnDelete:RESTRICT"`
	GroupKey                                                                                      string        `gorm:"not null;uniqueIndex:hook_generation"`
	Generation                                                                                    int64         `gorm:"not null;uniqueIndex:hook_generation"`
	Revision, PolicyVersion                                                                       int64
	State                                                                                         string `gorm:"index:hook_due,priority:1"`
	Reason, Branch, PolicyJSON, CandidateSHA, FinalSHA, BatchID, BuildIDsJSON, ReusedBuildIDsJSON string
	OpenedAt                                                                                      time.Time
	Deadline                                                                                      time.Time `gorm:"index:hook_due,priority:2"`
}

func (webhookWindowRecord) TableName() string { return "webhook_windows" }
