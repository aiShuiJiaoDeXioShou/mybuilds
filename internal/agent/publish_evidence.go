package agent

import (
	"encoding/json"
	"github.com/google/uuid"
	"os"
	"path/filepath"
)

// 原发布证据随原results保留；停止确认清journal不抹去尚待远端核对的回执。
func (e *taskExecution) savePublishEvidence(id string) error {
	j := e.journal
	j.mu.Lock()
	var record *publishCheckpoint
	directory := j.state.ResultDir
	for _, item := range j.state.Publishes {
		if item.IntentID == id {
			copyItem := item
			record = &copyItem
			break
		}
	}
	j.mu.Unlock()
	if record == nil || !exactUUID(id) || directory == "" {
		return failure("persistence_error")
	}
	base, err := filepath.EvalSymlinks(e.cfg.DataDir)
	if err != nil {
		return failure("persistence_error")
	}
	canonicalDirectory, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return failure("persistence_error")
	}
	relative, err := filepath.Rel(base, canonicalDirectory)
	if err != nil || !filepath.IsLocal(relative) || len(relative) < 8 || relative[:8] != "results/" {
		return failure("persistence_error")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return failure("persistence_error")
	}
	defer root.Close()
	if err = root.MkdirAll("publishes", 0700); err != nil {
		return failure("persistence_error")
	}
	directoryInfo, err := root.Lstat("publishes")
	if err != nil || !directoryInfo.IsDir() || directoryInfo.Mode().Perm() != 0700 {
		return failure("persistence_error")
	}
	if e.publishEvidence == nil {
		e.publishEvidence = map[string]os.FileInfo{}
	}
	name := filepath.Join("publishes", id+".json")
	before, err := root.Lstat(name)
	previous := e.publishEvidence[id]
	if previous == nil && !os.IsNotExist(err) || previous != nil && (err != nil || !before.Mode().IsRegular() || !os.SameFile(previous, before)) {
		return failure("persistence_error")
	}
	data, err := json.Marshal(record)
	if err != nil || len(data) > 64<<10 {
		return failure("persistence_error")
	}
	temp := filepath.Join("publishes", uuid.NewString()+".tmp")
	file, err := root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return failure("persistence_error")
	}
	_, err = file.Write(data)
	if err == nil {
		err = file.Sync()
	}
	info, statErr := file.Stat()
	closeErr := file.Close()
	if err != nil || statErr != nil || closeErr != nil {
		return failure("persistence_error")
	}
	// 不以新路径替换未知原叶；失败保留本次随机临时证据，不猜测清理。
	current, err := root.Lstat(name)
	if previous == nil && !os.IsNotExist(err) || previous != nil && (err != nil || !os.SameFile(previous, current)) {
		return failure("persistence_error")
	}
	if root.Rename(temp, name) != nil {
		return failure("persistence_error")
	}
	parent, err := root.Open("publishes")
	if err != nil {
		return failure("persistence_error")
	}
	err = parent.Sync()
	closeErr = parent.Close()
	if err != nil || closeErr != nil {
		return failure("persistence_error")
	}
	e.publishEvidence[id] = info
	return nil
}
