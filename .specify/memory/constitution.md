# mybuilds Constitution

## Core Principles

### I. 规范驱动，按功能交付

功能必须使用项目内 Spec Kit 的 specify → plan → tasks → analyze → implement → converge
流程；已有功能复用其 specs 文档。必须先解决阻塞问题，并以实际验收结果判断完成。

### II. 单模块双 CLI，共享实际能力

必须采用 Go 单模块，客户端和服务端分别由 cmd/mybuilds 与 cmd/mybuilds-server 进入。
实现位于 internal，流水线能力由本地执行与服务端共用；服务生命周期与调度归入 internal/server。
服务端在单机 macOS 执行，客户端远程命令须保持可跨平台编译。

### III. 最小实现，按需依赖

必须先检查现有代码和标准库，只为当前功能引入必要依赖并锁定版本。
不得预建空业务包、插件系统、Web UI、容器调度或分布式执行器。
飞书接入必须采用官方第三方 Go SDK，并验证真实机器人消息闭环。

### IV. 输入与执行边界明确

外部配置、请求和路径必须校验；日志、预览和配置快照不得泄露密钥。
必须持久化服务端构建和审批进度，不能静默重跑中断步骤或自动重发结果未知的上传。
子进程必须限制环境继承，取消必须只作用于本次构建；仅执行管理员登记的可信仓库。

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

**Version**: 1.0.0 | **Ratified**: 2026-10-04 | **Last Amended**: 2026-10-04
