# 014 规划验证记录

早期阶段完成Spec Kit specify/plan；后续tasks/analyze见下文，实施代码与提交尚未完成。30FR/8SC/20AC与项目原则2.1.0已映射；实际setup-plan、resolver与require-spec完成，hooks={}。根已阅读规范、计划、唯一Run/Store/Agent接口及精确检查点协议，核对九份源文件SHA后串行复制。

冻结清单 `/tmp/mybuilds-mvp.zKtK0e/approval014-plan-final-manifest.json` SHA256 `0325add42385f18486459247648982e09d864cb3ad9a796719e3a67dd4eb5fec`；覆盖清单 SHA256 `f9970dfbd3b479b5510f3ec60667fc79f11eadef4b03ee8506a3aa4b29bc12c4`。原spec/checklist字节未改。六本地链接、五bash块语法、围栏及diff检查通过；这些是规划检查，不代替实际双库、原节点恢复、签名Close/发布或真实TTY验收。

正式源码等待008、019、010/011交付；仍用一个Run及原Claim/ApplyEvent，checkpoint旧fullRef独立保存，未知停止/领取拒接管。012/020联合门后续完成，避免反向循环依赖。

## Tasks 与只读分析（2026-10-04T18:36Z）

独立worktree生成62项未实施任务，38项FR/SC与20个验收场景全部覆盖。首次两项HIGH顺序与一项MEDIUM唯一writer问题在analyze外修正，最终只读analyze零问题；实际setup-tasks与prerequisite、7处链接/5个shell块通过，hooks={}。任务SHA e9bc9fe0050271973be42a202c5d5b8b6278a149d3b6b9d5a9ac1b1cafa81cdc，planSHA 92864950ecf9e0b1b3a65102d45f370236c35d7115138ed7a79d24bfe5b60beb。这里只是规划，008/019/010/011及签名前置仍未验收，不实施或假造消费者。


## 当前完整实施准备与Foundation真实记录（2026-10-05）

Root授权fresh `approval014-current`基于2602094完整实施，旧规划仅为历史记录。用户最新要求完整代码和必要自动检查先交付，合法Apple/商店上传与最终Flutter案例人工验收；不会以缺材料预留空业务或模拟成功。

独立0700证据目录 `/tmp/mybuilds-mvp.zKtK0e/approval014-92_j1xaj`：第一403源baseline保存Root当前cmd/internal/mod/sum逐SHA字节；0058e624合入后第二435源baseline；009802cb小diff27路径只同步未由审批修改的Root实际文件，第三448源baseline为后续增量交付基准，前两份原字节保留。未修改Root/其它WT/020旧证据，未commit/push。

在技能外修正文档manual门/本worktreeA唯一writer/Root原event与reports独占，再执行只读speckit-analyze；38FRSC/20AC/62唯一task全覆盖、5原则、0阻塞，input前后SHA一致；prerequisite PASS、requirements16/16、hooks={}。Root明确无upload midrun approval允许保存当前checked revision而不虚称final/sealed，resume原collection继续实际后续run；发布段仍要求全ordinary通过final seal，原reportsFinal门不得放宽。

T003首先真实运行旧消息反射/JSON兼容门，缺Optional Approval/Resume runtime RED；新增具体可选wire后green0.416s。随后10个Ref/close/stop/cleanup/Started/负budget/null数组/next/phase非法shape实际runtime RED，具体CheckpointDigest绑定原Ref/seq/预算/cursors且不循环自身摘要；最小shape校验后green2.200s。一次unused time编译失败原日志保留，不冒充行为红。HTTP duplicate/null/unknown读取边界尚待实际HTTP阶段，因此T003不提前全勾。

T005迁移先通过真实Open/Migrate/CreateProject/Enqueue/Claim/ApplyEvent创建旧完整执行及终态receipt，再重复Migrate保持所有事实：SQLite/PostgreSQL都因缺approvals表runtime RED。实际approvalRecord/CurrentApprovalID迁移后green1.586s；补原真实父关系作模型约束夹具，build/index与build/revision唯一、revision正数、build FK、attempt/node RESTRICT以及重复Migrate两tops/4DB子项green2.092s。仅模型迁移门，不将direct约束夹具当安全挂起/批准/恢复业务PASS。

