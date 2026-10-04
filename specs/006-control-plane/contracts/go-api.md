# 006 具体Go接入契约（已冻结）

唯一执行基线2ab8991，不借005。这里只定义真实当前消费者，不能把声明做成stub、通用repository/interface/testhook。字段JSON安全边界见http.md；Config与Store类型由其唯一owner写入并串行同步，另一分区不得复制定义。

## config（C）

```go
// 不改既有Document/Build/Parse/Load流水线API。
type DatabaseConfig struct { Driver, DSN string }
type ServerConfig struct {
    Listen, DataDir, SecretsFile string
    Concurrency int
    Database DatabaseConfig
}
type ServerOverrides struct {
    Listen, DataDir, SecretsFile, DatabaseDriver, DatabaseDSN *string
    Concurrency *int
}
type ServerLoadOptions struct { Filename string; Explicit bool; CLI ServerOverrides }
func LoadServer(ServerLoadOptions) (ServerConfig, error)
type ClientConfig struct {
    Server string
    Timeout time.Duration
    RuntimeToken string // json:"-"，从明确env/ref解析，不可日志/公共JSON
}
type ClientLoadOptions struct {
    Filename string
    Explicit bool
    ServerURL *string
    Timeout *time.Duration
}
func LoadClient(ClientLoadOptions) (ClientConfig, error)
type BuildSettings struct { Params map[string]string }
type PipelineSettings struct {
    Source, File string
    Builds map[string]BuildSettings
    Params map[string]string // 旧单build default简写，与Builds互斥
}
type ProjectSettings struct { Pipeline *PipelineSettings }
func LoadProjectSettings(filename string) (ProjectSettings, error)
func ParseProjectSettings([]byte) (ProjectSettings, error)
func ValidateProjectSettings(ProjectSettings) error
```

数据库driver仅sqlite/postgres，SQLite DSN仅文件路径。LoadServer严格检查文件原类型，再按白名单环境/CLI覆盖，路径基于配置目录展开；默认不存在文件可以使用默认值，显式文件不存在拒绝。LoadClient只由远程命令调用，默认地址http://127.0.0.1:8787/timeout30s；token来自0600凭据文件中的有效literal/完整`${NAME}`或MYBUILDS_CLIENT_TOKEN覆盖；服务端普通配置不存token明文。HTTP仅loopback可明文，其他地址强制HTTPS且系统验证证书，无insecure开关。

ProjectSettings只支持pipeline显式块，source auto/repo，file默认mybuilds.yml；pipeline.builds.<name>.params提供命名默认参数，旧pipeline.params只归default并与builds互斥，不支持profile/方案。缺块不修改该块；provided块整体替换。unknown/null/重复键/绑定方案/notifications/triggers/retention均拒绝。配置真正消费者复用私有Node检查，Viper不代替严格类型检查。

## store（A）

