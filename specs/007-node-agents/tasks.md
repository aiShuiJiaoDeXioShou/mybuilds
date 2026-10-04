# Tasks：007 多节点 Agent 与远程执行闭环

**输入**：`specs/007-node-agents/{spec,plan,research,data-model,quickstart}.md`、`contracts/{go-api,node-protocol,config-cli}.md`及 constitution 2.1.0。

**基线**：006 已验收提交 `8e1397e`；005 未进入基线。本文件是待 analyze 的执行清单，不表示代码或验收已完成。

**测试**：规范要求真实安全、事务、取消与并发行为检查；先写行为测试并确认真实失败，再实现。复用现有双库夹具、实际 Git/process/HTTP，不加 mockexecutor、stub、泛型仓储、测试注入接口或第二执行器。

**格式**：`- [ ] Tnnn [P?] [USn?] 描述及具体文件`。`[P]` 仅表示同一已满足前置的波次中不同 writer、不同文件可并行；同一 writer 内仍串行。未标 `[P]` 不排除依赖满足后其他分区推进。

## 文件归属与串行交接

| writer | 唯一文件归属 | 交接规则 |
|---|---|---|
| A：config001 | `internal/store/**` | 节点身份/session、双库模型、claim/renew/event、取消/停止保护、日志/产物元数据。 |
| B：engine003 | `internal/agent/**`、`internal/scm/**`、`internal/pipeline/**`、`internal/process/**` | root 明确移交 process；唯一 Run、真实 OnStart、固定 SHA、journal/spool/回传及 doctor。 |
| C：preview001 | `internal/config/**`、`internal/server/**`、`internal/cli/**`、`cmd/mybuilds-agent/main.go` | 严格配置/CA、真实 Store/Agent 接线、HTTP/SSE/文件及三入口 CLI。 |
| root | `internal/protocol/**`、`go.mod`、`go.sum`、`README.md`、`docs/plans/DELIVERY.md`、`specs/007-node-agents/validation.md` | 协议先落地；共享文件串行同步、集成复验、文档与最终提交。 |

同文件不并发写。现有 config/server/types 等调整均交给对应唯一 writer；root 不直接同时修改分区文件。新增文件名称如下为具体落点，可在 implement 中最小同包合并，但不得改变 writer 或用空能力代替依赖。协议和实际公共 API 每批由 root 串行同步后才接消费者。

## Phase 1：初始化与协议冻结

**目标**：确认实际基线、约束与完整消息，而非提前构建执行框架。

- [x] T001 root 在 `specs/007-node-agents/validation.md` 记录实际 prereq、006 基线/工作区隔离、checklist/hooks、28FR/7SC/17AC、A/B/C/root ownership 与 process 明确移交；检查 005 未被复制，现有依赖无需新增。
- [x] T002 root 在 `internal/protocol/node_test.go` 先写实际编码/摘要检查：SessionGrant.NodeName、UUID/fence、UTC、nil 与显式 0 普通预算、纳秒 post 预算、private 字段不编码、terminal 显式 0 cursors/空 ArtifactSteps、具体 Progress/Records canonical digest 与 ArtifactExpectation/IDs 字段。
- [x] T003 root 按 `contracts/go-api.md` 在 `internal/protocol/node.go` 实现全部当前具体消息，含 NodeName、完整 LeaseRef、nullable budgets、LocalArtifacts/private PID/PGID/ResultDir；protocol 仅依赖标准库/config，序列正 int64/溢出拒绝，不加版本协商/空未来协议，运行 T002 后串行同步 A/B/C。

## Phase 2：共同基础

**阻塞条件**：T001–T003 完成且协议同步后，T004/T005 可并行；T006/T007 完成并同步前不得启动故事集成。

- [x] T004 [P] C 在 `internal/config/agent_test.go`、`internal/config/tls_test.go`、`internal/config/server_test.go`、`internal/config/client_test.go` 写严格 Agent/server 时序/client CA 行为测试：unknown/type/duplicate/null、0600/owner/普通文件/FIFO/叶 symlink、token 一次解析、仅 MYBUILDS_AGENT_TOKEN 覆盖、相对路径、policy 不合法、HTTPS CA 与 loopback 不读无关 CA、本地命令不受坏配置影响。
- [x] T005 [P] A 在 `internal/store/node_models_test.go` 以实际 SQLite/PostgreSQL 同套测试验证 NULL 未分配身份而非空 UNIQUE、墓碑名唯一、FK RESTRICT、部分唯一索引和分页/到期索引、重复迁移、rollback；所有测试用独立数据库/Schema，不 drop 他人夹具。
- [x] T006 C 在 `internal/config/agent.go`、`internal/config/tls.go`、`internal/config/server.go`、`internal/config/client.go` 实现 LoadAgent/AgentLoadOptions/AgentConfig、Server 时序与 Client CA 字段及 TLSRoots 两个实际消费者；capacity 1–32、heartbeat 1–30s、lease 10–180s 且 lease≥4*heartbeat+2s；受限 PEM≤1MiB、系统 roots 副本/hostname 校验、非空 SSL_CERT_FILE/DIR 安全拒绝；复用有界非阻塞普通文件读取，不 Setenv/InsecureSkipVerify，跑 T004 并同步实际配置 API。
- [x] T007 A 在 `internal/store/models.go`、`internal/store/store.go`、`internal/store/node_models.go` 实现 Node/Credential/Session/Attempt/ExecutionReceipt/StopConfirmation/LogChunk/Artifact 与 Build/StepProgress 扩展；未分配身份/到期用 NULL，Epoch 非负 default0/递增溢出拒绝，project_id/name NOT NULL；`UNIQUE(project_id,name) WHERE status='running' OR stop_unconfirmed=true`，容量取同集合；receipt/log 唯一(build,attempt,seq)，artifact ID 全局唯一及 attempt/seq 唯一，build FK RESTRICT；跑 T005 并同步模型/API。

