# 任务：020 项目保留策略与受限清理

**输入**：[spec.md](spec.md)、[plan.md](plan.md)、research/data-model/contracts/quickstart；冻结25FR、7SC、4US/16AC。任务已生成，020正在实施；完成项以实际验证记录为准；008/019已真实验收集成，不用任务勾选冒充020验收。

**前置**：008恢复与019报告必须真实验收并集成后才能开始下面实施任务。正式基线019 `fee97e8`（含008 `504dc6`及test夹具修复b2e659a）；006 `8e1397e`、007 `85b46bf`保留为验收历史。005真实Apple签名门与本功能状态独立。

**流程**：按实际项目模板生成；constitution 2.1.0的I–V保持，无新依赖/框架/执行器。测试来自规范明确要求，先真实红测再实现；不以手改业务状态、mock文件流、sleep或交叉编译替代行为门。每个checkpoint仅交接，不提交；主代理完成本功能converge后一次本地提交，无push。

## 格式与唯一归属

`- [ ] T### [P?] [US#?] 动作与具体文件路径`。`[P]`只指依赖满足后不同文件可并行；测试与同名生产文件必须由同一owner串行红绿。

- A：`internal/store/retention*.go`新文件，资源登记两文件`retention_resources.go`/`retention_resources_test.go`除外，后两者由root唯一writer。
- B：`internal/agent/retention*.go`新文件。
- C：`internal/config/retention*.go`、`internal/server/retention*.go`、`internal/cli/client/retention*.go`新文件。
- root：protocol及所有既有Store/config/Server/Agent/CLI入口、README/规划/validation、集成与提交。同一文件无第二writer；A/B/C交最小可消费文件及SHA再由root串行接入。非Unix拒绝文件只补实际跨编译消费者，不扩大本机删除平台。

## 阶段1：前置与实施基线

- [x] T001 在 `specs/020-project-retention/validation.md` 记录008/019真实验收提交、006/007历史与当前实际字段复核；前置未满足停止实施，不借未验收worktree或假模型。
- [x] T002 在 `specs/020-project-retention/contracts/go-api.md`、`specs/020-project-retention/contracts/node-http.md` 与 `specs/020-project-retention/plan.md` 按已集成真实消费者冻结字段、限额、共享文件writer；保留RetryOf RESTRICT/019原XML与seal/008精确receipt，不改旧wire digest。
- [x] T003 在 `specs/020-project-retention/quickstart.md` 落定自有SQLite、独立PG数据库、verified HTTPS三入口、固定可信Git SHA、节点私有目录与对照文件/进程；命令材料不含token argv，不停用户服务。

## 阶段2：共享基础与真实身份

**阻塞门**：T001–T003完成后才写代码。共享类型先串行交付真实消费者，不建repository、保护hook、GC registry、stub或第二executor。

- [x] T004 在 `internal/store/retention_models_test.go` 写真实两库迁移与唯一关系红测：每build唯一job、每资源稳定DeleteID、nullable可信时间、原FK/最小请求关系仍在；不关FK或硬删父行。
- [x] T005 在 `internal/store/retention_models.go`、`internal/store/models.go`、`internal/store/node_models.go`、`internal/protocol/node.go` 串行接入具体policy/job/object/evidenceRead/nodeResource/nodeDeletion及receipt字段；状态、UUID、固定reason、正int64 Seq、arrays显式[]沿data-model/contracts约束。
- [x] T006 在 `internal/store/retention_time_test.go` 写真实终态来源红测：完成、queued取消、创建skipped、Expire实际中断；重复事件/停止确认/一般更新不刷新，旧不明时间不猜。
- [x] T007 在 `internal/store/store.go`、`internal/store/enqueue.go`、`internal/store/event.go`、`internal/store/stop.go`、`internal/store/recovery.go`、`internal/store/node.go`、`internal/store/node_session.go` 串行接一次性服务端TerminalAt；旧迁移仅精确build_finished receipt.CreatedAt、无attempt skipped及真正取消最早审计可证，不用Node.At/UpdatedAt/LeaseExpiresAt回填。
- [x] T008 在 `internal/server/retention_read_test.go` 写实际同inode共享/排他非阻塞锁及fd关闭释放红测；macOS/Linux实跑，非Unix明确unsupported，不用TTL证明读者结束。
- [x] T009 在 `internal/server/retention_read_unix.go` 与 `internal/server/retention_read_other.go` 实现两个实际消费者所需的有限fd锁/identity操作；固定安全错误，非Unix分支仅支持既有纯Go编译并明确拒绝本机删除。

