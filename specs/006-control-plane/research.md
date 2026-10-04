# 006 技术研究与决策

日期：2026-10-04。研究仅支撑当前控制面，不把原型当成已实现功能验收。

## 双驱动与WAL补丁

- 决策：GORM v1.31.2、github.com/glebarez/sqlite v1.11.0、gorm.io/driver/postgres v1.6.3、Viper v1.21.0；保持glebarez/go-sqlite v1.21.2，显式锁定modernc.org/sqlite v1.55.0及libc v1.74.1。mathutil v1.7.1、memory v1.11.0随其模块图锁定，最终go.mod/go.sum由主代理统一写。
- 原因：glebarez默认链实际得到SQLite3.41.2，受WAL-reset缺陷影响；官方说明修复于3.51.3及以后，故必须补丁后使用WAL。现代modernc补丁原型实际得到3.53.3，不仅凭版本名推断。
- 已实际验证（config001独立原型，Go1.25.4/CGO=0）：四连接各WAL/FK1/busy5000、事务回滚/unique/FK、约192ms真实锁等待、pool1与pool4各400次并发事务计数、重复AutoMigrate/integrity均通过；race/vet/modverify及Linux/Windows静态跨编译通过。RESTRICT删除返回SQLite扩展1811，不能依赖驱动必定译成gorm.ErrForeignKeyViolated；需要固定错误映射。
- 根实际PG16.14持久session原型5/5通过：第一session固定key(1973481521,6)取锁，第二拒绝，仅终止自有backend后旧session SELECT失败，新session可取锁；证据`/tmp/mybuilds-mvp.zKtK0e/postgres006-ldh7rr0w/advisory-prototype.json`。这仍不能替代真实Store失锁行为。
- PG方言DryRun只证实语法，不能代替真实事务。主代理已准备自有PG16.14 socket-only夹具，最终两个数据库跑相同业务suite；不连接用户已有数据库。
- 替代方案：换CGO SQLite违背既定纯Go分发；换ORM/驱动栈没有必要；保留易受影响版本再靠单写连接降低概率不能替代补丁。
- 来源：[SQLite官方WAL-reset说明](https://sqlite.org/wal.html#walreset)、[modernc v1.55.0官方包文档](https://pkg.go.dev/modernc.org/sqlite@v1.55.0)、[GORM事务](https://gorm.io/docs/transactions.html)、[glebarez驱动](https://github.com/glebarez/sqlite)。modernc文档要求libc与其go.mod一致，升级时不能单独保留旧libc。

## 独占运行权

- 决策：SQLite规范化普通数据库路径，0700父目录、邻接flock文件永不unlink，记录identity并复检；拒绝hardlink数据库避免两个WAL路径。PG持有专用session连接和固定数据库范围advisory锁，同一连接完成串行写事务，不使用另一连接验证后写入。
- 原因：PG session锁在会话结束时释放；若检测后在pool另一连接提交，会出现失锁仍写窗口。同一持锁连接失效则事务不能继续提交，不能自动重连认领旧运行权。本机管理和迁移同样取得锁。
- 检查：自有第二进程竞争、别名DSN/SQLite路径竞争、正常关闭后重取、替换自有锁文件/终止自有PGsession后no writes。
- 来源：[PostgreSQL16 advisory locks](https://www.postgresql.org/docs/16/explicit-locking.html#ADVISORY-LOCKS)、[database/sql Conn](https://pkg.go.dev/database/sql#Conn)。不提供HA或另建锁manager。

## 严格配置与Viper

- 决策：沿用现有YAML Node深度/数量/重复键/类型检查，后用单个局部Viper实例合并typed值，明确env白名单与CLI Changed值，最终统一校验。
- 原因：原型证明Viper UnmarshalExact会把`database.driver: 7`转换成字符串，不能替代严格类型校验。LoadServer不把未知secrets_file内容批量注入环境；LoadClient只在远程入口调用，token解析后不进入JSON。
- 替代方案：全局Viper/AutomaticEnv/隐式弱转换使不同CLI调用污染状态并失去未知类型诊断，因此不采用。
- 来源：[Viper官方覆盖/环境文档](https://github.com/spf13/viper)、[维护中的YAML节点API](https://pkg.go.dev/go.yaml.in/yaml/v3#Node)。

## 只读Git与快照

- 决策：具体ReadPipeline函数每请求使用DataDir/scm下独立0700临时bare仓库和0600凭据副本，不共享可变refs缓存或注册器；所有Git命令禁止global/system config，固定空hooksPath，init使用空template，禁止ext/未许可protocol、submodules/auto maintenance；从精确branch取得并固定完整commitSHA。指定ref只接受完整OID并校验可达，不接受tag/短SHA/任意rev表达式。
- 读取采用literal-pathspecs ls-tree（mode100644/100755、type blob、单记录精确文件名、1MiB以内）再cat-file blob，不使用textconv/filters，不checkout。root/child symlink、submodule、缺失/非法文件均明确失败。
- 原因：bare fetch仍能调用reference-transaction hook；只用bare不是无副作用边界。树条目检查避免把symlink内容误读成合法配置。读取绑定OID后分支推进不改变快照。
- 凭据冻结：匿名HTTPS/loopback HTTP/本地Git，以及显式SSH私钥+known_hosts pair（GIT_SSH_KEY_FILE/GIT_SSH_KNOWN_HOSTS_FILE），受限0600普通文件、路径基于secrets_file；不允许URL明文密码、argv密码、任意SSH命令或完整宿主环境。固定派生ssh禁默认identity/agent/global known_hosts/proxy/交互/password/forward/localcommand，严格核对hostkey。HTTP凭据当前明确未支持，不增加auth registry。
- SHA256默认SHA1bare真实fetch失败，同对象格式bare成功；采用有界ls-remote精确ref行探测40/64format，init对应bare，真正fetch结果固定SHA。ls-remote模式可匹配额外后缀同名ref，必须按完整refname筛唯一行而非取首行。
- 来源：[git fetch](https://git-scm.com/docs/git-fetch)、[git ls-tree](https://git-scm.com/docs/git-ls-tree)、[git cat-file](https://git-scm.com/docs/git-cat-file)、[git hooks](https://git-scm.com/docs/githooks)。SCM研究由engine003提供：Git2.47.1实际首批26/26通过；精确fetch/固定SHA后分支推进保持旧blob，dirty源worktree/index未变；真实symlink/tree/gitlink/父symlink/超限与literal pathspec、ext/伪选项/非法branch、hook/filter哨兵负例通过。证据`/tmp/mybuilds-scm006-evidence.20jj457f/evidence/`。完整annotated tag对象也可能为40hex，必须cat-file -t确认commit，不能用^{commit}偷偷剥tag。后续实际process.Run deadline500ms约512ms timeout、ctx取消约65ms cancelled、输出cap4096字节后log_error约23ms，均CleanupFailed=false；SSH自有sshd/key/known_hosts实际4/4成功/错host/错key/无hook通过且sshd已停。证据process-results.json/ssh-commands.json/ssh-assertions.json。功能最终验收仍见validation，不把研究原型冒充已实现。

## 预检查、条件与幂等

- 决策：复用Select/ResolveParams/纯Preview做结构与字段校验；build.when单独按冻结branch/最终参数判定，手动changes忽略。模板所需编号/node/workspace尚缺只代表pending，不能变成build条件pending。所选含upload要求admin+allow_upload，与when真假无关。
- 幂等摘要用原始业务请求的确定JSON（已知默认值、显式选择顺序、排序映射键），不包含Git读取结果或新编号。先查身份+key，Git在事务外，Enqueue事务再次查键并保存完整批次结果。不同内容同键冲突；并发相同键返回原批次。
- 最少持久化结构：组、项目、identity、初始化metadata、批次、build、step、幂等、管理audit。build保存实际选定definition及普通参数/事实，秘密字段仅引用；公开DTO只给参数键和非敏感摘要。初始化预算不随查询/重启刷新，006不会写running或执行终态。
- 替代方案：调用Run做预检查会执行宿主读取/能力探测；以Preview整个Condition判断build会混入模板pending；事务中执行Git会扩大锁持有时间；这些均不采用。
