# Bug Fix：最终复核原进程组实际停止证据

- **Slug**: stale-group-stop-confirmation（本次明确指派）
- **Fixed**: 2026-10-04T18:46:40Z
- **Assessment**: [assessment.md](assessment.md)
- **Status**: applied

## Summary

旧实现真实 late_reap 红门已成立：首次原组清理窗口结束后，测试唯一 Wait 令原组真实 ESRCH，所有已确认成员已回收，但冻结的 false 仍拒绝停止确认。修复仅在最终 allStopped 分支重新读取原组 signal 0 的当前证据，不沿用旧 true/false。

## Changes

| File | Change | Notes |
|------|--------|-------|
| `internal/process/scope_unix.go` | 修改最终原组确认 | root>1 且实际 ESRCH 才可能成功；uncertain 仍拒；nil/EPERM 沿原剩余窗口复查，其它错误闭锁 |
| `internal/process/stale_group_test.go` | 新增真实进程测试 | 不使用执行器替身、内核 stub 或固定延迟判断清理阶段 |

首轮 TERM/KILL、成员归属/精确信号、unknown 与 uncertain 规则全部保留；仅新增信号 0 只读检查，不增加清理预算，不引入依赖、执行器或公共类型。

## Tests Added or Updated

- `TestStaleGroupMemberHelper`：实际独立 SID 测试 binary 接收 TERM 后写不超过 1KiB 的私有固定阶段回执。
- `TestScopeStopRechecksLateReapedGroup/late_reap`：原组 root 暂不 Wait，使首次窗口过后组仍存在；只有取得独立成员真实 TERM 回执并核实同 birth 原组存在，才唯一 Wait root，核对 ESRCH 后要求最终成功。
- `group_still_present`：直到 stop 返回都保留未 Wait 原组，要求失败。
- `uncertain`：组与成员 Gone 也不得清除已有 uncertain。
- `unknown_alive`：真实活对象的 unknown 保护保留且不得信号。全部子用例同时核对无关同用户对象仍为同 birth 且存活。

## Local Verification

环境：Go 1.25.4，Darwin ARM64。证据目录 `/tmp/mybuilds-mvp.zKtK0e/stale-group-evidence-qf51pdt8`。

- 旧源：`go test ./internal/process -run '^TestScopeStopRechecksLateReapedGroup/late_reap$' -count=1 -v` → exit 1，失败于最终停止结果仍 false，package 1.350s；红日志保留。
- 修复后：`go test -race ./internal/process -run '^TestScopeStopRechecksLateReapedGroup$' -count=1 -v` → exit 0，四门通过，package 11.334s。
- 完整 process：`go test -race ./internal/process -count=1 -v` → exit 0，package 33.198s；保留 pipe、OnStart、原 unknown 与并行真实 Run 门。
- `gofmt` 与 `git diff --check` → exit 0。
- `scope_unix.go` SHA256：`8ff6baede247382481d8b0d145e458b16e204ac3169f5e8d5fc888fb1944fa3f`。
- `stale_group_test.go` SHA256：`8ef96c89ddabf52aeeb281f2478e1d78a16d41e6ca52cf669e7df2c68a629b55`。

## Deviations from Assessment

无。207 的实际 CleanupFailed 根因仍未确认，未归因本缺陷；不覆盖原 journal/guard 或失败证据。

## Follow-ups

执行 `$speckit-bug-test slug=stale-group-stop-confirmation` 的本机验证记录已生成；Linux 原生晚回收门及应用消费者回归交根后续集成验证，当前记录不提前宣称完整跨平台验收。未进入 VM、未修改根、未提交。

后续根已补真实LinuxARM/AMD31门、全仓消费者normal/race/vet与fresh双库72应用，全通过；最终测试夹具以实际严格birth前置跨平台，保留原ARM失败，不改生产字节或停止保证。独立本缺陷verified见test.md，207未归因。
