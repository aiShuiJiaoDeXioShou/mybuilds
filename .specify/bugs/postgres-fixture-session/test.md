# 缺陷验证：PostgreSQL故障夹具精确自身会话

- **Slug**: postgres-fixture-session（显式交接）
- **Tested**: 2026-10-04T21:51:34.677300+00:00
- **Assessment**: ./assessment.md
- **Fix**: ./fix.md
- **Result**: verified

## Summary

实际同一PostgreSQL集群两个独立数据库持相同控制端advisory key时，修正测试终止自身事务会话并正确返回ErrLockLost/回滚零行；另一数据库控制端始终保持原backend PID10657、granted=true及verified HTTPS status200。SQLite/PostgreSQL完整Store和race均通过。

## Checks Performed

环境：Go1.25.4 darwin/arm64，PostgreSQL16.14自有socket55436；独立测试库mybuilds_bug_session_1h4j1pkz（每测试自有schema），对照库mybuilds019_faults_dphhjxf。基线504dc6fa8581f74a15ecc146a474976d5ae33a22，不借019源码；所有Go测试仅此Store范围，未重复根全量suite。

| Check | Command / Action | Result | Notes |
|---|---|---|---|
| 修后原症状等价场景 | `go test ./internal/store -run '^TestLockLostDuringTransactionRollsBack$' -v -count=1 -timeout=120s` | pass | 两库实跑，包1.289s，保留ErrLockLost/count0 |
| 回归 | `go test ./internal/store -count=1 -timeout=300s` | pass | 两库，20.354s |
| race | `go test -race ./internal/store -count=1 -timeout=300s` | pass | 两库，75.608s |
| vet | `go vet ./internal/store` | pass | exit0 |
| 差异检查 | `git diff --check` | pass | exit0，仅lock_test.go及本缺陷3报告 |
| 实际另一数据库对照 | 四次peer_probe verifiedHTTPS/status与限定数据库锁查询 | pass | before/after-target/after-fullStore/after-race同PID10657/granted=true/status200，无Agent/Run；之后已将窗口归还C |

上述Go测试都显式设置MYBUILDS_TEST_POSTGRES_DSN到自有库，未省略PG。实际命令、UTC、退出及日志SHA保存在外部证据目录 `/tmp/mybuilds-mvp.zKtK0e/postgres-fixture-session-evidence-1h4j1pkz`，peer证据已复制冻结，不依赖后续C运行状态。

## Output Excerpts

```text
--- PASS: TestLockLostDuringTransactionRollsBack
    --- PASS: TestLockLostDuringTransactionRollsBack/sqlite
    --- PASS: TestLockLostDuringTransactionRollsBack/postgres
ok mybuilds/internal/store 20.354s
ok mybuilds/internal/store 75.608s
10657|mybuilds019_faults_dphhjxf|t
verifiedHTTPS.status=200
```

目标UTC21:47:26.625859Z–21:47:30.409403Z；Store21:48:21.529687Z–21:48:42.718071Z；race21:49:04.510640Z–21:50:22.989229Z；最后peer probe21:50:32.033552Z，均2026-10-04。日志SHA依次target b4306eb509546576020347c601bfe8afe17b1c0b4366e4fd2222c2cb84d62fce、Store467be3e3d44c2a6bc5461ccf36c1b6ac1c4e0cae6150ce833470a00115f7a668、race937d4c826e1ce3a49150369101f9fd861cfddb8e41c8abff28307ecf9aede0a1。

## Residual Risks

原实际normal失败与另库control_lock_lost日志沿assessment保留；原错误被选择的PID未捕获，不猜测。未重复会误杀peer的旧实现。此结论只验证测试注入归属修复，不宣布019功能通过或修改生产数据库锁语义。

## Recommendation

可关闭此缺陷。根逐SHA集成此测试和3报告后独立本地提交，不夹带019，不push；原生产与其他Store测试保持。
