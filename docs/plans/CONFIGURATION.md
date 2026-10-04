# 配置格式与流水线执行约定

本文件保留原 [PLAN.md](PLAN.md) 的完整专题章节；业务功能按 [实施路线](SPECKIT_ROADMAP.md) 推进，当前状态见 [实施历史](../IMPLEMENTATION_HISTORY.md)。

## 配置文件

### 服务端配置 `~/.mybuilds/server.yml`（由 `mybuilds-server serve` 读取）

006 当前支持下方的 listen、data_dir、concurrency、database 与 secrets_file；build_profiles、retention、defaults 分别随对应功能接入，当前明确拒绝。可运行的最小配置见 [README](../../README.md#控制端与远程排队)。PostgreSQL 使用明确 DSN，不读取宿主 PG 环境/service/passfile；显式 TLS 材料受限普通文件读取后在内存校验。

数据库选型见[数据库设计选择](ARCHITECTURE.md#3-数据库默认-sqlite可切-postgresql统一走-gorm)。

```yaml
listen: 127.0.0.1:8787       # 本地监听；跨主机通过校验证书的 HTTPS 入口访问
data_dir: ~/.mybuilds
concurrency: 1              # 所有节点合计的上限，增加节点时显式提高

database:                   # 见设计选择 3，默认 sqlite
  driver: sqlite            # sqlite | postgres
  dsn: ~/.mybuilds/mybuilds.db

secrets_file: ~/.mybuilds/secrets.env     # 0600，控制端 Git 只读凭据和通知密钥

build_profiles:                         # 可复用构建方案，名称由管理员定义
  flutter-android:
    template: flutter-android            # 引用内置模板
  company-android:
    file: ~/.mybuilds/profiles/company-android.yml  # 用户维护的完整流水线

# retention 属于 MVP；notifications 随后续通知功能接入
retention:
  builds: 100                           # 每项目保留数量
  days: 30                              # 保留天数；活动/待审批/停止未确认/未知上传结果受保护
defaults:                                # 项目未指定时采用的全局默认通知
  notifications:
    enabled: true
    on: [success, failure, cancelled]
    webhooks:
      - type: feishu                     # 也支持 wechat / dingtalk / generic
        url: "${DEFAULT_FEISHU_WEBHOOK}"
    template: "{{project}} #{{build.number}} {{build.status}} {{build.url}}"
```

配置覆盖顺序为默认值 < 配置文件 < `MYBUILDS_` 环境变量 < CLI 参数。
环境变量示例：`MYBUILDS_LISTEN`、`MYBUILDS_CONCURRENCY`、`MYBUILDS_DATABASE_DRIVER`、`MYBUILDS_DATABASE_DSN`。
未知字段、无效地址、非正并发或无效数据库配置在启动前报错；路径支持展开 `~`，相对路径以配置文件目录为基准。
跨主机部署在监听地址前设置 HTTPS 反向代理，客户端与 Agent 校验证书；不提供跳过证书校验开关。
项目、节点、token 与发布记录存数据库；流水线来自仓库或控制端构建方案，签名和商店凭据在 Agent 节点解析。
token 由数据库管理；首次启动可设置 `MYBUILDS_BOOTSTRAP_ADMIN_TOKEN`，随后用 `token create/revoke` 管理。
日志、产物和工作区保留策略不清理排队、运行中、待审批或结果未知的构建。

### 项目独立通知与全局默认值（随通知功能实现）

- 项目管理设置、仓库 mybuilds.yml 和构建方案都可以直接填写 notifications.webhooks，
  每项包含 type 与 url；支持 URL 字面量或环境变量引用，不要求先在服务端注册机器人名称。
  defaults.notifications 仅提供全局默认值。
- 通知策略按字段覆盖：项目管理配置 > 选定流水线的 notifications > 全局 defaults.notifications。
  仓库流水线和服务端构建方案都是流水线来源；未填写字段继承下一层，列表整体替换，不追加合并。
- 未指定 webhooks 才继承下一层；项目直接配置后整体替换默认列表，不重复发送到全局机器人。
  基线默认 enabled: true、on: [success, failure, cancelled]、webhooks: []，直接填地址即可使用。
  enabled: false 明确关闭通知；webhooks: [] 明确不发送。所有通知层都未配置时不发送。
  null、未知 type、无效 URL 或缺失环境引用均明确报错；发送失败不改投全局 Webhook。
- approval.notify 使用布尔值，默认 false；通知模块完成后可显式开启，沿用最终 webhooks 并遵守 enabled，不再选择机器人名称；MVP CLI 审批不依赖通知。
- 通知仍由控制端发送；URL 与可选签名密钥属于敏感值，日志、dry-run、错误和详情输出均脱敏。
  直接填写的敏感值在导入或读取时自动保存至控制端受限凭据文件（0600），数据库/执行快照仅存内部引用，
  不要求用户预注册；环境引用从控制端受限环境解析，不把通知凭据注入构建进程。
- 只执行已注册可信项目的通知配置；校验 HTTPS URL 和 type 对应目标，generic 防止请求本机/内网/元数据地址，
  DNS 解析结果在连接时校验，禁用重定向，设置超时和响应大小上限；不得借通知读取控制端任意文件。
- 执行前保存最终通知策略和敏感值引用，记录来源；项目修改不改变本次策略，retry 沿用原快照。
  环境引用按当前凭据解析；失效或发送失败单独记录，不覆盖构建结果，也不回退到其他目的地。

### 项目管理设置（导入数据库）

项目设置文件由管理员导入数据库，示例（pipeline 在 MVP 接入，通知部分随后续功能接入）：

```yaml
pipeline:
  source: auto                           # auto / repo / profile
  file: mybuilds.yml                      # 仓库内相对路径
  builds:                                # 无仓库配置时使用的命名 build 与方案
    android:
      profile: flutter-android
      params:
        version: "1.0.0"
        channel: 内测
    ios:
      profile: flutter-ios
      params:
        version: "1.0.0"
notifications:
  webhooks:
    - type: feishu
      url: "https://open.feishu.cn/open-apis/bot/v2/hook/REPLACE_ME"  # 直接填项目机器人地址
  on: [failure]                          # 其余字段继承流水线/全局默认
```

拟定入口为 `project init/add <name> --settings <本地YAML>` 和 `project set <name> --settings <本地YAML>`；
--settings 接受绝对路径或相对执行命令当前目录的路径，客户端读取内容后导入，不要求提交到仓库，也不每次构建重读。
注册时 --file 则保存相对仓库根目录的流水线路径，由控制端读取固定 SHA 上的文件，两者不等价。
--file 默认 mybuilds.yml；--framework 与 --platform 可直接绑定内置方案，无需额外项目设置文件。
同一次命令使用 --settings 时，不得再传 --file/--framework/--platform，以免两处定义冲突。
set 只更新文件中显式提供的顶层设置块，每个块整体替换，不改变正在执行的构建，也不把密钥明文写入数据库。

### 流水线来源与可复用构建方案

仓库不再强制包含 mybuilds.yml。项目 pipeline 默认 source: auto、file: mybuilds.yml，回退方案须显式绑定或在初始化时选择框架和平台：

| source | 选用规则 |
|---|---|
| auto（默认） | 优先读取固定 SHA 的仓库配置；文件确实不存在时使用 pipeline.builds 绑定的方案集合；两者都没有则报错 |
| repo | 只使用仓库配置，缺失即报错 |
| profile | 只使用绑定方案集合，即使仓库里有 mybuilds.yml 也不读取 |

`build_profiles` 引用内置模板或管理员维护的单 build 完整 YAML，两种定义互斥；内置模板有 native-android、native-ios、flutter-android、flutter-ios，可直接绑定，无需手工注册同名方案。
文件存在但解析错误、路径越界、权限/读取失败时不得回退；不靠文件名或仓库内容自动猜测框架、平台、签名或发布渠道。
选用仓库定义或绑定方案集合，不合并两处的 build 列表，也不拼接 steps。只对声明的 params 覆盖，优先级为触发参数 > 项目 pipeline.builds.<名称>.params > 选定流水线默认参数。
原单 build 管理写法 pipeline.profile / pipeline.params 作为 default 的简写保留，与 pipeline.builds 互斥。
项目节点授权、发布权限和通知策略独立于来源，方案不能扩大权限。
执行前持久化最终展开的流水线及 SHA、来源模式、文件路径或方案名称、内容摘要和参数；模板/方案后续编辑不影响已开始的构建或原提交 retry。
本地 run 仍默认要求当前目录的 mybuilds.yml，显式 --file 缺失就报错，不连接控制端取得方案；init --template 可用于本地生成同一份方案。
构建方案是复用配置，不引入多项目批量构建、方案嵌套继承或另一套执行器。详情见 [BUILD_DISTRIBUTION.md](BUILD_DISTRIBUTION.md)。

### 同仓库多个命名 build

一个项目登记一个 Git 仓库，包含多个命名构建定义，例如 android、ios、android-demo；build 名称不是一次执行的 ID。
项目组、仓库、允许节点和构建号计数器仍由项目管理；每个 build 独立声明 runner、params、env 和有序 steps。
名称须为非空稳定标识，唯一且不包含路径分隔符；重命名定义不改写历史记录，旧任务和 retry 保留原名称与快照。

仓库使用一份 mybuilds.yml，示例（待实现）：

```yaml
version: 1
builds:
  android:
    runner:
      platform: android
      labels: [flutter, android-sdk]
    steps:
      - kind: run
        name: build
        run: bash ci/build-android.sh
      - kind: artifact
        paths: [build/app/outputs/bundle/release/*.aab]
  ios:
    runner:
      platform: ios
      labels: [flutter, xcode, ios-signing]
    steps:
      - kind: run
        name: build
        run: bash ci/build-ios.sh
      - kind: artifact
        paths: [build/ios/ipa/*.ipa]
```

原 version/runner/params/env/notifications/steps 单流水线格式视为 default，继续支持。
单流水线与每个命名 build 均可声明 when、timeout、post、reports；不增加新的普通步骤类型。
多 build 格式的顶层只接受 version、builds 与公共 notifications；runner/params/env/steps 放在各 build 内，拒绝混写。
通知沿用项目设置 > build.notifications > 文件公共 notifications > 全局 defaults；列表仍整体替换。
不增加步骤之间的 parallel、matrix、build 依赖、自动回滚或跨节点执行单条流水线。

无 YAML 时，project init --framework flutter --platform android,ios 自动登记 android/ios 两个 build，绑定内置方案；
单平台注册生成对应平台名称的一个 build；本地 init 的平台模板也始终生成 android/ios 命名 build，保证有无仓库 YAML 时名称一致。原生双平台同样选择两套模板；用户仍需准备各平台的实际工程与签名。
这与将两端命令放在同一个 macOS build 中顺序执行不同：命名 build 可以独立分配到 Linux/macOS 节点。
新增或修改回退定义使用 project set --settings；仓库定义则随 Git 提交修改。
旧根级流水线仍名为 default；切换来源后名称不一致须显式修改选择、参数覆盖与自动触发列表，不自动映射 default/android/ios。

trigger --build android、--build android,ios 或 --all 选择构建，--build 与 --all 互斥；
只有一个定义时可省略选择，多个时必须明确选择。未知名称或重复选择报错，不自动猜测平台。
本地 run 使用相同选择规则，多个 build 顺序执行，不启动本地并行调度。
批量触发先固定一个 SHA、读取和校验完整所选定义、参数与权限，再判断 build 级 when；任一校验失败不部分入队。
事务创建全部选择结果，满足条件的任务分配构建号并入队，不满足的记录 skipped、原因与空构建号，不分配节点。
控制端仅通过只读 Git 操作读取该 SHA 的 YAML，不执行仓库 shell；凭据值仍在对应节点按步骤解析。
每条记录保存 build_name、独立执行 ID、SHA、配置摘要/快照与参数；批次 ID 仅用于关联查询，不增加批次执行器。
不同 build 的构建号不同，由项目共享计数器分配；build 名称不增加另一套计数器。
每个执行分别拥有节点、租约、工作区、日志、产物、通知与发布记录，一个失败不取消其他；取消和 retry 均针对执行 ID。
同项目同名 build 串行，不同 build 可在容量允许时并行；审批只保留当前 build 的互斥。
共享同一商店应用的上传须按商店与应用身份互斥，并核对远端版本，避免不同 build 的并行发布冲突。
上传结果为 unknown 时继续阻止该应用的新上传，先查询或人工确认；不能通过切换 build 名称绕过保护。
后续 Webhook/轮询/cron 使用管理员显式设置的 build 名称列表；多 build 未设置自动触发范围时拒绝启用，不默认全部发布。

### Agent 配置 `~/.mybuilds/agent.yml`（待实现）

```yaml
server: https://build.example.com
node: mac-ios-01
token: "${MYBUILDS_AGENT_TOKEN}"   # 管理员创建节点时获取，不能复用用户 token
capacity: 1
data_dir: ~/.mybuilds/agent
secrets_file: ~/.mybuilds/agent-secrets.env  # 0600，Git/构建/上传所需的节点凭据
heartbeat_interval: 5s
lease_duration: 30s
```

平台、标签与项目允许节点由控制端管理；节点报告实际工具能力。
有效容量取管理员设置上限和 Agent 本地 capacity 的较小值，两者默认 1。
建议默认时序见 [MULTI_NODE.md](MULTI_NODE.md)，协议细节与参数合法性在对应 feature 中验证；跨主机 HTTPS 必须验证证书。

### 客户端配置 `~/.mybuilds/client.yml`（由 `mybuilds` 读取）

```yaml
server: http://127.0.0.1:8787
token: "${MYBUILDS_CLIENT_TOKEN}"   # 也支持环境变量覆盖
timeout: 30s                     # 普通 API 请求超时，不作为日志流总时长上限
```

### 项目注册（`mybuilds-server project` 写入数据库，自动触发后续接入）

```bash
mybuilds-server project add app-android \
  --repo git@gitlab.example.com:team/app.git \
  --provider gitlab --branches 'main,release/*' \
  --nodes linux-android-01 --default-node linux-android-01 \
  --hook          # 后续 Webhook 功能：打印 URL 和 secret
```

### `<repo>/mybuilds.yml`（优先采用的流水线；也可保存在控制端作为方案）

```yaml
version: 1                              # 流水线格式版本
runner:                                 # 控制端按平台、标签及项目授权调度
  platform: android                     # android | ios；本地 run 只校验宿主能力
  labels: [android-sdk]
params:
  version: "1.0.0"                      # 应用版本，可由手动触发参数覆盖
  channel: 内测
env:                                    # 明确支持 ${SECRET} 和 {{var}} 的字段
  GRADLE_OPTS: -Xmx4g
  BUILD_NUMBER: "{{build.number}}"
  APP_VERSION: "{{version}}"
  BUILD_CHANNEL: "{{channel}}"

notifications:                          # 可省略；项目设置优先，其次本块，最后全局默认
  enabled: true
  on: [success, failure, cancelled]
  webhooks:
    - type: feishu
      url: "${APP_FEISHU_WEBHOOK}"        # 也可直接填写该项目 URL，无需全局注册
  template: "{{project}} #{{build.number}} {{build.status}} {{build.url}}"

steps:
  - kind: run
    name: android-release
    run: |
      cd android
      ./gradlew assembleRelease bundleRelease --no-daemon \
        -PversionCode="$BUILD_NUMBER" -PversionName="$APP_VERSION"
    env:
      KEYSTORE_PASSWORD: ${ANDROID_KEYSTORE_PASSWORD}
      KEY_PASSWORD: ${ANDROID_KEY_PASSWORD}

  - kind: artifact                      # glob 收集，产物托管下载
    paths:
      - android/app/build/outputs/apk/release/*.apk
      - android/app/build/outputs/mapping/release/mapping.txt
      - android/app/build/outputs/bundle/release/*.aab

  - kind: approval                      # 挂起等人放行
    name: 发布到内测渠道
    notify: false                       # MVP 通过 CLI 审批；通知模块完成后可开启

  - kind: upload                        # 内置分发，非 shell 拼 curl
    target: google_play                 # google_play | app_store | custom（MVP）
    file: "*.aab"                       # 匹配已收集产物；零个或多个匹配均报错
    channel: "{{channel}}"
    track: internal                     # production 必须显式选择并授权
    credentials: "${GOOGLE_PLAY_CREDENTIALS_FILE}"  # 节点受限文件，配置只保存引用
```

步骤类型只做 4 种：`run` / `artifact` / `approval` / `upload`；checkout 是前置操作，最终通知属于统一收尾。
服务端固定到触发时确定的 SHA，浅克隆无法取得该 SHA 时补充 fetch，不退回分支最新提交。
本地 `run` 遇到 approval 时交互确认，无终端时拒绝执行该步骤；生效的 upload 在预检查时拒绝，发布请使用 trigger。
`--dry-run` 不运行命令、不发通知、不上传。
内置原生 Android/iOS 与 Flutter 模板默认只生成 run/artifact，用户可编辑；approval/upload 在配置中显式添加，完整发布示例不等于模板默认启用发布。
Google Play / App Store 的 upload 由 Go 封装第三方 fastlane；其他渠道或用户 Fastfile 使用 custom target，
同样受租约、授权、取消、脱敏与发布意图记录约束。自定义构建可用仓库脚本或本地 YAML 模板。
不加载动态 Go 插件、不建插件市场或另造 DSL，字段和结果契约见 [BUILD_DISTRIBUTION.md](BUILD_DISTRIBUTION.md)。

v1 步骤**顺序执行**；多渠道先通过参数分别触发，`parallel:` 和矩阵构建等真实需求出现后再加。

### shell 执行与脚本参数

run 支持多行内联 shell，也支持调用仓库脚本；与 Jenkins 的 shell 使用方式类似，但不承诺 Jenkins 插件或 Groovy 语法兼容。
参考 [Jenkins 环境变量与参数](https://www.jenkins.io/doc/book/pipeline/jenkinsfile/)、[sh 步骤](https://www.jenkins.io/doc/pipeline/steps/workflow-durable-task-step/)。
以下补全待实现的执行约定，不代表当前 CLI 已能运行脚本。

| run 步骤字段 | 默认与行为 |
|---|---|
| run | 必填非空脚本文本；支持 run: \| 或 bash ci/build.sh；正文不做模板替换 |
| shell | sh（默认）或 bash；分别以 sh -e / bash -e -o pipefail 执行，不加载交互或登录配置；缺少所选 shell 报错 |
| working_dir | 默认仓库根目录；相对路径始终以仓库根目录解析，必须存在且实际路径不能经符号链接越界 |
| env | 步骤级环境变量，覆盖同 build 的 env；值支持声明参数的模板变量与节点凭据引用 |
| timeout | 可选正数 duration，例如 30m；省略时不设步骤时间上限，取消与租约到期仍生效 |

每个 run 启动独立 shell，前一步的 cd/export 不延续；文件在本次工作区保留。脚本位置参数由普通命令传入，值须引用，例如 bash ci/build.sh "$APP_VERSION" "$FLAVOR"。
默认非零退出停止当前 build；超时和取消均终止本次进程组并执行收尾。引擎不默认启用 set -x，避免回显凭据。
run 内未启动新的 bash 时使用 shell 字段对应的解释器；显式 bash ci/build.sh 会启动子 Bash，外层选项不会自动传入，脚本自行设置 set -euo pipefail。
产物 paths 始终相对仓库根目录，与某一步 working_dir/cd 无关。

params 定义允许用户传入的普通参数及默认值，MVP 采用字符串值，支持默认值简写及 description/required/choices 约束，不预建复杂类型或表单。
参数名称须符合 [A-Za-z_][A-Za-z0-9_]*，不得占用既有上下文模板变量名；params 不自动变成环境变量，由 env 显式映射，避免与工具和系统变量冲突。
run/trigger 支持重复 --param key=value，按第一个等号分割；未知参数、重复键或空名称报错，空字符串值允许。
trigger 的 --version 与 --channel 是相应 --param 的快捷形式，同次传入同名 --param 时拒绝，不按参数顺序决定优先级；本地 run 直接使用 --param。
批量选择时参数用于全部选中 build，所有 build 都必须声明该参数；参数不同则分别触发。
本地覆盖优先于 YAML 默认值；远程覆盖优先于项目同名 build 参数，再优先于所选流水线默认值。
参数通过环境变量传入，不把值直接拼进 run 正文；密钥不能作为 --param，须使用授权节点的凭据引用。

远程构建自动提供以下上下文变量，使用 MYBUILDS_ 前缀，YAML env 不允许覆盖这些引擎变量：

| 环境变量 | 内容 |
|---|---|
| MYBUILDS_PROJECT | 项目名 |
| MYBUILDS_BUILD_NAME | 命名 build，例如 android |
| MYBUILDS_BUILD_ID | 本次执行 ID，与命名 build 不同 |
| MYBUILDS_BUILD_NUMBER | 项目统一分配的构建号 |
| MYBUILDS_GIT_SHA | 固定的完整提交 SHA |
| MYBUILDS_GIT_BRANCH | 授权来源分支 |
| MYBUILDS_NODE_NAME | 执行节点名称 |
| MYBUILDS_WORKSPACE | 本次仓库工作区的绝对路径 |
| MYBUILDS_STEP_NAME | 当前 run 步骤名称 |

以上远程上下文不在本地伪造；本地 run 只提供已知的 build 名称、工作区、步骤名与可检测的 Git 信息，配置模板引用远程专属变量应明确报错。
run 正文不扫描或替换变量，缺失环境变量由 shell/脚本处理；可用 ${MYBUILDS_BUILD_NUMBER:?缺少远程构建号} 显式失败。
本地版本调试可自行声明普通 build_number 参数并映射给脚本，不创建控制端构建号。
既有 {{project}}、{{build.number}}、{{git.sha}}、{{git.branch}} 等字段模板保留，新增 {{build.name}}、{{build.id}}；run 正文仍由 shell 解析 $VAR。
环境构成为允许的系统/工具变量 < build.env < step.env，加上不可覆盖的引擎上下文。
基础白名单含 PATH、HOME、TMPDIR、LANG、LC_ALL，以及检测到的 JAVA_HOME、ANDROID_HOME、ANDROID_SDK_ROOT、DEVELOPER_DIR；
密钥只注入声明引用它的步骤，控制端/Agent token、数据库配置、其他步骤密钥与通知 Webhook 不继承。
普通参数和构建信息会保存为快照；凭据只保存引用，预览、错误与分段日志脱敏。

参数与脚本示例（同样可放在 builds.android 内；本例使用原单 build 格式）：

```yaml
version: 1
params:
  version: "1.0.0"
  channel: internal
  flavor: production
env:
  APP_VERSION: "{{version}}"
  BUILD_CHANNEL: "{{channel}}"
  FLAVOR: "{{flavor}}"
steps:
  - kind: run
    name: package
    shell: bash
    working_dir: .
    timeout: 30m
    env:
      KEYSTORE_PASSWORD: "${ANDROID_KEYSTORE_PASSWORD}"
    run: |
      bash ci/build-android.sh "$APP_VERSION" "$FLAVOR"
```

ci/build-android.sh 可以读取位置参数或环境变量：

```bash
#!/usr/bin/env bash
set -euo pipefail
app_version="${1:?缺少版本参数}"
flavor="${2:?缺少 flavor 参数}"
flutter pub get
flutter build appbundle --release \
  --flavor "$flavor" \
  --build-name "$app_version" \
  --build-number "${MYBUILDS_BUILD_NUMBER:?需要远程构建号}"
```

调用示例：mybuilds trigger mobile-app --build android --param version=1.2.0 --param flavor=production。
不同平台的 configuration、scheme、entrypoint、export_options 等也可声明为普通参数，再由 env 映射给脚本；不需要引擎为每个工具增加专属 flag。
发布继续使用 upload 步骤；任意 shell 自行发布不具备系统的发布意图与未知结果核对保证。

### MVP 条件执行 when

when 可写在整个 build 或 run/artifact/approval/upload 步骤上，省略时正常执行。
只接受 branches（分支 glob 列表）、params（已声明参数与目标字符串的映射）、changes（仓库相对路径 glob 列表）。
不同字段 AND，列表内 OR，多个 params 条件 AND；比较使用冻结的最终参数，区分大小写，不解析 shell、Groovy 或任意表达式。
空 when、空匹配列表、未知字段/参数或非法模式报错，不能当成条件不满足；即使 build 会跳过，也先校验配置与权限。
条件配置与输入事实、判定结果和原因持久化，审批恢复和 retry 沿用快照，不重读最新分支或参数。

build 级条件在节点分配前判断，未满足时记录 skipped，不占节点或消耗移动端构建号，不发送成功通知。
步骤级条件在正常顺序到达该步时判断，不满足记录 skipped 后继续；失败即停仍生效，不能用 when 执行失败后的普通步骤。
全部选择均跳过时批次显示 skipped；单个 build 的全部普通步骤跳过也显示 skipped，未启动执行的 build 不运行 post。
when 不授予权限；所选配置含 upload 仍要求发布权限，不能靠未满足的条件绕过鉴权。
如果发布前的 approval 被跳过，后续 upload 不能视为已批准：应同时跳过发布，否则因缺少批准而阻止。
上传的应用、版本、唯一产物、摘要、租约与 unknown 状态保护继续生效，所需 artifact 被跳过不会让发布使用旧产物。

changes 使用区分大小写的仓库根目录相对路径，复用 doublestar glob；新增/修改/删除均匹配，重命名检查新旧路径。
自动触发对比同项目、同 build、同分支上次成功构建 SHA 与本次 SHA，冻结基线与完整改动列表。
首次构建或比较基线缺失/不可取得时按完整构建处理，记录原因；仓库本身读取或配置校验失败仍报错，不假装有变更。
公共代码、依赖清单、ci 和 mybuilds.yml 应加入 changes，避免仅匹配平台目录造成漏构建。
手动触发和本地 run 默认不做 changes 路径过滤，分支和参数条件仍生效；原提交 retry 使用原条件事实。
本地执行分支条件时需能可靠确定当前 Git 分支，否则明确报错；dry-run 缺少运行事实时显示待确定，不执行 Git 网络请求或命令。

条件发布示例（待实现）：

```yaml
version: 1
builds:
  android:
    when:
      changes: [android/**, lib/**, pubspec.*, ci/**, mybuilds.yml]
    runner:
      platform: android
      labels: [flutter, android-sdk]
    params:
      channel:
        default: internal
        description: 发布渠道
        choices: [internal, production]
    steps:
      - kind: run
        name: package
        run: bash ci/build-android.sh
      - kind: artifact
        name: collect
        paths: [build/app/outputs/bundle/release/*.aab]
      - kind: approval
        name: approve-production
        notify: false
        when:
          branches: [main]
          params: {channel: production}
      - kind: upload
        name: publish-production
        when:
          branches: [main]
          params: {channel: production}
        target: google_play
        file: "*.aab"
        track: production
        credentials: "${GOOGLE_PLAY_CREDENTIALS_FILE}"
```

### MVP 参数约束、超时与收尾

params 的字符串简写等价于 default；对象形式只接受 default（字符串）、description（字符串）、required（布尔，默认 false）、choices（非空且不重复的字符串列表）。
最终参数优先级不变，先合并再校验：required 为 true 时必须有非空值，choices 要求值属于列表，默认值也必须有效。
可选参数无默认值时取空字符串；不能通过跳过 when 绕过参数错误。参数仍经 env 映射，密钥不进入普通参数或 CLI。

build.timeout 是可选正数 duration；累计 Agent checkout、普通步骤和产物处理的实际执行时间，排队、等待审批不计入，post 使用独立预算。
步骤 timeout 与剩余 build 预算取较小值；批准后延续剩余预算，控制端/Agent 重启不重置，retry 作为新执行获得原配置预算。
超时停止本次进程组，记录 timeout 原因及失败结果；不会自动取消批次内其他 build 或重发上传。

post 复用现有 run/artifact 结构，仅支持 success、failure、always 三个有序列表和 timeout（整个收尾预算，默认 2m）。
普通步骤和测试报告先确定结果，再运行对应 success/failure，最后运行 always；运行中取消只运行 always；排队/审批挂起取消不启动用户 post，节点失联或执行权过期不在其他节点运行用户收尾脚本。
每个收尾步骤受剩余预算限制；一项失败或超时仍尝试剩余可运行项，预算耗尽则记录未执行项。
收尾失败使原成功结果变为失败，但不覆盖原失败/取消/上传 unknown 的证据，也不再次进入 failure 列表。
系统进程回收与临时 keychain 清理独立执行，不允许用户 post 替代或跳过；post 不含 approval/upload。
远程记录收尾进度与结果，已开始但结果未知的收尾不会因重启而自动重跑；任意用户 shell 的外部副作用仍不可自动识别。

```yaml
version: 1
timeout: 1h
params:
  configuration:
    default: Release
    description: 编译配置
    required: true
    choices: [Debug, Release]
env:
  CONFIGURATION: "{{configuration}}"
steps:
  - kind: run
    name: test
    run: bash ci/test.sh
post:
  timeout: 2m
  failure:
    - kind: run
      name: diagnostics
      run: bash ci/diagnostics.sh
  always:
    - kind: run
      name: cleanup
      run: bash ci/cleanup.sh
```

### MVP 日志、测试报告与保留策略

日志默认附带 UTC 时间戳、执行 ID、build 名称、步骤名称和输出流；同时保留 Agent 事件序号及服务端接收时间，跨节点时钟误差不改变事件顺序。
历史输出、SSE 与 --json 使用同一元数据，先按分段流脱敏再落盘/传输；无需每个项目开关 timestamps。

每个 build 可声明 reports.junit.paths（仓库相对 glob 列表）与 required（默认 true），例如下面的单 build 配置：

```yaml
version: 1
steps:
  - kind: run
    name: test
    run: bash ci/test.sh
reports:
  junit:
    paths: [build/test-results/**/*.xml]
    required: true
```

执行后即使测试命令失败也尝试收集报告，复用产物路径/符号链接/大小/摘要校验；不采集其他 build 或旧工作区文件。
使用 Go encoding/xml 解析 JUnit testsuite/testsuites，限制输入大小与嵌套深度，不展开外部实体或联网。
保存用例总数、失败/错误/跳过数、耗时与失败用例摘要，原 XML 作为产物下载；构建详情/JSON 展示摘要。
失败或错误用例使 build 失败并阻止之后的发布步骤；每个普通 run 结束后校验已生成的报告，普通执行结束完成最终收集，不能到上传后才发现测试失败。
required 的缺失检查在普通执行结束或发布审批/上传之前进行，不因前置准备步骤尚未生成报告而失败；required=false 的缺失不覆盖测试命令的非零退出。
同一路径报告重复解析按最终内容替换，不累加计数；required=true 无报告或非法 XML 报失败，required=false 仅允许无报告，有文件但非法仍报错。
本地 run 启动前记录匹配报告的文件身份、修改时间与内容摘要，只接受本次新建或改写的文件；不能证明新鲜的旧报告不计入，通过 required 规则处理。
不删除用户工作树中的旧文件；--step 同样检查报告/产物依赖，不能借旧文件绕过检查。远程使用全新执行工作区。
发布审批/首次上传前封存当前报告与产物 ID/摘要，保存同一执行的通过结论；审批恢复核对封存证据及实际上传产物，变化或缺失即失败。
MVP 含 upload 的流水线中，所有普通 run/artifact 必须位于 approval/upload 发布段之前，配置校验拒绝交错；不含 upload 的普通审批不受此顺序限制。
多个 upload 只消费同一封存证据；重新构建或测试须另建执行，不能在批准后替换产物。
post 只产生诊断日志/产物，不重写发布报告或审批证据；先确定普通执行和报告结果，再选择 post 列表，避免收尾测试在上传后改变放行条件。
MVP 不新增 unstable 状态或测试平台，必要检查失败不能仅标警告后继续发布。

项目管理设置增加 retention，与全局 server.retention 按字段继承，示例：

```yaml
retention:
  builds: 200
  days: 60
```

builds/days 须为正整数；省略字段继承全局，null/未知字段拒绝，不允许仓库 YAML 改写管理策略。
计数按项目所有命名 build 汇总；超出数量或天数的终态记录进入清理，但活动、待审批、停止未确认与上传 unknown 始终受保护。
日志、产物、测试原始报告及 Agent 工作区沿用同一策略；保护判断与执行删除前均重新核对状态，下载中的文件不半途删除。
中央删除使用持久化清理记录，先令产物不可再被新下载引用，等待已有下载结束再删除文件；计数器与必要操作审计不随历史文件清理重置。
节点工作区删除通过独立的管理指令领取/确认，不复用已经过期的执行租约；指令绑定节点、删除 ID 与工作区 ID，不接受任意路径。
删除前控制端复核保护状态，Agent 再确认无活动进程且路径在 data_dir 内；节点离线保留待清理记录，确认后才标完成，重复删除已不存在目录为成功。
MVP 完成受限清理与失败记录，部署模板和容量打磨仍后置。
