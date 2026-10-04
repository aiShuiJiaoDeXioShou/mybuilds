# mybuilds — 移动端构建发布工具（Go / CLI）

> 目标目录：`/Users/linghe/project/mybuilds`（项目已初始化，业务功能按路线逐步实现）

## Context

要造一个 Jenkins 式的构建发布工具，但**只服务移动端**，用 Go 实现，**纯 CLI 形态**（不做 Web 前端）。

与 Jenkins 的差异是刻意的：手机开发的痛点不是「缺一个调度器」，而是
**版本号/渠道/签名/分发链路**。Jenkins 需要插件拼装才能干的事，这里做成内置能力。

已确认的需求：

| 维度 | 决策 |
|---|---|
| 形态 | **服务端 + 客户端两个二进制，均为 CLI 分发**，无浏览器 UI |
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

**硬约束**：iOS 打包（`xcodebuild` + 签名）只能在 macOS 上执行。因此**服务端 `mybuilds-server` 部署在 macOS 上**，
Android 构建也在同一台机器跑 —— 单机即可覆盖两个平台，不引入「远程执行器」这一层。
客户端 `mybuilds` 可以跑在任何装了 git 的机器上。

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
                 ├─ serve          起 HTTP API + 调度 + 引擎
                 ├─ migrate        建表 / 升级 schema
                 ├─ project add|ls|rm
                 └─ token create|ls|revoke
cmd/mybuilds/  → mybuilds          客户端二进制（连服务端）
                 ├─ trigger / build ls|show|logs|cancel|retry
                 ├─ approvals / approve / reject
                 ├─ run           本地调试流水线，不经过服务端
                 └─ doctor / status / init
