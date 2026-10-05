# 009 研究与决策

> 2026-10-05用户最新交付边界：先完成完整可执行代码与必要自动检查。005签名API只等真实代码冻结交接，不等合法Apple材料人工验收；真实签名／双平台／最终联验保留全部原要求，由用户最后统一人工验收并独立记录pending。源码集成或提交不宣称这些门PASS。无stub、第二Run、宿主未知身份或安全降级。

日期：2026-10-04。仅规划与只读工具检查，不创建 Flutter 应用、下载 SDK、导入材料或执行 VM 任务。

## R1：锁定现有 SDK 与示例工具，不管理 SDK

**决定**：首个验收工程使用实际已有 Flutter 3.38.6 stable/Dart 3.10.7，framework commit `8b872868494e429d94fa06dca855c306438b22c0`，engine `78fc3012e45889657f72359b005af7beac47ba3d`。示例沿该版本官方默认 Gradle 8.14、AGP 8.11.1、Kotlin 2.2.20、SDK 36、NDK 28.2.13676358；不偷偷降低到原生 SDK35 或升级用户工程。

**依据**：主代理已实际执行 `flutter --version --machine`；本轮只读 `bin/cache/flutter.version.json`、Dart version 和 `git rev-parse HEAD` 均一致。默认工具值由该 SDK `packages/flutter_tools/lib/src/android/gradle_utils.dart` 直接核对，见 [官方 3.38.6 源码](https://github.com/flutter/flutter/blob/3.38.6/packages/flutter_tools/lib/src/android/gradle_utils.dart)。本机 SDK36/build-tools36.0.0/NDK28.2目录已存在，CocoaPods `pod --version` 实际返回1.16.2；目录存在仍不替代正式实际构建/工具测试。

**替代**：临时降 Flutter、全局 `flutter upgrade`、改用户 Android SDK 或系统 Ruby 均不采用。Linux Flutter3.38.6 工具与对应 SDK/NDK 尚未准备，也不能因已存在 Linux 原生 SDK35 就报告兼容。

[官方 SDK 归档](https://docs.flutter.dev/install/archive) 用于后续明确下载。此次官方 release metadata 的 linux/macos JSON 请求均返回 HTTP404，因此没有伪造 archive SHA 或下载成功记录；后续若需要安装独立 SDK，先从官方记录取得对应版本/架构摘要、校验自有下载并实跑，不能沿用未验证镜像。

## R2：体检必须实际运行，避免自动 bootstrap/账号检查

**决定**：解析已初始化 SDK 的真实 Flutter machine 版本及其捆绑 Dart，公共仅输出规范数字版本和固定原因；不使用独立 PATH 中不同 SDK 的 dart 冒配对。初始化缓存缺失时固定失败，不调用 bootstrap、precache、upgrade、doctor --android-licenses 或账号检查来修复。

**依据**：3.38.6 SDK 已存在 `bin/cache/flutter_tools.snapshot`、Dart SDK 与 tool package config；实现阶段实际用捆绑 Dart 启动现有工具 snapshot 的 machine version 路线验证，避免 `bin/flutter` shared.sh 在未初始化时下载/编译。若该路径不兼容，明确失败并通过已初始化固定 SDK 的实际流程核实，不回退到自动安装。体检不读取用户 Xcode 账户/私钥。

**额外源码边界**：[command runner](https://github.com/flutter/flutter/blob/3.38.6/packages/flutter_tools/lib/src/runner/flutter_command_runner.dart) 在版本命令中仍更新 informative artifact；[FlutterEngineStamp](https://github.com/flutter/flutter/blob/3.38.6/packages/flutter_tools/lib/src/flutter_cache.dart) 缺失/不匹配会下载engine_stamp.json。[version.dart](https://github.com/flutter/flutter/blob/3.38.6/packages/flutter_tools/lib/src/version.dart) 的main/master版本分支还会fetch tags。因此直调snapshot本身不保证无下载。实现须先对现有版本JSON、engine.stamp/engine_stamp.stamp与engine_stamp.json、legacy version、snapshot/Dart/tool package config作有界一致性检查，缺失固定失败；首验收使用已初始化stable SDK。每次调用加`--no-version-check --suppress-analytics`，不调用会永久修改偏好的disable-analytics。不能保障无隐式更新的SDK渠道/布局明确不兼容，不尝试在线修复。工具可能读取或创建telemetry配置，doctor使用自有临时HOME隔离这类写入、保留显式SDK/PATH与所需已有缓存位置，不写宿主偏好；只清理自己创建的目录。实际无网络下载、SDK不更新、用户偏好不变须由实施阶段前后字节/目录与实际命令负例证明。

**替代**：抓取 `flutter doctor -v` 的任意正文或仅检查文件存在均不能提供版本/签名事实。原生 Android/IOS CLI doctor 和有界公共 process 保持；清理不确定后停止后续工具调用。Run只通过FlutterDoctor的具体Platforms/Environment调用明确平台工具；不原样调用会读取ambient环境及执行gradlew用户脚本的AndroidDoctor，不在整批预检下载wrapper或执行工程脚本。

## R3：一个 YAML 与显式框架消费

**决定**：模板只组合两个独立 builds，增加 `runner.framework: flutter`；省略/native 保持原行为。节点 Tools 仅增加当前所需 flutter、dart、cocoapods 名称，匹配已有真实 passed 与平台前提。标签仍精确匹配，不从 `flutter` 标签推断能力，不引入泛型工具列表或协议新层。

**替代**：只靠 labels、扫描命令正文猜 Flutter、另加 build/flutter step、模板注册表/插件引擎均拒绝。完整定义/框架随现有冻结快照和008 retry深拷贝，不增加数据表。

## R4：版本、编号、flavor 与用户脚本

**决定**：version/channel/flavor/build_number 声明字符串默认1.0.0/internal/空/1，显式 env 传给 Bash 数组参数；空 flavor 不传参数，非空不猜另一变体。内部 `build.number` 在本地 Flutter Run 使用合法已声明本地编号，远程只用可信任务编号；公开 Facts 不能覆盖。本地复用remote trigger已有shared/scoped解析，PreviewOptions增加BuildParams，防Android/iOS不同必填参数互相形成unknown；不新增另一套参数语法。已声明的标准参数及有效编号整批预检查，未知参数仍由既有 ResolveParams 拒绝。

**依据**：[Flutter Android 版本说明](https://docs.flutter.dev/deployment/android) 和 [iOS 说明](https://docs.flutter.dev/deployment/ios) 说明 build-name/build-number 的平台映射；[Android flavor](https://docs.flutter.dev/deployment/flavors) 与 [iOS flavor](https://docs.flutter.dev/deployment/flavors-ios) 要求工程先准备变体与共享 scheme。Android 正整数上限沿既有2100000000，iOS 沿005与[Apple CFBundleVersion 当前文档](https://developer.apple.com/documentation/bundleresources/information-property-list/cfbundleversion)：一至三段十进制整数；Flutter本地约定收窄为一个正整数。当前官方正文未给数值上限，不沿用未经核实的历史四位限制，也不自造远程编号编码。Android上限依据[官方版本规则](https://developer.android.com/studio/publish/versioning)。channel 是应用渠道参数，不切换 SDK 分支。

**替代**：`eval`、拼命令、自动设置全部 params、借 `-Pversion` 假定任意工程已消费、用本地默认1替换远程编号均不采用。

## R5：Flutter Android 的真实产物验证

**决定**：实际执行 `flutter build appbundle --release --no-pub --build-name ... --build-number ...` 与 `flutter build apk` 同样参数；默认 fat APK，避免 split-per-abi 修改编号。工程明确通过 env配置 release keystore，模板要求预期 application_id。映射对应实际 flavor路径；仅工程确实启用R8且显式声明mapping时要求该文件。

构建后普通验证命令核对 APK（aapt2/apksigner）、AAB（jarsigner/keytool 与官方 bundletool manifest），声明证书使用 keytool的env密码读取。AGP8.11.1已有官方 bundletool1.18.1模块，004独立真实预检已成功通过 Gradle init task 的插件 classloader 调用 DumpCommand读取 manifest；009复用同具体机制，不下载未经核验的 standalone JAR、不手写 protobuf、不把 Gradle退出0当版本/签名通过。验证 init 文件仅属于本次工作区/临时路径，可嵌在可编辑 YAML 的普通命令；任何模块/工具/身份/版本不符失败。

**限制**：上述004原型证明读取机制，不证明Flutter工程已成功。本次Flutter默认/flavor、两OS、真实节点与下载仍全部要独立验收。AGP不同的工程须明确适配并实际验证，不静默跳过AAB检查；Dart混淆符号不等同R8 mapping。

**替代**：debug signing、旧产物glob、只检查传入参数、用APK版本推断AAB版本或以测试预置产物代替均拒绝。

## R6：Flutter iOS 先配置，再复用手动 archive/export

**决定**：`flutter build ios --release --config-only --no-codesign --no-pub --build-name ... --build-number ...`（仅非空flavor加选项），之后用005的 xcodebuild 手动 archive/export、实际版本检查、明确资源env和导出plist；新增把同一实际archive压缩为可下载zip，保留IPA/dSYM。使用scheme与profile匹配的明确身份，非空flavor须与明确共享scheme对应。

**依据**：[官方3.38.6 build_ios源码](https://github.com/flutter/flutter/blob/3.38.6/packages/flutter_tools/lib/src/commands/build_ios.dart) 明确 config-only用于避免重复归档工作；[对应mac.dart](https://github.com/flutter/flutter/blob/3.38.6/packages/flutter_tools/lib/src/ios/mac.dart) 在configOnly返回前更新工程和处理Pods，而codesign=true会尝试自动身份选择。因此配置阶段显式no-codesign，真正签名仍由005的archive/export执行；不是产出 unsigned IPA。

005契约里的P12/profile/password引用、BundleID/method、受保护MYBUILDS_IOS_*与Prepare→ordinary/artifact→post→独立Close保持；不添加keychain到用户search list，不调用自动provisioning或账号登录。用户工程依旧负责共享scheme、Podfile的flavor配置和应用身份。

**未证明**：005 T012–T014仍缺真实合法Apple材料，export在隔离keychain下的发现行为、实际IPA及完整取消生命周期不能由cgo编译或自产签名原型推出。009继承该真实门，不能先放行ios_signing passed。

**替代**：直接flutter build ipa自动选宿主身份、给Xcode全局search list临时追加、unsigned archive冒称签名、第二套keychain管理器均不采用。用户可编辑命令使用其他路径，但不能绕过明确材料与收尾边界。

## R7：预算、停止、报告与前置联验

**决定**：Run唯一执行器；配置/参数/工具/显式签名整批先查，实际run之前才Prepare，post结束后独立Close，artifact-only/全skip不准备。远程失权/persist失败禁止新动作和user post，系统安全清理独立执行；原失败Reason与CleanupFailed分别保留。008恢复/retry与019封存/报告使用各自正式消费者，009不预写状态机。

**实证**：LinuxAMD64原生Agent201/202的固定快照完整30m预算均实际在TCG-Xint lint超时，无产物；202100ms时序确认已ACK普通finished/StopConfirmed后独立Gradledaemon仍活并增长CPU，再退出。脱敏证据位于主代理自有 `linux-amd-agent007-ho9p4gr3/process-evidence/`，独立缺陷修复已随008验收交付；此处是原发现历史，不是当前仍阻塞全部源码。不能拿最终gone或预检成功替代取消门；Flutter正式验收必须基于该缺陷修复后的进程实现重新核验。

**替代**：延长活动任务、跳lint/R8、人工kill后说产品PASS、拷预检输出、失联自动迁移/重跑或恢复时重置预算均拒绝。

## 本轮结果边界

研究决策已明确，无需用户选架构/参数；七文档可继续tasks/analyze。仍缺：005合法Apple材料及签名验收、纯配置/预览当前基线组件交接、Linux Flutter工具和依赖的受控准备、Android/Flutter/iOS实际新工程与flavor构建。008/019及停止修复已正式交付，不再标未验收。未运行任何009应用构建或实现，所有成功标准仍未验收。

## R8：组件开发与整功能验收分离

**决定**：从已验收fee97e8建立独立源码WT，Flutter/Dart、工具-only iOS诊断、Android模板/参数/Run/节点资格使用当前真实接口开发。005唯一writer负责纯IOSSigning五字段及预览迁移，根核其真实检查与SHA后交009；009不复制pending源码、不定义第二套类型。iOS模板纯文本准备不等于其Parse/Preview已通过；组合检查等纯组件。签名可用性函数、Validate/Prepare/Close和材料doctor仍等005真实签名接口冻结交接后消费，不预建stub。

**依据**：MVP_EXECUTION允许缺工具/账号/签名时完成仍可执行实现和检查，保持“待真实验证”；主代理此次明确组件归属。008/019已正式提交，当前Agent ios_signing仍skipped/unsupported，标签不能解锁。官方[Android发布指南](https://docs.flutter.dev/deployment/android)、[iOS发布指南](https://docs.flutter.dev/deployment/ios)和[Android flavor指南](https://docs.flutter.dev/deployment/flavors)本轮重新浏览：平台签名与工程变体前提仍需真实工程满足，本轮不换SDK或承诺新版兼容。

**替代**：等待Apple凭据导致所有doctor/Android代码停摆，或复制005 dirty资源实现/返回固定false的空availability函数消除编译依赖，均不采用。完整平台门与整功能提交不拆成独立组件验收声明。
