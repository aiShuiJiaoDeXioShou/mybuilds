# 实施计划：002 本地流水线执行

**Branch**: `002-local-run` | **Date**: 2026-10-04 | **Spec**: [spec.md](spec.md)

## Summary
在 001 公共模型和纯预览基础上接入实际执行，共用校验/模板/条件。仅用标准库完成进程、受限环境、计时和日志，不建立插件或本地发布数据库。

## Technical Context
Go 1.25，既有 Cobra/YAML，零新增依赖；执行 macOS/Linux，其他平台可编译但明确拒绝本地执行。os/exec 独立 Unix 进程组、自定义取消、有限 WaitDelay；步骤采用自身与 build 剩余预算的最小值，post 独立预算。必要 Git 只读检测 SHA/分支；全批预检查后才运行 shell。EvalSymlinks 在预检查与启动前复检。日志每流保存未决秘密后缀，串行增量写出，长行分块。

## Constitution Check
设计前/后五原则通过：I 规范验收；II 共用 pipeline；III 零依赖；IV 受限环境、进程取消、输入/日志安全；V 中文与真实行为检查。可信 shell 不作为安全沙箱。

## Project Structure 与文件归属
- 主代理：internal/pipeline/run_types.go、specs/002、README、历史及集成。
- A：internal/pipeline/process_unix.go、process_other.go、process_unix_test.go，实现 runShell。
- B：internal/pipeline/run.go、run_test.go，预检查/环境/目录/Git/执行/预算/post；唯一可修改 preview.go，将既有扫描器提取为返回内部渲染值的共享 helper，保留预览行为。
- C：internal/pipeline/log.go、log_test.go，UTC 流式脱敏；internal/cli/client/run.go、local_run_test.go、原 pipeline_test.go 的普通执行拒绝用例调整；examples/local-run.yml。日志先交 B，CLI 后集成 Run。
- 接口冻结于 contracts/go-api.md，CLI 见 contracts/cli.md；不制造占位实现。

## 阶段与依赖
specify/plan/tasks/analyze 后写共享类型；三个独立 worktree 基于验收提交7ffa264。A/C可并行，B按冻结API编写，实际A/C同步后才验收。主代理串行集成、test/vet/race/二进制真实检查、converge及完整功能提交。

## Complexity Tracking
只创建当前本地执行所需具体函数/类型；不预建远程租约接口、产物/报告hook、通知或恢复层，后续功能扩展同一执行路径。