**Checkpoint**：协议、严格配置与双库真实模型已可用；不存在实现消费者所需的占位 API。

## Phase 3：US1 登记并诊断多个独立节点（P1）

**目标**：独立身份、真实诊断、管理与注册；此阶段不虚称已能执行任务。

**独立检查**：真实 macOS/Linux Agent 注册、诊断、实例竞争与管理操作；身份隔离、工具失败、drain/disable/墓碑行为可不依赖任务执行验收。

- [x] T008 [P] [US1] A 在 `internal/store/node_test.go`、`internal/store/node_session_test.go` 先写双库节点 CRUD/token/session 测试：一次 token/只存摘要、用户或其他节点身份拒绝、永久名唯一、撤销/轮换、同 session 幂等、新 session 不绕 running/guard、2*lease 替换窗口、3*heartbeat 健康窗口、heartbeat 不续任务。
- [x] T009 [P] [US1] B 在 `internal/agent/doctor_test.go`、`internal/agent/data_test.go` 用真实工具和私有目录写诊断/锁检查：工具实际不可运行不报 passed、Linux ARM 不伪造 Android、工具错误/清理不确定后停检查、未初始化 journal skipped、旧 journal unconfirmed、同 data_dir 第二实例拒绝且不按旧 PID kill。
- [x] T010 [P] [US1] C 在 `internal/server/node_test.go`、`internal/cli/agent/root_test.go`、`internal/cli/server/node_test.go`、`internal/cli/client/node_test.go` 写精确管理/角色/严格 JSON/分页及 CLI 行为测试；serve 缺凭据失败，agent 本地 doctor/help/version 不读业务配置/token、不联网；未知 flags/多余参数明确拒绝。
- [x] T011 [US1] A 在 `internal/store/node.go`、`internal/store/node_token.go` 实现 Create/List/Get/SetState/Rotate/Revoke/Delete/AuthenticateNode；NodeActor 每次写重查 CredentialID，labels≤64/项≤64bytes、name 1–64 安全字符兼容现有授权名、capacity 1–32；token 不授用户权限，delete 为 tombstone 不复用名；跑 T008 管理部分并同步。
- [x] T012 [US1] A 在 `internal/store/node_session.go` 实现 OpenNodeSession/Heartbeat 与可信 LeasePolicy，actual tools 固定 8 项集合、≤32 项/Version≤128bytes/固定 Reason；SessionGrant.NodeName 从实际节点派生，报告不能修改 admin 标签/授权；policy 不匹配拒绝，同 session 重发幂等，未确认旧任务禁止新实例接管；跑 T008 session 部分并同步。
- [x] T013 [US1] B 在 `internal/agent/doctor.go`、`internal/agent/data_unix.go`、`internal/agent/data_other.go` 实现 Doctor、0700 目录/0600 journal、owner/无 symlink/独占检查；复用 process.Run 固定工具命令与 9 项宿主环境白名单，单工具≤15s/合并输出≤32KiB/整体≤1m；Android 要求真实 shell/git/java/aapt2/apksigner passed，ios_signing 固定 unsupported，未知宿主签名材料不扫描；跑 T009。
- [x] T014 [US1] C 在 `internal/server/node.go`、`internal/server/http.go`、`internal/server/doctor.go` 接真实 T011/T012 管理、session/heartbeat 与 admin doctor，独立 node Bearer/user Bearer 不互通；路由严格 JSON≤1MiB/分页1–200，安全 NodeView/doctor 不含 token/hash/path/rawoutput；Store 锁失效不授 session/权限；跑 T010 HTTP 部分。
- [x] T015 [US1] B 在 `internal/agent/serve.go`、`internal/agent/http.go` 实现实际 Serve 注册/heartbeat、随机每进程 session、目录独占和 doctor 报告；显式 cfg.Capacity，cfg.Node 严格等于 SessionGrant.NodeName，策略一致且拒 redirect/验证 TLSRoots；旧 journal 不领取/不重放，当前阶段真实完成注册诊断，不用空 claim/Run 函数。
- [x] T016 [US1] C 在 `cmd/mybuilds-agent/main.go`、`internal/cli/agent/root.go`、`internal/cli/server/node.go`、`internal/cli/client/node.go`、`internal/cli/client/doctor.go` 接真实 Agent serve/doctor/version、两端 node create/ls/show/drain/enable/disable/rm/token rotate/revoke 和远程 doctor；本机在线独占拒绝改用 remote，doctor --server/--node 互斥并拒混本地工程 flags，现有本地 init/run/doctor/help/version 不加载远端坏配置；跑 T010 CLI 部分。
- [x] T017 [US1] root 按 `specs/007-node-agents/quickstart.md` 在 `specs/007-node-agents/validation.md` 记录实际 macOS/Linux 注册、不同 token/session/data_dir、二实例争锁、TLS 错 CA/hostname、doctor 无 token 与一次 token/角色/分页管理的二进制证据；核对 AC1.1–1.3。

**Checkpoint**：US1 可独立验收身份/诊断/管理。删除活动任务保护在 US4 加实机故障证据；不宣称任务执行已完成。

## Phase 4：US2 按授权与容量分配独立命名构建（P1）

**目标**：事务唯一领取与真实 generic 执行闭环。为避免假 checkpoint，提前实现本故事所需的 Run/Event/journal/日志确认；无 artifact 的真实任务可完整终态，产物支持留待 US5。

**独立检查**：两个真实节点执行无 artifact 的固定 SHA shell，保存完整事件/日志零或非零 cursor，验证唯一归属、容量、同名串行与不同名并行。只做 Claim 单测不能代替该检查。

