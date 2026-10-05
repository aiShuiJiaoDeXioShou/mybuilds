package store

import "time"

type EffectiveRetention struct {
	Builds         int64  `json:"builds"`
	Days           int64  `json:"days"`
	BuildsSource   string `json:"builds_source"`
	DaysSource     string `json:"days_source"`
	GlobalVersion  int64  `json:"global_version"`
	ProjectVersion int64  `json:"project_version"`
}

type RetentionEntry struct {
	BuildID            string     `json:"build_id"`
	Project            string     `json:"project"`
	BuildName          string     `json:"build_name"`
	Number             *int64     `json:"number"`
	TerminalAt         *time.Time `json:"terminal_at"`
	HistoryState       string     `json:"history_state"`
	CleanedAt          *time.Time `json:"cleaned_at"`
	Candidate          bool       `json:"candidate"`
	ProtectReasons     []string   `json:"protect_reasons"`
	JobID              string     `json:"job_id,omitempty"`
	CentralState       string     `json:"central_state"`
	NodeState          string     `json:"node_state"`
	Reason             string     `json:"reason"`
	EvaluatedAt        *time.Time `json:"evaluated_at,omitempty"`
	RetiredAt          *time.Time `json:"retired_at,omitempty"`
	CentralCompletedAt *time.Time `json:"central_completed_at,omitempty"`
	NodeCompletedAt    *time.Time `json:"node_completed_at,omitempty"`
}

type RetentionPage struct {
	Items  []RetentionEntry `json:"items"`
	Limit  int              `json:"limit"`
	Offset int              `json:"offset"`
}

// 中央文件消费者专用观察与事项，不进入外部HTTP响应。
type RetentionObjectObservation struct {
	Identity string `json:"-"`
	Size     int64  `json:"-"`
	SHA256   string `json:"-"`
}

type RetentionObjectResult struct {
	State    string `json:"-"`
	Reason   string `json:"-"`
	Identity string `json:"-"`
}

type RetentionObject struct {
	ID, JobID, BuildID, Kind, ObjectID string `json:"-"`
	StorageID, SHA256, Identity        string `json:"-"`
	QuarantineSlot, State, Reason      string `json:"-"`
	Size                               int64  `json:"-"`
}

// 与Server真实fd配对的私有读取登记；不作为新HTTP租约。
type EvidenceRead struct {
	ID, BuildID, Kind, ObjectID, StorageID string `json:"-"`
	SHA256                                 string `json:"-"`
	Size                                   int64  `json:"-"`
}

// 具体持久记录仅由Store事务消费，关系身份不随清理硬删。
type retentionPolicyRecord struct {
	ID             int       `gorm:"primaryKey;autoIncrement:false;check:retention_policy_singleton,id = 1"`
	Builds         int64     `gorm:"not null;check:retention_policy_builds,builds > 0"`
	Days           int64     `gorm:"not null;check:retention_policy_days,days > 0 AND days <= 106751"`
	Version        int64     `gorm:"not null;check:retention_policy_version,version > 0"`
	ChangedAt      time.Time `gorm:"not null"`
	ProjectCursor  string    `gorm:"not null;default:'';size:36"`
	ObjectCursor   string    `gorm:"not null;default:'';size:36"`
	FinalizeCursor string    `gorm:"not null;default:'';size:36"`
}

func (retentionPolicyRecord) TableName() string { return "retention_policies" }

