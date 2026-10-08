package pipeline

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/google/uuid"
	"mybuilds/internal/config"
	"mybuilds/internal/protocol"
	"mybuilds/internal/reports"
)

var (
	errReportInvalid = errors.New("report_invalid")
	errReportSave    = errors.New("report_error")
	errReportSecret  = errors.New("report_secret")
)

const reportMaxFile = 8 << 20
const reportMaxTotal = 64 << 20

// reportFailureReason只输出固定码；文件系统、解析器错误不进入日志或结果。
func reportFailureReason(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, errReportSecret), errors.Is(err, reports.ErrSecret):
		return "report_secret"
	case errors.Is(err, errReportInvalid), errors.Is(err, reports.ErrInvalid), errors.Is(err, reports.ErrLimit):
		return "report_invalid"
	default:
		return "report_error"
	}
}

type reportFingerprint struct {
	saved *protocol.ReportFingerprint
	info  fs.FileInfo
	hash  string
}
type reportEntry struct {
	fingerprint reportFingerprint
	local       protocol.CollectedReport
	result      protocol.JUnitResult
}

// 两个Root均由Run持有并关闭，collection只拥有本次UUID目录里的文件。
type reportCollection struct {
	workspace, resultRoot *os.Root
	patterns, secrets     []string
	required              bool
	maxFiles              int
	directory             string
	baseline              map[string]reportFingerprint
	current               map[string]reportEntry
	revision              int64
	lastIndex             int
	lastName              string
	final                 bool
	lastCanonical         []byte
	sealed                *protocol.ReportEvidence
	digest                string
}

