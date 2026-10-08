package config

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
)

// ReadPrivateJSON仅供发布决定消费者读取自有有界普通材料；不跟随叶链接。
func ReadPrivateJSON(filename string, limit int64) ([]byte, error) {
	if limit < 1 || limit > MaxConfigBytes {
		return nil, os.ErrPermission
	}
	file, err := openConfiguration(filename)
	if err != nil {
		return nil, os.ErrPermission
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !privateConfiguration(info) || !configurationSingleLink(info) || info.Size() > limit {
		return nil, os.ErrPermission
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, os.ErrPermission
	}
	after, err := file.Stat()
	current, e := os.Lstat(filename)
	if err != nil || e != nil || !os.SameFile(info, current) || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) || !configurationSingleLink(after) {
		return nil, os.ErrPermission
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	count := 0
	if publishJSONValue(decoder, 0, &count) != nil {
		return nil, os.ErrPermission
	}
	if _, err = decoder.Token(); err != io.EOF {
		return nil, os.ErrPermission
	}
	return data, nil
}
func publishJSONValue(d *json.Decoder, depth int, count *int) error {
	*count++
	if depth > 64 || *count > 10000 {
		return os.ErrPermission
	}
	token, err := d.Token()
	if err != nil || token == nil {
		return os.ErrPermission
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			k, e := d.Token()
			name, ok := k.(string)
			if e != nil || !ok || seen[name] || name != strings.ToLower(name) {
				return os.ErrPermission
			}
			seen[name] = true
			if e = publishJSONValue(d, depth+1, count); e != nil {
				return e
			}
		}
		end, e := d.Token()
		if e != nil || end != json.Delim('}') {
			return os.ErrPermission
		}
	case '[':
		for d.More() {
			if e := publishJSONValue(d, depth+1, count); e != nil {
				return e
			}
		}
		end, e := d.Token()
		if e != nil || end != json.Delim(']') {
			return os.ErrPermission
		}
	default:
		return os.ErrPermission
	}
	return nil
}

// ValidateStoreUpload严格约束当前两个商店的明确选择，预览不读取材料。
func ValidateStoreUpload(step Step, field string) error {
	if step.Kind != "upload" || step.Target == "custom" {
		return nil
	}
	if step.AppIdentifier == "" {
		return invalid(field+".app_identifier", "需要明确应用标识")
	}
	if !secretReference.MatchString(step.Credentials) {
		return invalid(field+".credentials", "需要完整节点环境引用")
	}
	if step.Target == "google_play" {
		if step.ReleaseStatus != "" && step.ReleaseStatus != "draft" && step.ReleaseStatus != "completed" {
			return invalid(field+".release_status", "当前只支持draft/completed")
		}
		if step.SubmitForReview != nil || step.AutomaticRelease != nil || step.Channel != "" {
			return invalid(field, "商店选择不适用")
		}
	} else {
		if step.Track != "" || step.ReleaseStatus != "" || step.Channel != "" {
			return invalid(field, "商店选择不适用")
		}
		if step.AutomaticRelease != nil && *step.AutomaticRelease && (step.SubmitForReview == nil || !*step.SubmitForReview) {
			return invalid(field+".automatic_release", "需要明确submit_for_review")
		}
	}
	return nil
}
