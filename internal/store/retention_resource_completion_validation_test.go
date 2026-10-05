package store

import (
	"strings"
	"testing"
	"time"
)

// 仅验证私有完成编码；不手写数据库成功状态或冒充实际Agent完成确认。
func TestRetentionResourceCompletionStrictEncoding(t *testing.T) {
	row := buildRecord{LastEventSeq: 2, LastLogSeq: 1, LastLogOffset: 100, LastArtifactSeq: 3}
	now := time.Now().UTC()
	valid := `{"last_event_seq":2,"last_log_seq":1,"last_log_offset":100,"last_artifact_seq":3,"stop_code":"process_group_reaped"}`
	resource := nodeResourceRecord{CompletedAt: &now, CompletionJSON: valid}
	if !retentionResourceCompleted(row, resource, "process_group_reaped") {
		t.Fatal("合法完整编码未通过")
	}
	for _, sample := range []struct{ name, body string }{
		{"empty", ""}, {"null", "null"}, {"array", "[]"}, {"missing", `{"last_event_seq":2,"last_log_seq":1,"last_log_offset":100,"stop_code":"process_group_reaped"}`},
		{"duplicate", strings.Replace(valid, `"last_event_seq":2`, `"last_event_seq":1,"last_event_seq":2`, 1)},
		{"unknown", strings.Replace(valid, `"last_event_seq":2`, `"unknown":0,"last_event_seq":2`, 1)},
		{"field-null", strings.Replace(valid, `"last_log_seq":1`, `"last_log_seq":null`, 1)},
		{"numeric-string", strings.Replace(valid, `"last_log_seq":1`, `"last_log_seq":"1"`, 1)},
		{"fraction", strings.Replace(valid, `"last_log_seq":1`, `"last_log_seq":1.0`, 1)},
		{"exponent", strings.Replace(valid, `"last_log_seq":1`, `"last_log_seq":1e0`, 1)},
		{"overflow", strings.Replace(valid, `"last_log_seq":1`, `"last_log_seq":9223372036854775808`, 1)},
		{"negative", strings.Replace(valid, `"last_log_seq":1`, `"last_log_seq":-1`, 1)},
		{"array-cursor", strings.Replace(valid, `"last_log_seq":1`, `"last_log_seq":[]`, 1)},
		{"event-stale", strings.Replace(valid, `"last_event_seq":2`, `"last_event_seq":1`, 1)},
		{"log-seq-stale", strings.Replace(valid, `"last_log_seq":1`, `"last_log_seq":0`, 1)},
		{"log-offset-stale", strings.Replace(valid, `"last_log_offset":100`, `"last_log_offset":99`, 1)},
		{"artifact-stale", strings.Replace(valid, `"last_artifact_seq":3`, `"last_artifact_seq":2`, 1)},
		{"manual-stop", strings.Replace(valid, "process_group_reaped", "admin_observed_stopped", 1)},
		{"extra-document", valid + "{}"}, {"size", valid + strings.Repeat(" ", 1024)},
	} {
		t.Run(sample.name, func(t *testing.T) {
			resource.CompletionJSON = sample.body
			if retentionResourceCompleted(row, resource, "process_group_reaped") {
				t.Fatal("非法/过期编码解除保护")
			}
		})
	}
	resource.CompletionJSON = valid
	for _, missing := range []*time.Time{nil, new(time.Time)} {
		resource.CompletedAt = missing
		if retentionResourceCompleted(row, resource, "process_group_reaped") {
			t.Fatal("未知完成时间解除保护")
		}
	}
	resource.CompletedAt = &now
	if retentionResourceCompleted(row, resource, "admin_observed_stopped") {
		t.Fatal("manual物理stop冒充Agent资源完成")
	}
}
