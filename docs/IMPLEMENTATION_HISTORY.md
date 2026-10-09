# 实施历史

本文件记录功能状态与验收/提交索引，由主代理随集成更新；任务与验证细节保留在对应 specs，不复制任务列表。
MVP 批次、真实验证与完成定义见 [执行计划](plans/MVP_EXECUTION.md)，范围见 [产品计划](plans/PLAN.md)。

| 功能 | 当前状态 | 规范与验证 / 集成记录 |
|---|---|---|
| 000-project-bootstrap | 已完成并验收 | [spec](../specs/000-project-bootstrap/spec.md)、[validation](../specs/000-project-bootstrap/validation.md)；功能提交 0742512；本轮规划基线 a752a67 |
| 001-pipeline-preview | 已完成并验收 | [spec](../specs/001-pipeline-preview/spec.md)、[validation](../specs/001-pipeline-preview/validation.md)；解析/预览/CLI 三分区独立 worktree 交付，全量 test/vet/race 和二进制 quickstart 通过，converge 无缺口；集成提交 `7ffa264`：feat(pipeline): 实现配置初始化与安全预览 |
| 002-local-run | 已完成并验收 | [spec](../specs/002-local-run/spec.md)、[validation](../specs/002-local-run/validation.md)；依赖7ffa264，真实shell/进程/预算/post/日志/CLI、全量test/vet/race与二进制SIGINT/超时通过，converge无缺口；集成提交 `256af8e`：feat(pipeline): 实现本地执行与安全取消 |
| short-timeout-cleanup | 已修复并验收 | [.specify缺陷记录](../.specify/bugs/short-timeout-cleanup/test.md)；003集成时在002基线真实复现Darwin瞬时EPERM，原预算50轮与100轮30ms回归通过；独立提交 `0937e83`：fix(pipeline): 修正短超时进程组停止确认 |
| 003-build-artifacts | 已完成并验收 | [spec](../specs/003-build-artifacts/spec.md)、[plan](../specs/003-build-artifacts/plan.md)、[tasks](../specs/003-build-artifacts/tasks.md)；前置256af8e，collector/执行/日志与CLI独立worktree；全量test/vet/race、真实二进制快照/manifest/hash/post/日志通过，converge无缺口；[validation](../specs/003-build-artifacts/validation.md)，集成提交 `b9d23a1`：feat(pipeline): 实现产物快照与结果日志 |
| 004-android-build | 已完成并验收 | [spec](../specs/004-android-build/spec.md)、[validation](../specs/004-android-build/validation.md)；前置b9d23a1，Android/共享process/CLI独立分区；真实APK/AAB为1.2.3/42，签名/mapping/快照/离线/错误密码/取消通过；根全量test/vet/race与Linux/Windows编译通过；集成提交 `2ab8991`：feat(android): 实现原生构建模板与环境检查 |
| 005-ios-build | 代码与自动检查完成，Apple人工待验 | [spec](../specs/005-ios-build/spec.md)、[validation](../specs/005-ios-build/validation.md)；完整Run/Agent/Store签名资源接线、私有临时keychain/profile与关闭确认；必要normal/race、双库守卫、三入口编译/vet通过，收敛无代码缺口；合法Apple IPA与签名材料按用户安排人工验收；本地提交 `8e6240f`：feat(ios): 实现签名资源管理与构建模板 |
| 006-control-plane | 已完成并验收 | [spec](../specs/006-control-plane/spec.md)、[plan](../specs/006-control-plane/plan.md)、[tasks](../specs/006-control-plane/tasks.md)、[validation](../specs/006-control-plane/validation.md)；003/004基线，20FR/6SC/42任务，零阻塞；store/server+SCM/config+CLI独立worktree；全量/race/vet、真实双库各37项及Linux37项CLI闭环PASS；converge无缺口；集成提交 `8e1397e`：feat(server): 实现控制端与持久化队列 |
| 007-node-agents | 已完成并验收 | [spec](../specs/007-node-agents/spec.md)、[plan](../specs/007-node-agents/plan.md)、[tasks](../specs/007-node-agents/tasks.md)、[validation](../specs/007-node-agents/validation.md)；28FR/7SC/17AC、74任务；Store/Agent/HTTP-CLI三独立worktree冻结交接；真实三Mac/Linux节点、双库各51项、七种故障/进程停止、日志/SSE/完整快照/中央下载、Android签名101及取消103与Linuxordinary/always取消通过；全量test/race/vet和三入口12次跨平台构建通过，converge零缺口；集成提交 `85b46bf`：feat(agent): 实现多节点构建与中央证据 |
| 008-build-recovery | 已完成并验收 | [spec](../specs/008-build-recovery/spec.md)、[tasks](../specs/008-build-recovery/tasks.md)、[validation](../specs/008-build-recovery/validation.md)、[convergence](../specs/008-build-recovery/convergence.md)；19FR/5SC/13AC、40任务，Store/Agent/HTTP-CLI分区；最终双库72应用、Linux关键路径/负例、12编译/6入口、全量normal/race/vet与零缺口收敛通过；真实ARM中央签名401/取消402强化通过；集成提交 `504dc6f`：feat(recovery): 实现重启核对与原快照重试 |
| postgres-fixture-session | 已修复并验收 | [缺陷验证](../.specify/bugs/postgres-fixture-session/test.md)；真实全量检查发现跨库锁会话误选，夹具改为自身连接PID；目标双库、全量Store/race及自有控制端存活通过；独立提交 `b2e659a`：fix(test): 限定数据库故障夹具自身会话 |
| 019-test-reports | 已完成并验收 | 基线008 504dc6；[spec](../specs/019-test-reports/spec.md)、[tasks](../specs/019-test-reports/tasks.md)、[validation](../specs/019-test-reports/validation.md)；18FR/6SC/12AC、46任务与正式analyze零阻塞。本地8场景、双库各92应用检查点、最终20故障192断言、macOS/Linux及全量test/race/vet/12编译通过；Spec Kit收敛无缺口；集成提交 `fee97e8`：feat(reports): 实现本次测试报告与中央证据 |
| 020-project-retention | 验收通过 | [spec](../specs/020-project-retention/spec.md)、[tasks](../specs/020-project-retention/tasks.md)、[validation](../specs/020-project-retention/validation.md)；前置008/019已验收，正式plan/tasks/analyze零阻塞；Store/Agent/边界三worktree与root共享串行。真实策略/继承与候选、资源登记、中央读/物理清理/管理CLI与60s后台已接通；双库Store与Mac/Linux实际文件/3×100TCP竞争通过，节点分段删除/重启/20次确认已通过Mac/Linux专项门，有界续扫修复已通过最新完整双库Store normal/race/vet；最终macOS/Linux各双库三CLI门、全量普通/必要race/vet和18编译已通过，Spec Kit收敛无缺口，整功能本地提交 `2602094`：feat(retention): 支持项目保留与节点清理 |
| 009-flutter-builds | 代码与自动检查完成，双平台签名人工待验 | [spec](../specs/009-flutter-builds/spec.md)、[validation](../specs/009-flutter-builds/validation.md)；单YAML单双平台/变体、scoped参数、真实SDK环境与框架匹配、唯一Run/报告/原生签名资源；005联合Parse/CLI/能力与必要双库检查通过，无新增Go依赖；本地提交 `802cb0c`：feat(flutter): 实现双平台模板与框架调度 |
| 010/011 主商店分发 | 代码与自动检查完成，商店人工待验 | [Google验证](../specs/010-google-play/validation.md)、[Apple验证](../specs/011-app-store/validation.md)；第三方分区已核对集成，共同Store/Agent/Run/HTTP/CLI、原产物与报告保护已接线；本地提交`1d1e323`：feat(distribute): 实现双商店发布与结果核对；商店真实操作由用户人工验收 |
| 012 可复用方案/custom | 代码与必要自动检查完成 | [规范](../specs/012-custom-workflows/spec.md)、[验证](../specs/012-custom-workflows/validation.md)；固定SHA来源、内置/文件方案、原发布链custom及metadata query；双库普通/race/vet通过；本地提交`76d3000`：feat(workflows): 实现可复用方案与自定义发布；人工平台验收独立待验。 |
| 014 审批 | 代码与必要自动检查完成 | [规范](../specs/014-release-approval/spec.md)、[验证](../specs/014-release-approval/validation.md)；精确审批/原节点续执行/累计预算/原报告与多epoch发布历史、本地TTY；真实重启与报告负例、双库race、最终三CLI联合118项与18编译通过；合法签名/商店/完整人工矩阵待验；本地提交`cd58300`：feat(approval): 实现原节点审批与续构建。 |
| 015 Webhook | 代码与必要自动检查完成 | [规范](../specs/015-webhook-trigger/spec.md)、[验证](../specs/015-webhook-trigger/validation.md)；四来源有限认证、固定窗口/去重/changes、profile与原唯一执行链；双库三CLI联合118项、必要race/vet与18编译通过，converge无代码缺口；外部push/完整人工矩阵待验；本地提交`a5aecf2`：feat(webhook): 实现去重触发与变更筛选。 |
| 013、016–018 | 未开始，后置 | 不作为本轮 MVP 完成前提 |