自有PG仅 `mybuilds014_tests`（root已有fixture socket55436），每case自有schema串行；未操作其他库/服务生命周期。Foundation尚未完整审批，继续真实checkpoint/角色/恢复/TTY/保留保护与必要检查；未通过014全功能或真实签名/商店人工门。

## 014 本轮真实实现交付（2026-10-05）

完整生产消费者已落地：原Run暂停/唯一Run续执行、原节点同attempt新epoch、精确审批决定、角色与CAS、HTTP/CLI、本地PTY、报告collection保存/恢复、实际资源身份与原artifact流式核验、关闭iOS私有team元数据、发布授权审批门及历史只读证明。没有新执行器、自动批准、秘密传输或伪造终态。第三448文件byte baseline为增量唯一来源，不直接用260的Git大diff覆盖后续005/009/发布/020。

关键真实检查：

- 协议旧可选nil消息兼容、严格shape及双库迁移/唯一/FK红→绿：原 red 日志保留，green见本目录指向的外部证据。
- 实际SQLite/PostgreSQL：Enqueue→Claim→真step事件→checkpoint→决定→同attempt epoch+1；旧fence拒、同决定幂等/冲突拒、20个一致并发决定只有一行、真正trigger身份拒；控制端Close/Open/Migrate/Recover保持原等待检查点。意见自有敏感标记不进入公共DTO，公共note_digest与原私有Note准确对应。
- 实际Agent/HTTP/固定自有Git/唯一Run/真正CLI approve：before一次→pending报告原XML中央确认→已确认pause→原Agent退出并按007原2×lease更换session安全窗口重启→原workspace同attempt续after一次→原XML ID复用→真正final/seal→post/终态与journal清除。最早3.05s单次CLI闭环已绿；最后加入真实重启的必要race门24.66s/package26.967s绿。没有重新checkout或重跑before。
- 真PTY yes/no/等待取消与无TTY；local Run只明确批准继续，拒绝无后步/post。实际AuthorizePublish双库检查：冻结跳过approval仍拒grant并没有发布intent；没有以when跳过代替批准。
- 恢复快照拒替换符号链接，读取有界/非阻塞/前后身份/内容SHA核对；原artifact采用≤128/4GiB的流式原快照验证，不分配整包、不重生成。原IOS资源team值在私有ownership和关闭摘要绑定，Close后发布段恢复该已验证值；没有从callerEnv取得分发TeamID。

最终证据文件位于 `/tmp/mybuilds-mvp.zKtK0e/approval014-92_j1xaj`，各文件SHA在交付manifest。`final-race4.json` 其它五包通过，但首次新增重启测试过早停在checkpoint ACK未确认时实际失败 `journal_unconfirmed`，保留原保护和失败记录；不将这轮整体写成通过。最终 `final-restart5.json` 在真实confirmed ACK后原Agent重启通过；`final-store-history.json` 73个top/sub结果、真实双库必要Store/历史回归race通过21.980s。`final-race2.json` 最早受影响六包目标race全绿，是较早源码检查点，不冒称最后bytes。最新源码vet相关七包通过；最后Store改动后再vet Store exit0；无依赖新增。

精确命令（PostgreSQL使用本区独立mybuilds014_tests，自有schema，不操作Root服务生命周期）：

```bash
MYBUILDS_TEST_POSTGRES_DSN='<本区专用DSN>' go test -race -count=1 -json ./internal/store -run '^(TestApproval|TestPublishActualAuthorizationRejectsSkippedApproval|TestPublishOnceUnknownAndExactDecision|TestPublishRecovery|TestRetentionHistory)'
go test -race -count=1 -json ./internal/agent -run '^TestServeApprovalActual'
go test -count=1 -json ./internal/cli/client ./internal/pipeline -run '^(TestApproval|TestIOSPreview)'
go vet ./internal/agent ./internal/pipeline ./internal/cli/client ./internal/server ./internal/store ./internal/protocol ./internal/mobile
```

