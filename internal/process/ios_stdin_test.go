//go:build darwin || linux

package process

import (
	"bytes"
	"context"
	"os/exec"
	"testing"
)

func TestIOSHelperBoundedByteStdin(t *testing.T) {
	path, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	result := Run(context.Background(), Command{Path: path, Args: []string{"-c", "read value; test \"$value\" = sensitive_stdin"}, Dir: t.TempDir(), Env: []string{}, Stdin: []byte("sensitive_stdin\n")}, &output, &output)
	if !result.Started || result.ExitCode != 0 || result.Reason != "" || result.CleanupFailed || output.Len() != 0 {
		t.Fatal("有限匿名stdin未被真实执行器消费", result.Reason)
	}
}
