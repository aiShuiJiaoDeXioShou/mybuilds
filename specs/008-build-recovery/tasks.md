# Tasks：008 重启核对与原快照重试

**输入**：[spec.md](spec.md)、[plan.md](plan.md)、[data-model.md](data-model.md)、[contracts](contracts/)、[quickstart.md](quickstart.md)。
**前置**：007已验收85b46bf；全部19FR/5SC/13AC。原则2.1.0，无新增依赖。恢复/事务/身份/进程要求真实行为检查，先红后绿。
**格式**：`- [ ] Txxx [P?] [USx?] 描述（具体路径）`。根唯一维护此文件；[P]仅不同文件且所列前置已完成。

## Phase 1：Setup

- [x] T001 核实007提交、独占锁、项目仓库不可变与既有消费者，在 specs/008-build-recovery/validation.md 记录基线；先读 README.md、docs/plans/PLAN.md。
- [x] T002 核实 .gitignore 对 bin/vendor/运行秘密与 .specify/feature.json 的忽略，以及 .specify/extensions.yml hooks；为Agent建立独立worktree并冻结文件所有权于 specs/008-build-recovery/validation.md。

## Phase 2：Foundational（阻塞所有故事）

- [x] T003 根在 internal/protocol/node.go 增加 contracts/protocol.md 唯一 TerminalReceiptRequest/TerminalReceipt 具体消息；沿完整Ref、正Seq、64位小写SHA，冻结SHA交B。
- [x] T004 C在 internal/store/models.go、internal/store/node_models.go 添加 `retry_of: nullable UUID，自关联build FK RESTRICT，索引`、`kind: string NOT NULL，旧迁移默认空值未知`、`stop_known: bool非空default false`；沿现有Migrate，无新表/框架。
- [x] T005 C在 internal/store/recovery_migration_test.go（保留原 internal/store/node_models_test.go 两门） 真实旧007 SQLite/PG迁移检查新增默认值、FK拒绝删除原构建、幂等二次迁移；旧receipt不能猜Kind/StopKnown。
- [x] T006 根在 specs/008-build-recovery/validation.md 记录实际迁移红绿、协议SHA与基础门；Agent只能在此门后实现，不借019源码。

## Phase 3：US1 重启后保留排队与有效执行（P1）

**目标**：启动核对成功才监听；queued与有效执行保留原快照/身份/预算。
**独立门**：真实控制端lease内重启，运行与queued各一个，无重复动作；锁/损坏拒绝监听。

- [x] T007 [US1] C先在 internal/store/recovery_test.go 编写Recover行为红测：queued/skipped/terminal不变、有效Ref/ordinary-post NS不变、恰好expiry保护、坏快照/身份/预算/FK安全拒绝。（FR001–005/015）
- [x] T008 [P] [US1] B先在 internal/agent/recovery_test.go 编写临时heartbeat/renew网络故障红测：同session、deadline不增长、不新claim、永久错误立即闭锁。（FR003/005）
- [x] T009 [US1] C在 internal/store/recovery.go 实现 Recover(ctx)；持锁受控分页 `每页100条按主键游标` 校验可调度持久结构，合法终态不重判，复用ExpireLeases，任何错误database_error且无新授予。（T007）
- [x] T010 [US1] 根在 internal/server/server.go 接 ListenAndServe监听前 `30s独立子期限` Recover；失败不接调度，周期仍ExpireLeases；在 internal/server/server_test.go 检查。（T009）
- [x] T011 [US1] B在 internal/agent/serve.go、internal/agent/lease.go 只允许network_error有界重试；heartbeat容忍上限最后成功时刻+LeaseDuration，当前执行仍受更早Authority deadline；断联期间不发新claim，已发原key/session只可在原请求起点+LeaseDuration-安全余量内精确幂等确认；超时保unknown且不误停其它有效lease，永久错误仍闭锁，迟到grant/续租不能复活。（T008）
- [x] T012 [US1] 根/B以真实进程控制端重启、queued与实际长脚本、推进分支验证原SHA/编号/Ref/次数/NS，双库同suite与mac/Linux关键路径，将证据写 specs/008-build-recovery/validation.md。（T010–011；SC001/005）

## Phase 4：US2 中断保护与已有证据（P1）

**目标**：普通/always失联无重放；未知停止保留guard，精确确认只解除guard。
**独立门**：实际普通及always Started后断联/退出，组被回收，旧回报拒绝且新进程不按旧PID操作。