## 阶段3：US1 管理项目有效策略（P1）

**目标**：只读/修改管理员策略，尚不启动删除。独立门：两项目四种继承、受控全局重启、全部非法/权限输入保持策略与文件不变（US1.AC1–3）。

- [x] T010 [P] [US1] 在 `internal/config/retention_test.go` 写严格类型红测：builds正int64、days `1..106751`；0/负/小数/string/null/未知/重复/溢出整块拒绝，默认100/30、空块恢复继承，仓库Document不接受retention。
- [x] T011 [P] [US1] 在 `internal/store/retention_policy_test.go` 写两库真实EffectiveRetention/版本/管理员更新红测，数量200与全局天数45逐字段合并、组迁移ProjectID不变、无权限无半变更或删除。
- [x] T012 [US1] 在 `internal/config/retention.go`、`internal/config/server.go`、`internal/config/project.go` 实现Retention/RetentionOverride及LoadServer/ProjectSettings两个实际严格消费者，保持文件/env/CLI与秘密边界。
- [x] T013 [US1] 在 `internal/store/retention.go` 与 `internal/store/project.go` 实现SyncGlobalRetention/EffectiveRetention及原管理员settings事务；启动持控制独占同步，值变更才增版本/审计，不造后台admin身份或热重载。
- [x] T014 [US1] 在 `internal/server/retention_test.go` 写实际HTTP角色/重复未知字段/null/整块拒绝红测：只有admin可改/看管理结果，trigger/approver/Node拒绝且文件不变。
- [x] T015 [US1] 在 `internal/server/retention.go`、`internal/server/server.go`、`internal/server/http.go` 接有效策略GET、serve受控全局同步和原project set导入，公开固定字段来源与版本，无私有路径/原值泄露。
- [x] T016 [US1] 在 `internal/cli/client/retention_test.go` 写真实CLI未支持红测后，在 `internal/cli/client/retention.go`、`internal/cli/client/root.go`、`internal/cli/client/project.go` 接retention show与原settings导入；只remote加载client，local init/run/doctor/help/version不受管理配置影响。
- [x] T017 [US1] 在 `specs/020-project-retention/validation.md` 记录两库/真实三入口四种继承、逐项非法/三类无权限和全局受控重启证据；全局更改不部分生效，US1只交接不宣称可删除。

## 阶段4：US2 项目候选与保护（P1）

**目标**：真实排序与当前保护只读评估。独立门：跨命名build/项目、OR/等号/并列、可信时间与停止保护、retry祖先和候选后竞争（US2.AC1–5）。

- [x] T018 [US2] 在 `internal/store/retention_candidates_test.go` 写两库固定评估UTC红测：完整有可信TerminalAt按项目 `DESC/ID DESC` 排名，包含无号skipped；rank>N或严格早于D×24h为候选、等号不因该维度选中，活动/旧未知/cleaned不占rank、有可信保护终态仍占rank。
- [x] T019 [US2] 在 `internal/store/retention_protection_test.go` 通过真实Claim/Event/Expire/日志文件与019报告消费者产生活动、stop_unconfirmed、执行/资源/log/artifact/report未确认及未知时间保护；核对固定多reason、文件不变，不手改审批/unknown字段。
- [x] T020 [US2] 在 `internal/store/retention_retry_test.go` 写真实RetryOf parent/child/grandchild两库红测，完整保留child保护全部祖先，越界/循环或闭包超限保守保留，保持RESTRICT。
- [x] T021 [US2] 在 `internal/store/retention.go` 实现EvaluateRetention与具体retentionProtection/稳定排序/OR/有界祖先闭包；闭包最多100000关系/SQL总期限≤5s（含最后CheckLock），不足以证明安全时保retry_dependency，不截断授权，未知状态fail-closed，不加入未消费的未来保护字段。
- [x] T022 [US2] 在 `internal/store/retention_candidates_test.go` 补策略/真实retry/停止及证据确认改变后的重复评估两库回归；候选不是持续删除权，停止确认不刷新TerminalAt或解除其它保护。
- [x] T023 [US2] 在 `internal/server/retention.go`、`internal/server/http.go` 与 `internal/cli/client/retention.go` 接候选GET及retention ls --candidates；默认20/max200、有界offset、明确Candidate/ProtectReasons及[]数组，只有admin可查询管理审计，零文件IO删除。
- [x] T024 [US2] 在 `specs/020-project-retention/validation.md` 记录真实两项目6终态/数量4与组迁移、OR精确集合/时间迁移/retry闭包的双库证据，未知旧时间明确保护，不把只读排序当实际删除验收。

