# Tasks：011 App Store 上传、显式提交与未知结果核对

**输入**：spec/plan/research/data-model/contracts/quickstart，28FR/8SC/17AC，原则2.1.0。
**状态**：仅规划；008/009/019验收集成、005真实分发签名和ASC/AppReview材料仍是实施/验收前置，不借dirty实现。
**格式**：连续T IDs；[P]仅阶段前置已完成、不同writer文件；先真实行为红测再实现，无fake工具/假seal/退出0冒充商店验收。

## Phase 1：Setup

- [x] T001 根在 specs/011-app-store/validation.md 核实008/009/019正式验收提交、005真实签名门及010共同接口实际消费者；重基线并记录未满足门阻止源码实施。规划任务可先准备。（FR027/028）
- [x] T002 根阅读 README.md、docs/plans/BUILD_DISTRIBUTION.md、MVP_EXECUTION.md 与原则2.1.0，在 specs/011-app-store/validation.md 记录selector/checklist/hooks、真实故障测试隔离、自有DB/工具/节点/应用明确授权范围；本轮不安装/操作商店。
- [x] T003 根在 specs/011-app-store/validation.md 冻结A Store新业务、B Apple publisher、C config/HTTP/CLI新接线、root所有共享既有文件/类型/Gem锁/集成；010/011同文件串行，协议以两个真实consumer红测冻结。

## Phase 2：Foundation

工具真实故障计数和共同锁→协议实际consumer→双库迁移→严格输入，无stub；未满足T001不能开始源码实施。

- [x] T004 B在 internal/distribute/apple_transport_test.go 先真实Ruby/受控endpoint原型红测POST/PATCH 500/504/429、接收后断开、401刷新及middleware，每不可逆endpoint计数1；GET有界。核实Transporter实际认证分支，JWT argv/fallback拒，完整会话/分块/提交须继续T025真材料首门验，工具锁不代表已证明安全、缺证据不得标核验；不把无凭据原型当上传。（FR003/004/016）
- [x] T005 根依T004证据在 internal/distribute/fastlane.go、Fastfile、Gemfile、Gemfile.lock 落两个实际publisher共用最小process.Run入口/有限stdin或受限文件/固定安全结果；精确Ruby/Bundler/fastlane真实锁定，2.240.1仅候选，禁止用户lane/宿主安装更新/个人交互登录。（FR003/016）
- [x] T006 根先实际协议consumer红测后在 internal/protocol/node.go、node_test.go 冻结010共同类型与Apple optional variant：完整Ref、原NS、数组[]、omitempty旧digest兼容、六Action/PreviousIntentID/RequestSHA256；ID≤128bytes、摘要64hex、状态白名单，不建第二AppleAction/序号/执行器。（FR006/013/018）
- [x] T007 A在 internal/store/apple_models_test.go 先真实两库旧迁移/二次迁移、全局唯一(Store,AppIdentifier)、唯一BindingID guard无TTL、IntentID及同Ref/Index/Action唯一、前意图关系/FK RESTRICT、重复幂等/冲突红测；不drop旧证据/造未来状态。（FR012/013/018/019）
- [x] T008 A在 internal/store/apple_models.go 落实际Apple约束；root串行接现有 internal/store/models.go、node_models.go、store.go 共同字段/migration（沿010单PublishIntent），旧receipt不猜确认；T007双库绿后SHA交接。（FR012/013/018）
- [x] T009 C在 internal/config/apple_test.go 写真实Step/API key JSON读取红测：JSON≤64KiB/0600/owner/单链接普通文件/no symlink/FIFO；key_id、UUID issuer_id、PKCS8 P-256 key；duration整数1..1200缺省200/in_house仅false，拒unknown/duplicate/null/错类型/key_filepath；credentials完整${NAME}。（FR004/008）

## Phase 3：US1 上传原固定签名IPA（P1，首个增量）

默认上传的真实绑定、doctor、grant和基础safe ls/show全部在本故事；独立门：native与Flutter各实际签名IPA上传ASC，无审核/公开请求。缺材料待验，不借US3未实现接口。

