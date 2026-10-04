# 技术选择、架构与状态

本文件保留原 [PLAN.md](PLAN.md) 的完整专题章节；业务功能按 [实施路线](SPECKIT_ROADMAP.md) 推进，当前状态见 [实施历史](../IMPLEMENTATION_HISTORY.md)。

## 关键设计选择

### 1. 技术选型：按实际能力引入依赖

| 需求 | 选择 | 理由 |
|---|---|---|
| CLI 框架 | `github.com/spf13/cobra` | 子命令树、自动补全、help 生成 |
| 服务端配置加载 | `github.com/spf13/viper` | 服务端配置与环境变量覆盖；固定覆盖优先级 |
| 流水线 / 客户端配置 | `go.yaml.in/yaml/v3` | 先检查节点类型与字段再转为结构模型，严格拒绝未知/重复字段与隐式标量转换；采用维护中的 YAML 项目 |
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