## 阶段5：US3 中央与原节点受限清理（P1）

**目标**：持久意图、真实读锁、隔离删除、当前节点身份与分态恢复。独立门：真实下载/SSE、部分删除/重启/离线、20次确认、越界/身份/无关进程保持（US3.AC1–5）。T025之后首次允许实际删除，必须已有US1/US2可信策略与保护。

- [x] T025 [US3] 在 `internal/store/retention_jobs_test.go` 写两库Schedule/Advance/Authorize/Confirm红测：每build/资源唯一删除身份、ObservedIdentity冲突/固定隔离slot/真实quarantined确认、业务保护不退役、只有reader保护可退役等待、最新政策/控制锁复核及SQL保存失败回滚。
- [x] T026 [US3] 在 `internal/store/retention.go` 实现ScheduleRetention/AdvanceRetention/AuthorizeRetentionObject/ConfirmRetentionObject/ListRetention；仅登记原log/artifact/019purpose及普通/post真实清单，不接任意路径；每轮100候选/100对象、SQL≤5s，IO不入tx。
- [x] T027 [US3] 在 `internal/store/retention_reads_test.go` 写两库Begin/Retire/Activate竞争红测：已active可读完、pending退役后不可输出、End失败保未确认；100次竞争与原运行权失效，不用TTL闭登记。
- [x] T028 [US3] 在 `internal/store/retention.go` 实现RegisterEvidenceReadOwner/BeginEvidenceRead/ActivateEvidenceRead(identity)/EndEvidenceRead具体短事务、同独占Store固定启动owner与失权/同inode身份复核；公开JSON不含StorageID/读取owner私有信息。
- [x] T029 [US3] 在 `internal/server/retention_read_test.go` 写真实二进制/XML/普通post日志/SSE慢流与退役红测：已有fd完整SHA或实际中断，新Begin/Activate/连接/chunk410，SH占用时不能删除，SSE空等待不锁无关文件。
- [x] T030 [US3] 在 `internal/server/files.go`、`internal/server/artifact.go`、`internal/server/log_stream.go` 串行接Begin→非阻塞安全open/SH与identity→Activate→字节→真实fd关闭→End；保留原10m下载/15mSSE预算和权限。
- [x] T031 [US3] 在 `internal/server/retention_test.go` 写实际central quarantine红测：普通目标缺失、权限拒绝、symlink/hardlink/FIFO/父leaf替换/未知StorageID各自结果；对照文件保持，hash/identity重查，无无限阻塞。
- [x] T032 [US3] 在 `internal/server/retention.go` 实现已登记UUID root内对象验证/同inode EX|NB/短事务再授权、事项专有隔离位置登记/rename/再验证/有限unlink/fsync；最多100000项/深度64/30s，失败真实partial/failed/paused，不删新出现陌生inode。
- [x] T033 [US3] 在 `internal/server/retention_test.go` 验证pending读登记、SH已持、退役、隔离、unlink与ConfirmSQL失败各间隙自有控制进程关闭/PG失锁/重启；同inode EX和旧owner失权共同证明后才修复旧登记，第二控制端拒绝，保原JobID/剩余真实对象。
- [x] T034 [US3] 在 `internal/server/retention.go`、`internal/server/server.go` 接原ListenAndServe每60s有界推进一轮与失权/ctx停止；管理POST只一次有限推进，离线不无限等、不运行pipeline/post/upload。
- [x] T035 [US3] 在 `internal/agent/retention_resources_test.go` 写真实checkout/Run结果根资源登记红测：一个attempt固定ResourceID/Ref，workspace槽不变、结果槽只终态前增加，原OwnershipDigest规则、成功journal移除后归属仍在、旧无登记目录不猜授权。
- [x] T036 [US3] 在 `internal/agent/retention_resources.go` 实现data_dir/resources私有普通文件/目录identity登记；摘要仅原Ref与固定槽归属，路径/PID不入wire，未确认journal/spool/签名资源保护。
- [x] T037 [US3] 在 `internal/store/retention_resources_test.go` 写真实两库RegisterNodeResource fence红测，别Node/旧session/旧fence/换workspace或终态扩槽拒绝；结果槽增加保原workspace与单资源身份。
- [x] T038 [US3] 在 `internal/store/retention.go` 与 `internal/server/retention.go` 接RegisterNodeResource/POST agent resources及严格≤32KiB JSON，unknown/duplicate/null/错误身份拒绝，记录具体真实归属不接受路径。
- [x] T039 [US3] 在 `internal/agent/execute.go`、`internal/agent/journal.go`、`internal/agent/recovery_journal.go`、`internal/agent/file_unix.go`、`internal/agent/file_other.go`、`internal/pipeline/run_types.go`、`internal/pipeline/remote.go`、`internal/pipeline/run.go` 串行把资源登记接入真实checkout停止确认后/用户Run前，原ensureResult实际MkdirTemp后ResultCreated登记再intent，最终精确receipt/stop/log确认后固定状态；独立Stop后仅零pending/全部文件ACK才保存并提交固定Completion游标，中央核同Ref资源/真实Stop/游标再确认，原恢复入口只恢复该精确事实。物理Stop本身不能授权中央退役，原唯一Run/process不复制。
- [x] T040 [US3] 在 `internal/store/retention_deletions_test.go` 写两库当前Node Claim/Authorize/Confirm红测：原Node/resource/delete身份、nonce≤5s、Seq正int64及溢出、完整receipt、20次sameSeq/sameDigest复用，冲突/撤销/别Node拒绝、同Node轮换恢复同ID。
- [x] T041 [US3] 在 `internal/store/retention.go` 实现ClaimNodeDeletions/AuthorizeNodeDeletion/ConfirmNodeDeletion，max10事项、当前最新策略/保护/身份复核；首次新Seq绑定原授权，旧已确认Seq先receipt重放，过期只补真实原回执不授新删除。
- [x] T042 [US3] 在 `internal/server/retention_test.go` 写真实独立Node HTTP/严格空authorize请求/确认digest/身份负例红测后，在 `internal/server/retention.go`、`internal/server/json.go` 接三管理路由，固定reason/分态、普通HTTP≤30s，无路径或命令下发。
- [x] T043 [US3] 在 `internal/agent/retention_test.go` 写实际资源锁与删除红测：活动Run/未确认执行或log/未知系统资源拒绝，原PID资料不解释不signal；symlink/hardlink/FIFO/父目录替换/未知对象拒绝，无关自有sleep与邻近文件保持。
- [x] T044 [US3] 在 `internal/agent/retention.go` 实现原data/resource锁下的delete-journal、短管理授权从本次请求发送起点≤5s不因响应延迟重置与单调期限复核、每段≤100项/500ms、再核父leaf/type/identity与停止/log事实、隔离/有限删除/fsync；无shell/Run/process调用、无signal，30s上限或失权停止新unlink。
- [x] T045 [US3] 在 `internal/agent/serve.go`、`internal/agent/retention.go` 接当前独立身份领取/authorize/真实结果persist→confirm，原ID/Seq/Digest恢复丢响应，partial→deleted严格Seq+1，每节点同时一个清理操作。
- [x] T046 [US3] 在 `internal/agent/retention_test.go` 与 `internal/server/retention_test.go` 验证实际离线重连、中央先完成、节点pending、身份轮换/撤销/墓碑、Agent重启/部分隔离/回执丢失与20次确认；只已知原目标确实不存在可成功，绝不转派或假NodeCompletedAt。
- [x] T047 [US3] 在 `internal/cli/client/retention_test.go` 写真实retention run/ls分态/非法limit/非admin红测，在 `internal/cli/client/retention.go` 接run和jobs ls，离线结果立即返回实际状态不无限等，不增加force/local删除入口。
- [x] T048 [US3] 在 `specs/020-project-retention/validation.md` 记录真实二进制/XML/log/SSE并发100次、两库SQL间隙恢复、原节点离线/20确认、每类边界/无关sleep和对照文件大小SHA；中央/节点分别证实，不拿交叉编译替代实机。

