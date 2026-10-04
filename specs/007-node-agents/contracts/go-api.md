# 007 具体 Go 接入契约

本文件为字段/函数的唯一声明；HTTP只引用消息名。JSON外围字段snake_case，非适用nil字段省略，不能发非法null；terminal显式发0 cursors/空ArtifactSteps。Task Definition响应按已有config.Build合法可空字段解释，不把它当管理请求任意null。time为UTC RFC3339Nano，duration/budget为int64纳秒。所有函数有ctx、有固定安全错误，不回显请求/命令/env/底层错误。现有006具体API保留；BuildView新增字段由server/client同步升级，不建版本协商系统。

## internal/protocol（root唯一写）

只依赖标准库和config；不依赖Store/Agent/Pipeline。下列字段按Go命名转换snake_case，标注private的字段json:"-"。

| 类型 | 字段 |
|---|---|
| LeaseRef | NodeID, SessionID, BuildID, AttemptID, LeaseID string; Epoch int64 |
| ToolCheck | Name, Status, Version, Reason string；status=passed/failed/skipped；版本严格解析，reason固定码；固定Name集合见下文 |
| NodeReport | OS, Arch string; Capacity int; Tools []ToolCheck |
| SessionRequest | SessionID string; Report NodeReport; HeartbeatNS, LeaseNS int64 |
| SessionGrant | NodeID, NodeName, SessionID string; HeartbeatNS, LeaseNS int64 |
| HeartbeatRequest | SessionID string; Report NodeReport |
| ClaimRequest | SessionID, ClaimKey string；ClaimKey每次领取UUID且本地先持久化 |
| TaskSnapshot | Project, BuildName string; Number int64; Repository, Branch, SHA, SourceDigest string; Definition config.Build; Parameters, Facts map[string]string |
| LeaseGrant | Ref LeaseRef; TTLNS int64; CancelRequested bool; Task *TaskSnapshot（仅Claim）；RemainingBudgetNS *int64; RemainingPostBudgetNS int64 |
| ExecutionProgress | Kind, Phase, Name, StepKind, Status, Reason, PostPhase string; Index int; Started, StopConfirmed, CleanupFailed bool; ExitCode int; ElapsedNS int64; RemainingBudgetNS *int64; RemainingPostBudgetNS int64; At time.Time; ArtifactIDs []string; LastLogSeq, LastLogOffset, LastArtifactSeq int64; ArtifactSteps []ArtifactExpectation; PID, PGID int、LocalResultDir string、LocalArtifacts []CollectedArtifact为private |
| ArtifactExpectation | Phase string; Index, Count int; IDs []string |
| CollectedArtifact（仅本地） | SnapshotPath, Name, SHA256 string; Size int64；只经private LocalArtifacts交Agent实际回传，不编码HTTP |
| ExecutionEvent | Ref LeaseRef; Seq int64; Digest string; Progress ExecutionProgress |
| EventAck | Seq int64; Digest string |
| LogRecord | UTC time.Time; Build, Phase, Step, Stream, Text string; Index int |
| LogChunk | Ref LeaseRef; Seq, Offset int64; Digest string; Records []LogRecord |
| LogAck | Seq, NextOffset int64; Digest string |
| ArtifactDeclaration | Ref LeaseRef; ID string; Seq int64; Phase, Step, Name string; Index int; Size int64; SHA256 string |
| ArtifactView | ID, BuildID, AttemptID, BuildName, Phase, Step, Name string; Index int; Size int64; SHA256 string; CompletedAt time.Time |
| StopConfirmation | Ref LeaseRef; EvidenceCode, Note string |

ToolCheck.Name只允许shell/git/java/android_aapt2/android_apksigner/xcode/ios_signing/node_journal；Android匹配要求前五项passed，ios当前无签名通过能力。节点自报不改变管理员labels或项目授权。

TaskSnapshot.Definition沿用config.Build的现有具体JSON编码，不复制步骤DSL。Task只下发冻结定义/最终参数/可信facts与仓库身份，无secret值/控制端token/用户token。SessionGrant.NodeName由AuthenticateNode的实际节点记录派生；Agent严格比较cfg.Node==grant.NodeName，不调用admin查询，不一致node_identity_mismatch并不领取。NodeID来自身份，Request中的同名字段必须相等。全标识UUID，Epoch>0。Progress.Kind只允许intent/started/finished/post_selected/skipped/build_finished；动作intent/started/finished/skipped的Phase只允许ordinary/success/failure/always，Index为对应冻结数组的1-based正数（沿006）；build_finished/post_selected不是步骤，Phase为空、Index=0，post_selected的PostPhase=success/failure/none，always始终单独处理。未选择post必须真实发送skipped/not_selected。终态result不含原始脚本/密钥/本地路径。

