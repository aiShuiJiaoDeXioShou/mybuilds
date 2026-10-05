package protocol

// Custom字段只由当前custom实际消费者使用，其他商店拒绝这些variant。
type CustomPublishAuthorization struct {
	CommandDigest       string `json:"command_digest"`
	ResultSchemaVersion int    `json:"result_schema_version"`
}
type CustomPublishEvidence struct {
	RemoteID        string `json:"remote_id"`
	Lifecycle       string `json:"lifecycle"`
	RequestSHA256   string `json:"request_sha256,omitempty"`
	ResponseSHA256  string `json:"response_sha256,omitempty"`
	ActionConfirmed bool   `json:"action_confirmed"`
}
type CustomBindingEvidence struct {
	Source         string `json:"source"`
	EvidenceCode   string `json:"evidence_code"`
	Note           string `json:"note"`
	EvidenceSHA256 string `json:"evidence_sha256"`
}
type CustomQueryContext struct {
	Repository          string            `json:"repository"`
	SHA                 string            `json:"sha"`
	WorkingDir          string            `json:"working_dir"`
	ResultFile          string            `json:"result_file"`
	QueryArgv           []string          `json:"query_argv"`
	Environment         map[string]string `json:"environment"`
	Params              map[string]string `json:"params"`
	Facts               map[string]string `json:"facts"`
	OriginalRef         LeaseRef          `json:"original_ref"`
	AuthorizationDigest string            `json:"authorization_digest"`
	ArtifactID          string            `json:"artifact_id"`
	ArtifactSHA256      string            `json:"artifact_sha256"`
	VersionName         string            `json:"version_name"`
	ReportSealDigest    string            `json:"report_seal_digest"`
	ArtifactSize        int64             `json:"artifact_size"`
	Number              int64             `json:"number"`
	ReportIDs           []string          `json:"report_ids"`
}

// CustomCommandDigest绑定原命令声明和精确产物/版本；不含节点秘密或运行目录。
func CustomCommandDigest(argv, query []string, workingDir, resultFile, app, version, id, sha string, number, size int64) (string, error) {
	return publishDigest(struct {
		Argv, Query                                                      []string
		WorkingDir, ResultFile, App, Version, ArtifactID, ArtifactSHA256 string
		Number, Size                                                     int64
	}{argv, query, workingDir, resultFile, app, version, id, sha, number, size})
}

// 私有查询回执绑定原精确授权；公共View只发布有限远端观察。
type CustomQueryEvidence struct {
	IntentID            string                `json:"intent_id"`
	AuthorizationDigest string                `json:"authorization_digest"`
	ArtifactID          string                `json:"artifact_id"`
	ArtifactSHA256      string                `json:"artifact_sha256"`
	VersionName         string                `json:"version_name"`
	VersionCode         int64                 `json:"version_code"`
	EvidenceCode        string                `json:"evidence_code"`
	Remote              CustomPublishEvidence `json:"remote"`
}
