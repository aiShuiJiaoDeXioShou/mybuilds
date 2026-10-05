# 020 数据模型与状态

## 策略

config.Retention{Builds int64,Days int64}为全局有效值；config.RetentionOverride{Builds *int64,Days *int64}为项目覆盖。空块/省略继承，null禁止，正数并乘24h不溢出。ServerConfig.Retention保存启动值，ProjectSettings.Retention为可省略覆盖，不放进流水线Document/BuildSnapshot。

全局retentionPolicyRecord单例保存Builds/Days/Version/ChangedAt；只真实管理文件启动同步，未变不重复审计。项目沿SettingsJSON和PolicyVersion，不再加平行配置表。查询EffectiveRetention{Builds,Days,BuildsSource,DaysSource,GlobalVersion,ProjectVersion}来源为global/project。

## 可信终态与最小墓碑

buildRecord新增TerminalAt *time.Time（UTC，nil=未知）、HistoryState（live/retiring/partial/cleaned，旧默认live）、CleanedAt *time.Time。业务Status不变，清理不取消/停止执行。真正终态转换事务设置一次TerminalAt；serverTime来源写入路径见plan，不用NodeAt/UpdatedAt/停止确认。当前状态之外未知业务状态一律保护，不人为加waiting_approval/UploadUnknown字段。

完整有时间的succeeded/failed/cancelled/skipped/interrupted按TerminalAt DESC/ID DESC跨build排序；时间未知保留并不计排名，已cleaned墓碑不占配额，保护记录仍计排名。数量/年龄OR；每次复核读最新策略和保护。

cleaned保留ID、ProjectID、BatchID、Position、Name、Number、Status/固定Reason、TerminalAt/CleanedAt、RetryOf及最小原SHA/请求关系。大Snapshot/参数/steps/reports/body metadata删除，必要batch/request/NextNumber/操作审计不删。BuildView返回history_state/terminal_at/cleaned_at，cleaned只有安全最小投影；正文/日志/文件410。batch重放仍完整返回旧build ID/name/number/status，不查已删正文，不执行/分号。已清理原构建retry拒绝retention_retired。

RetryOf selfFK RESTRICT、batch/project关联不变；保留完整child保护所有parent祖先，跨页闭包检测循环/越界保护（最多100000关系/SQL总期限≤5s（含最后CheckLock），超限保护不删）。被同批允许清理的child先完成内容清理，再允许parent；最小关联行始终保留，未出现实际必要性不硬删父行。后续任何硬删设计必须先所有实际child/其它FK消失，不是020能力。

## 当前保护事实

Evaluate/Retire/Authorize/Delete前共用具体retentionProtection(tx,build)：queued/running，StopUnconfirmed，steps CleanupFailed/未停，attempt缺完整终态/独立停止事实，日志/普通post文件/019报告未确认，时间未知，retry_dependency，资源归属未知。多个reason按固定顺序返回。只有读取占用时可退役等待，业务保护不允许退役。

waiting_approval/发布unknown只在010/011/014真实模型到位后加入同一函数及其原状态事务；不造空字段/“当前已通过”测试。停止确认只解除停止事实，不解除未来unknown、资源/报告/依赖等其它保护。

## 清理事项与对象

retentionJobRecord：ID UUID、ProjectID、BuildID（每build唯一）、State(pending/waiting/partial/failed/paused/completed)、Reason固定码、GlobalVersion/ProjectVersion、EvaluatedAt、RetiredAt、CentralCompletedAt、NodeCompletedAt、CreatedAt/UpdatedAt。overall completed须中央完成+存在原节点资源时节点真实确认；无节点合法终态有显式not_applicable，不伪NodeCompletedAt。

retentionObjectRecord：ID UUID、JobID、Kind(log/artifact/junit/history)、原具体ObjectID/StorageID（私有）、Size/SHA、已核对的设备/节点identity固定编码、State(pending/retired/quarantined/deleted/failed/paused)、Reason、ConfirmedAt。清单来自原DB已登记metadata；中央不接受任意路径/新对象。junit仍019purpose、revision/key/source和seal，不改写原XML。未知孤立文件不入清单。物理删除与DB确认之间失败，原事项恢复；已确认不存在的原/隔离目标成功，类型/归属不明失败，不能删新出现的同名未知inode。

意图/对象/退役同原write事务，实际hash/flock/rename/unlink不在DB锁内；每次物理操作前短事务重新CheckLock/最新policy/保护/归属。若已不满足停paused，保留实际剩余文件；已有部分删除不伪装恢复完整。中央对象全删只记CentralCompletedAt；中央与节点都完成（或合法无节点not_applicable）后才清正文/大记录并置HistoryState=cleaned。此前仍需精确terminal/StopConfirmation/journal恢复的receipt/attempt；未形成最小墓碑前仍是未完成清理历史，不提前排除rank。最终保留必要最小证据/审计，包括原attempt身份及精确terminal receipt摘要，避免移除仍被引用的FK行；删除无再消费需要的中间事件/大正文，不用保留全部原日志。

## 实际读取登记

evidenceReadRecord：ID UUID、BuildID、Kind、ObjectID、StorageID、Owner UUID（本控制进程实例）、State(pending/active/closed)、CreatedAt/ClosedAt。无TTL释放权限。Begin短事务在未退役且当前控制运行权下注册；非阻塞打开同文件并取SH、检查identity→Activate短事务再核对→输出，Close fd后End。各失败均实际关闭再记End，EndDB失败保未确认而非假成功。

