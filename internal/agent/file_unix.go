//go:build darwin || linux

package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/google/uuid"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// 文件只能发布在已知私有目录，重命名与目录fsync都完成后才允许发送。
func (lock *dataLock) atomicFile(name string, data []byte, previous os.FileInfo) (os.FileInfo, error) {
	if len(data) == 0 || len(data) > 1<<20 || !ownedFileName(name) {
		return nil, failure("persistence_error")
	}
	if err := lock.Check(); err != nil {
		return nil, err
	}
	parent := path.Dir(name)
	if err := lock.prepareDirectory(parent); err != nil {
		return nil, err
	}
	existing, err := lock.root.Lstat(name)
	if previous == nil {
		if !os.IsNotExist(err) {
			return nil, failure("persistence_error")
		}
	} else if err != nil || !privateInfo(existing, false) || !os.SameFile(existing, previous) {
		return nil, failure("persistence_error")
	}
	temporary := path.Join(parent, ".tmp-"+uuid.NewString())
	file, err := lock.root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0600)
	if err != nil {
		return nil, failure("persistence_error")
	}
	defer lock.root.Remove(temporary)
	written, writeErr := file.Write(data)
	syncErr := file.Sync()
	info, statErr := file.Stat()
	closeErr := file.Close()
	if writeErr != nil || written != len(data) || syncErr != nil || statErr != nil || closeErr != nil || !privateInfo(info, false) {
		return nil, failure("persistence_error")
	}
	if err = lock.Check(); err != nil {
		return nil, err
	}
	if err = lock.root.Rename(temporary, name); err != nil {
		return nil, failure("persistence_error")
	}
	if err = lock.syncDirectory(parent); err != nil {
		return nil, err
	}
	actual, err := lock.root.Lstat(name)
	if err != nil || !privateInfo(actual, false) || !os.SameFile(actual, info) {
		return nil, failure("persistence_error")
	}
	return actual, nil
}
func ownedFileName(name string) bool {
	if strings.Contains(name, "\\") || path.Clean(name) != name || path.Base(name) == "." {
		return false
	}
	if path.Dir(name) == "resources" || path.Dir(name) == "deletions" {
		base := path.Base(name)
		return strings.HasSuffix(base, ".json") && exactUUID(strings.TrimSuffix(base, ".json"))
	}
	return path.Dir(name) == "journal" || path.Dir(name) == "spool"
}
func (lock *dataLock) prepareDirectory(name string) error {
	if name != "journal" && name != "spool" && name != "results" && name != "resources" && name != "deletions" && name != "retention" {
		return failure("persistence_error")
	}
	if err := lock.root.Mkdir(name, 0700); err != nil && !os.IsExist(err) {
		return failure("persistence_error")
	}
	info, err := lock.root.Lstat(name)
	if err != nil || !privateInfo(info, true) {
		return failure("persistence_error")
	}
	return lock.syncDirectory(".")
}
func (lock *dataLock) syncDirectory(name string) error {
	file, err := lock.root.OpenFile(name, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return failure("persistence_error")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !privateInfo(info, true) || file.Sync() != nil {
		return failure("persistence_error")
	}
	return lock.Check()
}
func (lock *dataLock) removeFile(name string, previous os.FileInfo) error {
	if !ownedFileName(name) || previous == nil {
		return failure("persistence_error")
	}
	if err := lock.Check(); err != nil {
		return err
	}
	info, err := lock.root.Lstat(name)
	if err != nil || !privateInfo(info, false) || !os.SameFile(info, previous) {
		return failure("persistence_error")
	}
	if err = lock.root.Remove(name); err != nil {
		return failure("persistence_error")
	}
	return lock.syncDirectory(path.Dir(name))
}

func readSecretFile(name string) ([]byte, error) {
	before, err := os.Lstat(name)
	if err != nil || !privateReadableInfo(before) || before.Size() > 1<<20 {
		return nil, failure("secrets_invalid")
	}
	file, err := os.OpenFile(name, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, failure("secrets_invalid")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !privateReadableInfo(info) || !os.SameFile(before, info) {
		return nil, failure("secrets_invalid")
	}
	data, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	after, e := file.Stat()
	current, c := os.Lstat(name)
	if err != nil || e != nil || c != nil || len(data) > 1<<20 || !os.SameFile(info, current) || info.Size() != after.Size() || !info.ModTime().Equal(after.ModTime()) {
		return nil, failure("secrets_invalid")
	}
	return data, nil
}
func (lock *dataLock) resultParent() (string, error) {
	if err := lock.Check(); err != nil {
		return "", err
	}
	if err := lock.prepareDirectory("results"); err != nil {
		return "", err
	}
	return filepath.Join(lock.root.Name(), "results"), nil
}

// 只读取Collector已经完成的私有快照，拒绝链接、替换、特殊文件和内容变化。
func openSnapshot(ctx context.Context, resultDir string, artifact localArtifact) (*os.File, error) {
	root, err := os.OpenRoot(resultDir)
	if err != nil {
		return nil, failure("artifact_invalid")
	}
	defer root.Close()
	name := artifact.SnapshotPath
	if !filepath.IsLocal(name) || strings.Contains(name, "\\") {
		return nil, failure("artifact_invalid")
	}
	parts := strings.Split(filepath.ToSlash(name), "/")
	for index := range parts {
		partial := strings.Join(parts[:index+1], "/")
		info, e := root.Lstat(partial)
		if e != nil || info.Mode()&os.ModeSymlink != 0 || index < len(parts)-1 && !privateInfo(info, true) {
			return nil, failure("artifact_invalid")
		}
	}
	before, err := root.Lstat(name)
	if err != nil || !privateInfo(before, false) || before.Size() != artifact.Declaration.Size {
		return nil, failure("artifact_invalid")
	}
	file, err := root.OpenFile(name, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, failure("artifact_invalid")
	}
	valid := false
	defer func() {
		if !valid {
			file.Close()
		}
	}()
	info, err := file.Stat()
	if err != nil || !os.SameFile(info, before) {
		return nil, failure("artifact_invalid")
	}
	hash := sha256.New()
	buffer := make([]byte, 32<<10)
	var size int64
	for {
		if ctx.Err() != nil {
			return nil, failure("authority_lost")
		}
		n, e := file.Read(buffer)
		if n > 0 {
			hash.Write(buffer[:n])
			size += int64(n)
			if size > artifact.Declaration.Size {
				return nil, failure("artifact_invalid")
			}
		}
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, failure("artifact_invalid")
		}
	}
	after, e := file.Stat()
	current, c := root.Lstat(name)
	if e != nil || c != nil || !os.SameFile(current, info) || info.Size() != after.Size() || !info.ModTime().Equal(after.ModTime()) || size != artifact.Declaration.Size || hex.EncodeToString(hash.Sum(nil)) != artifact.Declaration.SHA256 {
		return nil, failure("artifact_invalid")
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return nil, failure("artifact_invalid")
	}
	valid = true
	return file, nil
}

// journal读取在私有Root内，不跟链接且非阻塞打开，读前后拒绝替换/尺寸/mtime变化。
func (lock *dataLock) readJournalFile(name string) ([]byte, os.FileInfo, error) {
	if err := lock.Check(); err != nil {
		return nil, nil, err
	}
	parent, err := lock.root.Lstat("journal")
	if err != nil || !privateInfo(parent, true) {
		return nil, nil, failure("data_invalid")
	}
	relative := path.Join("journal", name)
	if path.Dir(relative) != "journal" || strings.Contains(name, "\\") {
		return nil, nil, failure("data_invalid")
	}
	before, err := lock.root.Lstat(relative)
	if err != nil || !privateInfo(before, false) || before.Size() < 1 || before.Size() > 1<<20 {
		return nil, nil, failure("data_invalid")
	}
	file, err := lock.root.OpenFile(relative, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, nil, failure("data_invalid")
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !privateInfo(opened, false) || !os.SameFile(before, opened) {
		return nil, nil, failure("data_invalid")
	}
	data, readErr := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	after, statErr := file.Stat()
	current, pathErr := lock.root.Lstat(relative)
	currentParent, parentErr := lock.root.Lstat("journal")
	if readErr != nil || statErr != nil || pathErr != nil || parentErr != nil || !privateInfo(currentParent, true) || !os.SameFile(parent, currentParent) || !privateInfo(current, false) || !os.SameFile(opened, current) || int64(len(data)) != opened.Size() || opened.Size() != after.Size() || !opened.ModTime().Equal(after.ModTime()) || opened.Size() != current.Size() || !opened.ModTime().Equal(current.ModTime()) {
		return nil, nil, failure("data_invalid")
	}
	if err = lock.Check(); err != nil {
		return nil, nil, err
	}
	return data, opened, nil
}

func (lock *dataLock) journalNames() ([]string, error) {
	if err := lock.Check(); err != nil {
		return nil, err
	}
	before, err := lock.root.Lstat("journal")
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil || !privateInfo(before, true) {
		return nil, failure("data_invalid")
	}
	directory, err := lock.root.OpenFile("journal", os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, failure("data_invalid")
	}
	defer directory.Close()
	opened, err := directory.Stat()
	if err != nil || !privateInfo(opened, true) || !os.SameFile(before, opened) {
		return nil, failure("data_invalid")
	}
	entries, err := directory.ReadDir(129)
	if err != nil && err != io.EOF || len(entries) > 128 {
		return nil, failure("data_invalid")
	}
	current, err := lock.root.Lstat("journal")
	if err != nil || !privateInfo(current, true) || !os.SameFile(opened, current) {
		return nil, failure("data_invalid")
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	if err = lock.Check(); err != nil {
		return nil, err
	}
	return names, nil
}
