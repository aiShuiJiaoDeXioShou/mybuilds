# 实施计划：001 流水线初始化与安全预览

**Branch**: `001-pipeline-preview` | **Date**: 2026-10-04 | **Spec**: [spec.md](spec.md)

## Summary
复用双 CLI 和共享版本；引入一个已锁版本的 YAML 依赖，建立严格、可复用的流水线模型与纯预览。平台模板和实际执行留给后续功能。

## Technical Context
- Go 1.25，单模块 `mybuilds`；Cobra 1.10.2、go.yaml.in/yaml/v3 3.0.5。
- 仅本地配置文件，无数据库与网络；解析限 1 MiB、32 层和 10000 节点。
- 标准 testing，CLI 行为测试与小配置样本；go test ./...、go vet ./...，初始化与无副作用 quickstart。
- 客户端可在 Linux/macOS/Windows 编译；001 无宿主移动工具依赖。
- 错误只写位置/字段/原因；预览不打印脚本文本、参数值、环境值、凭据或通知地址，不读取环境密钥。

## Constitution Check
设计前/后均通过五条原则：I 完整 Spec Kit 与可验收范围；II 保持单模块，共享模型不引入 Agent 空包；III 只增加 YAML；IV 严格校验、无副作用、脱敏；V 中文文档与行为检查。无须更新既有 2.1.0 原则。

## Project Structure
- 主代理：`internal/config/types.go`、go.mod/go.sum、feature 文档、README 与集成。
- 分区 A：`internal/config/parse.go`、`validate.go`、`parse_test.go`，加载、结构/类型/约束与参数选择。
- 分区 B：`internal/pipeline/preview.go`、`preview_test.go`，模板合法性、条件三态、脱敏可序列化预览。
- 分区 C：`internal/cli/client/init.go`、`run.go`、`root.go`、`pipeline_test.go`，初始化/选择/参数/预览接线。
- 例子：`examples/pipeline-preview.yml`，CLI 分区维护。
- 规范及接口见 [数据模型](data-model.md)、[契约](contracts/cli.md)、[公共 API](contracts/go-api.md)。

## 实施阶段与并行
1. 完成 spec、研究、模型与契约、tasks、只读 analyze，解决阻塞项。
2. 主代理写最小共享类型；将相同规范、类型和依赖文件同步到三个独立 detached worktree。
3. A 先交付解析/校验；B 可按冻结类型实现，但测试待 A 复制后运行；C 先实现 init/参数接线，随后集成 A/B 公共 API。
4. 各区自测后交付限定差异；主代理整合、quickstart、现有行为回归、converge；必要修复继续 implement/converge。
5. 更新任务与验证记录、README、总历史；整项一次本地提交，不 push。

## Complexity Tracking
未引入解析器抽象、模板引擎、插件注册器、网络预览或新的运行时框架；仅结构化数据与纯函数。
