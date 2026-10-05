# 012 最小具体Go接入契约

本页仅规划候选，主代理在前置正式验收基线上冻结并串行实施。无源码stub/实现声明。类型省略imports，网络字段snake_case、新增可选variant均omitempty；所有秘密/原bytes/私有path json:"-"。不要重定义010共同Publish类型或019ReportSeal。

## config真实配置消费者

```go
type BuildProfile struct {
    Template string `yaml:"template,omitempty" mapstructure:"template"`
    File string `yaml:"file,omitempty" mapstructure:"file" json:"-"`
}
// ServerConfig/serverFile追加build_profiles；LoadServer路径以本机配置目录处理。
// BuildSettings追加Profile；PipelineSettings追加旧Profile，与Builds互斥。
type LoadedProfile struct {
    Name, Template, ContentDigest string
    Definition Build
    Notifications *Notifications
    Content []byte `json:"-"`
}
func ParseBuildProfile(data []byte) (*Build, *Notifications, error)
func LoadBuildProfiles(definitions map[string]BuildProfile,
    builtins map[string][]byte) (map[string]LoadedProfile, error)
```

ParseBuildProfile严格只root单build，禁止builds原键，复用现有YAML树和Validate；无别的配置引擎。LoadBuildProfiles真实唯一server.New消费者，builtin参数只四fixed名称对应实际模板bytes（不是可注册工具插件）；调用者server导入mobile模板函数，config不反向依赖mobile以避免005/009未来importcycle。自定义文件复用同包实际bounded读，已加载内容深复制。数量/大小/总量见plan；不读secret、不调用工具、不执行文件。builtin可直接使用且禁止用户同名覆盖；file不能请求里远程传入。

本地init --template继续config.Load/Parse合法完整Document，读取改用本包既有regular bounded consumer，不强迫模板单build。LoadServer/ProjectSettings的strict/Viper与旧字段继续，不自动导入任意环境映射或原配置秘密。

## SCM与Trigger唯一来源消费

```go
// scm.Options追加，仅现有ReadPipeline实现与Trigger消费。
FileMode string // omitted/required, optional, none
// scm.Snapshot追加。
Missing bool // optional且精确路径确实缺失时true；none不借Missing表达

// server私有类型/函数，不导出新service/registry接口。
type resolvedPipeline struct {
    SHA, SourceDigest, File string
    Document *config.Document
    Origins map[string]store.PipelineOrigin
}
func (s *Server) resolvePipeline(ctx context.Context,
    p store.Project, branch, ref string) (resolvedPipeline, error)
```

ReadPipeline同函数fetch固定commit再按FileMode读或skip；old空required完整兼容。optional缺失仍带SHA/File而Content空，无Digest（不得把空bytes哈希当已存在配置）；none带真实SHA，File沿options安全path，Missing=false/Content空。path相关父树逐段检查，父symlink等非缺失；旧code-only SCM error仍固定安全namespace。Server.resolvePipeline真实调用Config profiles map、同scm与config.Parse，输出repo或已深复制绑定Document；从不执行仓库shell。全部展开来源hash确定性，Origin供selected PreparedBuild消费，无新数据库profile表。

Trigger其余request digest/FindRequest、当前project/branch授权→来源→Select→参数/upload权限→Preview→buildCondition→Enqueue保持单入口。Origin赋给selected Snapshot；并发project更改仍靠PolicyVersion/CAS拒不一致，不在事务内读Git。repo Parse完整文档失败不可fallback，手动changes保持忽略；任何pending编号等模板事实不用于build.when。

## Store共享快照/安全视图

```go
type PipelineOrigin struct {
    Mode, Kind, SHA string
    File, Profile, Template string
    ContentDigest, DefinitionDigest string
}
// BuildSnapshot.Origin *PipelineOrigin `json:"origin,omitempty"`
// BuildView.Origin *PipelineOriginView `json:"origin,omitempty"`
```

