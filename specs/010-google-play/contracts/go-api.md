# 010 最小具体Go接口候选

主代理复核冻结后才实施。沿现有Store/Run/process，不建interface、泛型repository、FastlaneClient或独立执行器。公共JSON所有可选新增字段omitempty保证旧消息digest；秘密/私有路径json:"-"，数组明确[]。以下省略既有package imports和一致snake_case tags，不是源码stub。

## pipeline与具体publisher

```go
type PublishInput struct {
    Index int
    Step config.Step
    ArtifactID string
    Artifact ArtifactRecord // 私有snapshot/SourcePath只同进程
    ReportSealDigest string
    ReportIDs []string
    OnStart func(process.StartInfo) error // 实际step started/journal消费者
}
// RemoteOptions新增实际唯一consumer；nil仍拒本地发布。
Publish func(context.Context, PublishInput) (protocol.PublishReceipt, error)

// internal/distribute：Prepare真实读取、只读AAB校验/明确授权诊断。
type GooglePlayOptions struct {
    BundleDir, Bundletool, DataDir, CredentialFile string
    AppIdentifier, VersionName, UploadCertificateSHA256 string
    Number int64
    ArtifactPath string
    ArtifactSHA256 string
    ArtifactSize int64
}
type PreparedGooglePlay struct {
    workDir, credentialCopy, tokenFile, artifactCopy string
    options GooglePlayOptions
    materialDigest, toolLockDigest string
    closed bool
}
func PrepareGooglePlay(ctx context.Context, in GooglePlayOptions) (*PreparedGooglePlay, error)
func UploadGooglePlay(ctx context.Context, p *PreparedGooglePlay,
    grant protocol.PublishGrant, onStart func(process.StartInfo) error) (protocol.PublishReceipt, error)
func QueryGooglePlay(ctx context.Context, in GooglePlayOptions,
    task protocol.PublishQueryTask) (protocol.PublishQueryResult, error)
func (p *PreparedGooglePlay) Close() error
```

Prepare/所有前置、OAuth和Close耗时计普通预算；Close只自有文件、独立有限系统清理预算，不执行未知remote abort。不能把PreparedJSON反序列化成未来任意operation。Upload只有授权once，Query只GET。OnStart是process真实消费者转发，不泛型executor。固定error codes play_tools_missing/play_tools_unverified/play_credentials_invalid/play_app_invalid/play_artifact_invalid/play_version_conflict/play_reports_invalid/play_authority_lost/play_budget_exhausted/play_unknown/play_cleanup_failed；不拼rawErr/路径/网络body。

Run签名不变。pipeline调用Publish时传当前普通剩余budget+Authority交集ctx；Agent闭包负责Prepare→authorizeHTTP→journalfsync once→UploadGooglePlay→journal receipt→精确recordHTTP；只要授权记录可能已提交而response丢失，返回publish_unknown，不能自动请求第二grant。Prepare失败无intent但普通步骤失败；Prepare可从原lease授权读取秘密，不执行商店写。

## protocol实际类型

```go
type PublishAuthorization struct {
    IntentID string // 节点请求前UUID并fsync
    Ref LeaseRef
    Index int
    StepName string
    ArtifactID, ArtifactSHA256 string
    ArtifactSize int64
    ReportSealDigest string
    ReportIDs []string
    VersionName string
    VersionCode int64
    Track string
    ExplicitProduction bool
}
type PublishGrant struct {
    IntentID, AuthorizationDigest string
    Ref LeaseRef
    Action, AppIdentifier, Track, ReleaseName, ReleaseStatus string
    VersionName string
    VersionCode int64
    ArtifactID, ArtifactSHA256, ReportSealDigest string
    ArtifactSize int64
    ReportIDs []string
    ChangesNotSentForReview bool
}
type PublishRemoteEvidence struct {
    EditID, ReleaseName, Track, BundleSHA256, Lifecycle string
    VersionCode int64
    BundleAccepted, TrackAccepted, CommitAccepted bool
}
type PublishReceipt struct {
    IntentID, AuthorizationDigest, Digest string
    Ref LeaseRef
    Status, EvidenceCode, MutationStage string
    Started, StopConfirmed, CleanupFailed bool
    Remote PublishRemoteEvidence
}
type PublishExpectation struct { IntentID, ReceiptDigest, Status string }
```

PublishLookup{Ref LeaseRef, Index int, IntentID string}绑定原节点/本attempt/冻结step；没有intent行时也必须先校验原attempt和step归属，不能用任意UUID推断他人状态。NodePublishState仅返回IntentID/Ref/StepClosed/Authorized/Status/ReceiptDigest，不返回grant/token或私有remote材料。当前NodeActor.ID等于原Ref.NodeID，先复核当前身份；活动slot的not_found不证明未授权，因为请求仍可在途。实际upload step_finished先短事务关闭slot，后来的Authorize都拒绝，Agent再只读取得所有该step真实intent，组成最终manifest；未提交授权的候选ID在已关闭slot不造假intent/receipt，可返回Authorized=false。已提交则必须原ID+unknown保护，不能404清掉。此读取不续旧lease、不授执行权，绑定归属与008独立停止receipt保持区分。

ExecutionProgress增加PublishIntents []PublishExpectation `omitempty`，只终态complete manifest和具体upload阶段的实际回报消费；旧无uploadnil不改digest。ID/集合精确匹配Store当前attempt，unknown必须原intent记录存在，不能Agent自报消失。PublishReceipt.Digest计算不含Digest自身；与现有ExecutionEvent的Seq/Digest不同职责，phase进度保持真实upload intent/started/finished。

