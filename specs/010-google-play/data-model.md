# 010 数据模型与状态约束

所有实体是当前具体消费者所需的GORM模型，实际写事务使用现有唯一控制端锁连接与提交前再次失锁/fence检查；读公开DTO不包含材料路径、token或脚本。以下设计待008/009/019已提交基线串行实现，不创建占位表。

## ApplicationBinding

ID UUID；Store=`google_play`；AppIdentifier(package)；ProjectID不可变FK RESTRICT；NodeID诊断原节点；UploadCertificateSHA256（公开证书摘要）；AllowedTracks非空规范集合（默认internal）；Status pending/verified；VerifiedAt/CreatedAt服务端UTC；CredentialRef仅完整env名称引用（实际JSON/路径不保存）。唯一 `(store,app_identifier)`，跨项目冲突含pending绑定也拒。改组不改ProjectID。登记必须admin；指定节点当前身份、实际service account应用读权限、工具和初次设置核验后才verified，不靠管理员字符串声明伪核验。

## PublishIntent

ID UUID；BindingID/ProjectID/BuildID/AttemptID/NodeID/SessionID/LeaseID/Epoch完整原归属；StepIndex/StepName固定普通upload；BuildName/SHA/DefinitionDigest/ParametersDigest/Number固定原任务；ArtifactID/Size/SHA256为中央已确认普通AAB；ReportSealDigest/ReportIDs显式[]（没声明reports时digest空、ids[]，不是passed）；VersionName来自原params.version，VersionCode=Number；Track、ExplicitProduction、ChangesNotSentForReview固定授权；Action=`upload`；AuthorizationDigest/GrantedAt；Status unknown/uploaded/processing/submitted/published/failed；ReceiptDigest/RemoteEditID/RemoteReleaseName/RemoteVersionCode/RemoteBundleSHA256/RemoteLifecycle有限安全字段；UpdatedAt UTC。

参数仅digest，不把普通参数值复制公开记录；VersionName/包标识需验证非机密、无控制字符、长度有界，命中本次声明秘密拒绝而不公开。唯一 `(attempt_id,step_index,action)`，精确重复不产生第二动作。IntentID由节点在授权请求前生成并fsync；失去授权响应也知道原ID，不能用未知ID省略terminal证据。最初unknown表示已授权且结果未证，不推断Started；动作内edit→AAB→track→commit阶段摘要逐步写节点journal。Google原意图不能因stage成功而跳过未知commit；011以后单独upload/submit意图不复用Google结果。

ReleaseStatus为Store从冻结upload step派生的draft/completed字符串，省略规范化completed；不是授权请求自由字段。该值连同Track/ExplicitProduction/ChangesNotSentForReview进入AuthorizationDigest及原grant，具体publisher必须核实际track更新响应匹配该选择后才能声明TrackAccepted。不一致不能继续commit或接受另一选择的receipt链；retry与query不能改变原意图选择。

## ApplicationGuard

BindingID唯一FK；IntentID唯一FK；CreatedAt UTC。无TTL，不随lease/heartbeat/StopConfirmation/构建terminal清除。授权同一事务验证binding、原build与完整Ref、生效step/未开始其它发布、剩余ns>0、原AAB、中央019seal与上传许可，insertIntent+insertGuard后返回。20并发真实双库≤1grant；SQL失败rollback无命令。提交前再验原ExpiresAt与控制端锁，不能用等待前已读时间。

重放相同授权请求409，**不再返回可执行grant**；原状态由只读精确接口取得，原grant响应丢失已unknown，节点不跑命令。节点存grant/once标记fsync失败不执行；保存失败不等于central可删guard。相同app不同step、build、项目、节点均guard冲突，retry不自动替代动作。

## PublishReceipt

IntentID唯一；AuthorizationDigest；完整Ref；ReceiptDigest=canonical JSON摘要；CommandStarted/StopConfirmed/CleanupFailed及MutationStage；ResultStatus；Remote证据结构；EvidenceCode；At服务端UTC。网络回执≤64KiB，消息数组[]，原秘密/URL/路径/raw错误拒。精确重复幂等，冲突409；当前fence写提交前复查。过期Ref不能补写，改由admin显式query/confirm，不借008readonly receipt续权。

