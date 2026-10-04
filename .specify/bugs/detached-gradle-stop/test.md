# Bug Verification: 脱离原组的构建工具停止确认

- **Slug**: detached-gradle-stop（任务明确指定）
- **Tested**: 2026-10-04T16:18:28Z
- **Assessment**: ./assessment.md
- **Fix**: ./fix.md
- **Result**: verified

## Summary

原LinuxAMD真实Gradle206取消症状已通过严格同birth停止时序复验；最终统一源码又在原生LinuxARM真实签名401和取消402通过。中央APK/AAB/mapping、签名、版本、完整fullRef/事件、daemon早于finished.At消失及无关进程存活全部有原始实证。最终Darwin/Linux原生与完整normal/race/vet通过。207旧源码清理失败仍属未知，原guard/journal未解除；以下保留原失败及阶段性partial记录。

## Checks Performed

| Check | Command / Action | Result | Notes |
|-------|------------------|--------|-------|
| 旧实现红回归 | TestRunDetachedMembersStopBeforeResult | fail（预期） | 原组结束后 setsid helper 仍存在 |
| 修复后真实重现 | go test ./internal/process -run '^TestRunDetachedMembersStopBeforeResult$' -count=1 | pass | 四分支实际组、Gone、无关 sleep |
| 本机完整回归 | go test ./internal/process ./internal/pipeline ./internal/agent ./internal/mobile | pass | 19.875s / 5.044s / 47.101s / 26.764s |
| 本机 process race | go test -race ./internal/process | pass | 24.306s；包括原 slow pipe/same writer/OnStart |
| 四包 race | go test -race ./internal/process ./internal/pipeline ./internal/agent ./internal/mobile | pass | process cached；pipeline 10.342s / agent 55.595s / mobile 30.816s |
| 暂态边界重复门 | go test -race ./internal/process -run '^TestRunShell(ShortTimeoutConfirmsCleanup\|LogFailureCancels)$' -count=10 | pass | 50.242s；已有短期限循环，无数据 race |
| Linux ARM 原生完整测试 | /tmp/process-detached-linux-arm64.test -test.v | pass | Lima Linux006 实际 exit 0/PASS；不可读负例、pidfd、双派生均执行 |
| 静态检查 | go vet ./internal/process ./internal/pipeline ./internal/agent ./internal/mobile | pass | exit 0 |
| Windows 编译 | GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c ./internal/process | pass | 原平台无测试，编译 exit 0 |
| 差异检查 | git diff --check | pass | exit 0；既有 process/pipe 测试未修改 |
| 原实际 Gradle 门 | 新 Agent 执行原 AMD64 Android，连续采样 daemon/birth/nonce 与 finished At | not-run | 父代理/A 接管；尚无最终新二进制证据 |

## Output Excerpts

```text
ok mybuilds/internal/process 24.306s
ok mybuilds/internal/pipeline 10.342s
ok mybuilds/internal/agent 55.595s
ok mybuilds/internal/mobile 30.816s
Linux native complete process binary: PASS, exit 0
```

本机 Go 1.25.4 darwin/arm64、Darwin 27.0.0；实际 Linux ARM64 kernel 6.8.0-146-generic。Linux 完整原始输出保存于外部自有 `/tmp/mybuilds-mvp.zKtK0e/detached-probe007/linux-process-final.txt`，SHA256 `fcbafacf8fe76f3af22c6ccbbc6a6c623d7f25d79ea9c5c3cdb9d4550809344e`。实际运行测试二进制 SHA256 `31fdfe996cbf02e288cec81993b804291e9878e41d121a3914b7d3fc085ac7ee`。

冻结七源/测试清单 `/tmp/mybuilds-mvp.zKtK0e/detached-process-batch1.json`，SHA256 `88ce861ee3199e9e9b7b698b28b15c0a28a9779f02c9f5ca6d9c75d7585d724e`。检查期间源码未改写，没有提交或触及在线 Agent/数据库/控制端。

## Residual Risks

