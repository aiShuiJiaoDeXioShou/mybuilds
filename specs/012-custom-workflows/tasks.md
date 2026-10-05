# 任务：可复用构建方案与用户自定义发布

**输入**：本目录 spec、plan、research、data-model、quickstart 和三份 contracts；冻结30 FR、8 SC、4个P1故事、18 AC。按 core tasks-template组织。当前执行analyze→implement；不自动安装工具、访问未知凭据或发布。

**实施前置**：003/004/007/008/019/020已验收源码为基线；005/009/010/011按真实具体组件接口冻结并集成，一次授权/unknown保护与报告封存不削弱，不复制dirty源码、不造stub。缺Apple/商店材料不阻可执行组件实现及必要自动验证，真实签名与上传由用户最终人工验收。012自身全部门完成后一次功能提交；014审批和020保留的后续联合验收不反向阻塞012首提交，生效而未实现审批/通知仍明确拒绝。

**真实红绿**：权限、输入、来源、事务、恢复、副作用与进程取消先实际红测再最小实现转绿。双库同套，真实固定Git、真实三入口及四平台工程、自有custom接收端；解析通过、预置文件、交叉编译或模拟命令不能代替完整签名/执行。custom用户脚本受管理员信任，系统不承诺识别其内部网络重试或证明任意query脚本只读。

## 格式与唯一writer

`- [ ] T### [P?] [US#?] 描述及路径`；`[P]`仅前置就绪后文件不相交。真正进程/DB/副作用测试另约自有独占资源窗口。

| writer | 独占文件及接线 |
|---|---|
| A 配置/方案 | `internal/config/profiles.go`及tests、`types.go`/`project.go`/`server.go`的本功能字段与严格加载，`internal/cli/client/init.go`及本地模板tests；原`parse.go`所需最小共享改动由root串行 |
| B custom节点 | `internal/distribute/custom.go`/`custom_test.go`及相应query测试，`internal/agent/publish_custom.go`及tests |
| C 来源/管理 | `internal/scm/git.go`及对应FileMode tests，`internal/server/pipeline_source.go`及tests、`trigger.go`/`project.go`；client/server两端project管理文件及新tests，不改A的init/config文件 |
| root 共享 | Store models/enqueue/retry/recovery/query/publish及相关共享tests；protocol所有新optional variant；pipeline RunTypes/Run/Preview；Agent共同publish/query/journal/serve/http；server.New实际profiles加载接线；README、依赖、本功能tasks/validation及集成 |

T003在真实前置基线上锁定实际路径/签名，禁止same-file并写。源码worktree另从已验收集成基线建立；A/B/C冻结SHA交root顺序复制并实消费者复验，不提前写空方法或借其他dirty分区。config不反向import mobile；server.New直接组合四已验收mobile模板bytes交具体LoadBuildProfiles，无profile表、热重载、source/publisher registry或第二Run。

## Phase 1：准备

- [x] T001 由root在`specs/012-custom-workflows/validation.md`记录九设计SHA、selector/preset/core-template/hooks、30FR/8SC/18AC、前置及合法材料状态；明确此次只有文档，不声明平台/custom已通过。
- [x] T002 在`specs/012-custom-workflows/validation.md`核验全部实际前置提交及原模板/恢复/两店共同publisher/报告API，缺005或010/011具体代码接口则隔离该消费者，独立组件继续；缺真实材料记人工门pending；建立fresh源码WT，不改变014/020后续联验顺序、不按任务提交。
- [x] T003 在`specs/012-custom-workflows/contracts/go-api.md`、`plan.md`由root冻结实际BuildProfile/Origin/custom variant/私有CustomQueryContext与唯一归属，核旧digest/发布slot/ReportSeal/一次env渲染实际消费者可见性；不复制整Snapshot/Step进query，不导出通用执行/模板接口。

## Phase 2：共享基础（阻塞故事源码）

