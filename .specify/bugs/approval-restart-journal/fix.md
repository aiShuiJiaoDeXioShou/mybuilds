# 审批重启缺陷修复

- 状态：applied
- 日期：2026-10-05
- 评估：assessment.md

## 实际归因

两次真实复现均只有原审批journal，State=confirmed、StopConfirmed=true、CleanupFailed=false、PendingEvent/Log为空，却出现PendingStop。原完整测试分别22.71/22.07秒失败。ACK回调内立即cancel的确定性Run测试同样真实RED。

ACK后的服务退出使Run收尾调用remote.blocked，将已暂停执行降级为authority_lost。Agent仅识别ErrApprovalPaused，因此误走confirmStopped，改写已confirmed checkpoint。修复只让已经成功返回pause的Run不再依据随后authority取消降级；日志关闭/真实资源回收失败仍正常报错，未确认pause仍失败。测试退出后严格读取原journal，明确拒绝PendingStop污染。

## 追加已确认问题

主代理同时提供原019 extra_junit真实失败。014为允许审批中途旧XML历史而取消了全集严格匹配，但缺少历史资格核验。现集合外XML仅允许原审批canonical摘要、同attempt/Node历史Ref链、原Files中精确ID/元数据证明的旧revision文件；无审批、当前revision额外XML及历史元数据篡改均拒绝。新增纯元数据正负门，不将其冒称中央业务验收。

WindowsARM64审批恢复直接引用Unix资源类型/方法，真实交叉编译失败由主代理记录。恢复登记现在由registerApprovalResource具体平台helper消费；非支持平台固定unsupported，不移植Unix授权或执行实现。

## 修改

- pipeline/run.go：已ACK暂停的收尾保持暂停。
- pipeline/approval_test.go：ACK返回前立即取消的确定性RED/GREEN。
- agent/approval_test.go：真实旧Agent退出后严格读取checkpoint，再启动实际新Agent。
- agent/execute.go、approval_resources_{unix,other}.go：资源恢复登记平台入口。
- agent/reports.go、reports_checkpoint_test.go：原审批历史XML严格资格和正负门。

## 验证范围

只运行本次受影响Run/Agent目标及必要race、vet、WindowsARM64编译。旧全suite由主代理统一执行，不扩大矩阵。完整真实重启使用原20.1秒session安全窗口，未缩短租约/读取未知凭据/删除未知journal。
