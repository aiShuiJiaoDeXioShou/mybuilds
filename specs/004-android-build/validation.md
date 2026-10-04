# 004 Android 验证记录

日期：2026-10-04；基线003已验收b9d23a1，004工作树尚未提交，由主代理最终集成/收敛/提交。

## 规范流程

- specify/plan/tasks/analyze完成，12FR/4SC/10验收场景/14任务/5原则齐全；MYBUILDS_保留前缀阻塞已改APP_VERSION/BUILD_NUMBER并重新分析。hooks={}。
- implement质量清单6/6通过；共享API由主代理冻结。README writer已读；按用户中文约定及主代理明确授权，手工核对中文README，不采用缺失的英语润色附属技能。

## 实际环境与工具

- macOS darwin/arm64；Go1.25.4。测试选JDK17.0.15，宿主原java为GraalVM21.0.2，另安装JDK17/21等。
- SDK根/包来自用户已有安装；实际doctor报告platforms25–36、build-tools29.0.2/30.0.3/33.0.1/34.0.0/35.0.0/35.0.1/36.0.0。
- 真实工程锁Gradle8.13、AGP8.11.1、compile/target35、build-tools35.0.0；官方wrapper jar SHA25681a82aaea5abcc8ff68b3dfcb58b3c3c429378efd98e7433460610fecd7ae45f通过。Gradle分发SHA已锁research，所有构建显式禁止SDK下载。
- Gradle/AGP已有缓存。未安装/更新系统SDK，不写用户工程，不读取未知密钥；临时JKS由本次生成，store/key密码不同。

## Go 与行为验证

- 新增测试先运行，缺Android API产生真实红测；实现后Android测试通过。
- 最新go test ./internal/mobile -run '^TestAndroid'：7.875s；非私钥证书条目扩展后真实签名测试3.377s；最新Android race10.800s、go vet ./internal/mobile通过。
- 严格Parse/参数映射/副本、九类工具错误、安全输出、SDK链接同根、开始前取消；无效/注入/溢出构建号在真实shell拒绝且wrapper未启动。
- 真实JKS验证正确store/key、错误store/key/alias、证书非私钥、缺失/非法env；doctor不修改keystore。cleanup_error优先保留，随后检查不再调用外部工具。
- 主代理共享process/doctor工具helper与CLI测试通过：32KiB限制/15s预算/九项env/进程组清理、native init/default/冲突/已有文件/非法选项、doctor JSON安全非零。主代理负责最终全量race/vet/跨平台编译及记录。

## 真实完整CLI验证

客户端：`/tmp/mybuilds-mvp.zKtK0e/mybuilds-android`。
临时工程：`/var/folders/kq/z736pkm90hzf78qm_3hffry00000gn/T/mybuilds-android004-w7llynth/project`；证据脚本/日志/JSON与测试JKS在上级临时目录，私钥不提交。

1. doctor未声明签名：Java17.0.15/SDK库存/实际Gradle8.13 passed，签名skipped/not_declared；显式签名flags后四项passed，退出0。
2. init native/android实际生成builds.android。真实首次wrapper --offline构建13s成功；完整mybuilds run --param version=1.2.3 --param build_number=42：11.591s、run/artifact均succeeded。
3. 可编辑普通YAML增加--offline，再次完整mybuilds构建成功，52个UP-TO-DATE；19个已安装SDK包source.properties大小/mtime记录相同，未更新/增加SDK。
4. APK aapt实际元数据：com.example.mybuilds、versionName1.2.3、versionCode42；apksigner verify通过v1/v2，单个RSA2048签名。证书SHA25621bf565a72d087423ff6b2909d973b011183355e29b0d32e517d9e252a4981cb。
5. AAB jarsigner实际报告jar已验证；测试证书为自签名且30天有效，信任链/时间戳提示不代表商店发布授权。复用AGP runtime官方bundletool1.18.1的DumpCommand，对成功AAB快照dump manifest，实际核对com.example.mybuilds/1.2.3/42；模块与Google官方摘要相符。manifest.xml及原命令输出保存临时证据目录。
6. mapping465B非空且含com.example.mybuilds.MainActivity。三份独立快照重算实际大小/SHA256全部相符，源码工程从未作为不可变证据。

| 产物 | 大小 | SHA-256 |
|---|---:|---|
| APK | 8508 | 6914699dcd23302111110a46b2ec6363fbba5e58536fd9d2d6bb617a39e617c0 |
| AAB | 7171 | a37f98af954a020c38ebfd74531b38d27107690dc4604cde1da97f4a4fb5d7cb |
| mapping | 465 | dae525e42d4beedaa2f7cde5f133962189c428d553e13a3d7c2573889fc8c51e |

初次CLI result_dir：`/private/var/folders/kq/z736pkm90hzf78qm_3hffry00000gn/T/mybuilds-3331792799`。
离线结果及重算快照：`/private/var/folders/kq/z736pkm90hzf78qm_3hffry00000gn/T/mybuilds-181437010`，offline-result.json/offline-log.txt保存在证据目录。

## 真实失败和取消

- 对临时YAML增加--rerun-tasks，避免已有签名输出缓存掩盖错误，分别传错误store/key密码。两次真实Gradle构建均failed/exit/非零，artifact未运行、零产物记录，完整输出无测试密码。
- 失败result_dir分别mybuilds-2683306575/mybuilds-2690982901；store/key-failure-result.json及脱敏日志保留。
- 观察真实Gradle client与单次daemon两JVM后发送SIGINT；两个本次PID5958/6000均停止，结果cancelled/cancelled、cleanup_failed未出现，另起无关sleep仍存活后仅清理本次测试sleep。取消证据cancel-daemon-result.json/log.txt，result_dir为mybuilds-439161662。
- 初始JVM启动时取消也正确，PID5287停止，result_dir为mybuilds-2176523087。取消/失败不覆盖前述成功快照。

## 范围与最终集成

真实四项SC及全部实现任务已闭合；主代理集成验证完成，收敛后执行一次本地提交，不push。

GitHub可选bundletool1.18.2 CLI下载45s有界重试失败、完整慢下载已仅取消本次工具会话；未下载文件不当作证据。实际验收使用已缓存官方AGP模块，不新增产品运行依赖。

## 主代理最终集成验证

- `go test ./...` 全通过（mobile 24.800s），`go vet ./...` 全通过。
- `go test -race ./internal/process ./internal/mobile ./internal/pipeline ./internal/cli/client` 全通过（最新mobile 28.030s）；进程提取保留短超时EPERM修复及真实取消回归，无第二执行器。
- Linux/amd64和Windows/amd64客户端编译通过；这仅是可编译验证，真实Linux宿主移动构建随007双节点验收继续，不冒充已执行。
- 主代理读取离线成功快照，重新核对3份大小/SHA256、APK aapt元数据及apksigner、AAB jarsigner与官方模块生成manifest、mapping，以及两次真实签名失败/实际Gradle取消JSON，全部通过。
- 根README已同步Android入口、JDK/test前提、wrapper可能下载、模板消费约定、示例与process目录。未把未来iOS/Flutter、服务端或商店能力写为已实现。

## Spec Kit 收敛

主代理完成implement后，按converge检查当前实际代码：12 FR、4 SC、10验收场景、14任务、8项计划决策和5项原则均满足；missing/partial/contradicts/unrequested均为0，无阻塞。没有追加空Convergence阶段或改写tasks；检查前后SHA256均为16af9e68b8c774371444a6721217006557a38000d5894cdc737d73e8f74f232b。hooks={}。提交紧随收敛，由主代理执行。
