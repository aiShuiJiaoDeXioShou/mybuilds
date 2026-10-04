# 007 Phase 0 技术决策

读取006真实Store/Server/SCM、003Run/logger/process、004Android、MULTI_NODE/CONFIGURATION/INTERFACES与constitution 2.1.0。所有NEEDS CLARIFICATION已解决，无需用户再次确认。字段只在contracts定义。

## 唯一Run、实际开始与独立权限

**Decision**: Agent复用pipeline.Run/process.Run；Run接真实RemoteOptions，process.Command增加cmd.Start成功后的OnStart。intent先journal fsync再服务端确认；OnStart保存实际Started/PID/PGID，失败也必须一次Wait+group回收。结构化Log在原logger跨chunk脱敏后发送，不能解析Output文本。已有cancelOnWriteError会及时停当前子进程，远程消费者错误还闭锁整个Run和post。

**Rationale**: 现有内核已覆盖整批预检查、快照、取消与post预算。Start到Started回执有不可消除的崩溃间隙，intent保留不确定保护，不宣称恰好一次。进度包含真实post选择和未选phase的skipped，不由最终Status反推。

**Alternatives considered**: 第二executor、testhook、Run造doctor、重新拼shell流程，拒绝。

**Decision**: ExecutionContext与AuthorityContext分离。always只去掉用户取消，再合并Authority和剩余post预算；失租/持久化失败禁止全部用户post，系统回收独立。

**Rationale**: WithoutCancel没有deadline/Done/Err；AfterFunc可以将独立权限取消传播给实际动作。该权限策略是本项目设计，Go不会自动提供。[Go context](https://pkg.go.dev/context@go1.25.0#WithoutCancel)

**Alternatives considered**: 把取消等同失租或让WithoutCancel忽略权限，分别破坏合法always或越权，拒绝。

## 时序、会话与双库事务

**Decision**: 服务端可信5s/30s策略，握手必须匹配；本机期限从请求开始计算而非收到ACK。截止一旦触发不能被迟到响应复活。普通Heartbeat不续任务，新session不能继承未完成journal；旧PID不作为重启后的自动kill授权。

**Rationale**: 单调时钟不跨进程/JSON保存；重启不能重建执行权/预算。[Go time](https://pkg.go.dev/time@go1.25.0#hdr-Monotonic_Clocks)

**Alternatives considered**: 跨机墙钟推权限、应答后开始TTL、重启刷新预算、按存储PID杀进程，拒绝。

**Decision**: 沿Store.write短事务与PG持锁同sql.Conn，部分唯一(status=running OR stop_unconfirmed)兜底同名互斥，同条件计容量。保存实际事件receipt历史；提交前检查完整fence/真实UTC/锁，Renew尤其检查更新前oldExpires。

**Rationale**: 两库均支持部分唯一；索引不使用now，不以过期解除物理保护。[SQLite](https://www.sqlite.org/partialindex.html)、[PostgreSQL](https://www.postgresql.org/docs/16/indexes-partial.html)

**Alternatives considered**: 占用表/分布式锁、仅最后digest、启动全量interrupt，拒绝。A独立/tmp真实SQLite3.53.3/PG16.14原型已验证20竞争、partialunique、到期等值拒绝、guard保容量、FK/rollback/meta唯一，含race；这是研究，不替代最终Store实际时钟/中途到期/失锁验收。

## 固定SHA与节点秘密

**Decision**: Checkout复用受限gitRunner；精确分支fetch、固定SHA commit-type/可达验证、detached自有worktree、HEAD精确核对。禁宿主hooks/config/helper、submodule/replace/lazyfetch/过滤器，不改用户树，不fallback HEAD。

**Rationale**: --detach显式固定提交；smudge过滤器能执行命令，所以本地新仓库必须不继承来源config。分支正常推进仍同SHA，force-push导致不可达明确失败。[Git worktree](https://git-scm.com/docs/git-worktree)、[Git config](https://git-scm.com/docs/git-config#Documentation/git-config.txt-filterltdrivergtsmudge)

**Alternatives considered**: 直接用户worktree、git archive替代checkout、任意协议/默认agent，拒绝。

**Decision**: Checkout只接显式SSH key+known_hosts；Agent私有envfile包含build引用，但不能传给006仅允许两键的ReadPipeline。Run只接声明Secrets map，逐步env，不Setenv；本地LookupEnv保持。私有HTTPS认证仍不扩006。

**Rationale**: 并发Agent不能全局改env；token/其它任务值不交脚本。可信仓库不是OS沙箱，本功能不承诺隔离同OS用户恶意读文件。

## 日志与文件

**Decision**: 脱敏记录先有界spool fsync，再连续seq/offset/digest提交；ACK落盘才删前缀。满/写失败明确停Run。中央SSE有独立总时长、单次write deadline和心跳，不能沿普通client.Timeout/整体WriteTimeout。

**Rationale**: ResponseController提供每响应写期限；到期后不能延长，必须在写前设置并处理失败。[Go HTTP](https://pkg.go.dev/net/http@go1.25.0#ResponseController.SetWriteDeadline)

**Alternatives considered**: 丢弃满channel、无限spool、30s普通API读取代替SSE、无限流，拒绝。

**Decision**: stage/hash/fsync→同根新StorageID排他原子发布→最终短DB事务再验租约→complete才可List/下载。DB失败孤立文件不可下载；重复同ID返回canonical，冲突不覆盖。bulk文件I/O不占DB事务。

**Rationale**: 文件/DB无共同事务；Root.Link提供同根no-replace发布，然后删除stage。[Go Root.Link](https://pkg.go.dev/os@go1.25.0#Root.Link)

**Alternatives considered**: 先complete后写文件、任意路径、覆盖ID、整文件ReadAll、长事务网络流，拒绝。007不实现retention或孤立文件自动恢复。

## 自有CA与真实能力

**Decision**: ca_file可选受限PEM，扩充SystemCertPool副本，仍hostname verify/拒redirect，不改系统信任/全局env，不skip。非空SSL_CERT_FILE/SSL_CERT_DIR明确安全失败，避免隐式读取未知材料；SystemCertPool失败不静默弱化。[Go X509](https://pkg.go.dev/crypto/x509@go1.25.0#SystemCertPool)

**Rationale**: 当前真实跨VM与公司内网CA消费者。root网络原型Linux→mac192.168.5.2合法CA/SAN成功、wronghostname/unknownCA拒绝，证据`/tmp/mybuilds-mvp.zKtK0e/tls007-luc8st6v/network-verification.json`，只是研究材料。

**Alternatives considered**: skipTLS、改系统keychain、通用transport框架、httptest冒充跨VM，拒绝。

**Decision**: actual shell/git/java/aapt2/apksigner工具检查，不能按OS或admin标签宣称Android。项目wrapper在固定checkout后检查；Linux ARM不可执行SDK时android=false。当前ios signing未验收，ios能力不passed。

**Rationale**: 编译/文件存在不足以证明工具可运行。真实Android新repo/SDK/JKS材料准备不算007应用验收，最终版本/签名/取消独立核对。
