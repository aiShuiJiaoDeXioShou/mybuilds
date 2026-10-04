# Bug Verification：慢日志不再误判真实成功进程

- **Slug**: slow-log-pipe-drain
- **Tested**: 2026-10-04T13:16:51Z
- **Assessment**: ./assessment.md
- **Fix**: ./fix.md
- **Result**: partial

## Summary

真实成功子进程加超过原 500ms 的同步 writer 回归已由红变绿，双流二进制内容完整；原进程组取消、忽略 TERM、短预算、日志错误与无关 PID 边界全部通过。原真实远程 Android 签名构建尚待主代理用新 Agent 复跑，所以本记录保持 partial。

## Checks Performed

| Check | Command / Action | Result | Notes |
|-------|------------------|--------|-------|
| Reproduction (pre-fix) | `go test ./internal/process -run '^TestRunSlowWriterPreservesBothPipes$' -count=1 -v` | fail (expected red) | actual ExitCode=0、Reason=exit、Duration806.26475ms，证明旧 WaitDelay 路径。 |
| Reproduction (post-fix) | 同一真实子进程 16KiB/256KiB 双流回归 | pass | 各首写 800ms，按完整预期字节比较，无截断且成功。 |
| New / updated tests | `go test ./internal/process -count=1` | pass | 12.103s，含真实持写端 idle 和先组清理后 writer 放行（首次修复检查；最终增加共用writer保护后process完整suite亦再次通过）。 |
| Regression suite | `go test ./...` | pass | 接受006基线全包；process13.028s、pipeline4.109s、scm7.407s。 |
| Same writer regression | `go test -race ./internal/process -run 'TestRunSharedWriter|TestRunSlowWriter|TestDrainPipe|TestRunReapsBackground' -count=1 -v` | pass | 9.688s；无保护版本真实 race+丢字节，局部 Write 锁后完整512KiB，两个reader/cleanup不锁。 |
| Race | `go test -race ./internal/process` | pass | 16.745s。 |
| Lint / diff | `go vet ./...`、`git diff --check` | pass | 无新依赖，无第二执行器。 |
| 007 actual OnStart regression | `go test ./internal/agent ./internal/pipeline ./internal/process ./internal/scm -count=1` | pass | agent45.058s、pipeline5.703s、process14.476s、scm9.647s；OnStart失败原 progress_error、真实PID/PGID、唯一 Wait/ECHILD 回收证据均通过。 |
| 007 race / vet | `go test -race ./internal/agent ./internal/pipeline ./internal/process ./internal/scm`、同4包 `go vet` | pass | agent51.936s、pipeline8.547s、process18.288s、scm10.495s。 |
| Original Android end-to-end | 主代理以合法 HTTPS、自有JKS/固定Git工程、更新Agent真正Gradle执行及中央产物下载 | not-run here | 主代理正在补验，不能用单元回归替代原症状应用复现。 |

## Output Excerpts

修复前：

```text
真实成功进程被慢日志误判：{Started:true ExitCode:0 Reason:exit Duration:806.26475ms CleanupFailed:false}
```

修复后：

```text
PASS TestRunSlowWriterPreservesBothPipes/slow
PASS TestRunSlowWriterPreservesBothPipes/buffers
PASS TestDrainPipeIdleDeadlineExcludesWriterTime
PASS TestRunReapsBackgroundBeforeSlowWriterReturns
PASS TestRunSharedWriterSerializesBothPipes
ok mybuilds/internal/process 12.103s
```

## Residual Risks

- 原 Android 82行同步中央日志、签名 APK/AAB 与 binary 下载门仍待真实新二进制验证；本报告不宣称 verified。
- io.Writer 本身需由真实消费者有界返回；本修复不通过强关管道或丢弃已读取字节假装任意永不返回 writer 已完成。
- Linux 实际进程组回归尚未在此独立缺陷环境补跑；Darwin 实际运行、原取消和本组停止证据已通过。主代理可用其自有 Linux VM 补核，不改系统服务或信任。

## Recommendation

保留 applied 修复与 partial 验证状态；待主代理原 Android 全场景实际通过后再补本记录并独立提交缺陷，007移植不夹带其未验收文件。不要用增加 WaitDelay 或忽略 ErrWaitDelay 替代完整日志证据。

## 主代理最终补验（2026-10-04T13:26Z）

**最终结果：verified**。前述partial为分区交接时的历史状态，本补记保留原记录并闭合其真实环境缺口。

真正原Android链已用相同配置/签名材料与新Agent二进制复跑：构建62582d29-ac47-46d6-8c56-124c4ebfb6e1，node mac-android-pipefix、独立session、固定SHA7b7480804790a586d214635c1cf8149034b0430b（mybuilds.yml SourceDigest与原症状相同）。真实Gradle完成50任务/BUILD SUCCESSFUL，run exit0/Started/StopConfirmed→artifact3文件→中央完整终态succeeded，不再failed/exit_code0。完整中央76条UTC日志、实际ns与相应cursor/manifest闭合。

三份真正CLI中央下载size/SHA/byte相符；APK8567B SHA2568dc846ca2f3870cd215047fc4479f9aabe25cce68cc907351f976b737b088f1b，AAB7243B SHA256c29d7f873356d959c4c0e21886194c751ab1d11e73a4bd1645a40f240cd071c6，mapping465B包含实际MainActivity。aapt2核验com.example.mybuilds/1.7.0/101（真实项目build.number）；apksigner verify v1/v2、jarsigner和keytool实际APK/AAB同公开测试JKS证书82571519db6c5fc99a8bc05d01a0dd89367e86107bf21af1dbc161167001e142。官方AGP8.11.1运行时bundletool1.18.1读取真正AAB manifest，同package/1.7.0/101。

实际命令：三入口go build为mybuilds007-pipefix/server007-pipefix/agent007-pipefix；CLI trigger android007-signed --build android --idempotency-key android007-after-pipefix，artifact download三次；自有verify-android101007.py含签名/manifest检查实际exit0。证据目录/tmp/mybuilds-mvp.zKtK0e/us1-app007-48aqb9rf/android101-central-download，central-evidence.json/signature-assertions.json及原命令输出，未把metadata或ZIP字符串冒充真正签名/manifest。

Linux补验：CGO0 LinuxARM64 process测试二进制已copy到真正Ubuntu24.04 VM的/tmp/process-pipefix007.test，经LIMA_HOME独立linux006执行全process包，实际exit0/PASS，包含慢writer双流完整、shared writer、持FD idle、正常/取消/忽略TERM/leader已退出后台回收、日志写失败、短timeout、无关PID，以及007 OnStart/Wait一次。非交叉编译即假Linux通过。Darwin相同新增定向race12.073s，B006全包+007四包最终race/vet如上通过。

关闭此缺陷并按相关文件单独本地提交；007整功能/Android取消/双库最终应用门及整个MVP仍各自验收，本verified不扩大到未完成功能。
