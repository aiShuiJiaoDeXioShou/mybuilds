# 015 HTTP、provider与安全结果

仅在前置接受后实际实施；secret/limits见[config-changes](config-changes.md)，具体共享类型见[go-api](go-api.md)。现有admin/trigger/approver/node鉴权与路由继续；hook有独立凭据，不消费用户/节点Bearer。

## 唯一公开入口

`POST /hook/{project}`只已登记安全项目名，无query/token/重定向。合法Content-Type application/json（可UTF8 charset），先CheckLock→当前enabled policy/material→有限raw→固定provider验证→有限JSON/正规化→Store.ReceiveWebhook；认证或形状错不落event/window/号。providerheader重复、空、混杂伪认证模式拒绝；其它未知非关键headers不记录。

| Project.Provider | 固定事件/认证 | push字段与仓库身份 | delivery证据 |
|---|---|---|---|
| github | X-GitHub-Event=push；X-Hub-Signature-256严格sha256=64hex，HMAC(secret,rawbody)恒时 | ref/after/before；repository.id规范正整数须等repository_key；deleted或零after忽略，created合法新分支push | X-GitHub-Delivery必填；没有它invalid，不降级到generic |
| gitlab | X-Gitlab-Event=Push Hook、object_kind=push；X-Gitlab-Token与独立secret固定长度hash恒时比对 | ref/after/before；project.id须等repository_key；零after删除忽略 | 优先webhook-id，再Idempotency-Key，均存在须相等；没有两者则无ID；Event-UUID有递归复用风险不作本次delivery，Webhook-UUID不是event |
| gitee | X-Gitee-Event为Push Hook或push_hooks、hook_name=push_hooks一致；X-Gitee-Token只显式password模式与secret恒时比对 | ref/after/before、repository.id正整数须匹配；deleted/零after忽略；若payload password存在必须与已认证值一致且只内存 | 官方未证明稳定delivery头，hook_id/timestamp不能当唯一ID；无ID语义去重 |
| generic | 由admin有限Generic定义event/auth，默认X-Mybuilds-Event=push；默认X-Mybuilds-Signature=sha256=HMAC(rawbody)；token需显式 | 配置固定pointer取ref/after/before/repository；payloadURL忽略，repoKey匹配；零after删除忽略 | 默认可选X-Mybuilds-Delivery，声明为空或缺头走无ID；不借别provider别名切parser |

所有SHA为本次git对象格式40/64hex，before若缺可空（provider自身必填语义仍验证），after只删除时允许全零。ref=refs/tags/合法忽略；其它非普通ref拒。正常branch剥一次refs/heads/，mustcheck-ref-format/项目授权；只有当前Repository的branch HEAD/commit才作为最终任务SHA，after只是event证据。仓库ID是admin声明绑定，签名不代替可信仓库登记，也不能从payload Repository/compare URL发请求。

GitHub的ping是可靠连接事件，验证HMAC/有限JSON后Kind=ignored/Reason=hook_ping；provider明确tag/PR其它合法事件hook_event_ignored、分支删除hook_branch_deleted，均持久安全receipt无window。不发明GitLab/Gitee“测试请求”通用字段：UI若发送等同正常push且无可信区别，按普通push处理；quickstart说明可能产生测试构建。未知header事件只按固定recognized family忽略，坏shape不能装成有效push。

GitLab的新signing、Gitee的新加签当前不支持。配置没有两家signing模式字段，unknown配置严格拒绝；signing-only请求认证失败，不因为携带legacy token而自动切认证。GitLab webhook-signature与当前token模式混用拒绝，webhook-id/timestamp本身不是混用证据；Gitee密码模式的timestamp/sign metadata不当成另一种认证，也不能用它们替代正确密码。不存在统一timestamp要求；commit timestamp不是接收时钟/防重放证据。GitHub/GitLab/Gitee地址均部署HTTPS，关闭provider SSL verify不能作为通过门。

