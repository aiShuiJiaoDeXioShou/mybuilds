package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/google/uuid"
	"io"
	"mybuilds/internal/protocol"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type localArtifact struct {
	Declaration  protocol.ArtifactDeclaration `json:"declaration"`
	SnapshotPath string                       `json:"snapshot_path"`
	Confirmed    bool                         `json:"confirmed"`
}

func (execution *taskExecution) declare(p *protocol.ExecutionProgress) error {
	journal := execution.journal
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if p.Kind != "finished" || p.StepKind != "artifact" || !p.Started {
		return nil
	}
	expected := protocol.ArtifactExpectation{Phase: p.Phase, Index: p.Index, IDs: []string{}}
	var total int64
	for _, artifact := range journal.state.Artifacts {
		total += artifact.Declaration.Size
	}
	for _, artifact := range p.LocalArtifacts {
		if len(journal.state.Artifacts) >= 128 || artifact.Size < 0 || artifact.Size > 1<<30 || total+artifact.Size > 4<<30 {
			return failure("artifact_limit")
		}
		id := uuid.NewString()
		seq := int64(len(journal.state.Artifacts) + 1)
		declaration := protocol.ArtifactDeclaration{SourcePath: artifact.SourcePath, Ref: *journal.state.Ref, ID: id, Seq: seq, Phase: p.Phase, Index: p.Index, Step: p.Name, Name: artifact.Name, Size: artifact.Size, SHA256: artifact.SHA256}
		journal.state.Artifacts = append(journal.state.Artifacts, localArtifact{Declaration: declaration, SnapshotPath: artifact.SnapshotPath})
		expected.IDs = append(expected.IDs, id)
		total += artifact.Size
	}
	expected.Count = len(expected.IDs)
	p.ArtifactIDs = expected.IDs
	journal.state.ArtifactSteps = append(journal.state.ArtifactSteps, expected)
	if p.LocalResultDir != "" {
		journal.state.ResultDir = p.LocalResultDir
	}
	return journal.saveLocked()
}
func (execution *taskExecution) uploadArtifacts(ctx context.Context) error {
	journal := execution.journal
	for index := 0; ; index++ {
		journal.mu.Lock()
		if index >= len(journal.state.Artifacts) {
			journal.mu.Unlock()
			return nil
		}
		artifact := journal.state.Artifacts[index]
		resultDir := journal.state.ResultDir
		journal.mu.Unlock()
		if artifact.Confirmed {
			continue
		}
		if err := execution.lease.check(); err != nil {
			return err
		}
		deadline, cancel := context.WithTimeout(ctx, 2*time.Minute)
		err := execution.client.putArtifact(deadline, resultDir, artifact)
		cancel()
		if err != nil {
			return err
		}
		journal.mu.Lock()
		journal.state.Artifacts[index].Confirmed = true
		err = journal.saveLocked()
		journal.mu.Unlock()
		if err != nil {
			return err
		}
	}
}
func (client *agentHTTP) putArtifact(ctx context.Context, resultDir string, artifact localArtifact) error {
	declaration := artifact.Declaration
	encoded, err := json.Marshal(declaration)
	if err != nil {
		return failure("artifact_invalid")
	}
	for {
		file, err := openSnapshot(ctx, resultDir, artifact)
		if err != nil {
			return err
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodPut, client.endpoint+"/api/agent/artifacts/"+declaration.ID, file)
		if err != nil {
			file.Close()
			return failure("artifact_invalid")
		}
		request.ContentLength = declaration.Size
		request.Header.Set("Authorization", "Bearer "+client.token)
		request.Header.Set("Content-Type", "application/octet-stream")
		request.Header.Set("X-Mybuilds-Artifact", base64.RawURLEncoding.EncodeToString(encoded))
		response, sendErr := client.client.Do(request)
		file.Close()
		if sendErr != nil {
			safe := client.requestError(ctx, sendErr)
			if !temporaryNetwork(safe) {
				return safe
			}
		}
		if sendErr == nil {
			view, readErr := readArtifactResponse(response)
			if readErr == nil && artifactMatches(view, declaration) {
				return nil
			}
			if readErr == nil {
				return failure("invalid_response")
			}
			if temporaryNetwork(readErr) {
				client.paused.Store(true)
			}
			if !temporaryNetwork(readErr) {
				return readErr
			}
		}
		if ctx.Err() != nil {
			return failure("network_error")
		}
		// 丢失PUT回执时先按不可变ID查询中央元数据，不能重复分配新ID。
		view, getErr := client.getArtifact(ctx, declaration)
		if getErr == nil {
			if artifactMatches(view, declaration) {
				return nil
			}
			return failure("invalid_response")
		}
		if getErr.Error() != "agent_not_found" && getErr.Error() != "agent_network_error" {
			return getErr
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return failure("network_error")
		case <-timer.C:
		}
	}
}
func artifactMatches(view protocol.ArtifactView, d protocol.ArtifactDeclaration) bool {
	return view.ID == d.ID && view.BuildID == d.Ref.BuildID && view.AttemptID == d.Ref.AttemptID && view.Phase == d.Phase && view.Index == d.Index && view.Step == d.Step && view.Name == d.Name && view.Size == d.Size && view.SHA256 == d.SHA256 && view.Purpose == d.Purpose && view.ReportRevision == d.ReportRevision && view.ReportKey == d.ReportKey && !view.CompletedAt.IsZero()
}
func readArtifactResponse(response *http.Response) (protocol.ArtifactView, error) {
	defer response.Body.Close()
	var view protocol.ArtifactView
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil {
		return view, failure("network_error")
	}
	if len(data) > 1<<20 {
		return view, failure("invalid_response")
	}
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		return view, failure("redirect_denied")
	}
	if response.StatusCode != 200 {
		var safe struct {
			Error struct{ Code string } `json:"error"`
		}
		if json.Unmarshal(data, &safe) == nil && safeNodeError(safe.Error.Code) {
			return view, failure(safe.Error.Code)
		}
		return view, failure("request_failed")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&view) != nil {
		return view, failure("invalid_response")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return view, failure("invalid_response")
	}
	return view, nil
}
func (client *agentHTTP) getArtifact(ctx context.Context, declaration protocol.ArtifactDeclaration) (protocol.ArtifactView, error) {
	ref := declaration.Ref
	query := url.Values{"node_id": {ref.NodeID}, "session_id": {ref.SessionID}, "build_id": {ref.BuildID}, "attempt_id": {ref.AttemptID}, "lease_id": {ref.LeaseID}, "epoch": {strconv.FormatInt(ref.Epoch, 10)}}
	bounded, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(bounded, http.MethodGet, client.endpoint+"/api/agent/artifacts/"+declaration.ID+"?"+query.Encode(), nil)
	if err != nil {
		return protocol.ArtifactView{}, failure("invalid_request")
	}
	request.Header.Set("Authorization", "Bearer "+client.token)
	response, err := client.client.Do(request)
	if err != nil {
		return protocol.ArtifactView{}, client.requestError(ctx, err)
	}
	view, readErr := readArtifactResponse(response)
	if temporaryNetwork(readErr) {
		client.paused.Store(true)
	}
	return view, readErr
}
