# 008 数据模型与迁移

具体公共类型只在 [go-api.md](contracts/go-api.md) 定义；本文件说明持久约束。

## 原模型沿用

builds、steps、batches、attempts、execution_receipts、idempotency、nodes/credentials/sessions、stop_confirmations、log_chunks、artifacts继续使用007实体与FK。不得重置原LeaseRef、epoch、remaining NS、post_phase、原reason、步骤条件/进度、日志cursor或文件证据。

## 最小字段变化

| 表/字段 | 类型与约束 | 用途 |
|---|---|---|
| builds.retry_of | nullable UUID，自关联build FK RESTRICT，索引 | 新构建指向直接原构建；普通trigger/旧记录NULL，不用空串唯一键 |
| execution_receipts.kind | string NOT NULL，旧迁移默认空值未知 | 仅接受真实事件kind；核对终态要求build_finished |
| execution_receipts.stop_known | bool非空default false | 本次终态完整manifest+物理停止已确认且无CleanupFailed；与receipt/状态原子提交 |

不新增上传/审批/报告状态、恢复队列或缓存表。旧receipt不能安全推断Kind/StopKnown，保守保留未知；旧build不回填虚构retry关系。迁移必须幂等，在自有SQLite/PG实际旧007数据执行，不能drop共享库。

## 状态与物理保护

- queued保留号和快照，由正常Claim竞争；skipped与所有终态不自动变活动。
- running且UTC now < 实际lease_expires保持；now == expiry也到期。Expire沿原reason优先，置interrupted+stop_unconfirmed，不减物理占用。
- stop_unconfirmed持续占全局/节点容量、同名锁并隔离节点；独立当前node/admin精确StopConfirmation仅解除此旗标，保留原interrupted/失败/预算/文件。
- terminal receipt要求中央当前row已非活动，ref仍与原attempt一致、last_event_seq等于该seq、receipt摘要一致、kind=build_finished、stop_known=true、stop_unconfirmed=false。停止确认不能把未知终态收据变成已确认。

## Retry事务

1. 当前admin/trigger身份有效；输入、key与operation摘要合法。读取原构建、原batch、原snapshot/steps与所属当前project。重放也检查当前角色与同一原项目授权，返回原BatchResult而不因新构建活动再创建。
2. 新请求仅接受原有编号的succeeded/failed/cancelled/interrupted、无停止保护，且实际状态/快照可识别；queued取消已有编号且无attempt的cancelled也可retry，其未运行事实不是未知停止。没有attempt的其它不一致记录拒绝。
3. 精确原attempt有started/intent时，终态验证或独立停止记录须支持物理停止已知；不能把status单独当证据。原节点范围×当前节点范围非空，否则forbidden；无runner还须原default在交集中，否则forbidden。工具/健康/容量不足仅正常queued。
4. 检查当前分支授权覆盖原branch、项目存在和发布授权；定义含upload即admin+allow_upload，false when不能豁免。当前尚未实现的审批/上传/报告保持既有预检查边界；不创建假的unknown或待批准记录。
5. 深拷贝原快照。原SHA、source/source_digest/file、Definition、最终Params、Condition/Reasons不变。Facts仅更新新build.id/build.number；原project/build.name/git.sha/git.branch完整校验。原Repository由不可变同ProjectID取值。
6. 新步骤复制原Phase/Index/Name/Kind/Condition/Reasons（已核实007 reducer不写Condition/Reasons）；只用原Definition/最终Params与严格config校验核对原步骤投影，不调用当前Preview、不读取当前设置/分支，也不借旧执行状态判断条件。普通condition=skipped初始化skipped，其余pending；post全pending。清Intent/Started/Stopped/Cleanup/Elapsed/Exit与ArtifactIDs。新NS预算取原InitialBudgetNS/PostBudgetNS，并与原定义校验。新lease/session/attempt/cursors/stopflag为空/0，不复制日志/产物/报告证据。
7. 同Store.write事务写新batch、新build+retry_of、新steps、project计数器CAS、request、必要审计；末尾再次检查身份、授权/保护及控制端锁。原记录不写，任何失败无号/无新row。新Claim按007实际生成独立attempt/lease/epoch。

## 本地journal

沿007当前文件格式，不迁移旧PID为操作权限。只在data锁内有限枚举（≤128、各≤1MiB、owner/0600/regular/单链接、不跟symlink、不阻塞FIFO），严格JSON并验证完整同ref与本地待终态摘要。仅PendingEvent为build_finished、StopConfirmed=true且!CleanupFailed、无PendingStop/PendingLog时查询；任何未知claim/非终态/损坏/缺文件均保留阻止新session。本地journal全状态与PendingEvent.Progress均须StopConfirmed=true/!CleanupFailed，Kind/status合法且重新计算Digest与中央Status完全一致。成功只删除该条同inode自有journal并fsync目录，不删除spool/结果目录/未知文件或自动清理其它任务。其它unknown仍阻止Serve，不能把单条成功说成全部恢复；文件持久/删除/fsync不明同样拒继续。


终态发布前只停止续租worker并等待它当前请求有界结束；原Authority timer不停止、deadline不重置，永久身份/fence错误仍撤权。重新校验后单次终态请求，正确ACK才清journal；这防止中央terminal后的renew409撤销终态ACK读取。可能中央TTL上界仍取最后真实renew请求，仅用于物理回收后的独立停止确认。
