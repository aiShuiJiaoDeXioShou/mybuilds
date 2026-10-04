# 缺陷修复：短超时误判进程组清理失败

- **Slug**: short-timeout-cleanup（来自本次评估上下文）
- **Fixed**: 2026-10-04
- **Assessment**: ./assessment.md
- **Status**: applied

## 变更

TERM/KILL 的 EPERM 不再绕过原有限确认窗口；signal0 的 EPERM 在窗口内复查。只有 ESRCH 表示停止成功，持续权限错误、窗口耗尽和其他错误仍失败。

| 文件 | 变更 | 说明 |
|---|---|---|
| `internal/pipeline/process_unix.go` | 修改三个错误条件，补充中文注释 | 保留500ms grace、10ms探测、先TERM后KILL及pgid边界 |
| `internal/pipeline/process_unix_test.go` | 新增两个真实进程测试 | 100轮30ms超时及活组不能误判消失/无效pgid保护 |

## 回归检查

- `TestRunShellShortTimeoutConfirmsCleanup`：未修源码第41轮失败，duration31.377292ms、CleanupFailed=true；修复后100轮通过。
- `TestWaitProcessGroupGoneRequiresDisappearance`：活进程组探测耗尽不能成功，探测不杀进程，无效/全局pgid拒绝。

## 本地验证

- 新增两测试：`go test ./internal/pipeline -run '^(TestRunShellShortTimeoutConfirmsCleanup|TestWaitProcessGroupGoneRequiresDisappearance)$' -count=1` → 通过，4.718s。
- 原报告两预算用例：`go test ./internal/pipeline -run 'TestRun(PostBudgetMarksUnstartedItems|CumulativeBudgetAndIndependentPost)$' -count=50` → 通过，16.699s；修复前同命令失败3次。
- 既有进程/日志错误/无关进程保护和新增检查：`go test ./internal/pipeline -run '^(TestRunShell|TestWaitProcessGroup)' -count=1` → 通过，9.297s。
- `git diff --check` → 通过。

## 与评估差异

无。未修改 run、预算、公开错误、默认等待时间或003文件；未增加依赖、接口或测试hook。

## 后续

由 bug-test 继续全量/race/vet与交叉编译，主代理独立验收并单独提交缺陷。临时诊断程序及完整输出在交付前删除；事实保留评估及验证报告。
