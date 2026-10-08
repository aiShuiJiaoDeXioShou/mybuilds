# Tasks：019 本次执行的测试报告与发布证据

**输入**：[spec.md](spec.md)、[plan.md](plan.md)、[research.md](research.md)、[data-model.md](data-model.md)、[contracts](contracts/)、[quickstart.md](quickstart.md)。
**范围**：原18FR/6SC/4个P1故事/12AC，数量增量增加4FR/1SC/US5；沿同一Run与既有中央文件路径，无新增依赖。测试先行、实际红绿；新增边界测试如已通过，只记录现有真实行为，不人为造红或重写正确实现。
**状态**：基线008 504dc6已验收，正式analyze零阻塞后正在implement；已完成标记对应validation真实证据，完整双节点/全量与收敛尚待完成。任务与019原18FR/6SC/12AC保持。

## 文件归属与交接

- root唯一写共享protocol/node.go、pipeline/run_types.go及run.go/remote.go/artifact.go调用接入；Agent journal.go/execute.go/artifact.go共用接线；Store models.go/node_models.go/event.go/enqueue.go/query.go/artifact.go/retry.go/recovery.go共用接线；Server files.go/artifact.go/http.go/json.go及trigger.go跨消费者接线；规范/README/历史/validation/tasks/整合提交。共享文件先收具体最小补丁，串行写入并按SHA同步，不并发writer。
- A唯一写internal/reports/junit.go、junit_test.go、internal/store/reports.go、reports_test.go及本功能独立Store测试文件；parser只给pipeline/server两个实际消费者，Store不import执行器。
- B唯一写internal/pipeline/reports.go、reports_test.go及本功能独立pipeline测试文件，internal/agent/reports.go、reports_test.go及独立Agent报告测试；仍一次Run，不改process执行器。
- C唯一写internal/config/validate.go与报告独立tests、internal/pipeline/preview.go及报告独立preview测试、internal/server/reports.go及report_test.go独立解析消费者、internal/cli/client现有build/artifact展示和独立report_test.go。Root串行dispatch接C函数，不与C写同文件。

A/B/C是能力分区，不锁代理名字。最多root+A+B+C四个slot。下文[P]仅在已完成列出前置后、不同writer/文件可并行；同包已有共享文件仍root唯一写。

## Phase 1：Setup

- [x] T001 根核对README.md、docs/plans/CONFIGURATION.md、MVP_EXECUTION.md、SPECKIT_ROADMAP.md及007真实提交；008提交后在独立019工作区FF/rebase并记录基线/依赖/唯一writer于 specs/019-test-reports/validation.md，不改变后续005/商店/审批验收门。
- [x] T002 根实际运行 prerequisites、核对checklists/requirements.md、.gitignore及.specify/extensions.yml hooks；记录七设计+spec快照SHA和本功能同Run/无新依赖约束于 specs/019-test-reports/validation.md。

## Phase 2：Foundational（阻塞故事源码）

- [x] T003 根先在 internal/protocol/report_test.go 写具体消息红测：snake_case、Reports/ReportManifest/Purpose等omitempty不改变无报告旧digest，LocalReports不序列化，Files/Diagnostics/Manifest.IDs显式[]不得null；完整Ref/Seq/UTC沿原契约。
- [x] T004 根在 internal/protocol/node.go、internal/pipeline/run_types.go 增加冻结contracts/go-api.md实际报告类型和有限checked/sealed事件、私有CollectedReport、安全BuildRun字段；旧四步骤不变，不预建callback/interface或独立cursor。（T003）
- [x] T005 根在 internal/store/report_migration_test.go 先写旧007/已验收008 SQLite/PG真实迁移红测，在 models.go、node_models.go沿Migrate加入ReportRevision/Final/ReportsJSON/SealDigest/CheckedIndex及artifact Purpose/Revision/Key/VerifiedJUnitJSON，ArtifactCommit增加仅server内部的VerifiedJUnit *protocol.JUnitResult（不接受HTTP JSON）；旧记录均零/空，不回填假seal，重复迁移/旧FK及旧receipt.Kind/StopKnown门保留。（T004）
- [x] T006 C先在 internal/config/reports_test.go、internal/pipeline/reports_preview_test.go 写严格配置/预览红测，再接 validate.go、preview.go：最多32模式，required省略true，所有选中build及--step先校验、路径一次模板，渲染后拒绝绝对/盘符/反斜杠/控制字符/任意..段；合法待确定系统fact只pending，不伪装build.when false，不读报告或秘密。（T004）
- [x] T007 根复验协议与迁移两库门，逐SHA同步A/B/C实际类型和现有有限文件/Authority/预算入口，在 specs/019-test-reports/validation.md 记录基础门；未有真实consumer前不提前放开Trigger/Enqueue/Run的unsupported。

