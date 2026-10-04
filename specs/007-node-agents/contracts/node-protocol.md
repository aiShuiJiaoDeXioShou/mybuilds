# 007 HTTP / 文件 / 时序契约

消息字段唯一见[go-api](go-api.md)。沿006严格JSON：拒unknown/duplicate/null/错误大小写/多文档/type/control输入；不公开底层错误。节点专用Bearer与用户Bearer独立，不互授权限。控制端锁检查失败停止所有授予/回报。

## 节点接口

| 路由 | 消息/结果 | 行为 |
|---|---|---|
| POST /api/agent/session | SessionRequest→SessionGrant | 节点独立身份，握手策略一致；新session不能绕旧running/guard。 |
| POST /api/agent/heartbeat | HeartbeatRequest→SessionGrant | 更新actual report/健康，不续执行lease。 |
| POST /api/agent/claim | ClaimRequest→LeaseGrant或204 | 同claim key不重复授予；只当前session。 |
| POST /api/agent/renew | LeaseRef→LeaseGrant（Task=nil） | 完整fence/未到期/原凭据有效，返回持久CancelRequested。 |
| POST /api/agent/events | ExecutionEvent→EventAck | 连续seq/digest/合法步骤与预算，末尾复核。 |
| POST /api/agent/logs | LogChunk→LogAck | canonical Records文件/DB complete后确认，seq+offset去重。 |
| PUT /api/agent/artifacts/{id} | ArtifactDeclaration头+binary→ArtifactView | ID/声明与当前fence绑定，流式完整校验，最终DB再验。 |
| GET /api/agent/artifacts/{id} | 当前LeaseRef query→ArtifactView | lost response用当前lease查询本ID；无记录404；不读其它attempt。 |
| POST /api/agent/stop-confirmation | StopConfirmation→204 | 独立当前node身份，精确原attempt，不能代替执行result。 |

Artifact声明用单个`X-Mybuilds-Artifact` base64url canonical JSON头（≤8KiB）；禁止重复头、Transfer metadata路径、压缩/Content-Encoding，Content-Length必须等于Size，逐字节流到server自有stage。GET只允许完整LeaseRef六字段，不接受路径、token参数。请求错误固定401/403、非法400、冲突409、限额413；过期/旧fence409 lease_invalid/lease_expired；成功幂等200，无业务rawerror。

## 用户/管理员接口

| 路由 | 权限/结果 |
|---|---|
| POST/GET /api/nodes | admin；NodeInput→NodeCreated一次token /分页NodeView |
| GET/DELETE /api/nodes/{name} | admin；NodeView /软删204 |
| POST /api/nodes/{name}/{drain,enable,disable} | admin；204 |
| POST /api/nodes/{name}/token/{rotate,revoke} | admin；一次token或204 |
| GET /api/nodes/{name}/doctor | admin；最近actual工具+healthy/quarantine/journal诊断；离线明确failed，不虚构当前执行 |
| GET /api/doctor | admin；具体Store锁/可达性/容量/节点安全摘要 |
| POST /api/builds/{id}/cancel | admin；事务保存意图，返回安全BuildView，未回收不能报cancelled |
| POST /api/builds/{id}/stop-confirmation | admin；StopConfirmation精确原fence+实际Note，204 |
| GET /api/builds/{id}/log | admin/approver；step可选、after_seq≥0、limit1–200、follow=0/1 |
| GET /api/builds/{id}/artifacts | admin/approver；分页ArtifactView |
| GET /api/artifacts/{id} | admin/approver；安全ArtifactView，download先取Name/Size/SHA，不公开StorageID或路径。 |
| GET /api/artifacts/{id}/{file} | admin/approver；file必须等于View.Name安全leaf，完整binary，Content-Length与X-Content-SHA256 |

节点/trigger用户不能读取这些日志/产物。007不注册retry/approve/upload-resolution/清理/通知路由或stub。未声明runner又无默认节点的远程触发失败；runner不支持/暂时无匹配节点保留queued固定原因，不能随意默认到另一node。

## 明确上限