更新一行时补充实际 specs 链接、当前阶段、验证结论/证据位置、缺失真实条件及功能集成提交；验收未闭合不记“已完成”。提交哈希来自真实 Git 记录，规划状态不代表代码可用。

019从已验收008 504dc6进入implement，A/B/C独立worktree唯一writer交付，root串行集成；原18FR/6SC/12AC保持，最终源码/二进制与双库、Linux及20故障门验收已通过，正式收敛无缺口，整功能提交fee97e8完成，当前020正式plan/tasks/analyze已通过并进入implement，中央清理与墓碑消费者已接通并通过专项门；PostgreSQL实际文件/控制失锁/确认故障和Mac/Linux独立节点删除专项门已通过；Agent主循环与四组实际三CLI均通过，正式收敛无缺口，整功能本地提交；剩余模块转实现与统一人工验收。

008提前规范在独立worktree `/tmp/mybuilds-mvp.zKtK0e/recovery008-planning` 完成19FR/5SC/13AC、质量16/16，hooks={}；仅specify，待007验收后plan，真实恢复/原快照retry门不变。009 Flutter规范在独立worktree `/tmp/mybuilds-mvp.zKtK0e/flutter009-planning` 已保存28FR/8SC/19AC、质量16/16，仅specify；当前现有Flutter3.38.6真实工具已核对，自有双平台空工程创建成功，未构建签名或验收。

