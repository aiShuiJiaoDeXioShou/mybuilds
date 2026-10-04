# Bug Verification：最终原进程组停止复核

- **Slug**: stale-group-stop-confirmation
- **Tested**: 2026-10-04T18:48:14Z
- **Assessment**: [assessment.md](assessment.md)
- **Fix**: [fix.md](fix.md)
- **Result**: verified

## Summary

Darwin ARM64 的实际晚回收旧源红门、修复后四门 race 与完整 process race 均已执行，原 false 冻结症状已消失，保护负门保持。尚未执行新修复的 Linux 原生或应用消费者门，故完整跨平台结论保持 partial。

## Checks Performed

| Check | Command / Action | Result | Notes |
|-------|------------------|--------|-------|
| 旧源真实复现 | `go test ./internal/process -run '^TestScopeStopRechecksLateReapedGroup/late_reap$' -count=1 -v` | expected fail | exit 1，18:46:01.607Z–18:46:03.756Z |
| 四门真实 race | `go test -race ./internal/process -run '^TestScopeStopRechecksLateReapedGroup$' -count=1 -v` | pass | exit 0，18:46:40.223Z–18:46:52.432Z |
| 完整 process race | `go test -race ./internal/process -count=1 -v` | pass | exit 0，18:47:11.901Z–18:47:45.709Z |
| 独立 Darwin 孤儿再次 exec | 另一独立 WT `go test -race ./internal/process -run '^TestScopeDarwinOrphanExecAfterParentGoneKeepsUnknown$' -count=1 -v` | pass | exit 0，18:48:09.328Z–18:48:14.235Z，不与上述门重叠 |
| 格式/空白检查 | `gofmt`、`git diff --check` | pass | 无新依赖 |
| Linux 原生新门 | 根后续集成 | not-run | 本任务禁止进入 VM |
| 应用消费者/全仓 vet | 根后续统一验证 | not-run | 本任务只跑必要 process 一次，不重复全 Go |

## Output Excerpts

```text
旧源：最终实际原组证据/未知保护判定错误 false
四门：ok mybuilds/internal/process 11.334s
完整 process：ok mybuilds/internal/process 33.198s
独立孤儿 exec：ok mybuilds/internal/process 4.049s
```

原始安全日志与 UTC/exit JSON 位于 `/tmp/mybuilds-mvp.zKtK0e/stale-group-evidence-qf51pdt8`：

| Log | SHA256 |
|-----|--------|
| `red.log` | `6a3823ae2b407d0733f4326e90c9c7c904b6b4cabaa1d37bb1cccca4cd26b1d7` |
| `green-four-race.log` | `e67adbd1d8973e5e35d1ead306a8350bf3bb0c52ba8c7a55522aa3c747575afe` |
| `process-full-race.log` | `058864fc5c4b7a56455cbd676a9f15ffb0aa8ba3f779e4f075ad47a9c26e0748` |
| `darwin-orphan-exec-race.log` | `b5addb2f35490fe3823f247561d0170905092580ed0f58d26a09df826516cf4e` |

## Residual Risks

- 新修复 Linux 原生与应用消费者尚未复验；不能据本机门推断 207 原因。
- PGID 只读复核不改变原 Darwin 精确信号 TOCTOU 限界或原父版本 32 位回绕限界；unknown/uncertain 与原预算不放宽。
- 两个 WT 严格串行运行；18:48:14.235Z 后隔离窗口已归还根，无剩余自有测试或故障注入进程。

## Recommendation

本机最小修复与保护回归可供根逐 SHA 集成；补齐实际 Linux 与应用消费者门后再决定完整 verified。保留旧红日志及 207 未知保护证据，不据相关性清 guard。

## 最终跨平台与消费者验收（2026-10-04T19:06Z）

新夹具实际严格birthBefore，修正Linux同jiffy前置并保留首次失败日志；旧scope ARM late_reap再次红，新scope LinuxARM/AMD各31顶层全部通过、Darwin四门race9.902s通过，日志与逐源清单由根独立核SHA。汇总SHA c02ac24a72defbd2596486c445d321b77e5911ebce4ac00f64b12d04914aaca1，测试SHA85e92825f212a5c4dfa0132f90a88dc2b42fc81ddc2c2054880a9d227eb295f5，生产8ff6baede247382481d8b0d145e458b16e204ac3169f5e8d5fc888fb1944fa3f完全不变。

当前生产源码根完整normal/race/vet全部exit0（唯一后续变更为上述测试前置），12编译/6本机入口及fresh双库72应用通过；后者SHAee5eaf5d0f7cf669f210335cc845ee311935249f43d2ff0e41b80e57d81190de。此独立晚回收误拒症状与保护负例已验证，结论更新verified；207具体根因仍未知，原guard/journal不动，不能据此宣称整MVP或原Android正常制品门通过。上述早期not-run/partial是历史阶段，以下最终实证为准。
