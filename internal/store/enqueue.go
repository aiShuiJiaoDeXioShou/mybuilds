package store

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"mybuilds/internal/config"
)

func validHex(value string, lengths ...int) bool {
	if !slices.Contains(lengths, len(value)) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
func validKey(key string) bool {
	if len(key) == 0 || len(key) > 128 {
		return false
	}
	for _, r := range key {
		if unicode.IsSpace(r) || unicode.IsControl(r) || r == '/' || r == '\\' {
			return false
		}
	}
	return true
}
func safeReasons(reasons []string) bool {
	allowed := []string{"条件已满足", "参数条件不满足", "分支条件不满足", "分支事实待确定", "模板上下文待确定", "收尾阶段等待构建结果", "changes: 手动预览忽略变更筛选", "通知已关闭"}
	if len(reasons) > 128 {
		return false
	}
	for _, reason := range reasons {
		if !slices.Contains(allowed, reason) {
			return false
		}
	}
	return true
}
func sourceFileValid(file string) bool {
	return config.ValidateProjectSettings(config.ProjectSettings{Pipeline: &config.PipelineSettings{File: file}}) == nil && file != ""
}

func findRequest(db *gorm.DB, actor Actor, key, digest string) (*BatchResult, error) {
	var row requestRecord
	err := db.First(&row, "identity_id = ? AND key = ?", actor.ID, key).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if row.RequestDigest != digest {
		return nil, ErrConflict
	}
	result, err := batchView(db, row.BatchID)
	if err != nil {
		return nil, err
	}
	result.Replayed = true
	return &result, nil
}
func (s *Store) FindRequest(ctx context.Context, actor Actor, key, digest string) (*BatchResult, error) {
	if !validKey(key) || !validHex(digest, 64) {
		return nil, ErrInvalid
	}
	if err := s.CheckLock(ctx); err != nil {
		return nil, err
	}
	db := s.db.WithContext(ctx)
	if err := authorize(db, actor, "admin", "trigger"); err != nil {
		return nil, safeError(err)
	}
	result, err := findRequest(db, actor, key, digest)
	return result, safeError(err)
}

func validatePrepared(prepared PreparedBuild) (PreparedBuild, bool, error) {
	if !validName(prepared.Name) || !slices.Contains([]string{"queued", "skipped"}, prepared.Status) || !slices.Contains([]string{"ready", "skipped"}, prepared.Snapshot.Condition) || !safeReasons(prepared.Snapshot.Reasons) {
		return prepared, false, ErrInvalid
	}
	if (prepared.Status == "skipped") != (prepared.Snapshot.Condition == "skipped") {
		return prepared, false, ErrInvalid
	}
	if prepared.Reason != "" && prepared.Reason != "condition_skipped" {
		return prepared, false, ErrInvalid
	}
	if prepared.Status == "queued" && prepared.Reason != "" {
		return prepared, false, ErrInvalid
	}
	// 从具体快照深拷贝，规范化不改变调用方 map/定义。
	data, err := json.Marshal(prepared.Snapshot)
	if err != nil || len(data) > config.MaxConfigBytes {
		return prepared, false, ErrInvalid
	}
	if err = json.Unmarshal(data, &prepared.Snapshot); err != nil {
		return prepared, false, ErrInvalid
	}
	definition := &prepared.Snapshot.Definition
	if err = config.Validate(&config.Document{Version: 1, Builds: map[string]*config.Build{prepared.Name: definition}}); err != nil {
		return prepared, false, ErrInvalid
	}
	if definition.Notifications != nil {
		if definition.Notifications.Enabled == nil || *definition.Notifications.Enabled {
			return prepared, false, ErrInvalid
		}
		definition.Notifications = nil
	}
	if len(prepared.Snapshot.Params) > 128 {
		return prepared, false, ErrInvalid
	}
	for _, value := range prepared.Snapshot.Params {
		if len(value) > 4096 {
			return prepared, false, ErrInvalid
		}
	}
	prepared.Snapshot.Params, err = config.ResolveParams(definition, prepared.Snapshot.Params)
	if err != nil {
		return prepared, false, ErrInvalid
	}
	if prepared.InitialBudgetNS != nil && *prepared.InitialBudgetNS <= 0 {
		return prepared, false, ErrInvalid
	}
	if definition.Timeout != "" {
		duration, _ := time.ParseDuration(definition.Timeout)
		if prepared.InitialBudgetNS == nil || *prepared.InitialBudgetNS != int64(duration) {
			return prepared, false, ErrInvalid
		}
	} else if prepared.InitialBudgetNS != nil {
		return prepared, false, ErrInvalid
	}
	expectedPost := int64(2 * time.Minute)
	if definition.Post != nil && definition.Post.Timeout != "" {
		duration, _ := time.ParseDuration(definition.Post.Timeout)
		expectedPost = int64(duration)
	}
	if prepared.PostBudgetNS == 0 {
		prepared.PostBudgetNS = expectedPost
	}
	if prepared.PostBudgetNS != expectedPost {
		return prepared, false, ErrInvalid
	}
	phases := map[string][]config.Step{"ordinary": definition.Steps}
	if definition.Post != nil {
		phases["success"] = definition.Post.Success
		phases["failure"] = definition.Post.Failure
		phases["always"] = definition.Post.Always
	}
	expectedCount := 0
	hasUpload := false
	for _, steps := range phases {
		expectedCount += len(steps)
		for _, step := range steps {
			if step.Kind == "upload" {
				hasUpload = true
				if step.Credentials != "" && !secretReference(step.Credentials) {
					return prepared, false, ErrInvalid
				}
			}
		}
	}
	if len(prepared.Steps) != expectedCount {
		return prepared, false, ErrInvalid
	}
	seen := map[string]bool{}
	for _, progress := range prepared.Steps {
		definitions, exists := phases[progress.Phase]
		if !exists || progress.Index < 1 || progress.Index > len(definitions) {
			return prepared, false, ErrInvalid
		}
		key := fmt.Sprintf("%s/%d", progress.Phase, progress.Index)
		if seen[key] {
			return prepared, false, ErrInvalid
		}
		seen[key] = true
		step := definitions[progress.Index-1]
		name := step.Name
		if name == "" {
			name = fmt.Sprintf("%s-%d", step.Kind, progress.Index)
		}
		if progress.Name != name || progress.Kind != step.Kind || !slices.Contains([]string{"ready", "pending", "skipped"}, progress.Condition) || !safeReasons(progress.Reasons) || progress.ElapsedNS != 0 || !slices.Contains([]string{"pending", "skipped"}, progress.Status) {
			return prepared, false, ErrInvalid
		}
		if progress.Phase != "ordinary" && progress.Status != "pending" {
			return prepared, false, ErrInvalid
		}
		if progress.Status == "skipped" && progress.Condition != "skipped" {
			return prepared, false, ErrInvalid
		}
	}
	return prepared, hasUpload, nil
}
func secretReference(value string) bool {
	if !strings.HasPrefix(value, "${") || !strings.HasSuffix(value, "}") {
		return false
	}
	name := value[2 : len(value)-1]
	if name == "" {
		return false
	}
	for i, c := range name {
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '_' || (i > 0 && c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

func (s *Store) Enqueue(ctx context.Context, input EnqueueInput) (BatchResult, error) {
	if !validKey(input.Key) || !validHex(input.RequestDigest, 64) || !validID(input.ProjectID) {
		return BatchResult{}, ErrInvalid
	}
	var result BatchResult
	err := s.write(ctx, func(tx *gorm.DB) error {
		if err := authorize(tx, input.Actor, "admin", "trigger"); err != nil {
			return err
		}
		original, err := findRequest(tx, input.Actor, input.Key, input.RequestDigest)
		if err != nil {
			return err
		}
		if original != nil {
			result = *original
			return nil
		}
		var project projectRecord
		if err := tx.First(&project, "id = ?", input.ProjectID).Error; err != nil {
			return err
		}
		if project.PolicyVersion != input.ProjectVersion {
			return ErrConflict
		}
		if !validHex(input.SHA, 40, 64) || !validHex(input.SourceDigest, 64) || !validBranchPattern(input.Branch) || strings.ContainsAny(input.Branch, "*?[") || !slices.Contains([]string{"auto", "repo"}, input.Source) || !sourceFileValid(input.File) || len(input.Builds) == 0 || len(input.Builds) > 64 {
			return ErrInvalid
		}
		prepared := make([]PreparedBuild, len(input.Builds))
		names := map[string]bool{}
		queued := int64(0)
		hasUpload := input.HasUpload
		for i, b := range input.Builds {
			if names[b.Name] {
				return ErrInvalid
			}
			names[b.Name] = true
			validated, upload, err := validatePrepared(b)
			if err != nil {
				return err
			}
			prepared[i] = validated
			hasUpload = hasUpload || upload
			if b.Status == "queued" {
				queued++
			}
		}
		if hasUpload && (input.Actor.Role != "admin" || !input.AllowUpload) {
			return ErrForbidden
		}
		if queued > math.MaxInt64-project.NextNumber {
			return ErrConflict
		}
		batch := batchRecord{ID: uuid.NewString(), ProjectID: project.ID, IdentityID: input.Actor.ID, SHA: input.SHA, Branch: input.Branch, Source: input.Source, File: input.File, SourceDigest: input.SourceDigest, CreatedAt: time.Now().UTC()}
		if err := tx.Create(&batch).Error; err != nil {
			return err
		}
		var nodes []string
		if err := json.Unmarshal([]byte(project.NodesJSON), &nodes); err != nil {
			return errDatabase
		}
		number := project.NextNumber
		for position, b := range prepared {
			id := uuid.NewString()
			// 全部事实由真实事务来源构造，未就绪的节点/工作目录不预造。
			b.Snapshot.Facts = map[string]string{"project": project.Name, "build.name": b.Name, "build.id": id, "git.sha": input.SHA, "git.branch": input.Branch}
			b.Snapshot.AllowedNodes = slices.Clone(nodes)
			b.Snapshot.DefaultNode = project.DefaultNode
			var allocated *int64
			if b.Status == "queued" {
				value := number
				allocated = &value
				number++
				b.Snapshot.Facts["build.number"] = strconv.FormatInt(value, 10)
			}
			snapshot, err := encode(b.Snapshot)
			if err != nil {
				return err
			}
			keys := make([]string, 0, len(b.Snapshot.Params))
			for key := range b.Snapshot.Params {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			encodedKeys, _ := encode(keys)
			reasons, _ := encode(b.Snapshot.Reasons)
			row := buildRecord{ID: id, BatchID: batch.ID, ProjectID: project.ID, Number: allocated, Position: position, Name: b.Name, Status: b.Status, Reason: b.Reason, SnapshotJSON: snapshot, ParameterKeysJSON: encodedKeys, Condition: b.Snapshot.Condition, ReasonsJSON: reasons, InitialBudgetNS: b.InitialBudgetNS, RemainingBudgetNS: b.InitialBudgetNS, PostBudgetNS: b.PostBudgetNS, CreatedAt: batch.CreatedAt}
			if b.Status == "skipped" {
				row.Reason = "condition_skipped"
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
			for _, progress := range b.Steps {
				reasons, _ := encode(progress.Reasons)
				step := stepRecord{ID: uuid.NewString(), BuildID: id, Phase: progress.Phase, Index: progress.Index, Name: progress.Name, Kind: progress.Kind, Condition: progress.Condition, Status: progress.Status, ReasonsJSON: reasons, ElapsedNS: 0}
				if err := tx.Create(&step).Error; err != nil {
					return err
				}
			}
		}
		if queued != 0 {
			update := tx.Model(&projectRecord{}).Where("id = ? AND policy_version = ? AND next_number = ?", project.ID, input.ProjectVersion, project.NextNumber).Update("next_number", number)
			if update.Error != nil {
				return update.Error
			}
			if update.RowsAffected != 1 {
				return ErrConflict
			}
		}
		request := requestRecord{ID: uuid.NewString(), IdentityID: input.Actor.ID, Key: input.Key, RequestDigest: input.RequestDigest, BatchID: batch.ID, CreatedAt: batch.CreatedAt}
		if err := tx.Create(&request).Error; err != nil {
			return err
		}
		result, err = batchView(tx, batch.ID)
		return err
	})
	if err != nil {
		return BatchResult{}, err
	}
	return result, nil
}
