package mobile

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"
)

// 子测试进程执行同一原生Handler；生产入口由CLI注册，无测试stub。
func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == "__ios-signing" {
		if HandleIOSHelper(os.Stdin, os.Stdout) != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestIOSResourcesPreserveReplacedDirectory(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	owned := root + "/owned"
	if err := os.Mkdir(owned, 0700); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Lstat(owned)
	resources := &IOSResources{output: owned, outputInfo: info, env: map[string]string{}}
	if err := os.Rename(owned, root+"/original"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(owned, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(owned+"/keep", []byte("user"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := resources.Close(context.Background()); err != ErrIOSCleanup {
		t.Fatal("替换目录未报告清理失败")
	}
	if _, err := os.Stat(owned + "/keep"); err != nil {
		t.Fatal("未知替换目录被删除")
	}
}

func TestIOSResourcesCloseIdempotent(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	owned := root + "/owned"
	if err := os.Mkdir(owned, 0700); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Lstat(owned)
	resources := &IOSResources{output: owned, outputInfo: info, env: map[string]string{"KEY": "value"}}
	for range 2 {
		if err := resources.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if len(resources.Environment()) != 0 {
		t.Fatal("已关闭资源仍注入环境")
	}
}

func TestIOSResourcesPreserveModifiedProfile(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	filename := root + "/owned.mobileprovision"
	original := []byte("owned-profile")
	if os.WriteFile(filename, original, 0600) != nil {
		t.Fatal("自有profile写入失败")
	}
	info, _ := os.Lstat(filename)
	resources := &IOSResources{workspace: root, profile: filename, profileInfo: info, profileHash: sha256.Sum256(original), env: map[string]string{}}
	if os.WriteFile(filename, []byte("unknown-replacement"), 0600) != nil {
		t.Fatal("profile替换测试写入失败")
	}
	if resources.Close(context.Background()) != ErrIOSCleanup {
		t.Fatal("修改profile未标记清理失败")
	}
	data, err := os.ReadFile(filename)
	if err != nil || string(data) != "unknown-replacement" {
		t.Fatal("未知profile内容被删除")
	}
}
