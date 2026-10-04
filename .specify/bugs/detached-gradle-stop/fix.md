# Bug Fix: 脱离原组的构建工具停止确认

- **Slug**: detached-gradle-stop（任务明确指定）
- **Fixed**: 2026-10-04T16:18:28Z
- **Assessment**: ./assessment.md
- **Status**: applied

## Summary

在唯一 process.Run 中加入本次运行的私有归属证据。原组回收后仍须按 kernel birth 终止并确认实际 detached 成员消失；不可确认时沿原 CleanupFailed 保留停止保护。没有修改公共 Command/Result、执行器、模板、008 源码或依赖。

## Changes

| File | Change | Notes |
|------|--------|-------|
| internal/process/process_unix.go | 修改 | 启动前独立随机继承标记；绑定 birth 后才允许清理；同一个 cleanup/Wait；输出隐藏该标记 |
| internal/process/scope_unix.go | 新增 | 本次私有成员与暂态候选；实际父链/birth；TERM/KILL 后以真实对象消失确认；跨块内部值脱敏 |
| internal/process/scope_linux.go | 新增 | 受限 procfs 读取、真实 UID/start_ticks、pidfd 固定对象信号；Z 保留到 Gone |
| internal/process/scope_darwin.go | 新增 | 实际 KinfoProc birth/父链、procargs2 精确标记；EIO 暂态候选；信号前后 birth 复核 |
| internal/process/detached_test.go | 新增 | 实际 setsid、双重派生、取消/超时/正常 leader 退出、并行隔离、birth 不匹配、标记脱敏 |
| internal/process/scope_linux_test.go | 新增 | 实际 PR_SET_DUMPABLE=0 不可读负例、pidfd birth 不匹配 |
| internal/process/scope_darwin_test.go | 新增 | SIP 系统程序实际 birth/父组清理 |

## Tests Added or Updated

真实 helper 先在旧实现产生红回归：父组已消失而独立 PGID helper 仍存在。新门要求 Run 返回时 kill(pid,0) 已 ESRCH，同时自有无关 sleep 仍存活。双重派生的中间进程实际退出，最终成员独立 setsid；没有 mock executor 或测试执行接口。

旧 process_unix_test.go 和 pipe_test.go 完全保留。原慢 stdout/stderr 完整排空、同 writer race、OnStart 回执失败只 Wait 一次、日志写失败取消组等回归继续执行。

## Local Verification

- `go test -race ./internal/process` → PASS，24.306s。
- `go test ./internal/process ./internal/pipeline ./internal/agent ./internal/mobile` → PASS，分别 19.875s、5.044s、47.101s、26.764s。
- `go vet ./internal/process ./internal/pipeline ./internal/agent ./internal/mobile` → exit 0。
- Linux ARM64 原生完整 process 测试二进制 → exit 0/PASS；由实际 Lima Linux006 执行，自有 /tmp 路径，不触在线 Agent。
- `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c ./internal/process` → exit 0；该平台原无 process 测试。
- `git diff --check` → exit 0。
- 四包 race 已启动，最终结果记入 test.md。

## Deviations from Assessment

没有扩大文件范围。实现中真实 race 暴露 Start/Cancel 与 birth 绑定的竞争，增加每次 Run 的绑定完成通道，避免清理使用未初始化 root，且仍只一次 Wait。

Darwin procargs2 在 shell 子进程 exec 用户栈切换时真实返回 EIO；单次永久闭锁误伤正常运行。原组成员已有实际组/父链证据，独立候选的 EIO 则记录 pid+birth，只有后续真实环境可读并完成归属判定，或同 kernel 对象 Gone，才解除本条不明。最终仍存活且不可读、全表扫描失败、EPERM、限额或信号不明保持 CleanupFailed。对应重复短期限/日志失败 race 门通过；没有用固定等待次数推断停止。

Linux 已退出 Z 对象的 environ 可 EACCES；只有真实 stat 复核已退出才不读其失效环境，birth 仍保留至 Gone。活对象 PR_SET_DUMPABLE=0 的真实负例仍闭锁。

## Follow-ups

父代理/A 使用冻结源码构建的新 Agent 重跑原 Linux AMD64 Gradle 取消/超时；须真实证明 nonce 被单次 daemon 继承、daemon 消失不晚于 finished At、无关组存活。取得该门前验证结果保持 partial。

标记仅覆盖实际继承环境的可信构建工具，不能作为恶意清环境、提权或权限隔离保证。Darwin birth 核对与 kill 之间仍有平台 TOCTOU 限界；没有宣称 pidfd 同等绝对对象固定能力。实际探针见外部自有目录 /tmp/mybuilds-mvp.zKtK0e/detached-probe007。

## 后续集成发现（追加，不覆盖原报告）

根在实际20并发CLI/Git race门复现了Darwin procargs2成功返回不完整argc/argv导致的全scope永久uncertain。修正仍限原评估列出的scope_darwin.go与scope_darwin_test.go：私有darwinScopeMark严格区分完整可读与缺栈，同birth缺栈候选交已有unknown map，只有实际可读或对象Gone才清。活候选不得猜停止/发信号，范围越界仍硬失败。真实live-unknown门与完整/非本scope/不完整栈解析门新增，旧测试不变；目标race及全量test/race/vet通过，详情见test.md。