PublishQueryTask{ID,Nonce,Kind,BindingID,IntentID,Store,NodeID,SessionID,AppIdentifier,Track,ReleaseName,CredentialRef,UploadCertificateSHA256,VersionName,VersionCode,ExpiresAt}；PublishQueryResult{ID,Nonce,Kind,BindingID,IntentID,NodeID,SessionID,ObservedAt,ObservedLifecycle,Matches[],DoctorChecks[],ToolLockDigest,Reason}，Matches有限ReleaseName/Track/VersionCodes[]/Lifecycle。Kind仅doctor/query，doctor无IntentID（omitempty）但必须BindingID；query必须IntentID且由Store原intent派生其余字段，不能从用户选择另一app。DoctorChecks沿既有ToolCheck安全字段，不能raw工具输出。无privatekey/token/hash伪证明，fullqueryJSON≤64KiB。管理消息不用旧LeaseRef取得权；NodeActor当前身份绑定task原node+session。

PublishGrant.ReleaseStatus由Store从冻结upload step派生：仅draft/completed，省略规范化completed；不是PublishAuthorization中的用户自由输入。原Intent保存同值并纳入AuthorizationDigest。publisher用其构造一次track更新，实际响应的版本集合、releaseName及status必须与原grant一致，才能给出TrackAccepted；不一致保持unknown且不继续commit。receipt的原AuthorizationDigest必须覆盖该选择，不接受另一个release_status的回执链；查询不改变原选择。

## Store最小实际入口

```go
type BindApplicationInput struct {
    ProjectID, NodeID, Store, AppIdentifier, CredentialRef string
    UploadCertificateSHA256 string
    AllowedTracks []string
}
func (s *Store) BindApplication(ctx context.Context, actor Actor,
    in BindApplicationInput) (ApplicationView, error)
func (s *Store) AuthorizePublish(ctx context.Context, actor NodeActor,
    in protocol.PublishAuthorization) (protocol.PublishGrant, error)
func (s *Store) FindNodePublish(ctx context.Context, actor NodeActor,
    in protocol.PublishLookup) (NodePublishState, error)
func (s *Store) RecordPublish(ctx context.Context, actor NodeActor,
    in protocol.PublishReceipt) (PublishView, error)
func (s *Store) RequestPublishQuery(ctx context.Context, actor Actor,
    intentID string) (PublishQueryView, error)
func (s *Store) ClaimPublishQuery(ctx context.Context, actor NodeActor,
    sessionID string) (*protocol.PublishQueryTask, error)
func (s *Store) CompletePublishQuery(ctx context.Context, actor NodeActor,
    in protocol.PublishQueryResult) (PublishView, error)
func (s *Store) ConfirmPublish(ctx context.Context, actor Actor,
    in ConfirmPublishInput) (PublishView, error)
func (s *Store) ListPublishes(ctx context.Context, projectID string,
    limit int, after string) ([]PublishView, error)
func (s *Store) GetPublish(ctx context.Context, id string) (PublishView, error)
```

绑定初始pending，通过实际节点同一有限管理查询机制的明确doctor请求（Kind仅doctor/query两实际消费者）核验后verified；Kind=doctor需要当前NodeActor回报工具锁摘要、应用实际GET成功/前提固定codes，不能仅管理员声明verified。管理请求表具体保存这两种工作，task/result增加Kind、BindingID与doctorChecks字段（omitempty）；不抽象任务executor。重复Bind不重置既有project/verification，无credential内容公开。

ConfirmPublishInput{IntentID,Key,ExpectedIntentDigest,Outcome,EvidenceCode,Note,EvidenceSHA256,RemoteEvidence}，字段有限具体schema。Get/List鉴权由现有server入口先Actor角色控制，Store实际写也再次admin授权。ApplicationView/PublishView/PublishQueryView字段见node-http，不含argv、env、参数值、privatepath或credentialref。Recover扫描真实guard/intent一致性，不能过期自动clear；Retry保原upload管理员规则且不复用intent。

## 唯一共享冻结点

root唯一写protocol/node.go、RunTypes callback、Store现有models/migration/event/enqueue/retry/query/recovery、agent journal/execute/serve、server trigger/http/json/files与CLI root，；本次用户授权publish-channels唯一写internal/distribute整个具体两商店包（包括fastlane.go/Fastfile/Gemfile.lock）和新protocol/publish.go，root串行集成共享消费者。方法/字段首个真实consumer红测后落地，不提供stub。011已同批实现六个真实Action逐副作用、一次grant/receipt与同appguard，不新增发布registry。019 seal字段以其已提交最终契约为准，再做一次串行兼容审核。

## 2026-10实际共同工具交接

节点publish_tools配置仅bundle_dir与bundletool：bundle_dir含本包固定Gemfile/lock/Fastfile与三个Ruby文件，及按该锁部署的gems子目录；bundletool固定1.18.3且核对官方JAR摘要。真实锁固定fastlane2.240.1、google-apis-core1.2.5、faraday-net_http3.4.4，Gemfile.lock含全部103gem版本与校验和，Ruby3.4.1/Bundler2.6.2已在自有无商店凭据环境执行。Go provider不读取宿主ADC、session或proxy；工具摘要从实际固定文件计算。Google doctor固定四项fastlane/bundletool/google_play_credentials/google_play_application；Apple固定fastlane/apple_transport/app_store_credentials/app_store_application，必须全部passed与ToolLockDigest，不能把凭据解析或管理员声明当核验。Google Upload只有完整bundle/track/commit回执才返回uploaded并重算Digest；空GET结果仍unknown。
