# 020 研究与决策

日期：2026-10-05。实际执行setup-plan，模板来自项目.specify/templates/plan-template.md；selector=specs/020-project-retention，hooks={}。本文仅规划/官方接口及已有代码阅读，不声称原型或产品门通过。

## 1. 已有消费者与基线

**Decision**：在原Store、Server文件读取、Agent Serve和原项目设置接入，前置008+019验收后实施。
**Rationale**：正式集成基线019fee97e8，008504dc6、00785b46bf已验收，按当前实际模型/Store/Agent/Server复核。当前buildRecord只有CreatedAt，没有TerminalAt；step/receipt/guard在008保存真实停止证据。requestRecord→batch→build和project.NextNumber承载幂等/号段。中央文件是metadata StorageID对应data_dir/logs或artifacts的UUID；共享读取保护尚未实现。Agent checkout保留scm/checkout-*/workspace，结果根由ResultParent产生，成功后journal移除，不能从当前journal列表找到全部历史归属。
**Alternatives considered**：定时按mtime扫目录、删除整个项目、从lease过期推断进程退出、直接删build父行都丢证据/破坏FK或幂等。实际接入路径与owner见plan，以已提交8/19源码为正式消费者，不借005未验收源码。

## 2. 策略与排序

**Decision**：全局管理文件启动同步具体全局记录；项目Settings.Retention只有可省略正整数builds/days，逐字段继承。保留100/30默认。现有严格YAML node再Viper顺序不变，JSON重复/null/未知类型不放宽。
**Rationale**：cfg在启动时冻结，现有代码没有热重载；项目set事务已有PolicyVersion与管理员审计，变更后每次操作读取当前版本。days按24h纳秒安全转换≤106751，builds为正int64；任何溢出在生效前拒绝。全局修改通过受限管理文件与重启，不另造远程配置平台。
**Alternatives considered**：仓库retention、把项目部分值整体覆盖、浮点天数、使用墙钟自然日或snapshot冻结策略都不符原需求。候选分页只投影已知终态且有可信时间未清理记录，保护记录有时间仍计排名，边界严格OR。

## 3. 可信终态时间

**Decision**：服务端成功事务一次写真正转换UTC时间，不采用Node.Progress.At或一般UpdatedAt；同receipt重复/停止确认不改。旧精确build_finished receipt.CreatedAt可用；无attempt的skipped来自Enqueue CreatedAt；无attempt取消仅真正取消审计的最早成功时间可证。其余null保护。
**Rationale**：lease expiry是期限，ExpireLeases可能之后执行，不等于终态发生时间；停止确认时间更晚也不能刷新。旧Kind为空不能据此猜build_finished。queued取消审计的重复调用会再写审计，因此仅匹配状态转cancelled的最早真正依据，不选最后更新时间。
**Alternatives considered**：CreatedAt充当所有终态、lease expiry补中断、最新audit/receipt或后台当前时间回填都拒绝。迁移行为须真实两库与当前8修复后核对，无法证明宁多保留。

## 4. RetryOf与最小证据

**Decision**：保留完整子构建时保父祖先闭包，闭包遍历有界/检测异常循环并保守保护；清理完整内容按child→parent，原RESTRICT外键不改。留下最小build/batch/idempotency/项目计数器和审计，完整历史标记cleaned不占配额。
**Rationale**：008真实nullable RetryOf selfFK=OnDelete:RESTRICT；request和attempt/receipt也引用父行。旧键须仍返回原batch/build/number，不可通过删请求让它再执行。墓碑无脚本/参数/大进度或原文件，但原关联行仍存在；没有必要硬删这些必要证据。
**Alternatives considered**：ON DELETE CASCADE、关FK、把RetryOf置null、保留子却先删父、把文件清理当新项目均拒绝。祖先内容保护与最终仅保最小关系不同，不以永留空parent偷偷丢需要的完整证据。

## 5. 读取占用与重启

