# 008 正式 Spec Kit 收敛审阅（外部报告）

记录UTC：2026-10-04T19:22:53.226332+00:00
范围：当前root008源码与spec/plan/tasks/原则2.1.0。此前map不是本次converge。本次仅读源码/证据，不运行Git/Go/VM/SSH，不修改仓库。

## Findings 与结果

| Gap type | 数量 |
|---|---|
| missing | 0 |
| partial（新增实现工作） | 0 |
| contradicts | 0 |
| unrequested | 0 |

**收敛结果：008当前规定的实现义务已满足；无需新增implement任务，不追加空Convergence阶段。** 19FR/5SC/13AC、8计划决策、5原则和40原任务已逐项核对。T039由本次正式评估完成后root记录；T040既有最后提交仍待执行，SC005提交部分也待此生命周期门。不能称全部40项已完成或008已提交。

207的cleanupfalse/unknown原证据及guard保留，未归因、未作PASS。当前额外nativeARM完整签名正常/取消质量强化仍独立待验；006/007已有能力不扩展为005/009/019支持；整个MVP未完成。206真实严格取消、实际通用双库/原生Linux恢复/快照/终态门已经验证008要求，不能以额外移动质量门制造新的发布义务，亦不能隐藏其结果。

## 实际流程与字节边界

- 实际check-prerequisites --json --require-spec --require-tasks --include-tasks返回root specs/008-build-recovery；selector008；before/after hooks={}。
- tasks SHA开始/结束相同：`4cef288f53411644a7a0ce232c9454227cc94152bc494531c6f8c9778864a8aa`。无tasks/spec/plan/source/validation修改；外部报告为parent明确授权。
- 当前最新E1与E7直接读取核验；E6日志/源码清单核验。原Linux关键行为的实际记录及实现consumer共同核对，不拿本机编译冒充Linux运行。

## FR → 当前源码 → 行为门/证据

