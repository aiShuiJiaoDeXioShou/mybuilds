# 缺陷修复：发布路由正文限额

- Slug: publish-route-body-limit（复用本轮assessment）
- 状态: applied
- 日期: 2026-10-05

将internal/server/publish.go的64KiB MaxBytesReader移到路由匹配确认后，普通group/project继续使用自身1MiB上限。原TestActualHTTPStrictJSONQueryAndGroupCRUD原样保留。修前该实际HTTP门RED；修后同门与已有发布API检查、vet结果见test.md。无范围扩张、依赖或数据变更。
