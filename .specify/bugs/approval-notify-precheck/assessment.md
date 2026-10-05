# 审批通知预检查遗漏

- 来源：主代理C实际Client/Pipeline新负例；014 FR027/go-api禁止尚未交付的生效notify在任何用户动作之后才失败。
- 路径：internal/pipeline/run.go runPreparation.step。当前生效approval直接返回prepared，Notify字段被忽略。
- 影响：前置marker脚本已运行后才出现approval_requires_tty；remote也会挂起不存在的通知能力。
- 修复：在已有条件求值及skipped早返回之后、approval返回之前，仅Notify!=nil且true返回固定unsupported。false/nil及条件skipped保持现有行为，无新增通知实现。
- 验证：实际local/remote预检查无marker；false与skipped真实Run仍正常；只目标检查。