## 阶段6：US4 墓碑、编号、幂等与安全审计（P2）

**目标**：完整清理后仍可解释历史身份，旧请求不再执行。独立门：原101清理后旧键仍原关系、新请求102，原视图/410/失败分态审计与后续保护门（US4.AC1–3）。

- [x] T049 [US4] 在 `internal/store/retention_history_test.go` 写两库真正中央+节点完成前不可cleaned、合法无节点not_applicable、child内容先parent后、保最小FK/attempt终态摘要/请求关系红测；不提前剔除未完成历史rank。
- [x] T050 [US4] 在 `internal/store/retention.go` 实现完整分态完成后大正文/steps/中间事件/原文件metadata清理与安全墓碑，保Project.NextNumber/batch/request/RetryOf RESTRICT/必要attempt身份及精确终态摘要/审计，不硬删关系行或修改旧receipt digest。
- [x] T051 [US4] 在 `internal/store/retention_history_test.go` 写真实原触发及已确认retry身份同key同请求重放/冲突键/新触发编号连续/改组与retiring/partial/cleaned新retry拒绝，真实Retire→Retry拒绝及Retry→Retire受祖先保护双库门红测，不新增执行或分号。
- [x] T052 [US4] 在 `internal/store/query.go`、`internal/store/enqueue.go`、`internal/store/retry.go`、`internal/server/trigger.go` 串行接最小history_state/TerminalAt/CleanedAt视图、原batch/已确认retry键最小安全重放、cleaned视图不解已删JSON及新retry仅live、在frozenBuild前及末尾边界复核retiring/partial/cleaned均retention_retired拒绝；原参数、脚本和秘密不公开。
- [x] T053 [US4] 在 `internal/server/retention_test.go` 与 `internal/cli/client/retention_test.go` 验证partial/pending/failed/completed安全查询、正文/log/artifact/report退役410、审计无原日志/params/脚本/StorageID/系统错误及凭据；保原普通证据权限不扩大。
- [x] T054 [US4] 在 `internal/server/retention.go`、`internal/server/http.go`、`internal/cli/client/retention.go` 与 `internal/cli/client/remote.go` 串行完成安全状态/墓碑/固定错误展示、分页20/max200与原查询兼容，410不伪空成功。
- [x] T055 [US4] 在 `specs/020-project-retention/validation.md` 记录真实清理至101、原键重放/102继续、组迁移、3种角色与Node安全拒绝、旧FK和必要审计可查；原完整数据实际不可读而非仅标deleted。
- [x] T056 [US4] 在 `specs/020-project-retention/validation.md` 与 `specs/020-project-retention/quickstart.md` 明确登记后续014审批、010/011真实发布unknown联验门与实际保护函数接入义务；当前不预建状态、不手改DB充通过，停止确认不能解除unknown。此任务只完成联验清单，真正后续证据待其消费者交付，不形成020实施的循环前置。

