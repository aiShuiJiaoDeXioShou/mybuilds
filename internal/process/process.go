package process

import (
	"os"
	"time"
)

// Command 只保存已准备的命令，不自行读取宿主环境或配置。
type Command struct {
	Path    string
	Args    []string
	Dir     string
	Env     []string
	OnStart func(StartInfo) error
}

// StartInfo仅在真实Start成功后供节点持久化启动回执，不参与命令构造。
type StartInfo struct {
	PID, PGID int
	At        time.Time
}

type Result struct {
	Started       bool
	ExitCode      int
	Reason        string
	Duration      time.Duration
	CleanupFailed bool
}

// HostEnvironment 只读取允许继承的系统和工具变量，每次返回独立映射。
func HostEnvironment() map[string]string {
	env := map[string]string{}
	for _, key := range []string{"PATH", "HOME", "TMPDIR", "LANG", "LC_ALL", "JAVA_HOME", "ANDROID_HOME", "ANDROID_SDK_ROOT", "DEVELOPER_DIR"} {
		if value, ok := os.LookupEnv(key); ok {
			env[key] = value
		}
	}
	return env
}
