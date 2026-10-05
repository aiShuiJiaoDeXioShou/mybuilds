# 015 最小具体Go契约候选

主代理在008/012/014接受后复核冻结。这里只列当前真实consumer，不增加executor、repository interface、source registry、通用事件总线或未来callback。字段无json tag的业务结构不直接公开marshal；API safe DTO单独映射。数组规范[]，新增快照optional omitempty保旧digest。

## config

```go
type TriggerSettings struct {
    Builds []string `yaml:"builds,omitempty" json:"builds,omitempty"`
    QuietPeriod string `yaml:"quiet_period,omitempty" json:"quiet_period,omitempty"`
    AllowUpload bool `yaml:"allow_upload" json:"allow_upload"`
}
type HookSettings struct {
    Enabled bool `yaml:"enabled" json:"enabled"`
    RepositoryKey string `yaml:"repository_key,omitempty" json:"repository_key,omitempty"`
    Secret string `yaml:"secret,omitempty" json:"secret,omitempty"`
    Generic *GenericHookSettings `yaml:"generic,omitempty" json:"generic,omitempty"`
}
type GenericHookSettings struct {
    Auth, AuthHeader, EventHeader, PushEvent string
    SignaturePrefix, DeliveryHeader *string
    RefPointer, AfterPointer, BeforePointer, RepositoryPointer *string
}
// Generic每字段yaml/json为同名snake_case、可省略采用契约defaults；不接受extra字段。
// ProjectSettings追加Hook *HookSettings、Triggers *TriggerSettings，各omitempty。
// ServerConfig/serverFile追加WebhookSecretsFile，yaml/mapstructure webhook_secrets_file，json:"-"。
func LoadWebhookSecrets(filename string, names []string) (map[string]string, error)
```

HookSettings.Secret的JSON仅用于严格admin settings输入，值必须完整env引用、不能明文；安全响应一律WebhookPolicyView/ProjectSummary，不marshal原Settings。

LoadWebhookSecrets只有Server实际hook配置consumer：已声明names（strict/unique）从明确独立文件有限读取，不读宿主env、不把SecretsFile/Git SSH两键映射拿进来；错误固定。既有Parse/Validate不读材料，不网络。Hook.enabled=false仍严格检查显式字段形状；无hook不载文件。Generate key是Server实际admin管理动作，不放config parser。

## scm：固定provider与diff

```go
type HookInput struct {
    Provider, RepositoryKey string
    Header http.Header
    Body []byte `json:"-"`
    Secret string `json:"-"`
    Generic *config.GenericHookSettings
}
func ParseWebhook(ctx context.Context, in HookInput) (protocol.WebhookEvent, error)

type ChangeOptions struct {
    Source Options
    TargetSHA, BaselineSHA string
}
type ChangeResult struct {
    TargetSHA, BaselineSHA string
    BaselineAvailable bool
    Paths []string
    Digest string
}
func ReadChanges(ctx context.Context, in ChangeOptions) (ChangeResult, error)
```

ParseWebhook由唯一Server hook route消费，原body已经MaxBytesReader限定，但函数自身仍检查限额/ctx。固定分支switch仅四值；GitHub/GitLab调用已锁webhooks/v6固定push消费，GitHub可靠ping按认证后忽略，其余stdlib有限字段；不会请求Body任何URL。认证/event/JSON/仓库/ref/SHA按http契约，unknown event合法认证后Kind=ignored/Reason固定；非push不虚构Branch/SHA。ReceiptDigest=SHA256 canonical `{provider,kind,delivery_id,repository_key,branch,before,after,body_digest,reason}`，关键字段均固定排序/明确空，不带secret/原headers。BodyDigest仅原bytesSHA256，跨delivery改头同body由Store识别别名。sentinel errors.Is ErrHookInvalid/ErrHookUnauthorized/ErrHookLimit/ErrHookCancelled，Error()均固定hook_*码、不拼库错误。

ReadChanges沿既有Options.Source字段（包括012 FileMode）但不另读pipeline文件；TargetSHA必填完整commit、BaselineSHA可空，HTTP无对应用户任选URL接口。原gitRunner/process.Run复用每请求自有bare初始化/full授权分支fetch和安全env；Source.Ref不与TargetSHA冲突，实际取TargetSHA固定且仍验证可达。BaselineSHA空=>BaselineAvailable=false；完整授权fetch成功后cat-file batch-check精确missing才false，不拿普通exit/error猜missing；对象存在必须commit不剥tag。Target/config读取错误、timeout、cleanupfailed、大小/路径/status错误保持scm固定安全error，不退full。Paths sorted unique且完整，Digest=SHA256 canonical Paths；无base时Paths=[]，server决定fixed full reason。父总45秒由关闭consumer控制，原系统cleanup独立上限不授用户动作。

## protocol/快照/Run实际事实

