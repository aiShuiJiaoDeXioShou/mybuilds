# 010 HTTP、客户端和安全视图

沿既有verified HTTPS/CA、user/node token分离、strict JSON未知/重复/null/类型校验、限流和固定safe errors；普通请求≤1MiB，publish授权/receipt/query≤64KiB。不可把token/JSON材料放URL/query/argv。实际路由如下，复用原007节点身份与HTTP消费者。

## 用户接口

| 方法/路径 | 角色 | 输入/输出 |
|---|---|---|
| POST /api/projects/{project}/applications | admin | BindApplicationInput；202安全ApplicationView pending |
| GET /api/projects/{project}/applications | admin/approver | 安全绑定列表 |
| GET /api/publishes?project={id}&limit=20&after={id} | admin/approver | 按固定ID分页安全发布列表，1..100 |
| GET /api/publishes/{id} | admin/approver | PublishView |
| POST /api/publishes/{id}/query | admin | 空{}；202 QueryView，不阻塞等待/不upload |
| GET /api/publish-queries/{id} | admin/approver | 精确query状态和有限远端证据 |
| POST /api/publishes/{id}/confirm | admin | ConfirmPublishInput；200原记录，相同决定幂等/冲突409 |

binding doctor是POST绑定后的具体远程验证请求，App准备失败status保持pending，不取得upload权。相同app另一project409；原project重复相同binding返回原样，不改group/Counter。现有trigger/retry含upload定义一律admin+allow_upload，falsewhen也不能绕。trigger身份不能读publisher详情，approver只读不能bind/query/confirm。

## 节点接口

| 方法/路径 | 当前node token消费者 |
|---|---|
| POST /api/agent/publishes/authorize | 完整PublishAuthorization→201一次grant；重复409不再给执行grant，原记录只读查看 |
| POST /api/agent/publishes/lookup | PublishLookup完整原Ref/Index/IntentID，当前原node只读NodePublishState；POST语法但无写/续租；活动slot不存在不证明未授权 |
| POST /api/agent/publishes/{id}/receipt | exactRef+originalAuthDigest+boundedreceipt；当前fence、原ExpiresAt提交前重验；同receipt200/冲突409 |
| POST /api/agent/publish-queries/claim | {session_id}→200有限doctor/query task或204；真实原Node当前身份，不Claim构建 |
| POST /api/agent/publish-queries/{id}/result | exactnonce/session/expiry/result；原节点当前token、提交前期限复核；过期409不更新 |

授权响应丢失节点不能重试authorize当执行许可；查看原记录也不恢复command权。readonly管理结果可重发相同nonce/digest直到原30s期限，不延期。disabled/revoked/rotated/session changed或control locklost拒绝新增动作，unknown/appguard不消失。query仅GET远端，服务端没有商店凭据或发布子进程。

## 安全DTO

ApplicationView：ID/Store/AppIdentifier/ProjectID/NodeID/Status/AllowedTracks/UploadCertificateSHA256/VerifiedAt（可空）；不显示CredentialRef/材料。

PublishView：ID、ProjectID、BuildID、BuildName、Number、SHA、AttemptID、NodeID、Action、Store、AppIdentifier、Track、VersionCode、VersionName、ArtifactID/Size/SHA256、ReportSealDigest/ReportIDs、Status、EvidenceCode、有限Remote字段、ApplicationProtected bool、GrantedAt/UpdatedAt、安全核对决定。VersionName须先验证无本次声明secret；不输出Params/Definition/Environment/AuthorizationDigest/privatepath。与build view增加PublishIDs/安全结果omitempty关联；queued/skipped无假publish记录。

QueryView：ID/IntentID/Kind/Status/RequestedAt/CompletedAt/Reason/ObservedLifecycle/Matches；unknown原发布Status不因部分匹配改变，Observed字段清楚区分。发布回执有BundleSHA而GET release summary没有，禁止客户端把Partial Observed变成确认结果。人工Note需有界并拒机密/控制序列，无raw工具/HTTPbody。

固定错误：invalid_request/unauthorized/forbidden/not_found/conflict/database_error及有限publish_*原因；不得拼raw输入。列表表格与JSON共享DTO，错误不打印server response body机密。相同auth不能借更大/多对象/错误大小写或namespace规则逃unknown/null检查。

## CLI

```text
mybuilds project app bind PROJECT --store google_play --app-id APP --node NODE   --credentials-env GOOGLE_PLAY_JSON --upload-cert-sha256 HEX --track internal
mybuilds project app ls PROJECT [--json]
mybuilds publish ls --project PROJECT [--limit N --after ID --json]
mybuilds publish show INTENT [--json]
mybuilds publish query INTENT [--json]
mybuilds publish query-show QUERY [--json]
mybuilds publish confirm INTENT --decision-file PRIVATE_JSON [--json]
```

这些都是remote命令，仅在调用时加载client.yml/env token/CA；坏client配置不影响本地init/run/doctor/help/version。confirm文件自有0600普通有界JSON，键、原intent digest、外部证据，不能用argv直接拼secret/长Note。query自动重试只限用户显式另发核对，不自动上传；CLI丢authorize/触发响应不自动重发写请求。query保留原command一次语义，不newbuild/number。

发布诊断命令 `mybuilds doctor --target google-play --agent-config FILE --app-id APP --credentials-env NAME` 只读节点工具/指定材料和明确GET，既有doctor省略target完全不加载这些配置，也不要求远程token。原build触发 `--allow-upload` 明确显示发布许可，默认internal是安全目标但仍是实际远端副作用，只有用户已准备和授权应用才进入真实验证。