- [x] T004 在`internal/store/origin_test.go`先做双库旧snapshot/Origin/digest红测，再由root接`models.go`/安全query DTO：Origin可空旧记录保持未知；新增Origin严格Mode/Kind/SHA/file/profile/template/hash关系，DefinitionDigest不含运行Facts/参数覆盖；旧无Custom/Origin网络omitempty不改digest，不增加profile表或猜补来源。
- [x] T005 在`internal/config/profiles_test.go`先红后在`profiles.go`及A原`types.go`/`server.go`/`project.go`接具体字段：safeName≤64、build_profiles template/file二选一、禁止覆盖四builtin、旧profile/params normalize default与builds互斥、项目≤64build、source仅auto/repo/profile；unknown/duplicate/null/type/alias/merge及既有限额不降。
- [x] T006 在`internal/protocol/custom_test.go`先真实JSON红测后由root接optional Custom授权/回执/绑定/query类型：仅target custom、action custom_upload、schema1、arrays[]、私有context不进safe DTO，旧receipt digest兼容；伪造context/错kind/跨target/超64KiB/坏UTF8/重复/null拒，source仍由真实Store派生，不给新grant。
- [x] T007 在`internal/config/custom_test.go`先红后由A与root串行原parse/validate最小接custom upload静态约束：argv/query_argv≤128项、项≤4096B/总≤64KiB，原文不插值；唯一file/app/result、完整可选secret引用、安全一次working_dir/result_file、只既有upload；生效尚未实现approval/notification及local有效upload仍先拒。

**Checkpoint**：T001–T007通过才故事源码；010/011真实代码安全能力不能以本功能自有接收端替代；真实商店与签名门单列人工待验。

## Phase 3：US1 — 无仓库配置的明确单/双平台绑定（P1）

**目标**：四builtin和明确项目framework/platform绑定android/ios，不写仓库或猜材料；旧本地行为保持。

**独立测试**：真实无YAML工程四方案及至少一组双平台，在合法节点执行真实包/版本/签名与中央下载；非法输入无项目/号。

### 先写红测

- [x] T008 [P] [US1] 在`internal/config/profiles_test.go`写真实文件/模板加载红测：单方案≤1MiB、深度≤32/节点≤10000、≤64自定义/总≤16MiB，root单build且任何builds键拒；所有显式方案启动时加载，一坏全拒；FIFO/symlink/hardlink/替换/超大有限拒不执行，四builtin/alias深复制不共享可变map。
- [x] T009 [P] [US1] 在`internal/cli/client/project_profiles_test.go`及server本机project tests写真实CLI/HTTP红测：framework须platform、仅platform默认native、android/ios唯一规范序、settings/file/flags冲突，admin才注册/绑定、bad输入无部分项目；旧default/custom/local行为不加载server方案。

### 实现与故事验证

- [x] T010 [US1] 在`internal/config/profiles.go`实现ParseBuildProfile/LoadBuildProfiles，复用现有严格YAML树/Validate/bounded普通文件读，不多引擎；四内置提取对应唯一Build、别名与内容digest、已加载bytes/definition深复制，canonical路径只私有、相对server.yml/~处理，不远端下载/热重载，T008转绿。
- [x] T011 [US1] 在`internal/server/server.go`由root接唯一LoadBuildProfiles真实消费者，直接已验收四mobile模板函数map、不调用doctor/资源准备，启动加载一项坏则失败；C在`project.go`和client/server本机project文件接单/双framework/platform typed Settings，不改repo/计数器、不写仓库，T009转绿。
- [x] T012 [US1] 在`internal/cli/client/init.go`及`init_test.go`复用config受限文件读取/既有Parse合法single或named模板，不把方案的单build限制施加给本地template；排他拒覆盖、保名字和default/native/Flutter旧行为，本地run缺文件不连控制端取profile，dry-run零工具/secret/执行。
- [x] T013 集中验收准备与证据登记（原完整实际场景由用户/最终统一案例执行，不宣全部通过）：[US1] 在`specs/012-custom-workflows/validation.md`记录四真实single平台工程与native/Flutter双平台绑定，等US2真实来源resolver就绪后运行；同批SHA、不同buildID/Number/节点/ref/工作区/日志/原artifact，失败不取消另一项、同名串行，真实版本/签名/hash/归档符号独立核，不unsigned代签名。
- [x] T014 集中验收准备与证据登记（原完整实际场景由用户/最终统一案例执行，不宣全部通过）：[US1] 在`specs/012-custom-workflows/validation.md`用实际两端CLI+HTTP验证选择/重复/未知/空平台、缺配套参数、设置冲突、无选择双build、原default/custom/template与坏client隔离；仓库与工程文件字节不变，无SDK/材料自动创建或scheme迁移。
- [x] T015 [US1] 在`internal/server/project_profiles_test.go`与`validation.md`实际验证set只替换显式顶层块、未给保留、null拒，组迁移不继承/改绑定权限或编号，repo/default与profile/android/ios切换不自动映射旧选择；更新不改已queued任务，维持既有通知unsupported。

