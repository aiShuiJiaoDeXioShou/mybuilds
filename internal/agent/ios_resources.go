package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mybuilds/internal/mobile"
	"time"
)

func iosClosedState(state journalState) bool {
	if !state.IOSSigningRequired {
		return state.IOSResources == nil
	}
	return state.IOSResources == nil || state.IOSResources.Closed && !state.IOSResources.Preparing && mobile.ValidIOSResourceOwnership(*state.IOSResources)
}
func (execution *taskExecution) iosClosed() bool {
	execution.journal.mu.Lock()
	defer execution.journal.mu.Unlock()
	return iosClosedState(execution.journal.state)
}

// 具体资源intent必须在原生导入前落到私有journal，关闭证据在失权后仍可保存。
func (execution *taskExecution) saveIOSOwnership(ownership mobile.IOSResourceOwnership) error {
	if !mobile.ValidIOSResourceOwnership(ownership) {
		return failure("persistence_error")
	}
	journal := execution.journal
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if !journal.state.IOSSigningRequired {
		return failure("persistence_error")
	}
	if old := journal.state.IOSResources; old != nil && (old.Token != ownership.Token || old.Output != ownership.Output || old.Temporary != ownership.Temporary || old.OutputIdentity != ownership.OutputIdentity || old.TemporaryIdentity != ownership.TemporaryIdentity || old.Closed && !ownership.Closed) {
		return failure("persistence_error")
	}
	copy := ownership
	journal.state.IOSResources = &copy
	return journal.saveLocked()
}

// 重启只恢复并关闭可证明归属的原生资源，绝不重新准备或按旧PID发信号。
func closeRecoveredIOSResources(ctx context.Context, lock *dataLock) error {
	names, err := lock.journalNames()
	if err != nil {
		return err
	}
	for _, name := range names {
		data, info, e := lock.readJournalFile(name)
		if e != nil {
			return e
		}
		tokens := json.NewDecoder(bytes.NewReader(data))
		count := 0
		if journalJSONValue(tokens, 0, &count) != nil {
			return failure("journal_unconfirmed")
		}
		if _, e = tokens.Token(); e != io.EOF {
			return failure("journal_unconfirmed")
		}
		var state journalState
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		if dec.Decode(&state) != nil {
			return failure("journal_unconfirmed")
		}
		if state.IOSResources == nil {
			continue
		}
		if !state.IOSSigningRequired || state.ClaimKey+".json" != name || state.Ref == nil || !validRef(*state.Ref) || !mobile.ValidIOSResourceOwnership(*state.IOSResources) {
			return failure("journal_unconfirmed")
		}
		// 已关闭无需重查已删除目录，终态的完整wire摘要由原只读恢复入口另行验证。
		if state.IOSResources.Closed {
			continue
		}
		bounded, stop := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		resources, e := mobile.RestoreIOSResources(bounded, *state.IOSResources)
		if e == nil {
			e = resources.Close(bounded)
		}
		stop()
		if e != nil {
			return failure("journal_unconfirmed")
		}
		own := resources.Ownership()
		state.IOSResources = &own
		journal := &executionJournal{lock: lock, name: name, info: info, state: state}
		if e = journal.save(); e != nil {
			return e
		}
	}
	return nil
}

func iosDigestState(state journalState) string {
	if state.IOSResources == nil {
		return ""
	}
	digest, err := mobile.IOSResourceDigest(*state.IOSResources)
	if err != nil {
		return ""
	}
	return digest
}