| Stable ref | 当前源码（internal/） | 行为门 | 核对结论 |
|---|---|---|---|
| FR-001 | store/store.go:write、checkLock；server/server.go:ListenAndServe | recovery_lock_test.go:TestRecoveryRetryReceiptCannotUseLostControlLock；server recovery启动坏snapshot门 | 实际SQLite换lockfile/PG终止自有session；Recover失败先于监听 |
| FR-002 | store/recovery.go:Recover/validateRecoveryBuild | TestRecoverPreservesQueuedTerminalAndValidLease；101分页门 | E1 live_ref_number_sha_budget_identity_unchanged/queued_after_restart_uses_old_snapshot_once |
| FR-003 | store/recovery.go；agent/lease.go:accept/check；agent/serve.go | TestRecoveryShortDisconnectKeepsActualRunAndPendingClaim；TestRecoveryGrantCannotReviveExpiredAuthority | E1同物理PID与原完整Ref；heartbeat不写lease |
| FR-004 | store/stop.go:ExpireLeases；claim.go保护计数；agent/stop.go | TestRecoverExpiredLeaseRetainsEvidenceAndGuard；TestRecoverExpirationStopCompetitionPreservesOriginalEvidence | E2 ordinary/always真实失联、旧Ref与guard保持 |
| FR-005 | store/event.go budget/step reducer；recovery.go:frozenBuild；agent/lease.go | TestRecoverPreservesCentralLogArtifactAndNanosecondEvidence；active progress损坏拒绝 | 普通/post NS单调；intent/started/cleanup矛盾拒绝，不再运行旧步骤 |
| FR-006 | agent/recovery_journal.go；serve.go未知材料先拒；store/lease.go:checkBoundary | TestRecoveryUnknownJournalNeverSignalsOldPID；TestRecoveryConfirmedJournalAndOtherUnknownStillRejectServe | E2新Agent拒旧journal；E1旧终态写409 |
| FR-007 | store/stop.go:confirmStopped/ConfirmNodeStopped | TestRecoverExpirationStopCompetitionPreservesOriginalEvidence；TerminalReceiptIndependentStopCannotInventTerminalEvidence | 精确Ref独立停止只清guard，不改原status/reason或制造回执 |
| FR-008 | store/retry.go:Retry；recovery.go:frozenBuild | TestRetryOriginalSnapshotConcurrentAndRelation；retry_artifact_test.go | E1 HEAD/settings推进后新执行仍原SHA/参数/中央binary；无Git/Preview调用 |
| FR-009 | store/retry.go新batch/build/steps/retry_of；query.go | TestRetryFinishedResetsAllExecutionEvidence；TestRetryLeavesOriginalCentralFilesAndLogCursors | E1新号/attempt完整普通预算；旧中央详情/log/artifact不变 |
| FR-010 | store/retry.go状态/号/stop门与retryStopped | TestRetryRejectsWithoutAllocationAndRechecksReplay；TestRetryAttemptWithoutActualStopEvidenceIsNotKnownStopped | attempt全pending伪终态拒；真实checkout_error零动作完成允许；不凭status猜停止 |
| FR-011 | store/retry.go:retryAuthorization；claim.go能力/容量 | TestRetryRejectsWithoutAllocationAndRechecksReplay | 固定原default、原范围与当前范围交集；当前branch/role有效；E3缺工具仍queued |
| FR-012 | store/retry.go canonical operation SHA+同write事务；server/retry.go；cli/client/retry.go | TestRetryOriginalSnapshotConcurrentAndRelation；TestRetryTransactionRollbackPreservesNumberAndEvidence；TestRetryCLIExplicitKeyNoAutomaticResend | E1真实20CLI同key仅一号/一次执行；trigger同key冲突；失败回滚无号 |
| FR-013 | store/retry.go:retryStopped/guard再核；store/store.go事务末checkLock | TestRetryAttemptWithoutActualStopEvidenceIsNotKnownStopped；TestRetryActualConfirmedCheckoutFailureWithZeroStepActions | 精确停止记录或完整实际步骤证据；现发布能力unsupported，无伪unknown解锁 |
| FR-014 | store/enqueue.go:validatePrepared；retry.go生效kind门；pipeline/run.go预检查 | 既有unsupported门与TestRetryUploadPermissionPrecedesSkippedCondition | reports/approval/生效upload未借未来代码；明确false可skipped；005/009/019不算本功能 |
| FR-015 | recovery.go严格冻结结构；scm/checkout.go固定SHA；agent/execute.go zeroAction | Recover损坏结构门；CLI local isolation门 | E3真实删原commit→checkout_error/无用户动作、无新HEAD或伪artifact |
| FR-016 | store/query.go:buildView；server/http.go/trigger.go安全DTO；cli/client/build.go | query_test.go；build_progress_test.go；实际RetryHTTP/CLI关系检查 | E1 retry_relation_list_and_detail/private_tokens_not_in_public_evidence |
| FR-017 | store/retry.go authorize+upload许可先于when；server/retry.go userAuth；cli隔离 | TestRetryInputAndRoles；TestRetryUploadPermissionPrecedesSkippedCondition；HTTP strict/role门 | node/approver不得retry；admin+allow不可被false when豁免；本地坏client配置不影响 |
| FR-018 | 全部当前008范围及本次converge；当前真实验证记录 | E1–E7；完整normal/race/vet与12build/6help-version | 实现与行为门已核对；本次完成T039评估，T040一次提交尚待root执行 |
| FR-019 | store/event.go receipt同终态事务；terminal_receipt.go；agent/recovery_journal.go/file_unix.go | terminal_receipt*_test.go；recovery_journal_test.go；terminal_recovery_test.go；terminal_handoff_test.go | E1 fullartifact终态ACK丢失→当前rotate身份精确只读清本条；oldKind/cleanup/guard拒；单unlink+fsync，无Run/旧信号 |

## SC 与 AC

| SC | 实际证据 | 状态 |
|---|---|---|
| SC-001 | E1两库36同套实际controller exit/restart；有效running+queued各≥1，无重复；E4 Linux36同套 | 已验证行为 |
| SC-002 | E2 Linux普通/always各13门、26PASS；Agent实际网络/租约测试；E5严格206 detached组取消 | 已验证行为；207未知不计PASS |
| SC-003 | E1两库HEAD/settings推进、20CLI并发原SHA retry/newnumber/oldimmutable | 已验证行为 |
| SC-004 | Store双库负例/实际HTTPCLI负例；E3双库24门；journal 13响应负例、特殊文件/旧PID门 | 已验证行为 |
| SC-005 | E1双库72；E2/E4原生Linux关键门；E6/E7最终全检查/矩阵；本次正式converge | 实际检查已通过；既有T040提交待完成，不能宣布整功能已提交 |

