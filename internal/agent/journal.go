package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"mybuilds/internal/mobile"
	"mybuilds/internal/protocol"
)

type journalState struct {
	Publishes                             []publishCheckpoint            `json:"publishes,omitempty"`
	IOSSigningRequired                    bool                           `json:"ios_signing_required,omitempty"`
	IOSResources                          *mobile.IOSResourceOwnership   `json:"ios_resources,omitempty"`
	Reports                               *reportCheckpoint              `json:"reports,omitempty"`
	PendingStop                           *protocol.StopConfirmation     `json:"pending_stop,omitempty"`
	Artifacts                             []localArtifact                `json:"artifacts,omitempty"`
	ArtifactSteps                         []protocol.ArtifactExpectation `json:"artifact_steps,omitempty"`
	ClaimKey                              string                         `json:"claim_key"`
	ResourceID                            string                         `json:"resource_id,omitempty"`
	SessionID                             string                         `json:"session_id"`
	Ref                                   *protocol.LeaseRef             `json:"ref,omitempty"`
	LastEventSeq                          int64                          `json:"last_event_seq"`
	LastEventDigest                       string                         `json:"last_event_digest,omitempty"`
	PendingEvent                          *protocol.ExecutionEvent       `json:"pending_event,omitempty"`
	LastLogSeq                            int64                          `json:"last_log_seq"`
	LastLogOffset                         int64                          `json:"last_log_offset"`
	PendingLog                            *protocol.LogChunk             `json:"pending_log,omitempty"`
	Started, StopConfirmed, CleanupFailed bool
	PID, PGID                             int
	RemainingBudgetNS                     *int64 `json:"remaining_budget_ns,omitempty"`
	RemainingPostBudgetNS                 int64  `json:"remaining_post_budget_ns"`
	ResultDir                             string `json:"result_dir,omitempty"`
}
type executionJournal struct {
	mu             sync.Mutex
	lock           *dataLock
	name           string
	state          journalState
	info           os.FileInfo
	recoveryDigest string
}

func newJournal(lock *dataLock, claimKey, sessionID string) (*executionJournal, error) {
	if !exactUUID(claimKey) || !exactUUID(sessionID) {
		return nil, failure("persistence_error")
	}
	journal := &executionJournal{lock: lock, name: claimKey + ".json", state: journalState{ClaimKey: claimKey, SessionID: sessionID}}
	if err := journal.save(); err != nil {
		return nil, err
	}
	return journal, nil
}
func exactUUID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}
func (journal *executionJournal) save() error {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	return journal.saveLocked()
}
func (journal *executionJournal) saveLocked() error {
	data, err := json.Marshal(journal.state)
	if err != nil || len(data) > 1<<20 {
		return failure("persistence_error")
	}
	info, err := journal.lock.atomicFile("journal/"+journal.name, data, journal.info)
	if err != nil {
		return err
	}
	journal.info = info
	return nil
}
func (journal *executionJournal) setLease(ref protocol.LeaseRef, remaining *int64, post int64) error {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if ref.SessionID != journal.state.SessionID || !validRef(ref) || post < 0 || remaining != nil && *remaining < 0 {
		return failure("invalid_response")
	}
	copyRef := ref
	journal.state.Ref = &copyRef
	journal.state.RemainingBudgetNS = cloneBudget(remaining)
	journal.state.RemainingPostBudgetNS = post
	return journal.saveLocked()
}
func cloneBudget(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
func validRef(ref protocol.LeaseRef) bool {
	return exactUUID(ref.NodeID) && exactUUID(ref.SessionID) && exactUUID(ref.BuildID) && exactUUID(ref.AttemptID) && exactUUID(ref.LeaseID) && ref.Epoch > 0
}
func (journal *executionJournal) prepareEvent(p protocol.ExecutionProgress) (protocol.ExecutionEvent, error) {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if journal.state.Ref == nil || journal.state.PendingEvent != nil || journal.state.LastEventSeq == int64(^uint64(0)>>1) {
		return protocol.ExecutionEvent{}, failure("persistence_error")
	}
	if p.ArtifactSteps == nil {
		p.ArtifactSteps = []protocol.ArtifactExpectation{}
	}
	if p.At.IsZero() {
		p.At = time.Now().UTC()
	}
	journal.state.RemainingBudgetNS = cloneBudget(p.RemainingBudgetNS)
	journal.state.RemainingPostBudgetNS = p.RemainingPostBudgetNS
	if p.LocalResultDir != "" {
		journal.state.ResultDir = p.LocalResultDir
	}
	switch p.Kind {
	case "intent":
		journal.state.StopConfirmed = false
	case "started":
		journal.state.Started = true
		journal.state.StopConfirmed = false
		if p.PID > 0 {
			journal.state.PID, journal.state.PGID = p.PID, p.PGID
		}
	case "finished", "build_finished":
		journal.state.StopConfirmed = p.StopConfirmed
		journal.state.CleanupFailed = journal.state.CleanupFailed || p.CleanupFailed
	}
	// PID/快照路径只存在私有journal；消息摘要取真正网络编码。
	p.PID, p.PGID, p.LocalResultDir, p.LocalArtifacts, p.LocalReports = 0, 0, "", nil, nil
	data, err := json.Marshal(p)
	if err != nil {
		return protocol.ExecutionEvent{}, failure("persistence_error")
	}
	hash := sha256.Sum256(data)
	event := protocol.ExecutionEvent{Ref: *journal.state.Ref, Seq: journal.state.LastEventSeq + 1, Digest: hex.EncodeToString(hash[:]), Progress: p}
	journal.state.PendingEvent = &event
	if err = journal.saveLocked(); err != nil {
		return protocol.ExecutionEvent{}, err
	}
	return event, nil
}
func (journal *executionJournal) ackEvent(ack protocol.EventAck) error {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	pending := journal.state.PendingEvent
	if pending == nil || ack.Seq != pending.Seq || ack.Digest != pending.Digest {
		return failure("invalid_response")
	}
	journal.state.LastEventSeq, journal.state.LastEventDigest = ack.Seq, ack.Digest
	journal.state.PendingEvent = nil
	return journal.saveLocked()
}
func (journal *executionJournal) remove() error {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	return journal.lock.removeFile("journal/"+journal.name, journal.info)
}
