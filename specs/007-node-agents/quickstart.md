# 007 节点执行与验收指南

007 已实现并验收，接通独立节点、固定 SHA 执行、日志/SSE、中央制品和取消/停止保护。两个 macOS 节点与一个真实 Linux 节点闭环、SQLite/PostgreSQL 各 51 项实际应用检查、Android 签名构建号 101 的中央下载已通过；全量 test/race/vet、12 次三入口跨平台构建与本机 help/version 已通过，Linux 普通/always 真实取消强化与 Spec Kit 收敛也已通过，整功能本地提交见[实施历史](../../docs/IMPLEMENTATION_HISTORY.md)。005 的真实 Apple profile/archive/export 与整个 MVP 门未通过，不提供 iOS 签名、审批或发布执行。

## 准备与构建

要求 Go 1.25+、Git、sh；以下演示还用 OpenSSL 生成随机 token，用 Python 3 读取 CLI JSON。控制端和 Agent 支持 macOS/Linux；远程客户端可跨平台。先在项目根目录构建三个二进制：

```bash
project_dir="$(pwd)"
mkdir -p bin
go build -o bin/mybuilds ./cmd/mybuilds
go build -o bin/mybuilds-server ./cmd/mybuilds-server
go build -o bin/mybuilds-agent ./cmd/mybuilds-agent
client_bin="$project_dir/bin/mybuilds"
server_bin="$project_dir/bin/mybuilds-server"
agent_bin="$project_dir/bin/mybuilds-agent"
umask 077
FIXTURE="$(mktemp -d)"
chmod 0700 "$FIXTURE"
```

跨主机必须通过已配置的 HTTPS 反向代理。下面要求设置 `MYBUILDS_SERVER_URL` 为实际 HTTPS 入口、`MYBUILDS_CA_FILE` 为其实际 CA PEM 文件的绝对路径；代理将请求转发到本例的 `127.0.0.1:8787`，证书 SAN 匹配入口主机。CA 仅供本次客户端/Agent 使用，不安装到系统，不跳过校验。代理须流式转发 SSE，并允许制品上传/下载独立时限。产品当前没有内置 TLS 证书管理。

纯同机试用可把入口设为 `http://127.0.0.1:8787` 并删除配置中的 ca_file 行；这不验证跨主机 TLS。已有其他控制端占用 8787 时换端口，并同步代理后端。不要停止其他服务。

## 私有控制端与身份

```bash
: "${MYBUILDS_SERVER_URL:?先设置实际HTTPS入口}"
: "${MYBUILDS_CA_FILE:?先设置实际CA文件路径}"
cat > "$FIXTURE/server.yml" <<'YAML'
listen: 127.0.0.1:8787
data_dir: central
concurrency: 2
heartbeat_interval: 5s
lease_duration: 30s
database:
  driver: sqlite
  dsn: central/control.db
YAML
cat > "$FIXTURE/client.yml" <<YAML
server: $MYBUILDS_SERVER_URL
token: '\${MYBUILDS_CLIENT_TOKEN}'
ca_file: $MYBUILDS_CA_FILE
timeout: 30s
YAML
chmod 0600 "$FIXTURE/server.yml" "$FIXTURE/client.yml"
export MYBUILDS_BOOTSTRAP_ADMIN_TOKEN="$(openssl rand -hex 32)"
export MYBUILDS_CLIENT_TOKEN="$MYBUILDS_BOOTSTRAP_ADMIN_TOKEN"
"$server_bin" --config "$FIXTURE/server.yml" migrate
"$server_bin" --config "$FIXTURE/server.yml" serve &
server_pid=$!
for attempt in 1 2 3 4 5; do
  if "$client_bin" --config "$FIXTURE/client.yml" status --json >/dev/null 2>&1; then break; fi
  sleep 1
done
"$client_bin" --config "$FIXTURE/client.yml" status --json
```

bootstrap 仅在从未初始化身份的数据库生效，撤销后不会复活。离线先建其他角色会完成身份初始化；若选离线 token create 流程，应先创建 admin 并安全保存其一次返回的 token，再创建 trigger/approver。serve 在线期间同库 migrate/本机管理拒绝，使用远程命令；第二控制端无论监听端口是否不同均拒绝。同一数据库只归一个控制端，Agent 不连接数据库。

## 注册与启动一个实际节点

