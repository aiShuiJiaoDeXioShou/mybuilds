# 触发链路、目录与 CLI / HTTP 接口

006 已实现命令范围与最小示例见 [README](../../README.md#控制端与远程排队)；本文件仍描述完整目标接口，Agent、恢复、方案、发布、审批和 Webhook 需对应功能验收后使用。

本文件保留原 [PLAN.md](PLAN.md) 的完整专题章节；业务功能按 [实施路线](SPECKIT_ROADMAP.md) 推进，当前状态见 [实施历史](../IMPLEMENTATION_HISTORY.md)。

## 触发链路

| 来源 | 识别方式 | 校验 |
|---|---|---|
| GitLab | `X-Gitlab-Event: Push Hook` | `X-Gitlab-Token` 明文比对（constant-time） |
| GitHub | `X-GitHub-Event: push` | `X-Hub-Signature-256` HMAC-SHA256(body) |
| Gitee | `X-Gitee-Event` | `X-Gitee-Token` 明文比对 |
| 自建 Git | 项目注册时指定 provider | 请求头 token 或 HMAC；payload 用**可配置 JSON pointer** 取 ref/after |

统一入口 `POST /hook/{project}`，按项目注册的 provider 校验，不凭请求头切换解析器。
先校验原始请求体和事件，再在事务中持久化事件与待触发请求，成功后响应；窗口关闭后校验并创建执行任务，限制请求体大小，token 不放 URL。

事件去重使用 provider 的 delivery ID（例如 GitHub `X-GitHub-Delivery`）；没有事件 ID 的来源，
按项目、build 名称、分支、SHA、构建参数合并已有排队、运行中、待审批或成功的自动构建。
手动重跑及不同版本 / 渠道参数的构建允许同一 SHA，不能仅按 SHA 永久去重。
参考 [GitHub Webhook 重投与 delivery ID](https://docs.github.com/en/webhooks/using-webhooks/best-practices-for-using-webhooks)。
v1 只处理 push 事件，tag/PR 事件忽略。分支用 `--branches` 的 glob 过滤。

轮询用 `git ls-remote <url> <branch>` 拿 SHA 和上次比对 —— 不需要本地克隆就能探测变更，
轮询游标与待触发请求在同一事务提交，避免游标已推进但触发丢失；窗口关闭后原子入队。

### MVP 触发等待窗口 quiet_period

项目管理设置支持 triggers.builds（自动触发的命名 build 列表）、quiet_period（非负 duration，默认 0s）、allow_upload（布尔，默认 false）：

```yaml
triggers:
  builds: [android, ios]
  quiet_period: 30s
  allow_upload: false
```

多 build 启用 Webhook 时必须显式选择 triggers.builds；单 build 可推导唯一名称，自动发布仍须管理员显式允许。
triggers 与 retention 一样由管理员导入，不由仓库自行开启；allow_upload 不替代节点、商店或 approval 的授权校验。
窗口按项目、来源类型、分支、所选 build 集合与触发请求参数分组；第一条事件建立持久化截止时间，窗口内更新候选提交，不无限延长窗口。
关闭时核对授权分支 HEAD 并固定 SHA，不能因事件乱序选回旧提交；最终参数按该 SHA 的流水线解析后校验。
原始事件全部保留去重信息，被合并事件标明关系；窗口关闭后只读解析最终 SHA 的配置、计算 changes/when，再原子生成所选执行结果。
不覆盖已经入队或运行的任务，也不取消已有上传；手动 trigger/retry 不等待窗口，不改变现有固定 SHA 的任务。
控制端重启恢复窗口与截止时间，关闭与入队使用条件更新防止重复创建；窗口内配置/权限在真正入队时重新校验。
MVP 用 Webhook 验证连续 push 合并、分支/参数隔离、固定截止时间、重启恢复、路径筛选与发布授权。
轮询与 cron 在 016 接入时复用同一入口及等待规则，不是本次新增 MVP 的前提。

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
internal/server/scheduler.go    # 定时/轮询 + 并发槽 + 同项目同名 build 串行 + 审批扫描
internal/server/recovery.go     # 持久化租约核对、启动恢复与中断状态处理
internal/agent/agent.go         # 领取/续租、节点执行与断网停止
internal/agent/report.go        # 脱敏日志、事件与产物回传
internal/protocol/messages.go   # 双端共用的任务、租约与回报格式
internal/notify/notify.go       # 飞书(oapi-sdk-go) / 企微 / 钉钉 / 通用 webhook

internal/mobile/android.go      # gradle 辅助：版本、keystore 注入、JDK 与 SDK 检查
internal/mobile/ios.go          # archive/export、exportOptions、独立 keychain 与 DerivedData
internal/mobile/flutter.go      # Flutter doctor、Android/iOS 构建模板与版本参数
internal/distribute/fastlane.go  # Google Play / App Store 的受控第三方调用
internal/distribute/custom.go    # 自定义命令与结果回执，复用发布授权和状态记录

examples/android.mybuilds.yml
examples/ios.mybuilds.yml
examples/flutter-android.mybuilds.yml
examples/flutter-ios.mybuilds.yml
```

文件按实际实现需要拆分，不把文件数当作架构目标；平台相关进程控制另用构建约束文件。

<a id="http-api"></a>

## 服务端 HTTP API（`mybuilds-server`）

```
POST   /hook/{project}                     # webhook（外部，token/HMAC 校验）
GET    /api/projects                       # 项目列表（admin）
POST   /api/projects                       # 注册项目，group 缺省为 default（admin）
PATCH  /api/projects/{p}                   # 更新归属 group（admin）
GET    /api/groups                         # 组列表（admin）
POST   /api/groups                         # 创建组（admin）
PATCH  /api/groups/{id}                    # 修改组名（admin）
DELETE /api/groups/{id}                    # 删除空组，default 不可删（admin）
GET    /api/doctor                         # 控制端体检（admin）
GET    /api/nodes                          # 节点健康、标签与容量（admin）
GET    /api/nodes/{id}/doctor              # 节点工具链体检（admin）
POST   /api/projects/{p}/builds            # 手动触发，build_names 或 all，固定一个 SHA，返回执行 ID 列表
GET    /api/builds?project=&build_name=&status=&limit= # 构建列表（分页）
GET    /api/builds/{id}                    # 单个构建详情 + 步骤状态
GET    /api/builds/{id}/log?step=&follow=1 # 日志（follow 走 SSE tail）
POST   /api/builds/{id}/cancel             # 保存取消意图，节点终止本次进程组并确认
POST   /api/builds/{id}/retry              # 原 SHA / 配置 / 参数，新构建号，检查未知上传结果
POST   /api/builds/{id}/stop-confirmation   # 原节点停止证据确认（admin），不解除上传 unknown
POST   /api/builds/{id}/upload-resolution  # 确认未知上传结果并记录证据（admin）
GET    /api/approvals                      # 待审批列表
POST   /api/builds/{id}/approve            # 需要 approver/admin 角色
POST   /api/builds/{id}/reject             # 需要 approver/admin 角色
GET    /api/artifacts/{id}/{file}          # 产物下载
```

项目列表支持 group 查询参数，构建列表也支持按项目当前归属的 group 过滤；返回项目时包含组 ID 与名称。
触发 API 中 build_names 与 all 互斥，多 build 不得隐式全部触发；批量校验、权限和入队为一个原子操作。
响应包含 batch_id 与每个 build_name、执行 ID、构建号；查询可按 batch_id 过滤，执行详情和产物均标明 build 名称。
客户端和控制端本机管理命令复用同一组管理逻辑与事务校验，不维护两套归属信息。

### Agent HTTP 接口（待 007 契约细化）

节点登录身份从独立 token 推导；领取、续租、步骤事件、日志、产物与发布意图均通过专用节点路由。
每个执行请求绑定 build/attempt/lease 和节点身份，校验过期执行权、顺序、重复、路径及请求体大小。
复用标准库 HTTP，不允许节点直接访问数据库或管理员 API。

## CLI 面

以下为完整目标接口；已实现子集见 README，最终行为、输出和退出码以对应 Spec Kit feature 的 contracts 与验收记录为准。
`<name>` / `<id>` 表示必填位置参数；列表与详情支持 `--json`，默认输出表格。token 通过环境变量或受限配置文件读取，不提供明文 token 参数。

### 服务端 `mybuilds-server`（运维 / 管理员用）

在控制端主机执行，读取本机配置；全局 `--config` 默认 `~/.mybuilds/server.yml`，`-h/--help` 查看帮助。

| 命令 | 参数与默认值 | 用途 |
|---|---|---|
| `serve` | `--listen`、`--data-dir`、`--concurrency`，未传时取配置值 | 启动 API 与调度，不执行构建 shell |
| `migrate` | 共用 `--config`，数据库参数取文件/环境变量 | 建表或升级 schema；升级前停止服务并备份 |
| `project add <name>` | `--repo` 必填；`--provider` 默认 generic，可选 github/gitlab/gitee/generic；`--branches` 默认 main | 注册可信仓库与允许分支，分支列表支持 glob |
| 同上：分组 | `--group` 默认 default | 指定现有项目组 |
| 同上：节点授权 | `--nodes` 必填，逗号分隔；`--default-node` 可选且须在允许集合内 | 限定项目可用节点；未声明 runner 时使用默认节点 |
| 同上：版本 | `--build-number-start` 默认 1，正整数 | 设置首次分配的构建号，兼容商店既有版本 |
| 同上：项目设置 | `--settings <本地YAML>` 可选 | 导入 pipeline、triggers、retention；直接配置通知 Webhook 随通知功能接入 |
| 同上：流水线路径 | `--file` 默认 mybuilds.yml | 保存仓库根目录相对路径，与 --settings 互斥 |
| 同上：内置方案 | `--framework native/flutter` 与 `--platform android/ios/android,ios` 配套传入 | 登记对应命名 build 与回退方案；不生成仓库文件，与 --settings 互斥 |
| 同上：自动触发 | `--hook` 默认 false，MVP 接入；`--poll` 如 60s 与五段 cron `--schedule` 随 016 接入 | Webhook 使用 triggers 中的 build 范围与等待窗口；轮询/cron 后置 |
| `project set <name>` | `--settings <本地YAML>` 必填 | 更新显式提供的设置块；正在执行的构建使用原快照 |
| `project ls` / `project rm <name>` | ls 支持 `--group` 与 `--json` | 列表/删除；有活动、待审批或未知发布结果时拒绝删除 |
| `project move <name>` | `--group <组名>` 必填 | 将项目迁移至目标组，保留历史与配置 |
| `group create/ls/rename/rm` | 参数与客户端 group 命令一致 | 本机管理项目组 |
| `token create` | `--role` 必填：admin/trigger/approver，随对应功能接入 | MVP 包含审批身份；创建时仅显示一次明文 |
| `token ls` / `token revoke <id>` | ls 支持 `--json` | 查看身份/角色/撤销状态；不显示明文 |
| `node create <name>` | `--labels` 逗号分隔；`--capacity` 默认 1 | 注册节点并仅显示一次独立 Agent token |
| `node ls` | `--json` | 查看平台、能力、标签、容量和心跳 |
| `node drain/enable/disable/rm <name>` | 节点名必填 | 停接新任务/启用/撤销执行授权/删除；有活动或待审批构建不得删除 |
| `version` | 无参数 | 输出二进制版本 |

注册示例（待实现）：

```bash
mybuilds-server serve --config ~/.mybuilds/server.yml --concurrency 2
mybuilds-server node create linux-android-01 --labels android-sdk,flutter --capacity 1
mybuilds-server project add app-android \
    --repo git@gitlab.example.com:team/app.git \
    --provider gitlab --branches 'main,release/*' \
    --nodes linux-android-01 --default-node linux-android-01 \
    --build-number-start 1000
mybuilds-server token create --role admin
```

### Agent `mybuilds-agent`（构建节点，待 007 实现）

全局 `--config` 默认 `~/.mybuilds/agent.yml`；serve 从文件读取连接、容量和租约参数，doctor 检查本机工具链，version 无参数。

```bash
mybuilds-agent serve --config ~/.mybuilds/agent.yml
mybuilds-agent doctor
mybuilds-agent version
```

### 客户端 `mybuilds`（开发者 / 审批人用）

| 全局参数 | 默认值 / 用途 |
|---|---|
| `--config` | `~/.mybuilds/client.yml`，客户端连接配置 |
| `--server-url` | 覆盖配置中的 server；使用不同名称避免与 doctor --server 冲突 |
| `--timeout` | 默认取配置中的 timeout（30s），普通 API 请求超时 |
| `-h/--help` | 查看帮助 |

本地 init/run/doctor 不要求服务端或 token；远程命令才读取和校验连接凭据。

| 命令 | 参数与默认值 | 用途 |
|---|---|---|
| `init` | 无选项生成最小 default shell 配置；`--platform`：android/ios/android,ios；指定平台时 `--framework` 默认 native，显式 framework 必须配 platform | 平台模板始终生成对应名称的 builds；双平台仍只有一个 mybuilds.yml，已有文件拒绝覆盖 |
| 同上：用户模板 | `--template <本地文件>`，不能与 framework/platform 混用 | 校验并生成用户模板 |
| `project init <name>` | `--repo`、`--nodes` 必填；`--group` 默认 default；provider/branches/default-node/build-number-start/settings/file/framework/platform 与服务端 project add 一致 | 注册远程项目、选择分组与命名 build；需要 admin；框架/平台绑定方案不要求仓库有 YAML |
| `project set <name>` | `--settings <本地YAML>` 必填 | 更新显式设置块，可修改或增加回退 build；需要 admin |
| `project ls` | `--group <组名>` 可选；`--json` | 列出项目及当前归属；需要 admin |
| `project move <name>` | `--group <组名>` 必填 | 修改项目归属；需要 admin |
| `group create <name>` | 组名必填 | 创建项目组；需要 admin |
| `group ls` | `--json` | 列出项目组；需要 admin |
| `group rename <name>` | `--name <新名称>` 必填 | 修改普通组名；需要 admin |
| `group rm <name>` | 组名必填 | 删除空组；需要 admin |
| `run` | `--file` 默认 mybuilds.yml；`--build <名称列表>` / `--all` 互斥；`--param key=value` 可重复；`--step <name>` 可选；`--dry-run` 默认 false | 本地执行或脱敏预览，多 build 顺序执行；--step 仅允许选中一个 build，仍校验依赖 |
| `trigger <project>` | `--branch` 默认 main；`--ref <sha>` 可与 branch 同用，须可达于该授权分支；`--build <名称列表>` / `--all` 互斥；`--param key=value` 可重复；`--version`、`--channel` 可选 | 固定一个 SHA，返回所选 build 的独立执行 ID；单 build 可省略选择；ref 须属于授权分支 |
| `build ls` | `--project`、`--group`、`--build-name`、`--batch`、`--status` 可选；`--limit` 默认 20；`--offset` 默认 0；`--json` | 按命名 build 或批次查询；项目与组同时传入时取交集 |
| `build show <id>` | `--json` | 查看 build 名称、批次、节点、步骤、工具版本、产物与发布记录 |
| `build cancel <id>` / `build retry <id>` | admin 权限 | 请求取消/用原 SHA、配置和参数生成新构建号 |
| `build confirm-stopped <id>` | `--note` 必填；admin 权限 | 记录原节点已停机或构建进程已终止的证据，解除停止保护；不替代上传确认 |
| `build resolve-upload <id>` | `--step`、`--result sent/not-sent`、`--note` 均必填；admin 权限 | sent 表示远端已接受；not-sent 需证明确实未被接受/无发布副作用，不推断已上架 |
| `logs <build-id>` | `--step <name>` 可选；`-f/--follow` 默认 false | 历史日志或 SSE 跟随 |
| `artifact ls <build-id>` | `--json` | 列出产物 ID、名称、大小与摘要 |
| `artifact download <artifact-id>` | `--output <文件>` 必填，已存在拒绝覆盖 | 下载并校验 SHA-256 |
| `status` | `--json` | 查询连通性、版本和全局容量 |
| `doctor` | 无参数检查本机；`--server` / `--node <name>` 互斥；`--json` | 控制端/节点检查需要 admin，不返回密钥 |
| `approvals`（MVP） | `--json` | 列出待审批任务 |
| `approve/reject <build-id>`（MVP） | `--note` 可选；approver/admin 权限 | 批准或拒绝并记录身份、时间和意见 |
| `version` | 无参数 | 输出客户端版本 |

使用示例（待实现）：

```bash
mybuilds init --framework flutter --platform android
mybuilds run --file mybuilds.yml --dry-run
mybuilds trigger app-android --branch main --version 1.0.0 --channel 内测
mybuilds build ls --project app-android --limit 20 --json
mybuilds logs 123 --follow
mybuilds artifact ls 123
mybuilds doctor --node linux-android-01
mybuilds project init mobile-app \
  --repo git@gitlab.example.com:team/app.git \
  --nodes linux-android-01,mac-ios-01 \
  --framework flutter --platform android,ios
mybuilds trigger mobile-app --build android --param version=1.2.0 --param flavor=production
mybuilds trigger mobile-app --all
```

项目组示例（待实现）：

```bash
mybuilds group create mobile
mybuilds project init app-android \
  --repo git@gitlab.example.com:team/app.git \
  --nodes linux-android-01 --default-node linux-android-01 --group mobile
mybuilds project ls --group mobile
mybuilds group rename mobile --name apps
mybuilds project move app-android --group default
mybuilds group rm apps
```

列表与详情命令支持 `--json`；`trigger --branch` 未指定 ref 时解析并固定分支 HEAD；指定 ref 时固定该 SHA 并校验它属于所选授权分支，when 使用同一分支上下文；`retry` 始终重跑原提交。
`doctor --server` 返回控制端检查结果，`doctor --node` 返回对应节点工具链状态，两者不返回密钥；`resolve-upload` 记录人工确认依据，远端已接收则不再上传，确认远端未接受且未产生发布副作用才允许重试。
`build show` 展示执行节点、attempt / 租约状态、配置快照、工具版本、审批记录和上传结果；产物记录包含大小和 SHA-256，下载与收集校验路径边界及符号链接。

`doctor` 不是锦上添花 —— iOS 签名失败是移动端 CI 的头号故障，一个能提前告诉你
「keychain 没解锁 / 描述文件过期 / Gradle 用错 JDK」的命令能省掉大量排查时间。