共享process慢日志排空缺陷按SpecKit bug-assess→fix→test完成，Darwin/Linux真实进程回归及原Android远程签名/日志/中央下载通过，独立本地提交 `58bc9c8`（`fix(process): 防止慢日志回传误判成功进程`）；不包含007未验收实现。验证见[缺陷记录](../.specify/bugs/slow-log-pipe-drain/test.md)。

020保留策略规范在独立worktree `/tmp/mybuilds-mvp.zKtK0e/retention020-planning` 已完成25FR/7SC/16AC、质量16/16，仅specify；可信终态时间、跨build筛选、删除保护及Agent幂等门已明确，待008/019验收后plan/实现。

共享process派生工具/原组最终停止确认修复已通过原AMD取消206、最终ARM签名中央401/取消402、Darwin及两Linux原生门，完整test/race/vet；本地提交 `dd8fb4a`（fix(process): 修正派生工具与最终停止确认）。三项关联缺陷均verified，旧204/207保护未改，原因不回填，见[缺陷验证](../.specify/bugs/detached-gradle-stop/test.md)。


2026-10-05实施依赖细化见MVP_EXECUTION：整功能真实验收门保持，缺Apple/商店材料时可在独立worktree准备不消费缺失前置的组件。005先规划当前基线最小移植；009按已验收004/007接口形成独立诊断/Android部分plan/tasks，iOS完整签名与双平台验收仍待005。不开放缺前置入口，不把分区实现或规划当功能交付。