**Checkpoint**：具体模型/兼容及报告配置可消费，旧无报告编码/行为不变。没有空Store/parser/Run函数充当完成。

## Phase 3：US1 读取本次执行的测试结果（P1）

**目标**：本地真实Run读取并正确汇总稳定原XML，同路径替换，命令失败保原Reason。
**可独立验证增量**：真实本地脚本覆盖US1.AC1–3；节点端同一能力在US4实际事件/文件consumer完成后复验，不声称远程已独立交付。

- [x] T008 [US1] A先在 internal/reports/junit_test.go 写纯parser红测：testsuite/testsuites/嵌套case计数、failure/error/skipped互斥、矛盾属性、非负十进制time及ns精度、零case、重复属性/非法结构、DOCTYPE/entity、ctx/限额与secret；suite汇总不累加case，标准库Token语法不能代替JUnit语义。（FR002/008/010；AC1.1）
- [x] T009 [US1] A在 internal/reports/junit.go 实现实际 ParseJUnit(ctx,io.Reader,secrets) 与固定sentinel：UTF-8无namespace有限结构、每token检查ctx、不装Entity/CharsetReader；单XML8MiB、depth64、每元素64属性、属性名256B/值4096B、case名称512B、100000cases、case24h/汇总365天；Diagnostic最多20条/总20KiB、Message1024B，控制符处理/脱敏后UTF-8安全截断，raw及解码属性/文本secret都拒原XML。（T008）
- [x] T010 [P] [US1] B先在 internal/pipeline/reports_test.go 写具体集合/快照红测：真实脚本生成通过/失败/错误/跳过，嵌套suite和同路径改写替换、稳定ID、源修改不改原快照、非零命令仍有新XML；使用真实文件/进程，不fake executor。（基础门T007；FR003/004/009；AC1.1–3）
- [x] T011 [US1] B在 internal/pipeline/reports.go 实现有限reportCollection：实际build首次ordinary动作前有界identity/mtime/SHA基线，当前map按路径替换/删除、独立中性UUID快照；Path≤1024B/leaf255B、Key=规范Path的SHA256，Files按Path排序且Path/Key/ID唯一；基线和当前集合均最多64文件/64MiB且单文件8MiB，当前解析最多100000cases，Parser读取同一稳定快照，私有路径不公开。（T009–010）
- [x] T012 [US1] B先在 internal/pipeline/reports_run_test.go 编写真实本地Run报告红测及无reports回归，在 reports.go完成具体逐run检查/最终required/本地seal函数；root在 run.go/remote.go/artifact.go最小调用：真实finished先确认Started/Stop，再checked，final封存后post；只此时放开Run.prepare.build的严格Reports，零动作不造Evidence。（T011；FR001/004/005/011/013）
- [x] T013 [US1] C在 internal/cli/client/report_test.go 先验证本地run JSON的Counts/Outcome/Reason/安全XML元数据与原快照SHA，再仅按缺口接原run结果展示；不新增report命令，不展示rawXML/secret/私有SnapshotPath。（T012；FR014；AC1.3）
- [x] T014 [US1] 根执行 quickstart.md 的真实本地通过/失败/错误/跳过与1→5case替换、非零exit7仍收新XML、原命令Reason/ExitCode/Started/Stopped保留，将实际字节/摘要/UTC/命令写 specs/019-test-reports/validation.md。（T008–013；SC001/FR017）

