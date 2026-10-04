# 构建框架、商店分发与用户扩展

本文件是 [PLAN.md](PLAN.md) 的组成部分，描述待实现的 MVP 范围。
当前原生Android模板、本地流水线及控制端/Agent多节点执行已验收；005真实Apple签名、Flutter、商店上传与自定义发布契约仍待后续功能，状态见[实施历史](../IMPLEMENTATION_HISTORY.md)。

## 构建框架与目标平台分开

Android/iOS 是目标平台，原生、Flutter 是构建框架，Google Play/App Store 是发布渠道。
原生与 Flutter 构建都生成同类平台产物，共用签名、收集、节点调度和商店上传能力。

| MVP 内置模板 | 调用工具 | 发布产物 | 节点要求 |
|---|---|---|---|
| 原生 Android | Gradle wrapper | AAB；APK 用于下载/测试 | Linux/macOS、JDK、Android SDK |
| 原生 iOS | xcodebuild archive/export | IPA 与调试符号 | macOS、Xcode、分发签名 |
| Flutter → Android | flutter build appbundle | AAB | Android 工具链、Flutter |
| Flutter → iOS | flutter build ipa | IPA、archive 与可用符号文件 | macOS、iOS 工具链、Flutter |

Flutter 构建与 fastlane 发布已有 [Flutter 官方持续交付指南](https://docs.flutter.dev/deployment/cd)。
iOS 的版本参数与导出选项见 [Flutter iOS 发布说明](https://docs.flutter.dev/deployment/ios)。
工具版本由项目声明和节点环境锁定，doctor 报告实际版本；不自动更新节点 SDK。
React Native、Kotlin Multiplatform 等先由用户配置调用其工具链，不承诺 MVP 内置适配。

## 内置支持采用模板，保持流水线可编辑

拟定 CLI（待实现）：

```text
mybuilds init --framework native --platform android
mybuilds init --framework native --platform ios
mybuilds init --framework flutter --platform android
mybuilds init --framework flutter --platform ios
mybuilds init --framework flutter --platform android,ios
mybuilds init --template ./ci/mybuilds.template.yml
```

项目维护四套模板和对应 doctor 检查。单平台也生成对应 android/ios 命名 build，双平台组合现有两套模板，保持本地 init 与远程 project init 的名称一致。
无参数 init 仅生成最小 default shell 配置；旧根级单流水线仍解析为 default，自定义模板保留原名称，不自动猜测映射。
init 生成普通 mybuilds.yml，平台模板默认只用 run/artifact；发布需要显式添加 approval/upload，不添加 build/flutter 等新步骤类型。
用户可编辑生成结果，增加仓库脚本、测试、flavor、签名配置与工具参数；已有文件仍拒绝覆盖。
自定义模板仅加载明确指定的本地 YAML，严格校验字段；不下载或执行远端模板。
内置模板增加需要的节点标签，Flutter iOS 同时要求 Flutter、Xcode 与签名能力。
Flutter 模板声明 version、channel、flavor 字符串参数；flavor 默认空，非空时才向 Flutter 传 --flavor，具体 flavor 需由应用工程准备。
本地 run 检查宿主工具，远程调度匹配节点，不在 Android 节点隐式执行 iOS 构建。
本地只构建/收集产物/交互审批；生效的 upload 在运行任何命令前拒绝。实际发布经控制端与 Agent，包括同机部署；dry-run 可以预览发布配置。

## 可复用构建方案与无仓库配置的项目

构建方案（build profile）是一份可复用的完整流水线，可使用内置方案或管理员在控制端 build_profiles 中定义的方案；
可引用四种内置模板，也可引用控制端本地单 build YAML。项目的每个命名 build 绑定方案名称，多个 build 或项目可共用。
内置方案可直接绑定，无需管理员注册同名方案；自定义名称由控制端 build_profiles 管理。方案不接受嵌套 builds。
执行各自拥有工作区与授权，同一项目的所有 build 共用构建号计数器。
它与 init 模板共用同一配置格式和引擎，不增加新的步骤种类，也不要求修改应用仓库。
项目组（project group）管理项目归属；同一组内项目可以使用不同方案，项目改组不会改变已绑定方案或通知。

项目设置示例：

```yaml
pipeline:
  source: auto
  file: mybuilds.yml
  builds:
    android:
      profile: flutter-android
      params:
        channel: 内测
    ios:
      profile: flutter-ios
```

默认 auto 优先固定 SHA 上的仓库文件，仅在缺失时回退到绑定方案；配置语法错误或读取失败直接报错。
仓库定义与绑定方案集合不合并；仓库使用 builds 时以该文件的定义集合为准。原单流水线格式视为 default。
repo 模式要求仓库文件存在；profile 模式直接使用方案。项目不指定方案且仓库无配置时须明确报错，不能猜测或偷偷改用其他类型。
repo 文件路径限制在仓库内；方案文件由管理员在控制端加载并校验，不接受仓库提供的控制端路径。
不混合两份 steps；触发参数覆盖项目默认参数，项目默认参数覆盖所选流水线声明的默认参数，未知参数拒绝。
通知可直接填写项目 Webhook，沿用项目 > 所选流水线 > 全局 defaults 的字段覆盖规则，目的地列表整体替换；
无需在服务端注册渠道，通知敏感值由控制端受限保存并发送，执行/上传凭据仍在授权节点。
执行前将展开后的完整流水线、参数、来源名称/路径及摘要保存为快照；重试不重新读取已变化的方案。
项目节点限制和发布授权对两种来源都生效；不预建方案嵌套、多层 steps 合并或批量项目编排。
具体服务端、项目设置结构见 [配置设计](CONFIGURATION.md#配置文件)。

## 一个仓库多个命名 build

一个项目可定义 android、ios、android-demo 等 build，各自声明 runner、params、env、steps。
有仓库配置时，一份 mybuilds.yml 的 builds 映射包含完整定义；没有 YAML 时，客户端初始化可以直接绑定方案：

```bash
mybuilds project init mobile-app \
  --repo git@gitlab.example.com:team/app.git \
  --nodes linux-android-01,mac-ios-01 \
  --framework flutter --platform android,ios
mybuilds trigger mobile-app --build android
mybuilds trigger mobile-app --build android,ios
mybuilds trigger mobile-app --all
```

以上均待实现。只存在一个 build 时可省略选择，多个时须显式选择；一个 build 仍固定一个节点、步骤顺序执行。
批量触发固定同一个 SHA，全部参数与权限校验后判断 when，原子创建所选结果；满足条件的 build 分配不同项目构建号与执行 ID，不满足的记录 skipped，不占节点或构建号。
Android 可调度 Linux/macOS，iOS 调度 macOS；同项目同名 build 串行，不同 build 在容量允许时可并行。
失败不取消其他 build，日志、产物、通知和发布记录独立，重试使用原名称/SHA/配置快照。
同商店同应用发布仍需互斥与版本检查，不因不同 build 而绕过发布保护；不增加 build 间依赖或自动回滚。
--file 保存仓库根目录相对路径，--settings 读取相对当前目录的本地管理设置文件；普通初始化不必额外准备设置 YAML。

## 两大商店进入 MVP

统一使用 fastlane 的第三方工具与 Ruby 库，Go 通过 os/exec 调用，仍由本项目提供 upload 入口。
它不是 Go 包；节点增加 Ruby/Bundler，使用受控 Gemfile/Gemfile.lock 锁定依赖，执行 bundle exec fastlane。
锁文件在接入原型确认兼容版本后生成并提交，不在本次规划中安装工具或编造版本。
安装方式依据 [fastlane 官方说明](https://docs.fastlane.tools/getting-started/ios/setup/)。

| 内置 target | 第三方能力 | MVP 范围 |
|---|---|---|
| google_play | supply / upload_to_play_store | 上传 AAB、指定 track，支持测试轨道及显式 production 发布 |
| app_store | deliver / upload_to_app_store | 上传签名 IPA 至 App Store Connect，显式提交 App Review 及选择审核后是否自动发布 |
| custom | 用户指定的仓库命令 | 通过同一发布记录与租约调用自定义渠道或 Fastfile |

Google 的 AAB、轨道与发布参数见 [supply 文档](https://docs.fastlane.tools/actions/upload_to_play_store/)。
Apple 的 API key、上传与提交审核参数见 [deliver 文档](https://docs.fastlane.tools/actions/upload_to_app_store/)。
默认 Google track 为 internal；默认 Apple 只上传、不提交审核、不自动正式上架。
Apple 支持 submit_for_review 与 automatic_release 参数，默认均为 false；真实字段在 feature contracts 中约束。
公开发布须在配置/请求中明确选择，远程发布须由管理员授权；含 upload 的构建仅允许 admin 发起，trigger 身份不能借流水线发布；项目内 approval 节点可再次人工放行。
MVP 不能只做 TestFlight 上传而将其称为 App Store 发布；TestFlight 可作为后续独立 target 增加。

## 商店身份、状态与安全

- Google 使用已授权的 service account；Apple 使用 App Store Connect API key，分别核对应用 ID 与权限。
  认证依据 [fastlane Apple 认证说明](https://docs.fastlane.tools/getting-started/ios/authentication/)。
  密钥仅保存在授权节点的受限凭据文件，配置保存引用，不把 JSON/p8 明文放入 argv、日志或数据库。
- 前提是商店账号、应用记录、授权、签名与发布所需元数据已由用户准备。
  Google 首次接入的前置上传要求见 supply 的 setup/quick start；doctor 将缺失前提明确报告。
  MVP 不自动注册账号、接受协议、生成商店截图、补全隐私声明或管理内购。
- 构建号需与商店既有版本兼容；首次注册项目可设置起始构建号，校验冲突，不替换已存在版本。
  已核验的 (store, app_identifier) 唯一绑定项目，防止不同项目独立计数；同项目各 build 共用上传锁，外部发布导致版本冲突时明确失败。
- 上传只接受唯一已收集产物，核对 AAB/IPA、应用标识、版本和摘要，审批后不能重新构建替换。
  发布意图在控制端持久化，绑定 node/build/attempt/lease、商店、应用、产物摘要及报告放行证据；授予执行权后才启动命令。
  普通 run/artifact 必须先于 approval/upload 发布段，已配置 JUnit 必须在发布审批/上传前通过并封存，审批后改写或丢失证据不得发布；post 诊断不替代此前测试。
- 发布结果分别记录 uploaded、processing、submitted、published、failed、unknown 与远端标识。
  节点进程退出 0 只证明相应动作完成，不能推断已经通过审核或公开上架。
  商店处理/审核状态在任务结束后通过显式查询更新，不长期占用构建槽等待审核。
  Apple 的提交、审核与发布状态依照 [官方状态说明](https://developer.apple.com/help/app-store-connect/reference/app-information/app-and-submission-statuses)。
- 控制端已授权执行但缺少可信终态回执时默认 unknown，包含节点启动前后失联；不能凭进程未报告启动认定未发送。
  只有确证无发布副作用（未发出请求或远端明确拒绝）才记 failed 并解除应用锁；unknown 一直持锁，停止确认不能替代上传确认。
  适配器先按应用/版本/构建号查询远端，再允许人工确认；
  本项目不得自动重跑整个发布命令。接入原型须核实 fastlane 的内部重试，限制不可确认的非幂等重发。

## 用户自定义与内置能力共用执行边界

构建扩展优先使用普通 run 步骤和仓库脚本，产物仍通过 artifact 明确声明。
run 支持 sh/bash、working_dir、步骤 env 与 timeout；params 由 --param key=value 覆盖，经 env 显式映射给脚本，可作为位置参数传入。
远程构建注入 MYBUILDS_PROJECT、MYBUILDS_BUILD_NAME、MYBUILDS_BUILD_ID、MYBUILDS_BUILD_NUMBER、Git/节点/工作区等上下文，
不自动导出全部 params，也不继承控制端/Agent 完整环境。每步独立 shell，cd/export 不跨步保留；完整字段、变量表与脚本案例见[配置设计的 shell 小节](CONFIGURATION.md#shell-执行与脚本参数)。
用户已有 Fastfile/lane 可通过 upload 的 custom target 调用，其他渠道也使用同一入口。
不预建插件市场、动态 Go 插件或通用适配器注册服务。

自定义发布的最小契约：

- 配置给出 argv 列表和工作目录，不把用户参数拼接进 shell；命令必须在可信仓库和授权节点执行。
- 输入包含已校验的产物路径、摘要、应用/版本、发布参数与凭据引用；没有完整控制端环境或管理员 token。
- 引擎负责租约、取消、脱敏、发布意图和结果记录；自定义命令输出一个结构化结果文件；限制路径、大小与字段，回传前脱敏。
- 退出 0 且结果声明动作完成才确认成功；控制端授权后失败或缺失有效回执则默认 unknown。
  确认无发布副作用（未发送或远端明确拒绝）才可记 failed；没有远端查询能力时由管理员确认，不自动重发。
- 可选指定查询命令用于人工触发核对，沿用鉴权与输出校验；协议字段在发布 feature 的 contracts 中确定。

约定所有发布动作放在 upload 中，run 用于构建与测试。
任意 shell 内自行发布的远端副作用无法由系统自动识别，不能获得内置上传的结果核对保证。

## MVP 交付路线与验收

完成基础配置、引擎与产物 → 原生/Flutter 构建 → 控制端/多 Agent → 停止确认与原提交恢复 → JUnit 报告及发布检查 → 两大商店与 custom 上传。
具体拆分见 [实施路线](SPECKIT_ROADMAP.md)；MVP 基础测试覆盖官方模板和自定义命令同一执行路径。
when、参数约束、总超时、post、日志时间戳、变更路径筛选、Webhook/等待窗口、项目保留策略、JUnit 报告和发布审批均进入 MVP。
飞书等机器人通知、轮询/cron、fir.im/generic 内置渠道与部署打磨继续后置；MVP 审批使用 CLI，不依赖通知模块。

必须验收：四套模板的真实产物与版本、双平台组合与同仓库多 build；仓库配置优先、仅缺失时方案回退、错误配置拒绝、强制来源与快照重试；自定义脚本与参数构建；两节点独立调度；Google Play internal 实际可见版本；
App Store Connect 可见构建与显式提交审核路径；custom 结果文件解析；错误凭据、产物不匹配、租约过期拒绝、
上传成功但回报丢失保持 unknown，重复请求不执行第二次上传。
新增验收：when 分支/参数/changes 组合及跳过原因、手动路径豁免、审批跳过不上传、重试条件冻结；参数 choices/required、累计超时与独立收尾预算；
UTC 日志与流式脱敏、触发窗口合并/隔离/恢复、JUnit 失败阻止发布及原始报告下载、项目保留继承与未知/待审批数据保护。
正式审核和上架由商店决定，不以外部审核通过时间作为本项目测试通过条件。
