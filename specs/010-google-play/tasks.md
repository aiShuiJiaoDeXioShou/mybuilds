# 任务：Google Play 发布与未知结果核对

**输入**：本目录冻结 spec、plan、research、data-model、quickstart 与三份 contracts；26 FR、7 SC、4 用户故事、17 AC。按 core tasks-template 组织，当前仅文档 tasks→只读 analyze，不开发、安装工具、获取商店凭据或发布。

**前置**：008、009、019 必须真实验收并集成后才实施；本规划 WT 保留 007 已验收基线，不借未提交源码。候选 fastlane 2.240.1/core 1.2.5/bundletool 1.18.3 不是项目已验证锁。无 Google 应用/凭据只可通过工具故障原型门，不能宣称真实 internal 已通过。

**真实红绿**：涉及输入、权限、材料、数据库、恢复、副作用和取消先写实际行为红测，再最小实现转绿。Store 同一套 SQLite/PG；工具计数用真正锁定 Ruby 库与自有 HTTP 故障端点，不用 mock 证明不重发。商店成功和接受后丢回执必须真实 Google；未知请求、非零退出或进程停止不是发布失败证据。

## 格式与文件归属

格式 `- [ ] T### [P?] [US#?] 描述及路径`；`[P]` 只表示前置就绪、文件不交叉。红测与对应实现有依赖；实际 DB/进程/HTTP 故障资源仍须独占协调。

| writer | 唯一写文件 |
|---|---|
| A | `internal/store/publish_models.go`、`publish.go`、`publish_query.go` 及相应新 tests |
| B | `internal/distribute/google_play.go`、`aab.go` 与相应 tests；`internal/pipeline/publish.go` 与新 tests；`internal/agent/publish.go`、`publish_query.go` 与相应新 tests |
| C | `internal/config/publish.go`、`internal/server/publish.go`、`internal/cli/client/publish.go` 与相应新 tests |
| root | `internal/distribute/fastlane.go`、其 tests、`fastlane/Fastfile`、`Gemfile`、真实 `Gemfile.lock`；protocol 的全部共享声明；config 原 types/parse/validate/agent；pipeline 原 Run/RunTypes/Preview；Store 原 models/migration/event/enqueue/recovery/retry/query/artifact；Agent 原 journal/execute/serve/http；server 原 trigger/http/json/files；client 原 root/doctor；README、依赖、tasks/validation 和集成 |

root 写共享代码仅在 A/B/C 真实批次交付后串行接线，禁止同文件并写或抽空方法。业务文件最小接口由 T003 冻结实际类型，不建 GenericSDK/registry/repository/第二 Run/通用任务工作流。011 将来复验同工具目录/锁及应用 guard，不提前建 Apple 分支或空消费者。源码 worktree 从 T002 的真实验收基线新建，分批 SHA 冻结→根复制→真实消费者复验。

## Phase 1：准备

- [x] T001 由 root 在 `specs/010-google-play/validation.md` 保存输入 SHA、selector/preset/core-template/hooks、26 FR/7 SC/17 AC 清单、候选版本/工具与商店条件；区分源码研究、工具原型与真正 internal 证据。
- [x] T002 由 root 在 `specs/010-google-play/validation.md` 核验 008/009/019 正式验收提交与消费者、原生/Flutter 真 AAB 及 019 seal，再建立实施 worktree；缺前置不写 stub，010 提交只等自己的全部门，不把 011/014/020 后续联验作为反向前置。
- [x] T003 由 root 在 `specs/010-google-play/contracts/go-api.md`、`plan.md` 对实际前置 API、唯一归属、完整 publish/terminal 类型做串行冻结；配置 draft/completed 的 release_status 与固定 changes_not_sent_for_review=false 必须进入原授权及实际 track mutation，不在运行中改变选择；数组[]/omitempty 保旧 digest，不加未用未来类型。

## Phase 2：阻塞基础与工具单发门

