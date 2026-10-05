# 实施计划：Flutter 双平台构建

> 2026-10-05用户最新交付边界：先完成完整可执行代码与必要自动检查。005签名API只等真实代码冻结交接，不等合法Apple材料人工验收；真实签名／双平台／最终联验保留全部原要求，由用户最后统一人工验收并独立记录pending。源码集成或提交不宣称这些门PASS。无stub、第二Run、宿主未知身份或安全降级。

**分支**：`009-flutter-builds` | **日期**：2026-10-04 | **规范**：[spec.md](spec.md)

**输入**：已冻结 28 FR、8 SC、4 用户故事、19 AC；需求质量 16/16。本轮已进入可执行源码实施；组件依赖按真实代码交接推进，完整签名与双平台门保留为最终人工待验。

## 摘要

单个 YAML 生成 Flutter Android／iOS 命名 build，全部仍由 `pipeline.Run` 执行普通 run/artifact。Flutter/Dart 体检、参数及平台工具在整批用户动作前校验。Android 调用 Flutter appbundle/apk，显式 Gradle 签名，独立核验实际版本、证书和本次产物；iOS 先配置 Flutter，再复用 005 的手动 archive/export、临时 keychain、profile 与独立 Close。节点沿现有身份、fence、预算、日志、产物流，只扩展实际消费的框架标志及工具集合。

## 技术上下文

- **语言／项目**：Go 1.25 单模块、三 CLI；中文文档与注释，无新 Go 库。
- **现有依赖**：Cobra、严格 YAML、现有标准库及 process/mobile/pipeline/protocol/store；不新增执行器、插件框架、repository 或模板 DSL。
- **实际工具基线**：Flutter 3.38.6 stable、Dart 3.10.7，framework commit `8b872868494e429d94fa06dca855c306438b22c0`、engine `78fc3012e45889657f72359b005af7beac47ba3d`。示例锁 Gradle 8.14、AGP 8.11.1、Kotlin 2.2.20、SDK 36、NDK 28.2.13676358、JDK 17+；iOS 复用 005 的 macOS 15+/现代 Xcode SDK/cgo，CocoaPods 1.16.2。锁定验收基线不等于已验证所有新旧 SDK。
- **持久化**：不新增数据库表。冻结 Definition 保留 `runner.framework`，工具沿 NodeReport.Tools；构建号、进度、报告、产物继续使用前置功能的记录。
- **目标环境**：真实 Linux/macOS Flutter Android；有已验收原生签名支持及合法材料的 macOS Flutter iOS。Windows 保持远程客户端能力，交叉编译不证明工具可执行。
- **测试**：实际模板/配置/纯预览、真实有界工具负例，真实 Flutter 工程默认及非空 flavor，独立 APK/AAB/IPA/归档/dSYM 检查，本地与远程取消/失权/清理/下载；全量 test/race/vet 与三入口跨编译。
- **性能与限额**：沿前置纳秒累计预算、post 独立预算、系统清理预算、HTTP/日志/产物限制；体检复用 15 秒单工具与 32 KiB 合并输出。虚拟机吞吐慢不能跳 lint、R8、签名或修改活动任务预算。
- **约束／规模**：一个真实单 app 工程、默认与一个预配置 flavor、单/双 build；不自动安装或升级工具、不接受新许可证、不改变用户签名/账户/search list、不猜多 target/profile 映射。

## 原则检查

constitution 2.1.0，Phase 0 前与 Phase 1 后均检查。

| 原则 | 设计检查 | 结果 |
|---|---|---|
| I 规范驱动 | 按组件实际依赖推进 plan/tasks/analyze；独立开发不使用未冻结接口，完整验收与提交门不变 | PASS |
| II 单模块多节点 | 本地与 Agent 唯一 Run，单控制端；iOS 必须 macOS 与签名能力 | PASS |
| III 最小实现 | 普通 run/artifact、两个内置模板、具体工具检查、一个 runner 字段；无新 Go 依赖/数据库表/框架 | PASS |
| IV 执行边界 | 整批预检查、明确密钥引用、可信远程编号、原 fence/预算/停止保护/独立资源清理 | PASS |
| V 真实验收中文 | 真工具、真双平台产物、大小/hash/证书/取消证据；缺 Apple 材料不标完成 | PASS |

无原则例外。本轮由主代理按 MVP_EXECUTION 的缺环境条款明确修正组件启动边界；原 FR/SC/AC 与真实门不变。checklist 的旧启动句仅保留为历史阶段记录，当前权限与前置以 spec 修正和下表为准，清单原字节不改。

## 前置与启动门