- 原实际 Gradle 单次 daemon 的继承与停止时序未完成复验，必须核对同 start_ticks，不将 PID 重用、发送信号成功或仅 API StopConfirmed 当物理停止。
- 只覆盖实际继承本次标记或有真实父链证据的可信工具，不是恶意清环境、提权、未知逃逸的隔离保证。
- Darwin SIP 可以省略系统程序环境，需实际父链/原组证据；Darwin birth 核对与 kill 有平台 TOCTOU 限界，不宣称 Linux pidfd 同等保证。
- 全表/权限/限额/仍存活不可读候选等无法确定时必须 CleanupFailed，保留停止保护。Linux 活对象 PR_SET_DUMPABLE=0 负例已实际证明该行为。

## Recommendation

保持 partial，先集成冻结源码供原实际 Linux AMD64 Android 门使用。取得同一 daemon birth 的真实继承、daemon Gone 不晚于 finished At、无关进程存活与正常后续构建回归证据后，由主代理补齐记录并决定 verified/提交。不要在缺少原工程证据时关闭缺陷。

## 后续检查（追加，结果仍partial）

- 第一批冻结Linux源码在实际AMD64运行完整26项process测试PASS；实际ELF SHA2b82be11b1482f52cafb5b295229126b25214c4abd27a8b5ed360215f1b7c3cc。ARM通过不能替代该平台门，AMD测试也不能替代真实Gradle门。
- 203实际Gradle shell/wrapper/daemon继承本次nonce，中央cancelled；连续采样在CLIcancel之前已结束，未覆盖finished.At，故partial。204在checkout阶段停止未确认，未执行用户步骤；原保护保持。安全证据归档SHA54f9d2473c9ae8c950c59c6319ca5ea0c385856eb633913ca14f277db2caf5f8，交付表SHAd69d42a32e512f2d37d6ce4e15d778310b502bc92bb743274201c08c1289506e。没有将晚时Gone或中央StopConfirmed当作所需时序。
- 第一批集成完整race中的既有20并发CLI触发门实际失败；root安全诊断证实Darwin成功返回短argc/argv，scope错误永久闭锁。最小候选修正后目标race2.690s/5.874s、Darwin/Detached/Shell race18.469s通过，全量test/race/vet均通过（process21.481s/race25.546s、Agent120.290s/race135.988s、client36.381s/race43.938s）。原临时诊断完全移除，不记录argv/环境。
- Linux实际诊断基线24个Git全部正常；并发自有新SSH时2/24 cleanup_error，捕获live同UID同birth environ EACCES(errno13)。后续Linux候选修正尚待最终实际复验；既有活对象不可读负例不得放宽。仍不能关闭缺陷或提交未验收修复。

- 新Linux候选门在原冻结源码真实red（单对象权限暂态永久whole-scope uncertain），在最终两文件真实AMD四门green。Linux ARM完整process实际PASS；AMD全套首次一个既有detached/cancel helper未能在10s内READY，未达到cleanup路径且不能推为修复导致，失败原日志保留并单独复验。完整AMD与SSH24组结果待后续记录，不以定向绿代全门。
- 根最终统一process字节集成后，Darwin全量test/race（有效Go缓存）与vet exit0；当前源码的SQLite/PG三入口应用各36/72PASS，evidence SHAbebb01a8ba833b09c9da2eef9c35cc2312b8908766910fc489ced2f087cf7f37。三入口四平台12构建/本机6help-version全部exit0，evidence SHA93af41d1499cc7dca7f50657d4cc7f44f7ed7db29d747e901e55a5fdaefb8fa5。这些不替代原Gradle停止时序，结果仍partial。

## 最终原生与原工程取消证据（正常签名回归仍待207）

最终Linux AMD/ARM完整native各29项PASS，真实SSH并发readonlyGit24/24无CleanupFailed，Linux vet通过；汇总SHAd8b5000fcd7c15d8037d78d06b19f176264a38df7522cad59c89d94dc707cfda，所有失败/夹具握手偏差保留。根逐SHA核验两份完整日志和并发绿日志，不将阶段定向门冒充完整native。