| AC | 源码/实际门映射 |
|---|---|
| US1/AC1 | FR-002/008；E1 queued旧SHA一次，推进HEAD仍不变 |
| US1/AC2 | FR-003/005；E1实际同PID完整Ref/号/NS；有界renew |
| US1/AC3 | FR-001/015；真实损坏快照监听前拒、实际lock lost |
| US2/AC1 | FR-004/005；E2到期interrupted/guard/预算与日志产物保留 |
| US2/AC2 | FR-006；E2 ordinary/always无重放、新Agent不信旧PID、旧fence拒 |
| US2/AC3 | FR-007；精确停止竞争门及E2 admin只清guard，不确认结果 |
| US3/AC1 | FR-008/009/011；E1原SHA参数/条件/新号与新预算，原证据不变 |
| US3/AC2 | FR-012；E1 20同key一次新执行/新号，变allow/trigger冲突 |
| US3/AC3 | FR-010/011/013；无号/无row负例、actual checkout窗口与零动作正例 |
| US4/AC1 | FR-016；safe view/关系/original reason/NS/post/stop门 |
| US4/AC2 | FR-017；node/approver/role拒及local配置隔离 |
| US4/AC3 | FR-015；E3真实缺commit/缺tools无替代动作 |
| US4/AC4 | FR-019；E1与E4完整manifest终态ACK丢失，只清当前身份精确自有journal |

## 所有原任务逐项核对

| Task | 核对结果（不依据checkbox） |
|---|---|
| T001 | validation基线/007已验收与当前008消费者记录 |
| T002 | validation所有权/忽略/hook记录；本次selector/hook实读 |
| T003 | protocol/node.go真实TerminalReceipt类型与protocol测试 |
| T004 | models.go/node_models.go/store.go三个最小字段与namedFK |
| T005 | recovery_migration_test.go实际旧007双库迁移；原node_models两门保留 |
| T006 | validation真实migration红绿与冻结交接 |
| T007 | recovery_test.go queued/validlease/坏持久结构测试 |
| T008 | agent/recovery_test.go真实短网络/永久TLS/迟grant测试 |
| T009 | recovery.go 100条主键游标，静态frozenBuild校验+Expire |
| T010 | server.go 30s Recover先于Listen；真实坏queue启动拒 |
| T011 | serve.go/lease.go原claimkey窗口、heartbeat容忍、Authority不复活 |
| T012 | E1双库72+E4 Linux36实际重启/旧SHA |
| T013 | recovery_evidence_test.go到期/Stop竞争与原log/artifact/NS |
| T014 | agent恢复测试与E2两阶段实际组/无关进程/新Agent |
| T015 | http.go/execute.go/artifact.go/spool.go Authority内原消息重试；stop.go实际可能TTL独立确认 |
| T016 | 既有lease.go/stop.go/event.go真实fence与物理保护，无发布假状态 |
| T017 | E2 ordinary/always26+E5严格206取消及旧guard证据 |
| T018 | retry_test.go双库20并发/关系/原证据完整不变 |
| T019 | retry_test.go/retry_evidence_test.go角色/范围/原default/坏facts/upload负例 |
| T020 | retry.go同write事务operation摘要/号CAS/身份授权末检查 |
| T021 | retry.go复制快照仅新id/number/授权收窄，运行证据清零；artifact保持门 |
| T022 | server/retry.go与retry_test.go真实201/200/409/角色严格JSON |
| T023 | client/retry.go/retry_test.go必填key/单request/无参数覆写 |
| T024 | E1推进HEAD/settings、20实际CLI及binary下载/oldimmutable |
| T025 | query.go/http.go/trigger.go/build.go RetryOf安全视图与实际查询 |
| T026 | terminal_receipt*_test.go fullreceipt/unknown/guard/cleanup/跨node只读负例 |
| T027 | event.go receipt.Kind/StopKnown同manifest终态write事务 |
| T028 | terminal_receipt.go当前凭据+原fullRef/seq/digest，不currentExecution/写旧fence |
| T029 | server/agent.go真实terminal-receipt route/严格JSON/currentnode鉴权 |
| T030 | recovery_journal/terminal_recovery tests真实ACKloss、篡改/特殊文件/响应负例 |
| T031 | recovery_journal.go data锁内readonly→重新同inode+fullhash→单unlink/fsync；未知仍拒 |
| T032 | file_unix.go有限128/1MiB/private owner0600nlink1/noFollow/nonblock；不清spool/result |
| T033 | CLI隔离/角色与E3实际缺原commit/tools |
| T034 | E1/E4真实完整artifact终态ACKloss/rotate身份/只读中央不变 |
| T035 | validation串行SHA/gofmt/diff与全范围映射，本次不执行Git命令 |
| T036 | README/PLAN/DELIVERY/history当前状态/命令，未称005/全MVP完成 |
| T037 | E6 normal/race -p1/vet全exit0，E7 12build/6native入口 |
| T038 | E1/E2/E3/E4/E5五组真实quickstart门与安全SHA |
| T039 | 本次正式speckit-converge，0新增缺口/tasks byte未改；由root记录并勾选 |
| T040 | 既有最后一次本地commit阶段待root；不虚构、不重复追加 |

