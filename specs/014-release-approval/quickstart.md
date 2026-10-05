# 014 真实审批与原节点续执行验收入口

代码和必要自动验证已完成，真实签名、商店及跨OS完整人工矩阵按本文验收。最新结果见[validation](validation.md)，集中案例见[Flutter指南](../../examples/mvp/acceptance.md)。接口见[HTTP/CLI](contracts/http-cli.md)、[checkpoint协议](contracts/checkpoint-protocol.md)与[Go API](contracts/go-api.md)。

## 实际准备

使用独立SQLite/独立PostgreSQL应用库及自有server/Agent进程、两台节点macOS/Linux、私有0700 data_dir、0600 client/node凭据、verifiedHTTPS自有CA；不动用户服务、节点、宿主未知签名身份或系统trust。依006/007真实初始化与节点管理，admin/approver/trigger分离凭据；不把token传argv。相同fixture/API/CLI suite逐库运行且独占锁互不竞争，第二控制端实际被拒。

专用可信Git仓库中准备两个命名build：release有真实ordinary测试、019 JUnit、普通artifact、approval、明确授权upload；probe是真实短run/独立文件计数。固定完整SHA与项目/原node，repository所有动作在自己fixture，发布需要明确商店材料/应用授权与010/011真实验收，不能以fake publisher、exit0或无公开副作用的脚本代商店门。原签名资源需005真实指定材料与Close证明，无材料保该验收未完成。

本地TTY分支使用不含生效upload的相同approval配置；默认notify:false，通知真配置必须动作前拒。不要完整复制另一Run或借脚本trap假系统资源清理。

## G1 安全checkpoint与容量

```bash
mybuilds trigger "$PROJECT_ID" --branch "$BRANCH" --ref "$FULL_SHA" --build release --allow-upload --idempotency-key approval-fixture-1 --json
mybuilds approvals --project-id "$PROJECT_ID" --state pending --limit 20 --json
mybuilds build show "$BUILD_ID" --json
mybuilds trigger "$PROJECT_ID" --branch "$BRANCH" --ref "$FULL_SHA" --build probe --idempotency-key approval-probe-1 --json
```

实际release等待：原进程组/PID回收、真实签名Close、log spool/原artifact/hash/JUnitSeal中央确认、nullable普通和postNS准确封存，才waiting_approval与安全审批视图。probe在释放容量后真实执行；同项目同名release第二任务queued不越过guard。记录serverUTC/中央seq/完整manifest、自有PID/PGID与无关ownsleep存活，不用延时猜stop。

逐处真实故障：checkpoint POST未commit/已commit但ACK丢、Close失败、log未ACK、原file/upload不完整；不安全组合不能显示pending/释放guard。ACK丢后只读精确checkpoint proof并fsync原journal；控制端和Agent各实际退出/重启一次，等待不自动继续。unknown journal原inode/完整JSON保持，不人工删guard或假status。

## G2 精确决定与20竞争

从安全view抄原ApprovalID/Revision/CheckpointDigest，不自动从latest填。使用approver或admin受限client配置：

```bash
mybuilds approve "$BUILD_ID" --approval-id "$APPROVAL_ID" --revision "$REVISION" --checkpoint-digest "$CHECKPOINT_DIGEST" --note '已核对本次产物与测试' --json
mybuilds build show "$BUILD_ID" --json
```

20个真实HTTP/CLI相同决定并发，只有一条决定/审计，精确重复原结果；反向reject、改note、旧revision/另approval/hash冲突409且不覆盖。trigger/node身份403；机密扫描全部JSON/table/error无params/scripts/material/token/privatepath。拒绝独立fixture变cancelled/approval_rejected、不运行post/Prepare/upload，原证据保留。

## G3 原node/newlease/原NS

批准后Claim仅原node，保持build/number/SHA/attempt/definition/params/facts/已完成step与原日志/产物source，不再次checkout或执行旧动作；新session/lease/epoch++, next step实际一次，旧Ref所有event/log/artifact/publish拒。新的终态full manifest同时包含旧与新证据，Store真实核对，不改旧body/digest。