真实Gradle206固定原SHA76a0/1.8.0/30m，新独立已登记node/project/data，Agent SHAb78cdcf1a3e4bd7d5a45c4e58116bcf27195e5ac69f5858e895dab746bb9445d。实测shell26236/birth2527095、wrapper26238/birth2527105、daemon26264/birth2529181均继承精确本次标记（只记录匹配布尔）；daemon独立PGID/SID26264。一次真实CLIcancel后daemon Gone上界17:31:56.567651874Z，早于ordinary.finished.At17:31:56.719311583Z共151659709ns，shell/wrapper也已Gone，无At后同birth活对象，Z从未当Gone。无关sleep26166同birth2523647持续活，未人工发信号。terminal.At17:31:56.890370780Z，采样保留至2.049s之后。

driver收尾ready布尔解析导致exit1，保留原脚本/输出/异常；原完整87208B/972probe采样及精确seq1–6/fullRef/StopConfirmed/!CleanupFailed没有丢失。根独立解析原285行并严格重新计算，原症状取消门PASS。四安全文件归档SHA91fe7096ad7cd24fe287894d24e366b339b93f62b00e0ac43836af497eeef73b，交付表SHA689b80ad0237615d0c39bc17fc80cc435072971dafb8cdc457b189500e3bf11a，根独立复核SHA5681869f144e50d57fd1a25a1114e4696e3a7b965648a9dfead38e0e8e8c8092。

原取消症状已不复现；207正常签名/中央文件及完整终态回归尚未完成，所以最终Result保持partial，不提交。旧204停止未知保护未解除；新验收不修改其journal/guard，也不伪造旧物理停止时刻。

## Darwin SIP 环境边界（进行中）

完整argv且空环境不能证明标记已核验。新增真实独立/SIP对象门在前版实际red，空环境保unknown后定向race通过5.061s；随后全process race实际FAIL25.985s（短期限第80轮CleanupFailed），新增两个正常并行系统shell/睡眠门实际FAIL，不能将该候选修正当最终通过。正在核对实际父链排除外部对象；PID1重父对象仍必须未知保护。008 T035/T037重新打开，旧全量、72应用与matrix记录保持，但不当后续Darwin字节的最终门。无提交/无原204保护解除。

## SIP 父链候选修正与最终字节

