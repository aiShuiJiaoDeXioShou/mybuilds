# 006 配置与CLI契约（已冻结）

## 配置

server.yml当前只支持listen（127.0.0.1:8787）、data_dir（~/.mybuilds）、concurrency（1）、database.driver（sqlite）、database.dsn（data_dir/mybuilds.db）、secrets_file（~/.mybuilds/secrets.env，可缺省不用凭据）。unknown/null/重复键/错误类型与未来build_profiles/retention/defaults/triggers配置明确失败。sqlite是本地普通文件；postgres DSN保持受限内存不得公开。相对路径基于配置目录，`~`仅当前用户，不能展开任意用户名。

覆盖：默认<文件<MYBUILDS_LISTEN/MYBUILDS_DATA_DIR/MYBUILDS_CONCURRENCY/MYBUILDS_DATABASE_DRIVER/MYBUILDS_DATABASE_DSN/MYBUILDS_SECRETS_FILE<明确CLI Changed值。Git secrets_file仅SSH两键GIT_SSH_KEY_FILE/GIT_SSH_KNOWN_HOSTS_FILE，0600普通文件、相对路径基于该文件目录，未知/重复/部分声明失败；HTTP凭据当前明确未支持，不能使用任意sshcommand或宿主agent。PostgreSQL只接受明确DSN，拒绝宿主非空PG*/SSL_CERT_FILE/SSL_CERT_DIR和service/servicefile/passfile；显式TLS普通文件至1MiB、私钥私有权限，内存验证，不退到宿主材料。bootstrap只从MYBUILDS_BOOTSTRAP_ADMIN_TOKEN读取，数据库只存摘要与初始化metadata；撤销不复活。普通配置不能写bootstrap/token。

client.yml当前支持server、token（0600受限凭据文件中的有效literal或完整`${NAME}`引用）、timeout正duration（30s）；MYBUILDS_SERVER_URL/MYBUILDS_CLIENT_TOKEN/MYBUILDS_CLIENT_TIMEOUT及CLI覆盖。文件含token须普通文件且权限0600（Windows按可用平台边界检查），空/过弱token拒绝。URL无userinfo/query/fragment，loopbackHTTP或验证证书HTTPS；普通HTTP timeout不影响本地build/post预算。

settings仅pipeline，source auto/repo、file仓库相对路径、builds.<name>.params字符串映射。原单build简写pipeline.params归default，并与pipeline.builds互斥；pipeline.profile仍是未支持的绑定方案。profile/绑定方案/通知/triggers/retention未实现明确拒绝。

## CLI

现有NewCommand()保留。全局--config指客户端连接或控制端配置，但只在相应业务命令读取；--server-url避免doctor --server冲突；--timeout仅remote普通API解释，不创建本地全局PreRun加载。help/version/init/run/本地doctor无需client.yml/token，即使该文件坏配置也不受影响。

| 命令 | 当前行为 |
|---|---|
| mybuilds-server serve | --listen/--data-dir/--concurrency明确覆盖；读取配置、独占/migrate/bootstrap、HTTP，取消优雅关闭 |
| mybuilds-server migrate | 取得相同独占、迁移；不执行Git/流水线 |
| mybuilds-server token create/ls/revoke | create --role必填，单次token；本机须独占，role三档；ls --json与分页 |
| mybuilds-server project add/set/ls/move/rm | add --repo/--nodes必填、provider generic、branches main、group default、--build-number-start默认1；--file或--settings互斥；set --settings必填；move --group必填；列表--group/--json/分页 |
| mybuilds-server group create/ls/rename/rm | rename <旧名> --name <新名>；同一事务规则；本机须独占 |
| mybuilds project init/set/ls/move/rm | 远程同规则；init对应server project add；--settings客户端当前目录读取内容后发送 |
| mybuilds group create/ls/rename/rm | 远程同规则，admin |
| mybuilds trigger <project> | --branch main、--ref完整SHA、--build/--all互斥、--param重复、--version/--channel快捷参数、--allow-upload显式；--idempotency-key可复用原key |
| mybuilds build ls/show | ls project/group/build-name/batch/status/limit20/offset0过滤，--json；show非敏感证据，admin/approver |
| mybuilds status | --json，三角色，显示仅queued/skipped、无Agent |

未有节点命令/审批/retry/logSSE/download/doctor --server闭环，此阶段不挂空命令树；现有明确未支持选项保留安全错误。项目framework/platform/hook/poll/schedule即使提供也明确报未支持，不传给服务假装生效。

trigger参数语法：`--param key=value`共享；`--param android:key=value`限定命名build（冒号区分scope），按第一个等号切分。相同scope/key重复拒绝；未选build或未知参数拒绝。共享参数所有选定build须声明；限定覆盖优先。--version/--channel同scope共享参数重复拒绝，限定覆盖可按既定优先级覆盖共享值。密钥不得用参数。

客户端每次新trigger用crypto/rand生成一次幂等key；同次网络重试沿用，不自动生成新key；显式--idempotency-key复用先前key。成功输出batch/sha/build独立ID/编号/状态，并提供非机密request_key供网络失败后恢复；错误不能打印原参数或底层网络响应。CLI不得自动重跑shell或变更用户Git工作树。

默认列表用tabwriter、--json用encoding/json同一安全DTO；诊断stderr，结果stdout。成功/全部skipped退出0；输入/鉴权/HTTP/独占/Git失败非0。服务正常收到取消并完成关闭为0，资源关闭或运行权失效失败非0。token仅create结果显示一次，不写普通日志。