本轮源码收敛核30FR/8SC/20AC、5原则：0新增代码缺口，仍有1项Root最终串行集成/实际字节验证的partial，按技能只append T063；未宣布整功能已converged/已提交。原T056/T057与T061/T062由Root最终whole检查/整功能提交继续，旧未实际执行的大矩阵不勾成通过。用户最新全MVP最后统一Flutter人工案例、合法Apple签名/商店上传保留人工待验；原材料未知不改变Close、currentAuthority或unknown保护。没有操作旧VM/Agent/guard，没有commit/push。

发布串行接线注意：AuthorizePublish在真实publishReports/原artifact授权同一事务调用validatePublishApprovals；Custom/Apple/Play共用此实际门。FindNodePublish旧Ref只可经approvalHistoricalRef同build/attempt/Node的实际已批准续执行链只读查询，并且存储Grant.Ref须仍exact等于请求原Ref；Record/新授权仍currentExecution拒旧fence。Agent原Grant与Receipt.Ref不重写，receipt.Ref==grant.Ref，终态state只通过approvalRefKnown核历史链。Root最新publisher终态恢复helper若原强制grant.Ref==terminalRef，合并时需消费此同一具体链，不能放宽跨attempt/Node。


## Root集成与最终代码验收（2026-10-05）

基线008/019/020、005/009与010/011已本地提交；012提交76d3000，发布HTTP限额独立修复55750d1。主代理按448实际字节baseline三方集成，保留当前PublishIDs/Origin、原发布Grant/Receipt Ref和签名资源、预算/报告/保留消费者。正式收敛核30FR/8SC/20AC、六项计划决定、5原则；发现并关闭T063–T066代码缺口：最终集成、旧发布Ref的精确manifest、notify:true提前拒绝、最后审批恢复后原post。无新增代码缺口，未追加空阶段。原材料/跨OS大矩阵保持明确人工待验。

真实ACK后服务取消竞争、原019额外XML资格和Windows资源兼容已按缺陷流程修复，见.specify/bugs/approval-restart-journal；通知预检查和两次暂停最后仅post的真实红→绿分别见approval-notify-precheck、approval-resume-post。未放宽unknown、同节点/attempt历史、Close或发布锁；旧失败日志保留。

- 最后一次整项目普通检查使用双库`go test -p 1 -json ./...`：2173个top/sub项通过，发现上述真缺陷和旧未支持断言；保留失败记录并仅重验受影响范围，不把原失败写PASS。无变更的distribute/mobile/process/protocol/reports/scm及完整Store通过，Store105.194s。
- 修后config/pipeline/client/serverCLI完整normal通过1.442/9.358/33.447/2.965s；既有失败断言与HTTP门5包目标全部通过，新增真实Run三场景正常通过。
- 最新Agent原实际重启/原报告canonical/历史资格及Store审批双库race通过28.024/8.266s；最后受影响pipeline/config/client/serverCLI/Server目标race通过2.825/3.426/8.723/6.737/9.232s。
- 最终whole`go vet ./...`、diff/gofmt通过；三个入口×darwin/linux/windows×arm64/amd64共18次CGO0编译和6次本机help/version通过。编译不声称Windows本地执行或Linux iOS能力。
- 最终实际三二进制+自有Git/profile/custom verifiedHTTPS接收器：SQLite/PostgreSQL各手动及自动同一流水线，合计118/118项通过。每次两精确批准后同node/attempt/number epoch3终态；upload一次、原epoch2 Grant及manifest不改、实际always收尾、原binary/XML完整下载、只读metadata GET query不重写终态。自动待审批时新无ID事件复用exact原ID且不占号；手动构建不被自动语义合并。8个自有Popen均wait退出，source-before/after一致另见案例验证记录。

合法Apple签名、真实两大商店发布、外部托管push及完整macOS/Linux人工故障矩阵由用户按quickstart和examples/mvp指南验收；本记录的通过仅覆盖上述实际自动门及自有custom案例，不宣称真实商店成功。

最终脱敏118项证据SHA-256 `0c1922150daeab9af079a46cdaac7557affdb7a7b71221ea1a8cb50760ca5eeb`；清单 `fb80c40c246f199c2ae43a43ade4643a6caa3a4c8da7175f6277af3e0bda89ff`；生产文件集合摘要 `1e390b1e06b5e4ed0d7ca3c1cc5ed58418b33d3c93ef419328ae7d1cd5204cc1`。所有80证据文件逐SHA核对，生产字节与已执行案例一致。
