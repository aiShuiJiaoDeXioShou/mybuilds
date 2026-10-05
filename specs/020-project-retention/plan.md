# 实施计划：020 项目保留策略与受限清理

**Branch**: `020-project-retention` | **Date**: 2026-10-05 | **Spec**: [spec.md](spec.md)
**Input**: 原冻结25FR、7SC、4US/16AC，质量检查16/16。
**状态**: 既有spec/plan/tasks已完成，正式前置为已验收008 `504dc6fa8581f74a15ecc146a474976d5ae33a22`、019 `fee97e8ea32fc4f582abfb445c8d33f690f6f70e`，含独立测试夹具修复 `b2e659a`。当前集成分支020-project-retention-integration；按实际字段校正设计/任务后重新analyze，零阻塞才implement，不借005未验收源码。


## Summary

保留策略只从控制端管理配置与管理员项目设置生效，默认100条/30天，项目逐字段继承全局。终态用服务端事务保存的真正完成时间排序，以同项目全部命名build、无编号skipped汇总，数量/天数OR；保护优先，停止确认不刷新完成时间。旧时间无法证明则保留。

中央先保存具体清理事项，再退役新读取引用；业务保护存在时不能退役。实际读取以短DB登记和文件描述符共享锁保护，删除持排他锁并再次事务核对，重启不能凭TTL猜读者结束。既有文件经归属/身份检查、移入本事项专有隔离目录再有限删除，部分失败持久可恢复。原Agent使用当前独立身份领取同节点专用事项，查自己的归属登记/停止回执，不能凭过期执行租约、旧PID或请求路径删文件。中央与节点完成分别记录。

保留构建、批次、原身份请求与项目计数器的最小墓碑及必要审计，删除正文/日志/产物/报告/已登记节点数据，不重新入队。008 RetryOf RESTRICT原样保留：保留完整子构建时保护所有祖先完整证据，清理完整历史先child后parent；最小关联行不硬删，不降外键约束。后续审批/发布unknown尚不存在，020不虚构字段或保护成功，需要010/011/014真实消费者接入后联验。

## Technical Context

- **Language/Version**: Go1.25+，中文注释/文档，单模块与原三CLI。
- **Primary Dependencies**: 不新增；标准库os.Root/context/time、既有x/sys/unix文件锁、GORM双库、严格YAML/Viper/Cobra与标准HTTP。
- **Storage**: 原DB新增可信终态时间/清理状态，有限具体事项/对象/读取登记/资源归属记录；中央data_dir内私有隔离目录，Agent data_dir内自有资源登记与删除journal。
- **Testing**: 实际红绿、SQLite/PG同suite、当前模型旧迁移、同项目跨build排序、策略/读者/停止确认/retry竞态、真实三二进制verified HTTPS、macOS/Linux删除和离线重启。未来审批/unknown另留真实联验，不能手改状态计通过。
- **Target Platform**: 控制端与Agent macOS/Linux；远程客户端跨平台，Windows不实施本机删除。
- **Project Type**: 已有Store/Server/Agent的具体功能，不建GC框架、独立执行器、定时触发平台或看板。
- **Performance Goals**: 查询默认20/max200；每轮最多100候选、最多100具体文件对象，SQL短事务≤5s；每个中央/节点删除操作≤30s，HTTP普通30s；节点领取max10事项且同节点同时一个清理操作。后台每60s推进一轮，不为失败无限spin；每次重试仍重新授权。
- **Constraints**: 策略builds正int64；days正整数且24h纳秒乘法可表示（≤106751），不做溢出/浮点转换。目录遍历≤100000项、深度64且ctx有界；超限保留失败，不扩范围。不提供force，不扫描未知孤立文件、源码/本地run/SDK缓存，不用TTL或延迟当停止/读取结束证明。
- **Scale/Scope**: 只管理已登记项目终态完整历史及归属数据；清理状态与原业务状态分离，不新增成功/中断判断或解除锁/隔离。SQLite文件和PG持锁session不变，不支持多控制端/共享网络证据目录。

## Constitution Check

设计前后I–V均PASS：I只在008/019验收集成、tasks/analyze后实施，最终converge一次功能提交；II原三入口与单控制端、Agent主动领管理事项，同一Run不变；III现有依赖、具体消费者、不预建泛型GC/保护hook；IV严格输入/独立身份/事务复核/归属/未知保护/真实占用与停止证据；V中文、双库/两平台/行为记录与README实际状态。无例外。

## Phase 0：研究

见[research.md](research.md)：当前没有终态时间/retention、没有持久工作区登记、中央下载未持共享读锁；需要有限实际接入，不能只定时RemoveAll。007文件UUID/大小SHA与008receipt/guard/retry关系、019sealed原XML为真实消费者依据。文件锁官方语义已核对，不把研究当行为验收。

