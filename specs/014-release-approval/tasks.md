# Tasks：发布审批与原节点检查点恢复

**Input**：spec/plan/research/data-model/contracts/quickstart。
**状态**：完整实施；fresh2602094+Root403源baseline。005同步后消费原签名生命周期；商店/Apple真实材料人工待验。
**Tests**：FR-029/SC及原则要求安全/事务/恢复/取消真实红绿；不fake executor/等待推断停止。

## 格式/唯一writer

连续T IDs；US任务有[US1]–[US5]，准备/基础/Polish无story。[P]只标每故事已完成前置后的A/B/C不同测试文件。当前A独占本worktree全部014业务/协议/模型/Run/Agent/Server/CLI/测试/docs；Root独占原event.go/reports.go接线与最终集成，B在独立012 WT仅更新发布消费者。业务文件从其故事始至结束都同writer；同writer不同任务也不能实际同时写同一文件。
路径相对项目根。原任务中的root/A/B/C分区在当前worktree统一由A顺序实施；event.go/reports.go仍Root唯一writer，A只提供实际helper及最小接线patch。最终Root串行合并，不允许跨WT并写。下一分区只能用真编译/行为绿SHA交接，无未来stub。

## Phase 1：前置验收与准备

所有源码任务依本阶段；Root已授权本区完整实施，先保存实际共享baseline并复核契约。

- [x] T001 （root）在 specs/014-release-approval/validation.md 核对已交付008/019/020与Root当前010/011实际发布消费者、保存403源baseline和005后续冻结同步，记录真实hash后在独立014实施WT，逐项复核现有Run/receipt/报告/Publish契约；未知材料与当前发布未完成整功能提交不得当真实验收；不阻塞完整代码与必要自动检查（FR-030）。
- [x] T002 （root）在 specs/014-release-approval/plan.md、specs/014-release-approval/contracts/go-api.md 锁A/B/C/root唯一writer与实际前置类型，复核constitution2.1/hooks和旧行为门，不新executor/框架/依赖，不提供stub（FR-001,FR-029,FR-030）。

## Phase 2：共享协议与真实迁移

依Setup；真实协议与旧008迁移红测→声明/约束绿→SHA交接，之后才能故事业务。

- [x] T003 （root）先在 internal/protocol/approval_test.go 写旧nil/omitempty消息digest不变、explicit[]、null/unknown/重复字段拒、完整旧Ref/seq/digest/nullable与0NS/Resume严格类型红门（FR-001,FR-013,FR-017,FR-028）。
- [x] T004 （root）在 internal/protocol/node.go、internal/pipeline/run_types.go 落具体ApprovalCheckpointEvidence/ApprovalCheckpointLookup/ApprovalCheckpointReceipt/ApprovalResumeEvidence、ErrApprovalPaused与RunOptions.Resume/ConfirmApproval、RunResult.Paused；复用实际019seal/010manifest，不opaque/interface；“body≤64KiB”“JSON深度≤32/节点≤10000”，私有路径不序列化，依协议红门（FR-001,FR-002,FR-028）。
- [x] T005 （A）在 internal/store/approval_migration_test.go 写旧008双库真实迁移红门：原receipt/log/artifact/RetryOf/FK不变、重复Migrate、审批FK RESTRICT与unique(build_id,index)/unique(build_id,revision)、attempt新Ref不能丢原checkpoint归属（FR-005,FR-009,FR-029）。
- [x] T006 （root）在 internal/store/models.go、internal/store/node_models.go、internal/store/store.go 串行落approvalRecord/CurrentApprovalID与真实receipt关联；“Revision正数、不可变”“RemainingBudgetNS nullable”“Status pending/approved/rejected/cancelled/resumed”；immutable完整CheckpointRefJSON+原Seq/Digest/manifest，不只Kind，迁移双库旧门绿后冻结（FR-002,FR-005,FR-009,FR-017,FR-029）。

## Phase 3：US1 安全挂起并查看证据（P1）

独立增量：真实Run→中央完整checkpoint→waiting释放容量，另命名构建执行而同名guard保持；ACK丢/重启不假挂起。

