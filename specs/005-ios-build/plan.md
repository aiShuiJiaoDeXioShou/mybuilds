# 实施计划：005 原生 iOS 构建

**Branch**: `005-ios-current` | **Date**: 2026-10-04 | **Spec**: [spec.md](spec.md)

## Summary
当前移植基于已验收fee97e8，沿用唯一Run/process/Agent、003快照及004工具helper。新增iOS doctor、可编辑run/artifact模板与显式签名资源生命周期；实际文件归属及阶段见下文。完整代码与必要自动检查先交付，真实Apple材料/签名IPA与相关组合由用户人工验收；不把材料缺口变成空实现或真实PASS。

## Technical Context
- Go 1.25，现有 Cobra/YAML/doublestar；不增加 Go 模块依赖。
- macOS 原生 Security.framework/CoreFoundation（Darwin+cgo）内存传入 P12 密码，其他平台/无 cgo 明确不支持签名。
- 本次受影响native/unsigned工具门已实际运行；旧版本曾核对 Xcode 27.0（27A266a），具体本次命令见validation；导出当前 method 为 debugging/release-testing/app-store-connect/enterprise，destination=export，manual，manageAppVersionAndBuildNumber=false。
- 标准 testing，原生临时 keychain 真负例、自制测试身份非交互签名、参数/版本/材料验证、unsigned 编译；真实授权 IPA/dSYM 材料暂缺，转为用户人工验收，不阻塞完整代码及必要自动检查交付。
- P12 上限 16MiB、profile 上限 2MiB，只读取明确传入的文件。CLI 密码只用完整环境引用，秘密不进入 argv/日志。
- 安全原型通过后才接入系统生命周期；参考 [research.md](research.md)、[Go 契约](contracts/go-api.md)。

## Constitution Check
I：完整 Spec Kit、真实验证条件保留；II：单模块，pipeline 唯一执行及系统生命周期所有者；III：系统框架/既有依赖，不预建插件或另一执行器；IV：显式凭据、秘密不出 argv、自有资源清理、不改用户 default/search list；V：中文与真实行为证据。设计仍遵守全部原则。

## Project Structure 与文件归属（2026-10-05 移植修订）

本次 worktree 为 `ios005-current`，分支 `005-ios-current`，基于已验收 `fee97e8ea32fc4f582abfb445c8d33f690f6f70e`。旧 `ios005-pending` 的源码及证据原字节保留；旧验证记录只说明历史版本，不代表当前移植通过。没有新增 Go 依赖。

- A 唯一写入本worktree中的005所有真实实现、测试、README及功能文档；Root已完成纯Preview/stdin/临时目录最小接线并明确释放这些文件，后续没有并发writer。最终Root只取005相对基线的最小差异串行合并020，不覆盖020结果注册、删除保护与恢复实现。
- 执行仍使用当前Run/process；新增具体私有IOSCheckpoint消费者，准备前持久化原生资源所有权。Agent恢复/Store停止确认增加仅本功能必要的可选关闭证据与摘要，旧nil配置不改变wire JSON/digest。不得复制旧Run/process/Agent文件覆盖当前实现。
- 全部外部工具仍复用当前 `process.Run`，保留 OnStart、进程出生身份、逃离进程组检测、累计预算及清理不确定闭锁。禁止新增执行器、签名插件 registry/interface 或通用生命周期 hook。

## 实施阶段与接入依赖

1. 在现有规范上重新 plan/tasks/analyze，保存真实 selector/template/hooks 记录。T001–T014 的勾选为旧版本历史，新增移植任务未验证前保持未勾选；T008/T010/T011/T012–T014 不因此转为通过。
2. 先完成 T015–T017 纯组件：五字段 schema/严格解析、已声明参数与现有 `config.RenderField` 单次渲染、纯 Preview、无环境读取/无进程/无资源。009 T003 仅消费该同一组件，不重复 IOSSigning 定义，不等待完整 Apple 签名验收；组件阶段实际 Run 入口仍明确拒绝尚未接入的 ios_signing；全部安全消费者真实接入并自动检查通过后开放，不等人工Apple验收。
3. A接Root已落地的T018匿名 stdin 与当前共享临时目录；A T019 移植 mobile 到真实公共 helper。只有 Root 已同步实际依赖才编译；不写临时 stub 或绕过工具执行器。明确工具窗口已放行；自动验证仅用自产材料和隔离资源，不访问未授权Apple身份。
4. A T020 接当前唯一 Run/Agent：整批预检查、首次实际普通 run 前 Prepare、普通/artifact、用户 post、独立 Close；elapsed 与确认延迟均消费现有预算，清理不确定不得宣称 StopKnown。T021 接具体资源所有权 journal/恢复与 Node 诊断，签名环境只取本任务明确 secrets。远端入口在实际恢复与权限检查通过前保持拒绝，不据 runner 或宿主身份数量假报能力。
5. T022 在明确窗口复验非 Apple 的真实机制与无签名工程；T012–T014提供明确Apple材料/工程的可执行人工指南，实际IPA/dSYM与相应组合待用户验证。T023/T011在完整实现、必要自动检查及人工步骤就绪后允许整功能本地提交；人工待验不得标成真实PASS。工具存在、自签测试证书、unsigned archive 和纯组件交付均不替代真实签名结论。