func reportHasSecret(value string, secrets []string) bool {
	for _, secret := range secrets {
		if secret != "" && strings.Contains(value, secret) {
			return true
		}
	}
	return false
}
func reportValidPath(name string) bool {
	if len(name) > 1024 || !utf8.ValidString(name) || !fs.ValidPath(name) || name == "." || strings.Contains(name, `\`) || strings.ContainsFunc(name, unicode.IsControl) {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if len(part) > 255 || len(part) > 1 && part[1] == ':' {
			return false
		}
	}
	return true
}

func newReportCollection(ctx context.Context, workspaceRoot, resultRoot *os.Root, patterns []string, required bool, secrets []string, maxFiles int) (*reportCollection, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if workspaceRoot == nil || resultRoot == nil || maxFiles < 1 || maxFiles > config.MaximumJUnitMaxFiles || len(patterns) > 32 || validateArtifactPatterns(patterns) != nil {
		return nil, errReportInvalid
	}
	for _, pattern := range patterns {
		if !reportValidPath(pattern) {
			return nil, errReportInvalid
		}
	}
	c := &reportCollection{workspace: workspaceRoot, resultRoot: resultRoot, patterns: slices.Clone(patterns), secrets: slices.Clone(secrets), required: required, maxFiles: maxFiles, directory: path.Join("reports", uuid.NewString()), baseline: map[string]reportFingerprint{}, current: map[string]reportEntry{}}
	names, err := c.names(ctx)
	if err != nil {
		return nil, err
	}
	var total int64
	for _, name := range names {
		data, fp, err := c.read(ctx, name)
		if err != nil {
			return nil, err
		}
		total += int64(len(data))
		if total > reportMaxTotal {
			return nil, errReportInvalid
		}
		c.baseline[name] = fp
	}
	return c, nil
}

// reportFS沿原collector的Root/非阻塞边界，拒绝实际遍历位置中的符号链接。
type reportFS struct{ artifactFS }

func (f reportFS) safe(name string) error {
	if err := f.ctx.Err(); err != nil {
		return err
	}
	if name == "." {
		return nil
	}
	if !reportValidPath(name) {
		return errReportInvalid
	}
	parts := strings.Split(name, "/")
	for i := range parts {
		info, err := f.root.Lstat(strings.Join(parts[:i+1], "/"))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return err
			}
			return errReportSave
		}
		if info.Mode()&os.ModeSymlink != 0 || i < len(parts)-1 && !info.IsDir() {
			return errReportInvalid
		}
	}
	return nil
}
func (f reportFS) Open(name string) (fs.File, error) {
	if err := f.safe(name); err != nil {
		return nil, err
	}
	file, err := f.artifactFS.Open(name)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, errReportSave
	}
	return file, err
}
func (f reportFS) Stat(name string) (fs.FileInfo, error) {
	if err := f.safe(name); err != nil {
		return nil, err
	}
	return f.artifactFS.Stat(name)
}
func (f reportFS) ReadDir(name string) ([]fs.DirEntry, error) {
	file, err := f.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader, ok := file.(fs.ReadDirFile)
	if !ok {
		return nil, errReportInvalid
	}
	info, err := file.Stat()
	if err != nil {
		return nil, errReportSave
	}
	if !info.IsDir() {
		return nil, errReportInvalid
	}
	var entries []fs.DirEntry
	for {
		if err := f.ctx.Err(); err != nil {
			return nil, err
		}
		batch, e := reader.ReadDir(128)
		for _, entry := range batch {
			if entry.Type()&os.ModeSymlink != 0 {
				return nil, errReportInvalid
			}
		}
		entries = append(entries, batch...)
		// 大目录即使尚未匹配也不能形成无界的中间切片。
		if len(entries) > 100000 {
			return nil, errReportInvalid
		}
		if e != nil {
			if !errors.Is(e, io.EOF) {
				return nil, errReportSave
			}
			break
		}
	}
	slices.SortFunc(entries, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	return entries, nil
}
func (c *reportCollection) names(ctx context.Context) ([]string, error) {
	names := map[string]bool{}
	for _, pattern := range c.patterns {
		err := doublestar.GlobWalk(reportFS{artifactFS{c.workspace.FS(), c.workspace, ctx}}, pattern, func(name string, _ fs.DirEntry) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if !reportValidPath(name) {
				return errReportInvalid
			}
			if reportHasSecret(name, c.secrets) {
				return errReportSecret
			}
			info, err := c.workspace.Lstat(name)
			if err != nil {
				return errReportSave
			}
			if info.IsDir() {
				return nil
			}
			if !info.Mode().IsRegular() {
				return errReportInvalid
			}
			names[name] = true
			if len(names) > c.maxFiles {
				return errReportInvalid
			}
			return nil
		}, doublestar.WithNoFollow(), doublestar.WithFilesOnly(), doublestar.WithFailOnIOErrors())
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			if errors.Is(err, errReportSecret) {
				return nil, errReportSecret
			}
			if errors.Is(err, errReportInvalid) {
				return nil, errReportInvalid
			}
			return nil, errReportSave
		}
	}
	list := make([]string, 0, len(names))
	for name := range names {
		list = append(list, name)
	}
	slices.Sort(list)
	return list, nil
}

// read的实际字节和身份一起校验，第二次有界读摘要防止保mtime的复制竞争。
func (c *reportCollection) read(ctx context.Context, name string) ([]byte, reportFingerprint, error) {
	empty := reportFingerprint{}
	if err := ctx.Err(); err != nil {
		return nil, empty, err
	}
	if reportHasSecret(name, c.secrets) {
		return nil, empty, errReportSecret
	}
	fsy := reportFS{artifactFS{c.workspace.FS(), c.workspace, ctx}}
	if err := fsy.safe(name); err != nil {
		if ctx.Err() != nil {
			return nil, empty, ctx.Err()
		}
		if errors.Is(err, errReportSave) {
			return nil, empty, errReportSave
		}
		return nil, empty, errReportInvalid
	}
	before, err := c.workspace.Lstat(name)
	if err != nil {
		return nil, empty, errReportSave
	}
	if !before.Mode().IsRegular() || before.Size() > reportMaxFile {
		return nil, empty, errReportInvalid
	}
	file, err := c.workspace.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, empty, errReportSave
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !sameArtifactFile(before, opened) {
		return nil, empty, errReportInvalid
	}
	data, err := reportReadBytes(ctx, file)
	if err != nil {
		return nil, empty, err
	}
	after, err := file.Stat()
	if err != nil || !sameArtifactFile(opened, after) || int64(len(data)) != opened.Size() {
		return nil, empty, errReportInvalid
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return nil, empty, errReportSave
	}
	verified, err := reportReadBytes(ctx, file)
	if err != nil {
		return nil, empty, err
	}
	hash := sha256.Sum256(data)
	if sha256.Sum256(verified) != hash {
		return nil, empty, errReportInvalid
	}
	after, err = file.Stat()
	if err != nil || !sameArtifactFile(opened, after) {
		return nil, empty, errReportInvalid
	}
	if err = fsy.safe(name); err != nil {
		if ctx.Err() != nil {
			return nil, empty, ctx.Err()
		}
		if errors.Is(err, errReportSave) {
			return nil, empty, errReportSave
		}
		return nil, empty, errReportInvalid
	}
	current, err := c.workspace.Lstat(name)
	if err != nil || !sameArtifactFile(opened, current) {
		return nil, empty, errReportInvalid
	}
	if err = ctx.Err(); err != nil {
		return nil, empty, err
	}
	return data, reportFingerprint{info: opened, hash: hex.EncodeToString(hash[:])}, nil
}
func reportReadBytes(ctx context.Context, file *os.File) ([]byte, error) {
	var out bytes.Buffer
	buffer := make([]byte, 32<<10)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, e := file.Read(buffer)
		if n > 0 {
			if out.Len()+n > reportMaxFile {
				return nil, errReportInvalid
			}
			out.Write(buffer[:n])
		}
		if e != nil {
			if !errors.Is(e, io.EOF) {
				return nil, errReportSave
			}
			break
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
func sameReportFingerprint(a, b reportFingerprint) bool {
	if a.hash != b.hash {
		return false
	}
	if a.saved != nil || b.saved != nil {
		left, err := savedReportFingerprint(a)
		if err != nil {
			return false
		}
		right, err := savedReportFingerprint(b)
		return err == nil && left == right
	}
	return sameArtifactFile(a.info, b.info)
}

func (c *reportCollection) snapshot(ctx context.Context, name string, data []byte, fp reportFingerprint, index int, step string) (reportEntry, error) {
	for _, secret := range c.secrets {
		if err := ctx.Err(); err != nil {
			return reportEntry{}, err
		}
		if secret != "" && bytes.Contains(data, []byte(secret)) {
			return reportEntry{}, errReportSecret
		}
	}
	if err := ctx.Err(); err != nil {
		return reportEntry{}, err
	}
	id := uuid.NewString()
	relative := path.Join(c.directory, id+".xml")
	if err := c.resultRoot.MkdirAll(c.directory, 0700); err != nil {
		return reportEntry{}, errReportSave
	}
	target, err := c.resultRoot.OpenFile(relative, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return reportEntry{}, errReportSave
	}
	success := false
	defer func() {
		target.Close()
		if !success {
			_ = c.resultRoot.Remove(relative)
		}
	}()
	for offset := 0; offset < len(data); {
		if err := ctx.Err(); err != nil {
			return reportEntry{}, err
		}
		end := min(offset+(32<<10), len(data))
		n, e := target.Write(data[offset:end])
		if e != nil || n != end-offset {
			return reportEntry{}, errReportSave
		}
		offset = end
	}
	if err := target.Sync(); err != nil {
		return reportEntry{}, errReportSave
	}
	if err := target.Close(); err != nil {
		return reportEntry{}, errReportSave
	}
	reader, err := c.resultRoot.Open(relative)
	if err != nil {
		return reportEntry{}, errReportSave
	}
	parsed, parseErr := parseReportSnapshot(ctx, reader, c.secrets)
	closeErr := reader.Close()
	if ctx.Err() != nil {
		return reportEntry{}, ctx.Err()
	}
	if closeErr != nil {
		return reportEntry{}, errReportSave
	}
	if parseErr != nil {
		return reportEntry{}, parseErr
	}
	key := sha256.Sum256([]byte(name))
	for i := range parsed.Diagnostics {
		parsed.Diagnostics[i].PathKey = hex.EncodeToString(key[:])
	}
	local := protocol.CollectedReport{SnapshotPath: relative, File: protocol.ReportFile{Key: hex.EncodeToString(key[:]), Path: name, ArtifactID: id, SourceIndex: index, SourceStep: step, Size: int64(len(data)), SHA256: fp.hash, Counts: parsed.Counts}}
	success = true
	return reportEntry{fp, local, parsed}, nil
}

type reportSnapshotReader struct {
	file      *os.File
	readError error
}

func (r *reportSnapshotReader) Read(buffer []byte) (int, error) {
	n, err := r.file.Read(buffer)
	if err != nil && !errors.Is(err, io.EOF) {
		r.readError = err
	}
	return n, err
}
func parseReportSnapshot(ctx context.Context, file *os.File, secrets []string) (protocol.JUnitResult, error) {
	reader := &reportSnapshotReader{file: file}
	result, err := reports.ParseJUnit(ctx, reader, secrets)
	if ctx.Err() != nil {
		return protocol.JUnitResult{}, ctx.Err()
	}
	// Parser的Reader错误统一为Invalid；真实文件consumer仍须区分快照IO并闭锁。
	if reader.readError != nil {
		return protocol.JUnitResult{}, errReportSave
	}
	return result, err
}

func (c *reportCollection) check(ctx context.Context, index int, name string, final bool) (protocol.ReportEvidence, []protocol.CollectedReport, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return protocol.ReportEvidence{}, nil, err
	}
	if c.final || final && (index != 0 || name != "") || !final && (index <= 0 || name == "") {
		return protocol.ReportEvidence{}, nil, errReportInvalid
	}
	names, err := c.names(ctx)
	reason := ""
	if err != nil {
		if ctx.Err() != nil {
			return protocol.ReportEvidence{}, nil, ctx.Err()
		}
		if errors.Is(err, errReportSave) {
			return protocol.ReportEvidence{}, nil, errReportSave
		}
		reason = reportFailureReason(err)
		names = nil
		c.current = map[string]reportEntry{}
	}
	present := map[string]bool{}
	for _, name := range names {
		present[name] = true
	}
	for name := range c.current {
		if !present[name] {
			delete(c.current, name)
		}
	}
	var total int64
	sourceIndex, sourceStep := index, name
	if final {
		sourceIndex, sourceStep = c.lastIndex, c.lastName
	}
	for _, fileName := range names {
		data, fp, e := c.read(ctx, fileName)
		if e != nil {
			delete(c.current, fileName)
			if ctx.Err() != nil {
				return protocol.ReportEvidence{}, nil, ctx.Err()
			}
			if errors.Is(e, errReportSave) {
				return protocol.ReportEvidence{}, nil, e
			}
			if reason == "" {
				reason = reportFailureReason(e)
			}
			continue
		}
		total += int64(len(data))
		if total > reportMaxTotal {
			reason = "report_invalid"
			c.current = map[string]reportEntry{}
			break
		}
		if old, ok := c.current[fileName]; ok && sameReportFingerprint(old.fingerprint, fp) {
			continue
		}
		delete(c.current, fileName)
		if old, ok := c.baseline[fileName]; ok && sameReportFingerprint(old, fp) {
			continue
		}
		if sourceIndex <= 0 || sourceStep == "" {
			continue
		}
		entry, e := c.snapshot(ctx, fileName, data, fp, sourceIndex, sourceStep)
		if e != nil {
			if ctx.Err() != nil {
				return protocol.ReportEvidence{}, nil, ctx.Err()
			}
			if errors.Is(e, errReportSave) {
				return protocol.ReportEvidence{}, nil, e
			}
			if reason == "" {
				reason = reportFailureReason(e)
			}
			continue
		}
		c.current[fileName] = entry
	}
	ordered := make([]string, 0, len(c.current))
	for name := range c.current {
		ordered = append(ordered, name)
	}
	slices.Sort(ordered)
	evidence := protocol.ReportEvidence{Revision: c.revision + 1, Required: c.required, Diagnostics: []protocol.JUnitDiagnostic{}, Files: []protocol.ReportFile{}}
	locals := make([]protocol.CollectedReport, 0, len(ordered))
	inputs := make([]protocol.JUnitResult, 0, len(ordered))
	for _, name := range ordered {
		e := c.current[name]
		evidence.Files = append(evidence.Files, e.local.File)
		locals = append(locals, e.local)
		inputs = append(inputs, e.result)
	}
	merged, e := reports.MergeJUnit(inputs)
	if e != nil {
		reason = "report_invalid"
		evidence.Files = []protocol.ReportFile{}
		locals = []protocol.CollectedReport{}
		c.current = map[string]reportEntry{}
	} else {
		evidence.Counts, evidence.Diagnostics = merged.Counts, merged.Diagnostics
	}
	missing := false
	for _, pattern := range c.patterns {
		matched := false
		for _, name := range ordered {
			ok, _ := doublestar.Match(pattern, name)
			matched = matched || ok
		}
		missing = missing || !matched
	}
	evidence.Outcome = "pending"
	if final {
		evidence.Outcome = "passed"
	}
	if final && missing {
		if c.required {
			evidence.Outcome, evidence.Reason = "failed", "report_missing"
		} else if len(evidence.Files) == 0 {
			evidence.Outcome = "missing"
		}
	}
	if evidence.Counts.Failures > 0 || evidence.Counts.Errors > 0 {
		evidence.Outcome, evidence.Reason = "failed", "report_failed"
	}
	if reason != "" {
		evidence.Outcome, evidence.Reason = "failed", reason
	}
	if err := ctx.Err(); err != nil {
		return protocol.ReportEvidence{}, nil, err
	}
	canonical, err := json.Marshal(evidence)
	if err != nil {
		return protocol.ReportEvidence{}, nil, errReportSave
	}
	c.revision++
	c.final = final
	c.lastCanonical = canonical
	if !final {
		c.lastIndex, c.lastName = index, name
	}
	return evidence, locals, nil
}

// seal只封存当前final快照与canonical结果；远端仍必须先完整确认XML再发sealed事件。
func (c *reportCollection) seal(ctx context.Context, candidate protocol.ReportEvidence) (protocol.ReportEvidence, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return protocol.ReportEvidence{}, "", err
	}
	if !c.final || candidate.Sealed {
		return protocol.ReportEvidence{}, "", errReportInvalid
	}
	given, e := json.Marshal(candidate)
	if e != nil || !bytes.Equal(given, c.lastCanonical) {
		return protocol.ReportEvidence{}, "", errReportInvalid
	}
	if c.sealed != nil {
		sealed := *c.sealed
		sealed.Files = slices.Clone(sealed.Files)
		sealed.Diagnostics = slices.Clone(sealed.Diagnostics)
		return sealed, c.digest, nil
	}
	// 只核对独立快照，不在封存时重新读取普通段或post的源XML。
	for _, file := range candidate.Files {
		entry, ok := c.current[file.Path]
		if !ok || entry.local.File != file {
			return protocol.ReportEvidence{}, "", errReportSave
		}
		if err := c.verifySnapshot(ctx, entry.local); err != nil {
			return protocol.ReportEvidence{}, "", err
		}
	}
	candidate.Sealed = true
	data, err := json.Marshal(candidate)
	if err != nil {
		return protocol.ReportEvidence{}, "", errReportSave
	}
	if err = c.resultRoot.MkdirAll(c.directory, 0700); err != nil {
		return protocol.ReportEvidence{}, "", errReportSave
	}
	target, err := c.resultRoot.OpenFile(path.Join(c.directory, "manifest.json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return protocol.ReportEvidence{}, "", errReportSave
	}
	defer target.Close()
	n, e := target.Write(data)
	if e != nil || n != len(data) {
		return protocol.ReportEvidence{}, "", errReportSave
	}
	if err = target.Sync(); err != nil {
		return protocol.ReportEvidence{}, "", errReportSave
	}
	if err = target.Close(); err != nil {
		return protocol.ReportEvidence{}, "", errReportSave
	}
	directory, err := c.resultRoot.Open(c.directory)
	if err != nil {
		return protocol.ReportEvidence{}, "", errReportSave
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	if syncErr != nil || closeErr != nil {
		return protocol.ReportEvidence{}, "", errReportSave
	}
	if err = ctx.Err(); err != nil {
		return protocol.ReportEvidence{}, "", err
	}
	digest := sha256.Sum256(data)
	saved := candidate
	saved.Files = slices.Clone(saved.Files)
	saved.Diagnostics = slices.Clone(saved.Diagnostics)
	c.sealed = &saved
	c.digest = hex.EncodeToString(digest[:])
	return candidate, c.digest, nil
}

func (c *reportCollection) verifySnapshot(ctx context.Context, local protocol.CollectedReport) error {
	bounded := reportFS{artifactFS{c.resultRoot.FS(), c.resultRoot, ctx}}
	if err := bounded.safe(local.SnapshotPath); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errReportSave
	}
	file, err := c.resultRoot.OpenFile(local.SnapshotPath, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return errReportSave
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != local.File.Size {
		return errReportSave
	}
	data, err := reportReadBytes(ctx, file)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errReportSave
	}
	hash := sha256.Sum256(data)
	after, err := file.Stat()
	if err != nil || !sameArtifactFile(info, after) || int64(len(data)) != local.File.Size || hex.EncodeToString(hash[:]) != local.File.SHA256 {
		return errReportSave
	}
	if err := file.Close(); err != nil {
		return errReportSave
	}
	return nil
}