**Checkpoint**：US1四AC需US2来源接线后才完整；模板/注册可先分区实现，但不提前假无YAML执行成功。

## Phase 4：US2 — 仓库与方案来源二选一（P1）

**目标**：一个ReadPipeline固定SHA，auto只真正缺失回退，repo强制存在、profile跳过文件，任何错误无fallback。

**独立测试**：真实Git固定提交完整存在/缺失/目录/叶或父symlink/gitlink/坏YAML/失败三模式矩阵，比较实际来源与完整集合，错误零入队零号。

### 先写红测

- [x] T016 [US2] 在`internal/scm/file_mode_test.go`写真实Git红测：FileMode空/required/optional/none，同次固定SHA含SHA1/SHA256仓库；optional仅精确路径确实无记录Missing=true、空Content/Digest，none真实SHA且Missing=false；父/叶symlink/目录/gitlink/超限/Git权限/branch/ref/timeout失败不Missing，不运行repo hooks/过滤器。
- [x] T017 [US2] 在`internal/server/pipeline_source_test.go`写真实Store+Git+文件profiles来源红测：auto存在只repo完整集合、缺失绑定只profile、无绑定报错；repo缺失报错，profile无论配置坏/存在都不读但必须真repo/SHA；坏YAML或文件错误无回退/步骤合并，旧根default不映射。

### 实现与故事验证

- [x] T018 [US2] 在`internal/scm/git.go`最小扩ReadPipeline FileMode/Missing，仍现有bare fetch/安全URL/env/限输出process、精确commit普通文件逐父检查；不查第二HEAD、不写用户worktree，不用空bytes hash伪存在，T016转绿。
- [x] T019 [US2] 在`internal/server/pipeline_source.go`实现唯一resolvePipeline，消费已加载profiles与同ReadPipeline/config.Parse，完整来源二选一、深复制bound name、不registry；计算repo rawblobhash及profile全展开name→Origin字典序canonical摘要，未选集合可入源hash但不造build记录，T017转绿。
- [x] T020 [US2] 在`internal/server/trigger.go`把实际resolver接既有Select/ResolveParams/Preview/buildCondition/唯一Enqueue；FindRequest按原key先读已持久batch、不读新profile，事务外Git/文件，入队复核Project.PolicyVersion/CAS；root Store只验证所选Origin与hash，不从selected子集猜重建全源摘要。
- [x] T021 [US2] 在`specs/012-custom-workflows/validation.md`以SQLite/PG同三入口跑完整来源矩阵、两个来源刻意不同名称/步骤、unsafe profile启动拒、exact Git nonregular/坏blob/权限/timeout；核mode/实际kind/file/profile/template/digest正确、错误零回退/部分入队/号，profile也固定原SHA。

**Checkpoint**：US2五AC通过后才可完成US1的无YAML真工程门；scheme文件/源码错误不能被auto掩盖。

## Phase 5：US3 — 参数优先级、快照和原事实重试（P1）

**目标**：named>shared>project named>definition default，先全批参数/权限后when；Origin/Definition/Params/Facts/SHA固定、retry新号不重展开。

**独立测试**：不同项目/名称20同key原子触发、编辑profile/settings/HEAD后真实执行和retry仍原定义/条件/digest，角色/非法参数不占号。

### 先写红测

- [x] T022 [P] [US3] 在`internal/server/profile_params_test.go`写实际来源参数红测：四层优先级、旧profile/params default互斥builds、共享必须所有所选声明，unknown/required/choices/重复/未选择scope在when前整批拒；pending编号/节点/workspace模板不作build.when失败，admin+allowupload先于when。
- [x] T023 [P] [US3] 在`internal/store/origin_retry_test.go`写双库原子Origin/retry红测：20同key仅一批一号、skipped无号；ProjectPolicy变化/SQL失败/锁失效回滚；queued原完整定义/hash、旧nil未知、原Snapshot静态条件和SourceDigest deep-copy，retry新号新reports/publish证据、授权原×当前不扩。

