# mybuilds

面向原生 Android/iOS 与 Flutter 工程的构建发布工具，用 Go 实现。
目标架构是客户端 `mybuilds`、控制端 `mybuilds-server` 与构建节点 `mybuilds-agent` 三种 CLI。
一个控制端管理多个构建节点，按平台、标签和容量分配任务；客户端也支持本地调试流水线。

本文是整个项目的概览入口。产品与技术决策见 [产品计划](docs/plans/PLAN.md)，功能顺序见 [实施路线](docs/plans/SPECKIT_ROADMAP.md)。

## 当前状态

已完成 `000-project-bootstrap`：Go 单模块、双 CLI 帮助与共享版本、Spec Kit 项目原则和开发流程。
多节点设计已确定，见 [MULTI_NODE.md](docs/plans/MULTI_NODE.md)，Agent 与节点协议尚未实现。
客户端已接入 `init`、本地模板、严格配置校验与 `run --dry-run` 脱敏预览；支持单/多 build 选择、参数覆盖和条件三态。
`001-pipeline-preview` 已实现并通过集成验证，证据见[验证记录](specs/001-pipeline-preview/validation.md)。实际流水线执行、HTTP 服务、数据库、审批、通知和上传仍待实现。
MVP 目标已扩展至原生/Flutter 双平台、多节点构建、Google Play/App Store 分发与用户自定义，见 [构建与分发设计](docs/plans/BUILD_DISTRIBUTION.md)。
项目组列入控制端规划：客户端注册时选择组，未指定归入 default，支持组改名与项目迁移；尚未实现。
同仓库多命名 build 已列入规划：一个项目定义多个独立构建，支持无 YAML 绑定双平台方案；shell 参数与执行约定见产品计划，均尚未实现。
MVP 新增 when、参数约束、build 总超时、post 收尾、日志时间戳、变更筛选、Webhook 等待窗口、项目保留策略和 JUnit 报告，发布审批也纳入；均为待实现规划。

## 开发与运行

要求 Go **1.25 或更新版本**、Git。首次下载 Go 依赖需要网络；直接依赖为 Cobra 与 YAML v3。
当前帮助与版本命令无需 Xcode、JDK 或 Android SDK；真正的移动端构建在对应功能接入时再检测工具链。

在项目根目录运行：

```bash
go mod download
go run ./cmd/mybuilds --help
go run ./cmd/mybuilds version
go run ./cmd/mybuilds-server --help
go run ./cmd/mybuilds-server version
go run ./cmd/mybuilds run --file examples/pipeline-preview.yml --all --dry-run
```

两个版本命令默认输出相同：

```text
dev (commit: unknown, built: unknown)
```

构建本机二进制：

```bash
mkdir -p bin
go build -o bin/mybuilds ./cmd/mybuilds
go build -o bin/mybuilds-server ./cmd/mybuilds-server
./bin/mybuilds --help
./bin/mybuilds-server version
```

`bin/` 不进入 Git。未指定远端仓库，Go 模块名暂为 `mybuilds`。
无参数运行显示帮助；未知子命令或 `version` 多余参数返回非零退出码。

在目标仓库运行 `mybuilds init` 创建最小 `mybuilds.yml`，已有目标拒绝覆盖；`init --template ./ci/template.yml` 使用经校验的本地模板。
`run --dry-run` 只输出脱敏 JSON，不执行脚本、Git 或网络请求，也不读取密钥。多 build 必须用 `--build android,ios` 或 `--all`，参数用重复的 `--param key=value`；`--step` 仅限单 build。
没有 `--dry-run` 的执行与平台模板选项目前明确报未支持。

## 目录结构

当前已经创建的目录：

