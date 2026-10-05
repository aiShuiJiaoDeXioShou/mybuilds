# 最后审批恢复post预检查遗漏

- 来源：B实际两审批Agent构建已succeeded/epoch3，但always脚本未执行。
- 路径：runPreparation.build只按剩余ordinary求anyActive；NextOrdinaryIndex=len+1时旧已启动run被跳过，导致post沿默认skipped。
- 修复：resume时按原已确认Evidence.Steps中ordinary、合法旧index且Started置anyActive。旧动作仍由restoreApprovalSteps严格验证/恢复，不重跑；executeBuild仍以实际restore.started决定是否执行post。
- 验证：两次真实Run pause→resume，最后仅剩post；已启动run执行一次/always一次。条件全跳过/纯审批不运行post。