**Checkpoint**：可运行本地计数及原XML证据，远程尚不放开触发；剩US2/3安全/生命周期与US4中央门必须完成后整功能验收。

## Phase 4：US2 拒绝旧报告与不安全内容（P1）

**目标**：旧报告不能通过，本次文件和解析有界，required=false仅允许缺失。
**独立验证**：US1本地真实入口上逐项负例，无越界读取/未知文件删除；US2.AC1–3全部可验。

- [x] T015 [US2] B先在 internal/pipeline/reports_freshness_test.go 写真实预置旧文件、不改/改写保留mtime/同内容新inode、已接纳后删除、多glob去重和每模式缺失、--step准备但旧XML存在门；旧文件不解析/不删除，准备阶段缺失不误required失败，final true每模式至少一个当前新鲜文件。（FR005/006；AC2.1–2）
- [x] T016 [US2] B在 internal/pipeline/reports.go 仅修T015实际缺口，记录首次身份/mtime/hash与当前接纳版本、稳定未变ID、删除移出、required默认true/optional缺失Outcome=missing且Counts0；optional坏文件仍失败，无reports/build.when false/precheck/全steps skipped不读目录或假seal。（T015）
- [x] T017 [US2] A在 internal/reports/junit_security_test.go 补每限额边界及非法XML/编码/namespace/多root/数值溢出/负数指数NaNInf/巨大诊断/重复结局的真实字节样例；若T009已有正确行为只记录，通过真实缺口修 junit.go，不引第三方resolver/通用codec。（FR002/008/010；AC2.3）
- [x] T018 [P] [US2] B在 internal/pipeline/reports_boundary_test.go 补os.Root/非阻塞打开后fstat、symlink目录/leaf、FIFO、路径越界/动态模板越界、打开/复制竞争及snapshot写失败门；在 reports.go复用原有限collector边界，复制前/打开后/复制后身份与SHA稳定，不用后台弃置读goroutine。（T016；FR008/009；AC2.3）
- [x] T019 [US2] B在 internal/pipeline/reports_secret_test.go 验证rawXML、实体编码属性/文本、相对Path中的实际声明secret均report_secret，诊断/JSON/日志安全，原XML不上传/公开、不脱敏改写伪造原字节；必要修 reports.go，root保持LocalReports仅私有字段。（FR010/014）
- [x] T020 [US2] 根按 quickstart.md 新鲜性/输入表实际运行全部负例与optional缺失/非法、--step、零动作及无报告回归，记录旧文件identity/hash保持、有限退出/错误不含raw内容于 specs/019-test-reports/validation.md。（T015–019；SC002/005/FR017）

## Phase 5：US3 及时阻止后续动作并固定报告结果（P1）

**目标**：每ordinary run结束及时阻断，final/文件确认/seal耗时计ordinary，普通结果确定后post不改证据。
**独立验证增量**：本地真实prepare→test→哨兵/post与预算/Authority门。中央post/manifest因果约束由US4加入后复验，不能为checkpoint使用stub。

