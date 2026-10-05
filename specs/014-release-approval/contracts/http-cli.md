# 014 HTTP与CLI候选接口

沿既有认证、有限strict JSON、角色、错误码与verifiedHTTPS/CA；不新服务/执行器/通知依赖。此页仅规划，当前007无这些路由/命令。body≤64KiB；未知/duplicate/null/类型错/多对象/control拒绝，time用既有严格RFC3339Nano校验。

## 用户入口

| HTTP | consumer/权限 | 行为 |
|---|---|---|
| GET /api/approvals?project_id=UUID&state=pending&limit=20&offset=0 | ListApprovals，admin/approver | project_id/state/offset可选；state只pending/approved/rejected/cancelled/resumed，limit1..100、offset0..1000000 |
| GET /api/approvals/{approval_id} | GetApproval，admin/approver | 安全checkpoint、原build/step/evidence、预算/决定审计 |
| POST /api/builds/{build_id}/approve | DecideApproval，admin/approver | 精确approval_id/revision/checkpoint_digest/note；路由决定approve |
| POST /api/builds/{build_id}/reject | DecideApproval，admin/approver | 同精确字段，决定reject |
| POST /api/builds/{build_id}/cancel | 既有Stop，admin | 安全paused直接CAS terminal，无post；新running沿实际stop |
| GET /api/builds/{build_id} | 既有安全BuildView角色规则 | current_approval_id/revision、safecheckpoint、resume_reason与历史证据 |

请求示例仅结构，不用户机密：

```json
{"approval_id":"<exact-uuid>","revision":1,"checkpoint_digest":"<exact-64-hex>","note":"核对本次封存证据后批准"}
```

精确同审批/同决定/同note响应200原结果且不二次审计；创建决定也200。不同内容/旧current approval/终态冲突409；错误固定invalid_input/forbidden/not_found/conflict/database_error，不回显原body、脚本或值。读取不授批准，trigger/node身份决定403，node token不转user。

客户端候选：

```bash
mybuilds approvals --project-id "$PROJECT_ID" --state pending --limit 20 --json
mybuilds approve "$BUILD_ID" --approval-id "$APPROVAL_ID" --revision "$REVISION" --checkpoint-digest "$CHECKPOINT_DIGEST" --note '已核对本次证据' --json
mybuilds reject "$BUILD_ID" --approval-id "$APPROVAL_ID" --revision "$REVISION" --checkpoint-digest "$CHECKPOINT_DIGEST" --note '证据不满足要求' --json
mybuilds build show "$BUILD_ID" --json
mybuilds build cancel "$BUILD_ID" --json
```

表格/JSON沿现有client配置/token受限文件或env、HTTPS/CA、单次timeout及安全错误。决定各精确字段required，不默默show/latest补齐，不自动重试写请求；丢响应用户可携原同内容重发。notes不能包含声明秘密/材料，外部控制字符/超限/非UTF8拒，原证据IDs/hash安全显示不下载未知路径。CLI本地run/init/doctor/help/version仍不加载remote凭据。

## Agent入口

沿POST /api/agent/events提交approval_checkpoint、原claim/session/heartbeat/renew。新增POST /api/agent/approval-checkpoint严格body是checkpoint-protocol的ApprovalCheckpointLookup，输出精确只读ApprovalCheckpointReceipt。授权只当前NodeActor且same NodeID、未撤销/disabled，旧Ref即便失去租约仍可验证已确认pause；不存在/不匹配409，不授执行、不renew、不submit旧event。

Claim现有route返回optionalTask.Resume；无Resume普通行为不变。合法approved grant保持同build/attempt，新session/lease/epoch；Agent使用原workspace，所有新动作走当前Authority。旧log/file/report source历史保留；新message的omitempty additions旧nil省略。未知checkpoint或未知领取不能自动newRun；原localtask cleanup/renew join未结束不二次claim。

## 本地CLI

mybuilds run沿已有config/params/step/dry-run，无--yes/默认批准、无remote决策token。只有生效approval且无生效upload、notify时，真实TTY输出安全build/step提示，输入明确yes或no；空/非法/EOF/取消均不批准。≤256bytes，有限poll依ctx、TTY验证，无reader goroutine泄漏。预算计时暂停输入等待，不暂停父ctx；后续复核/执行继续原NS。拒绝/无TTY/cancel不执行后续或用户post，但系统Close仍独立预算。

local生效upload/notify在整批预检查动作前拒；notifyfalse默认。dry-run仍纯Preview不创建approval、读input/secret/材料或网络；when skippedapproval不互动、不产生记录，后有效upload在远端因无真实批准拒。

## 当前入口兼容

批准不是Store app guard解锁、不是Artifact/JUnit真实性替代。controller ordinary WriteTimeout不能打断既有SSE/download独立预算；新增管理请求沿普通有限timeout，不延长stream budget。无新通知发送或外部审批消息；014依赖真实008/010/011/019，012/custom/020后续联验不反向形成提交依赖。


## 当前实施修订（2026-10-05）

本功能已获准在fresh2602094+Root实际共享baseline完整实施，不再停留007规划基线。签名资源消费005冻结真实组件，发布消费Root/B当前实现；合法Apple/商店实际上传与最终Flutter案例人工待验，不能用自产签名或脚本成功冒充。必要自动门按安全/事务/恢复/权限/容量/预算/原文件/TTY实际路径验证，不因缺材料留下空实现。当前A独占本WT，Root原event/reports和最后发布接线独占。
