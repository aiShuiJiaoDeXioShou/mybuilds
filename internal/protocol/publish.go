package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// PublishAuthorization 固定本次请求与原执行证据，不携带节点秘密。
type PublishAuthorization struct {
	IntentID           string                     `json:"intent_id"`
	Ref                LeaseRef                   `json:"ref"`
	Index              int                        `json:"index"`
	StepName           string                     `json:"step_name"`
	ArtifactID         string                     `json:"artifact_id"`
	ArtifactSHA256     string                     `json:"artifact_sha256"`
	ArtifactSize       int64                      `json:"artifact_size"`
	ReportSealDigest   string                     `json:"report_seal_digest,omitempty"`
	ReportIDs          []string                   `json:"report_ids"`
	VersionName        string                     `json:"version_name"`
	VersionCode        int64                      `json:"version_code"`
	Track              string                     `json:"track,omitempty"`
	ExplicitProduction bool                       `json:"explicit_production,omitempty"`
	Apple              *ApplePublishAuthorization `json:"apple,omitempty"`
}
type ApplePublishAuthorization struct {
	Action             string `json:"action"`
	PreviousIntentID   string `json:"previous_intent_id,omitempty"`
	RequestSHA256      string `json:"request_sha256"`
	AppStoreVersionID  string `json:"app_store_version_id,omitempty"`
	BuildID            string `json:"build_id,omitempty"`
	ReviewSubmissionID string `json:"review_submission_id,omitempty"`
	ReviewItemID       string `json:"review_item_id,omitempty"`
	SubmitForReview    bool   `json:"submit_for_review"`
	AutomaticRelease   bool   `json:"automatic_release"`
}
type PublishGrant struct {
	IntentID                string                     `json:"intent_id"`
	AuthorizationDigest     string                     `json:"authorization_digest"`
	Ref                     LeaseRef                   `json:"ref"`
	Action                  string                     `json:"action"`
	AppIdentifier           string                     `json:"app_identifier"`
	Track                   string                     `json:"track,omitempty"`
	ReleaseName             string                     `json:"release_name,omitempty"`
	ReleaseStatus           string                     `json:"release_status,omitempty"`
	VersionName             string                     `json:"version_name"`
	VersionCode             int64                      `json:"version_code"`
	ArtifactID              string                     `json:"artifact_id"`
	ArtifactSHA256          string                     `json:"artifact_sha256"`
	ArtifactSize            int64                      `json:"artifact_size"`
	ReportSealDigest        string                     `json:"report_seal_digest,omitempty"`
	ReportIDs               []string                   `json:"report_ids"`
	ChangesNotSentForReview bool                       `json:"changes_not_sent_for_review,omitempty"`
	Apple                   *ApplePublishAuthorization `json:"apple,omitempty"`
}
type AppleRemoteEvidence struct {
	AppID              string     `json:"app_id,omitempty"`
	BuildID            string     `json:"build_id,omitempty"`
	AppStoreVersionID  string     `json:"app_store_version_id,omitempty"`
	ReviewSubmissionID string     `json:"review_submission_id,omitempty"`
	ReviewItemID       string     `json:"review_item_id,omitempty"`
	ProcessingState    string     `json:"processing_state,omitempty"`
	VersionState       string     `json:"version_state,omitempty"`
	ReviewState        string     `json:"review_state,omitempty"`
	ReleaseType        string     `json:"release_type,omitempty"`
	TransportID        string     `json:"transport_id,omitempty"`
	RequestSHA256      string     `json:"request_sha256,omitempty"`
	ResponseSHA256     string     `json:"response_sha256,omitempty"`
	UploadedAt         *time.Time `json:"uploaded_at,omitempty"`
	ActionConfirmed    bool       `json:"action_confirmed"`
}
type PublishRemoteEvidence struct {
	EditID         string               `json:"edit_id,omitempty"`
	ReleaseName    string               `json:"release_name,omitempty"`
	Track          string               `json:"track,omitempty"`
	BundleSHA256   string               `json:"bundle_sha256,omitempty"`
	Lifecycle      string               `json:"lifecycle,omitempty"`
	VersionCode    int64                `json:"version_code,omitempty"`
	BundleAccepted bool                 `json:"bundle_accepted,omitempty"`
	TrackAccepted  bool                 `json:"track_accepted,omitempty"`
	CommitAccepted bool                 `json:"commit_accepted,omitempty"`
	Apple          *AppleRemoteEvidence `json:"apple,omitempty"`
}
type PublishReceipt struct {
	IntentID            string                `json:"intent_id"`
	AuthorizationDigest string                `json:"authorization_digest"`
	Digest              string                `json:"digest"`
	Ref                 LeaseRef              `json:"ref"`
	Status              string                `json:"status"`
	EvidenceCode        string                `json:"evidence_code"`
	MutationStage       string                `json:"mutation_stage"`
	Started             bool                  `json:"started"`
	StopConfirmed       bool                  `json:"stop_confirmed"`
	CleanupFailed       bool                  `json:"cleanup_failed"`
	Remote              PublishRemoteEvidence `json:"remote"`
}
type PublishExpectation struct {
	IntentID      string `json:"intent_id"`
	ReceiptDigest string `json:"receipt_digest"`
	Status        string `json:"status"`
}
type PublishLookup struct {
	Ref      LeaseRef `json:"ref"`
	Index    int      `json:"index"`
	IntentID string   `json:"intent_id"`
}
type PublishQueryTask struct {
	AppleAuthorization      *ApplePublishAuthorization `json:"apple_authorization,omitempty"`
	VersionName             string                     `json:"version_name"`
	Store                   string                     `json:"store"`
	UploadCertificateSHA256 string                     `json:"upload_certificate_sha256,omitempty"`
	ID                      string                     `json:"id"`
	Nonce                   string                     `json:"nonce"`
	Kind                    string                     `json:"kind"`
	BindingID               string                     `json:"binding_id"`
	IntentID                string                     `json:"intent_id,omitempty"`
	NodeID                  string                     `json:"node_id"`
	SessionID               string                     `json:"session_id"`
	AppIdentifier           string                     `json:"app_identifier"`
	Track                   string                     `json:"track,omitempty"`
	ReleaseName             string                     `json:"release_name,omitempty"`
	CredentialRef           string                     `json:"credential_ref"`
	VersionCode             int64                      `json:"version_code"`
	ExpiresAt               time.Time                  `json:"expires_at"`
	Apple                   *AppleRemoteEvidence       `json:"apple,omitempty"`
}
type PublishMatch struct {
	ReleaseName  string               `json:"release_name,omitempty"`
	Track        string               `json:"track,omitempty"`
	VersionCodes []int64              `json:"version_codes"`
	Lifecycle    string               `json:"lifecycle"`
	Apple        *AppleRemoteEvidence `json:"apple,omitempty"`
}
type PublishQueryResult struct {
	ID                string         `json:"id"`
	Nonce             string         `json:"nonce"`
	Kind              string         `json:"kind"`
	BindingID         string         `json:"binding_id"`
	IntentID          string         `json:"intent_id,omitempty"`
	NodeID            string         `json:"node_id"`
	SessionID         string         `json:"session_id"`
	ObservedAt        time.Time      `json:"observed_at"`
	ObservedLifecycle string         `json:"observed_lifecycle"`
	Matches           []PublishMatch `json:"matches"`
	DoctorChecks      []ToolCheck    `json:"doctor_checks,omitempty"`
	ToolLockDigest    string         `json:"tool_lock_digest"`
	Reason            string         `json:"reason"`
}

func publishDigest(in any) (string, error) {
	data, err := json.Marshal(in)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// PublishAuthorizationDigest 只覆盖实际请求；服务端派生选择另由 Grant 摘要绑定。
func PublishAuthorizationDigest(in PublishAuthorization) (string, error) { return publishDigest(in) }

// PublishGrantDigest 排除自身摘要，包含服务端派生轨道与审核选择。
func PublishGrantDigest(in PublishGrant) (string, error) {
	in.AuthorizationDigest = ""
	return publishDigest(in)
}

// PublishReceiptDigest 排除自身摘要；Started/停止与远端事实都参与摘要。
func PublishReceiptDigest(in PublishReceipt) (string, error) {
	in.Digest = ""
	return publishDigest(in)
}
