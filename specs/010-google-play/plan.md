# 实施计划：Google Play 发布与未知结果核对

**Branch**：`010-google-play-planning` | **Date**：2026-10-05 | **Spec**：[spec.md](spec.md)

## Summary

在同一 `pipeline.Run` 的 upload 步骤接入受控 fastlane publisher。普通构建、中央原产物与019声明测试封存通过后，节点先核验固定AAB，控制端以真实短事务写原意图并取得应用保护，再允许一次具体命令。失联后保护不随执行租约、停止确认或构建槽释放消失；显式查询和管理员有依据确认只更新原动作，不重传。

本轮只完成设计。HEAD为已验收007 `85b46bfaccb45c4626fcffbbcb526b2f7f8c9301`；008、009、019未作为已验收基线，原前置不变。实施必须先FF到三项真实交付基线并核对共享签名；005真实Apple签名、011/014/020和完整MVP不因本计划通过而通过。

## Technical Context

**Language/Version**：现有Go1.25单模块；发布节点Ruby至少3.1与兼容Bundler，现有系统Ruby2.6.10/Bundler1.17.2不满足，未安装升级。

**Primary Dependencies**：现有Cobra/Viper、GORM双驱动、SQLite3.53.3补丁及标准库HTTP/files；不新增Go商店SDK。fastlane2.240.1、google-apis-core1.2.5、bundletool1.18.3是官方已存在的接入候选，不是已验证项目锁。接入原型生成真实Gemfile.lock并记录全部网络传递依赖、Ruby/Java/Bundler和JAR摘要；失败不允许真实发布。

**Storage**：现有SQLite/PostgreSQL控制端锁与同连接事务；新增具体应用绑定、应用保护、发布意图/回执/核对记录；节点现有自有0700data/spool和0600材料副本。

**Testing**：同套双库行为、真实process/PID/PGID、无凭据本机故障HTTP端点计数、真实native/Flutter internal材料；Go test/race/vet、三入口跨平台构建、实际CLI/HTTPS及机密扫描。

**Target Platform**：Linux/macOS控制端与Google Play节点；Windows客户端可编译，本地upload仍拒绝。

**Project Type**：现有三入口CLI/控制端/Agent；一个Run、一个process执行路径。

**Performance Goals**：20个同应用不同申请至多一条授权；查询整体30s、不占构建槽；不等待商店审核/公开处理。

**Constraints**：完整LeaseRef、Authority每一新动作前核对，原普通ns预算且0拒启动；断网与清理沿007/008。publisher结构回执≤64KiB，原始工具输出各流≤64KiB且不公开；AAB沿原中央1GiB单文件及attempt128文件/4GiB总额，不另获发布配额。节点读密钥JSON≤1MiB，bounded/no-follow普通文件；工具包只读、所有权明确。

**Scale/Scope**：一个控制端、多个节点，两种Android构建框架、一应用一项目；仅AAB和明确轨道。未实现approval仍在整批前拒绝；不建registry、泛型FastlaneClient、另一Run、商店账号管理或自动素材/协议操作。

## Constitution Check

| 原则2.1.0 | 研究前 | 设计后 | 依据 |
|---|---|---|---|
| I 规范驱动 | PASS | PASS | 冻结spec/checklist，仅plan；原依赖交付后再tasks/analyze/implement/converge |
| II 单模块多节点 | PASS | PASS | 真实Agent→Run→publisher→process；无控制端发布子进程 |
| III 最少依赖 | PASS | PASS | 第三方fastlane具体消费者；仅实际网络副作用路径，候选锁须原型证明 |
| IV 安全边界 | PASS | PASS | 一次授权、持续应用guard、完整fence、unknown不重传、有限材料与输出 |
| V 实际中文验收 | PASS | PASS | 双库/原生/Flutter真实internal门与缺材料待验证；不把源码研究当商店通过 |

无原则例外。技术决策已定；真实工具兼容和商店素材是实施验证门，不是由占位实现补齐的澄清项。

## Project Structure

```text
specs/010-google-play/
  plan.md research.md data-model.md quickstart.md
  contracts/google-play.md contracts/go-api.md contracts/node-http.md
internal/distribute/
  fastlane.go google_play.go aab.go fastlane/Fastfile
  fastlane/Gemfile fastlane/Gemfile.lock
internal/store/publish.go publish_query.go publish_models.go
internal/pipeline/publish.go
internal/agent/publish.go publish_query.go
internal/server/publish.go
internal/cli/client/publish.go
```

这是未来实际文件归属，不创建源码包或锁文件。现有路径接入点：config/types.go/validate.go/parse.go（既有upload严格顺序及字段）、pipeline/run.go/run_types.go/preview.go（当前明确拒upload）、server/trigger.go与store/enqueue.go（目前拒upload/reports）、store/event.go/node_models.go/query.go/recovery.go/retry.go、agent/execute.go/journal.go/serve.go、protocol/node.go、server/http.go/json.go/files.go、client/root.go。

## 最小流程与依赖

