# Implementation Plan: 多节点 Agent 与远程执行闭环

**Branch**: `007-node-agents` | **Date**: 2026-10-04 | **Spec**: [spec.md](spec.md)

**Input**: 28 FR、7 SC、17验收场景；基线006 `8e1397e`。005没有进入基线。

## Summary

接通单控制端、多个独立Agent主动连接的真实执行闭环：节点管理、事务领取与逐任务租约、固定SHA检出、共享Run、逐动作证据、中央日志/产物与实际CLI。控制端不执行仓库脚本。复用Store.write/GORM和process.Run/pipeline.Run，不加第二执行器、消息队列、repository接口、泛用事件框架或未实现步骤stub。

全部执行消息绑定完整fence。Agent从请求开始的单调时间计算保守期限；独立AuthorityContext约束普通和所有post。用户取消可在有效权限内执行既定always，失租或持久化失败禁止所有后续用户动作。未知停止持续占容量/同名锁，节点隔离，不迁移、不重放。

## Technical Context

**Language/Version**: Go 1.25.0，单模块mybuilds。

**Primary Dependencies**: 保持已锁定Cobra/Viper/yaml.v3、GORM/SQLite/pgx、uuid/x/sys/doublestar；HTTP/TLS/SSE、哈希、受限文件与spool用标准库，无新增依赖建议。

**Storage**: 控制端独占SQLite/PostgreSQL，现有flock/专用连接advisory lock；具体节点、attempt、事件回执与文件元数据。Agent独立0700 data_dir，0600 journal/spool，工作区与结果不共享。

**Testing**: 双库同一真实行为suite、实际进程/Git/HTTP/TLS/SSE；go test/vet/race；macOS与Linux独立节点、真实Android/mac产物版本/签名/取消。交叉编译不算工具能力。

**Target Platform**: 控制端/Agent Linux/macOS，客户端跨平台编译；现有Linux ARM只验通用协议，不宣称Android工具可执行。005签名、Linux真实Android和全MVP原门保持。

**Project Type**: 三入口CLI/HTTP，新增cmd/mybuilds-agent。

**Performance Goals**: 默认global/node capacity=1；显式提高可双节点并行。JSON≤1MiB、日志批次≤64KiB、文件流式处理；默认5s心跳/30s租约，失联从最后有效请求开始25s内停止用户动作，随后既有有界物理回收。

**Constraints**: 同项目同名串行；已领取不迁移；短事务末尾复核原到期与锁；spool满/保存失败闭锁；文件DB间隙不可见。时序/消息/文件/分页上限见contracts。

**Scale/Scope**: 单控制端少量可信节点；capacity1–32；不实现HA、retry/审批恢复、通知/报告/发布/retention。未知选项明确拒绝。

## Constitution Check

*Phase 0前检查与Phase 1后重新读取constitution 2.1.0复核均PASS。*

| 原则 | 具体落实 |
|---|---|
| I规范驱动 | setup-plan JSON确认分支与限定路径；plan只产设计，不生成tasks/源码。 |
| II单模块多节点 | 独立Agent入口；本地/远程同一Run；控制端不执行脚本；iOS签名不冒充。 |
| III最小依赖 | 无新增依赖/第二executor/未来hook/repository接口/空业务包。 |
| IV安全与持久化 | 身份隔离、fence、先intent后真实Started/cleanup、纳秒预算、失联保护不重跑。 |
| V真实中文验收 | 两节点/双库/Android/TLS实际证据，README与历史由root同步。 |

hooks={}，before_plan/after_plan无执行项；没有原则例外。已验证材料准备只作研究，不计最终验收。

## Project Structure

```text
specs/007-node-agents/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
└── contracts/{go-api,node-protocol,config-cli}.md
cmd/mybuilds-agent/main.go
internal/protocol/node.go
internal/{store,agent,scm,pipeline,process,server,config}/
internal/cli/{agent,server,client}/
```

**Structure Decision**: 沿现有职责，protocol只依赖config/标准库，不反向依赖Store/Pipeline/Agent；process不依赖业务包。具体消息与实际回调只为当前消费者，不抽象插件或事件总线。

## 分区与依赖

| 分区 | 唯一文件归属 | 实际交付 |
|---|---|---|
| A config001 | internal/store/** | 节点身份/session、claim/renew/event、cancel/expire/stop、日志/产物元数据；双库同suite。 |
| B root分配owner | internal/agent/**、scm/**、pipeline/**；process由root明确移交 | Checkout、同一Run远程增量、OnStart、journal/续租/spool/回传/doctor。 |
| C root分配owner | internal/server/**、config/**、cli/**、Agent入口 | 真实Store/Agent接线、严格配置/CA/时序、HTTP/SSE/文件、实际CLI。 |
| root | protocol/**、go.mod/sum、README/history/validation、共享串行同步 | 冻结具体共享消息后分区；不得并发修改共享文件。 |

先root协议→A/C身份与配置+B真实Checkout/Run→A租约/事件+B Agent journal→C真实HTTP接线→文件/SSE/CLI→双节点/双库与Android闭环。测试先红后绿，不用mockexecutor/stub拼装。A/B独立实际行为可先推进，C须等真实依赖。

实施必须覆盖intent→Start→回执间隙、原到期事务边界、失租always、spool满、文件发布后DB失败、控制端重启有效lease、旧journal不重放。具体决定见research与contracts；所有28FR/7SC/17AC由后续tasks映射，不在plan改规范。必要检查和converge通过后root整功能一次本地提交、不push。

## Complexity Tracking

无constitution违反项或例外；不引入新框架。独立attempt/回执/文件元数据只服务当前实际消费者。
