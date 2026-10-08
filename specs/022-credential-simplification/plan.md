# 实施计划：精简凭据读取与重复校验

**Branch**: `main` | **Date**: 2026-10-08 | **Spec**: [spec.md](spec.md)

## Summary

沿用现有执行器和凭据读取函数，删除无效错误通道、配置加载时重复读取秘密及同一句柄的身份比较。只读私有材料接受 0400/0600，基础脱敏及写入/恢复保护保持。

## Technical Context

- Go 1.25 单模块，现有 os/syscall/x/sys；不新增依赖。
- 三个 CLI 与内部配置、Agent、SCM、分发包；不改变 HTTP、YAML 或数据库模型。
- macOS/Linux Agent；Windows 客户端保持现有能力。
- 使用现有 Go 行为测试，扩展只读权限、延迟读取及筛选规则检查；完整测试必须 `-p 1`。
- 目标是减少重复逻辑，不承诺性能提升，不引入可配置的防护等级。

## Constitution Check

研究前及设计后均通过 I–V：完整 Spec Kit、单模块多节点、最小依赖、明确读取与执行边界、中文文档与真实测试。0400 保留账户独占可读；实际使用前仍读取验证；日志不泄露已知秘密；写入锁/取消/恢复路径不放宽。

## Phase 0：核对现有实现

证据与决策见 [research.md](research.md)。所有调用方已搜索；没有待明确技术选型。基础日志匹配已经很小，保留现有算法与测试。

## Phase 1：实施边界与文件归属

主代理在主工作区串行修改以下文件，无并发写入：

- `internal/config/server.go`：复用包内私有只读权限判断；`client.go`、`agent.go`、`publish_doctor.go`、`webhook.go`、`publish.go` 消费该判断；Agent 配置不读取秘密文件。
- `internal/agent/data_unix.go`、`file_unix.go`：读入秘密与内部可写数据分别验证；内部 0600/0700 不变。
- `internal/scm/credentials_unix.go`：现有私有材料读取接受 0400；不替换 SSH 实现。
- `internal/agent/secrets.go`、`execute.go`、`publish_custom.go`、`publish_query.go`：选择函数只返回集合；商店查询复用其规则。
- `internal/config/webhook.go`、`publish.go`、`internal/distribute/material.go`：仅删除同一打开句柄的重复 SameFile 比较，保留路径替换及内容稳定检查。
- 上述包现有测试：增加正反例，签名测试适配简化返回值；日志测试不改算法。
- `README.md`、`docs/USAGE.md`、`docs/plans/ARCHITECTURE.md`、`docs/IMPLEMENTATION_HISTORY.md` 与本功能规范：同步实际行为和验收记录。

模型、读取契约、验收命令见 [data-model.md](data-model.md)、[contracts/credentials.md](contracts/credentials.md)、[quickstart.md](quickstart.md)。

## 验证与提交

先让只读文件正例失败，再实现。受影响包行为检查、相关 race、`go test -p 1 ./...` 与 `go vet ./...`；编译三个 CLI 的 Windows/Linux 目标确认没有跨平台回归。全量检查只运行一次，失败后按原因修复。记录必要证据，执行 converge；没有缺口时一次本地提交，不 push。继续留在 main，不额外创建 worktree。

## Complexity Tracking

无原则例外；不新增安全包、接口、配置开关、依赖或日志框架。