## 关键设计决策
- build.ios_signing 五个明确字段：p12/profile/password 完整环境引用，bundle_id/export_method 明确标识/参数模板；不根据 runner 自动导入。
- 系统环境由资源函数返回，仅注入本次运行；密码及源文件引用名不注入 Xcode 子进程。
- 唯一本次仓库相对 OUTPUT_DIR 通过 `{{ios.output_dir}}` 供 artifact；预览 pending，不生成真实目录；路径由 Run 预检查生成但不创建；Prepare 排他创建。artifact-only 不准备，引用该上下文却无 run 拒绝。
- 原生同步 API 仅在同一二进制隐藏 __ios-signing helper 内运行，通过既有 process 组与有限匿名 stdin 受控；父层持有排他路径/资源所有权。正常及半准备失败的系统清理独立15s；ErrIOSCleanup 标记清理不确定，保留原原因并禁止后续 build。禁止 goroutine 弃置仍在运行的 cgo，禁止第二执行器。
- CMS signer 密码学签名、public BasicX509 与内嵌固定 SHA256 Apple DER roots（anchorsOnly、禁网络fetch）、profile用途marker校验必须通过；自签 profile 明确拒绝，不以 private policy/CN 授权。当前实际 Apple 材料兼容性待真实验证；Xcode导出和codesign/Bundle ID/版本核验仍是SC-002必要条件。

## Complexity Tracking
无原则豁免。Darwin cgo 是密码不进入 argv 的必要系统调用边界；匿名 stdin partition 命令复用 Apple 已有逻辑，不使用私有 Security API。

## 交付标准修订

用户2026-10-05最新授权覆盖旧的“无真实Apple材料不得提交”限制。继续完整实现所有本功能代码及当前Run/Agent消费者，必要安全/恢复/取消自动门必须通过；真实合法Apple链、archive/export发现临时identity与签名IPA/dSYM明确人工待验。开发交付可本地提交，不因材料缺失预留空方法或提前成功。必要自动门范围由变更决定，不重复已绿纯组件suite。

## 具体恢复与停止边界（实际消费者）

PlanIOSResources只验证选项、创建排他空output/temp并决定随机keychain/profile目标；Ownership包含版本、随机token、路径及真实Device/Inode/Mode，不包含P12/profile原路径或密码。Agent具体IOSCheckpoint在任何native import/profile写之前保存Plan和Preparing intent；Prepare完成或安全失败后再保存实际叶身份/profile摘要。prepare中崩溃而无可靠叶身份不猜测归属，Restore/Close闭锁；可验证计划在重启时只Close，不重跑导入或脚本。Close后checkpoint的canonical SHA通过终态IOSResourceDigest绑定；只读终态恢复重算摘要后才调用原精确TerminalReceipt。

ExecutionProgress仅build_finished可声明IOSCleanupConfirmed及IOSResourceDigest；非签名快照不接受这些声明。Store从冻结BuildSnapshot识别签名定义：缺Close证据只能保持interrupted/stop_unconfirmed，不将所有进程组StopKnown代替原生Close；独立StopConfirmation也必须明确原生关闭。两个字段omitempty保旧wire不变，stop_confirmations只追加默认false/空摘要列，不改原receipt内容。普通run准备失败沿既有intent→finished(start_error，Started=false)→inactive post，原失败保留。用户post之后Close独立15s，不受普通/post取消预算取消；未知清理禁止后续build/节点行动。

节点ios_signing passed仅说明Darwin+cgo/macOS15原生组件可用且真实Xcode检查passed，不代表材料授权。调度ios需要Darwin与xcode/ios_signing能力；非iOS运行不查签名材料。已验证TeamID只从真实资源Environment取到BuildRun私有iosTeamID，供后续同一发布消费者使用，不从用户env取值。
