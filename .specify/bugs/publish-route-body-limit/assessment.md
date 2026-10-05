# 缺陷评估：发布路由误缩小普通API正文上限

- Slug: publish-route-body-limit（全MVP目标执行自动生成）
- Created: 2026-10-05T05:12Z
- Source: 最终实际Go测试输出，无外部URL
- Verdict: valid
- Severity: medium

## 实际报告

全项目检查的 TestActualHTTPStrictJSONQueryAndGroupCRUD 超过普通1MiB正文门得到400而非原413。

## 症状与复现

使用原HTTP夹具向普通group创建端点发送超1MiB正文；原要求413。当前已提交010/011的publishRoutes先包装64KiB MaxBytesReader才检查是否自己的路由，无关路由被提前截断，readJSON返回invalid400。最终失败日志在私有mvp-final-normal.jsonl。

## 代码与根因

internal/server/publish.go: publishRoutes。判断binding或publishes/publish-queries/applications前即改写r.Body，HTTP链后续接收到不属于本路由的有限reader。置信度高。

## 首选修复

将既有64KiB包装移到确认发布路由匹配之后。保持真实发布请求64KiB、角色和JSON校验；普通API保原1MiB。只改internal/server/publish.go，原HTTP大小边界测试即可复现和防回归；必要Publish相关HTTP检查与vet/diff。无依赖、迁移或权限变更。

## 风险与问题

不放宽发布输入限额，不触碰原节点路由。没有未决产品问题。
