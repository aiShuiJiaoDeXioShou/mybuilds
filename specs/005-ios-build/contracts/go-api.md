# 005 共享接入契约

本worktree005全部文件已由主代理明确交由A唯一实现；主代理最终串行集成最小差异。本区不写 stub 冒充 process/schema/CLI。接口命名和资源方向由主代理冻结，具体公开字段以此交接。

## 体检与模板
```go
func IOSTemplate() []byte
// DoctorCheck 已由主代理实现；状态 passed/failed/skipped，Reason 固定安全值。
func IOSDoctor(context.Context, IOSDoctorOptions) []DoctorCheck
type IOSDoctorOptions struct {
    Workspace string
    Signing *IOSSigningOptions // nil 仅检查工具与身份数量，材料 skipped
}
```
不扫未知 profile、不使用宿主 app identity。用户指定材料使用同一无副作用校验；数量不等于授权。

## 显式配置
```yaml
builds:
  ios:
    runner: {platform: ios, labels: [xcode, ios-signing]}
    ios_signing:
      p12: "${IOS_P12_FILE}"
      profile: "${IOS_PROFILE_FILE}"
      password: "${IOS_P12_PASSWORD}"
      bundle_id: "{{bundle_id}}"
      export_method: "{{export_method}}"
```
前三项必须完整合法 `${NAME}`，不得 literal password；后两项支持已声明参数/上下文一次渲染，不二次解释插入值。method 仅 debugging/release-testing/app-store-connect/enterprise。TeamID/identity/UUID 从明确材料核对，不选择未知宿主资源。

## 具体生命周期
```go
type IOSSigningOptions struct {
    Workspace, OutputDir string
    P12File, ProfileFile, Password, BundleID, ExportMethod string
}
var ErrIOSCleanup error // 导出 sentinel；errors.Is 可识别，不按任意字符串判断
func ValidateIOSSigning(context.Context, IOSSigningOptions) error
func PlanIOSResources(context.Context, IOSSigningOptions) (*IOSResources, error)
func (*IOSResources) Ownership() IOSResourceOwnership
func (*IOSResources) Prepare(context.Context, IOSSigningOptions) error
func RestoreIOSResources(context.Context, IOSResourceOwnership) (*IOSResources, error)
func ValidIOSResourceOwnership(IOSResourceOwnership) bool
func IOSResourceDigest(IOSResourceOwnership) (string, error)
func PrepareIOSResources(context.Context, IOSSigningOptions) (*IOSResources, error)
func (*IOSResources) Environment() map[string]string
func (*IOSResources) Close(context.Context) error
func HandleIOSHelper(io.Reader, io.Writer) error
```
Validate 使用内存 P12/CMS 校验，不创建 keychain/profile/输出目录。Prepare 再校验当前材料，防预检查后更换。Password 不进入公共数据、argv、日志；仅通过受限匿名 stdin JSON 进入本功能 helper。

OutputDir 是主代理预检查生成的仓库相对唯一随机目录；预检查不 mkdir。Prepare 在实际 run 前排他创建并保存删除所有权。只选 artifact 不创建签名资源；引用 ios.output_dir 但无实际 run 明确拒绝。pipeline 唯一生命周期所有者：整个批次预检查→实际 run 前 Prepare→普通/artifact→用户 post→独立有限预算 Close。跳过/未开始不 Prepare。Prepare 失败半清理也使用独立预算；清理不确定返回 ErrIOSCleanup，禁止后续 build，原失败/取消原因保留。

仅注入不可由用户覆盖的系统键：
- MYBUILDS_IOS_KEYCHAIN、MYBUILDS_IOS_SIGNING_IDENTITY（明确证书 SHA1）。
- MYBUILDS_IOS_PROFILE_UUID、MYBUILDS_IOS_TEAM_ID、MYBUILDS_IOS_BUNDLE_ID。
- MYBUILDS_IOS_EXPORT_OPTIONS、MYBUILDS_IOS_BUILD_DIR（自有外部临时目录）。
- MYBUILDS_IOS_OUTPUT_DIR（本次仓库相对目录）。

模板 artifact 使用 `{{ios.output_dir}}/*.ipa` 与 `{{ios.output_dir}}/*.dSYM.zip`。该 fact 由 Run 系统生成，Preview 无事实 pending、不查询密钥；用户 Facts 不得伪造签名资源。资源环境只注入实际 run，密码和源材料路径不注入子进程。

## 受限 helper 与公共进程依赖
同一客户端二进制仅固定 argv `__ios-signing`，主代理注册隐藏 CLI 入口；本功能实现 Handler。仅 inspect/prepare/close 三个本功能动作，无通用 RPC/hook/interface。有限匿名 stdin JSON（上限 64KiB）传明确文件、密码与控制路径；严格禁止未知字段/多 JSON 对象。stdout 上限 16KiB，只含安全结构或固定原因，不输出材料/密码/原生错误正文；stderr 不做公共日志。

当前已验收的 internal/process 保持不变；主代理仅追加 `process.Command.Stdin []byte` 和 `toolCommand.Stdin []byte`；只有非 nil 时才设置 `cmd.Stdin = bytes.NewReader(command.Stdin)`，nil 保持旧行为。本区复用真实 toolOutput/process，父操作预算15s，Close 独立预算15s。父层在启动前持有并持久化自有资源目录/目标路径。只有可靠叶身份才可调用cleanup helper；准备中崩溃且缺叶身份不能仅凭父目录删除未知keychain。同步 cgo 仅发生于该受管理进程，不能宣称 Go context 可直接打断 cgo。

