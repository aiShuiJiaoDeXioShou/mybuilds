//go:build darwin || linux

package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRetentionResourceCanonicalResultRoot(t *testing.T) {
	j, w, r := resourceFixture(t)
	in, err := j.stageWorkspace(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	if err = j.ackResource(in); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(r)
	if err != nil {
		t.Fatal(err)
	}
	// 使用真实ensureResult的系统规范路径，不跟随用户slot里的链接。
	if _, err = j.stageResult(context.Background(), canonical); err != nil {
		t.Fatal("same physical result root rejected", err)
	}
}
func TestRetentionResourceDoesNotResolveLinkedInputSlot(t *testing.T) {
	j, w, r := resourceFixture(t)
	in, err := j.stageWorkspace(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	if err = j.ackResource(in); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(filepath.Dir(r), "mybuilds-linked")
	if err = os.Symlink(r, link); err != nil {
		t.Fatal(err)
	}
	if _, err = j.stageResult(context.Background(), link); err == nil {
		t.Fatal("linked result input resolved into valid slot")
	}
}