### 实现与故事验证

- [x] T024 [US3] 在`internal/server/trigger.go`复用原参数解析和Preview组合项目默认/触发覆盖，不隐式导出env，不把普通参数值公开；所有选中节点范围/上传权限/模板参数先查再when，方案不得扩大项目授权/通知，T022转绿。
- [x] T025 [US3] 在`internal/store/enqueue.go`/`recovery.go`/`retry.go`由root接严格Origin验证与原快照深复制、原SourceDigest保持，DefinitionDigest用规范完整定义、不含运行事实/参数覆盖；旧nil不猜，批次SHA/Mode/Kind/所选定义一致、同事务原PolicyVersion与身份复核，T023转绿，无文件/Git进tx。
- [x] T026 [US3] 在`internal/store/query.go`/`internal/server/project.go`及安全build/project视图只露origin相对file/方案名/模板名/摘要/SHA和parameter_keys；不露私有profile File/内容/steps/参数值/秘密，不把旧nil断言repo；真实身份读负例沿已有HTTP，typed Settings不发客户端路径。
- [x] T027 集中验收准备与证据登记（原完整实际场景由用户/最终统一案例执行，不宣全部通过）：[US3] 在`specs/012-custom-workflows/validation.md`双库20同key/丢触发响应原key显式恢复、各参数脚本实际值与非法/falsewhen门，不同项目/绑定名无共享map修改；role/node范围/upload无权限不占号，SQL/失锁/CAS无部分入队。
- [x] T028 集中验收准备与证据登记（原完整实际场景由用户/最终统一案例执行，不宣全部通过）：[US3] 在`specs/012-custom-workflows/validation.md`先入队后改profile文件/builtin部署/settings/branch HEAD并重启控制端，实际原任务/明确retry仍原SHA/定义/参数/条件/Origin/hash，新号新证据，旧详情/日志/包XML字节保持；无重新来源读取、自动Run或扩大权限，四builtin摘要可追溯。

**Checkpoint**：US3四AC完整通过；来源复用不赋予新节点/应用/上传许可，来源编辑也不改变原幂等结果。

## Phase 6：US4 — custom一次动作、unknown与显式核对（P1）

**目标**：同一受控upload/Publish闭包，可信argv一次process、稳定原artifact/封存报告/应用guard；结果足够才确认，否则unknown不重发。

**独立测试**：自有接收端真实发送计数、授权/ACK断连/重启/取消、strict新result文件、当前节点只读query与有依据admin确认；两库及两节点，不操作用户商店。

### 先写红测

- [x] T029 [P] [US4] 在`internal/distribute/custom_test.go`写真实文件/process红测：Prepare无副作用，唯一原artifact稳定copy/hash，旧result/FIFO/symlink/multilink/父替换/超限/坏UTF8/重复/null/未知schema拒；input/result JSON≤64KiB、深度≤16/值≤4096，原stdout/stderr合计≤64KiB私有、秘密命中拒而非改写。
- [x] T030 [P] [US4] 在`internal/agent/publish_custom_test.go`与root Store custom测试写真实HTTP/双库授权红测：admin manual_attested 绑定审计非remote GET认证、Google/Apple不能借manual，命令摘要/原Ref/原AAB或IPA验证/任意文件原bytes/JUnit seal/ordinaryNS/guard；grant前IntentID fsync、grant可能已提交后unknown、once保存失败无command、20同app≤1grant。
- [x] T031 [US4] 在`internal/agent/custom_query_test.go`先做真实Store管理task红测：Store派生原Repository/SHA/QueryArgv/声明Env/Params/必要Facts/原Ref/AuthDigest/artifact元数据/seal，当前原node/session/nonce/30s；伪造ctx/旧会话/越限/错secretref拒，不依赖旧journal、不读旧artifact、不借旧lease或Run普通/post。

### 实现与故事验证