发布HTTP正文限额缺陷按Spec Kit bug-assess→fix→test完成：仅匹配发布路由后设置其64KiB上限，普通API保1MiB/413；原实际HTTP红→绿、发布相关回归与vet通过，见[缺陷记录](../.specify/bugs/publish-route-body-limit/test.md)。

发布HTTP正文上限修复已本地提交`55750d1`：fix(server): 限定发布路由正文上限。

本轮001–012、014–015、019–020共16个模块代码已交付；合法Apple/Flutter签名、真实商店发布及外部投递按[集中案例](../examples/mvp/acceptance.md)人工验收。未执行矩阵保留待验标记，不将代码完成等同所有人工验收通过。

2026-10-05 README验收说明更新：补本地、双平台案例、控制端/两节点、internal与store、扩展/故障五阶段及通过标准；同步案例的工具复制、多终端token、真实返回ID与应用doctor说明。三个CLI构建/help/version、本地普通/post四快照内容/大小/SHA-256、双平台dry-run通过；80处链接/锚点、33个bash代码块语法和9组实际CLI帮助核对通过。本次仅更新文档，未执行真实签名或商店发布，人工项仍待验。

2026-10-05主分支与文档整理：经用户授权，main从初始化基线6c13f93快进至43fc5ab并切换主工作区；16个MVP功能提交均在main历史中。README改为用户入口，验收步骤/标准/记录模板移至docs/ACCEPTANCE.md，详细运行与开发说明分别保留于docs/USAGE.md、docs/DEVELOPMENT.md；两处规划链接已更新。实际三个CLI help/version、默认init/dry-run/run及参数/when/post示例通过，117处链接/锚点与29个bash代码块语法通过；仅整理文档，生产代码未改动，真实签名/商店待验标记保留。

2026-10-08 凭据读取精简（022）：完整 Spec Kit 与 converge 无缺口，主工作区 main、基线 `22e99ee`；[规范](../specs/022-credential-simplification/spec.md)、[验证](../specs/022-credential-simplification/validation.md)。删除秘密选择的不可达错误通道和重复排除规则；Agent 配置不再提前读取秘密；只读运行材料接受 0400/0600，保留鉴权、基础日志脱敏与可写状态/停止/恢复保护。八项红→绿读取检查、真实缺失/公开秘密零外部动作、并发隔离及五包必要 race、vet 和六个跨平台编译通过。全量首轮其余包通过，客户端旧诊断测试受本机部署影响，隔离凭据后整个包重跑通过，原非零结果保留在验证记录。不新增依赖或防护开关，不执行外部商店发布。

2026-10-08 JUnit报告数量增量（019）：默认64→256，每个build的reports.junit.max_files可设1–1024；扫描/恢复/Agent/Store/终态和发布引用统一数量，普通制品128份另外计数。报告消息8MiB、执行journal64MiB，字节/cases/诊断/时间预算保持；审批归属在单次核验按Ref复用摘要，解决大量报告恢复重复编码。真实1024上传/封存/原XML下载及暂停重启批准通过，全套测试分组覆盖、必要race/vet和六个跨平台编译通过；首轮环境隔离/时序与race预算失败及复验如实保留于[验证记录](../specs/019-test-reports/validation.md)。未实跑PostgreSQL或本次Linux节点，不冒充外部商店验收。

2026-10-09 本机维护与FRP规划：main@2d18771已推送；本机客户端升级为v0.1.1-dev.2d18771，沿用既有配置/身份并保留旧程序备份。Java本地工作流改为逐份兼容报告和max_files=1024；dry-run及实际70份生成报告/失败/统计矛盾/数量超限检查通过，未重新运行实际Java项目。见[维护验证](../specs/021-installers-skills/validation.md)。[FRP接入](plans/FRP_ACCESS.md)已完成023的specify/plan/tasks/analyze与frpc0.64.0配置检查，11项部署任务仍待实施；远端控制端/Agent未升级，公网服务未启用，原SSH保持。
