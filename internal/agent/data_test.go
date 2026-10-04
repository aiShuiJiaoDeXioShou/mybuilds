//go:build darwin || linux

package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestDataLockActualCrossProcessAndReplacement(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "private")
	lock, err := lockDataDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	executable, _ := os.Executable()
	command := exec.Command(executable, "--agent-data-lock", directory)
	output, err := command.CombinedOutput()
	if err == nil || string(output) != "agent_data_locked" {
		t.Fatalf("second owner: %q %v", output, err)
	}
	if err := lock.Check(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(directory, "agent.lock"), filepath.Join(directory, "old.lock")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "agent.lock"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := lock.Check(); err == nil {
		t.Fatal("replaced lock retained authority")
	}
}

func TestDataPrivateBoundaryAndFIFO(t *testing.T) {
	for _, kind := range []string{"weak-dir", "symlink-dir", "fifo-lock", "symlink-lock", "weak-lock"} {
		t.Run(kind, func(t *testing.T) {
			parent := t.TempDir()
			dir := filepath.Join(parent, "data")
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "weak-dir":
				os.Chmod(dir, 0755)
			case "symlink-dir":
				target := dir
				dir = filepath.Join(parent, "link")
				os.Symlink(target, dir)
			case "fifo-lock":
				syscall.Mkfifo(filepath.Join(dir, "agent.lock"), 0600)
			case "symlink-lock":
				os.WriteFile(filepath.Join(parent, "secret"), nil, 0600)
				os.Symlink(filepath.Join(parent, "secret"), filepath.Join(dir, "agent.lock"))
			case "weak-lock":
				os.WriteFile(filepath.Join(dir, "agent.lock"), nil, 0644)
			}
			start := time.Now()
			lock, err := lockDataDir(dir)
			if lock != nil {
				lock.Close()
			}
			if err == nil || !strings.HasPrefix(err.Error(), "agent_") {
				t.Fatalf("accepted %s: %v", kind, err)
			}
			if time.Since(start) > time.Second {
				t.Fatal("special file blocked")
			}
		})
	}
}

func TestUnconfirmedJournalDoesNotKillStoredPID(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	journal := filepath.Join(dir, "journal")
	if err := os.Mkdir(journal, 0700); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("sleep", "60")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { command.Process.Kill(); command.Wait() }()
	if err := os.WriteFile(filepath.Join(journal, "old.json"), []byte(`{"pid":`+fmtPID(command.Process.Pid)+`}`), 0600); err != nil {
		t.Fatal(err)
	}
	report, err := Doctor(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, check := range report.Tools {
		if check.Name == "node_journal" {
			found = check.Status == "failed" && check.Reason == "journal_unconfirmed"
		}
	}
	if !found {
		t.Fatal(report)
	}
	if err := syscall.Kill(command.Process.Pid, 0); err != nil {
		t.Fatalf("doctor killed unrelated PID: %v", err)
	}
}
func fmtPID(pid int) string { return fmt.Sprintf("%d", pid) }

func TestDoctorJournalAndSDKSpecialFilesAreBounded(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "journal"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(dir, "journal", "pending.json"), 0600); err != nil {
		t.Fatal(err)
	}
	sdk := filepath.Join(dir, "sdk")
	os.Mkdir(sdk, 0700)
	syscall.Mkfifo(filepath.Join(sdk, "build-tools"), 0600)
	t.Setenv("ANDROID_HOME", sdk)
	t.Setenv("ANDROID_SDK_ROOT", "")
	t.Setenv("JAVA_HOME", filepath.Join(dir, "missing"))
	start := time.Now()
	report, err := Doctor(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("FIFO blocked doctor")
	}
	if report.Tools[0].Reason != "data_invalid" {
		t.Fatal(report.Tools[0])
	}
	for _, check := range report.Tools {
		if strings.HasPrefix(check.Name, "android_") && check.Status == "passed" {
			t.Fatal(check)
		}
	}
}

func TestEmptyPrivateJournalPasses(t *testing.T) {
	dir := t.TempDir()
	os.Chmod(dir, 0700)
	os.Mkdir(filepath.Join(dir, "journal"), 0700)
	if reason := inspectData(dir); reason != "" {
		t.Fatal(reason)
	}
}
