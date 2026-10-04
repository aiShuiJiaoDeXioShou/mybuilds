package server

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"mybuilds/internal/store"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"mybuilds/internal/protocol"
)

func reportDeclarationRequest(t *testing.T, d protocol.ArtifactDeclaration, raw []byte) *http.Request {
	t.Helper()
	if raw == nil {
		var err error
		raw, err = json.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
	}
	r := httptest.NewRequest(http.MethodPut, "/api/agent/artifacts/"+d.ID, bytes.NewReader(nil))
	r.ContentLength = d.Size
	r.Header.Set("X-Mybuilds-Artifact", base64.RawURLEncoding.EncodeToString(raw))
	return r
}

func reportHeaderDeclaration() protocol.ArtifactDeclaration {
	data := []byte(`<testsuite tests="1"><testcase name="actual"/></testsuite>`)
	sum := sha256.Sum256(data)
	return protocol.ArtifactDeclaration{Ref: protocol.LeaseRef{NodeID: uuid.NewString(), SessionID: uuid.NewString(), BuildID: uuid.NewString(), AttemptID: uuid.NewString(), LeaseID: uuid.NewString(), Epoch: 1}, ID: uuid.NewString(), Seq: 1, Phase: "ordinary", Step: "tests", Index: 1, Name: "result.xml", Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:]), Purpose: "junit", ReportRevision: 1, ReportKey: strings.Repeat("a", 64)}
}

func TestReportArtifactDeclarationBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*protocol.ArtifactDeclaration)
		valid  bool
	}{
		{"完整junit声明", func(*protocol.ArtifactDeclaration) {}, true},
		{"旧空用途", func(d *protocol.ArtifactDeclaration) { d.Purpose = ""; d.ReportRevision = 0; d.ReportKey = "" }, true},
		{"显式artifact旧用途", func(d *protocol.ArtifactDeclaration) { d.Purpose = "artifact"; d.ReportRevision = 0; d.ReportKey = "" }, true},
		{"artifact夹带报告字段", func(d *protocol.ArtifactDeclaration) { d.Purpose = "artifact" }, false},
		{"未知用途", func(d *protocol.ArtifactDeclaration) { d.Purpose = "UNKNOWN_PRIVATE" }, false},
		{"无用途但有报告字段", func(d *protocol.ArtifactDeclaration) { d.Purpose = "" }, false},
		{"无revision", func(d *protocol.ArtifactDeclaration) { d.ReportRevision = 0 }, false},
		{"负revision", func(d *protocol.ArtifactDeclaration) { d.ReportRevision = -1 }, false},
		{"无key", func(d *protocol.ArtifactDeclaration) { d.ReportKey = "" }, false},
		{"短key", func(d *protocol.ArtifactDeclaration) { d.ReportKey = strings.Repeat("a", 63) }, false},
		{"非hexkey", func(d *protocol.ArtifactDeclaration) { d.ReportKey = strings.Repeat("g", 64) }, false},
		{"大写key", func(d *protocol.ArtifactDeclaration) { d.ReportKey = strings.Repeat("A", 64) }, false},
		{"post报告", func(d *protocol.ArtifactDeclaration) { d.Phase = "always" }, false},
		{"8MiB边界", func(d *protocol.ArtifactDeclaration) { d.Size = 8 << 20 }, true},
		{"8MiB超限", func(d *protocol.ArtifactDeclaration) { d.Size = (8 << 20) + 1 }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := reportHeaderDeclaration()
			tc.change(&d)
			_, err := artifactDeclaration(reportDeclarationRequest(t, d, nil), d.ID)
			if (err == nil) != tc.valid {
				t.Fatalf("声明有效性=%v，错误=%v", tc.valid, err)
			}
			if err != nil && strings.Contains(err.Error(), "UNKNOWN_PRIVATE") {
				t.Fatal("错误回显声明正文")
			}
		})
	}
}

func TestReportArtifactDeclarationStrictJSON(t *testing.T) {
	d := reportHeaderDeclaration()
	canonical, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		raw  []byte
	}{
		{"伪verified摘要", append(append([]byte{}, canonical[:len(canonical)-1]...), []byte(`,"verified_junit":{"counts":{"tests":1}}}`)...)},
		{"重复用途", append(append([]byte{}, canonical[:len(canonical)-1]...), []byte(`,"purpose":"junit"}`)...)},
		{"null用途", []byte(strings.Replace(string(canonical), `"purpose":"junit"`, `"purpose":null`, 1))},
		{"大小写用途", []byte(strings.Replace(string(canonical), `"purpose"`, `"Purpose"`, 1))},
		{"nullkey", []byte(strings.Replace(string(canonical), `"report_key":"`+d.ReportKey+`"`, `"report_key":null`, 1))},
		{"字符串revision", []byte(strings.Replace(string(canonical), `"report_revision":1`, `"report_revision":"1"`, 1))},
		{"非canonical", append([]byte(" "), canonical...)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := artifactDeclaration(reportDeclarationRequest(t, d, tc.raw), d.ID); err == nil {
				t.Fatal("非法JSON被接受")
			}
		})
	}
	r := reportDeclarationRequest(t, d, nil)
	r.Header.Set("X-Mybuilds-Artifact", strings.Repeat("x", 8193))
	if _, err := artifactDeclaration(r, d.ID); err == nil {
		t.Fatal("8KiB头部上限失效")
	}
}

func junitStageFile(t *testing.T, data []byte) (*os.File, string) {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(t.TempDir(), "stage.xml"), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if _, err = f.Write(data); err != nil {
		t.Fatal(err)
	}
	if err = f.Sync(); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return f, hex.EncodeToString(sum[:])
}

