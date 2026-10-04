//go:build darwin || linux

package agent

import (
	"context"
	"golang.org/x/sys/unix"
	"io"
	"mybuilds/internal/config"
	"mybuilds/internal/pipeline"
	"mybuilds/internal/protocol"
	"os"
	"path/filepath"
	"testing"
)

func TestUploadSourceIsActualCollectorSnapshotAndRejectsUnsafeFiles(t *testing.T) {
	for _, mode := range []string{"valid", "symlink", "fifo", "public", "hardlink", "tamper", "outside", "size", "hash", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			workspace := t.TempDir()
			if err := os.WriteFile(filepath.Join(workspace, "output.apk"), []byte("APK"), 0600); err != nil {
				t.Fatal(err)
			}
			document := &config.Document{Version: 1, Builds: map[string]*config.Build{"fixture": {Steps: []config.Step{{Kind: "artifact", Name: "collect", Paths: []string{"output.apk"}}}}}}
			result, err := pipeline.Run(context.Background(), document, pipeline.RunOptions{Workspace: workspace, Output: io.Discard})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.RemoveAll(result.ResultDir) })
			record := result.Builds[0].Steps[0].Artifacts[0]
			artifact := localArtifact{SnapshotPath: record.SnapshotPath, Declaration: protocol.ArtifactDeclaration{Size: record.Size, SHA256: record.SHA256}}
			snapshot := filepath.Join(result.ResultDir, record.SnapshotPath)
			ctx := context.Background()
			switch mode {
			case "symlink":
				os.Remove(snapshot)
				if err := os.Symlink(filepath.Join(workspace, "output.apk"), snapshot); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				os.Remove(snapshot)
				if err := unix.Mkfifo(snapshot, 0600); err != nil {
					t.Fatal(err)
				}
			case "public":
				if err := os.Chmod(snapshot, 0644); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(snapshot, filepath.Join(result.ResultDir, "linked")); err != nil {
					t.Fatal(err)
				}
			case "tamper":
				if err := os.WriteFile(snapshot, []byte("bad"), 0600); err != nil {
					t.Fatal(err)
				}
			case "outside":
				artifact.SnapshotPath = "../output.apk"
			case "size":
				artifact.Declaration.Size++
			case "hash":
				artifact.Declaration.SHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
			case "cancel":
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			}
			file, err := openSnapshot(ctx, result.ResultDir, artifact)
			if mode != "valid" {
				if file != nil {
					file.Close()
				}
				if err == nil {
					t.Fatal("unsafe snapshot opened", mode)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			data, err := io.ReadAll(file)
			if err != nil || string(data) != "APK" {
				t.Fatal("not actual snapshot", err)
			}
		})
	}
}
