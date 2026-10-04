# Bug Assessment：SIP未知归属误阻正常TLS构建

- **Slug**: darwin-sip-parent-identity（已授权自动目标中生成，唯一新目录）
- **Created**: 2026-10-04T18:19:00Z
- **Source**: pasted text；本机真实SCM/Agent回归与内核身份诊断
- **Verdict**: valid
- **Severity**: high

## Report

本机Darwin27/Go1.25.4，空环境按未知闭锁后，单独 `go test ./internal/scm -run '^TestReadPipelineAnonymousTLSDoesNotDisableCertificateVerification$' -count=1` 重现：实际Git TLS拒绝正确，却返回scm_cleanup_failed。根1791137809/370523，candidate56453出生1791137809/409717、PPID1/PGID56453/stat2/flags4004/oppid0、短comm为trustevaluationa；无读取或打印环境/argv。临时诊断已逐SHA恢复，旧评估不改。

## Symptom

信任校验经launchd新启动系统服务，SIP省略其环境。该对象既新生又PPID1，现有父链无法排除，正常Git/API工具检查与并行Agent被错误闭锁。反向把空环境视无归属又会错误确认真实SIP orphan已停止。不能用名称、路径、UID白名单绕过。

## Reproduction

1. 保持scope_darwin空环境/缺栈unknown及实际外部父链核验。
2. 执行上述单独真实TLS负例，记录精准unknown的pid/ppid/pgid/birth/stat/flags与errno，scope正常扫描、原组Gone却unknown活。
3. 新的真实/bin/sh父退出→PPID1/SIP sleep负例必须仍unknown/noSignal；两个正常SIP并行Run必须互不影响。

## Suspected Code Paths

- internal/process/scope_darwin.go scopeProcesses/darwinScopeMark/darwinExternalParent：SIP未知保守正确，但当前父链不足以排除由init原生启动的对象。
- internal/process/scope_darwin_test.go：需要实际init原父与重新父化身份能力检查，不能mock系统接口。
- internal/process/scope_unix.go stop：原unknown后果保持，不以等候推断成功。

## Root Cause Hypothesis

置信度高：孤儿与init原生子进程当前均PPID1；仅P_starttime/环境/现PPID不能区分。独立-p1检查仍失败，故先前“故障注入互相影响”归因不成立。

## Proposed Remediation

**Preferred**：先实际验证Darwin PROC_PIDUNIQIDENTIFIERINFO（proc_info call2/flavor17/ABI56B）通过现有标准syscall与固定byte buffer可读unique/parentunique/idversion/original-parent-version。仅对unreadable且PPID1候选，用固定kernel元数据核对candidate的parentunique与init unique、original-parent-version与init idversion精确匹配，并重复查询及Kinfo birth/父/flags复核，才能作为“原本由init创建”的外部对象排除unknown；不按名称忽略。普通重父/原父版不匹配/读取或identity不明均保unknown/noSignal；marked与已有members始终优先，仍返回out以清真实已排除unknown。不得增加public API、执行器、cgo、依赖或全局进程框架。真实fork/exec后重父行为先probe，若ABI/原父语义不成立则停止修正，保持未验收。

**Alternatives**：保持所有PPID1未知（安全但正常TLS不可用）；按名字/SIP排除或把空环境视完整（不能保证安全，拒绝）；ptrace/cgroup/trampoline（超当前范围，拒绝）。

**Files likely to change**：仅internal/process/scope_darwin.go、scope_darwin_test.go及本缺陷fix/test记录；确有必要才改私有scope消费者并记录，不改变Linux已通过字节或008协议。

**Tests to add or update**：真实syscall固定对象/原父能力、重新父化SIP仍unknown且无Signal、正常SIP并行、真实Git TLS期望安全错误、Agent并发/续租/中断、完整test/race/vet与双库三入口应用。

## Risks & Considerations

Darwin原父元数据私有flavor需实际ABI/语义验证；unsupported/权限/短返回全部未知，不发明支持。源码显示普通reparent不改parentunique，但exec可能重设它，必须同时原父version核对。PIDversion有限位数/exec-shadow变化需研究与保守处理，不宣称永久历史或pidfd同等TOCTOU能力。不会手动停止真实系统服务或解除旧204保护。

## Official Sources

[内核固定结构](https://raw.githubusercontent.com/apple-oss-distributions/xnu/main/bsd/sys/proc_info_private.h)、[实际info消费者](https://raw.githubusercontent.com/apple-oss-distributions/xnu/main/bsd/kern/proc_info.c)、[fork/exec与父身份](https://raw.githubusercontent.com/apple-oss-distributions/xnu/main/bsd/kern/kern_proc.c)、[普通重父](https://raw.githubusercontent.com/apple-oss-distributions/xnu/main/bsd/kern/kern_exit.c)。这些是主动技术研究，非用户外部bug URL。

## Open Questions

无需求歧义；kernel原父字段真实行为是实施能力门，未经probe通过不实现排除。