```go
type Options struct { Driver, DSN string }
type Store struct { /* GORM、持锁session/file、私有写mutex，仅具体实现 */ }
func Open(context.Context, Options) (*Store, error) // 先取得独占；不自动重连锁
func (*Store) Close() error
func (*Store) Migrate(context.Context) error // default组及metadata，实际AutoMigrate
func (*Store) CheckLock(context.Context) error // 真实运行权；失效后不可恢复
func (*Store) Bootstrap(context.Context, string) error // 空值无操作；非空仅首次初始化

type Actor struct { ID, Role string }
func (*Store) Authenticate(context.Context, string) (Actor, error)
func (*Store) CreateToken(context.Context, Actor, string) (TokenCreated, error)
func (*Store) ListTokens(context.Context, Actor, Page) ([]TokenView, error)
func (*Store) RevokeToken(context.Context, Actor, string) error
// 本机Actor固定ID=local-admin/Role=admin，只有取得独占的本机CLI组装；不是HTTP输入。
type TokenCreated struct { ID, Role, Token string } // Token唯一一次明文返回
// TokenView只有ID/Role/CreatedAt/RevokedAt，不含Digest/Token。
type Page struct { Limit, Offset int }
func (*Store) CreateGroup(context.Context, Actor, string) (Group, error)
func (*Store) ListGroups(context.Context, Page) ([]Group, error)
func (*Store) RenameGroup(context.Context, Actor, string, string) (Group, error)
func (*Store) DeleteGroup(context.Context, Actor, string) error
// Group{Name,ID,CreatedAt,UpdatedAt}，改名/删除参数按name；HTTP路径转换同一语义。
type ProjectInput struct {
    Name, Group, Repository, Provider, DefaultNode string
    Branches, AllowedNodes []string
    BuildNumberStart int64
    Settings config.ProjectSettings
}
func (*Store) CreateProject(context.Context, Actor, ProjectInput) (Project, error)
func (*Store) GetProject(context.Context, string) (Project, error) // 稳定全局name
func (*Store) ListProjects(context.Context, ProjectFilter) ([]Project, error)
func (*Store) SetProjectSettings(context.Context, Actor, string, config.ProjectSettings) (Project, error)
func (*Store) MoveProject(context.Context, Actor, string, string) (Project, error)
func (*Store) DeleteProject(context.Context, Actor, string) error // 有历史/队列拒绝
// Project持有ID/GroupID/GroupName、输入策略、NextNumber/PolicyVersion/UTC时间；Repository/Settings/参数不可直接JSON。
type ProjectFilter struct { Group string; Page Page }

type EnqueueInput struct {
    Actor Actor
    ProjectID string
    ProjectVersion int64
    Key, RequestDigest, SHA, Branch, Source, File, SourceDigest string
    HasUpload, AllowUpload bool
    Builds []PreparedBuild
}
type BuildSnapshot struct {
    Definition config.Build // 未渲染的实际选定定义，含秘密引用；仅内部持久化
    Params, Facts map[string]string
    Condition string
    Reasons []string
    AllowedNodes []string
    DefaultNode string
}
type PreparedBuild struct {
    Name, Status, Reason string // status只queued/skipped
    Snapshot BuildSnapshot // 具体共享schema，A/B无需猜JSON；json:"-"
    InitialBudgetNS *int64 // nil无上限；非nil必须正数
    PostBudgetNS int64
    Steps []StepProgress
}
type StepProgress struct {
    Phase string // ordinary/success/failure/always
    Index int // 从1开始
    Name, Kind, Condition, Status string
    Reasons []string
    ElapsedNS int64 // 初始0
}
func (*Store) FindRequest(context.Context, Actor, string, string) (*BatchResult, error)
func (*Store) Enqueue(context.Context, EnqueueInput) (BatchResult, error)
// 未命中FindRequest返回nil,nil；同key不同digest返回ErrConflict。
// Enqueue事务复检身份/项目版本/幂等，queued统一编号，skipped Number=nil，逐项步骤同事务。
// FindRequest命中与Enqueue事务复用Replayed=true，新建=false；仅HTTP 200/201消费者，不公开JSON。
type BatchResult struct { ID, SHA string; Builds []BuildView; Replayed bool `json:"-"` }
type BuildFilter struct { Project, Group, BuildName, BatchID, Status string; Page Page }
func (*Store) ListBuilds(context.Context, BuildFilter) ([]BuildView, error)
func (*Store) GetBuild(context.Context, string) (BuildView, error)
func (*Store) Status(context.Context) (QueueStatus, error)
```

BuildSnapshot是A/B共享的具体当前数据模型。Store事务内编码该类型到内部SnapshotJSON；不import server/pipeline，也不猜opaque JSON。已分配编号/ID可在内部Facts补写，保持未渲染Definition，NULL编号的skipped不得伪造事实。普通参数键/condition由已知schema派生。

BuildView是专门安全视图：ID/Project/Group/BatchID/Name/Number(*int64)/Status/Reason/SHA/Branch/SourceDigest/参数键/条件事实键或非敏感Git值/预算/ordinary与post步骤摘要/UTC。不含Snapshot、run/argv/env正文、参数值、credentials、token摘要、仓库URL或节点密钥。QueueStatus仅项目数/queued/skipped计数。上述view准确JSON字段由http.md冻结；后续Agent内部快照读取API在007实际接入时新增，006不暴露快照下载路由。