- [x] T013 [US2] C先在 internal/store/recovery_test.go 验证到期/stop-confirm/恢复竞争，原reason/日志/制品/ordinary-post NS保持、容量/同名/节点隔离不释放。（FR004/005/007/013）
- [x] T014 [P] [US2] B先在 internal/agent/recovery_test.go 以真实子进程验证短失联继续、长失联Authority到期组回收/always禁用及无关进程存活；新Agent对未知journal拒绝且无旧PID信号。（FR006）
- [x] T015 [US2] B在 internal/agent/http.go、internal/agent/execute.go、internal/agent/spool.go 收窄当前Authority内普通event/log/file网络重试，保持原seq/digest/offset/spool限额和预算；终态只一次post，永久/fence/保存失败立即停止；连续丢续租ACK时记录最后实际请求窗口，组回收后的独立停止确认覆盖可能中央TTL上界，只解guard不延长Authority或执行动作。（T014；FR003–006）
- [x] T016 [US2] C仅按T013实际缺口调整 internal/store/lease.go、internal/store/stop.go、internal/store/event.go，沿007过期/精确停止事务；不重写执行器、不建发布unknown假字段。（FR004/007/014）
- [x] T017 [US2] 根/B在macOS/Linux真实ordinary和always中断、Agent重启与独立精确停止确认，核对本组消失/无关组存活、旧动作不重放、原证据不改写，写 specs/008-build-recovery/validation.md。（T013–016；SC002/004/005）

## Phase 5：US3 显式原快照新构建（P1）

**目标**：新号/新身份从原完整快照执行，当前授权检查且原记录不可变。
**独立门**：改HEAD/设置后retry仍原SHA/参数/条件；20同key一个新号；所有负例无号/row。

- [x] T018 [US3] C先在 internal/store/retry_test.go 编写快照/预算/关系/原记录不可变红测与20并发幂等、trigger同key/不同输入冲突；SQLite/PG同suite。（FR008/009/012）
- [x] T019 [US3] C先在 internal/store/retry_test.go 编写queued/active/skipped/guard、缺项目/角色/原default与授权交集为空/分支授权/损坏事实/含upload无许可等拒绝且不占号红测；重放也重新检查当前授权。（FR010/011/013/015/017）
- [x] T020 [US3] C在 internal/store/retry.go 实现 RetryInput 与 Store.Retry短write事务；操作摘要固定operation/build_id/allow_upload，沿identity+key与计数CAS，同事务末再次核对锁/身份/授权/保护，不调用Git/网络/当前Preview。（T018–019）
- [x] T021 [US3] C在 internal/store/retry.go 深拷贝原Definition/Params/Facts/source/SHA/digest/file/步骤Condition-Reasons；只更新build.id/number，节点范围交集/固定default；清全部运行/lease/attempt/日志/制品证据；普通skipped其余pending，post全pending，预算取原定义完整值并校验。（FR008/009/011/015）
- [x] T022 [US3] 根在 internal/server/retry_test.go 先测真实HTTP角色/严格未知JSON/无key/错误UUID/201与200/409/安全字段，后在 internal/server/http.go、internal/server/retry.go 接 POST /api/builds/{uuid}/retry 用户鉴权。（T020–021）
- [x] T023 [US3] 根在 internal/cli/client/retry_test.go 先测必填key/无输入覆写/无自动重发与响应丢失同key，再在 internal/cli/client/build.go、internal/cli/client/retry.go 接 build retry <id> --idempotency-key、--allow-upload/--json；失败安全显示原key。（T022）
- [x] T024 [US3] 根实际推进HEAD和项目设置、retry执行新完整Run及制品，20并发/丢响应/触发冲突/权限变化复验；比对原构建证据摘要不变，将双库及两平台结果写 specs/008-build-recovery/validation.md。（T018–023；SC003–005）

## Phase 6：US4 查询关系与精确已确认终态（P1）

**目标**：安全公开原/新关系；只对已确认完整终态安全清本条journal。
**独立门**：真实丢build_finished ACK，重启同data dir、当前node精确只读确认，无旧写权；所有unknown/冲突保持。

- [x] T025 [US4] C在 internal/store/query.go（Store侧），根在 internal/server/trigger.go、internal/server/http.go、internal/cli/client/build.go 接 BuildView/BuildSummary RetryOf安全字段省略空值；更新 internal/store/query_test.go、internal/cli/client/build_progress_test.go 关系/秘密不公开检查。（FR016/017）
- [x] T026 [US4] C先在 internal/store/terminal_receipt_test.go 编写完整已确认终态只读正例，错ref/seq/digest/actor、缺失/旧无Kind/StopKnown、active/guard/cleanup与停确认冒充结果的拒绝红测，查询前后状态/seq/NS相同。（FR019）
- [x] T027 [US4] C在 internal/store/event.go 写真实execution receipt.Kind/StopKnown，build_finished完整manifest+物理停止无Cleanup与receipt同事务；旧receipt保持空/false。（T026）
- [x] T028 [US4] C在 internal/store/terminal_receipt.go 实现 TerminalReceipt受控只读事务，当前NodeActor与完整原build/attempt/receipt精确核对；不currentExecution/续租/写回/停止确认。（T027）
- [x] T029 [US4] 根在 internal/server/agent_test.go 先测当前node只读API、节点撤销/禁用/跨node/用户身份和未注册session，后在 internal/server/agent.go 接 POST /api/agent/terminal-receipt 严格JSON与安全错误。（T028）
- [x] T030 [P] [US4] B先在 internal/agent/journal_test.go、internal/agent/recovery_test.go 写终态ACK丢失、合法清本条与另unknown仍阻止Serve红测；wrong NodeName/fullRef/status/digest、文件替换/非法权限/symlink/FIFO/坏JSON/超过上限均保留。（FR006/019）
- [x] T031 [US4] B在 internal/agent/journal.go、internal/agent/serve.go、internal/agent/http.go 接data锁→受限旧journal→当前node只读查询→同inode unlink/fsync→未知拒绝→Doctor/新session；只允许双方StopConfirmed&&!Cleanup、PendingEvent build_finished、无PendingStop/PendingLog，重新算摘要且完整结果相等。（T030）
- [x] T032 [US4] B在 internal/agent/journal.go 落实 `≤128、各≤1MiB、owner/0600/regular/单链接、不跟symlink、不阻塞FIFO`；保存/删除/fsync不明拒继续，仅清本条journal不删spool/结果/未知文件、不旧PID kill/Run。（T031）
- [x] T033 [US4] 根在 internal/cli/client/retry_test.go 验证node/approver拒绝、本地help/version/run/doctor不读远程配置，缺原commit新重试只固定Checkout安全失败、缺工具保持queued；错误无脚本/参数/私有路径。（FR015–017）
- [x] T034 [US4] 根/B真实拦截已提交terminal响应并重启同data_dir，用新/当前独立token核对自有journal；只清精确已确认项，readonly中央证据前后不变，旧终态重放仍拒绝；mac/Linux与双库结果写 specs/008-build-recovery/validation.md。（T025–033；SC004/005）

