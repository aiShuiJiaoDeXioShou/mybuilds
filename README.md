# mybuilds

用 YAML 定义 Android、iOS 和 Flutter 构建流程，在本地调试后，交给自己的构建节点执行和发布。

mybuilds 复用工程已有的 Gradle、Xcode、Flutter 和 shell 脚本，管理构建参数、任务排队、日志、制品与发布审批。适合通过命令行操作、共用 Linux/macOS 构建机并保留构建记录的移动开发团队。

[安装](#安装) · [最小可用案例](#最小可用案例) · [模式对比](#模式对比) · [移动项目接入](#移动项目接入) · [使用指南](docs/USAGE.md)

## 目录

- [安装](#安装)
- [最小可用案例](#最小可用案例)
- [模式对比](#模式对比)
- [使用流程](#使用流程)
- [移动项目接入](#移动项目接入)
- [配置与示例](#配置与示例)
- [平台与限制](#平台与限制)
- [文档与开发](#文档与开发)
- [许可证](#许可证)

## 安装

目前需从源码构建。准备 **Go 1.25.0+** 和 Git，取得仓库源码后，进入包含 `go.mod` 的项目根目录。以下安装命令和本地案例适用于 macOS/Linux 的 shell。

### 本地使用：安装客户端

```bash
go mod download
mkdir -p bin
go build -o bin/mybuilds ./cmd/mybuilds
export PATH="$PWD/bin:$PATH"

mybuilds version
mybuilds --help
```

未注入版本信息时，`mybuilds version` 输出：

```text
dev (commit: unknown, built: unknown)
```

`export PATH` 对当前终端生效。如需在新终端中使用，将 `bin` 的绝对路径加入 shell 启动配置。`init`、`--help` 和 `run --dry-run` 无需移动 SDK；实际移动构建的工具要求见[移动项目接入](#移动项目接入)。

### 远程构建：按角色安装控制端和 Agent

在控制端机器和构建节点分别取得源码后，进入各自的项目根目录，构建对应程序：

```bash
mkdir -p bin

# 控制端机器
go build -o bin/mybuilds-server ./cmd/mybuilds-server

# 构建节点
go build -o bin/mybuilds-agent ./cmd/mybuilds-agent
```

三种程序可部署在同一台机器，也可分开部署。iOS 签名节点需要在 macOS 上启用 cgo 编译 Agent。数据库初始化、身份与节点配置见[远程部署指南](docs/USAGE.md#控制端与远程排队)。

## 最小可用案例

这个案例创建一个文本文件，将它保存为构建制品，并输出结果 JSON。完成客户端安装后，在同一终端按以下步骤执行。此案例无需移动工程、SDK 或控制端。

### 1. 创建独立工作目录

```bash
demo_dir="$(mktemp -d)"
cd "$demo_dir"
```

### 2. 定义流水线

```bash
cat > mybuilds.yml <<'YAML'
version: 1
steps:
  - kind: run
    name: build
    run: |
      mkdir -p output
      printf 'hello mybuilds\n' > output/hello.txt
  - kind: artifact
    name: collect
    paths: [output/hello.txt]
YAML
```

`run` 执行脚本；`artifact` 收集文件，保存独立快照、文件大小和 SHA-256。此配置简化自[制品示例](examples/local-artifacts.yml)。

### 3. 预览并执行

```bash
mybuilds run --dry-run
mybuilds run > result.json
cat output/hello.txt
cat result.json
```

`--dry-run` 输出计划，不执行脚本。预览成功后执行流水线。生成的文本文件内容为：

```text
hello mybuilds
```

`result.json` 中的关键字段：

| 字段 | 本例结果 |
|---|---|
| `builds[0].status` | `succeeded` |
| `builds[0].steps[1].artifacts[0].source_path` | `output/hello.txt` |
| 制品的 `size` | `15` 字节 |
| 制品的 `sha256`、`snapshot_path` | 内容摘要与独立快照路径 |
| `result_dir` | 本次日志和快照所在的结果目录 |

构建日志写入 stderr，结果 JSON 写入 stdout，可分别保存。脚本以当前目录为工作目录；`--file` 只选择配置文件。日志与快照保存在工作树之外，删除源制品不会改变已保存的快照。

## 模式对比

先用本地模式调通脚本，需要团队排队、集中记录和商店发布时再部署控制端与 Agent。

| 指标 | 本地执行 | 远程构建：SQLite | 远程构建：PostgreSQL |
|---|---|---|---|
| 程序种类 | 1：客户端 | 3：客户端、控制端、Agent | 3：客户端、控制端、Agent |
| 常驻控制端 / Agent 数量 | 0 / 0 | 1 / 至少 1 | 1 / 至少 1 |
| 额外数据库服务数 | 0 | 0，控制端使用 SQLite 文件 | 1，PostgreSQL 服务 |
| 执行位置 | 当前工作目录 | Agent 检出固定 Git 提交 | Agent 检出固定 Git 提交 |
| 多个命名构建 | 顺序执行 | 不同构建按容量并行 | 不同构建按容量并行 |
| 日志与制品 | 本地日志、快照与 SHA-256 | 中央保存、查询与下载 | 中央保存、查询与下载 |
| 商店上传 | 符合条件的 `upload` 在执行脚本前被拒绝 | 管理员授权后由 Agent 执行 | 管理员授权后由 Agent 执行 |

远程模式的全局并发上限和每节点容量默认均为 **1**，可通过配置调整。同项目同名构建始终串行；单次构建固定在 **1 个节点**执行。SQLite 也支持多个 Agent。两种数据库均由单个控制端独占。

以上对比列出部署数量和已实现行为，依据[使用指南](docs/USAGE.md)与[多节点设计](docs/plans/MULTI_NODE.md)。项目尚无可复现的速度、内存或成本基准。

## 使用流程

仓库中的 `mybuilds.yml` 定义流水线。一个流水线可包含多个命名构建（`builds`），例如 `android`、`ios` 或应用变体；每个构建包含参数和按顺序执行的步骤。

```text
本地：mybuilds.yml → mybuilds run → 脚本执行 → 日志与制品快照
远程：客户端 / Git Webhook → 控制端排队 → Agent 构建 → 中央日志与制品
```

| 程序 | 使用者与职责 |
|---|---|
| `mybuilds` | 开发者：生成配置、本地运行、触发远程构建、查看日志和下载制品 |
| `mybuilds-server` | 管理员：管理项目、权限、队列、审批和构建记录 |
| `mybuilds-agent` | 构建节点：连接控制端，领取任务，执行脚本并回传结果 |

### 本地调试

在移动工程根目录编辑配置，预览成功后再执行。配置仅包含一个构建时可省略 `--build`；包含多个构建时必须选择名称或使用 `--all`：

```bash
mybuilds run --build android --dry-run
mybuilds run --build android --param version=1.2.3
mybuilds run --all
```

`--param key=value` 为所有所选构建提供参数；`--param android:key=value` 只覆盖 `android`。参数通过配置中的 `env` 映射进入脚本，写法见[参数与条件示例](examples/local-run.yml)。

### 远程构建

首次部署时，按[使用指南](docs/USAGE.md#控制端与远程排队)依次完成以下步骤：

1. 配置控制端的数据库和首次管理员凭据，再启动控制端。
2. 登记节点，为 Agent 配置独立 token，启动 Agent。
3. 登记 Git 仓库并授权节点；无 `runner` 的通用脚本构建需指定 `default_node`。

仓库需提交流水线文件，控制端和 Agent 均需能读取对应 Git 提交。完成部署后，在客户端配置文件 `client.yml` 中设置服务地址和用户 token。先触发构建，再用返回的构建 ID 查询详情、日志和制品，最后用制品 ID 下载文件。将 `PROJECT`、`BUILD_ID`、`ARTIFACT_ID` 替换为实际项目名和返回的 ID。

```bash
mybuilds --config client.yml trigger PROJECT --build android --json
mybuilds --config client.yml build show BUILD_ID --json
mybuilds --config client.yml logs BUILD_ID --follow --stream-timeout 15m
mybuilds --config client.yml artifact ls BUILD_ID --json
mybuilds --config client.yml artifact download ARTIFACT_ID --output ./app.apk
```

`trigger` 返回排队结果，构建 ID 位于 `builds[].id`；没有符合条件的 Agent 时，任务保持 `queued`。制品 ID 来自 `artifact ls`。下载会校验大小和 SHA-256，并拒绝覆盖已有文件。

发布流程可配置审批。触发包含 `upload` 的构建时，必须使用管理员身份，并显式传入 `--allow-upload`。Google Play 与 App Store 发布还需在节点上准备 Ruby/Bundler、锁定的 fastlane 工具及渠道凭据。Google Play 另需指定版本的 bundletool。详见[发布与审批指南](docs/USAGE.md#审批与webhook)。

## 移动项目接入

在**已有移动工程根目录**执行对应的初始化命令。若已有 `mybuilds.yml`，初始化会拒绝覆盖。生成后需按工程填写和调整模板；初始化不会创建移动工程或安装 SDK。

| 工程 | 初始化命令 | 构建前准备 |
|---|---|---|
| 原生 Android | `mybuilds init --framework native --platform android` | Java 17+、Android SDK、项目 Gradle wrapper |
| 原生 iOS | `mybuilds init --framework native --platform ios` | macOS 15+、Xcode/iOS SDK、有效签名材料、启用 cgo 的程序 |
| Flutter 双平台 | `mybuilds init --framework flutter --platform android,ios` | Flutter SDK，以及各目标平台所需工具与签名材料 |

生成模板后，填写应用标识、版本、工程路径和签名环境引用。完成配置后，执行 `run --dry-run`。实际依赖版本、构建任务和制品路径以你的工程为准。

接入步骤可参考：[Android 示例](examples/android/README.md)、[Flutter 双平台示例](examples/flutter/README.md)、[原生 iOS 指南](specs/005-ios-build/quickstart.md)。

## 配置与示例

| 文件 | 用途 |
|---|---|
| 工程内 `mybuilds.yml` | 构建参数、步骤、条件、超时、`post` 收尾、报告与制品 |
| `client.yml` | 远程服务地址、用户 token、请求超时；默认 `~/.mybuilds/client.yml` |
| `server.yml` | 监听地址、数据库、数据目录和全局并发；默认 `~/.mybuilds/server.yml` |
| `agent.yml` | 节点身份、工具和秘密文件、节点数据目录；默认 `~/.mybuilds/agent.yml` |
| 项目 `settings.yml` | 通过 `project init/set --settings` 登记，选择仓库配置或可复用构建方案 |

本地 `init/run` 无需客户端、控制端或 Agent 的管理配置。远程命令与服务进程用 `--config` 选择管理配置；完整字段、路径与凭据规则见[使用指南](docs/USAGE.md)。

| 示例 | 展示内容 |
|---|---|
| [local-run.yml](examples/local-run.yml) | 参数传递、条件步骤、成功/失败/始终执行的收尾 |
| [local-artifacts.yml](examples/local-artifacts.yml) | 递归文件匹配、普通步骤与收尾分别保存制品快照 |
| [local-reports.yml](examples/local-reports.yml) | JUnit 报告；故意生成失败结果，运行返回非零 |
| [pipeline-preview.yml](examples/pipeline-preview.yml) | 多个命名构建、配置预览 |
| [自定义发布](examples/custom/README.md) | 自有发布脚本与应用绑定 |
| [集中案例](examples/mvp/acceptance.md) | 多节点、审批、Webhook 和可复用方案的组合使用 |

## 平台与限制

| 系统 | 客户端远程操作 / 配置预览 | 本地执行 / 控制端 / Agent | 移动构建目标 |
|---|---|---|---|
| macOS | 支持 | 支持 | Android、iOS、Flutter Android/iOS，需对应工具链 |
| Linux | 支持 | 支持 | Android、Flutter Android，需对应工具链 |
| Windows | 支持 | 暂不支持 | 通过远程节点构建 |

已实现 GitHub、GitLab、Gitee 和 generic Webhook 触发，以及 JUnit 报告、可复用构建方案和记录保留策略。当前使用 CLI；Web 界面、机器人通知发送、轮询与 cron 触发尚未实现。

### 验证状态

2026-10-05 的[集成验收记录](examples/mvp/validation.md#最终mvp集成)包含以下结果：

| 验收指标 | 结果 |
|---|---|
| 数据库后端 | SQLite、PostgreSQL，共 2 种 |
| 构建次数 | 每种数据库手动 / 自动触发各 1 次，共 4 次 |
| 检查项 | 118 / 118 通过 |

这些结果来自[已保存的证据](examples/mvp/evidence.json)，覆盖自有 Git 仓库、审批与 custom 发布接收器。真实 iOS/Flutter 签名、Apple/Google 商店发布和外部 Git 平台 push 仍需按[验收指南](docs/ACCEPTANCE.md)验证。上述验收结果不包含这些场景。

Agent 直接在宿主机执行仓库脚本，请使用可信仓库和独立构建账户。跨主机部署需要验证证书的 HTTPS 入口；控制端可使用反向代理提供 HTTPS。token、密码和签名私钥应保存在私有配置或秘密文件中。

## 文档与开发

- [使用指南](docs/USAGE.md)：部署、完整配置、管理命令与故障恢复。
- [验收指南](docs/ACCEPTANCE.md)：真实工具链与账号环境的验证步骤。
- [开发指南](docs/DEVELOPMENT.md)：代码结构、测试与版本注入。
- [产品规划](docs/plans/PLAN.md)与[实施历史](docs/IMPLEMENTATION_HISTORY.md)：设计决策、功能状态与验证索引。

| 目录 | 职责 |
|---|---|
| `cmd/` | 客户端、控制端、Agent 入口 |
| `internal/` | 配置、执行、移动工具链、调度、存储与发布实现 |
| `examples/` | 可运行配置、工程与辅助脚本 |
| `docs/` | 用户指南、开发指南与产品规划 |
| `specs/` | 功能规范、计划、任务与验证记录 |
| `.agents/skills/`、`.specify/` | 项目开发技能与 Spec Kit 配置 |

参与开发前阅读 [AGENTS.md](AGENTS.md)。注释与文档使用中文；功能开发和缺陷修复遵循项目内的 Spec Kit 流程。提交前运行：

```bash
go test -p 1 ./...
go vet ./...
```

进程安全测试包含故障注入。运行全套测试时需使用 `-p 1`，并与真实应用验收分开运行。报告问题时附版本、系统、复现命令和脱敏日志。

## 许可证

仓库尚未指定项目许可证，也没有仓库级 `LICENSE` 文件。第三方依赖遵循各自的许可证。
