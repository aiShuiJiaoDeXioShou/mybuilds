package protocol

// 以下仅写节点私有journal；ExecutionProgress.LocalApproval不进入网络JSON。
type ApprovalLocalArtifact struct {
	SourcePath, SnapshotPath, SHA256 string
	Size                             int64
}
type ApprovalLocalStep struct {
	Name, Kind, Status, Reason, LogPath string
	ExitCode                            int
	DurationMS                          int64
	Started, CleanupFailed              bool
	Artifacts                           []ApprovalLocalArtifact
}
type ReportFingerprint struct {
	Device, Inode    uint64
	Mode             uint32
	Size, ModifiedNS int64
	SHA256           string
}
type ReportCollectionEntry struct {
	SnapshotPath string
	Path         string
	Fingerprint  ReportFingerprint
	Local        CollectedReport
	Result       JUnitResult
}
type ReportCollectionCheckpoint struct {
	Patterns      []string
	Required      bool
	Directory     string
	Baseline      map[string]ReportFingerprint
	Current       []ReportCollectionEntry
	Revision      int64
	LastIndex     int
	LastName      string
	Final         bool
	LastCanonical []byte
	Sealed        *ReportEvidence `json:"sealed,omitempty"`
	Digest        string
}
type ApprovalLocalCheckpoint struct {
	Workspace, ResultDir string
	WorkspaceIdentity    string
	Steps                []ApprovalLocalStep
	Collection           *ReportCollectionCheckpoint `json:"collection,omitempty"`
	IOSteamID            string
}
