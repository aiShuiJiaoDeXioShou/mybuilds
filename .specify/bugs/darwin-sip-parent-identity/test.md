# Bug Verification：SIP原父身份区分

- **Slug**: darwin-sip-parent-identity（取当前明确缺陷上下文）
- **Tested**: 2026-10-04T18:42:00Z
- **Assessment**: ./assessment.md
- **Fix**: ./fix.md
- **Result**: verified

## Summary

真实内核ABI、重新父化SIP负门及正常TLS定向race通过；最新全包并行仍有两消费者cleanup失败，尚未最终验收。独立两目标22次正常/race通过不能替代同批完整回归。

## Checks Performed

| Check | Command / Action | Result | Notes |
|---|---|---|---|
| ABI与重父能力 | std syscall56B与真实fork/exec/PPID1 | pass | unique及原父version不同init真实核对 |
| 定向新门 | process Darwin/SIP及SCM TLS race | pass | 5.102s/2.764s |
| 完整normal | go test ./... | fail | 108.707s，Agent与CLI cleanup未确认 |
| 故障定位 | 独立WT两目标合计8/14次 | pass | 包括并行race；仅一个瞬态Z后Gone，无false样本 |
| 完整race/vet与双库应用 | 最终字节 | not-run | 必须先核对并行失效原因与负门 |
| 父先Gone后exec负门 | 新独立WT已写，未运行 | not-run | 暂隔离，不与全Go消费者重叠 |

## Output Excerpts

实际full正常：unknown claim mis-stopped valid execution: agent_execution_unconfirmed；CLI真实节点注册等待超时、Agent退出agent_cleanup_error。原日志SHA7d467bbb9d0e130a074f128de8746bdb891b44c4798f65790719978cbc9896cf。

## Residual Risks

- 原父version为32位，XNU全局fork/exec计数可回绕；原父unique与version组合不应宣称永久无碰撞的历史证明。未制造回绕，记录理论局限；可信常规工具范围不等于恶意隔离。
- init exec改变其version可导致真实init子对象仍unknown，是保守误拒，不放宽。
- Darwin核对与kill存在平台TOCTOU；私有flavor实际支持按当前Darwin27验证，不假称所有系统支持。
- 真实不可读孤儿故障注入可能保守影响同用户同时运行的其它scope，需整套隔离定位。

## Recommendation

保持partial，不关闭、不提交。先补真实父Gone后exec负门与并行消费者身份诊断，再全套检查/应用复验；不改unknown规则或测试断言来隐藏失败。

## 实际父先Gone再exec负门（2026-10-04T18:52:32.391172+00:00）

新增真实helper先观察PPID1并记录身份，随后exec SIP sleep：同birth/unique、exec后parentunique=init但原父version仍不同init。严格核对darwinBornToInit=false、unknown保持、stop=false且同对象仍活/noSignal；定向race exit0/4.049s，日志SHAb5addb2f35490fe3823f247561d0170905092580ed0f58d26a09df826516cf4e。新测试SHA8304bafe3edf0fac26190a813955fdba886c08dbcb289dbcf6635c2db37a1638已根逐SHA集成，现需最终统一字节完整回归与实际双库应用。

## 当前统一源码验收（2026-10-04T19:06Z）

完整normal/race/vet顺序-p1全部exit0、12跨平台编译/6本机入口及fresh双库72应用全部通过。normalSHA960e9baa001ad8c6cf226d13517c1bbc3305db89a53b659d7df5445dd1cd1473/raceSHAcf0ec9dc7156bc92f5bd8eef77a5dc4f5c0a1d411b0f43f1fe4c561073aecb1a；应用SHAee5eaf5d0f7cf669f210335cc845ee311935249f43d2ff0e41b80e57d81190de。Darwin两个真实重父顺序均保持unknown/noSignal，正常TLS消费者不误拒；该缺陷更新verified。之前not-run/失败阶段保留，上述最终证据为准；测试跨包真实故障注入隔离，不放产品unknown规则，32位版本/平台TOCTOU局限仍成立。原detached Gradle真实正常签名门单独保持待验，不用本项覆盖。
