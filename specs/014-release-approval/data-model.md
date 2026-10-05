# 014 状态、检查点与持久约束

## 审批记录

approvalRecord：UUID ID，BuildID/AttemptID/NodeID FK RESTRICT，OrdinaryIndex与StepName，Revision（本build审批序数、正数、不可变），CheckpointRefJSON完整旧6字段、EventSeq/EventDigest、CheckpointDigest、SnapshotDigest、WorkspaceID/ResultID中性UUID、NextOrdinaryIndex、RemainingBudgetNS nullable、RemainingPostBudgetNS、LastLogSeq/Offset/LastArtifactSeq、完整ArtifactSteps/报告SealManifest/既有PublishIntents/普通步骤ledgerJSON、StopConfirmed/CleanupFailed/SystemResourcesClosed、CreatedAt；Status pending/approved/rejected/cancelled/resumed。unique(build_id,index)、unique(build_id,revision)、原EventSeq/Digest绑定原execution_receipt。私有workspace路径/目录inode只Agent，不DB或HTTP。

decision只记录一次Decision(approve/reject)、ActorID FK、DecidedAt服务端UTC、Note与DecisionDigest（canonical ID/revision/checkpoint_digest/decision/note，不依最新build状态猜）。审批判断在短write事务，admin/approver当前角色再次核对；重复精确同内容返回原决定，不重复审计，冲突或旧record不是current拒。ApprovalID/Revision/CheckpointDigest必须请求显式给出。

build.Status增加waiting_approval/approved，CurrentApprovalID nullable、ResumeReason有限code；running/terminal原状态保留。waiting/approved的ref不是可续执行权，保不可变checkpoint OldRef，LeaseExpiresAt仍历史值但ExpireLeases不将安全pause改interrupted。pending仅在完整checkpoint事务commit后存在，没有checkpointing伪等候状态。StopUnconfirmed/CleanupFailed时保持running至原expiry interrupted，不能进入待审批。

## 原归属与收据

checkpoint.ApplyEvent提交同attempt原seq/digest、Kind=approval_checkpoint、完整manifest，receipt.StopKnown只能表示本次暂停的真实物理停止，008TerminalReceipt仍必须Kind=build_finished而不会接受pause。approvalRecord存原CheckpointRef，不靠attempt未来更新倒推；关联的旧receipt及record immutable digest证明旧commit，不能随后补造原seq/body。

approved Claim只original NodeID，重新验证current session/credential、node enabled/health/tools/capacity、项目范围/分支/发布权与原AllowedNodes交集，AppBinding/JUnit另在Publish再核。保持BuildID/AttemptID/Number/初始预算/Definition/Params/Facts/SHA/旧steps/evidence；当前attempt/build更新SessionID/CredentialID/LeaseID/epoch++/Expires，receipt和checkpoint OldRef不变。新ClaimKey/session记录精确去重/原request期限，不允许newprocess接管未知领取。

所有事件/日志/文件/Publish用新current Ref，旧ref拒。原confirmed ArtifactDeclaration/LogRecord/ReportFile.Source及IDs/seq/hash不重写、不重新上传；新文件继续同attempt LastArtifactSeq+1，新log/event同原cursor递增。新terminal的当前Ref携全旧+新manifest，Store查原stable归属和counts，不从新session猜旧文件来源。旧最终build_finished只原TerminalReceipt精确核对；old pause查询走独立ReadApprovalCheckpoint，不认latest row.LastEventSeq为旧checkpoint proof。

## 状态与保护

| 当前 | 行为/条件 | 下一个 | 容量/保护 |
|---|---|---|---|
| running | 证据全ACK、真stop/Close/flush、original fence commit | waiting_approval | node/global执行容量释放；同名、原node/workspace/证据保留 |
| running | save/stop/Close/checkpoint未知 | running→expiry interrupted | 不假pending；stopunknown隔离/同名/容量按007保持 |
| waiting_approval | 精确approve当前record | approved | 仍不占执行容量、只原node等待，无新lease |
| waiting_approval | reject当前record | cancelled/approval_rejected | 真已安全pause，无post，历史保留；独立publishunknown仍保护 |
| waiting_approval/approved | admin cancel且未新lease领取 | cancelled/cancelled | 无post，不复活；checkpoint记录保留 |
| approved | 原node合法Claim+CAS | running，epoch++ | 使用原剩余budget占容量，checkpoint marked resumed |
| running续段 | 正常原Run剩余范围 | 另一waiting或原terminal | 原manifest/ledger与全部决定不改 |

decide/claim/cancel行锁与CurrentApprovalID/CAS同事务。取消与claim谁先commit决定是否按安全pause直接终态或按新running真实停止，迟到approve不能复活。禁止同名新build越过waiting/approved；不同名可容量执行。node delete有安全pause/approved也拒；drain不领resume，disable取消活动权但不造在跑pause进程。Rotate后同node当前身份可读取旧pause，newsession必须真实合法节点报告。

## 可恢复本地证据

