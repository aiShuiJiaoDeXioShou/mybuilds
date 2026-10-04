# mybuilds — 移动端构建发布工具（Go / CLI）

> 目标目录：`/Users/linghe/project/mybuilds`（项目已初始化，业务功能按路线逐步实现）

## Context

要造一个 Jenkins 式的构建发布工具，但**只服务移动端**，用 Go 实现，**纯 CLI 形态**（不做 Web 前端）。

与 Jenkins 的差异是刻意的：手机开发的痛点不是「缺一个调度器」，而是
**版本号/渠道/签名/分发链路**。Jenkins 需要插件拼装才能干的事，这里做成内置能力。

已确认的需求：

| 维度 | 决策 |
|---|---|
| 形态 | **客户端 + 控制端 + Agent 三种 CLI**；一个控制端管理多个构建节点，无浏览器 UI |
| 节点架构 | Agent 主动连接控制端；按平台、标签和容量调度；单次流水线固定节点 |
| 数据库 | 默认 SQLite，支持 PostgreSQL，统一通过 **ORM（GORM）** 访问 |
| 技术选型 | 标准库覆盖的能力直接使用；CLI / ORM / Webhook / cron / 飞书等采用成熟第三方库 |
| 配置 | 仓库内 `mybuilds.yml`（配置即代码） |
| 代码源 | GitLab / GitHub / Gitee / 任意自建 Git（通用 git 协议） |
| 触发 | Webhook（含签名校验）+ 轮询，外加手动触发 |
| 审批 | 支持发布审批（流水线中途挂起等人放行） |
| 隔离 | 裸机 shell，不用 Docker |
| 工程类型 | 先原生 Android(Gradle) + iOS(xcodebuild) |

原规划记录的本机环境：Go 1.25.4 · Xcode 27.0 · JDK 21 · Android SDK 齐全 · Node 24。
实施前由 `doctor` 重新检测，构建记录保存实际工具版本，不把这份环境记录当成固定要求。

**硬约束**：iOS 打包（`xcodebuild` + 签名）只能在 macOS 构建节点执行。
控制端 `mybuilds-server` 可部署 Linux/macOS，Agent 在具备工具链的节点执行任务：
macOS 节点支持 iOS/Android，Linux 节点支持 Android；客户端的远程命令保持跨平台。
支持一个控制端加一个或多个 Agent，同机部署同样使用 Agent。节点协议、故障边界见 [多节点设计](MULTI_NODE.md)。
当前仅完成双 CLI 初始化，多节点是设计目标，尚未实现。

## 关键设计选择

### 1. 技术选型：按实际能力引入依赖

| 需求 | 选择 | 理由 |
|---|---|---|
| CLI 框架 | `github.com/spf13/cobra` | 子命令树、自动补全、help 生成 |
| 服务端配置加载 | `github.com/spf13/viper` | 服务端配置与环境变量覆盖；固定覆盖优先级 |
| 流水线 / 客户端配置 | `go.yaml.in/yaml/v3` | 直接解码到结构体，严格拒绝未知字段；采用维护中的 YAML 项目 |
| HTTP 服务 | stdlib `net/http` | ServeMux 路由、鉴权中间件、SSE 流式刷新，无需 Gin |
| ORM / DB | `gorm.io/gorm` + `github.com/glebarez/sqlite`（默认）/ `gorm.io/driver/postgres` | 见设计选择 3 |
| Webhook 解析 | `github.com/go-playground/webhooks/v6` | 一期接 GitHub / GitLab，不为库支持的其他平台提前增加适配 |
| Gitee / 自建 Git | 自写轻量解析器 | 已核实 v6 **不支持 Gitee**；自建 Git 走可配置 JSON pointer 取 ref/sha |
| 定时构建 / 分支轮询 | `github.com/robfig/cron/v3` / stdlib `time` | cron 处理定时表达式；ticker 处理固定轮询间隔 |
| 产物 glob | `github.com/bmatcuk/doublestar/v4` | stdlib `path.Match` 不支持 `**` 递归，artifact 路径必须要 |
| CLI 列表渲染 | stdlib `text/tabwriter` + `encoding/json` | 默认表格，`--json` 供脚本使用 |
| 飞书通知 | `github.com/larksuite/oapi-sdk-go/v3` | 保留官方第三方 SDK，接入时先验证自定义机器人 Webhook 发消息闭环 |
| 企微 / 钉钉 / 通用通知 | stdlib `net/http` | 机器人本质是「POST 一个 JSON」，SDK 不带来增益 |

Git 操作走 `git` 命令行（`os/exec`），不用 go-git —— go-git 体积大、行为细节和真 git 有偏差，
而我们本来就要求机器上装了 git。

