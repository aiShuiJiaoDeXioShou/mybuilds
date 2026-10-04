# 任务：001 流水线初始化与安全预览

输入：[spec.md](spec.md)、[plan.md](plan.md)、[contracts](contracts/go-api.md)。每项任务包含实现与相应行为验证，不按任务单独提交。

## Phase 1：契约与基础

- [x] T001 完成 specs/001-pipeline-preview 的 spec、研究、数据模型、CLI/API 契约及 quickstart（FR-001–FR-016、SC-001–SC-005）。
- [x] T002 锁定 go.mod/go.sum 中 YAML v3.0.5，核实严格类型及安全错误处理研究（FR-003、FR-013）。
- [x] T003 主代理建立 internal/config/types.go 最小共享模型，并同步独立 worktree 的相同契约和依赖（FR-002、FR-007–FR-009）。

## Phase 2：解析与选择（US2/US3，分区 A）

- [x] T004 [P] [US3] 在 internal/config/parse.go、validate.go、parse_test.go 实现并验证限输入/深度/节点、单文档、拒绝别名/null/merge、未知/重复字段、严格标量类型、规范化 default/builds 和不回显错误（FR-002、FR-003、FR-013）。
- [x] T005 [US2] 在 internal/config/validate.go、parse_test.go 验证参数默认/required/choices、Select/ResolveParams、四种专属步骤字段/名字/路径/超时、when/post/reports/通知及发布段顺序（FR-004–FR-009、FR-014、FR-015）。

## Phase 3：纯预览（US2/US3，分区 B）

- [x] T006 [P] [US2] 在 internal/pipeline/preview.go、preview_test.go 实现模板合法性/参数映射、条件三态与原因、选择/--step 规则和稳定 JSON 预览；测试待 A API 同步后运行（FR-004–FR-006、FR-010、FR-011）。
- [x] T007 [US3] 在 internal/pipeline/preview_test.go 验证无 exec/Git/网络/密钥解析，参数/env/脚本/argv/凭据/Webhook 不回显，未知模板安全报错（FR-012、FR-013）。

## Phase 4：CLI 与初始化（US1/US2/US3，分区 C）

- [x] T008 [P] [US1] 在 internal/cli/client/init.go、pipeline_test.go 实现最小 init/本地模板/不覆盖与写失败清理，平台模板及冲突选项明确未支持（FR-001、FR-016、SC-001）。
- [x] T009 [US2] 在 internal/cli/client/run.go、root.go、pipeline_test.go、examples/pipeline-preview.yml 接线 Load/Preview、选择/重复参数/文件路径/--step/dry-run，保持帮助/version；A/B 完成后验证（FR-004、FR-006、FR-012、FR-016）。

## Phase 5：集成与收敛

- [x] T010 主代理整合所有差异，在 specs/001-pipeline-preview/validation.md 记录 go test ./...、go vet ./...、quickstart 正反例、无副作用与脱敏证据（SC-001–SC-005）。
- [x] T011 主代理执行 speckit-converge，缺口追加任务并修复；同步 README.md、docs/IMPLEMENTATION_HISTORY.md 和执行进度，验收后完整功能本地提交（SC-001–SC-005、原则 I/V）。

## 依赖与并行

T001/T002 → T003；T004/T006/T008 可以基于冻结模型在独立 worktree 开始。T005 依赖 T004；T007 依赖 T006；T009 的编译验收依赖 A/B 公共实现，不能用占位实现交付。T010 依赖所有分区完成；T011 依赖 T010。主代理拥有 types.go/go.mod/specs/README，其余按 plan 文件归属分区。
