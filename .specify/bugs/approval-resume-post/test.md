# 最后审批恢复post验证

- 结果：verified
- 命令：go test -run '^TestApprovalFinalResumePreparesPostFromStartedHistory$' -count=1 -v ./internal/pipeline
- 修前：历史普通run真实启动后最终post未执行，RED；全普通条件skipped与纯approval原GREEN。
- 修后：三门全部PASS；三轮真实Run两次pause/resume，最后only-post；前脚本执行一次/always一次、未重发ordinary，skipped/纯approval仍无post。
- 不重跑全套，原B真实Agent两审批case由主代理集成再验；未改协议/恢复权限/执行器。
