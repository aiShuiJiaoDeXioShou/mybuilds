# 014 Phase0研究与决策

## 实际工作流

独立approval014-planning基线85b46bf，仅spec/checklist原own untracked，原SHA ba3b99386c730abade5c5e7f2dc20f56de2258c7c116ab71d863034829e40859/c021d46176991cf948bca3f9974d5cb7b6b230a4af7a9c07553c7cdec6d912fd核对保存，未修改。正式读本区README/AGENTS、spec/quality、plan技能、constitution2.1、CONFIGURATION/MULTI_NODE/INTERFACES/路线；实际setup-plan --json复制模板，resolve plan-template成功，prereq require-spec成功，selector14/hooks={}。根008/019/010/011及020冻结文档只读作为候选，未复制未验收源码。所有设计疑点已决策，无待澄清项。

## D1 唯一Run保存cursor，不建立第二executor

**Decision**：RunOptions.Resume只由批准后新Task消费者填具体ApprovalResume，字段是已完成StepRun/普通next index/原result及manifest/remainingNS/sealedReports；普通/收尾循环和原executeStep/executePost/资源helper唯一保留。初次到approval先从普通循环退出、不post，再Close/flush、提交checkpoint，返回ErrApprovalPaused+暂停结果。续段同函数验证原state并从next index进入；旧Run不加Resume仍原语义。
**Rationale**：实际Run当前whole loop、p.build整批准备、remote确保result/末尾terminal；直接再Run原definition会重跑或重建reports。须在原prepare/executeBuild/remote初始化处最小接cursor，校验冻结旧steps但不准备其env/工具/材料、更不发旧事件；执行后续时继续原累计seq/log/artifact cursors、post尚未选择。
**Alternatives**：另一个ResumeExecutor/第二step循环、裁剪Definition再Run、把旧steps改when=false会丢名字/索引/条件/历史与证据归属，均拒。

## D2 checkpoint只有真实停止/清理/中央证据后可安全挂起

**Decision**：先封存019原报告及全部已确认artifact/log；Run处理真正资源Close/本次子进程停止，日志close刷尾；私有journal保存ledger/目录identity/全部确认，再停renew等待当前请求、复核原Authority，以新Kind=approval_checkpoint的同ApplyEvent提交。Store完全核对步cursor/NS/fullmanifest/clean/StopConfirmed，才waiting_approval、不再续lease并释放node/global容量，保同名/原node/workspace。
**Rationale**：签名Close不能由用户post替代，whole-Run defer在当前executePost之后必须为pause增加不运行post的具体分支。checkpoint创建前不先展示waiting或清容量；文件IO/parse在DB外，中央摘要等短事务核对。准备/封存/Close到ready耗时计普通NS，pending ACK阶段无新动作且仍原Authority限；不会因丢ACK扩大期限。
**Alternatives**：用approval intent或Node声明idle释放槽、用StopConfirmation当完整checkpoint、先释放再补文件都会伪可审批；资源未知不安全。

## D3 不可变旧Ref与current-only resume

**Decision**：approvalRecord自身持CheckpointRef全旧6字段、Seq/Digest/ApprovalID/Revision/CheckpointDigest及完整ledger，关联原receipt.Kind=approval_checkpoint；不会靠未来会更新的attempt.Ref倒推旧归属。Resume同build/attempt，Claim更新当前session/credential/lease及epoch++，保持计数/原steps/logs/artifacts/report来源不变、全attempt序号继续。Task.Resume含原proof与next cursor。
**Rationale**：实际007 attempt是一build一个且当前Ref字段会改变，receipt只有attempt/seq/digest（008补Kind/StopKnown）。必须有已提交immutable checkpoint自己保旧Ref。旧checkpoints不是terminal，008TerminalReceipt仍只最新完整build_finished；approved/cancelled时独立ReadApprovalCheckpoint仍能证明旧提交但不授运行。新lease权不因旧proof读取恢复。
**Alternatives**：只receipt.Kind或每次approval创建新attempt会丢旧关联/反演证据，重复复用oldLease会接受迟到writes；新pipeline执行器不可取。

## D4 checkpoint ACK丢与安全新进程

**Decision**：本地phase checkpoint_pending保存准确PendingEvent/全私有proof；新进程持data锁、严格受限读取后，以当前node身份精确只读核对CheckpointRef/ID/revision/seq/digest/manifest。仅中央真已commit和本地SameFile/whole digest完全一致、StopConfirmed/noCleanup时fsync转paused，不删除workspace/spool/results或猜恢复。unknown仍阻止session/claim；approved/cancelled查询同样只有旧checkpoint证明，取消可保存retired安全状态无动作。
**Rationale**：008清journal只合法终态，不能把新的pause套入TerminalReceipt；当前token rotate允许同node核对自有，跨node禁。旧process任务bookkeeping尚未退出时Agent不重复Claim/Resume；pause ACK/readonly确认→fsync→原worker结束→active map释放→新Claim顺序。
**Alternatives**：旧PendingEvent重POST、删unknownjournal、只中央state而忽略本地证据均违反不接管未知执行；没有oldPID signal。