Digest=SHA256(json.Marshal具体Progress或Records)的lowerhex；Go map编码排序，服务端自行重算，不信客户端digest。Artifact.SHA256仅为实际文件内容摘要，Declaration没有另一个Digest字段；稳定ID/seq/phase/index/name/size/SHA全字段必须与原确认meta相同才幂等。Event/Log/Artifact各自连续序列从1开始、按attempt独立；same sequence same digest只返回原ACK，冲突或跳号拒绝。Log Offset累计规范Records JSON实际字节数，不是终端显示字节。所有幂等写先校验当前未过期权限，过期/终态不得凭旧ACK复活。

## internal/store（A）

具体类型：NodeActor{ID,CredentialID string}；NodeInput{Name string; Labels []string; Capacity int}；NodeCreated{Node NodeView; Token string}仅创建/轮转一次返回token。NodeView{ID,Name,State,OS,Arch string; Labels []string; MaxCapacity,LocalCapacity,EffectiveCapacity,Running int; Healthy,Quarantined,SessionActive bool; Tools []protocol.ToolCheck; LastHeartbeat *time.Time; CreatedAt time.Time}无token/digest/私有路径。NodeFilter{Page Page}。LeasePolicy{Concurrency int; Heartbeat,Duration time.Duration}仅server可信配置传入。

SetNodeState只接受enabled/draining/disabled，deleted是DeleteNode内部墓碑。Runner.Labels逐项精确匹配管理员标签集合，不解释glob。QueueStatus明确为Projects/Queued/Skipped/Running/Interrupted/Nodes/HealthyNodes七个int64；Running只计数据库running（含取消意图），Interrupted只计interrupted，Nodes排除墓碑，HealthyNodes计enabled且健康且无quarantine；guard占容量但不冒充running。Status现有Version/Concurrency保留，加上述七个snake_case统计字段。StepProgress保留现有字段，并增加Intent/Started/StopConfirmed/CleanupFailed bool、Reason string、ExitCode int；BuildView除前文列出新增字段外增加PostPhase string。

```go
CreateNode(ctx context.Context, actor Actor, in NodeInput) (NodeCreated, error)
ListNodes(ctx context.Context, actor Actor, f NodeFilter) ([]NodeView, error)
GetNode(ctx context.Context, actor Actor, name string) (NodeView, error)
SetNodeState(ctx context.Context, actor Actor, name, state string) error
RotateNodeToken(ctx context.Context, actor Actor, name string) (NodeCreated, error)
RevokeNodeToken(ctx context.Context, actor Actor, name string) error
DeleteNode(ctx context.Context, actor Actor, name string) error
AuthenticateNode(ctx context.Context, token string) (NodeActor, error)
OpenNodeSession(ctx context.Context, actor NodeActor, in protocol.SessionRequest, policy LeasePolicy) (protocol.SessionGrant, error)
Heartbeat(ctx context.Context, actor NodeActor, in protocol.HeartbeatRequest, policy LeasePolicy) (protocol.SessionGrant, error)
Claim(ctx context.Context, actor NodeActor, in protocol.ClaimRequest, policy LeasePolicy) (*protocol.LeaseGrant, error)
Renew(ctx context.Context, actor NodeActor, ref protocol.LeaseRef, policy LeasePolicy) (protocol.LeaseGrant, error)
ApplyEvent(ctx context.Context, actor NodeActor, in protocol.ExecutionEvent) (protocol.EventAck, error)
Cancel(ctx context.Context, actor Actor, buildID string) (BuildView, error)
ExpireLeases(ctx context.Context) error
ConfirmStopped(ctx context.Context, actor Actor, in protocol.StopConfirmation) error
ConfirmNodeStopped(ctx context.Context, actor NodeActor, in protocol.StopConfirmation) error
CheckExecution(ctx context.Context, actor NodeActor, ref protocol.LeaseRef) error
CommitLogChunk(ctx context.Context, actor NodeActor, in LogCommit) (LogCommitted, error)
CommitArtifact(ctx context.Context, actor NodeActor, in ArtifactCommit) (ArtifactCommitted, error)
FindNodeArtifact(ctx context.Context, actor NodeActor, ref protocol.LeaseRef, id string) (protocol.ArtifactView, error)
ListLogChunks(ctx context.Context, actor Actor, buildID string, afterSeq int64, page Page) ([]LogStored, error)
ListArtifacts(ctx context.Context, actor Actor, buildID string, page Page) ([]protocol.ArtifactView, error)
GetArtifact(ctx context.Context, actor Actor, id string) (ArtifactStored, error)
```