- [x] T018 [P] [US2] A 在 `internal/store/lease_test.go` 先写双库 20 竞争、ClaimKey 幂等、授权交集/default/runner/实际工具/labels、global/node capacity、同名锁、旧 session/credential/代次、真实 now==expires 和事务中途到期/失锁测试；Renew 必须提交前检查更新前 oldExpires，Heartbeat 不隐式续全部 lease。
- [x] T019 [P] [US2] B 在 `internal/process/start_test.go`、`internal/pipeline/remote_test.go` 先用真实子进程测试 OnStart 在实际 Start 后调用、错误仍 Wait 一次并组清理、单 Remote build/禁止 All/Step、进度保存失败/日志失败闭锁整 Run 和 post、nil 与 0 预算、Authority 失效的普通/always；本地 nil Remote 行为保持。
- [x] T020 [US2] B 在 `internal/scm/checkout_test.go` 写真实 Git SHA1/SHA256、branch 移动/不可达/commit 类型、HEAD 精确、受限 SSH、hooks/filters/submodule/replace/helper 哨兵、dirty 用户树不变和 cancel/output 上限测试，不读取宿主默认密钥。
- [x] T021 [US2] A 在 `internal/store/lease.go` 实现 Claim/Renew/CheckExecution：当前授权与冻结 AllowedNodes 交集、健康/标签/平台/工具、两级容量/同名保护；ClaimKey 绑定 session，不重复 attempt；完整 fence/TTL/cancel/budgets，queued 原因固定且无 runner 只取默认节点；所有短事务提交前再验原 fence/UTC/锁，Renew 再验 oldExpires；跑 T018 并同步。
- [x] T022 [US2] A 在 `internal/store/event_test.go` 写双库实际事件状态机：intent→started/finished、post_selected/未选 skipped、全 ordinary skipped、完整历史 receipts 同 seq/digest 重放、冲突/跳号/错误 phase/index/预算增长/非法 null/负 ns 拒绝，终态不能残留 intent；artifact/terminal 清单负例先按契约定义，US5 补完整文件成功行为。
- [x] T023 [US2] A 在 `internal/store/event.go`、`internal/store/query.go` 实现 ApplyEvent/完整 ExecutionReceipt 历史、安全 Step/BuildView 与真实 ns 预算；按冻结数组 1-based 索引及真实 post 选择校验，nil 预算语义不能变化，ElapsedNS 不负/预算不增长；零文件 terminal 显式 ArtifactSteps 空及 cursors 核对，非空清单不得绕文件确认；原失败不被 post/cancel 覆盖；跑 T022 当前无产物闭环并同步。
- [x] T024 [US2] B 在 `internal/process/process.go`、`internal/process/process_unix.go`、`internal/process/process_other.go` 实现唯一 `Command.OnStart(StartInfo)` 实际消费者：cmd.Start 成功后给 PID/PGID/UTC，回调失败固定 progress_error 并 cancel/Wait/回收本组，仅一次 Wait，保留已有 process cleanup 保护；跑 T019 process 部分。
- [x] T025 [US2] B 在 `internal/scm/checkout.go`、`internal/scm/git.go` 实现 Checkout 具体 API，自有 attempt 根/detached checkout/HEAD 固定 SHA，复用 gitRunner URL/ref/格式/受限凭据与固定错误；整次≤2m/metadata≤32KiB/Authority 取消，无 hook/filter/submodule/replace/lazyfetch/默认 SSH 或 HEAD 回退；显式复制受限 SSH 文件，旧 ReadPipeline 不接整份 Agent envfile、不改变行为；跑 T020。
- [x] T026 [US2] B 在 `internal/pipeline/run_types.go`、`internal/pipeline/run.go`、`internal/pipeline/preview.go` 实现 RemoteOptions 同一 Run 增量、单选 build、受控 Facts/Secrets/ResultParent 和 Progress；逐动作 Authority→同步 intent→再验 Authority→实际 OnStart→Wait/collector→finished→下一动作，保存错误立即闭锁；本地 API/整批预检查不变，结果根 workspace 外/0700/实际动作后创建；使用纳秒预算并发送 post_selected/未选 skipped，全 ordinary skipped 不执行 post；跑 T019。
- [x] T027 [US2] B 在 `internal/pipeline/log.go`、`internal/pipeline/log_test.go` 接脱敏后、格式化前的真实结构化 Log，提供 UTC/build/phase/index/step/stream；保持跨 chunk 秘密/UTF8/锁语义，Log 回调错误沿 writer 取消当前进程并闭锁整个 Run/always，不能只停日志或解析 Output 文本。
- [x] T028 [US2] B 在 `internal/agent/journal.go`、`internal/agent/spool.go`、`internal/agent/journal_test.go`、`internal/agent/spool_test.go` 实现先本地原子 fsync intent/Started/finished/claimKey 再 HTTP、完整 fence/摘要/ns 与回执 journal；每 attempt spool≤16MiB/4096记录、node≤64MiB/8192记录，先脱敏落盘再提交、ACK cursor 落盘后释放；响应丢失同 seq/digest 重发，满/写失败停 Run；重启只诊断不重放/不按旧 PID kill。
- [x] T029 [US2] A 在 `internal/store/log_test.go` 写双库 LogCommit 的 current fence/UTC/锁、连续 seq+offset/大小≤64KiB/规范 Records 大小/幂等原 ACK、冲突/跳号/终态/旧 lease 拒绝及持久 cursor 测试；日志 bytes 不进入 SQL。
- [x] T030 [US2] A 在 `internal/store/log.go` 实现 CommitLogChunk/ListLogChunks 与 private StorageID/Created 的具体元数据消费者，用户 read 仅 admin/approver、NodeActor 无用户读权限；receipt/offset/current authority 末尾复核，跑 T029 并同步。
- [x] T031 [US2] C 在 `internal/server/agent_test.go`、`internal/server/log_ingest_test.go` 写真实 node HTTP claim/renew/event/log 严格消息/fence/角色/到期与文件-DB 发布间隙测试；限制 JSON≤1MiB、日志≤64KiB/16 Records/Text≤8KiB、错误固定，真实失锁拒绝授予与回报；在 `internal/server/json.go` 的测试中明确 Progress.At/LogRecord.UTC 的 time.Time 是合法 RFC3339Nano 字符串，unknown/duplicate/null 仍拒绝，Task.Definition 则保持现有 config.Build 合法可空字段规则。
- [x] T032 [US2] C 在 `internal/server/agent.go`、`internal/server/http.go` 接 T021/T023 的真实 claim/renew/events/check authority；user/node 路由独立，完整六字段 fence，旧凭据/session/attempt/lease/epoch 拒绝，30s 普通 API 上限与 live BaseContext 取消；最小调整 `internal/server/json.go` 只为 time.Time 校验 RFC3339Nano 字符串，不泛化 codec/绕过严格键与 null 检查，Task.Definition 响应不能套管理请求的任意 null 拒绝；跑 T031 节点路由部分。
- [x] T033 [US2] C 在 `internal/server/files.go`、`internal/server/log_ingest.go` 实现 canonical 日志 stage/hash/fsync→同 Root 新 StorageID 排他发布→短 DBCommit 末尾再验→ACK；事务外文件 I/O、重复删除本候选、DB 失败孤立文件不可读，不接受任何对端路径/非 regular 文件；跑 T031 日志接收部分。
- [x] T034 [US2] B 在 `internal/agent/serve.go`、`internal/agent/lease.go`、`internal/agent/execute.go`、`internal/agent/http.go` 接真实 Claim/Run/Progress/Log：最多 cfg.Capacity 任务、工作区/结果/秘密/journal 独立，续租串行且与 heartbeat 分离；deadline=requestStart+TTL-max(2s,heartbeat)，迟到 ACK 不复活；checkout 耗时扣普通预算、事件/日志按当前有效 fence 同步确认，终态带真实完整零 ArtifactSteps/cursors；先跑无 artifact fixture，不以未实现上传假成功。
- [x] T035 [US2] C 在 `internal/server/trigger.go`、`internal/server/json.go`、`internal/cli/client/build.go`、`internal/cli/client/status.go` 更新无 runner/default 拒绝、queued 固定匹配原因、真实 running/node/session/attempt/lease/epoch/cancel/guard/ns 安全视图与统计；006 server/client 同步升级，旧严格 decoder 兼容限制明确，不公开 Snapshot/token/private 路径、不建版本框架。
- [x] T036 [US2] root 在 `specs/007-node-agents/validation.md` 记录两实际节点 generic 无 artifact Run 的 Git/事件/日志/终态和时间线：一任务只执行一次，同项目同名跨节点串行、不同 build/project 并行，节点/全局容量、未授权/工具缺失排队与 default 规则；核对 AC2.1–2.3/SC1–2，不把 Store 竞争测试当实机执行。