可信回执须AABupload返回Code/SHA匹配、目标track只包含授权版本、commit成功且原release标记关联，才uploaded；RecordPublish只写该原receipt，不清guard，step_finished同事务完整核对该step全部动作、StopConfirmed且!CleanupFailed后才清对应保护；任何unknown继续持锁。process exit0无此回执仍unknown。已可信完成上传的原记录由GET精确匹配后可submitted/published；unknown缺hash证据不自动解除。若写链未明确完成或停止不确定，guard保留；StopKnown可以独立确认本次进程停止，与ResultStatus unknown并存。

failed仅有限EvidenceCode对应**真实结构证据**：受控publisher完备回执证明未调用任一发布写请求，或Google具体操作明确未接受且之前没有已接受/未知提交；拒绝泛用network_error/timeout/nonzero/404/empty-query。保守不确定一律unknown。管理员有外部证据确认failed可沿下述审计路径，不能修改工具退出值冒充证据。

## PublishQuery与PublishDecision

Query ID、Kind（doctor/query）、BindingID、IntentID（doctor为空）、RequestedBy、OriginalNodeID、CurrentSessionID、Nonce、ExpiresAt（30s管理期限）、Status pending/completed/failed、安全Evidence和CreatedAt/CompletedAt。只有admin创建显式绑定doctor或query；绑定原节点当前身份和原app，不另选节点、不Claim构建，容量可满仍处理有限管理查询（每节点同时1，最多1个待处理请求/绑定或意图）。Node当前token/revoked/disabled、nonce/expiry核对后保存结果，同样失锁事务拒。超时unknown不释放guard，旧过期nonce回报不能更新。节点只GET发布summary，禁止insert/delete edit或其它写API。GET没有AAB摘要，unknown通常只得到ObservedLifecycle/远端版本证据，仍unknown供管理员确认；已可信完成上传的原记录可精确绑定后推进已证实生命周期。

决定ID、IntentID、DecisionKey（UUID，原操作者+键唯一）、ExpectedIntentDigest、Outcome、EvidenceCode、EvidenceNote（≤2048B，控制字符/机密拒）、EvidenceSHA256/有限远端标识、ActorID/CreatedAt。query自动确定也保存同型审计；人工必须admin、提供原digest与真实外部依据。事务再读原guard/状态：同决定返回原样；与已有确定决定冲突拒；精确intent更新且只删这条guard。`stop_confirmed`/exit/no_result不合法failed依据。应用继续使用不改变历史证据。

## 状态转换与构建关系

| 原状态 | 可信输入 | 新状态/guard |
|---|---|---|
| 未授权 | 全部前置与短事务成功 | unknown，持guard后给一次grant |
| unknown | 完整bundle+track+commit回执 | uploaded，中间receipt不释放；原upload step_finished完整核对后释放guard |
| unknown | 不足/空query、失联、停止、retry、进程错误 | unknown，guard不变 |
| unknown | GET不足仍unknown；有依据admin精确决定 | 仅决定后为已证实适用状态/failed，审计后释放对应guard |
| uploaded/processing/submitted | 显式query实际生命周期 | 仅证实状态更新；不再上传 |
| 任意确定态 | 冲突决定/旧fence/其它intent | 拒绝，无覆盖 |

`PUBLISHED`表示原目标track可用，不宣称internal已production；`NOT_APPROVED`是审核拒绝，不能推成上传未发生。状态不强制假单调，因为远端后续撤回/审核等可变化，保存观察时间/原始有限enum和每次审计；查询不足不降低已证实原动作结果。

build status/Reason仍由Run决定：publisher knownfailed→failed，unknown→failed/reason publish_unknown且真实停止后照原failure/always；原已发生run失败不再publisher。post不放行/重跑发布。用户cancel在有效Authority内可always，失权/日志或journal失败闭锁全部后续动作；所有publisher/preflight/auth/回执耗时计ordinary累计预算，预算0不得新动作。独立query不扣旧build预算，也不取得upload权。

008完整build_finished manifest新增本attempt发布意图ID/回执digest与unknown状态集合，Store核对确切step和全部intent，不信Agent给“已发布”。完整停止终态可StopKnown以释放本地执行证据，但guard仍中央存在且关联证据不能清；retry新执行不得沿用旧Intent/Receipt。

## 保护与删除

ApplicationGuard存在及其原Intent/Receipt/Decision关联的build、AAB、JUnit原XML/seal和RetryOf父关系都为实际020保留根。无已交付020时先不新增清理入口；未来实现以真实表查询闭包，并在删除前同一锁事务再次核对guard，不能只按build terminal。守卫、保留和download竞争沿已有files/短DB fence。binding有历史intent时不可删除/跨项目重绑；不dropFK改关系。
