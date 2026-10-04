# 缺陷评估：短超时误判进程组清理失败

- **Slug**: short-timeout-cleanup（任务指定自动采用）
- **Created**: 2026-10-04
- **Source**: pasted text
- **Verdict**: valid
- **Severity**: medium

## 原始报告

> engine003 的 TestRunPostBudgetMarksUnstartedItems 在 30ms timeout 时，Post[0] duration32ms、CleanupFailed=true，下一 always 为 cleanup_error；两 budget isolated-count10 出现一次，同批累计150ms例 post 缺失。

## 症状

真实 shell 超时后，进程组已经退出，但退出/回收期间瞬时的 EPERM 被当作最终停止未确认。执行器因此跳过本应独立执行的 failure post，或将剩余 always 标成 cleanup_error；应继续有限等待，且只在 ESRCH 确认组消失时成功。

## 复现

1. 在已验收 002 基线 256af8e 的独立工作树执行 `go test ./internal/pipeline -run 'TestRun(PostBudgetMarksUnstartedItems|CumulativeBudgetAndIndependentPost)$' -count=50`。
2. Darwin arm64 / Go1.25.4 实际失败 3 次：30ms post 两次 CleanupFailed=true（duration30ms/32ms），150ms 累计预算一次缺少 post 文件。
3. 仅在本 BUG_DIR 临时复制原 runShell，加 pgid/信号/errno 数字诊断，以 30ms 执行 `/bin/sh -e -c 'sleep 1'`；第 16 轮出现 signal0 EPERM、signal9 EPERM，CleanupFailed=true，duration32.351834ms。
4. 第二份安全诊断在 EPERM 后读取仅 pid/ppid/pgid/uid/stat 的进程表，筛选本次 pgid，再以 signal0 复查；600 轮捕获多次 EPERM，均未发现该组成员且后续返回 ESRCH。该诊断引入的耗时本身使后续 SIGKILL 获得 ESRCH，因此没有错误结果；它是瞬时状态的证据，不是修复。

## 涉及路径

- `internal/pipeline/process_unix.go:stopProcessGroup`：TERM/KILL 遇到非 ESRCH 错误立即失败。
- `internal/pipeline/process_unix.go:waitProcessGroupGone`：signal0 的任何非 ESRCH 错误立即失败，未用完 500ms 确认窗口。
- `internal/pipeline/process_unix.go:runShell`：取消回调与正常 Wait 后共用一次 cleanup，将误判写入 CleanupFailed。
- `internal/pipeline/run.go:executeStep/executeBuild/executePost`：唯一生产 runShell 调用；正确地以 CleanupFailed 阻止不安全的后续动作，不应修改这些策略。
- `internal/pipeline/process_unix_test.go`：既有前台/后台、忽略TERM、超时、日志错误及无关进程保护用例。

## 根因

信心：高。Darwin 进程组退出/回收边界的瞬时 EPERM 已通过真实 errno 与后续 ESRCH 复查观察到。现有检查把“本轮无法发送/探测”直接当成“窗口内不能确认消失”，导致在约32ms而不是确认窗口耗尽后返回失败。保留 EPERM 的不确定含义，继续有限探测即可；不能直接把 EPERM 视为成功。

## 修复方案

**首选**：仅调整进程组停止路径。TERM/KILL 的 EPERM 仍进入原有限确认窗口；signal0 的 EPERM 在同一窗口内重试。其他未知错误立即失败，只有 ESRCH 可确认停止，窗口耗尽仍失败。保留 EINTR 重试、先TERM后KILL、pgid>1及仅本次进程组边界，不添加接口/hook/依赖，不改 run 或预算。

**修改文件**：`internal/pipeline/process_unix.go`、`internal/pipeline/process_unix_test.go`。

**验证**：新增真实30ms超时的重复进程回归；验证活进程组不会被有限探测误判消失。复跑原两预算50轮、既有进程与post/取消回归、全量 test/vet 和相关 race。测试不得通过扩大预算或弱化 CleanupFailed 断言绕过问题。

## 风险与限制

- 真正权限不足的组会耗尽确认窗口后失败，不能将其当作停止成功。
- 不改变停止策略、日志公开诊断或用户环境继承；临时诊断不进入生产代码。
- 原问题已在 Darwin 实际复现；Linux 仍需后续真实节点验收，交叉编译不作实际运行证据。

## 未决问题

无实现阻塞。不会推断瞬时 EPERM 的内核内部原因；已观察的退出/回收边界足以支持有限重试。
