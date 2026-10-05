# 020 最小具体Go接口候选

所有方法仅在实施红绿后真实落地，当前只是冻结候选，不能提前填空实现。无repository/interface/通用GC或保护hook；路径不来自HTTP。

## config与Store

```go
// internal/config：由现有LoadServer和ProjectSettings两个实际消费者使用
type Retention struct { Builds, Days int64 }
type RetentionOverride struct { Builds, Days *int64 }

// internal/store：具体管理、读取与清理消费者
func (s *Store) SyncGlobalRetention(ctx context.Context, policy config.Retention) error
func (s *Store) EffectiveRetention(ctx context.Context, actor Actor, projectID string) (EffectiveRetention, error)
func (s *Store) EvaluateRetention(ctx context.Context, actor Actor, projectID string, page Page) (RetentionPage, error)
func (s *Store) ScheduleRetention(ctx context.Context, actor Actor, projectID string, limit int) (RetentionPage, error)
func (s *Store) ListRetention(ctx context.Context, actor Actor, projectID string, page Page) (RetentionPage, error)

// Server后台与实际中央文件删除consumer，均再次核对本控制端运行权
func (s *Store) AdvanceRetention(ctx context.Context, projectID string, limit int) ([]RetentionObject, error)
func (s *Store) AuthorizeRetentionObject(ctx context.Context, id string, observed RetentionObjectObservation) (RetentionObject, error)
func (s *Store) ConfirmRetentionObject(ctx context.Context, id string, result RetentionObjectResult) error
```

SyncGlobalRetention仅serve启动持控制独占后的受限管理配置consumer，不暴露Node/仓库HTTP。管理员修改项目仍用ProjectSettings和原UpdateProject事务；AdvanceRetention由实际Server后台调用，内部复用ScheduleRetention的具体私有schedule函数，在原控制运行权下有界扫描项目/恢复事项，不创建虚拟admin token；因此后台不会调用需要用户Actor的远程管理入口。Evaluate/Schedule/List必须authorize admin，不同版本候选每次操作重新计算。Evaluate返回当前项目的完整有界评估页，含Candidate=false的保留/活动/时间未知解释；Candidate只代表数量/时间原始条件，ProtectReasons独立。Server/CLI不再过滤或重排页，避免分页遗漏。

具体内部retentionProtection/terminal时间/闭包函数只被同包实际Evaluate、退役/Authorize以及原事务调用。retentionJob/对象由Store生成和登记；Server只消费已确认中央对象，拿到原ObjectID/StorageID/Size/SHA和事项归属（私有），不能任意扫描；RetentionObject包含ID/JobID/BuildID/Kind及私有StorageID/Size/SHA/Identity与已登记隔离位置；RetentionObjectResult只含固定State(quarantined/deleted/failed/paused)、Reason及实际复核的Identity，不接受外部任意路径。对象授权/确认两个具体短事务由同一Server清理consumer使用，文件IO在事务外，提交再CheckLock，不以随意boolean跳检查。

## 原文件读取与删除

```go
// kind只允许log/artifact；junit仍artifact元数据purpose分支
func (s *Store) RegisterEvidenceReadOwner(ctx context.Context) (string, error)
func (s *Store) BeginEvidenceRead(ctx context.Context, actor Actor, kind, objectID, owner string) (EvidenceRead, error)
func (s *Store) ActivateEvidenceRead(ctx context.Context, id, identity string) error
func (s *Store) EndEvidenceRead(ctx context.Context, id string) error
```

