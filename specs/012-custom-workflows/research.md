# 012 研究决策与实际读取记录

## 规划执行记录

2026-10-05独立WT由已验收007 85b46bf创建，实际Git分支012-build-presets-planning。正式读取本区README/AGENTS、specify/plan技能、constitution2.1.0、BUILD_DISTRIBUTION/CONFIGURATION/INTERFACES/ARCHITECTURE/MVP_EXECUTION/SPECKIT_ROADMAP；root008及独立009/010/011/019设计只读，不复制运行源码。before/after specify/plan hooks={}。实际resolve spec-template/checklist-template→创建spec/checklist和仅本WT selector；质量迭代保留既定012-custom-workflows全部范围，16/16通过；spec/checklist先独立SHA冻结。初次check-prerequisites --require-spec在未setup-plan时按实际脚本仍要求plan，exit1；随后实际setup-plan --json复制resolved plan模板，第二次prereq成功。resolver plan-template实际调用成功，无手工跳过模板步骤。

## D1 三来源沿一个真实Git函数

**Decision**：扩现有scm.Options.FileMode为旧空required/optional/none。auto=optional，repo=旧required，profile=none；全部先同一个ReadPipeline安全fetch/固定SHA。optional仅ls-tree精确请求路径确实没有记录时返回Missing；非普通叶/父symlink、目录、gitlink、超限、非法编码/YAML或Git失败均拒。none不读文件但照旧校验repo/branch/fullcommit。
**Rationale**：现git.go一次安全bare fetch、matching SHA1/SHA256、hook禁用/有界process，config_missing目前作为error丢SHA；只让真实Trigger消费者区分缺失并携带同次fetch的SHA，避免二次HEAD/网络竞争。
**Alternatives**：把所有scm error当缺失会绕坏路径/权限；先查另一个HEAD再读取使绑定快照错提交；新Git解析器或通用source接口无必要。

## D2 方案启动加载与有限单构建模型

**Decision**：ServerConfig.BuildProfiles映射template/file二选一，四builtin不用注册；管理员别名可引用builtin，自定义文件只root-level单Build，不允许任何builds键（即使仅一项）。复用checkTree/checkType/Parse/Validate的最小私有入口，没有另套YAML模型。内置009命名YAML恰含一个对应平台Build，提取后按项目绑定名称深复制，用户文件不借此放宽嵌套builds。
**Rationale**：LoadServer局部Viper已验证Node输入；本地普通文件读取已NOFOLLOW/NONBLOCK+fstat有限，真实方案与init --template可复用。server.New加载全部明确声明方案内容为启动时固定副本；重启读取新内容仅影响新触发，排队/retry沿Snapshot，无热加载/多版本数据库。
**Alternatives**：每个build触发时临时读不同文件有TOCTOU与混版本风险；新profiles CRUD/继承层/插件注册器没有产品要求。

## D3 参数、命名与快照

**Decision**：框架平台flags只生成项目绑定，不造仓库YAML。boundName覆盖提取Build的名称，labels/framework和工程版本参数仍真实模板原定义；不自动移除平台标签或增加节点权限。沿ResolveParams、Preview、buildCondition、Enqueue：named trigger>shared trigger>project named params>definition defaults，旧profile/params互斥normalize为default。项目builds对repo来源只作为同名参数默认，不混profile步骤；有参数覆盖但不存在于所选集合则沿未知scope拒绝。
**Rationale**：现Trigger已先upload权限再when；Preview的pending是未定系统模板而非build.when；batch原子编号与冻结Fact由Store完成。每BuildSnapshot.Origin保存实际来源/完整Definition/固定SHA/hash，原batch Source保持请求模式，精确Origin补实际来源。Origin=nil旧记录保持未知不猜；008Retry只复制Origin与原Definition，根核对原BuildID/number仅系统事实替换。
**Alternatives**：复用当前方案展开retry会违反008；为scheme增加独立编号/runner会破坏单项目计数与同名互斥。

## D4 与009/014/015关系

