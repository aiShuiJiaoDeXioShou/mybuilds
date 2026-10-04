package pipeline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReportCollectionFileConsumersPreserveActualDeadline(t *testing.T) {
	work, wr, dr := reportRoots(t)
	c, err := newReportCollection(context.Background(), wr, dr, []string{"result.xml"}, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(work, "result.xml"), []byte(`<testsuite><testcase/></testsuite>`), 0600); err != nil {
		t.Fatal(err)
	}
	_, locals := reportCheck(t, c, 1, false)
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	// 上一文件/目录枚举已经成功并不赋予下一有限read新的时间窗口。
	if _, _, err = c.read(ctx, "result.xml"); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("实际source期限误归为业务非法: %v", err)
	}
	if err = c.verifySnapshot(ctx, locals[0]); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("实际snapshot期限误归为保存错误: %v", err)
	}
}