依赖按实施阶段引入并锁定版本，不在 P0 一次性安装全部依赖。
依据：[Go HTTP 路由](https://pkg.go.dev/net/http#ServeMux)、[YAML 维护项目](https://github.com/yaml/go-yaml)、[飞书官方 SDK](https://github.com/larksuite/oapi-sdk-go)。

CLI 分发形态：

```
cmd/mybuilds-server/  → mybuilds-server   服务端二进制
                 ├─ serve          起 HTTP API + 调度，不执行构建 shell
                 ├─ migrate        建表 / 升级 schema
                 ├─ project add|ls|rm
                 ├─ token create|ls|revoke
                 └─ node create|ls|drain|enable|disable|rm
cmd/mybuilds/  → mybuilds          客户端二进制（连服务端）
                 ├─ trigger / build ls|show|logs|cancel|retry
                 ├─ approvals / approve / reject
                 ├─ run           本地调试流水线，不经过服务端
                 └─ doctor / status / init
cmd/mybuilds-agent/ → mybuilds-agent      构建节点二进制（待 007 创建）
                 └─ serve / doctor / version
```

三个角色共享 internal 下的配置、流水线、scm、mobile 和 version，入口随功能创建。
Agent 与客户端本地 run 共用同一套引擎；控制端通过持久化队列和 Agent 协议管理执行。
本地模式结果写临时目录，不连接控制端数据库；平台进程控制放到对应构建约束文件。
Agent 使用标准库 HTTPS 主动领取任务、续租并回传日志/产物；一期不引入消息队列或共享文件系统。

### 2. 不用 Docker

Android/iOS 构建本来就要用宿主机 SDK（Xcode 无法在容器里好好跑）。
裸机 shell 直接省掉容器调度、镜像管理、缓存挂载一整套复杂度。

只构建管理员注册的可信仓库与分支，不执行外部 PR 或不可信流水线。控制端与各构建节点使用专用系统用户；
密钥按步骤注入，子进程不继承控制端或 Agent 完整环境。环境变量限制和日志脱敏不是安全隔离，
同一系统用户执行的任意脚本仍可能读取该用户可访问的文件。

### 3. 数据库默认 SQLite，可切 PostgreSQL，统一走 GORM

```yaml
database:
  driver: sqlite              # sqlite | postgres
  dsn: ~/.mybuilds/mybuilds.db
# 切 PostgreSQL：
# database:
#   driver: postgres
#   dsn: postgres://user:pass@host:5432/mybuilds?sslmode=disable
```

- 默认 SQLite：单机部署零运维。用 `github.com/glebarez/sqlite` —— **纯 Go（无 CGO）**驱动，
  不需要 C 编译器，可交叉编译，适合直接把二进制发给用户。
- PostgreSQL（`gorm.io/driver/postgres`，基于 pgx）作为可选数据库后端，启动时按配置选择。
  切换配置不会搬迁已有数据。数据库仅由单控制端访问，多 Agent 使用 API；SQLite 文件不能跨节点共享。
  多节点执行不要求 PostgreSQL；多控制端高可用不在一期范围。
- 两种驱动共用同一套 GORM 模型与仓库层，驱动在启动时按配置注入。
- SQLite 启用 WAL、外键约束和锁等待超时；保持短事务，日志按文件写入，不逐行入库。
  WAL 允许读写并行，但仍只有一个写入者。见 [SQLite WAL](https://sqlite.org/wal.html)。
- 构建号在事务中按项目原子分配，项目与构建号加唯一约束；审批与任务领取使用带原状态条件的更新。
- v1 用 GORM `AutoMigrate` 建表；涉及字段改名、数据回填等变化时采用显式版本迁移，不把 AutoMigrate 当作完整迁移方案。
- 纯 Go SQLite 驱动先验证与选定 Go / GORM 版本的兼容性，两种数据库运行相同的事务、分页和状态更新用例。

### 4. 审批与执行进度持久化

引擎到审批节点时保存 `waiting_approval` 并退出本次执行，释放节点与全局构建槽；调度器每 5s 查询数据库，
已批准的任务重新入队，从审批后的步骤继续。同项目在审批期间仍保持串行，其他项目可以使用空出的槽。
挂起前节点清理临时签名资源，并确认日志和产物已回传控制端；保留工作区与节点归属。
批准后从原节点以新租约恢复，节点离线时等待并提示；挂起不视为成功，不发送最终通知。
审批仅允许 `approver/admin`，一期不做审批分组；记录 token 身份、时间、意见，重复或相互冲突的决定不得覆盖。

构建保存 commit SHA、流水线配置快照、当前步骤、工作区、产物记录与工具版本；快照保留密钥引用，不存密钥明文。
控制端启动时恢复排队与待审批任务，并核对持久化节点租约；不能将控制端重启当作节点中断。
节点崩溃或租约过期时普通执行步骤标为 `interrupted`，由用户显式重试，不自动迁移重跑。
工作区或产物缺失时终止恢复并报告原因，不能从头静默重跑。

上传前保存操作记录；若远端接收后本地未记录成功，标为结果未知，优先查询远端状态，无法确认时交由人工处理，
禁止自动重发。普通重试生成新的构建号，复用原 SHA、配置和构建参数；未知上传结果确认前不能重试发布。
管理员可查询或人工确认上传结果，记录证据与意见后解除未知状态；确认已发布时不再执行该次上传。

### 5. 版本号是一等公民

移动端最高频需求：构建号自增。内置 `{{build.number}}`、`{{git.sha}}`、`{{git.branch}}`、
`{{version}}` 模板变量，直接喂给 `versionCode` / `CURRENT_PROJECT_VERSION`。

显式记录移动端版本与渠道参数，同一 SHA 可构建不同渠道；一期同一平台、同一应用的渠道放在同一项目内，
避免多个项目独立分配构建号后出现冲突。版本规则按平台校验，不假设 Android 与 iOS 完全一致。

### 6. 工作区、收尾与资源控制

- Agent 先按控制端确定的 SHA 准备节点工作区，再读取该提交的 `mybuilds.yml`，checkout 是前置操作；本地 `run` 使用当前工作树，不重置用户修改。
- 常规步骤失败即停；成功、失败、取消均执行统一收尾：最终通知、临时 keychain 清理。通知由控制端发送，失败单独记录，不覆盖构建结果。
- 默认每节点容量为 1、全局并发为 1，均可配置；同项目跨节点串行；记录排队时间、步骤耗时、峰值内存与工作区大小后再提高并发。
- 各节点工作区按构建隔离，复用工具自身的依赖下载缓存；iOS DerivedData 和临时 keychain 按构建隔离，不新增通用缓存系统。
- 取消先向构建进程组发送 TERM，超时后 KILL 并回收子进程；不得杀同用户的全部 Java / Xcode 进程。Gradle 默认 `--no-daemon`，真实构建验证残留进程。

### 7. 配置变量与鉴权

- 配置先解码与校验，再对明确支持的字段插值；仅解析 `env`、凭据等字段里的 `${SECRET}`，缺失时报错。
  `run` 正文的 `$VAR` / `${VAR}` 留给 shell；分支名、版本等值通过环境变量传入，不直接拼接到 shell 命令中。
- 支持既定的 `{{var}}`，但 `run` 正文不做模板替换；未知模板变量报错，`--dry-run` 输出不展示密钥。
- token 存数据库，只保存高熵 token 的摘要和身份、角色、撤销状态；创建时仅显示一次明文，列表不回显。
  首次启动且 token 表为空时，可用 `MYBUILDS_BOOTSTRAP_ADMIN_TOKEN` 初始化管理员，之后撤销不会被配置重新创建。
- 项目管理可由本机管理员 CLI 操作；admin 可访问全部 API，trigger 仅能触发及查询基本服务状态，
  approver 可列待审批任务、读取其详情 / 日志 / 产物并批准或拒绝。取消、重试、上传结果确认与控制端 / 节点 doctor 仅限 admin。

## 架构

```text
客户端 / Webhook / 轮询 / 定时
              │
              ▼
mybuilds-server 控制端（单进程，Linux/macOS）
  ├─ HTTP API、鉴权、项目、构建号、审批与调度
  ├─ GORM → SQLite / PostgreSQL
  └─ 中央日志与产物存储
              ▲ HTTPS：Agent 主动领取、续租、回传
              ├─ macOS Agent A → Xcode / Gradle
              ├─ macOS Agent B → Xcode / Gradle
              └─ Linux Agent C → Gradle

mybuilds run → 本机同一流水线引擎 → 临时目录
```

完整约束见 [MULTI_NODE.md](MULTI_NODE.md)：节点注册、授权、能力匹配、并发、租约、
失联、控制端重启、审批固定节点、日志与产物回传、签名资源和上传结果未知。
一次流水线固定一个节点；增加节点提高不同项目的并发，不做跨节点分步执行或多控制端高可用。

## 配置文件

### 服务端配置 `~/.mybuilds/server.yml`（由 `mybuilds-server serve` 读取）

```yaml
listen: 127.0.0.1:8787       # 本地监听；跨主机通过校验证书的 HTTPS 入口访问
data_dir: ~/.mybuilds
concurrency: 1              # 所有节点合计的上限，增加节点时显式提高

database:                   # 见设计选择 3，默认 sqlite
  driver: sqlite            # sqlite | postgres
  dsn: ~/.mybuilds/mybuilds.db

secrets_file: ~/.mybuilds/secrets.env     # 0600，仅控制端通知等密钥；构建密钥在节点解析
retention: {builds: 100, days: 30}        # 每个项目保留策略
notifiers:
  feishu:   {webhook: "${FEISHU_WEBHOOK}"}    # oapi-sdk-go
  wechat:   {webhook: "${WECHAT_WEBHOOK}"}    # stdlib POST
  dingtalk: {webhook: "${DINGTALK_WEBHOOK}"}  # stdlib POST
```

配置覆盖顺序为默认值 < 配置文件 < `MYBUILDS_` 环境变量 < CLI 参数。
token 由数据库管理；首次启动可设置 `MYBUILDS_BOOTSTRAP_ADMIN_TOKEN`，随后用 `token create/revoke` 管理。
日志、产物和工作区保留策略不清理排队、运行中、待审批或结果未知的构建。

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
建议默认时序见 MULTI_NODE，协议细节与参数合法性在对应 feature 中验证；跨主机 HTTPS 必须验证证书。

### 客户端配置 `~/.mybuilds/client.yml`（由 `mybuilds` 读取）

```yaml
server: http://127.0.0.1:8787
token: "${MYBUILDS_CLIENT_TOKEN}"   # 也支持环境变量覆盖
```

### 项目注册（`mybuilds-server project` 写入数据库）

```bash
mybuilds-server project add app-android \
  --repo git@gitlab.example.com:team/app.git \
  --provider gitlab --branches main,release/* \
  --hook          # 打印 webhook URL 和 secret
mybuilds-server project add app-ios --repo ... --poll 60s    # 轮询模式
```

### `<repo>/mybuilds.yml`（流水线，随代码走）

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

notifications:                          # 统一收尾通知，常规步骤失败后仍执行
  on: [success, failure, cancelled]
  to: [feishu]
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
    notify: [feishu]

  - kind: upload                        # 内置分发，非 shell 拼 curl
    target: fir                         # fir | generic
    file: "*.apk"                       # 匹配已收集产物；零个或多个匹配均报错
    channel: "{{channel}}"
    api_key: ${FIR_API_KEY}
```

步骤类型只做 4 种：`run` / `artifact` / `approval` / `upload`；checkout 是前置操作，最终通知属于统一收尾。
服务端固定到触发时确定的 SHA，浅克隆无法取得该 SHA 时补充 fetch，不退回分支最新提交。
本地 `run` 遇到 approval 时交互确认，无终端时拒绝执行该步骤；`--dry-run` 不运行命令、不发通知、不上传。
**不做插件系统** —— 非内置能力（App Store、Google Play、自定义市场）就是 `kind: run` 调它们的 CLI / fastlane。
这些市场 API 又长又会变，硬编码进来只会变成维护负担。

v1 步骤**顺序执行**；多渠道先通过参数分别触发，`parallel:` 和矩阵构建等真实需求出现后再加。

## 触发链路

| 来源 | 识别方式 | 校验 |
|---|---|---|
| GitLab | `X-Gitlab-Event: Push Hook` | `X-Gitlab-Token` 明文比对（constant-time） |
| GitHub | `X-GitHub-Event: push` | `X-Hub-Signature-256` HMAC-SHA256(body) |
| Gitee | `X-Gitee-Event` | `X-Gitee-Token` 明文比对 |
| 自建 Git | 项目注册时指定 provider | 请求头 token 或 HMAC；payload 用**可配置 JSON pointer** 取 ref/after |

统一入口 `POST /hook/{project}`，按项目注册的 provider 校验，不凭请求头切换解析器。
先校验原始请求体和事件，再在事务中持久化事件与排队任务，成功后响应；限制请求体大小，token 不放 URL。

事件去重使用 provider 的 delivery ID（例如 GitHub `X-GitHub-Delivery`）；没有事件 ID 的来源，
按项目、分支、SHA、构建参数合并已有排队、运行中、待审批或成功的自动构建。
手动重跑及不同版本 / 渠道参数的构建允许同一 SHA，不能仅按 SHA 永久去重。
参考 [GitHub Webhook 重投与 delivery ID](https://docs.github.com/en/webhooks/using-webhooks/best-practices-for-using-webhooks)。
v1 只处理 push 事件，tag/PR 事件忽略。分支用 `--branches` 的 glob 过滤。

轮询用 `git ls-remote <url> <branch>` 拿 SHA 和上次比对 —— 不需要本地克隆就能探测变更，
轮询游标与任务入队在同一事务提交，避免游标已推进但构建未创建。

## 文件清单

以下为采用的目标结构；业务文件、examples 和 deploy 随对应功能创建，不预建空包。
当前已实现内容见项目 [README](../../README.md)。
控制端生命周期、HTTP、调度和恢复归入 internal/server；节点运行归入 internal/agent。
CLI 按 client/server/agent 分包，共享的网络消息契约放 internal/protocol，执行引擎不依赖控制端数据库。

```
go.mod
cmd/mybuilds-server/main.go    # 服务端二进制入口：serve / migrate / project / token
cmd/mybuilds/main.go           # 客户端二进制入口：trigger / build / approvals / run / doctor
cmd/mybuilds-agent/main.go     # Agent 入口：serve / doctor / version（007 创建）

internal/cli/server/root.go    # 服务端子命令（cobra：serve/migrate/project/token）
internal/cli/client/root.go    # 客户端子命令（cobra：trigger/build/approvals/run/doctor）
internal/cli/agent/root.go     # Agent 子命令（cobra：serve/doctor/version）
internal/config/pipeline.go     # YAML 结构体、严格校验、按字段插值，run 正文保留 shell 变量
internal/config/server.go       # server.yml：listen/dsn/secrets/retention，Viper 覆盖
internal/config/client.go       # client.yml：server 地址 + token
internal/config/agent.go        # agent.yml：控制端、节点凭据、容量与本地目录
internal/version/version.go     # 版本信息（ldflags 注入，两端共用）
internal/pipeline/engine.go     # 顺序执行、恢复位置、ctx 取消、进程组 kill、统一收尾
internal/pipeline/run.go        # shell 执行与进程取消
internal/pipeline/artifact.go   # 产物收集与路径校验
internal/pipeline/approval.go   # 审批挂起与继续
internal/pipeline/upload.go     # 分发与未知结果处理
internal/pipeline/mask.go       # 日志 writer，把密钥值替换成 ***

internal/store/store.go         # GORM 模型 + 仓库层，AutoMigrate，sqlite/postgres 双驱动注入
internal/store/models.go        # Project / Node / Build / Lease / Step / Approval / Event / Token，含上传操作记录
internal/scm/git.go             # clone / fetch / ls-remote / sha / 工作区准备
internal/scm/hook.go            # webhooks/v6（GitHub/GitLab）+ Gitee/自建解析与事件去重

internal/server/server.go       # 服务生命周期、组件组装与优雅关闭
internal/server/http.go         # net/http 路由、鉴权、JSON API、日志 SSE 与产物下载
internal/server/scheduler.go    # 定时/轮询 + 并发槽 + 同项目串行 + 审批扫描
internal/server/recovery.go     # 持久化租约核对、启动恢复与中断状态处理
internal/agent/agent.go         # 领取/续租、节点执行与断网停止
internal/agent/report.go        # 脱敏日志、事件与产物回传
internal/protocol/messages.go   # 双端共用的任务、租约与回报格式
internal/notify/notify.go       # 飞书(oapi-sdk-go) / 企微 / 钉钉 / 通用 webhook

internal/mobile/android.go      # gradle 辅助：版本、keystore 注入、JDK 与 SDK 检查
internal/mobile/ios.go          # archive/export、exportOptions、独立 keychain 与 DerivedData

examples/android.mybuilds.yml
examples/ios.mybuilds.yml
```

文件按实际实现需要拆分，不把文件数当作架构目标；平台相关进程控制另用构建约束文件。

## 服务端 HTTP API（`mybuilds-server`）

```
POST   /hook/{project}                     # webhook（外部，token/HMAC 校验）
GET    /api/projects                       # 项目列表
GET    /api/doctor                         # 控制端体检（admin）
GET    /api/nodes                          # 节点健康、标签与容量（admin）
GET    /api/nodes/{id}/doctor              # 节点工具链体检（admin）
POST   /api/projects/{p}/builds            # 手动触发
GET    /api/builds?project=&status=&limit= # 构建列表（分页）
GET    /api/builds/{id}                    # 单个构建详情 + 步骤状态
GET    /api/builds/{id}/log?step=&follow=1 # 日志（follow 走 SSE tail）
POST   /api/builds/{id}/cancel             # 保存取消意图，节点终止本次进程组并确认
POST   /api/builds/{id}/retry              # 原 SHA / 配置 / 参数，新构建号，检查未知上传结果
POST   /api/builds/{id}/upload-resolution  # 确认未知上传结果并记录证据（admin）
GET    /api/approvals                      # 待审批列表
POST   /api/builds/{id}/approve            # 需要 approver/admin 角色
POST   /api/builds/{id}/reject             # 需要 approver/admin 角色
GET    /api/artifacts/{id}/{file}          # 产物下载
```

### Agent HTTP 接口（待 007 契约细化）

节点登录身份从独立 token 推导；领取、续租、步骤事件、日志、产物与发布意图均通过专用节点路由。
每个执行请求绑定 build/attempt/lease 和节点身份，校验过期执行权、顺序、重复、路径及请求体大小。
复用标准库 HTTP，不允许节点直接访问数据库或管理员 API。

## CLI 面

### 服务端 `mybuilds-server`（运维 / 管理员用）

```bash
mybuilds-server serve   --config ~/.mybuilds/server.yml   # 起控制端（API + 调度）
mybuilds-server migrate --config ...                     # 建表 / 升级 schema（sqlite 或 postgres）
mybuilds-server project add app-android \
    --repo git@gitlab.example.com:team/app.git \
    --provider gitlab --branches main,release/* \
    --hook          # 打印 webhook URL 与 secret
mybuilds-server project add app-ios --repo ... --poll 60s --schedule "0 9 * * *"
mybuilds-server project ls|rm
mybuilds-server token create --role approver             # 生成 approver / trigger / admin token
mybuilds-server token ls|revoke
mybuilds-server node create mac-ios-01 --labels xcode,ios-signing  # 一次性显示独立节点 token
mybuilds-server node ls|drain|enable|disable|rm
```

### Agent `mybuilds-agent`（构建节点，待 007 实现）

```bash
mybuilds-agent serve --config ~/.mybuilds/agent.yml
mybuilds-agent doctor
mybuilds-agent version
```

### 客户端 `mybuilds`（开发者 / 审批人用）

```bash
mybuilds init                             # 在当前仓库生成 mybuilds.yml 模板
mybuilds run [--step name] [--dry-run]    # 本地直接跑流水线，不经服务端（调试用）
mybuilds trigger <project> [--branch main] [--ref sha] [--version 1.0.0] [--channel 内测]
mybuilds build ls|show|cancel|retry
mybuilds build resolve-upload <id> --step name --result sent|not-sent --note "确认依据"
mybuilds logs <build-id> [-f]             # -f 流式 tail（SSE）
mybuilds approvals                        # 看哪些在等人放行
mybuilds approve <build-id> [--note "已测过"]
mybuilds reject  <build-id> [--note "打回原因"]
mybuilds status                           # 连通性 + 服务端版本 + 当前并发
mybuilds doctor [--server] [--node name]    # 本机 / 控制端 / 指定节点（远程需 admin）
mybuilds version
```

列表与详情命令支持 `--json`；`trigger --branch` 在触发时解析并固定最新 SHA，`retry` 始终重跑原提交。
`doctor --server` 返回控制端检查结果，`doctor --node` 返回对应节点工具链状态，两者不返回密钥；`resolve-upload` 记录人工确认依据，已发布则不再上传，未发送才允许重试。
`build show` 展示执行节点、attempt / 租约状态、配置快照、工具版本、审批记录和上传结果；产物记录包含大小和 SHA-256，下载与收集校验路径边界及符号链接。

`doctor` 不是锦上添花 —— iOS 签名失败是移动端 CI 的头号故障，一个能提前告诉你
「keychain 没解锁 / 描述文件过期 / Gradle 用错 JDK」的命令能省掉大量排查时间。

## 实施步骤

### P0 骨架与流水线预览

- [x] `000-project-bootstrap`：Go 单模块、Cobra、双 CLI 帮助与共享 version；README 与 AI 阅读入口
- [ ] `001-pipeline-preview`：复用已有入口，引入 YAML，增加 `init` 和 `run --dry-run`；其他依赖随对应阶段加入并锁定版本
- [ ] `internal/config/pipeline.go`：严格字段与步骤校验、参数默认值、按字段插值；保留 run 正文中的 shell 变量
- [ ] `internal/pipeline/mask.go` + 单测
- [ ] `mybuilds run --dry-run` 打印脱敏计划，不触发命令或外部动作

### P1 本地执行与真实移动端构建

- [ ] `internal/scm/git.go`：按 SHA 准备节点工作区；本地 run 使用当前工作树，不改写用户仓库
- [ ] `run` / `artifact`：顺序执行、失败即停、统一收尾，产物大小与 SHA-256 记录
- [ ] 节点工作区 `<agent.data_dir>/builds/<project>/<number>/{src,logs,artifacts}`；本地结果写临时目录，日志按步骤分文件
- [ ] ctx 取消、进程组 TERM/KILL 与回收；按平台隔离实现，避免影响远程客户端编译
- [ ] 假项目验证成功、失败、取消均执行收尾，日志与 dry-run 脱敏
- [ ] Android 模板：Gradle 版本参数、keystore 注入、apk / aab / mapping 收集，复用依赖缓存
- [ ] iOS 模板：archive/export、按当前 Xcode 支持值生成 ExportOptions、独立 DerivedData 和临时 keychain、ipa / dSYM 收集与清理
- [ ] 基础 `doctor`：git 凭据、JDK / Android SDK、Xcode、签名证书及描述文件检查
- [ ] 一个真实 Android 与一个真实 iOS 工程构建成功，核对版本、产物、签名和取消后的残留进程

### P2 控制端 + 多节点调度

- [ ] 加入 Viper、GORM 与双驱动；验证纯 Go SQLite 驱动兼容性，设置 WAL / 外键 / 锁等待
- [ ] 模型保存构建配置快照、参数、SHA、当前步骤、工具版本与产物；项目构建号事务分配
- [ ] SQLite 与 PostgreSQL 跑相同的 CRUD / 事务 / 条件状态更新 / 唯一约束 / 分页用例
- [ ] token 摘要存储、一次性管理员初始化、创建 / 撤销 / 身份审计，统一读写权限检查
- [ ] 数据库持久化队列、默认并发 1、同项目串行，状态变更使用条件更新；单控制端分配给多个 Agent 执行
- [ ] `net/http` JSON API、鉴权、日志 SSE tail、受鉴权的产物下载与控制端/节点 doctor
- [ ] 节点注册/独立 token/标签/容量/drain，Agent 跨主机 HTTPS 主动领取与续租
- [ ] 原子节点分配、attempt/租约归属、过期回报拒绝、断网停止，无匹配节点时保持排队
- [ ] 节点脱敏日志与校验产物回传；中央保存，不共享数据库或工作区
- [ ] `mybuilds-server serve` + 多个 `mybuilds-agent serve` + `trigger` + `build ls/show` + `logs -f` 闭环
- [ ] 列表 / 详情支持 `--json`；记录排队、步骤耗时、峰值内存与磁盘占用

### P3 重启恢复与审批

- [ ] approval 保存进度并释放全局槽；5s 扫描已批准任务重新入队，保持同项目串行
- [ ] `approve` / `reject` API + CLI：角色、身份、时间、意见、重复决定与取消竞态检查
- [ ] 启动时恢复排队与待审批任务；控制端恢复与节点核对；节点租约过期的普通执行任务标为 interrupted，不自动迁移重跑 shell
- [ ] 重试固定原 SHA / 配置 / 参数并分配新构建号，不受分支后续提交影响
- [ ] 飞书使用 `oapi-sdk-go/v3`，先验证自定义机器人 Webhook 的消息、签名与错误处理；企微 / 钉钉 / generic 用 stdlib HTTP
- [ ] 最终通知覆盖成功 / 失败 / 取消；审批挂起不执行终态通知或删除恢复所需文件
- [ ] 重启租约核对、节点断网、审批释放槽、原节点恢复、重复批准、工作区缺失、本地交互审批验证

### P4 自动触发：Webhook + 轮询

- [ ] 加入 webhooks/v6，接 GitHub / GitLab；Gitee 与自建 Git 用轻量解析器
- [ ] 按已注册 provider 校验原始 body、签名 / token、事件与分支，限制请求大小
- [ ] 安全用例：HMAC 篡改 body、错误 token、provider 伪装均拒绝
- [ ] delivery ID 去重，事件与任务事务入库后响应；不同渠道与手动重试允许同 SHA
- [ ] 每项目 ticker + `git ls-remote`，游标与任务入队事务提交；cron 实现定时构建
- [ ] 分支 glob 过滤，忽略 tag / PR / 分支删除事件

### P5 产物与分发

- [ ] `upload`：fir.im / generic multipart，上传前保存操作记录，校验产物匹配唯一性
- [ ] 上传结果未知时查询远端或由 admin 用 `resolve-upload` 确认并记录依据，阻止自动重发及未确认的发布重试
- [ ] 配置快照与上传产物绑定，审批后发布原产物，不重新构建
- [ ] retention 清理终态构建（保留 N 个 / N 天），保护待审批与未知结果任务
- [ ] fir.im 与 generic 上传闭环，验证失败与中断后的状态

### P6 部署与打磨

- [ ] macOS `launchd` 与 Linux `systemd` 模板；控制端/Agent 分别自动启动，运行目录独立
- [ ] 优雅关闭：控制端停止分配，Agent 停止领取并取消自身构建进程，按租约保存状态；专用用户和可信分支部署说明
- [ ] `build ls` 过滤与分页、日志流重连、doctor 输出打磨
- [ ] 基于真实构建数据调整并发；按实际瓶颈优化依赖缓存，不新增通用缓存系统

App Store / Google Play 上传**不做内置**，文档里给 `kind: run` + fastlane 的示例即可。

## Verification

**单元测试（只测值得测的）**

- `internal/config`：未知字段 / 模板变量报错、env 字段插值、run 正文保留 `${VAR}`、引用的密钥缺失时报错、dry-run 无副作用
- `internal/store`：**双驱动**跑同一套事务 / 构建号分配 / 去重 / 条件更新 / 分页用例；token 撤销后不得再次初始化
- `internal/pipeline`：成功 / 失败 / 取消均收尾，分段日志中的密钥被脱敏，产物路径及符号链接不能越界
- `internal/scm/hook.go`：**四种来源的签名校验**，含 GitHub HMAC 篡改 body 必须拒绝、错误 token 必须拒绝
- `internal/server`：同项目串行、审批释放全局槽、重复事件只入队一次、同 SHA 不同参数可触发、轮询游标与入队同时提交
- `internal/agent` / 节点协议：独立身份、双节点竞争、错误平台、租约过期、断网取消、幂等回报、日志与产物回传
- 审批与发布：重复批准 / 取消不能覆盖终态；重启后原节点正确继续；未知上传结果不能自动重发

**端到端（本地，无需真机）**

```bash
mybuilds run                       # fake 项目验证 run/artifact；本地 approval 交互确认
mybuilds run --dry-run             # 脱敏输出，不执行任何外部动作
mybuilds-server serve --config ~/.mybuilds/server.yml &   # 起控制端
# 按节点配置，另行启动至少两个 mybuilds-agent serve 进程
mybuilds trigger demo --branch main --version 1.0.0 --channel 内测
# 四类 Webhook 使用对应 testdata 与请求头签名 / token 发送，不能用 URL token 代替 GitHub HMAC
mybuilds logs <id> -f               # 看到流式日志
mybuilds approvals                  # 卡在 approval 步骤
mybuilds approve <id> && mybuilds build show <id>   # 继续并成功
# 构建失败 / 取消 → 收尾通知与 keychain 清理仍执行，进程无残留
# 一个任务等审批 → 其他项目仍可构建，同项目保持串行
# kill 控制端 → 重启 → 核对节点租约并恢复排队 / 待审批，不误判仍在续租的构建
# 节点断网 / 崩溃 → 租约过期，本次进程停止；不把已开始构建自动迁移到另一节点
# 分支更新后 retry → 原 SHA / 配置 / 参数、新构建号；缺失工作区不能静默重跑
# 模拟远端收到上传但本地未写成功 → 结果未知且不会自动重发
# 数据保留清理 → 不删除待审批和未知结果任务
# 独立 PostgreSQL 测试库运行同一套用例，不假设切配置会迁移 SQLite 数据
```

**真实构建验证**

1. 一个真 Android 工程：出 apk + aab + mapping.txt，versionCode 等于构建号，产物可下载并校验摘要
2. 一个真 iOS 工程：archive → export → ipa + dSYM，签名有效（`codesign -dv` 校验）
3. GitLab / GitHub / Gitee / 一个自建 Git 各连一次，push 触发成功；把某个 hook 的 secret 改错，确认被拒
4. 轮询模式：手动在仓库推一次 commit，确认在间隔内被探测并触发
5. 飞书官方 SDK：真实 Webhook 发消息成功，错误凭据 / 签名能报告失败，日志不泄露密钥
6. Android / iOS 取消与失败后无本次构建残留进程、临时 keychain；节点独立缓存且不共享构建工作区
7. 两个真实节点执行不同项目，同项目跨节点串行；iOS 不分配给 Linux，无合格节点明确排队
8. 暂停原 Agent 后模拟过期回报，确认拒绝；审批后原节点离线不迁移，失联上传不自动重发

## 明确不做（一期）

- 不做 Web UI（纯 CLI）
- 不做插件系统 / 自定义步骤 DSL
- 不做 Docker 隔离；支持多构建节点，不做多控制端高可用或单条流水线跨节点迁移
- 不做 RBAC / 多租户（token 角色只三档）
- 不做 App Store / Google Play 内置上传
- 不做并行步骤块、矩阵构建

## 已确定的部署与范围

1. 一个控制端（Linux/macOS）管理多个 Agent。iOS 仅在 macOS 节点构建，Android 可在 Linux/macOS；客户端远程命令跨平台。
2. 数据库默认 SQLite，支持 PostgreSQL；初期用 GORM `AutoMigrate` 建表，需要改名或回填时再增加显式版本迁移。
3. 审批通知渠道按 飞书（oapi-sdk-go）/ 企业微信 / 钉钉 / 通用 webhook 实现，无内置邮件与短信；企微与钉钉机器人走 stdlib HTTP POST。
4. 构建号用「每项目自增整数」，不做语义化版本自动推导；应用版本由 YAML 参数或触发参数提供，一期忽略 tag 事件。