- [x] T021 [US3] B先在 internal/pipeline/reports_lifecycle_test.go 写真实准备无报告继续、失败/错误/非法后普通哨兵不启动、原exit/timeout/cancel优先、failure/success/always正确选择、post改写通过XML不改变失败证据，post失败/取消不覆盖原Reason。（FR004/005/011/012；AC3.1–3）
- [x] T022 [US3] 根在 internal/pipeline/run.go、remote.go 按T021最小接入真实reported失败和post因果门；B的 reports.go仅保存普通结论，后置选项仍未支持，发布前只提供同执行seal核验，不运行approval/upload或伪artifact步骤。（T021）
- [x] T023 [US3] B先在 internal/pipeline/reports_budget_test.go 写真实普通剩余0、检查≤10s与剩余ns更早、确认耗尽ordinary预算、原失败不被report_error覆盖、post独立预算未借用、用户cancel仍受Authority、失权/保存/log失败禁止always门。（FR013；SC003/005）
- [x] T024 [US3] B在 internal/pipeline/reports.go落实本地扫描/快照/解析每次“≤10s且不超过ordinary剩余ns”，root在 run.go/remote.go沿原Authority/累积budget接保存与回执耗时；0明确拒启动，不退无限，WithoutCancel不恢复失权；真实命令Stop/清理事实先确认，CleanupFailed后不检查或继续动作。（T023）
- [x] T025 [US3] 根实际执行准备→替换失败→哨兵拒绝→failure/always改写、失权与自有文件保存失败，记录原快照/SealDigest和真实PID/PGID停止及无关进程存活于 specs/019-test-reports/validation.md；不把等待时间或延迟当停止证明。（T021–024；SC003/FR017）

## Phase 6：US4 查询本地与中央的可核对证据（P1）

**目标**：真实Node事件/完整XML/server重新解析/Store seal同fence，安全查询下载与终态核对。
**独立验证**：US1–3稳定本地入口+以下真实中央consumer组成增量；SQLite/PG及macOS/Linux同套，不借mock或手工status。覆盖US4.AC1–3并复验前三故事的远程路径。

