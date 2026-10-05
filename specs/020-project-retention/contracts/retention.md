# 020 管理配置、排序与结果契约

## 配置与导入

```yaml
# server.yml：仅受控管理配置，启动时同步有效全局值
retention:
  builds: 100
  days: 30
```

```yaml
# 管理员project set --settings导入，非仓库mybuilds.yml
retention:
  builds: 200
# days省略→当前全局；retention: {}或省略块→两个字段继承
```

builds正int64；days正整数且24h纳秒乘法可表示（1..106751）；0/负/小数/string/null/未知/重复/溢出拒绝，整个导入不部分变更。全局缺省100/30，由受限文件更新+受控重启，不热重载；项目设置沿既有管理员原事务，组迁移按稳定ProjectID保持覆盖。不得从仓库/param/Node请求接受。原pipeline省略保持已有值、显式块沿原规则替换；retention独立覆盖，省略或{}恢复继承，retention-only不重置pipeline。

## 筛选与保护

查询当前有效值及global/project来源、两版本。完整终态有可信TerminalAt才计rank，跨全部build含无编号skipped；按TerminalAt DESC/ID DESC稳定排序。候选=(rank>builds)||(TerminalAt<evaluationUTC-days×24h)。排名等号及时间等号不因该维度候选，任意另一维度可独立选中。活动、未知时间和cleaned墓碑不占终态rank；有可信时间但保护的完整终态仍占rank，保护后实际保留可超过数量。

固定保护reason：active、terminal_time_unknown、stop_unconfirmed、execution_unconfirmed、resource_unconfirmed、logs_unconfirmed、artifacts_unconfirmed、reports_unconfirmed、retry_dependency、readers_active、ownership_unknown、state_unknown。实际每个reason来自具体已有记录；未来审批/unknown真实消费者加入后再冻结其reason，不预建虚假字段。业务保护不退役，只有readers_active可先退役新引用然后等实际SH释放。candidate不授持续删除权。

RetryOf保留child→保护parent祖先闭包；同批清理内容child先parent后，保留最小关联行不改RESTRICT。清理不重置TerminalAt/号段、解除锁或确认unknown，不启动执行/post/上传。

## 结果与错误

安全RetentionEntry：ID、Project/BuildID/BuildName/Number、TerminalAt nullable、HistoryState、Candidate、ProtectReasons[]、JobID可选、CentralState/NodeState、Reason固定码、时间。查询默认20/max200，数组显式[]。不公开路径/StorageID/脚本/params/raw日志/凭据/底层错误。已清正文/文件410 code=retention_retired，不返回空成功；原幂等触发返回原最小批次关系及history_state=cleaned。

固定错误：retention_invalid、retention_protected、retention_retired、retention_readers_active、retention_ownership_unknown、retention_object_invalid、retention_io_error、retention_limit、retention_timeout、retention_cancelled、retention_receipt_conflict；鉴权/控制锁失效沿原固定错误。不可用force跳保护。已知目标不存在≠权限/类型/身份不明；只有前者可幂等确认。

每轮最多100候选/100文件对象；SQL事务≤5s、实际单删除≤30s、遍历100000项/深度64，过限保留固定失败，后台每60s推进有界一轮。离线事项不会无限内循环；管理员run只是推进有界轮次并返回实际状态，不等待离线节点无限阻塞。