现有journal增加optional ApprovalCheckpointState（只实际pause出现），含原CheckpointRef/ID/Revision/Seq/EventDigest/CheckpointDigest、central确认状态、原ClaimKey、WorkspaceID/ResultID与私有路径、目录dev/inode身份、StepRun历史/next index/remaining NS/原manifest、封存文件snapshot身份/hash。本地phase checkpoint_pending→paused_confirmed→resume_claim_pending→running；cancelled中央proof可标paused_retired，绝不自动Run/用户post或删除未知材料。

data锁与原128条/1MiB、owner0600/regular/单link/NOFOLLOW/NONBLOCK/严格duplicate/null/unknown限制保持。中央当前NodeActor只读lookup精确旧checkpoint，approved/cancelled仍可返回已提交proof；网络后重新read SameFile+whole JSON digest，再fsync本地转换。不满足任一步仍unknown拒Serve/claim，旧PID不signal；008UnknownTerminal机制不能误删pause。paused合法旧journal只降低“有证据未确认”的阻塞，不授动作；newlease Grant才允许Resume。

Agent先完成旧task shutdown/renew join/本地fsync并从active bookkeeping移除，之后才能再claim。原进程收到commit ACK前Server可能已有approved但该data锁消费者不会抢领自己尚未结束的任务；另进程不能同data_dir取锁。恢复新Claim响应未知仍原session/key/request deadline，跨新process保未知不接管。

## Run游标与复核

Resume是具体被验证ledger：精确ApprovalID/Revision/CheckpointDigest、OriginalRef、LastEventSeq/LastLog/LastArtifact、next ordinary idx、过去StepRun、remainingNS、resultRoot/原artifact私有paths/SealedReports。Agent不重新Checkout；打开同属WorkspaceID的原directory，复核dev/inode/权限/root、真实固定Git SHA/原Definition摘要及sealed artifact/report实际bytes；缺失/变化失败，不reset/rebuild/download中央证据替代原workspace。

Run只校验过去步骤模型与manifest，不重新解析其secret/启动工具、发旧事件或改旧Source/elapsed；只准备后续有效steps的实际当前目录/env/material/能力。remote初始化从原ledger设置Started历史/PostPhase未选择/seq预算，不再build_started、不建新报告baseline/resultRoot。普通结束沿原executePost一次选择；新approval又Close/flush/no post。

发布段续执行只用封存artifact，原IOSResources已Close，不重复Prepare。无upload审批后确有ordinary run需要签名时才当前材料Validate、创建新独立本次资源，Close与cleanup真实，不修改旧artifact/reportseal。SealedReports不可重开；后续ordinary或下一approval发现原封存文件变化安全失败，不创建第二报告epoch。

等待不扣active NS，挂起NS从真实准备/封存/Close/fsync完成点冻结，checkpoint ACK等待不得动作/增长预算仍受原Authority；恢复校验和Prepare耗时继续减原NS。nil普通budget仍无上限语义，0不能当nil；post NS从原未使用值保留。拒绝/挂起取消/本地noTTY/EOF/拒绝不启动post；必要系统清理总独立WithoutCancel有限预算，cleanup未知仍guard。

## 查询、发布与后续

安全ApprovalView含ID/BuildID/Project/Name/Number/Step/Index/Revision/CheckpointDigest/State/NodeName/原artifactIDs+hash/ReportSeal/remainingNS/DecisionActor/time/NoteDigest（原Note仅私有DB审计）；无Params/Run脚本/私有paths/material/oldtokens。BuildView增加current_approval_id/revision/resume_reason及safe ledger，旧字段/JSON省略兼容。

010AuthorizePublish检查本build当前upload之前的全部approval定义（包括skipped）：必须每个有匹配当前checkpoint/DecisionApprove，绑定same attempt/封存artifact/JUnit，任何skip不是approve；配置无approval沿旧明确admin+allow规则。后续审批无法解除先前unknown publishguard，Resume亦不newPublishIntent替代unknown。020保护waiting/approved/checkpoint/原workspace/manifest，012/custom与020真实联验后续计整MVP，不预造其当前不存在字段或反向等待。

## 008已交付消费者兼容

Recover不得从已更新的attempt.Ref还原旧checkpoint：old record/receipt先独立验证，waiting/approved不会Expire；新running再核对当前node/session/credential/epoch。TerminalReceipt仍只精确current Ref的最终build_finished，不能把已取消pause变伪node终态。Retry对于安全pause后reject/cancel，仅中央完整checkpoint+row未进入新epoch/未resumed且真实StopKnown/Closed/manifest吻合可证明停止；不能单凭terminal/pending推断。已resume的独立StopConfirmation必须精确新fullRef，旧pause proof不能证明新checkout/进程已停。


### 真实报告检查点边界

含upload的审批属于发布段：全部普通run/artifact完成、报告passed且final/sealed后才能安全挂起，Root reportsFinal门不放宽。没有upload的midrun审批可位于后续run之前（CONFIGURATION:507、FR001/002/014/017）：仅持久已checked的同一revision、原XML快照与具体collection状态，不虚称final/sealed；Resume继续同一collection/原baseline/当前entries，后续实际run再检查，全部ordinary结束才final seal。没有第二report epoch、重采旧XML或重新执行脚本。私有报告collection记录只Agent，中央核当前checked证据/已确认XML/cursors；变化/缺失闭锁。