## Phase 1：状态与生命周期

1. LoadServer严格接受全局retention，启动持DB独占后将有效全局值同步入库（值变更才增版本/审计）；不热重载。改变全局修改管理文件并受控重启，项目set通过原settings事务保存覆盖。仓库Document不接受管理策略。
2. 终态事务一次写TerminalAt=服务端UTC：完整build_finished、queued取消、创建即skipped、真正running→interrupted；重复事件/停止确认不改。旧记录仅从精确build_finished receipt.CreatedAt、无attempt的skipped.CreatedAt、无attempt且真正取消审计可证明的最早时间补齐；旧中断仅有LeaseExpiresAt不足以证明转换时间，不补猜。
3. 按稳定项目ID、TerminalAt DESC/ID DESC排序，排除活动/时间未知/已完整清理墓碑；有可信时间但被保护的完整终态仍计排序。rank>N或TerminalAt<评估UTC减D×24h为候选，等号不因该维度入选。每次退役/物理操作再按当前政策与保护重算。
4. 当前保护：活动、StopUnconfirmed、step/receipt无法证明清理停止、未确认日志/文件/019报告、时间未知、保留retry子构建祖先闭包。旧未登记节点数据为归属待确认，不能借目录扫描补授权。后续waiting_approval/unknown接真实模型后加入相同检查及操作事务；现在任何未知状态fail-closed，不加未来空字段/接口。读取占用与业务保护区别见contracts。
5. 同一build保持一个清理事项/每节点资源一个稳定删除ID；意图事务锁定对象清单与当前策略版本，存具体已有StorageID/归属，不接受外部路径。只读取占用可退役引用并等待，业务保护不能退役。文件状态可partial/failed，失败不是不存在。
6. 下载/日志/SSE原实际打开入口先BeginEvidenceRead短事务检查未退役，再开实际文件并取SH|NB锁、核对身份后Activate，再给字节；关闭fd后结束读者。删除持EX|NB且复核当前事务才物理处理。重启对同inode取EX成功且旧请求不再拥有当前DB运行权，才清旧占用；不能仅因server实例变化或记录过期假定已释放。SSE按正在读取的每个chunk持锁，无数据等待不持无关文件锁。已有读取沿10m下载/15mSSE有界结束，关闭服务等待实际handler/fd释放。
7. 中央具体UUID文件核对受限root、parent/leaf identity、类型/nlink1/Size/SHA；将确知对象移入已登记事项专有隔离位置、再次核对identity后删除/fsync并记录结果。替换/未知对象保留并固定错误，恢复只找原对象与本事项隔离位置，不删新出现未知同名文件；SQL失败不假完成。
8. Agent首次成功checkout停止确认后、任何用户Run前将workspace父目录登记到自己的resources记录，再通过当前fence注册中性资源ID；Run产生结果根时先登记再发送其intent。最终精确008/019终态回执/独立停止确认和无pending日志保存后，登记具备可清理事实；未确认journal仍保护。执行时锁自有data_dir/资源登记，不运行shell，不解释旧PID，不发任何进程信号。
9. 当前节点领取/授权/确认管理事项均核对独立身份、原NodeID/资源ID/原Ref归属、当前策略与保护。短管理授权不是构建lease；每有限删除段前复核授权、父/目标identity与停止/日志事实，失效不开始新unlink。离线/墓碑不转派，不以中央完成冒充节点完成。固定删除ID/digest确认幂等，冲突拒绝。
10. 中央对象真实全删标中央完成，原节点事项仍可pending；只有中央完成且节点完成/合法not_applicable才清正文与大JSON/步骤/文件记录并标cleaned。此前保留008精确停止/journal恢复的receipt/attempt；最终保留最小build/batch/request/计数器/retry关联/必要attempt身份及精确终态摘要审计。原同键重放返回原关系+cleaned标记，不执行、不分号；正文/文件410，无法retry已清理正文。保留child会保护其parent完整历史，清理顺序child→parent；FK从不关闭或drop。

## Project Structure

```text
specs/020-project-retention/
  spec.md / checklists/requirements.md  # 原冻结
  plan.md / research.md / data-model.md / quickstart.md
  contracts/retention.md / go-api.md / node-http.md
internal/store/retention.go, retention_models.go, retention_test.go
internal/server/retention.go, retention_read_unix.go, retention_test.go
internal/agent/retention.go, retention_resources.go, retention_test.go
internal/config/retention.go, retention_test.go
internal/cli/client/retention.go, retention_test.go
```