根在原七文件范围内修正：完整argv但空环境、EIO/EINVAL/ESRCH且同birth仍活均交unreadable；仅对此类对象逐Kinfo实际核对≤64层非零birth/父关系/存活/无P_TRACED及P_oppid，严格早于本次root的非PID1外部祖先再复核全路径才解除unknown，不移除marked/owned、不缓存外部身份。PID1重父、读失败/变化/未就绪/追踪仍保unknown且不信号。普通孤儿/ptrace例外核对官方[XNU exit](https://raw.githubusercontent.com/apple-oss-distributions/xnu/main/bsd/kern/kern_exit.c)、[ptrace](https://raw.githubusercontent.com/apple-oss-distributions/xnu/main/bsd/kern/mach_process.c)、[Kinfo导出](https://raw.githubusercontent.com/apple-oss-distributions/xnu/main/bsd/kern/kern_sysctl.c)，属于普通可信工具条件的保守判断，非永久内核历史证明，Darwin TOCTOU限制仍保留。

新增真实/bin/sh父退出→PPID1的SIP睡眠对象不可误确认/误杀，以及两个正常SIP shell/sleep并行互不影响。父链候选完整process race27.501s PASS；整路径/oppid/非零birth复核后Darwin定向race5.136s PASS；最终errno闭锁字节另跑必要全门，尚未将前次源码检查计为最终。冻结清单SHA256 `8eed83099eb5286801cda63734a01ca3ce5f20a06e1a5d14353ba87c488e40b4`，Darwin源码 `da1721aa4d47d66cb85ec47878d5365cc2277c61695c8d31eb8e481e87b2cbca`、测试 `e57547715710d298a5c5fde75529c89a17cd36160263c9eb60ec96ca4ba88533`；Linux最终两文件不变，原native/206证据仍适用。207正常签名回归与最终统一门仍在运行，无提交。

## 最终字节并行检查失败保留

在同机同时运行多个go test/race与真实应用时，全量进程消费者出现cleanup_error/agent_execution_unconfirmed，双库应用第一库lost-terminal门进入interrupted/lease_expired（失败记录 `/tmp/mybuilds-mvp.zKtK0e/app008-sip-w4o3dzlr/failure.json`，源码无修改）。新SIP负例真实制造PPID1活且环境不可读对象，可能使同birth扫描其它正在执行的Run保守闭锁；这个归因目前待串行隔离证实，不能把失败删去或当通过。四次全包退出1保留在工具输出，后两次为最终Darwin字节。三入口12编译/6本机命令最终字节exit0，证据SHA `7d0585d12ce282cf80065cdbde6f91a642c228d351a11bd4ebd9a5a5265696f7`，不代行为验收。正在以-p 1串行整个包集，normal→race→vet及应用互不重叠，保持未知对象failclosed，不改测试断言或超时掩盖。

## 重新评估：正常TLS派生系统服务

串行-p1全包仍FAIL179.102s，normal日志SHA `98bc2b65e13beee829bfd09de2ae274214504cfb0e0b466a85f336a04b67741f`；race/vet后续未跑。故“只因故障注入互相影响”的初步推断不成立。单独SCM TLS负例真实重现；仅记录pid/parent/group/birth/stat/flags/短comm的私有临时诊断，发现trustevaluationa新生PPID1/独立PGID/无可读环境，扫描unknown导致正常TLS失败也cleanup_failed；不是扫描whole error、不允许按名字忽略。诊断日志 `/tmp/mybuilds-mvp.zKtK0e/darwin-sip-scm-diagnostic.txt`。现有父链无法区分launchd真实服务与本次SIP重父工具；原preferred需补精确内核原父身份能力重新评估，停止继续改修复源码，先实际验证Darwin proc_info parent unique identity可用性。所有临时诊断已移除，七文件逐SHA恢复冻结字节；结果仍partial，008验收未通过/无提交。

## 207正常构建反例保留（2026-10-04T18:26Z）

实际Gradle50任务全部执行，45m41s BUILD SUCCESSFUL，但收尾cleanup_error/StopConfirmed=false；中央interrupted、节点隔离、无制品收集。原预算仍848.896s。daemon第一Gone样本在finished后22.386ms，粗采样不能断言瞬间真实状态；scope候选/errno缺失不能归因。安全原tarSHA7b1cf8d9fa09db0e4f7ee582e4cad24af9c909b0fe317844b975c50265ce8a32，公开摘要SHAe256d894accd7a1cf52c6a942e22cf4fc35f2401c8bc8042a61b381bd8462b59。206取消门通过，但正常移动构建验收未闭合，维持partial；旧204和新207停止保护、journal保持，不重复触发或人工信号。

## 移动端质量强化（2026-10-04T19:25Z）

正式speckit-converge逐项核对19FR/5SC/13AC、40项任务、8项设计和5项原则，零missing/partial/contradicts/unrequested，不追加任务。实际prerequisite选择008，hooks={}；报告已在技能外持久为convergence.md，外部review SHAa60a6886ab57b5a307734292fa5ce52d87655b8a44caafd060d6053d54017ff8。收敛前后tasks SHA4cef288f53411644a7a0ce232c9454227cc94152bc494531c6f8c9778864a8aa完全不变；根随后在技能外勾T039，T040待提交。

原生LinuxARM通过最新真实mybuilds/pipeline.Run完成全新Android工程签名构建，Run普通182467ms、artifact1ms均succeeded/noCleanupFailed。Java17/Gradle8.13为nativeARM，官方x86 aapt2经受限QEMU userspace wrapper执行；不能据此宣称所有Android SDK x86工具均受支持。真实R8/lint/assemble/bundle/sign任务、APK8526B/AAB7130B/mapping465B与三个独立snapshot核对通过，版本1.9.0、手动local310（不是中央计数器）、包com.example.mybuilds，APK/AAB证书SHA905af4ece9d46b8ecde47e7573ca05f8565641067365d6cd8064b21136cfadb4。原有许可证缓存未改，不安装系统包/注册binfmt。

安全tar SHA121dc7bb00cfed26a304340fe2dcbbe1f502fdc992a1b633e628bc3b2544b539，35文件交付清单SHAe1748e67a27ac2e0322f9d34816b90b40edde10cf46a56e58340a8ae1fe9a0e5，root全部Size/SHA独立核验；之前工具preflight清单61项也逐项验证，SHA93950d5a5d1e7cdb777cd5c6111c0119409595d6709636f1068672d1ee7a19f6。第一次verify把SnapshotPath误按workspace解释失败，保留工具失败，仅按实际result_dir修正核验，未重跑或替换构建。

local CLI没有finished.At字段，终观察同birth Gone只能证明观察时点，不能伪造远程At排序。新独立ARM节点真实中央签名正常与精确取消正在准备，旧204/207保护与失败证据保持；这些额外质量门及原process缺陷结论仍待后续实证，不等于全MVP完成。

## 最终真实移动端与停止确认验收（2026-10-04T19:40Z）

最新Agent LinuxARM ELF SHA598bcae8c53118867229219ba5de130816c2d5bce9e21a9f666c4ef9157fdf64，可信仓SHA4596f99c04370acf8481e9f3f3648e77166f962f，独立node11f4fcc7-6034-48b0-b586-f7cab74ea313/session4f101990-d1ba-4656-b7f4-995edffc7c69；真实Doctor linux/arm64/java17/git2.43/aapt2/apksigner通过。原AMD的206与最终ARM两个平台实证共同闭合，ARM不是伪装AMD；旧204/207停止保护和journal均未改，无管理员假确认。

正常401 build705e0d15-8df2-4b68-81f8-8d5f0c1b202d：fresh固定Checkout、真实Gradle/R8/lint、两步骤succeeded/StopConfirmed/!CleanupFailed、中央自然号401。CLI真下载APK8521B SHAa48ff315be5f6ee5808ec73f6b8c6ee3b440590009c78d746133d277606df695、AAB7134B SHAcd9b2e3cf7a589b6e33b5c0d00116bc9a9119dabe3ff9d9e03a7793a1fbca718、mapping465B SHAdae525e42d4beedaa2f7cde5f133962189c428d553e13a3d7c2573889fc8c51e；真aapt2/bundletool核1.9.0/401/com.example.mybuilds、apksigner/jarsigner/keytool核原临时证书905af4...，root逐五工具原stdout SHA与内容核对。shell13400/birth3688678、wrapper13402同birth、独立daemon13436/birth3688732三nonceMatch真布尔；三Gone上界在finished.At19:34:08.802944116Z之前231.671197/237.056667/242.546012ms，8seq/fullRef一致、terminal+2.045876874s连续捕获、35次无关sleep13375/birth3686073存活。原sample SHA0bc4a0ef46b25e7f56b9fcf6d9d71401e16563aab54b35048d4de86d5f1ff63c；root独立证明SHAa8ea7c59c6e90a8f1e39b9e917eeeadcaf51d32134ef4c70f0858ae1d2a96269（该阶段证书标pending，后续以下最终报告已核原工具）。

取消402 build7d822507-8794-44b5-b021-72cee4bfc7be：READY19:38:14.122650845Z后实际一次CLIcancel，中央cancelled/cancelled、Started/StopConfirmed/!CleanupFailed、artifact未启动。shell13663/wrapper13665 birth3716137、daemon13699/birth3716184独立PGID/SID及nonceMatch真；三Gone上界分别比finished.At19:38:18.349065972Z早730.933073/389.321054/22.627329ms。6seq/fullRef一致，无At后同birth活对象/人工signal，terminal19:38:18.362656904Z后2.012581242s捕获、1074次实际probe、13次无关同birth仍活。raw sample SHAe5bed66b211017a68e69a60aa6ff412d9ad6a3d579a5d3d7ce4ad4e0b03d0475；root重新计算6事件/身份/精确探测后UTC/Gone及原5工具输出，最终独立报告SHA42a2a8c801cc00df980bb971c7af90818afd999599e8359ff454e509092daf36。

最终统一生产源码normal/race/vet及fresh双库72门、矩阵12/6门见008记录；最终Darwin四门与Linux两平台31顶层原生门均通过。不将207误归因本修复或清其保护。原症状及正常回归已verified，可按这一停止生命周期修复整体本地提交；不代表008以外全MVP完成。