## D5 单次决定、同名保护与恢复复核

**Decision**：approve/reject保现有build定位CLI/HTTP，但输入必须ApprovalID/Revision/CheckpointDigest；无隐藏GET latest自动补字段。record immutable decision identity/time/note，重复相同决定原结果，不同意见/过时ID冲突。批准不授lease；Claim沿当前node健康/未drain/授权/能力/容量与原exact node，仅ready paused本地journal可继续，缺工作区/文件安全failed而非Checkout。
**Rationale**：迟到buildID决定不能误作用第二approval。等待释放容量但同名占位；待审批/approved拒node删除，disable/rotate不把安全旧pause变成在跑进程；当前授权恢复必须重新核验，等待时原预算不扣。
**Alternatives**：等批准直接Run/counter++、迁移node/重新检出、TTL自动审批或单buildID批量approve皆无产品授权。

## D6 budgets、resources、reports、publish

**Decision**：Remote/local同具体active ledger；封存/Close/restore验证/实际action耗时减原NS，等待不减、不重置。普通0明确拒启动，post budget原值未选择不执行。old已Close的签名资源不复建以“证明原身份”；发布段续执行只取原snapshot artifact，不Prepare。无upload的approval后ordinary run才需要真实重新验证/准备本次材料，新资源只服务后续命令，Close独立有限。019旧ReportSeal不重开；后续普通步骤不得悄改旧sealed放行证据，复核变化安全失败，不新建report epoch。
**Rationale**：已有发布段结构保证run/artifact全部在approval/upload前；非发布审批可在普通序列中间。原artifact declarations、Source run字段、LogRecords保持旧Ref，不改为新Ref或重传；新progress仅当前Ref携旧+新完整manifest，Store按同attempt稳定seq/IDs与source核对。010AuthorizePublish另查精确本构建、此step之前全部approval条件；skipped任何approval不是已批准，不豁免admin/allow/appguard/JUnit。
**Alternatives**：resettimeout、重签重建产物、复制旧消息newRef、把approval当unknown解除均改变事实。未知publish应用保护不由审批/StopKnown清除。

## D7 本地TTY与无通知

**Decision**：本地Run唯一具体ConfirmApproval callback由CLI实际TTY消费者提供，stdin实际fd TTY检查、x/sys有限poll+ctx、≤256输入，不起弃置read goroutine；nil/noTTY/EOF/拒绝/ctx取消不放行、无后续/post。等待暂时停active预算timer，真实prepare/seal/cleanup计费，确认后重新从剩余NS计；上层用户ctx始终可取消。prompt只安全build/step/原证据IDs与摘要，输入/note不原样日志。
**Rationale**：Go stdlib无通用TTY判定但项目已用x/sys，无需新依赖。local不连server/不读取坏client配置，不能生成远程approval凭据；local有效upload先预检拒。notify=false默认；true通知模块未交付先unsupported，dryrun不读取TTY或等待。
**Alternatives**：--yes自动批准、非TTY默认继续、stdin goroutine长期泄漏、用户post代清理不符合规范。

## 实施/验收边界

这都是设计候选，无真实pause/Resume代码或测试PASS。正式008/010/011与019已交付后再对最终模型/receipt/Artifact/JUnit/资源类型核对。014自身真实商店放行、checkpoint/取消/TTY/双库/两平台门必须过；012/custom与020 protection联验是后续整MVP门，不反向依赖形成循环。不得把平台/签名材料缺失降为模拟验收。


## 当前实施修订（2026-10-05）

本功能已获准在fresh2602094+Root实际共享baseline完整实施，不再停留007规划基线。签名资源消费005冻结真实组件，发布消费Root/B当前实现；合法Apple/商店实际上传与最终Flutter案例人工待验，不能用自产签名或脚本成功冒充。必要自动门按安全/事务/恢复/权限/容量/预算/原文件/TTY实际路径验证，不因缺材料留下空实现。当前A独占本WT，Root原event/reports和最后发布接线独占。


### 真实报告检查点边界

含upload的审批属于发布段：全部普通run/artifact完成、报告passed且final/sealed后才能安全挂起，Root reportsFinal门不放宽。没有upload的midrun审批可位于后续run之前（CONFIGURATION:507、FR001/002/014/017）：仅持久已checked的同一revision、原XML快照与具体collection状态，不虚称final/sealed；Resume继续同一collection/原baseline/当前entries，后续实际run再检查，全部ordinary结束才final seal。没有第二report epoch、重采旧XML或重新执行脚本。私有报告collection记录只Agent，中央核当前checked证据/已确认XML/cursors；变化/缺失闭锁。