**Checkpoint**：US2 是真实 generic/no-artifact 执行增量，日志可确认、无产物终态完整；有 artifact 的远程终态必须等待 US5 上传和清单核对，不能宣称本阶段已交付该能力。

## Phase 5：US3 执行固定快照并保存真实进度（P1）

**目标**：深化冻结上下文、逐步秘密、ns/post 证据与能力预检查。Android 完整中央产物门明确依赖 US5，最终 T070 复验。

**分段独立检查**：当前可用 generic 真实执行证明固定 SHA/定义/参数、普通/post 与预算/秘密边界；已有 artifact collector 在本地回执中检查快照。Android 中央回传完整场景在 US5 之后验收，不能提前标故事全部通过。

- [x] T037 [US3] B 在 `internal/agent/secrets_test.go`、`internal/pipeline/remote_test.go` 写任务声明秘密/可信 Facts/系统 env 测试：参数伪造无效、host env 不回退、Agent/client/控制端 token 不可引用、其它任务/步骤值不注入，参数/秘密插入 `${...}` 或 `{{...}}` 不再解释，敏感字符串不入日志/安全结果/journal。
- [x] T038 [US3] B 在 `internal/agent/secrets.go`、`internal/agent/execute.go`、`internal/pipeline/run.go` 完成私有 envfile≤1MiB/256键/单值≤64KiB 的 NAME=value 一次受限加载、重复/NUL/格式/缺失安全拒绝；每任务仅声明引用，SSH 两专用键只供 Checkout，token 实际环境名/值禁止注入；由冻结 Task/实际 node/workspace 构造 project/build.name/build.id/build.number/node/git.sha/git.branch/workspace，remote 不回查 Git/宿主秘密；跑 T037。
- [x] T039 [US3] B 在 `internal/pipeline/remote_budget_test.go`、`internal/agent/execute_test.go` 用真实 shell/post/收集器测试 intent 与 Started 间隙、实际开始/回收、ns 累积含准备/日志/进度延迟、0 耗尽、不由 ms 反算/不增长、独立 post 累积、success-post 失败/原失败保留、所有 ordinary skipped；approval/upload/reports/notifications 的生效整条预检查及未验收 ios_signing 拒绝。
- [x] T040 [US3] B 在 `internal/pipeline/run.go`、`internal/agent/execute.go` 完善 T039 所验证的真实 ns checkpoint/预算、实际 post 选择及未选 skipped、原失败/cleanup 副标志、全条预检查；unsupported 的 inactive 条件沿现有 when，不把待定模板事实当 build.when false，不恢复/重置步骤预算；本地 run/003 collector 回归保持。
- [x] T041 [US3] B 在 `internal/agent/journal.go`、`internal/agent/execute.go` 为实际 LocalArtifacts 预分配稳定 ID/连续 seq，artifact finished 的 ArtifactIDs 先落 journal 再发事件；本地 ResultDir/PID/PGID/快照路径仅 private，build_finished 由真实 journal 生成完整 ArtifactSteps/cursors，非空文件 ACK 未完成不得提交成功；US5 T059 接实际上传，不用空清单掩盖产物。
- [x] T042 [US3] root 在 `specs/007-node-agents/validation.md` 记录实际分支/项目设置/触发参数变化后的首次固定 SHA/定义/最终参数、generic 可信上下文/声明秘密/普通与 post/ns/未实现能力负例；artifact collector 独立快照负例可此阶段核对，但中央及 Android 完整门留 T064/T070；核对 AC3.1–3.4 当前可执行部分。
- [x] T043 [US3] C 在 `internal/server/build_evidence_test.go`、`internal/cli/client/build_test.go` 检查真实安全 Build/Step 视图的 phase/index/Started/StopConfirmed/CleanupFailed/ns/post/归属，保留失败原因，扫描 token/秘密/脚本/快照/private 路径；不能用最终 status 推断 post_phase，也不能把 failed 当已物理停止。

