# Bug Fix：成功进程与慢日志排空分别等待

- **Slug**: slow-log-pipe-drain
- **Fixed**: 2026-10-04T13:16:51Z
- **Assessment**: ./assessment.md
- **Status**: applied

## Summary

同一个 process.Run 自持 stdout/stderr 两条 OS 管道，真实子进程只 Wait 一次。leader 退出后立即按原规则回收本组，日志按真实 EOF 完整排空，不再用 WaitDelay 把同步 writer 积压误判成进程 exit。

## Changes

| File | Change | Notes |
|------|--------|-------|
| `internal/process/process_unix.go` | modified | 去掉 WaitDelay，Start 前检查管道读取期限支持；两条复制循环独立处理，读取 idle 不包含 writer 时间；原取消、日志写失败、本组 TERM/KILL 与安全原因保留；清理结果使用原子标记避免并发写结果；本次 Run 局部 mutex 只串行实际 Write，保留同一 writer 的原安全行为。 |
| `internal/process/pipe_test.go` | added test | 真子进程与二进制双流完整内容、慢 writer、真实持写端 idle、后台回收先于 writer 返回、无关 PID 保留、同一 bytes.Buffer 双流合写。 |
| `internal/process/process_unix_test.go` | modified | 两条 leader-exits 用例等待实际 ready 文件，避免把旧 WaitDelay 启动窗口作为能力要求。 |

## Tests Added or Updated

- `TestRunSlowWriterPreservesBothPipes`：16KiB 双流加各 800ms 写延迟，修复前真实 ExitCode=0/Reason=exit，修复后成功；256KiB 双流逐字节完整。
- `TestDrainPipeIdleDeadlineExcludesWriterTime`：真实 OS pipe 缓冲完整写入，800ms writer 后仍有独立 500ms 读取 idle；持写端不 EOF 时明确失败而非假完成。
- `TestRunReapsBackgroundBeforeSlowWriterReturns`：leader 退出、后台忽略 TERM 时真实组回收先于被阻塞 writer 放行；无关 sleep PID 仍活，最终日志完整且成功。
- `TestRunSharedWriterSerializesBothPipes`：真实子进程两条流并行写入，合用同一 bytes.Buffer；未锁版本实际 race 且预期512KiB只得到425984字节，锁定后 race 与完整内容通过。
- 旧取消/正常退出 leader-exits 用例改为实际 ready 同步；只有取消前物理进程已经消失且 ExitCode=0 才容许先完成，否则仍严格 cancelled；保留真正运行中的取消强断言。

## Local Verification

- 修复前 `go test ./internal/process -run '^TestRunSlowWriterPreservesBothPipes$' -count=1 -v` → FAIL：真实 `{Started:true ExitCode:0 Reason:exit Duration:806.26475ms CleanupFailed:false}`。
- 最终修复后新 4 行为测试 `go test -race ./internal/process -run 'TestRunSharedWriter|TestRunSlowWriter|TestDrainPipe|TestRunReapsBackground' -count=1 -v` → PASS，9.688s（含测试构建时间）。
- `go test ./internal/process -count=1` → PASS，12.103s。
- `go test ./...` → PASS，process13.028s、pipeline4.109s、scm7.407s，所有实际 006 包通过。
- `go test -race ./internal/process` → PASS，16.745s。
- `go vet ./...`、`git diff --check` → PASS。
- 同一修复已移植至独立未验收 007 工作树，保留实际 OnStart 回执/失败优先原因和唯一 Wait；`go test ./internal/agent ./internal/pipeline ./internal/process ./internal/scm -count=1` → PASS，agent45.058s、pipeline5.703s、process14.476s、scm9.647s。最终4包race → PASS，agent51.936s、pipeline8.547s、process18.288s、scm10.495s；同4包vet PASS。007 文件不纳入本缺陷独立提交。

## Deviations from Assessment

主代理明确授权额外修改 `process_unix_test.go` 中两条旧 leader-exits 测试：立即清理会在 helper 尚未实际启动时回收组，旧测试原本依赖 500ms WaitDelay 窗口才出现 READY；改为自有 ready 文件同步并不降低物理停止/取消边界，另加 blocked writer 真实顺序检查。主代理亦明确要求保留原 os/exec 对同一个可比 writer 的串行行为；实际 same bytes.Buffer race 回归验证后，只增加本次 Run 的局部 Write 锁，不增加检测框架或队列。

## Follow-ups

- 主代理重建新 Agent 后复跑原真实 Android 签名 APK/AAB、完整中央日志、artifact/download，再完善 test.md；未补前保持 partial，不将自动测试冒充原应用联验。
- 不修改依赖、不增加 executor/interface/日志队列，不调整 500ms 本组 TERM/KILL 窗口，不忽略截断错误。