- [x] T007 [P] [US1] （B）在 internal/pipeline/approval_test.go 先真ordinary run/artifact/019报告→approval红门：停止/Close/seal/flush后才pause，无post/build_finished/后续动作；失败/Close/log/0预算不假Paused，原Reason不覆盖（FR-001,FR-002,FR-003,FR-005,FR-017；US1/AC1,AC3）。
- [x] T008 [P] [US1] （A）在 internal/store/approval_evidence_test.go 先真ApplyEvent/中央file/log/report双库红门：完整IDs/cursors/hash/Source/step ledger/stop/noCleanup/Closed，expires/DB锁commit前再验，rollback不waiting/释放容量；先补真paused Recover/controller重启红门（FR-002,FR-003,FR-004；US1/AC1,AC3）。
- [x] T009 [P] [US1] （C）在 internal/server/approval_test.go 先真Store+Handler列表/详情/strict JSON/角色/有界分页红门，params/scripts/material/token/privatepath全不公开，无安全proof不假pending（FR-006,FR-028；US1/AC3）。
- [x] T010 [US1] （B）在 internal/pipeline/approval.go 实现具体pause准备/封存结果，复用唯一Run的process/logger/collector/019seal；系统Close独立15s不依post/trap、无动作/whenfalse不造checkpoint，普通预算含封存/Close，依Run红门（FR-001,FR-002,FR-003,FR-005,FR-017）。
- [x] T011 [US1] （root）在 internal/pipeline/run.go、internal/pipeline/remote.go 串行接前项真实consumer：approval离开普通loop→清理/flush→原Progress checkpoint→ErrApprovalPaused/no post/no terminal；0不回无限，原失败Reason优先，nilResume旧行为保留（FR-001,FR-002,FR-003,FR-005,FR-017）。
- [x] T012 [US1] （A）在 internal/store/approval.go 实现applyApprovalCheckpoint、ListApprovals/GetApproval；短事务原fullRef/seq/hash/ledger/预算/manifest核对才安全commit/immutable保存；“limit1..100、offset0..1000000”，依Store红门（FR-002,FR-003,FR-004,FR-006,FR-028）。
- [x] T013 [US1] （root）在 internal/store/event.go、internal/store/claim.go、internal/store/lease.go、internal/store/query.go、internal/store/node.go、internal/store/recovery.go 串行接真实waiting/approved状态：不Renew/Expire为interrupted、不占node/global容量，同名/原node/workspace守护仍在，node delete拒，未知stop保护不清；Recover先核原immutable checkpoint/receipt/ledger/manifest，paused启动不授权，safe view/count正确（FR-004,FR-005,FR-006,FR-023,FR-024）。
- [x] T014 [US1] （A）在 internal/store/approval_checkpoint_test.go 先双库当前sameNode token/跨node/revoked/disabled与真pending immutable原Ref只读核对红门，再在 internal/store/approval.go 实现ReadApprovalCheckpoint；pending仅原proof，无renew/执行权/伪terminal；approved/cancelled/resumed及新epoch后的门于T037用真实决定/领取验证，不先伪造状态（FR-003,FR-005,FR-013,FR-018；US1/AC4）。
- [x] T015 [US1] （C）在 internal/server/approval.go 接真列表/详情/agent checkpoint lookup，strict body≤64KiB/固定错误/现有角色；root在 internal/server/http.go、internal/server/agent.go、internal/server/json.go 串行注册；依实际Store方法绿，不fake路由（FR-006,FR-028）。
- [x] T016 [US1] （B）在 internal/agent/approval_journal_test.go 先actual inode/全JSON/fsync/PendingEvent红门，再在 internal/agent/approval.go、internal/agent/approval_journal.go 实现有限paused ledger：journal128条/1MiB、0600/单link、FIFO/替换/duplicate/null拒，网络后SameFile+whole digest/fsync确认；unknown拒Serve/no旧PID signal，不用008terminal删pause（FR-003,FR-005,FR-013,FR-018,FR-028；US1/AC3,AC4）。
- [x] T017 [US1] （root）在 internal/agent/execute.go、internal/agent/serve.go、internal/agent/lease.go、internal/agent/journal.go、internal/agent/spool.go 串行接真实pause：proof fsync→stopRenew/join实际request→原Authority内一次checkpoint→确认fsync→旧task bookkeeping结束；丢ACK只readonly，不先idle/重复claim（FR-003,FR-004,FR-005,FR-018）。
- [ ] T018 [US1] （root）在 internal/server/approval_lifecycle_test.go 用实际Server+Agent+Run跑US1/AC1–4/SC-001/SC-007：中央原file/JUnit/log、容量释放/同名guard、controller/Agent真实重启、未知ACK/stop/Close不假waiting；保存UTC/PID/PGID/安全证据于 specs/014-release-approval/validation.md（FR-002,FR-003,FR-004,FR-005,FR-023,FR-024,FR-029）。

