# Bug Assessment：进程组停止结果未在最终确认时复核

- **Slug**: stale-group-stop-confirmation（自动目标生成的唯一目录）
- **Created**: 2026-10-04T18:42:00Z
- **Source**: pasted text；根与独立代理只读实际代码
- **Verdict**: valid
- **Severity**: medium

## Report

scope.stop首先以stopProcessGroup取得groupStopped，之后执行成员TERM/KILL清理窗口。所有members/unknown已经核实Gone时仍返回旧groupStopped && !uncertain。原组若在第一次窗口后、后续窗口内被真实回收，旧false导致误报cleanup失败。207真实Android构建成功却cleanup_error仅提示相关性，其捕获没有scope内部errno，不归因本缺陷。

## Symptom

晚回收原组在最终核查已Gone也不能正常结束；可能错误隔离节点并拒绝后续步骤。不是停止过早，本修复不得减弱未知停止保护。

## Reproduction

1. 自有sleep作为独立PGID/root；在首次清理窗口不Wait，使其实际Z且group signal0仍存在。
2. 另外自有测试binary作为setsid且继承本次nonce成员，SIGTERM后写有限私有阶段回执。
3. 只有收到成员TERM回执、核实原组仍存在后，唯一Wait回收root；核实group ESRCH，等待独立成员唯一Wait。
4. 原scope.stop仍因冻结false返回false。不是固定sleep猜阶段，红绿以实际回执和kernel对象为准。

## Suspected Code Paths

- internal/process/scope_unix.go processScope.stop：旧groupStopped布尔值跨后续清理阶段不再复核。
- internal/process/process_unix.go stopProcessGroup：返回代表其当时的窗口结论，不能当后续最终状态。

## Root Cause Hypothesis

置信度高，直接代码路径证明；真实红门尚待执行。不猜207具体候选或errno。

## Proposed Remediation

**Preferred**：保留原组TERM/KILL与所有members/unknown/uncertain规则；最终allStopped时重新读取原PGID signal0，仅root>1且实际ESRCH、没有uncertain时可成功。nil/EPERM表示仍未知或存在，在已有剩余窗口继续检查，其它错误闭锁；不得沿用旧true或旧false，不重置/增加清理预算，不增加额外信号、执行器或公共类型。

**Alternatives**：延长固定等待会减缓而不修正冻结值；忽略原组或unknown会产生假停止，拒绝。

**Files likely to change**：
- internal/process/scope_unix.go（唯一最终证据判断）
- internal/process/stale_group_test.go（Darwin/Linux真实进程红绿/负门）
- 本目录fix.md与test.md；原detached缺陷验证和008验证可追加实际结果。

**Tests to add or update**：真实late-reap门、原组仍存在拒、uncertain仍拒、unknown活对象仍拒，无关自有sleep保持；现有process与应用消费者回归。

## Risks & Considerations

PGID signal0仅在已有本次kernel birth成员与完整scope检查后使用；ESRCH证明当前实际原组不存在，不按名称/UID杀进程。保留原Darwin kill TOCTOU限界、Linux pidfd、不可读unknown闭锁与预算；新测试不可与SIP未知孤儿故障注入或其它全量应用重叠。207未证明具体归因，仍保留原journal/guard和失败证据。

## Open Questions

无需求歧义；真实红绿可在自有进程执行，不需要用户材料。
