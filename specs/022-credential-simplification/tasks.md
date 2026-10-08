# 实施任务：凭据精简

输入：[spec.md](spec.md)、[plan.md](plan.md)。沿用已验收的执行路径，不建立新基础设施。涉及读取边界，因此使用现有行为测试验证。

## Phase 1：准备

- [x] T001 核对 `.specify/memory/constitution.md`、`.gitignore`、既有调用方和 `specs/022-credential-simplification/research.md`，完成 analyze 无阻塞。

## Phase 2：US1 只读凭据（P1）

- [x] T002 [US1] 在 `internal/config/{client,agent,publish_doctor,publish,webhook}_test.go`、`internal/agent/secrets_test.go`、`internal/scm/credentials_test.go` 增加只读正例与延迟读取断言，先确认失败。（FR-001/002，SC-001）
- [x] T003 [US1] 在 `internal/config/server.go` 集中私有读取判断，并更新 `client.go`、`agent.go`、`publish_doctor.go`、`publish.go`、`webhook.go`；Agent 仅解析秘密路径。（FR-001/002）
- [x] T004 [US1] 在 `internal/agent/data_unix.go`、`file_unix.go` 与 `internal/scm/credentials_unix.go` 接受只读秘密，保持内部可写权限；删除 `internal/config/{webhook,publish}.go`、`internal/distribute/material.go` 同句柄重复比较。（FR-001/004）

## Phase 3：US2 单一筛选规则（P2）

- [x] T005 [US2] 扩展 `internal/agent/secrets_test.go` 覆盖上传/post、所有控制凭据及等值 token；验证 `internal/agent/concurrent_secrets_test.go` 与 `internal/pipeline/log_test.go`。（FR-003/004，SC-002/003）
- [x] T006 [US2] 简化 `internal/agent/secrets.go` 的返回值与 `execute.go`、`publish_custom.go`、`ios_resources_test.go`、`secrets_test.go` 调用；`publish_query.go` 复用选择规则。（FR-003）

## Phase 4：文档、验收与收敛

- [x] T007 更新 `README.md`、`docs/USAGE.md`、`docs/plans/ARCHITECTURE.md` 与 `docs/IMPLEMENTATION_HISTORY.md` 的读取行为。（FR-005）
- [x] T008 运行 `specs/022-credential-simplification/quickstart.md` 的行为/race/全量/vet 与跨平台编译检查，写入 `validation.md`；执行 converge 无缺口后检查差异并本地提交。（SC-001–004，FR-005）

## 顺序与交付

T001 → T002 → T003/T004 → T005/T006 → T007/T008。两用户故事可独立验证；共享文件串行修改，不并行写入。本次复用主工作区，完整功能验收后一次提交，不按任务提交，不 push。

| 覆盖 | 任务 |
|---|---|
| FR-001/002、SC-001 | T002–T004 |
| FR-003、SC-002 | T005/T006 |
| FR-004、SC-003 | T004/T005/T008 |
| FR-005、SC-004 | T007/T008 |
