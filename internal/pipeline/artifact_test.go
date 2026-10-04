package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func artifactWorkspace(t *testing.T) (*os.Root, string) {
	t.Helper()
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root, dir
}

func artifactWrite(t *testing.T, root *os.Root, name, data string) {
	t.Helper()
	if err := root.MkdirAll(filepath.Dir(name), 0700); err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile(name, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestArtifactRecursiveSnapshot(t *testing.T) {
	root, _ := artifactWorkspace(t)
	files := map[string]string{"out/app.apk": "zero", "out/a/app.apk": "one", "out/a/b/app.apk": "two"}
	for name, data := range files {
		artifactWrite(t, root, name, data)
	}
	destination := filepath.Join(t.TempDir(), "snapshot")
	records, err := collectArtifacts(context.Background(), root, destination, []string{"out/**/*.apk", "out/a/*.apk"})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != len(files) {
		t.Fatalf("匹配去重错误: %+v", records)
	}
	previous := ""
	for _, record := range records {
		if record.SourcePath <= previous {
			t.Fatal("清单未稳定排序")
		}
		previous = record.SourcePath
		data, err := os.ReadFile(filepath.Join(destination, filepath.FromSlash(record.SnapshotPath)))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if string(data) != files[record.SourcePath] || record.Size != int64(len(data)) || record.SHA256 != hex.EncodeToString(sum[:]) {
			t.Fatal("快照与清单不一致", record)
		}
		info, err := os.Stat(filepath.Join(destination, record.SnapshotPath))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatal("快照权限过宽", info.Mode())
		}
	}
	manifest, err := os.ReadFile(filepath.Join(destination, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved []ArtifactRecord
	if err := json.Unmarshal(manifest, &saved); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(records, saved) {
		t.Fatal("manifest与返回清单不一致")
	}
	artifactWrite(t, root, "out/app.apk", "post rewrite")
	data, err := os.ReadFile(filepath.Join(destination, "files/out/app.apk"))
	if err != nil || string(data) != "zero" {
		t.Fatal("源变化改写既有快照")
	}
}

func TestArtifactEachPatternNeedsFile(t *testing.T) {
	root, _ := artifactWorkspace(t)
	artifactWrite(t, root, "out/app.apk", "data")
	for _, pattern := range []string{"out/missing*.apk", "out", "absent/**/*.apk"} {
		parent := t.TempDir()
		destination := filepath.Join(parent, "snapshot")
		records, err := collectArtifacts(context.Background(), root, destination, []string{"out/*.apk", pattern})
		if err == nil || len(records) != 0 {
			t.Fatal("每模式零合法文件未失败", pattern)
		}
		entries, err := os.ReadDir(parent)
		if err != nil || len(entries) != 0 {
			t.Fatal("失败留下部分清单或stage", entries, err)
		}
	}
}

func TestArtifactPatternValidation(t *testing.T) {
	for _, patterns := range [][]string{nil, {""}, {"/secret"}, {"../secret"}, {"a/../b"}, {"a/./b"}, {"C:/secret"}, {`a\b`}, {"[bad"}, {"{a,b"}, {"{../secret,out}/*.apk"}} {
		if err := validateArtifactPatterns(patterns); err == nil {
			t.Fatal("非法模式未拒绝", patterns)
		}
	}
	for _, pattern := range []string{"out/**/*.apk", "{out,build}/*.apk", "out/[ab]?.apk"} {
		if err := validateArtifactPatterns([]string{pattern}); err != nil {
			t.Fatal("合法模式被拒绝", pattern, err)
		}
	}
}

func TestArtifactLinks(t *testing.T) {
	root, dir := artifactWorkspace(t)
	artifactWrite(t, root, "actual/nested/app.apk", "data")
	for name, target := range map[string]string{"file.apk": "actual/nested/app.apk", "alias": "actual", "actual/loop": "../actual"} {
		if err := root.Symlink(target, name); err != nil {
			t.Skipf("当前环境不能创建链接: %v", err)
		}
	}
	for _, patterns := range [][]string{{"file.apk"}, {"alias/**/*.apk"}, {"actual/**/*.apk"}} {
		records, err := collectArtifacts(context.Background(), root, filepath.Join(t.TempDir(), "snapshot"), patterns)
		if err != nil || len(records) != 1 {
			t.Fatal("合法链接或循环处理错误", patterns, records, err)
		}
	}
	records, err := collectArtifacts(context.Background(), root, filepath.Join(t.TempDir(), "snapshot"), []string{"**/*.apk"})
	if err != nil || len(records) != 2 {
		t.Fatal("通配跟随目录链接", records, err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.apk"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(dir, filepath.Join(outside, "secret.apk"))
	if err != nil {
		t.Fatal(err)
	}
	for name, target := range map[string]string{"SECRET-out.apk": relative, "SECRET-absolute.apk": filepath.Join(dir, "actual/nested/app.apk"), "SECRET-dangling.apk": "absent.apk", "SECRET-dir": outside} {
		if err := root.Symlink(target, name); err != nil {
			t.Fatal(err)
		}
		parent := t.TempDir()
		pattern := name
		if name == "SECRET-dir" {
			pattern += "/*.apk"
		}
		records, err := collectArtifacts(context.Background(), root, filepath.Join(parent, "snapshot"), []string{pattern})
		if err == nil || len(records) != 0 || strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), outside) {
			t.Fatal("非法链接未安全拒绝", records, err)
		}
		artifactAssertEmpty(t, parent)
	}
}

func artifactAssertEmpty(t *testing.T, parent string) {
	t.Helper()
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 0 {
		t.Fatal("失败留下可见目标或暂存目录", entries, err)
	}
}

func TestArtifactExistingTarget(t *testing.T) {
	root, _ := artifactWorkspace(t)
	artifactWrite(t, root, "file", "snapshot data")
	for _, kind := range []string{"file", "empty-directory", "directory", "link"} {
		t.Run(kind, func(t *testing.T) {
			parent := t.TempDir()
			destination := filepath.Join(parent, "snapshot")
			switch kind {
			case "file":
				if err := os.WriteFile(destination, []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
			case "empty-directory", "directory":
				if err := os.Mkdir(destination, 0700); err != nil {
					t.Fatal(err)
				}
				if kind == "directory" {
					if err := os.WriteFile(filepath.Join(destination, "keep"), []byte("keep"), 0600); err != nil {
						t.Fatal(err)
					}
				}
			case "link":
				if err := os.Symlink("missing", destination); err != nil {
					t.Skip(err)
				}
			}
			before, err := os.Lstat(destination)
			if err != nil {
				t.Fatal(err)
			}
			records, err := collectArtifacts(context.Background(), root, destination, []string{"file"})
			if err == nil || len(records) != 0 {
				t.Fatal("已有目标未拒绝")
			}
			after, err := os.Lstat(destination)
			if err != nil || !os.SameFile(before, after) {
				t.Fatal("已有目标被替换", err)
			}
			entries, err := os.ReadDir(parent)
			if err != nil || len(entries) != 1 {
				t.Fatal("拒绝后留下stage", entries, err)
			}
			if kind == "file" {
				data, _ := os.ReadFile(destination)
				if string(data) != "keep" {
					t.Fatal("已有文件被覆盖")
				}
			}
			if kind == "directory" {
				data, _ := os.ReadFile(filepath.Join(destination, "keep"))
				if string(data) != "keep" {
					t.Fatal("已有目录被覆盖")
				}
			}
		})
	}
}

func TestArtifactCancelledTraversal(t *testing.T) {
	root, _ := artifactWorkspace(t)
	for i := 0; i < 800; i++ {
		if err := root.MkdirAll(fmt.Sprintf("tree/%04d/deep", i), 0700); err != nil {
			t.Fatal(err)
		}
	}
	parent := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	started := time.Now()
	records, err := collectArtifacts(ctx, root, filepath.Join(parent, "snapshot"), []string{"tree/**/*.missing"})
	if !errors.Is(err, context.DeadlineExceeded) || len(records) != 0 {
		t.Fatal("无匹配遍历不响应取消", records, err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("取消后遍历仍长时间运行")
	}
	artifactAssertEmpty(t, parent)
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	if _, err := collectArtifacts(ctx, root, filepath.Join(parent, "snapshot"), []string{"tree/**"}); !errors.Is(err, context.Canceled) {
		t.Fatal("已取消仍开始收集", err)
	}
	artifactAssertEmpty(t, parent)
}

func TestArtifactCopyCancellationAndChanges(t *testing.T) {
	for _, action := range []string{"cancel", "resize", "mtime", "replace"} {
		t.Run(action, func(t *testing.T) {
			root, dir := artifactWorkspace(t)
			source, err := root.Create("large")
			if err != nil {
				t.Fatal(err)
			}
			if err := source.Truncate(128 * 1024 * 1024); err != nil {
				t.Fatal(err)
			}
			if err := source.Close(); err != nil {
				t.Fatal(err)
			}
			parent := t.TempDir()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			stop := make(chan struct{})
			defer close(stop)
			go func() {
				ticker := time.NewTicker(time.Millisecond)
				defer ticker.Stop()
				for {
					select {
					case <-stop:
						return
					case <-ticker.C:
						copies, err := filepath.Glob(filepath.Join(parent, ".artifact-*", "files", "large"))
						if err != nil {
							done <- err
							return
						}
						for _, copy := range copies {
							info, err := os.Stat(copy)
							if err != nil || info.Size() == 0 {
								continue
							}
							switch action {
							case "cancel":
								cancel()
							case "resize":
								err = os.Truncate(filepath.Join(dir, "large"), 1)
							case "mtime":
								err = os.Chtimes(filepath.Join(dir, "large"), time.Unix(1, 0), time.Unix(1, 0))
							case "replace":
								err = os.Rename(filepath.Join(dir, "large"), filepath.Join(dir, "original"))
								if err == nil {
									err = os.WriteFile(filepath.Join(dir, "large"), []byte("replacement"), 0600)
								}
							}
							done <- err
							return
						}
					}
				}
			}()
			records, err := collectArtifacts(ctx, root, filepath.Join(parent, "snapshot"), []string{"large"})
			select {
			case actionErr := <-done:
				if actionErr != nil {
					t.Fatal(actionErr)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("没有在复制期间触发真实源变更")
			}
			if err == nil || len(records) != 0 {
				t.Fatal("运行中取消或源变化未拒绝", records, err)
			}
			if action == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatal("取消原因未保留", err)
			}
			artifactAssertEmpty(t, parent)
		})
	}
}

func TestArtifactConcurrentTarget(t *testing.T) {
	root, _ := artifactWorkspace(t)
	artifactWrite(t, root, "file", "content")
	parent := t.TempDir()
	destination := filepath.Join(parent, "snapshot")
	done := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := collectArtifacts(context.Background(), root, destination, []string{"file"})
			done <- err
		}()
	}
	first, second := <-done, <-done
	if (first == nil) == (second == nil) {
		t.Fatal("并发发布必须只成功一次", first, second)
	}
	data, err := os.ReadFile(filepath.Join(destination, "files/file"))
	if err != nil || string(data) != "content" {
		t.Fatal("并发快照受损", err)
	}
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 1 {
		t.Fatal("并发失败stage未清理", entries, err)
	}
}

func TestArtifactUnreadableSource(t *testing.T) {
	root, _ := artifactWorkspace(t)
	artifactWrite(t, root, "SECRET-unreadable", "data")
	if err := root.Chmod("SECRET-unreadable", 0); err != nil {
		t.Fatal(err)
	}
	defer root.Chmod("SECRET-unreadable", 0600)
	if source, err := root.Open("SECRET-unreadable"); err == nil {
		source.Close()
		t.Skip("当前用户可以绕过文件读取权限")
	}
	parent := t.TempDir()
	records, err := collectArtifacts(context.Background(), root, filepath.Join(parent, "snapshot"), []string{"SECRET-unreadable"})
	if err == nil || len(records) != 0 || strings.Contains(err.Error(), "SECRET") {
		t.Fatal("复制失败泄露路径或返回部分清单", records, err)
	}
	artifactAssertEmpty(t, parent)
}