- [x] T004 在 `internal/distribute/fastlane_test.go` 与自有测试脚本 `internal/distribute/fastlane/test/` 写真实 Ruby 客户端红原型：edit insert、resumable start/content、track update、commit 各自收到请求后断连以及 401/429/500/redirect/迟响应，逐写 endpoint ≤1、原失败后不继续链；实际draft/completed请求及track响应status不一致不得继续commit，对实际 googleauth/底层 HTTP 隐式重发计数，非 mock，不暴露生产 root_url/testhook。
- [x] T005 在 `internal/distribute/fastlane.go`、`fastlane/Fastfile`、`Gemfile` 实现固定 preflight/upload/query 具体入口与实际 Ruby 客户端单发配置：request retries0、无 retry/follow_redirect middleware、net_http max_retries0、普通 token 字符串无写中刷新、commit rescue false；正常一次 resumable start+content 不误判整包重复，无用户 lane/Pluginfile/仓库 Fastfile/交互加载。
- [x] T006 在 `internal/distribute/fastlane/Gemfile.lock` 用真实受限原型验证候选兼容后生成项目锁，并在 `validation.md` 保存完整 transitive 版本/摘要、Ruby≥3.1/Bundler/Java/JAR、实际端点计数与停止证据，T004 全门转绿；任一库不能禁未知非幂等重发则阻塞授权，不能锁文件先写成“已验证”。
- [x] T007 在 `internal/store/publish_models_test.go` 先写实际双库迁移/约束红测，再在 `publish_models.go` 与 root 的 `store.go`/已有模型做最小迁移：binding 唯一(store,app_identifier)、ProjectID 不可变 FK RESTRICT，guard BindingID/IntentID 唯一且无 TTL、intent 唯一(attempt_id,step_index,action)、决定原 actor+key 唯一，旧数据/旧消息保持，不关闭 FK 或新增占位表。
- [x] T008 在 `internal/protocol/publish_test.go` 由 root 先做真实 JSON 红测，再沿 `node.go` 落当前 grant/receipt/lookup/query/terminal expectation 实际类型与安全 digest：可选新增字段 omitempty、秘密 json:-、消息数组[]、receipt digest 排除自身、旧无 upload 事件 digest 不变；未知/重复/null/type/超限拒绝，不泛化协议。
- [x] T009 在 `internal/config/publish_test.go` 与 root 原 parse/validate tests 先红后在 `publish.go`/原类型接入 Google upload 和可选 Agent publish_tools：完整 credentials env 引用、一次模板渲染、唯一固定目标、track 省略 internal、release_status 仅 draft/completed，其余不足 rollout 字段明确 unsupported；未知/重复/null/type/深度/字节边界不降，未配置不加载工具或影响 generic。

**Checkpoint**：T001–T009 是故事实施前置。工具单发门不能用需要真实商店秘密的验收替代，也不能冒称商店成功；绑定/一次授权在 US2 落地后才可接 US1 的实际发布。

## Phase 3：US1 — 发布原生与 Flutter 的固定 Android AAB（P1）

**目标**：唯一原 AAB、真实包/版本/证书及声明 JUnitSeal 都核对后，沿同 Run 一次受控 internal 发布。

**独立测试**：同一分发实现分别接真实原生和 Flutter AAB，保原 build 名/Number/SHA；真实 Google 可见并对应原证据。无需 production 发布。

### 先写红测

- [x] T010 [P] [US1] 在 `internal/distribute/aab_test.go` 写真实已签 AAB 与篡改/未签 entry/错 cert/package/name/code、多 AAB/APK、symlink/hardlink/FIFO/替换/大小摘要冲突门；同 attempt 中央确认普通 artifact 唯一匹配，拒别 build/post/工作树重新 glob，用实际 JDK/bundletool，不以预置包当新构建成功。
- [x] T011 [P] [US1] 在 `internal/agent/publish_test.go` 写真实私有材料/journal 红测：0600 普通 no-follow/nonblock/fstat 文件≤1MiB、service_account JSON/固定 token_uri，拒 ADC/authorized_user/external_account/credential_source/宿主材料，复制/hash 稳定，密钥内容/token/路径不出 argv、日志或中央；请求前 UUID+fsync、收到 grant 后 once fsync 失败不运行。
- [x] T012 [US1] 在 `internal/pipeline/publish_test.go` 写真实 Run 红测：只普通构建/收集后 upload，019声明报告必须 passed+sealed（required=false missing 仍拒），无报告为无证据而非假 passed；local 有效 upload 全批首脚本前拒，dry-run 无 secret/OAuth/商店，nil callback 仍明确 unsupported，生效未交付 approval/Apple/custom 拒绝。

### 实现与真实故事门

