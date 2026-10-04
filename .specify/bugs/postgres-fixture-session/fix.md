# 缺陷修复：PostgreSQL故障夹具精确自身会话

- **Slug**: postgres-fixture-session（显式交接）
- **Fixed**: 2026-10-04T21:48:07.083674+00:00
- **Assessment**: ./assessment.md
- **Status**: applied

## Summary

事务失锁测试在自己的连接尚未进入阻塞事务前读取真实pg_backend_pid，故障注入只终止该确定会话。移除全集群按advisory key猜PID的查询，避免干扰另一自有数据库控制端。

## Changes

| File | Change | Notes |
|---|---|---|
| internal/store/lock_test.go | 修改 | 仅TestLockLostDuringTransactionRollsBack前置读取自身PID，保留SQLite、ErrLockLost和count=0断言 |

## Tests Added or Updated

- TestLockLostDuringTransactionRollsBack：SQLite及PostgreSQL实际事务失锁均必须回滚。
- 不新建通用测试框架；原故障症状保留根final-normal-dual.log，具体被误杀PID未知，不补猜。

## Local Verification

- Go1.25.4；基线504dc6fa8581f74a15ecc146a474976d5ae33a22。
- `MYBUILDS_TEST_POSTGRES_DSN=<自有mybuilds_bug_session_1h4j1pkz> go test ./internal/store -run '^TestLockLostDuringTransactionRollsBack$' -v -count=1 -timeout=120s` → exit0，SQLite/PostgreSQL均PASS，包1.289s；UTC2026-10-04T21:47:26.625859Z–21:47:30.409403Z。
- 同集群另一自有数据库mybuilds019_faults_dphhjxf，目标前后精确锁会话10657不变、advisory granted=true、verified HTTPS status200，无Agent/Run。
- 目标日志：/tmp/mybuilds-mvp.zKtK0e/postgres-fixture-session-evidence-1h4j1pkz/target.log，SHA256 b4306eb509546576020347c601bfe8afe17b1c0b4366e4fd2222c2cb84d62fce。
- `git diff --check` → exit0。

## Deviations from Assessment

无。未改生产实现、依赖、迁移或其他测试。未重跑有误杀风险的旧查询，原实际红证据沿评估保留。

## Follow-ups

按speckit-bug-test运行同双库完整Store、race、vet及另库peer对照后记录最终结果；由根独立提交本缺陷，不夹带019。