**Checkpoint**：T007–T018真实绿后验该增量；只US1是安全等候MVP演示，不是014完整交付。

## Phase 4：US2 精确唯一决定（P1）

依US1真实安全checkpoint；approved仅等待，US3恢复消费者完成前不能提前授lease。

- [x] T019 [P] [US2] （A）在 internal/store/approval_decision_test.go 先双库当前Actor/row/CAS与≥20决定红门：same ID/revision/hash/decision/note原结果，反向/改note/旧审批409、不覆盖审计，reject cancelled/no后续动作（FR-007,FR-008,FR-009,FR-010,FR-011,FR-020；US2/AC1–4,SC-002）。
- [x] T020 [P] [US2] （C）在 internal/cli/client/approval_test.go 先实际HTTP一次写请求/列表/approve/reject/tableJSON红门，精确字段required，不GETlatest补齐/不写自动重试，remote错误固定安全（FR-006,FR-008,FR-010,FR-028；US2/AC3,AC4）。
- [x] T021 [US2] （A）在 internal/store/approval.go 实现DecideApproval，短事务当前approver/admin+CurrentApprovalID/revision/hash，immutable actor/UTC/note/digest；“note≤1024 UTF8字节/无control/拒明确secret引用或token格式，公共意见仅摘要”，重复原结果/冲突固定，依决定红门（FR-007,FR-008,FR-009,FR-010,FR-028）。
- [x] T022 [US2] （root）在 internal/store/event.go、internal/store/query.go 串行接安全reject cancelled/approval_rejected与审计safe view，不造build_finished/旧nodeevent，无post/Prepare/upload；original checkpoint保持，approved未Claim无authority（FR-006,FR-011,FR-013）。
- [x] T023 [US2] （C）在 internal/server/approval.go 接真实DecideApproval的approve/reject/buildID关联/strict body与固定错误；200同内容/409冲突/trigger和node403，角色与状态事务再验，依Store决定绿（FR-007,FR-008,FR-009,FR-010,FR-011,FR-028）。
- [x] T024 [US2] （C）在 internal/cli/client/approval.go 实现既定approvals、approve/reject及--approval-id/--revision/--checkpoint-digest/--note，沿clientHTTPS/CA/timeout/受限token，tableJSON等价，依CLI红门和真HTTP绿（FR-006,FR-008,FR-009,FR-010,FR-028）。
- [x] T025 [US2] （root）在 internal/cli/client/root.go、internal/server/http.go 串行注册实际新入口，复验本地run/init/doctor/help/version不加载坏remote配置，不新增审批token/通知（FR-006,FR-007,FR-028）。
- [ ] T026 [US2] （root）在 internal/server/approval_decision_evidence_test.go 两库真实HTTP/CLI角色+20竞争/丢响应原内容重发/改note或旧revision冲突、审计/原证据/私有串扫描及approved仍不占执行容量/原node delete拒/Recover保持安全pause，覆盖US2/AC1–4与SC-002/SC-005并记 specs/014-release-approval/validation.md（FR-007,FR-008,FR-009,FR-010,FR-011,FR-020,FR-029）。

**Checkpoint**：T019–T026真实绿后验该增量；只US1是安全等候MVP演示，不是014完整交付。

