# Implementation Plan：发布审批与原节点检查点恢复

**Branch**：014-approval-current | **Date**：2026-10-05 | **Spec**：[spec.md](spec.md)
**Input**：30FR/8SC/20AC已冻结；现有tasks复用；fresh2602094保存Root实际发布消费者baseline，修正文档后analyze再完整implement。真实商店/Apple上传与最终Flutter案例人工待验。

## Summary

唯一pipeline.Run在生效approval形成可保存检查点，先封存产物/报告/日志、回收真实进程及Close本次签名资源，不执行post；Agent持久本地准备态、停renew并等待请求退出，在原Authority内以现有ApplyEvent新approval_checkpoint事件提交完整证据。Store短事务核对原归属/seq/digest/预算/所有确认，才waiting_approval并释放执行容量，保同名互斥及原node/workspace。决定精确ApprovalID+Revision+CheckpointDigest；approved后Claim只原合法node、新session/lease、同build/attempt epoch++，Task.Resume携具体ledger。仍调用原Run且跳到下一未执行索引，不checkout/重建/重跑，不另建executor。新进程对已确认paused证据只读核对，unknown全部拒绝。

## Technical Context

- **Language/Version**：现有Go1.25、标准库HTTP/context/JSON/os.Root、已有x/sys Unix poll/TTY和process.Run；中文注释文档。
- **Primary Dependencies**：原Cobra/Viper/YAML/GORM双库；不新增依赖、通用state-machine库或持久化runner框架。Unix真实TTY用现有x/sys，不引入x/term。
- **Storage**：新增具体approvalRecord/immutable checkpoint JSON与decision audit；现有build/attempt/step/cursor/lease/claims更新。Agent仍journal/spool/结果目录；只有真实paused记录有最小私有workspace身份。
- **Testing**：先真实旧模型/严格消息/Run红绿，双库同suite20竞争、旧fence/receipt/NS/容量；真实TTY/EOF/SIGINT，macOS/Linux两个节点，原工作区与file篡改，实际签名Close及中央XML/原产物。
- **Target Platform**：server/Agent macOS/Linux，客户端原交叉构建，Windows本地Run仍不支持；005完整组件合入后消费其真实生命周期，合法Apple材料人工待验，不计真实签名通过。
- **Project Type**：现有三CLI与HTTP，CLI审批，不通知/Web/外部审批。
- **Performance Goals**：不新增SLA；等待不占执行容量且不扣原active NS，准备/封存/清理/恢复校验与执行耗时照原累计，0预算拒新动作。
- **Constraints**：approval/decision消息≤64KiB，私有Resume ledger≤1MiB、JSON深度≤32/节点≤10000；note≤1024 UTF8字节/无control/拒明确secret引用或token字段格式；原意见只私有DB保存，公共DTO仅SHA-256摘要，不承诺识别未送中央的Node实际值；列表limit1..100、offset0..1000000，沿现有Page。共享原artifact128/4GiB、reports limit不增加，journal128/1MiB/0600/单link。局部TTY输入≤256字节，以有限poll响应ctx，无弃置reader goroutine。系统Close独立15s/more-early-stop约束；审批无自动超时批准。
- **Scale/Scope**：每个冻结approval一个检查点、一次决定、多审批顺序；没有多项目审批编排、通知、迁移/重做、旧PID kill或任意resume入口。

## Constitution Check

原则2.1.0 Phase0前/Phase1后PASS，无例外。I：当前仅规范/规划/tasks/只读分析，正式前置交付后才源码红绿/converge；II：继续同一Run/Store三入口；III：只实际审批record/Resume消费者，不plugin/second executor/未来placeholder；IV：完整fence、原证据/工作区/预算、unknown保守、当前审批角色/发布授权；V：真实双库两平台TTY/签名Close/坏证据红绿，中文与状态如实。

## Project Structure

七文件plan/research/data-model/quickstart与contracts/{go-api,checkpoint-protocol,http-cli}.md；spec/checklist不改。本轮创建tasks.md并只读分析；不创建validation/源码。

| 当前唯一writer | 实际文件范围 | 串行边界 |
|---|---|---|
| A 本worktree完整014 | 审批protocol/models/migrations、Store新approval文件及原claim/lease/query/stop/recovery/retry/node/retention接线；唯一Run/remote/Agent/journal/CLI/server实际消费者、测试与014文档 | 仅本worktree写，保存403源基线后增量交付；不在Root/其它WT并写 |
| Root | 本轮原internal/store/event.go、reports.go；最终005字段同步、商店publish与014最小diff集成 | A给实际private方法/最小patch，Root串行接；不写并行占有文件 |
| B 012/custom | 独立WT的真实Custom发布消费者 | 审批不新增Node可伪造proof；同Store私有validatePublishApprovals在实际AuthorizePublish事务核验 |

原A/B/C拆分任务保留稳定ID，但本worktree当前完整审批由A顺序实现；不创建第二executor/泛型状态框架/空方法。Root已授权共享入口（除event/reports），最终只应用snapshot后的增量，不覆盖020关闭/资源登记/删除保护。

