package scm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

func fixtureGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "-C", dir}, args...)...)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "LANG=C", "GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid", "GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid"}
	data, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("夹具git失败 %v: %v %s", args, err, data)
	}
	return strings.TrimSpace(string(data))
}
func gitFixture(t *testing.T, format string) (Options, string) {
	t.Helper()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("控制端仅支持macOS/Linux")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatal(err)
	}
	repo := t.TempDir()
	fixtureGit(t, repo, "init", "--object-format="+format, "--initial-branch=main", "--template=")
	writeFixture(t, repo, "mybuilds.yml", "version: 1\nsteps: [{kind: run, run: 'touch MUST_NOT_EXECUTE'}]\n")
	fixtureGit(t, repo, "add", "mybuilds.yml")
	fixtureGit(t, repo, "commit", "-m", "first")
	return Options{DataDir: t.TempDir(), Repository: repo, Branch: "main", File: "mybuilds.yml"}, fixtureGit(t, repo, "rev-parse", "HEAD")
}
func writeFixture(t *testing.T, dir, name, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}
func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	var failure *Error
	if !errors.As(err, &failure) || failure.Code != code {
		t.Fatalf("需要安全错误%s，得到%v", code, err)
	}
}
func requireClean(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, "scm"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("残留请求目录: %v", entries)
	}
}