**Checkpoint**：冻结执行与 generic 证据通过；US3 的真实 Android 中央产物验收尚待 US5，执行次序在依赖图和最终门明确表达。

## Phase 6：US4 取消与断网时停止本次执行（P1）

**目标**：区分用户取消、执行权消失与物理清理；持续停止保护并独立确认。

**独立检查**：已有真实 generic Run 进行 queued/running/always 取消与断网，核对本次进程组回收、无关 PID、旧回报/停止确认。无需等待 Android 或 SSE/download。

- [x] T044 [P] [US4] A 在 `internal/store/stop_test.go` 写双库 cancel intent/queued cancelled、真实到期等值/中途到期、controller 重启保有效 lease、guard 保同名和容量、node quarantine/删除保护、精确旧 fence 独立确认与原状态/原因不变的检查，expired 成功回报必须拒绝。
- [x] T045 [P] [US4] B 在 `internal/agent/lease_test.go`、`internal/pipeline/authority_test.go` 用真实 HTTP/进程组测试普通及 always 的网络断开、disable/revoke/迟到 renew、忽略 TERM/后台子进程/已退出 leader，合法用户取消可 always、失租/保存/日志错误不可 always，无关进程存活，旧 journal 不重放/不按 PID 自动 kill。
- [x] T046 [P] [US4] C 在 `internal/server/stop_test.go`、`internal/cli/client/cancel_test.go` 写真实 API/CLI queued/running cancel_requested、admin/原 node 独立 stop-confirmation 与全 fence/note、错误身份/空证据/控制字符拒绝、未知 retry/approve flags 拒绝；提前发 TERM 或到期不宣称 cancelled。
- [x] T047 [US4] A 在 `internal/store/stop.go` 实现 Cancel/ExpireLeases/ConfirmStopped/ConfirmNodeStopped：ticker≤1s 核对只有到期项，running 未确认不提前 cancelled，interrupted+guard 占同名/容量；确认只用于原 interrupted+stop_unconfirmed 精确 fence，Note 1–1024bytes 无控制字符、固定证据码，只清物理保护不提交旧 success/解除外部 unknown；跑 T044 并同步。
- [x] T048 [US4] A 在 `internal/store/node.go`、`internal/store/node_token.go`、`internal/store/lease.go` 完成 drain/disable/revoke/enable/delete 与活动/guard 的真实事务约束：drain 原 renew 继续，disable/revoke 旧授予/回报拒绝，enable 不绕 quarantine，墓碑删除无活动/等待/guard且名不复用；跑 T008/T044 关联回归。
- [x] T049 [US4] B 在 `internal/agent/lease.go`、`internal/pipeline/run.go` 完成 ExecutionContext 与独立 AuthorityContext：ordinary/non-always 受用户 ctx，always 去用户取消后立即合并 Authority/剩余 post 预算；失权/日志或持久化错误永久闭锁，迟到 ACK 不复活；系统 process 回收独立有界，CleanupFailed 留真实原失败及停止不确定；跑 T045。
- [x] T050 [US4] B 在 `internal/agent/stop.go`、`internal/agent/journal.go` 保存实际回收证据与 pending 回报；原节点当前独立身份仅在到期 guard 后按精确旧 attempt 确认停止，不以 expired lease 补终态；回执失败保 journal，新进程不重放/不自动 kill，serve 退出仅停止自身当前组。
- [x] T051 [US4] C 在 `internal/server/stop.go`、`internal/server/server.go`、`internal/server/http.go` 接实际 cancel/stop-confirmation 与 expire ticker，控制端重启保有效租约，BaseContext 取消停止后台核对、不把 ctx 取消当 DB 损坏，Store 失锁停止授予/回报；用户与 node 独立确认路由不能互相越权；跑 T046 HTTP 部分。
- [x] T052 [US4] C 在 `internal/cli/client/build.go` 实现 build cancel 与 confirm-stopped 的精确 --attempt/--session/--lease/--epoch/--node-id/--note；结果仅安全持久意图/确认，在线本机管理独占仍拒绝；跑 T046 CLI 部分。
- [x] T053 [US4] root 在 `specs/007-node-agents/validation.md` 保存真实普通/always 断网、取消、撤销、TERM 忽略/后台进程、无关 PID、controller/Agent 重启、旧 ACK/fence 拒绝与 admin/node 停止确认的证据；实测默认最后有效请求起 25s 内停新用户动作后有界回收，不把 timeout 当回收证据；核对 AC4.1–4.4/SC4。

**Checkpoint**：取消、失联及停止保护能在 generic 实机链路独立验收；不可自动迁移/retry，不提供恢复 stub。

## Phase 7：US5 查询中央日志与下载已确认产物（P1）

**目标**：完整产物上传/终态清单核对，中央证据查询、SSE 和校验下载；补齐 US3 的 Android 回传前置。

**独立检查**：真实日志重发与 SSE 重连、spool 背压、快照完整/冲突上传与下载；节点离线后已确认文件可读，部分/坏文件无可信记录。

