# mybuilds Constitution

## Core Principles

### I. 规范驱动，按功能交付

功能必须使用项目内 Spec Kit 的 specify → plan → tasks → analyze → implement → converge
流程；已有功能复用其 specs 文档。必须先解决阻塞问题，并以实际验收结果判断完成。

### II. 单模块，控制端与多构建节点

必须采用 Go 单模块，客户端、控制端与构建 Agent 分别由
cmd/mybuilds、cmd/mybuilds-server 与 cmd/mybuilds-agent 进入，入口随对应功能创建。
控制端负责状态、鉴权、调度与审批，Agent 复用 internal/pipeline 执行构建，本地 run 共用同一引擎。
必须支持单控制端管理多个构建节点；iOS 只分配给具备 Xcode 与签名能力的 macOS 节点。
控制端可部署 Linux/macOS；客户端远程命令须可跨平台编译。

### III. 最小实现，按需依赖

必须先检查现有代码和标准库，只为当前功能引入必要依赖并锁定版本。
不得预建空业务包、插件系统、Web UI 或容器调度；一期不实现多控制端高可用或跨节点拆分单次流水线。
飞书接入必须采用官方第三方 Go SDK，并验证真实机器人消息闭环。

### IV. 输入与执行边界明确

外部配置、请求和路径必须校验；日志、预览和配置快照不得泄露密钥。
必须持久化服务端构建和审批进度，不能静默重跑中断步骤或自动重发结果未知的上传。
子进程必须限制环境继承，取消必须只作用于本次构建；仅执行管理员登记的可信仓库。
节点必须独立鉴权，任务归属与租约必须持久化；过期执行权的回报与新外部动作必须被拒绝。
节点失联不得自动迁移已开始的构建；不能承诺外部副作用恰好执行一次。

### V. 可运行验收与中文文档

注释与文档必须使用中文。非平凡逻辑必须留下最小可运行检查，
涉及安全、事务、恢复和进程取消必须有对应行为测试，不采用虚构覆盖率指标。
README 必须区分已实现能力与规划，并随入口、运行方式和结构变化更新。

## 技术与产品边界

产品和技术决策以 docs/plans/PLAN.md 为依据，功能路线见 docs/plans/SPECKIT_ROADMAP.md。
CLI 使用 Cobra，HTTP 使用标准库；数据库默认 SQLite、可选 PostgreSQL，统一通过 GORM。
流水线顺序执行，步骤仅 run、artifact、approval、upload；运行数据存用户目录，不能混入源码。
业务依赖和具体文件随对应功能创建，不在初始化阶段安装全部依赖。

## 开发流程与验收

项目概览从 README.md 阅读，AI 执行约定见 AGENTS.md。
每个功能必须保存 spec、plan、tasks 和验证记录；analyze 的阻塞问题须先修复，
converge 发现缺口则继续实现和收敛。缺陷使用 bug-assess → bug-fix → bug-test。
首次建立 Git 基线；每个功能或缺陷验收通过后必须自动本地提交一次，不按任务提交，不自动 push。
提交前检查差异，只包含本次相关文件，提交信息遵循 git-commit-message 技能。

## Governance

本文件约束所有功能规范与实现。修改原则必须说明原因、影响和迁移方式，并同步受影响的规划或规范。
不兼容原则变更升主版本，新增原则或实质扩展升次版本，文字澄清升修订版本。
每次 plan、analyze、converge 必须检查原则；具体实现细节留在 feature plan 中。

2.0.0 变更依据：用户要求多 node 架构，替换单机 macOS、双入口及禁止远程执行器的边界。
影响：PLAN、MULTI_NODE 与实施路线、README 同步更新；已完成的 000 初始化文档保留为历史验收，
新 Agent 与节点协议由后续功能实现，当前无业务数据库或执行器需要迁移。

**Version**: 2.0.0 | **Ratified**: 2026-10-04 | **Last Amended**: 2026-10-04