PipelineOriginView只同安全来源名/仓库相对File/摘要/SHA，无content/Definition/params/profile私有File；可沿同具体value创建safe副本，不设泛型metadata。root唯一改Store models/validatePrepared/Enqueue/Recover/Retry/query，profile Source和new Origin严格字段、无raw私有path；旧nil保未知。Retry原Snapshot深复制Origin、不从配置加载，Origin.DefinitionDigest不含运行facts/新build.number，所以新号不改原完整定义hash。旧SourceDigest保originalsource含义，无猜补。

## 具体custom publisher

```go
type CustomOptions struct {
    Workspace, DataDir, WorkingDir, ResultFile string `json:"-"`
    Argv, QueryArgv []string `json:"-"`
    Environment map[string]string `json:"-"`
    Params map[string]string `json:"-"`
    Credentials, ArtifactPath string `json:"-"`
    AppIdentifier, VersionName, ArtifactID, ArtifactSHA256 string
    Number, ArtifactSize int64
}
type PreparedCustom struct {
    workDir, inputPath, artifactCopy, resultPath string
    options CustomOptions
    // 真实目录/叶文件identity与本次资源状态；非interface/hook。
}
func PrepareCustom(ctx context.Context, in CustomOptions) (*PreparedCustom, error)
func UploadCustom(ctx context.Context, p *PreparedCustom,
    grant protocol.PublishGrant, onStart func(process.StartInfo) error) (protocol.PublishReceipt, error)
func QueryCustom(ctx context.Context, in CustomOptions,
    task protocol.PublishQueryTask) (protocol.PublishQueryResult, error)
func (p *PreparedCustom) Close() error
```

全部在internal/distribute，Run全批预检查先校验custom静态参数/声明env/argv可执行路径/工作目录与结果parent；启动前重查安全目录。实际原artifact/JUnit尚未生成，普通结束后Prepare再核对真实证据，不能把预检当已封存。准备只本次验证/有限读取/自有copy不发发布；Upload只一次process.Run，调用者原Run Publish闭包以普通NS∩Authority授权、journalfsync/receipt/Record。cmd/env来自原冻结custom Step与声明密钥，一次受控渲染working_dir/result_file；argv/query正文原样不模板插值。app/version/number来源可信snapshot与中央原产物；AAB/IPA复用010/011原检查，任意其它artifact只证明原bytes/绑定版本上下文，具体格式身份由用户可信验证脚本/结果主张，不能声称自动解析未知格式。

QueryCustom实际由010已有管理任务consumer调用；无query数组固定custom_query_unavailable且不运行，不自动启动upload。有数组时先固定原SHA安全Checkout到自有隔离工作区，校验冻结工作目录与原QueryArgv，只执行query command，没有pipeline.Run普通/post；ctx任务ExpiresAt∩30s，原lease不用于新权。Query command只读是用户信任声明，系统不会认证GET-only。Close独立系统15s清自产目录/文件，替换/失败ErrCleanup沿既有sentinel由根串行实际关联，不能用任意string推断clean。

## 010协议/Store唯一扩展点

```go
type CustomPublishAuthorization struct {
    CommandDigest string
    ResultSchemaVersion int
}
type CustomPublishEvidence struct {
    RemoteID, Lifecycle, RequestSHA256, ResponseSHA256 string
    ActionConfirmed bool
}
type CustomBindingEvidence struct {
    Source, EvidenceCode, Note, EvidenceSHA256 string
}
type CustomQueryContext struct {
    Repository, SHA, WorkingDir, ResultFile string
    QueryArgv []string
    Environment, Params, Facts map[string]string
    OriginalRef LeaseRef
    AuthorizationDigest string
    ArtifactID, ArtifactSHA256, VersionName, ReportSealDigest string
    ArtifactSize, Number int64
    ReportIDs []string
}
```