- [x] T054 [P] [US5] A 在 `internal/store/artifact_test.go`、`internal/store/terminal_test.go` 先写双库文件期望 ID/phase/index/name/完整 canonical meta、连续 artifact seq/幂等/冲突/限额及终态 LastLogSeq/Offset/LastArtifactSeq/ArtifactSteps 精确集合检查；遗漏/额外/重复/count 不符/仅 ACK 子集/未完成 intent 拒绝，零项显式 0/空清单。
- [x] T055 [P] [US5] C 在 `internal/server/artifact_test.go`、`internal/server/log_stream_test.go`、`internal/cli/client/stream_test.go` 写真实 HTTP/SSE/文件传输测试：Header≤8KiB/重复头/Content-Length/Content-Encoding、短流/坏 SHA/旧租约、排他发布后 DB 失败不可见、慢 SSE 超普通 API timeout、Last-Event-ID 冲突、坏中央文件/已有输出不覆盖与角色边界。
- [x] T056 [P] [US5] B 在 `internal/agent/artifact_test.go`、`internal/agent/spool_test.go` 用真实 HTTP/快照验证稳定 ID/seq 先 journal、响应丢失同声明查询/重传、源篡改不改快照、manifest 必须来源全部实际 collector 文件与日志 cursors、超限/背压明确停 Run，不能把成功子集当完整终态。
- [x] T057 [US5] A 在 `internal/store/artifact.go`、`internal/store/event.go` 实现 CommitArtifact/FindNodeArtifact/ListArtifacts/GetArtifact 与最终精确 manifest 核对；每文件≤1GiB、attempt≤128文件/累计4GiB、name 安全 leaf≤255bytes、SHA lowerhex64；只能匹配步骤已 journal/事件确认的 ID 集合，重复所有声明字段相同才返回原 canonical，完整 meta/current fence/真实到期/锁提交前重验；无未完成 intent/可信 cleanup/全部日志与文件确认才终态；跑 T054 并同步。
- [x] T058 [US5] C 在 `internal/server/artifact.go`、`internal/server/files.go` 接真实受限 binary 上传/当前 lease 查询：单 base64url canonical 声明头≤8KiB、禁止重复/压缩、长度与 Size 一致；stage/hash/fsync→排他 StorageID→短 CommitArtifact 再验原到期/锁；artifact流≤2m/Authority 截止且续租可并发，DB 失败孤立文件不可 List/download，冲突不覆盖，同声明重复删本候选；跑 T055 上传部分。
- [x] T059 [US5] B 在 `internal/agent/artifact.go`、`internal/agent/execute.go`、`internal/agent/journal.go` 接实际 collector 快照受限流上传/ACK fsync，绝不直接传 workspace 源；稳定 ID/seq/完整字段重试仅当前 lease，最终 flush 全部日志/产物并由真实 journal 填完整 ArtifactSteps/IDs/cursors，匹配中央 ACK 后同步 build_finished，非空文件不能用空 manifest 提前完成；跑 T056。
- [x] T060 [US5] C 在 `internal/server/log_stream.go`、`internal/server/artifact.go`、`internal/server/http.go` 实现 admin/approver 历史日志/分页/SSE/产物 List/download；SSE id=seq、Last-Event-ID/after_seq 一致、heartbeat15s/写期限5s/默认15m，解除该响应整体 WriteTimeout并用 ResponseController 实际设置，终态完整 manifest 后 end；download 同 fd 检完整 Size/SHA 后发送≤10m，safe ID/name 且 node 离线仍可用，trigger/node 拒读；跑 T055 read 部分。
- [x] T061 [US5] C 在 `internal/cli/client/remote.go`、`internal/cli/client/stream.go`、`internal/cli/client/download.go` 实现具体 JSON/SSE/download 三真实消费者共用配置/token/CA 检查；SSE 不走 1MiB ReadAll/普通整请求 timeout，event≤64KiB、stream-timeout1s–1h、确认 seq 重连不重复、Ctrl-C 关闭，控制字符安全显示；download metadata→同目标目录 stage→流式大小/SHA→排他发布，取消仅删自己 stage、目标存在拒绝，URL/原错误/token 不回显。
- [x] T062 [US5] C 在 `internal/cli/client/logs.go`、`internal/cli/client/artifact.go`、`internal/cli/client/root.go` 接 logs --step/--after-seq/--limit/-f/--stream-timeout（follow 不混 --json）、artifact ls --limit/--offset/--json、download --output 必填；安全元数据/历史 JSON 与现有本地 run 不冲突，未知未来选项拒绝；跑 T055 CLI 部分。
- [x] T063 [US5] C 在 `internal/cli/client/evidence_test.go`、`internal/cli/server/node_test.go`、`internal/cli/agent/root_test.go` 以实际 Server/Store/Agent/Git/Run 运行完整 CLI 管理/doctor/trigger/build/logs/cancel/artifact 链路，检查用户/node 三角色隔离、安全文本/JSON、stream/body/timeout/flags、本地坏 clientconfig 不影响 init/run/doctor/help/version；不使用固定 fake 成功。
- [x] T064 [US5] root 在 `specs/007-node-agents/validation.md` 保存真实日志丢响应重发/断流续读/背压、SSE 慢读和取消、完整快照上传/终态 manifest、中央下载 size/SHA/字节、节点离线、坏摘要/短流/路径/冲突/失权/文件发布后 DB 失败不可见证据；核对 AC5.1–5.3/SC5。

**Checkpoint**：中央日志/文件与终态完整性已通过，US3 的 Android 全闭环此时可执行 T070；仍不声称发布/审批/恢复/清理策略已实现。

## Phase 8：横切检查、真实最终门与收敛

**前置**：T064 完成后 T065–T067 为不同 writer 的同一实际检查波次；root 集成后 T068–T074 串行完成。