```bash
"$client_bin" --config "$FIXTURE/client.yml" node create demo-generic \
  --labels generic --capacity 1 --json > "$FIXTURE/node.private.json"
export MYBUILDS_AGENT_TOKEN="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["token"])' "$FIXTURE/node.private.json")"
cat > "$FIXTURE/agent.yml" <<YAML
server: $MYBUILDS_SERVER_URL
node: demo-generic
token: '\${MYBUILDS_AGENT_TOKEN}'
capacity: 1
data_dir: agent-data
ca_file: $MYBUILDS_CA_FILE
heartbeat_interval: 5s
lease_duration: 30s
YAML
chmod 0600 "$FIXTURE/agent.yml" "$FIXTURE/node.private.json"
"$agent_bin" doctor --data-dir "$FIXTURE/agent-data" --json
"$agent_bin" --config "$FIXTURE/agent.yml" serve &
agent_pid=$!
"$client_bin" --config "$FIXTURE/client.yml" node show demo-generic --json
```

等待 node show 的 healthy 为 true 再触发。Agent doctor 不加载 agent.yml、不需要 token、不联网，也不创建缺失数据目录；移动工具检查失败可能使 doctor 返回非零，通用 shell/git 与实际移动平台能力应分别核对。data_dir 由启动用户独占且为 0700，不与其他实例共享。Agent 与服务端时序配置必须一致；heartbeat 范围 1s–30s、lease 范围 10s–180s，且 lease 至少为 `4×heartbeat+2s`。

私有材料由节点的 `secrets_file` 指向自有 0600 普通文件，只有声明它们的实际 run 才注入。Agent 私有 SSH Git 使用其中显式 `MYBUILDS_GIT_SSH_KEY` 与 `MYBUILDS_GIT_KNOWN_HOSTS`，控制端在自己的 secrets_file 中使用 `GIT_SSH_KEY_FILE` 与 `GIT_SSH_KNOWN_HOSTS_FILE`；两端材料分开，不读取宿主未知私钥或 SSH agent。仓库、分支与脚本必须可信，Agent 在宿主执行，没有容器隔离。

## 固定提交、执行与中央证据

同机演示使用自建 Git 仓库；跨主机改为双方均可读取的受信 SSH/匿名 HTTPS 仓库：

```bash
mkdir "$FIXTURE/repo"
cat > "$FIXTURE/repo/mybuilds.yml" <<'YAML'
version: 1
builds:
  generic:
    params:
      channel: {default: internal}
    env:
      CHANNEL: '{{channel}}'
      BUILD_NUMBER: '{{build.number}}'
    timeout: 30s
    steps:
      - kind: run
        name: create-output
        run: |
          printf 'ordinary_marker\n'
          printf 'channel=%s number=%s\n' "$CHANNEL" "$BUILD_NUMBER" > output.bin
      - kind: artifact
        name: collect-output
        paths: [output.bin]
    post:
      success:
        - kind: run
          name: success-post
          run: printf 'success_marker\n'
      always:
        - kind: run
          name: cleanup-post
          run: printf 'cleanup_marker\n'
YAML

git -C "$FIXTURE/repo" init --initial-branch=main
git -C "$FIXTURE/repo" add mybuilds.yml
git -C "$FIXTURE/repo" -c user.name=Example -c user.email=example@example.test commit -m '节点执行演示'
fixed_sha="$(git -C "$FIXTURE/repo" rev-parse HEAD)"
"$client_bin" --config "$FIXTURE/client.yml" group create apps
"$client_bin" --config "$FIXTURE/client.yml" project init demo \
  --repo "$FIXTURE/repo" --nodes demo-generic --default-node demo-generic --group apps
"$client_bin" --config "$FIXTURE/client.yml" trigger demo --build generic \
  --ref "$fixed_sha" --param generic:channel=prod --idempotency-key demo-generic-1 --json > "$FIXTURE/batch.json"
BUILD_ID="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["builds"][0]["id"])' "$FIXTURE/batch.json")"
"$client_bin" --config "$FIXTURE/client.yml" logs "$BUILD_ID" --follow --stream-timeout 15m
"$client_bin" --config "$FIXTURE/client.yml" build show "$BUILD_ID" --json
"$client_bin" --config "$FIXTURE/client.yml" logs "$BUILD_ID" --json
"$client_bin" --config "$FIXTURE/client.yml" artifact ls "$BUILD_ID" --json > "$FIXTURE/artifacts.json"
ARTIFACT_ID="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["items"][0]["id"])' "$FIXTURE/artifacts.json")"
mkdir -m 0700 "$FIXTURE/downloads"
"$client_bin" --config "$FIXTURE/client.yml" artifact download "$ARTIFACT_ID" --output "$FIXTURE/downloads/output.bin"
cat "$FIXTURE/downloads/output.bin"
```