- [x] T010 [US1] A在 internal/store/apple_publish_test.go 写pending绑定→当前原节点实际GET doctor verified、跨项目拒、原Number/Ref/IPA/report seal精确归属、20同app一次grant与存储失败无授权的双库红测。（FR006/007/012/013）
- [x] T011 [P] [US1] B在 internal/distribute/apple_test.go 用自有真实IPA/签名工具写原唯一文件边界/替换/链接/多匹配、bundle/version/number/distribution TeamID和签名错误、受限材料/输出红测，无假codesign。（FR002–005）
- [x] T012 [P] [US1] C在 internal/config/apple_preview_test.go 写app_store目标字段/一次模板/布尔缺省false、跨目标拒、本地生效upload任何脚本前整批拒/纯dryrun无secret或网络红测。（FR008/026）
- [x] T013 [US1] A在 internal/store/apple_binding.go、apple_publish_test.go 接共同BindApplication Apple规则和doctor完成：唯一store/app、ProjectID不可变、原NodeID/credential引用内部、pending不授权，实际GET app/bundle一致才verified，改组不变归属。（FR003/012）
- [x] T014 [US1] A在 internal/store/apple_publish.go 实现AuthorizePublish/RecordPublish基本动作：短Store.write先guard+原意图unknown再一次grant，末尾重查控制锁/原ExpiresAt/actor/admin+allow/原Artifact-size-SHA/report seal精确IDs，有声明才必须真实seal、无声明不造seal/pass；重复不重授/摘要冲突拒，中间receipt不解guard。（FR006/007/011/013/015–020）
- [x] T015 [US1] B在 internal/distribute/apple.go 实现PrepareApple：原IPA普通文件/ZIP边界/真实签名profile证书/005 DistributionTeamID/版本号核验、自产0700 HOME及0600密钥/IPA副本、受限只读工具和GET前提/固定错误；issuer_id不是签名TeamID，不改签重建、不扫描个人资源。（FR002–007/014）
- [x] T016 [US1] B在 internal/distribute/apple.go、apple.rb 实现仅upload_binary UploadApple：固定入口/真实process.Run、一次grant会话/请求摘要/onStart、真实transport关联回执/有界安全输出；同会话同range恢复须真证据，未知创建/完成不重发，exit0不足证明uploaded。（FR001/004/014/016/018/023）
- [x] T017 [US1] C在 internal/config/apple.go 使T009/T012绿，app_store仅明确Apple字段、Runner ios/macOS实际能力、credentials完整引用；两布尔缺省false，automatic=true要求submit=true，保旧目标参数模板行为。（FR002/004/008/026）
- [x] T018 [US1] root在 internal/agent/apple_management.go、serve.go 接真实binding doctor管理消费者：当前独立node/session、有限只读工具/GET、固定安全结果，不借旧lease/不占build槽/不导入用户token或个人账户。（T013/015；FR003/004/012）
- [x] T019 [US1] root在 internal/agent/apple_publish.go、journal.go、execute.go 接唯一RemoteOptions.Publish：Prepare→候选IntentID先fsync→Authorize→grant fsync→Upload→私有receipt→Record；丢授权响应不重发，真实step started只首次进程一次，后action私有证据。（FR006/013/015–018）
- [x] T020 [US1] root在 internal/pipeline/run.go、run_types.go、apple_publish_test.go 接同Run upload/OnStart：批次预检查、普通run/report seal后上传原artifact、NS与Authority交集；nil Publish/本地拒，不解析日志/造伪artifact/第二Run。（FR001/006/007/015/026）
- [x] T021 [US1] C在 internal/server/apple.go、apple_test.go 接实际binding/基础publish ls-show HTTP：admin管理、approver只读、trigger/node不冒用户、严格JSON≤64KiB/固定安全错；DTO无credential/私有path/argv/env。（FR003/011/012/025）
- [x] T022 [US1] C在 internal/cli/client/apple.go、apple_test.go 接 project app bind/ls --store app_store、publish ls/show、真实doctor --target app-store；root串行在 internal/cli/client/root.go、project.go、doctor.go 注册实际新命令/target消费者（010已有命令只扩目标，不重复建入口）；仅credentials-env引用、remote时才读client配置，旧本地doctor无材料skipped/无需远程token。（FR003/004/025/026）
- [x] T023 [US1] A在 internal/store/apple_publish_test.go 两库真实并发/insert失败rollback/第二控制端/失锁/过期/旧node-session-token/错app-version-artifact-report核验；20不同build/node同应用一次grant、不同应用独立。（FR006/007/012–015；SC003/004）
- [x] T024 [US1] root串行接 internal/store/event.go、recovery.go、retry.go、query.go 及 internal/agent/apple_publish.go：真实upload.finished先关slot，之后各候选ID逐条FindNodePublish只读、完整PublishIntents终态集合；旧omitempty digest兼容、008物理停止receipt不当发布确认，中间Record不解guard。（FR013/017–019/027）
- [x] T025 人工验收准备与证据登记（原真实验收由用户集中执行，不宣通过）：[US1] root/B在 specs/011-app-store/validation.md 记录明确授权真实native签名IPA默认上传ASC：app/version/number/原IPA hash/report seal/实际transport ID，无select/version/metadata/review/release请求；实际会话/分块/最终提交重试边界须证明，缺材料待验。（SC001/003/008；US1.AC1/3/4）
- [x] T026 人工验收准备与证据登记（原真实验收由用户集中执行，不宣通过）：[US1] root/B在 specs/011-app-store/validation.md 实测Flutter签名IPA同publisher/原build归属、默认仅上传及远端构建核对；不能用native成功代Flutter，Linux/签名/商店能力不足明确负例。（SC001/003；US1.AC2/4）