Linux实际SSH并发诊断又观察到同UID新生live对象environ EACCES。根批准以同对象复核的unreadable候选替代该种全scope永久错误；实现与原工程验证正在继续，未改assessment，最终字节与验收将在后续追加。该修正不能忽略永久PR_SET_DUMPABLE=0，不可读且存活必须仍保留停止保护。

Linux最小修正已实际集成，仅scope_linux.go/scope_linux_test.go。私有linuxUnreadableProcess复核原birth；只有权限拒读的同对象live才标unreadable，真实对象Gone/出生变化不归属，已退出对象继续保birth直到Gone，其余IO或metadata错误仍硬失败。新增真实setsid/PPID1/PR_SET_DUMPABLE=0的持活未归属门与候选真正Gone解除门，旧直接已知原组负例改为确认该组已Gone时cancelled/!CleanupFailed；这不是放宽不可读活对象，新增持活未知门仍stopfalse/noSignal。两文件冻结SHA为8e363f735fcef2f4736bb8dfd466e5e60a30b4a7b98f731a3a6a95ebafcce00e、7725978ae91b9ad28c2a0a7bce4745da3a5f8df43e7b4dc0aebd720202af50f4，未扩大原评估的文件范围。

## Darwin SIP 环境边界（进行中）

完整argv且空环境不能证明标记已核验。新增真实独立/SIP对象门在前版实际red，空环境保unknown后定向race通过5.061s；随后全process race实际FAIL25.985s（短期限第80轮CleanupFailed），新增两个正常并行系统shell/睡眠门实际FAIL，不能将该候选修正当最终通过。正在核对实际父链排除外部对象；PID1重父对象仍必须未知保护。008 T035/T037重新打开，旧全量、72应用与matrix记录保持，但不当后续Darwin字节的最终门。无提交/无原204保护解除。

## SIP 父链候选修正与最终字节

根在原七文件范围内修正：完整argv但空环境、EIO/EINVAL/ESRCH且同birth仍活均交unreadable；仅对此类对象逐Kinfo实际核对≤64层非零birth/父关系/存活/无P_TRACED及P_oppid，严格早于本次root的非PID1外部祖先再复核全路径才解除unknown，不移除marked/owned、不缓存外部身份。PID1重父、读失败/变化/未就绪/追踪仍保unknown且不信号。普通孤儿/ptrace例外核对官方[XNU exit](https://raw.githubusercontent.com/apple-oss-distributions/xnu/main/bsd/kern/kern_exit.c)、[ptrace](https://raw.githubusercontent.com/apple-oss-distributions/xnu/main/bsd/kern/mach_process.c)、[Kinfo导出](https://raw.githubusercontent.com/apple-oss-distributions/xnu/main/bsd/kern/kern_sysctl.c)，属于普通可信工具条件的保守判断，非永久内核历史证明，Darwin TOCTOU限制仍保留。

新增真实/bin/sh父退出→PPID1的SIP睡眠对象不可误确认/误杀，以及两个正常SIP shell/sleep并行互不影响。父链候选完整process race27.501s PASS；整路径/oppid/非零birth复核后Darwin定向race5.136s PASS；最终errno闭锁字节另跑必要全门，尚未将前次源码检查计为最终。冻结清单SHA256 `8eed83099eb5286801cda63734a01ca3ce5f20a06e1a5d14353ba87c488e40b4`，Darwin源码 `da1721aa4d47d66cb85ec47878d5365cc2277c61695c8d31eb8e481e87b2cbca`、测试 `e57547715710d298a5c5fde75529c89a17cd36160263c9eb60ec96ca4ba88533`；Linux最终两文件不变，原native/206证据仍适用。207正常签名回归与最终统一门仍在运行，无提交。

## 重新评估：正常TLS派生系统服务

串行-p1全包仍FAIL179.102s，normal日志SHA `98bc2b65e13beee829bfd09de2ae274214504cfb0e0b466a85f336a04b67741f`；race/vet后续未跑。故“只因故障注入互相影响”的初步推断不成立。单独SCM TLS负例真实重现；仅记录pid/parent/group/birth/stat/flags/短comm的私有临时诊断，发现trustevaluationa新生PPID1/独立PGID/无可读环境，扫描unknown导致正常TLS失败也cleanup_failed；不是扫描whole error、不允许按名字忽略。诊断日志 `/tmp/mybuilds-mvp.zKtK0e/darwin-sip-scm-diagnostic.txt`。现有父链无法区分launchd真实服务与本次SIP重父工具；原preferred需补精确内核原父身份能力重新评估，停止继续改修复源码，先实际验证Darwin proc_info parent unique identity可用性。所有临时诊断已移除，七文件逐SHA恢复冻结字节；结果仍partial，008验收未通过/无提交。