| 前置 | 当前已知状态 | 009 的实际消费与门 |
|---|---|---|
| 004 | 已验收 `2ab8991` | Android doctor、显式测试签名与原有产物实证 |
| 005 | 待真实 Apple 验证；纯配置/预览组件由005唯一writer在当前基线移植、冻结并验证 | 独立 Flutter/Android 不等待005；iOS模板的严格Parse/Preview消费已交接的纯组件，不在009定义第二套IOSSigning。Validate/Prepare/Close、材料doctor、签名能力与真实iOS执行须005真实代码接口冻结交付 |
| 007 | 已验收 `85b46bfaccb45c4626fcffbbcb526b2f7f8c9301` | NodeReport、实际能力匹配、冻结 task、唯一 Run、进度/日志/产物与停止保护 |
| 008 | 已验收 `504dc6fa8581f74a15ecc146a474976d5ae33a22` | 保留 framework/定义/参数的原快照 retry；恢复不重启 Flutter/签名/user post，不增长预算 |
| 019 | 已验收 `fee97e8ea32fc4f582abfb445c8d33f690f6f70e` | 编辑模板的真实测试报告沿同 Run/封存边界，post 不改已确认报告，不另建 Flutter 报告格式 |
| detached-gradle-stop及后续停止修复 | 已随008正式交付；原发现留历史证据 | 正式 Flutter 取消门须以修复并验收后的进程基线验证独立 Gradle daemon；最终 PID gone 不代替 StopConfirmed 当时事实 |

独立组件源码 worktree 从当前已验收 fee97e8 基线重新建立；本规划 WT 的8e1397e不用于实现。根按组件manifest同步真正通过检查的005纯配置/预览，不借其dirty签名源。未交付005纯组件前，Android初始化/预览/工具诊断先做，iOS或双平台模板Parse/Preview的组合门明确pending；不能用空方法或临时IOSSigning类型消除依赖。无材料iOS工具诊断只检查工具，显式签名请求与生效iOS执行在整批用户动作前固定unsupported且不读秘密。005实际代码未交接不报告ios_signing passed、不授iOS任务。009完整实现、集成和代码提交不再等待合法材料人工门；两OS Android与合法iOS本地/远程真实全门独立记录待用户最后统一验收。

### 组件执行门（不改变最终验收范围）

| 组件 | 实際前置与对应任务 | 完成边界 |
|---|---|---|
| Flutter/Dart、Android初始化/参数/工具/模板/本地构建 | 已验收004/007/008/019；T004–T024中Android/公共工具部分 | 可先红绿与实际两OS Android验收；不借005 dirty、不等待iOS材料 |
| iOS工具-only诊断 | 同一FlutterDoctor及既有process；T011/T013/T014工具部分 | 实际Xcode/pod，签名skipped/unsupported，不调用未交付IOSDoctor |
| iOS/双平台模板严格Parse/Preview | 005唯一writer已冻结验证的纯IOSSigning配置/预览manifest；T008/T010/T012/T014–T016/T025纯组合部分 | 消费既定五字段，不创建第二套类型；只纯检查，不证明签名 |
| iOS签名和节点能力 | 005真实签名接口冻结交付；合法材料用于最后人工门；T026/T028–T030/T031–T034的签名部分 | 真Validate/Prepare/Close，iOS只授权macOS节点；以前不声明占位API、不放行 |
| Android节点/冻结输入/retry/reports | 已验收007/008/019及真实FlutterDoctor/Android Run；T031–T036的Android部分 | 可独立检查；不等iOS子项，未完成任务子项不伪勾整任务 |
| 整功能 | 全部组件+所有默认/flavor/双平台/取消/秘密/下载门；T037–T044 | 源码converge与一次功能提交以可执行实现及必要自动检查为门；真实签名/双平台联验保留独立人工待验，不冒称PASS |

## Phase 0 研究与 Phase 1 设计

[research.md](research.md) 记录官方依据、已安装只读证据和替代方案；设计选择已明确，无待用户澄清项。网络下载与真实 Apple 材料属于实施/验收前提，不能用规划检查冒充已准备。

[data-model.md](data-model.md) 定义参数、框架/能力、签名生命周期与证据。共享 API 见 [go-api.md](contracts/go-api.md)，命令/配置见 [config-cli.md](contracts/config-cli.md)，模板及真实产物规则见 [templates.md](contracts/templates.md)，验收步骤见 [quickstart.md](quickstart.md)。

### 最小实施顺序与所有权