## 阶段7：全功能联验与收敛

- [x] T057 在 `internal/store/retention_test.go` 与 `internal/server/retention_test.go` 跑同一SQLite/PG政策、排序、保护、读取、失锁/SQL失败、离线恢复、receipt/幂等/最小证据suite及原008/019关联回归，独立DB且保服务生命周期，失败继续修。
- [x] T058 在 `specs/020-project-retention/validation.md` 跑macOS/Linux真实控制端+Agent+客户端 verified HTTPS门，记录固定SHA/UTC/真实PIDbirth/删除ID/原对象与对照字节SHA；缺环境明确待实跑，不拿compile或fake消费代替。
- [x] T059 在 `specs/020-project-retention/validation.md` 记录SQL≤5s、删除≤30s、段≤500ms/100项、授权≤5s、遍历100000/深度64、每轮100及有界续扫公平性（101项目、protected/已有job/failed/pending前缀、持久位置重开与同事务回滚）、离线不spin与控制权/身份失效中止实际负例，检查不碰DB/token/SDK缓存/源码/local结果/未知孤立对象。
- [x] T060 在 `README.md`、`docs/IMPLEMENTATION_HISTORY.md`、`specs/020-project-retention/quickstart.md` 串行更新实际命令/目录/已验收范围与008/019、后续14/10/11独立待联验；005真实Apple门不误写通过，中文记录。
- [x] T061 在 `specs/020-project-retention/validation.md` 完成实际全量 `go test ./...`、必要race/vet/三CLI纯Go跨编译和原本地配置隔离回归；仅非Unix编译不宣称本机删除能力，检查未通过继续修复。
- [x] T062 在 `specs/020-project-retention/validation.md` 执行speckit-converge复核25FR/7SC/16AC与真实当前前置、范围/时间/安全事实；缺口继续implement/converge。当前保护门通过才整功能本地提交、无push，未来真实审批/unknown门保持独立待联验且不能宣称全MVP已验收。