EvidenceRead.ID/BuildID/StorageID/Size/SHA为实际Server私有值，不编码外部JSON。Begin/Activate原权限与未退役复核，owner仅由实际Server启动调用RegisterEvidenceReadOwner生成，不接受请求指定。同一持控制锁Store实例重复绑定返回同一UUID；真正新独占Store启动替换metadata.EvidenceReadOwner，旧Store失权不能Activate。Activate持久化实际SH锁fd出生Identity。Server实际fd的SH锁与此登记配对；具体Unix共享/排他非阻塞入口只供现有download/log与本功能delete，非Unix明确unsupported。无新HTTP读取租约endpoint。重启修复active登记必须持同inode实际EX、观察Identity精确等于原读取Identity且旧owner无法Activate，不按时间释放；新替换叶子的EX不能证明旧fd已释放。旧pending未Activate/未输出字节，只在旧控制owner明确失权后修复。原LogStored增加私有ID，ListLogChunks传真实logChunkRecord.ID，不能用StorageID代替业务对象ID。

Server具体retention.go后台/管理HTTP调用sameStore schedule/对象授权/确认；原ListenAndServe维护一条有界循环，不另开daemon。实际目录隔离和SameFile/文件锁函数归该功能文件，路径由Store已登记UUID推导。原file roots、artifact/log/stream入口由root串行接入，保持旧流budget。

## Agent资源与管理事项

```go
func (s *Store) RegisterNodeResource(ctx context.Context, actor NodeActor, in protocol.NodeResourceRegistration) error
func (s *Store) ClaimNodeDeletions(ctx context.Context, actor NodeActor, limit int) ([]protocol.NodeDeletion, error)
func (s *Store) AuthorizeNodeDeletion(ctx context.Context, actor NodeActor, id string) (protocol.DeletionAuthority, error)
func (s *Store) ConfirmNodeDeletion(ctx context.Context, actor NodeActor, in protocol.NodeDeletionConfirmation) error
```

具体wire见[node-http.md](node-http.md)。实际Agent资源登记先于用户动作、Run结果创建时补固定结果槽；不能借注册覆盖原身份或路径。终态资源登记只允许当前同Node对已存在且完整字段完全一致原请求只读ACK；不新建/扩槽或写归属/时间/terminal/guard，供真实注册响应丢失恢复。删除consumer在原Serve中有限处理，当前活动/未确认journal保护；不调用pipeline.Run/process.Run，不解释或signal旧PID。局部delete-journal先persist，原ID固定内容确认，当前NodeActor同NodeID旋转身份可恢复。

共享protocol、新nullable TerminalAt/HistoryState与最小BuildView、原enqueue/event/stop/node/retry/recovery/project事务、Server文件读取/主循环、Agent journal/execute/serve由root唯一串行writer。A只Store新文件、B只Agent新文件、C只config/server/CLI新文件，具体路径见plan。008504dc6与019fee97e8真实字段已核对；以后真实审批/unknown接入时继续复核，不现在写占位。

## 正式T002具体观察与短事务

AdvanceRetention projectID空仅受信后台全局，当管理员run指定项目必须传已解析ID，不能删除另一项目。Store.write持s.mu，公开Effective/Evaluate/Schedule/Advance/Authorize不可在write内互调，仅复用具体私有tx函数。

```go
type RetentionObjectObservation struct {
    Identity string `json:"-"`
    Size int64 `json:"-"`
    SHA256 string `json:"-"`
}
type RetentionObjectResult struct {
    State string `json:"-"` // quarantined/deleted/failed/paused
    Reason string `json:"-"`
    Identity string `json:"-"`
}
```

Server实际受限打开/EX/hash后观察，不来自HTTP。首次Authorize核原StorageID/Size/SHA、当前政策保护/控制权，持久固定Identity与Store自推retention/quarantine/<JobID>/<ObjectID> slot，不接受路径。后续同对象不可换身份/slot。rename/核同fd/fsync后Confirm(quarantined)，再次Authorize同身份后unlink/fsync→Confirm(deleted)，真实阶段单调与同结果幂等，SQL失败保留原事项不假完成。IO不在SQL内。RetentionObject全部字段json:"-"，只安全RetentionEntry编码外部JSON。

## 3. 固定文件身份编码候选