```text
mybuilds/
├── cmd/
│   ├── mybuilds/main.go          # 客户端薄入口
│   └── mybuilds-server/main.go   # 服务端薄入口
├── internal/
│   ├── cli/
│   │   ├── client/root.go        # 客户端命令
│   │   ├── server/root.go        # 服务端命令
│   │   └── cli_test.go           # 双端 CLI 行为验收
│   ├── config/                  # YAML 严格解析、约束及参数选择
│   ├── pipeline/                # 纯预览与模板/条件校验
│   └── version/version.go       # 共享版本与构建信息
├── examples/pipeline-preview.yml # 多 build 预览示例
├── specs/                       # 各功能规范、计划、任务与验证
├── docs/plans/                  # 产品决策与功能实施路线
├── .agents/skills/              # 项目内 Codex 技能
├── .specify/                    # Spec Kit 原则、模板、脚本与集成配置
├── .pi/prompts/                 # 保留的 Pi 提示词
├── AGENTS.md                    # AI 执行约定
├── README.md                    # 项目概览
├── go.mod
└── go.sum
```

采用的后续模块位置，随功能创建：

| 目录 | 职责 |
|---|---|
| `internal/config` | 流水线、客户端及服务端配置与校验 |
| `internal/pipeline` | 本地与服务端共享的执行、产物、审批、上传和脱敏 |
| `cmd/mybuilds-agent`、`internal/cli/agent` | 后续 Agent 入口与节点命令 |
| `internal/server` | 控制端生命周期、HTTP API、节点调度与重启恢复 |
| `internal/agent` | 任务领取、续租、节点执行、日志与产物回传 |
| `internal/protocol` | 控制端与 Agent 共用的任务、租约及回报格式 |
| `internal/store` | 数据模型、事务和数据库访问 |
| `internal/scm` | Git 工作区与 Webhook 来源处理 |
| `internal/mobile` | 原生/Flutter 模板、doctor、版本与签名辅助 |
| `internal/distribute` | fastlane 商店封装与 custom 上传，共用发布记录 |
| `internal/notify` | 飞书等通知渠道 |
| `examples` | 可运行的配置与工程示例 |
| `deploy` | 部署模板与操作说明 |

保持单个 Go 模块；测试跟随所在包，必要数据放包内 `testdata/`。
计划中的控制端数据位于 `~/.mybuilds`；Agent 使用独立 data_dir 保存工作区与日志缓冲，本地流水线结果写临时目录。

## 多节点目标

控制端部署于 Linux/macOS，负责数据库、队列、审批和中央日志/产物；Agent 主动通过 HTTPS 连接控制端。
iOS 分配到具备 Xcode 和签名资源的 macOS 节点，Android 可分配到 Linux/macOS 节点。
默认每节点容量和全局并发上限均为 1，可配置；同项目同名 build 跨节点串行，不同 build 可并行，单次流水线固定一个节点。
节点失联不自动迁移已开始的构建，停止未确认时保留同名 build 互斥并隔离原节点；审批后核验原产物、报告，在原节点继续。
中央数据库由唯一控制端独占，节点不共享 SQLite 文件；启动锁阻止误运行第二调度进程。
同机可部署控制端和一个 Agent；一期支持多个构建节点，保留单控制端。

## 技术方向

