package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"unicode"

	"github.com/bmatcuk/doublestar/v4"
)

var errArtifact = errors.New("产物收集失败")
var artifactPublish sync.Mutex

func validateArtifactPatterns(patterns []string) error {
	if len(patterns) == 0 {
		return errors.New("产物模式不能为空")
	}
	for _, pattern := range patterns {
		if strings.TrimSpace(pattern) == "" || !fs.ValidPath(pattern) || strings.Contains(pattern, `\`) || strings.ContainsFunc(pattern, unicode.IsControl) || !doublestar.ValidatePattern(pattern) {
			return errors.New("产物模式无效")
		}
		// 分支内的向上路径也必须在脚本启动前拒绝，不把错误留到文件访问时。
		for _, part := range strings.FieldsFunc(pattern, func(r rune) bool { return strings.ContainsRune("/{},", r) }) {
			if part == "." || part == ".." || (len(part) > 1 && part[1] == ':') {
				return errors.New("产物模式无效")
			}
		}
		if strings.Contains(pattern, "{/") || strings.Contains(pattern, ",/") {
			return errors.New("产物模式无效")
		}
	}
	return nil
}

// artifactFS 让没有任何匹配的目录遍历也检查取消，实际访问仍由 Root.FS 限制。
type artifactFS struct {
	fs.FS
	root *os.Root
	ctx  context.Context
}

func (f artifactFS) Open(name string) (fs.File, error) {
	if err := f.ctx.Err(); err != nil {
		return nil, err
	}
	// 字面目录前缀也可能是 FIFO，不能在检查取消之前阻塞打开。
	return f.root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
func (f artifactFS) Stat(name string) (fs.FileInfo, error) {
	if err := f.ctx.Err(); err != nil {
		return nil, err
	}
	info, err := fs.Stat(f.FS, name)
	if cancel := f.ctx.Err(); cancel != nil {
		return nil, cancel
	}
	return info, err
}
func (f artifactFS) ReadDir(name string) ([]fs.DirEntry, error) {
	file, err := f.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader, ok := file.(fs.ReadDirFile)
	if !ok {
		return nil, errArtifact
	}
	var entries []fs.DirEntry
	for {
		if err := f.ctx.Err(); err != nil {
			return nil, err
		}
		batch, err := reader.ReadDir(128)
		entries = append(entries, batch...)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return nil, err
			}
			break
		}
	}
	slices.SortFunc(entries, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	if err := f.ctx.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func collectArtifacts(ctx context.Context, workspace *os.Root, destination string, patterns []string) ([]ArtifactRecord, error) {
	if err := validateArtifactPatterns(patterns); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if workspace == nil {
		return nil, errArtifact
	}
	if _, err := os.Lstat(destination); !errors.Is(err, fs.ErrNotExist) {
		return nil, errArtifact
	}
	sources := make(map[string]bool)
	for _, pattern := range patterns {
		count := 0
		err := doublestar.GlobWalk(artifactFS{workspace.FS(), workspace, ctx}, pattern, func(name string, _ fs.DirEntry) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if !fs.ValidPath(name) || name == "." {
				return errArtifact
			}
			info, err := workspace.Stat(name)
			if err != nil {
				return errArtifact
			}
			if info.IsDir() {
				return nil
			}
			if !info.Mode().IsRegular() {
				return errArtifact
			}
			count++
			sources[name] = true
			return nil
		}, doublestar.WithNoFollow(), doublestar.WithFilesOnly(), doublestar.WithFailOnIOErrors())
		if cancel := ctx.Err(); cancel != nil {
			return nil, cancel
		}
		if err != nil {
			return nil, errArtifact
		}
		if count == 0 {
			return nil, errors.New("产物模式未匹配普通文件")
		}
	}
	names := make([]string, 0, len(sources))
	for name := range sources {
		names = append(names, name)
	}
	slices.Sort(names)
	stage, err := os.MkdirTemp(filepath.Dir(destination), ".artifact-")
	if err != nil {
		return nil, errArtifact
	}
	defer os.RemoveAll(stage)
	stageRoot, err := os.OpenRoot(stage)
	if err != nil {
		return nil, errArtifact
	}
	defer stageRoot.Close()
	records := make([]ArtifactRecord, 0, len(names))
	for _, name := range names {
		record, err := copyArtifact(ctx, workspace, stageRoot, name)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	manifest, err := json.Marshal(records)
	if err != nil {
		return nil, errArtifact
	}
	if err := stageRoot.WriteFile("manifest.json", manifest, 0600); err != nil {
		return nil, errArtifact
	}
	if err := stageRoot.Close(); err != nil {
		return nil, errArtifact
	}
	// ponytail: 发布使用进程内全局锁；发布争用明显时改为按目标锁。
	artifactPublish.Lock()
	defer artifactPublish.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := os.Lstat(destination); !errors.Is(err, fs.ErrNotExist) {
		return nil, errArtifact
	}
	if err := os.Rename(stage, destination); err != nil {
		return nil, errArtifact
	}
	return records, nil
}

func copyArtifact(ctx context.Context, workspace, stage *os.Root, name string) (ArtifactRecord, error) {
	var empty ArtifactRecord
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	before, err := workspace.Stat(name)
	if err != nil || !before.Mode().IsRegular() {
		return empty, errArtifact
	}
	// 非阻塞打开避免普通文件被替换为 FIFO 时无限等待；打开后必须复查类型和身份。
	source, err := workspace.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return empty, errArtifact
	}
	defer source.Close()
	opened, err := source.Stat()
	if err != nil || !opened.Mode().IsRegular() || !sameArtifactFile(before, opened) {
		return empty, errArtifact
	}
	snapshot := path.Join("files", name)
	if err := stage.MkdirAll(path.Dir(snapshot), 0700); err != nil {
		return empty, errArtifact
	}
	target, err := stage.OpenFile(snapshot, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return empty, errArtifact
	}
	defer target.Close()
	hash := sha256.New()
	writer := io.MultiWriter(target, hash)
	buffer := make([]byte, 64*1024)
	var size int64
	for {
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		n, err := source.Read(buffer)
		if n > 0 {
			written, writeErr := writer.Write(buffer[:n])
			size += int64(written)
			if writeErr != nil || written != n {
				return empty, errArtifact
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return empty, errArtifact
			}
			break
		}
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	after, err := source.Stat()
	if err != nil || !sameArtifactFile(opened, after) || size != opened.Size() {
		return empty, errArtifact
	}
	current, err := workspace.Stat(name)
	if err != nil || !sameArtifactFile(opened, current) {
		return empty, errArtifact
	}
	if err := source.Close(); err != nil {
		return empty, errArtifact
	}
	if err := target.Close(); err != nil {
		return empty, errArtifact
	}
	return ArtifactRecord{SourcePath: name, SnapshotPath: snapshot, Size: size, SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
}

func sameArtifactFile(a, b fs.FileInfo) bool {
	return b.Mode().IsRegular() && os.SameFile(a, b) && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}
