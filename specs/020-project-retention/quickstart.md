# 020 规划验收指南

当前规范/设计/62任务已生成，020正在实施；019fee97e8与008504dc6均已验收。策略、候选、管理清理与事项命令已接入，完整macOS/Linux各双库三程序联验已通过，实际检查点见validation。不借005未验收源码或Apple材料，未来审批/unknown门单独联验。

## 自有夹具与策略

先复用[007独立三入口部署指南](../007-node-agents/quickstart.md)与[019报告指南](../019-test-reports/quickstart.md)的实际verified HTTPS/CA/私有token、独立SQLite/PG数据库和受信固定SHA；其中client_bin和CLIENT_CONFIG指向自己的二进制与0600客户端配置。Node须自有data_dir，不访问别人的服务/工作区。全局在自己的server.yml设置retention: {builds: 100, days: 30}，受控重启后查询，项目只覆盖数量：

```bash
umask 077
cat > "$FIXTURE/retention-settings.yml" <<'YAML'
pipeline:
  source: repo
  file: mybuilds.yml
retention:
  builds: 200
YAML
"$client_bin" --config "$CLIENT_CONFIG" project set retention-demo --settings "$FIXTURE/retention-settings.yml"
"$client_bin" --config "$CLIENT_CONFIG" retention show retention-demo --json
"$client_bin" --config "$CLIENT_CONFIG" retention ls retention-demo --candidates --json
```

预期有效200/30、来源project/global。把自有全局天数改45并真正重启，期望200/45；项目retention空块恢复100/45。组迁移不改变ProjectID/策略。逐项0/负/小数/string/null/未知/重复/溢出、仓库retention与trigger/approver/Node提交都拒，无半变更/文件变动。所有token只受限文件或env，不写argv。

## 真实终态、排序与保护

同项目至少6个真实命名build终态、另一项目对照，设置数量4；含创建skipped、queued后实际cancel及完成/失败报告，跨名排序保最新4。普通执行终态时间来自server事务，查询StopConfirm前后TerminalAt必须相同；原精确receipt/取消审计迁移需真实两库旧基线夹具。不能用任意UpdatedAt或手改业务状态证明保护。

精确年龄/相同时间边界由测试实际可信终态事务留下时间，再在纯筛选的固定evaluationUTC验证：恰等于D×24h与rankN不因该维度选中，另一维度OR仍生效。双库相同输入同结果；这是排序算法门，不替代实际HTTP删除。缺少可信旧终态时间永远标保护，不参加rank；保护终态已有时间仍占rank，已cleaned墓碑不占。

当前实际running、断网lease expiry→interrupted+guard、原日志/报告/停止事实未确认都保护。真实retry构成parent→child→grandchild，保留grandchild须保祖先完整文件，三个都可删时先child后parent；FK保持RESTRICT、原关联保留。候选后策略增限或新增真实retry/读取/保护，退役/物理操作前复核不能沿旧授权。未来waiting_approval/unknown必须由010/011/014真实流程另验，不能手写状态声称SC003全部通过。

```bash
"$client_bin" --config "$CLIENT_CONFIG" retention run retention-demo --limit 100 --json
"$client_bin" --config "$CLIENT_CONFIG" retention ls retention-demo --json
```

一次命令可返回waiting/partial/pending，离线不假completed。周边自有对照文件/SDKcache/local独立run结果/源码与未知孤立文件不变，原project.NextNumber保留。

## 下载、文件锁与重启

使用真实二进制artifact和019原XML、普通/post日志，开始实际慢下载及SSE读取，再启动retention。已开始读取字节完整/摘要正确或明确断流，退役后新读410；物理文件在真实fd SH仍在时不能删除。检查100次并发Begin/Retire/Activate与收尾路径，不以sleep当读取结束。

控制端在只登记pending、读SH、退役、隔离、删除、DB确认各间隙关闭/失去自有PG锁/重启：当前实际SH仍持时保持waiting；取得同inodeEX才允许核对并结束旧占用，TTL不是授权。断流、cancel、10m下载/15mSSE自然预算、关闭服务真实handler/fd结束均释放占用。第二控制端不能创建或推进删除事项。

