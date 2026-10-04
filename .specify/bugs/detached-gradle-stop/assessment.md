# Bug Assessment: 脱离进程组的 Gradle 未停止却已确认回收

- **Slug**: detached-gradle-stop（本自动任务明确指定）
- **Created**: 2026-10-04T15:35:00Z
- **Source**: pasted text；实际 Linux AMD64 Android 构建私有夹具证据
- **Verdict**: valid
- **Severity**: high

## Report

构建 `7137989f-aff0-4d72-b147-df9cb1e15d52` 超时：shell 14205、wrapper 14207 消失后，独立 PGID/SID 14251 的 Gradle 单次 daemon 仍为 S、ppid=1、start_ticks=1580841 且 CPU 增长。15:22:51.922894 已 ordinary finished ACK、journal.StopConfirmed=true；daemon 至 15:22:52.867 仍运行、15:22:53.184 才 Z、53.512 才消失。未人工发信号，无 PID 重用。

原始证据位于 `/tmp/mybuilds-mvp.zKtK0e/linux-amd-agent007-ho9p4gr3/process-evidence/` 的 retry-timeout-dense-samples.jsonl、retry-stop-confirmed-window.json、retry-gradle-detached-public.json、retry-timeout-timing-summary.json；父代理提供 tar SHA256 `32cce31494b68964acaa653e5e9da8dbe0bddd786e90f55f51e9affa85c1957f`。评估在独立 `85b46bf` 已验收基线进行，008 源码保持冻结。

## Symptom

当前 `process.Run` 仅确认原 leader PGID 消失。Gradle `--no-daemon` 可以派生独立 session 的单次 daemon，原组消失不代表实际构建工具已停止，导致过早发布 StopConfirmed、解除调度保护或启动 post。

## Reproduction

1. 用既有 Android 模板、真实 Gradle wrapper/JDK 构建，JVM 设置不匹配会派生单次 daemon。
2. ordinary Started 后使累计预算到期，连续采样 shell/wrapper/daemon 的 pid/ppid/pgid/sid/start_ticks/CPU。
3. 对照真实中央事件 ACK/journal，出现 StopConfirmed=true 而同 birth daemon 仍活跃即为失败。
4. 自动回归用真实测试二进制创建 setsid 子进程：父组结束后 detached helper 仍运行；同时运行同用户无关 sleep，禁止将其停止。

## Suspected Code Paths

- `internal/process/process_unix.go:Run` — cleanup 的唯一停止证据来自原进程组。
- `internal/process/process_unix.go:stopProcessGroup` — 只向 `-pgid` 发信号与检查 ESRCH。
- `internal/pipeline/run.go` — 沿 CleanupFailed 决定停止证据与后续步骤/post；应复用已有闭锁，无第二执行器。
- `internal/mobile/templates/android.yml` — `--no-daemon` 并非绝不 fork 的承诺。

## Root Cause Hypothesis

置信度高。Gradle 官方说明客户端 JVM 参数/JDK 不匹配时，即便 --no-daemon 仍创建单次 daemon。真实 daemon 已 setsid，因此不属于原 shell 的进程组。当前 cleanup 的判断域不足，pipe EOF 也不能证明 detached 工具停止。

## Proposed Remediation

**Preferred**：在唯一 `process.Run` 内补私有、一次运行的精确归属，不改变公共 Command/Result、执行器或协议。每次启动前产生独立随机继承环境标记；仅匹配本次标记或已核实实际父链的进程，并记录 kernel birth identity。保持原父组清理，清理前后受限读取当前实际归属；对 detached 成员逐一核对 birth 并终止/确认其消失。Linux 使用已有 x/sys 的 pidfd_open/pidfd_send_signal 固定对象，核对打开前后 start_ticks，不向复用 PID 发信号。Darwin 使用已有 SysctlKinfoProc/SysctlRaw 的 P_starttime 和实际环境；每次信号前复核身份。扫描/归属/停止无法确认时 CleanupFailed=true，沿现有 Run/Agent 保留停止保护并禁止后续动作。标记不持久化、不加入 Result/日志/协议；输出只对本次内部随机值做有界跨块脱敏，完整保留其它慢 writer 数据。

该标记是可信工具的实际归属证据，不是恶意脚本的权限隔离。真实 Gradle/JDK 与 helper 的继承需分别验证。不能把按名字/用户杀进程、固定等候、仅信号成功或父组消失当整个工具已停止。