PostgreSQL仅消费显式DSN（标准keyword/value或postgres/postgresql URL），宿主非空`PG*`、`SSL_CERT_FILE`、`SSL_CERT_DIR`环境明确拒绝；不修改全局环境、不退到`~/.pgpass`或`~/.postgresql`材料。`service/servicefile/passfile`及未知连接键固定ErrInvalid。支持host/port/user/dbname/password/connect_timeout/sslmode、application_name/search_path/timezone/options与当前pgx连接策略字段；TLS支持显式sslrootcert（含system）、sslcert+sslkey与sslpassword。TLS叶子文件非阻塞/no-follow打开，只读取普通文件至1MiB，私钥不得有组/其他权限；从同次字节构建内存TLS配置，不让pgx重新读取来源。保留verify-full主机名/证书链验证及require+显式CA的verify-ca语义。具体pgx ConnConfig交stdlib.OpenDB后由GORM复用，不再次解析原始DSN。

Store写操作统一验证角色、未撤销身份与独占运行权（local-admin依赖本机取得锁的前提）。Bootstrap sticky metadata不能因token全撤销而复活。所有PG写事务使用仍持锁的同一sql.Conn，非pool自动替换；SQLite记录锁文件identity、拒hardlink/URI。Close终止持锁资源，不能删除flock文件。

固定安全sentinel：ErrLocked、ErrLockLost、ErrInvalid、ErrNotFound、ErrConflict、ErrForbidden、ErrUnauthorized。Error()不拼入DSN/SQL/YAML/Git原始输入；errors.Is用于HTTP映射。数据库原生约束错误在Store映射，不能假定所有SQLite extended code已翻译。全库错误不公开底层driver日志。

## scm（B）

```go
type Options struct {
    DataDir, Repository, Branch, Ref, File, SecretsFile string
}
type Snapshot struct { SHA string; Content []byte; Digest, File string }
func ReadPipeline(context.Context, Options) (Snapshot, error)
```

唯一当前入口；不实现checkout/Agent/Git插件。server先检查分支授权，SCM仍验证branch/ref/path类型，不允许任意revision表达式。精确branch单次fetch后固定SHA；ref须该HEAD祖先。自有bare与空hooks/template，禁global/systemconfig与unsafeprotocol，树mode/blob/路径/size检查后有限cat-file。Content不公开。SCM返回固定安全错误，必要时具体Error{Code}供errors.As分类，不拼接raw Git/URL。过程预算取调用ctx与30s较小值，独立有限清理，只删除自有目录；错误固定不回显URL或Git stderr。密码不在URL/argv。支持匿名HTTPS、仅loopback HTTP、本地Git，以及显式SSH；HTTP凭据当前未支持，userinfo/password URL拒绝，不退到宿主credential.helper/askpass。SSH SecretsFile只识别GIT_SSH_KEY_FILE与GIT_SSH_KNOWN_HOSTS_FILE；两项完整、0600无symlink普通文件，引用路径相对secrets_file目录。派生固定ssh命令，不接收任意外部sshcommand；禁宿主agent/default identity/global known_hosts，BatchMode/StrictHostKeyChecking=yes，禁密码/交互/agentforward/proxy/localcommand/controlmaster。

支持SHA1/SHA256：有界ls-remote精确refs/heads/branch，输出refname必须精确匹配唯一行，只识别40/64对象格式；init同格式bare，再fetch该branch并从受控ref固定真正SHA，不能把前次探测SHA当快照。指定Ref先cat-file -t==commit，拒绝tag对象，不用^{commit}剥tag。metadata/stdout stderr各32KiB，blob1MiB，总过程30s（与ctx更早截止取小）；超限停止本次组并等待确认，每请求在DataDir/scm下MkdirTemp建立独立0700 bare/empty-hooks/0600凭据副本，不共享可变refs/cache registry。材料先限额读取普通无symlink文件再复制，固定ssh只引用自产副本；不得在确认前验证后重新读未知字节。自有bare清理失败不能宣称成功；process CleanupFailed时不能删除仍可能使用的目录并假称完成。

## server（B）与CLI（C）

```go
type Server struct { /* 具体Store/config，不持有执行器 */ }
func New(*store.Store, config.ServerConfig) *Server
func (*Server) Handler() http.Handler // 标准net/http实际路由
func (*Server) ListenAndServe(context.Context) error // 监听cfg.Listen，取消Shutdown，失锁停止写业务

type TriggerRequest struct {
    Branch string `json:"branch"`
    Ref string `json:"ref,omitempty"`
    BuildNames []string `json:"build_names,omitempty"`
    All bool `json:"all"`
    Params map[string]string `json:"params,omitempty"`
    BuildParams map[string]map[string]string `json:"build_params,omitempty"`
    AllowUpload bool `json:"allow_upload"`
}
func (*Server) Trigger(context.Context, store.Actor, string, string, TriggerRequest) (store.BatchResult, error)
// project/key分开传入，key来自Idempotency-Key；Actor只来自真实鉴权。
```