具体文件Store值（StorageID/Created全部private，不编码公共响应）：

| 类型 | 字段 |
|---|---|
| LogCommit | Ref protocol.LeaseRef; Seq, Offset, Size int64; Digest, StorageID string; RecordCount int |
| LogCommitted | Ack protocol.LogAck; StorageID string; Created bool |
| LogStored | BuildID, AttemptID string; Seq, Offset, Size int64; Digest, StorageID string; RecordCount int; CreatedAt time.Time |
| ArtifactCommit | Declaration protocol.ArtifactDeclaration; StorageID string |
| ArtifactCommitted | View protocol.ArtifactView; StorageID string; Created bool |
| ArtifactStored | View protocol.ArtifactView; StorageID string |

StorageID由server生成受限UUID文件标识，实际server依Created删除重复candidate。Log Size≤64KiB；只存元数据，不把日志bytes放SQL。FindNodeArtifact只查本次当前lease的ID，不能访问用户读接口或其他attempt。

管理方法只admin；Cancel/ConfirmStopped只admin；用户日志/产物只有admin/approver。NodeActor每次写重查CredentialID及撤销。OpenNodeSession同session握手重发幂等；过2*lease无心跳且no running/no guard才可替换为新session，旧session不复活。Heartbeat健康窗口3*heartbeat，仅影响新Claim；它不隐式续lease。

Claim nil=没有任务，queued记录留固定原因：node_unavailable/node_unauthorized/capability_mismatch/capacity_wait/build_name_locked/default_node_missing。ClaimKey绑定session，重复且仍有效只返回原grant，不能产生第二attempt。校验当前项目授权与冻结AllowedNodes交集、快照runner/admin标签、实际工具、健康、容量、同名锁；边界末尾再验。Renew提交前oldExpires仍有效，now==expires拒绝。所有执行写末尾复核原fence/UTC和control lock。

ApplyEvent匹配冻结步骤和合法phase选择，先intent再started/finished；预算不能增长、nil语义不变，ElapsedNS不得负。started只能来自实际Start/collector调用；finished与build_finished保留Started/StopConfirmed/CleanupFailed。artifact finished事件带ArtifactIDs（Agent由实际LocalArtifacts预分配稳定UUID并先写journal），Store保存该步骤预期ID集合；CommitArtifact须属于该集合且phase/index/name一致。build_finished带Agent本地真实最后LastLogSeq/LastLogOffset/LastArtifactSeq与ArtifactSteps（各实际artifact步骤phase/index/Count/IDs），Store精确核对中央已确认log cursor、连续artifact seq与各步骤预期ID/完整meta集合，无遗漏/额外/重复/count不符才能提交。零日志/零文件显式0/空清单，不靠“先flush”口头约定。终态要求无未完成intent、cleanup可信且日志/产物已确认；cleanup不确定转interrupted+guard。无法提交回执保留本地journal，旧结果不补成功。StepProgress扩展同名真实字段；BuildView增加NodeID/NodeName/SessionID/AttemptID/LeaseID/LeaseEpoch/CancelRequested/StopUnconfirmed/RemainingPostBudgetNS和真实Step证据，session/lease UUID仅为不可授权的证据标识，仍不公开token/digest/快照。原失败原因保留，post失败独立呈现。

Expire只核对到期，不把有效lease整批中断；过期变interrupted/lease_expired+guard。停止确认仅针对interrupted且stop_unconfirmed的原attempt，running正常停止用当前有效执行回执；进度保存失败但物理已回收时保留journal，待到期核对产生guard后才走独立确认，不伪造finished。停止确认必须精确原node/session/attempt/lease/epoch，EvidenceCode=process_group_reaped或admin_observed_stopped，Note必填1–1024字节无控制字符；仅清物理保护。Node端确认使用当前独立节点身份，可确认旧执行但不能提交旧result。删除为tombstone，禁止活动/等待/guard，名不复用。

错误沿用006 safe store errors，新增固定码node_unauthorized/session_conflict/session_expired/lease_invalid/lease_expired/event_conflict/sequence_invalid/budget_invalid/stop_unconfirmed/artifact_conflict/log_conflict；任何底层DB/raw消息不公开。