- [x] T013 [US1] 在 `internal/distribute/aab.go` 实现稳定快照副本、实际 bundletool validate/dump 与 jarsigner 全内容签名核验；versionName 等原 params.version、code 等 Task.Number 且≤2100000000、公开 upload cert SHA256 等 verified binding；使用自有公开 truststore 避免合法自签误拒，不读默认 keystore、不重新签名/构建，T010 转绿。
- [x] T014 [US1] 在 `internal/distribute/google_play.go` 实现 PrepareGooglePlay：具体受限材料/工具包摘要/只读包检查、preflight GET+OAuth，token 在授权前取得且覆盖原受限窗口，不写中刷新；私有0700目录/0600输入、空 HOME 与明确必要 Env，去除宿主代理/FASTLANE/BUNDLE/RUBYOPT/DEBUG，沿 process.Run 无第二 executor，T011 材料门转绿。
- [x] T015 [US1] 在 `internal/distribute/google_play.go` 实现 UploadGooglePlay：仅原 grant 固定 app/track/releaseName/ReleaseStatus/code/摘要/选择，实际一次 edit→原 edit 版本冲突检查→AAB→核 code/SHA→track响应核版本/名称/draft或completed→commit；阶段异常/不一致/丢回执停止链且 unknown，可信完整链才 uploaded；原冲突不改计数/包、不再建 edit 或 commit，调用真实 OnStart 与独立自有 Close。
- [x] T016 [US1] 在 `internal/agent/publish.go` 与 root 原 `journal.go`/`execute.go` 接唯一 Publish 实际 consumer：Prepare→原 IntentID 已 fsync→authorize→grant/once 已 fsync→Upload→原 receipt 已 fsync→精确回报；grant 响应可能已持久化却丢失不能再 authorize/执行，保存/失权闭锁而保 guard；T011 完整门转绿，依赖 T026 真实授权 Store。
- [x] T017 [US1] 在 `internal/pipeline/publish.go` 与 root 原 `run.go`/`run_types.go` 接 RemoteOptions.Publish，传真实 Index/冻结 Step/原 artifact/report/OnStart、当前剩余普通预算×Authority；所有前置/授权/回执耗时计普通 ns、post 保原规则、Close 独立有限清理，T012 转绿，不调用第二 Run。
- [x] T018 [US1] 在 `internal/distribute/google_play_test.go` 和 `internal/agent/publish_test.go` 用实际 process/真实 Store HTTP 验 raw stdout/stderr 各≤64KiB、结构 result≤64KiB、解码/超限/secret/无完整 result exit0 仍 unknown、清理未知禁止后续动作；公开只固定 summary，组已停与远端结果分开。
- [x] T019 [US1] 在 `specs/010-google-play/validation.md` 核诊断真实 Ruby/Bundler/锁/JAR、明确应用已有首接入条件与只读权限；缺工具/错 credentials/权限不假通过，不交互登录/接受协议/下载工具，Google 写权限仅在原受控发布中真实验证，不以 GET 冒称可上传。
- [x] T020 人工验收准备与证据登记（原真实验收由用户集中执行，不宣通过）：[US1] 在 `specs/010-google-play/validation.md` 用授权 Google 应用实际发布原生 AAB internal，核原SHA/build名称/Number/version/AAB SHA/019 seal/intent/receipt 与真实 Google 轨道可见；对错 app/code/name/cert/改包/报告 missing/fail/changed 证明任何发布写前拒绝，材料不足 pending。
- [x] T021 人工验收准备与证据登记（原真实验收由用户集中执行，不宣通过）：[US1] 在 `specs/010-google-play/validation.md` 用 009 已验收真 Flutter AAB复验同一发布链与命名 build选择/独立编号，中央原AAB/XML下载重算；无重新构建/替换产物/默认 production，不能以原生结果代 Flutter 门。

**Checkpoint**：US1 四个 AC 必须在 US2 的 verified binding/一次授权实接后才完成；US1 工具包与 AAB检查可先独立实现，真实发布不提前绕授权。

## Phase 4：US2 — 明确许可、唯一归属与应用串行（P1）

**目标**：admin+allow_upload 先于 when，已核验唯一项目 binding，一应用无 TTL guard，一 grant 只允许运行一次。

**独立测试**：SQLite/PG 同套 20并发至多一 grant；不越轨道/归属/期限，SQL失败无命令，重复授权不重新发执行权。