## Phase 5：US3 原node新lease与同Run续执行（P1）

依US1/US2；原已完成证据不能在newRef下改写或重放。只续下一未执行步骤，不重新Checkout。

- [x] T027 [P] [US3] （A）在 internal/store/approval_resume_test.go 先双库真实Claim红门：原node/current session/身份/health/tool/scope/capacity，新lease epoch++同build/attempt/number，old Ref/seq/历史不改；离线/drain/disabled原因、20领取/ACK丢只一次（FR-012,FR-013,FR-014,FR-018；US3/AC1,AC2,AC5）。
- [x] T028 [P] [US3] （B）在 internal/pipeline/approval_resume_test.go 先真command计数/原file/report/log红门：同Run只next index，无旧env/Prepare/baseline/build_started，nil/0/postNS不增长，等待不扣而restore/后续实际扣费（FR-014,FR-015,FR-016,FR-017；US3/AC1,AC3,AC4）。
- [x] T029 [US3] （root）在 internal/store/claim.go、internal/store/lease.go、internal/store/query.go 串行接批准CAS/原node/newcurrent session与权限/能力/容量，Task.Resume完整original proof/ledger/nullableNS，同attempt epoch+1不newnumber；lategrant/未知Claim保持原期限，依Claim红门（FR-012,FR-013,FR-014,FR-017,FR-018）。
- [x] T030 [US3] （root）在 internal/store/event.go、internal/store/artifact.go、internal/store/log.go 串行接oldRef写拒与多epoch fullmanifest核对，保old confirmed Source/declaration/log/hash/body/seq，不重PUT；新seq继续，approval非进程不造Started（FR-013,FR-014,FR-015）。
- [x] T031 [US3] （A）在 internal/store/approval_recovery_test.go 先真双库Recover/TerminalReceipt/retryStopped红门：original fullRef不可变/新active一致；safe pause reject/cancel未resume凭exact中央proof可Retry，已resume未知checkout/旧stop拒，不造build_finished（FR-005,FR-013,FR-014,FR-018,FR-020）。
- [x] T032 [US3] （root）在 internal/store/recovery.go、internal/store/retry.go、internal/store/terminal_receipt.go、internal/store/stop.go 串行接前项：重用T013已完成的paused Recover分支，复验approved并扩resumed-running新session/credential/epoch一致、不Expire/假终态；TerminalReceipt只current build_finished，retry仅真checkpoint未resumed/newEpoch/Closed/fullmanifest，新stop必须currentRef（FR-005,FR-013,FR-018,FR-020）。
- [x] T033 [US3] （B）在 internal/agent/approval_resume_test.go 先实际workspace dev/inode/GitSHA/bytes和journal红门，再在 internal/agent/approval.go、internal/agent/approval_journal.go 实现精确grant/fsync/同node当前token原proof核对；missing/tamper不Checkout/重建，未知grant拒newprocess接管，no旧PID signal（FR-013,FR-014,FR-015,FR-018；US3/AC3,AC5）。
- [x] T034 [US3] （B）在 internal/pipeline/approval.go 实现具体Resume ledger/原StepRun/resultRoot/seq与NS，只prepare未来有效steps，不裁剪Definition/新reports baseline/旧事件；sealedReport变化失败不重开，依Run红门及实际Agent本地证据绿（FR-014,FR-015,FR-017）。
- [x] T035 [US3] （root）在 internal/pipeline/run.go、internal/pipeline/remote.go、internal/agent/execute.go、internal/agent/serve.go 串行接前项唯一Run.Resume/next index；原NS/post/ctx/cursors，迟oldgrant/ACK不启动，原worker stopRenew/join/bookkeeping结束后才新Claim，不复制循环/执行器（FR-005,FR-012,FR-013,FR-014,FR-017,FR-018）。
- [x] T036 [US3] （root）在 internal/pipeline/run.go 与实际005资源接入复核：原Close后发布段无新ordinary不Prepare，真正后续签名run才当前材料Validate/Prepare本次新resources、独立Close；restore/材料计原NS，原artifact/JUnit seal不改（FR-015,FR-016,FR-017）。
- [x] T037 [US3] （A）在 internal/store/approval_manifest_test.go 双库核对新terminal完整旧+新IDs/cursors/Source、oldRef迟写拒、nil/0/ns不增长，用真Decide/Stop/Claim推进approved/cancelled/resumed后，原checkpoint query仍readonly且oldRef不从新attempt反推；008最终receipt必须真正currentKind与stop证据（FR-002,FR-013,FR-014,FR-017,FR-018）。
- [ ] T038 [US3] （root）在 internal/server/approval_resume_evidence_test.go 真Server/Agent/同Run与两node跑US3/AC1–5，controller/原Agent各重启一次，旧动作次数不变next一次、20竞争/丢grant、节点不足等待不迁移、missing/tamper不重建；等待长于NS后合法且restore收费，记录 specs/014-release-approval/validation.md（FR-012,FR-013,FR-014,FR-015,FR-017,FR-018,FR-029；SC-003,SC-004,SC-005,SC-007）。
- [ ] T039 [US3] （root）在 internal/agent/approval_handoff_test.go 真current token/session/readonly checkpoint验证旧worker join/fsync/bookkeeping与新Claim边界，late旧ACK不清新unknown；复验 internal/server/approval_test.go 安全view/resume_reason/SSE下载，真实签名Close/后续Prepare证据记 specs/014-release-approval/validation.md（FR-005,FR-006,FR-012,FR-013,FR-016,FR-018,FR-028,FR-029；US3/AC5,SC-007）。