```

两个二进制共享 `internal/` 下的引擎、配置、scm、mobile 等包，各自带 `version` 子命令
（共享 `internal/version`），不额外开第三个二进制。客户端的 `run` 直接复用同一套引擎，
本地模式把运行结果写入临时目录，不连接服务端数据库。CLI 的远程命令可跨平台使用；
本地执行按宿主机能力限制平台，macOS 进程组操作放到平台文件中，避免影响客户端交叉编译。

### 2. 不用 Docker

Android/iOS 构建本来就要用宿主机 SDK（Xcode 无法在容器里好好跑）。
裸机 shell 直接省掉容器调度、镜像管理、缓存挂载一整套复杂度。

只构建管理员注册的可信仓库与分支，不执行外部 PR 或不可信流水线。用专用 macOS 用户运行服务；
密钥按步骤注入，子进程不继承服务端完整环境。环境变量限制和日志脱敏不是安全隔离，
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
  切换配置不会搬迁已有数据，也不提供集群能力；任务抢占、进程归属、工作区与产物共享需要另行设计。
- 两种驱动共用同一套 GORM 模型与仓库层，驱动在启动时按配置注入。
- SQLite 启用 WAL、外键约束和锁等待超时；保持短事务，日志按文件写入，不逐行入库。
  WAL 允许读写并行，但仍只有一个写入者。见 [SQLite WAL](https://sqlite.org/wal.html)。
- 构建号在事务中按项目原子分配，项目与构建号加唯一约束；审批与任务领取使用带原状态条件的更新。
- v1 用 GORM `AutoMigrate` 建表；涉及字段改名、数据回填等变化时采用显式版本迁移，不把 AutoMigrate 当作完整迁移方案。
- 纯 Go SQLite 驱动先验证与选定 Go / GORM 版本的兼容性，两种数据库运行相同的事务、分页和状态更新用例。

### 4. 审批与执行进度持久化

引擎到审批节点时保存 `waiting_approval` 并退出本次执行，释放全局构建槽；调度器每 5s 查询数据库，
已批准的任务重新入队，从审批后的步骤继续。同项目在审批期间仍保持串行，其他项目可以使用空出的槽。
挂起前清理临时签名资源，保留工作区与产物；挂起不视为成功，不发送最终通知。
审批仅允许 `approver/admin`，一期不做审批分组；记录 token 身份、时间、意见，重复或相互冲突的决定不得覆盖。

构建保存 commit SHA、流水线配置快照、当前步骤、工作区、产物记录与工具版本；快照保留密钥引用，不存密钥明文。
启动时恢复排队与待审批任务；崩溃时正在执行的普通步骤标为 `interrupted`，由用户重试。
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

- 服务端先按确定的 SHA 准备工作区，再读取该提交的 `mybuilds.yml`，checkout 是前置操作；本地 `run` 使用当前工作树，不重置用户修改。
- 常规步骤失败即停；成功、失败、取消均执行统一收尾：最终通知、临时 keychain 清理。通知失败单独记录，不覆盖构建结果。
- 默认全局并发为 1，可配置；记录排队时间、步骤耗时、峰值内存与工作区大小后再提高并发。
- 工作区按构建隔离，复用工具自身的依赖下载缓存；iOS DerivedData 和临时 keychain 按构建隔离，不新增通用缓存系统。
- 取消先向构建进程组发送 TERM，超时后 KILL 并回收子进程；不得杀同用户的全部 Java / Xcode 进程。Gradle 默认 `--no-daemon`，真实构建验证残留进程。

### 7. 配置变量与鉴权

- 配置先解码与校验，再对明确支持的字段插值；仅解析 `env`、凭据等字段里的 `${SECRET}`，缺失时报错。
  `run` 正文的 `$VAR` / `${VAR}` 留给 shell；分支名、版本等值通过环境变量传入，不直接拼接到 shell 命令中。
- 支持既定的 `{{var}}`，但 `run` 正文不做模板替换；未知模板变量报错，`--dry-run` 输出不展示密钥。
- token 存数据库，只保存高熵 token 的摘要和身份、角色、撤销状态；创建时仅显示一次明文，列表不回显。
  首次启动且 token 表为空时，可用 `MYBUILDS_BOOTSTRAP_ADMIN_TOKEN` 初始化管理员，之后撤销不会被配置重新创建。
- 项目管理可由本机管理员 CLI 操作；admin 可访问全部 API，trigger 仅能触发及查询基本服务状态，
  approver 可列待审批任务、读取其详情 / 日志 / 产物并批准或拒绝。取消、重试、上传结果确认与服务器 doctor 仅限 admin。

## 架构

```
  git push ──────┐   ┌───────────────────────────────────────────────┐
  (webhook)      ├──►│  mybuilds-server          (macOS 守护进程)     │
                 │   │   ├─ HTTP API      net/http (127.0.0.1:8787) │
  git ls-remote ◄┘   │   ├─ 调度器        cron + ticker             │
  (轮询)              │   ├─ 调度队列      并发槽 + 同项目串行         │
                     │   ├─ ORM  GORM ──► SQLite(默认) / PostgreSQL │
                     │   ├─ 引擎 → 子进程 (gradlew / xcodebuild)     │
                     │   └─ 存储           产物 / 日志               │
                     └───────────────────────────────────────────────┘
                                      ▲
                                      │ HTTP + Bearer token（role）
                     ┌────────────────┴───────────────────────┐
                     │  mybuilds                 客户端二进制    │
                     │  trigger / approve / logs -f / build ls │
                     └─────────────────────────────────────────┘
                              │
                              │ 直接跑本地流水线（调试用，不经服务端）
                              └─► mybuilds run → 同一套引擎 → 临时目录
```

服务端与客户端是**两个独立二进制**，均以 CLI 形式分发，共享 `internal/` 包。
客户端 `mybuilds run` 直接复用引擎，在本机跑一次流水线，用于调试 yaml。

## 配置文件

### 服务端配置 `~/.mybuilds/server.yml`（由 `mybuilds-server serve` 读取）

```yaml
listen: 127.0.0.1:8787
data_dir: ~/.mybuilds
concurrency: 1              # 全局并发构建槽，实测资源占用后调整

database:                   # 见设计选择 3，默认 sqlite
  driver: sqlite            # sqlite | postgres
  dsn: ~/.mybuilds/mybuilds.db

secrets_file: ~/.mybuilds/secrets.env     # 0600，显式引用的 ${VAR} 从这里和环境变量解析
retention: {builds: 100, days: 30}        # 每个项目保留策略
notifiers:
  feishu:   {webhook: "${FEISHU_WEBHOOK}"}    # oapi-sdk-go
  wechat:   {webhook: "${WECHAT_WEBHOOK}"}    # stdlib POST
  dingtalk: {webhook: "${DINGTALK_WEBHOOK}"}  # stdlib POST