Server.Trigger业务顺序按plan，命名参数优先于共享参数；使用config与纯Preview，不调用Run或在控制端解析节点秘密。HTTP DTO/请求严格读取与固定错误由server负责；CLI不手工复制触发业务。本机组/项目/token管理调用真实Store相同方法，需取得独占；客户端remote经Handler同一规则。

CLI配置及CRUD/Trigger适配仅C写；A/B可在真实服务测试用net/http/httptest及实际进程，而不构造fakeexecutor/repository。主代理独占go.mod/公共接入及最终验证。

## 公共视图字段（A定义，B/C使用）

```go
type Group struct {
    ID string `json:"id"`
    Name string `json:"name"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}
type TokenView struct {
    ID string `json:"id"`
    Role string `json:"role"`
    CreatedAt time.Time `json:"created_at"`
    RevokedAt *time.Time `json:"revoked_at,omitempty"`
}
type Project struct {
    ID, Name, GroupID, GroupName, Provider, DefaultNode string
    Repository string `json:"-"`
    Branches, AllowedNodes []string
    Settings config.ProjectSettings `json:"-"`
    NextNumber, PolicyVersion int64
    CreatedAt, UpdatedAt time.Time
}
// Project必须由B显式映射HTTP安全ProjectView，不能直接编码内部模型。
type BuildView struct {
    ID string `json:"id"`
    Project string `json:"project"`
    Group string `json:"group"`
    BatchID string `json:"batch_id"`
    Name string `json:"build_name"`
    Number *int64 `json:"number"`
    Status string `json:"status"`
    Reason string `json:"reason,omitempty"`
    SHA string `json:"sha"`
    Branch string `json:"branch"`
    Source string `json:"source"`
    File string `json:"file"`
    SourceDigest string `json:"source_digest"`
    ParameterKeys []string `json:"parameter_keys"`
    Condition string `json:"condition"`
    Reasons []string `json:"reasons"`
    InitialBudgetNS *int64 `json:"initial_budget_ns"`
    RemainingBudgetNS *int64 `json:"remaining_budget_ns"`
    PostBudgetNS int64 `json:"post_budget_ns"`
    Steps []StepProgress `json:"steps"`
    Post []StepProgress `json:"post"`
    CreatedAt time.Time `json:"created_at"`
}
type QueueStatus struct { Projects, Queued, Skipped int64 }
```

StepProgress JSON字段分别为phase/index/name/kind/condition/status/reasons/elapsed_ns；BatchResult的ID映射batch_id、SHA映射sha、Builds映射builds；TokenCreated为id/role/token。A内部DB字段与view分离，安全view不得有SnapshotJSON/Params/Facts/token摘要。B只对公共BuildView/Group/TokenView编码，并显式映射Project/Status，不编码任意GORM模型。

## Server实际公共安全视图（B实现，C真实复用）

`server.StatusDTO` 的 `Version string`、`Concurrency int`、`Projects/Queued/Skipped/Running/Nodes int64` 分别使用 `version/concurrency/projects/queued/skipped/running/nodes` JSON字段，计数来自真实Store.Status；本阶段running/nodes固定0。

`server.ProjectView` 明确声明 `ID/Name/GroupID/Group/Provider/DefaultNode/PipelineSource/PipelineFile string`、`Branches/Nodes []string`、`NextNumber int64`、`CreatedAt/UpdatedAt time.Time`，JSON字段为冻结HTTP契约的snakecase。`server.ProjectSummary(store.Project) ProjectView` 是HTTP与本机CLI共用的实际mapper，默认来源auto、文件mybuilds.yml，不返回Repository、Settings、参数或PolicyVersion。

`server.BatchView{ID,SHA string; Builds []BuildSummary}` 的JSON为batch_id/sha/builds；`BuildSummary{ID,Name string; Number *int64; Status,Reason string}` 为id/build_name/number/status/reason。仅输出排队摘要，不编码Store.BatchResult.Replayed或内部快照。CLI可以在自己成功输出中加入本次request_key，不改变服务端契约。
