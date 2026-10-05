# 014 精确checkpoint与新lease续执行协议

本页仅候选，root唯一wire writer；JSON snake_case、optional additions omitempty、数组explicit[]、body≤64KiB，记录ledger≤1MiB。旧无approval消息不改digest；私有paths/files json:"-"。沿原ExecutionEvent、完整LeaseRef、EventSeq/Digest与中央manifest消费者，不第二执行协议框架。

## ApplyEvent新kind

ExecutionProgress.Kind=approval_checkpoint，Phase=ordinary、Index=原approval索引、Name=原step、StepKind=approval；新增Approval *ApprovalCheckpointEvidence。证据包含ID(UUID)、Revision(正数)、SnapshotDigest、WorkspaceID/ResultID(UUID)、NextOrdinaryIndex(Index+1)、CheckpointDigest、Artifacts []ArtifactExpectation、ReportManifest（019原真实seal）、PublishIntents（010既有完整confirmed或unknown真实集合）、SystemResourcesClosed bool；RemainingBudgetNS与PostBudget/LastLog/LastArtifact沿progress原字段，不平行计时。

true StopConfirmed、false CleanupFailed、PostPhase=none；approval无子进程，不造Started/PID/PGID。之前普通步骤均真实成功或冻结skipped、无pending/intent/unknown/失败；后续尚pending。manifest全数/ID集合及旧Source一致，每个file/hash已中央确认，ReportSeal未改，日志无未确认spool。CheckpointDigest对该有限完整证据+旧Ref+seq+budget/cursors规范hash（不包含自身），EventDigest沿原Progress含CheckpointDigest计算；两个职责明确，存DB不可后补或body任意opaque。

Store currentExecution原expires/fence/控制锁在commit前最后再验；短事务创建approvalRecord、同seq receipt.Kind=approval_checkpoint、保存freeze预算/cursors，Status waiting_approval及CurrentApprovalID，旧lease标不续执行权。EventAck仅原seq/digest，不是resume grant。Agent发送前stopRenew join原当前request、复核Authority/data锁，localPrepared已fsync；只一次checkpoint POST。响应丢失保PendingEvent+localpaused proof，不旧fence重发/假knownterminal。

## 独立只读回执

```go
type ApprovalCheckpointLookup struct {
    Ref LeaseRef
    ApprovalID string
    Revision, Seq int64
    Digest, CheckpointDigest string
}
type ApprovalCheckpointReceipt struct {
    Ref LeaseRef
    ApprovalID string
    Revision, Seq int64
    Digest, CheckpointDigest, State, NodeName string
    StopConfirmed, SystemResourcesClosed bool
}
```

POST /api/agent/approval-checkpoint 当前NodeActor，同NodeID/当前未revoked/禁用规则，严格读取approvalRecord不可变OldRef+seq/digest/checkpoint_hash及原receipt.Kind，不是检查旧Ref仍有lease。State pending/approved/rejected/cancelled/resumed只说明checkpoint历史/当前state，不授运行。approved/cancelled乃至resumed仍只证明原commit；若另当前resume未知，本地不能据此清resume_claim_pending或运行。

读取网络后Agent复核原localPending event/ledger/manifest/stop/noCleanup/目录资料和SameFile/wholefiledigest，只fsync转paused_confirmed（不清journal/删spool/results），再恢复Doctor/session流程；unknown仍拒。此lookup不Renew/ApplyEvent/StopConfirmation，不改旧receipt；不能用008TerminalReceipt证明checkpoint，不靠用户build status猜中央commit。

## Resume Grant

Claim现有NodeActor/session/ClaimKey入口保留。仅approved当前审批、原Node健康enabled/不drain/capacity/项目范围/工具/当前身份满足时给grant；TaskSnapshot新增Resume *ApprovalResumeEvidence omitempty，含ApprovalID/Revision/CheckpointDigest/CheckpointRef、SnapshotDigest、NextOrdinaryIndex、原StepProgress ledger、EventSeq/LastLog/LastArtifact完整manifest及sealedReports。StepRun私有paths由本地journal恢复，网络没有路径。waiting原node离线只显示固定reason，不迁移。

Ref为同BuildID/AttemptID/NodeID、新SessionID/LeaseID且Epoch=oldCurrent+1，nullable RemainingBudgetNS与原PostNS精确carry，不newnumber/checkout/source。Claim回复原key幂等，但原requested+TTL-margin deadline仍保守；迟grant不可accept，未知领取不得newprocess接管。旧node/session/epoch全部event/log/artifact/publish写拒。

原approval完成状态由精确approved Claim事务核对决定并标本step succeeded，Intent来自原checkpoint且Started=false/StopConfirmed=true（审批非子进程真实事实）。没有伪started事件；新Run只开始NextOrdinaryIndex。seq/cursors不reset，过去消息Ref/body/digest不改，当前terminal携全旧+新manifest按stable attempt关系核对。原ArtifactDeclaration Ref只历史证明，不重发oldPUT；后续新增file新Ref/Seq。

## 决定与约束

Decision输入ApprovalID+Revision+CheckpointDigest、Decision approve/reject和Note（≤1024UTF8、控制字符/明确secret引用或token字段格式拒），目标build路由须相同关联，用户当前approver/admin。approval ID每个冻结step独立，Revision本build正递增不可变；无自动GET latest填充。same内容重复原结果，不同Note/decision或旧current record409，无静默覆盖/重复审计。

安全paused reject/cancel终态可事务写原StopConfirmed事实与服务端时间（供020后续）；无需fake build_finished/node旧event。008TerminalReceipt对于此类server安全终态没有完整node build_finished仍拒、独立checkpoint lookup只paused证明。旧node继续执行或回报一律拒；unknown领取未安全暂停不能套cancel分支清guard。


## 当前实施修订（2026-10-05）

本功能已获准在fresh2602094+Root实际共享baseline完整实施，不再停留007规划基线。签名资源消费005冻结真实组件，发布消费Root/B当前实现；合法Apple/商店实际上传与最终Flutter案例人工待验，不能用自产签名或脚本成功冒充。必要自动门按安全/事务/恢复/权限/容量/预算/原文件/TTY实际路径验证，不因缺材料留下空实现。当前A独占本WT，Root原event/reports和最后发布接线独占。


### 真实报告检查点边界

含upload的审批属于发布段：全部普通run/artifact完成、报告passed且final/sealed后才能安全挂起，Root reportsFinal门不放宽。没有upload的midrun审批可位于后续run之前（CONFIGURATION:507、FR001/002/014/017）：仅持久已checked的同一revision、原XML快照与具体collection状态，不虚称final/sealed；Resume继续同一collection/原baseline/当前entries，后续实际run再检查，全部ordinary结束才final seal。没有第二report epoch、重采旧XML或重新执行脚本。私有报告collection记录只Agent，中央核当前checked证据/已确认XML/cursors；变化/缺失闭锁。