- [x] T026 [US4] A先在 internal/store/reports_test.go 写双库checked/seal红测：冻结Definition.required、真实ordinary run来源Started/Stopped/无Cleanup、Revision从1严格递增、run检查与Index0 final、未checked拒下一intent、failed阻断、final后不可变/无动作不假建seal。（FR007/011/013）
- [x] T027 [US4] A在 internal/store/reports.go 实现具体checked约束与有限ReportsJSON：Files/Diagnostics“必需非null，包括0条”、按Path排序、Path/Key/ID唯一、每File Counts及aggregate有界；root在 event.go只接reports_checked dispatch和progressReasons，并在 enqueue.go接受严格Reports、以 enqueue_report_test.go真实入队验证，未配置不得产生报告事件。seal实现等待T030，Trigger仍拒未接齐的远程报告能力，不另建事件cursor。（T026）
- [x] T028 [US4] C先在 internal/server/report_test.go 写真实稳定stage有限解析红测，在 internal/server/reports.go 实现调用 ParseJUnit(ctx,stableStage,nil)的具体函数；验证实际原字节/SHA、限额/ctx及解析结果，VerifiedJUnit只来自server内部。此项不要求尚未实现的Store用途提交或HTTP正例；完整purpose HTTP接线等待T029，不以stub绕过。（T009/027；FR009/015）
- [x] T029 [US4] A先在 internal/store/reports_files_test.go 写双库归属/限额门，在 reports.go实现对当前final revision/Key/UUID/Size/SHA/Source的确认；root在 Store artifact.go接真实确认函数，再在 Server files.go/artifact.go接purpose=junit、8KiB声明及原stage→hash/fsync→exclusivePublish→shortDBfence；C在 report_test.go跑真实HTTP用途正负例，包括未知/duplicate/null/大小写/伪verified summary拒绝。普通用途禁VerifiedJUnit、junit必须真实parsed并持久化VerifiedJUnitJSON，原所有purpose合计128文件/4GiB，XML另64文件/64MiB，不按用途各算总配额。（T028；FR007/009/015）
- [x] T030 [US4] A在 internal/store/reports_seal_test.go 写完整文件+server Counts/Diagnostics重算、缺ID/部分传输/旧revision/假source/summary-only/冲突/非法required状态拒绝及seal后不改；在 reports.go实现 ReportSealDigest=Sealed=true的canonical Evidence SHA256，seal事务不做XML IO/网络，固定失败可无坏XML但不得passed。（T029；FR012/013）
- [x] T031 [US4] 根在 internal/store/event.go 接T030真实reports_sealed dispatch及post_selected与build_finished报告门：原ordinary失败/cancel/timeout优先，pending/unsealed拒post，manifest精确seal+IDs[]及全部Counts/原Log/ArtifactSteps/cursors，LastArtifactSeq含两用途、ArtifactSteps只真实artifact；只有完整终态可写008StopKnown，同事务fail无seq推进。（T030；FR011/012/015）
- [x] T032 [P] [US4] B先在 internal/agent/reports_test.go 写真实Store+HTTP事件/XML回传红测，在 internal/agent/reports.go 实现逐run checked、Index0 final、只最终revision上传及seal确认；root在 journal.go/execute.go/artifact.go串行接具体consumer，网络前清LocalReports，重发只原Seq/Digest/offset/ID，不重采、不重置ordinary预算。（T028–031；FR007/013）
- [x] T033 [US4] C在 internal/server/report_stream_test.go 验证junit上传沿旧≤2m且受ordinary剩余/Authority、≤1s读deadline唤醒/old expires commit前复核、失权Connection close与有界drain；root仅按真实缺口调整files.go，不另建网络执行器或扩大超时。（T029/032；FR013/015）
- [x] T034 [US4] 根在 internal/server/trigger.go 取消Reports专用unsupported且保留严格校验；新增 trigger_report_test.go 接实际Reports触发并复验T027 enqueue_report_test.go与T012 Run已接受严格Reports，再由同一Run检查/seal，build.when与待定模板区别正确、所选含upload仍admin+allow且当前未支持能力不放开。（T006/031–032；FR001/007/012）
- [x] T035 [US4] 根在 internal/store/query.go/artifact.go与共享views接实际sealed安全Reports/SealDigest和当前junit元数据；C在 internal/cli/client/build.go/artifact.go及report_test.go复用 build show/artifact ls/download展示Counts/Outcome/Purpose/ID/Size/SHA，不加report命令或泄SnapshotPath。（T030/034；FR014）
- [x] T036 [US4] C先在 internal/server/report_access_test.go、internal/cli/client/report_download_test.go 写实际admin/approver正例和trigger/node负例、未seal/旧revision不可见、节点离线中央原字节可下载、SHA/短流/既存输出拒绝；root在sharedHTTP/file入口仅按缺口接权限及seal过滤。（T035；FR014/015；AC4.1–3）
- [x] T037 [US4] A/C在 internal/store/reports_commit_test.go、internal/server/report_publication_test.go（各唯一文件）证明真实SQL失败发布candidate不可见/不推进seq、完整同ID重传canonical、末尾旧expires/失锁拒且无可信seal；用自有第二DB连接/trigger，不生产testhook、不删未知孤立文件。（T029–031/036）
- [x] T038 [US4] B在 internal/agent/reports_receipt_test.go 真实丢checked/seal/PUT回执后原消息有界恢复，过期/rotate/disabled保存journal闭锁、不重采/重置budget/启动always；完整报告terminal才允许008精确readonly清本条journal，checked/部分文件不能被停止确认冒充结果。（T032/036；FR013/015）
- [x] T039 [US4] 根在 internal/store/retry.go/recovery.go及其报告兼容测试核对008集成：retry继承Definition.Reports但ReportRevision/Final/Files/IDs/Seal/cursors均新空，Recover不重解析post XML、不重置预算；TerminalReceipt完整Digest含ReportManifest，仅真实完整terminal拥有StopKnown，无019占位。（008已验收基线、T031/038）
- [x] T040 [US4] 根用独立SQLite/PG数据库、私有token/session/DataDirs、自有verified HTTPS/CA与受信固定Git，真实三二进制跑同套报告通过/失败/替换/post/20幂等/取消/失租/保存失败/节点离线下载/角色/第二控制端门，将CommandsUTC/固定SHA/原XMLSHA/安全断言写 specs/019-test-reports/validation.md。（T026–039；FR016/SC004–005）
- [x] T041 [US4] 根/B在真实macOS/Linux节点各生成通过/失败/非法/新鲜报告，推进HEAD仍原固定SHA、不同build workspace不混文件；验证组停止/无关进程及原预算、中央下载/完整manifest与无reports回归，写 specs/019-test-reports/validation.md。（T040；FR007/016/017/SC004–005）