```go
// root sole writer internal/protocol/node.go；由SCM/parser与Store两个当前consumer共用。
type WebhookEvent struct {
    Provider, Kind, DeliveryID string
    RepositoryKey, Branch, Before, After string
    BodyDigest, ReceiptDigest, Reason string
}
type ChangeFacts struct {
    Mode string `json:"mode"`             // full|diff
    Reason string `json:"reason,omitempty"` // full: baseline_missing|baseline_unavailable
    BaselineBuildID string `json:"baseline_build_id,omitempty"`
    BaselineSHA string `json:"baseline_sha,omitempty"`
    TargetSHA string `json:"target_sha"`
    Paths []string `json:"paths"`
    Digest string `json:"digest"`
}
// BuildSnapshot追加Changes *protocol.ChangeFacts `json:"changes,omitempty"`
// ComparisonKey string `json:"comparison_key,omitempty"`
// AutomaticWindowID string `json:"automatic_window_id,omitempty"`
// PreviewOptions/RunOptions追加Changes *protocol.ChangeFacts（不新增Run签名或事件kind）。
```

server统一准备生成ComparisonKey，Store重新校验与本次sorted selection/Definition/Params/Origin/branch一致；snapshotomitempty不猜旧证据。事实TargetSHA必须等于Task/Enqueue SHA，路径/摘要重新核对。full reason固定且Paths=[]；diff Reason空、base非空，[]代表真实无变化。server Preview与独立build.when、Agent→Run都实际消费同字段；SourceParams/immutable definition正常保留，本地Run用户未提供Changes保持原语义，CLI不接受伪造Changes文件/Fact。Retry继承原Changes/ComparisonKey但不AutomaticWindowID；原预算/执行证据规则不变。

## Store：有限具体模型与方法

```go
type WebhookActor struct {
    ProjectID, CredentialID, SecretFingerprint string
    PolicyVersion int64
}
type WebhookPolicy struct {
    ProjectID, Provider, RepositoryKey string
    Enabled bool
    CredentialID, SecretRef, SecretFingerprint string
    Generic *config.GenericHookSettings
    BuildNames []string
    Params map[string]string
    BuildParams map[string]map[string]string
    QuietPeriod time.Duration
    AllowUpload bool
    PolicyVersion int64
}
type WebhookPolicyInput struct {
    Policy WebhookPolicy
    Settings config.ProjectSettings
    ExpectedPolicyVersion int64
}
type WebhookEvent struct {
    ID, ProjectID, CredentialID, WindowID string
    Event protocol.WebhookEvent
    ReceivedAt time.Time
}
type WebhookReceipt struct {
    EventID, WindowID, Status, Reason string
    Replayed bool
}
type WebhookWindow struct {
    ID, ProjectID, GroupKey string
    Generation, Revision, PolicyVersion int64
    State, Reason, Branch string
    Policy WebhookPolicy
    OpenedAt, Deadline time.Time
    CandidateSHA, FinalSHA, BatchID string
    BuildIDs, ReusedBuildIDs []string
}
type WebhookBaseline struct {
    BuildID, SHA, AttemptID, ReceiptDigest string
    Seq int64
    ConfirmedAt time.Time
}
type WebhookComparison struct {
    Name, Key string
    Baseline *WebhookBaseline
    Changes protocol.ChangeFacts
}
type WebhookCloseRef struct {
    WindowID string
    Revision, PolicyVersion int64
}
type WebhookCloseInput struct {
    Ref WebhookCloseRef
    Actor WebhookActor
    SHA, Source, File, SourceDigest string
    SelectedBuilds []string
    Builds []PreparedBuild
    Comparisons []WebhookComparison
    HasUpload bool
}
type WebhookWindowResult struct {
    WindowID, State, Reason, SHA, BatchID string
    BuildIDs, ReusedBuildIDs []string
    Replayed bool
}
func (s *Store) ConfigureWebhook(ctx context.Context, actor Actor,
    in WebhookPolicyInput) (WebhookPolicy, error)
func (s *Store) ReadWebhookPolicy(ctx context.Context, project string) (WebhookPolicy, error)
func (s *Store) ReceiveWebhook(ctx context.Context, actor WebhookActor,
    event protocol.WebhookEvent) (WebhookReceipt, error)
func (s *Store) DueWebhookWindows(ctx context.Context, page Page) ([]WebhookWindow, error)
func (s *Store) ReadWebhookWindow(ctx context.Context, id string) (WebhookWindow, Project, error)
func (s *Store) FindWebhookBaseline(ctx context.Context, comparisonKey string) (*WebhookBaseline, error)
func (s *Store) CloseWebhookWindow(ctx context.Context, in WebhookCloseInput) (WebhookWindowResult, error)
func (s *Store) FailWebhookWindow(ctx context.Context, ref WebhookCloseRef,
    reason string) (WebhookWindowResult, error)
func (s *Store) ListWebhookEvents(ctx context.Context, actor Actor,
    project string, page Page) ([]WebhookEvent, error)
func (s *Store) ListWebhookWindows(ctx context.Context, actor Actor,
    project string, page Page) ([]WebhookWindow, error)
```

Store只消费protocol.WebhookEvent安全具体值，不导入scm、不调用Git或网络，不能复制声明或引仓库接口。

