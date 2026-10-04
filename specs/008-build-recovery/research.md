# 008 Phase0研究

研究基线为已验收007 `85b46bf`，这里只读分析，未把研究当008实现或验收。

## 1. 控制端启动与租约

**决定**：复用Store独占锁和ExpireLeases，在真实ListenAndServe监听前调用具体Recover核对可调度记录；有效running身份/期限/NS预算不改，到期只执行现有中断保护规则。周期核对继续原入口。

**理由**：`internal/server/server.go`已在监听前/200ms ticker执行到期处理；`store.write`末尾再次核对锁与捕获的原期限。PG锁属于实际持锁连接，不能用新连接假装保留；SQLite沿既有单写事务。

**替代**：将所有running重置queued会重跑副作用；恢复状态表和多控制端协调没有当前消费者。官方依据：[PostgreSQL advisory lock](https://www.postgresql.org/docs/16/explicit-locking.html#ADVISORY-LOCKS)、[SQLite事务](https://www.sqlite.org/lang_transaction.html)。

## 2. 短暂断连与执行期限

**决定**：只对明确network_error进行有界重试，执行仍受原executionLease保守deadline；所有迟到续租按请求开始时刻计算，不能复活到期权限。注册/未知claim/终态重放不纳入恢复重试；首次claim请求可沿007同key的当前有界确认，原起点与journal不可重置。短断联时claim可能先于heartbeat失败，必须暂停新key并只核对原请求，不能直接退出误停其它有效执行。具体上限/unknown保留见Go契约。

**理由**：当前serve heartbeat或renew单轮network_error会退出/取消，可能使短控制端重启误停仍有效脚本。改动只落现有HTTP实际消费者；临时网络等待计NS预算，日志仍受spool上限。永久鉴权、fence/冲突、保存和日志本地错误立即闭锁。长断网直到旧deadline一定回收本次真实组。

**替代**：不受期限的重连、自动新session接管或WithoutCancel恢复动作均破坏fence。[Go WithoutCancel](https://pkg.go.dev/context#WithoutCancel)不保留取消/期限，因此原AuthorityContext仍不可移除；[Go单调时钟](https://pkg.go.dev/time#hdr-Monotonic_Clocks)用于当前进程保守计时，持久NS不从毫秒反算、不从重启后墙钟臆测活跃耗时。

## 3. 原快照retry

**决定**：Store独立Retry事务深拷贝原Definition/最终Params/Facts/条件证据，复用原SHA/source digest/来源/文件；只替换新build.id/number，当前授权取原节点范围与当前项目范围交集，原default固定。新普通/post预算来自原完整定义，旧剩余预算不改。

**理由**：Enqueue会重写Facts与AllowedNodes；不能直接借用。当前when仅依赖Params/git.branch，均冻结；旧step的Condition/Reasons不被ApplyEvent改写，可用于初始化新进度，无需再Preview当前设置，也无需新的条件执行接口。pending代表原模板依赖实际新node/workspace，不能复制旧动作状态或假定可执行。Repository在project.go登记后无更新入口，FK RESTRICT禁止删除仍被引用项目，因此同ProjectID的仓库就是可信原来源，不额外新增迁移。

**替代**：重新Trigger/读HEAD/读项目参数会改变结果；复制旧剩余预算、日志、产物或started证据会把retry冒充resume。19后续扩展报告时保留原Reports定义，但新报告ID/seal/seq为空，不复制旧pass。

## 4. 终态ACK丢失

**决定**：execution_receipts仅增Kind与StopKnown；与真实build_finished、完整中央manifest和物理停止确认同事务记录。提供当前节点身份的只读精确查询。Agent锁内严格读取已保存的PendingEvent，对ref/seq/digest/terminal内容一致且StopKnown的自有journal按同inode删除。

**理由**：007收据只有seq/digest，不能证明某条是完整终态；ApplyEvent必须继续拒绝重复终态。StopKnown不是节点提交的自由字段，而是中央根据本次完整终态实际验证得到。旧无证据收据保持未知，不通过猜当前status补写。

**替代**：旧fence重放/续租、仅看BuildView终态、只看停止确认、按旧PIDkill都不能证明该pending终态被确认。无新通用receipt或恢复动作框架。

## 5. 幂等与后置边界

**决定**：沿identity+key唯一表，retry摘要含固定operation、原build_id和allow_upload；同key不同操作/输入冲突，编号、batch、关联、request和审计同事务。每次请求仍检查当前身份与授权；已有重放不能扩大授权。

**理由**：已有FindRequest/Enqueue的短事务与20竞争用例可复用，不能先分配号再验证保护。现阶段没有审批/upload unknown业务状态，故不建空字段；所有异常/未知持久状态拒绝retry，未来真实发布保护在同事务增加实际检查。

**替代**：客户端每次随机换key/自动重试、新幂等注册器和假发布回执都不需要。CLI明确用户提供key，失败保留该key以便原样重发。
