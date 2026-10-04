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
	Phase     string   `json:"phase"`
	Index     int      `json:"index"`
	Name      string   `json:"name"`
	Kind      string   `json:"kind"`
	Condition string   `json:"condition"`
	Status    string   `json:"status"`
	Reasons   []string `json:"reasons"`
	ElapsedNS int64    `json:"elapsed_ns"`
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
	ID                string         `json:"id"`
	Project           string         `json:"project"`
	Group             string         `json:"group"`
	BatchID           string         `json:"batch_id"`
	Name              string         `json:"build_name"`
	Number            *int64         `json:"number"`
	Status            string         `json:"status"`
	Reason            string         `json:"reason,omitempty"`
	SHA               string         `json:"sha"`
	Branch            string         `json:"branch"`
	Source            string         `json:"source"`
	File              string         `json:"file"`
	SourceDigest      string         `json:"source_digest"`
	ParameterKeys     []string       `json:"parameter_keys"`
	Condition         string         `json:"condition"`
	Reasons           []string       `json:"reasons"`
	InitialBudgetNS   *int64         `json:"initial_budget_ns"`
	RemainingBudgetNS *int64         `json:"remaining_budget_ns"`
	PostBudgetNS      int64          `json:"post_budget_ns"`
	Steps             []StepProgress `json:"steps"`
	Post              []StepProgress `json:"post"`
	CreatedAt         time.Time      `json:"created_at"`
}
type QueueStatus struct{ Projects, Queued, Skipped int64 }

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
	ID                                        string        `gorm:"primaryKey;size:36"`
	BatchID                                   string        `gorm:"not null;size:36"`
	Batch                                     batchRecord   `gorm:"foreignKey:BatchID;constraint:OnDelete:RESTRICT"`
	ProjectID                                 string        `gorm:"not null;size:36;uniqueIndex:project_number"`
	Project                                   projectRecord `gorm:"foreignKey:ProjectID;constraint:OnDelete:RESTRICT"`
	Number                                    *int64        `gorm:"uniqueIndex:project_number"`
	Position                                  int
	Name, Status, Reason, SnapshotJSON        string
	ParameterKeysJSON, Condition, ReasonsJSON string
	InitialBudgetNS, RemainingBudgetNS        *int64
	PostBudgetNS                              int64
	CreatedAt                                 time.Time
}

func (buildRecord) TableName() string { return "builds" }

type stepRecord struct {
	ID                                         string      `gorm:"primaryKey;size:36"`
	BuildID                                    string      `gorm:"not null;size:36;uniqueIndex:build_phase_index"`
	Build                                      buildRecord `gorm:"foreignKey:BuildID;constraint:OnDelete:RESTRICT"`
	Phase                                      string      `gorm:"not null;uniqueIndex:build_phase_index"`
	Index                                      int         `gorm:"not null;uniqueIndex:build_phase_index"`
	Name, Kind, Condition, Status, ReasonsJSON string
	ElapsedNS                                  int64
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
