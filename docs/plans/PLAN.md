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
| 配置 | 默认优先仓库内 `mybuilds.yml`，缺失时使用项目绑定的可复用构建方案 |
| 项目组 | 客户端注册项目时选择归属组，未指定进入 default；支持组改名与项目迁移 |
| 多 build | 一个项目绑定一个仓库，可定义多个命名 build；一次选择一个、多个或全部，分别调度和记录结果 |
| MVP 执行控制 | build/步骤 when、参数约束、build 总超时、post 收尾、日志时间戳、变更筛选、触发等待窗口、项目保留策略和测试报告 |
| 代码源 | GitLab / GitHub / Gitee / 任意自建 Git（通用 git 协议） |
| 触发 | Webhook（含签名校验）+ 轮询，外加手动触发 |
| 审批 | 支持发布审批（流水线中途挂起等人放行） |
| 隔离 | 裸机 shell，不用 Docker |
| 工程类型 | MVP 内置原生 Android / iOS 与 Flutter 模板，支持仓库脚本和用户本地模板 |
| 主要分发渠道 | MVP 支持 Google Play / App Store，统一封装第三方 fastlane；自定义发布共用 upload 契约 |

节点工具链增加 Flutter 与商店发布所需的 Ruby/Bundler/fastlane，按实际任务安装与锁定版本。
原规划记录的本机环境：Go 1.25.4 · Xcode 27.0 · JDK 21 · Android SDK 齐全 · Node 24。
实施前由 `doctor` 重新检测，构建记录保存实际工具版本，不把这份环境记录当成固定要求。

**硬约束**：iOS 打包（`xcodebuild` + 签名）只能在 macOS 构建节点执行。
控制端 `mybuilds-server` 可部署 Linux/macOS，Agent 在具备工具链的节点执行任务：
macOS 节点支持 iOS/Android，Linux 节点支持 Android；客户端的远程命令保持跨平台。
支持一个控制端加一个或多个 Agent，同机部署同样使用 Agent。节点协议、故障边界见 [多节点设计](MULTI_NODE.md)。
当前仅完成双 CLI 初始化，多节点、构建模板和商店分发均尚未实现。
MVP 构建、分发及扩展边界见 [BUILD_DISTRIBUTION.md](BUILD_DISTRIBUTION.md)。

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
| Google Play / App Store 分发 | 第三方 fastlane（supply / deliver） | Go 封装子进程，使用成熟商店能力；节点 Ruby/Bundler 与 Gemfile.lock 锁定版本 |
| 原生 / Flutter 构建与扩展 | 官方工具链 + 本项目可编辑模板 + 仓库命令 | 构建框架、目标平台与分发渠道独立组合，不新增步骤 DSL |

Git 操作走 `git` 命令行（`os/exec`），不用 go-git —— go-git 体积大、行为细节和真 git 有偏差，
而我们本来就要求机器上装了 git。

