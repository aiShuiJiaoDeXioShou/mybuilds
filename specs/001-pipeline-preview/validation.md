# 001 验证记录

日期：2026-10-04；宿主 Darwin arm64，Go 1.25.4。集成分支 `001-pipeline-preview`，前置基线 `a752a67`。三个独立 detached worktree 分区交付解析、预览和 CLI，由主代理按 plan 文件归属整合，未使用占位实现。

## 已运行检查

- `go test ./...`、`go vet ./...`：通过；保留双入口帮助、版本和错误退出行为。
- `go test -race ./internal/config ./internal/pipeline ./internal/cli/...`：通过，包含 16 并发初始化、现存文件/符号链接不覆盖，以及 Unix 实际部分写入后的清理。
- 配置边界：严格标量类型、所有嵌套字段、重复/未知键、空/多文档、null/别名/merge、1 MiB/32 层/10000 节点、参数/条件/发布段/路径/步骤种类均有正反例。
- 预览：确定条件与 pending/skipped、稳定顺序、参数全部隐藏、已知模板插值后路径校验、构造模型不被修改，以及脚本/Git/HTTP 无副作用通过。
- 分区解析模糊测试：单 worker 3 秒，40126 次，无失败；这是补充检查，不替代边界用例。
- 真实客户端二进制 quickstart：初始化/重复初始化 SHA-256 不变、default JSON、双 build JSON、无选择与互斥选择拒绝、普通 run/平台模板未支持、脚本 marker 不存在、敏感标记未输出，全部通过。
- `GOOS=windows GOARCH=amd64 go build ./cmd/mybuilds` 和 Linux amd64 客户端交叉编译：通过；这只证明可编译，不作为平台运行验证。
- `git diff --check`：通过。拆分规划的原 24 个代码块保持原字节；专题/规范文档本地链接、YAML 和 shell 示例检查通过。

## Spec Kit 与范围

specify → plan → tasks → analyze → implement 已完成，分析未发现阻塞项；converge 已核对 16 个 FR、5 个 SC、3 用户故事、全部 11 项任务及 5 条原则：missing/partial/contradicts/unrequested 均为 0；收敛阶段没有修改 tasks 原字节。验收按 FR-001–FR-016、SC-001–SC-005、三用户故事执行，不依据任务勾选判断行为。
YAML 深度/节点检查在库完成 AST 解析后执行，原始输入先限 1 MiB；预览不读取运行密钥或探测宿主路径。真实执行、平台模板、产物收集、审批、数据库、上传和通知均未在 001 实现，正常执行明确失败。

本功能不要求移动端/节点/商店环境，当前无外部验收缺项。完整功能本地提交可用 `git log --all -- specs/001-pipeline-preview` 追溯，不自动 push。
