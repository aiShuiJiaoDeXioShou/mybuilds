# 缺陷评估：PostgreSQL故障夹具误选其他数据库会话

- Slug: postgres-fixture-session（目标模式自动生成）
- Created: 2026-10-04T21:39:00Z
- Source: 全MVP自动验收实际输出，无外部URL
- Verdict: valid
- Severity: high（测试误中止另一自有数据库的控制端锁会话）

## 实际报告

最终全量normal双库TestLockLostDuringTransactionRollsBack/postgres失败：mid-transaction loss: <nil>；独立reports019-faults-dphhjxf_准备控制端同期记录control_lock_lost。原真实失败日志保留，未作为产品Store.write失败。

## 症状与复现

1. 在同一自有PostgreSQL集群的两个独立数据库启动持相同advisory key的控制端/Store。
2. 运行现有事务失锁测试。旧查询只按classid/objid筛pg_locks，不按database或Store会话限定，能得到另一数据库的PID。
3. 旧测试中止错误会话，自己的事务提交而测试预期失锁；另一准备控制端报control_lock_lost。C已停止自己preparedserver，保留原日志；根全量normal失败证据在reports019-final-3y4liwqb/final-normal-dual.log。具体被选择PID未捕获，不补猜其数值。

## 代码与原因

internal/store/lock_test.go的TestLockLostDuringTransactionRollsBack使用SELECT pid FROM pg_locks全cluster扫描。生产internal/store/store.go已持s.conn独立会话，writer也绑定同一s.conn，checkLock按pg_backend_pid核对，不存在此全cluster猜PID行为。归因置信度高；不修改生产锁或降低rollback断言。

## 首选修正

只修改internal/store/lock_test.go：启动阻塞事务前，从自己的s.conn读取pg_backend_pid；故障注入只中止此确知PID，不再查询集群中的任意锁。保留SQLite门、ErrLockLost与未提交行count=0。

验证同集群其他自有库的preparedserver保持鉴权status200且原锁会话存活，修正测试失锁正确返回/回滚；运行同两库Store必要回归、race及vet/diff。现有MVP允许自有测试会话故障注入，不涉及外部应用。

## 风险与未解决项

无API、数据库迁移或依赖变更，只修测试故障目标归属。中止独立准备会话的旧错误日志保留；不重启未知服务、不运行生产故障注入。无业务澄清项。