## Phase 4：US2 显式提交App Review与选择正式发布（P1）

依US1。原Run普通预算内六个具体变更，每请求独立意图；独立门：真实已准备版本明确提交，MANUAL/明确许可策略，缺前提不伪成功，不等待审核通过。

- [x] T027 [US2] A在 internal/store/apple_review_test.go 先两库原flags/精确PreviousIntentID顺序/Ref-Index/body摘要不可换/前confirmed后unknown、不接管外部draft红测。（FR008–010/018）
- [x] T028 [P] [US2] B在 internal/distribute/apple_review_test.go 写锁版本真实Ruby六变更method/path/body allowlist和未知POST/PATCH计数1红测；上传不等提交，元数据/version仅GET，固定build不选latest。（FR009/016/018）
- [x] T029 [P] [US2] C在 internal/server/apple_review_test.go 写默认false/admin+allow/automatic须submit/safe uploaded-submitted-published区分/无terminal submit入口红测。（FR008/010/011/023）
- [x] T030 [US2] A在 internal/store/apple_publish.go、apple_review_test.go 接 upload_binary→select_build→set_release_policy→create_review→add_review_item→submit_review 同链guard；前confirmed再后授、原flags/RequestSHA冻结、选择/策略已精确满足仅只读前提，不造意图/Started。（FR008–010/013/018–020）
- [x] T031 [US2] B在 internal/distribute/apple.rb、apple.go 每次UploadApple只一个grant具体变更：PATCH build关系、releaseType(MANUAL/AFTER_APPROVAL)、POST review/item、PATCH submitted=true，核对原ID/body/关系；无ensure_version/素材/合规/pricing/release-now扩展。（FR009/010/016/018）
- [x] T032 [US2] B在 internal/distribute/apple.go、apple_review_test.go 实现原ctx/普通NS内processing GET和前提，每GET/总query≤30s、响应/分页有界/ctx更早优先；VALID/原元数据权限齐备才继续，0预算保uploaded+未授submit，不重传或终态续动作。（FR009/015/024）
- [x] T033 [US2] root在 internal/agent/apple_publish.go、journal.go 串行接六请求：各候选先fsync/grant-receipt精确digest，后action start仅私有证据，前动作已confirmed不再做、unknown断链，不重跑deliver整lane。（FR013/015–019）
- [x] T034 [US2] root在 internal/pipeline/apple_review_test.go、run.go 核验Prepare/GET/授权/写/确认计原累计NS含0耗尽，cancel/失权、有限独立Close、原Reason优先；unknown/物理stop区分，CleanupFailed禁post/后续，不复活预算。（FR015/017/019/027）
- [x] T035 [US2] C在 internal/config/apple.go、server/apple.go、cli/client/apple.go 接原flags预览/安全显示；root串行接 internal/pipeline/preview.go；默认/retry仍false，无terminal submit命令/新发布生命周期。（FR008/010/023/026）
- [x] T036 人工验收准备与证据登记（原真实验收由用户集中执行，不宣通过）：[US2] root/B在 specs/011-app-store/validation.md 实测同Run明确submit=true/automatic=false：processing、原build/version、MANUAL、精确submission/item正式submitted；缺前提不伪提交，不等外部审核。（SC002/008；US2.AC1–3）
- [x] T037 人工验收准备与证据登记（原真实验收由用户集中执行，不宣通过）：[US2] root/B仅在用户明确授权范围实测automatic=true→AFTER_APPROVAL及submit前提，默认不公开/无release-now请求；缺真实授权材料待验，不默许公开上架，记录 specs/011-app-store/validation.md。（FR010；SC002；US2.AC1/3）
- [x] T038 [US2] root/A/B用真实客户端/两库验后动作失败/超预算保已uploaded、未授不是unknown、已授缺ACKunknown、原Reason/NS/单次Started保持；非admin/缺allow/whenfalse不越权，记录 specs/011-app-store/validation.md。（FR008–011/018–020；SC002–004；US2.AC4）