### 先写红测

- [x] T022 [P] [US2] 在 `internal/store/publish_test.go` 写真双库红测：pending/verified binding 唯一项目且组迁移不改归属、node 当前 token/session、精确 Ref/冻结步/普通 artifact/seal、ordinary剩余ns>0，20同 app不同 build/node申请≤1 grant；从冻结step派生draft/completed（省略completed）及AuthorizationDigest，错值/另一选择回执拒绝；重复请求409、不返新 grant，SQL回滚和提交前 lock/旧 expiry失效拒绝。
- [x] T023 [P] [US2] 在 `internal/server/publish_test.go` 与 root trigger/retry tests 写真实身份红测：定义有 upload 即 when false也须 admin+allow_upload，trigger/approver无写权；track省略internal、binding允许精确集合，production原明确选择、retry不扩大；坏client配置不影响local help/init/doctor。

### 实现及故事验证

- [x] T024 [US2] 在 `internal/store/publish.go` 实现 BindApplication 与实际绑定doctor排入 `publish_query.go`：AllowedTracks非空默认internal、Status pending/verified、ProjectID不可变、明确 NodeID/公开证书 SHA、中央只 CredentialRef 名不含内容；真实当前原node的限时 GET/锁摘要结果才能 verified，相同binding幂等不重置验证、不跨项目。
- [x] T025 [US2] 在 `internal/server/publish.go` 与 root 原 `trigger.go`、Store `enqueue.go`/`retry.go` 接绑定/触发许可及严格 Google 有效能力门；冻结一次渲染后的目标/轨道/状态/显式 production，未交付生效 approval 始终拒绝，019 seal 真实接入后才放开上传拒绝，不复制 Trigger→Preview→Enqueue。
- [x] T026 [US2] 在 `internal/store/publish.go` 实现 AuthorizePublish、RecordPublish 与 FindNodePublish 的最小当前消费者：授权短事务重验原 NodeActor、oldExpires/控制锁，insertIntent+guard 后默认unknown才一次grant；原 ID/digest/Task/产物/seal全绑定，精确receipt幂等/冲突、stage receipt不清guard、readonly lookup不发grant，T022/T023转绿，不用 TTL/停止释放 app保护。
- [x] T027 [US2] 在 `internal/server/publish.go` 接真实 node authorize/receipt/lookup 与 admin绑定 routes，沿 strictJSON≤64KiB/角色+当前节点分离/固定安全错误；实际 Store方法接线无stub，冻结 app/轨道/Version不让客户端或query覆盖，不公开授权digest/token。
- [x] T028 [US2] 在 `internal/agent/publish_test.go`、`internal/server/publish_test.go` 与 root原 event测试先红后实接 `store/event.go` 的一次命令/slot闭合：step_finished短事务核全部intent停止且确定才释放对应guard并拒后来授权；之后 readonly FindNodePublish 当前原node只能取原Ref/Index/IntentID，不取得新 grant、活动slot404不证未授权，Agent准备全部已授权原ID/digest/unknown，最终manifest核对由T032补齐。
- [x] T029 [US2] 在 `specs/010-google-play/validation.md` 记录实际双库20申请/SQL失败/失锁/expiry/production拒绝/跨项目binding/变组/授权丢失、真实Google版本冲突；至多一次授权/调用、不改计数器或包，20竞争门与版本冲突必须实际验证，public production不作默认测试。

**Checkpoint**：US2 五个 AC 与全部实际权限门通过后，US1真实 internal 方可完成；已授但无回执的原请求始终 unknown，不能因为没看到 Started 当未发送。

## Phase 5：US3 — 中断后只读核对与精确决定（P1）

**目标**：unknown guard 跨重启、停止、retry 保持；查询只GET原轨道，不创建/删除 edit，证据不足只留观察，不假确认。

**独立测试**：实际已接受商店上传丢回执，节点/控制端重启无第二上传；精确 admin 外部证据确认和冲突/重复事务套件，停止仅解除物理guard。

### 先写红测

