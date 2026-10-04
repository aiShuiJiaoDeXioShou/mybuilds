# 任务：003 产物与本地证据

## Phase 1：准备
- [x] T001 主代理完成specs/003的spec/plan/research/model/contracts/quickstart与只读analyze，确认产物和产品P1本地日志范围（全部FR/SC）。

## Phase 2：共享基础
- [x] T002 主代理在go.mod/go.sum锁定doublestar/v4 v4.10.2，核实模块源码；修改internal/pipeline/run_types.go冻结ArtifactRecord/ResultDir/LogPath，同步三个worktree（FR-001、FR-005、FR-010）。

## Phase 3：US1 收集结果
独立验收：真实嵌套文件与JSON/hash一致。
- [x] T003 [P] [US1] 在internal/pipeline/artifact.go、artifact_test.go先验证递归glob/每模式零匹配/重叠去重/同名，再实现validateArtifactPatterns与受限collector（FR-001–FR-005）。
- [x] T004 [US1] 在internal/pipeline/run.go、run_test.go接入整批模式渲染与artifact执行、真实started、ResultDir与清单路径，复用预算/取消/when/--step（FR-001、FR-008–FR-010、FR-013）。

## Phase 4：US2 原证据边界
独立验收：错误输入/取消无部分清单、post原副本不变。
- [x] T005 [US2] 在internal/pipeline/artifact.go、artifact_test.go及必要artifact_unix_test.go验证链接/特殊文件/源变化/复制失败/取消/原子manifest，既有目标不覆盖（FR-003–FR-007）。
- [x] T006 [US2] 在internal/pipeline/run.go、run_test.go验证普通/post独立副本、原失败诊断产物、预算超时不遗留部分文件和证据（FR-007–FR-009、FR-012）。

## Phase 5：US3 本地证据
独立验收：结果根/每步脱敏日志与真实CLI输出。
- [x] T007 [P] [US3] 在internal/pipeline/log.go、log_test.go镜像既有脱敏记录到Root下每步日志，0700/0600、两流、logPath/close、安全写错误，先跑行为红测（FR-010–FR-012）。
- [x] T008 [US3] 在internal/cli/client/local_artifact_test.go和examples/local-artifacts.yml验证JSON/result_dir/hash/post/日志与失败/跳过/dry-run不创建目录；B/C真实实现同步后验收（SC-001–SC-005）。

## Phase 6：集成
- [x] T009 主代理整合并在specs/003-build-artifacts/validation.md记录全量test/vet、相关race、二进制真实复制/hash/post/日志/用户文件/跨平台编译证据，执行converge并修复缺口；更新README/历史，完整功能一次提交。

## 依赖与并行
T001→T002；A:T003/T005、B:T004/T006、C:T007/T008为三分区。T003/T004/T007可按冻结接口并行编写，B实际验收需A/C同步，C CLI验收需B；不写stub。T005依赖T003，T006依赖T004，T008依赖实际B/C，T009依赖全部分区。主代理维护types/deps/docs，文件唯一写入者。
先collector与日志，再执行集成，完整范围验收后提交；不以单task或编译代替验收。
