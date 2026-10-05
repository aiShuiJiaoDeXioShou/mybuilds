# mybuilds

面向原生 Android、iOS 和 Flutter 的命令行构建发布工具。

在仓库中定义流水线，通过客户端发起构建，由控制端将任务分配给具备对应工具链的节点。一个仓库可以包含多个命名 build，例如 Android、iOS 或不同应用变体；也可以在本地运行流水线进行调试。

## 目录

- [功能](#功能)
- [平台与限制](#平台与限制)
- [安装](#安装)
- [快速开始](#快速开始)
- [移动项目接入](#移动项目接入)
- [配置](#配置)
- [常用命令](#常用命令)
- [文档与示例](#文档与示例)
- [项目结构](#项目结构)
- [参与开发](#参与开发)
- [许可证](#许可证)

## 功能

- **移动端构建**：提供 Android、iOS、Flutter 可编辑模板，支持仓库 shell 脚本和自定义模板。
- **多节点调度**：按平台、标签、容量和项目授权分配任务，固定 Git 提交执行，支持取消及原快照重试。
- **流水线控制**：多 build、命名参数、`when` 条件、超时、`post` 收尾和发布审批。
- **发布与扩展**：通过锁定的 fastlane 工具支持 Google Play、App Store，也支持自定义发布脚本。
- **自动触发**：接收 GitHub、GitLab、Gitee 和 generic Webhook，支持重复投递去重、固定等待窗口与变更路径筛选。
- **构建记录**：中央日志、制品下载、SHA-256 核对、JUnit 测试报告、项目组和保留清理策略。
- **配置复用**：优先使用仓库配置，文件缺失时可回退到项目绑定的构建方案，也可强制使用方案。

mybuilds 包含三个程序：

| 程序 | 职责 |
|---|---|
| `mybuilds` | 初始化与预览流水线、本地执行、远程触发、查询、下载和审批 |
| `mybuilds-server` | 管理项目、身份、队列、审批、数据库与中央构建记录 |
| `mybuilds-agent` | 在节点上诊断工具、领取任务、执行构建并回传日志与制品 |

控制端使用 SQLite 或 PostgreSQL，一个控制端可以管理多个 Agent。单次 build 固定在一个节点执行；同项目同名 build 串行，不同 build 可以按容量并行。

## 平台与限制

| 环境 | 支持范围 |
|---|---|
| macOS | 客户端、本地执行、控制端及 Agent；具备工具链时可构建 Android/iOS/Flutter |
| Linux | 客户端、本地执行、控制端及 Agent；具备工具链时可构建 Android/Flutter Android |
| Windows | 客户端远程 API、配置生成与预览；本地执行、控制端和 Agent 运行暂不支持 |

帮助、版本、配置生成与 dry-run 不需要移动工具链。实际构建需自行准备工程依赖：Android 使用 Java 17+、Android SDK 和项目 Gradle wrapper；Flutter 需要 Flutter SDK；iOS 签名需要 macOS 15+、Xcode/iOS SDK、合法签名材料，以及在 macOS 上启用 cgo 编译的程序。

商店分发还需 Ruby/Bundler、项目锁定的 fastlane 工具及对应渠道凭据，Google Play 另需指定版本的 bundletool。具体要求见[构建与发布指南](docs/USAGE.md)。iOS 签名及真实商店发布需要在你的应用和账号环境中验证，步骤见[验收指南](docs/ACCEPTANCE.md)。

节点直接在宿主机执行仓库脚本，请使用可信仓库和独立构建账户。跨主机部署使用验证证书的 HTTPS 入口；控制端可通过反向代理提供 HTTPS，客户端和 Agent 不支持跳过证书校验。

当前版本采用 CLI，尚未提供 Web 界面、机器人通知发送和轮询/cron 触发。

## 安装

要求 Go **1.25 或更新版本**和 Git。从已有项目源码目录构建三个程序；以下命令以 macOS/Linux 为例：

```bash
go mod download
mkdir -p bin
go build -o bin/mybuilds ./cmd/mybuilds
go build -o bin/mybuilds-server ./cmd/mybuilds-server
go build -o bin/mybuilds-agent ./cmd/mybuilds-agent

export MYBUILDS_ROOT="$PWD"
export PATH="$MYBUILDS_ROOT/bin:$PATH"
mybuilds --help
mybuilds-server --help
mybuilds-agent --help
```

`bin/` 不进入 Git。默认源码构建的版本输出为 `dev (commit: unknown, built: unknown)`；版本注入方法见[开发指南](docs/DEVELOPMENT.md#验证与版本注入)。

## 快速开始

先体验本地流水线，无需启动控制端或准备商店账号。在完成安装的同一终端执行：

```bash
demo_dir="$(mktemp -d)"
cd "$demo_dir"
mybuilds init
mybuilds run --dry-run
mybuilds run
```

`init` 创建最小 `mybuilds.yml`，已有文件时拒绝覆盖。dry-run 输出脱敏计划，不执行脚本、不读取密钥；实际 run 执行默认 `hello` 步骤，提示你编辑构建配置。

实际执行的日志写入 stderr，结果 JSON 写入 stdout；结果中的 `result_dir` 指向本次日志和快照目录。配置文件路径不改变脚本工作目录，本地结果存放在工作树之外。

体验参数、条件与收尾可在同一临时目录运行仓库中的示例：

```bash
mybuilds run --file "$MYBUILDS_ROOT/examples/local-run.yml" \
  --param message=hello --param release=no
```

完整服务端初始化、项目登记和节点启动步骤见[使用指南](docs/USAGE.md#控制端与远程排队)。

## 移动项目接入

在已有移动工程中生成配置；执行前确认该目录尚无 `mybuilds.yml`。按项目类型选择其中一条命令：

```bash
# 原生 Android
mybuilds init --framework native --platform android

# 原生 iOS
mybuilds init --framework native --platform ios

# Flutter：一个配置包含 Android、iOS 两个 build
mybuilds init --framework flutter --platform android,ios
```

这些命令只生成流水线，不创建移动工程或安装 SDK。编辑生成的配置，填写应用标识、工程参数与签名环境引用，并按工程实际要求调整脚本。多 build 运行时使用 `--build android,ios` 或 `--all`；参数可用 `--param version=1.0.0` 共享，也可用 `--param android:flavor=production` 只覆盖某个 build。

内置模板、本地 `--template` 文件和服务端可复用构建方案的接入方式见[使用指南](docs/USAGE.md#本地配置与执行)及[方案指南](specs/012-custom-workflows/quickstart.md)。可直接参考[Android 工程](examples/android/README.md)和[Flutter 双平台工程](examples/flutter/README.md)。

## 配置

| 配置 | 用途与路径规则 |
|---|---|
| 仓库 `mybuilds.yml` | 定义构建参数、环境引用、步骤、条件、报告、审批与上传 |
| 项目 `settings.yml` | 登记项目时通过 `--settings` 读取，选择仓库文件或构建方案、各 build 参数和项目策略 |
| `client.yml` | 控制端地址、用户 token、请求超时与可选 CA；默认 `~/.mybuilds/client.yml` |
| `server.yml` | 监听地址、数据库、并发、数据目录及默认策略；默认 `~/.mybuilds/server.yml` |
| `agent.yml` | 节点名称、独立 token、数据目录、工具和秘密文件；默认 `~/.mybuilds/agent.yml` |

`--settings ./settings.yml` 由客户端按当前目录读取；其中 `pipeline.file` 是仓库相对路径。项目设置支持 `repo`、`auto`、`profile`：repo 要求仓库文件，auto 仅在该文件确实缺失时回退到绑定方案，profile 使用完整方案且不与仓库配置合并。非法配置不会触发回退。

客户端、控制端和 Agent 用 `--config` 指定管理配置。相对服务端/Agent 路径基于各自配置文件目录。token 和秘密文件使用自有 `0600` 普通文件，token 也可使用完整 `${ENV_NAME}` 引用；每个节点保持独立数据目录。

配置继承、凭据、TLS 和环境变量的详细规则见[使用指南](docs/USAGE.md)；完整设计见[配置专题](docs/plans/CONFIGURATION.md)。

## 常用命令

下面是远程命令的用法；先在 `client.yml` 配置服务地址和身份，并将 `PROJECT`、`BUILD_ID`、`ARTIFACT_ID` 替换为实际返回值。

| 操作 | 命令 |
|---|---|
| 服务状态 | `mybuilds --config client.yml status --json` |
| 项目与节点 | `mybuilds --config client.yml project ls --json`、`mybuilds --config client.yml node ls --json` |
| 触发构建 | `mybuilds --config client.yml trigger PROJECT --build android --json` |
| 查看构建 | `mybuilds --config client.yml build show BUILD_ID --json` |
| 跟随日志 | `mybuilds --config client.yml logs BUILD_ID --follow --stream-timeout 15m` |
| 查看制品 | `mybuilds --config client.yml artifact ls BUILD_ID --json` |
| 下载制品 | `mybuilds --config client.yml artifact download ARTIFACT_ID --output ./downloaded-artifact` |
| 查看待审批项 | `mybuilds --config client.yml approvals --state pending --json` |

所选定义包含 upload 时，即使被条件跳过，也要求管理员显式 `--allow-upload`。实际发布由控制端授权并交给 Agent 执行；本地 run 用于构建调试，不能执行生效的 upload。

新触发可以显式指定 `--idempotency-key`；网络响应丢失后，用原 key 和相同输入恢复原请求。下载拒绝覆盖已有文件。精确审批、取消、重试、Webhook 和发布查询等命令见[使用指南](docs/USAGE.md)。

## 文档与示例

| 文档 | 内容 |
|---|---|
| [使用指南](docs/USAGE.md) | 本地流水线、控制端、节点、配置、日志、下载、恢复与管理 |
| [MVP 验收指南](docs/ACCEPTANCE.md) | 验收准备、分阶段步骤、通过标准、故障检查和记录模板 |
| [Flutter 集中案例](examples/mvp/acceptance.md) | 两节点、签名与商店工具、审批、Webhook、方案及 custom 的具体操作 |
| [已有验证记录](examples/mvp/validation.md) | 已执行检查的范围与人工待验项目 |
| [自定义发布示例](examples/custom/README.md) | 用户发布脚本、应用绑定与结果查询 |
| [开发指南](docs/DEVELOPMENT.md) | 代码结构、架构、测试、版本信息与 Spec Kit 工作流 |

其他可运行示例：[配置预览](examples/pipeline-preview.yml)、[制品快照](examples/local-artifacts.yml)、[JUnit 报告](examples/local-reports.yml)、[原生 iOS 模板](examples/native-ios.yml)。JUnit 示例会主动制造失败报告并返回非零，用于观察失败处理。

## 项目结构

| 目录 | 内容 |
|---|---|
| `cmd/` | 客户端、控制端、Agent 三个程序入口 |
| `internal/` | 配置、流水线、进程、构建、分发、服务、节点、存储与 Git 实现 |
| `examples/` | 可运行配置、工程和辅助脚本 |
| `docs/` | 使用、验收、开发说明与产品设计 |
| `specs/` | 功能规范、实施计划、任务和验证记录 |
| `.agents/skills/`、`.specify/` | 项目开发技能与 Spec Kit 配置 |

各模块职责和多节点边界见[开发指南](docs/DEVELOPMENT.md#目录结构)。

## 参与开发

报告问题时请附程序版本、操作系统、复现命令和脱敏日志，不附 token、密码或签名私钥。

从 `main` 创建开发分支。代码修改遵循 [AGENTS.md](AGENTS.md)；功能规范与历史见[开发指南](docs/DEVELOPMENT.md)和[实施历史](docs/IMPLEMENTATION_HISTORY.md)。文档使用中文，修改公开命令或行为时同步更新用户说明。

## 许可证

仓库目前尚未指定项目许可证，未提供仓库级 `LICENSE` 文件。第三方依赖使用各自的许可证。
