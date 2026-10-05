//go:build darwin || linux

package client

import (
	"context"
	"mybuilds/internal/pipeline"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestApprovalTTYHelper(t *testing.T) {
	if os.Getenv("MYBUILDS_TEST_TTY_HELPER") != "1" {
		return
	}
	ctx, stop := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer stop()
	allowed, err := confirmLocalApproval(os.Stdin, os.Stdout)(ctx, pipeline.ApprovalPrompt{})
	switch os.Getenv("MYBUILDS_TEST_TTY_EXPECT") {
	case "yes":
		if err != nil || !allowed {
			os.Exit(21)
		}
	case "no":
		if err != nil || allowed {
			os.Exit(22)
		}
	case "cancel":
		if err != context.DeadlineExceeded || allowed {
			os.Exit(23)
		}
	}
	os.Exit(0)
}
func TestApprovalTTYActualTerminalAndNoTerminal(t *testing.T) {
	if allowed, err := confirmLocalApproval(nil, os.Stdout)(context.Background(), pipeline.ApprovalPrompt{}); err == nil || allowed {
		t.Fatal("无终端被默认批准")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal("实际PTY检查需要已安装python3")
	}
	script := `import os,pty,subprocess,sys,select,time
master,slave=pty.openpty()
env=os.environ.copy();env['MYBUILDS_TEST_TTY_HELPER']='1';env['MYBUILDS_TEST_TTY_EXPECT']=sys.argv[2]
p=subprocess.Popen([sys.argv[1],'-test.run=^TestApprovalTTYHelper$'],stdin=slave,stdout=slave,stderr=slave,env=env)
os.close(slave)
try:
 ready,_,_=select.select([master],[],[],2)
 if not ready: raise RuntimeError('prompt_timeout')
 os.read(master,4096)
 if sys.argv[2]!='cancel':os.write(master,(sys.argv[2]+'\n').encode())
 code=p.wait(timeout=3)
 if code:raise RuntimeError('tty_failed')
finally:
 if p.poll() is None:p.kill();p.wait()
 os.close(master)
`
	for _, answer := range []string{"yes", "no", "cancel"} {
		t.Run(answer, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := exec.CommandContext(ctx, python, "-c", script, os.Args[0], answer).Run(); err != nil {
				t.Fatal("实际PTY确认失败", err)
			}
		})
	}
}
