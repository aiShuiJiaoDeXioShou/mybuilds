package protocol

import (
	"strings"
	"testing"
)

func TestChangeFactsFrozenIntegrityAndPathBoundary(t *testing.T) {
	target := strings.Repeat("a", 40)
	p := []string{"android/a", "shared/b"}
	f := &ChangeFacts{Mode: "diff", TargetSHA: target, BaselineSHA: strings.Repeat("b", 40), BaselineBuildID: "old", Paths: p, Digest: ChangesDigest(p)}
	if !ValidateChanges(f, target) {
		t.Fatal("真实排序事实被拒")
	}
	bad := *f
	bad.Paths = []string{"shared/b", "android/a"}
	bad.Digest = ChangesDigest(bad.Paths)
	if ValidateChanges(&bad, target) {
		t.Fatal("未拒乱序")
	}
	for _, s := range []string{"../escape", "a/../b", "/absolute", "a\\b", "a\x00b", "a//b"} {
		if ValidChangePath(s) {
			t.Fatalf("危险路径被接受 %q", s)
		}
	}
	bad = *f
	bad.Digest = strings.Repeat("0", 64)
	if ValidateChanges(&bad, target) {
		t.Fatal("未核摘要")
	}
	bad = *f
	bad.Mode = "full"
	bad.Reason = "baseline_missing"
	if ValidateChanges(&bad, target) {
		t.Fatal("full不应含路径")
	}
}