仅已登记自有regular file；固定字符串最多160B：`v1:<darwin|linux>:<device-16hex>:<inode-16hex>:<birth-seconds-signed-decimal>:<birth-nanos-9decimal>`。无文件名、路径、UID、脚本、秘密或底层错误。kind由对象记录及实际regular检查限定，不用mtime/ctime当出生身份；rename可能改变ctime，内容SHA/Size另外复核。

Darwin从同一fd fstat取得device/inode/birthtimespec；Linux从同一fd statx(AT_EMPTY_PATH)请求并验证STATX_INO/STATX_BTIME等返回mask，设备字段规范化。不支持/缺少可证明出生字段、范围非法或观察不一致=>retention_ownership_unknown并保留，不用dummy0代替，不退化到仅路径或mtime。实际两平台/自有文件系统门在实施阶段，当前没有能力验收声明；本建议不承诺对有权篡改内核身份/私有目录的恶意宿主绝对安全。

官方技术依据：[Apple stat(2)](https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/stat.2.html)、[Linux man-pages statx原源](https://kernel.googlesource.com/pub/scm/docs/man-pages/man-pages/+/master/man/man2/statx.2)。Linux必须检查返回mask，不能把请求STATX_BTIME视作实际支持。节点目录身份采用同样出生身份；目录自身size/mtime随合法子项清理变化，不拿它们当固定身份。

## 4. 具体安全响应字段与现有019字段

EffectiveRetention：Builds/Days/GlobalVersion/ProjectVersion int64，BuildsSource/DaysSource string(global/project)。RetentionEntry：BuildID/Project/BuildName/HistoryState/JobID/CentralState/NodeState/Reason string，Number *int64，TerminalAt/CleanedAt *time.Time，Candidate bool，ProtectReasons []string（显式[]）；Page仅已有安全分页。列表不同模式仍用明确BuildID/JobID，不以一个ID字段在两种身份间偷偷切换。

BuildView新增HistoryState string、TerminalAt/CleanedAt *time.Time；cleaned返回安全最小投影与明确410读取结果。Build记录现有019 ReportRevision/ReportFinal/ReportsJSON/ReportSealDigest/ReportCheckedIndex及artifact Purpose=junit/ReportRevision/ReportKey/VerifiedJUnitJSON必须作为当前消费者核对，不能Outcome==passed即视停止/清理许可。ReportManifest{SealDigest,IDs}不重写、不重算旧terminal digest，不新建artifactStep。

## 5. 保护不是永久placeholder

- 无ordinary Started的合法precheck/checkout/allskipped：ReportRevision0且无seal合法，按真实无动作/StopKnown判断；不能仅配置Reports就永远reports_unconfirmed。
- 真正ordinary Started后：未final/sealed、文件未确认、checkpoint无法核对则保护；合法0文件sealed失败/optional missing不是文件未确认。
- interrupted+StopUnconfirmed=false：单独不证明全部资源安全。停止事实可来自精确同Ref独立stopConfirmation，而不是要求旧steps全StopConfirmed（当前ConfirmStopped不改旧step）；reports/log/ownership等其它保护独立保留。不能强求interrupted也拥有build_finished receipt。
- in-progress Job物理删过的对象：保留原确认metadata/完整清理计划至整体completed；当前保护只验证提交证据与此Job已确认删除事实，不能再按物理XML存在性判reports_unconfirmed，造成中央删完后节点永远不能Authorize。未知缺文件与本Job确认deleted不可混同。
- 旧未登记节点资源、旧时间不明、unknown journal仍保护，不用扫描补授权或旧PID信号。新资源登记/结果槽的失败不能把known变unknown=false。
- future审批/发布unknown不建空布尔/hook。现未知业务状态state_unknown；未来真实PublishIntent/appguard即使业务Status仍failed也需在同一保护函数核对，并由未来授权事务反向拒retiring/partial/cleaned。StopConfirm不解除unknown。

## 6. 墓碑保留与008消费者

当前Recover对合法终态已经不解析Snapshot（validateRecoveryBuild只queued/running进入frozenBuild），不要因此新建第二恢复器。保留原buildRef/LeaseExpiresAt与attempt/session/credential/node关联、LastEventSeq、Status/StopUnconfirmed以及精确末尾executionReceipt{BuildID,AttemptID,Seq,Digest,Kind,StopKnown,CreatedAt}。Kind在DB，TerminalReceipt wire没有Kind；独立停止记录同原Ref也保留。

真正必须接：buildView对cleaned早做最小安全投影，不能再解已清ReportsJSON/ParameterKeysJSON/ReasonsJSON；新Retry在frozenBuild前固定retention_retired。已有同identity/key/digest的成功请求重放沿保留request/batch最小投影，不能因为原正文已清重新分号或重新解析Definition；新请求才按当前规则拒。T051应包含既有retry key精确重放与新retry拒绝。Recover继续核墓碑非活动/精确关联；TerminalReceipt可原Ref/seq/digest只读确认，不授旧权。

## 7. 无递归write、无30s事务

Store.write持s.mu；Effective/Evaluate/Schedule/Advance/Authorize等公开方法不能在另一个write回调内调用。共用具体私有tx函数，每公开入口仅一次write；Server不持DB事务做flock/hash/rename/unlink。

祖先闭包最多100000关系，实际总期限取ctx与SQL≤5s更早值（包括最后CheckLock），不能拿“闭包30s”延长SQL。期限/数量/未知关系不足以证明安全时只保retry_dependency并停止本轮删除，不以截断子集授删除权。限制只需改文档/T021，非增框架。


### 退役与原快照重试的双向门

原samekey/digest已确认请求先沿最小安全batch投影重放，不新分号。新Retry只允许原HistoryState=live；retiring/partial/cleaned在同一原Retry事务、frozenBuild前及提交边界复核后均retention_retired。Retry先提交则祖先闭包保护原完整证据，Retire先提交则拒新Retry，不能在部分unlink后新增需要完整祖先的child。此为FR007/009/019与原008接口的实际竞争门，不建新重试器。

## 具体模型表名

Foundation固定私有record表：retentionPolicyRecord→retention_policies（单例1）、retentionJobRecord→retention_jobs（BuildID unique）、retentionObjectRecord→retention_objects（JobID+Kind+ObjectID unique）、evidenceReadRecord→evidence_reads、nodeResourceRecord→node_resources（AttemptID unique）、nodeDeletionRecord→node_deletions（ResourceID unique）、nodeDeletionReceiptRecord→node_deletion_receipts（DeleteID+Seq unique）。均保原build/attempt/node/project等RESTRICT关系，不能通过清理硬删被引用身份。Identity/隔离slot只Server私有持久字段；协议只独立管理身份/nonce/固定安全结果。A写新models，root原models/迁移/协议唯一writer。

## 真实结果目录登记消费者

原pipeline.RemoteOptions增加唯一实际Agent消费者ResultCreated func(context.Context,string) error。原remote.ensureResult实际MkdirTemp后立即固定resultDir并调用登记，再打开日志/执行；executeStep将ensureResult移到intent前。回调沿当前Authority与原累计剩余预算，不重新授执行权。Agent只在真实checkout成功且停止确认持久化后登记workspace；实际结果目录创建时补同ResourceID结果槽并持久化pending，收到精确注册ACK后才intent。基线报告失败也保留已登记结果，不靠LocalResultDir事件猜目录创建；本地Run和无动作分支不伪造回调。Terminal ACK后本地精确receipt落盘再移除原journal，独立stop ACK单独记录。原共享pipeline/run_types.go、remote.go、run.go及Agent入口由root串行修改。

RegisterNodeResource在既有POST的可选Completion中消费当前真实中断完成事实；具体NodeResourceCompletion见node-http。已存在同完整归属+真实独立Stop+中央游标核对，保存CompletedAt/CompletionJSON并精确幂等；不改旧TerminalSeq/时间/guard。Agent只有零pending、实际停止ACK与目录确认才能提交，原共享recovery入口有限恢复此精确本次事实，不扩大为通用恢复器。