内部读policy/window/baseline/due不是公开路由，CheckLock及受控短事务；公开list需要admin/approver实际Authorize且最后复核锁。业务模型不直接marshal为安全DTO。ConfigureWebhook重新鉴权admin/expected version，Settings显式块、hook policy和审计同事务更新并增加项目policy；输入Policy所有project/provider/node/选择权限派生真实记录而非信请求。

Receive及Close重新读HookActor所对应的项目credential与策略，事务末尾复核enabled/version/fingerprint/控制锁；ProviderSecret仅Server内存。Receive取Store UTC而非event提交时钟；ignored也有receipt无window，永久理由固定。Close调用同一私有enqueuePreparedTx而非导出Tx/GORM给server；原manual Enqueue仍重新admin/trigger授权。EnqueueInput追加SelectedBuilds，空旧输入按当前requested prepared names规范，完整selection供ComparisonKey与所有定义权限验证；自动复用全部结果见[data-model](../data-model.md)，不是拼接旧snapshot或改其batch。

Close snapshot封存SHA并保存Changes，窗口新batch只有fresh子集，safe result BuildIDs含所有selected对应结果、ReusedBuildIDs标旧引用；没有fresh batch时BatchID为空，不能造空batch或分号。全部新选择定义先验证，即使会reuse仍不能绕过当前发布权。错误沿既有ErrInvalid/ErrForbidden/ErrConflict/ErrLockLost/安全database_error；窗口Reason只固定hook_policy_changed/hook_pipeline_invalid/hook_source_error/hook_limit/hook_timeout/hook_forbidden，不回显raw原因。

## Server/CLI私有实际消费者

```go
type WebhookPolicyView struct {
    Enabled bool `json:"enabled"`
    Provider string `json:"provider"`
    RepositoryKey string `json:"repository_key"`
    Builds []string `json:"builds"`
    QuietPeriod string `json:"quiet_period"`
    AllowUpload bool `json:"allow_upload"`
    PolicyVersion int64 `json:"policy_version"`
}
type WebhookConfigured struct {
    View WebhookPolicyView `json:"hook"`
    Secret string `json:"secret,omitempty"`
}
func (s *Server) ConfigureWebhook(ctx context.Context, actor store.Actor, project string,
    settings config.ProjectSettings, rotate bool) (WebhookConfigured, error)
func (s *Server) receiveWebhook(w http.ResponseWriter, r *http.Request, project string)
func (s *Server) closeDueWebhookWindows(ctx context.Context) error
func (s *Server) closeWebhookWindow(ctx context.Context, window store.WebhookWindow) error
```

现有Trigger中仅提取私有prepareTrigger(ctx,project,固定source,names,inputparams,perbuildChanges)以供manual/auto；已冻结012.resolvePipeline唯一来源，不public列出可伪造source actor。close在DB外读取真实branch→source→baseline→changes→Preview，Store末尾CAS。周期只在现有ListenAndServe持锁context运行，停止/失锁不再接收/关闭，不调用pipeline.Run。管理安全DTO与固定HTTP/status表见http.md。

ConfigureWebhook由管理HTTP与本机admin CLI两实际consumer调用：解析所有显式settings、验证scope/材料准备、Store同事务写，不能先独立SetProjectSettings再失败才写hook。无hook块的旧pipeline导入不加载材料、沿原方法。

`WebhookConfigured`只管理初始化/轮换响应含SafePolicy及一次Secret；Secret有值仅实际自产ID与本次成功Configure一致，失败/重放无值。普通ProjectSummary只新增hook_enabled/triggers安全选择/quiet/allow_upload；不输出SecretRef/privatekey/params/env/Generic认证头值。

## 本次实际消费者修正

空指针采用缺省，显式空SignaturePrefix/DeliveryHeader可关闭前缀/标识，显式空JSON pointer拒绝；避免把显式空与缺省混淆。provider payload的普通metadata允许null，固定必需身份/ref/SHA仍必须类型有效；管理员配置JSON保持null/duplicate/unknown严格拒绝。有限原始body由四真实consumer共用，不将第三方SDK读取活请求body。

实际source复用repositoryRead/openRepository：ReadPipeline和ReadChanges共享唯一受控bare/fetch/fixedSHA，不另建executor。cat-file --batch-check使用有限匿名stdin，process.Command新增Stdin io.Reader并沿原process.Run转发；真实missing响应才允许full。

实际新文件使用hook.go/hook_close.go/hook_routes.go与webhook_close.go，不重复创建规划中的同义webhook_*路径。CreateProject追加准备证据ID/PreparedHook，只供实际Server.CreateProject消费，Store同原创建事务调用configureWebhookTx；无材料准备的直接Hook创建/SetProjectSettings拒绝，普通pipeline-only导入保原Hook/Triggers并生效新policy版本。初始化的ProjectConfigured保持旧ProjectView扁平字段，仅实际成功新增webhook响应；generated Secret仅本次配置成功且CLI明确--json时输出。

新增管理员 POST /api/projects/{project}/hook/enable 为原配置重新启用，严格空object，不生成或回传旧secret；不是新的凭据来源。CLI对应 project hook enable/show/rotate/disable/events/windows，本机show/enable/rotate/disable保持原Store独占。