**Checkpoint**：T027–T039真实绿后验该增量；只US1是安全等候MVP演示，不是014完整交付。

## Phase 6：US4 条件/取消/多审批与发布（P1）

依US3真实恢复；同一精确checkpoint决定不能代其它审批或解除发布unknown。

- [x] T040 [P] [US4] （A）在 internal/store/approval_stop_test.go 先双库approve/reject/cancel/claim红竞争：safe paused直接terminal no post，新running实际stop，迟决定不复活，多approval old ID不影响下个（FR-019,FR-020,FR-022；US4/AC2–4）。
- [x] T041 [P] [US4] （B）在 internal/pipeline/approval_condition_test.go 先真冻结when approvalfalse/uploadtrue缺批准拒、bothfalse无互动/记录/发布、多checkpoint仅当前决定、原reportseal变化拒/no新epoch红门（FR-001,FR-002,FR-021,FR-022；US4/AC1,AC4）。
- [x] T042 [US4] （root）在 internal/store/stop.go、internal/store/event.go、internal/store/claim.go 串行接安全paused cancel与currentapproval/epoch/CAS，已Claim沿原running实际stop；旧pause不能确认新checkout进程已停，依真实竞态红门（FR-019,FR-020）。
- [x] T043 [US4] （root）在 internal/store/node.go、internal/store/project.go、internal/store/query.go 串行接waiting/approved delete拒、drain不resume/disable不伪运行、rotate/currentsameNode/权限收窄；原node/同名/证据guard保持（FR-012,FR-023,FR-024）。
- [x] T044 [US4] （root）在实际010/011 internal/store/publish.go 的AuthorizePublish消费者串行检查本upload前全部approval含skipped，各exact approved record/同attempt/artifact/JUnit；无approval沿原授权，unknown appguard不因approve/stop清；共享实际路径于T002复核（FR-016,FR-021,FR-022,FR-023）。
- [x] T045 [US4] （root）在 internal/pipeline/run.go、internal/agent/execute.go 串行复用B独占internal/agent/approval.go的真实helper接多approval顺序/条件与取消：再次pause Close/seal/原NS，reject/安全cancel no post，sealed report不重开，依真实条件与Store取消/Publish绿（FR-011,FR-019,FR-020,FR-021,FR-022）。
- [ ] T046 [US4] （C）在 internal/server/approval_cancel_test.go 用真Store/Handler safe paused200/late决定409、newrunning原stop receipt、role/unknown保护与现有build cancel；不新增取消API，依真实Store停止分支（FR-019,FR-020,FR-028）。
- [ ] T047 [US4] （root）在 internal/server/approval_publish_evidence_test.go 真双库/两node跑US4/AC1–4：条件/20竞态/多审批/currentstore-app-node授权/unknown，resumed取消actualPID/PGID回收/无关ownsleep活；specs/014-release-approval/validation.md 记012/custom与020后续保护联合门无循环，不造未来字段（FR-016,FR-019,FR-020,FR-021,FR-022,FR-023,FR-024,FR-029,FR-030；SC-005,SC-007）。