## Phase 7：Polish与整功能验收

- [x] T042 根逐SHA串行整合所有分区，复读全部18FR/6SC/12AC与共享文件唯一writer，gofmt/diff检查；将实际红绿/版本/UTC/限制映射写 specs/019-test-reports/validation.md，发现缺口继续implement，不能只编译或summary冒充原文件确认。
- [x] T043 C提供命令/路径最小文档补丁；根唯一更新 README.md、docs/IMPLEMENTATION_HISTORY.md、docs/plans/DELIVERY.md、specs/019-test-reports/quickstart.md与plan/research状态；明确已实现Reports、008兼容和仍未实现商店/审批/Apple资源，整个MVP未完成。（FR018）
- [x] T044 根跑真实 go test ./...、go test -race ./...、go vet ./...，三CLI原12次CGO0跨平台构建与help/version/local配置隔离，以及无报告007/008必要回归；写exitcode/实际版本于 specs/019-test-reports/validation.md。（T042；SC005–006）
- [x] T045 根实际重跑 quickstart.md 所有门与双库/macOS/Linux证据，核对本地/中央原XML字节/Counts/Source/seal和全部权限/发布哨兵/失权完整manifest，再执行 speckit-converge；保存 specs/019-test-reports/convergence.md，非0缺口继续implement/converge，不缩FR。（T040–044；SC001–006）
- [x] T046 根只在全部验收/收敛通过后按git-commit-message检查工作区与暂存，本地一次整019提交规范/任务/代码/验证；在 docs/IMPLEMENTATION_HISTORY.md 记录真实hash/信息，不按任务提交、不push。（T045；FR018/SC006）

## 依赖与执行顺序

- Setup T001–002 → 基础T003–007 → 四故事 → 全功能T042–046。基础仅具体类型/迁移/配置，不能以stub放开实际Reports入队。
- US1最先形成真实本地增量（T008–014）；US2/US3沿该入口验证/修实际边界。US4双库可在基础后先写T026红测，落地必须等协议及parser真实交接；Agent必须等真实Store+HTTP到位，不挂未来空consumer。
- US4核心顺序：T026 → T027实际checked与严格Store入队 → T028稳定stage解析 → T029实际Store文件确认及HTTP用途 → T030完整seal → T031seal/post/terminal接线 → T032–034真实Agent与公开Trigger → T035–039 → T040–041。T039只有008真实已验收整合后执行，不在019Plan WT偷借8未验收source。
- 所有后置动作都等final checked→完整XML确认→seal；Store报告检查/归属约束属于US4，前三故事远程验收在US4后复验。没有宣称四US源码完全互不依赖；每个checkpoint都用实际入口，不创建第二Run或假artifact步骤。
- A Storereports与root event/enqueue/model/artifact dispatch同包不同文件仍先冻结具体函数再root同步；C server解析与root files/artifact dispatch同理；B Run/Agent reports函数先交root串行接共用文件。共享protocol、RunTypes、journal、Store终态类型定SHA后才跨分区消费。

## 可执行并行示例

- 基础T007后：A T008–009纯parser与B T010真实文件/脚本红测不同writer，可并行；T011使用真实ParseJUnit须等T009。
- US2：A T017解析负例与B T018文件边界负例不同文件；B own reports.go仍只一个writer，修复顺序化。
- US3：B T023预算/Authority红测与A US4 T026 Store红测在基础后不同包/文件，可并行；root T024与T027共享event接线不并发写。
- US4：T031完成后B T032真实Agent消费者与C T035视图/CLI验证可并行准备，真实详情需T034已接Reports入队；T037 A/C每人只写注明的独立test文件，其DB夹具独立且同数据库锁测试串行。