- [x] T065 [P] A 在 `internal/store/lease_test.go`、`internal/store/event_test.go`、`internal/store/terminal_test.go`、`internal/store/stop_test.go` 完成最终同套 SQLite/PostgreSQL 20 并发/权限/分页/实际时钟边界/原 oldExpires 提交前再验/独占连接失锁/no writes/receipt 历史/terminal 完整集合/停止保护/重启有效 lease 检查，不用原型成功代替实际 Store。
- [x] T066 [P] B 在 `internal/agent/serve_test.go`、`internal/pipeline/authority_test.go`、`internal/process/start_test.go` 完成真实保存/ACK/journal/spool 文件故障（权限/磁盘等实际失败、无 testhook）、OnStart Wait 一次与日志失败整 Run 停、延期 renew/失租 always/cleanup 未确认/并发秘密/旧 journal 不重放，回归本地 001–004 行为。
- [x] T067 [P] C 在 `internal/server/agent_test.go`、`internal/server/log_stream_test.go`、`internal/server/artifact_test.go`、`internal/config/agent_test.go`、`internal/cli/client/evidence_test.go` 完成所有外部消息/文件/FIFO/symlink/大小/分页/溢出/严格 JSON、CA/hostname/redirect、角色/流关闭/坏下载/无 token 本地 doctor 与未知能力的安全扫描，安全错误不公开输入/raw DB/Git/script。
- [x] T068 root 在 `specs/007-node-agents/validation.md` 运行最终实际应用 SQLite 与独立 PostgreSQL 完全相同 API/CLI/调度/停止/事件/日志/文件场景，记录真实版本/命令/UTC/结果、每库独立资源与失锁证据，不把 PG DryRun 或旧研究当验收；核对 FR025/SC2。
- [x] T069 root 在 `specs/007-node-agents/validation.md` 运行实际两个 macOS Agent 与第三 Linux 节点（独立 token/session/data_dir）跨主机 verified HTTPS；保存合法 CA/SAN、wrong-host/unknownCA、双节点竞争/并行/串行/容量/drain/disable/revoke/断网/保护/日志文件证据；Linux ARM 只报真实通用能力、不冒充 Android，核对 FR026/SC1/4/6。
- [x] T070 root 按 `specs/007-node-agents/quickstart.md` 在 `specs/007-node-agents/validation.md` 完成 US3 的真实 Android fixed-SHA 远程 APK/AAB/version=build.number/JKS 签名/快照/中央回传与下载/取消，核对工具和签名，不改宿主未知材料；T057–T064/T069 完成才验收，核对 AC3.1–3.4/SC3，005真实 Apple及全MVP Linux Android 原门保留。
- [x] T071 root 在 `specs/007-node-agents/validation.md` 记录集成后的 go test ./...、go vet ./...、go test -race ./...、三二进制实际构建/版本/帮助/必要客户端 Windows/Linux 纯 Go 编译、真实双库与 Mac/Linux/Android 结果；只在新失败/改动/疑点下扩大重测，交叉编译不替代工具执行。
- [x] T072 root 更新 `README.md`、`docs/plans/DELIVERY.md`、`specs/007-node-agents/validation.md` 的实际入口/目录/节点配置/CA/管理/日志/SSE/下载/取消/停止证据指南及真实验收状态，清楚区分未实现 retry/审批/发布/通知/报告/retention 和未验收 005；不缩减后续功能门。
- [x] T073 root 对 `specs/007-node-agents/tasks.md`、`specs/007-node-agents/validation.md` 执行 speckit-converge，逐条核对下表 28FR/7SC/17AC 与实际证据；缺口返回所属 writer implement/复验直至闭合，不以任务勾选或 prepared fixture 冒充通过。
- [x] T074 root 在 `specs/007-node-agents/validation.md` 记录最终 hooks/工作区与暂存差异检查，按 git-commit-message 整功能一次本地提交及 hash/message；只暂存007相关规范/任务/代码/验证/文档，不夹005或用户改动、不按任务提交、不 push；仅 T068–T073 全部真实通过后执行。

## Dependencies & Execution Order

```text
T001 → T002 → T003
                 ├─ C:T004 → T006 ─┐
                 └─ A:T005 → T007 ─┴→ US1:T008–T017
US1 → US2:T018–T036（真实 generic/no-artifact Run + 事件/日志确认）
US2 → US3:T037–T043（generic 冻结/秘密/ns/post；Android 全门后置）
US3 generic checkpoint → US4:T044–T053
US4 → US5:T054–T064（artifact 上传/manifest/SSE/download）
US5 → T065–T067 → T068 → T069 → T070（US3 Android完整复验）
    → T071 → T072 → T073 → T074
```

### 实际分区交接

- US1：T011/T012 → C:T014 → B:T015 → C:T016；B:T013 可与 A 身份实现流水推进，但 C 不引用缺失的 Serve/Store。
- US2：A:T021/T023 和 B:T024–T028 完成实际依赖；A:T030 → C:T032/T033 → B:T034 → C:T035 → root:T036。T022 的全文件 terminal 负例可先红，US5 T057 补齐成功闭环；零文件路径不能跳过 cursor 检查。
- US3：T037→T038、T039→T040→T041→T042/T043；本故事 Android 中央产物的独立验收依赖 US5，不宣称全部故事从基础后无依赖。
- US4：A:T047/T048 → C:T051；B:T049/T050 可在真实 A 停止 API 到位后接线；C:T052 和 root:T053 只消费已同步代码。
- US5：A:T057 → C:T058 → B:T059，C:T060→T061→T062→T063，再 root:T064；terminal 包含完整 ArtifactSteps/IDs/cursors，不从已 ACK 子集生成。T070 明确依赖 US5。
- root 协议/共享同步是唯一串行 writer，不与 A/B/C 改同文件；tests 与 implementation 同 writer 串行，标 `[P]` 的测试波次可以并行先红。

## Parallel Examples