**Checkpoint**：T040–T047真实绿后验该增量；只US1是安全等候MVP演示，不是014完整交付。

## Phase 7：US5 本地真实TTY/noPost（P1）

依US1同Run暂停与US3原NS/next cursor；共用引擎，本地yes不授远程发布权。

- [x] T048 [P] [US5] （C）在 internal/cli/client/local_approval_test.go 先actualPTY yes/no/空/非法/EOF/noTTY/SIGINT/超256bytes红门，prompt安全/ctx有限退出/无reader泄漏，dryrun无input/network/material（FR-025,FR-027,FR-028；US5/AC1–3,SC-006）。
- [x] T049 [P] [US5] （B）在 internal/pipeline/local_approval_test.go 真run计数/预算红门：唯一ConfirmApproval，等待长于NS后yes只next、false/EOF/noTTY/cancel无后续/post；Close仍真，upload/notify整批用户动作前拒（FR-025,FR-026,FR-027；US5/AC1–3）。
- [x] T050 [US5] （C）在 internal/cli/client/local_approval_unix.go、internal/cli/client/local_approval_other.go 实现真实FD/TTY/xsys有限poll+userctx、输入≤256、明确yes/no；无--yes/reader goroutine/原样输入日志，不支持平台固定错误，依真实TTY红门（FR-025,FR-028）。
- [x] T051 [US5] （B）在 internal/pipeline/approval.go 实现本地ConfirmApproval消费者，等待只停active timer不关userctx，0不回无限；nil/noTTY/false/EOF/cancel固定Reason+noPost，本地确认无remoteproof，依本地Run红门和TTY绿（FR-025,FR-026）。
- [x] T052 [US5] （root）在 internal/cli/client/run.go、internal/pipeline/run.go 串行接真实callback与原budget/noPost/系统Close，本地不读server/token/坏clientconfig、不创建DB审批，未来steps一次（FR-025,FR-026,FR-027）。
- [x] T053 [US5] （root）在 internal/config/validate.go、internal/pipeline/preview.go、internal/pipeline/run.go 串行保严格approval when/模板/notifyfalse；local有效upload/生效notify整批在任何用户动作前拒，--step不绕validate，dryrun纯数据/noTTY/secret/network（FR-001,FR-026,FR-027,FR-028）。
- [ ] T054 [US5] （C）在 internal/cli/client/local_approval_evidence_test.go 实际客户端subprocess+自有PTY跑US5/AC1–3，yes/no/EOF/noTTY/SIGINT/长等待NS、post计数0、坏client隔离、upload/notify预检/pure dryrun；依真实callback与precheck绿（FR-025,FR-026,FR-027,FR-028；SC-006）。
- [ ] T055 [US5] （root）在 specs/014-release-approval/validation.md 保存macOS/Linux实际TTY/noTTY/ctx退出/无进程或reader泄漏、真实Close/NS证据，本地yes非远端授权，无平台/材料门不计PASS（FR-025,FR-026,FR-029；SC-006,SC-007）。

**Checkpoint**：T048–T055真实绿后验该增量；只US1是安全等候MVP演示，不是014完整交付。

## Phase 8：完整验收/文档/收敛

所有US绿后最终门；一次整功能提交，不按任务/故事提交。

