# 缺陷验证：短超时误判进程组清理失败

- **Slug**: short-timeout-cleanup（来自已完成的评估/修复上下文）
- **Tested**: 2026-10-04
- **Assessment**: ./assessment.md
- **Fix**: ./fix.md
- **Result**: verified

## 结论

原问题已在未修改的002基线复现，并由真实 errno/pgid 诊断确认瞬时 EPERM 随后转为 ESRCH。修复后原两预算用例50轮、新增100轮30ms超时、既有进程保护与相关race全部通过；只有 ESRCH 才确认组消失，未扩大默认等待时间。

## 检查

| 检查 | 命令/动作 | 结果 | 证据 |
|---|---|---|---|
| 原报告修复前复现 | `go test ./internal/pipeline -run 'TestRun(PostBudgetMarksUnstartedItems\|CumulativeBudgetAndIndependentPost)$' -count=50` | fail（预期红测） | 30ms两次CleanupFailed，累计150ms一次post缺失 |
| 新回归修复前复现 | `go test ./internal/pipeline -run '^TestRunShellShortTimeoutConfirmsCleanup$' -count=1` | fail（预期红测） | 第41轮CleanupFailed，duration31.377292ms |
| errno诊断 | BUG_DIR临时复制原runShell，记录本次pgid/信号/errno；第二轮仅数字进程状态表与signal0复查 | pass | 第一轮第16次失败；第二轮600次捕获24次EPERM，均无组成员并随后ESRCH |
| 原报告修复后 | `go test ./internal/pipeline -run 'TestRun(PostBudgetMarksUnstartedItems\|CumulativeBudgetAndIndependentPost)$' -count=50` | pass | 16.699s |
| 新增回归 | `go test ./internal/pipeline -run '^(TestRunShellShortTimeoutConfirmsCleanup\|TestWaitProcessGroupGoneRequiresDisappearance)$' -count=1` | pass | 4.718s；含100轮30ms和活组保护 |
| 进程回归 | `go test ./internal/pipeline -run '^(TestRunShell\|TestWaitProcessGroup)' -count=1` | pass | 9.297s；前台/后台/忽略TERM/输出错误/无关进程 |
| 全量回归 | `go test ./...` | pass | pipeline10.294s，其余包通过 |
| 相关race | `go test -race ./internal/pipeline -run '^(TestRunShell\|TestWaitProcessGroup\|TestRun.*(Budget\|Cancellation\|Post))' -count=1` | pass | 10.389s |
| 静态检查 | `go vet ./...`、`git diff --check` | pass | 退出码0 |
| 交叉编译 | `GOOS=linux GOARCH=amd64 go build ./cmd/mybuilds`、`GOOS=windows GOARCH=amd64 go build ./cmd/mybuilds`（输出到临时目录） | pass | 退出码0；不作Linux实际运行证据 |

## 安全与范围复核

- 生产修改仅三个错误分支及中文解释，500ms grace与10ms探测间隔保持；不改run/预算、组选择、公开日志或错误，不增加依赖。
- EPERM不能确认成功，只在原窗口内重试；持续EPERM和窗口耗尽仍失败，其他未知错误仍立即失败。
- 仅处理本次pgid>1，不按进程名杀进程；既有无关进程保护测试通过。
- 临时诊断代码/输出已删除，正式交付仅两个process文件和assessment/fix/test三份文档。003文件未改动。

## 剩余限制

- Darwin arm64 / Go1.25.4 为本次实际复现与验证平台；Linux只有交叉编译，仍需后续真实节点验收。
- 真实调度竞态通过重复真实进程锁定，没有添加可替换系统调用或测试hook；不宣称穷尽所有调度时序。
- 瞬时EPERM的内核内部原因未推断；修复不把未知权限状态视为成功。

## 建议

关闭缺陷；由主代理在集成树独立复验后，仅暂存两个process文件与三份缺陷文档，执行一次独立本地缺陷提交，不夹带003或push。
