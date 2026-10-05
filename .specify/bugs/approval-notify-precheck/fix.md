# 通知预检查修复

- 状态：applied
- runPreparation.step在真实when已求值/skipped返回后，Notify=true固定unsupported。
- 新增TestApprovalNotifyPrecheckBeforeAnyAction：local/remote各true、false、skipped六门。true此前真实RED，前marker动作已发生；false/skipped此前均绿。
- 不接通知能力，不改CLI/协议/审批runtime，不重复全套。