仅三个实际业务consumer与具体安全文件操作，不创建internal/gc、registry/interface/hook或第二执行器。

### 唯一writer与共享交接

| 范围 | 唯一writer | 共享约束 |
|---|---|---|
| Store新retention/models与实际双库tests | A | 不importServer/Agent执行器；方法冻结后交root接原事务 |
| Agent新retention/resources与真实tests | B | 只自有登记、无进程信号；原Run仍唯一 |
| config/retention新文件、server新retention/read与tests、client新retention/HTTPtests | C | 共用严格输入/鉴权/下载路线，不引框架 |
| protocol/node.go、Store models/node_models/store/enqueue/event/query/stop/node/retry/recovery及project.go原事务；Server files/artifact/log_stream/server/http/json/trigger；Agent execute/journal/recovery_journal/file/serve；config server/project与CLI原project展示/接线 | root串行 | A/B/C先交具体最小补丁与SHA；同文件不并发写；与008/019已提交基线合并后复验 |
| README/产品专题/历史/spec状态/validation/tasks/converge/提交 | root | 最多root+A+B+C四slot，不锁名字，无分区提交 |

019的ReportEvidence/原XML与完整manifest字段保留，清理只退役/删除其已登记副本；没有假artifact步骤或重算报告。008RetryOf/Kind/StopKnown保留原意义，新时间与cleaned字段不修改旧wire消息digest，旧未知不补假时间。发布/014未来共享业务保护及恢复入口须由root串行联验，不提前实现。

## 真实验收与交付门

US1四种继承/非法权限；US2跨build/OR/等号/排序/真实终态来源/保留retry链及删除前竞争；US3真实下载与SSE、共享锁、重启与部分删除、当前身份离线重连/20次确认、symlink/FIFO/hardlink/替换/未知目录与旧PID不被操作；US4原键/编号/墓碑/安全审计、两库macOS/Linux同三二进制与008/019回归。010/011/014真正审批/unknown保护仍是独立待联验门，当前不通过伪状态宣称。必要全量/race/vet/跨编译及converge通过后一次本地提交。

## Complexity Tracking

无原则例外。读取共享锁与短事务有两个现成文件consumer，持久清理事项用于实际崩溃恢复，资源登记用于实际Agent删除；不扩展为通用读锁/垃圾收集服务。

## 25FR / 7SC / 16AC设计覆盖

| 需求 | 实际设计/未来实施验收入口 |
|---|---|
| FR001–003 | Phase1.1；retention配置契约；US1严格导入/继承/角色 |
| FR004–005 | Phase1.3；稳定项目rank/UTC OR边界；US2.AC1–2 |
| FR006 | Phase1.2；可信时间与旧未知迁移；US2.AC5 |
| FR007–008 | Phase1.4/6/9；当前保护及未来真实消费者联验；US2.AC3 |
| FR009 | Phase1.3/5/7/9；最新策略/身份/保护事务复核；US2.AC4 |
| FR010–013 | Phase1.5–7/10；完整中央对象/读锁/部分恢复/归属；US3.AC1–2/4 |
| FR014–018 | Phase1.8–9；资源登记/当前Node管理事项/独立确认Seq；US3.AC3–5 |
| FR019 | Phase1.10；最小墓碑/原请求/连续编号/RESTRICT；US4.AC1 |
| FR020–021 | 管理HTTP/CLI与安全投影/410；US1.AC3、US4.AC2 |
| FR022–023 | Technical Context及contracts限额/范围；真实失权/未知对象负例 |
| FR024 | 当前基线/实施前置、Phase1.4/008/019共享与真实010/011/014后续联验；US4.AC3 |
| FR025 | 真实验收门/quickstart两库与macOS/Linux，非编译替代 |
| SC001 | quickstart策略四种覆盖与每项非法/身份负例；US1.AC1–3 |
| SC002 | quickstart跨build/项目、OR/并列/精确边界/可信时间；US2.AC1–2/5 |
| SC003 | quickstart当前真实保护与候选后竞态，未来审批/unknown另待实际流程；US2.AC3–4、US4.AC3 |
| SC004 | quickstart真实二进制/XML/log/SSE读取与退役/重启、正确SHA；US3.AC1 |
| SC005 | quickstart部分删除、离线重连、重启及20次确认/同ID；US3.AC2–3 |
| SC006 | quickstart受限目录/错误身份/对照进程、编号/旧键/安全审计；US3.AC4–5、US4.AC1–2 |
| SC007 | 两库/两平台真实门、008/019基线、010/011/014待真实联验；全部16AC |

