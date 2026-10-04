# Bug Assessment：成功进程被慢日志排空误判失败

- **Slug**: slow-log-pipe-drain
- **Created**: 2026-10-04T12:57:00Z
- **Source**: 本目标真实 Android 二进制联验
- **Verdict**: valid
- **Severity**: high

## Report

实际构建83fb0d87-8903-4d32-8b18-ac32b131f172、固定e7ab37c108ef9c71fe922721cec1b97e76e48a38、节点mac-a。声明的测试JKS配对经keytool实际验证，Gradle完成50个任务并输出BUILD SUCCESSFUL in14s；中央却保存step failed/reason exit/exit_code0、artifact未执行。实际日志末尾与完整build详情位于自有us1-app007-48aqb9rf/android-second-log.json。

## Symptom

正常退出0的run有真实同步HTTP日志回传，待排空耗时超过500ms后，被报告为exit失败，阻止后续产物；不能通过忽略错误把已被强关pipe的遗漏日志伪装完整。

## Reproduction

1. 实际server/Agent/CLI、verifiedHTTPS、自有只读Git工程，触发android007 --build android --idempotency-key android007-paired-signing。
2. 声明自己的测试JKS/别名/密码env，真正Gradle --no-daemon --offline执行APK/AAB签名构建。
3. 真正中央日志看到50任务/BUILD SUCCESSFUL，而build show步骤failed/exit/exit_code0且产物列表为空。
4. 自动回归应使用实际process.Run真实shell及超过500ms的受控writer，核对stdout/stderr完整内容和成功；同时保留真实后台/忽略TERM/leader已退出/ctx取消检查。

## Suspected Code Paths

- internal/process/process_unix.go Run：cmd.Stdout/Stderr是同步writer，cmd.WaitDelay=processTermGrace=500ms；cmd.Wait同时等待子进程和os/exec的管道复制。
- internal/process/process_unix.go：所有非nil waitError都变为reason exit，不区分进程退出与管道强关。
- internal/agent/execute.go、spool.go：结构化脱敏日志先journal/fsync，再同步真实HTTP确认；实际每行约60ms，属于正确的有界确认消费者。
- 本机Go1.25.4 src/os/exec/exec.go：WaitDelay自进程exit或ctx取消开始，I/O未完则强关pipe并返回ErrWaitDelay，即使ProcessState.ExitCode()==0。

## Root Cause Hypothesis

置信度高：500ms被同时用于物理TERM/KILL回收窗口和日志复制截止。真实子进程成功不能保证同步远程日志在500ms内处理完；cmd.Wait先等复制，导致物理组清理也晚于leader退出。现有实现会强关未排空pipe而把成功报告为exit失败。

## Proposed Remediation

**Preferred**：在同一个process.Run内最小拆分实际子进程Wait、独立stdout/stderr pipe复制和进程组回收。仍一次Start/Wait、原OnStart/取消/fail闭锁。child退出后立即按原500ms TERM→KILL规则确认本组回收；输出按实际EOF完整排空，不能用固定500ms writer截止强关已缓冲输出。仅对真正无数据仍持有的pipe读取设置有限idle deadline，writer写入时间不冒充idle读取；ctx/日志失败继续只回收本组。采用标准库/os.Pipe而非新executor、队列/依赖或测试注入。实际OS pipe deadline不支持时需安全明确失败/关闭，不忽略错误。

**Files likely to change**：internal/process/process_unix.go、internal/process/pipe_test.go（新增行为测试）；必要pipeline/agent关联真实用例仅原B分区文件。

**Tests to add or update**：真实成功exit0+慢writer>500ms、多buffer stdout/stderr逐字节完整；正常/取消/写失败、后台持pipe/忽略TERM/leader已退出、OnStart仅真实Start后/错误Wait一次、无关PID保留；真实Remote日志回传延迟后Run成功完整cursor。最终重新构建Agent，实际Android签名APK/AAB/mapping及中央下载核验。

## Risks & Considerations

- 不忽略ErrWaitDelay或仅增加秒数掩盖日志丢失，不能靠quiet Gradle或跳lint降低输出来过门。
- 不增加第二executor/通用接口/不受限队列，进程组与copy结束均需真正证据，不能把定时器当回收。
- 保持原取消/日志故障/Authority/post闭锁、独立工具链和无关进程边界。
- 修复属于已验收process共享路径，须按缺陷流程完整验证且独立相关提交，不能夹带未验收007完整代码。若新OnStart等007增量使独立提交无法保持自包含，应在已验收004基线worktree作相同最小修复并验收提交，再以该提交集成007增量（主代理决定）。

## Open Questions

无待用户澄清；本目标已授权必要实现、真实验证与本地提交。
