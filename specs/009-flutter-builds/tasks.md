# 任务：Flutter 双平台构建

> 2026-10-05用户最新交付边界：先完成完整可执行代码与必要自动检查。005签名API只等真实代码冻结交接，不等合法Apple材料人工验收；真实签名／双平台／最终联验保留全部原要求，由用户最后统一人工验收并独立记录pending。源码集成或提交不宣称这些门PASS。无stub、第二Run、宿主未知身份或安全降级。

**输入**：本目录冻结的 spec、plan、research、data-model、quickstart 与三份 contracts；28 FR、8 SC、4 个 P1 用户故事、19 AC。按项目 core tasks-template 组织。

**当前范围**：已进入源码实施。Flutter3.38.6/Dart3.10.7空工程研究不是签名证据；004/007/008/019基线已验收。005纯配置和实际签名API分别按冻结代码与自动检查交接后消费，不能stub。用户最后统一人工签名／双平台联验，当前缺材料不再阻塞完整代码实现、集成或提交；未运行的真实门独立pending。


**测试规则**：规范 FR-026 与原则 V 要求非平凡逻辑、输入、鉴权、事务和取消的真实行为检查。以下代码任务先运行对应红测，再最小实现转绿；编译、模拟 Flutter 输出和预置产物不能替代真工具门。任务完成由根记录，不按任务或批次提交。

## 格式与唯一所有权

`- [x] T### [P?] [US#?] 描述及路径`；`[P]` 仅表示同一已满足前置下文件不重叠的工作。阶段内红测先于对应实现；集成后的测试运行仍须协调实际进程域和 VM 窗口，不能因 `[P]` 并发注入取消故障。

| 分区 | 唯一 writer / 文件 |
|---|---|
| 根 | `internal/config/**`；`internal/cli/client/**`；`internal/mobile/doctor.go` 及原工具 helper 测试；`internal/pipeline/**`；`internal/mobile/ios.go` 的实际可用性消费（以验收后 005 实际路径为准）；`internal/protocol/**` 如固定工具校验由其承载；`README.md`、`docs/plans/**`、本功能 tasks/validation、依赖与串行集成 |
| A | `internal/mobile/flutter.go`、`internal/mobile/flutter_test.go`、`internal/mobile/templates/flutter-android.yml`、`internal/mobile/templates/flutter-ios.yml`、`examples/flutter/**`，覆盖具体 Flutter API、模板与自有真工程；不改原生资源实现 |
| C | 根在 T003 明确移交后独占 `internal/agent/doctor.go` 与对应测试、`internal/store/node_session.go`、`internal/store/lease.go` 与专属框架测试；不与根或其他功能同时修改这些文件 |

2026-10-05最新实施移交：根授予engine003在fresh `flutter009-current`独占全部009 source/docs/README接线；上表为原划分，当前只有这一writer在该WT修改009。005资源实现仍归005唯一writer，020仍归其各owner；根负责最终最小diff串行集成、联合验证与提交，不共享dirty签名源码。

各独立源码分区从 T002 核对的已验收fee97e8基线创建 fresh worktree，不沿用本规划 WT 的 8e1397e 源。A/C 交付冻结 SHA，根顺序复制并用实际消费者复验后解除冻结。新增测试文件随所属目录同一 writer；跨分区测试需要依赖真正交接，不造假接口。root-only 文件不能因任务标 `[P]` 分给第二 writer。

## Phase 1：准备

- [x] T001 在 `specs/009-flutter-builds/validation.md` 由根记录冻结输入 SHA、实际 selector/preset/core template/hooks、28 FR/8 SC/19 AC 清单及当前前置状态；只记录研究工具版本，不声明空工程已通过签名门。
- [x] T002 在 `specs/009-flutter-builds/validation.md` 核验004/007/008/019及停止修复正式提交，为独立Flutter/Dart/Android/工具-only iOS建立fresh源码worktree；分别记录005纯配置/预览和实际Validate/Prepare/Close接口的冻结代码与自动检查交接。代码尚未交接只阻塞其真实consumer，不引入替代实现；合法Apple archive/export与成功/失败/取消完整签名门在T029/T030/T037–T039保留人工待验，不再反向阻塞T042–T044源码交付。
- [x] T003 在 `specs/009-flutter-builds/contracts/go-api.md` 与 `plan.md` 由根核对验收基线的实际 API/文件及上表归属，先冻结FlutterTemplate/FlutterDoctorOptions/ValidateFlutterParameters/BuildParams和固定工具名的实际模板、CLI、Run、Claim消费者；纯IOSSigning五字段由005 config唯一writer冻结验证后按manifest同步，不在009预定义第二套类型。IOSDoctor、IOSSigningAvailable、Validate/Prepare/Close签名API等005真实签名接口冻结交接后再冻结接入；未实现路径固定unsupported、不读取秘密、无stub。前置API变化先修设计并只读analyze，无新消息/表/执行器/依赖。

