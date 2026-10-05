# 通知预检查验证

- 结果：verified
- go test -run '^TestApprovalNotifyPrecheckBeforeAnyAction$' -count=1 -v ./internal/pipeline
- 真实RED：local返回nil并执行前脚本，remote返回approval_paused，均错；false/skipped四门原绿。
- 修后六门全部PASS。Notify=true在所有动作前固定unsupported、result=nil、无marker、无任何remote事件；false/skipped原能力保留。
- 没有运行重复全套、修改协议或交付通知能力；C原Client/Pipeline整体验证由主代理串行复验。
