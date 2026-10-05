# mybuilds

面向原生 Android/iOS 与 Flutter 工程的构建发布工具，用 Go 实现。
目标架构是客户端 `mybuilds`、控制端 `mybuilds-server` 与构建节点 `mybuilds-agent` 三种 CLI。
一个控制端管理多个构建节点，按平台、标签和容量分配任务；客户端也支持本地调试流水线。

本文是整个项目的概览入口。产品与技术决策见 [产品计划](docs/plans/PLAN.md)，功能顺序见 [实施路线](docs/plans/SPECKIT_ROADMAP.md)。

## 导航

- [当前状态](#当前状态)
- [开发与运行](#开发与运行)
- [控制端与远程排队](#控制端与远程排队)
- [独立节点、日志与制品](#独立节点日志与制品)
- [目录结构](#目录结构)
- [多节点目标](#多节点目标)
- [技术方向](#技术方向)
- [验证与版本注入](#验证与版本注入)
- [使用 Spec Kit 开发](#使用-spec-kit-开发)

## 当前状态

MVP有16个功能模块，目前15个已经完成代码与必要自动检查并本地提交；Apple/Flutter合法签名和真实商店发布由用户集中人工验收。实现顺序、精确提交与验证记录见[实施历史](docs/IMPLEMENTATION_HISTORY.md)，不把代码交付等同于全部人工验收通过。

| 能力 | 状态与验证入口 |
|---|---|
| 001–003 配置预览、shell执行、产物快照 | 已完成；单YAML多build、scoped参数、when、累计超时、post与脱敏日志 |
| 004 Android | 已完成实际APK/AAB签名与版本验证；[指南](specs/004-android-build/quickstart.md) |
| 005 iOS、009 Flutter | 代码与自动检查完成；临时签名资源、可编辑双平台模板与框架调度；[iOS指南](specs/005-ios-build/quickstart.md)、[Flutter工程](examples/flutter/README.md)；合法IPA等由用户人工验收 |
| 006–008 控制端、节点、恢复 | 已完成双库与macOS/Linux实际程序验收；项目组、多节点授权/调度、原SHA入队、日志/下载、恢复与显式retry |
| 019 测试报告、020 保留 | 已完成实际联验；JUnit原XML解析/封存与下载、受保护的中央及原节点清理；[报告记录](specs/019-test-reports/validation.md)、[清理记录](specs/020-project-retention/validation.md) |
| 010/011 Google Play/App Store | 代码与必要自动检查完成；共同发布授权、工具、应用保护、精确GET核对与CLI；[Google记录](specs/010-google-play/validation.md)、[Apple记录](specs/011-app-store/validation.md)；真实商店人工待验 |
| 012 可复用方案/custom | 代码与必要自动检查完成；repo/auto/profile完整来源选择、四内置方案、命名参数与原快照重试、用户argv发布和metadata查询；[指南](specs/012-custom-workflows/quickstart.md)、[脚本案例](examples/custom/README.md) |
| 014 审批 | 代码与必要自动检查完成；精确CLI决定、释放容量与原节点续执行、本地TTY；[指南](specs/014-release-approval/quickstart.md) |
| 015 Webhook | 源码已集成，最终双库联合案例与整功能提交正在完成 |

[Flutter集中验收案例](examples/mvp/acceptance.md)从实际CLI模板生成一个仓库的Android/iOS配置、控制端和两个节点设置，串联测试、下载、审批与商店分发。当前生成/dry-run已通过，015最终提交正在完成，双平台合法签名与真实商店由用户人工验收。通知013、轮询/cron016、额外渠道017与部署打磨018属于后续功能。

## 开发与运行

要求 Go **1.25 或更新版本**、Git。首次下载 Go 依赖需要网络；已使用 Cobra、YAML v3、doublestar/v4，以及管理配置的 Viper 1.21.0、数据库访问的 GORM 1.31.2 与 SQLite/PostgreSQL 双驱动。SQLite 引擎锁定 modernc.org/sqlite 1.55.0（实际 SQLite 3.53.3），包含 WAL 修补；依赖版本见 [go.mod](go.mod) 和 [go.sum](go.sum)。
帮助、版本、init 和 dry-run 无需移动工具链。Android doctor 要求 Java 17+、已有 Android SDK 和工程内 Gradle wrapper；完整构建以工程实际要求为准，不自动安装 SDK。新增签名行为测试需要 JDK/keytool。
iOS签名要求macOS15+、现代Xcode/iOS SDK以及Darwin+cgo构建；不自动下载或管理签名材料。非macOS/无cgo仍能init和dry-run，实际签名明确未支持。
本地执行和控制端支持 macOS/Linux；本地脚本需要 sh 或所选 bash，Git 快照需要 Git。Windows 客户端可纯 Go 编译和调用远程 API，本地执行与本机控制端明确未支持。

在项目根目录运行：

```bash
go mod download
go run ./cmd/mybuilds --help
go run ./cmd/mybuilds version
go run ./cmd/mybuilds-server --help
go run ./cmd/mybuilds-server version
# 007已验收的Agent入口
go run ./cmd/mybuilds-agent --help
go run ./cmd/mybuilds-agent version
go run ./cmd/mybuilds-agent doctor --json
go run ./cmd/mybuilds run --file examples/pipeline-preview.yml --all --dry-run
```

三个版本命令默认输出相同：

```text
dev (commit: unknown, built: unknown)
```

构建本机二进制：

```bash
mkdir -p bin
go build -o bin/mybuilds ./cmd/mybuilds
go build -o bin/mybuilds-server ./cmd/mybuilds-server
go build -o bin/mybuilds-agent ./cmd/mybuilds-agent
./bin/mybuilds --help
./bin/mybuilds-server version
```

`bin/` 不进入 Git。未指定远端仓库，Go 模块名暂为 `mybuilds`。
无参数运行显示帮助；未知子命令或 `version` 多余参数返回非零退出码。

在目标仓库运行 `mybuilds init` 创建最小 `mybuilds.yml`，已有目标拒绝覆盖；`init --template ./ci/template.yml` 使用经校验的本地模板。
`mybuilds init --platform android` 默认使用 native 框架，等价于 `--framework native --platform android`，生成一个 `builds.android`。模板默认 app/release，构建 APK/AAB/mapping；工程须显式读取 `APP_VERSION/BUILD_NUMBER` 和签名环境变量，release 须开启 R8。参数默认 1.0.0/1，覆盖使用 `--param version=1.2.3 --param build_number=42`；任务与路径均可编辑。
`mybuilds doctor --platform android --json --working-dir <工程目录>` 检查实际 Java、SDK 包和 wrapper。签名检查须显式提供 `--keystore`、`--key-alias`、`--store-password-env`、`--key-password-env`，两个密码仅从指定环境变量读取；未声明签名为 skipped，任一 failed 返回非零。doctor 默认 android；007 支持 `doctor --node NODE` 和 `doctor --server`；节点诊断读取最近实际报告，节点离线明确失败。首次 wrapper 检查可能下载工程锁定的 Gradle 到缓存，普通 run 不会自动调用 doctor。
`run --dry-run` 只输出脱敏 JSON，不执行脚本、Git 或网络请求，也不读取密钥。多 build 必须用 `--build android,ios` 或 `--all`，参数用重复的 `--param key=value`；`--step` 仅限单 build。
去掉 `--dry-run` 执行本地脚本：日志写 stderr，脱敏结果 JSON 写 stdout，失败/取消返回非零。配置路径不改变当前工作目录，多个 build 顺序执行；`--step` 只运行选中普通步骤，不自动执行前序依赖。
先校验整批再启动脚本；本地审批要求真实终端明确确认，远程审批释放容量后只在原节点续执行；通知仍明确未支持，`enabled: false`可关闭。本地生效upload在任何脚本前拒绝，商店分发使用远程控制端授权和节点实际工具。
artifact 支持相对根目录递归 glob（`**`）；每个模式须匹配普通文件，按步骤保存独立快照、大小和 SHA-256。结果 JSON 的 `result_dir` 定位临时结果根，`log_path` 定位 UTC 脱敏步骤日志，失败后仍保留完整证据。普通与 post 的快照分开；结果数据不进入源码工作树。预览、预检查失败、全部跳过不创建结果目录。
可运行的本地例子见 [local-run.yml](examples/local-run.yml) 和 [local-artifacts.yml](examples/local-artifacts.yml)，在临时目录以已构建二进制的绝对路径和 `--file` 指向该例子运行。

JUnit 本地示例见 [local-reports.yml](examples/local-reports.yml)：在独立临时工作目录运行 `mybuilds run --file <示例绝对路径> --build junit`。示例会生成失败报告并返回非零，结果只统计同路径最后一次普通执行生成的5个case；failure/always仍运行，post改写原文件不改变封存结果。实际配置使用 `reports.junit.paths: [results/*.xml]`，`required` 默认true；false只允许缺失，非法XML或测试失败仍失败。开始前的旧XML不计入，报告耗时计入普通构建预算。
结果JSON包含报告计数、有限诊断、安全相对路径和原XML的大小/SHA-256；独立快照和manifest位于结果目录。未配置报告或全部跳过不生成报告结果。远程构建也使用同一检查流程，最终XML完整上传并经服务端重新解析后才封存。`build show`显示已封存计数、来源和摘要，`artifact ls`列出junit用途，`artifact download`下载中央确认的原字节，节点离线仍可用；admin/approver可读，trigger/node身份不可读取用户报告。019最终故障验收已通过，见[验证记录](specs/019-test-reports/validation.md)。
报告路径模板支持参数及project、build.name/build.id/build.number、node.name、git.sha/git.branch；本地运行须有相应事实，缺失在执行前报错。workspace、step.name属于私有步骤上下文，不能用于报告路径。
原生iOS使用`mybuilds init --framework native --platform ios`（当前目录，已有配置不覆盖）；五个必填参数为xcode_project、scheme、bundle_id、version、build_number，export_method默认debugging。IOS_P12_FILE/IOS_PROFILE_FILE/IOS_P12_PASSWORD仅通过ios_signing完整环境引用声明，预览不读取这些值。实际执行用单一app/profile、临时keychain、排他profile副本与工作树外的DerivedData/archive/export目录；普通步骤及用户post结束后系统独立Close，清理不确定保留原原因并闭锁，不修改用户default/search list。可编辑模板见[native-ios.yml](examples/native-ios.yml)，完整预览、实际命令和人工IPA/dSYM核验见[005指南](specs/005-ios-build/quickstart.md)。

Android 工程接入、临时测试签名和构建命令见 [Android 示例](examples/android/README.md)；完整包核验见 [004 验收指南](specs/004-android-build/quickstart.md)。

## 控制端与远程排队

控制端读取已登记仓库的固定提交，校验参数、条件与权限，再原子创建 queued/skipped 记录。所选 build 共享提交 SHA、按项目统一分配编号；skipped 不占号。Agent 只领取授权范围内、平台/标签匹配且容量可用的任务；无合格节点保持排队。

下面保留 006 的排队演示，不启动 Agent。先按上文构建客户端和控制端两个二进制；示例使用 OpenSSL 生成随机管理员 token，也可由密码管理器预先注入同名环境变量。token 通过环境传递，不写入命令参数。

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

这个演示没有登记并启动合格 Agent，结果保持 queued，示例的 `printf` 不会执行。完整节点执行见下节。`build show` 默认输出安全详情表格，`--json` 返回同一视图。参数值、脚本正文和凭据不公开，列表默认 20 条、最大 200 条，支持 limit/offset 及项目、组、build 名、批次和状态过滤。

`--settings ./settings.yml` 由客户端按当前目录读取内容；`pipeline.file` 和注册时的 `--file ci/mybuilds.yml` 都是仓库相对路径，`--file` 与 `--settings` 互斥。`project set mobile --settings ./settings.yml` 替换设置；项目支持framework/platform与profile绑定；Webhook管理见015指南。poll/schedule属于后续016。

触发可用 `--build android,ios` 或 `--all`。共享 `--param key=value` 应用到所有所选 build，`--param android:key=value` 只覆盖指定 build；命名值优先，同 scope 重复参数拒绝。`--version`、`--channel` 是共享参数快捷选项。客户端每次新请求生成一个随机 key，在发起网络请求前写到 stderr，成功 JSON 含 `request_key`；网络失败后用原 `--idempotency-key` 和相同参数恢复，重放返回原 SHA/编号/结果，不自动重试或换 key。

管理命令如下，列表支持 `--json`：

| 命令端 | 当前命令 |
|---|---|
| 客户端 | group create/ls/rename/rm；project init/set/ls/move/rm/app；trigger；build ls/show/cancel/confirm-stopped/retry；node；logs；artifact ls/download；approvals/approve/reject；publish；status；doctor |
| 服务端本机 | serve/migrate；group create/ls/rename/rm；project add/set/ls/move/rm；token create/ls/revoke |

serve 在线时同一数据库被独占，本机 migrate、project/group/token 管理会拒绝，使用客户端远程管理或鉴权 HTTP API。停止示例控制端后，可创建身份并查看安全列表：

```bash
"$server_bin" --config "$work_dir/server.yml" token create --role trigger --json
"$server_bin" --config "$work_dir/server.yml" token ls --json
```

token create 只显示一次明文，列表不显示 token；首次管理员通过 `MYBUILDS_BOOTSTRAP_ADMIN_TOKEN` 初始化，撤销全部身份后重启不会重新创建 bootstrap 身份。

角色为 admin/trigger/approver：admin 管理与读写；trigger 只读 status 并触发所选定义不含 upload 的构建；approver可读脱敏证据并明确批准或拒绝精确审批检查点。所选定义含 upload 时，即使条件为 false，也要求 admin 与显式 `--allow-upload`；节点执行仍须当前应用、报告和审批授权。

配置默认位于 `~/.mybuilds/server.yml`、`~/.mybuilds/client.yml`。覆盖顺序为默认值 < 文件 < 明确白名单环境变量 < 显式 CLI；服务端白名单为 `MYBUILDS_LISTEN`、`MYBUILDS_DATA_DIR`、`MYBUILDS_CONCURRENCY`、`MYBUILDS_DATABASE_DRIVER`、`MYBUILDS_DATABASE_DSN`、`MYBUILDS_SECRETS_FILE`；客户端为 `MYBUILDS_SERVER_URL`、`MYBUILDS_CLIENT_TOKEN`、`MYBUILDS_CLIENT_TIMEOUT`、`MYBUILDS_CA_FILE`。相对服务端路径基于配置文件目录，data_dir 创建为或要求 0700；客户端 token 可以是 0600 文件中的字面量或完整 `${NAME}` 引用，空/弱 token 拒绝。PostgreSQL 可通过 database.driver 和私有环境中的 MYBUILDS_DATABASE_DSN 配置；切换数据库不迁移已有数据。PostgreSQL 只使用明确 DSN，拒绝非空宿主 PG 环境变量和隐式 service/passfile；显式 TLS 材料需受限普通文件，读入内存后验证，不继承宿主凭据。

客户端只有远程命令读取 client 配置；损坏的 client.yml 或缺 token 不影响本地 init/run/doctor/help/version。`--server-url` 覆盖远程地址，`--timeout` 只控制普通 API 请求，不改变本地 YAML 的 build/post 预算。客户端仅允许回环 HTTP 或验证证书的 HTTPS，并拒绝重定向。当前服务端提供 HTTP 监听，可放在终止 TLS 的反向代理后；跨主机客户端必须通过 HTTPS 入口连接并验证服务器证书，不能跳过校验。

实际 API、配置与验收步骤见 [006 契约](specs/006-control-plane/contracts/http.md)、[配置/CLI](specs/006-control-plane/contracts/config-cli.md) 和 [验收指南](specs/006-control-plane/quickstart.md)。

## 独立节点、日志与制品

以下命令使用已验收的 007 功能。控制端提供 HTTP 监听；跨主机部署须先配置 HTTPS 反向代理，把流量转发到控制端。客户端和 Agent 验证证书与主机名，私有 CA 用 `ca_file` 或客户端 `--ca-file` 指定，不修改系统信任，不支持跳过 TLS 校验。代理应支持 SSE 流式转发并保留上传/下载的独立时限。

管理员使用上节的私有客户端配置登记节点；节点 token 与用户 token 分开，创建/轮换只返回一次明文。不要把 token 放在 argv 或普通日志中：

```bash
umask 077
./bin/mybuilds --config ./client.yml node create mac-android-a --labels android --capacity 1 --json > ./mac-android-a.private.json
./bin/mybuilds --config ./client.yml node create mac-android-b --labels android --capacity 1 --json > ./mac-android-b.private.json
./bin/mybuilds --config ./client.yml node create linux-generic --labels generic --capacity 1 --json > ./linux-generic.private.json
./bin/mybuilds --config ./client.yml node ls --json
```

在每个节点分别保存自己的 `agent.yml` 为 0600，并从安全传输或密码管理器注入 `MYBUILDS_AGENT_TOKEN`。节点名、token 和 data_dir 不共用；控制端与 Agent 的 heartbeat/lease 策略必须一致，默认分别 5s/30s：

```yaml
server: https://builds.example.test
node: mac-android-a
token: '${MYBUILDS_AGENT_TOKEN}'
capacity: 1
data_dir: ./agent-data
ca_file: ./control-ca.pem
heartbeat_interval: 5s
lease_duration: 30s
# secrets_file: ./secrets.env
```

`server` 替换为实际 HTTPS 入口，证书 SAN 必须匹配该主机；`control-ca.pem` 替换为实际 CA 文件。相对路径基于 agent.yml 所在目录。data_dir 必须是节点用户自有的 0700 目录，首次启动可创建；secrets_file 必须是自有 0600 普通文件，只向实际声明的步骤注入对应秘密。私有 SSH 仓库分别给控制端和节点配置显式 key/known_hosts，不使用宿主未知私钥或 SSH agent。

```bash
chmod 0600 ./agent.yml
./bin/mybuilds-agent doctor --data-dir ./agent-data --json
./bin/mybuilds-agent --config ./agent.yml serve
```

`mybuilds-agent doctor` 只检查本机工具和记录，不加载连接配置、token，也不访问控制端。Linux 缺少 Xcode/Android 工具时报告实际失败或跳过，不能把通用 shell 能力当成 Android/iOS 能力。客户端本地 init/run/doctor/help/version 也不加载远程配置。

项目必须授权目标节点。无 runner 的通用 build 只使用明确 default_node；Android runner 还要求节点的实际 Android 检查通过。同项目同名 build 跨节点串行，全局/节点容量同时限制领取，容量实际取管理员配置与节点配置的较小值。完整配置、可执行演示与三节点验收见 [007 快速指南](specs/007-node-agents/quickstart.md)。

```bash
# 用项目实际仓库路径替换 /path/to/repository，所有节点须能读取相同固定提交。
./bin/mybuilds --config ./client.yml project init demo --repo /path/to/repository \
  --nodes mac-android-a,mac-android-b,linux-generic --default-node linux-generic
./bin/mybuilds --config ./client.yml trigger demo --build generic --idempotency-key demo-generic-1 --json

# 从 trigger 的 builds[].id 取构建 ID；从 artifact ls 的 items[].id 取产物 ID。
./bin/mybuilds --config ./client.yml build show "$BUILD_ID" --json
./bin/mybuilds --config ./client.yml logs "$BUILD_ID" --json
./bin/mybuilds --config ./client.yml logs "$BUILD_ID" --follow --stream-timeout 15m
./bin/mybuilds --config ./client.yml artifact ls "$BUILD_ID" --json
mkdir -m 0700 ./downloads
./bin/mybuilds --config ./client.yml artifact download "$ARTIFACT_ID" --output ./downloads/output.bin
./bin/mybuilds --config ./client.yml doctor --node mac-android-a --json
./bin/mybuilds --config ./client.yml doctor --server --json
```

日志仅返回中央已确认的脱敏记录；历史 JSON 的 `next_seq` 可用于 `--after-seq` 续读，SSE 按确认 chunk 重连去重并在终态结束。普通 API `--timeout` 不替代 `--stream-timeout`；控制端 SSE 每条连接最多 15 分钟，下载独立总预算 10 分钟。下载校验大小与 SHA-256 后发布到自有私有目录，拒绝覆盖已有文件；Agent 离线后中央已确认日志/制品仍可读取。

```bash
./bin/mybuilds --config ./client.yml build cancel "$BUILD_ID" --json
./bin/mybuilds --config ./client.yml node drain mac-android-a
./bin/mybuilds --config ./client.yml node enable mac-android-a
./bin/mybuilds --config ./client.yml node disable mac-android-a
./bin/mybuilds --config ./client.yml node token rotate mac-android-a --json > ./mac-android-a.rotated.private.json
./bin/mybuilds --config ./client.yml node token revoke mac-android-a
```

取消运行中构建先进入 cancel_requested，真实进程组回收后才确认停止；仍有有效执行权时才运行相应 post。drain 停新领取但原任务继续续租，disable/revoke/轮换立即撤销原执行权，不能再启动 always。失联/过期不迁移任务；停止未确认的 interrupted 保留互斥和节点隔离。enable 不解除此保护，旧 journal 重启不会重放用户步骤。

管理员只有在实际核实原执行进程及整个进程组已停止后才能确认。用 `build show --json` 中原 `node_id/session_id/attempt_id/lease_id/lease_epoch` 填入变量，不能用节点名字或新租约代替；确认保留原终态和原因，只解除停止保护：

```bash
./bin/mybuilds --config ./client.yml build confirm-stopped "$BUILD_ID" \
  --node-id "$NODE_ID" --session "$SESSION_ID" --attempt "$ATTEMPT_ID" \
  --lease "$LEASE_ID" --epoch "$LEASE_EPOCH" --note '已核实原PID和整个PGID均不存在'
```

admin 管理并读取证据；approver 可读脱敏 build/log/artifact，但不管理、不触发、不取消；trigger 只能读 status 和允许的触发入口。节点 token 只用于节点协议。以上007验收仅覆盖节点执行；当前iOS、发布与审批进度见本文当前状态，Android签名不代替合法Apple签名验收。

## 重启与原快照重试

控制端在监听前核对持久任务，最多30秒；失败安全退出。合法queued保持原号、原SHA和原配置，有效执行保持原完整租约及预算，过期执行沿原interrupted/停止保护规则处理。短网络中断仅在原执行权限期限内继续，心跳不延长执行权；未知claim仍保留证据并阻止新进程接管。

已停止的成功、失败、取消或中断构建可显式创建新构建。将原构建ID填入 `ORIGINAL_ID`，key为本次非机密请求标识：

```bash
./bin/mybuilds --config ./client.yml build retry "$ORIGINAL_ID" \
  --idempotency-key retry-demo-1 --json
```

retry必须明确提供key，响应丢失后使用同ID、同key和同 `--allow-upload` 输入重发；客户端不自动重发。原SHA、定义、参数和when事实保持，新的编号与attempt从原定义完整预算开始，当前节点授权只收窄。不能用retry替换分支或参数，需另发trigger；活动/queued/skipped/停止未知拒绝。包含upload定义仍要求admin与明确 `--allow-upload`，实际分发进度见当前状态。列表和详情中的 `retry_of` 指向原构建，原证据保持。

终态响应丢失时，Agent重启先用当前独立节点凭据只读核对精确原回执。只有本地已知停止、完整终态/归属/序号/摘要/制品清单匹配中央才清该条journal；results/spool保持。其余未知journal仍阻止新session，不按旧PID发送信号或重放动作。文件必须是自有0600普通文件，最多128条、每条1MiB；链接、替换、损坏和未确认停止均保留。轮换后先把当前token更新到私有配置；新session仍遵守原注册窗口。

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
| `internal/scm` | 已接入只读 Git 固定提交与 SSH 显式凭据；Webhook 来源后续接入 |
| `internal/mobile` | 已接入 Android/Flutter 模板、真实受限doctor和纯参数检查；iOS签名生命周期由005提供 |
| `internal/distribute` | 锁定fastlane商店工具与具体单发动作；custom由012扩展同一授权/记录 |
| `internal/notify` | 后续013飞书等通知渠道，尚未创建 |
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

用户于2026-10-05调整交付方式：剩余模块先完成代码和必要自动验证，最后使用统一Flutter双平台案例集中人工验收；真实Apple签名、两大商店和外部Webhook成功门明确列为人工待验，不阻塞并行开发，也不冒称已通过。