## Phase 2：共享基础（阻塞全部故事源码）

- [x] T004 在 `internal/config/flutter_test.go` 先做真实解析红测：`runner.framework` “仅省略/native/flutter；省略等价 native”，Flutter “Platform明确android/ios”；unknown/type/null/重复/alias/merge 及现有限额全部拒绝，已有runner仍须platform，旧无runner generic保持合法。
- [x] T005 在 `internal/config/types.go`、`parse.go`、`validate.go` 最小接入 `Runner.Framework`，沿既有严格 YAML 边界与固定安全错误，不读 env 或调用工具；T004 转绿并验证冻结 Definition 保留字段。
- [x] T006 在 `internal/mobile/doctor_test.go` 先做真实工具红测：显式 `toolCommand.Env` 路径查找、九项宿主白名单、nil 保持旧行为、并行任务不全局 Setenv、15 秒/32 KiB 合并输出和写失败/取消实际组回收；不回显参数、env 或原始工具输出。
- [x] T007 在 `internal/mobile/doctor.go` 最小实现具体 Env 消费，复制调用方 map、拒非法环境名/缺失/NUL，显式 PATH 查找，继续使用唯一 `process.Run`；真实 cleanup_error 闭锁后续工具，T006 及既有 native doctor 门转绿。

**Checkpoint**：T001–T007通用组件基础完成后开始独立故事源码。T002按组件检查而非整项等待；005纯配置与签名两个专门门由根实际交接，US1纯预览不能冒充签名验收。

## Phase 3：US1 — 初始化并检查 Flutter 配置（P1）

**目标**：三种平台选择生成单 YAML，体检只检查明确目标，预览零外部动作。

**独立测试**：实际 CLI init 的三个组合与拒覆盖/冲突；完整双 build 参数纯预览；真实有界工具正负例及 SDK/用户配置前后不变。签名未声明时必须单独 skipped。

### 先写红测

- [x] T008 [P] [US1] 在 `internal/mobile/flutter_test.go` 写模板组合红测：android/ios/两者恰好对应 build、根 version 一份、平台空/重复/未知拒绝、只含普通 run/artifact；默认参数 string 值 `1.0.0/internal/空/1`，Android 与 iOS 必填字段互不混入；iOS/双组合typed Parse/Preview仅待005纯配置manifest可运行，未交接不提供替代类型。
- [x] T009 [P] [US1] 在 `internal/cli/client/flutter_test.go` 写 init/doctor 选项红测：原 default/native/custom 行为、排他创建不覆盖字节、framework/platform 与 template 冲突、远程 doctor 与本地参数冲突、跨无关平台签名 flags 和部分材料固定拒绝；不读取未知宿主身份。
- [x] T010 [US1] 在 `internal/pipeline/flutter_test.go` 写纯预览/选择红测：单/显式多 build/全选/step、双 build 未选择拒绝、when 保持；`BuildParams` 为“声明默认<共享Params<该build命名覆盖，再ResolveParams”，重复/未知/未选择 scope 与共享未知参数整批拒绝，预览工具/secret/网络调用和工作区/结果创建均为零。
- [x] T011 [US1] 在 `internal/mobile/flutter_test.go` 写真实 Flutter/Dart 体检红测：同 SDK 配对、选择外缺工具不失败、未声明签名 skipped；已安装/缺失/不可执行/坏版本/取消/超时及缺缓存拒初始化；有空格路径、解析安全数字点 Version、SDK/用户偏好前后不变，不用 fake 工具冒充可执行能力。

### 实现及故事验证

