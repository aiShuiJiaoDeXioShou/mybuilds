# 019 数据模型与状态

## 配置与兼容

沿用config.Build.Reports → Reports.JUnit{Paths []string, Required *bool, MaxFiles *int}；nil表示未配置，Required=nil等价true，MaxFiles=nil等价256，显式值只接受1–1024。路径一次模板/渲染后仍须满足既有仓库相对glob边界。入队保留于原BuildSnapshot.Definition，不新建项目报告schema。无reports配置不扫描、不建立事件/报告结果；旧artifact用途为空时仍是artifact，新可选wire字段omitempty不改变原事件digest。

## 本地执行报告集合（仅私有）

每实际build建立一次reportCollection，生命周期在唯一Run普通段内：rendered patterns、required、基线map[path]{file identity/mtime/sha256}、当前map[path]{独立快照、解析结果、真实run来源}。基线和SnapshotPath不公开。每路径新建/重建/改写替换当前项；未变化的本次已接受文件复用，删除路径移除，不重复叠加。原用户文件不删除。

每文件有Key=SHA256(规范仓库相对路径)、Path（受限且无已声明secret）、UUID ArtifactID、SourceIndex/SourceStep（实际ordinary run）、Size/SHA256、Counts。快照存原ResultDir/reports内中性UUID私有路径，新版本独立目录，不覆写已发布快照。解析同一快照，生成有限Counts与Diagnostic；输入本身含secret时不成为公共File。

## 公共具体类型

类型定义见[go-api.md](contracts/go-api.md)。JUnitCounts为Tests/Failures/Errors/Skipped/DurationNS；Diagnostic为PathKey/Case/Outcome/Message，仅20条及总20KiB，字符串受限且控制符安全。File.Counts只含整数，不复制每文件失败文本；最终Evidence.Diagnostics从各真实解析结果按Path排序取受限前20条。

ReportEvidence：Revision、Sealed、Outcome(pending/passed/failed/missing)、Reason、Required、Counts、Diagnostics[]、Files[]。Files按Path排序，Path/Key/ArtifactID均唯一；集合≤64且合计≤64MiB。两种数组都必需非null，包括0条。Revision从1严格递增，每个run检查及final检查各一次；Sealed只在final后从false→true，不再次修改。

- checked且尚缺文件：pending，缺失不提前失败；已生成有效文件可以passed；检测failure/error→failed/report_failed；非法/secret/安全收集错误→failed固定reason。
- final且required=true模式缺失：failed/report_missing；required=false只缺失：missing、counts0/无文件可封存，但不能覆盖命令非零。
- sealed后通过要求计数与中央完整确认逐文件一致；failed也可保留安全有效的原XML，坏/secret XML永不公开。XML无法作为可信原文件时只保存固定失败原因，不编造文件或计数。
- 尚未实际执行普通动作、precheck失败、全部when false：不建立ReportEvidence/seal，无假required失败。

## Store持久记录

原buildRecord增加ReportRevision int64、ReportFinal bool、ReportsJSON（当前完整checked/sealed数据）、ReportSealDigest string、ReportCheckedIndex int（最后被检查ordinary run）；ReportFinal只在最后Index0 checked置true，seal后不可变。未配置旧记录均零值。last_event_seq/receipt仍原同一事务，不新建独立事件cursor。

原artifactRecord增加Purpose、ReportRevision、ReportKey、VerifiedJUnitJSON：Purpose空或artifact按原规则；junit记录server针对稳定stage重新解析的Counts/Diagnostics，Node请求不能填写VerifiedJUnitJSON。artifactID、attempt、内容不可变，StorageID仍私有。junit归属必须原Definition有reports、最后final checked revision中声明的Key/ID、真实ordinary run来源且Stopped/!CleanupFailed。数量按用途分别计算、size仍合计，普通制品最多128份，junit数量按冻结max_files（默认256，1–1024）；总计4GiB与junit累计64MiB不变。

只有final checked的报告文件允许上传；seal事务要求当前Files每ID已CommitArtifact、Ref/Revision/Key/Size/SHA/Source匹配、server解析Counts一致，再由server验证结果重算aggregate。旧revision/缺文件/只有summary/冲突均不能seal成功。失败型错误允许无坏XML文件的固定原因，但永不成为passed；有效Files仍需完整确认。

ReportManifest={SealDigest,IDs[]}放在build_finished，精确等于本attempt已seal的canonical报告集合（空也是显式[]）。ReportSealDigest为SHA256(Sealed=true的ReportEvidence规范JSON)，不含本地路径、lease未来期限或自引用digest。Log/ArtifactSteps旧完整manifest仍核对；LastArtifactSeq沿原所有用途文件序号。

## Store执行约束

ordinary run finished后若reports配置且Started/StopConfirmed、无CleanupFailed，下一ordinary intent前须收到对应index reports_checked；假source、未Started、post来源、旧revision拒绝。报告failed后拒下一ordinary intent，允许原真实failure/cancel结果后的合法post流程；condition/not_started跳过不伪造run。

final checked Index0仅ordinary全部terminal后接受（零动作不接受）；post_selected须ReportFinal且sealed，同普通原失败/取消/timeout和ReportOutcome推导对应phase/reason。terminal必须exact seal+IDs+中央解析的count，不能通过node自报summary放行。

原命令失败Reason/ExitCode、CleanupFailed、cancel与ns预算沿007。报告错误只能将原成功转失败，不能覆盖已知原失败；post不改变普通ReportEvidence。失权或保存不确定不继续用户动作，Guard/隔离保持；partial report上传不作为可信终态。

## 与008恢复/重试

008的receipt.Kind/StopKnown只有完整build_finished事务成功后才可信；报告seal/manifest包含在此验证内。尚在checked/传输/不完整seal的journal不清理或重放。retry使用原Definition.Reports/参数/事实和新build.number、新attempt，全部ReportRevision/Files/Seal/seq为空；不复制原报告通过结论。恢复不读取post改写文件，不重置ordinary/post ns。

2026-10-08 数量增量：默认256、max_files=1–1024；普通制品128份单独计数，累计字节/cases/诊断/时间上限不变。报告相关消息8MiB，执行journal64MiB/100万节点；冻结配置、审批与恢复使用同一配额，极端元数据与历史仍可能触及独立预算。