partition 的 `/usr/bin/security -i -q` 是 helper 内固定工具子进程，匿名 stdin 单条命令，继承 helper 的进程组；外层既有 process 同组取消，不新增自己的进程组或执行器。TestMain 可识别固定 helper argv 调用真实原生 Handler；不构建生产测试开关或 stub。

## Profile 真实性与所有权
CMS 每个 signer 必须密码学签名有效；public BasicX509 + SecTrustSetAnchorCertificates/Only(true) 仅接受内嵌、固定SHA256的 Apple 官方DER根，禁止网络fetch与宿主自定义根。不调用 private provisioning policy、不以 CN 作为授权；profile signer leaf 以公开 DER 检查 Apple profile marker OID 与签名用途。自签 profile 拒绝；合法 Apple 链及具体 marker 对实际用户材料的兼容性需要真实验证。继续检查 profile期限、UUID、TeamID、Bundle ID/export类型、DeveloperCertificates 与指定 P12。

profile 使用当前 Xcode 自有目录内随机排他副本，记录文件身份与摘要；被替换/符号链接/未知文件不删除，报清理失败。构建、keychain和输出目录排他创建且记录目录身份；删除前复核目录/父路径，禁止越界删除。系统 keychain default/search list 不改写、不扫描未知身份材料。

## 当前基线的纯组件及消费者边界

`config.Build.IOSSigning *config.IOSSigning` 为可选字段；`IOSSigning` 只含 `P12, Profile, Password, BundleID, ExportMethod string`，YAML 名称分别为 p12/profile/password/bundle_id/export_method。解析沿当前 Node 严格结构检查，显式 null/空 mapping/缺字段/未知或重复键/错误 scalar 类型均拒绝；原因不得回显输入值或引用名。前三项仅完整合法 `${NAME}`，解析/Validate/Preview 不 lookup 环境。后二项使用现有一次模板语法，literal BundleID 为合法反向域标识，method 限上述四值；渲染后的值在实际动作前重新核验。

Preview 继续调用 `config.RenderField`，只增加上述五字段检查及系统 `ios.output_dir` fact 的 pending 语义。Caller Facts 中该键必须丢弃；无系统输出 fact 时不猜路径、不准备资源。报告是 build 层公开路径，不允许借 ios.output_dir、workspace 或 step.name 引入私有资源路径。

匿名 stdin 为固定有限 byte slice，不接受可阻塞的通用 Reader；64KiB JSON 上限在序列化前后核验，15s 实际工具预算不变。mobile 的现有 toolCommand 仅追加 Stdin 并传给同一 process.Command，不创建第二工具 wrapper。`process.TemporaryDirectory(workspace,prefix)` 由 Root 从当前结果目录逻辑提取，保持外部目录与 symlink 检查，供现有 pipeline 与本功能两个实际消费者复用。

远端 Agent 的同一可执行文件需隐藏 __ios-signing 入口；`taskSecrets` 仅为 p12/profile/password 三个明确引用加入所需值，沿原受限 secrets_file/禁止控制凭据规则，无 ambient fallback。诊断不得将宿主身份数量当能力授权。当前 journal 还需记录本次 native 资源所有权及关闭证据：准备外部动作前持久化，恢复不重跑导入/archive/export，清理未知保留 guard；具体私有字段由 Root 在实现该消费者前冻结。process_group_reaped 只证明物理进程停止，不能覆盖原生资源 Close 未确认。Store 事件固定原因与 Started/StopKnown 由 Root 接现有协议，不新增伪动作/终态。

纯配置公开具体校验入口为 `config.ValidateIOSSigning(signing *IOSSigning, field string) error`；nil合法。配置Validate、纯Preview及后续Run同一规则，field仅由消费者提供固定安全路径；渲染后只校验actualliteral，不二次解释插入值。

## 实际Agent关闭checkpoint

IOSResourceOwnership仅进入私有journal，含Version=1、Token、Workspace/Output/Temporary/Keychain/Profile、真实OutputIdentity/TemporaryIdentity及可选KeychainIdentity/KeychainDBIdentity/ProfileIdentity、ProfileSHA256和Preparing/Prepared/Closed。Identity为Device/Inode/Mode。Ownership返回独立副本；Restore逐一比较真实身份及profile内容摘要，当前unknown叶或替换目录不授权。已关闭原checkpoint无需重新创建已删除目录。

RemoteOptions.IOSCheckpoint func(mobile.IOSResourceOwnership) error只有Agent具体journal消费者；nil仅本地合法，远端实际签名run没有此消费者在整批预检查拒绝。失败保存不允许继续native准备；关闭失败保留原步骤原因，BuildRun.CleanupFailed独立为true，禁止后续build。

ExecutionProgress的IOSCleanupConfirmed bool、IOSResourceDigest string只build_finished可用，均omitempty；摘要为关闭后的Ownership JSON SHA256，不含原材料或正文。StopConfirmation同字段，stop receipt保存并按原完整Ref精确重放；signed snapshot禁止单纯process_group_reaped解除native未知。未启动native的precheck/checkout/condition分支无资源，关闭已知true且摘要空；实际普通run启动过的签名终态需非空摘要。Agent丢终态ACK恢复同时核完整原progress摘要与私有Ownership摘要，不重跑资源/动作。

CLI build confirm-stopped增加可选--ios-cleanup-confirmed，仅管理员实际观察本次原生资源已关闭后声明；不是force或自动清理工具，原完整Ref/note均必填。IOS资源不确定时不得声明。