- [x] T032 [US4] 在`internal/distribute/custom.go`实现PrepareCustom及具体新result strict读取：受限workspace相对目录、结果必须原先不存在、实际输入0600和原artifact自有copy/identity、MYBUILDS_PUBLISH_INPUT/RESULT及仅显式CREDENTIAL env；argv/query正文不插值，声明env一次解析、不继承整宿主/admin/node token，T029安全门转绿。
- [x] T033 [US4] 在`internal/distribute/custom.go`实现一次UploadCustom真实process.Run/OnStart，当前普通NS×Authority；成功须exit0+精确intent/AuthDigest/app/artifact/version+action_confirmed/remote证据，未发送/明确远端拒绝须真实结构依据，普通退出或无result不推failed；原reason优先，Close独立15s只SameFile自产，替换/未知保CleanupFailed，不删除未知证据。
- [x] T034 [US4] 在`internal/agent/publish_custom.go`与root共同`publish.go`/`journal.go`/pipeline Publish/Store publish接同一once闭包、manual binding和custom variant验证；上传slot关闭后原候选lookup生成全部终态intent集合，receipt同摘要幂等、旧fence/另一cmd digest拒；unknown不随StopKnown/expiry/retry/终态清guard，T030转绿，不第二调度或新通用publisher。
- [x] T035 [US4] 在root `internal/store/publish_query.go`/protocol和`internal/agent/publish_custom.go`实接私有CustomQueryContext：原Store派生而非用户自由输入，Facts只project/build.name/git.branch/step.name原冻结，sha/id/number原context；node.name当前真实身份、workspace本次query目录不冒称旧事实，秘密仅当前明确引用；task≤64KiB过限拒不截断、context不进safeDTO，T031转绿。
- [x] T036 [US4] 在`internal/distribute/custom.go`与root共同Agent管理consumer接QueryCustom：固定原Repository/SHA安全Checkout，自有isolated工作区、原QueryArgv一次process、原nonce/Expiry∩30s，无query明确unavailable不command；query input artifact只有id/size/hash无path、不下载旧artifact/PrepareUpload/Run/新grant，输出同精确schema、不足保unknown，只读是用户信任声明。
- [x] T037 [US4] 在root `internal/store/publish.go`/`publish_query.go`及C server/client既有绑定/query/confirm入口接Custom审计/variant/safeview，verification-file0600普通≤64KiB strictJSON、manual来源区别doctor；admin原digest/充分依据/Note限额与秘密拒、重复幂等冲突拒，stop/空query/exit不能clearguard，不新增execute-confirm API。
- [x] T038 集中验收准备与证据登记（原完整实际场景由用户/最终统一案例执行，不宣全部通过）：[US4] 在`specs/012-custom-workflows/validation.md`运行真正自有脚本/接收端一次发送，package.bin及实际AAB/IPA复用既有核验、声明XML/seal原证据，特殊参数input/env单值/argv原式、有效回执与中央关联/hash；同app并发无绕锁、非admin/未allow/坏产物凭据/report/失权/本地upload副作用前拒。
- [x] T039 集中验收准备与证据登记（原完整实际场景由用户/最终统一案例执行，不宣全部通过）：[US4] 在`specs/012-custom-workflows/validation.md`实际grant响应丢失/发送接受后断ACK/启动前失联/缺或无效result/冲突receipt，控制端/Agent重启或retry不重发同意图、appguard持有且原意图完整manifest；节点停止只解物理保护，原query/有依据manual精确核对才变结果，无query/不足仍unknown。
- [x] T040 集中验收准备与证据登记（原完整实际场景由用户/最终统一案例执行，不宣全部通过）：[US4] 在`specs/012-custom-workflows/validation.md`两个真实节点实际ordinary/post取消/累计timeout/lease/revoke/log或journal写fail/Close路径替换，核本次前后台PID+birth已停与无关sleep活、原Reason/post/普通ns不重置、失权不开always、系统清理独立；raw输出/输入/privatepath/params/密钥标记不公开，完整原诊断证据保持。
- [x] T041 集中验收准备与证据登记（原完整实际场景由用户/最终统一案例执行，不宣全部通过）：[US4] 在`specs/012-custom-workflows/validation.md`双库重复T038–T040及当前节点query/旧session/nonce超时/原repoSHA/metadata-only/secretref/无artifact读取门；自有接收端计数和新result/readback真实，不模拟普通步骤完成，014尚未验收生效approval明确unsupported、不伪造批准，020保护联合门归其后续接入。

**Checkpoint**：US4五AC、真实单次自有发送与unknown/安全门全部通过。只能保证系统授一次command，不宣称任意脚本内部网络恰好一次或query绝无副作用。