- [x] T030 [P] [US3] 在 `internal/store/publish_query_test.go` 写双库红测：unknown跨 Recover/Retry/Expire/StopConfirmation保护不变，query原node当前身份/nonce/session/30s提交前期限、每node同时1、每绑定/intent待处理≤1；不足/无/多匹配/obsolete/网络fail不改原结果，confirm精确digest/有界依据/幂等与并发冲突。
- [x] T031 [P] [US3] 在 `internal/distribute/google_play_query_test.go` 写实际锁Ruby客户端 GET计数红门：query不得 insert/delete-edit/upload/update/commit、不得凭releaseName/code当AAB hash；unknown缺完整原Bundle链仍unknown，已可信uploaded精确track/name/code才推进实证生命周期，30s原期限不延旧 lease或占slot。

### 实现及故事验证

- [x] T032 [US3] 在 `internal/store/publish.go` 与 root原 `event.go`、`recovery.go`、`retry.go`、artifact关联中对T026实际回执入口联验完整terminal核对，stage receipt不清guard，unknown不随build终态或StopKnown清除；可信明确未调用写/远端拒绝+之前无接受或unknown链才failed，exit/timeout/404/emptyquery不足；retry新号不复用旧intent，T030保护门转绿。
- [x] T033 [US3] 在 `internal/store/publish_query.go` 实现 Request/Claim/CompletePublishQuery 与实际绑定doctor管理消费者，原node当前token/session重验、30snonce期限、同结果digest幂等，满构建容量仍可一次只读管理查询；GET不足保unknown及安全观察，不获得旧Ref执行权。
- [x] T034 [US3] 在 `internal/distribute/google_play.go` 实现 QueryGooglePlay 仅官方TLS GET releases≤20、有限safe remote字段/限输出/无redirect与旧写入口；空/歧义/权限失败保原结果，原unknown不能因code/marker推uploaded，internal PUBLISHED只证该轨道可用，T031转绿。
- [x] T035 [US3] 在 `internal/agent/publish_query.go` 与 root原 `serve.go`/`http.go` 接具体 doctor/query循环，用当前独立node身份、原nonce与原期限，固定process调用、不Claim/Run、不重建/重传/占build槽；材料仍指定secret受限读取，节点离线保持原intent，错误/撤销闭锁新管理动作。
- [x] T036 [US3] 在 `internal/store/publish_query.go` 实现 ConfirmPublish：ExpectedIntentDigest/原intent/原app、actor+DecisionKey 唯一、Outcome/EvidenceCode实际充分性、Note≤2048B且拒控制/机密、依据摘要与有限远端标识；重复原结果、冲突409、事务只改原行/删精确guard并审计，不接受stop/exit/no_result当failed依据。
- [x] T037 [US3] 在 `internal/server/publish.go` 接admin query/confirm/查询结果与当前node管理routes，真实Store角色及nonce/fence复核，文件/JSON限额与安全DTO；Node不能调用admin决定，approver只读、错归属/旧session不写，T030 HTTP关联门转绿。
- [x] T038 人工验收准备与证据登记（原真实验收由用户集中执行，不宣通过）：[US3] 在 `specs/010-google-play/validation.md` 用真正Google接受写后丢回执、grant丢失及启动前失联证unknown，控制端/Agent重启、retry/停止/重复receipt无重发同意图；保原AAB/XML/intent/journal，用真实GET不足仍保护，外部控制台精确有依据confirm可解除，仅工具原型不代SC004。
- [x] T039 [US3] 在 `specs/010-google-play/validation.md` 记录双库 query/confirm竞争、重复/冲突、wrongactor/ref/digest、原记录已可信上传的后续实证状态；未知保护证据关联已存在且无清理入口；014/020真实联验留其各自交付门，不反向阻塞10或提前清数据。

**Checkpoint**：US3 五个 AC 通过；进程已停止可释放构建执行保护，却不是商店结果确认，也不能释放未知应用保护。

## Phase 6：US4 — 安全详情、真实生命周期与非占槽查询（P2）

**目标**：CLI 表格/JSON共享safe DTO，退出0不等于published，上传动作停止后继续排队任务，核对不重传。

**独立测试**：实际二进制不同身份访问发布详情；真实商店查询对照，机密标记扫描各公开面；构建结束后下一个任务运行、满capacity query仍只读。

### 先写红测

