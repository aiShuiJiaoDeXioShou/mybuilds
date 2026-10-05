package scm

import (
	"context"
	"mybuilds/internal/protocol"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestReadChangesFixedTreesRenameDeleteAndMissing(t *testing.T) {
	o, _ := gitFixture(t, "sha1")
	writeFixture(t, o.Repository, "old.txt", "same")
	writeFixture(t, o.Repository, "deleted.txt", "delete")
	fixtureGit(t, o.Repository, "add", ".")
	fixtureGit(t, o.Repository, "commit", "-m", "base")
	base := fixtureGit(t, o.Repository, "rev-parse", "HEAD")
	if err := os.Rename(filepath.Join(o.Repository, "old.txt"), filepath.Join(o.Repository, "new.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(o.Repository, "deleted.txt")); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, o.Repository, "added.txt", "new")
	fixtureGit(t, o.Repository, "add", "-A")
	fixtureGit(t, o.Repository, "commit", "-m", "target")
	target := fixtureGit(t, o.Repository, "rev-parse", "HEAD")
	got, err := ReadChanges(context.Background(), ChangeOptions{Source: o, TargetSHA: target, BaselineSHA: base})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"added.txt", "deleted.txt", "new.txt", "old.txt"}
	if !got.BaselineAvailable || got.TargetSHA != target || !slices.Equal(got.Paths, want) || got.Digest != protocol.ChangesDigest(want) {
		t.Fatalf("不完整的固定tree差异: %+v", got)
	}
	missing, err := ReadChanges(context.Background(), ChangeOptions{Source: o, TargetSHA: target, BaselineSHA: strings.Repeat("1", 40)})
	if err != nil || missing.BaselineAvailable || missing.Paths == nil {
		t.Fatalf("精确missing失败: %+v %v", missing, err)
	}
	same, err := ReadChanges(context.Background(), ChangeOptions{Source: o, TargetSHA: base, BaselineSHA: base})
	if err != nil || !same.BaselineAvailable || len(same.Paths) != 0 {
		t.Fatalf("空差异不是full: %+v %v", same, err)
	}
	requireClean(t, o.DataDir)
}
