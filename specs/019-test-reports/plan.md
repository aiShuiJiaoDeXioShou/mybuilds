# 实施计划：019 本次执行的测试报告与发布证据

**Branch**: `019-test-reports` | **Date**: 2026-10-04 | **Spec**: [spec.md](spec.md)
**Input**: `specs/019-test-reports/spec.md`，18 FR、6 SC、4 US / 12 AC。
**状态**: 正式analyze与implement完成；本地、双库HTTP/Agent、macOS/Linux、最后20故障和必要全量检查通过，正式converge无缺口，待一次整功能提交。
**已验收基线**: `504dc6fa8581f74a15ecc146a474976d5ae33a22`（008），含已提交共享process修复。003、007、008前置满足；005未验收代码不借入。

## Summary

复用同一 pipeline.Run：普通 run 启动前建立本次报告基线，结束后对新建/改写的 XML 做受限快照、解析及按路径替换；任何已生成失败/错误/非法报告立即阻止下一普通动作。准备阶段缺失不失败，普通结束才按 required 做最终检查，完整确认最终 XML 后封存，再选择 post。命令原失败/取消/超时不被后续报告错误覆盖；post 不重读、不替换普通报告。

原 XML 由已有中央文件上传/下载路径承载，新增有限 purpose=junit / revision / key 的真实归属，不伪造 artifact 步骤。服务端解析已校验 stage 原字节并核对摘要；Store 约束 run 来源、revision、完整确认、封存、post 和终态。含已声明秘密的 XML 在 Agent/本地拒绝回传，合法可下载内容不重写。报告耗时及传输确认计普通纳秒预算，失权/持久化失败沿007闭锁。

## Technical Context

- **Language/Version**: Go 1.25+，现有单模块；注释/文档中文。
- **Primary Dependencies**: 不加依赖；encoding/xml 流式 token、标准库整数/时间/哈希/文件与现有 doublestar、Cobra、GORM/Viper。现有 SQLite 3.53.3 修补版本与 PostgreSQL 驱动不变。
- **Storage**: 本地私有结果根的 XML 快照和报告 manifest；Agent 原 journal；中央原 artifact 文件/表增加真实 junit 用途，build/attempt 的报告 checkpoint/seal 通过既有事件事务保存；双库同规则。
- **Testing**: 测试先行；真实本地脚本、Store 双库、真实 Server/Client/Agent+Git/HTTPS，macOS/Linux 节点实际生成 XML、断权/数据库和文件失败、中央离线下载；test/vet/race 与客户端纯 Go 跨编译。
- **Target Platform**: 执行/控制端 macOS/Linux，远程客户端跨平台；不扩展 iOS/Flutter/商店工具链。
- **Project Type**: 既有三个 CLI 和鉴权 HTTP，有限纯报告解析包供两个实际消费者。
- **Performance Goals**: 单 XML≤8MiB，当前报告集合默认≤256文件（max_files=1–1024）/64MiB/100000 cases，流式深度≤64；纯本地检查/快照/解析每次≤10s且不超过剩余普通预算，网络文件沿007单次2m但同受剩余普通预算限制。
- **Constraints**: 原工作树旧文件不删除；只读本 build 匹配的普通文件；拒 symlink/FIFO/DOCTYPE/外部实体/超限/秘密；固定执行权、完整回执和纳秒预算不可重置；无报告配置不读目录、不增加事件或结果字段。
- **Scale/Scope**: 只实现 reports.junit；保持四种步骤及 succeeded/failed/cancelled/skipped/interrupted，不加 unstable、插件/报告平台/第二执行器。每attempt普通制品≤128份，JUnit数量按冻结max_files独立计；合计4GiB与报告累计64MiB保持。

## Constitution Check

规划前与设计后均通过：I 既有 specify/checklist →实际 setup-plan →七份设计，后续tasks/analyze通过才实施；II 唯一Run与三个原入口；III 标准库/两个真实解析消费者，无泛型仓储或通用文件协议；IV 输入有界、受信脚本、秘密拒绝、旧fence拒绝、事务与完整文件确认、失权不重跑；V 中文与实际双库/macOS/Linux行为门。没有原则例外。

## Phase 0：研究结论