**Decision**：仅引用009已规划FlutterTemplate及纯参数/实际doctor/labels；009尚未验收，四平台真实门不以模板Parse成功代替。014审批未正式接入前含生效approval仍拒，不能custom自报approved；015以后以冻结source/buildnames/param规则调用唯一Trigger，不在012造changes diff或quiet_period。项目通知结构仍后续模块，012不移除明确unsupported gate。
**Rationale**：配置复用不是审批或自动触发，已有三态/手动changes忽略/原快照retry必须保持。
**Alternatives**：把方案选择当平台doctor、上传授权或审批通过没有证据，也扩大前置范围。

## D5 custom共享发布与手工绑定

**Decision**：internal/distribute/custom.go仅四具体Prepare/Upload/Query/Close方法接010唯一共同类型；custom action单条意图，命令可能包含任意用户子动作但系统不再发第二次。应用唯一(store=custom,app_identifier)沿010绑定表；custom新增真实admin消费者ManualEvidence及VerificationSource=manual_attested，记录原project/node/app与非秘密依据审计，不声称remote GET真实验证，商店doctor来源保持区分。
**Rationale**：custom query可选、用户命令没有可通用证明的应用检查API；管理员信任指定脚本，只确认确定归属与互斥。root已接受此边界，无新registry/verification框架。
**Alternatives**：把退出0/任意query输出当远端真实性核验或自动verified误报；强求任意渠道GET适配器/强制query会偏离产品可选契约。

## D6 结构化输入、结果与命令安全

**Decision**：argv/query_argv正文不插值、长度受限，process.Run直接数组exec。明确使用用户仓库命令（例如argv=[bash,ci/upload.sh]；shell显式子命令是用户信任，不由引擎拼接参数）。固定系统env给0600输入JSON路径/新结果路径与本次秘密，普通params经有限输入map或声明env，不导出宿主完整环境或admin token。结果≤64KiB严格JSON/普通文件/singlelink/NOFOLLOW/NONBLOCK，绑定IntentID/AuthDigest/原artifact/version/app；只有限远端ID/状态/摘要，无任意payload/URL或字符串错误正文。声明秘密命中拒回传，不改写证据。stdout/stderr有限私有捕获而非直接公开；logger仅固定安全状态。
**Rationale**：existing Step已有Argv/QueryArgv/ResultFile，Preview未渲染argv；不改raw正文处理，custom值通过输入。读文件需要先非阻塞open再fstat，不能只先Lstat再阻塞Open。结果绑定防旧文件；Prepare拒既存result，运行后重新验证文件身份/亲目录且有限读取，用户修改input/材料不影响中央原Artifact证据。
**Alternatives**：shell插值、stdout当JSON可信result、给子进程server token、截断/改写结果后标success都会破坏证据。任意用户脚本内部网络重试无法由系统禁止；计划明确信任范围不保证外部恰好一次。

## D7 unknown、查询与预算

**Decision**：Prepare无发布写，失败不授意图；IntentID在网络前fsync，grant可能提交后任何模糊结果unknown。Upload只执行一次；receipt confirm沿当前Authority和Store原intent。upload.finished先关闭slot，再原候选精确lookup形成terminal完整manifest；unknown appguard保持，物理stop独立。Query当前NodeActor/session任务、原QueryArgv/原SHA/原设置、30s独立bounded ctx，不借旧lease；只读是用户承诺，无法系统证明任意脚本不写，管理员显式发起。空结果/exit0/缺correlation不得解除；足够精确证据才同Store边界确认，否则manual原摘要+有依据幂等confirm。
**Rationale**：直接沿010/011的原意图/guard/发布slot协议与008 readonly终态receipt，没有另套retry执行器。Prepare/exec/回执/报告核对均消耗原普通NS含0耗尽，post独立且失Authority即停；Close独立有限系统预算只自产临时文件，CleanupFailed保留。
**Alternatives**：task结束重新发上传、旧Ref query授权或未知时解guard都会绕共同安全门；非零exit不证明未发送。

## 只读范围与冻结前提

本轮只有产品/已存在源码与已规划contract研究，不访问商店、凭据或宿主应用，不装工具/业务依赖，无真实发布。所有012 API是拟实施具体consumer契约，不能计源码存在。正式实施须先拉入已验收008/009/010/011/019基线，重核对最终公共类型、摘要omitempty及迁移版本；没有NEEDS CLARIFICATION，也没有声明这些前置已通过。