## Phase 5：US3 共用应用保护并核对未知动作（P1）

依US1/2真实动作；扩GET query/admin confirm，unknown不重放。独立门：20竞争、缺ACK跨重启保guard、查询不足不清、精确依据决定幂等。

- [x] T039 [US3] A在 internal/store/apple_unknown_test.go 先两库授权可能提交启动前后断联、guard跨restart/cancel/StopKnown、归属/摘要/决定冲突、query-confirm竞态红测。（FR012/017–022）
- [x] T040 [P] [US3] B在 internal/distribute/apple_query_test.go 真实客户端故障服务验证仅GET、零/多候选/仅uploaded不能解submit unknown、origin/path/分页/大小/ctx、wrongCA/redirect拒，不建query草稿或远端写。（FR021）
- [x] T041 [P] [US3] C在 internal/server/apple_query_test.go 先严格query/confirm角色/JSON/重复冲突/safeDTO/decision-file归属红测，无自动新授权。（FR021/022/025）
- [x] T042 [US3] A在 internal/store/apple_publish.go、apple_unknown_test.go 保guard无TTL：grant即unknown、仅可信未发送/明确拒绝才failed，审核REJECTED不抹前动作，restart/retry/stop不解锁；必要证据不孤立解绑删除。（FR012/014/017–020/025）
- [x] T043 [US3] A在 internal/store/apple_query.go 接共同Request/Claim/CompletePublishQuery Apple规则：当前node/session/nonce/expiry、原Intent派生，Kind仅doctor/query、Matches≤16/fullJSON≤64KiB/总30s、提交前锁/expiry复核；部分观察不解unknown，不占build槽。（FR021/024）
- [x] T044 [US3] A在 internal/store/apple_confirm.go、apple_unknown_test.go 接admin精确原intent/action/原摘要-当前revision，证据码仅remote_receipt/remote_state/confirmed_not_sent/remote_rejected、有限非机密依据；同决定幂等/冲突/仅PID或StopKnown拒，审计身份UTC，不授新write。（FR020/022）
- [x] T045 [US3] B在 internal/distribute/apple.go、apple.rb 实现QueryApple固定官方origin仅GET、精确app/build/version/submission/item/release关系；来源不足保upload unknown，submit须原submission/item已提交，空结果不证明未发送、不补version/draft。（FR021/023）
- [x] T046 [US3] root在 internal/agent/apple_management.go、serve.go、http.go 接真实query消费者：当前身份不用旧Ref、独立30s/无Run/无slot/不续预算、原nonce/digest至原expiry，持久化失败/撤销/过期拒，不terminal启动提交。（FR017/021/024）
- [x] T047 [US3] C在 internal/server/apple.go、apple_query_test.go 接query/query-show/confirm：admin发起/决定、approver只读、trigger/node不冒用户；严格重复null/超限/冲突/固定码，202排队不等Apple。（FR021/022/025）
- [x] T048 [US3] C在 internal/cli/client/apple.go、apple_query_test.go 接publish query/query-show/confirm --decision-file PRIVATE_JSON：有限0600普通/no symlink/FIFO/duplicate/null拒、exactIntent/digest/key、一次HTTP不重发；root在 internal/cli/client/root.go 串行注册尚未存在的query/confirm实际命令（010已有入口只扩Apple）；旧本地命令不读remote配置。（FR021/022/025/026）
- [x] T049 [US3] root在 internal/agent/apple_recovery_test.go、journal.go、internal/store/event.go 复验候选ID lookup/StepClosed/完整manifest：活动404不删unknown、闭slot未授权仅排除该候选、不授新grant；008StopKnown不解publish guard。（FR017–019/027）
- [x] T050 [US3] A在 internal/store/apple_unknown_test.go 两库真实失锁/原ExpiresAt到期/20grant/决定竞态/Recover一致性，原artifact/report/Ref/NS保持；原Retry/recovery消费者兼容，无未来假状态。（FR013/015/019/022/027；SC004/005）
- [x] T051 人工验收准备与证据登记（原真实验收由用户集中执行，不宣通过）：[US3] root在 specs/011-app-store/validation.md 实际三入口两库/两mac节点20同app多build一次grant、不同app独立/跨项目拒、版本冲突/控制端restart/启动前失联unknown实证。（SC004/005；US3.AC1–4）
- [x] T052 人工验收准备与证据登记（原真实验收由用户集中执行，不宣通过）：[US3] root/B在 specs/011-app-store/validation.md 对明确授权真实ASC上传/提交分别丢中央receipt响应/请求、重启保unknown/appguard，结合T028六变更故障计数、不重做前动作/不以stop代结果；缺真实材料不PASS。（SC005/008；US3.AC3–5）
- [x] T053 人工验收准备与证据登记（原真实验收由用户集中执行，不宣通过）：[US3] root/B实际GET query与有依据admin精确confirm，记录app/version/build/submission/item/action关系、GET计数、零/多候选/仅上传不解提交、幂等冲突和原证据于 specs/011-app-store/validation.md。（SC005；US3.AC5–6）

