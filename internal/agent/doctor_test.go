//go:build darwin || linux

package agent

import (
	"context"
	"fmt"
	"mybuilds/internal/mobile"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == "__ios-signing" {
		if mobile.HandleIOSHelper(os.Stdin, os.Stdout) != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	if len(os.Args) == 3 && os.Args[1] == "--agent-doctor-helper" {
		switch os.Args[2] {
		case "overflow":
			fmt.Print(strings.Repeat("private-tool-output", 4000))
		case "environment":
			fmt.Print(os.Getenv("MYBUILDS_AGENT_TOKEN"))
		case "failure":
			fmt.Print("private-tool-output")
			os.Exit(2)
		}
		os.Exit(0)
	}
	if len(os.Args) == 3 && os.Args[1] == "--agent-data-lock" {
		lock, err := lockDataDir(os.Args[2])
		if err != nil {
			fmt.Print(err)
			os.Exit(3)
		}
		lock.Close()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestDoctorDoesNotCreateDataOrReadToken(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "not-created")
	t.Setenv("JAVA_HOME", filepath.Join(root, "missing-jdk"))
	t.Setenv("ANDROID_HOME", filepath.Join(root, "missing-sdk"))
	t.Setenv("ANDROID_SDK_ROOT", "")
	t.Setenv("MYBUILDS_AGENT_TOKEN", "private-agent-token")
	report, err := Doctor(context.Background(), missing)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(missing); !os.IsNotExist(err) {
		t.Fatalf("doctor created directory: %v", err)
	}
	statuses := map[string]string{}
	reasons := map[string]string{}
	for _, tool := range report.Tools {
		statuses[tool.Name] = tool.Status
		reasons[tool.Name] = tool.Reason
	}
	if statuses["shell"] != "passed" || statuses["git"] != "passed" {
		t.Fatalf("actual shell/git: %+v", report)
	}
	if statuses["java"] == "passed" || statuses["android_aapt2"] == "passed" || statuses["android_apksigner"] == "passed" {
		t.Fatalf("missing tools passed: %+v", report)
	}
	if mobile.IOSSigningSupported() && statuses["xcode"] == "passed" {
		if statuses["ios_signing"] != "passed" || reasons["ios_signing"] != "" {
			t.Fatal("可用原生组件未报告", report)
		}
	} else if statuses["ios_signing"] != "skipped" || reasons["ios_signing"] != "unsupported" {
		t.Fatal("未支持宿主虚报原生能力", report)
	}
	if statuses["node_journal"] != "skipped" || reasons["node_journal"] != "uninitialized" {
		t.Fatal(report)
	}
}

func TestToolOutputActualFailureBoundsAndEnvironment(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("MYBUILDS_AGENT_TOKEN", "private-agent-token")
	for _, tc := range []struct{ arg, reason string }{{"failure", "tool_error"}, {"overflow", "output_limit"}, {"environment", ""}} {
		out, reason := toolOutput(context.Background(), executable, []string{"--agent-doctor-helper", tc.arg}, t.TempDir())
		if reason != tc.reason {
			t.Fatalf("%s reason=%s", tc.arg, reason)
		}
		if out != "" {
			t.Fatalf("raw tool output leaked: %q", out)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, reason := toolOutput(ctx, executable, nil, t.TempDir()); reason != "cancelled" {
		t.Fatal(reason)
	}
}

func TestDoctorDoesNotPassInvalidSDKExecutable(t *testing.T) {
	root := t.TempDir()
	sdk := filepath.Join(root, "sdk")
	tools := filepath.Join(sdk, "build-tools", "35.0.0")
	if err := os.MkdirAll(tools, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tools, "aapt2"), []byte("not executable binary"), 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "unexpected-signer-call")
	script := "#!/bin/sh\n: > '" + strings.ReplaceAll(marker, "'", "'\\''") + "'\nexit 2\n"
	if err := os.WriteFile(filepath.Join(tools, "apksigner"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANDROID_HOME", sdk)
	t.Setenv("ANDROID_SDK_ROOT", "")
	report, err := Doctor(context.Background(), filepath.Join(root, "uninitialized"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(marker); !os.IsNotExist(err) {
		t.Fatal("continued diagnostics after tool error")
	}
	for _, check := range report.Tools {
		if (check.Name == "android_aapt2" || check.Name == "android_apksigner") && check.Status == "passed" {
			t.Fatal(check)
		}
	}
}

func TestDoctorCancelledIsBounded(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	report, err := Doctor(ctx, filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("cancel accepted", report)
	}
	if time.Since(start) > time.Second {
		t.Fatal("cancel blocked")
	}
}
