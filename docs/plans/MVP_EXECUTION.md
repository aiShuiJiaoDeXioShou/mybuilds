# MVP 执行与集成计划

本轮目标为 001–012、014–015、019–020 共 16 个待实现功能；000 已完成。范围与设计以 [PLAN](PLAN.md) 及其专题为准，功能要求和逐功能流程见 [SPECKIT_ROADMAP](SPECKIT_ROADMAP.md)。本文只规定依赖、验收和协作，不另建任务或状态系统；实际任务保存在各功能的 specs 中，整体进度只记 [实施历史](../IMPLEMENTATION_HISTORY.md)。

## 依赖与可并行批次

前置指功能已验收并由主代理集成后的提交，不能只凭完成任务勾选或另一 worktree 中存在代码就开始依赖功能。批次表示满足依赖后的最早启动位置，前一批无依赖关系的功能无需等其他功能结束；编号不代表顺序。

| 批次 | 功能 | 必须前置 | 该功能必须验收的行为 |
|---|---|---|---|
| 1 | 001-pipeline-preview | 000 | init 不覆盖；单/多 build 选择与参数覆盖；严格拒绝未知/非法结构、参数/when/timeout/post/reports/发布顺序错误；变量与错误脱敏，dry-run 不执行外部动作。 |
| 2 | 002-local-run | 001 | 顺序 shell、参数/分支条件和 skipped；独立 shell/受限 env/工作目录；累计超时、独立 post 和 UTC 日志；失败即停，取消无本次进程残留，不重置工作树；生效 upload 在任何命令前拒绝。 |
| 3 | 003-build-artifacts | 002 | 递归 glob、路径及符号链接边界；匹配错误拒绝；产物名称、大小和 SHA-256 可核对。 |
| 4 | 004-android-build | 003 | Android doctor、可编辑的默认 run/artifact 模板；真实工程生成 APK/AAB/mapping，版本正确；JDK/SDK/签名错误可诊断，失败/取消不留本次进程。 |
| 4 | 005-ios-build | 003 | iOS doctor、archive/export 模板；真实签名有效的 IPA/dSYM，版本正确；独立 DerivedData/临时 keychain，失败/取消清理签名资源与本次进程。 |
| 5 | 006-control-plane | 004、005 | SQLite/PostgreSQL 同行为；唯一控制端锁；项目组/default/改名迁移/历史保留、token 与鉴权；固定 SHA 与快照、批量校验/原子入队、幂等构建号；skipped 不占号/节点；API/CLI、条件事实/预算/收尾进度持久化。 |
| 5 | 009-flutter-builds | 004、005 | Flutter doctor、Android/iOS 模板；真工程双平台版本/flavor/签名/产物正确；模板可编辑，复用已有取消、签名和产物契约。 |
| 6 | 007-node-agents | 006 | 独立节点身份、HTTPS、能力/授权/容量匹配；双节点竞争只领取一次，同名串行、不同 build 并行；续租/过期回报拒绝/断网停止；UTC 脱敏日志、SSE、校验产物回传下载；管理停止确认。 |
| 7 | 008-build-recovery | 007 | 控制端重启核对仍有效租约；排队恢复，预算不重置、已开始未知 post 不重跑；失联不迁移，stop_unconfirmed 保互斥/隔离/禁止清理；停止确认；retry 固定原 SHA/参数/条件/配置且产生新号。 |
| 7 | 019-test-reports | 003、007 | JUnit 大小/深度/路径边界与非法 XML；本次报告新鲜性、按路径替换汇总、原 XML 下载；准备步骤不误判缺失，最终缺失/测试失败阻止发布；审批/上传前封存原证据，post 不修改放行证据。 |
| 8 | 010-google-play | 008、009、019 | 锁定 fastlane 与非交互认证；应用唯一项目绑定/版本与唯一 AAB 校验；真实 internal 版本可见，production 显式授权；持久化发布意图、原产物/报告、回执和查询；unknown 持应用锁且不重发，错误应用/凭据拒绝。 |
| 8 | 011-app-store | 008、009、019 | 锁定 deliver/API key；应用绑定/版本/签名 IPA 校验；App Store Connect 真实构建可见，显式提交审核路径；上传/处理/审核/公开状态分开；缺失前提明确失败、原证据与 unknown 保护、查询/人工确认。 |
| 8 | 020-project-retention | 008、019 | 项目字段继承全局、跨 build 数量/天数筛选；活动/审批/停止未确认/unknown/下载中保护及删除前复核；中央删除记录；Agent 独立受限删除指令/幂等确认，离线保持待清理；计数器与必要审计保留。 |
| 9 | 012-custom-workflows | 003、007、010、011 | 仓库脚本/本地模板；单 build 可复用方案；无 YAML 双平台注册；auto 仅缺失回退、repo/profile 强制来源且集合不合并；命名参数/原快照重试；custom argv 与受限结构化结果，共用应用锁/发布授权/unknown。 |
| 10 | 014-release-approval | 008、010、011 | CLI 身份/权限/决定审计；报告/产物/进度封存和释放槽；保持同名互斥，批准在原节点复核授权/工作区/证据并续剩余预算；重复/冲突决定、挂起取消安全，跳过 approval 不放行 upload；不依赖通知。 |
| 11 | 015-webhook-trigger | 006、008、012、014 | 四来源原始 body/签名/provider/分支校验；显式 build 范围/发布授权；事件去重、固定 quiet_period 窗口与重启原子关闭；changes 新增/删除/重命名/公共目录、首次/缺基线全执行；手动豁免和 retry 条件冻结。 |

