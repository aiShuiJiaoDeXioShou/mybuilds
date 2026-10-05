package pipeline

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"mybuilds/internal/protocol"
	"mybuilds/internal/reports"
	"os"
	"path"
	"reflect"
	"slices"
	"strings"
)

// 保存原baseline、已解析快照和指纹；恢复绝不重新初始化baseline或重采旧XML。
func (c *reportCollection) checkpoint() (*protocol.ReportCollectionCheckpoint, error) {
	out := &protocol.ReportCollectionCheckpoint{Patterns: slices.Clone(c.patterns), Required: c.required, Directory: c.directory, Baseline: map[string]protocol.ReportFingerprint{}, Current: []protocol.ReportCollectionEntry{}, Revision: c.revision, LastIndex: c.lastIndex, LastName: c.lastName, Final: c.final, LastCanonical: slices.Clone(c.lastCanonical), Sealed: c.sealed, Digest: c.digest}
	for name, f := range c.baseline {
		saved, err := savedReportFingerprint(f)
		if err != nil {
			return nil, err
		}
		out.Baseline[name] = saved
	}
	names := make([]string, 0, len(c.current))
	for name := range c.current {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		entry := c.current[name]
		saved, err := savedReportFingerprint(entry.fingerprint)
		if err != nil {
			return nil, err
		}
		out.Current = append(out.Current, protocol.ReportCollectionEntry{SnapshotPath: entry.local.SnapshotPath, Path: name, Fingerprint: saved, Local: entry.local, Result: entry.result})
	}
	return out, nil
}
func restoreReportCollection(ctx context.Context, workspace, result *os.Root, patterns, secrets []string, required bool, saved *protocol.ReportCollectionCheckpoint) (*reportCollection, error) {
	if saved == nil || saved.Revision < 1 || !slices.Equal(saved.Patterns, patterns) || saved.Required != required || saved.Baseline == nil || saved.Current == nil || len(saved.Baseline) > 64 || len(saved.Current) > 64 || !strings.HasPrefix(saved.Directory, "reports/") || !reportValidPath(path.Join(saved.Directory, "x.xml")) {
		return nil, errReportSave
	}
	c := &reportCollection{workspace: workspace, resultRoot: result, patterns: slices.Clone(patterns), secrets: slices.Clone(secrets), required: required, directory: saved.Directory, baseline: map[string]reportFingerprint{}, current: map[string]reportEntry{}, revision: saved.Revision, lastIndex: saved.LastIndex, lastName: saved.LastName, final: saved.Final, lastCanonical: slices.Clone(saved.LastCanonical), sealed: saved.Sealed, digest: saved.Digest}
	for name, f := range saved.Baseline {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !reportValidPath(name) || len(f.SHA256) != 64 {
			return nil, errReportSave
		}
		fp := f
		c.baseline[name] = reportFingerprint{hash: f.SHA256, saved: &fp}
	}
	total := int64(0)
	for _, entry := range saved.Current {
		entry.Local.SnapshotPath = entry.SnapshotPath
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !reportValidPath(entry.Path) || c.current[entry.Path].local.SnapshotPath != "" || entry.Local.File.Path != entry.Path || !reportValidPath(entry.Local.SnapshotPath) || !strings.HasPrefix(entry.Local.SnapshotPath, saved.Directory+"/") {
			return nil, errReportSave
		}
		data, fp, err := c.read(ctx, entry.Path)
		if err != nil {
			return nil, err
		}
		expected := entry.Fingerprint
		if !sameReportFingerprint(fp, reportFingerprint{saved: &expected, hash: expected.SHA256}) {
			return nil, errReportSave
		}
		snapshotReader := &reportCollection{workspace: result}
		raw, _, err := snapshotReader.read(ctx, entry.Local.SnapshotPath)
		if err != nil || len(raw) > reportMaxFile {
			return nil, errReportSave
		}
		hash := sha256.Sum256(raw)
		if hex.EncodeToString(hash[:]) != entry.Local.File.SHA256 || int64(len(raw)) != entry.Local.File.Size || !slices.Equal(raw, data) {
			return nil, errReportSave
		}
		total += int64(len(raw))
		if total > reportMaxTotal {
			return nil, errReportSave
		}
		parsed, err := reports.ParseJUnit(ctx, bytes.NewReader(raw), secrets)
		if err != nil {
			return nil, err
		}
		for i := range parsed.Diagnostics {
			parsed.Diagnostics[i].PathKey = entry.Local.File.Key
		}
		if !reflect.DeepEqual(parsed, entry.Result) || parsed.Counts != entry.Local.File.Counts {
			return nil, errReportSave
		}
		c.current[entry.Path] = reportEntry{fingerprint: fp, local: entry.Local, result: entry.Result}
	}
	return c, nil
}
