//go:build darwin || linux

package process

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestRunOnStartReceivesActualGroup(t *testing.T) {
	command := shellTestCommand(t, "sleep 0.05")
	calls := 0
	before := time.Now()
	command.OnStart = func(info StartInfo) error {
		calls++
		if info.PID <= 1 || info.PGID != info.PID || info.At.Before(before) || info.At.Location() != time.UTC {
			t.Errorf("invalid actual start: %+v", info)
		}
		if err := syscall.Kill(info.PID, 0); err != nil {
			t.Errorf("process not started: %v", err)
		}
		group, err := syscall.Getpgid(info.PID)
		if err != nil || group != info.PGID {
			t.Errorf("actual group %d: %v", group, err)
		}
		return nil
	}
	result := Run(context.Background(), command, io.Discard, io.Discard)
	if calls != 1 || !result.Started || result.Reason != "" || result.ExitCode != 0 || result.CleanupFailed {
		t.Fatal(calls, result)
	}
}
func TestRunOnStartFailureWaitsAndReapsActualGroup(t *testing.T) {
	command := shellTestCommand(t, "sleep 60")
	calls, pid := 0, 0
	command.OnStart = func(info StartInfo) error {
		calls++
		pid = info.PID
		return errors.New("PRIVATE_START_CALLBACK_SECRET")
	}
	started := time.Now()
	result := Run(context.Background(), command, io.Discard, io.Discard)
	if calls != 1 || !result.Started || result.Reason != "progress_error" || result.CleanupFailed || time.Since(started) > 3*time.Second {
		t.Fatal(calls, result)
	}
	assertProcessStopped(t, pid)
	// 已由唯一Wait消费直接子进程；再次Wait4不得收获漏掉的孩子。
	var state syscall.WaitStatus
	if _, err := syscall.Wait4(pid, &state, syscall.WNOHANG, nil); !errors.Is(err, syscall.ECHILD) {
		t.Fatalf("unreaped child: %v", err)
	}
}
func TestRunOnStartNotCalledForStartFailure(t *testing.T) {
	command := shellTestCommand(t, "touch PRIVATE_SCRIPT_MARKER")
	command.Path = filepath.Join(command.Dir, "missing")
	calls := 0
	command.OnStart = func(StartInfo) error { calls++; return nil }
	result := Run(context.Background(), command, io.Discard, io.Discard)
	if calls != 0 || result.Started || result.Reason != "start_error" {
		t.Fatal(calls, result)
	}
	if _, err := os.Stat(filepath.Join(command.Dir, "PRIVATE_SCRIPT_MARKER")); !os.IsNotExist(err) {
		t.Fatal(err)
	}
}
