package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPublishDecisionPrivateBoundaries(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "decision.json")
	valid := []byte(`{"intent_id":"explicit","remote_evidence":{}}`)
	if err := os.WriteFile(filename, valid, 0600); err != nil {
		t.Fatal(err)
	}
	if data, err := ReadPrivateJSON(filename, 64<<10); err != nil || string(data) != string(valid) {
		t.Fatal("私有普通文件", err)
	}
	for _, content := range []string{`{"key":"one","key":"two"}`, `{"Key":"one"}`, `{"key":null}`, `{"key":{}} {}`, `{"key":`} {
		if err := os.WriteFile(filename, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadPrivateJSON(filename, 64<<10); err == nil {
			t.Fatal("非法JSON被接受")
		}
	}
	if err := os.WriteFile(filename, valid, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filename, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPrivateJSON(filename, 64<<10); err == nil {
		t.Fatal("公开决定材料被接受")
	}
	os.Chmod(filename, 0600)
	link := filepath.Join(dir, "link.json")
	if err := os.Symlink(filename, link); err == nil {
		if _, err := ReadPrivateJSON(link, 64<<10); err == nil {
			t.Fatal("叶链接被接受")
		}
	}
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		hard := filepath.Join(dir, "hard.json")
		if err := os.Link(filename, hard); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadPrivateJSON(filename, 64<<10); err == nil {
			t.Fatal("硬链接私有材料被接受")
		}
	}
}