014 可在 010/011 集成后与 012 并行；015 须等两者。020 不必等待 012/014/015，但必须使用已经定义的审批/unknown/停止/下载保护状态，并在后续集成时复验这些功能实际产生的保护状态。009 的节点模板契约在 007 集成后补验，不能因此提前实现另一套节点协议。

## Worktree、文件归属与主代理集成

每个功能由主代理维护一个集成 worktree/feature 分支，以所需前置的最新集成提交建立。功能内的实现分区分别使用独立 worktree 和不同分支或 detached HEAD，主代理同步已冻结的规范、公共类型与依赖基线；未提交修改不会自动跨 worktree 传播。每个文件只归一个写入者，分区交付限定差异由主代理串行集成。不同功能不共用未提交工作树，依赖以已验收的集成提交交付。

各功能在 plan 中先列实际文件归属与共享接口。下面只是路径职责，不能据此预建空文件；已有 helper/类型/执行路径优先复用。

| 功能 | 主要写入职责 | 常见共享点 |
|---|---|---|
| 001 | internal/config 的流水线解析、internal/pipeline 的纯预览、客户端 init/dry-run | client/root.go、go.mod/go.sum |
| 002–003 | internal/pipeline 的执行、进程、日志/脱敏、产物 | pipeline 类型、客户端 run |
| 004 / 005 / 009 | internal/mobile 的对应平台/Flutter 文件、各自模板与示例 | doctor/init 命令、共用模板和签名/版本参数 |
| 006 | internal/store、server 控制面/HTTP、scm 只读 SHA、server/client 配置与命令 | 模型/迁移、CLI 入口、go.mod/go.sum |
| 007–008 | cmd/mybuilds-agent、internal/agent/protocol、调度/租约/恢复 | store 模型、pipeline 执行入口、HTTP 路由 |
| 019 | pipeline 报告收集/解析、详情摘要/下载证据 | pipeline 流程、产物、store/protocol |
| 010 / 011 | distribute 的 Google/Apple 各自适配、对应 fastlane lane | fastlane.go、Gemfile.lock、发布意图/应用锁/store/HTTP；先在两份 plan 对齐契约，主代理指定唯一共享文件写入者 |
| 012 | distribute/custom、方案展开/来源选择、项目绑定与用户模板 | config/mobile/CLI、发布记录与快照 |
| 014 / 015 / 020 | 各自审批、Webhook/窗口、retention 业务文件及必要接入 | HTTP/CLI、store、调度、报告/产物保护 |

每个功能独占 specs/<实际功能目录>/；全部代理只修改自身声明的文件。README、AGENTS、产品专题、实施历史以及多个功能同时需要改的入口/模型/依赖/锁文件，由主代理协调唯一写入者并串行集成。功能代理可提交明确补丁交付主代理；不要为了避开共享文件制造重复业务入口、模型或执行器。出现共享接口变更时先更新功能 plan/contracts 并通知使用方，再继续依赖实现。