| 波次 | 已满足前置 | 真正并行的不同 writer 任务 |
|---|---|---|
| 基础 | T003 协议同步 | C:T004 与 A:T005；其后 C:T006/A:T007 在各自测试完成后推进。 |
| US1 | T006/T007 同步 | A:T008、B:T009、C:T010。 |
| US2 | T017 | A:T018 与 B:T019；B:T020 同 owner 后续串行，不额外标 P。 |
| US3 | T036 | B 在冻结/秘密/ns 文件推进时，C 只在 T040/T041 实际证据字段可用后做 T043；不把存在前置的任务标 P。 |
| US4 | T042/T043 generic checkpoint | A:T044、B:T045、C:T046。 |
| US5 | T053 | A:T054、C:T055、B:T056。 |
| 最终检查 | T064 | A:T065、B:T066、C:T067。 |

## Requirements / Acceptance Coverage

以下映射是实现和真实检查的入口，不是通过声明；每项最终在 validation 留证。AC 使用 `AC故事.场景` 与 spec 顺序对应。

| FR | 任务 |
|---|---|
| FR-001 | T008/T011/T014/T016/T048 |
| FR-002 | T004/T006/T015/T017/T067/T069 |
| FR-003 | T009/T012/T013/T014/T017/T021/T069 |
| FR-004 | T009/T013/T015/T028/T050/T053 |
| FR-005 | T018/T021/T036/T065/T069 |
| FR-006 | T008/T012/T018/T021/T032/T048/T065 |
| FR-007 | T034/T045/T049/T053/T066 |
| FR-008 | T002/T003/T022/T023/T029/T030/T054/T057/T065 |
| FR-009 | T020/T025/T034/T042/T070 |
| FR-010 | T026/T037/T038/T042/T070 |
| FR-011 | T019/T024/T026/T027/T034/T066 |
| FR-012 | T027/T037/T038/T043/T066/T067 |
| FR-013 | T019/T022/T023/T024/T026/T028/T039/T040/T041/T065/T066 |
| FR-014 | T026/T039/T040/T045/T049/T053 |
| FR-015 | T044/T046/T047/T051/T052/T053 |
| FR-016 | T007/T018/T021/T044/T047/T048/T050/T053 |
| FR-017 | T044/T046/T047/T050/T051/T052/T053 |
| FR-018 | T008/T011/T048/T053/T069 |
| FR-019 | T027/T028/T029/T030/T033/T034/T056/T059/T064/T066 |
| FR-020 | T055/T060/T061/T062/T063/T064/T067 |
| FR-021 | T041/T054/T057/T058/T059/T064/T065 |
| FR-022 | T055/T060/T061/T062/T064/T067 |
| FR-023 | T010/T016/T035/T043/T046/T052/T061/T062/T063/T069 |
| FR-024 | T019/T039/T040/T042/T063/T070/T072 |
| FR-025 | T005/T007/T018/T022/T029/T044/T054/T065/T068 |
| FR-026 | T017/T036/T053/T064/T069/T070 |
| FR-027 | T002/T004/T006/T010/T031/T037/T043/T055/T067 |
| FR-028 | T001/T068/T069/T070/T071/T072/T073/T074 |

| SC | 真实验收任务 |
|---|---|
| SC-001 | T036/T069 |
| SC-002 | T018/T021/T032/T065/T068/T069 |
| SC-003 | T039/T042/T043/T059/T070 |
| SC-004 | T045/T047/T049/T050/T053/T066/T069 |
| SC-005 | T028/T054/T055/T056/T057/T064 |
| SC-006 | T016/T052/T063/T067/T069/T070/T072 |
| SC-007 | T068/T071/T072/T073/T074 |

| AC | 任务与最终证据 |
|---|---|
| AC1.1 | T008/T011/T014/T016/T017 |
| AC1.2 | T004/T006/T009/T013/T015/T017/T069 |
| AC1.3 | T008/T012/T013/T015/T048/T053 |
| AC2.1 | T018/T021/T034/T036/T065/T069 |
| AC2.2 | T018/T021/T035/T036/T069 |
| AC2.3 | T007/T021/T036/T047/T053/T069 |
| AC3.1 | T020/T025/T034/T042/T070 |
| AC3.2 | T037/T038/T042/T043/T070 |
| AC3.3 | T019/T022/T023/T024/T028/T039/T040/T041/T043/T059/T066/T070 |
| AC3.4 | T019/T039/T040/T042/T063/T072 |
| AC4.1 | T044/T046/T047/T049/T051/T052/T053 |
| AC4.2 | T045/T049/T053/T066/T069 |
| AC4.3 | T018/T021/T044/T047/T048/T050/T053/T065 |
| AC4.4 | T044/T046/T047/T050/T051/T052/T053 |
| AC5.1 | T027/T028/T029/T030/T033/T055/T060/T061/T064/T066 |
| AC5.2 | T054/T055/T056/T057/T058/T059/T064 |
| AC5.3 | T055/T060/T061/T062/T063/T064/T070 |

## Implementation Strategy

先完成基础与 US1，作为可真实演示的最小节点管理增量；它不代表007整功能完成。随后 US2 用真实 generic/no-artifact 单 Run 完成调度/事件/日志终态，US3/US4 补固定执行证据及停止保护，再 US5 补中央文件与查询流。最后双库、两个 macOS 加 Linux 节点与真实 Android 签名/版本/中央文件复验，converge 缺口继续回到所属分区实现。

所有任务初始未勾选，实施仅在 root speckit-analyze 无阻塞后另行交接。最终只按整功能一次本地提交，不按故事/任务提交、不自动 push；005、后续功能及全MVP原真实验收门不变。

**Hooks**：实际读取 `.specify/extensions.yml`，`hooks: {}`；before_tasks/after_tasks 均无执行项。setup-tasks 实际返回 root `specs/007-node-agents` 和任务模板，按模板保留初始化/基础/US1–US5/横切/依赖/并行/策略结构。
