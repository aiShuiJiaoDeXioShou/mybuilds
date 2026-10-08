# 019 研究与决策

日期：2026-10-04；最初研究参考007 `85b46bfaccb45c4626fcffbbcb526b2f7f8c9301`，正式实施基线为已验收008 `504dc6f`。下文保留设计时的代码接点与取舍，当前实现和实际检查结果见[validation.md](validation.md)，不把研究本身当验收。

## 1. 已有实际接入点

**Decision**：复用原Run/Trigger/Enqueue与完整中央文件路径，三个reports拒绝点一并接入。
**Rationale**：config/types.go已有Reports.JUnit.Paths/Required，validate.go路径结构已严格检查，preview.go扫描模板；server/trigger.go194、store/enqueue.go114、pipeline/run.go286分别拒reports。只改Run不能让真实远程触发入队。run.go普通段先executeStep再executePost；remote.go、Agent.execute/journal、Store.event保持实际Started、fence、ns预算和full receipts。artifact.go只认实际artifact step，不能伪造run为artifact。
**Alternatives considered**：第二执行器、只在上传后读XML、把summary塞日志、伪artifact步骤均违反同一Run/发布前结果与完整归属要求。

## 2. XML语法与JUnit语义

**Decision**：用encoding/xml Decoder.Token的有限状态解析，Strict=true，不设置Entity/CharsetReader，不使用无界递归Unmarshal/Skip。只接受UTF-8、无namespace的testsuite/testsuites、testcase、failure/error/skipped、properties/property、system-out/system-err受限结构；未知语义结构拒绝。允许受限metadata属性，重复属性拒绝。只XML开头的标准xml声明可接受；Directive（包含DOCTYPE）与其它处理指令拒绝。
**Rationale**：Go Decoder默认严格检查配对标签，且只内建XML标准转义；并不替代应用结构、深度和计数限制。Token流适合每次检查ctx及层级上限。[Go encoding/xml官方接口](https://pkg.go.dev/encoding/xml#Decoder)
**Alternatives considered**：外部DTD/XSD/实体resolver、第三方JUnit框架、泛用反射decoder无当前必要。JUnit变体不能靠存在文件或suite声明总数放行；支持明确子集并留下真实Gradle/脚本样例。

计数以实际testcase为唯一来源；嵌套suite的汇总属性仅核对其子树实际计数，不再次加总。case只能有一个结局（pass/failure/error/skipped）；同case冲突/重复结局非法。suite/testsuites tests/failures/errors/skipped属性若存在须非负整数并精确匹配实际子树，不以摘要属性虚构case。合法零用例suite可表示0；空文件/无root/多root非法。

耗时采用非负十进制秒（最多9位小数、不接受指数/NaN/Inf），受限整数转换成ns；每case缺time记0，汇总case耗时，不再加suite的汇总time。suite time可作为受限metadata但不要求等于case总时长（并行/overhead不能误当矛盾计数）。最大单case与总duration分别受reports契约限制。[Go strconv整数解析接口](https://pkg.go.dev/strconv#ParseInt)。不是浮点推断预算，报告耗时与Run剩余ns是不同事实。

## 3. 新鲜性与快照

**Decision**：每build实际执行前保存有界匹配的identity/mtime/hash；identity、mtime或内容摘要任一改变，或本次新路径，才有本次文件证据；三者相同视旧。同内容重建可由不同identity证明；仅mtime变化表示修改证据，不按墙钟“晚于启动”比较。baseline不存在不创建输出。旧文件不解析、不删除；最终缺失按required处理。当前接纳路径消失时移出集合，不用已消失路径放行。
**Rationale**：本地用户可重复运行、保留mtime或重复相同内容。远端新clone也可能包含仓库预置XML，仍建立baseline，不能把clone时刻当测试产出。按路径替换避免重复汇总。保持原os.Root边界并明确拒symlink，非阻塞open后fstat复检避免FIFO TOCTOU。[Go os.Root官方接口](https://pkg.go.dev/os#Root)
**Alternatives considered**：删除旧目录、只看mtime、只看hash、仅依赖全新远端workspace都不足以覆盖需求。

现有collectArtifacts每模式必须有匹配、目标目录不可覆盖；报告准备缺失和同路径多次替换不能直接调用整套。最小提取已有具体glob/普通文件稳定复制入口供artifact和reports真实两消费者，保留其原行为。报告每次复制到新的私有snapshot，再解析同一字节并记录SHA；后续workspace或post改写不改变该快照。

## 4. 原XML秘密

**Decision**：在发布快照/公共metadata之前，对原字节、解析后所有属性/文本以及报告相对路径检查本次已声明secret，非空secret命中即report_secret，原XML不上传、不作为公共产物；固定失败码、摘要脱敏。合法XML保持原字节，禁止重写“脱敏原XML”冒充原证据。
**Rationale**：实体编码可能隐藏原字节秘密；必须同时扫描解码token。Agent只使用实际声明的秘密，不任意读宿主环境。server没有节点秘密值，负责对已上传字节重复解析/计数/摘要一致性及大小SHA；秘密防护仍在实际声明secret持有端执行。
**Alternatives considered**：下载时替换、只脱敏diagnostic、只扫描原字节不扫描解码结果都不能满足原XML安全及摘要对应。

## 5. 中央确认与同一执行

**Decision**：reports_checked保存当前路径集合声明（逐run及最终边界），只最终revision允许上传；purpose=junit使用既有artifact二进制路由，具体ReportKey/ReportRevision和真实ordinary run index绑定声明。server稳定stage重复解析得到摘要后，沿已有排他发布→短DBfence提交；reports_sealed要求完整文件确认与summary一致，build_finished精确重核。没有新文件路由、没有通用RPC。
**Rationale**：007 artifact manifest/CommitArtifact只认artifact step；简单把XML加到原ArtifactSteps会虚构步骤。有限purpose分支可复用文件预算、权限、receipt恢复和下载，又给Store真实报告约束。每attempt普通制品最多128份，junit使用冻结max_files（默认256，1–1024），合计4GiB与junit累计64MiB不变。
**Alternatives considered**：单独报告文件存储与下载执行器、仅信任Node自报summary、seal先于原XML完成均拒绝。

## 6. 预算、失败和008

**Decision**：收集/快照/解析/上传/checked与seal确认全计ordinary剩余ns；10s检查上限与剩余普通预算较小者生效，0预算拒新读/传输。普通命令原失败/取消/timeout优先，报告业务失败可在Authority有效时选failure/always；Authority/保存失败闭锁不执行always。post后不再封存。
**Rationale**：007已有独立Authority、原失败优先和post独立预算；报告不能以用户取消为由恢复失权或重置预算。final required缺失可发生所有run成功以后，Store必须认真实报告封存结论并参与post/terminal，不能只依赖step状态。
**Alternatives considered**：报告处理另开无限预算、把report failed仅当warning、post改通过XML放行发布、零动作生成假seal均拒绝。

008规划writer实际沟通：其仅新增execution receipt Kind/StopKnown与TerminalReceiptRequest/TerminalReceipt精确只读接口；不新增报告字段。019 seal和manifest须在build_finished事务完成前验证，StopKnown仍由完整终态提交产生。retry沿原Definition.Reports，报告证据/seq/seal全部新建，不继承旧pass；恢复不重新读post改写XML。共享文件root串行写入，不把独立plan当源码已实现。

## 未解决事项

无业务NEEDS CLARIFICATION。46任务与正式analyze已通过；实际parser、collection、Store、Stage、Agent和CLI已逐SHA串行集成，使用标准库和原同一Run，不新增依赖。原失败与后续真实修正、双库/两平台/权限/原XML/预算等证据保留在validation，最终全量及双库20故障验收已通过，正式converge无缺口，待一次整功能提交。

2026-10-08 数量增量：默认256、max_files=1–1024；普通制品128份单独计数，累计字节/cases/诊断/时间上限不变。报告相关消息8MiB，执行journal64MiB/100万节点；冻结配置、审批与恢复使用同一配额，极端元数据与历史仍可能触及独立预算。

## 2026-10-08 规模审计

只读子代理审计实际消费者：1024个ReportFile约14350 JSON节点，旧10000不足；合法1024字节路径经过Go的<>&转义可使证据超过6MiB。一次审批含baseline/current、两份event及原私有artifacts，最坏长路径合成journal约57MiB，无诊断约123082节点。采用事件/claim8MiB、事件65536节点、执行journal64MiB/100万节点，所有相关写读与启动iOS前置扫描同步；小型resource/deletion/secret仍原限制。合成核算不是实际XML上传证明，真实闭环由validation记录。

发布候选原先Limit129后过滤junit会丢失普通制品，改为SQL先排除junit。中央审批完整文件集合上限亦随冻结配置调整；普通制品128份仍保留。

大规模实际恢复另暴露 confirmedReportFiles 在每个候选文件上先计算审批历史摘要，1024报告会重复编码整份证据约百万次。改为先匹配ID并在单次调用内每个Ref核验一次；declareReports同样复用本次核验结果。缓存不跨状态/调用保存，归属、内容摘要与完整文件确认不放宽。实际1024暂停→退出→重启→批准→完整终态案例验证该路径。