结果应为 succeeded，下载内容包含 `channel=prod number=1`。整个批次在入队时冻结 SHA、定义、参数与条件；后续 branch/settings 修改不改变任务。相同 key 与相同输入重放返回原批次/编号，输入改变拒绝；客户端新请求的 key 在网络前写 stderr，故障后用原 key 恢复。

日志 UTC 包含 build/phase/index/step/stream，声明秘密会脱敏；构建详情不公开参数值、凭据或脚本正文。历史日志 `next_seq` 配合 `--after-seq` 续读；SSE 自动按已确认 chunk 重连去重，并在终态结束。`--timeout` 是普通 API 期限，`--stream-timeout` 为客户端整体跟随期限，控制端 SSE 单连接上限 15m。制品下载独立总预算 10m，核对大小/SHA-256 后排他发布，输出必须位于自有私有目录，不覆盖已有文件；节点离线不影响中央已确认内容。

## 三个独立节点与管理命令

实际门使用 mac-a、mac-b 和 linux-generic（以下管理命令采用示例名称 mac-android-a/mac-android-b），分别拥有独立 token/session/data_dir。两 macOS 节点的真实 Android 检查通过，Linux ARM 节点仅按已验证 shell/git 能力执行 generic，不能据此声称 Linux Android 已验收。按上节方法逐个注册并把自己的 0600 配置安全传给对应主机：

```bash
"$client_bin" --config "$FIXTURE/client.yml" node create mac-android-a --labels android --capacity 1 --json > "$FIXTURE/mac-a.private.json"
"$client_bin" --config "$FIXTURE/client.yml" node create mac-android-b --labels android --capacity 1 --json > "$FIXTURE/mac-b.private.json"
"$client_bin" --config "$FIXTURE/client.yml" node create linux-generic --labels generic --capacity 1 --json > "$FIXTURE/linux.private.json"
"$client_bin" --config "$FIXTURE/client.yml" node ls --json
"$client_bin" --config "$FIXTURE/client.yml" doctor --node demo-generic --json
"$client_bin" --config "$FIXTURE/client.yml" doctor --server --json
"$client_bin" --config "$FIXTURE/client.yml" node drain demo-generic
"$client_bin" --config "$FIXTURE/client.yml" node enable demo-generic
```

项目 --nodes 是授权范围，--default-node 必须在范围内。无 runner 只分配 default_node；Android runner 必须同时满足授权、平台实际工具能力、标签及容量；无合格节点保持 queued。同项目同名 build 跨节点串行，不同项目可并行；全局容量与节点两端配置的较小容量同时生效。drain 不领取新任务但原任务继续续租；disable/token revoke/token rotate 撤销原执行权，不能继续用户步骤或 always，enable 不能解除停止未确认保护。token rotate 只显示一次新凭据，应像 create 一样重定向至 0600 私有文件。

## 取消与精确停止确认

对另一个仍在运行的构建执行：

```bash
"$client_bin" --config "$FIXTURE/client.yml" build cancel "$RUNNING_BUILD_ID" --json
"$client_bin" --config "$FIXTURE/client.yml" build show "$RUNNING_BUILD_ID" --json
```

queued 取消不执行脚本/post；running 先 cancel_requested，确认真实进程组回收后才 cancelled，仍有执行权才运行有效 post。ordinary 失败不会被 post 失败或后续取消覆盖。失联、Agent 退出或过期记 interrupted，停止未确认保留互斥/容量并隔离节点，不迁移任务；旧 journal 重启拒绝重放，不通过删除 journal 绕过保护。

只有管理员实际核实原 PID 与整个 PGID 已停止后，才能使用原 build show 中完整 fence 确认。不要根据等待时长或 interrupted 字样推断进程已停止；将 node_id/session_id/attempt_id/lease_id/lease_epoch 分别赋给下列变量：