## Phase 7：正式联验、文档与一次交付

- [x] T042 由root在`README.md`、`docs/plans/MVP_EXECUTION.md`和本功能`quickstart.md`更新三来源/四builtin/命名参数/本地模板/绑定与custom输入结果/unknown/query/manual真实能力，source名称profile保持、不新加bindings source；014/015/020待正式接入联验，无插件/热重载/工程迁移或内部脚本网络保证。
- [x] T043 执行本模块必要普通/race/vet、全包编译和三实际入口help/version；最终全MVP统一执行一次全量及跨平台检查，原四平台合法签名和商店材料仍人工待验，不按功能重复长矩阵。
- [x] T044 集中验收准备与证据登记（原完整实际场景由用户/最终统一案例执行，不宣全部通过）：在`specs/012-custom-workflows/validation.md`按下表30FR/8SC/18AC核UTC/实际工具版本/前置源二进制SHA/四builtin摘要/固定提交/原artifact/XML/intentreceipt/自有接收计数及命令脱敏证据，quickstart完整真实门不缺，014/020后续联合不形成反向前置。
- [x] T045 对`specs/012-custom-workflows/spec.md`/`plan.md`/`tasks.md`/最终代码与`validation.md`执行只读 `$speckit-converge`，真实缺口继续implement/必要复验/收敛；自身或前置未验收保持pending，不以配置解析代真实平台/custom。
- [x] T046 按项目提交技能检查本功能差异/暂存，在`specs/012-custom-workflows/validation.md`核完整必要门后一次本地012提交，不按任务/批次提交、不push、不夹其他未验收功能源；014/020后续联验由其功能记录。

## 依赖与增量/并行策略

1. T001→T002→T003→基础T004–T007；实际协议/配置/Origin冻结后才故事源码。A config同文件全串行，root共享原parse/Store/protocol/Agent接线只在真实批次交接后进行。
2. US1 T008/T009跨config/CLI文件可并行红，T010→T011，T012复用真实安全读取；T013/T014/T015要T018–T020真来源resolver及T024/T025入队事实，不提前造执行stub。US1为首最小业务增量但实际无YAML构建需US2及共享安全基础。
3. US2 T016→T018，T017→T019→T020→T021；T019等T010真实LoadedProfiles。C SCM/Server源码同owner分时写，不双writer；repo/profile错误矩阵可用自有无移动SDK通用脚本，但四平台成功门仍必须真工程。
4. US3 T022/T023跨server/store可并行红，T024→T025→T026→T027/T028；消费T020真实来源，不读新profile retry。T015与T028涉及源编辑/restart，用各自私有夹具，不修改root/用户服务。
5. US4 T029/T030跨distribute/agent+Store可并行红，T031先红；T032/T033→T034，T035等根真正Store query/protocol、T036等实际context/安全Checkout，T037接原真实确认；T038–T041等全部真consumer，故障进程域独占。
6. T042–T046依赖四故事与所有本功能正式门。014有效审批未交付仍先拒，014/020真正联验在其功能完成，不反向堵012提交；010/011真实接口与安全前置不可解除，签名和商店材料门独立记人工待验。

**并行例子**：US1 A模板/文件红T008、C项目flags红T009；US2 C的SCM与来源红不同文件可按明确移交分时并行，但共用git.go不并写；US3 C参数T022、root事务T023；US4 B文件/processT029、root真实授权Store测试T030可并行写，实际进程与DB故障分别独占。根共用schema/Store/pipeline/Agent序列始终串行，只有真实交接后消费API，不造仓库/发布interface。

## FR/SC/AC 覆盖