## 依赖与交接顺序

```text
008验收 + 019验收 → T001–T003 → T004–T009
→ US1(T010–T017) → US2(T018–T024)
→ US3(T025–T048) → US4(T049–T056) → T057–T062
```

- T004红→T005实际模型；T006红→T007真实终态；T008红→T009真实锁。Foundation不授权删除。
- US1同包红绿串行；T010与T011不同owner/文件可并行。T012/T013完成且共享文件SHA接入后，才接HTTP/CLI；完整US1可独立验证不删文件。
- US2基于US1真实策略和Foundation真实终态/receipt；T018–T020同A owner串行，T021实现后C接候选只读API，最后交T024。
- US3中央读登记T027→T028、实际fd T029→T030依赖T009；T025→T026事务清单与T031→T032物理删除完成后T033/T034恢复。资源登记只依赖T005真实类型、既有fence与US1鉴权，不依赖US2候选逻辑或尚未建立的删除事项：该无删除前置可与US2并行：B执行T035→T036，root唯一实现T037→T038 Store两新资源文件，C接真实HTTP，root串行T039原执行入口；T025→T026仍必须等待US2独立门与T039完成，才验证/建立真正已登记且可清理的完成构建事项。不得手填nodeResource业务行绕过旧未登记资源保护。T040→T041→T042提供真正管理API，B再T043→T044→T045；T046–T048联验不能使用stub。
- US4必须中央/节点真实完成和最小关联模型已存在；T049→T050→T051→T052→T053→T054→T055。T056是后续联验义务登记，不是假已完成审批/unknown验收、不阻塞014/010/011依赖020当前能力。
- 全功能门T057–T062完成后才converge/本地提交；若实际当前门缺失，不提交功能或标任务done。

## 并行示例

- US1：依赖Foundation后，C的T010配置红测与A的T011两库政策红测可并行；同文件实现串行。
- US2：A完成T021真实方法交接后，C执行T023只读接线，同时A做T022独立文件回归；联验T024最后。
- US3：先B独占T035/T036资源登记新文件与root/C实际T037/T038并行（可与US2独立前置并行），root接T039后再T025/T026；中央读锁与删除按真实消费者随后推进；root分次串行接所有原文件，当前层消费者未到位便等待，不写假的helper。
- US4：A真实T050清理方法交付后，C准备T053显示/410红测；root串行修改既有查询/重放文件，完整编号与FK联验T055最后。

## 覆盖清单