## Phase 6：US4 查看真实状态和安全证据（P2）

依前三故事；US1已有基础safe ls/show，此处补完整状态/安全/槽释放/兼容。独立门：真实后续查询、下一queued、角色与秘密负例、原关联证据保持。

- [x] T054 [US4] C在 internal/cli/client/apple_view_test.go、internal/server/apple_view_test.go 先表格/JSON同safeDTO、Observed与confirmed分开、secret/脚本/私有path/参数值不公开、node/trigger/approver权限红测。（FR023/025）
- [x] T055 [P] [US4] B在 internal/distribute/apple_security_test.go 实际工具错误/verbose/私有p8/JWT/路径注入测有界诊断/固定安全结果，不靠仅env路径脱敏；无完整环境/个人session/代理绕TLS/argv秘密。（FR004/025；SC006）
- [x] T056 [US4] A在 internal/store/apple_view.go、apple_view_test.go 接Get/List安全结果及不可变audit，只证实uploaded/processing/submitted/published；unknown/必要原IPA/report/关系有实际读取/删除拒消费者，不造未来approval/retention字段。（FR023/025/027）
- [x] T057 [US4] C在 internal/server/apple.go、internal/cli/client/apple.go 使T054绿：分页limit1..100/固定ID游标、不CredentialRef/private AuthDigest、上传/提交独立状态；部分GET观察不假confirmed/published。（FR023/025）
- [x] T058 [US4] root在 internal/agent/apple_queue_test.go、internal/store/claim_test.go 真upload.finished/terminal后下一queued运行，不等processing/review；unknown仍appguard、物理停止可释放普通槽，query不建号/Run/submit。（FR019/024；SC007；US4.AC1）
- [x] T059 [US4] B在 internal/distribute/apple_security_test.go、apple.go 实测Prepare半失败/正常/失败/取消有限Close仅自产目录/副本，身份替换不删未知、不remote abort；原process仅本scope取消、无关进程活。（FR004/015；SC006）
- [x] T060 [US4] C在 internal/cli/client/apple_doctor_test.go、B在 internal/distribute/apple_security_test.go 串行复验工具锁不符拒/无材料skipped/指定材料不读个人账户/Linux缺Apple能力/坏client配置不影响local init/run/doctor/help/version。（FR002–004/026）
- [x] T061 [US4] root在 internal/server/apple_integration_test.go、internal/store/retry_test.go、internal/agent/terminal_recovery_test.go 复验008fullreceipt/新retry空发布权、019原seal IDs/测试失败阻发布、009框架/节点/原artifact-log-NS及旧digest；生效未实现approval整批拒。（FR006/007/017/027）
- [x] T062 [US4] root在 specs/011-app-store/validation.md 留实际unknown/AppBinding/IPA/report/audit保护IDs及014/020后续联验，未来接入须验实际消费者；不让011反向等待后续提交、不造未来状态。（FR025/027；US4.AC3）
- [x] T063 人工验收准备与证据登记（原真实验收由用户集中执行，不宣通过）：[US4] root实际三入口两库安全查询/角色、后续processing/review状态、下一任务、无再次上传/提交和公开秘密扫描验证US4，记录 specs/011-app-store/validation.md；不等审核通过或fake published。（SC006/007/008；US4.AC1–3）

