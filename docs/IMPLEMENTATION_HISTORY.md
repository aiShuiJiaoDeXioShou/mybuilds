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
| 005-ios-build | 实现与可执行验证中 | 待验收005完整保存于 `/tmp/mybuilds-mvp.zKtK0e/ios005-pending`（48文件双份SHA核对）；mobile签名/doctor/模板、config/CLI、主代理pipeline三个写入分区；原生keychain/P12非交互签名及自有清理原型通过，用户default/search list不变；真实Apple工程/P12/profile/Bundle ID待用户提供，IPA/dSYM验收未闭合 |
| 006-control-plane | 已完成并验收 | [spec](../specs/006-control-plane/spec.md)、[plan](../specs/006-control-plane/plan.md)、[tasks](../specs/006-control-plane/tasks.md)、[validation](../specs/006-control-plane/validation.md)；003/004基线，20FR/6SC/42任务，零阻塞；store/server+SCM/config+CLI独立worktree；全量/race/vet、真实双库各37项及Linux37项CLI闭环PASS；converge无缺口；集成提交 `8e1397e`：feat(server): 实现控制端与持久化队列 |
| 007-node-agents | 已完成并验收 | [spec](../specs/007-node-agents/spec.md)、[plan](../specs/007-node-agents/plan.md)、[tasks](../specs/007-node-agents/tasks.md)、[validation](../specs/007-node-agents/validation.md)；28FR/7SC/17AC、74任务；Store/Agent/HTTP-CLI三独立worktree冻结交接；真实三Mac/Linux节点、双库各51项、七种故障/进程停止、日志/SSE/完整快照/中央下载、Android签名101及取消103与Linuxordinary/always取消通过；全量test/race/vet和三入口12次跨平台构建通过，converge零缺口；整功能本地提交按此spec的Git日志定位，信息feat(agent): 实现多节点构建与中央证据 |
| 008–012、014–015、019–020 | 未开始 | 满足依赖并集成后按 [MVP 批次](plans/MVP_EXECUTION.md#依赖与可并行批次)推进 |
| 013、016–018 | 未开始，后置 | 不作为本轮 MVP 完成前提 |

更新一行时补充实际 specs 链接、当前阶段、验证结论/证据位置、缺失真实条件及功能集成提交；验收未闭合不记“已完成”。提交哈希来自真实 Git 记录，规划状态不代表代码可用。

019提前规范在独立worktree `/tmp/mybuilds-mvp.zKtK0e/reports019-planning` 完成18FR/6SC/12AC、质量16/16，hooks={}；仅specify，不修改007执行源码，不在其验收前实现。

008提前规范在独立worktree `/tmp/mybuilds-mvp.zKtK0e/recovery008-planning` 完成19FR/5SC/13AC、质量16/16，hooks={}；仅specify，待007验收后plan，真实恢复/原快照retry门不变。009 Flutter规范在独立worktree `/tmp/mybuilds-mvp.zKtK0e/flutter009-planning` 已保存28FR/8SC/19AC、质量16/16，仅specify；当前现有Flutter3.38.6真实工具已核对，自有双平台空工程创建成功，未构建签名或验收。

共享process慢日志排空缺陷按SpecKit bug-assess→fix→test完成，Darwin/Linux真实进程回归及原Android远程签名/日志/中央下载通过，独立本地提交 `58bc9c8`（`fix(process): 防止慢日志回传误判成功进程`）；不包含007未验收实现。验证见[缺陷记录](../.specify/bugs/slow-log-pipe-drain/test.md)。

020保留策略规范在独立worktree `/tmp/mybuilds-mvp.zKtK0e/retention020-planning` 已完成25FR/7SC/16AC、质量16/16，仅specify；可信终态时间、跨build筛选、删除保护及Agent幂等门已明确，待008/019验收后plan/实现。