func TestReadPipelineRealGitPinnedFormatsAndDirtyWorktree(t *testing.T) {
	for _, format := range []string{"sha1", "sha256"} {
		t.Run(format, func(t *testing.T) {
			options, first := gitFixture(t, format)
			old, _ := os.ReadFile(filepath.Join(options.Repository, options.File))
			fixtureGit(t, options.Repository, "branch", "refs/heads/main")
			writeFixture(t, options.Repository, options.File, string(old)+"# second\n")
			fixtureGit(t, options.Repository, "add", options.File)
			fixtureGit(t, options.Repository, "commit", "-m", "advance")
			second := fixtureGit(t, options.Repository, "rev-parse", "HEAD")
			writeFixture(t, options.Repository, options.File, "DIRTY_PRIVATE_CONFIG_MARKER")
			status := fixtureGit(t, options.Repository, "status", "--porcelain")
			index, _ := os.ReadFile(filepath.Join(options.Repository, ".git/index"))
			options.Ref = first
			snap, err := ReadPipeline(context.Background(), options)
			if err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256(old)
			if snap.SHA != first || string(snap.Content) != string(old) || snap.Digest != hex.EncodeToString(digest[:]) {
				t.Fatalf("固定快照错误: %s", snap.SHA)
			}
			options.Ref = ""
			latest, err := ReadPipeline(context.Background(), options)
			if err != nil || latest.SHA != second || !strings.Contains(string(latest.Content), "second") {
				t.Fatalf("分支HEAD错误: %v", err)
			}
			after, _ := os.ReadFile(filepath.Join(options.Repository, ".git/index"))
			dirty, _ := os.ReadFile(filepath.Join(options.Repository, options.File))
			if string(after) != string(index) || fixtureGit(t, options.Repository, "status", "--porcelain") != status || string(dirty) != "DIRTY_PRIVATE_CONFIG_MARKER" {
				t.Fatal("修改了源工作树或index")
			}
			if _, err := os.Stat(filepath.Join(options.Repository, "MUST_NOT_EXECUTE")); !os.IsNotExist(err) {
				t.Fatal("执行了仓库脚本")
			}
			requireClean(t, options.DataDir)
		})
	}
}
func TestReadPipelineRefAndFileBoundaries(t *testing.T) {
	options, first := gitFixture(t, "sha1")
	fixtureGit(t, options.Repository, "checkout", "-b", "other")
	writeFixture(t, options.Repository, "other", "only other")
	fixtureGit(t, options.Repository, "add", "other")
	fixtureGit(t, options.Repository, "commit", "-m", "other")
	other := fixtureGit(t, options.Repository, "rev-parse", "HEAD")
	fixtureGit(t, options.Repository, "checkout", "main")
	fixtureGit(t, options.Repository, "tag", "-a", "release", "-m", "tag")
	tag := fixtureGit(t, options.Repository, "rev-parse", "refs/tags/release")
	for _, ref := range []string{other, tag, first[:12], "release", "HEAD~1"} {
		t.Run("ref_"+ref[:min(len(ref), 12)], func(t *testing.T) {
			o := options
			o.Ref = ref
			_, err := ReadPipeline(context.Background(), o)
			if err == nil {
				t.Fatal("接受不可达或非commit完整ref")
			}
			requireClean(t, o.DataDir)
		})
	}
	if err := os.Symlink(options.File, filepath.Join(options.Repository, "link.yml")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(options.Repository, "directory"), 0700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, options.Repository, "directory/file", "inside")
	writeFixture(t, options.Repository, "large.yml", strings.Repeat("x", (1<<20)+1))
	fixtureGit(t, options.Repository, "add", "link.yml", "directory", "large.yml")
	fixtureGit(t, options.Repository, "update-index", "--add", "--cacheinfo", "160000,"+first+",gitlink")
	fixtureGit(t, options.Repository, "commit", "-m", "boundaries")
	for _, tc := range []struct{ file, code string }{{"link.yml", "scm_config_not_regular"}, {"directory", "scm_config_not_regular"}, {"gitlink", "scm_config_not_regular"}, {"large.yml", "scm_config_limit"}, {"absent.yml", "scm_config_missing"}, {"../mybuilds.yml", "scm_input_invalid"}, {"/mybuilds.yml", "scm_input_invalid"}, {"a/../mybuilds.yml", "scm_input_invalid"}, {"mybuilds*.yml", "scm_input_invalid"}} {
		t.Run(tc.file, func(t *testing.T) {
			o := options
			o.File = tc.file
			_, err := ReadPipeline(context.Background(), o)
			requireCode(t, err, tc.code)
			requireClean(t, o.DataDir)
		})
	}
}
func TestReadPipelineRejectsUnsafeInputAndCancellation(t *testing.T) {
	options, _ := gitFixture(t, "sha1")
	for _, repo := range []string{"--upload-pack=PRIVATE_MARKER", "ext::sh PRIVATE_MARKER", "https://user:PRIVATE_MARKER@example.invalid/repo", "https://example.invalid/repo?token=PRIVATE_MARKER", "http://example.invalid/repo", "https://example.invalid/repo\nPRIVATE_MARKER"} {
		o := options
		o.Repository = repo
		_, err := ReadPipeline(context.Background(), o)
		requireCode(t, err, "scm_input_invalid")
		if strings.Contains(err.Error(), "PRIVATE_MARKER") {
			t.Fatal("错误回显输入")
		}
	}
	for _, branch := range []string{"../main", "main*", "main:other", "main@{1}", "main\nPRIVATE_MARKER"} {
		o := options
		o.Branch = branch
		_, err := ReadPipeline(context.Background(), o)
		if err == nil || strings.Contains(err.Error(), "PRIVATE_MARKER") {
			t.Fatal("非法分支未安全拒绝")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ReadPipeline(ctx, options)
	requireCode(t, err, "scm_cancelled")
	requireClean(t, options.DataDir)
}

func TestReadPipelineRejectParentSymlinkAndAmbientExecution(t *testing.T) {
	options, _ := gitFixture(t, "sha1")
	outside := t.TempDir()
	writeFixture(t, outside, "mybuilds.yml", "PRIVATE_OUTSIDE_MARKER")
	if err := os.Symlink(outside, filepath.Join(options.Repository, "linked")); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, options.Repository, "add", "linked")
	fixtureGit(t, options.Repository, "commit", "-m", "link")
	options.File = "linked/mybuilds.yml"
	_, err := ReadPipeline(context.Background(), options)
	if err == nil {
		t.Fatal("读取了符号链接父目录")
	}
	requireClean(t, options.DataDir)
	options.File = "mybuilds.yml"
	hookdir := t.TempDir()
	marker := filepath.Join(hookdir, "HOOK_MUST_NOT_EXECUTE")
	for _, hook := range []string{"reference-transaction", "post-checkout", "post-merge"} {
		if err := os.WriteFile(filepath.Join(hookdir, hook), []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	ambient := filepath.Join(hookdir, "config")
	writeFixture(t, hookdir, "config", "[core]\n hooksPath = "+hookdir+"\n[filter \"danger\"]\n smudge = touch '"+marker+"'\n[credential]\n helper = !touch '"+marker+"'\n[init]\n templateDir = "+hookdir+"\n")
	t.Setenv("GIT_CONFIG_GLOBAL", ambient)
	t.Setenv("GIT_CONFIG_SYSTEM", ambient)
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.hooksPath")
	t.Setenv("GIT_CONFIG_VALUE_0", hookdir)
	t.Setenv("GIT_TEMPLATE_DIR", hookdir)
	writeFixture(t, options.Repository, ".gitattributes", "mybuilds.yml filter=danger\n")
	fixtureGit(t, options.Repository, "add", ".gitattributes")
	fixtureGit(t, options.Repository, "commit", "-m", "filter")
	snapshot, err := ReadPipeline(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(snapshot.Content), "MUST_NOT_EXECUTE") {
		t.Fatal("原始blob被转换")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("执行了hook/filter/helper/template")
	}
	requireClean(t, options.DataDir)
}
func TestReadPipelineAnonymousTLSDoesNotDisableCertificateVerification(t *testing.T) {
	options, _ := gitFixture(t, "sha1")
	tls := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("未信任TLS不应读取HTTP") }))
	defer tls.Close()
	options.Repository = tls.URL + "/repo"
	t.Setenv("GIT_SSL_NO_VERIFY", "1")
	t.Setenv("GIT_HTTP_EXTRA_HEADER", "Authorization: PRIVATE_MARKER")
	_, err := ReadPipeline(context.Background(), options)
	requireCode(t, err, "scm_git_failed")
	requireClean(t, options.DataDir)
}