| 需求 | 任务 |
|---|---|
| FR-001 | T010–T013、T017 |
| FR-002 | T011–T013、T017 |
| FR-003 | T010、T014–T017 |
| FR-004 | T018、T021、T024、T049–T050 |
| FR-005 | T018、T021–T024 |
| FR-006 | T006–T007、T018、T022、T024 |
| FR-007 | T019–T022、T025–T030、T043、T056 |
| FR-008 | T019、T022、T043–T045、T056 |
| FR-009 | T022、T025–T026、T031–T034、T040–T045 |
| FR-010 | T025–T026、T029–T030、T035–T039、T049–T050 |
| FR-011 | T025–T033 |
| FR-012 | T027–T030、T033、T048 |
| FR-013 | T031–T033、T048、T059 |
| FR-014 | T035–T042、T045–T046 |
| FR-015 | T035–T036、T043–T048、T059 |
| FR-016 | T040–T042、T045–T048 |
| FR-017 | T040–T042、T045–T048 |
| FR-018 | T025–T026、T033–T034、T044–T048 |
| FR-019 | T049–T052、T055 |
| FR-020 | T023、T047、T052–T055 |
| FR-021 | T014–T017、T023、T038、T040–T042、T047、T053–T054 |
| FR-022 | T023、T026、T032–T034、T041–T045、T059 |
| FR-023 | T031–T034、T043–T044、T059 |
| FR-024 | T001–T002、T056–T058、T060、T062 |
| FR-025 | T004–T061的各真实两库/两平台门、T057–T058、T061 |
| SC-001 | T010–T017 |
| SC-002 | T018、T021–T024 |
| SC-003 | T019–T022、T025–T033、T040–T048；未来实际门T056仅登记 |
| SC-004 | T027–T033、T048 |
| SC-005 | T033、T040–T042、T045–T048 |
| SC-006 | T031–T033、T043–T048、T049–T055、T059 |
| SC-007 | T001–T003、T056–T062；后续14/10/11真实门独立待联验 |

| 验收场景 | 任务 |
|---|---|
| US1.AC1 / AC2 / AC3 | T010–T013/T017；T010/T014/T017；T014–T017 |
| US2.AC1 / AC2 | T018/T021/T024；T018/T021/T024 |
| US2.AC3 / AC4 / AC5 | T019–T022/T025–T033/T056；T022/T025/T032/T041；T006–T007/T018/T024 |
| US3.AC1 / AC2 | T027–T033/T048；T025–T026/T032–T034/T048 |
| US3.AC3 / AC4 / AC5 | T040–T042/T045–T048；T031–T033/T040–T044/T048；T035–T039/T043–T046 |
| US4.AC1 / AC2 / AC3 | T049–T052/T055；T047/T049–T055；T056登记，后续真实14/10/11消费者联验 |

## 实施策略与当前状态

先交US1管理与US2只读评估，可独立演示却不能声称清理功能已完成；随后中央读锁/有限物理删除与原节点真实管理事项并行逐批交接，最后墓碑、幂等和全平台门。四US全部当前行为门通过才提交020，未来审批/unknown必须按真实消费者另验，未完成时全MVP仍待验收。

2026-10-05：实际setup-tasks使用 `.specify/templates/tasks-template.md`，before_tasks/after_tasks检查extensions hooks={}，无待执行hook。62任务全部未勾选；当前只是规划，未改源码/依赖/共享入口、未注册服务或提交。

正式基线校正：T021祖先闭包总期限≤5s（含末尾CheckLock），数量/期限不充分时保护；T019/T022覆盖零动作ReportRev0合法、interrupted精确独立stop而非强求旧steps全改；T025/T032/T033覆盖ObservedIdentity/固定隔离slot与真实quarantined确认；T050–052保008精确Ref/receipt及cleaned安全投影/原retry key重放。此为既有需求具体化，不添加未来保护占位或缩FR。

2026-10-05 实施校正：只读analyze发现US3登记被错误依赖T026删除事项，形成真实消费者循环；在分析之外修正为US2→并行T035–038→root T039→T025–026。62个ID/32要求/16AC不变，阶段内按显式依赖执行而非数字顺序。当前T001–017完成，后续保持未验收。

登记前置分区校正：T035–039不删除、不读取候选，无US2数据依赖，基于Foundation+US1可与US2并行；T025起删除门仍严格等待US2与T039。root专属新增Store retention_resources.go/test，A继续候选/保护/闭包独占，其它职责不变。
