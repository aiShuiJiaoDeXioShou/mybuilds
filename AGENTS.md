# AGENTS.md

需要了解整个项目的信息时，先阅读 [`README.md`](README.md)，查看项目定位、当前状态、目录职责、运行方式与文档导航。
修改目录、入口、运行命令或已实现能力时，同步更新 README。

产品需求与设计决策见 [`docs/plans`](docs/plans) 目录。改架构、加依赖前先读。

## 开发流程

- 功能开发必须使用项目内的 Spec Kit 技能。首次开发前，用 `$speckit-constitution` 根据规划补全项目原则。
- 每个功能依次执行 `$speckit-specify` → `$speckit-plan` → `$speckit-tasks` → `$speckit-analyze` → `$speckit-implement` → `$speckit-converge`；需求有实质歧义时使用 `$speckit-clarify`。
- 已有功能继续使用其 `specs/` 文档；先修正分析发现的阻塞问题，再实现。收敛发现缺口时继续 implement / converge，直到验收与必要检查通过。
- 缺陷修复使用 `$speckit-bug-assess` → `$speckit-bug-fix` → `$speckit-bug-test`。

## Git 提交

- 首次开发前检查 Git 仓库；尚未初始化时先初始化并提交现有规划与工具配置作为基线。
- 每完成并验收一个功能或缺陷修复，自动执行一次本地提交，无需再次确认；不按单个任务提交，不自动 push。
- 提交包含该功能的规范、任务、代码与验证记录；先检查工作区和暂存差异，只暂存本次相关改动，不夹带用户其他修改。
- 提交信息遵循 `git-commit-message` 技能；结束时报告验收结果、提交哈希与提交信息。检查未通过时继续修复，不提交未完成的功能。

## 编码约定

- 注释与文档用中文。