```

配置覆盖顺序为默认值 < 配置文件 < `MYBUILDS_` 环境变量 < CLI 参数。
token 由数据库管理；首次启动可设置 `MYBUILDS_BOOTSTRAP_ADMIN_TOKEN`，随后用 `token create/revoke` 管理。
日志、产物和工作区保留策略不清理排队、运行中、待审批或结果未知的构建。

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
    target: pgyer                       # pgyer | fir | generic
    file: "*.apk"                       # 匹配已收集产物；零个或多个匹配均报错
    channel: "{{channel}}"
    api_key: ${PGYER_API_KEY}
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
服务生命周期、HTTP、调度和恢复统一归入 internal/server；客户端与服务端 CLI 分包。

```
go.mod
cmd/mybuilds-server/main.go    # 服务端二进制入口：serve / migrate / project / token
cmd/mybuilds/main.go           # 客户端二进制入口：trigger / build / approvals / run / doctor

internal/cli/server/root.go    # 服务端子命令（cobra：serve/migrate/project/token）
internal/cli/client/root.go    # 客户端子命令（cobra：trigger/build/approvals/run/doctor）
internal/config/pipeline.go     # YAML 结构体、严格校验、按字段插值，run 正文保留 shell 变量
internal/config/server.go       # server.yml：listen/dsn/secrets/retention，Viper 覆盖
internal/config/client.go       # client.yml：server 地址 + token
internal/version/version.go     # 版本信息（ldflags 注入，两端共用）
internal/pipeline/engine.go     # 顺序执行、恢复位置、ctx 取消、进程组 kill、统一收尾
internal/pipeline/run.go        # shell 执行与进程取消
internal/pipeline/artifact.go   # 产物收集与路径校验
internal/pipeline/approval.go   # 审批挂起与继续
internal/pipeline/upload.go     # 分发与未知结果处理
internal/pipeline/mask.go       # 日志 writer，把密钥值替换成 ***

internal/store/store.go         # GORM 模型 + 仓库层，AutoMigrate，sqlite/postgres 双驱动注入
internal/store/models.go        # Project / Build / Step / Approval / Event / Token，含上传操作记录
internal/scm/git.go             # clone / fetch / ls-remote / sha / 工作区准备
internal/scm/hook.go            # webhooks/v6（GitHub/GitLab）+ Gitee/自建解析与事件去重