- [x] T040 [P] [US4] 在 `internal/server/publish_view_test.go` 写真Store角色/分页/安全DTO红测：admin/approver只读、trigger/错误身份拒读，limit1..100、稳定after ID，Application/Publish/Query不含credentialref/参数/授权digest/privatepath/script/raw HTTP；ObservedLifecycle 不混作已确认结果。
- [x] T041 [P] [US4] 在 `internal/cli/client/publish_test.go` 写真实HTTP CLI红测：app bind/ls、publish ls/show/query/query-show/confirm，表格JSON同DTO、confirm0600普通≤合同限额 strictJSON私有文件、写失败不自动重发；本地help/version/init/doctor隔离无client材料，显式target doctor也不要求控制端/node token，敏感值不入argv/stderr。

### 实现及故事验证

- [x] T042 [US4] 在 `internal/server/publish.go` 与 `internal/store/publish_query.go` 实现安全绑定/发布/查询读方法与审计view，root原build/query DTO添加可选PublishIDs及安全结果；只记录实证适用uploaded/processing/submitted/published/failed/unknown，保观察时间/有限enum，退出码或internal可见不证production，T040转绿。
- [x] T043 [US4] 在 `internal/cli/client/publish.go` 与 root原 `root.go`/`doctor.go` 接具体命令和 `doctor --target google-play --agent-config --app-id --credentials-env` 显式GET诊断；未指定target不加载业务gems/config/token，沿当前TLS/ca/token与safe errors，query不暗建edit/上传，T041转绿。
- [x] T044 [US4] 在 `internal/agent/publish_test.go` 与 `specs/010-google-play/validation.md` 实际 publisher返回后普通/failure/always规则、原ns/取消/失权/journal/log错误、实际独立后台组回收与无关sleep存活，Close自有资源无替换误删；未知应用guard可保同时StopKnown释放buildslot，下一queued任务运行，满capacity query不占槽。
- [x] T045 [US4] 在 `specs/010-google-play/validation.md` 记录真实CLI/HTTPS/Google状态对照与所有角色读写边界，注入自有密钥JSON/private_key/token/path敏感标记在预览、argv、raw外泄、中央日志/snapshot/表格/JSON/error为零；bounded工具输出失败仍unknown，完整原产物与报告可下载核验。

**Checkpoint**：US4 三个 AC 通过；安全远端观察不越权升级为确定结果，节点不为商店处理长期占build槽。

## Phase 7：兼容、验收和一次交付

- [x] T046 由 root 在 `README.md`、`docs/plans/MVP_EXECUTION.md`、本功能 `quickstart.md` 和相关Android示例准确写工具部署/候选锁验证、受限材料、显式internal与production/unknown/query/confirm、安全命令；框架与渠道独立，不自动登录/安装/协议/元数据管理，011/014/020仍标后续联验。
- [x] T047 执行本功能必要普通/race/vet与三入口六平台18次编译、6本机help/version，登记真实退出码；全MVP合并后再统一执行一次全量检查，避免按模块重复长矩阵。原真实商店验收按用户安排独立待验。
- [x] T048 在 `specs/010-google-play/validation.md` 按下表逐FR/SC/AC核命令UTC/源码二进制及实际工具锁SHA/受限授权引用/端点计数/Google实际版本/原包XML/intentreceipt与证据，原生及Flutter真实internal门都必须完成，缺凭据/接入条件pending而非accepted；未来014/020联合门明确归各自功能。
- [x] T049 对 `specs/010-google-play/spec.md`、`plan.md`、`tasks.md`、最终代码与 `validation.md` 执行只读 `$speckit-converge`，实质缺口继续 implement/必要复验/再次收敛；001–009/019依赖与本功能全门未过不标完成，不等待011共享未来消费者才交010。
- [x] T050 按项目提交技能检查本功能差异/暂存，在 `specs/010-google-play/validation.md` 确认所有必要门后一次本地010功能提交；不按批次/任务提交、不push、不夹未验收source，后续011共享兼容及014/020联合证据在其功能正式接入时记录。

## 依赖、增量策略与并行例子

