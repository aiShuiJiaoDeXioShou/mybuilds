package client

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"mybuilds/internal/protocol"
)

func downloadName(name string) bool {
	if name == "" || len(name) > 255 || name == "." || name == ".." || strings.TrimSpace(name) != name || strings.ContainsAny(name, "/\\") {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func downloadRemoteArtifact(ctx context.Context, cmd *cobra.Command, id, output string) error {
	deadline, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	invalid := errors.New("产物下载失败")
	absolute, e := filepath.Abs(output)
	if e != nil || !downloadName(filepath.Base(absolute)) {
		return errors.New("输出路径无效")
	}
	if _, e = os.Lstat(absolute); e == nil || !os.IsNotExist(e) {
		return errors.New("输出已存在或路径无效")
	}
	cfg, e := remoteSettings(cmd)
	if e != nil {
		return e
	}
	var meta protocol.ArtifactView
	if e = requestJSON(deadline, cfg, "GET", "/api/artifacts/"+url.PathEscape(id), nil, &meta, ""); e != nil {
		return e
	}
	hashBytes, e := hex.DecodeString(meta.SHA256)
	if meta.ID != id || !downloadName(meta.Name) || meta.Size < 0 || meta.Size > 1<<30 || e != nil || len(hashBytes) != 32 || strings.ToLower(meta.SHA256) != meta.SHA256 {
		return invalid
	}
	transport, e := remoteTransport(cfg)
	if e != nil {
		return e
	}
	defer transport.CloseIdleConnections()
	transport.DisableCompression = true
	transport.ResponseHeaderTimeout = cfg.Timeout
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	r, e := http.NewRequestWithContext(deadline, "GET", cfg.Server+"/api/artifacts/"+url.PathEscape(id)+"/"+url.PathEscape(meta.Name), nil)
	if e != nil {
		return invalid
	}
	r.Header.Set("Authorization", "Bearer "+cfg.RuntimeToken)
	r.Header.Set("Accept", "application/octet-stream")
	res, e := client.Do(r)
	if e != nil {
		return invalid
	}
	defer res.Body.Close()
	digests := res.Header.Values("X-Content-SHA256")
	if res.StatusCode != 200 || res.ContentLength != meta.Size || len(digests) != 1 || digests[0] != meta.SHA256 || len(res.Header.Values("Content-Encoding")) != 0 {
		return invalid
	}
	directory := filepath.Dir(absolute)
	if e = os.MkdirAll(directory, 0700); e != nil {
		return invalid
	}
	info, e := os.Lstat(directory)
	if e != nil || !downloadPrivate(info, true) {
		return invalid
	}
	root, e := os.OpenRoot(directory)
	if e != nil {
		return invalid
	}
	defer root.Close()
	held, e := root.Stat(".")
	if e != nil || !os.SameFile(held, info) {
		return invalid
	}
	stage := ".mybuilds-download-" + uuid.NewString()
	file, e := root.OpenFile(stage, os.O_WRONLY|os.O_CREATE|os.O_EXCL|downloadOpenFlags(), 0600)
	if e != nil {
		return invalid
	}
	defer file.Close()
	owned, e := file.Stat()
	if e != nil || !downloadPrivate(owned, false) {
		return invalid
	}
	defer func() {
		current, e := root.Lstat(stage)
		if e == nil && downloadPrivate(current, false) && os.SameFile(current, owned) {
			_ = root.Remove(stage)
		}
	}()
	hash := sha256.New()
	n, e := io.Copy(io.MultiWriter(file, hash), io.LimitReader(res.Body, meta.Size+1))
	if e != nil || n != meta.Size || hex.EncodeToString(hash.Sum(nil)) != meta.SHA256 || deadline.Err() != nil {
		return invalid
	}
	if e = file.Sync(); e != nil {
		return invalid
	}
	if e = file.Close(); e != nil {
		return invalid
	}
	current, e := root.Lstat(stage)
	parent, pe := os.Lstat(directory)
	if e != nil || pe != nil || !downloadPrivate(current, false) || !os.SameFile(current, owned) || !os.SameFile(parent, info) {
		return invalid
	}
	if deadline.Err() != nil {
		return invalid
	}
	if e = root.Link(stage, filepath.Base(absolute)); e != nil {
		return errors.New("输出已存在或发布失败")
	}
	if e = root.Remove(stage); e != nil {
		return invalid
	}
	dir, e := root.OpenFile(".", os.O_RDONLY|downloadOpenFlags(), 0)
	if e != nil {
		return invalid
	}
	e = dir.Sync()
	closed := dir.Close()
	if e != nil || closed != nil {
		return invalid
	}
	return nil
}