- [ ] T056 （root）按 specs/014-release-approval/quickstart.md 真SQLite/PG同suite、macOS/Linux两个node完成全部20AC/30FR/8SC；签名Close/JUnit原XML/中央file/真实商店授权/容量恢复取消/unknown/SSE下载；落UTC/versions/commands/脱敏证据于 specs/014-release-approval/validation.md，不fake材料/publisher（FR-029,FR-030；SC-001,SC-002,SC-003,SC-004,SC-005,SC-006,SC-007,SC-008）。
- [x] T057 （root）必要受影响go test、race、go vet（原内核故障suite按窗口串行，不重复已绿无变化门）与现有三入口12跨平台build/help/version/gofmt/diffcheck；旧007/008/010/011/019兼容真实门，结果记 specs/014-release-approval/validation.md，不用编译代行为（FR-029；SC-008）。
- [x] T058 （root）同步 README.md、docs/plans/DELIVERY.md、docs/IMPLEMENTATION_HISTORY.md、specs/014-release-approval/quickstart.md 实际入口/能力/限制；自动实现门与用户人工门明确分开，不宣称真实商店/Apple全门或MVP已验收（FR-006,FR-029,FR-030；SC-008）。
- [x] T059 （root）正式speckit-converge以 specs/014-release-approval/spec.md、specs/014-release-approval/plan.md、specs/014-release-approval/tasks.md 稳定FR/SC/AC对真实源码/证据闭环，缺口回implement再验；结果记 specs/014-release-approval/validation.md，不能靠checkbox推断完成（FR-029；SC-008）。
- [x] T060 （root）在 specs/014-release-approval/tasks.md、specs/014-release-approval/validation.md 仅凭真实验收勾完成，检查前置交付与未知材料/012custom/020后续联合门，blocking缺口不得提交（FR-029,FR-030；SC-008）。
- [x] T061 （root）依AGENTS/git-commit-message检查仅相关差异，必要自动门/converge及人工指南就绪后Root一次本地提交不push；交付hash/message关联 specs/014-release-approval/validation.md，避免提交后自改文件造成dirty（FR-029；SC-008）。
- [x] T062 （root）交付 specs/014-release-approval/validation.md、提交hash/message/真实验收与后续012/custom/020联合门追踪，不以局部完成宣称全MVP完成（FR-029,FR-030；SC-008）。

## Dependencies & Execution Order

- Setup→Foundation，前置实际消费者与baseline冻结后进入源码实施，商店/Apple材料人工待验；模型/wire/迁移实际绿SHA串行同步后才能US1。
- US1 Run红→B具体pause→root唯一Run接入；Store红→Acheckpoint/list/get→root状态/容量/guard/paused Recover启动；精确readonly proof绿→C真实route→B本地ledger→rootstopRenew/bookkeeping→完整US1实际门。
- US2依US1真安全pause；A决定红→真实Decide→rootreject/view→C HTTP→C CLI→root注册→20竞争门；US3前approved只安全等待，无placeholderlease。
- US3依US1+US2：Claim/Run两个不同writer红→rootapproved newepoch/完整manifest；A兼容红→root扩current active Recover/Retry/TerminalReceipt（重用US1安全paused Recover）；B本地证据→唯一RunResume→root实际execute/serve接入→真实资源/多epoch/重启/竞争。
- US4依US3的真实恢复Ref：A停止竞态与B条件红→root stop/node/publish/run共享串行→C实际HTTP取消→真实两库/两node/多审批/发布门。
- US5依US1真实暂停与US3原NS/cursor；C TTY与B本地Run红→C真实TTY→B callback→root本地接入/precheck→真PTY全路径。不是五故事全独立，不跨story同文件假[P]。
- 完整验收所有US绿后才全量/文档/converge/一次提交。012/custom及020后续联合门计整MVP，不能反向成为014提交前置。

## Parallel Examples

| 故事 | 前置 | 真实不同writer文件 | 串行后续 |
|---|---|---|---|
| US1 | Foundation | T007与T008独立红测试，另C列表红测 | 实际核心→root共享→增量门 |
| US2 | US1 | T019与T020独立红测试 | 实际核心→root共享→增量门 |
| US3 | US1+US2 | T027与T028独立红测试 | 实际核心→root共享→增量门 |
| US4 | US3 | T040与T041独立红测试 | 实际核心→root共享→增量门 |
| US5 | US1+US3 | T048与T049独立红测试 | 实际核心→root共享→增量门 |