- [x] T012 [US1] 在 `internal/mobile/flutter.go` 实现具体 `FlutterTemplate(platforms)`，组合顺序 android、ios，复用同一配置模型与本故事模板声明；在 `internal/mobile/templates/flutter-android.yml`、`flutter-ios.yml` 写普通可编辑 run/artifact，后续平台故事完善实际命令，T008 转绿。
- [x] T013 [US1] 在 `internal/mobile/flutter.go` 实现 `FlutterDoctor(ctx, FlutterDoctorOptions)`，Platforms 空只查公共项，非空严格唯一 android/ios；Environment 显式复制，不传签名秘密；核对 stable 已初始化 SDK/缓存再直接实际 bundled Dart snapshot 有界 machine version，不调用可能 bootstrap 的 wrapper，不升级/下载/接受许可证，T011 转绿。
- [x] T014 [US1] 在 `internal/cli/client/init.go`、`doctor.go` 接入模板和具体 doctor，公共项只出现一次，固定名称/Reason 沿 contracts，Android实际签名flags复用004，未声明材料单独skipped；iOS工具-only由FlutterDoctor直接查Xcode/pod，未交付材料路径固定unsupported且不读取秘密，部分/跨平台声明拒绝。005纯CLI/配置交接后完成组合选项/预览，005实际代码交接后才复用真实IOSDoctor材料检查；保留native wrapper原行为，T009分组件转绿不假签名通过。
- [x] T015 [US1] 在 `internal/pipeline/preview.go`、`run.go` 实现单一共享/scoped 参数解析与校验入口，添加已校验 platform/framework 安全预览字段；本地 `internal/cli/client/run.go` 沿现有 remote 的 `build:key=value` 语法接线，不复制第二条件引擎，不从公开 Facts 接受编号或 ios.*，T010 转绿。
- [x] T016 [US1] 按 `specs/009-flutter-builds/quickstart.md` 用真实 CLI 覆盖三个 init、重复/冲突/非法输入、双 build 显式参数 dry-run 与真实 SDK 各项 doctor；在 `validation.md` 记录工具/取消、零 secret/外部动作、private HOME 与 SDK/用户配置不变证据。Android/公共工具可先记录，iOS/双组合Parse/Preview依005纯组件，iOS材料检查依005真实签名接口冻结交付，未满足子项保持pending；只读版本查询successful exit不代替后续构建门。

**Checkpoint**：US1可先交独立Android/公共工具增量；三模板Parse/Preview等005纯配置、材料检查等005实际代码交接后才计五AC完整通过，不能把完整009标完成。

## Phase 4：US2 — Android 版本、flavor 与发布包（P1）

**目标**：同一真工程产出签名 AAB 与 fat APK，版本/编号/变体、证书及可选 mapping 均独立验证。

**独立测试**：macOS 和 Linux 分别实际构建默认与一个预配置非空 flavor，逐包核验版本/签名/hash；坏参数、材料、旧文件和硬编码版本实际失败。

### 先写红测

- [x] T017 [US2] 在 `internal/mobile/flutter_test.go` 写纯参数红测：version “三段非负十进制整数；拒控制/NUL和非法表示”，本地 number “正十进制整数”且 Android ≤2100000000、iOS 不臆造四位上限；未声明标准参数不强补，channel 可有空格/shell 字符，flavor 单个实参，错误只含固定字段原因。
- [x] T018 [US2] 在 `internal/pipeline/flutter_test.go` 写整批 Run 红测：所选最后 build 参数/实际工具非法时前一个用户脚本不启动，flutter 预检查只调用受限具体 doctor、不执行仓库 wrapper；005未交付时生效iOS整批unsupported且不读秘密；仅artifact/全skipped不伪调用准备，正式签名行为在T026验证；生效未交付approval/upload明确拒绝，原生/custom 行为保持。
- [x] T019 [US2] 在 `examples/flutter/android/` 的验收脚本与 `internal/mobile/flutter_test.go` 留可运行反例：release debug fallback、AAB 与 APK 不同版本、未知 flavor、旧输出、R8 声明缺 mapping 和含 shell 字符参数；用真实工程/SDK 执行，不把预置包或模拟 stdout 当红绿证据。

### 实现及故事验证