## Phase 7：Polish与整功能验收

四故事全部属于011交付；每增量checkpoint验证但不分任务提交，014/020后续联验不反向依赖。

- [x] T064 人工验收准备与证据登记（原真实验收由用户集中执行，不宣通过）：root在 specs/011-app-store/validation.md 汇总28FR/8SC/17AC实际源码/证据/SHA/UTC/版本/命令/安全ID，失败原样保留；native/Flutter签名、ASC上传/AppReview任缺则待验不提交。（FR028；SC001–008）
- [x] T065 root当前统一字节执行SQLite/PG同suite事务/20竞争/恢复/原expiry/应用锁/原证据/有界JSON-files/失锁unknown门，在 specs/011-app-store/validation.md 记录，不用DryRun PG代事务。（SC003–006/008）
- [x] T066 人工验收准备与证据登记（原真实验收由用户集中执行，不宣通过）：root在 specs/011-app-store/validation.md 汇总两mac实际native/Flutter发布/Linux能力负例/cancel-Authority/appguard/verifiedTLS-CA/角色，副作用仅原明确测试授权，不默认公开。（SC001/002/005–008）
- [x] T067 执行本功能必要普通/race/vet与三入口六平台18次编译、6本机help/version，登记真实退出码；全MVP合并后再统一执行一次全量检查，避免按模块重复长矩阵。原真实商店验收按用户安排独立待验。
- [x] T068 root同步 README.md、docs/plans/PLAN.md、docs/plans/DELIVERY.md、docs/IMPLEMENTATION_HISTORY.md、specs/011-app-store/quickstart.md 实际工具锁/命令/状态/材料缺口；保014/020后续联验/全MVP未完成，不伪已公开。（FR025/027/028）
- [x] T069 root执行 speckit-converge，缺口只append继续implement至0，0缺口tasks字节不变；真实报告在技能外写 specs/011-app-store/validation.md。（SC008）
- [x] T070 root全部011自身真实门通过后核对仅本功能差异，用 git-commit-message 一次本地提交规范/代码/任务/证据并记 docs/IMPLEMENTATION_HISTORY.md 哈希，不单任务提交/push；014/020后续联验，缺真实ASC/AppReview不得宣称验收完成。（FR028；SC008）

## 依赖、唯一writer与并行