见 [research.md](research.md)。明确已有 Reports schema、Trigger/Enqueue/Run 三道 unsupported；已有 collector 非空/目录不可覆盖语义不能直接替代准备缺失与路径替换。复用文件边界而保留具体 report 集合。标准库 parser 的 XML 语法正确性不等于 JUnit 结构/计数/限额正确性，增加有限 token 状态检查。

## Phase 1：执行及持久化设计

1. 整批预检查所有选中 build 的报告 glob/一次模板与渲染路径；最终 Trigger、Store.validatePrepared 和 Run.prepare.build 均接受严格 reports（先形成真实本地Run；Store checked批接严格入队供真实中央测试，完整文件/seal/Agent消费者到位后才放开公开Trigger），仍拒其它生效未实现能力。dry-run 只预览不读文件。后续节点才确定的合法系统变量保持 pending，不能把它当 build.when false。
2. 在每个实际 build 第一个普通动作前，以 os.Root 和非阻塞普通文件读取记录匹配文件 identity/mtime/SHA。旧 XML只作有界基线，不解析计入；全新远端 workspace 也不把仓库内预置 XML 当本次产出。零动作 precheck失败/build.when false/所有steps条件跳过，不读取或建立假seal/required失败。
3. 每个实际 ordinary run 停止确认后，尚有 Authority 且无 CleanupFailed 时检查报告。用字节稳定的私有快照解析，同一路径替换，未修改的已接纳路径不重复汇总；消失的路径从当前集合删除。若未产生新文件，准备阶段可继续。已有 XML非法或失败立即失败，剩余普通动作 not_started。
4. finished run 的真实Started/StopConfirmed先持久化，然后 reports_checked 事件确认报告 checkpoint；下一普通动作须在该检查回执之后。来源必须该真实 ordinary run，ArtifactSteps只保存真实artifact。
5. 普通段完成后执行最后检查及 required（每声明模式至少一个本次文件；required=false只豁免缺失）。final reports_checked 的 Index=0 表示普通边界，然后只上传该最终revision的 XML，最后 reports_sealed。无效/秘密文件不进公共文件集合；安全失败证据有固定原因，无伪造原文件或计数。
6. Store 校验最后revision/按路径唯一key/文件完整回执与服务端重新解析摘要，再原子seal；post_selected 必须与原普通命令/取消/报告结论一致。报告检查、快照、解析、上传和seal回执耗时全计ordinary预算；0预算不能退回无限。报告收集在用户cancel后只可用原Authority及剩余预算的短上下文，不能因WithoutCancel恢复失权。
7. 已确认普通命令原失败/超时/取消优先；只有原成功才因 report_failed/report_invalid/report_missing/report_secret/report_error 改failed。剩余预算为0时原成功为timeout。记录报告自身失败但不覆盖原Reason。日志/报告保存失败或Authority失效闭锁整Run，不能启动always；普通报告业务失败可在仍有效Authority内选择failure/always。
8. post仅诊断，最终ReportEvidence不可更新；build_finished完整核对seal、全部最终报告ID、已确认内容摘要/服务端解析结果、日志/ArtifactSteps原manifest及原budget。precheck/全跳过保持007零动作终态规则；不假建报告文件。可信成功不能只有summary/部分传输。
9. 本地BuildRun与中央BuildView复用受限摘要及XML元数据；artifact现有下载可访问已seal报告，未seal/旧revision仅Agent同fence确认入口可读。admin/approver读，trigger/node不读用户证据；内核私有SnapshotPath不进网络或JSON。

## Project Structure

```text
specs/019-test-reports/
├── spec.md / checklists/requirements.md  # 原规范冻结，未改
├── plan.md / research.md / data-model.md / quickstart.md
└── contracts/
    ├── reports.md
    ├── go-api.md
    └── node-http.md
internal/reports/junit.go, junit_test.go  # 纯解析，真实pipeline+server消费者
internal/pipeline/reports.go, reports_test.go
internal/pipeline/run.go, remote.go, run_types.go, artifact.go  # 原Run与有限快照边界复用
internal/agent/reports.go, reports_test.go  # 原journal/事件/中央上传消费者
internal/store/reports.go, reports_test.go # 双库检查/确认/封存
internal/server/reports.go, report_test.go # 稳定stage解析及原文件HTTP purpose=junit分支
internal/cli/client/report_test.go        # 原build详情/artifact下载闭环
```

### 唯一 writer 与008交接

