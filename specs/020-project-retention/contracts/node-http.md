# 020 管理HTTP、节点事项与CLI

## 管理接口（只admin）

- GET /api/projects/{name}/retention：有效值/字段来源/版本。
- GET /api/projects/{name}/retention/candidates?limit=20&offset=0：只读有界评估页，包含Candidate=false的保留/活动/时间未知解释与全部保护原因；Candidate仅原始数量/时间条件，不授删除权，HTTP/CLI不二次筛页。
- POST /api/projects/{name}/retention：严格JSON `{ "limit": 100 }`，推进有限中央事项，返回实际安全RetentionPage；重复调用不新建已有删除ID，不等待离线节点完成。
- GET /api/projects/{name}/retention/jobs?limit=20&offset=0：有界事项/中央及节点分态/必要清理审计，不包含私有路径。
- 原project set/settings只admin导入项目retention；全局来自server.yml受控重启，非法全块拒绝。触发/approver/Node不能启动管理清理或看其审计；原status与admin/approver普通证据权限不扩大。

原build show安全view增加history_state/terminal_at/cleaned_at可省略字段，cleaned给明确最小墓碑；正文/log/artifact/report读取已退役对象410，原同key批次重放不重新执行。无force/任意路径/管理员解除业务保护API。

## 节点独立事项（只当前NodeActor）

- POST /api/agent/resources：NodeResourceRegistration{Ref,ID,OwnershipDigest,HasWorkspace,HasResults}。ID中性UUID；Ref完整Node/Session/Build/Attempt/Lease/Epoch，新建或扩槽仅以当前running fence核对。已终态只能由当前合法同一NodeActor对已存在且ID/完整Ref/OwnershipDigest/两槽字段完全一致的原请求作只读204确认；不要求过期旧session重新有效，不新建/扩槽/换归属/刷新RegisteredAt或terminal字段，不解除任何guard。HasWorkspace/HasResults固定槽只增加、不换目录；terminal后不可变。网络不含路径/PID/secret，本地私有登记含实际路径及identity。
- GET /api/agent/deletions?limit=10：返回只绑定当前NodeID的NodeDeletion[]{ID,ResourceID,BuildID,AttemptID,OwnershipDigest,HasWorkspace,HasResults}，全部完整ID与数组[]。没有构建Task/命令/执行lease；只有已获当前策略及业务保护复核的事项才可领。
- POST /api/agent/deletions/{id}/authorize：严格空object，当前token、原对象归属和最新政策/保护复核后，返回DeletionAuthority{ID,NodeID,ResourceID,OwnershipDigest,Nonce,ExpiresAt}；固定有效期≤5s，Node从本次authorize请求发送起点计单调期限（≤5s并扣保守余量），响应到达不重新计时；响应已越截止不开始删除段，ExpiresAt仅审计，不靠墙钟延长。授权不复活旧执行权，不授用户脚本权限。
- POST /api/agent/deletions/{id}/confirm：NodeDeletionConfirmation{ID,ResourceID,OwnershipDigest,Nonce,Seq,Digest,WorkspaceState,ResultsState,Reason}，Digest=规范确认JSON不含Digest自身的SHA256；slot states deleted/not_applicable/partial/failed，固定安全Reason；completed要求应删槽均真实deleted且本地确认journal已persist。冲突内容409，同ID同内容幂等200。进度失败确认不改对象/删除ID，后续实际成功需使用已冻结递增确认Seq以区别新内容，不把冲突当重试。

确认消息增加Seq正int64：第一确认1，每次有真实新结果严格+1，重复Seq必须sameDigest；Store保存完整receipt后确认，响应丢失只原Seq/Digest恢复。删除身份固定，Seq不授执行/新路径，不是构建事件cursor。中央不可将失败partial当completed。首次新Seq确认必须绑定该ID已实际颁发的管理Nonce，未授权不能报completed；已确认Seq原Digest重放先取原receipt，不受后续Nonce轮换影响。确认只是保存已执行的真实事实，不重新授权：当前合法身份可补传原ID/Seq/Digest回执，即使原短授权已过期；过期授权绝不允许新增删除段，若确认仍有保护或对象不一致则不宣称整体完成。

每有限删除段前实际authorize，段最多100项且≤500ms并受5sdeadline；下段必须新核对，所有目录/leaf仍身份/type检查，失权或响应不明保存原journal、不开始下一unlink。原data lock和资源私有登记锁覆盖活动Run/管理删除，未知停止/日志阻止执行；旧PID绝不发信号。离线/撤销/墓碑保pending，不转其它节点，身份轮换同NodeID可继续同ID。已知目标确实不存在幂等deleted；未知、权限错或替换固定失败。

严格JSON沿现有unknown/duplicate/null/精确字段名/限额检查，timestamp RFC3339Nano；请求体≤32KiB。普通管理/节点HTTP≤30s，实际单个删除≤30s，ctx/控制运行权失效停止新授权；DeleteSeq溢出拒绝。跨主机verified HTTPS/CA及现有独立token规则不变。

## CLI

- `mybuilds retention show <project> [--json]`：有效策略与来源。
- `mybuilds retention run <project> [--limit 100] [--json]`：一次有界推进，实际分态。
- `mybuilds retention ls <project> [--candidates] [--limit 20] [--offset 0] [--json]`：默认事项，--candidates只评估，不清文件。
- `mybuilds project set <project> --settings ./settings.yml`：沿原命令管理导入；pipeline省略保已有值，显式块沿原规则替换。Retention独立替换覆盖，省略或{}恢复全局继承；retention-only导入不重置已有pipeline。

所有retention命令仅remote读取client config/token，不影响local init/run/doctor/help/version。普通API timeout不替换下载/SSE预算，不在token argv放明文。原Agent serve自动有限推进自己事项，doctor仅诊断data_dir，不删文件、不需token；不新增shell清理或local run目录清理命令。

## 中断资源完成的实际确认

仅物理Stop与HasResults不证明节点无待确认日志。既有POST resources可在中断停止后携带completion{last_event_seq,last_log_seq,last_log_offset,last_artifact_seq,stop_code}；非负int64，StopCode只process_group_reaped，绑定原精确归属和已存在独立Stop记录，当前同Node合法凭据，中央各游标必须精确一致。不能新建/扩槽或更新旧TerminalSeq/Digest。首次完成保存固定CompletionJSON及CompletedAt，精确重放不刷新；冲突/任意pending/不明资源保持保护。

Agent先核实际Stop ACK、零PendingEvent/PendingLog、全部artifact确认、登记及目录identity完整，保存本次固定完成事实/fsync→原POST204→本地确认/fsync，才移除该journal。响应丢失恢复同原stop/归属/游标，无Run/旧fence/旧PID操作。缺completion的interrupted仍resource_unconfirmed；独立Stop可解除execution_unconfirmed，不强求build_finished或重写旧步骤。完成证据只当前真实consumer，没有通用准备状态/未来hook。
