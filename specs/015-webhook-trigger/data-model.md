# 015 持久状态与事务规则

具体字段/类型只定义于[Go契约](contracts/go-api.md)，配置/大小只定义于[config-changes](contracts/config-changes.md)。本页规定约束与迁移；不新建通用事件/凭据/调度框架。

## 项目来源与密钥

Project.Provider/Repository不变。HookPolicy唯一ProjectID FK RESTRICT，admin导入/启停/rotate增加项目PolicyVersion；配置Secret完整引用、生成CredentialID、SecretFingerprint均不进入安全ProjectView。自产key在控制端自己的hooks/<project UUID>/<credential UUID>.key，0700目录/0600排他普通文件；只管理员一次响应可带明文。DB提交前创建文件并fsync，DB失败没有有效policy，不把孤儿当credential；实际同inode/root受限打开，链接/特殊文件/权限/替换固定失败。外部引用独立envfile已解析内存，只声明名字和值；启动与请求前指纹不一致拒认证，必须显式rotate/configure才更新，不静默授新权。禁用/轮换使旧HookActor失效，旧event仍留；不自动清文件或碰别种材料。

HookActor是server验证当前项目秘密后形成的具体内部证据；绝不来自JSON/admin Actor伪造。Store每次Receive/Close末尾复核实际ProjectID/CredentialID/PolicyVersion/SecretFingerprint及enabled。自动upload资格来自当前admin导入grant而非提交者、仓库when、Webhook头或角色token。

## 接收与窗口

HookEvent不存body/完整headers/提交人/commit message/URL，只安全正规化数据、BodyDigest、DeliveryID、ReceiptDigest、ReceivedAt、WindowID/Reason。DeliveryID若存在，唯一(project_id,provider,delivery_id)，same digest精确重投回原；different digest conflict。认证仍先核对当前credential；密钥轮换不重新授同delivery的窗口/编号，合法新凭据重投仍返回原关系或冲突，原event所记credential不改。无ID的EventID每次生成，可保接收审计，最终执行按语义key去重。header delivery未被所有provider的body签名保护，BodyDigest完全相同且同credential/provider/kind/分支/after/PolicyVersion时，在未截止的pending窗口或全部结果仍queued/running/waiting_approval/approved/succeeded/停止未知的closed窗口内另作原接收别名，不能仅换ID重复同请求；ID别名仍持久唯一且核对digest。过期pending、failed窗口与已明确失败/取消的结果允许新的无ID/新ID请求；原delivery ID精确重投始终保留原归属。不保存秘密body。

GroupKey=SHA256 canonical `{project_id,provider,credential_id,policy_version,branch,sorted_build_names,input_params,input_build_params}`，输入参数只来自已导入项目策略而非providerpayload；原选择集合排序唯一/非空，单build推导发生在admin启用时并保存，接收不Git。PolicyVersion加入key防旧grant混进新策略。

Window.Generation为group单调正整数，UTC起点/截止由Store事务内获取；第一个事件总归自己的窗口，后续只在ReceivedAt<Deadline且同GroupKey时加入。quiet_period=0不接纳第二事件；恰好Deadline新代。UNIQUE(group_key,generation)，沿Store.write串行短事务读同group最大generation并插入唯一新generation（唯一冲突整体回滚）；允许旧due但未关闭与新代同时pending，不错误使用“一个group仅一个pending”索引。generation/Revision防int64溢出；索引(group_key,generation)与(state,deadline,id)，不另建group计数表或带now条件的索引。

接收tx同提交event/alias、窗口与归属，候选SHA只是安全展示（不决定最终执行）；窗口Revision只在有效合并时递增，OpenedAt/Deadline永远不改。接收commit后才HTTP2xx；失败回滚不分号。原始事件的去重证据都保留关系，merged表示所属固定窗口，不表示丢弃原事件。

## 准备关闭与最后CAS

窗口状态只有pending→closed或failed，无执行租约/临时closing授权。Server有唯一控制端锁，按Deadline/ID分页读due；每窗口45秒父ctx内：