type retentionJobRecord struct {
	ID                 string        `gorm:"primaryKey;size:36"`
	ProjectID          string        `gorm:"not null;index;size:36"`
	Project            projectRecord `gorm:"foreignKey:ProjectID;constraint:OnDelete:RESTRICT"`
	BuildID            string        `gorm:"not null;uniqueIndex;size:36"`
	Build              buildRecord   `gorm:"foreignKey:BuildID;constraint:OnDelete:RESTRICT"`
	State              string        `gorm:"not null"`
	Reason             string        `gorm:"not null;default:''"`
	GlobalVersion      int64
	ProjectVersion     int64
	EvaluatedAt        time.Time
	RetiredAt          *time.Time
	CentralCompletedAt *time.Time
	NodeCompletedAt    *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (retentionJobRecord) TableName() string { return "retention_jobs" }

type retentionObjectRecord struct {
	ID             string             `gorm:"primaryKey;size:36"`
	JobID          string             `gorm:"not null;uniqueIndex:retention_object_identity;size:36"`
	Job            retentionJobRecord `gorm:"foreignKey:JobID;constraint:OnDelete:RESTRICT"`
	Kind           string             `gorm:"not null;uniqueIndex:retention_object_identity"`
	ObjectID       string             `gorm:"not null;uniqueIndex:retention_object_identity;size:36"`
	StorageID      string             `gorm:"not null;default:'';size:36"`
	Size           int64              `gorm:"not null;default:0;check:retention_object_size,size >= 0"`
	SHA256         string             `gorm:"not null;default:'';size:64"`
	Identity       string             `gorm:"not null;default:'';size:160"`
	QuarantineSlot string             `gorm:"not null;default:''"`
	State          string             `gorm:"not null"`
	Reason         string             `gorm:"not null;default:''"`
	ConfirmedAt    *time.Time
	QuarantinedAt  *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (retentionObjectRecord) TableName() string { return "retention_objects" }

type evidenceReadRecord struct {
	ID        string      `gorm:"primaryKey;size:36"`
	BuildID   string      `gorm:"not null;index;size:36"`
	Build     buildRecord `gorm:"foreignKey:BuildID;constraint:OnDelete:RESTRICT"`
	Kind      string      `gorm:"not null"`
	ObjectID  string      `gorm:"not null;index;size:36"`
	StorageID string      `gorm:"not null;size:36"`
	Owner     string      `gorm:"not null;size:36"`
	Identity  string      `gorm:"not null;default:'';size:160"`
	State     string      `gorm:"not null"`
	CreatedAt time.Time
	ClosedAt  *time.Time
}

func (evidenceReadRecord) TableName() string { return "evidence_reads" }

type nodeResourceRecord struct {
	ID              string            `gorm:"primaryKey;size:36"`
	BuildID         string            `gorm:"not null;index;size:36"`
	Build           buildRecord       `gorm:"foreignKey:BuildID;constraint:OnDelete:RESTRICT"`
	AttemptID       string            `gorm:"not null;uniqueIndex;size:36"`
	Attempt         attemptRecord     `gorm:"foreignKey:AttemptID;constraint:OnDelete:RESTRICT"`
	NodeID          string            `gorm:"not null;index;size:36"`
	Node            nodeRecord        `gorm:"foreignKey:NodeID;constraint:OnDelete:RESTRICT"`
	SessionID       string            `gorm:"not null;size:36"`
	Session         nodeSessionRecord `gorm:"foreignKey:SessionID;constraint:OnDelete:RESTRICT"`
	LeaseID         string            `gorm:"not null;size:36"`
	Epoch           int64             `gorm:"not null;check:node_resource_epoch,epoch > 0"`
	OwnershipDigest string            `gorm:"not null;size:64"`
	HasWorkspace    bool              `gorm:"not null;default:false"`
	HasResults      bool              `gorm:"not null;default:false"`
	CompletedAt     *time.Time
	CompletionJSON  string `gorm:"not null;default:''"`
	TerminalDigest  string `gorm:"not null;default:'';size:64"`
	TerminalSeq     int64  `gorm:"not null;default:0;check:node_resource_terminal_seq,terminal_seq >= 0"`
	RegisteredAt    time.Time
}

func (nodeResourceRecord) TableName() string { return "node_resources" }

type nodeDeletionRecord struct {
	ID              string             `gorm:"primaryKey;size:36"`
	JobID           string             `gorm:"not null;index;size:36"`
	Job             retentionJobRecord `gorm:"foreignKey:JobID;constraint:OnDelete:RESTRICT"`
	ResourceID      string             `gorm:"not null;uniqueIndex;size:36"`
	Resource        nodeResourceRecord `gorm:"foreignKey:ResourceID;constraint:OnDelete:RESTRICT"`
	NodeID          string             `gorm:"not null;index;size:36"`
	Node            nodeRecord         `gorm:"foreignKey:NodeID;constraint:OnDelete:RESTRICT"`
	BuildID         string             `gorm:"not null;size:36"`
	Build           buildRecord        `gorm:"foreignKey:BuildID;constraint:OnDelete:RESTRICT"`
	AttemptID       string             `gorm:"not null;size:36"`
	Attempt         attemptRecord      `gorm:"foreignKey:AttemptID;constraint:OnDelete:RESTRICT"`
	OwnershipDigest string             `gorm:"not null;size:64"`
	State           string             `gorm:"not null"`
	Nonce           string             `gorm:"not null;default:'';size:36"`
	ExpiresAt       *time.Time
	Seq             int64  `gorm:"not null;default:0;check:node_deletion_seq,seq >= 0"`
	Digest          string `gorm:"not null;default:'';size:64"`
	WorkspaceState  string `gorm:"not null;default:''"`
	ResultsState    string `gorm:"not null;default:''"`
	Reason          string `gorm:"not null;default:''"`
	CompletedAt     *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (nodeDeletionRecord) TableName() string { return "node_deletions" }

// 保留每个确认的完整wire字段；后续Nonce变化不影响原Seq/Digest回放。
type nodeDeletionReceiptRecord struct {
	ID              string             `gorm:"primaryKey;size:36"`
	DeleteID        string             `gorm:"not null;uniqueIndex:node_deletion_confirmation;size:36"`
	Deletion        nodeDeletionRecord `gorm:"foreignKey:DeleteID;constraint:OnDelete:RESTRICT"`
	Seq             int64              `gorm:"not null;uniqueIndex:node_deletion_confirmation;check:node_deletion_receipt_seq,seq > 0"`
	ResourceID      string             `gorm:"not null;default:'';size:36"`
	OwnershipDigest string             `gorm:"not null;default:'';size:64"`
	Nonce           string             `gorm:"not null;size:36"`
	Digest          string             `gorm:"not null;size:64"`
	WorkspaceState  string             `gorm:"not null"`
	ResultsState    string             `gorm:"not null"`
	Reason          string             `gorm:"not null;default:''"`
	CreatedAt       time.Time
}

func (nodeDeletionReceiptRecord) TableName() string { return "node_deletion_receipts" }
