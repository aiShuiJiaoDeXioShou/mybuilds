# 实施历史

本文件记录功能状态与验收/提交索引，由主代理随集成更新；任务与验证细节保留在对应 specs，不复制任务列表。
MVP 批次、真实验证与完成定义见 [执行计划](plans/MVP_EXECUTION.md)，范围见 [产品计划](plans/PLAN.md)。

| 功能 | 当前状态 | 规范与验证 / 集成记录 |
|---|---|---|
| 000-project-bootstrap | 已完成并验收 | [spec](../specs/000-project-bootstrap/spec.md)、[validation](../specs/000-project-bootstrap/validation.md)；功能提交 0742512；本轮规划基线 a752a67 |
| 001-pipeline-preview | 已完成并验收 | [spec](../specs/001-pipeline-preview/spec.md)、[validation](../specs/001-pipeline-preview/validation.md)；解析/预览/CLI 三分区独立 worktree 交付，全量 test/vet/race 和二进制 quickstart 通过，converge 无缺口；集成提交 `7ffa264`：feat(pipeline): 实现配置初始化与安全预览 |
| 002-local-run | 已完成并验收 | [spec](../specs/002-local-run/spec.md)、[validation](../specs/002-local-run/validation.md)；依赖7ffa264，真实shell/进程/预算/post/日志/CLI、全量test/vet/race与二进制SIGINT/超时通过，converge无缺口；集成提交 `256af8e`：feat(pipeline): 实现本地执行与安全取消 |
| short-timeout-cleanup | 已修复并验收 | [.specify缺陷记录](../.specify/bugs/short-timeout-cleanup/test.md)；003集成时在002基线真实复现Darwin瞬时EPERM，原预算50轮与100轮30ms回归通过；独立提交 `0937e83`：fix(pipeline): 修正短超时进程组停止确认 |
| 003-build-artifacts | 已完成并验收 | [spec](../specs/003-build-artifacts/spec.md)、[plan](../specs/003-build-artifacts/plan.md)、[tasks](../specs/003-build-artifacts/tasks.md)；前置256af8e，collector/执行/日志与CLI独立worktree；全量test/vet/race、真实二进制快照/manifest/hash/post/日志通过，converge无缺口；[validation](../specs/003-build-artifacts/validation.md)，完整功能本地提交见Git记录 |
| 004–012、014–015、019–020 | 未开始 | 满足依赖并集成后按 [MVP 批次](plans/MVP_EXECUTION.md#依赖与可并行批次)推进 |
| 013、016–018 | 未开始，后置 | 不作为本轮 MVP 完成前提 |

更新一行时补充实际 specs 链接、当前阶段、验证结论/证据位置、缺失真实条件及功能集成提交；验收未闭合不记“已完成”。提交哈希来自真实 Git 记录，规划状态不代表代码可用。