func TestReadPipelinePinsActualFetchedSHAWhileBranchAdvances(t *testing.T) {
	options, first := gitFixture(t, "sha1")
	original, err := os.ReadFile(filepath.Join(options.Repository, options.File))
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, options.Repository, options.File, string(original)+"# advanced branch\n")
	fixtureGit(t, options.Repository, "add", options.File)
	fixtureGit(t, options.Repository, "commit", "-m", "advance")
	second := fixtureGit(t, options.Repository, "rev-parse", "HEAD")
	fixtureGit(t, options.Repository, "update-ref", "refs/heads/main", first)
	fixtureGit(t, options.Repository, "update-server-info")
	gitdir := filepath.Join(options.Repository, ".git")
	refs, err := os.ReadFile(filepath.Join(gitdir, "info/refs"))
	if err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	advanceError := make(chan error, 1)
	files := http.FileServer(http.Dir(gitdir))
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/info/refs" {
			count := requests.Add(1)
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write(refs)
			if count == 2 {
				// 返回fetch的固定引用后推进真实源分支，对象仍可读取，但后续HEAD已不同。
				command := exec.Command("git", "-c", "core.hooksPath=/dev/null", "--git-dir="+gitdir, "update-ref", "refs/heads/main", second)
				command.Env = []string{"PATH=" + os.Getenv("PATH"), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null"}
				advanceError <- command.Run()
			}
			return
		}
		files.ServeHTTP(w, r)
	}))
	defer service.Close()
	options.Repository = service.URL
	snapshot, err := ReadPipeline(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() < 2 {
		t.Fatal("未进行独立真实探测和fetch")
	}
	if err := <-advanceError; err != nil {
		t.Fatal(err)
	}
	if snapshot.SHA != first || string(snapshot.Content) != string(original) || fixtureGit(t, gitdir, "rev-parse", "refs/heads/main") != second {
		t.Fatalf("未固定本次fetch提交: %s", snapshot.SHA)
	}
	requireClean(t, options.DataDir)
}

func TestReadPipelineRejectNonCommitBranchEvenWithCommitRef(t *testing.T) {
	options, first := gitFixture(t, "sha1")
	fixtureGit(t, options.Repository, "tag", "-a", "tagged", "-m", "tagged")
	tag := fixtureGit(t, options.Repository, "rev-parse", "refs/tags/tagged")
	// 自有夹具直接构造非法heads对象；不能依靠Git默认剥tag推断分支类型。
	writeFixture(t, options.Repository, ".git/refs/heads/main", tag+"\n")
	options.Ref = first
	_, err := ReadPipeline(context.Background(), options)
	if err == nil {
		t.Fatal("非commit分支被剥tag并接受")
	}
	requireClean(t, options.DataDir)
}