- [x] T020 [US2] 在 `internal/mobile/flutter.go` 实现 `ValidateFlutterParameters(platform, params, effectiveNumber)`，只校验声明标准字段和实际编号；在 `internal/pipeline/run.go` 整批前检查配置/参数/所选工具/已交付Android显式签名前提，未实现iOS先整批安全拒绝且不启动工具/secretresolver；本地合法声明编号内部形成 build.number，remote 只接受 Task.Number；T017/T018 转绿，不新建执行接口。
- [x] T021 [US2] 在 `internal/mobile/templates/flutter-android.yml` 实现明确 pub get 与 `--no-pub` appbundle/apk release 命令、引用 env/Bash 数组、仅非空 flavor 参数；显式签名引用、不 split ABI，不清未知路径，确定本次 variant 输出新鲜性，不 SDK update、eval、明文 properties 或回退签名。
- [x] T022 [US2] 在 `examples/flutter/` 创建本目标真实单 app/default+flavor 工程与锁文件、显式 Gradle release 签名接入和自定义参数脚本；在 `flutter-android.yml` 独立用 SDK/JDK 核 APK/AAB 身份/版本/编号/预期证书。AAB 使用已锁 AGP 官方 bundletool 模块的固定 init 调用，值经 env，不动态下载 JAR/加 Go 解码器；文档给可选 R8 mapping 声明。
- [x] T023 **人工验收准备**：实现以下验收所需代码并交付quickstart/统一案例；原真实验收由用户最后执行并登记，当前不声称通过。原验收内容：[US2] 在 `specs/009-flutter-builds/validation.md` 记录真实 macOS 默认/flavor AAB+APK、启用 R8 mapping、无 R8 编辑声明与自定义脚本消费三项参数；独立读元数据/证书及逐字节大小/SHA，实际坏 password/alias/JDK/SDK/硬编码版本/旧变体门，T019 转绿。
- [x] T024 **人工验收准备**：实现以下验收所需代码并交付quickstart/统一案例；原真实验收由用户最后执行并登记，当前不声称通过。原验收内容：[US2] 在 `specs/009-flutter-builds/validation.md` 用真实 Linux 可执行 Android 工具复验 T023 同一工程/默认/flavor/脚本和包核验；缺离线缓存真实失败、已准备缓存实际复用，保存源码签名/版本配置和 SDK 前后摘要，无交叉编译或 macOS 证据替代。

**Checkpoint**：US2 五个 AC 和两 OS Android 门通过，Android 产物不能代替 iOS 签名或远程门。

## Phase 5：US3 — 明确 Apple 材料的 Flutter iOS（P1）

**目标**：先 Flutter 配置，再唯一 005 手动 archive/export，生成关联可核验 IPA/归档/dSYM 并独立清理。

**独立测试**：验收后 005 的实际合法材料，在授权 macOS 真工程默认/flavor 成功；全批错材料和六类生命周期组合对照原用户资源与无关进程。

### 先写红测

- [x] T025 [P] [US3] 在 `internal/mobile/flutter_test.go` 写iOS模板纯文本红测（不调用签名API；typed Parse依005纯配置交接）：required project/scheme/bundle、显式 p12/profile/password 引用、export_method 沿 005 默认 debugging/四合法值；config-only/no-codesign 后使用系统隔离输出和手动 archive/export，缺或不匹配 flavor scheme 不猜测，不自动改工程。
- [x] T026 [P] [US3] 在 `internal/pipeline/flutter_ios_test.go` 使用已验收真实 005 API 写实际资源红测：later build 错团队/过期/错 app/错 cert 整批无脚本；有实际 run 才 Prepare，artifact-only/allskip 不 Prepare；半准备/替换自有路径/取消/普通与 post 失败系统 Close 独立有限预算、原 reason 保留，密码不入 argv。

### 实现及故事验证

- [x] T027 [US3] 在 `internal/mobile/templates/flutter-ios.yml` 完成 `flutter build ios --config-only --no-codesign --no-pub` 与明确 flavor/version/number，再沿005冻结模板契约准备明确系统输出env的手动archive/export/ditto文本，实际执行另等005真实签名接口冻结交付；在 `examples/flutter/ios/` 配置自有已共享 scheme/flavor、锁 CocoaPods 与接入说明，生成 IPA/archive.zip/dSYM.zip，不创建另一 keychain/profile 生命周期，T025 转绿。
- [x] T028 [US3] 在 `internal/pipeline/run.go` 与已验收 `internal/mobile/ios.go` 最小接入实际 `IOSSigningAvailable()` 和 Flutter iOS 材料/Prepare/Close 消费；普通累计预算含 Prepare/配置/回执，post 独立、系统 Close 独立，无实际动作不 Prepare、未知清理停止后续，T026 转绿，不修改 005 的原生编号语义。
- [x] T029 **人工验收准备**：实现以下验收所需代码并交付quickstart/统一案例；原真实验收由用户最后执行并登记，当前不声称通过。原验收内容：[US3] 在 `specs/009-flutter-builds/validation.md` 记录授权 macOS 实际默认及非空 flavor IPA、archive.zip、dSYM.zip，独立核 Info/profile/codesign/Mach-O 与 dSYM UUID、预期身份/版本/编号、大小/SHA；缺真实 Apple 材料保持 pending，unsigned 或自产非 Apple profile 不标完成。
- [x] T030 **人工验收准备**：实现以下验收所需代码并交付quickstart/统一案例；原真实验收由用户最后执行并登记，当前不声称通过。原验收内容：[US3] 在 `specs/009-flutter-builds/validation.md` 跑成功、实际半准备失败、编译/导出失败、普通取消、累计超时、post 失败六路径与错材料整批拒绝；核本次 keychain/profile/隔离目录、进程 birth 与 StopConfirmed 当时已停止、用户 default/search list 和无关进程不变，替换/未知明确 CleanupFailed+保护，原原因不覆盖。