## Implementation Strategy

先真实前置与契约重核，最小wire/迁移红绿。US1真安全挂起可演示，再精确决定、原node同Run新fence续段、取消/发布保护、本地TTY。仅root+A+B+C四槽，root收SHA串行集成复验；无stub/第二执行器/通用状态框架，不移除旧行为门。故事checkpoint供增量验证，全部故事/真实门/converge通过后唯一整功能本地提交。

## 覆盖索引

每任务括号显式FR/SC；US1/AC1–4、US2/AC1–4、US3/AC1–5、US4/AC1–4、US5/AC1–3各有真实增量/evidence门，最后全门再验30FR/8SC/20AC。完整逐ID映射由本轮只读分析输出。

## 当前实施唯一writer覆盖与人工门

T001–T062稳定ID和30FR/8SC/20AC不变。当前A负责全部实际任务，Root唯一接原Store event/reports两文件和最后集成/提交；其余原共享文件在本worktree授权A写。T056中的合法签名、真实商店上传及最终Flutter案例记人工待验；自动部分仍需真实双库/HTTP/原节点/文件/预算/TTY检查，不用模拟上传冒充商店PASS。T061由Root最终提交，本区只交冻结manifest与baseline增量patch。

## 本轮实施交接说明

当前448源字节baseline后的实际014增量由A冻结交付，Root负责最终串行集成与整功能提交。上方生产任务勾选表示真实消费者已落地并受本轮target检查，不将未运行的历史大矩阵任务勾成通过。测试文件按最少实际消费者合并于 approval_test.go 等文件，具体名称与命令见 validation.md。T056/T057/T059–T062 的最终集成、全量复验、整功能收敛和提交由Root继续；合法Apple材料/商店上传及全MVP Flutter联合案例仍人工待验。没有新增执行器、通知或伪造平台能力。

## Phase 9: Convergence

- [x] T063 （Root）按 FR-029、SC-008（partial）串行应用448字节baseline后的014增量，保留最新005/009/010/011/012/015真实消费者；最终发布Authorize及历史只读证明调用仍消费同一审批链、原Grant/Receipt Ref不改，汇总已通过target与Root最后整功能检查记录再提交。不重复新增已绿矩阵，不将独立WT编译或交付manifest当合并完成。

## Phase 9: Convergence

- [x] T064 HIGH 在原Agent publishManifest实际消费已批准同attempt/node审批历史Ref，用原Grant.Ref只读lookup并核原响应，不能改Grant/Receipt摘要或把当前epoch当旧动作归属；联验发布后再次approval的原终态集合。来源FR027/FR029、plan: 发布历史原归属（partial）。

## Phase 10: Convergence

- [x] T065 HIGH 按FR-027、US5/AC3（contradicts）在唯一Run生效步骤的整批预检查拒绝approval notify:true，notify:false与条件跳过保持可用；不能执行任意前序脚本后才发现尚未实现通知。以实际marker的Client/Pipeline红→绿验证。

## Phase 11: Convergence

- [x] T066 HIGH 按FR-014/FR-017、US3/AC1（partial）在最后ordinary审批恢复后，根据已确认原ordinary动作准备原post，真正收尾仍消费原started/预算，不重新执行旧脚本；仅approval/全部普通步骤跳过保持无post。实际两次审批→仅post恢复的原Run及三CLI双库案例红→绿验证。

## 最终实现结论与待人工矩阵

所有生产任务已实现；测试按最少实际文件合并于 approval_test.go、approval_notify_test.go、approval_post_test.go 等，具体证据见validation。勾选只表示本轮已完成的实现/必要自动门；原未勾的跨OS、两节点故障大矩阵/合法签名/真实商店与扩展TTY场景保留集中人工验收，不虚称运行过。用户最新授权将外部及整合人工验收移到完整代码之后，不阻塞整功能代码提交。T061/T062提交及交付结果由实施历史与最终报告追溯，不在提交内自写自身哈希。