internal/server/server.go       # 服务生命周期、组件组装与优雅关闭
internal/server/http.go         # net/http 路由、鉴权、JSON API、日志 SSE 与产物下载
internal/server/scheduler.go    # 定时/轮询 + 并发槽 + 同项目串行 + 审批扫描
internal/server/recovery.go     # 启动恢复与中断状态处理
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
GET    /api/doctor                         # 构建机体检（admin）
POST   /api/projects/{p}/builds            # 手动触发
GET    /api/builds?project=&status=&limit= # 构建列表（分页）
GET    /api/builds/{id}                    # 单个构建详情 + 步骤状态
GET    /api/builds/{id}/log?step=&follow=1 # 日志（follow 走 SSE tail）
POST   /api/builds/{id}/cancel             # 取消（kill 进程组）
POST   /api/builds/{id}/retry              # 原 SHA / 配置 / 参数，新构建号，检查未知上传结果
POST   /api/builds/{id}/upload-resolution  # 确认未知上传结果并记录证据（admin）
GET    /api/approvals                      # 待审批列表
POST   /api/builds/{id}/approve            # 需要 approver/admin 角色
POST   /api/builds/{id}/reject             # 需要 approver/admin 角色
GET    /api/artifacts/{id}/{file}          # 产物下载
```

## CLI 面

### 服务端 `mybuilds-server`（运维 / 管理员用）

```bash
mybuilds-server serve   --config ~/.mybuilds/server.yml   # 起服务（API + 调度 + 引擎）
mybuilds-server migrate --config ...                     # 建表 / 升级 schema（sqlite 或 postgres）
mybuilds-server project add app-android \
    --repo git@gitlab.example.com:team/app.git \
    --provider gitlab --branches main,release/* \
    --hook          # 打印 webhook URL 与 secret
mybuilds-server project add app-ios --repo ... --poll 60s --schedule "0 9 * * *"
mybuilds-server project ls|rm
mybuilds-server token create --role approver             # 生成 approver / trigger / admin token
mybuilds-server token ls|revoke
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
mybuilds doctor [--server]                # 默认体检本机；--server 体检构建机（需 admin）
mybuilds version
```

列表与详情命令支持 `--json`；`trigger --branch` 在触发时解析并固定最新 SHA，`retry` 始终重跑原提交。
`doctor --server` 返回构建机检查结果，不返回密钥；`resolve-upload` 记录人工确认依据，已发布则不再上传，未发送才允许重试。
`build show` 展示配置快照、工具版本、审批记录和上传结果；产物记录包含大小和 SHA-256，下载与收集校验路径边界及符号链接。

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

- [ ] `internal/scm/git.go`：按 SHA 准备服务端工作区；本地 run 使用当前工作树，不改写用户仓库
- [ ] `run` / `artifact`：顺序执行、失败即停、统一收尾，产物大小与 SHA-256 记录
- [ ] 服务端工作区 `~/.mybuilds/builds/<project>/<number>/{src,logs,artifacts}`；本地结果写临时目录，日志按步骤分文件
- [ ] ctx 取消、进程组 TERM/KILL 与回收；按平台隔离实现，避免影响远程客户端编译
- [ ] 假项目验证成功、失败、取消均执行收尾，日志与 dry-run 脱敏
- [ ] Android 模板：Gradle 版本参数、keystore 注入、apk / aab / mapping 收集，复用依赖缓存
- [ ] iOS 模板：archive/export、按当前 Xcode 支持值生成 ExportOptions、独立 DerivedData 和临时 keychain、ipa / dSYM 收集与清理
- [ ] 基础 `doctor`：git 凭据、JDK / Android SDK、Xcode、签名证书及描述文件检查
- [ ] 一个真实 Android 与一个真实 iOS 工程构建成功，核对版本、产物、签名和取消后的残留进程

### P2 服务端 + 调度

- [ ] 加入 Viper、GORM 与双驱动；验证纯 Go SQLite 驱动兼容性，设置 WAL / 外键 / 锁等待
- [ ] 模型保存构建配置快照、参数、SHA、当前步骤、工具版本与产物；项目构建号事务分配
- [ ] SQLite 与 PostgreSQL 跑相同的 CRUD / 事务 / 条件状态更新 / 唯一约束 / 分页用例
- [ ] token 摘要存储、一次性管理员初始化、创建 / 撤销 / 身份审计，统一读写权限检查
- [ ] 数据库持久化队列、默认并发 1、同项目串行，状态变更使用条件更新；一期单服务进程执行
- [ ] `net/http` JSON API、鉴权、日志 SSE tail、受鉴权的产物下载与服务器 doctor
- [ ] `mybuilds-server serve` + 客户端 `trigger` + `build ls/show` + `logs -f` 闭环
- [ ] 列表 / 详情支持 `--json`；记录排队、步骤耗时、峰值内存与磁盘占用

### P3 重启恢复与审批

- [ ] approval 保存进度并释放全局槽；5s 扫描已批准任务重新入队，保持同项目串行
- [ ] `approve` / `reject` API + CLI：角色、身份、时间、意见、重复决定与取消竞态检查
- [ ] 启动时恢复排队与待审批任务；正在执行的任务标为 interrupted，不自动重跑 shell
- [ ] 重试固定原 SHA / 配置 / 参数并分配新构建号，不受分支后续提交影响
- [ ] 飞书使用 `oapi-sdk-go/v3`，先验证自定义机器人 Webhook 的消息、签名与错误处理；企微 / 钉钉 / generic 用 stdlib HTTP
- [ ] 最终通知覆盖成功 / 失败 / 取消；审批挂起不执行终态通知或删除恢复所需文件
- [ ] 重启恢复、审批释放槽、重复批准、工作区缺失、本地交互审批验证

### P4 自动触发：Webhook + 轮询

- [ ] 加入 webhooks/v6，接 GitHub / GitLab；Gitee 与自建 Git 用轻量解析器
- [ ] 按已注册 provider 校验原始 body、签名 / token、事件与分支，限制请求大小
- [ ] 安全用例：HMAC 篡改 body、错误 token、provider 伪装均拒绝
- [ ] delivery ID 去重，事件与任务事务入库后响应；不同渠道与手动重试允许同 SHA
- [ ] 每项目 ticker + `git ls-remote`，游标与任务入队事务提交；cron 实现定时构建
- [ ] 分支 glob 过滤，忽略 tag / PR / 分支删除事件

### P5 产物与分发

- [ ] `upload`：蒲公英 / fir.im / generic multipart，上传前保存操作记录，校验产物匹配唯一性
- [ ] 上传结果未知时查询远端或由 admin 用 `resolve-upload` 确认并记录依据，阻止自动重发及未确认的发布重试
- [ ] 配置快照与上传产物绑定，审批后发布原产物，不重新构建
- [ ] retention 清理终态构建（保留 N 个 / N 天），保护待审批与未知结果任务
- [ ] 蒲公英真实上传闭环，验证失败与中断后的状态

### P6 部署与打磨

- [ ] `launchd` plist 模板（开机自启，日志走 `~/.mybuilds/serve.log`）
- [ ] 优雅关闭：停止接新任务，取消构建进程并保存状态；专用用户和可信分支部署说明
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
- 审批与发布：重复批准 / 取消不能覆盖终态；重启后从正确步骤继续；未知上传结果不能自动重发

**端到端（本地，无需真机）**

```bash
mybuilds run                       # fake 项目验证 run/artifact；本地 approval 交互确认
mybuilds run --dry-run             # 脱敏输出，不执行任何外部动作
mybuilds-server serve --config ~/.mybuilds/server.yml &   # 起服务端
mybuilds trigger demo --branch main --version 1.0.0 --channel 内测
# 四类 Webhook 使用对应 testdata 与请求头签名 / token 发送，不能用 URL token 代替 GitHub HMAC
mybuilds logs <id> -f               # 看到流式日志
mybuilds approvals                  # 卡在 approval 步骤
mybuilds approve <id> && mybuilds build show <id>   # 继续并成功
# 构建失败 / 取消 → 收尾通知与 keychain 清理仍执行，进程无残留
# 一个任务等审批 → 其他项目仍可构建，同项目保持串行
# kill 服务端 → 重启 → 恢复排队 / 待审批；普通执行中任务为 interrupted
# 分支更新后 retry → 原 SHA / 配置 / 参数、新构建号；缺失工作区不能静默重跑
# 模拟远端收到上传但本地未写成功 → 结果未知且不会自动重发
# 数据保留清理 → 不删除待审批和未知结果任务
# 独立 PostgreSQL 测试库运行同一套用例，不假设切配置会迁移 SQLite 数据
```

**真实构建验证**

1. 一个真 Android 工程：出 apk + aab + mapping.txt，versionCode 等于构建号，上传到蒲公英成功
2. 一个真 iOS 工程：archive → export → ipa + dSYM，签名有效（`codesign -dv` 校验）
3. GitLab / GitHub / Gitee / 一个自建 Git 各连一次，push 触发成功；把某个 hook 的 secret 改错，确认被拒
4. 轮询模式：手动在仓库推一次 commit，确认在间隔内被探测并触发
5. 飞书官方 SDK：真实 Webhook 发消息成功，错误凭据 / 签名能报告失败，日志不泄露密钥
6. Android / iOS 取消与失败后无本次构建残留进程、临时 keychain；复用缓存且不共享构建工作区

## 明确不做（一期）

- 不做 Web UI（纯 CLI）
- 不做插件系统 / 自定义步骤 DSL
- 不做 Docker 隔离、不做分布式远程执行器
- 不做 RBAC / 多租户（token 角色只三档）
- 不做 App Store / Google Play 内置上传
- 不做并行步骤块、矩阵构建

## 已确定的部署与范围

1. 服务端（`mybuilds-server`）部署在这台 macOS 上（Android 与 iOS 同机构建），客户端可跑在任意装了 git 的机器上。
2. 数据库默认 SQLite，支持 PostgreSQL；初期用 GORM `AutoMigrate` 建表，需要改名或回填时再增加显式版本迁移。
3. 审批通知渠道按 飞书（oapi-sdk-go）/ 企业微信 / 钉钉 / 通用 webhook 实现，无内置邮件与短信；企微与钉钉机器人走 stdlib HTTP POST。
4. 构建号用「每项目自增整数」，不做语义化版本自动推导；应用版本由 YAML 参数或触发参数提供，一期忽略 tag 事件。
