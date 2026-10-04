# 006 HTTP契约（已冻结）

标准net/http JSON；仅loopback可直接HTTP，跨主机通过验证证书的HTTPS入口。Authorization: Bearer <token>，不得URL/日志传token。所有路由鉴权；不存在/尚未支持路由404固定安全JSON，不注册Agent/日志/下载/审批/upload路由。对象ID是不含路径分隔符的随机稳定标识，项目/组路径使用安全name。

## 当前路由

| 方法/路径 | 身份 | 输入/结果 |
|---|---|---|
| GET /api/status | admin/trigger/approver | 版本、concurrency、queued/skipped数量；running=0、nodes=0明确未有Agent |
| GET /api/groups | admin | limit/offset；安全组列表 |
| POST /api/groups | admin | `{name}`；201组 |
| PATCH /api/groups/{name} | admin | `{name:新名}`；200组，不允许default |
| DELETE /api/groups/{name} | admin | 仅空普通组；204 |
| GET /api/projects | admin | group/limit/offset；项目公开摘要 |
| POST /api/projects | admin | 注册项目；201 |
| PATCH /api/projects/{name} | admin | `{group}`或`{settings}`其中一个；move或set，块整体替换 |
| DELETE /api/projects/{name} | admin | 有历史/queued拒绝；204 |
| GET /api/tokens | admin | limit/offset；安全身份列表 |
| POST /api/tokens | admin | `{role}`；201，唯一一次返回明文token |
| DELETE /api/tokens/{id} | admin | 撤销；204；同token下一请求401 |
| POST /api/projects/{name}/builds | admin/trigger | 见触发；201首批次，200幂等重放 |
| GET /api/builds | admin/approver | project/group/build_name/batch_id/status/limit/offset，交集过滤 |
| GET /api/builds/{id} | admin/approver | 脱敏详情和步骤进度；不返回内部快照 |

项目注册字段：name、repo、provider（generic/github/gitlab/gitee）、group(default)、branches(main)、nodes必填、default_node可选、build_number_start(1)、settings可选。HTTP file路径通过settings.pipeline.file表达，不接受服务器本地settings文件名。客户端--file与--settings冲突由CLI在发送前拒绝，server仍严格检查请求结构与策略。

项目结果只含id/name/group_id/group/provider/branches/nodes/default_node/pipeline_source/pipeline_file/next_number/UTC时间。repo地址、参数默认值及原管理配置不返回。列表为`{items:[...],limit,offset}`，同一稳定顺序，无无限量查询。

## 触发

必须Idempotency-Key（安全非空<=128字节），JSON：

```json
{"branch":"main","ref":"","build_names":["android"],"all":false,"params":{"version":"1.2.0"},"build_params":{"android":{"channel":"internal"}},"allow_upload":false}
```

branch默认main；ref可省略，仅完整40/64位十六进制commitOID且可达于固定HEAD。build_names/all互斥，单定义可省略选择，多定义必须显式；按显式顺序或all字典序返回。共享参数应用所有所选build，每build命名覆盖优先；未知/未选build命名覆盖、重复字段、参数类型错误均拒绝整批。所选含upload即admin+allow_upload，无论when真假；当前仅queued、不授发布权限。trigger身份即使allow_upload=true也不得发布入队。

结果：`{batch_id,sha,builds:[{id,build_name,number,status,reason}]}`；skipped的number为null且无node，不消耗计数。普通模板所需执行上下文缺失不改变明确true的build.when判定。幂等重放原sha/id/number，不重新解析HEAD；不同内容同key409。

详情：id/project/group/batch_id/build_name/number/status/reason/sha/branch/source/file/source_digest/parameter_keys/condition（判定+安全reason）/initial_budget_ns/remaining_budget_ns/post_budget_ns/steps/post/created_at。无上限普通预算为null。步骤含phase/index/name/kind/condition/reasons/status/elapsed_ns，未开始为pending或条件skipped；post实际未执行为pending。缺少节点不编造node/toolversion/产物/成功。

## 输入与错误

请求Content-Type application/json，body<=1MiB；禁止重复/未知字段、null替代非可空字段、多JSON对象、非法数值/类型；query仅白名单字段，不接受重复scalar，limit1..200、offset0..1000000。不能把JSON decoder弱转换或unknown输入写入日志。

错误统一`{"error":{"code":"固定值","message":"固定安全中文"}}`，不包含原请求/参数值/token/repoURL/DSN/SQL/Git输出。

| HTTP | code | 情况 |
|---|---|---|
| 400 | invalid_request / invalid_pipeline / unsupported_setting | 结构/参数/模板/路径/尚未支持能力 |
| 401 | unauthorized | 缺少/错误/撤销token |
| 403 | forbidden | 角色、分支、发布显式许可不符 |
| 404 | not_found | 当前对象不存在或路由未接入 |
| 409 | conflict | 名称/幂等不同内容/策略变化/删除保护 |
| 413 | request_too_large | body超限 |
| 503 | control_lock_lost | 运行权丢失；不得写业务或恢复旧session |
| 500 | internal_error | 固定安全错误；不可返driver原文 |

鉴权在昂贵Git读取前，身份/角色在Enqueue事务再次验证。只读详情也不显示秘密，CreateToken的单次返回是明确授权例外。安全日志可记录method/固定路由类别/status/随机请求ID，不能完整path/query/body/header或底层错误。