自有第二连接仅测试SQL trigger造成保存失败：已删/隔离对象真实状态保留、不伪完成；去掉自己trigger恢复同ID。权限/特殊文件/symlink/hardlink/parent/leaf替换、越界/未知StorageID均固定失败，不能删除陌生inode或误当不存在。只在私有夹具修改FS/DB，不触碰真实用户资料。

## Agent离线与幂等

停止自己的Agent后清理中央合法副本，NodeState保持pending；原节点当前合法token重新连接后领取同DeleteID/ResourceID，登记Ref与OwnershipDigest一致。已知目标真实不存在成功；20次原Seq/Digest确认仅一个receipt，不扩大删范围。丢响应按同确认恢复，冲突/交叉节点/撤销token/旧nonce拒；轮换同NodeID可以继续，墓碑和永久离线不假完成、不转派。

Node持未确认journal/日志/系统资源或实际活动Run时拒删；真实已停止确认与无pending才授权。替换自己登记的目录/leaf、放置links/FIFO/hardlinks等失败且对照文件留存。旧journal的PID/PGID作为未知资料，代码与实机证明没有signal；用仍活着的无关自有sleep对照，不能把新时间/kill0当旧停止事实。新执行资源登记必须先于用户Run，成功journal移除后归属仍持久；旧无登记目录仅报告unknown，不按prefix扫描删掉。

## 最小证据与最终门

清理至原编号101历史，重放原身份同键同请求仍原批次/执行/编号且不Run，新任务继续102；冲突键拒。已清正文410、最小view有cleaned，计数器/幂等/retry关系/必要审计可查，原参数/脚本/原日志/私有路径不公开。已清原构建不能retry未保留snapshot。

SQLite/PG同管理/保护/排序/读取/SQL失败/离线重启/20确认/旧键/第二控制端suite，macOS/Linux真实中央+Agent独立闭环，记录固定SHA、CommandsUTC、actualPID/PGID与对照、文件大小SHA、实际删除ID/状态及无关对象保持。必要全量test/race/vet/三CLI纯Go跨编译/原local配置隔离与008/019兼容，通过converge才整020一次本地提交无push。010/011/014真实审批/unknown联验须另有证据，未交付时明确待联验，整个MVP和005真实Apple验收不因此完成。

新增Retry竞争门：使用真实已完成parent，Retire先成功后新Retry须retention_retired且不分号；Retry先成功则Retire受retry_dependency保护完整parent。retiring/partial/cleaned均禁止新Retry，原已确认key/digest仍回原最小批次关系，不重解析已删正文。


## 后续审批与发布的真实联验义务

014必须把真实waiting_approval/审批记录接入现有retentionProtection及retentionDeletionAllowed的同事务判断：创建审批意图前反向拒绝retiring/partial/cleaned；候选已读取后进入等待审批，真正清理须被当前保护挡住。审批放行或拒绝后仍按真实构建、日志/报告和资源状态复核，不能仅删除审批行就解除其它保护。

010/011必须用真实发布意图、attempt与原制品绑定接入同一保护函数；已产生可能提交的远端副作用且响应不明时保持unknown，即使普通构建status为failed也不能清理。原StopConfirmation只证明本地停止，不能解除发布unknown。新发布授权也须在同一事务拒绝已退役历史，防止候选与发布意图之间的竞争。

上述三个后续feature分别记录以下真实双库与HTTP/Agent门：正常状态允许依据策略清理；审批等待/发布unknown不删除中央或节点证据；先建保护则退役拒、先退役则新授权拒；实际停止确认不解除unknown；状态恢复明确后重新复核原事项身份及原制品；保护记录仍计历史配额、凭据或节点轮换不扩大范围。复用当前真实FinalizeRetention核对，不预建未来状态或拿模拟行当已验收消费者。它们是各功能的必需联验，不阻塞020当前模型的独立实现，也不代表已完成MVP。