退役与Begin/Activate同DB写互斥，退役后拒新读取；早于退役的实际已Active可以继续，尚未Activate不得输出。清理取同文件EX|NB，有占用返回waiting_readers，不阻塞无限等待。重启先检查实际同inode锁；只有EX成功及旧owner失权、所有pending Activate不能成功，才把旧登记关闭。旧活HTTP读者即使控制DB锁失效仍由SH保护，不能猜进程已退出。SSE只锁正在读chunk，等待下一事件不持锁；退役后的新chunk/新连接410，已打开的chunk完整传完或实际中止释放。

## 节点资源与删除回执

nodeResourceRecord：ID UUID、BuildID、AttemptID、NodeID、原Session/Lease/Epoch、OwnershipDigest、HasWorkspace/HasResults、TerminalDigest/Seq、RegisteredAt。私有路径只在Agent data_dir/resources/{ResourceID}.json，不发给中央。OwnershipDigest只涵盖原Ref和固定归属槽/identity，不含PID、进度或停止状态；新增结果槽时可在终态前更新一次归属摘要，已登记workspace槽不能改变，terminal后冻结，删除必须对本地同一摘要。一个attempt固定ResourceID，登记只能在同当前fence首次创建/增加自有结果槽，不能换节点/工作区；terminal后不扩槽。节点本地记录data_dir/root及workspace/result目录identity、固定相对路径、原Ref、真实终态/stop确认、pending日志事实；日志/报告快照都在实际已登记结果目录内。

nodeDeletionRecord：ID UUID（同资源唯一）、JobID、ResourceID、NodeID、原BuildID/AttemptID与OwnershipDigest、State(pending/authorized/partial/failed/completed)、当前管理Nonce/ExpiresAt、确认Seq/Nonce/Digest/固定结果、时间。授权短nonce/deadline不授用户动作或重用构建lease，过期不新删；当前身份轮换仍同NodeID继续，撤销/别节点拒。节点data_dir内delete-journal/{DeleteID}.json先落盘，保存各槽pending/quarantined/deleted，实际删除后原ID原Seq/Digest确认；同Seq同Digest回执幂等，不同Digest冲突；真实partial→deleted新进度严格Seq+1，不新建删除身份。

节点先复核自己的data lock/当前活动run登记、未知journal/spool/未清系统资源/原停止回执；不得从旧PID存在/消失推断或发信号。路径必须映射登记的自有scm父目录或results目录，任何未知/链接/hardlink/特殊文件/identity替换失败。有限遍历与隔离后删除，剩余真实对象继续pending/partial；离线/永久墓碑不转派，中央完成和节点完成独立。

## 正式基线兼容与对象观察

零ordinary Started且ReportRevision0无seal的precheck/checkout/allskipped合法，不因配置Reports永远保护；真实Started未封存/未确认则保护，合法0文件封存不算文件未确认。interrupted以精确同Ref独立StopConfirmation可证明停止，不能要求ConfirmStopped重写旧step；日志/报告/资源保护独立。本Job已确认物理deleted的对象以原已确认metadata+本事项结果核对，不重新要求存在造成节点永远无法授权。

retentionObject首次Authorize消费私有ObservedIdentity/Size/SHA，持久固定Identity与Store生成的QuarantineSlot，后续同ID不可更换。quarantined是rename+同身份复核+fsync后的真实私有确认，之后再授权unlink；恢复只认原位置/已登记slot。Identity编码与私有JSON见go-api。

最小墓碑保持008原Ref/LeaseExpiresAt/attempt/node/session/credential关联、LastEventSeq/Status/StopUnconfirmed、精确末尾receipt Kind/StopKnown/CreatedAt/Seq/Digest和独立StopConfirmation。cleaned视图早做安全投影不解已删JSON；新Retry先retention_retired，已有原retry键按原request/batch最小投影重放，不再解析已清Definition。Recover对合法终态原本不解析Snapshot，继续原关联核对而非新恢复器。

### 退役与原快照重试的双向门

原samekey/digest已确认请求先沿最小安全batch投影重放，不新分号。新Retry只允许原HistoryState=live；retiring/partial/cleaned在同一原Retry事务、frozenBuild前及提交边界复核后均retention_retired。Retry先提交则祖先闭包保护原完整证据，Retire先提交则拒新Retry，不能在部分unlink后新增需要完整祖先的child。此为FR007/009/019与原008接口的实际竞争门，不建新重试器。

### 实际owner与fd身份

metadata.EvidenceReadOwner绑定当前独占Store实例；RegisterEvidenceReadOwner在原实际Server启动生成随机UUID，同实例不轮换。evidenceRead.Identity只由Activate(actual SH fd identity)写入。旧active修复要求当前观察与持久Identity一致且同inode EX和旧owner失权；旧pending从未Activate/输出字节，明确失权才可关闭。读登记没有TTL。LogStored.ID为私有实际logChunk业务ID。结果资源以真实ensureResult创建回调登记，目录先落盘pending，再同fence ACK，不能由后续事件替代创建事实。

nodeResourceRecord追加CompletedAt nullable与CompletionJSON（固定StopCode及四个已确认游标，不含秘密/路径/日志）；只实际中断资源完成consumer写，归属登记与物理Stop不能单独设置。正常终态仍沿精确TerminalSeq/Digest；旧未知/null保保护。


I4有界推进校正：每种具体扫描记录最后实际访问的原UUID，global policy持项目/中央对象/finalize位置；project持candidate/object/finalize位置；node持delete位置。私有默认空列与旧库迁移兼容，不进入公开视图，也不代替原排序时间、完成或授权。分别从仍保留的原build/object/job/delete行取terminal_at或created_at，与ID组成稳定有限回绕窗口；保护/失败/已有job同样推进，原业务时间/receipt保持。位置更新与原动作同write事务，控制/SQL失败rollback，非法UUID拒。无新表、任意scope注册/API/GC平台。