PublishAuthorization/Grant加Custom *CustomPublishAuthorization，PublishRemoteEvidence/QueryResult Matches加Custom *CustomPublishEvidence，BindApplicationInput加Custom *CustomBindingEvidence，全部omitempty。只用于实际target custom，其他target出现该variant拒绝。Store旧共同AppBinding额外VerificationSource明确doctor_verified/manual_attested，安全ApplicationView也公开该来源，custom manual绑定合法可直接返回200已确认归属，商店doctor待验证仍202/pending；Custom admin当前身份与证据审计真实写consumer；不新建verification framework/假远端真实性。

PublishQueryTask加Custom *CustomQueryContext（omitempty），仅custom且Kind=query允许，不给doctor或其他target。Store从精确原intent与原BuildSnapshot派生，不从请求接收context：Repository不可变原项目来源、SHA原冻结提交，QueryArgv只原查询数组（不传上传Argv），Environment为原build与该upload步合并的已校验声明，秘密仅引用、无值；Params原最终参数，Facts只project/build.name/git.branch/step.name四项原冻结事实，step.name来自原intent。其余git.sha/build.id/build.number从SHA/OriginalRef/Number派生。node.name来自当前同NodeID真实身份，workspace是本次受授权查询隔离Checkout的实际目录，两者不是原执行环境证据。沿同一既有一次声明渲染/显式secret读取规则；未知事实固定拒绝，不复制整Step/Snapshot或另建模板引擎。

Context仅原当前节点、当前session和精确管理task授权私有传输，不进admin ls/show或safe DTO。task整体沿既有snapshot边界且≤64KiB，越限固定拒绝不截断，ReportIDs明确[]。Query输入的artifact只有id/size/sha256，无path；无旧artifact下载权，不依赖旧journal，不调用Prepare上传/Run。原Ref/AuthDigest仅关联证据，不能续旧lease/授新grant；只固定原Repository/SHA安全Checkout、一次QueryArgv进程和当前明确secret引用。WorkingDir/ResultFile沿上述上下文一次解析与既有安全路径规则。

Custom action精确custom_upload；AppIdentifier/Number/ArtifactID/size/hash/JUnitSeal沿共同字段。CommandDigest canonical冻结Step.Argv/QueryArgv/working_dir/result_file与原artifact/app/version描述，不含任何secret/env值；Variant.ResultSchemaVersion=1固定。授权末尾当前actor/fullRef/ordinaryNS/原artifact/JUnit/绑定/应用guard复检沿010。grant响应丢失仍原候选IntentID unknown，不执行第二grant/command；receipt同摘要幂等、旧fence拒，slot关闭后原候选lookup及终态PublishIntents沿共同消费者。

custom结果/查询格式见[custom-publish.md](custom-publish.md)，仅有限schema真实消费；相同known结果/真实停止时释放guard、unknown永不靠expiry或StopKnown解除。manual confirm沿010 ConfirmPublishInput精确expected digest/证据/幂等，不新增execute-confirm API。014approval未接前unsupported，不能用customActionConfirmed代替批准。

## 安全error / 兼容红绿门

固定custom_input_invalid/custom_credentials_invalid/custom_artifact_invalid/custom_app_invalid/custom_reports_invalid/custom_command_invalid/custom_result_invalid/custom_query_unavailable/custom_authority_lost/custom_budget_exhausted/custom_unknown/custom_cleanup_failed，无OS/argv/参数/HTTPbody原文。scheme错误按config现有field固定原因；server source模式失败保pipeline_invalid/scm固定码，不把异常隐藏为missing。

实际前置基线冻结后root核对010最终variant/JUnit字段、008deepcopy、009Framework及005签名resources；old没有Origin/Custom消息省略，新数组沿共同明确[]，metadata/private字段从不进网络digest。旧收据对任意新custom结果不能猜补；migration/原API/Run与三入口兼容必须真实红绿，不以规划代码块当可编译consumer。