1. 实施前对齐008Recover/Retry/TerminalReceipt、009模板版本、019ReportSeal/ArtifactCommit和中央下载。019在首approval/upload前封存普通构建/测试段；010只核验该seal及原文件，不post重封、不产生假artifact步骤。
2. 先真实红测权限、配置、版本/签名/产物/report保护、旧本地行为，再无凭据原型验证**每个不可逆endpoint至多一次请求**。完整锁未验证或身份刷新/隐式重发未关闭，doctor失败，不能授权发布。
3. 应用预登记并由指定节点真实诊断核验；全批选中build有upload即要求admin+allow_upload，先于when。普通run/artifact之后，匹配中央同attempt唯一原AAB；metadata/签名/摘要与当前seal核验成功，短事务授权。
4. Store授权在返回前即持guard并默认unknown，完整一次动作ID。节点journal先fsync授权/once标记再启动固定publisher。Node先生成原IntentID并fsync；授权返回丢失不再申请/运行命令；重启不重放。网络事件可重发精确原回执，不能重跑商店动作。
5. tool完成先确认本次进程停止再回执，构建继续原post与terminal；原fail/cancel Reason优先。unknown可以释放已停止的构建槽，却持续锁应用。upload step_finished先关闭发布slot拒后来授权，Agent随后只读核对已提交原intent；终态manifest包含全部原IntentID/ReceiptDigest或明确unknown，Store核对完整关系，008只读停止回执不解除应用锁。
6. 管理员显式query派给原节点的独立有限管理请求，不Claim/Run或占build槽；只读轨道GET，不创建edit；该GET缺原AAB摘要，空/歧义/不足保持unknown，给管理员安全远端证据并沿精确人工确认解决。
7. 人工confirm绑定原意图与外部证据，stop/exit不是证据。020在实际集成后以真实guard/intent关联保护RetryOf依赖闭包、原XML/AAB，不加虚构未来字段。

## 唯一writer及011共享

| writer | 独占业务文件 | 主代理串行共享文件 |
|---|---|---|
| A Store | store/publish_models.go、publish.go、publish_query.go及同名tests | store/models.go、node_models.go、store.go迁移、event/query/recovery/retry/enqueue/artifact |
| B Run/Agent/publisher | distribute上述具体文件/tests、pipeline/publish.go、agent/publish*.go/tests | pipeline/run.go/run_types.go、agent/execute/journal/serve/http、process必要最小消费者补丁 |
| C 配置/API/CLI | config/publish.go、server/publish.go、client/publish.go及tests | config/types/parse/validate/agent、preview、server/trigger/http/json/files、client/root/node doctor |
| root | protocol唯一类型、集成与文档/验证 | protocol/node.go、全部上表共享、README/deps/模板和两个功能合并 |

011仅对齐具体工具目录/锁与应用guard键 `(store,app_identifier)`、动作级Intent/Receipt/Query。Google一个上传流程自身有edit→AAB→track→commit阶段；Apple后续upload/submit必须各自授权，前动作成功不确认后动作。root独占fastlane.go/Fastfile/Gemfile.lock及公共模型实际集成，同文件不并写；010只实现google_play分支，不预建Apple空消费者、通用SDK或渠道registry。未来012 custom另验，不消费用户Fastfile/lane。

## 覆盖与验收门

| 设计门 | FR | SC | AC |
|---|---|---|---|
| G1 工具/材料、固定AAB与JUnitSeal | 001–007、012 | 001–002 | US1.1–4、US2.5 |
| G2 管理员/显式轨道/绑定/一次授权 | 008–011、013–014 | 002–003 | US2.1–4 |
| G3 unknown、查询与精确决定 | 015–019、023 | 004 | US3.1–5 |
| G4 状态/槽/角色/安全输出 | 004、020–022 | 005–006 | US4.1–3 |
| G5 兼容/真实联合验收 | 024–026 | 007 | 全部17AC复验 |

未来tasks先红再实现并按G1→G2→G3→G4→G5接真实消费者；不可用fake商店证明SC001/004。应用/授权/首上传/合法签名、native/Flutter及本功能全部真实门是010提交前提，不因单元绿整项提交。011共同发布兼容在其接入时复验；014/020联合门在各自真实接入后完成并计入整MVP验收，不反向作为010提交前置。014依赖010/011，此顺序避免循环；当前生效且未实现审批仍拒绝，未知发布保护不能提前清理。

## 工作流实测记录

本WT实际setup-plan --json和resolve-template plan-template --json完成，项目preset模板复制/填充；selector只010，脚本返回FEATURE_DIR=010，实际Git分支010-google-play-planning。before_plan/after_plan的extensions hooks={}。spec/checklist原SHA冻结，未tasks、源码、commit。旧007API与008/019冻结设计仅只读，未复制未验收源码。

## Complexity Tracking

无原则违反、无新增框架。

## 2026-10-05 用户验收安排与实施归属（覆盖原阶段门措辞）

用户明确要求先完成全部模块代码及必要自动验证，真实 Apple/Play 上传在最后统一案例由用户人工验收；没有凭据不阻塞代码交付/本地提交。原真实远端需求与 SC/AC 不删除、不宣 PASS，验证记录标人工待验；无凭据自动门仍须实际锁版本、真实本机故障端点单发、未知保护与安全失败。008/019/020已验收基线2602094；005/009按其实际模块接口接入而不虚构真实素材通过。

唯一 writer：publish-channels 分区独占 internal/distribute/**（两店具体 Go/Ruby、Fastfile、Gemfile/lock、测试）及新 internal/protocol/publish.go；root独占既有protocol/node.go、Store、Agent、Pipeline、config、server、CLI共享消费者与全局文档。原表/任务中分散在B/root的 distribute 与公共发布新类型任务统一归publish-channels；其他任务仍root协调唯一writer。一个Run/原process.Run、完整Ref与guard/unknown不变，无registry/第二执行器。
