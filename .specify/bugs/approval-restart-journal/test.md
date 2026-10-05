# 审批重启缺陷验证

- 日期：2026-10-05
- 结果：verified（本次具体缺陷）
- 评估：assessment.md；修复：fix.md

## 真实红门

- 原真实HTTP审批重启：22.71/22.07秒均失败 agent_journal_unconfirmed。原journal只有一份且已confirmed，却新增PendingStop；并非第二未知Claim。
- ACK回调内立即cancel：真实Run返回批次失败，而非ErrApprovalPaused。
- 原019 extra_junit：额外未归属XML被错误接受。

## 修后检查

| 检查 | 命令 | 结果 |
|---|---|---|
| 确定性ACK取消、恢复不重跑、替换快照拒绝 | go test -run '^TestApproval(Acknowledged\|Remote\|Restored)' -count=1 -v ./internal/pipeline | PASS 0.976s |
| 原真实HTTP审批退出再新Agent、完整报告canonical五负例 | go test -run '^(TestServeApprovalActualPauseHTTPResumeReports\|TestReportCheckpointTerminalRequiresCanonicalAndConfirmedMetadata)$' -count=1 -v ./internal/agent | PASS 25.789s |
| 最终Agent源码：真实重启、原019完整负例、历史XML精确资格 | go test -race -run '^(TestServeApprovalActualPauseHTTPResumeReports\|TestReportCheckpointTerminalRequiresCanonicalAndConfirmedMetadata\|TestApprovalReportHistoryOnlyAcceptsCheckpointFiles)$' -count=1 -v ./internal/agent | PASS 27.498s |
| Run相关race | go test -race -run '^TestApproval(Acknowledged\|Remote\|Restored)' -count=1 ./internal/pipeline | PASS 1.624s |
| 两包静态检查 | go vet ./internal/agent ./internal/pipeline | exit0 |
| WindowsARM64实际交叉编译 | GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build ./cmd/mybuilds-agent | exit0；仅编译，不宣运行支持 |

原失败日志保留，日志不包含journal原文、令牌、env或密码。新测试在旧Agent真正退出后核验原checkpoint仍严格可读，再保留20.1秒原session换届窗口。确认同Node、同attempt、新epoch、原工作区不重复checkout/脚本，最终真实报告sealed。

## 边界

主代理最终合并全MVP检查另行执行，本次没有重复全suite/操作线上Agent。Windows保持unsupported。合法Apple/真实商店发布仍依原人工门。
