package client

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mybuilds/internal/store"
)

func TestPublishCLIActualBindingListDoctorAndLocalIsolation(t *testing.T) {
	_, endpoint := realRemoteAPI(t)
	dir := t.TempDir()
	call := func(args ...string) string {
		t.Helper()
		out, err := executeRemote(t, append([]string{"--server-url", endpoint}, args...)...)
		if err != nil || strings.Contains(out, "PRIVATE_") {
			t.Fatal("真实发布管理CLI", out, err)
		}
		return out
	}
	call("node", "create", "linux", "--json")
	call("project", "init", "publisher", "--repo", dir, "--nodes", "linux", "--json")
	out := call("project", "app", "bind", "publisher", "--store", "google_play", "--app-id", "com.example.cli", "--node", "linux", "--credentials-env", "PRIVATE_PLAY_JSON", "--upload-cert-sha256", strings.Repeat("a", 64), "--json")
	var app store.ApplicationView
	if err := json.Unmarshal([]byte(out), &app); err != nil || app.Status != "pending" || app.ID == "" {
		t.Fatal("未显示真实pending绑定", err)
	}
	call("project", "app", "ls", "publisher", "--json")
	// 初始诊断仍pending，显式再请求不得创建第二个并行管理动作。
	if _, err := executeRemote(t, "--server-url", endpoint, "project", "app", "doctor", app.ID, "--json"); err == nil {
		t.Fatal("重复pending诊断获得第二次动作")
	}
	if out := call("publish", "ls", "--project", "publisher", "--json"); !strings.Contains(out, `"items":[]`) {
		t.Fatal("未发布列表虚构远端结果", out)
	}
	broken := filepath.Join(dir, "client.yml")
	if err := os.WriteFile(broken, []byte("invalid: PRIVATE_CLIENT_CONFIG\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"publish", "--help"}, {"project", "app", "--help"}, {"doctor", "--help"}} {
		if _, err := executeRemote(t, append([]string{"--config", broken}, args...)...); err != nil {
			t.Fatal("本地help加载控制身份", err)
		}
	}
}
