# 019 节点事件、中央文件与CLI契约

## 现有HTTP保持

- POST /api/projects/{name}/builds：沿原Trigger固定SHA/权限/参数/when流程，去除reports专用unsupported后仍严格校验原schema，不新增入队入口；Store.Enqueue也必须接受已验证Reports。
- POST /api/agent/events：原NodeActor/Ref/Seq/Digest/完整receipt。新增reports_checked与reports_sealed的具体校验，其余事件不假称report为step。
- PUT /api/agent/artifacts/{id}：原有界二进制传输及X-Mybuilds-Artifact声明，purpose=junit携带ReportRevision/ReportKey，限原8KiB声明；原artifact Purpose省略。stage安全hash/fsync、server从稳定stage解析、排他发布、短DBfence CommitArtifact。junit实际语义及count必须由server验证，HTTP body不允许独立“verified summary”。
- GET /api/agent/artifacts/{id}：原完整Ref查询。回执丢失只找同fence/ID/Revision/Key/Size/SHA的当前完整确认元数据，不信任2xx或只存在文件。
- GET /api/builds/{id}、GET /api/artifacts/{id}、GET /api/artifacts/{id}/{name}、GET /api/builds/{id}/artifacts：原权限/安全元数据/受限下载；公开报告只取当前sealed集合，与原XML摘要对应。

所有旧purpose空消息编码保持旧digest。新的Reports/ReportManifest指针omitempty，不加字段到无配置旧事件；新字段的nested JSON仍严格拒未知/重复/null/大小写变体。At保持既定UTC RFC3339Nano。

## checked → 完整文件 → sealed → post/terminal

1. 每个已真实Started并StopConfirmed、无CleanupFailed的ordinary run finished之后，报告consumer保存有界snapshot和pending journal，再发送reports_checked。Index/Name/StepKind对应刚完成run，Revision=前一+1，Sealed=false；无法采集时failed固定reason，不假报passed。
2. Store从冻结Definition确认reports存在/required，SourceIndex/Step必须真实ordinary run已finished/Started/StopConfirmed；Path/Key/UUID唯一且合法，Counts整数有界，各File.Counts之和等于Evidence.Counts，Diagnostics有限。当前reported failed禁止下一ordinary intent。缺失只pending，准备步骤不误required失败。下一run前必须完成上一个run checked回执。
3. 普通所有steps terminal后，再一次final checked，Index0；验证零动作不得借此制造seal。最新数据仍按路径替换，消失/旧文件不计入；required在此决定结果。记录ReportFinal=true，之后禁止新增ordinary intent/改revision。
4. 此最新final revision才允许PUT purpose=junit；CommitArtifact对照已经声明的ReportFile、真实run来源及fence，server重新解析Result.Counts与File.Counts一致，固定byte Size/SHA且对应同XML。原所有用途总文件/size/seq限制不变。SQL失败的已发布candidate为不可见孤立文件，不伪确认/不删未知文件。
5. reports_sealed同一Revision/内容，Sealed=true；Store逐ID找完整文件、同purpose/revision/key/source，取server VerifiedJUnit重算Counts/Diagnostics并核对；缺文件/旧fence/冲突/summary-only拒绝且cursor不前进。required=false缺失可有显式空Files与Counts0；非法/secret文件不上传，仅fail-closed固定失败，不把它们列成可信原XML。若剩余budget耗尽，不能改为passed；失败结论可记录安全原因，但不虚构未确认文件。
6. 成功seal后才post_selected。原ordinary失败/timeout/cancel优先，不被报告Reason覆盖；原成功且Reports failed转failure，optional missing可保持原成功。pending/unsealed拒任何用户post；Authority/持久化失败仍沿007闭锁不开始always。
7. build_finished含ReportManifest{SealDigest,IDs[]}；与当前sealed的精确final IDs（允许空数组）及server聚合结果对应；LastArtifactSeq包括所有用途，ArtifactSteps仅实际artifact步骤。所有metadata/ID/日志cursor仍须完整。原precheck/零动作/全条件跳过不要求假seal；未配置ReportManifest必须省略。

## 时限与竞争

report本地扫描/快照/解析合计≤10s且合并原Authority与剩余ordinary预算；原XML PUT沿007上传≤2m但实际总deadline=min(2m,ordinary剩余)，seal回执也计ordinary。0预算拒启动、普通结果为timeout。继续使用既有≤1s读deadline唤醒/执行权检查、原expires提交前再次核对、Connection close及有界错误drain，不新增第二组网络执行器。

disabled/revoked/rotated/过期/ctx失权时保存真实journal与清理保护，不发送旧成功或新post。旧seq相同digest返回原receipt，不同digest冲突；报告checked/seal回执丢失可在同租约按原pending重发，不重新收集或换ID。Agent重启仍遵循007/008原恢复门，不强行清journal。

## CLI

不新增report命令：mybuilds run结果JSON含reports；build show --json/文本展示Counts/Outcome/Reason/实际seal，artifact ls显示当前报告purpose及原XML大小SHA，artifact download使用原ID与私有--output排他校验。logs仍日志，不作为report summary运输。admin/approver可读，trigger及node token不可访问用户证据。节点离线中央已确认XML可读，坏内容/摘要/短流/输出存在拒绝。

approval/upload/商店恢复尚未实现，不因报告passed放开入口。报告字段是封存证据，未来审批/首次上传只核验同执行seal与实际文件，不能因post变更重采或偷换。

## 2026-10-08 数量及消息容量

max_files默认256、显式1–1024；普通制品128份与报告分别计数，合计4GiB/报告累计64MiB不变。events请求和审批checkpoint摘要、claim恢复响应、CLI构建/审批响应最多8MiB；events严格JSON最多65536节点，普通管理请求仍1MiB/10000节点。执行journal64MiB/100万节点，秘密/resources/deletions/spool原限额不变，旧fence/完整回执/原预算保持。多个大报告列表可用--limit 1分页。
