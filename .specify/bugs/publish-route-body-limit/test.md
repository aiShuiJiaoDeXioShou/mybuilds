# 缺陷验证：发布路由正文限额

- Slug: publish-route-body-limit（复用assessment/fix）
- 日期: 2026-10-05
- 结果: verified

## 已执行

原 TestActualHTTPStrictJSONQueryAndGroupCRUD 实际HTTP检查修前RED400、修后PASS413；发布API同目标回归通过。

- `go test ./internal/server -run 'TestActualHTTPStrictJSONQueryAndGroupCRUD|TestPublish' -count=1`：PASS 1.588s。
- `go vet ./internal/server`、`git diff --check`：exit0。

私有原红日志SHA256 `262376fb1cef3fe053e19c83d6ba05784350b419fa9cf39c718f30f5a403129f`，绿日志SHA256 `bda5f76e38f65581a7933b9b942920ecf9f79fd7bbe63e552e1cd12940a0d6dd`。未变测试断言或普通/发布限额，无待决项。MVP整项目其它检查另记，不将本具体门当全部功能验收。