CLI 使用 Cobra，流水线配置使用 YAML；后续服务端 HTTP 使用标准库，数据库使用 GORM，默认 SQLite、可选 PostgreSQL。
飞书采用官方第三方 `oapi-sdk-go/v3`，其他机器人通知使用标准库 HTTP。
构建产物由控制端托管下载；MVP 商店渠道为 Google Play 与 App Store，Go 封装第三方 fastlane 工具，节点需 Ruby/Bundler。
原生/Flutter 提供可编辑的内置模板，默认只构建/收集产物，单/双平台均生成对应名称的 builds；无参数 init 生成最小 default shell 配置。
用户可使用仓库脚本、本地模板或 custom 上传调用自己的 Fastfile；本地 run 用于构建调试，实际上传统一通过控制端与 Agent。
远程项目默认优先仓库 mybuilds.yml，缺失时可使用项目绑定的可复用构建方案；显式支持只用仓库或只用方案。
一个项目可含 Android/iOS 或多个应用的命名 build，批量触发固定同一 SHA，每个执行独立记录、统一分配项目构建号。
shell 支持内联命令和仓库脚本；参数经 env 映射传递，支持步骤工作目录、超时及受限的构建上下文变量。
项目可直接配置通知 Webhook；未配置时继承全局 defaults，也可显式关闭，无需预先注册渠道；敏感值受限保存。
项目组用于归属和查询，构建方案用于复用配置；改组保留项目身份、历史、构建号和独立配置。
上传、提交审核、正式上架分别记录；Webhook 与 CLI 发布审批进入 MVP，其他内置分发渠道、机器人通知、轮询/cron 和部署打磨后置。
MVP 功能范围为 001–012、014–015、019–020；019/020 分别交付测试报告和项目保留策略；019 是 010/011 发布功能的前置，报告在审批/上传前封存，停止未确认或未知上传数据不清理。
编号不代表执行顺序，具体配置与验收见产品计划及实施路线。
这些业务依赖随功能引入并锁定版本，具体边界与安全、恢复要求见 [PLAN.md](docs/plans/PLAN.md)。

## 验证与版本注入

```bash
go test ./...
go vet ./...
```

发布构建可注入版本信息；以下命令在当前模块名下可直接执行：

```bash
go build -ldflags '-X mybuilds/internal/version.Version=0.0.1 -X mybuilds/internal/version.Commit=demo -X mybuilds/internal/version.BuildDate=2026-10-04' -o bin/mybuilds ./cmd/mybuilds
./bin/mybuilds version
```

输出为 `0.0.1 (commit: demo, built: 2026-10-04)`。服务端使用相同参数与字段。
完整初始化验收见 [quickstart.md](specs/000-project-bootstrap/quickstart.md)，结果见 [validation.md](specs/000-project-bootstrap/validation.md)。

## 使用 Spec Kit 开发

项目使用 Spec Kit 1.0.13，Codex 为默认集成。项目技能位于 `.agents/skills/`，不需要额外的 `.codex/` 目录。
`specify` 是工具 CLI；以下 `$speckit-*` 在 Codex 聊天中调用，不是终端命令：

```text
$speckit-specify → $speckit-plan → $speckit-tasks → $speckit-analyze → $speckit-implement → $speckit-converge
```

需求有实质歧义时先 clarify；缺陷使用 bug-assess → bug-fix → bug-test。
每个功能的规范、计划、任务与验收记录保存在 `specs/`；契约稳定且依赖满足后允许独立 worktree 并行实现，由主代理集成，每完成并验收一个功能自动本地提交一次，不自动 push。
已有功能继续使用原规范；新增功能按路线逐项推进。

- [AGENTS.md](AGENTS.md)：AI 阅读入口、开发流程与提交约定。
- [项目原则](.specify/memory/constitution.md)：2.1.0，所有功能的稳定约束。
- [多节点设计](docs/plans/MULTI_NODE.md)：角色职责、调度、租约与故障边界。
- [构建与分发设计](docs/plans/BUILD_DISTRIBUTION.md)：内置模板、两大商店、第三方工具与用户扩展。
- [配置设计](docs/plans/CONFIGURATION.md#配置文件)：server/client/agent 配置结构；命令面见 [INTERFACES.md](docs/plans/INTERFACES.md#cli-面)。
- [实施路线](docs/plans/SPECKIT_ROADMAP.md)：功能依赖、顺序与 001 操作案例。
- [MVP 执行计划](docs/plans/MVP_EXECUTION.md)：并行批次、worktree 分区、集成与验收标准。
- [实施历史](docs/IMPLEMENTATION_HISTORY.md)：功能状态、规范与验证索引。
- [初始化规范](specs/000-project-bootstrap/spec.md)：本次范围与验收要求。

变更入口、目录、运行方式或已实现能力时，同步更新本文。
