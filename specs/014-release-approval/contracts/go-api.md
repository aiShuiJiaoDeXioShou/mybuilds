# 014 具体Go接入契约

当前实施基线2602094+Root逐SHA共享baseline，真实类型直接复用当前008/019/020及发布消费者；商店/Apple实际材料人工待验。沿现有Run/Agent/Store，以下是实际consumer所需增量，不另建repository、执行器或通用checkpoint接口。JSON snake_case；私有paths/material不序列化；旧可选字段omitempty保持旧消息摘要。

## 唯一Run

```go
// 保持现有签名与选择/参数规则。
func Run(ctx context.Context, doc *config.Document, opts RunOptions) (*RunResult, error)

// RunOptions增量，只有Agent提供Resume；本地CLI提供ConfirmApproval。
Resume *ApprovalResume
ConfirmApproval func(context.Context, ApprovalPrompt) (bool, error)

var ErrApprovalPaused error // 固定sentinel，必须errors.Is；不是普通失败或终态。

type ApprovalPrompt struct {
    Build, Step string
    Index int
}
type ApprovalResume struct {
    Evidence protocol.ApprovalResumeEvidence
    Completed []StepRun
    ResultDir string `json:"-"`
    // 本地已有snapshot身份/manifest与原workspace证据；不得网络提供路径。
    Artifacts []ArtifactRecord `json:"-"`
    // 019具体已封存证据，使用其交付类型，不创建重复报告结构。
}
```

RunResult追加Paused *ApprovalPause omitempty；ApprovalPause携checkpoint evidence/原ledger/真实剩余NS，私有resultDir/workspace证据仅Agent保存。其wire类型与checkpoint-protocol一致，实际声明由root结合019类型串行落地；不提前用interface{}或opaque JSON占位。Resume.Evidence必须精确匹配同frozen Definition、原审批、顺序/预算/manifest及本地证据，nil保持原Run行为。

实现是在run.go现有prepare/build循环增加具体恢复分支，复用executeStep/executePost/resources/log/collector；不复制步骤循环、切割Definition再重新Run或另起worker。completed StepRun原name/status/elapsed/LogPath/artifact原metadata保持；不解析旧env/secret、重新校验旧run command的目录、重新发旧started或重置budget/resultRoot。当前权限/原证据严格复核由Agent与当前Store grant共同提供，只有未来有效step准备新动作。

remote在approval普通step：前序report seal/产物和log全部确认、flush/Close及进程停止成功、剩余预算冻结，本地checkpoint先持久，再沿已有RemoteOptions.Progress提交approval_checkpoint。Agent将stopRenew/join/fence验证绑定这一个实际Progress消费者，不能新增不被调用的OnPause接口。Run返回ErrApprovalPaused+非terminal result，无post、无build_finished；Close/log失败沿原错误/authority闭锁，不返回安全Paused。原命令失败不能被审批清理错误覆盖。

本地ConfirmApproval只在真实生效approval调用；nil/noTTY固定approval_requires_tty，false拒绝approval_rejected，EOF/ctx按固定原因，不运行post。输入等待暂停active预算计时但保留user ctx；确认后的校验/执行继续原剩余NS，不能重设timeout。CLI用现有x/sys的真实FD/TTY/poll，≤256输入、context可打断，不弃置reader goroutine。dry-run不调用此函数；有生效upload/notify的整批本地预检查先拒绝。

## Store真实消费者

```go
type ApprovalFilter struct { ProjectID, State string; Page Page }
type ApprovalDecision struct {
    ApprovalID string
    Revision int64
    CheckpointDigest string
    Decision, Note string
}
func (s *Store) ListApprovals(context.Context, Actor, ApprovalFilter) ([]ApprovalView, error)
func (s *Store) GetApproval(context.Context, Actor, string) (ApprovalView, error)
func (s *Store) DecideApproval(context.Context, Actor, string, ApprovalDecision) (ApprovalView, error)
func (s *Store) ReadApprovalCheckpoint(context.Context, NodeActor, protocol.ApprovalCheckpointLookup) (protocol.ApprovalCheckpointReceipt, error)
```

DecideApproval第三参是buildID，必须等record.BuildID；Actor当前角色/admin或approver、ID/revision/hash/state与同内容重复在同一短write事务复核。Note持久前有限UTF8/control/明确secret引用或token字段式格式检查；不读取或传输Node实际secret，公共View只含原意见SHA-256摘要，原Note仅私有DB审计；安全view不含Definition/Params/command/私有路径/token。列表limit1..100/offset0..1000000沿现有Page，不新分页框架。

现有ApplyEvent消费approval_checkpoint并调用具体私有applyApprovalCheckpoint；现有Claim批准分支返回Task.Resume，不另建Resume/queue API。Claim只原node，epoch++同attempt，当前identity/health/tool/allowed scope/capacity与旧checkpoint证明分别验证；nullable NS保持。现有Stop安全paused取消同事务CAS，恢复已Claim则走原真实stop。node delete/same-name guard/capacity/status/query/recovery消费者必须增加waiting_approval/approved分支，不能仅新增表而调度仍当terminal。

