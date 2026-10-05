# 开发指南

项目使用 Go 单模块，主分支为 `main`。功能开发与缺陷修复遵循 [AGENTS.md](../AGENTS.md) 和项目内的 Spec Kit 技能；用户使用说明见 [README](../README.md)。

## 目录结构

当前已经创建的目录：

```text
mybuilds/
├── cmd/
│   ├── mybuilds/main.go          # 客户端薄入口
│   ├── mybuilds-server/main.go   # 服务端薄入口
│   └── mybuilds-agent/main.go    # 007节点薄入口
├── internal/
│   ├── cli/
│   │   ├── client/root.go        # 本地与远程客户端命令
│   │   ├── server/root.go        # 服务启动与本机管理命令
│   │   ├── agent/root.go         # 节点启动与本地诊断
│   │   └── cli_test.go           # 双端 CLI 行为验收
│   ├── config/                  # 流水线及管理配置
│   ├── pipeline/                # 预览、执行、预算/收尾、日志与产物快照
│   ├── process/                 # pipeline/doctor 共用的进程执行与取消
│   ├── mobile/                  # Android/iOS/Flutter诊断、签名与可编辑模板
│   ├── distribute/              # 锁定第三方商店工具与单次发布
│   ├── server/                  # 鉴权HTTP、排队与节点路由
│   ├── agent/                   # 节点诊断、注册与执行接线
│   ├── protocol/                # 节点消息、租约与执行证据
│   ├── store/                   # 双数据库、独占、事务与进度
│   ├── scm/                     # 只读Git固定SHA与受限认证
│   └── version/version.go       # 共享版本与构建信息
├── examples/pipeline-preview.yml # 多 build 预览示例
├── examples/local-run.yml       # 可执行本地 shell 示例
├── examples/local-artifacts.yml # 快照、日志与 post 示例
├── examples/android/           # 原生 Java 工程与参数/签名接入示例
├── examples/flutter/           # 可编辑 Flutter 双平台工程、参数与签名接入示例
├── examples/mvp/               # 双平台集中验收生成器、配置与指南
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

当前模块及后续职责，尚未创建的位置随对应功能交付：

| 目录 | 职责 |
|---|---|
| `internal/config` | 流水线、客户端及服务端配置与校验 |
| `internal/pipeline` | 同一Run的预览、脚本、预算、post、产物、报告与发布屏障；审批暂停/原节点续执行 |
| `cmd/mybuilds-agent`、`internal/cli/agent` | 007 已验收帮助/版本/doctor/serve、实际执行闭环与全量检查；008恢复/重试已验收 |
| `internal/server` | 已接入控制端生命周期、鉴权 HTTP、节点调度、租约、中央日志/制品与停止保护 |
| `internal/agent` | 007 已验收诊断/注册/心跳、任务领取/续租、同一 Run 执行与日志/产物回传；008网络及终态核对已验收 |
| `internal/protocol` | 控制端与 Agent 共用的任务、租约及回报格式 |
| `internal/store` | 已接入双数据库独占、业务事务、快照与步骤进度持久化 |
| `internal/scm` | 已接入只读 Git 固定提交与 SSH 显式凭据；四种Webhook来源认证、固定SHA变更比较 |
| `internal/mobile` | 已接入 Android/Flutter 模板、真实受限doctor和纯参数检查；iOS签名生命周期由005提供 |
| `internal/distribute` | 锁定fastlane商店工具与具体单发动作；custom由012扩展同一授权/记录 |
| `internal/notify` | 后续013飞书等通知渠道，尚未创建 |
| `examples` | 可运行的配置与工程示例 |
| `deploy`（后续） | 部署模板与操作说明，当前尚未创建 |

保持单个 Go 模块；测试跟随所在包，必要数据放包内 `testdata/`。
控制端默认数据位于 `~/.mybuilds`，SQLite 文件不与节点共享；Agent 使用独立 data_dir 保存工作区与日志缓冲，本地流水线结果写临时目录。


## 多节点目标

控制端部署于 Linux/macOS，负责数据库、队列、审批和中央日志/产物；Agent 主动通过 HTTPS 连接控制端。
iOS 分配到具备 Xcode 和签名资源的 macOS 节点，Android 可分配到 Linux/macOS 节点。
默认每节点容量和全局并发上限均为 1，可配置；同项目同名 build 跨节点串行，不同 build 可并行，单次流水线固定一个节点。
节点失联不自动迁移已开始的构建，停止未确认时保留同名 build 互斥并隔离原节点；审批后核验原产物、报告，在原节点继续。
中央数据库由唯一控制端独占，节点不共享 SQLite 文件；启动锁阻止误运行第二调度进程。
同机可部署控制端和一个 Agent；一期支持多个构建节点，保留单控制端。


## 技术方向

CLI 使用 Cobra，流水线配置使用严格 YAML，管理配置使用严格节点检查后局部 Viper 合并。鉴权 HTTP 使用标准库，数据库使用 GORM，默认 SQLite、可选 PostgreSQL；两种驱动使用相同业务事务规则，PG 持锁 session 丢失不能自动重连继续写。
后续013通知规划使用飞书 `oapi-sdk-go/v3`，其他机器人通知使用标准库 HTTP；当前尚未接入消息发送。
构建产物由控制端托管下载；MVP 商店渠道为 Google Play 与 App Store，Go 封装第三方 fastlane 工具，节点需 Ruby/Bundler。
原生/Flutter 提供可编辑的内置模板，默认只构建/收集产物，单/双平台均生成对应名称的 builds；无参数 init 生成最小 default shell 配置。
用户可使用仓库脚本、本地模板或 custom 上传调用自己的 Fastfile；本地 run 用于构建调试，实际上传统一通过控制端与 Agent。
远程项目支持 `repo`、`auto`、`profile` 三种来源：repo 必须读取固定提交中的指定配置；auto 仅在该文件不存在时回退到绑定方案；profile 强制使用完整绑定方案，不与仓库配置合并。非法配置不会回退。
一个项目可含 Android/iOS 或多个应用的命名 build，批量触发固定同一 SHA，每个执行独立记录、统一分配项目构建号。
shell 支持内联命令和仓库脚本；参数经 env 映射传递，支持步骤工作目录、超时及受限的构建上下文变量。
项目通知配置已支持直接声明 Webhook、继承全局 defaults 与显式关闭；发送能力留待013。015用于接收 Git push 触发构建，与通知 Webhook 分开配置。
项目组用于归属和查询，构建方案用于复用配置；改组保留项目身份、历史、构建号和独立配置。
上传、提交审核、正式上架分别记录；Webhook 与 CLI 发布审批进入 MVP，其他内置分发渠道、机器人通知、轮询/cron 和部署打磨后置。
MVP 功能范围为 001–012、014–015、019–020；019/020 分别交付测试报告和项目保留策略；019 是 010/011 发布功能的前置，报告在审批/上传前封存，停止未确认或未知上传数据不清理。
编号不代表执行顺序，具体配置与验收见产品计划及实施路线。
这些业务依赖随功能引入并锁定版本，具体边界与安全、恢复要求见 [PLAN.md](plans/PLAN.md)。


## 验证与版本注入

```bash
go test -p 1 ./...
go vet ./...
```

进程安全测试会真实制造不可读的孤儿进程；全套测试用 `-p 1` 隔离跨包故障注入，不与真实应用验收同时运行。未知归属仍保留停止保护，不能为测试并行而放宽。

发布构建可注入版本信息；以下命令在当前模块名下可直接执行：

```bash
go build -ldflags '-X mybuilds/internal/version.Version=0.0.1 -X mybuilds/internal/version.Commit=demo -X mybuilds/internal/version.BuildDate=2026-10-04' -o bin/mybuilds ./cmd/mybuilds
./bin/mybuilds version
```

输出为 `0.0.1 (commit: demo, built: 2026-10-04)`。服务端使用相同参数与字段。
完整初始化验收见 [quickstart.md](../specs/000-project-bootstrap/quickstart.md)，结果见 [validation.md](../specs/000-project-bootstrap/validation.md)。


## 使用 Spec Kit 开发

项目使用 Spec Kit 1.0.13，Codex 为默认集成。项目技能位于 `.agents/skills/`，不需要额外的 `.codex/` 目录。
`specify` 是工具 CLI；以下 `$speckit-*` 在 Codex 聊天中调用，不是终端命令：

```text
$speckit-specify → $speckit-plan → $speckit-tasks → $speckit-analyze → $speckit-implement → $speckit-converge
```

需求有实质歧义时先 clarify；缺陷使用 bug-assess → bug-fix → bug-test。
每个功能的规范、计划、任务与验收记录保存在 `specs/`；契约稳定且依赖满足后允许独立 worktree 并行实现，由主代理集成，每完成并验收一个功能自动本地提交一次，不自动 push。
已有功能继续使用原规范；新增功能按路线逐项推进。

- [AGENTS.md](../AGENTS.md)：AI 阅读入口、开发流程与提交约定。
- [项目原则](../.specify/memory/constitution.md)：2.1.0，所有功能的稳定约束。
- [多节点设计](plans/MULTI_NODE.md)：角色职责、调度、租约与故障边界。
- [构建与分发设计](plans/BUILD_DISTRIBUTION.md)：内置模板、两大商店、第三方工具与用户扩展。
- [配置设计](plans/CONFIGURATION.md#配置文件)：server/client/agent 配置结构；命令面见 [INTERFACES.md](plans/INTERFACES.md#cli-面)。
- [实施路线](plans/SPECKIT_ROADMAP.md)：功能依赖、顺序与 001 操作案例。
- [MVP 执行计划](plans/MVP_EXECUTION.md)：并行批次、worktree 分区、集成与验收标准。
- [实施历史](IMPLEMENTATION_HISTORY.md)：功能状态、规范与验证索引。
- [初始化规范](../specs/000-project-bootstrap/spec.md)：项目初始化范围与验收要求。

变更入口、目录、运行方式或已实现能力时，同步更新 README 与相关使用说明。