## Phase 7：Polish 与整功能验收

- [x] T035 根串行SHA集成Agent、C Store与自己的HTTP/CLI，检查所有权和 gofmt、git diff --check；最终对照19FR/5SC/13AC在 specs/008-build-recovery/validation.md 写真实覆盖，失败继续修。（FR018）
- [x] T036 [P] 根同步 README.md、docs/plans/PLAN.md、docs/plans/DELIVERY.md 与 docs/IMPLEMENTATION_HISTORY.md 的已实现命令/恢复边界/008状态；不宣称005/019/全MVP完成。
- [x] T037 根跑实际 go test ./...、go test -race ./...、go vet ./... 与007三入口12次交叉构建及本机help/version，将退出码与工具版本写 specs/008-build-recovery/validation.md。（T035）
- [x] T038 根完成 quickstart.md 五组真实门：SQLite/PG同suite、mac/Linux重启/网络/receipt/retry、20并发、真实PGID/无关进程与旧证据；归档SHA、安全ID/UTC到 specs/008-build-recovery/validation.md。（T012/017/024/034）
- [x] T039 根执行 speckit-converge；仅发现缺口时在 specs/008-build-recovery/tasks.md 追加，继续implement/converge至0阻塞/缺口，保存报告；0缺口tasks字节不变。（T036–038）
- [x] T040 根检查工作区/暂存仅本次相关差异，按 git-commit-message 一次本地提交完整008规范/任务/代码/证据；在 docs/IMPLEMENTATION_HISTORY.md 下一交付记录实际哈希，报告验收结果，不push。（全部验收通过）

## 依赖与并行

Setup→基础门T003–006→各US；US1为首个可运行增量，不单独功能提交。US2沿US1期限；US3共享模型可与Agent并行，真实远程门需US1/2；US4只读Store接口完成后与Bjournal串行集成。所有共享文件同一owner，不允许[P]跨未完成前置。

根唯一写 protocol/server/client、spec/tasks/README/产品与历史；C唯一写 internal/store，B唯一写 internal/agent。A独立Linux实际Android验证；019七份Plan冻结，无任何019 source实施。根在协议/模型门后发送SHA与文档，不让019源码混入008。

- US1 并行例：T007 Store红测与T008 Agent红测不同文件；前置基础门后分别T009/T011。
- US2 并行例：T013 Store保护与T014 Agent真实组取消；T016仅实际缺口，T015不改process/Run。
- US3 并行例：C T018–021与B的US2网络实现；根T022–023按冻结Store接口接线；Store/API/CLI自身依赖顺序保持。
- US4 并行例：C T026–028具体receipt与根T029 HTTP与B T030本地红测；T031集成须接口冻结完成。

## 覆盖与实施策略

| 要求 | 任务 |
|---|---|
| FR001–005 | T007–017 |
| FR006–007 | T013–017、T030–034 |
| FR008–009 | T018、T020–024 |
| FR010–013 | T018–024 |
| FR014 | T016、T019–021、T035 |
| FR015 | T007、T019–024、T033 |
| FR016–017 | T022–025、T033 |
| FR018 | T035–040 |
| FR019 | T026–034 |
| SC001 | T012/038 |
| SC002 | T017/038 |
| SC003 | T024/038 |
| SC004 | T019/024/026/030/033/034/038 |
| SC005 | T005/012/017/024/034/037–040 |

全部4个P1故事是008验收范围。先独立验证首个US，再接实际retry与终态只读核对；不自动repair未知、不恢复旧动作、不建未来能力。所有40任务完成和收敛后才整功能一次提交。