主代理在集成前检查当前 worktree、暂存差异及各功能 diff，仅接收该功能相关修改；用户的其他修改保持独立。逐个集成，解决共享文件冲突后复验该功能及受影响的已有功能。各功能仍执行 specify → clarify（有实质歧义时）→ plan → tasks → analyze → implement → converge，先修阻塞分析，再实现和收敛；发现缺口继续 implement/converge。

功能验收通过后只做一次包含该功能 specs、实现与验证记录的本地提交，不按任务提交，不自动 push；主代理记录集成提交和最终验证结论。若功能交付已有完整验收提交，集成该提交并复验，不把同一功能拆成额外任务提交；因集成发现实质缺陷则按缺陷流程修复，不把未验收代码标为完成。其他 feature 在依赖功能集成并验收后才基于该提交继续。

## 平台真实验证与全局完成定义

每个功能运行其 quickstart/contracts 中的行为验收，安全/事务/恢复/进程取消须有可重复检查；集成后运行 go test ./... 与 go vet ./...，覆盖对应 CLI 帮助、退出码和版本构建。非平凡逻辑保留最小可运行检查；不以覆盖率数字、勾选任务、能编译或 mock 成功代替验收。完整验证要求保留在 [DELIVERY Verification](DELIVERY.md#verification)，后置功能的通知/轮询用例不计本轮 MVP。

真实验证须记录日期、提交、节点 OS、doctor 实际工具版本、命令/配置引用、脱敏结果与证据位置：

- 原生 Android 和 iOS，以及 Flutter Android 和 iOS 四套真实工程；核对应用版本/构建号、APK/AAB/mapping、签名 IPA/dSYM、flavor 和下载摘要。iOS 在 macOS；Android 在实际 Linux/macOS 节点验证，不把交叉编译当宿主构建验证。
- 至少两个真实节点竞争/并行，含 macOS 与 Linux 的平台限制、无合格节点排队、同项目同名跨节点串行；控制端/Agent 实际重启、断网、过期回报、审批释放槽和原节点恢复。真实失败/取消后查本次进程组与临时 keychain 无残留，不能杀同用户无关进程。
- SQLite 与独立 PostgreSQL 测试数据库跑相同事务/构建号/唯一约束/状态更新/分页/组迁移用例，并实际拒绝第二控制端。
- GitHub、GitLab、Gitee、自建 Git 各实际 push 一次，验证成功触发与错误 secret/签名拒绝；同时验证 changes、quiet_period、重投/重启和禁止未经授权的自动发布。
- 使用已准备应用与授权账号：Google Play internal 可见真实 AAB；App Store Connect 可见真实 IPA，并验证显式提交审核路径。记录远端标识和查询状态；审核通过或正式上架耗时由商店决定，不作为本项目等待测试条件。production/自动发布按显式授权验证，不默认扩大公开发布。
- custom 命令结构化结果、错误凭据/产物、报告失败/旧报告/审批后篡改；模拟授权后回执丢失并核对 unknown 持锁且没有第二次上传；停止确认不能解除应用锁。
- 保留策略实际保护活动/待审批/停止未确认/unknown/下载中数据；离线 Agent 保持待清理，重连删除幂等，计数器不重置。

工具链、真实工程、签名证书、测试节点、商店账号/应用/凭据缺失时，先完成仍可执行的实现和验证，在对应 validation.md 与实施历史列出缺失项、已通过项和待执行命令。依赖真实验证的功能保持“待真实验证”，不得虚构证据或宣称验收完成。提供条件后继续相同功能文档的验收，不能以“环境不支持”静默删掉要求。

全局完成同时满足：16 个功能各自验收和 converge 无阻塞缺口；上述真实验证有证据；全量检查通过；未实现的后置选项明确报未支持；安全/发布授权/租约/unknown/原证据/保留保护闭合；README 与专题反映实际行为；每功能本地提交、验证和集成哈希可追溯，没有夹带用户修改或自动 push。MVP 不以 012 完成或一次演示成功单独判定。

## 上下文交接最小记录

每次暂停、换代理或上下文交接，更新对应 specs 的 validation.md 和 [实施历史](../IMPLEMENTATION_HISTORY.md)，只记录：功能/分支/worktree 与依赖提交；当前 Spec Kit 阶段和未完任务 ID；本次文件归属/共享接口变化；已运行检查、结果与证据；未解决阻塞/真实环境缺口；下一条可执行任务及集成/最终提交哈希。配置和日志只给脱敏证据位置，密钥仅写引用；不要另建看板、数据库或复杂追踪工具。
