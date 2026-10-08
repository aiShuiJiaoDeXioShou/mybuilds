//go:build darwin || linux

package agent

import (
	"context"
	"golang.org/x/sys/unix"
	"mybuilds/internal/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSecretsFailureStopsActualTaskBeforeCheckout(t *testing.T) {
	for _, mode := range []string{"missing", "public"} {
		t.Run(mode, func(t *testing.T) {
			st, _, cfg, _, id := actualFaultControl(t, "version: 1\nsteps: [{kind: run, run: touch must-not-run}]\n", "")
			cfg.SecretsFile = filepath.Join(t.TempDir(), "secrets.env")
			if mode == "public" {
				if err := os.WriteFile(cfg.SecretsFile, []byte("SECRET=value\n"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			done := make(chan error, 1)
			go func() { done <- Serve(ctx, cfg) }()
			defer func() {
				cancel()
				select {
				case <-done:
				case <-time.After(3 * time.Second):
					t.Error("Agent未停止")
				}
			}()
			for ctx.Err() == nil {
				view, err := st.GetBuild(ctx, id)
				if err != nil {
					t.Fatal(err)
				}
				if view.Status == "failed" {
					paths, err := filepath.Glob(filepath.Join(cfg.DataDir, "scm", "checkout-*"))
					if err != nil || len(paths) != 0 || view.Reason != "precheck_error" || len(view.Steps) != 1 || view.Steps[0].Started {
						t.Fatal("秘密读取失败后执行了外部动作", err)
					}
					return
				}
				time.Sleep(20 * time.Millisecond)
			}
			t.Fatal("未回报预检查失败")
		})
	}
}

func TestSecretsActualPrivateFileAndTaskIsolation(t *testing.T) {
	file := filepath.Join(t.TempDir(), "secrets.env")
	input := "# own\nJKS_PASSWORD=private-test-marker\nUNRELATED=not-selected\nMYBUILDS_GIT_SSH_KEY=/own/key\nMYBUILDS_GIT_KNOWN_HOSTS=/own/hosts\n"
	if err := os.WriteFile(file, []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := config.AgentConfig{SecretsFile: file, TokenEnv: "OWN_NODE_TOKEN", RuntimeToken: strings.Repeat("t", 32)}
	values, err := loadSecrets(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(file, 0400); err != nil {
		t.Fatal(err)
	}
	if got, err := loadSecrets(cfg); err != nil || got["JKS_PASSWORD"] != values["JKS_PASSWORD"] {
		t.Fatal("只读秘密被拒绝", err)
	}
	build := config.Build{Env: map[string]string{"PASSWORD": "${JKS_PASSWORD}"}}
	selected := taskSecrets(cfg, build, values)
	if len(selected) != 1 || selected["JKS_PASSWORD"] != "private-test-marker" {
		t.Fatal("task isolation")
	}
	values["TOKEN_ALIAS"] = cfg.RuntimeToken
	for _, name := range []string{"OWN_NODE_TOKEN", "MYBUILDS_AGENT_TOKEN", "MYBUILDS_CLIENT_TOKEN", "MYBUILDS_SERVER_TOKEN", "MYBUILDS_CONTROL_TOKEN", "MYBUILDS_BOOTSTRAP_TOKEN", "MYBUILDS_GIT_SSH_KEY", "MYBUILDS_GIT_KNOWN_HOSTS", "TOKEN_ALIAS"} {
		if name != "TOKEN_ALIAS" {
			values[name] = "private-test-marker"
		}
		build.Env["PASSWORD"] = "${" + name + "}"
		selected = taskSecrets(cfg, build, values)
		if len(selected) != 0 {
			t.Fatal("credential reference accepted", name)
		}
		selected = taskSecrets(cfg, config.Build{Steps: []config.Step{{Kind: "upload", Credentials: "${" + name + "}"}}}, values)
		if len(selected) != 0 {
			t.Fatal("查询凭据筛选与构建不一致", name)
		}
	}
	build.Env["PASSWORD"] = "prefix-${JKS_PASSWORD}-${UNRELATED}"
	selected = taskSecrets(cfg, build, values)
	if len(selected) != 2 {
		t.Fatal("combined explicit refs")
	}
	selected = taskSecrets(cfg, config.Build{Steps: []config.Step{{Kind: "upload", Credentials: "${JKS_PASSWORD}"}}, Post: &config.Post{Always: []config.Step{{Env: map[string]string{"SECRET": "${UNRELATED}"}}}}}, values)
	if len(selected) != 2 {
		t.Fatal("上传与post秘密丢失")
	}
	if err = os.Chmod(file, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = loadSecrets(cfg); err == nil {
		t.Fatal("public secrets file")
	}
	if err = os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if _, err = loadSecrets(cfg); err == nil {
		t.Fatal("缺失秘密必须在使用时失败")
	}
	if err = unix.Mkfifo(file, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = loadSecrets(cfg); err == nil {
		t.Fatal("FIFO secrets file")
	}
}
func TestSecretsRejectDuplicateNULAndUnknownSyntax(t *testing.T) {
	for _, input := range []string{"SECRET=one\nSECRET=two\n", "SECRET=private-test-marker\x00", "export SECRET=value", "BAD-NAME=value", "SECRET=value\r\n"} {
		file := filepath.Join(t.TempDir(), "secrets.env")
		if err := os.WriteFile(file, []byte(input), 0600); err != nil {
			t.Fatal(err)
		}
		_, err := loadSecrets(config.AgentConfig{SecretsFile: file})
		if err == nil || strings.Contains(err.Error(), "private-test-marker") {
			t.Fatal("unsafe parser result", err)
		}
	}
}