## Phase 0 / Phase 1

[research.md](research.md)基于实际007 Run/Agent/store及root008/019/010/011/020规划只读；WT基线007 85b46bf，008正在根收尾，未来前置未计已验收。设计最难点是把当前whole-Run执行改为具体cursor而不重放：只RunOptions.Resume追加已核对ledger，旧无Resume完全原行为；remote后续progress保持全attemptseq/cursors，不重新发build_started/旧steps。

实施顺序：①共享模型/协议/旧JSON与迁移真实门；②Run pause/no-post/Close/flush/完整checkpoint事件，Store同事务等待与容量/guard及真实paused Recover启动；③决定精确ID/revision审计及竞争；④原node Claim恢复新epoch、Agent已有paused事实/目录身份验证+Run.Resume、旧fence/manifest/history不改；⑤本地TTY/noTTY预算暂停与取消；⑥双库/mac/Linux实际故障/签名/产物/预算、全必要检查、converge/一次本地提交。

批准后的重新授予不是第二个Run引擎：同一Run函数再次进入时在内部准备/循环接入受验证的Resume状态，只有尚未执行范围能创建命令/签名资源。首次暂停与续段是一个构建/attempt的不同有效lease区间，不重放普通/post。原签名资源已Close，发布段无新ordinary时不Prepare；非发布普通审批后真正还有run才重新Validate/Prepare新的本次资源，预算仍原剩余值，原封存artifact与ReportSeal不重建。

## Coverage / 真实门

| 门 | US/AC | FR | SC |
|---|---|---|---|
| G1 安全checkpoint/容量/重启 | US1/1–4 | 001–006,017,023,024,028 | 001,004,007,008 |
| G2 精确决定/角色/并发 | US2/1–4 | 006–011,020,022,028 | 002,005,007,008 |
| G3 原节点Resume/新fence/原NS | US3/1–5 | 012–018,023,024 | 003–005,007,008 |
| G4 条件/取消/多审批/发布 | US4/1–4 | 019–024,027,030 | 005,007,008 |
| G5 真实本地TTY/无post | US5/1–3 | 025–029 | 006,008 |

30FR/8SC/20AC都有门。等待/approved时不产生StopKnown终态，checkpoint ACK丢不使用008terminal清理；actual Publish未知保护独立。020保护待审批/approved/原工作区，012/custom与020后续联验计整MVP，不反向成为014提交前置，避免依赖循环。

## Complexity Tracking

无违例。只一种具体审批检查点/决定、原Claim的Resume分支和唯一Run游标；不创建runner interface、分段executor、snapshot DSL或第二审批状态体系。

## 当前实际基线与用户人工门

2602094已交付020；Root当前发布cmd/internal/mod/sum共403文件作为额外冻结baseline逐SHA复制并保存独立字节快照。已有发布消费者编译真实存在，005尚未合入时不伪造其资源能力，等待Root冻结同步后直接消费。014完整实现不等合法profile/商店材料；真实Apple签名与商店副作用人工待验，必要自动门用真实Run/文件/报告/Store/HTTP/身份/TTY，不能用fakePublisher证明上传成功。最后统一Flutter案例说明由Root整合。


### 真实报告检查点边界

含upload的审批属于发布段：全部普通run/artifact完成、报告passed且final/sealed后才能安全挂起，Root reportsFinal门不放宽。没有upload的midrun审批可位于后续run之前（CONFIGURATION:507、FR001/002/014/017）：仅持久已checked的同一revision、原XML快照与具体collection状态，不虚称final/sealed；Resume继续同一collection/原baseline/当前entries，后续实际run再检查，全部ordinary结束才final seal。没有第二report epoch、重采旧XML或重新执行脚本。私有报告collection记录只Agent，中央核当前checked证据/已确认XML/cursors；变化/缺失闭锁。

### 本轮实际收敛边界

实际源基于第三448文件冻结byte baseline交付；只将014增量合并，不覆盖005/009/010/011/020。原Claim实现位于 lease.go，HTTP实际入口注册于 server.go/agent.go；TTY具体文件为 client/approval_tty_{unix,darwin,linux,other}.go。私有 ReportCollectionCheckpoint 保存原指纹、baseline与有界快照路径，恢复不重采。私有 IOSResourceOwnership.TeamID 与关闭摘要绑定；发布段只恢复真TeamID，无后续ordinary不重新Prepare，有后续run才按已批准的旧关闭摘要开始新资源生命周期。公开意见仅SHA-256摘要，原意见私有审计；不能承诺识别未传中央的Node secret值。

用户最新安排优先完整实现与必要自动检查，最后以Flutter统一案例做人工联合验收；本轮不为重复平台/商店材料矩阵增加执行器或假状态。最终Root集成检查仍须执行，未完成的既有任务保持未勾，不能以目标测试代替整功能正式验收。