- T001–003→T004–009→US1→US2→US3→US4→Polish；T001真实依赖是源码开工门，规划可先准备。US1默认上传首个增量，完整011必须四故事及自身真实门。
- US1 T010/011/012三writer红测可并行；共同基础后A T013/014、B T015/016、C T017可流水推进。后继真实接线T018–024须API已交接，无stub，T025/026才真实默认上传门。
- US2 T027/028/029、US3 T039/040/041、US4 T054/055各阶段前置齐备后不同writer测试并行；各实际源码/HTTP/CLI/API按顺序交接，同文件跨story串行。T068 docs在T067 checks后由root串行更新。
- A独占internal/store/apple_*.go及tests；B独占internal/distribute/apple*.go/tests/apple.rb；C独占internal/config/apple*.go、internal/server/apple*.go、internal/cli/client/apple*.go。root唯一protocol、共同fastlane/Fastfile/Gem锁、现有Store模型/migration/event/enqueue/recovery/retry/query、Agent/Pipeline既有文件、CLI root及globaldocs。root新增集成测试与已列A/B文件交接后串行，不同时改写。
- 010/011共同类型/Store/HTTP/工具以010单一定义。010未交付时仅root协调两实际consumer共用实现与门，不复制Publish模型/registry；010整项Google材料验收不作为011新增反向依赖。
- 014依赖010/011；020未知证据保护在其真实接入后联验。不造未来状态；未实现生效approval拒绝。011自身全部真实Apple门是提交前置，后续联验计整MVP。

## 覆盖与实施策略

每故事先实际行为红测，真实API/双库/文件/process/锁版本Rubyconsumer绿后接下一分区；不假lane/假ASC成功/空未来字段。
Setup3、Foundation6、US1=17、US2=12、US3=15、US4=10、Polish7，总70任务。4US都是011范围；无Gem安装或商店操作在本轮发生，implementation按依赖/明确材料授权另行启动。

| Stable ref | 任务 |
|---|---|
| FR-001 | T016, T019, T020, T025, T026 |
| FR-002 | T011, T015, T017, T026, T060, T066 |
| FR-003 | T004, T005, T015, T018, T022, T060 |
| FR-004 | T004, T009, T011, T015, T016, T017, T021, T055, T059 |
| FR-005 | T011, T015, T023, T025, T026 |
| FR-006 | T006, T010, T014, T019, T020, T024, T061 |
| FR-007 | T010, T014, T020, T023, T061 |
| FR-008 | T009, T012, T017, T027, T029, T030, T035 |
| FR-009 | T027, T028, T030, T031, T032, T033, T036, T038 |
| FR-010 | T027, T029, T030, T031, T035, T037 |
| FR-011 | T014, T021, T027, T029, T038 |
| FR-012 | T007, T008, T010, T013, T023, T042, T051 |
| FR-013 | T006, T007, T010, T014, T019, T023, T024, T030, T033, T050 |
| FR-014 | T015, T016, T023, T042 |
| FR-015 | T014, T019, T020, T023, T032, T033, T034, T050, T059 |
| FR-016 | T004, T005, T016, T028, T031 |
| FR-017 | T014, T019, T024, T033, T034, T039, T042, T046, T049, T061 |
| FR-018 | T006, T007, T008, T016, T019, T024, T027, T028, T030, T031, T033, T038, T049 |
| FR-019 | T014, T024, T033, T034, T039, T042, T049, T050, T058 |
| FR-020 | T014, T030, T038, T039, T042, T044 |
| FR-021 | T039, T040, T041, T043, T045, T046, T047, T048, T053 |
| FR-022 | T039, T041, T044, T047, T048, T050, T053 |
| FR-023 | T016, T029, T035, T045, T054, T056, T057, T063 |
| FR-024 | T032, T043, T046, T058, T063 |
| FR-025 | T021, T022, T041, T042, T047, T048, T054, T055, T056, T057, T062, T068 |
| FR-026 | T012, T017, T020, T022, T035, T048, T060 |
| FR-027 | T001, T024, T034, T049, T050, T056, T061, T062, T068 |
| FR-028 | T001, T025, T026, T036, T052, T064, T068, T070 |
| SC-001 | T025, T026, T064, T066 |
| SC-002 | T036, T037, T038, T064, T066 |
| SC-003 | T011, T015, T023, T025, T026, T034, T038, T065 |
| SC-004 | T023, T038, T050, T051, T065 |
| SC-005 | T050, T051, T052, T053, T065, T066 |
| SC-006 | T054, T055, T056, T057, T059, T063, T065 |
| SC-007 | T058, T063, T066 |
| SC-008 | T025, T026, T036, T052, T064, T065, T066, T067, T069, T070 |
| US1/AC1 | T014, T016, T019, T024, T025 |
| US1/AC2 | T015, T026 |
| US1/AC3 | T011, T015, T023, T025 |
| US1/AC4 | T004, T005, T018, T022, T026, T060 |
| US2/AC1 | T017, T029, T035, T036, T037 |
| US2/AC2 | T030, T031, T032, T033, T036 |
| US2/AC3 | T028, T032, T034, T036, T038 |
| US2/AC4 | T027, T029, T038 |
| US3/AC1 | T010, T013, T023, T051 |
| US3/AC2 | T015, T023, T042, T051 |
| US3/AC3 | T019, T039, T042, T052 |
| US3/AC4 | T024, T039, T049, T050, T052 |
| US3/AC5 | T040, T043, T045, T046, T053 |
| US3/AC6 | T041, T044, T047, T048, T053 |
| US4/AC1 | T054, T056, T057, T058, T063 |
| US4/AC2 | T054, T055, T059, T060, T063 |
| US4/AC3 | T042, T056, T061, T062, T063 |