func TestParseJUnitStageActualFDAndCounts(t *testing.T) {
	data := []byte(`<testsuites tests="4" failures="1" errors="1" skipped="1"><testsuite><testcase name="passed" time="0.25"/><testcase name="failed" time="0.5"><failure message="expected mismatch"/></testcase><testcase name="errored"><error>safe diagnostic</error></testcase><testcase name="skipped"><skipped/></testcase></testsuite></testsuites>`)
	f, digest := junitStageFile(t, data)
	before, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	// stage写入后offset在尾部，解析必须只读同一fd的完整原字节。
	result, err := parseJUnitStage(context.Background(), f, int64(len(data)), digest)
	if err != nil || result == nil {
		t.Fatalf("真实stage解析失败：%v", err)
	}
	want := protocol.JUnitCounts{Tests: 4, Failures: 1, Errors: 1, Skipped: 1, DurationNS: 750000000}
	if result.Counts != want || len(result.Diagnostics) != 2 {
		t.Fatalf("解析摘要不符：%+v", result)
	}
	after, err := f.Stat()
	if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("解析修改或关闭stage")
	}
	actual, err := os.ReadFile(f.Name())
	if err != nil || !bytes.Equal(actual, data) {
		t.Fatal("原XML字节被改写")
	}
}

func TestParseJUnitStageRejectsBadContentAndBinding(t *testing.T) {
	for _, tc := range []struct {
		name, xml string
		sizeDelta int64
		digest    string
		want      error
	}{
		{name: "计数不符", xml: `<testsuite tests="9"><testcase/></testsuite>`, want: store.ErrInvalid},
		{name: "实体指令", xml: `<!DOCTYPE testsuite><testsuite/>`, want: store.ErrInvalid},
		{name: "尾部垃圾", xml: `<testsuite/>PRIVATE_XML_CONTENT`, want: store.ErrInvalid},
		{name: "缺root", xml: `PRIVATE_XML_CONTENT`, want: store.ErrInvalid},
		{name: "空文件", xml: ``, want: store.ErrInvalid},
		{name: "声明少字节", xml: `<testsuite/>`, sizeDelta: -1, want: store.ErrInvalid},
		{name: "声明多字节", xml: `<testsuite/>`, sizeDelta: 1, want: store.ErrInvalid},
		{name: "hash不符", xml: `<testsuite/>`, digest: strings.Repeat("0", 64), want: store.ErrInvalid},
		{name: "非法hash", xml: `<testsuite/>`, digest: "PRIVATE_HASH_INPUT", want: store.ErrInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, digest := junitStageFile(t, []byte(tc.xml))
			if tc.digest != "" {
				digest = tc.digest
			}
			result, err := parseJUnitStage(context.Background(), f, int64(len(tc.xml))+tc.sizeDelta, digest)
			if result != nil || !errors.Is(err, tc.want) {
				t.Fatalf("非法stage获得摘要：%+v %v", result, err)
			}
			for _, value := range []string{"PRIVATE_XML_CONTENT", "PRIVATE_HASH_INPUT", f.Name()} {
				if strings.Contains(err.Error(), value) {
					t.Fatal("错误泄露原文或路径")
				}
			}
		})
	}
}

func TestParseJUnitStageSizeAndDeadline(t *testing.T) {
	prefix, suffix := "<testsuite><system-out>", "</system-out></testsuite>"
	data := []byte(prefix + strings.Repeat("a", (8<<20)-len(prefix)-len(suffix)) + suffix)
	f, digest := junitStageFile(t, data)
	result, err := parseJUnitStage(context.Background(), f, int64(len(data)), digest)
	if err != nil || result == nil || result.Counts.Tests != 0 || result.Diagnostics == nil {
		t.Fatalf("8MiB合法原字节未接受：%v", err)
	}
	if result, err = parseJUnitStage(context.Background(), f, (8<<20)+1, digest); result != nil || !errors.Is(err, errTooLarge) {
		t.Fatalf("超限声明未拒绝：%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result, err = parseJUnitStage(ctx, f, int64(len(data)), digest); result != nil || !errors.Is(err, errEvidence) {
		t.Fatalf("取消仍接受结果：%v", err)
	}
	ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if result, err = parseJUnitStage(ctx, f, int64(len(data)), digest); result != nil || !errors.Is(err, errEvidence) {
		t.Fatalf("父deadline被扩大：%v", err)
	}
}

func TestParseJUnitStageRejectsNonRegularOrChangedFD(t *testing.T) {
	f, digest := junitStageFile(t, []byte(`<testsuite/>`))
	if err := f.Chmod(0644); err != nil {
		t.Fatal(err)
	}
	if result, err := parseJUnitStage(context.Background(), f, 12, digest); result != nil || !errors.Is(err, errEvidence) {
		t.Fatalf("非私有stage未拒绝：%v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if result, err := parseJUnitStage(context.Background(), f, 12, digest); result != nil || !errors.Is(err, errEvidence) {
		t.Fatalf("已关闭fd未拒绝：%v", err)
	}
	directory, err := os.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	if result, err := parseJUnitStage(context.Background(), directory, 12, digest); result != nil || !errors.Is(err, errEvidence) {
		t.Fatalf("目录被当报告：%v", err)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	if result, err := parseJUnitStage(context.Background(), reader, 12, digest); result != nil || !errors.Is(err, errEvidence) {
		t.Fatalf("pipe进入解析：%v", err)
	}
}