**Checkpoint**：US3 四个 AC 必须真 Apple 成功与负门全部通过；保持 005 和全 MVP 门，无材料不能发布“009 真实签名／双平台已验收”；可以完成代码交付。

## Phase 6：US4 — 本地和节点同一取消与证据边界（P1）

**目标**：既有 Agent/Claim/Run 仅消费 Flutter 事实和真正工具，可信远程编号、日志及完整快照同一路径。

**独立测试**：真三入口控制端/Agent/CLI，在授权 Linux/macOS Android 与 macOS iOS 跑冻结任务并下载；SQLite/PG 相同资格套件，实际取消/失权/保存失败无新用户动作，系统清理及诊断快照仍准确。

### 先写红测

- [x] T031 [P] [US4] 在 `internal/agent/flutter_doctor_test.go` 写真Doctor分组件红测：flutter/dart 来自同 SDK，pod 只实际 iOS 需求、native/generic 不因缺 Flutter 失败；005实际代码未交接先保持原ios_signing skipped/unsupported且iOS不Claim；实际签名代码交接后才测真实机制可用能力，不冒称材料授权；cleanup_error 闭锁，固定工具名/安全 Version/Reason。
- [x] T032 [P] [US4] 在 `internal/store/flutter_claim_test.go` 用实际 SQLite/PG 写资格红测：精确 labels+项目授权+当前 node/session/fence；Flutter Android 要 flutter/dart/java/aapt2/apksigner passed，iOS 要 macOS/flutter/dart/xcode/ios_signing/cocoapods；假标签/坏工具/Linux ARM 不可执行/不授权 queued，native/generic 原匹配保持。
- [x] T033 [US4] 在 `internal/pipeline/flutter_remote_test.go` 写冻结输入/预算/日志/快照红测：remote Task.Number 不被本地参数或 Facts 覆盖，独立 build 顺序/step/when，原失败不自动取消下一项，停止/清理未知闭锁；有效用户 cancel 可选 post，失权/保存失败不启动任何 always，完整诊断副本在边界取消/log_error 后保留。

### 实现及故事验证