## internal/pipeline / process / scm（B）

RunOptions增加`Remote *RemoteOptions`，nil保持当前本地行为。RemoteOptions{AuthorityContext context.Context; Facts,Secrets map[string]string; ResultParent string; RemainingBudgetNS *int64; RemainingPostBudgetNS int64; Progress func(context.Context,protocol.ExecutionProgress) error; Log func(context.Context,protocol.LogRecord) error}。这两个函数是Agent实际消费者，不是hook/test接口。Remote必须单个选中build，禁止--step/All快捷选择；Agent从冻结Task构造Document和最终参数。全批预检查仍覆盖所有生效普通/post、不支持功能先失败，且remote不回查宿主Git facts或秘密。

Facts仅由Task+实际node/workspace构造可信project/build.name/build.id/build.number/node.name/git.sha/git.branch/workspace，不允许Params/PreviewOptions.Facts覆盖。Secrets仅此任务声明引用；缺失安全失败，当前step环境只有其build/step引用，其他值不注入；本地LookupEnv保持。ResultParent为Agent受控结果根，Run只在预检查通过且有实际动作后创建0700独立结果目录，必须在workspace外，ResultDir在本地回执private字段/journal和RunResult中，不发公共路径。

每动作：检查Authority→同步Progress(intent)成功→再次检查Authority→实际调用→Started消费者→Wait/回收/collector返回→finished(ns预算)同步保存→下一动作。预算由单调实际活跃耗时累计，含本步骤准备/日志/进度延迟，不从DurationMS反算；剩余按max(0,previous-spent)，post不复用普通预算；保存/网络延迟可在下一checkpoint进一步扣减，不能把旧ACK增加预算。Remote不恢复已执行步骤/重置预算；007重启不调用Run重放。全部ordinary条件skipped时沿已有Run返回skipped/condition，不强行succeeded或执行post。

用户ctx控制普通/非always，always的WithoutCancel立即合并Authority和post剩余；authority/保存错误导致全部后续用户post skipped/authority_lost或persistence_error。Run保存错误即闭锁，不依赖Agent后续判断。日志回调复用原logger脱敏/锁/UTC，在格式化前给真实phase/index，不解析Output；回调失败返回writer错误，沿现有process回收，整个Run不继续always。原exit/timeout/log_error等失败保留，cleanup副标志独立。

process.Command仅增`OnStart func(StartInfo) error`；StartInfo{PID,PGID int; At time.Time}。cmd.Start成功后调用，失败设置固定progress_error并取消/回收本组，仍只Wait一次，保持所有旧清理修复。不在Run外调用第二executor。PID/PGID仅journal，不公开；崩溃重启不按旧PID自动发信号。

SCM新增`Checkout(ctx context.Context, options CheckoutOptions) (CheckoutResult,error)`；CheckoutOptions{DataDir,Repository,Branch,SHA,SSHKey,KnownHosts string}，CheckoutResult{Workspace string; StopConfirmed bool}。每次自有attempt根，不共享Gitcache；复用gitRunner与006固定URL/refs/SHA1+SHA256/安全错误。整次≤2m、元数据输出≤32KiB、fetch/checkouts受Authority取消；固定commit可达与HEAD等于SHA，无hook/filter/submodule/helper/默认key，无HEADfallback。SSH参数显式copy受限普通材料，不能把整份node envfile传ReadPipeline；旧ReadPipeline行为不变。

## internal/agent（B）

`Serve(ctx context.Context, cfg config.AgentConfig) error`、`Doctor(ctx context.Context, dataDir string) (protocol.NodeReport,error)`为实际入口。Doctor只检查本地dataDir/journal与工具，不加载业务配置/token/秘密/连接；默认Capacity=1，Serve明确使用cfg.Capacity生成注册报告。未初始化目录可报告node_journal skipped/uninitialized，不创建目录或领取。Serve私有目录独占，journal读取/claimKey先落盘，session随机每进程；未确认旧journal不接新执行、不重放，doctor固定journal_unconfirmed。最多cfg.Capacity个实际执行，每个独立工作区/ResultParent/Secrets/日志seq与journal。

Agent仅直连具体HTTP和Run，不引入client/executor/repository接口。私有envfile严格NAME=value（不source、不插值），≤1MiB/256键/每值≤64KiB、重复/unknown格式/缺失/NUL安全失败；按此任务env声明取值；MYBUILDS_AGENT_TOKEN/MYBUILDS_CLIENT_TOKEN/控制端专用键不得引用。SSH只取MYBUILDS_GIT_SSH_KEY/MYBUILDS_GIT_KNOWN_HOSTS供Checkout，不进脚本。token实际env名/值不得落日志/journal/Task。