| FR | 对应任务 |
|---|---|
| FR-001 | T005 T008 T010 T011 T013 |
| FR-002 | T005 T008 T010 T021 |
| FR-003 | T009 T011 T014 |
| FR-004 | T016 T017 T018 T019 T020 T021 |
| FR-005 | T008 T016 T017 T018 T021 |
| FR-006 | T005 T015 T017 T019 T021 |
| FR-007 | T009 T013 T020 T023 T025 T027 |
| FR-008 | T005 T009 T011 T015 T020 T023 |
| FR-009 | T005 T022 T024 T027 |
| FR-010 | T007 T022 T024 T027 |
| FR-011 | T007 T022 T023 T024 T027 T030 T034 |
| FR-012 | T004 T019 T020 T023 T025 T028 |
| FR-013 | T002 T010 T011 T013 T043 |
| FR-014 | T007 T009 T012 T014 T038 T043 |
| FR-015 | T004 T009 T012 T021 T024 T026 T027 T040 |
| FR-016 | T007 T029 T032 T033 T034 |
| FR-017 | T029 T030 T032 T034 T038 |
| FR-018 | T006 T030 T034 T037 T038 |
| FR-019 | T007 T029 T030 T032 T033 T034 T040 |
| FR-020 | T006 T029 T033 T034 T038 |
| FR-021 | T030 T033 T034 T039 T041 |
| FR-022 | T006 T030 T034 T039 T040 T041 |
| FR-023 | T003 T006 T031 T035 T036 T037 T039 T041 |
| FR-024 | T030 T034 T037 T039 T041 |
| FR-025 | T006 T029 T032 T033 T035 T037 T040 |
| FR-026 | T029 T033 T034 T040 T041 |
| FR-027 | T007 T022 T027 T041 T042 |
| FR-028 | T005 T009 T022 T024 T027 T030 T037 T041 |
| FR-029 | T001 T004 T016 T017 T021 T023 T027 T028 T030 T041 T043 T044 T045 |
| FR-030 | T002 T013 T014 T028 T038 T039 T040 T041 T043 T044 |

| SC | 对应任务 |
|---|---|
| SC-001 | T010 T011 T013 T014 T043 |
| SC-002 | T008 T016 T017 T018 T019 T020 T021 |
| SC-003 | T005 T009 T022 T023 T024 T025 T026 T027 |
| SC-004 | T004 T019 T023 T025 T028 |
| SC-005 | T029 T030 T033 T034 T038 T039 T041 |
| SC-006 | T006 T007 T029 T030 T031 T033 T034 T035 T036 T037 T041 |
| SC-007 | T013 T021 T023 T027 T028 T038 T039 T040 T041 T043 T044 T045 |
| SC-008 | T007 T009 T012 T014 T015 T021 T026 T040 T042 T043 |

| 原spec验收场景 | 对应任务 |
|---|---|
| US1.AC1 | T005 T008 T010 T011 T013 |
| US1.AC2 | T013 T020 T023 T025 T027 |
| US1.AC3 | T005 T009 T011 T014 T022 T027 |
| US1.AC4 | T009 T012 T014 T043 |
| US2.AC1 | T017 T019 T020 T021 |
| US2.AC2 | T016 T017 T018 T019 T021 |
| US2.AC3 | T008 T016 T017 T018 T021 |
| US2.AC4 | T016 T017 T018 T019 T021 |
| US2.AC5 | T005 T008 T010 T021 |
| US3.AC1 | T005 T022 T024 T027 |
| US3.AC2 | T010 T013 T022 T023 T025 T027 |
| US3.AC3 | T004 T019 T023 T025 T028 |
| US3.AC4 | T007 T022 T023 T024 T027 T030 T034 |
| US4.AC1 | T029 T030 T032 T033 T034 T037 T038 |
| US4.AC2 | T029 T030 T033 T034 T039 T041 |
| US4.AC3 | T006 T030 T033 T034 T038 T041 |
| US4.AC4 | T031 T035 T036 T037 T039 T041 |
| US4.AC5 | T007 T029 T030 T033 T034 T040 T041 |

## 本轮工作流记录

2026-10-05：主代理授权仅tasks→readonly analyze。开始时九设计与root逐SHA相同，spec/checklist冻结不改。根批准最窄CustomQueryContext当前Store→Node私有管理消费者补齐与query输入metadata-only（不下载/不旧journal/不授新权），修custom示例file为真实collect的dist/package.bin；无需求改变。实际selector012，preset list无安装preset、resolve core tasks-template及setup-tasks成功；hooks={}前后无可执行项。分析通过消息交接、不创建额外报告；本轮不手工Git、不源码/工具安装/VM/发布/提交。

## 最终代码交付收敛

按用户集中验收安排，46项勾选对应实际实现、必要自动检查或完整人工场景准备；不把合法签名/外部商店或未执行故障组合宣PASS。30FR/8SC/18AC/5原则已核当前代码，剩余联合验证归014/015及全MVP最后检查，无新增代码任务。