**Alternatives**：
- 模板匹配 JAVA_OPTS/GRADLE_OPTS/org.gradle.jvmargs：只能减少特定工程的 fork；JDK/daemon criteria、已冻结自定义脚本仍可派生，不能作为此缺陷完整修复。
- 通用 cgroup/ptrace/启动 trampoline/进程管理框架：涉及权限、跨平台能力及新生命周期，超过当前最小修复，不引入。
- Darwin kqueue NOTE_TRACK：实际 kernel 返回 ENOTSUP，不因 x/sys 常量存在而假称支持。

**Files likely to change**：
- internal/process/process_unix.go（唯一 Run 的归属、清理和标记脱敏消费者）
- internal/process/scope_unix.go（必要的本次私有归属/有界脱敏实现）
- internal/process/scope_linux.go、scope_darwin.go（实际 kernel 身份/环境/信号）
- internal/process/detached_test.go、scope_linux_test.go、scope_darwin_test.go（真实 helper/身份/安全负例）
- 既有 process_unix_test.go 或 pipe_test.go 仅当真实既有行为断言必要同步；须在 fix.md 记录偏离。
- 本缺陷 assessment/fix/test.md；不修改模板/README/008 Agent/Store/protocol 或依赖。

**Tests to add or update**：
- actual setsid helper：正常 leader 退出、用户取消、timeout、忽略 TERM、双重派生；Run 返回前确认本次 detached 停止，无关同用户进程仍存活。
- 实际 birth identity 不匹配不能发信号；Linux pidfd 固定对象；Darwin birth 复核。
- 不可读/超过限额/未知归属必须 CleanupFailed，不靠延迟得出 success。
- 独立两个并行 Run 的不同标记，不影响另一任务；随机值跨 stdout/stderr 分块不会出现在输出。
- 保留既有慢两管道完整排空、同 writer race、OnStart 一次 Wait、取消/失败预算/post 等门。
- 原真实 Linux AMD64 Android 复验：daemon 消失不晚于 finished At；macOS 真实工具或 helper 对等门；未取得原实际工程复验前 test.md 保持 partial。

## Risks & Considerations

- **Darwin 实测能力**：27.0.0 下第三方 Go helper 和 JDK17 可读真实 nonce/P_starttime；SIP 限制的 /bin/sleep 只返回 argv，环境被内核省略。不能将空环境直接解释为“所有子孙都已停止”。已观测父链成员仍需记录 birth；身份/读取不明必须闭锁。
- Darwin kill(pid) 无 pidfd 原子对象接口，核对与 kill 间仍有 TOCTOU；必须如实记录平台限制，不发明绝对 PID 重用安全保证。
- 环境继承被故意清除/权限升级/未知逃逸不能被环境标记当成已证明完整；本次未能形成可靠停止证据时保保护，不以模糊轮询补保证。
- 读取为有限普通内核元数据，不输出 argv、环境或宿主秘密；无关进程即使同用户同名也不在信号范围。
- 取消/原失败 reason 不覆盖，CleanupFailed 作为副标志；未知停止不可启动 always 或宣称 finished 已停止。

## Official Technical Sources

- [Gradle daemon 官方文档](https://docs.gradle.org/current/userguide/gradle_daemon.html)：JVM 设置不匹配的单次 daemon；仅调整模板不足。
- [Linux pidfd_open](https://man7.org/linux/man-pages/man2/pidfd_open.2.html)、[pidfd_send_signal](https://man7.org/linux/man-pages/man2/pidfd_send_signal.2.html)：通过 fd 引用实际进程对象，不以复用 PID 发送信号。
- [Apple XNU kern_sysctl.c](https://raw.githubusercontent.com/apple-oss-distributions/xnu/main/bsd/kern/kern_sysctl.c)：KERN_PROCARGS2 对 cs_restricted 等进程省略环境变量。

以上是主动技术资料研究，不是外部 bug URL。未收到需按 bug URL trust policy 抓取的用户 URL。

## Open Questions

无需求歧义。平台 kernel 与真实 Gradle 能力以行为检查判定；若 preferred 无法形成可靠归属，停止修改、在 fix.md 记录偏离并重新评估，不将不成立的保证发布为成功。
