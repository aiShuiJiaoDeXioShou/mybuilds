# CLI 命令速查

本页按使用任务汇总三个程序的公开命令。首次部署见 [README](../README.md#首次部署控制端客户端与同机-agent)，详细配置见[使用指南](USAGE.md)。完整参数以对应命令的 `--help` 为准。

## 选择程序与配置

| 程序 | 用途 | 默认配置 |
|---|---|---|
| `mybuilds` | 本地构建，或通过 HTTP API 管理远程项目与构建 | `~/.mybuilds/client.yml`，仅远程操作读取 |
| `mybuilds-server` | 启动控制端，或直接管理本机数据库 | `~/.mybuilds/server.yml` |
| `mybuilds-agent` | 在构建机执行任务 | `~/.mybuilds/agent.yml`，仅 `serve` 读取 |

三个程序都支持 `--config PATH`、`version`、`--help`、`help` 和 shell 补全 `completion`。指定配置只选择读取位置，不会创建文件。服务配置需自行准备；`mybuilds init` 只生成流水线文件。

| 参数 | 含义 |
|---|---|
| `--config` | 当前程序的管理或连接配置；不是流水线 |
| `mybuilds run --file` | 本地流水线，默认当前目录的 `mybuilds.yml`；允许绝对路径，脚本工作目录仍从当前目录开始 |
| `project init --file` / `project add --file` | Git 仓库内的流水线相对路径，同时选择 `repo` 来源 |
| `project init --settings` / `project add --settings` / `project set --settings` | 读取本地项目设置并导入控制端，可包含 `pipeline`、`hook`、`triggers`、`retention` 等设置块 |

创建项目时，`--file`、`--settings`、`--framework/--platform` 三种配置入口互斥。更新设置时，显式提供的块整体替换原块，未提供的块保留；多构建项目更新 `pipeline` 时应保留所需的全部绑定。

客户端连接示例：

```yaml
# client.yml；token 替换为管理员签发的用户 token
server: http://127.0.0.1:8787
token: USER_TOKEN
timeout: 30s
```

```bash
chmod 600 ./client.yml
mybuilds --config ./client.yml status --json
```

客户端支持 `--server-url`、`--timeout`、`--ca-file` 覆盖连接设置。也可使用 `MYBUILDS_SERVER_URL`、`MYBUILDS_CLIENT_TOKEN`、`MYBUILDS_CLIENT_TIMEOUT`、`MYBUILDS_CA_FILE`；优先级为显式参数、环境变量、配置文件、默认值。没有明文 `--token` 参数。跨主机连接需使用验证证书的 HTTPS。

## 身份与管理方式

| 身份 | 允许的操作 |
|---|---|
| `admin` | 项目、分组、节点、保留策略等管理；构建触发、查询、取消、审批及发布管理 |
| `trigger` | 查询 `status`，触发或重试普通构建；不能读取构建详情、日志或制品，不能批准发布 |
| `approver` | 查询构建、日志、制品、审批、发布与 Webhook 记录，批准或拒绝审批；不能触发构建或修改项目设置 |
| Agent token | 仅用于节点协议，不能代替上述用户 token |

含发布定义的触发与重试需 `admin` 加 `--allow-upload`。审批批准和发布授权是两个独立条件。

**`mybuilds-server` 的数据库管理命令用于服务未运行时。** 包括只读列表在内，它们都取得与 `serve` 相同的独占权；服务运行时会被拒绝。在线项目、分组与节点管理使用 `mybuilds` 加管理员连接配置。用户 token 的 CLI 管理入口目前只在 `mybuilds-server token` 下。

## 初始化与启动服务

以下在控制端机器执行；首次部署前先准备 `server.yml`。已有数据库升级前先停服务并备份。

```bash
mybuilds-server --config ./server.yml migrate
mybuilds-server --config ./server.yml token create --role admin --json
mybuilds-server --config ./server.yml node create local-builder --capacity 1 --json
mybuilds-server --config ./server.yml serve
```

用户 token 与节点 token 是不同凭据，创建时只显示一次，分别保存到客户端和 Agent 的私有配置。服务运行后，在另一个终端启动已配置的 Agent：

```bash
mybuilds-agent --config ./agent.yml serve
```

| 命令 | 作用与主要参数 |
|---|---|
| `mybuilds-server migrate` | 取得独占并初始化或迁移数据库 |
| `mybuilds-server serve` | 启动控制端；`--listen`、`--data-dir`、`--concurrency` 覆盖对应设置 |
| `mybuilds-server token create` | `--role admin/trigger/approver` 创建用户 token；可用 `--json` |
| `mybuilds-server token ls` | 查询 token 元数据，不返回明文；支持分页与 `--json` |
| `mybuilds-server token revoke TOKEN_ID` | 撤销指定用户 token |
| `mybuilds-agent serve` | 读取 Agent 配置，连接控制端并领取任务 |
| `mybuilds-agent doctor` | 检查本机工具与节点记录；`--data-dir` 默认 `~/.mybuilds/agent`，可用 `--json`；不读取 `--config` |

同机部署也需要单独登记并运行 Agent，控制端不会执行构建。`local-builder` 只是示例名称，不会自动创建或成为默认节点。项目的 `--nodes` 授权可用节点；无 `runner` 的流水线还需显式设置 `--default-node local-builder`。

## 本地配置、诊断与执行

以下本地命令不需要控制端或用户 token。

| `mybuilds` 命令 | 作用与主要参数 |
|---|---|
| `init` | 创建最小 `mybuilds.yml`，已有文件时拒绝覆盖 |
| `init --framework native --platform android` | 原生 Android 模板；iOS 改为 `--platform ios` |
| `init --framework flutter --platform android,ios` | Flutter 双平台模板，也可仅选 Android 或 iOS |
| `init --template PATH` | 校验并复制本地模板，与框架/平台选项互斥 |
| `run` | 执行流水线；`--file` 指定配置，`--build NAME[,NAME]` 或 `--all` 选择构建 |
| `run --dry-run` | 校验并预览，不执行步骤或检查实际 SDK |
| `doctor` | 默认检查当前目录的原生 Android 环境；用 `--framework`、`--platform`、`--working-dir` 改目标 |
| `doctor --target google-play` / `doctor --target app-store` | 检查本机发布环境；结合 `--agent-config`、`--app-id`、`--credentials-env` 等选项 |

`run --param key=value` 可重复传入；`--param android:key=value` 只覆盖指定构建的参数。`--step NAME` 只适用于选中一个构建的情况。本地执行的日志写入 stderr，结果 JSON 写入 stdout；有效 `upload` 步骤只能在远程发布流程执行。

```bash
mybuilds doctor --framework flutter --platform android --working-dir ./app
mybuilds run --file ./app/mybuilds.yml --build android --param version=1.2.3 --dry-run
```

Android 签名检查使用 `--keystore`、`--key-alias` 及密码环境变量名；iOS 使用 `--p12`、`--profile`、`--password-env`、`--bundle-id`、`--export-method`。这里的 `--profile` 是签名描述文件，不是控制端构建方案。完整材料要求见各平台接入指南。

## 项目、分组与构建方案

下表为客户端远程命令，均需 `admin`。控制端离线管理对应使用 `mybuilds-server`，其中 `project init` 改名为 `project add`，其余命令层级相同。

| `mybuilds` 命令 | 作用与主要参数 |
|---|---|
| `project init PROJECT` | 登记仓库；必填 `--repo`、`--nodes`，可选 `--group`、`--branches`、`--default-node`、`--build-number-start`、`--provider` |
| `project set PROJECT --settings PATH` | 导入需要更新的项目设置块 |
| `project ls` | 列出项目，支持 `--group`、分页、`--json` |
| `project move PROJECT --group GROUP` | 调整归属，保留项目历史、配置和构建号 |
| `project rm PROJECT` | 删除符合条件的项目；活动、待审批或结果未知保护可能阻止删除 |
| `group create GROUP` | 创建项目组 |
| `group ls` | 列出项目组，支持分页、`--json` |
| `group rename GROUP --name NEW_NAME` | 修改普通组名 |
| `group rm GROUP` | 删除空组；`default` 不可删除 |

仓库无 YAML 时，可绑定内置方案：

```bash
mybuilds --config ./client.yml project init mobile \
  --repo git@github.com:your-org/your-app.git \
  --nodes local-builder --default-node local-builder \
  --framework native --platform android
```

`--framework/--platform` 使用 `auto` 来源：优先读仓库配置，仅缺文件时用绑定方案。`--file mybuilds.yml` 选择 `repo`；强制方案来源 `profile` 或自定义方案通过 `--settings` 导入，见 [README 的接入说明](../README.md#接入项目)。命令帮助中的 `--poll`、`--schedule` 尚未实现，不能用于配置自动触发。

## 节点与远程诊断

除 `status` 外，以下客户端命令需 `admin`。`node` 及其子命令也有同层级的 `mybuilds-server node` 离线入口。

| `mybuilds` 命令 | 作用与主要参数 |
|---|---|
| `node create NODE` | 登记节点并返回独立 token；`--labels`、`--capacity`，容量默认 1，可用 `--json` |
| `node ls` / `node show NODE` | 查看状态、平台、容量与健康信息 |
| `node drain NODE` | 停止领取新任务，原任务继续 |
| `node enable NODE` | 启用节点，不解除停止未确认等保护 |
| `node disable NODE` | 停用节点，撤销现有执行授权 |
| `node rm NODE` | 删除符合条件的节点；活动或待审批构建会阻止删除 |
| `node token rotate NODE` | 更换凭据，返回一次新 token，需同步更新 Agent 配置 |
| `node token revoke NODE` | 撤销节点凭据 |
| `doctor --server` | 检查控制端 |
| `doctor --node NODE` | 查看节点最近实际诊断报告，不是即时远程执行 |
| `status` | 查看控制端状态；三种用户角色均可查询 |

节点停用、token 撤销或轮换会撤销原执行授权，不能当作无影响的维护操作；仅暂停接单时使用 `drain`。

## 构建、日志、制品与报告

| `mybuilds` 命令 | 作用与主要参数 |
|---|---|
| `trigger PROJECT` | 固定提交并排队；`--branch` 默认 `main`，`--ref` 指定该授权分支可达的完整 SHA；`--build` / `--all` 选择构建 |
| `build ls` | 按 `--project`、`--group`、`--build-name`、`--batch`、`--status` 筛选，支持分页 |
| `build show BUILD_ID` | 查看提交、步骤、节点、预算、报告摘要及发布关联 |
| `build cancel BUILD_ID` | 保存取消意图；需管理员，实际停止后才确认 |
| `build retry BUILD_ID --idempotency-key KEY` | 按原 SHA、配置、参数与条件新建构建，分配新编号；不修改原记录 |
| `build confirm-stopped BUILD_ID` | 管理员凭实际证据确认原执行已停止，需原 `--node-id`、`--session`、`--attempt`、`--lease`、`--epoch`、`--note` |
| `logs BUILD_ID` | 历史日志；`--step` 筛选，`--after-seq` 续读，`--follow` 跟随，`--stream-timeout` 限制流时长 |
| `artifact ls BUILD_ID` | 列出制品及原始报告文件，支持分页与 `--json` |
| `artifact download ARTIFACT_ID --output PATH` | 下载并校验 SHA-256，目标已存在时拒绝覆盖 |

`trigger` 支持 `--param key=value`、`--param build:key=value`，以及共享参数简写 `--version`、`--channel`。`--build` 与 `--all` 互斥。无合格 Agent 时保持排队；同项目同名构建在远程串行。

```bash
mybuilds --config ./client.yml trigger mobile --build android --version 1.2.3 --json
mybuilds --config ./client.yml build show BUILD_ID --json
mybuilds --config ./client.yml logs BUILD_ID --follow
mybuilds --config ./client.yml artifact ls BUILD_ID --json
mybuilds --config ./client.yml artifact download ARTIFACT_ID --output ./app.apk
```

从触发结果 `builds[].id` 获取 `BUILD_ID`，从制品列表获取 `ARTIFACT_ID`。没有独立的 `report` 命令：报告摘要在构建详情，原 XML 沿制品下载入口获取。

`trigger` 会输出本次 `request_key`，也可显式提供 `--idempotency-key`；请求结果不确定时，重发相同内容与相同 key。`retry` 必须显式传 key，不能替换原分支、配置或参数。停止确认不等于发布结果确认；iOS 资源清理确认需实际核实后再用 `--ios-cleanup-confirmed`。

## 审批、应用绑定与发布核对

| `mybuilds` 命令 | 作用与主要参数 |
|---|---|
| `approvals` | 查询审批记录；`--state`、`--project-id`、`--limit`、`--offset`、`--json` |
| `approve BUILD_ID` / `reject BUILD_ID` | 决定精确审批；需 `--approval-id`、`--revision`、`--checkpoint-digest`，可附 `--note` |
| `project app bind PROJECT` | 绑定实际应用；`--store`、`--node`、`--app-id`，商店凭据使用 `--credentials-env` |
| `project app ls PROJECT` | 查询应用绑定 |
| `project app doctor BINDING_ID` | 请求指定绑定的只读诊断 |
| `publish ls` | 查询发布记录；`--project`、`--limit`、`--after`、`--json` |
| `publish show INTENT_ID` | 查看原发布动作与保护状态 |
| `publish query INTENT_ID` | 请求原节点核对远端结果，不重新上传 |
| `publish query-show QUERY_ID` | 查看上述查询结果 |
| `publish confirm INTENT_ID --decision-file PATH` | 管理员根据真实外部证据确认原动作；文件需为本人所有的 0600 普通 JSON |

Google Play 绑定另需上传证书摘要 `--upload-cert-sha256`，可用 `--track` 限定轨道；custom 绑定使用 `--verification-file` 提供私有归属证据。字段和完整案例见 [Google Play](../specs/010-google-play/quickstart.md)、[App Store](../specs/011-app-store/quickstart.md)、[自定义发布](../examples/custom/README.md)。

审批参数来自当前 `approvals --json` 的记录，不能复用旧审批标识：

```bash
mybuilds --config ./client.yml approvals --state pending --json
mybuilds --config ./client.yml approve BUILD_ID \
  --approval-id APPROVAL_ID --revision REVISION \
  --checkpoint-digest CHECKPOINT_DIGEST --note "已核对制品与测试报告"
```

`REVISION` 替换为实际整数，其他大写占位替换为同一记录的标识或摘要。发布由流水线 `upload` 步骤执行，`publish` 命令用于查询和核对。上传、审核提交与公开上架分别记录；未知结果不会自动重发。

## Webhook 与历史清理

| `mybuilds` 命令 | 作用与主要参数 |
|---|---|
| `project hook show PROJECT` | 查看 Webhook 配置与状态 |
| `project hook enable PROJECT` / `project hook disable PROJECT` | 启用或停用已配置的入口 |
| `project hook rotate PROJECT --json` | 轮换独立 Webhook 密钥，明文只返回一次 |
| `project hook events PROJECT` | 查看投递记录，支持分页、`--json` |
| `project hook windows PROJECT` | 查看合并窗口与触发结果，支持分页、`--json` |
| `retention show PROJECT` | 查看有效保留数量、天数与继承来源 |
| `retention ls PROJECT` | 查看清理事项；加 `--candidates` 只评估候选和保护原因 |
| `retention run PROJECT` | 推进实际清理；`--limit` 为本轮上限，默认 100，最大 100 |

Webhook 也提供同层级的 `mybuilds-server project hook` 离线入口。首次启用需在项目设置中声明来源、仓库身份与自动构建范围；快捷注册可用 `--hook --hook-repository-key VALUE --json`。完整设置见[使用指南](USAGE.md#审批与webhook)。

保留数量和天数通过 `project set --settings` 更新，`retention show` 不修改设置。清理会保护活动、待审批、停止未确认或发布结果未知的记录；节点离线时节点清理可能仍待完成。

## 查询输出与占位符

多数查询支持 `--json`，常规列表使用 `--limit` / `--offset`；发布列表使用 `--after` 游标。日志流使用 `--follow`，其 JSON 选项仅用于历史查询。具体上限和组合以各命令帮助为准。

`PROJECT`、`GROUP`、`NODE` 是已登记名称；`BUILD_ID`、`ARTIFACT_ID`、`BINDING_ID`、`INTENT_ID`、`QUERY_ID`、`TOKEN_ID` 使用对应响应中的真实 ID。`approvals --project-id` 特别要求项目 UUID，不是项目名称。`PATH` 是用户准备的路径，`KEY` 是同一次请求可重用的非机密唯一标识；不同请求不要复用。

```bash
mybuilds project app bind --help
mybuilds build retry --help
mybuilds-server token create --help
mybuilds-agent serve --help
```
