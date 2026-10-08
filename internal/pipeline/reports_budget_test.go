package pipeline

import (
	"context"
	"fmt"
	"mybuilds/internal/config"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReportCollectionActiveDeadlineInterruptsRealBaseline(t *testing.T) {
	work, wr, dr := reportRoots(t)
	payload := make([]byte, 1<<20)
	for i := range payload {
		payload[i] = ' '
	}
	for i := 0; i < 64; i++ {
		name := filepath.Join(work, fmt.Sprintf("report-%d.xml", i))
		if err := os.WriteFile(name, payload, 0600); err != nil {
			t.Fatal(err)
		}
	}
	// 实际64MiB基线的读/摘要必须检查更早的普通预算，不另开无限后台读取。
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := newReportCollection(ctx, wr, dr, []string{"*.xml"}, true, nil, config.DefaultJUnitMaxFiles)
	if err == nil || reportFailureReason(err) != "timeout" || time.Since(started) > time.Second {
		t.Fatalf("基线未服从实际期限: %v", err)
	}
}

func TestReportCollectionZeroDeadlineDoesNotOpenSnapshot(t *testing.T) {
	work, wr, dr := reportRoots(t)
	c, err := newReportCollection(context.Background(), wr, dr, []string{"result.xml"}, true, nil, config.DefaultJUnitMaxFiles)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(work, "result.xml"), []byte(`<testsuite><testcase/></testsuite>`), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	_, _, err = c.check(ctx, 1, "actual", false)
	if err == nil || reportFailureReason(err) != "timeout" {
		t.Fatalf("耗尽预算仍开始检查: %v", err)
	}
	entries, err := os.ReadDir(dr.Name())
	if err != nil || len(entries) != 0 {
		t.Fatal("耗尽预算创建了快照")
	}
}
