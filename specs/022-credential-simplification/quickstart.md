# 验收

在 macOS/Linux 专用测试工作区运行：

```bash
go test -p 1 ./internal/config ./internal/agent ./internal/scm ./internal/distribute ./internal/pipeline
go test -race -p 1 ./internal/config ./internal/agent ./internal/scm ./internal/distribute ./internal/pipeline -run 'Test(Client|Agent|PublishDoctor|PublishDecision|WebhookSecret|Secrets|IOSSecretRefs|ActualConcurrentTasks|Data|SSHCredentials|RestrictedMaterial|Log)' -count=1
go test -p 1 ./...
go vet ./...
```

权限、叶链接/FIFO、延迟秘密读取、控制凭据隔离与跨块日志断言在既有 Go 测试内。使用 `chmod 0400` 的管理配置、secrets.env、SSH key/known_hosts 和发布/通知材料应成功读入；公开权限仍失败。真实商店人工验收不作为本次内部精简的替代测试，不触发商店发布。
