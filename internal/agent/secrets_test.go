//go:build darwin || linux

package agent

import (
	"golang.org/x/sys/unix"
	"mybuilds/internal/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
	build := config.Build{Env: map[string]string{"PASSWORD": "${JKS_PASSWORD}"}}
	selected, err := taskSecrets(cfg, build, values)
	if err != nil || len(selected) != 1 || selected["JKS_PASSWORD"] != "private-test-marker" {
		t.Fatal("task isolation", err)
	}
	for _, name := range []string{"OWN_NODE_TOKEN", "MYBUILDS_AGENT_TOKEN", "MYBUILDS_CLIENT_TOKEN", "MYBUILDS_SERVER_TOKEN", "MYBUILDS_GIT_SSH_KEY"} {
		values[name] = "private-test-marker"
		build.Env["PASSWORD"] = "${" + name + "}"
		selected, err = taskSecrets(cfg, build, values)
		if err != nil || len(selected) != 0 {
			t.Fatal("credential reference accepted", name)
		}
	}
	build.Env["PASSWORD"] = "prefix-${JKS_PASSWORD}-${UNRELATED}"
	selected, err = taskSecrets(cfg, build, values)
	if err != nil || len(selected) != 2 {
		t.Fatal("combined explicit refs", err)
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