journal先fsync本地intent，再同步HTTP事件；Started/finished同顺序。artifact finished消费者从LocalArtifacts分配稳定ID/seq，填ArtifactIDs先落盘再发事件，之后仅从LocalResultDir受限快照流上传；build_finished消费者从journal真实生成最后cursors与ArtifactSteps，flush并核对中央ACK后发终态，不能用已确认子集假装完整清单。响应丢失重发同seq/digest，持久失败停止当前Run且禁止post。重新启动只能诊断/管理停止证据，不凭旧PID重放/自动kill。运行中真实回收后可用独立ConfirmNodeStopped确认旧attempt，仅清guard，不提交expired success。

## 无动作预检查失败的具体回执

Run真实返回nil result且为安全预检查失败（无用户动作启动），有效Authority内由Agent只journal并提交build_finished failed/precheck_error、Started=false、StopConfirmed=true、PostPhase=none和真实最后cursors/空ArtifactSteps；不在Agent复制步骤遍历或伪造动作事件。Store仅在没有任何intent/started的这一终态分支，将尚pending步骤派生skipped/precheck_error，Intent/Started保持false。已有RunResult、执行权或持久化失败不得套该分支；用户取消依据真实取消状态，不伪装预检查失败。Checkout失败/停止未知保留实际来源，不虚称清理成功。现有T019/T023/T026/T039覆盖，无新增执行器/协议框架。

## 实际回收与完整诊断快照边界

同步 collector 无需新增回调：intent 已确认后捕获实际调用前时间，collector 真正返回后发布 Started（保留捕获时间）与 finished；未调用不发布 Started，调用中崩溃依 intent 保持停止保护。实际完整快照可以在收集成功之后遇到取消、超时或日志关闭失败：只要 Started/StopConfirmed 为 true、CleanupFailed 为 false，finished 的 succeeded/failed/cancelled 均允许携带完整 LocalArtifacts 对应的稳定 ArtifactIDs；部分收集或未调用不能声明文件。Agent 始终以全部实际完整快照生成 manifest，不以最终 step status 清空诊断证据；失权后不能补上传。

CheckoutResult.StopConfirmed 依据实际 gitRunner 子进程组回收结果，在 error 返回时同样可用；Workspace 只在成功时提供。有效 Authority 内 Checkout 失败且明确停止、未启动用户动作，Agent 可只提交 failed/checkout_error、Started=false、StopConfirmed=true、PostPhase=none 与真实 cursors/空清单；Store 无任何 intent/started 才把 pending 步骤派生 skipped/checkout_error。回收未知只保 journal/guard，不猜测 error 即停止。

SetNodeState(disabled)、RevokeNodeToken 与 RotateNodeToken 在同一真实 Store.write 事务中，将受影响凭据的 running attempt 标记 interrupted+stop_unconfirmed，保留已有失败原因（否则 authority_lost）及完整 fence/容量保护。随后 enable 或新凭据不能续租旧 attempt、绕过 quarantine 或重新 Claim 同名。只有当前独立节点身份或管理员的精确停止证据可解除保护，不能凭禁用推断物理停止。无需新增撤销表或第二 writer。

## 最后普通步骤确认延迟耗尽预算

已确认的真实succeeded步骤不改写。最后普通动作的finished/日志/完整诊断快照确认延迟仍计入普通活跃预算；若下一post checkpoint确有ordinary Started且普通原本全成功、有限RemainingBudgetNS为0，post_selected允许Reason=timeout/PostPhase=failure，保留原失败（空时记timeout），终态必须failed/timeout。nil/非零预算或全ordinary条件skipped不能借此转换；全skipped仍none/skipped。post使用独立预算，Authority仍必须有效；没有新协议字段/状态或第二执行器。

## 节点私有材料零动作预检查

envfile格式或明确材料预检查失败可以复用failed/precheck_error零动作终态，须本attempt无intent/Started、无未确认的Checkout或系统子进程、Authority仍有效，并填写真实最后cursors/空manifest。只进行受限Go文件检查、没有外部动作时StopConfirmed=true有明确依据；材料引用缺失仍交唯一Run预检查，不复制步骤执行逻辑。若已有系统进程，其停止确认必须来自真实返回；未知或失权保journal/guard，不能凭‘预检查’名称伪造停止。