## 接收响应

| 结果 | status | 有限JSON |
|---|---|---|
| 首次接收并持久化 | 202 | event_id/window_id/status=accepted（ignored无window_id）/reason |
| 已确认精确重投或body别名 | 200 | 原event_id/window_id/status/reason，replayed=true |
| 合法忽略且首次保存 | 200 | event_id/status=ignored/reason，无window_id |
| 错认证/未启用/项目不可用 | 401 | 固定hook_unauthorized；不区分不存在/禁用/密钥错 |
| 错branch/repository/自动权限 | 403 | hook_forbidden |
| 同delivery异内容/策略竞态 | 409 | hook_conflict；无新号 |
| 超body/JSON/header/资源 | 413 | hook_limit |
| Content-Type/encoding不支持 | 415 | hook_media_unsupported |
| 合法配置之外坏请求/格式 | 400 | hook_invalid |
| 锁/DB/保存失败 | 503 | 固定service_unavailable或database_error，无接收成功 |

公共receipt字段明确event_id、window_id(omitempty)、status、reason(omitempty)、replayed bool；无需返回rawbody/digests/secret或即时BuildID，202只承诺提交接收。关闭结果由鉴权查询读取。provider自行重投仍原delivery/alias，不自动请求商店。

## 管理与查询

沿已有`POST /api/projects`/`PUT /api/projects/{p}` settings导入；当显式hook/triggers块存在，由实际Server.ConfigureWebhook准备私有材料并一次Store.ConfigureWebhook原子写全部块。本机管理员入口也调用该具体Server方法，不能直接Store.SetProjectSettings绕过secret准备。设置与公开provider不可改变仓库；普通pipeline-only旧请求仍沿原CRUD。

`POST /api/projects/{p}/hook/rotate`只admin，空object严格JSON；成功一次200返回WebhookConfigured中的实际自产Secret。外部引用轮换需先更新明确材料并显式hook settings重配，不把host变量当rotate。丢响应后普通GET不恢复旧secret，不自动重发rotate；管理员查当前safe version后明确再轮换。`POST .../hook/disable`只admin，空object，200安全policy，无secret；queued/running不被这个管理动作取消。

`GET /api/projects/{p}/hook`返回WebhookPolicyView；`GET .../hook/events`、`GET .../hook/windows`分页，admin/approver可读，trigger/node拒；管理/生成secret仅admin。ProjectView只新增安全enabled/selection/quiet/allow字段，配置Secret/Generic内部字段不直接序列化。

EventView字段：id、provider、kind、delivery_id(可空)、window_id(可空)、branch/after(可空)、received_at UTC、status、reason、body_digest、receipt_digest。不带CredentialID/SecretFingerprint/Before提交人/URL/完整headers/password。

WindowView字段：id、generation、revision、provider、branch、build_names[]、opened_at/deadline UTC、state、reason、candidate_sha/final_sha(可空)、batch_id(可空)、build_ids[]、reused_build_ids[]；仅安全选择名，无Params/原Definition/Policy秘密。BuildView变更摘要：mode/reason、baseline_build_id/sha、target_sha、path_count、digest，完整Paths只私有冻结执行快照，不公开原工作区。客户端event/windowJSON与table同安全DTO，stdout错误固定、不打印responsebody原文。

Close window failed的reason限Go契约固定集合；不会用HTTP202伪造pipeline成功。list结果不提供“重新关闭/自动retry/批准发布”操作；显式manual trigger/retry继续原入口。

## 接受前置与安全兼容

无hook的旧server/client配置、Trigger request key、008Retry/TerminalReceipt digest、007节点日志/产物协议保持；新增字段省略不产生旧消息变化。无HookMaterials不影响本地命令；错误server hook材料只影响实际启用项目，不读取其它秘密。正式实现后必须复验完整旧管理/trigger/8恢复/审批/报告/商店未知保护，不以payload测试替代provider真实投递。