- 管理/握手/事件JSON≤1MiB，事件单个，步骤总数沿原配置上限；fence UUID、epoch正int64、seq正int64（溢出拒绝）。labels≤64每项≤64bytes，工具≤32，每Version≤128bytes/Reason固定码，node名称1–64安全字符。
- 日志body≤64KiB，Records≤16、单Text≤8KiB，stream=stdout/stderr/system；每attempt未确认spool≤16MiB/4096记录，node总未确认≤64MiB/8192记录。到限返回log_backpressure并停止，不能drop。确认cursor写盘后才能释放。
- artifact每文件≤1GiB、每attempt≤128文件/累计4GiB；Name安全leaf≤255bytes（无控制字符/分隔符/点目录），SHA256 lowercase64hex。超过限制明确artifact_limit，不能静默只传一部分。source/snapshot路径不作为下载路径。
- 本地/中央stage和journal普通文件、叶子无symlink，非阻塞打开后核对regular/owner/mode；结果/工作目录0700，材料/journal/logs/产物0600，不读取FIFO或host未知凭据。server/node不接受任意对端路径。
- 普通API≤30s（client已有timeout允许更早）；Agent请求≤min(heartbeat,当前权限剩余)，metadata/body读取有界；artifact单流≤2m且受当前Authority截止，续租独立并发。下载≤10m；工具单项≤15s/合并stdout+stderr≤32KiB，doctor整体≤1m。

## 策略与权限期限

server可信heartbeat_interval默认5s、lease_duration默认30s：heartbeat1–30s、lease10–180s、lease≥4*heartbeat+2s。Agent同字段必须与SessionGrant匹配，不一致握手明确policy_mismatch、不能执行；不协商放大TTL。Grant.TTLNS来自服务端Duration。

Agent记录请求发起时time.Now（保留单调部分），deadline=requestStart+TTL-max(2s,heartbeat)。ACK到达时若deadline已过或Authority已经关闭，拒绝复活。每次有效Renew只在当前Authority未失效时替换本地deadline；并发续租串行、带当前fence，不接受旧请求ACK回退/覆盖新值。健康窗口3*heartbeat；session替换窗口2*lease且必须no running/no guard。用户Cancel不取消续租/回传Authority；disable/撤销/expired403/409立即失权，网络错误最多等待当前保守deadline。

控制端核对ticker≤1s、重启读现有到期，仅过期项interrupted+guard；绝不把有效lease全中断。旧session/credential/fence回报被拒，节点独立停止确认仅可清物理保护，不能重获运行/终态权。

## SSE与完整文件

follow=0返回分页Records和next_seq/next_offset。具体JSON使用`server.LogPage{Records []protocol.LogRecord json:"records"; NextSeq int64 json:"next_seq"; NextOffset int64 json:"next_offset"}`，由server生成、CLI专用日志消费者读取。limit是chunk数，历史响应另在完整chunk边界限制≤1MiB（预留JSON外壳），不拆chunk或丢记录；step筛选也推进已处理chunk的next_seq/next_offset，续读从该next_seq开始。follow=1用SSE `id: seq`、`event: log`、JSON data（同LogPage，一个log事件对应一个完整chunk），Last-Event-ID与after_seq不一致拒绝；terminal已按build_finished完整manifest确认且已无新日志发event:end并关闭。每连接默认15m，CLI --stream-timeout=1s–1h独立于普通APItimeout；心跳15s，每次写deadline5s，总时长/ctx取消关闭连接。server解除该响应普通整体WriteTimeout、用ResponseController设置实际单次期限；不支持controller明确失败，不伪流。客户端专用SSE读取，不能走当前1MiB ReadAll remoteRequest；只打印确认后新seq，重连从最后显示seq，单事件≤64KiB，控制字符安全呈现。

stage完整写/hash/fsync→同Root新StorageID排他原子发布→短DBCommit再次fence/到期/锁校验→complete后可见。文件I/O不在长DB事务。DB失败孤立文件不可下载，007无自动清理/恢复；同stable ID响应丢失允许当前lease查询/重发；canonical冲突不覆盖。中央下载打开同fd校验完整Size/SHA后发送，坏/short/非regular失败，节点离线不影响确认文件；客户端stage校验并排他发布，原输出存在拒绝覆盖。
