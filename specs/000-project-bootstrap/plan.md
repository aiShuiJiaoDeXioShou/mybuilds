# Implementation Plan: 项目初始化

**Branch**: `000-project-bootstrap` | **Date**: 2026-10-04 | **Spec**: [spec.md](spec.md)

**Input**: 初始化双 CLI、项目概览及 AI 阅读入口。

## Summary

采用 Go 单模块与两个薄 main，命令分别位于 internal/cli/client 和 internal/cli/server。
共享 internal/version 提供版本字段与 version 子命令。仅引入 Cobra；README 记录当前能力与后续目录。

## Technical Context

**Language/Version**: Go 1.25.0 起；当前验证环境 Go 1.25.4。
**Primary Dependencies**: Cobra v1.10.2（通过 go list -m 查询并锁定）。
**Storage**: 无业务存储，仅源码、文档和本地 Git。
**Testing**: Go testing 验证双入口 CLI 契约；go vet、实际构建与命令验收。
**Target Platform**: 当前 macOS；客户端额外交叉编译 Linux/Windows。
**Project Type**: 单模块、双 CLI。
**Performance Goals**: 本次不设性能指标，没有构建执行或调度。
**Constraints**: 中文文档；不引入 YAML/数据库/HTTP/SDK，不注册未实现命令。
**Scale/Scope**: 仅初始化；module mybuilds，不假设远端仓库。

## Constitution Check

设计前与设计后均通过：Spec Kit 文档齐全；双入口共享版本；按需依赖和目录；
帮助/版本无业务副作用；参数失败路径有行为测试；中文文档与单功能本地提交。
本次无配置、网络或子进程执行，持久化、安全与取消规则留给对应功能。

## Project Structure

### Documentation (this feature)

```text
specs/000-project-bootstrap/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── contracts/cli.md
├── quickstart.md
├── checklists/requirements.md
├── tasks.md
└── validation.md
```

### Source Code (repository root)

```text
go.mod / go.sum
cmd/mybuilds/main.go
cmd/mybuilds-server/main.go
internal/cli/client/root.go
internal/cli/server/root.go
internal/cli/cli_test.go
internal/version/version.go
README.md
AGENTS.md
docs/plans/PLAN.md
docs/plans/SPECKIT_ROADMAP.md
```

**Structure Decision**: 测试跟随 internal/cli，跨两个真实 root 验证契约；后续业务包随对应功能创建。
internal/version 的版本命令供两端复用；不额外引入命令工厂或抽象接口。

## 实施阶段

1. 锁定模块依赖；准备 CLI 契约检查，先确认失败。
2. 实现共享版本、双命令根和薄 main；验证参数失败与构建注入。
3. 同步规划路径与完成标记，写 README，修改 AGENTS 阅读指引。
4. 按 quickstart 验收、记录结果、收敛，然后单次本地功能提交。

## Complexity Tracking

无原则例外，无额外抽象。
