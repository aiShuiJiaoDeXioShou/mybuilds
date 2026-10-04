# 007 配置与真实CLI契约

只新增本功能真实入口，原006配置/命令与本地init/run/doctor行为保留；unknown/未实现flags拒绝。消息字段见[go-api](go-api.md)，上限/时序见[node-protocol](node-protocol.md)。

## 配置

Agent默认`~/.mybuilds/agent.yml`，只有serve加载；本地doctor/help/version不读业务配置/token/秘密、不连接控制端，doctor仅运行明确本地诊断工具。

```yaml
server: https://build.example.com
node: mac-android-01
token: "${MYBUILDS_AGENT_TOKEN}"
capacity: 1
data_dir: ~/.mybuilds/agent
secrets_file: ~/.mybuilds/agent-secrets.env
ca_file: ~/.mybuilds/company-ca.pem   # 可选，普通受限PEM，仅HTTPS读取
heartbeat_interval: 5s
lease_duration: 30s
```

`config.AgentConfig`具体字段Server/Node/DataDir/SecretsFile/CAFile string、Capacity int、HeartbeatInterval/LeaseDuration time.Duration、RuntimeToken/TokenEnv string（后两json:"-"）。`AgentLoadOptions{Filename string;Explicit bool}`与`LoadAgent(AgentLoadOptions)(AgentConfig,error)`沿现有严格配置/相对路径展开/受限普通文件，不读未知宿主profile。Agent配置含token声明须0600，data_dir实际0700/owner/无symlink且serve独占；secrets_file如被声明或任务需要须0600 owner普通有限文件。token沿Client格式一次解释完整ENV引用或0600文件明文，不二次插值；环境覆盖仅MYBUILDS_AGENT_TOKEN，runtime值不出安全响应/错误。node必填安全名称、capacity/time约束见协议；启动握手必须比较配置node与实际SessionGrant.NodeName，UUID NodeID用于执行归属。

server.yml增加heartbeat_interval/lease_duration，ServerConfig对应HeartbeatInterval/LeaseDuration time.Duration（YAML解析duration字符串）；默认/范围沿协议，只读可信配置定租约，不接受node扩大。ServerLoadOptions CLI无需新增公开时序flags，实际测试可配置文件。policy握手返回实际值，不一致明确失败；现有Concurrency仍是全局上限。

client.yml增加可选ca_file；ClientConfig.CAFile string、ClientLoadOptions.CAFile *string；MYBUILDS_CA_FILE与`--ca-file`覆盖遵循已有file<env<CLI优先级。仅远程命令加载，不影响本地init/run/doctor。所有URL沿现有HTTPS/loopbackHTTP与凭据/query/redirect限制。

`config.TLSRoots(caFile string)(*x509.CertPool,error)`是Agent与remoteClient两个真实消费者共用的最小CA函数。只在HTTPS调用：SystemCertPool副本+受限普通PEM≤1MiB、CA解析失败/无有效CA/系统roots失败明确tls_configuration_error，保留hostname/TLS验证，不安装系统信任、不InsecureSkipVerify。SSL_CERT_FILE/SSL_CERT_DIR若非空明确unsupported_tls_environment，不全局修改；不影响PG。HTTP loopback不读取无关CA材料，不建通用transport框架。

## Agent

| 命令 | 参数/实际行为 |
|---|---|
| mybuilds-agent serve | --config，默认路径；flock/session、实际doctor、claim/renew/Run/journal/spool。取消serve停止所有自身组，未确认留journal。 |
| mybuilds-agent doctor | --data-dir默认~/.mybuilds/agent、--json；只检查本地data_dir/journal和实际OS/arch/shell/git/java/aapt2/apksigner；不加载配置/token/秘密、不连接、不搜未知签名材料、不运行仓库脚本。 |
| mybuilds-agent version | 无参数，统一version。 |

Doctor使用已有process.Run固定工具命令、9项宿主白名单，单工具/整体/输出有界；不能只检查SDK文件就通过aapt2/apksigner，尤其Linux ARM。Android实际所需工具全passed才报平台能力；Gradle wrapper依具体checkout precheck，不在node doctor运行任意项目script。Xcode可诊断但当前ios signing固定skipped/unsupported，不授ios runner。失败/未确认清理后不继续工具。serve缺失配置/凭据固定失败；doctor不要求凭据，未初始化data_dir报告skipped/uninitialized，无rawoutput/path/token；局部工具缺失可安全报告失败且不谎报能力，generic shell/git通过的节点仍可接默认通用任务。

## 管理与客户端

服务端本机命令复用同Store admin逻辑；客户端node远程命令必须user admin，没有节点身份豁免。

| 命令 | flags与行为 |
|---|---|
| mybuilds-server node create <name> / mybuilds node create <name> | --labels逗号列表、--capacity默认1、--json；一次返回NodeCreated token。 |
| 同上 node ls/show <name> | ls分页--limit/--offset，--json；show单节点安全视图。 |
| 同上 node drain/enable/disable/rm <name> | 无业务可选flags；rm tombstone且永久不复用名称，活动/guard拒绝。 |
| 同上 node token rotate/revoke <name> | rotate仅一次新token，revoke立即撤权限；实际事务。 |
| mybuilds doctor --server / --node <name> | 互斥，与本地Android flags互斥；--json安全实际摘要，离线不假装实时检查。 |
| mybuilds build cancel <id> | admin；返回持久cancel_requested或queued cancelled，不等待/伪造停止。 |
| mybuilds build confirm-stopped <id> | admin，--attempt/--session/--lease/--epoch/--node-id均显式、--note必填；实际观察依据，不恢复旧success。 |
| mybuilds logs <id> | --step、--after-seq默认0、--limit默认200、-f/--follow、--stream-timeout默认15m（1s–1h）；历史JSON可--json，follow不混--json。 |
| mybuilds artifact ls <build-id> | --json、--limit/--offset。 |
| mybuilds artifact download <artifact-id> | --output必填，已有文件拒绝覆盖；先Get安全meta，再固定ID/name流、Size/SHA校验、同目标目录stage排他发布。 |

remote JSON沿现有remoteRequest；SSE和download为具体独立消费者，共用配置/token/CA检查，不能用1MiB ReadAll或普通Timeout读流，不建泛用RPC。SSE Ctrl-C关闭连接，confirmed seq续读不重复；下载Ctrl-C/短流/坏digest只删除自身stage，原目标不覆盖。终端展示日志控制字符安全处理；错误为固定码，不拼URL/原HTTP/rawscript。

现有BuildView/client show一起升级真实node/session/attempt/lease/epoch/cancel/guard/ns/step证据；opaque执行ID不授权限，方便admin依据精确fence确认停止；status安全DTO使用go-api冻结的完整七项实际统计，不能声称只有queued。没有API版本协商、全量Snapshot公共输出或未实现retry/upload/approval flags。
