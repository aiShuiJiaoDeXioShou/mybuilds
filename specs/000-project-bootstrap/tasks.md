# Tasks: 项目初始化

**Input**: spec.md、plan.md、research.md、data-model.md、contracts/cli.md、quickstart.md。
**Tests**: 按规范与原则，用 Go testing 覆盖命令参数失败等行为。
**Organization**: 按用户故事实施；每个功能验收后提交一次。

## Phase 1: Setup

- [X] T001 初始化 go.mod 为 mybuilds，锁定 Cobra v1.10.2，生成 go.sum。（FR-001）

## Phase 2: Foundational

- [X] T002 检查 .gitignore 覆盖 bin、测试产物与本地配置，确认 .specify 脚本可执行和 Git 基线存在。（FR-007）

## Phase 3: User Story 1 - 双入口（P1）

目标：两个独立入口可运行，默认版本一致且错误参数失败。
独立验收：构建并运行两端帮助、版本、无参数与错误参数。

- [X] T003 [US1] 在 internal/cli/cli_test.go 添加双命令根的帮助、默认版本、未知命令与多余参数检查，先确认失败。（FR-002、FR-003）
- [X] T004 [US1] 在 internal/version/version.go 实现共享版本命令；字段默认值必须为 Version=dev、Commit=unknown、BuildDate=unknown，并可通过 ldflags 注入。（FR-002）
- [X] T005 [US1] 实现 internal/cli/client/root.go、internal/cli/server/root.go 及 cmd/mybuilds/main.go、cmd/mybuilds-server/main.go，仅接入当前有效命令。（FR-001、FR-003）

## Phase 4: User Story 2 - 项目概览（P2）

目标：人和 AI 能从概览找到准确状态及后续路线。
独立验收：README 文档链接与命令均可用，AGENTS 明确指向 README。

- [X] T006 [US2] 同步 docs/plans/PLAN.md 和 docs/plans/SPECKIT_ROADMAP.md 新目录、000 状态及 001 复用入口范围。（FR-006）
- [X] T007 [US2] 在 README.md 写中文概览、实际目录和后续模块职责、可运行命令、文档入口与 Spec Kit 工作流；按用户要求跳过语言审校。（FR-004）
- [X] T008 [US2] 在 AGENTS.md 写阅读整个项目先看 README 的指引，并要求入口、结构、状态变化同步概览。（FR-005）

## Phase 5: Polish & Cross-Cutting Concerns

- [X] T009 按 specs/000-project-bootstrap/quickstart.md 验收默认和注入版本、双端退出码、测试、vet、客户端跨平台构建、文档链接和路径；在 validation.md 记录结果。（SC-001 至 SC-003）
- [X] T010 对 specs/000-project-bootstrap/spec.md、plan.md、tasks.md 与实现执行 converge；验收通过后一次本地提交本功能相关文件并记录提交结果。（FR-007、SC-004）

## Dependencies & Execution Order

T001 → T002 → T003 → T004 → T005 → T006 → T007 → T008 → T009 → T010。
US1 是最小可运行成果；US2 可独立审阅，但运行示例依赖 US1 完成。

## Parallel Examples

US1：T004 的版本包与 T005 的双入口存在调用依赖，按顺序实现；两个 root 文件可在共享版本完成后并行。
US2：T006 的规划与 T008 的 AI 指引可在概览内容确定后并行；同一文件不可同时改。
本次规模小，顺序实施代码，README 发现与选型研究已并行只读完成。

## Implementation Strategy

先通过 US1 实际运行验收，再完成 US2 文档与链接验收，最终收敛并提交整个功能。
无业务存储、执行器或通知实现；依赖和目录随后续功能加入。
