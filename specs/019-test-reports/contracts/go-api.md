# 019 最小Go接口候选（主代理复核后冻结）

不改变Run(ctx,*config.Document,RunOptions)(*RunResult,error)或新建执行器。配置现有Reports/JUnitReport不增加执行钩子。类型实际consumer是pipeline、Agent、server、Store、CLI，不建泛型repository/文件接口。

## 纯解析：internal/reports

```go
func ParseJUnit(ctx context.Context, input io.Reader, secrets []string) (protocol.JUnitResult, error)
```

固定errors.Is sentinel ErrInvalid、ErrLimit、ErrSecret、ErrCancelled；错误不拼rawXML/path/secret。input实际是有界、稳定私有snapshot或server已hash stage；parser本身再强制大小/深度/ctx与Counts边界。Secrets只含本次实际已声明值；server无节点secret列表，用nil重复验证语义。Result实际被两consumer使用，Counts/Diagnostics不作为执行器接口。

实际Reader仅为有限稳定普通文件/stage；parser在有界read/token前后核对ctx，不为任意blocking Reader创建可能泄漏的goroutine。非EOF Reader错误映射固定ErrInvalid；collector源快照IO另映射report_error。纯parser不填PathKey，Run/server真实consumer按ReportFile.Key补后再以集合限额聚合；必要共用聚合/UTF-8截断只新增实际两consumer使用的具体函数，不建formatter/collector interface。严格父子矩阵、可缺name、XML声明/BOM和诊断字节口径见[reports.md](reports.md)。

## protocol有限类型

```go
type JUnitCounts struct {
    Tests, Failures, Errors, Skipped, DurationNS int64
}
type JUnitDiagnostic struct {
    PathKey, Case, Outcome, Message string
}
type JUnitResult struct {
    Counts JUnitCounts
    Diagnostics []JUnitDiagnostic
}
type ReportFile struct {
    Key, Path, ArtifactID string
    SourceIndex int
    SourceStep string
    Size int64
    SHA256 string
    Counts JUnitCounts
}
type ReportEvidence struct {
    Revision int64
    Sealed bool
    Outcome, Reason string
    Required bool
    Counts JUnitCounts
    Diagnostics []JUnitDiagnostic
    Files []ReportFile
}
type ReportManifest struct {
    SealDigest string
    IDs []string
}
```

网络均明确snake_case json tags。Counts、Diagnostics与Files是有限具体schema；数组显式[]，不能null。ReportFile.Key=SHA256(规范Path)，Path无私有绝对路径或已声明secret；Source来自真实ordinary run。每新快照使用新UUID ArtifactID，未变化的本次报告沿稳定ID；本地也生成中性UUID，不读网络分配。

ExecutionProgress新增Reports *ReportEvidence `json:"reports,omitempty"`、ReportManifest *ReportManifest `json:"report_manifest,omitempty"`、LocalReports []CollectedReport `json:"-"`。CollectedReport只有对应ReportFile和私有SnapshotPath，沿LocalArtifacts语义，在Agent网络marshal前清空；不添加未来callback。BuildRun、BuildView新增Reports *ReportEvidence及ReportSealDigest（omitempty）；本地结果仅带实际安全XML元数据，不输出Collector基线或快照原私有路径。

新增Progress.Kind只有reports_checked/reports_sealed：逐run checked Phase=ordinary/Index=真实刚完成run/Name=step/StepKind=run；final checked和seal为无phase/name/step_kind、Index=0。原intent/started/finished保持真实执行事实，report不是step。ReportManifest只在build_finished且需要报告seal时出现。

ArtifactDeclaration/View增加Purpose string `json:"purpose,omitempty"`、ReportRevision int64 `json:"report_revision,omitempty"`、ReportKey string `json:"report_key,omitempty"`。空Purpose沿旧artifact，不改旧message/digest；显式artifact允许同原规则，junit为唯一新增用途。junit Phase=ordinary、Index/Step为真实生成报告run来源。

## Store实际入口复用

ApplyEvent原签名不变，dispatch checked/sealed到具体私有验证函数（实现internal/store/reports.go）。checked保存当前revision，seal必须逐XML完整确认并重算parsed聚合，不相信Node给的Counts。

CommitArtifact(ctx,NodeActor,ArtifactCommit)(ArtifactCommitted,error)原签名不变；ArtifactCommit新增VerifiedJUnit *protocol.JUnitResult（仅server内部参数，不接受HTTP JSON设置）。server在稳定stage读取ParseJUnit，CommitArtifact核对junit当前final revision/ID/Key/来源/Size/SHA，并存VerifiedJUnitJSON。普通artifact不得填该字段，junit必须填。文件内容仍stage→hash/fsync→排他发布→短DBfence，不在DB锁内做XML IO或网络。

沿ListArtifacts/GetArtifact/FindNodeArtifact原API；未seal/过期revision的junit不能通过用户列表/下载；同node当前fence的确认GET用于丢PUT回执恢复。BuildView只读实际seal结果。ReportManifest terminal必须核对完整ID/summary/seal digest和LastArtifactSeq，不重复造一套HTTP/DB入口。

## 共享冻结与008

新增可选字段omitempty，未配置reports时nil保持007和008旧终态Digest完全相同；新用途XML文件占同attempt原总配额。008TerminalReceipt使用原PendingEvent seq+digest完整匹配，不假造019receipt；receipt.Kind/StopKnown在完整terminal提交后记录。retry Definition.Reports可沿原快照，ReportEvidence/Manifest/IDS/cursor必须新执行为空。

protocol/node.go、Agent journal/execute/artifact、Store event/model/query/enqueue/artifact及retry/recovery、server files/artifact/http/json/trigger、pipeline run_types及run/remote/artifact由root串行owner收最小补丁；config/validate.go报告限额与pipeline/preview.go由C唯一写，业务新文件按plan A/B/C分区。契约只定义这次真实消费者，root review后才写源码。

## 实施阶段有限XML上传预算接点

既有CheckExecution只校验运行权，新增具体Store.ReportUploadBudget(ctx,NodeActor,LeaseRef)(*int64,error)只给实际Server junit上传消费者：同write/currentExecution/oldExpires复核，当前final且尚未sealed，最后已持久receipt必须reports_checked，使用该receipt的控制端CreatedAt从RemainingBudgetNS扣减累计时间。所有XML共用同一锚，不按文件重置；nil保原ordinary无限但网络仍≤2m，0拒启动。validateJUnitArtifact提交末尾复用同一私有预算计算，事务不读XML，不新增字段/表/cursor。Server实际上传deadline=min(2m,remaining)且定期检查原Authority；stage解析还受≤10s及该deadline约束。此具体接点落实原FR013/015，不放开Reports或预建通用接口。

## 2026-10-08 配置数量接入

config.JUnitReport新增MaxFiles *int，FileLimit()省略时返回256，Validate接受1–1024；preparedBuild将有效数量传入newReportCollection/restoreReportCollection的maxFiles参数。Store证据/确认/retention与Agent活动声明读取冻结Definition，普通制品数量排除junit；仅无快照的恢复以1024绝对上界检查。protocol.MaxReportMessageBytes=8MiB供实际相关消息消费者共用，journal容量见node-http契约。不新增表或依赖。