- [x] T034 [US4] 在 `internal/agent/doctor.go` 先接具体FlutterDoctor的Android/公共工具与工具-only iOS，显式环境/版本/固定名沿NodeReport，无新消息/HTTP；005实际代码交接前ios_signing沿现有unsupported，无availability stub、iOS不授任务。005实际代码交接后再消费真实IOSSigningAvailable，剩余iOS子门由同一C writer完成；在 `internal/store/node_session.go`、`lease.go` 最小扩固定工具校验与 framework 资格条件，T031/T032分平台双库转绿，未满足iOS子门保留pending，不把标签当可执行证明。
- [x] T035 [US4] 在 `internal/pipeline/run.go`、`preview.go` 和相关安全摘要消费者复验/最小接线同一 Run 的可信编号、ordinary/post 纳秒预算、日志与完整 collector；沿真实 Authority/进度保存/StopConfirmed，无第二执行器、hook 或 Flutter 状态机，T033 转绿，保持原失败/清理副标志及普通失败可继续规则。
- [x] T036 [US4] 在 `internal/pipeline/flutter_retry_reports_test.go` 联验已交付 008/019：真实 retry 保留原 framework/Definition/最终参数/条件事实、只新身份编号与新执行证据，恢复不自动重跑；显式 reports 同原格式、真实 checkpoint/seal 与新 attempt，post 不改封存证据，缺报告/测试失败不能向后发布，不预建 Flutter 报告/发布框架。
- [x] T037 **人工验收准备**：实现以下验收所需代码并交付quickstart/统一案例；原真实验收由用户最后执行并登记，当前不声称通过。原验收内容：[US4] 在 `specs/009-flutter-builds/validation.md` 用真实三二进制与授权 Linux/macOS Android、授权 macOS iOS 节点复验默认/flavor、冻结 SHA/实际远程编号、单/多/全选与各独立日志；中央下载 APK/AAB/IPA/归档/dSYM/已声明 mapping，与 node 完整清单字节/SHA 相同，真实缺工具/授权不领，双 build 不强求同号。
- [x] T038 **人工验收准备**：实现以下验收所需代码并交付quickstart/统一案例；原真实验收由用户最后执行并登记，当前不声称通过。原验收内容：[US4] 在 `specs/009-flutter-builds/validation.md` 实际用户 cancel/超时于普通、准备、archive/export、post；采样自有 PID/birth/前后台独立 daemon，StopConfirmed 时已 gone、无关同用户进程活，原失败及 post 原预算不变；租约过期/revoke/日志或进度保存失败禁止新 user/post、系统 Close 保留且 unknown guard 不释放，不依据后来 gone 补写早先停止。
- [x] T039 **人工验收准备**：实现以下验收所需代码并交付quickstart/统一案例；原真实验收由用户最后执行并登记，当前不声称通过。原验收内容：[US4] 在 `specs/009-flutter-builds/validation.md` 实际完整收集后取消/log_error 与回传摘要错误/迟到旧 fence，验证保真实完整诊断、部分/旧文件非成功、普通/post 分离 manifest；本次敏感标记在 doctor/preview/argv/log/public DTO/冻结任务为零，全部逐字节重算安全 evidence。

**Checkpoint**：US4 五个 AC 通过，单一 Run/节点授权/停止保护未退化；不得把本地成功当成远程能力或证据确认。

## Phase 7：最终集成与收敛

- [x] T040 在 `README.md`、`examples/flutter/README.md`、`docs/plans/MVP_EXECUTION.md` 由各 owner 经根串行集成准确运行方式、目录、单/双 init、scoped/shared 参数、显式 doctor、手工缓存/signing 前提、mapping 编辑与未知清理边界；规划/已验收分开，不声称 SDK 自动安装/应用迁移/商店发布或无 YAML 绑定。
- [x] T041 在 `specs/009-flutter-builds/validation.md` 用最终集成源码运行本次必要的目标 test/race/vet（MVP全部模块集成后统一全量）、SQLite/PG 同 Store 资格门、三入口既定跨编译矩阵及原生/通用/自定义模板回归；新失败定位修复后只重复受影响门，不把 compile 当 Flutter/Apple 运行证据。
- [x] T042 **人工验收准备**：实现以下验收所需代码并交付quickstart/统一案例；原真实验收由用户最后执行并登记，当前不声称通过。原验收内容：在 `specs/009-flutter-builds/validation.md` 对 `quickstart.md` 的真实 CLI/工具/默认/flavor/本地远程/取消下载流程做最终逐项证据核验，覆盖下表 28 FR/8 SC/19 AC，记录实际版本、冻结源 SHA、合法材料引用、UTC/命令/脱敏证据路径和缺失门；SDK 研究或干跑不是签名验收。
- [x] T043 对 `specs/009-flutter-builds/spec.md`、`plan.md`、`tasks.md`、`validation.md` 与真实最终代码执行只读 `$speckit-converge`，真实缺口继续 implement/复验/收敛；未运行签名/双 OS 真实门保持人工待验，不把代码交付等同真实验收，结果记录仍根唯一 writer。
- [x] T044 按项目提交技能检查本功能最终差异/暂存，在 `specs/009-flutter-builds/validation.md` 核对可执行实现及必要自动检查完成、人工门明确待验后仅一次本地功能代码提交，包含规范/任务/代码/证据；不按任务提交、不 push、不夹其他功能未验收源。

## 依赖与执行顺序