executionReceipt只增加明确approval_checkpoint Kind；旧007未知Kind仍不猜。approvalRecord完整OldRef独立于current attempt.Ref，存seq/digest/manifest与original credential/session事实；新epoch不重写旧receipt、文件declaration/source或LogRecord。ApplyEvent新terminal携旧+新完整manifest：校验旧Source原Ref与新Source当前Ref的实际已确认归属，不要求全attempt文件来源等当前session，不重发旧HTTP PUT。

Recover验证不可变checkpoint/receipt、原step ledger与预算/完整manifest，然后：waiting/approved保安全pause不授权；resumed-running另验当前attempt/node/session/credential/epoch一致，旧proof不能替代新Authority。旧TerminalReceipt保持Kind=build_finished/current Ref/seq/digest/完整清理；旧pause即使已cancelled不放行008journal单unlink。ReadApprovalCheckpoint仅当前同node身份只读原proof，approved/cancelled也不给lease。

008Retry兼容：安全checkpoint后server reject/cancel可无node build_finished。retryStopped只能在中央已确认原checkpoint、terminal row未进入新epoch且未resumed、StopKnown/Closed/完整manifest真实匹配时认安全停止；不能造build_finished收据或靠terminal状态/普通pending推断。已恢复后未知checkout/stop则仍拒，原StopConfirmation必须精确当前fullRef，旧pause证明不能确认新进程已停。

## Agent具体消费者

internal/agent/approval.go保存/读取有限具体paused ledger，Serve/execute使用同Run。普通任务无Resume继续原Checkout；Resume只原workspace SameFile/固定SHA/封存bytes验证，缺失或篡改安全失败，不重新checkout/rebuild/下载替代。原node新current session只在paused_confirmed与旧task bookkeeping结束后可Claim。

checkpoint_pending先完整fsync原PendingEvent/ledger/stop/cleanup/资源关闭/文件manifest；旧renew worker退出，原Authority有效内提交一次；ACK丢失只ReadApprovalCheckpoint精确读取，并在network后再SameFile+whole-file hash核对/fsync转paused_confirmed。unknown任何组合拒Serve/claim；不用008terminal规则删除pause journal。resumed/cancelled lookup仅证原pause历史，不能清另未知resume grant或授动作。读取当前token同NodeID、禁用/撤销拒，原node范围不扩大。

Resume校验/本次真正后续ordinary的签名Prepare耗时计原NS。发布段恢复只原artifact，不再Prepare已Close签名资源；后续确需签名run时准备新的本次resources，独立有限Close，不修改原seal。019封存report不可重开或以post/newbaseline替换；当前store-app/node授权由010/011真实Publish consumer再验，批准不代替unknown处置。

## 当前唯一writer

A在approval014-current实现全部014增量；Root独占原Store event.go/reports.go及最终串行合并，A提供已编译真实helper与最小dispatch patch，不写stub。B独立012发布沿当前PublishAuthorization，只加Custom自身字段；审批批准只由同Store私有validatePublishApprovals(tx,row,index,in)查询真实approval记录，不新增Node可自行填的approved proof。Root最终串行接到三种发布消费者同一AuthorizePublish事务。


### 真实报告检查点边界

含upload的审批属于发布段：全部普通run/artifact完成、报告passed且final/sealed后才能安全挂起，Root reportsFinal门不放宽。没有upload的midrun审批可位于后续run之前（CONFIGURATION:507、FR001/002/014/017）：仅持久已checked的同一revision、原XML快照与具体collection状态，不虚称final/sealed；Resume继续同一collection/原baseline/当前entries，后续实际run再检查，全部ordinary结束才final seal。没有第二report epoch、重采旧XML或重新执行脚本。私有报告collection记录只Agent，中央核当前checked证据/已确认XML/cursors；变化/缺失闭锁。

### 最终具体串行消费者

`validatePublishApprovals(tx *gorm.DB,row buildRecord,index int,in protocol.PublishAuthorization) error` 在AuthorizePublish的真实短事务、publishReports之前调用，检查此前全部approval（skipped也不是批准）；无approval原语义不变。`approvalHistoricalRef(tx *gorm.DB,row buildRecord,ref protocol.LeaseRef)(bool,error)` 只读原证据时验证真实审批续执行链，不授旧写权限。FindNodePublish还核原持久Grant.Ref==请求Ref与原授权digest，Agent保原Grant/Receipt Ref，仅经approvalRefKnown证明历史归属。

公共ApprovalView只返回 `NoteDigest string json:"note_digest,omitempty"`，原Note私有DB审计；不读取或传输Node实际secret。输入UTF8/1024/control与明确材料引用/token字段格式检查，可精确重放原Note；不声称能识别中央未收到的秘密值。私有ApprovalLocalCheckpoint.IOSteamID与原已关闭IOSResourceOwnership.TeamID/IOSResourceDigest精确绑定；公共wire只含已有资源摘要/中性ID。发布段恢复此真值不重新Prepare；需要后续签名run才在已批准关闭proof下新Prepare并更新真实资源team。