依赖按实施阶段引入并锁定版本，不在 P0 一次性安装全部依赖。
依据：[Go HTTP 路由](https://pkg.go.dev/net/http#ServeMux)、[YAML 维护项目](https://github.com/yaml/go-yaml)、[飞书官方 SDK](https://github.com/larksuite/oapi-sdk-go)。

CLI 分发形态：

```
cmd/mybuilds-server/  → mybuilds-server   服务端二进制
                 ├─ serve          起 HTTP API + 调度，不执行构建 shell
                 ├─ migrate        建表 / 升级 schema
                 ├─ project add|set|ls|move|rm / group create|ls|rename|rm
                 ├─ token create|ls|revoke
                 └─ node create|ls|drain|enable|disable|rm
cmd/mybuilds/  → mybuilds          客户端二进制（连服务端）
                 ├─ project init|set|ls|move / group create|ls|rename|rm
                 ├─ trigger / build ls|show|cancel|retry|resolve-upload
                 ├─ logs / artifact ls|download
                 ├─ approvals / approve / reject
                 ├─ run           本地调试流水线，不经过服务端
                 └─ doctor / status / init
cmd/mybuilds-agent/ → mybuilds-agent      构建节点二进制（待 007 创建）
                 └─ serve / doctor / version
```

三个角色共享 internal 下的配置、流水线、scm、mobile 和 version，入口随功能创建。
Agent 与客户端本地 run 共用同一套引擎；控制端通过持久化队列和 Agent 协议管理执行。
本地模式结果写临时目录，不连接控制端数据库；平台进程控制放到对应构建约束文件。
MVP 本地 run 只执行构建、产物和交互审批；有生效的 upload 步骤时在执行任何命令前拒绝，dry-run 可预览全部步骤。
实际发布统一经控制端和 Agent，即使二者在同一台机器；共用引擎按执行上下文检查发布权限，不新增本地发布数据库。
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

引擎到审批节点时完成以下检查点后保存 `waiting_approval` 并退出本次执行，释放节点与全局构建槽；调度器每 5s 查询数据库，
已批准的任务进入固定原节点的恢复队列，从审批后的步骤继续。同项目同名 build 在审批期间仍保持串行，其他 build 可以使用空出的槽。
挂起前节点确认本次步骤进程已退出、清理临时签名资源，并确认日志、产物及已有测试报告已回传控制端；发布审批还必须满足完整报告检查并封存证据。
控制端事务提交审批检查点后才释放租约和容量；保留工作区、产物/报告摘要与节点归属。
批准后从原节点以新租约恢复，节点离线时等待并提示；挂起不视为成功，不发送最终通知。
审批仅允许 `approver/admin`，一期不做审批分组；记录 token 身份、时间、意见，重复或相互冲突的决定不得覆盖。

构建保存 commit SHA、流水线配置快照、当前步骤、工作区、产物记录与工具版本；快照保留密钥引用，不存密钥明文。
控制端启动时恢复排队与待审批任务，并核对持久化节点租约；不能将控制端重启当作节点中断。
节点崩溃或租约过期时普通执行步骤标为 `interrupted`，由用户显式重试，不自动迁移重跑。
无法确认进程停止时另存 stop_unconfirmed 保护标志，保留同项目同名 build 互斥、隔离原节点并禁止清理；租约过期不等于物理进程已停止。
原节点经独立身份回报进程组已回收，或管理员记录已停机/终止进程的证据后，才能解除停止保护；上传 unknown 另行处理。
工作区或产物缺失时终止恢复并报告原因，不能从头静默重跑。

上传前由控制端事务保存发布意图、取得应用互斥并授权当前租约；授权后缺少可信终态回执即标为 unknown，不能假设节点尚未发送。
优先查询远端状态，无法确认时交由人工处理，
禁止自动重发。普通重试生成新的构建号，复用原 SHA、配置和构建参数；未知上传结果确认前不能重试发布。
管理员可查询或人工确认上传结果，记录证据与意见后解除未知状态；确认已发布时不再执行该次上传。

### 5. 版本号是一等公民

移动端最高频需求：构建号自增。内置 `{{build.number}}`、`{{git.sha}}`、`{{git.branch}}`、
`{{version}}` 模板变量，直接喂给 `versionCode` / `CURRENT_PROJECT_VERSION`。

显式记录移动端版本与渠道参数，同一 SHA 可构建不同渠道；一期同一平台、同一应用的渠道放在同一项目内，
避免多个项目独立分配构建号后出现冲突。发布接入时将已核验的 (store, app_identifier) 唯一绑定项目，拒绝其他项目使用独立计数器发布同一应用；
同项目多个 build 共用此绑定和应用上传锁。核对商店既有版本；外部工具占用新版本时明确失败，不覆盖也不自动改号。版本规则按平台校验，不假设 Android 与 iOS 完全一致。

### 6. 工作区、收尾与资源控制

- 控制端按固定 SHA 只读选择仓库定义或构建方案、校验并保存所选 build 快照后入队；
  Agent 检出该 SHA 准备隔离工作区并执行快照，checkout 是前置操作。本地 `run` 使用当前工作树，不重置用户修改。
- 常规步骤失败即停；系统资源清理独立于用户 post，最终通知随通知功能接入，由控制端发送且失败不覆盖构建结果。
  用户 post 仅由持有效执行权的原节点运行；未开始执行的取消、审批挂起后的取消或拒绝不启动用户脚本，记录未执行原因。
- 默认每节点容量为 1、全局并发为 1，均可配置；同项目同名 build 跨节点串行，不同 build 可并行；记录实际资源使用后再提高并发。
- 各节点工作区按构建隔离，复用工具自身的依赖下载缓存；iOS DerivedData 和临时 keychain 按构建隔离，不新增通用缓存系统。
- 取消先向构建进程组发送 TERM，超时后 KILL 并回收子进程；不得杀同用户的全部 Java / Xcode 进程。Gradle 默认 `--no-daemon`，真实构建验证残留进程。

### 7. 配置变量与鉴权

- 配置先解码与校验，再对明确支持的字段插值；仅解析 `env`、凭据等字段里的 `${SECRET}`，缺失时报错。
  `run` 正文的 `$VAR` / `${VAR}` 留给 shell；分支名、版本等值通过环境变量传入，不直接拼接到 shell 命令中。
- 支持既定的 `{{var}}`，但 `run` 正文不做模板替换；未知模板变量报错，`--dry-run` 输出不展示密钥。
- token 存数据库，只保存高熵 token 的摘要和身份、角色、撤销状态；创建时仅显示一次明文，列表不回显。
  首次启动且 token 表为空时，可用 `MYBUILDS_BOOTSTRAP_ADMIN_TOKEN` 初始化管理员，之后撤销不会被配置重新创建。
- 项目管理可由本机管理员 CLI 操作；admin 可访问全部 API，trigger 仅能触发不含 upload 的构建及查询基本服务状态，
  approver 可列待审批任务、读取其详情 / 日志 / 产物并批准或拒绝。取消、重试、上传结果确认与控制端 / 节点 doctor 仅限 admin。

### 8. 项目组与项目归属

- 项目组（project group）用于分类与查询；构建方案（build profile）用于复用流水线。
  一个项目属于一个组，仍独立绑定构建方案、节点授权与通知，组不新增配置继承层。
- 控制端初始化数据库时创建 default 组；注册项目未传 --group 时进入该组，指定不存在的组则报错。
  default 为固定兜底组，不能改名或删除；普通组支持创建、列表、改名和删除，名称唯一，空名称拒绝。
- 客户端 project init 注册远程项目并选择组，使用 admin token；项目组与远程项目管理仅允许 admin。
  原有 mybuilds init 继续生成本地流水线，不连接控制端；归属保存在数据库，不由仓库 mybuilds.yml 修改。
- 数据模型采用 project_groups(id, name) 与 projects.group_id（非空外键）；组 ID 与项目 ID 不因改名或改组改变。
  项目名称保持全局唯一，trigger、Webhook 路径和构建号使用原项目身份，不增加 group/project 命名空间。
- group rename 修改组名；project move 修改项目归属，可移入其他组或移回 default。
  迁移只在事务中更新归属和操作记录（身份、时间、原组/目标组），保留构建历史、产物、工作区、配置、凭据与编号，
  允许构建期间改组，运行任务继续使用原配置快照。迁移到当前组为无操作成功，目标不存在时不修改。
- 分组列表按项目当前归属查询，迁移后历史构建随项目可查；组删除仅允许空组，数据库外键阻止并发写入下的误删。
  双数据库验证默认归属、改名、迁移、历史保留和删除竞态。

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
一次流水线固定一个节点；增加节点提高不同项目或同项目不同 build 的并发，不做跨节点分步执行或多控制端高可用。

### 模块边界与持久化规则

- CLI 只解析输入、调用业务入口并展示结果；server 管理鉴权、配置快照、队列与持久化，agent 管理领取、续租和宿主资源。
  pipeline 执行已解析的定义，不直接访问数据库或 HTTP；protocol 只定义传输数据。store 不依赖 CLI/执行器，mobile/distribute 不依赖 server。
  本地上下文与 Agent 上下文提供所需执行能力；遇到实际替换或测试隔离需求再提取接口，不预建注册器、事件总线或插件框架。
- 单控制端约束必须在启动时执行：SQLite 用规范化数据库路径旁的进程文件锁，PostgreSQL 用专用连接持有 session advisory lock；
  同一数据库拒绝第二个调度进程，锁丢失立即停止分配和发布授权。PostgreSQL 的锁生命周期见 [官方说明](https://www.postgresql.org/docs/current/explicit-locking.html#ADVISORY-LOCKS)。
  该锁用于防止误启动，不提供自动主备切换；日志和产物目录仍由唯一控制端持有。
- 手动 trigger/retry 请求携带客户端生成的 Idempotency-Key；以身份与 key 唯一约束保存请求摘要和结果 ID。
  鉴权后先按原请求内容查询 key，再解析 HEAD/快照；已完成的同 key/内容返回原结果，不重新分配构建号。
  不同内容复用 key 返回冲突；入队事务再次检查 key，应对并发重试。网络超时重试沿用原 key，新的用户操作使用新 key。
  参数校验在入队前完成；结果、计数器与幂等记录在同一短事务提交，不在事务内执行 Git、shell 或商店请求。
  Webhook 使用来源事件 ID 独立去重；Agent 回报用 attempt/lease_epoch/事件序号去重，三者不互相替代。
- 数据库与文件系统不共享事务：产物/报告先写受限临时路径，校验后原子改名，再保存可见记录；未完成文件不可下载。
  启动恢复核对未完成记录和无引用文件，后者延迟清理，不能把部分文件当作完整产物。审批、上传只引用已确认的产物 ID 与摘要；实时日志按已确认偏移读取，并以事件序号去重。

### 执行状态与保护条件

| 执行状态 | 进入条件与下一步 |
|---|---|
| skipped | build.when 不满足；无构建号、无节点，保留原因 |
| queued | 初始执行等待合格节点；审批批准后的恢复还必须固定原节点 |
| running | 事务领取并取得有效租约；只接受当前节点/attempt 的回报 |
| waiting_approval | 进程退出、资源清理和产物/报告回传已确认；保留 build 互斥，不占执行槽 |
| succeeded / failed | 普通执行、报告检查和可执行 post 已结束，系统清理记录已保存 |
| cancelled | 排队取消，或正在执行的停止已确认，或挂起审批被取消/拒绝；拒绝记录 approval_rejected 原因 |
| interrupted | 执行权失效且不能继续；不得自动迁移或重复 shell |

cancel_requested 是操作意图，stop_unconfirmed 是停止保护；上传结果是独立记录，不用一个执行状态代替三者。
终态不得被旧回报复活；interrupted 且停止未确认、或存在上传 unknown 的执行仍受互斥/保留保护。
运行中取消由 Agent 停止进程并在有效租约内尝试 always；排队/挂起取消由控制端条件更新，不创建恢复租约执行 post。
审批恢复再次检查原节点授权、工具、容量、工作区和产物摘要；领取时允许任务持有自己的 build 互斥，不绕过其他任务的互斥。
重试须先解除停止保护；涉及发布还须解除应用 unknown 保护。人工确认仅记录既有事实，不会重新执行原任务。

## 配置文件

### 服务端配置 `~/.mybuilds/server.yml`（由 `mybuilds-server serve` 读取）

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
构建方案是复用配置，不引入多项目批量构建、方案嵌套继承或另一套执行器。详情见 BUILD_DISTRIBUTION。

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
建议默认时序见 MULTI_NODE，协议细节与参数合法性在对应 feature 中验证；跨主机 HTTPS 必须验证证书。

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
不加载动态 Go 插件、不建插件市场或另造 DSL，字段和结果契约见 BUILD_DISTRIBUTION。

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
--version 与 --channel 是相应 --param 的快捷形式，同次传入同名 --param 时拒绝，不按参数顺序决定优先级。
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

以下为拟定接口，除双端帮助与 version 外均待实现；最终行为、输出和退出码在对应 Spec Kit feature 的 contracts 中确认。
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

## 实施步骤

### P0 骨架与流水线预览

- [x] `000-project-bootstrap`：Go 单模块、Cobra、双 CLI 帮助与共享 version；README 与 AI 阅读入口
- [ ] `001-pipeline-preview`：复用已有入口，引入 YAML，增加 `init` 和 `run --dry-run`；其他依赖随对应阶段加入并锁定版本
- [ ] `internal/config/pipeline.go`：单流水线 default 与多 build 格式、选择规则、参数默认值/覆盖与按字段插值；保留 run 正文中的 shell 变量
- [ ] when 三类条件、字符串/对象参数约束、build timeout、post 与 reports 的严格结构校验；预览条件、跳过原因或待确定状态
- [ ] `internal/pipeline/mask.go` + 单测
- [ ] `mybuilds run --dry-run` 打印脱敏计划，不触发命令或外部动作

### P1 本地执行与原生 / Flutter 构建

- [ ] `internal/scm/git.go`：按 SHA 准备节点工作区；本地 run 使用当前工作树，不改写用户仓库
- [ ] `run` / `artifact`：顺序执行、失败即停、统一收尾，产物大小与 SHA-256 记录
- [ ] run 的 sh/bash、working_dir、step.env、timeout 与脚本参数；独立 shell、不继承 cd/export、受限环境与上下文变量
- [ ] 参数/分支 when 与步骤 skipped、累计 build 超时、独立 post 预算和系统收尾、默认时间戳日志
- [ ] 节点工作区 `<agent.data_dir>/builds/<project>/<number>/{src,logs,artifacts}`；本地结果写临时目录，日志按步骤分文件
- [ ] ctx 取消、进程组 TERM/KILL 与回收；按平台隔离实现，避免影响远程客户端编译
- [ ] 本地预检查拒绝本次选中且 when 生效的 upload；模板默认只构建/收集产物；运行中成功、失败、取消验证收尾，日志与 dry-run 脱敏
- [ ] Android 模板：Gradle 版本参数、keystore 注入、apk / aab / mapping 收集，复用依赖缓存
- [ ] iOS 模板：archive/export、按当前 Xcode 支持值生成 ExportOptions、独立 DerivedData 和临时 keychain、ipa / dSYM 收集与清理
- [ ] 基础 `doctor`：git 凭据、JDK / Android SDK、Xcode、签名证书及描述文件检查
- [ ] Flutter Android / iOS 模板与 doctor：复用底层签名/收尾，flutter build appbundle / ipa，支持版本、flavor 和用户参数
- [ ] 内置模板可编辑，自定义本地模板和仓库脚本构建；不新增执行器或新步骤 DSL
- [ ] 原生 Android / iOS 与 Flutter 双平台真实构建成功，核对版本、产物、签名和取消后的残留进程

### P2 控制端 + 多节点调度

- [ ] 加入 Viper、GORM 与双驱动；验证纯 Go SQLite 驱动兼容性，设置 WAL / 外键 / 锁等待；启动取得唯一控制端锁
- [ ] 模型保存 build_name、batch_id、构建配置快照、参数、SHA、当前步骤、工具版本与产物；项目构建号事务分配
- [ ] 项目组模型、default 初始化、客户端 project init 选组、组改名/空组删除与项目事务迁移；构建历史保留，列表按当前归属过滤
- [ ] 项目命名 build 设置、--file 与 --settings 路径区分、branch/ref 可达校验、批量固定 SHA/参数与权限校验、请求幂等/原子入队、批次关联查询
- [ ] build 级 when 在分配节点前判断，skipped 不占节点/构建号；记录条件事实、剩余预算和收尾进度，恢复不重置
- [ ] 控制端具名构建方案、项目 auto/repo/profile 来源选择；仅缺文件回退，错误配置拒绝，执行快照与重试不受方案编辑影响
- [ ] SQLite 与 PostgreSQL 跑相同的 CRUD / 事务 / 条件状态更新 / 唯一约束 / 分页用例
- [ ] token 摘要存储、一次性管理员初始化、创建 / 撤销 / 身份审计，统一读写权限检查
- [ ] 数据库持久化队列、默认并发 1、同项目同名 build 串行，状态变更使用条件更新；单控制端分配给多个 Agent 执行
- [ ] `net/http` JSON API、鉴权、日志 SSE tail、受鉴权的产物下载与控制端/节点 doctor
- [ ] 节点注册/独立 token/标签/容量/drain，Agent 跨主机 HTTPS 主动领取与续租
- [ ] 原子节点分配、attempt/租约归属、过期回报拒绝、断网停止，无匹配节点时保持排队
- [ ] 节点脱敏日志与校验产物回传；临时文件校验/原子改名/可见记录与重启恢复，不共享数据库或工作区
- [ ] `mybuilds-server serve` + 多个 `mybuilds-agent serve` + `trigger` + `build ls/show` + `logs -f` 闭环
- [ ] 列表 / 详情支持 `--json`；记录排队、步骤耗时、峰值内存与磁盘占用

### P3 重启恢复与审批

- [ ] approval 确认进程停止与资源清理、产物/报告封存后保存检查点并释放槽；已批准任务固定原节点，复核授权/容量/证据并保留自身 build 互斥
- [ ] `approve` / `reject` API + CLI：角色、身份、时间、意见、重复决定与取消竞态检查
- [ ] 启动时恢复排队与待审批任务并核对租约；过期记 interrupted，停止未确认则保留互斥/隔离原节点/禁止清理；停止证据与上传结果分别确认
- [ ] 重试固定原 SHA / 配置 / 参数并分配新构建号，不受分支后续提交影响
- [ ] 飞书使用 `oapi-sdk-go/v3`，先验证自定义机器人 Webhook 的消息、签名与错误处理；企微 / 钉钉 / generic 用 stdlib HTTP
- [ ] 最终通知覆盖成功 / 失败 / 取消；审批挂起不执行终态通知或删除恢复所需文件
- [ ] 项目直接配置 Webhook 与 defaults.notifications；按字段继承、列表替换、显式关闭；敏感值受限保存/脱敏、URL 校验、策略快照和失败不改投全局
- [ ] 重启租约核对、停止未确认保护、审批释放槽、原节点恢复、重复批准、挂起取消/拒绝、工作区缺失、本地交互审批验证

### P4 自动触发：Webhook + 轮询

- [ ] 加入 webhooks/v6，接 GitHub / GitLab；Gitee 与自建 Git 用轻量解析器
- [ ] 按已注册 provider 校验原始 body、签名 / token、事件与分支，限制请求大小
- [ ] 安全用例：HMAC 篡改 body、错误 token、provider 伪装均拒绝
- [ ] delivery ID 去重，事件与待触发请求事务入库后响应；窗口关闭与任务创建原子提交，不同渠道与手动重试允许同 SHA
- [ ] 后续 016 的每项目 ticker + `git ls-remote`、游标与触发请求事务提交；cron 复用入口
- [ ] 分支 glob 过滤，忽略 tag / PR / 分支删除事件
- [ ] MVP Webhook 的 changes 路径筛选与 quiet_period，固定窗口、不同参数/分支隔离、持久化恢复与重复关闭拒绝

### P5 商店分发（MVP）与后续产物运维

- [ ] 019 报告检查先完成再接入发布；核对应用唯一项目绑定、版本、AAB/IPA、封存产物/报告、节点/attempt/租约，事务取得应用锁并保存授权意图
- [ ] Google Play 使用 fastlane supply，service account 认证，支持 internal 与显式 production
- [ ] App Store 使用 fastlane deliver 与 API key，支持上传 IPA、显式提交审核及选择审核后自动发布；不把上传成功等同公开上架
- [ ] custom 上传接用户命令/Fastfile，共用输入、结果文件、租约、取消、脱敏与未知结果确认
- [ ] 授权后无可信回执默认 unknown 并保持应用锁；查询远端或由 admin 用 `resolve-upload` 确认依据，不自动重发，不能用停止确认代替
- [ ] 配置快照与上传产物绑定，审批后发布原产物，不重新构建
- [ ] retention 清理终态构建（保留 N 个 / N 天），保护活动、待审批、停止未确认与未知上传任务，保留项目计数器/必要审计
- [ ] MVP 项目 retention 按字段继承全局；中央删除保护已有下载，Agent 独立管理指令校验工作区/进程并幂等确认，离线保留待清理
- [ ] MVP JUnit 报告收集与 CLI/JSON 摘要、原始 XML 下载；本地旧报告不放行，发布前封存证据，post 不改证据，缺失/非法/越界报告可诊断
- [ ] 两大商店真实上传与可查询回执，自定义命令结果校验，验证凭据错误、失败与回报丢失的 unknown 状态
- [ ] 原型验证工具内置重试、非交互认证及版本兼容；Ruby/Bundler/fastlane 按节点锁定
- [ ] fir.im / generic 内置分发不属于 MVP，后续有需求再接入

### P6 部署与打磨

- [ ] macOS `launchd` 与 Linux `systemd` 模板；控制端/Agent 分别自动启动，运行目录独立
- [ ] 优雅关闭：控制端停止分配，Agent 停止领取并取消自身构建进程，按租约保存状态；专用用户和可信分支部署说明
- [ ] `build ls` 过滤与分页、日志流重连、doctor 输出打磨
- [ ] 基于真实构建数据调整并发；按实际瓶颈优化依赖缓存，不新增通用缓存系统

MVP 包含手动/Webhook 触发、同仓库多命名 build、原生与 Flutter 双平台、多节点执行、日志/产物下载及两大商店分发与 custom 扩展。
新增 when、参数约束、build 总超时、post、日志时间戳、changes 筛选、quiet_period、项目 retention 与 JUnit 报告，发布审批同步进入 MVP。
机器人通知、轮询/cron、其他内置渠道和部署打磨可后置；发布授权、租约、测试失败阻止发布与上传 unknown 处理不能后置。
Google Play/App Store 接入不自写完整市场协议；具体认证、默认发布模式、前置条件及验收见 BUILD_DISTRIBUTION。

## Verification

**单元测试（只测值得测的）**

- `internal/config`：单/多 build 混写拒绝、名称选择与参数校验、未知字段 / 模板变量报错、env 字段插值、run 正文保留 `${VAR}`、引用的密钥缺失时报错、dry-run 无副作用
- `internal/store`：**双驱动**跑同一套事务 / 构建号分配 / 去重 / 条件更新 / 分页用例；token 撤销后不得再次初始化
- `internal/pipeline`：脚本参数与环境隔离、sh/bash、超时、工作目录越界拒绝、独立 shell；成功 / 失败 / 取消均收尾，分段日志中的密钥被脱敏，产物路径及符号链接不能越界
- `internal/scm/hook.go`：**四种来源的签名校验**，含 GitHub HMAC 篡改 body 必须拒绝、错误 token 必须拒绝
- `internal/server`：批量同 SHA/原子入队、统一计数不重复、同项目同名 build 串行及不同 build 并行、审批释放全局槽、重复事件只入队一次、同 SHA 不同参数可触发、轮询游标与入队同时提交
- `internal/agent` / 节点协议：独立身份、双节点竞争、错误平台、租约过期、断网取消、幂等回报、日志与产物回传
- 审批与发布：重复批准 / 取消不能覆盖终态；重启后原节点正确继续；未知上传结果不能自动重发
- MVP 执行控制：when AND/OR、手动路径豁免、删除/重命名/公共目录、首构建与缺失基线、冻结事实重试；审批跳过不放行上传
- MVP 超时/收尾/日志：累计预算与重启恢复、独立 post 预算、失败不覆盖原证据、节点失联不迁移收尾、时间戳与流式脱敏
- MVP 触发/保留/报告：窗口持久化与原子关闭、项目继承与保护竞态、JUnit 汇总不重复、非法 XML/越界/缺失拒绝、测试失败不上传

**端到端（本地，无需真机）**

```bash
mybuilds run                       # fake 项目验证 run/artifact；本地 approval 交互确认
mybuilds run --dry-run             # 脱敏输出，不执行任何外部动作
mybuilds-server serve --config ~/.mybuilds/server.yml &   # 起控制端
# 按节点配置，另行启动至少两个 mybuilds-agent serve 进程
mybuilds trigger demo --branch main --version 1.0.0 --channel 内测
# 四类 Webhook 使用对应 testdata 与请求头签名 / token 发送，不能用 URL token 代替 GitHub HMAC
build_exec_id=123                   # 替换为 trigger 返回的执行 ID
mybuilds logs "$build_exec_id" -f  # 看到流式日志
mybuilds approvals                  # 卡在 approval 步骤
mybuilds approve "$build_exec_id"
mybuilds build show "$build_exec_id"  # 检查恢复后的状态
# 运行中构建失败 / 取消 → 有效执行权内运行 post、系统 keychain 清理，进程无残留；通知随 013 验证
# 一个任务等审批 → 其他 build 仍可构建，同项目同名 build 保持串行
# kill 控制端 → 重启 → 核对节点租约并恢复排队 / 待审批，不误判仍在续租的构建
# 节点断网 / 崩溃 → 租约过期，不迁移；无法确认停止则保留同名互斥、隔离原节点、禁止清理
# 分支更新后 retry → 原 SHA / 配置 / 参数、新构建号；缺失工作区不能静默重跑
# 模拟远端收到上传但本地未写成功 → 结果未知且不会自动重发
# 数据保留清理 → 不删除待审批、停止未确认和未知上传任务；离线节点清理保持待确认
# 独立 PostgreSQL 测试库运行同一套用例，不假设切配置会迁移 SQLite 数据
```

**真实构建验证**

1. 一个真 Android 工程：出 apk + aab + mapping.txt，versionCode 等于构建号，产物可下载并校验摘要
2. 一个真 iOS 工程：archive → export → ipa + dSYM，签名有效（`codesign -dv` 校验）
3. GitLab / GitHub / Gitee / 一个自建 Git 各连一次，push 触发成功；把某个 hook 的 secret 改错，确认被拒
4. 轮询模式：手动在仓库推一次 commit，确认在间隔内被探测并触发
5. 飞书官方 SDK：真实 Webhook 发消息成功，错误凭据 / 签名能报告失败，日志不泄露密钥
6. Android / iOS 取消与失败后无本次构建残留进程、临时 keychain；节点独立缓存且不共享构建工作区
7. 两个真实节点执行不同项目，同项目同名 build 跨节点串行；iOS 不分配给 Linux，无合格节点明确排队
8. 暂停原 Agent 后模拟过期回报，确认拒绝；审批后原节点离线不迁移，发布授权后失联不自动重发
9. Flutter Android/iOS 真实构建，核对版本、flavor、签名与产物；自定义仓库脚本同路径执行
10. Google Play internal 轨道可见真实 AAB；App Store Connect 可见真实 IPA，显式提交审核路径可验证
11. custom 上传按结果文件确认；授权后无可信回执保持 unknown，不重发；上传/审核/公开状态分别记录
12. 同一触发 key 网络重试只产生一批结果与构建号；不同内容复用 key 被拒；误启动第二控制端被拒
13. 失联后同名 build 与原节点保持保护，停止确认和上传确认分别解除；审批挂起取消不启动用户脚本
14. 旧本地 JUnit 文件不算本次通过；报告失败/缺失阻止审批与上传，审批后产物被改写不能发布

## 明确不做（一期）

- 不做 Web UI（纯 CLI）
- 不做插件系统 / 自定义步骤 DSL
- 不做 Docker 隔离；支持多构建节点，不做多控制端高可用或单条流水线跨节点迁移
- 不做 RBAC / 多租户（token 角色只三档）
- 不自动注册商店账号或代填合规元数据；不在 MVP 内置其他分发渠道
- 不做并行步骤块、矩阵构建

## 已确定的部署与范围

1. 一个控制端（Linux/macOS）管理多个 Agent。iOS 仅在 macOS 节点构建，Android 可在 Linux/macOS；客户端远程命令跨平台。
2. 数据库默认 SQLite，支持 PostgreSQL；初期用 GORM `AutoMigrate` 建表，需要改名或回填时再增加显式版本迁移。
3. 审批通知渠道按 飞书（oapi-sdk-go）/ 企业微信 / 钉钉 / 通用 webhook 实现，无内置邮件与短信；企微与钉钉机器人走 stdlib HTTP POST。
4. 构建号用「每项目自增整数」，首次注册允许指定兼容商店既有版本的起始值；应用版本由 YAML 参数或触发参数提供，一期忽略 tag 事件。
5. 原生与 Flutter 模板及用户脚本进入 MVP；Google Play/App Store 与 custom upload 进入 MVP，平台与商店身份分别校验。