1. 读当前window/Project/HookPolicy、冻结原input；禁用/PolicyVersion不等原版则失败policy_changed，不用当前新授权偷偷重解释旧窗口。
2. 以登记Repository、授权branch取得当时HEAD固定SHA；用012同resolvePipeline在精确SHA读来源、选择、参数、定义/upload授权，错误安全失败。branch后来推进不换本次SHA；变成不可达则失败，不能追新HEAD。
3. 为所有所选build计算ComparisonKey，读最新可信成功baseline；每个比较证据带baseline buildID/SHA/terminal seq/digest和中央receipt时间。SCM在tx外读固定两个tree、Preview用ChangeFacts准备所有build/step；全部定义/参数/上传权限必须验证，when=false仍不豁免。
4. CloseWebhookWindow同短Store.write复核锁、窗口pending/Revision/Deadline、原项目PolicyVersion/当前secret身份、全比较键最新baseline精确等于输入。最新成功在准备间变化时ErrConflict，Server用原窗口/原截止重新做完整准备，不复用旧diff。最多按原45秒预算允许的次数，不重置预算；耗尽记录hook_timeout，不悄悄关闭成无变化。
5. 查每所选build的自动SemanticKey；可reuse状态为queued/running/cancel_requested/waiting_approval/succeeded以及有stop_unconfirmed保护的原自动记录，确保不重复保护中的执行。未来审批状态必须以已接受014实际枚举串行接入，不能按猜出的别名绕过。其它已确认failed/cancelled/interrupted可重新创建；skipped不作永久成功去重，否则首构建/基线变化会被误挡。
6. 全部fresh PreparedBuild沿原私有enqueueTx创建新batch/记录，条件skipped无号；reuse保存原BuildIDs，不拷贝执行记录/新授上传。窗口closed、FinalSHA/安全结果、fresh计数器与alias归属一次commit；任何错误整体rollback，重复close返同结果。

不保证多个仓库HEAD读取与远端push线性化；保证本次已选SHA之后所有来源/diff/执行相同。新事件属于原区间或下一代，不能覆盖正在执行任务。Git系统进程CleanupFailed时沿scm安全错误/保守文件保护，不执行构建或假停止。

## 比较key与成功基线

ComparisonKey=SHA256 canonical `{project_id,branch,build_name,sorted_selected_names,definition_digest,params,origin}`，DefinitionDigest是原完整Build（含when/reports/post/步骤）且Origin只固定Mode/Kind/File/Profile/Template/ContentDigest/DefinitionDigest（明确排除Origin.SHA），不含当前SHA或动态build.id/number/node/workspace。SHA若进key将永远无baseline，禁止。最终参数按012优先级解析，不能拿params键名或rawrequest代替值。

新manual/auto Trigger统一准备此key，Snapshot追加ComparisonKey；retry继承原key/原Changes，完全清空新执行证据。原成功build只在同key且status=succeeded、stop_unconfirmed=false、完整manifest/报告/发布结论已确认时可作baseline。成功时间采用008已存在executionReceipt精确 `(build_id,attempt_id,last_event_seq)`，Kind=build_finished、StopKnown=true、Digest合法与中央CreatedAt；按CreatedAt DESC, buildID ASC稳定排序，不使用UpdatedAt、节点时间、停止确认时间或queued reason。旧没有Kind/完整证据/key默认不补猜，视无baseline/full；无需另建baseline表或借020未接受CompletedAt。

SemanticKey=SHA256 canonical `{comparison_key,target_sha}`，只对原自动记录参与reuse，manual/retry即使同key/SHA也不受自动永久去重；它们的成功仍可作为本次comparison baseline。Snapshot.AutomaticWindowID标原自动来源（内部关系，不是执行权），新retry不能继承自动去重来源，需要只保RetryOf+原Changes/ComparisonKey。

## Changes快照与迁移

ChangeFacts在protocol具体字段见Go契约：full/diff；full只因baseline_missing或baseline_unavailable，Paths显式[]；diff保实际两tree规范、去重排序路径与摘要，空[]确指无变化。多build baseline不同，所以每build独立保存。Root字段omitempty保旧manual/local语义和旧无Changes事件digest；现有Facts map只保系统身份，不把用户rawFacts伪造的changes.*当可信条件。

Preview/Run每个when用同份冻结事实；full使changes通过，其它branch/params继续AND，diff任一声明模式命中路径即通过。动态env只在原Run渲染，不重判Git；retry照原facts，不从当前branch/基线重做。changes build skipped无号；ordinary全部条件skipped沿原Run，不post。

两库相同迁移/唯一/FK/CAS套件：旧project hook默认disabled/无Credential；新增optional快照字段旧nil保持；receipt并不猜补Kind/StopKnown；可按原受限snapshot投影显式判断无key→full。SQLite不能禁全池FK/GORM重建含旧attempt触发已知冲突；实际增量迁移先旧007/008满记录红门，PG相同seed/约束。窄tx末尾还复核控制端锁，永久失锁不自动重连写。

## 安全读与失败

公开event/window仅provider、event/window UUID、branch/SHA/digests、接收/截止/结果ID、状态/固定reason和路径数量/摘要；不公开密钥身份指纹、原body、input参数值/定义、私有Repository/keypath。admin/approver读自动证据，trigger只status/原授权trigger，node只原节点协议。分页沿现有Page；输入limit按契约固定，不建审计通用框架。

永久关闭失败只保存固定reason与原事件关系，不创建假batch或分号；网络/资源问题不伪缺baseline。一次已接受事件后最终窗口failed是诚实可查询结果，provider2xx只表示已接收，不表示执行/发布成功。新push或管理员手动trigger可另请求，不能重投delivery获得新发布授权。