此表为设计覆盖，不是已执行测试计数；62任务已生成，正式基线校正后analyze再implement。四故事最终均需真实验收，依赖实际模型/HTTP/资源consumer分阶段交接，不以首个checkpoint代替完整020。

## 正式基线字段与T002补齐

019当前ReportRevision/Final/ReportsJSON/SealDigest/CheckedIndex及artifact Purpose/Revision/Key/VerifiedJUnitJSON保留原意义；008原buildRef/attempt关联、LastEventSeq、精确terminal receipt（DB Kind/StopKnown，wire无Kind）和独立stopConfirmation保留。具体观察输入与固定隔离slot、quarantined私有确认、无递归write、零动作报告与墓碑消费者见更新的go-api契约。闭包在SQL≤5s内有界，超限保护而非截断授删除权；节点授权从本次请求发送起点保守计时。

### 退役与原快照重试的双向门

原samekey/digest已确认请求先沿最小安全batch投影重放，不新分号。新Retry只允许原HistoryState=live；retiring/partial/cleaned在同一原Retry事务、frozenBuild前及提交边界复核后均retention_retired。Retry先提交则祖先闭包保护原完整证据，Retire先提交则拒新Retry，不能在部分unlink后新增需要完整祖先的child。此为FR007/009/019与原008接口的实际竞争门，不建新重试器。

### 具体消费者交接

实际Server启动绑定Store.RegisterEvidenceReadOwner，同持控制锁Store重复绑定不换UUID，旧控制失权禁止Activate；Activate持久实际SH fd出生Identity，重启不能用替换叶子的EX修复旧active。现有LogStored传真实私有chunk ID。Agent结果目录仅由原ensureResult实际MkdirTemp后的ResultCreated回调登记（先于intent），沿原累计预算/Authority；原pipeline共享入口由root唯一修改，A/B/C均不复制Run。

## 实施依赖校正（2026-10-05）

US2只读保护完成后，先并行Agent私有资源登记T035–036与Store/HTTP真实登记T037–038，再由root接原checkout/Run/终态consumer T039。完成后T025–026才建立真实已登记节点构建的清理事项；资源登记不依赖删除job，节点删除领取T040以后仍依赖实际job。旧未登记资源保持保护，不用手填登记行绕过。此修正仅重排62任务内的真实依赖，不扩展需求或修改原则。

登记前置不执行清理，可在Foundation/US1后与US2独立并行：B私有资源、root Store登记两新文件、C实际HTTP，root共享原执行接入；T025以后的实际清理仍等待US2与T039行为通过。该最小分区避免A同文件双writer，root新增retention_resources.go/test唯一所有权。

I3整改：物理Stop不能证明未上传日志已确认。T039实际独立Stop ACK后，Agent固定完成事实/fsync及既有resources POST的窄Completion游标消费者，中央真实Stop/完整归属/全部游标验证后存CompletedAt/CompletionJSON；保护无该证据的中断资源，不造终态receipt或解除unknown。由root协议/Store资源两文件/共享入口、A模型/保护、B本地consumer分区实现，实际红绿后再开清理门。


### I4：有界扫描必须能继续推进

真实双库反例确认：固定前100项目/候选或前limit未完成对象、事项/Node删除，再跳过protected/已有job/pending/failed，会永久饿死后部。保持每轮项目/候选/对象/事项各原限额、SQL整体5s与全部授权复核，采用五种具体消费者的独立持久续扫位置，不增加无界扫描、权限或GC框架。

私有位置只存最后实际扫描的原UUID；项目按ID推进，其余按原terminal_at或created_at及ID稳定顺序，从仍保留的原记录取排序时间。分两段不重叠窗口（位置之后，再有限回绕之前）且总扫描≤原limit；protected、已job、failed、pending同样推进位置，不表示授权、删除或完成。位置与原动作同write事务保存，SQL/控制失败全回滚，不能借扫描刷新TerminalAt/CompletedAt/receipt或业务UpdatedAt。

分别使用现有全局retentionPolicyRecord的ProjectCursor/ObjectCursor/FinalizeCursor，projectRecord私有RetentionCandidateCursor/RetentionObjectCursor/RetentionFinalizeCursor，nodeRecord私有RetentionDeletionCursor；空值是初始位置，非法持久UUID安全拒绝，缺失原排序记录有限回绕。管理项目范围与全局后台位置分开；node位置不随token/session轮换重置。列默认空字符串，双库迁移兼容旧库；无新表或公开游标/HTTP参数，原wire/唯一ID/请求/幂等保持。

原T046/T048/T059增加101项目、受保护/已登记/失败前缀、未完成前缀真实多轮与控制重开位置保留、轮转token不重置、失败事务回滚门；62ID及25FR/7SC/16AC不变。