```bash
"$client_bin" --config "$FIXTURE/client.yml" build confirm-stopped "$INTERRUPTED_BUILD_ID" \
  --node-id "$NODE_ID" --session "$SESSION_ID" --attempt "$ATTEMPT_ID" \
  --lease "$LEASE_ID" --epoch "$LEASE_EPOCH" --note '已实际核实原PID与整个PGID不存在'
```

错误 epoch/fence 拒绝；正确确认仅清除停止保护，保留原终态/原因，不接受旧成功回报。admin 可管理并读取证据；approver 只读安全 build/log/artifact；trigger 不可读取日志/制品或管理节点；node token 不可访问用户 API。approval/upload/reports/notifications 的生效能力仍在执行前拒绝。

结束本演示只关闭自己的节点与控制端，保留数据供检查，不删除未确认 journal 或孤立文件：

```bash
kill -TERM "$agent_pid"
wait "$agent_pid"
kill -TERM "$server_pid"
wait "$server_pid"
unset MYBUILDS_AGENT_TOKEN MYBUILDS_CLIENT_TOKEN MYBUILDS_BOOTSTRAP_ADMIN_TOKEN
```

## 已验证事实与最终门

- SQLite 与独立 PostgreSQL 16.14 同一套真实三二进制检查，各 51 项：管理、固定 SHA/参数、ordinary/post、20 次并发同 key 实际一次、真实取消/进程组、续租、SSE、二进制中央下载、角色、第二控制端、旧 journal 与停止确认；全部通过。
- 两 macOS 节点并行、真实 Linux ARM generic、verified HTTPS/私有 CA 与独立身份通过；实际 Android 构建号 101 的 APK/AAB/mapping 中央下载与签名/版本检查通过。Linux ARM generic 通过不等于 Linux Android 验收。
- 网络断开（ordinary/always）、disable/revoke、drain 续租、控制端重启有效租约、Agent 退出/旧 journal 拒绝、精确停止确认已有实际子进程证据；原 PID/PGID 回收按实际观察确认，无关进程存活。

真实 Android 取消也已通过：运行中的 Gradle 进程组已回收、无关自有进程存活，取消终态有 Started/StopConfirmed 且没有制品。全量 test/vet/race、darwin arm64/Linux amd64+arm64/Windows amd64 的 12 次三入口纯 Go 构建与本机帮助/版本检查已通过；Linux 普通/always 真实取消强化与 Spec Kit 收敛也已通过。主代理统一记录到 [validation.md](validation.md)，通过后整功能一次本地提交，无 push。以下故障门继续作为回归要求；005 真实 Apple 签名与后续 MVP 功能的门保留。

## 故障与有界负例

- 用户cancel覆盖queued/running/always：queued无脚本/post，running先cancel_requested，Wait/组清理成功后才cancelled；合法always按post累计预算。普通exit失败不被post/取消覆盖。
- 自有前台+后台进程、独立无关PID：切断控制连接/撤token/迟到renew，权限期限前停止本组，全部always禁止，无关PID存活。旧session/fence/epoch/expiry now==expires/event冲突均拒。
- 注入真实磁盘/权限写失败（非testhook），intent/OnStart/finished/log ACK保存失败不启动下一用户动作，保留真实原因与cleanup证据；纳秒剩余不能由ms反推或增长。
- 强制停止Agent留下journal，重启不重放/不按旧PIDkill。控制端重启保留有效lease；过期interrupted+guard继续占容量/同名、原节点quarantine。原node当前独立身份或admin真实确认仅解物理保护，不接受旧success。证据记录实际观察，不虚构已停止。
- 日志同seq重发/响应丢失/断流后从confirmed seq续读无重复；真实慢读SSE超过普通40s仍按独立stream期限工作，取消/单次write超时关闭连接；spool限额满导致明确停止，无静默drop。
- 产物同ID同内容幂等、冲突/短流/错误SHA/超限/路径/旧lease拒绝。用真实事务失败或失权时机验证发布后DB失败孤立文件不可List/下载；下载坏文件/输出已存在不覆盖。最终文件从独立快照，绝不直接上传workspace源。
- 生效approval/upload/reports/notifications全执行前失败，ios_signing解析不支持；inactive unsupported按既有when规则处理，不创建stub。trigger/node不可读logs/files/admin API；安全DTO、错误、终端扫描不含token/secret/rawsnapshot。
