package client

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestActualRemoteNodeCommands(t *testing.T) {
	_, address := realRemoteAPI(t)
	call := func(args ...string) string {
		t.Helper()
		out, err := executeRemote(t, append([]string{"--server-url", address}, args...)...)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	out := call("node", "create", "remote-node", "--capacity", "2", "--labels", "generic,release", "--json")
	var created struct{ Token string }
	if err := json.Unmarshal([]byte(out), &created); err != nil || len(created.Token) < 32 {
		t.Fatal(out, err)
	}
	for _, args := range [][]string{{"node", "ls", "--json"}, {"node", "show", "remote-node", "--json"}} {
		out := call(args...)
		if !strings.Contains(out, "remote-node") || strings.Contains(out, created.Token) {
			t.Fatal("公开节点视图错误", out)
		}
	}
	call("doctor", "--server", "--json")
	out, err := executeRemote(t, "--server-url", address, "doctor", "--node", "remote-node", "--json")
	if err == nil || !strings.Contains(out, "node_offline") || !json.Valid([]byte(out)) {
		t.Fatal("离线节点被当实时健康", out, err)
	}
	for _, args := range [][]string{{"doctor", "--server", "--node", "remote-node"}, {"doctor", "--node", "remote-node", "--working-dir", "."}, {"node", "create", "invalid", "--capacity", "0"}, {"node", "ls", "--limit", "201"}} {
		if _, err := executeRemote(t, append([]string{"--server-url", address}, args...)...); err == nil {
			t.Fatal("非法命令被接受", args)
		}
	}
	for _, action := range []string{"drain", "enable", "disable"} {
		call("node", action, "remote-node")
	}
	call("node", "token", "rotate", "remote-node", "--json")
	call("node", "token", "revoke", "remote-node")
	call("node", "rm", "remote-node")
}

func TestRemoteUnknownActionsAndEmptyNodeDoctorFailBeforeConfig(t *testing.T) {
	t.Setenv("MYBUILDS_CLIENT_CONFIG", t.TempDir()+"/missing.yml")
	for _, args := range [][]string{{"PRIVATE_UNKNOWN"}, {"node", "PRIVATE_UNKNOWN"}, {"node", "token", "PRIVATE_UNKNOWN"}, {"doctor", "--node", ""}} {
		out, e := executeRemote(t, args...)
		if e == nil || strings.Contains(out, "PRIVATE_UNKNOWN") || strings.Contains(e.Error(), "PRIVATE_UNKNOWN") {
			t.Fatal("未知操作被接受或回显", out, e)
		}
	}
}
