# 最后审批恢复post修复

- 状态：applied
- 在原anyActive初始化后增加5行，读取resume已确认ordinary旧index的Started；不执行旧步骤、不改post真正started判定。
- 新增真实Run三轮（pause/pause/最终post）测试，覆盖历史run、条件skipped、纯approval。
- 已启动历史动作此前RED，skipped/纯approval此前GREEN；不扩大协议、runtime或矩阵。
