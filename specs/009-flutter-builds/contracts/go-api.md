# 009 具体 Go 接入契约

> 2026-10-05用户最新交付边界：先完成完整可执行代码与必要自动检查。005签名API只等真实代码冻结交接，不等合法Apple材料人工验收；真实签名／双平台／最终联验保留全部原要求，由用户最后统一人工验收并独立记录pending。源码集成或提交不宣称这些门PASS。无stub、第二Run、宿主未知身份或安全降级。

此次是规划冻结建议，后续由主代理统一审查并串行改共享源码；不存在本轮stub或已实现声明。

## mobile 的最小接口

```go
func FlutterTemplate(platforms []string) ([]byte, error)
type FlutterDoctorOptions struct {
    Workspace string
    Environment map[string]string
    Platforms []string
}
func FlutterDoctor(context.Context, FlutterDoctorOptions) []DoctorCheck
func ValidateFlutterParameters(platform string, params map[string]string, effectiveNumber string) error
// 以下签名函数仅005真实签名接口冻结交接后加入，不为独立组件编译创建占位。
func IOSSigningAvailable() bool
```

FlutterTemplate仅接受android、ios或二者的唯一集合，返回独立字节副本，单YAML/root version/所选build；重复/未知/空选择固定安全错误。不写文件、不读env或工具、不调用iOS资源。AndroidTemplate和旧无参init不改语义；IOSTemplate及IOSSigning解析沿005唯一writer交接，不在009复制实现。纯iOS/双平台模板严格Parse/Preview等005纯配置组件冻结验证；此前Android及工具-only诊断可独立交付检查，iOS组合门pending。

FlutterDoctor返回Flutter/Dart公共项及Platforms所选android/ios的实际JDK/SDK或Xcode/pod版本项；Platforms空只检查公共项，非空严格按已选择的唯一合法集合检查，不增加未选平台；未选ios不读取Xcode或签名声明。CLI先组合已有AndroidDoctor的wrapper/明确签名诊断，公共项只出现一次。iOS工具由本函数直接核Xcode/pod，不依赖尚未交付的IOSDoctor；无材料只工具+签名skipped。显式iOS签名请求在005实际代码未交接时固定unsupported且不读取秘密，005实际代码交接后再组合其真实IOSDoctor。SDK由明确有效Environment.PATH（或明确FLUTTER_ROOT）解析canonical安装根，使用其捆绑Dart及已初始化flutter_tools.snapshot，避免bootstrap自动下载；先有界核对已有版本/engine stamps、engine信息、snapshot及tool配置。直调snapshot仍可能更新informative缓存或fetch tags，不能保障无更新的渠道/布局固定不兼容，缺缓存固定失败，无在线修复回退。实际版本调用加--no-version-check/--suppress-analytics，doctor私有临时HOME隔离telemetry文件，保留明确SDK与必要缓存，不修改宿主偏好。缺缓存/坏版本/不配对/无法执行均固定failed。Workspace同CLI/本次Run工作区。Environment nil沿现有HostEnvironment；非nil为已构造的本次受限环境，不重新读取全部os.Environ，不临时os.Setenv。只给实际工具所需键，材料密码与无关secret不传Flutter/Dart体检。公共结果保持DoctorCheck{Name,Status,Version,Reason}。

ValidateFlutterParameters是纯数据检查：仅校验已声明的标准Flutter参数与可信有效编号，平台取android/ios；不强迫用户自定义脚本声明模板未用的参数，不读env/工具/密钥。错误不包含任何参数值。平台具体签名/路径校验仍沿004/005。

IOSSigningAvailable只复用005已有native availability实际查询，非cgo/非macOS或前提不符为false，不选identity、不导入/创建keychain；仅在005真实签名接口冻结交接后接节点机制能力。它不证明某次材料匹配，任务材料仍由ValidateIOSSigning验证。

## 私有工具helper实际消费

