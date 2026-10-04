package scm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCheckoutActualPinnedFormatsAndIndependentWorkspaces(t *testing.T) {
	for _, format := range []string{"sha1", "sha256"} {
		t.Run(format, func(t *testing.T) {
			input, sha := gitFixture(t, format)
			if err := os.Chmod(input.DataDir, 0700); err != nil {
				t.Fatal(err)
			}
			writeFixture(t, input.Repository, "mybuilds.yml", "new branch contents\n")
			fixtureGit(t, input.Repository, "add", "mybuilds.yml")
			fixtureGit(t, input.Repository, "commit", "-m", "advance")
			options := CheckoutOptions{DataDir: input.DataDir, Repository: input.Repository, Branch: "main", SHA: sha}
			first, err := Checkout(context.Background(), options)
			if err != nil || !first.StopConfirmed || first.Workspace == "" {
				t.Fatal(first, err)
			}
			second, err := Checkout(context.Background(), options)
			if err != nil || second.Workspace == first.Workspace {
				t.Fatal(second, err)
			}
			for _, result := range []CheckoutResult{first, second} {
				if got := fixtureGit(t, result.Workspace, "rev-parse", "HEAD"); got != sha {
					t.Fatal(got, sha)
				}
				data, err := os.ReadFile(filepath.Join(result.Workspace, "mybuilds.yml"))
				if err != nil || !strings.Contains(string(data), "MUST_NOT_EXECUTE") {
					t.Fatal(err)
				}
				if _, err = os.Stat(filepath.Join(result.Workspace, "MUST_NOT_EXECUTE")); !os.IsNotExist(err) {
					t.Fatal("script executed")
				}
				info, err := os.Stat(result.Workspace)
				if err != nil || info.Mode().Perm() != 0700 {
					t.Fatal("workspace permissions", err)
				}
			}
			requireDifferent := fixtureGit(t, input.Repository, "rev-parse", "HEAD")
			if requireDifferent == sha {
				t.Fatal("fixture did not advance")
			}
			if data, _ := os.ReadFile(filepath.Join(input.Repository, "mybuilds.yml")); string(data) != "new branch contents\n" {
				t.Fatal("source worktree mutated")
			}
		})
	}
}
func TestCheckoutRejectsUnreachableTagsAndInvalidInputs(t *testing.T) {
	input, sha := gitFixture(t, "sha1")
	os.Chmod(input.DataDir, 0700)
	fixtureGit(t, input.Repository, "tag", "-a", "annotated", "-m", "tag", sha)
	tag := fixtureGit(t, input.Repository, "rev-parse", "annotated")
	fixtureGit(t, input.Repository, "checkout", "--orphan", "other")
	fixtureGit(t, input.Repository, "commit", "--allow-empty", "-m", "unreachable")
	other := fixtureGit(t, input.Repository, "rev-parse", "HEAD")
	fixtureGit(t, input.Repository, "checkout", "main")
	for _, bad := range []CheckoutOptions{
		{DataDir: input.DataDir, Repository: input.Repository, Branch: "main", SHA: tag},
		{DataDir: input.DataDir, Repository: input.Repository, Branch: "main", SHA: other},
		{DataDir: input.DataDir, Repository: input.Repository, Branch: "main", SHA: sha[:12]},
		{DataDir: input.DataDir, Repository: input.Repository, Branch: "main", SHA: strings.Repeat("a", 64)},
		{DataDir: input.DataDir, Repository: input.Repository, Branch: "main\nprivate", SHA: sha},
		{DataDir: input.DataDir, Repository: "https://user:PRIVATE_PASSWORD@example.invalid/repo", Branch: "main", SHA: sha},
	} {
		result, err := Checkout(context.Background(), bad)
		if err == nil || result.Workspace != "" || !result.StopConfirmed || strings.Contains(err.Error(), "PRIVATE_PASSWORD") {
			t.Fatal(result, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := Checkout(ctx, CheckoutOptions{DataDir: input.DataDir, Repository: input.Repository, Branch: "main", SHA: sha})
	if err == nil || !result.StopConfirmed || result.Workspace != "" {
		t.Fatal(result, err)
	}
	requireClean(t, input.DataDir)
}

func TestCheckoutActualHTTPAuthorityCancellation(t *testing.T) {
	input, sha := gitFixture(t, "sha1")
	if err := os.Chmod(input.DataDir, 0700); err != nil {
		t.Fatal(err)
	}
	reached := make(chan struct{})
	var once sync.Once
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { once.Do(func() { close(reached) }); <-r.Context().Done() }))
	defer control.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan struct {
		result CheckoutResult
		err    error
	}, 1)
	go func() {
		result, err := Checkout(ctx, CheckoutOptions{DataDir: input.DataDir, Repository: control.URL + "/repo", Branch: "main", SHA: sha})
		done <- struct {
			result CheckoutResult
			err    error
		}{result, err}
	}()
	select {
	case <-reached:
		cancel()
	case result := <-done:
		t.Fatal("未实际访问HTTP", result.err)
	case <-ctx.Done():
		t.Fatal("未访问HTTP")
	}
	select {
	case actual := <-done:
		if actual.err == nil || actual.result.Workspace != "" || !actual.result.StopConfirmed {
			t.Fatal(actual)
		}
		requireCode(t, actual.err, "scm_cancelled")
	case <-time.After(3 * time.Second):
		t.Fatal("取消未结束")
	}
	requireClean(t, input.DataDir)
}
func TestCheckoutIgnoresSourceReplaceAndDoesNotFetchSubmodules(t *testing.T) {
	input, sha := gitFixture(t, "sha1")
	if err := os.Chmod(input.DataDir, 0700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, input.Repository, ".gitmodules", "[submodule \"nested\"]\n path = nested\n url = ext::touch MUST_NOT_EXECUTE_SUBMODULE\n")
	fixtureGit(t, input.Repository, "add", ".gitmodules")
	fixtureGit(t, input.Repository, "update-index", "--add", "--cacheinfo", "160000,"+sha+",nested")
	fixtureGit(t, input.Repository, "commit", "-m", "gitlink")
	selected := fixtureGit(t, input.Repository, "rev-parse", "HEAD")
	writeFixture(t, input.Repository, "mybuilds.yml", "replace must not be used\n")
	fixtureGit(t, input.Repository, "add", "mybuilds.yml")
	fixtureGit(t, input.Repository, "commit", "-m", "replacement")
	replacement := fixtureGit(t, input.Repository, "rev-parse", "HEAD")
	fixtureGit(t, input.Repository, "replace", selected, replacement)
	result, err := Checkout(context.Background(), CheckoutOptions{DataDir: input.DataDir, Repository: input.Repository, Branch: "main", SHA: selected})
	if err != nil || !result.StopConfirmed {
		t.Fatal(result, err)
	}
	data, err := os.ReadFile(filepath.Join(result.Workspace, "mybuilds.yml"))
	if err != nil || !strings.Contains(string(data), "MUST_NOT_EXECUTE") {
		t.Fatal("使用replace对象", err)
	}
	entries, err := os.ReadDir(filepath.Join(result.Workspace, "nested"))
	if err != nil || len(entries) != 0 {
		t.Fatal("submodule执行", err)
	}
}
