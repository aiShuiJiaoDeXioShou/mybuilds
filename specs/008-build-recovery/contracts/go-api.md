# 008 Go契约

所有既有007签名/消息保持，以下仅列实际新增；根唯一写protocol、C写Store类型，根/B按此消费，无interface/stub。

## Store

```go
// BuildView增加安全关系；无关联省略。内部buildRecord用nullable字段。
RetryOf string `json:"retry_of,omitempty"`

type RetryInput struct {
    BuildID string
    Key string
    AllowUpload bool
}
func (s *Store) Retry(ctx context.Context, actor Actor, in RetryInput) (BatchResult, error)
func (s *Store) Recover(ctx context.Context) error
func (s *Store) TerminalReceipt(ctx context.Context, actor NodeActor,
    in protocol.TerminalReceiptRequest) (protocol.TerminalReceipt, error)
```

Retry返回既有单build BatchResult（公开batch_id/sha/builds），新结果Replayed=false，原样重放true且不公开该标志。server既有BuildSummary只增加同名安全RetryOf字段，batchView实际映射，用于retry响应与CLI结果关联。key沿现有长度/字符边界；Store计算canonical JSON `{operation:"retry",build_id,allow_upload}` SHA256，沿同identity+key命名空间，不能与trigger同key混用。创建、授权/停止核对、计数器、幂等与审计同事务。细节见 [data-model.md](../data-model.md)。Store不调用Git/Preview/Run或网络；缺失原commit交给实际固定SHA Checkout安全失败。

Recover为ListenAndServe监听前唯一实际消费者：服务端以30s独立子期限调用，持当前控制端锁、每页100条按主键游标核对校验queued/running的快照/身份/预算/进度结构及必要FK，未知持久状态/无效保护组合也拒绝；失败返回安全database_error、不接调度；验证后的到期处理复用ExpireLeases同规则。合法终态不重判，valid running不写身份/期限/预算；周期继续ExpireLeases。不得以恢复补造Started或终态。

TerminalReceipt以**当前**NodeActor重新鉴权（含撤销/禁用/删除），actor.ID等于Ref.NodeID，不要求旧session/credential仍有执行权。沿受控读事务复核控制端锁与当前凭据，精确检查原build/attempt/receipt和完整已提交终态；不调用currentExecution、Renew、ApplyEvent或StopConfirmation。未知/活动/保护返回conflict；不存在返回not_found；非法输入invalid_request；跨节点node_unauthorized。没有副作用及旧fence写权。

## 协议

只读回执消息及唯一字段定义见 [protocol.md](protocol.md)。

## Agent

保持`Serve(ctx, config.AgentConfig) error`、Doctor及Run的冻结签名。顺序为持data锁→受限读取旧journal→当前身份readonly查询→合法清本条→任何其它unknown仍拒Serve→Doctor/新session。私有具体函数核对journal全状态与PendingEvent.Progress均StopConfirmed且!CleanupFailed、Kind=build_finished、合法terminal Status，并重新计算网络摘要；返回Status/Ref/Seq/Digest必须精确相等。成功仅按同inode unlink并fsync目录，文件保存/删除/同步不明都拒绝继续；不掩盖另一条unknown。所有失败保留文件并返回现有journal_unconfirmed/invalid_response/data_invalid安全码，绝不打印journal或secret。

原进程短暂断连：只network_error可重试，heartbeat不延长执行lease；renew请求起点+TTL-原安全余量决定deadline，旧deadline到期不可因迟到回复复活。保持当前session，不重新注册/领取替代执行。心跳network_error不立即停止有效执行；该session网络失联容忍上限取最后成功心跳起的cfg.LeaseDuration，活跃任务仍各受自己更早的Authority deadline，未得到合法回复不延长任何期限；期间不发新的claim。永久鉴权/fence/保存/日志本地失败立即Authority cancel，全部always禁止。用户cancel在仍有效Authority下仍走原always规则。

普通events/logs重发原seq/digest、日志offset、artifact原声明和完整内容仅当前Authority允许；保持原spool上限和HTTP流期限，不能因重试重置预算/增长期限。build_finished使用单次post，响应丢失仍保存PendingEvent，原终态重发仍被拒绝，只有上述readonly核对可清本地待确认。

连续续租ACK丢失后，本进程仅记录最后实际续租请求起点及本轮HTTP期限，物理组已回收后独立stop-confirmation等待上界覆盖该请求可能已提交的最大TTL（原起点+本轮timeout+LeaseDuration+安全余量）。该等待不延长Authority.deadline、不运行用户动作、不再次续租、不确认未知终态；中央到期后精确停止确认只解除guard，原reason不变。服务退出仍只单次有界独立确认，失败保journal。

同进程claim断联：首次network_error立即暂停创建新ClaimKey；仅对尚未完成的同一请求、原key/session/journal，沿007已有幂等Claim在首次请求起点+LeaseDuration-安全余量内确认。原请求起点不可重置；无task确认可清本条，有task仍按原起点验证Authority，超期限不得执行。确认未知/期限耗尽保持journal，不新claim、不删未知，不因该单次网络错误误停其它尚有效任务；它们各依自己的Authority期限停止。heartbeat恢复不消除未确认claim，只有原key精确确认后才恢复新领取。永久auth/fence/本地保存错误仍闭锁；新进程绝不接管此unknown claim。

## 019协调

不增加报告字段/虚构seal。Retry深拷贝原Definition.Reports；当019实际接入后，新的attempt报告证据/cursor/IDs/seal均空，原报告证据留在原build。Recover不重解析XML或依据post改写重新判报告；TerminalReceipt只核对原完整事件Digest，019新增已确认报告manifest必须由同一终态事务完整验证后才给StopKnown。当前008验收保留reports unsupported门。
