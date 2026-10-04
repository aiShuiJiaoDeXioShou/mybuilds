# 任务：002 本地执行

输入 spec/plan/contracts；每项包含对应行为验证，按完整功能提交。

## Phase 1：准备
- [x] T001 完成 specs/002-local-run 的规范质量、研究、模型、契约、quickstart 和只读 analyze（全部 FR/SC）。

## Phase 2：共享基础
- [x] T002 主代理建立 internal/pipeline/run_types.go 冻结类型，按 plan 同步三个 worktree；依赖基线 7ffa264。

## Phase 3：US1 顺序执行
目标：真实 shell 顺序和全批预检查；独立验证文件/参数/when。
- [x] T003 [P] [US1] 在 internal/pipeline/run.go、run_test.go 和 preview.go 复用验证、提取渲染值、实现全批选择/参数/模板/Git/环境/目录/生效能力预检查和顺序运行（FR-001–FR-007、FR-013–FR-014）。

## Phase 4：US2 安全结束
目标：取消/预算/post；独立验证本次后台进程停止与无关进程存活。
- [x] T004 [P] [US2] 在 internal/pipeline/process_unix.go、process_other.go、process_unix_test.go 实现独立进程组、日志写失败取消、TERM/KILL/Wait/正常清理与停止确认，真实 helper 先验证再实现（FR-004、FR-009、FR-014）。
- [x] T005 [US2] 在 internal/pipeline/run.go、run_test.go 实现累计预算、失败/跳过、取消批次与独立 post 默认2m，保留原原因并处理停止未确认（FR-007–FR-010、FR-012）。

## Phase 5：US3 日志与客户端
目标：UTC 流式脱敏，真实客户端退出码；独立验证分片/长行/失败输出。
- [x] T006 [P] [US3] 在 internal/pipeline/log.go、log_test.go 实现两流前缀、串行写、分片/重叠秘密脱敏、长行与安全写错误，先跑失败检查（FR-011、SC-004）。
- [x] T007 [US3] 在 internal/cli/client/run.go、local_run_test.go、pipeline_test.go 接入真实 Run、信号和 JSON/日志分流，在 examples/local-run.yml 给出可运行配置，保留 dry-run（FR-012–FR-014、SC-005）。

## Phase 6：集成收敛
- [x] T008 主代理按文件归属整合，在 specs/002-local-run/validation.md 记录全量test/vet、受影响race、二进制真实超时/取消/secret/工作树/整批拒绝及跨平台编译（SC-001–SC-005）。
- [x] T009 主代理执行 converge，必要时追加任务并修复；更新 README.md、docs/IMPLEMENTATION_HISTORY.md，完整功能验收提交（原则I/V）。

## 依赖与并行
T001→T002。T003/T004/T006 为独立写入分区可并行；T003/T005实际运行验收需同步真实T004/T006。T005依赖T003；T007依赖日志T006和真实Run；T008需全部分区，T009需T008。不使用临时stub。US1/US2业务共用同一B写入者，C日志先交B再接CLI，主代理协调共享types/docs。
策略：先最小顺序run，再预算与post，最后全量集成；不是缩减本轮已约定范围。