1. T001→T002→T003核当前已验收fee97e8与组件门，再T004→T005、T006→T007通用基础红绿；不依赖005签名的真实组件可以启动。005纯配置只守iOS/双模板Parse/Preview与相应CLI组合；005签名守实际资源/材料/能力和全部最终签名门，不能用跨WT dirty代码或stub补前置。
2. US1：T008/T009 独立红测可并行；T010/T011 写红后，T012/T013→T014，T015 消费 T005；T016按组件等T012–T015实际consumer；Android/公共工具独立，iOS/双Parse/Preview等005纯配置，材料检查等005实际代码交接。A 的 flutter.go/tests 单 writer，不同时跑 T008 与 T011 写同文件。
3. US2：T017/T018/T019 红门先行，T020→T021→T022→T023→T024；仅消费US1的Android/公共工具受限环境、模板/doctor/参数解析，不等T016的iOS材料或双组合子门；macOS与Linux使用独立工具资源窗口。
4. US3：T025纯模板文本可与Android分时准备，typed Parse等005纯配置；T027依T025准备模板/工程而不调用签名。T026真实资源红门及T028消费等005实际签名代码冻结交付，T028另依T020；T029/T030等两实现与合法材料。US2 与 US3 各平台模板可分时并行，但 A 同 flutter.go、根同 run.go 均串行。
5. US4：T031/T032 可在基础后由 C 的独占文件并行与 A 模板工作；T034的Android/公共工具部分依两红门与T013，iOS能力子项另等T028及005实际代码交接，不提前标整任务完成。T033→T035先消费Android本地真实实现，不等iOS子门；T036等已交付008/019和T035，可先验证Android；iOS联验等T028。T037–T039 等 T023/T024/T029/T030/T034–T036，并协调故障注入/实际进程域。
6. T040–T044 是全故事最终门；US1 可作为首增量独立演示，但 Android/iOS/远程未完成不能叫完整 MVP 交付。任一清理 unknown 或真实材料缺失继续保留 pending。

### 并行示例与安全串行点

- US1：A 做 `flutter_test.go` 的模板红测 T008，根做 `client/flutter_test.go` 的 CLI 红测 T009；A 的两个 Flutter API 在同文件串行。
- US2：A 修改 `templates/flutter-android.yml` 与自有工程；根在已获得真实 Flutter API 后接 `pipeline/run.go`；这是依赖就绪后的文件分区并行，不许可提前写 mock。
- US3：A的iOS模板文本门T025可独立准备，根的`pipeline/flutter_ios_test.go`真实资源红测T026等005签名验收；真正工具故障测试须另约同主机独占窗口。
- US4：C 的 agent 红测 T031 与 store 双库红测 T032 文件不交叉，可与 A 的模板增量并行；共享工具名由根冻结后交接，实际 DB/进程资源互不干扰。

## FR／SC／AC 覆盖表

每个编号映射至少一个具体实施/行为门；T001–T003/T040–T044 是共同前置与交付门，不以计数代替代码或真签名。

| 功能需求 | 任务 |
|---|---|
| FR-001 | T008 T009 T012 T014 T016 |
| FR-002 | T008 T012 T021 T027 T040 |
| FR-003 | T009 T014 T016 |
| FR-004 | T010 T015 T016 T033 T035 |
| FR-005 | T006 T007 T011 T013 T014 T016 |
| FR-006 | T009 T011 T014 T016 T031 T034 |
| FR-007 | T011 T013 T016 T021 T023 T024 T027 |
| FR-008 | T008 T017 T020 T023 T024 T029 T037 |
| FR-009 | T019 T021 T022 T023 T024 T025 T027 T029 |
| FR-010 | T006 T007 T010 T017 T019 T020 T022 T023 T033 T039 |
| FR-011 | T019 T021 T022 T023 T024 |
| FR-012 | T021 T022 T023 T024 T040 |
| FR-013 | T019 T022 T023 T024 T039 T040 |
| FR-014 | T025 T027 T028 T029 |
| FR-015 | T025 T026 T027 T029 T030 |
| FR-016 | T018 T026 T028 T030 T038 |
| FR-017 | T026 T028 T030 T038 |
| FR-018 | T004 T005 T010 T018 T020 T026 T028 T036 |
| FR-019 | T006 T007 T018 T020 T026 T028 T030 T033 T035 T038 |
| FR-020 | T019 T023 T024 T029 T033 T035 T037 T039 |
| FR-021 | T031 T032 T034 T037 |
| FR-022 | T031 T032 T034 T037 T041 |
| FR-023 | T005 T010 T015 T020 T033 T035 T036 T037 |
| FR-024 | T030 T033 T035 T036 T038 T039 |
| FR-025 | T022 T023 T024 T027 T029 T037 T038 |
| FR-026 | T019 T023 T024 T026 T029 T030 T037 T038 T039 T041 |
| FR-027 | T001 T002 T016 T023 T024 T029 T030 T037 T038 T039 T042 |
| FR-028 | T003 T012 T018 T022 T027 T036 T040 T043 |

