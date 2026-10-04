//go:build darwin || linux

package server

import (
	"context"
	"golang.org/x/sys/unix"
	"io"
	"mybuilds/internal/store"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLogDirectoryRejectsNonregularAndReplacement(t *testing.T) {
	for _, kind := range []string{"symlink", "fifo", "public"} {
		t.Run(kind, func(t *testing.T) {
			s, _, _, _, _ := executionHTTPFixture(t)
			p := filepath.Join(s.config.DataDir, "logs")
			switch kind {
			case "symlink":
				if e := os.Symlink(t.TempDir(), p); e != nil {
					t.Fatal(e)
				}
			case "fifo":
				if e := unix.Mkfifo(p, 0600); e != nil {
					t.Fatal(e)
				}
			case "public":
				if e := os.Mkdir(p, 0755); e != nil {
					t.Fatal(e)
				}
			}
			start := time.Now()
			root, e := s.logFiles()
			if e == nil {
				root.Close()
				t.Fatal("不安全目录接受")
			}
			if time.Since(start) > time.Second {
				t.Fatal("FIFO打开阻塞")
			}
		})
	}
	s, _, _, _, _ := executionHTTPFixture(t)
	root, e := s.logFiles()
	if e != nil {
		t.Fatal(e)
	}
	defer root.Close()
	candidate, e := publishEvidence(context.Background(), root, []byte("canonical"))
	if e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(s.config.DataDir, "logs", candidate.id)
	if e = os.Rename(p, p+".owned-old"); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(p, []byte("UNKNOWN"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = removeEvidenceCandidate(root, candidate); e == nil {
		t.Fatal("删除非自有替换文件")
	}
	data, e := os.ReadFile(p)
	if e != nil || string(data) != "UNKNOWN" {
		t.Fatal("未知文件被删除", e)
	}
	logs := filepath.Join(s.config.DataDir, "logs")
	if e = os.Rename(logs, logs+".owned-old"); e != nil {
		t.Fatal(e)
	}
	if e = os.Mkdir(logs, 0700); e != nil {
		t.Fatal(e)
	}
	if _, e = publishEvidence(context.Background(), root, []byte("never")); e == nil {
		t.Fatal("替换目录后继续发布")
	}
}
func TestLogPublicationCancelledBeforeCreate(t *testing.T) {
	s, _, _, _, _ := executionHTTPFixture(t)
	root, e := s.logFiles()
	if e != nil {
		t.Fatal(e)
	}
	defer root.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = publishEvidence(ctx, root, []byte("never")); e == nil {
		t.Fatal("取消仍发布")
	}
	entries, e := os.ReadDir(root.Name())
	if e != nil || len(entries) != 0 {
		t.Fatal("取消生成文件", entries, e)
	}
}

func TestActualArtifactDownloadRejectsNonownedLeaf(t *testing.T) {
	for _, kind := range []string{"symlink", "fifo", "hardlink", "public"} {
		t.Run(kind, func(t *testing.T) {
			s, st, h, token, declaration, data := artifactHTTPFixture(t)
			if code, body := uploadArtifactHTTP(t, h, token, declaration, data); code != 200 {
				t.Fatal(code, string(body))
			}
			admin, err := st.Authenticate(context.Background(), adminToken)
			if err != nil {
				t.Fatal(err)
			}
			stored, err := st.GetArtifact(context.Background(), admin, declaration.ID)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(s.config.DataDir, "artifacts", stored.StorageID)
			if kind == "public" {
				err = os.Chmod(path, 0644)
			} else {
				if err = os.Remove(path); err != nil {
					t.Fatal(err)
				}
				unknown := filepath.Join(t.TempDir(), "unknown")
				if err = os.WriteFile(unknown, data, 0600); err != nil {
					t.Fatal(err)
				}
				switch kind {
				case "symlink":
					err = os.Symlink(unknown, path)
				case "fifo":
					err = unix.Mkfifo(path, 0600)
				case "hardlink":
					err = os.Link(unknown, path)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			request, err := http.NewRequestWithContext(ctx, "GET", h.URL+"/api/artifacts/"+declaration.ID+"/"+declaration.Name, nil)
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Authorization", "Bearer "+adminToken)
			response, err := h.Client().Do(request)
			if err != nil {
				t.Fatal("非普通叶文件读取阻塞", err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil || response.StatusCode != 500 || strings.Contains(string(body), string(data)) {
				t.Fatal(response.StatusCode, string(body), err)
			}
			// 读取拒绝不会篡改已确认元数据或删除替换文件。
			if _, err = st.ListArtifacts(context.Background(), admin, declaration.Ref.BuildID, store.Page{Limit: 1}); err != nil {
				t.Fatal(err)
			}
			if _, err = os.Lstat(path); err != nil {
				t.Fatal("拒绝读取删除了未知叶文件", err)
			}
		})
	}
}