## 2026-10-05 用户验收安排与实施归属（覆盖原阶段门措辞）

用户明确要求先完成全部模块代码及必要自动验证，真实 Apple/Play 上传在最后统一案例由用户人工验收；没有凭据不阻塞代码交付/本地提交。原真实远端需求与 SC/AC 不删除、不宣 PASS，验证记录标人工待验；无凭据自动门仍须实际锁版本、真实本机故障端点单发、未知保护与安全失败。008/019/020已验收基线2602094；005/009按其实际模块接口接入而不虚构真实素材通过。

唯一 writer：publish-channels 分区独占 internal/distribute/**（两店具体 Go/Ruby、Fastfile、Gemfile/lock、测试）及新 internal/protocol/publish.go；root独占既有protocol/node.go、Store、Agent、Pipeline、config、server、CLI共享消费者与全局文档。原表/任务中分散在B/root的 distribute 与公共发布新类型任务统一归publish-channels；其他任务仍root协调唯一writer。一个Run/原process.Run、完整Ref与guard/unknown不变，无registry/第二执行器。

## Phase 8: Convergence

- [x] T071 HIGH 在原Store Record/Confirm消费者核对各Apple具体动作的原grant标识、请求摘要与真实完成关系；上传不能确认submit，任意ActionConfirmed/工具退出0不能替代具体证据，实际双库负例转绿。来源FR006/018/020/022（partial）。
- [x] T072 HIGH 在实际Apple GET query与共同CompletePublishQuery中核对原version/build/submission/item关系；充分证据同事务更新精确原动作及审计，原upload缺transport关联/空/多候选保unknown，不发写请求。来源FR021/US3/AC5、plan: 精确原动作核对（partial）。
- [x] T073 HIGH 真实Apple query最多16匹配、总JSON64KiB、GET30s及管理Close journal故障保护；沿当前Node/session身份，不借原epoch执行新副作用，014正式接入时按其任务联合核验原审批恢复证明，本模块不伪造审批记录。来源FR015/017/024/027（partial）。
- [x] T074 MEDIUM 在011验证记录与统一案例保存最终代码检查、原生/Flutter合法签名及ASC/AppReview人工待验步骤，使用实际命令与默认false选项，不声称已公开。来源FR028/SC008、用户2026-10-05验收安排（partial）。

## 最终实施记录（用户调整后的代码交付门）

所有勾选表示代码、必要自动检查或人工验收准备已完成，不表示真实商店 SC/AC 通过。任务列举的概念已沿共同 publish 文件实现，实际文件/测试映射见 validation.md；不为计划中占位文件名创建第二实现。最终全MVP检查集中执行，不逐模块重复全量；真实签名、商店与外部服务在集中案例由用户执行。