## FR/SC与12AC映射

| 要求 | 任务 |
|---|---|
| FR001 | T006/012/027/034 |
| FR002 | T008–009/017/030 |
| FR003 | T010–012/015–016/027 |
| FR004 | T010–012/021–022/027/032 |
| FR005 | T012/015–016/020/027/030 |
| FR006 | T015–016/020 |
| FR007 | T003–005/007/026–034/039–041 |
| FR008 | T006/008–009/017–018/020 |
| FR009 | T010–011/018/028–030/036–037 |
| FR010 | T008–009/017/019–020/036/040 |
| FR011 | T021–022/027/030–031 |
| FR012 | T021–022/030–031/034/039 |
| FR013 | T012/023–025/027/031–033/037–039 |
| FR014 | T013/019/035–036/040 |
| FR015 | T028–033/036–041 |
| FR016 | T040–041 |
| FR017 | T008–041/044–045 |
| FR018 | T001–007/042–046 |
| SC001 | T014/040–041/045 |
| SC002 | T015–020/040–041/045 |
| SC003 | T021–025/040–041/045 |
| SC004 | T028–041/045 |
| SC005 | T023–025/037–041/044–045 |
| SC006 | T042–046 |
| US1.AC1 | T008–012/014/040–041 |
| US1.AC2 | T010/012/014/021–022/040 |
| US1.AC3 | T011/013–014/028–036/040 |
| US2.AC1 | T015–016/020/041 |
| US2.AC2 | T012/015–016/020/030/040 |
| US2.AC3 | T006/008–009/017–020/028/036–037 |
| US3.AC1 | T012/021–022/027/032/040 |
| US3.AC2 | T021–025/031/040–041 |
| US3.AC3 | T022/030–031/039–041 |
| US4.AC1 | T013/035–036/040–041 |
| US4.AC2 | T028–031/036–041 |
| US4.AC3 | T023–025/033/036–041 |

## 实施策略

首个MVP增量为真实本地US1，随后在相同执行入口验证US2/3和完整US4远程归属；整个019的四故事都是验收范围，不以首个checkpoint替代完整功能。每轮只修实际缺口，记录真实红/绿及尚未通过的门；双库/两平台和中央原XML完整性/秘密/Authority证明不可用模拟执行或手工状态代替。最终只有一次功能提交。
## Phase 8：2026-10-08 报告数量增量

- [x] T047 [US5] 根在 internal/config/types.go、validate.go、reports_test.go 增加 max_files 默认256、范围1–1024与严格类型边界。（FR019）
- [x] T048 [US5] 根在 internal/pipeline/reports.go、reports_checkpoint.go、run.go 传递有效数量并验证默认/自定义扫描、初始基线和恢复；在 reports_limits_test.go 留下真实XML检查。（T047；FR020）
- [x] T049 [US5] 根在 internal/agent/reports.go、artifact.go、approval.go 与 internal/store/reports.go、artifact.go、retention_protection.go、publish_origin.go 统一报告配额并分开普通制品128份，验证混合上传与发布候选。（T048；FR020）
- [x] T050 [US5] 根协调 protocol/approval.go、server/agent.go/json.go、agent/http.go/journal及其所有恢复/retention消费者、cli/client/remote.go 的有限元数据容量；验证1024份及长路径消息、journal恢复。（T049；FR021）
- [x] T051 [US5] 根用真实Run、Store和HTTP验证最大数量收集、上传封存、审批恢复与下载；跑必要全量test、相关race、vet与构建，记录 validation.md。（T047–050；FR022/SC007）
- [x] T052 [US5] 根同步 README、配置文档、quickstart/contracts/研究/数据模型、实施历史；analyze/converge无缺口后仅暂存相关修改，一次本地提交。（T051；FR022/SC007）