**Decision**：具体Begin/Activate/End EvidenceRead短事务，实际fd共享flock，清理同inode排他非阻塞flock；所有下载和实际log/SSE chunk读取都接此路径。现有10m下载/15mSSE上限不扩大。
**Rationale**：[Apple flock手册](https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/flock.2.html)与[Linux flock手册](https://man7.org/linux/man-pages/man2/flock.2.html)说明共享/排他冲突与非阻塞方式；Linux说明所有相关fd关闭释放。锁是协作式，所有本应用读取/删除入口必须实际使用，不能防止同用户另一个进程无锁恶改。这里限单控制端自有本地证据目录，不承诺NFS/SMB跨主机锁语义。
**Alternatives considered**：纯内存readers重启丢失、记录TTL过期假释放、PG独占失锁后假定旧HTTP进程死掉、为每文件常设通用锁服务均不用。已退休引用不能新增Begin；已Begin但未Activate在持当前DB运行权核对前不输出字节。重启取得同inodeEX实际证明无共享读者后清旧登记，而非按超时删文件。

## 6. 文件边界与部分失败

**Decision**：复用os.Root和非阻塞open/fstat、uid/type/nlink/parent identity；记录已登记对象的identity和大小SHA后，移入本删除ID排他专有隔离位置，再核对实际同inode，有限删除并fsync。持久清理状态恢复原路径或此唯一隔离位置，不扩大扫描。
**Rationale**：[Go os.Root接口](https://pkg.go.dev/os#Root)限制根内路径但不能替代对象归属；仅RemoveAll用户传入目录会越界/误删。移入私有隔离位置后复核可发现源被替换，错误对象不被继续删除。目录树拒links/特殊文件，逐项检查/有限遍历；原工作区由本次Agent创建，无其它节点/宿主工程授权。
**Alternatives considered**：只做Clean/前置Lstat、吞权限错误当不存在、扫描孤立文件/共享缓存、重新跑用户post做清理均拒绝。最多100000项/深度64/30s，超限保留真实partial/failed；策略变更或新保护导致paused，不能继续旧候选授权。

## 7. 节点管理事项与未来保护

**Decision**：新增具体本地资源登记和当前NodeActor独立领取/Authorize/Confirm删除ID；不使用旧执行lease，不杀进程。资源ID只映射Agent私有登记路径，terminal/停止回执与journal/spool已确认才可删。当前未注册旧工作区只报告归属待确认，不能扫prefix猜归属。
**Rationale**：007成功journal移除后丢私有ResultDir，需真实登记先于Run动作且与原Ref绑定；008精确终态readonly确认只证明原执行，不授权任意删除。离线/撤销留pending，轮换同NodeID可继续，另节点拒。
**Alternatives considered**：中央传绝对路径、expiry清工作区、信号旧PID、给管理事项用户shell、预建审批/unknown假的字段都不用。未来010/011/014在真实状态事务加入同一retention复核；现在unknown状态保守拒绝，但不声称已验收所有未来保护。

## 未解决事项

无业务NEEDS CLARIFICATION。技术字段已按008/019正式基线复核；旧终态补齐与资源登记仍须实际红绿验证。文件锁/隔离/流式下载/同事务竞争及两库macOS/Linux是实施门，未以API阅读代替验证；未来发布与014真实联验仍待其消费者交付。

## 8. 实际对象观察、短事务与墓碑消费者

Decision：Server同fd EX/hash取得ObservedIdentity Size/SHA后由Store短事务固定身份与自推UUID隔离slot；rename/fsync后可确认quarantined，重新授权后unlink/fsync确认deleted。所有私有字段json:"-"，HTTP不授路径。节点授权从本次请求发送起点保守≤5s，SQL内闭包取ctx/5s更早值，公开方法不递归write。
Rationale：现metadata只有StorageID/Size/SHA，没有inode，若无观察保存入口则无法安全重启核对；响应到达才计时会延长授权。cleaned buildView要早投影，新retry先拒正文退役，原成功retry键仍重放最小batch。当前Recover合法终态本就不解析Snapshot，不创建新恢复器。
Alternatives considered：临时failed冒充quarantined、路径/mtime身份、假0出生时间、TTL清读者、每个公开方法互相write、清正文后重新分号均拒。

身份编码按go-api冻结，Darwin同fd birthtimespec，Linux同fd statx实际返回STATX_BTIME mask；未支持failclosed，不冒称能力已验收。官方依据[Apple stat(2)](https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/stat.2.html)、[Linux statx源手册](https://kernel.googlesource.com/pub/scm/docs/man-pages/man-pages/+/master/man/man2/statx.2)。实际两平台文件系统支持及删除/重启门仍须实现验收。