1. 根先核004/007/008/019已验收基线，冻结 `Runner.Framework`、具体 mobile API（FlutterDoctor 的 Platforms/Environment）、doctor/init 选项与共享工具环境边界；Android配置/纯预览及初始化真实红绿先行。005纯IOSSigning配置/预览单独交接后才接iOS/双组合校验；签名API仅005实际代码交接后冻结，暂不声明空函数。
2. A 独占 `internal/mobile/flutter.go`、`flutter_test.go`、`templates/flutter-{android,ios}.yml` 与 `examples/flutter/**`；实现 Flutter/Dart 实际体检、Android模板和真工程；iOS模板纯文本可先准备，其Parse/Preview等005纯配置交接，签名运行等005实际代码交接。不改 native 签名/第二执行器。
3. 根唯一写 `internal/config/{types,parse,validate}.go`、相关配置测试、`internal/cli/client/{init,doctor}.go` 与测试、`internal/mobile/doctor.go` 的确切 Env 消费、`internal/pipeline/{run,preview}.go` 与 Flutter 预检查/编号/共享和命名参数覆盖测试；依 005 已验收 API 接资源，不复制资源实现。
4. C 的独立分区经根指定，独占 `internal/agent/doctor.go` 与测试、`internal/store/{node_session,lease}.go` 与框架匹配测试；沿原协议增加 flutter/dart/cocoapods 工具名，无新消息/状态表。共享文件分区在 tasks 前最终冻结，禁止同文件并写。
5. 根串行集成，每批 SHA 核对并用真实消费者复验；先本地 Android/macOS，再真实 Linux Android与远程，再合法 iOS 本地/远程与取消，最后 008/019 联验。可执行实现及必要自动检查后一次代码提交，真实门独立标人工待验，不自动 push。

CLI/移动资源/Store 接线共享文件由根串行协调；README、实施历史、最终 validation、tasks 和依赖仍根唯一所有。本轮只写本功能spec状态/依赖、七份规划、tasks及validation；checklist保持原字节。

## 需求覆盖与完成门

| 故事 / AC | FR | SC / 真实门 |
|---|---|---|
| US1 / 5 | 001–007 | 001/002/007/008：三种 init、拒覆盖/冲突、纯预览零动作、真工具逐项负例 |
| US2 / 5 | 008–013、018–020 | 003/004/007/008：两 OS 的签名 APK/AAB、默认/flavor、版本/证书、mapping 与错误参数/旧文件 |
| US3 / 4 | 014–020、025–027 | 003/005/007/008：合法 Apple IPA/归档/dSYM、整批拒错材料、Prepare/post/Close 真实组合 |
| US4 / 5 | 018–028 | 005/006/007/008：节点能力/编号/fence、真实取消、封存证据/中央下载与 008/019 联验 |

28 FR、8 SC、19 AC 全保留；此次重新生成组件有序任务并正式只读 analyze。测试范围不能用 mock Flutter、交叉编译、原生预检输出或既有未验收 iOS 代码代替。

## 目录结构

```text
specs/009-flutter-builds/
  spec.md / checklists/requirements.md  # 范围冻结；仅spec状态/前置修正，checklist不改
  plan.md / research.md / data-model.md / quickstart.md
  contracts/go-api.md / contracts/config-cli.md / contracts/templates.md
internal/mobile/flutter.go / flutter_test.go
internal/mobile/templates/flutter-android.yml / flutter-ios.yml
examples/flutter/                      # 实施阶段最小真实工程、锁文件与接入文档
```

其余共享源码按上表由唯一 owner 修改；不创建额外框架包、数据库层或运行目录。tasks.md在plan后由speckit-tasks更新；validation.md记录本次组件修正及未执行门，后续由根接管。

## 复杂度记录

无原则违例。框架字段用于真实调度，具体参数/体检 API 用于已有 CLI 与 Run。BuildParams 复用 remote 已存在的命名参数语法，解决双 build 不同必填字段，保持旧共享覆盖语义。Android 产物验证使用已锁 AGP 的官方 bundletool 模块和 SDK/JDK，不增加 Go 解码器或工具安装器；iOS 使用 005 的唯一资源实现。

## 工作流证据

2026-10-04 实际读取 README、BUILD_DISTRIBUTION、SPECKIT_ROADMAP、MVP_EXECUTION、constitution、005 pending 契约/验证、root 007 源码及 008 契约。ignored `.specify/feature.json` 选择本目录；`setup-plan.sh --json` 返回 `/private/tmp/.../specs/009-flutter-builds/plan.md`，分支 `009-flutter-builds`。plan-template 经 resolve-template 的 core `.specify/templates` 层复制；before_plan/after_plan 为 `hooks: {}`，无可执行 hook。未执行 tasks/analyze/implement 或修改应用源码。2026-10-05 完成文档复核：主代理确认本地 BuildParams 复用命名覆盖，以及 FlutterDoctor 严格 Platforms/受限 Environment 用于 CLI 与 Run 两消费者；Run 预检不调用仓库 wrapper、不初始化 Flutter。

2026-10-05 组件前置修正：再次setup-plan解析现有plan，core模板resolver/no preset/hooks={}；本轮只文档，旧plan未tasks等句为历史记录。正式依赖004/007/008/019已验收，005纯组件和签名整功能分开交接，两个门均不能伪造。
