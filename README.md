# mybuilds

面向原生 Android/iOS 与 Flutter 工程的构建发布工具，用 Go 实现。
目标架构是客户端 `mybuilds`、控制端 `mybuilds-server` 与构建节点 `mybuilds-agent` 三种 CLI。
一个控制端管理多个构建节点，按平台、标签和容量分配任务；客户端也支持本地调试流水线。

本文是整个项目的概览入口。产品与技术决策见 [产品计划](docs/plans/PLAN.md)，功能顺序见 [实施路线](docs/plans/SPECKIT_ROADMAP.md)。

## 导航

- [当前状态](#当前状态)
- [开发与运行](#开发与运行)
- [控制端与远程排队](#控制端与远程排队)
- [目录结构](#目录结构)
- [多节点目标](#多节点目标)
- [技术方向](#技术方向)
- [验证与版本注入](#验证与版本注入)
- [使用 Spec Kit 开发](#使用-spec-kit-开发)

## 当前状态

已完成 `000-project-bootstrap`：Go 单模块、双 CLI 帮助与共享版本、Spec Kit 项目原则和开发流程。
多节点设计已确定，见 [MULTI_NODE.md](docs/plans/MULTI_NODE.md)，Agent 与节点协议尚未实现。
客户端已接入 `init`、本地模板、严格配置校验与 `run --dry-run` 脱敏预览；支持单/多 build 选择、参数覆盖和条件三态。
`001-pipeline-preview` 已实现并通过集成验证，证据见[验证记录](specs/001-pipeline-preview/validation.md)。`002-local-run` 已接入本地顺序执行、进程组取消、预算/post 和 UTC 脱敏日志，已通过验收；`003-build-artifacts` 已接入产物快照与本地结果/步骤日志并通过验收，见[验证记录](specs/003-build-artifacts/validation.md)。审批、通知和上传仍待实现。
MVP 目标已扩展至原生/Flutter 双平台、多节点构建、Google Play/App Store 分发与用户自定义，见 [构建与分发设计](docs/plans/BUILD_DISTRIBUTION.md)。
`004-android-build` 已完成本机 `doctor`、可编辑模板和真实工程验收：APK/AAB 版本与签名、mapping、快照摘要、离线缓存、失败和取消均通过，见[验证记录](specs/004-android-build/validation.md)。`005-ios-build` 在独立 worktree 实现，真实 Apple profile 与签名 archive/export 尚未验收；当前已集成代码不提供 iOS 签名执行。Flutter 模板仍待后续功能交付。
`006-control-plane` 已接入严格管理配置、双数据库 Store、单控制端独占、鉴权 HTTP、只读 Git 固定提交与远程 CLI。已通过最终全量/race/vet、SQLite 与 PostgreSQL 各 37 项真实二进制验收，以及 Linux 上 37 项闭环；Spec Kit 收敛无缺口并按整功能提交，见[验证记录](specs/006-control-plane/validation.md)。触发结果只有 queued/skipped；没有 Agent、实际远程构建、发布或审批操作。
项目组已经接入：注册时可选组，未指定归入 default，支持普通组改名、空组删除和项目迁移，项目历史与编号保持。
一个 YAML 的多个命名 build、本地参数/env 映射、when、累计超时、post 和日志时间戳已经实现；无 YAML 绑定双平台方案、自动变更筛选、Webhook 等待窗口、保留策略、JUnit 与发布审批仍待实现。

## 开发与运行

要求 Go **1.25 或更新版本**、Git。首次下载 Go 依赖需要网络；已使用 Cobra、YAML v3、doublestar/v4，以及管理配置的 Viper 1.21.0、数据库访问的 GORM 1.31.2 与 SQLite/PostgreSQL 双驱动。SQLite 引擎锁定 modernc.org/sqlite 1.55.0（实际 SQLite 3.53.3），包含 WAL 修补；依赖版本见 [go.mod](go.mod) 和 [go.sum](go.sum)。
帮助、版本、init 和 dry-run 无需移动工具链。Android doctor 要求 Java 17+、已有 Android SDK 和工程内 Gradle wrapper；完整构建以工程实际要求为准，不自动安装 SDK。新增签名行为测试需要 JDK/keytool。
本地执行和控制端支持 macOS/Linux；本地脚本需要 sh 或所选 bash，Git 快照需要 Git。Windows 客户端可纯 Go 编译和调用远程 API，本地执行与本机控制端明确未支持。

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
`mybuilds init --platform android` 默认使用 native 框架，等价于 `--framework native --platform android`，生成一个 `builds.android`。模板默认 app/release，构建 APK/AAB/mapping；工程须显式读取 `APP_VERSION/BUILD_NUMBER` 和签名环境变量，release 须开启 R8。参数默认 1.0.0/1，覆盖使用 `--param version=1.2.3 --param build_number=42`；任务与路径均可编辑。
`mybuilds doctor --platform android --json --working-dir <工程目录>` 检查实际 Java、SDK 包和 wrapper。签名检查须显式提供 `--keystore`、`--key-alias`、`--store-password-env`、`--key-password-env`，两个密码仅从指定环境变量读取；未声明签名为 skipped，任一 failed 返回非零。doctor 默认 android；远端节点检查尚未接入。首次 wrapper 检查可能下载工程锁定的 Gradle 到缓存，普通 run 不会自动调用 doctor。
`run --dry-run` 只输出脱敏 JSON，不执行脚本、Git 或网络请求，也不读取密钥。多 build 必须用 `--build android,ios` 或 `--all`，参数用重复的 `--param key=value`；`--step` 仅限单 build。
去掉 `--dry-run` 执行本地脚本：日志写 stderr，脱敏结果 JSON 写 stdout，失败/取消返回非零。配置路径不改变当前工作目录，多个 build 顺序执行；`--step` 只运行选中普通步骤，不自动执行前序依赖。
先校验整批再启动脚本，当前生效的 approval/reports/notifications 会报未支持；有效通知 `enabled: false` 可关闭。实际上传通过后续远程控制端，生效 upload 在任何流水线脚本前拒绝。
artifact 支持相对根目录递归 glob（`**`）；每个模式须匹配普通文件，按步骤保存独立快照、大小和 SHA-256。结果 JSON 的 `result_dir` 定位临时结果根，`log_path` 定位 UTC 脱敏步骤日志，失败后仍保留完整证据。普通与 post 的快照分开；结果数据不进入源码工作树。预览、预检查失败、全部跳过不创建结果目录。
可运行的本地例子见 [local-run.yml](examples/local-run.yml) 和 [local-artifacts.yml](examples/local-artifacts.yml)，在临时目录以已构建二进制的绝对路径和 `--file` 指向该例子运行。
Android 工程接入、临时测试签名和构建命令见 [Android 示例](examples/android/README.md)；完整包核验见 [004 验收指南](specs/004-android-build/quickstart.md)。

## 控制端与远程排队

当前控制端只读取已登记仓库的固定提交，校验参数、条件与权限，再原子创建 queued/skipped 记录。所选 build 共享提交 SHA、按项目统一分配编号；skipped 不占号。节点名现在用于登记授权范围，007 才匹配已注册节点，当前不会执行仓库脚本。

下面在自建临时目录演示完整流程。先按上文构建两个二进制；示例使用 OpenSSL 生成随机管理员 token，也可由密码管理器预先注入同名环境变量。token 通过环境传递，不写入命令参数。

```bash
project_dir="$(pwd)"
client_bin="$project_dir/bin/mybuilds"
server_bin="$project_dir/bin/mybuilds-server"
work_dir="$(mktemp -d)"
umask 077
mkdir "$work_dir/repo"

cat > "$work_dir/repo/mybuilds.yml" <<'YAML'
version: 1
builds:
  android:
    runner: {platform: android}
    params:
      version: {required: true}
      channel: {default: internal}
    steps:
      - kind: run
        run: printf queued_example
YAML

git -C "$work_dir/repo" init --initial-branch=main
git -C "$work_dir/repo" add mybuilds.yml
git -C "$work_dir/repo" -c user.name=Example -c user.email=example@example.test \
  commit -m '演示排队配置'

cat > "$work_dir/server.yml" <<'YAML'
listen: 127.0.0.1:8787
data_dir: data
concurrency: 1
database:
  driver: sqlite
  dsn: data/control.db
YAML
cat > "$work_dir/client.yml" <<'YAML'
server: http://127.0.0.1:8787
token: '${MYBUILDS_CLIENT_TOKEN}'
timeout: 30s
YAML
cat > "$work_dir/settings.yml" <<'YAML'
pipeline:
  source: repo
  file: mybuilds.yml
  builds:
    android:
      params: {channel: internal}
YAML
chmod 0600 "$work_dir/client.yml"

export MYBUILDS_BOOTSTRAP_ADMIN_TOKEN="$(openssl rand -hex 32)"
export MYBUILDS_CLIENT_TOKEN="$MYBUILDS_BOOTSTRAP_ADMIN_TOKEN"
"$server_bin" --config "$work_dir/server.yml" migrate
"$server_bin" --config "$work_dir/server.yml" serve &
server_pid=$!

# 最多等待五秒；最后一次status失败时检查服务端诊断。
for attempt in 1 2 3 4 5; do
  if "$client_bin" --config "$work_dir/client.yml" status --json >/dev/null 2>&1; then break; fi
  sleep 1
done
"$client_bin" --config "$work_dir/client.yml" status --json

cd "$work_dir"
"$client_bin" --config ./client.yml group create apps
"$client_bin" --config ./client.yml project init mobile \
  --repo "$work_dir/repo" --nodes android-1 --group apps --settings ./settings.yml
"$client_bin" --config ./client.yml trigger mobile --build android \
  --param version=1.2.0 --param android:channel=internal \
  --idempotency-key demo-request-1 --json
"$client_bin" --config ./client.yml build ls --project mobile --group apps --json

# 用触发结果中的id替换BUILD_ID，可查看预算、条件与ordinary/post步骤进度。
# "$client_bin" --config ./client.yml build show BUILD_ID --json

kill -TERM "$server_pid"
wait "$server_pid"
cd "$project_dir"
```

结果为 queued，示例的 `printf` 不会执行。`build show` 默认输出安全详情表格，`--json` 返回同一视图。参数值、脚本正文和凭据不公开，列表默认 20 条、最大 200 条，支持 limit/offset 及项目、组、build 名、批次和状态过滤。

`--settings ./settings.yml` 由客户端按当前目录读取内容；`pipeline.file` 和注册时的 `--file ci/mybuilds.yml` 都是仓库相对路径，`--file` 与 `--settings` 互斥。`project set mobile --settings ./settings.yml` 替换设置；项目 framework/platform 绑定方案、hook/poll/schedule 当前明确报未支持。流水线必须存在于所选提交，source 仅支持 auto/repo，尚无缺文件时的绑定方案回退。

触发可用 `--build android,ios` 或 `--all`。共享 `--param key=value` 应用到所有所选 build，`--param android:key=value` 只覆盖指定 build；命名值优先，同 scope 重复参数拒绝。`--version`、`--channel` 是共享参数快捷选项。客户端每次新请求生成一个随机 key，在发起网络请求前写到 stderr，成功 JSON 含 `request_key`；网络失败后用原 `--idempotency-key` 和相同参数恢复，重放返回原 SHA/编号/结果，不自动重试或换 key。

管理命令如下，列表支持 `--json`：

| 命令端 | 当前命令 |
|---|---|
| 客户端 | group create/ls/rename/rm；project init/set/ls/move/rm；trigger；build ls/show；status |
| 服务端本机 | serve/migrate；group create/ls/rename/rm；project add/set/ls/move/rm；token create/ls/revoke |

serve 在线时同一数据库被独占，本机 migrate、project/group/token 管理会拒绝，使用客户端远程管理或鉴权 HTTP API。停止示例控制端后，可创建身份并查看安全列表：

```bash
"$server_bin" --config "$work_dir/server.yml" token create --role trigger --json
"$server_bin" --config "$work_dir/server.yml" token ls --json
```

token create 只显示一次明文，列表不显示 token；首次管理员通过 `MYBUILDS_BOOTSTRAP_ADMIN_TOKEN` 初始化，撤销全部身份后重启不会重新创建 bootstrap 身份。

角色为 admin/trigger/approver：admin 管理与读写；trigger 只读 status 并触发所选定义不含 upload 的构建；approver 只读 status 和全部脱敏 build 证据。所选定义含 upload 时，即使条件为 false，也要求 admin 与显式 `--allow-upload`；本阶段仍只排队，不执行上传。

配置默认位于 `~/.mybuilds/server.yml`、`~/.mybuilds/client.yml`。覆盖顺序为默认值 < 文件 < 明确白名单环境变量 < 显式 CLI；服务端白名单为 `MYBUILDS_LISTEN`、`MYBUILDS_DATA_DIR`、`MYBUILDS_CONCURRENCY`、`MYBUILDS_DATABASE_DRIVER`、`MYBUILDS_DATABASE_DSN`、`MYBUILDS_SECRETS_FILE`；客户端为 `MYBUILDS_SERVER_URL`、`MYBUILDS_CLIENT_TOKEN`、`MYBUILDS_CLIENT_TIMEOUT`。相对服务端路径基于配置文件目录，data_dir 创建为或要求 0700；客户端 token 可以是 0600 文件中的字面量或完整 `${NAME}` 引用，空/弱 token 拒绝。PostgreSQL 可通过 database.driver 和私有环境中的 MYBUILDS_DATABASE_DSN 配置；切换数据库不迁移已有数据。PostgreSQL 只使用明确 DSN，拒绝非空宿主 PG 环境变量和隐式 service/passfile；显式 TLS 材料需受限普通文件，读入内存后验证，不继承宿主凭据。

客户端只有远程命令读取 client 配置；损坏的 client.yml 或缺 token 不影响本地 init/run/doctor/help/version。`--server-url` 覆盖远程地址，`--timeout` 只控制普通 API 请求，不改变本地 YAML 的 build/post 预算。客户端仅允许回环 HTTP 或验证证书的 HTTPS，并拒绝重定向。当前服务端提供 HTTP 监听，可放在终止 TLS 的反向代理后；跨主机客户端必须通过 HTTPS 入口连接并验证服务器证书，不能跳过校验。

实际 API、配置与验收步骤见 [006 契约](specs/006-control-plane/contracts/http.md)、[配置/CLI](specs/006-control-plane/contracts/config-cli.md) 和 [验收指南](specs/006-control-plane/quickstart.md)。

## 目录结构

当前已经创建的目录：

```text
mybuilds/
├── cmd/
│   ├── mybuilds/main.go          # 客户端薄入口
│   └── mybuilds-server/main.go   # 服务端薄入口
├── internal/
│   ├── cli/
│   │   ├── client/root.go        # 本地与远程客户端命令
│   │   ├── server/root.go        # 服务启动与本机管理命令
│   │   └── cli_test.go           # 双端 CLI 行为验收
│   ├── config/                  # 流水线及管理配置
│   ├── pipeline/                # 预览、执行、预算/收尾、日志与产物快照
│   ├── process/                 # pipeline/doctor 共用的进程执行与取消
│   ├── mobile/                  # Android doctor 与可编辑内嵌模板
│   ├── server/                  # 鉴权HTTP与仅排队控制端
│   ├── store/                   # 双数据库、独占、事务与进度
│   ├── scm/                     # 只读Git固定SHA与受限认证
│   └── version/version.go       # 共享版本与构建信息
├── examples/pipeline-preview.yml # 多 build 预览示例
├── examples/local-run.yml       # 可执行本地 shell 示例
├── examples/local-artifacts.yml # 快照、日志与 post 示例
├── examples/android/           # 原生 Java 工程与参数/签名接入示例
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
| `internal/pipeline` | 已接入本地预览、执行、产物与脱敏；审批/上传后续接入 |
| `cmd/mybuilds-agent`、`internal/cli/agent` | 后续 Agent 入口与节点命令 |
| `internal/server` | 已接入控制端生命周期、鉴权 HTTP 与排队；节点调度/恢复后续接入 |
| `internal/agent` | 任务领取、续租、节点执行、日志与产物回传 |
| `internal/protocol` | 控制端与 Agent 共用的任务、租约及回报格式 |
| `internal/store` | 已接入双数据库独占、业务事务、快照与步骤进度持久化 |
| `internal/scm` | 已接入只读 Git 固定提交与 SSH 显式凭据；Webhook 来源后续接入 |
| `internal/mobile` | 已接入 Android 模板/doctor；iOS/Flutter、签名生命周期随后交付 |
| `internal/distribute` | fastlane 商店封装与 custom 上传，共用发布记录 |
| `internal/notify` | 飞书等通知渠道 |
| `examples` | 可运行的配置与工程示例 |
| `deploy` | 部署模板与操作说明 |

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
飞书采用官方第三方 `oapi-sdk-go/v3`，其他机器人通知使用标准库 HTTP。
构建产物由控制端托管下载；MVP 商店渠道为 Google Play 与 App Store，Go 封装第三方 fastlane 工具，节点需 Ruby/Bundler。
原生/Flutter 提供可编辑的内置模板，默认只构建/收集产物，单/双平台均生成对应名称的 builds；无参数 init 生成最小 default shell 配置。
用户可使用仓库脚本、本地模板或 custom 上传调用自己的 Fastfile；本地 run 用于构建调试，实际上传统一通过控制端与 Agent。
远程项目当前读取固定提交中的 mybuilds.yml 或明确指定文件，缺失即失败；012 再交付项目绑定的可复用构建方案与来源回退。
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
