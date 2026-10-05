package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPublishDoctorDoesNotLoadControlIdentity(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "agent.yml")
	if err := os.WriteFile(file, []byte("token: '${MISSING_PRIVATE_AGENT_IDENTITY}'\npublish_tools: {bundle_dir: tools, bundletool: tools/bundletool.jar}\ndata_dir: data\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MYBUILDS_AGENT_TOKEN", "UNRELATED_PRIVATE_IDENTITY")
	tools, data, err := LoadPublishDoctor(file)
	if err != nil || tools.BundleDir != filepath.Join(dir, "tools") || tools.Bundletool != filepath.Join(dir, "tools/bundletool.jar") || data != filepath.Join(dir, "data") {
		t.Fatal("诊断意外需要控制端身份", err)
	}
	if err := os.Chmod(file, 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err = LoadPublishDoctor(file); err == nil {
		t.Fatal("公开身份配置被接受")
	}
	if _, _, err = LoadPublishDoctor(""); err == nil {
		t.Fatal("诊断猜测默认Agent配置")
	}
}