| 范围 | 019 writer | 约束 |
|---|---|---|
| internal/reports新parser及tests、internal/store/reports.go及tests | A | Store只保存/约束具体类型，不import执行器 |
| internal/pipeline/reports.go/tests、internal/agent/reports.go/tests | B | 仍一次Run，真实回执失败闭锁；原进程代码不改 |
| config/validate.go与报告tests、pipeline/preview.go与报告tests、server/reports.go及独立HTTP/解析tests、CLI原build/artifact消费者与tests | C | 复用严格schema/旧路由；不改原流程为第二入口 |
| protocol/node.go报告类型+omitempty字段、pipeline/run_types.go字段及run.go/remote.go/artifact.go原Run调用接入、Agent journal.go/execute.go/artifact.go共用接线、Store event.go/enqueue.go/models/node_models/query/artifact.go及retry.go/recovery.go共用字段与dispatch、Server files/artifact/http/json/trigger共用接线 | root 串行 owner | 子区先交具体最小补丁，root一次写入/同步各WT，既有文件不与008并发写 |
| README/历史/validation/tasks/最终go.mod与整合提交 | root | 不新增依赖，不由分区提交 |

A/B/C为能力分区，不锁代理名字，最多root+A+B+C四slot；不同writer共享文件先冻结具体补丁、root串行同步，未完成实际前置不标并行。

008仅新增terminal receipt的Kind/StopKnown与精确只读回执，不新增Report字段；019 terminal完整核对完成后才产生该StopKnown回执。旧artifact purpose为空、所有新增可选字段omitempty，原digest不改变。008retry继承Definition.Reports，但报告revision/IDs/seal/seq/原工作区不继承；新执行新证据。008恢复只引用已seal确认数据，不重解析post XML、不重置预算。具体共享字段以本contracts为候选，root复核后串行冻结，不要求008新建报告占位能力。

## 真实验收与交付门

US1真实脚本生成通过/失败/错误/跳过、嵌套suite和同路径改写，命令非零仍收新XML；US2预置旧XML/同内容重建/缺失/required=false/--step、非法/实体/深度/秘密/路径/非普通文件；US3准备→测试→哨兵、failure/success/always与post改写、普通及post纳秒预算/失权/磁盘保存失败；US4真实macOS/Linux节点、SQLite/PG相同三二进制、回执丢失/部分文件/旧fence、中央离线下载/权限/坏文件。再回归无reports全部007行为及必要test/vet/race/纯Go编译。008恢复/审批/商店/iOS门不由本功能冒充。

## Complexity Tracking

无原则违反，无需复杂度例外。唯一新包internal/reports只有真实纯JUnit解析，不提供registry/interface/hook/多格式框架。新增checked/sealed是本功能实际阶段证据，不新增脚本步骤或第二执行协议。

## 2026-10-08 增量实施计划

基线 main@21116f7。仅提高报告文件数量并支持每 build 配置；既有64份及报告与制品合计128份描述由本节取代。默认256，配置1–1024；普通制品仍128，JUnit累计数量使用冻结配置，字节/cases/诊断/执行预算不变。

1. config定义唯一默认/最大值及有效值读取；严格schema验证max_files，可省略，不接受零/null/非整数。pipeline准备、构造、扫描、检查点恢复均传入有效值。
2. Agent声明和审批上传、Store证据和文件确认使用冻结有效值；普通制品配额排除purpose=junit，发布候选先过滤用途再Limit，保持原顺序与幂等。无快照的只读恢复使用绝对1024边界。
3. 报告events请求与claim响应、build详情响应及审批摘要允许8MiB；普通管理请求仍1MiB。严格JSON节点上限按报告容量提高，重复/null/深度检查不变。journal写入、审批/终态恢复、retention只读守卫统一64MiB及100万节点；该独立预算仍可拒绝极端诊断或过多历史，不能将max_files宣称无限容量。
4. 先验证配置与默认/自定义边界，再执行真实大量XML收集、Store+HTTP上传封存/恢复与中央下载、长路径消息，回归报告失败和无报告流程；不新增依赖/框架，不改XML解析器。
5. 根代理串行拥有所有代码、文档与提交；Phase0仅授权只读研究子代理审计传输/恢复消费者，不并发改写。必要全量test、相关race、vet与纯Go构建通过后converge并本地一次提交，无push。

Constitution Check：复用019与唯一Run，无新依赖或抽象；有限输入/租约/持久化/原XML与失败门保持；中文文档与本地提交满足原则，无例外。
