package store

import (
	"mybuilds/internal/config"
	"time"
)

type Options struct{ Driver, DSN string }
type Actor struct{ ID, Role string }
type Page struct{ Limit, Offset int }
type Group struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type TokenCreated struct {
	ID    string `json:"id"`
	Role  string `json:"role"`
	Token string `json:"token"`
}
type TokenView struct {
	ID        string     `json:"id"`
	Role      string     `json:"role"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}
type ProjectInput struct {
	Name, Group, Repository, Provider, DefaultNode string
	Branches, AllowedNodes                         []string
	BuildNumberStart                               int64
	Settings                                       config.ProjectSettings
}
type Project struct {
	ID, Name, GroupID, GroupName, Provider, DefaultNode string
	Repository                                          string `json:"-"`
	Branches, AllowedNodes                              []string
	Settings                                            config.ProjectSettings `json:"-"`
	NextNumber, PolicyVersion                           int64
	CreatedAt, UpdatedAt                                time.Time
}
type ProjectFilter struct {
	Group string
	Page  Page
}
type EnqueueInput struct {
	Actor                                                       Actor
	ProjectID                                                   string
	ProjectVersion                                              int64
	Key, RequestDigest, SHA, Branch, Source, File, SourceDigest string
	HasUpload, AllowUpload                                      bool
	Builds                                                      []PreparedBuild
}
type BuildSnapshot struct {
	Definition    config.Build
	Params, Facts map[string]string
	Condition     string
	Reasons       []string
	AllowedNodes  []string
	DefaultNode   string
}
type PreparedBuild struct {
	Name, Status, Reason string
	Snapshot             BuildSnapshot `json:"-"`
	InitialBudgetNS      *int64
	PostBudgetNS         int64
	Steps                []StepProgress
}
type StepProgress struct {
	Phase         string   `json:"phase"`
	Index         int      `json:"index"`
	Name          string   `json:"name"`
	Kind          string   `json:"kind"`
	Condition     string   `json:"condition"`
	Status        string   `json:"status"`
	Reasons       []string `json:"reasons"`
	ElapsedNS     int64    `json:"elapsed_ns"`
	Intent        bool     `json:"intent"`
	Started       bool     `json:"started"`
	StopConfirmed bool     `json:"stop_confirmed"`
	CleanupFailed bool     `json:"cleanup_failed"`
	Reason        string   `json:"reason,omitempty"`
	ExitCode      int      `json:"exit_code"`
}
type BatchResult struct {
	Replayed bool        `json:"-"`
	ID       string      `json:"batch_id"`
	SHA      string      `json:"sha"`
	Builds   []BuildView `json:"builds"`
}
type BuildFilter struct {
	Project, Group, BuildName, BatchID, Status string
	Page                                       Page
}
type BuildView struct {
	RetryOf               string         `json:"retry_of,omitempty"`
	ID                    string         `json:"id"`
	Project               string         `json:"project"`
	Group                 string         `json:"group"`
	BatchID               string         `json:"batch_id"`
	Name                  string         `json:"build_name"`
	Number                *int64         `json:"number"`
	Status                string         `json:"status"`
	Reason                string         `json:"reason,omitempty"`
	SHA                   string         `json:"sha"`
	Branch                string         `json:"branch"`
	Source                string         `json:"source"`
	File                  string         `json:"file"`
	SourceDigest          string         `json:"source_digest"`
	ParameterKeys         []string       `json:"parameter_keys"`
	Condition             string         `json:"condition"`
	Reasons               []string       `json:"reasons"`
	InitialBudgetNS       *int64         `json:"initial_budget_ns"`
	RemainingBudgetNS     *int64         `json:"remaining_budget_ns"`
	PostBudgetNS          int64          `json:"post_budget_ns"`
	Steps                 []StepProgress `json:"steps"`
	Post                  []StepProgress `json:"post"`
	NodeID                string         `json:"node_id,omitempty"`
	NodeName              string         `json:"node_name,omitempty"`
	SessionID             string         `json:"session_id,omitempty"`
	AttemptID             string         `json:"attempt_id,omitempty"`
	LeaseID               string         `json:"lease_id,omitempty"`
	LeaseEpoch            int64          `json:"lease_epoch"`
	CancelRequested       bool           `json:"cancel_requested"`
	StopUnconfirmed       bool           `json:"stop_unconfirmed"`
	RemainingPostBudgetNS int64          `json:"remaining_post_budget_ns"`
	PostPhase             string         `json:"post_phase,omitempty"`
	CreatedAt             time.Time      `json:"created_at"`
}
type QueueStatus struct{ Projects, Queued, Skipped, Running, Interrupted, Nodes, HealthyNodes int64 }

// 数据库记录私有，公开响应只编码上面的安全视图。
type groupRecord struct {
	ID                   string `gorm:"primaryKey;size:36"`
	Name                 string `gorm:"not null;uniqueIndex;size:64"`
	CreatedAt, UpdatedAt time.Time
}

func (groupRecord) TableName() string { return "project_groups" }

type projectRecord struct {
	ID                                    string      `gorm:"primaryKey;size:36"`
	Name                                  string      `gorm:"not null;uniqueIndex;size:64"`
	GroupID                               string      `gorm:"not null;size:36"`
	Group                                 groupRecord `gorm:"foreignKey:GroupID;constraint:OnDelete:RESTRICT"`
	Repository, Provider, DefaultNode     string
	BranchesJSON, NodesJSON, SettingsJSON string
	NextNumber                            int64 `gorm:"not null;check:next_number > 0"`
	PolicyVersion                         int64 `gorm:"not null;check:policy_version > 0"`
	CreatedAt, UpdatedAt                  time.Time
}

func (projectRecord) TableName() string { return "projects" }

type identityRecord struct {
	ID        string `gorm:"primaryKey;size:36"`
	Digest    string `gorm:"not null;uniqueIndex;size:64"`
	Role      string `gorm:"not null"`
	CreatedAt time.Time
	RevokedAt *time.Time
}

func (identityRecord) TableName() string { return "identities" }

type metadataRecord struct {
	ID                  int  `gorm:"primaryKey;autoIncrement:false"`
	IdentityInitialized bool `gorm:"not null"`
}

func (metadataRecord) TableName() string { return "control_metadata" }

type auditRecord struct {
	ID                                  string `gorm:"primaryKey;size:36"`
	ActorID, Action, ObjectID, From, To string
	CreatedAt                           time.Time
}

func (auditRecord) TableName() string { return "audits" }

type batchRecord struct {
	ID                                                  string        `gorm:"primaryKey;size:36"`
	ProjectID                                           string        `gorm:"not null;size:36"`
	Project                                             projectRecord `gorm:"foreignKey:ProjectID;constraint:OnDelete:RESTRICT"`
	IdentityID, SHA, Branch, Source, File, SourceDigest string
	CreatedAt                                           time.Time
}

func (batchRecord) TableName() string { return "batches" }

type buildRecord struct {
	RetryOf                                                  *string       `gorm:"size:36;index"`
	RetryOriginal                                            *buildRecord  `gorm:"foreignKey:RetryOf;references:ID;constraint:OnDelete:RESTRICT"`
	ID                                                       string        `gorm:"primaryKey;size:36;index:build_status_created,priority:3"`
	BatchID                                                  string        `gorm:"not null;size:36"`
	Batch                                                    batchRecord   `gorm:"foreignKey:BatchID;constraint:OnDelete:RESTRICT"`
	ProjectID                                                string        `gorm:"not null;size:36;uniqueIndex:project_number"`
	Project                                                  projectRecord `gorm:"foreignKey:ProjectID;constraint:OnDelete:RESTRICT"`
	Number                                                   *int64        `gorm:"uniqueIndex:project_number"`
	Position                                                 int
	Name                                                     string `gorm:"not null"`
	Status                                                   string `gorm:"index:build_status_created,priority:1;index:build_node_status,priority:2;index:build_status_expiry,priority:1"`
	Reason, SnapshotJSON                                     string
	NodeID                                                   *string    `gorm:"index:build_node_status,priority:1;size:36"`
	Node                                                     nodeRecord `gorm:"foreignKey:NodeID;constraint:OnDelete:RESTRICT"`
	SessionID, AttemptID, LeaseID                            *string
	LeaseEpoch                                               int64      `gorm:"not null;default:0;check:lease_epoch >= 0"`
	LeaseExpiresAt                                           *time.Time `gorm:"index:build_status_expiry,priority:2"`
	CancelRequested                                          bool       `gorm:"not null;default:false"`
	StopUnconfirmed                                          bool       `gorm:"not null;default:false"`
	RemainingPostBudgetNS                                    int64
	PostPhase                                                string
	LastEventSeq, LastLogSeq, LastLogOffset, LastArtifactSeq int64
	ParameterKeysJSON, Condition, ReasonsJSON                string
	InitialBudgetNS, RemainingBudgetNS                       *int64
	PostBudgetNS                                             int64
	CreatedAt                                                time.Time `gorm:"index:build_status_created,priority:2"`
}

func (buildRecord) TableName() string { return "builds" }

type stepRecord struct {
	ID                                            string      `gorm:"primaryKey;size:36"`
	BuildID                                       string      `gorm:"not null;size:36;uniqueIndex:build_phase_index"`
	Build                                         buildRecord `gorm:"foreignKey:BuildID;constraint:OnDelete:RESTRICT"`
	Phase                                         string      `gorm:"not null;uniqueIndex:build_phase_index"`
	Index                                         int         `gorm:"not null;uniqueIndex:build_phase_index"`
	Name, Kind, Condition, Status, ReasonsJSON    string
	ElapsedNS                                     int64
	Intent, Started, StopConfirmed, CleanupFailed bool
	Reason                                        string
	ExitCode                                      int
	ArtifactIDsJSON                               string
}

func (stepRecord) TableName() string { return "step_progress" }

type requestRecord struct {
	ID            string `gorm:"primaryKey;size:36"`
	IdentityID    string `gorm:"not null;uniqueIndex:identity_key"`
	Key           string `gorm:"not null;uniqueIndex:identity_key;size:128"`
	RequestDigest string
	BatchID       string      `gorm:"not null;size:36"`
	Batch         batchRecord `gorm:"foreignKey:BatchID;constraint:OnDelete:RESTRICT"`
	CreatedAt     time.Time
}

func (requestRecord) TableName() string { return "idempotency" }