| 成功标准 | 任务 |
|---|---|
| SC-001 | T008 T009 T010 T012 T014 T015 T016 |
| SC-002 | T006 T007 T011 T013 T014 T016 |
| SC-003 | T023 T024 T029 T037 |
| SC-004 | T017 T018 T019 T020 T022 T023 T024 T026 T029 T030 |
| SC-005 | T026 T028 T030 T033 T035 T038 |
| SC-006 | T031 T032 T034 T036 T037 T038 T039 |
| SC-007 | T006 T009 T010 T016 T023 T030 T033 T035 T039 |
| SC-008 | T001 T002 T023 T024 T029 T030 T037 T038 T039 T042 T043 |

| 原 spec 的验收场景 | 任务 |
|---|---|
| US1.AC1 | T008 T012 T014 T016 |
| US1.AC2 | T009 T014 T016 |
| US1.AC3 | T010 T015 T016 |
| US1.AC4 | T006 T007 T011 T013 T014 T016 |
| US1.AC5 | T009 T011 T014 T016 |
| US2.AC1 | T021 T022 T023 T024 |
| US2.AC2 | T017 T019 T020 T021 T022 T023 T024 |
| US2.AC3 | T019 T022 T023 T024 T039 |
| US2.AC4 | T010 T017 T018 T019 T020 T022 T023 |
| US2.AC5 | T018 T019 T020 T023 T024 |
| US3.AC1 | T025 T027 T028 T029 |
| US3.AC2 | T025 T027 T029 |
| US3.AC3 | T026 T028 T029 T030 |
| US3.AC4 | T026 T028 T030 T038 |
| US4.AC1 | T010 T018 T020 T026 T028 T033 T035 T037 |
| US4.AC2 | T026 T028 T030 T033 T035 T038 |
| US4.AC3 | T031 T032 T034 T037 |
| US4.AC4 | T033 T035 T036 T038 T039 |
| US4.AC5 | T033 T035 T037 T039 |

## 本阶段工作流记录

2026-10-05：主代理明确授权对提前冻结设计执行 tasks→只读 analyze；spec/plan 的“仅规划、未生成 tasks”是此前阶段记录，本次不改冻结输入。实际 selector 指向本目录，`specify preset list` 无安装 preset；`resolve-template.sh tasks-template --json` 解析 core 层，`setup-tasks.sh --json` 给出绝对 FEATURE_DIR/core 模板和四项可用设计文档。`extensions.yml` 的 before/after tasks/analyze 都在空 `hooks: {}` 中，无可执行 hook。analyze 结果由消息交接，不创建报告文件，不实现/提交；这是历史阶段记录；本轮下方组件修正替代旧整项启动门。

2026-10-05组件前置修正：实际setup-plan→setup-tasks→只读analyze重新执行，仍44项合理粒度任务与原28FR/8SC/19AC映射。组件归属/root串行共享不变；T002/T003按已验收公共基线、005纯配置manifest、005签名正式验收三类证据守具体consumer。005纯schema仍归005唯一writer，009不得借dirty代码或写占位。独立Android/工具可实现，未完成iOS子项和完整feature保持pending，不按组件提交。


2026-10-05实施边界再次修正：用户明确完整代码及必要自动检查先交付，合法材料／发布与统一平台案例最后人工验收。上述首次组件规划中的“005签名正式验收前置”等历史文字仅说明当时阶段；当前T002与开头交付边界是有效依赖。FR/SC/AC与任务粒度未变化，真实门未执行不得勾PASS，代码可完整接入005经自动检查冻结的实际接口。

## Phase 8：Convergence（独立009源码交接）

- [x] T045 [HIGH] 根串行消费005已经实现、冻结且自动验证的IOSSigning五字段与availability/Validate/Prepare/Close实际代码，合并009 framework CLI分支与原005原生iOS选项，验证iOS/双模板typed Parse、scoped纯Preview、跨平台材料拒绝、未声明签名skipped、真实机制能力匹配和ErrFlutterCleanup未知保护（FR-001/004/014/018/021，partial；原T003/T008/T014/T025–T028/T034/T041集成缺口）。不借dirty实现或stub，不等待合法Apple材料人工验收来编码；实际签名/两OS构建/最终案例仍按用户最后统一人工门独立pending，本任务实际完成后再执行原T043/T044最终交付。