## 计划决策与原则

| ID | 义务 | 当前核对 |
|---|---|---|
| D1 | 单Go module/现有Run/Checkout/process | agent/execute.go唯一pipeline.Run调用；无第二executor/恢复执行旧journal |
| D2 | 独占短事务与fence | Store.write/Recover/Retry/receipt实际锁；lost lock双库真实门 |
| D3 | 原NS预算/intent/post持久 | frozenBuild+event reducer+lease Authority；新retry完整定义预算 |
| D4 | 最小向前迁移 | namedFK ALTER、不关FK/不drop证据；legacy migration双库 |
| D5 | 私有有限journal精确readonly清理 | recovery_journal/file_unix真实消费者；sameinode/fullhash/单unlink/fsync |
| D6 | bounded network/claim/heartbeat/renew | 原起点/原key/原session、迟grant拒；last actual renew仅独立停止确认 |
| D7 | 安全可用HTTPCLI | 完整Ref、固定错误、StrictJSON、RetryOf安全DTO、本地隔离 |
| D8 | terminal/renew ACK交接 | execute.go stopRenew等current request→原Authority复核→单terminal；真实800msACK延迟门 |
| I | SpecKit全链/真实验收/单功能commit | 前置实际成功、红绿/双库实机证据；本次converge；commit既有T040 pending |
| II | Go三入口/单Run/多节点 | 沿既有三入口与唯一Run，无二执行器 |
| III | 最小真实依赖 | 无新增依赖/registry/interface；Recover/Retry/Receipt concrete实际consumer |
| IV | 秘密/授权/fence/Unknown | safeDTO/privatejournal/当前身份；无法确认时guard+未知保留而非放行 |
| V | 中文/docs/真实行为 | 中文规范/注释/README命令边界与原失败保留，不以build当实机门 |

## 实际证据索引

相对路径以 `/tmp/mybuilds-mvp.zKtK0e/` 为根；E5指当前root验证记录保留的原安全证据。

| ID | 文件 | SHA256 | 内容 |
|---|---|---|---|
| E1 | app008-reap-bb1nxhrc/evidence.json | ee5eaf5d0f7cf669f210335cc845ee311935249f43d2ff0e41b80e57d81190de | 最新当前三入口SQLite/PG各36、72全passed，2026-10-04T19:05:24Z |
| E2 | app008-linux-authority-pn_5ybzy/results/evidence.json | 8a8ffdbc7988fcbc7445c724679e63368494664ce738b98387dcd51a63b9c107 | Linux普通/always各13真实断联；原PID/组与无关sleep |
| E3 | app008-negative-zbqmzs5m/evidence.json | f13e9e47c17ea20ff8de8f73104cfa27017502b3180351aeb408f2b2868cba8f | 两库各12实际缺commit/tools/原SHA拒替代 |
| E4 | app008-linux-handoff-_d_2msnc/evidence-native.json | 48e0f15bb5f032686a5862fd825558d6981f0790c668b6dbc4f0ef85c8540b78 | Linux原生36同套snapshot/restart/retry/fullreceipt |
| E5 | specs/008-build-recovery/validation.md：206原采样+root独立核对 | 5681869f144e50d57fd1a25a1114e4696e3a7b965648a9dfead38e0e8e8c8092 | shell/wrapper/detached daemon严格早于finished Gone，未杀无关sleep；driver收尾失败保留 |
| E6 | final008-reap-37a6cuhp/check-evidence.json | 6f7408301ec6e4fe9aeea32b5ed0189b3de989a8576b5a06a6c13b78a0669733 | test -p1 -count=1/race -p1 -count=1/vet全exit0；223来源仅后续stale_group_test严格birth夹具变化 |
| E7 | final008-reap-37a6cuhp/build-evidence.json | 4eebba8979fc93d3e5e9d15b29847a8f3327cb9d9e09154183afe80dd5a7624a | 三入口×四平台12exit0；6nativehelp/version；13process source SHA当前全部一致 |

下一步：root在技能外持久记录本报告并完成现有T039标记；待其决定当前额外质量门结果后按既有T040一次提交。无需追加新的实现任务。
