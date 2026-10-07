# 使用指南

基础安装与快速开始见 [README](../README.md)。本页介绍本地流水线、控制端初始化、多节点部署及常用管理命令；集中验收见 [MVP 验收指南](ACCEPTANCE.md)。

## 导航

首次单机部署见 [README](../README.md#首次部署控制端客户端与同机-agent)；按命令查找用法见[命令速查](CLI.md)。

- [本地配置与执行](#本地配置与执行)
- [控制端与远程排队](#控制端与远程排队)
- [独立节点、日志与制品](#独立节点日志与制品)
- [重启与原快照重试](#重启与原快照重试)
- [审批与 Webhook](#审批与webhook)

## 本地配置与执行

要求 Go **1.25 或更新版本**、Git。首次下载 Go 依赖需要网络；已使用 Cobra、YAML v3、doublestar/v4，以及管理配置的 Viper 1.21.0、数据库访问的 GORM 1.31.2 与 SQLite/PostgreSQL 双驱动。SQLite 引擎锁定 modernc.org/sqlite 1.55.0（实际 SQLite 3.53.3），包含 WAL 修补；依赖版本见 [go.mod](../go.mod) 和 [go.sum](../go.sum)。
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
# 独立构建节点入口
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
可运行的本地例子见 [local-run.yml](../examples/local-run.yml) 和 [local-artifacts.yml](../examples/local-artifacts.yml)，在临时目录以已构建二进制的绝对路径和 `--file` 指向该例子运行。

JUnit 本地示例见 [local-reports.yml](../examples/local-reports.yml)：在独立临时工作目录运行 `mybuilds run --file <示例绝对路径> --build junit`。示例会生成失败报告并返回非零，结果只统计同路径最后一次普通执行生成的5个case；failure/always仍运行，post改写原文件不改变封存结果。实际配置使用 `reports.junit.paths: [results/*.xml]`，`required` 默认true；false只允许缺失，非法XML或测试失败仍失败。开始前的旧XML不计入，报告耗时计入普通构建预算。
结果JSON包含报告计数、有限诊断、安全相对路径和原XML的大小/SHA-256；独立快照和manifest位于结果目录。未配置报告或全部跳过不生成报告结果。远程构建也使用同一检查流程，最终XML完整上传并经服务端重新解析后才封存。`build show`显示已封存计数、来源和摘要，`artifact ls`列出junit用途，`artifact download`下载中央确认的原字节，节点离线仍可用；admin/approver可读，trigger/node身份不可读取用户报告。019最终故障验收已通过，见[验证记录](../specs/019-test-reports/validation.md)。
报告路径模板支持参数及project、build.name/build.id/build.number、node.name、git.sha/git.branch；本地运行须有相应事实，缺失在执行前报错。workspace、step.name属于私有步骤上下文，不能用于报告路径。
原生iOS使用`mybuilds init --framework native --platform ios`（当前目录，已有配置不覆盖）；五个必填参数为xcode_project、scheme、bundle_id、version、build_number，export_method默认debugging。IOS_P12_FILE/IOS_PROFILE_FILE/IOS_P12_PASSWORD仅通过ios_signing完整环境引用声明，预览不读取这些值。实际执行用单一app/profile、临时keychain、排他profile副本与工作树外的DerivedData/archive/export目录；普通步骤及用户post结束后系统独立Close，清理不确定保留原原因并闭锁，不修改用户default/search list。可编辑模板见[native-ios.yml](../examples/native-ios.yml)，完整预览、实际命令和人工IPA/dSYM核验见[005指南](../specs/005-ios-build/quickstart.md)。

Android 工程接入、临时测试签名和构建命令见 [Android 示例](../examples/android/README.md)；完整包核验见 [004 验收指南](../specs/004-android-build/quickstart.md)。


## 控制端与远程排队

控制端读取已登记仓库的固定提交，校验参数、条件与权限，再原子创建 queued/skipped 记录。所选 build 共享提交 SHA、按项目统一分配编号；skipped 不占号。Agent 只领取授权范围内、平台/标签匹配且容量可用的任务；无合格节点保持排队。

下面演示控制端排队，不启动 Agent。先按上文构建客户端和控制端两个二进制；示例使用 OpenSSL 生成随机管理员 token，也可由密码管理器预先注入同名环境变量。token 通过环境传递，不写入命令参数。

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

这个演示没有登记并启动合格 Agent，结果保持 queued，示例的 `printf` 不会执行。完整节点执行见下一节。`build show` 默认输出安全详情表格，`--json` 返回同一视图。参数值、脚本正文和凭据不公开，列表默认 20 条、最大 200 条，支持 limit/offset 及项目、组、build 名、批次和状态过滤。

`--settings ./settings.yml` 由客户端按当前目录读取内容；`pipeline.file` 和注册时的 `--file ci/mybuilds.yml` 都是仓库相对路径，`--file` 与 `--settings` 互斥。`project set mobile --settings ./settings.yml` 替换设置；项目支持framework/platform与profile绑定；Webhook管理见015指南。poll/schedule属于后续016。

触发可用 `--build android,ios` 或 `--all`。共享 `--param key=value` 应用到所有所选 build，`--param android:key=value` 只覆盖指定 build；命名值优先，同 scope 重复参数拒绝。`--version`、`--channel` 是共享参数快捷选项。客户端每次新请求生成一个随机 key，在发起网络请求前写到 stderr，成功 JSON 含 `request_key`；网络失败后用原 `--idempotency-key` 和相同参数恢复，重放返回原 SHA/编号/结果，不自动重试或换 key。

管理命令如下，列表支持 `--json`：

| 命令端 | 当前命令 |
|---|---|
| 客户端 | group create/ls/rename/rm；project init/set/ls/move/rm/app/hook；trigger；build ls/show/cancel/confirm-stopped/retry；node；logs；artifact ls/download；approvals/approve/reject；publish；status；doctor |
| 服务端本机 | serve/migrate；group create/ls/rename/rm；project add/set/ls/move/rm/hook；token create/ls/revoke |

serve 在线时同一数据库被独占，本机 migrate、project/group/token 管理会拒绝，使用客户端远程管理或鉴权 HTTP API。停止示例控制端后，可创建身份并查看安全列表：

```bash
"$server_bin" --config "$work_dir/server.yml" token create --role trigger --json
"$server_bin" --config "$work_dir/server.yml" token ls --json
```

token create 只显示一次明文，列表不显示 token；首次管理员通过 `MYBUILDS_BOOTSTRAP_ADMIN_TOKEN` 初始化，撤销全部身份后重启不会重新创建 bootstrap 身份。

角色为 admin/trigger/approver：admin 管理与读写；trigger 只读 status 并触发所选定义不含 upload 的构建；approver可读脱敏证据并明确批准或拒绝精确审批检查点。所选定义含 upload 时，即使条件为 false，也要求 admin 与显式 `--allow-upload`；节点执行仍须当前应用、报告和审批授权。

配置默认位于 `~/.mybuilds/server.yml`、`~/.mybuilds/client.yml`。覆盖顺序为默认值 < 文件 < 明确白名单环境变量 < 显式 CLI；服务端白名单为 `MYBUILDS_LISTEN`、`MYBUILDS_DATA_DIR`、`MYBUILDS_CONCURRENCY`、`MYBUILDS_DATABASE_DRIVER`、`MYBUILDS_DATABASE_DSN`、`MYBUILDS_SECRETS_FILE`；客户端为 `MYBUILDS_SERVER_URL`、`MYBUILDS_CLIENT_TOKEN`、`MYBUILDS_CLIENT_TIMEOUT`、`MYBUILDS_CA_FILE`。相对服务端路径基于配置文件目录，data_dir 创建为或要求 0700；客户端 token 可以是 0600 文件中的字面量或完整 `${NAME}` 引用，空/弱 token 拒绝。PostgreSQL 可通过 database.driver 和私有环境中的 MYBUILDS_DATABASE_DSN 配置；切换数据库不迁移已有数据。PostgreSQL 只使用明确 DSN，拒绝非空宿主 PG 环境变量和隐式 service/passfile；显式 TLS 材料需受限普通文件，读入内存后验证，不继承宿主凭据。

客户端只有远程命令读取 client 配置；损坏的 client.yml 或缺 token 不影响本地 init/run/doctor/help/version。`--server-url` 覆盖远程地址，`--timeout` 只控制普通 API 请求，不改变本地 YAML 的 build/post 预算。客户端仅允许回环 HTTP 或验证证书的 HTTPS，并拒绝重定向。当前服务端提供 HTTP 监听，可放在终止 TLS 的反向代理后；跨主机客户端必须通过 HTTPS 入口连接并验证服务器证书，不能跳过校验。

实际 API、配置与验收步骤见 [006 契约](../specs/006-control-plane/contracts/http.md)、[配置/CLI](../specs/006-control-plane/contracts/config-cli.md) 和 [验收指南](../specs/006-control-plane/quickstart.md)。


## 独立节点、日志与制品

以下命令用于独立节点部署。控制端提供 HTTP 监听；跨主机部署须先配置 HTTPS 反向代理，把流量转发到控制端。客户端和 Agent 验证证书与主机名，私有 CA 用 `ca_file` 或客户端 `--ca-file` 指定，不修改系统信任，不支持跳过 TLS 校验。代理应支持 SSE 流式转发并保留上传/下载的独立时限。

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

项目必须授权目标节点。无 runner 的通用 build 只使用明确 default_node；Android runner 还要求节点的实际 Android 检查通过。同项目同名 build 跨节点串行，全局/节点容量同时限制领取，容量实际取管理员配置与节点配置的较小值。完整配置、可执行演示与三节点验收见 [007 快速指南](../specs/007-node-agents/quickstart.md)。

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

admin 管理并读取证据；approver 可读脱敏 build/log/artifact，但不管理、不触发、不取消；trigger 只能读 status 和允许的触发入口。节点 token 只用于节点协议。实际平台要求见 [README](../README.md#平台与限制)；Android 签名结果不能替代 iOS 签名或商店发布验证。


## 重启与原快照重试

控制端在监听前核对持久任务，最多30秒；失败安全退出。合法queued保持原号、原SHA和原配置，有效执行保持原完整租约及预算，过期执行沿原interrupted/停止保护规则处理。短网络中断仅在原执行权限期限内继续，心跳不延长执行权；未知claim仍保留证据并阻止新进程接管。

已停止的成功、失败、取消或中断构建可显式创建新构建。将原构建ID填入 `ORIGINAL_ID`，key为本次非机密请求标识：

```bash
./bin/mybuilds --config ./client.yml build retry "$ORIGINAL_ID" \
  --idempotency-key retry-demo-1 --json
```

retry必须明确提供key，响应丢失后使用同ID、同key和同 `--allow-upload` 输入重发；客户端不自动重发。原SHA、定义、参数和when事实保持，新的编号与attempt从原定义完整预算开始，当前节点授权只收窄。不能用retry替换分支或参数，需另发trigger；活动/queued/skipped/停止未知拒绝。包含upload定义仍要求admin与明确 `--allow-upload`；发布相关操作见本页的审批与 Webhook 说明以及渠道指南。列表和详情中的 `retry_of` 指向原构建，原证据保持。

终态响应丢失时，Agent重启先用当前独立节点凭据只读核对精确原回执。只有本地已知停止、完整终态/归属/序号/摘要/制品清单匹配中央才清该条journal；results/spool保持。其余未知journal仍阻止新session，不按旧PID发送信号或重放动作。文件必须是自有0600普通文件，最多128条、每条1MiB；链接、替换、损坏和未确认停止均保留。轮换后先把当前token更新到私有配置；新session仍遵守原注册窗口。


## 审批与Webhook

`approvals --project-id PROJECT_UUID --state pending --json` 读取安全检查点；`approve BUILD_ID --approval-id ID --revision N --checkpoint-digest SHA256 --json` 或 `reject`只决定该次审批。批准后原节点复核原工作区、制品和剩余预算，再续未执行步骤；等待不扣执行预算，不迁移或重跑。多审批各自决定，跳过审批不会授权上传。

项目settings中的`hook`与`triggers`启用自动触发，也可用`project hook enable/show/events/windows/disable/rotate PROJECT`管理；初始化/轮换秘密仅显示一次，使用`--json`保存私有文件。真实外部入口需验证证书的HTTPS；GitHub/GitLab/Gitee/generic仅可信普通分支push触发，payload URL不成为仓库来源。固定quiet_period不随后续push延长；手动构建豁免changes，自动构建使用冻结的变化事实。四个外部托管来源的真实投递按[人工指南](../specs/015-webhook-trigger/quickstart.md)验收。