1. T001→T002→T003；T004红→T005→T006真工具单发锁门不可跳。T007/T008/T009可在接口冻结后分别独占文件实现，但root共享models/protocol/config/tool所有接线串行。无工具确认不能进入任何商店发布。
2. US1前置/准备：T010/T011独立不同目录红门，T012需019真实消费者；T013/T014→T015。US2的T022/T023可与AAB/材料实现分区并行，T024/T025→T026→T027。T016/T017等真授权Store+HTTP后接线，T018等工具与consumer；绑定verified及T019/T020/T021/T029实际整门还等T033/T034/T037→T035的真实doctor管理消费者，以及T028/T032完整回执/slot门，不会先无权限“独立发布”。
3. US2是US1真实发布的必要安全依赖；同P1可交错实施，但不能绕过它。在等待商店材料时仅推进已授权无凭据工具/实际Store门，story真实internal仍pending。
4. US3：T030/T031独立红门→T032/T033/T034；T036及T033→T037真实管理routes，再T035接Agent管理consumer，最后T038/T039。T027先接实际已完成的授权/回执/lookup/绑定入口，不接不存在的管理方法；T033/T037/T035就绪后才验证binding doctor、释放US1/US2真实发布门。原node仅查询管理工作，不新Claim，终态后无旧权限。
5. US4：T040/T041分别新server/client tests可并行写；T042/T043等真Store和routes，不用fake服务宣称完整链；T044/T045等实际publisher/query/CLI，进程域与商店副作用测试独占。
6. T046–T050等全故事与自身native/Flutter真实internal门；本次仅完成task文档，US1是首个最小业务增量但安全依赖US2，不能把两者及unknown/权限门裁成另一个MVP。

**实际可并行例子**：US1 B写aab红门T010、root/另一独占Agent文件writer按表B串行T011；两任务文件不冲突但单一B写入仍顺序。US2 A写Store T022、C写Server T023可并行；US3 A写Store T030、B写library GET T031可并行；US4 C的server/client文件T040/T041无交叉可拆经root明确移交后并行，不能默认为同文件多writer。root工具/协议/所有旧共享文件永远串行。真正publisher使用同process，Store/user/node始终真实身份。

## 需求覆盖

| FR | 对应任务 |
|---|---|
| FR-001 | T009 T015 T020 T021 T025 T026 |
| FR-002 | T013 T020 T021 T047 |
| FR-003 | T004 T005 T006 T014 T019 T024 T043 |
| FR-004 | T008 T011 T014 T018 T040 T041 T043 T045 |
| FR-005 | T010 T013 T018 T020 |
| FR-006 | T008 T012 T013 T016 T017 T022 T026 T028 T032 |
| FR-007 | T012 T017 T020 T021 T022 T026 T047 |
| FR-008 | T009 T022 T023 T025 T026 T029 T047 |
| FR-009 | T003 T009 T023 T025 T026 T029 |
| FR-010 | T007 T022 T024 T026 T029 |
| FR-011 | T007 T011 T016 T022 T026 T027 T028 T029 |
| FR-012 | T013 T015 T020 T029 |
| FR-013 | T008 T016 T017 T022 T026 T032 T044 |
| FR-014 | T004 T005 T006 T015 T016 T018 T028 T029 |
| FR-015 | T011 T016 T018 T028 T030 T032 T038 |
| FR-016 | T007 T026 T028 T030 T032 T038 T039 T044 |
| FR-017 | T015 T018 T030 T032 T036 T038 |
| FR-018 | T030 T031 T033 T034 T035 T037 T038 T043 |
| FR-019 | T030 T036 T037 T038 T039 T041 T043 |
| FR-020 | T008 T015 T018 T031 T032 T034 T040 T042 T045 |
| FR-021 | T017 T033 T035 T038 T044 |
| FR-022 | T027 T037 T040 T041 T042 T043 T045 |
| FR-023 | T007 T030 T032 T038 T039 T045 T048 |
| FR-024 | T009 T012 T017 T023 T047 |
| FR-025 | T002 T003 T008 T009 T012 T025 T028 T032 T046 T047 |
| FR-026 | T001 T006 T019 T020 T021 T029 T038 T039 T044 T045 T047 T048 T049 |

| SC | 对应任务 |
|---|---|
| SC-001 | T015 T020 T021 |
| SC-002 | T009 T010 T011 T012 T013 T022 T023 T026 T029 |
| SC-003 | T004 T005 T006 T011 T016 T022 T026 T028 T029 T038 |
| SC-004 | T030 T031 T032 T033 T034 T036 T038 T039 |
| SC-005 | T008 T011 T018 T023 T027 T037 T040 T041 T043 T045 |
| SC-006 | T015 T017 T031 T033 T034 T035 T042 T044 T045 |
| SC-007 | T001 T002 T006 T019 T020 T021 T029 T038 T039 T044 T045 T047 T048 T049 |

