package scm

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSSHCredentialsPrivateCopiesAndBoundaries(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("受限材料读取只支持macOS/Linux")
	}
	source := t.TempDir()
	workspace := t.TempDir()
	writeFixture(t, source, "key", "PRIVATE_KEY_MARKER")
	writeFixture(t, source, "known_hosts", "explicit public host")
	writeFixture(t, source, "secrets.env", "GIT_SSH_KEY_FILE=key\nGIT_SSH_KNOWN_HOSTS_FILE=known_hosts\nOTHER_BUSINESS_SECRET=DO_NOT_EXPORT\n")
	env, err := sshEnvironment(workspace, filepath.Join(source, "secrets.env"))
	if err != nil {
		t.Fatal(err)
	}
	if len(env) != 2 || env["GIT_SSH_VARIANT"] != "ssh" || strings.Contains(env["GIT_SSH_COMMAND"], source) || strings.Contains(env["GIT_SSH_COMMAND"], "PRIVATE_KEY_MARKER") || strings.Contains(env["GIT_SSH_COMMAND"], "DO_NOT_EXPORT") {
		t.Fatal("凭据路径、内容或宿主环境进入SSH命令")
	}
	copy, err := os.ReadFile(filepath.Join(workspace, "GIT_SSH_KEY_FILE"))
	if err != nil || string(copy) != "PRIVATE_KEY_MARKER" {
		t.Fatal("没有真实私有副本")
	}
	writeFixture(t, source, "key", "CHANGED_AFTER_COPY")
	copy, _ = os.ReadFile(filepath.Join(workspace, "GIT_SSH_KEY_FILE"))
	if string(copy) != "PRIVATE_KEY_MARKER" {
		t.Fatal("副本依赖后续源变化")
	}
	for _, text := range []string{"GIT_SSH_KEY_FILE=key\n", "GIT_SSH_KEY_FILE=key\nGIT_SSH_KEY_FILE=key\nGIT_SSH_KNOWN_HOSTS_FILE=known_hosts\n", "GIT_SSH_COMMAND=PRIVATE_COMMAND\n", "bad-line", "GIT_SSH_KEY_FILE=absent\nGIT_SSH_KNOWN_HOSTS_FILE=known_hosts\n"} {
		writeFixture(t, source, "bad.env", text)
		_, err := sshEnvironment(t.TempDir(), filepath.Join(source, "bad.env"))
		requireCode(t, err, "scm_credentials_invalid")
	}
	for _, mode := range []os.FileMode{0644, 0666} {
		if err := os.Chmod(filepath.Join(source, "key"), mode); err != nil {
			t.Fatal(err)
		}
		_, err := sshEnvironment(t.TempDir(), filepath.Join(source, "secrets.env"))
		requireCode(t, err, "scm_credentials_invalid")
	}
	os.Chmod(filepath.Join(source, "key"), 0600)
	if err := os.Symlink("key", filepath.Join(source, "key.link")); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, source, "bad.env", "GIT_SSH_KEY_FILE=key.link\nGIT_SSH_KNOWN_HOSTS_FILE=known_hosts\n")
	_, err = sshEnvironment(t.TempDir(), filepath.Join(source, "bad.env"))
	requireCode(t, err, "scm_credentials_invalid")
	_, err = readPrivate(source, 1<<20)
	requireCode(t, err, "scm_credentials_invalid")
	writeFixture(t, source, "large", strings.Repeat("x", (1<<20)+1))
	_, err = readPrivate(filepath.Join(source, "large"), 1<<20)
	requireCode(t, err, "scm_credentials_invalid")
}

