# 007 数据与状态模型

字段与函数只在[Go契约](contracts/go-api.md)定义；本文件规定关系、约束与状态。节点token/任务快照/StorageID不进入公共DTO。

## 实体关系

- Node为永久唯一名称的身份；NodeCredential独立于用户Identity，保存摘要/撤销，NodeSession保存每进程session与实际报告。Node软删墓碑，名称不复用，历史与项目引用保留。
- Build扩展当前fence/cancel/stop保护/ns预算/cursors；Attempt独立保留归属历史、claim key、CredentialID/到期和停止证据。007仅首次attempt，无自动retry/迁移。
- StepProgress记录真实phase/index、intent/Started/回收/结果与ns耗时。ExecutionReceipt保存每个已接受事件seq/digest/固定ACK，不能只保存最后摘要，不存run/env/原日志。
- LogChunk与Artifact仅存不可变完整文件元数据；server生成私有StorageID。各自sequence独立，不构建通用事件仓库。
- StopConfirmation保存精确原fence、确认身份/UTC/固定证据类型/依据，只解除物理保护。
- AgentJournal为本地0700/0600有限记录：完整fence、快照摘要、intent/实际PID/PGID、回收、预算、pending进度、日志/产物确认位置。原子替换+file/directory fsync；重启不继承执行、不按旧PIDkill。

## 约束

Node name永久唯一；credential digest唯一，token只显示一次。未连接/未领取的SessionID/AttemptID/LeaseID/NodeID/到期使用NULL，不能给UNIQUE键填空字符串。Epoch非负default0，递增溢出拒绝。project_id/name为NOT NULL。

同名部分唯一索引`UNIQUE(project_id,name) WHERE status='running' OR stop_unconfirmed=true`；global/node容量计同集合。索引条件不依赖now。建(status,created_at,id)、(node_id,status)、(status,lease_expires_at)与实际分页/核对所需索引。receipt/log唯一(build,attempt,seq)，artifact ID全局唯一与attempt/seq唯一，build FK RESTRICT。

Claim短事务取快照允许节点与当前项目授权交集，校验enabled/健康/无guard、实际平台工具、admin标签、两级容量与同名互斥，queued条件更新后产生完整fence。runner=nil仅默认节点，无默认远程触发拒绝。Android实际工具，ios当前签名能力不足。提交前再验锁/UTC/归属；Renew须oldExpires仍有效，now==expires拒绝。

## 状态

| 输入 | 状态与保护 |
|---|---|
| when false | skipped，无号/租约。 |
| queued取消 | cancelled，不执行post。 |
| Claim | queued→running，占容量/同名。 |
| 用户取消 | running保持占用，CancelRequested=true；公开effective status=cancel_requested；有效lease允许既定always。 |
| 完整实际回执/日志/产物确认 | succeeded/failed/cancelled；没有未确认动作且cleanup成功才释放。 |
| 过期或停止不确定 | interrupted+stop_unconfirmed；同名/容量保持，节点派生quarantined，不迁移。 |
| 精确停止确认 | 仅清stopflag，记录依据；保留原状态/原因，不恢复过期成功/未来外部unknown。 |
| controller重启 | 保留仍有效lease，核对到期；不全量interrupt，不重置预算。 |

动作phase沿006 ordinary/success/failure/always、index从1开始；build_finished/post_selected非步骤phase空/index0。步骤pending→intent→实际started→finished，全部ordinary条件跳过时终态skipped/condition，不执行post。未收到Started回执不能证明没有执行。真实post选择记录在事件中，不由最终Status推断；未选post标skipped/not_selected。普通失败原因保留，post单独留证，post失败仅把原成功改failed/post_error。失权/保存失败全部post禁止。

drain只停新Claim；disable/撤销禁止旧续租/事件，保护保持到停止或到期；enable不绕guard。Delete检查running/等待/guard后软删，历史终态不永久阻止rm。

## 文件确认

服务端短鉴权→事务外stage/hash/fsync→排他发布新StorageID→短事务末尾再验原租约并提交完整meta→ACK。只有DB完成记录可下载；DB失败孤立文件不可见。同ID同内容返回canonical并删除本次多余候选；冲突不覆盖。007不自动retention/恢复过期结果。

Agent日志脱敏→spool fsync→序列提交→ACK cursor fsync→删确认前缀。满/失败停Run，未确认记录保留。terminal前flush全部已接受日志/实际产物；terminal后旧租约重复也不恢复写权。