| 原spec的验收场景 | 对应任务 |
|---|---|
| US1.AC1 | T012 T013 T015 T016 T017 T020 |
| US1.AC2 | T013 T020 T021 |
| US1.AC3 | T010 T012 T013 T018 T020 |
| US1.AC4 | T006 T011 T014 T019 T024 T043 |
| US2.AC1 | T022 T023 T025 T026 T029 |
| US2.AC2 | T003 T009 T023 T025 T026 T029 |
| US2.AC3 | T007 T022 T024 T026 T029 |
| US2.AC4 | T007 T011 T016 T022 T026 T028 T029 |
| US2.AC5 | T015 T020 T029 |
| US3.AC1 | T011 T016 T018 T026 T028 T032 T038 |
| US3.AC2 | T030 T032 T038 T039 T044 |
| US3.AC3 | T030 T031 T033 T034 T035 T037 T038 |
| US3.AC4 | T030 T036 T037 T038 T039 T041 T043 |
| US3.AC5 | T015 T018 T032 T036 T038 |
| US4.AC1 | T015 T018 T031 T034 T040 T042 T045 |
| US4.AC2 | T017 T033 T035 T038 T044 |
| US4.AC3 | T008 T011 T018 T040 T041 T042 T043 T045 |

## 当前工作流记录

2026-10-05：主代理授权仅tasks→readonly analyze；先逐SHA与根核对九设计，spec/checklist不变。plan更新为44eadc18…1c95e、quickstart为df1b5c8f…928ec5，来自根已批准交付循环修正：010自身门→提交，011共享兼容/014审批/020清理各自后续联验；不是本轮改需求。本WT selector仍010，preset list无安装preset，resolve-template/core及setup-tasks实际执行成功；extensions hooks={}，before/after tasks/analyze无可执行项。分析消息交接，不写额外报告，不source/工具安装/凭据/VM/发布/提交。

## 2026-10-05 用户验收安排与实施归属（覆盖原阶段门措辞）

用户明确要求先完成全部模块代码及必要自动验证，真实 Apple/Play 上传在最后统一案例由用户人工验收；没有凭据不阻塞代码交付/本地提交。原真实远端需求与 SC/AC 不删除、不宣 PASS，验证记录标人工待验；无凭据自动门仍须实际锁版本、真实本机故障端点单发、未知保护与安全失败。008/019/020已验收基线2602094；005/009按其实际模块接口接入而不虚构真实素材通过。

唯一 writer：publish-channels 分区独占 internal/distribute/**（两店具体 Go/Ruby、Fastfile、Gemfile/lock、测试）及新 internal/protocol/publish.go；root独占既有protocol/node.go、Store、Agent、Pipeline、config、server、CLI共享消费者与全局文档。原表/任务中分散在B/root的 distribute 与公共发布新类型任务统一归publish-channels；其他任务仍root协调唯一writer。一个Run/原process.Run、完整Ref与guard/unknown不变，无registry/第二执行器。

## Phase 8: Convergence

- [x] T051 HIGH 在 `internal/agent/publish_query.go`及独立行为测试确认管理query在实际process前创建原有限journal；具体ErrCleanup保journal并闭锁重启session，只有已知Close后删除。原claim/nonce/30s不改，不猜PID，不以compile代故障验证。来源FR004/014/015/018、Constitution IV（partial）。
- [x] T052 HIGH 在 `internal/server/publish.go`和原Agent HTTP消费者把发布请求/响应真实限制64KiB，保旧普通1MiB协议；严格JSON/角色/原期限行为测试验证真实超限拒绝。来源plan: HTTP有界输入、FR022（partial）。
- [x] T053 MEDIUM 在010验证记录、quickstart与README登记最终源码检查、真实工具锁、已实现命令和人工待验；交付统一Flutter双平台实际案例，真实SC001/004不伪标PASS。来源FR026/SC007、用户2026-10-05验收安排（partial）。

## 最终实施记录（用户调整后的代码交付门）

所有勾选表示代码、必要自动检查或人工验收准备已完成，不表示真实商店 SC/AC 通过。任务列举的概念已沿共同 publish 文件实现，实际文件/测试映射见 validation.md；不为计划中占位文件名创建第二实现。最终全MVP检查集中执行，不逐模块重复全量；真实签名、商店与外部服务在集中案例由用户执行。