等待时间实际大于剩余普通预算，等候不扣NS；恢复校验/Prepare/续执行扣原值，0预算禁止新动作，post预算不增长。实际两领取者与丢ClaimACK至多一次后续run；本地旧task bookkeeping/renew join未结束不得再claim，未知领取新process不接管。原nodeoffline/drain/disabled/current权限或tool不满足，显示固定resume_reason且不迁移。

工作区缺失/同path替换inode、GitSHA改变、artifact/report bytes篡改、localledger替换、重复键/null/额外字段各真实负例；安全失败不重建/中央下载替代/重新报告测试。发布段不再次Prepare已Close资源；后续确需ordinary签名run才使用当次显式材料/新resources，原reportseal不重开。审批后真实store-app授权收窄/unknown保护仍阻止发布。

Recover重启必须验证original checkpoint fullRef+receipt，即使attempt currentRef已更新也可只读核对原pause；运行中仍验新currentRef，旧receipt不成新authority。008TerminalReceipt只最终build_finished，拒approval_checkpoint；安全pause rejection/cancel可以精确centralproof证明未恢复停止，不能伪build_finished或清未知resume journal。

## G4 条件/多审批/取消竞态

分别冻结approvalfalse/uploadtrue与bothfalse：前者无批准拒发布，后者无互动/审批记录/发布。两审批各用独立精确ID/revision，旧第一请求不能批准第二；20次approve/reject/cancel/claim竞争不复活cancelled。实际approved未Claim取消直接terminal且no post；已Claim取消必须回收真实当前进程再证stop，旧pause proof不能代确认新运行。

```bash
mybuilds build cancel "$BUILD_ID" --json
mybuilds reject "$BUILD_ID" --approval-id "$APPROVAL_ID" --revision "$REVISION" --checkpoint-digest "$CHECKPOINT_DIGEST" --note '证据不符合要求' --json
```

waiting/approved时delete原node拒，rotate/currenttoken查询按sameNode规则；别node不能读取proof。未知publisher锁仍在，不因取消、物理stop或approve解除。020保留/012自定义发布联合验证在各功能真实交付后补，计整MVP，不反向制造014前置循环。

## G5 真实本地交互

```bash
mybuilds run --file "$LOCAL_APPROVAL_YAML" --build local-debug
mybuilds run --file "$LOCAL_APPROVAL_YAML" --build local-debug --dry-run
mybuilds run --file "$LOCAL_APPROVAL_YAML" --build local-debug < /dev/null
```

真实TTY输入yes继续仅下一步，no/空/非法/EOF/无TTY/SIGINT均不默许、后续与post计数0，系统Close仍执行；输入等待大于原NS后yes仍按剩余预算，ctx取消有限响应无reader泄漏。dry-run无stdin/secret/network/material/approval写。坏client配置不能影响本地run/preview/help/doctor；生效upload或notify在任何用户动作前拒，不把本地yes当远端授权。

## 最终完整门

G1–G5对应plan覆盖表30FR/8SC/20AC；双库同suite、macOS/Linux/两节点/真实TTY/真实签名/发布、日志SSE及中央原file可核对。必要go test ./...、go test -race ./...、go vet ./...、三入口平台编译/help/version，行为门不能被编译代替；再speckit-converge按每稳定FR/SC/AC与源码/真实证据闭环。未知材料如实保留人工待验；完整代码与必要自动门通过后本地提交，不将代码交付称为全部人工验收通过。


## 当前实施修订（2026-10-05）

本功能已获准在fresh2602094+Root实际共享baseline完整实施，不再停留007规划基线。签名资源消费005冻结真实组件，发布消费Root/B当前实现；合法Apple/商店实际上传与最终Flutter案例人工待验，不能用自产签名或脚本成功冒充。必要自动门按安全/事务/恢复/权限/容量/预算/原文件/TTY实际路径验证，不因缺材料留下空实现。当前A独占本WT，Root原event/reports和最后发布接线独占。