现有mobile.toolCommand增加具体 `Env map[string]string` 字段供FlutterDoctor消费，nil沿原HostEnvironment，非nil使用明确受限副本；不新增runner/interface/registry。可执行路径按该Env.PATH明确解析为绝对路径，不用无关ambient PATH代替本次环境；共享helper仍调用真实process.Run、单次15s、合并32KiB、取消/超限主动回收、固定安全错误。旧Android/IOS消费者不改Env就保持原行为。

清理不确定停止后续doctor工具调用；调用取消/更早deadline优先。公开从不返回argv/env/raw工具输出、未知字段或材料值。

## config / pipeline

config.Runner增加Framework string（yaml framework），省略/native/flutter；已有runner始终要求platform明确android/ios，旧无runner generic保持合法；未知组合安全拒绝，手动Node严格校验/重复未知/null边界保持。现有BuildPreview增加已校验Platform与Framework摘要（JSON platform/framework可省略），不公开参数值。PreviewOptions增加BuildParams map[string]map[string]string；每build先复制共享Params，再覆盖其BuildParams，最后沿ResolveParams严格验证。未知/未选择scope和同scope重复项整批拒绝，原Params语义保持；Run与Preview使用同一合并结果。

Run在整批用户动作前，对有效Flutter构建调用纯参数检查和FlutterDoctor的实际Flutter/所选平台工具检查，使用最终受限env。Run预检不调用仓库gradlew或其他工程脚本；现有AndroidDoctor会用ambient环境并调用wrapper，不能原样充当Run预检，旧CLI诊断行为保持。ios_signing静态Parse/Preview只在005纯配置组件交接后可用；生效iOS Run在005真实签名代码接口未交接期间整批固定unsupported、不调用材料resolver。005真实签名接口冻结交接后才调用ValidateIOSSigning；实际run前才Prepare。框架为flutter时，本地合法已声明build_number内部成为build.number，远程仅沿Remote.Facts中可信task.Number。公开PreviewOptions.Facts白名单不扩大，签名Facts/资源env仍系统独占。既有纯Preview不调用doctor或读secret。

iOS完整资源类型与生命周期100%沿005，不增加FlutterResources、hook或替代Close。Agent依旧只有pipeline.Run，progress/log/artifact由原具体回调持久化；008恢复与019报告同消费者，不改fence/预算协议。

## Agent / Store

既有protocol.ToolCheck结构保持；Store validReport明确增加flutter/dart/cocoapods三个固定名称，版本/状态/原因规则沿既有，不能接受任意工具名或标签推导事实。

Agent先实际检查SDK配对与pod，ios_signing保持当前skipped/unsupported；签名可用性wrapper不提前声明或伪passed。005实际代码交接后才消费其实际availability。没有Flutter不能让native工具检查退化，新增可选工具的普通缺失/版本失败不阻断已确认native/generic匹配；cleanup_error仍全局停止。Store Claim只在Runner.Framework==flutter时增加框架工具要求，iOS始终macOS与实际签名代码交接后的机制能力。Definition/Framework沿现有冻结编码，008 retry保原值并新号。

## 共享文件与兼容门

根唯一协调config、pipeline、CLI、mobile共享helper/native availability包装、Agent/Store原文件的owner；实施分区不并写、不另造入口。旧native/default/custom模板、既有strict输入、真实双库Claim与三入口编译都需回归。无新的Go依赖、数据库表或网络接口。

共享交接：T003分别记录已验收公共基线、005纯配置/预览manifest与005签名整功能提交；两个005门不能互换。新增Runner.Framework/BuildParams/Env只有对应实际模板、Preview、doctor、Run消费者的红绿后才接入，没有未来接口框架。

实际009停止证据边界：`pipeline.ErrFlutterCleanup`只表示本次真实工具预检清理不确定。Agent `errors.Is`消费该错误，持久化CleanupFailed/StopConfirmed=false并撤销Authority、保留journal，不能用零用户动作precheck终态或旧PID信号确认停止。工具已确认停止的缺失/版本/timeout错误仍沿既有precheck_error零动作分支。