func TestGitRunnerActualDeadlineCancellationAndOutputLimit(t *testing.T) {
	options, _ := gitFixture(t, "sha1")
	for _, kind := range []string{"timeout", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			reached := make(chan struct{})
			var once sync.Once
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { once.Do(func() { close(reached) }); <-r.Context().Done() })}
			go server.Serve(listener)
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			if kind == "cancelled" {
				ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				go func() {
					select {
					case <-reached:
						cancel()
					case <-ctx.Done():
					}
				}()
			}
			o := options
			o.Repository = "http://" + listener.Addr().String()
			_, err = ReadPipeline(ctx, o)
			requireCode(t, err, "scm_"+kind)
			requireClean(t, o.DataDir)
			select {
			case <-reached:
			default:
				t.Fatal("未实际访问自有HTTP服务")
			}
		})
	}
	git, _ := exec.LookPath("git")
	runner := gitRunner{path: git, dir: options.Repository, env: []string{"PATH=" + os.Getenv("PATH"), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null"}, prefix: []string{"-c", "core.hooksPath=/dev/null"}}
	_, _, err := runner.run(context.Background(), 4096, "for-each-ref", "--format="+strings.Repeat("x", 10000))
	requireCode(t, err, "scm_output_limit")
	if runner.cleanupFailed {
		t.Fatal("输出限额没有确认清理")
	}
}

// 自有sshd只监听loopback，临时密钥/hostkey不借宿主agent/default identity。
func TestReadPipelineActualSSHAuthentication(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("SSH控制端只支持macOS/Linux")
	}
	daemon, err := exec.LookPath("sshd")
	if err != nil {
		if _, e := os.Stat("/usr/sbin/sshd"); e != nil {
			t.Skip("本机未安装自有SSH夹具所需sshd")
		}
		daemon = "/usr/sbin/sshd"
	}
	keygen, err := exec.LookPath("ssh-keygen")
	if err != nil {
		t.Skip("未安装ssh-keygen")
	}
	options, sha := gitFixture(t, "sha1")
	directory := t.TempDir()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"client", "host", "wrong"} {
		cmd := exec.Command(keygen, "-t", "ed25519", "-N", "", "-f", filepath.Join(directory, name))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("生成自有SSH key失败: %v %s", err, out)
		}
	}
	public, err := os.ReadFile(filepath.Join(directory, "client.pub"))
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, directory, "authorized_keys", string(public))
	current, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	// StrictModes=no仅服务端/tmp测试夹具；客户端始终严格检查hostkey。
	conf := "Port " + strconv.Itoa(port) + "\nListenAddress 127.0.0.1\nHostKey " + filepath.Join(directory, "host") + "\nPidFile " + filepath.Join(directory, "sshd.pid") + "\nAuthorizedKeysFile " + filepath.Join(directory, "authorized_keys") + "\nAllowUsers " + current.Username + "\nPubkeyAuthentication yes\nAuthenticationMethods publickey\nPasswordAuthentication no\nKbdInteractiveAuthentication no\nUsePAM no\nStrictModes no\nPermitUserRC no\nPermitRootLogin yes\nAllowAgentForwarding no\nAllowTcpForwarding no\nLogLevel ERROR\nForceCommand " + git + " -c core.hooksPath=/dev/null upload-pack " + options.Repository + "\n"
	writeFixture(t, directory, "sshd.conf", conf)
	log, err := os.Create(filepath.Join(directory, "sshd.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	server := exec.Command(daemon, "-D", "-e", "-f", filepath.Join(directory, "sshd.conf"))
	server.Stdout = log
	server.Stderr = log
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { server.Process.Kill(); server.Wait() }()
	ready := false
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		connection, e := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), 50*time.Millisecond)
		if e == nil {
			connection.Close()
			ready = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ready {
		data, _ := os.ReadFile(log.Name())
		t.Fatalf("自有SSH夹具未启动: %s", data)
	}
	known := func(name string) string {
		data, err := os.ReadFile(filepath.Join(directory, name+".pub"))
		if err != nil {
			t.Fatal(err)
		}
		fields := strings.Fields(string(data))
		return "[127.0.0.1]:" + strconv.Itoa(port) + " " + fields[0] + " " + fields[1] + "\n"
	}
	writeFixture(t, directory, "known_hosts", known("host"))
	writeFixture(t, directory, "secrets.env", "GIT_SSH_KEY_FILE=client\nGIT_SSH_KNOWN_HOSTS_FILE=known_hosts\n")
	options.Repository = "ssh://" + current.Username + "@127.0.0.1:" + strconv.Itoa(port) + options.Repository
	options.SecretsFile = filepath.Join(directory, "secrets.env")
	snapshot, err := ReadPipeline(context.Background(), options)
	if err != nil || snapshot.SHA != sha {
		t.Fatalf("真实显式SSH认证失败: %v", err)
	}
	requireClean(t, options.DataDir)
	writeFixture(t, directory, "known_hosts", known("wrong"))
	_, err = ReadPipeline(context.Background(), options)
	if err == nil {
		t.Fatal("错误known_hosts通过")
	}
	requireClean(t, options.DataDir)
	writeFixture(t, directory, "known_hosts", known("host"))
	writeFixture(t, directory, "secrets.env", "GIT_SSH_KEY_FILE=wrong\nGIT_SSH_KNOWN_HOSTS_FILE=known_hosts\n")
	_, err = ReadPipeline(context.Background(), options)
	if err == nil {
		t.Fatal("错误key通过")
	}
	requireClean(t, options.DataDir)
}
