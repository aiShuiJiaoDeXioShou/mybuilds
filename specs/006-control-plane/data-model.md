# 006 数据模型与不变量

所有时间为UTC。具体Store持有GORM，迁移/查询/事务不依赖CLI、HTTP或执行器；公开数据使用专门视图，不能直接序列化内部快照。

| 实体 | 最少字段与约束 | 关系/规则 |
|---|---|---|
| ProjectGroup | ID稳定；Name非空安全标识、<=64字节、全局唯一；CreatedAt/UpdatedAt | 固定default不可改名/删除；projects非空FK且ON DELETE RESTRICT |
| Project | ID、Name唯一<=64、GroupID非空；Repository/Provider；Branches、AllowedNodes、DefaultNode；Settings；NextNumber正int64；PolicyVersion正int64 | repo受信管理员登记，不含密码；default_node属于AllowedNodes；settings源仅auto/repo；下一编号不能溢出 |
| Identity | ID、Digest唯一SHA256、Role admin/trigger/approver、CreatedAt、RevokedAt可空 | 32字节随机token仅创建返回一次，列表/DB无明文；摘要不出公开JSON |
| ControlMetadata | 固定单行ID、IdentityInitialized bool | 首次创建bootstrap或本机token时原子置true；撤销不清除，不凭token表为空复活 |
| Audit | ID、ActorID（本机为local-admin）、Action、ObjectID、From/To非敏感身份、CreatedAt | 改名/迁移/身份管理记录；移到当前组不产生虚假记录；不记录token/DSN/repo秘密 |
| Batch | ID、ProjectID、IdentityID、SHA、Branch、Source/File/SourceDigest、CreatedAt | 一次请求固定同一SHA，关联选择顺序；不增加批次执行器 |
| Build | ID、BatchID、ProjectID、Name、Number可空、Status、Reason、SnapshotJSON、InitialBudgetNS可空、RemainingBudgetNS可空、PostBudgetNS、CreatedAt | 项目+非空Number唯一；006只有queued/skipped；skipped无号/节点；queued无节点且不会伪成功 |
| StepProgress | ID、BuildID、Phase ordinary/success/failure/always、Index、Name、Kind、Condition/Reasons、Status、ElapsedNS | build+phase+index唯一；ordinary初始pending或条件skipped；post尚未执行为pending；不记录虚假started/finished |
| Idempotency | IdentityID+Key唯一、RequestDigest、BatchID、CreatedAt | 同身份同key同内容返回原结果；不同内容冲突；与批次/编号同一事务提交 |

## 快照

具体共享store.BuildSnapshot由A定义、B准备，A事务编码成SnapshotJSON；Definition config.Build、Params/Facts、Condition/Reasons、AllowedNodes/DefaultNode字段固定，无需猜JSON、不import server/pipeline。Snapshot JSON保存被选中的规范化config.Build、最终普通参数、固定project/name/git事实、条件输入/判定/理由、runner/节点授权策略与来源摘要。保留模板原文与秘密引用，node/workspace尚未就绪不填假值。系统编号/ID由事务分配并成为后续执行事实，不能参数伪造。配置原文、definition、普通参数值与秘密引用都不经公共视图输出；真正密钥不得落库。

凭据专用字段在远程排队仅接受完整合法`${NAME}`引用；通知能力未实现时有效通知拒绝，enabled:false可显式关闭且关闭后的敏感目的地不写入快照。普通params为非密钥字符串，env普通字面量不被系统当成可公开值；秘密使用明确引用。run/argv正文保留原文不插值、不显示。

sourceDigest为固定Git blob原内容SHA256；snapshot保存确定JSON内容以供后续一致性核对。没有Agent时不创建Lease/Node/Upload/Approval表或虚构执行记录；后续对应功能实际接入时迁移扩展。

## 输入限制

- 名称：安全稳定标识，非空、无路径分隔符/控制字符；项目/组/节点名<=64字节。
- Branch：必须合法Git分支名且匹配项目branches；Ref只完整SHA1/SHA256 commitOID，不接受短SHA/tag/rev表达式。文件路径为仓库相对普通路径，拒绝绝对、反斜杠、盘符、控制字符、任意`..`段及glob。
- 请求体和配置<=1MiB；最多64个所选build；共享/每build参数最多128项、名称遵循001规则、单值<=4096字节；字段列表最多128项，不接受空或重复项。
- 幂等Key非空可打印安全标识、<=128字节；请求摘要SHA256；同key原请求不重新读取Git。
- 分页limit默认20，范围1..200；offset范围0..1000000；稳定CreatedAt降序+ID降序。
- Duration正且可由time.ParseDuration解析；无build.timeout表示无普通预算上限（数据库NULL），不能用0表示已耗尽。post默认2m，查询不重置。

## 状态与事务

006写入边界：不存在→queued或skipped；查询/重启不会把queued变running/succeeded。StepProgress先持久化模板与条件状态，执行状态保持未开始；steps.when false不意味着整个build跳过。没有节点只保留queued。

Enqueue：取写运行权→再次检查identity未撤销与项目PolicyVersion→再次查幂等→为queued分配项目统一号→插入Batch、Build、StepProgress、Idempotency→提交；事务内禁止Git/HTTP/shell。所选任一错误、计数溢出或DB失败全部回滚。真实20并发同/不同key与两个驱动同样验证。

组改名/项目迁移与audit同事务；FK保护并发删组。项目删除有历史/queued执行时拒绝，以最小保守保护保留证据。PG事务用持锁专用连接，锁失效不能从pool继续写；SQLite锁文件identity改变即不可恢复拒绝写。
