# 实施步骤、验收与范围

本文件保留原 [PLAN.md](PLAN.md) 的完整专题章节；业务功能按 [实施路线](SPECKIT_ROADMAP.md) 推进，当前状态见 [实施历史](../IMPLEMENTATION_HISTORY.md)。

## 当前交付状态

本文后续 P0–P6 保留产品分阶段验收范围，未勾选项不能直接作为当前实现状态；逐功能状态以 [README](../../README.md)、[实施路线](SPECKIT_ROADMAP.md) 和 [实施历史](../IMPLEMENTATION_HISTORY.md) 为准。

000–004、006 已验收并按整功能集成。007 已实现并验收，提供三个独立 CLI、节点身份/标签/容量/drain、固定 SHA 执行、租约及预算/步骤/post 证据、UTC 脱敏日志/SSE、中央制品与取消/停止保护；两个 macOS 节点与一个真实 Linux 节点闭环通过。SQLite 与 PostgreSQL 各 51 项相同真实三二进制应用检查、Android 签名构建号 101 的 APK/AAB/mapping 中央下载及实际 Gradle 取消通过；全量 test/race/vet 与 12 次三入口跨平台纯 Go 构建通过。Linux 普通/always 真实取消强化与 Spec Kit 收敛通过；整功能本地提交见[实施历史](../IMPLEMENTATION_HISTORY.md)。

008 原快照重试、启动核对与终态只读核对已完成并验收，最终双库应用各36正例、此前负例及Linux普通/always中断检查通过；共享停止确认修复、真实ARM签名/中央下载/取消、全量test/race/vet及Spec Kit收敛通过。详情见[008验证](../../specs/008-build-recovery/validation.md)。

整个 MVP 未完成。005 真实 Apple profile 与签名 archive/export 尚未验收，当前集成代码不提供 iOS 签名执行；Flutter、审批、发布、Webhook和保留策略等后续门不因 007 通过而放宽。当前生效 approval/upload/notifications 在执行前明确拒绝。019已接通本地与Agent报告检查、中央确认/封存/下载；双库各92应用检查点、20故障192断言、macOS/Linux与全量test/race/vet/12编译通过，Spec Kit收敛无缺口，已完成并验收，见[019验证](../../specs/019-test-reports/validation.md)。

### 当前可运行入口

部署需要一个 Linux/macOS 控制端与各自独立的 Agent。跨主机使用 HTTPS 反向代理和实际证书/CA；客户端/Agent 验证主机名，不修改系统信任。控制端 HTTP 监听位于受控后端，节点不共享数据库或 data_dir。节点凭据通过 0600 配置或完整环境引用，用户与节点 token 分开，不通过 argv 传入。

完整的私有配置、首次管理员初始化、节点注册、固定提交执行与清理命令见 [007 快速指南](../../specs/007-node-agents/quickstart.md)；下列命令使用该指南已设置的二进制、配置和构建变量：

```bash
"$client_bin" --config "$FIXTURE/client.yml" node ls --json
"$client_bin" --config "$FIXTURE/client.yml" doctor --node demo-generic --json
"$client_bin" --config "$FIXTURE/client.yml" doctor --server --json
"$client_bin" --config "$FIXTURE/client.yml" build show "$BUILD_ID" --json
"$client_bin" --config "$FIXTURE/client.yml" logs "$BUILD_ID" --follow --stream-timeout 15m
"$client_bin" --config "$FIXTURE/client.yml" artifact ls "$BUILD_ID" --json
"$client_bin" --config "$FIXTURE/client.yml" artifact download "$ARTIFACT_ID" --output "$FIXTURE/downloads/output.bin"
```

中央仅公开已确认日志和校验后的制品；下载检查大小/SHA-256 后排他发布，不覆盖既有文件。Agent 离线后仍可下载中央已确认内容。日志跟随和下载具有独立预算，普通 API timeout 不截断流。serve 在线时同库本机管理拒绝，使用远程管理；第二控制端拒绝。

同项目同名 build 串行，无 runner 仅使用 default_node，未匹配到授权平台/标签/容量时保持排队。drain 允许原任务续租但不领取新任务；disable/revoke/轮换撤销原执行权，不能再执行 always。取消与失联通过实际进程组回收/证据处理，不靠等待推断；停止未确认的 interrupted 保持 guard 和隔离，管理员完整 fence 的实际停止确认只解除保护，不改变原终态。显式retry创建新构建；审批或发布恢复仍属后续功能，不能用停止确认代替结果确认。

## 实施步骤

### P0 骨架与流水线预览

- [x] `000-project-bootstrap`：Go 单模块、Cobra、双 CLI 帮助与共享 version；README 与 AI 阅读入口
- [ ] `001-pipeline-preview`：复用已有入口，引入 YAML，增加 `init` 和 `run --dry-run`；其他依赖随对应阶段加入并锁定版本
- [ ] `internal/config/pipeline.go`：单流水线 default 与多 build 格式、选择规则、参数默认值/覆盖与按字段插值；保留 run 正文中的 shell 变量
- [ ] when 三类条件、字符串/对象参数约束、build timeout、post 与 reports 的严格结构校验；预览条件、跳过原因或待确定状态
- [ ] `internal/pipeline/mask.go` + 单测
- [ ] `mybuilds run --dry-run` 打印脱敏计划，不触发命令或外部动作

### P1 本地执行与原生 / Flutter 构建

- [ ] `internal/scm/git.go`：按 SHA 准备节点工作区；本地 run 使用当前工作树，不改写用户仓库
- [ ] `run` / `artifact`：顺序执行、失败即停、统一收尾，产物大小与 SHA-256 记录
- [ ] run 的 sh/bash、working_dir、step.env、timeout 与脚本参数；独立 shell、不继承 cd/export、受限环境与上下文变量
- [ ] 参数/分支 when 与步骤 skipped、累计 build 超时、独立 post 预算和系统收尾、默认时间戳日志
- [ ] 节点工作区 `<agent.data_dir>/builds/<project>/<number>/{src,logs,artifacts}`；本地结果写临时目录，日志按步骤分文件
- [ ] ctx 取消、进程组 TERM/KILL 与回收；按平台隔离实现，避免影响远程客户端编译
- [ ] 本地预检查拒绝本次选中且 when 生效的 upload；模板默认只构建/收集产物；运行中成功、失败、取消验证收尾，日志与 dry-run 脱敏
- [ ] Android 模板：Gradle 版本参数、keystore 注入、apk / aab / mapping 收集，复用依赖缓存
- [ ] iOS 模板：archive/export、按当前 Xcode 支持值生成 ExportOptions、独立 DerivedData 和临时 keychain、ipa / dSYM 收集与清理
- [ ] 基础 `doctor`：git 凭据、JDK / Android SDK、Xcode、签名证书及描述文件检查
- [ ] Flutter Android / iOS 模板与 doctor：复用底层签名/收尾，flutter build appbundle / ipa，支持版本、flavor 和用户参数
- [ ] 内置模板可编辑，自定义本地模板和仓库脚本构建；不新增执行器或新步骤 DSL
- [ ] 原生 Android / iOS 与 Flutter 双平台真实构建成功，核对版本、产物、签名和取消后的残留进程

### P2 控制端 + 多节点调度

- [ ] 加入 Viper、GORM 与双驱动；验证纯 Go SQLite 驱动兼容性，设置 WAL / 外键 / 锁等待；启动取得唯一控制端锁
- [ ] 模型保存 build_name、batch_id、构建配置快照、参数、SHA、当前步骤、工具版本与产物；项目构建号事务分配
- [ ] 项目组模型、default 初始化、客户端 project init 选组、组改名/空组删除与项目事务迁移；构建历史保留，列表按当前归属过滤
- [ ] 项目命名 build 设置、--file 与 --settings 路径区分、branch/ref 可达校验、批量固定 SHA/参数与权限校验、请求幂等/原子入队、批次关联查询
- [ ] build 级 when 在分配节点前判断，skipped 不占节点/构建号；记录条件事实、剩余预算和收尾进度，恢复不重置
- [ ] 控制端具名构建方案、项目 auto/repo/profile 来源选择；仅缺文件回退，错误配置拒绝，执行快照与重试不受方案编辑影响
- [ ] SQLite 与 PostgreSQL 跑相同的 CRUD / 事务 / 条件状态更新 / 唯一约束 / 分页用例
- [ ] token 摘要存储、一次性管理员初始化、创建 / 撤销 / 身份审计，统一读写权限检查
- [ ] 数据库持久化队列、默认并发 1、同项目同名 build 串行，状态变更使用条件更新；单控制端分配给多个 Agent 执行
- [ ] `net/http` JSON API、鉴权、日志 SSE tail、受鉴权的产物下载与控制端/节点 doctor
- [ ] 节点注册/独立 token/标签/容量/drain，Agent 跨主机 HTTPS 主动领取与续租
- [ ] 原子节点分配、attempt/租约归属、过期回报拒绝、断网停止，无匹配节点时保持排队
- [ ] 节点脱敏日志与校验产物回传；临时文件校验/原子改名/可见记录与重启恢复，不共享数据库或工作区
- [ ] `mybuilds-server serve` + 多个 `mybuilds-agent serve` + `trigger` + `build ls/show` + `logs -f` 闭环
- [ ] 列表 / 详情支持 `--json`；记录排队、步骤耗时、峰值内存与磁盘占用

### P3 重启恢复与审批

- [ ] approval 确认进程停止与资源清理、产物/报告封存后保存检查点并释放槽；已批准任务固定原节点，复核授权/容量/证据并保留自身 build 互斥
- [ ] `approve` / `reject` API + CLI：角色、身份、时间、意见、重复决定与取消竞态检查
- [ ] 启动时恢复排队与待审批任务并核对租约；过期记 interrupted，停止未确认则保留互斥/隔离原节点/禁止清理；停止证据与上传结果分别确认
- [ ] 重试固定原 SHA / 配置 / 参数并分配新构建号，不受分支后续提交影响
- [ ] 飞书使用 `oapi-sdk-go/v3`，先验证自定义机器人 Webhook 的消息、签名与错误处理；企微 / 钉钉 / generic 用 stdlib HTTP
- [ ] 最终通知覆盖成功 / 失败 / 取消；审批挂起不执行终态通知或删除恢复所需文件
- [ ] 项目直接配置 Webhook 与 defaults.notifications；按字段继承、列表替换、显式关闭；敏感值受限保存/脱敏、URL 校验、策略快照和失败不改投全局
- [ ] 重启租约核对、停止未确认保护、审批释放槽、原节点恢复、重复批准、挂起取消/拒绝、工作区缺失、本地交互审批验证

### P4 自动触发：Webhook + 轮询

- [ ] 加入 webhooks/v6，接 GitHub / GitLab；Gitee 与自建 Git 用轻量解析器
- [ ] 按已注册 provider 校验原始 body、签名 / token、事件与分支，限制请求大小
- [ ] 安全用例：HMAC 篡改 body、错误 token、provider 伪装均拒绝
- [ ] delivery ID 去重，事件与待触发请求事务入库后响应；窗口关闭与任务创建原子提交，不同渠道与手动重试允许同 SHA
- [ ] 后续 016 的每项目 ticker + `git ls-remote`、游标与触发请求事务提交；cron 复用入口
- [ ] 分支 glob 过滤，忽略 tag / PR / 分支删除事件
- [ ] MVP Webhook 的 changes 路径筛选与 quiet_period，固定窗口、不同参数/分支隔离、持久化恢复与重复关闭拒绝

### P5 商店分发（MVP）与后续产物运维

- [ ] 019 报告检查先完成再接入发布；核对应用唯一项目绑定、版本、AAB/IPA、封存产物/报告、节点/attempt/租约，事务取得应用锁并保存授权意图
- [ ] Google Play 使用 fastlane supply，service account 认证，支持 internal 与显式 production
- [ ] App Store 使用 fastlane deliver 与 API key，支持上传 IPA、显式提交审核及选择审核后自动发布；不把上传成功等同公开上架
- [ ] custom 上传接用户命令/Fastfile，共用输入、结果文件、租约、取消、脱敏与未知结果确认
- [ ] 授权后无可信回执默认 unknown 并保持应用锁；查询远端或由 admin 用 `resolve-upload` 确认依据，不自动重发，不能用停止确认代替
- [ ] 配置快照与上传产物绑定，审批后发布原产物，不重新构建
- [ ] retention 清理终态构建（保留 N 个 / N 天），保护活动、待审批、停止未确认与未知上传任务，保留项目计数器/必要审计
- [ ] MVP 项目 retention 按字段继承全局；中央删除保护已有下载，Agent 独立管理指令校验工作区/进程并幂等确认，离线保留待清理
- [ ] MVP JUnit 报告收集与 CLI/JSON 摘要、原始 XML 下载；本地旧报告不放行，发布前封存证据，post 不改证据，缺失/非法/越界报告可诊断
- [ ] 两大商店真实上传与可查询回执，自定义命令结果校验，验证凭据错误、失败与回报丢失的 unknown 状态
- [ ] 原型验证工具内置重试、非交互认证及版本兼容；Ruby/Bundler/fastlane 按节点锁定
- [ ] fir.im / generic 内置分发不属于 MVP，后续有需求再接入

### P6 部署与打磨

- [ ] macOS `launchd` 与 Linux `systemd` 模板；控制端/Agent 分别自动启动，运行目录独立
- [ ] 优雅关闭：控制端停止分配，Agent 停止领取并取消自身构建进程，按租约保存状态；专用用户和可信分支部署说明
- [ ] `build ls` 过滤与分页、日志流重连、doctor 输出打磨
- [ ] 基于真实构建数据调整并发；按实际瓶颈优化依赖缓存，不新增通用缓存系统

MVP 包含手动/Webhook 触发、同仓库多命名 build、原生与 Flutter 双平台、多节点执行、日志/产物下载及两大商店分发与 custom 扩展。
新增 when、参数约束、build 总超时、post、日志时间戳、changes 筛选、quiet_period、项目 retention 与 JUnit 报告，发布审批同步进入 MVP。
机器人通知、轮询/cron、其他内置渠道和部署打磨可后置；发布授权、租约、测试失败阻止发布与上传 unknown 处理不能后置。
Google Play/App Store 接入不自写完整市场协议；具体认证、默认发布模式、前置条件及验收见 [BUILD_DISTRIBUTION.md](BUILD_DISTRIBUTION.md)。

<a id="verification"></a>

## 验证

**单元测试（只测值得测的）**

- `internal/config`：单/多 build 混写拒绝、名称选择与参数校验、未知字段 / 模板变量报错、env 字段插值、run 正文保留 `${VAR}`、引用的密钥缺失时报错、dry-run 无副作用
- `internal/store`：**双驱动**跑同一套事务 / 构建号分配 / 去重 / 条件更新 / 分页用例；token 撤销后不得再次初始化
- `internal/pipeline`：脚本参数与环境隔离、sh/bash、超时、工作目录越界拒绝、独立 shell；成功 / 失败 / 取消均收尾，分段日志中的密钥被脱敏，产物路径及符号链接不能越界
- `internal/scm/hook.go`：**四种来源的签名校验**，含 GitHub HMAC 篡改 body 必须拒绝、错误 token 必须拒绝
- `internal/server`：批量同 SHA/原子入队、统一计数不重复、同项目同名 build 串行及不同 build 并行、审批释放全局槽、重复事件只入队一次、同 SHA 不同参数可触发、轮询游标与入队同时提交
- `internal/agent` / 节点协议：独立身份、双节点竞争、错误平台、租约过期、断网取消、幂等回报、日志与产物回传
- 审批与发布：重复批准 / 取消不能覆盖终态；重启后原节点正确继续；未知上传结果不能自动重发
- MVP 执行控制：when AND/OR、手动路径豁免、删除/重命名/公共目录、首构建与缺失基线、冻结事实重试；审批跳过不放行上传
- MVP 超时/收尾/日志：累计预算与重启恢复、独立 post 预算、失败不覆盖原证据、节点失联不迁移收尾、时间戳与流式脱敏
- MVP 触发/保留/报告：窗口持久化与原子关闭、项目继承与保护竞态、JUnit 汇总不重复、非法 XML/越界/缺失拒绝、测试失败不上传

**当前端到端验收**

使用 [007 快速指南](../../specs/007-node-agents/quickstart.md) 创建自己的普通文件、0700 数据目录、数据库、端口、CA、token 与受信 Git 仓库，实际启动 Server/Client/Agent 三二进制。SQLite 与独立 PostgreSQL 数据库执行相同管理、固定 SHA、ordinary/post、20 次幂等触发、取消/guard、续租、日志/SSE、二进制制品下载、角色与第二控制端检查；记录 UTC、退出码、固定 SHA、实际 PID/PGID 和摘要。旧 journal 不重放，停止确认要求完整执行归属和实际停止依据；不得删除记录或改状态绕过保护。

007 已有同套双库各 51 项实际应用结果及真实两 macOS + Linux 节点证据。以下真实构建与完整 MVP 项目仍是后续验收清单，不表示命令或能力已经交付。本地 approval 交互、审批 API/CLI、通知、Webhook、上传、retention 和 JUnit 尚未交付，不把模拟成功当成验收。

**真实构建验证**

1. 一个真 Android 工程：出 apk + aab + mapping.txt，versionCode 等于构建号，产物可下载并校验摘要
2. 一个真 iOS 工程：archive → export → ipa + dSYM，签名有效（`codesign -dv` 校验）
3. GitLab / GitHub / Gitee / 一个自建 Git 各连一次，push 触发成功；把某个 hook 的 secret 改错，确认被拒
4. 轮询模式：手动在仓库推一次 commit，确认在间隔内被探测并触发
5. 飞书官方 SDK：真实 Webhook 发消息成功，错误凭据 / 签名能报告失败，日志不泄露密钥
6. Android / iOS 取消与失败后无本次构建残留进程、临时 keychain；节点独立缓存且不共享构建工作区
7. 两个真实节点执行不同项目，同项目同名 build 跨节点串行；iOS 不分配给 Linux，无合格节点明确排队
8. 暂停原 Agent 后模拟过期回报，确认拒绝；审批后原节点离线不迁移，发布授权后失联不自动重发
9. Flutter Android/iOS 真实构建，核对版本、flavor、签名与产物；自定义仓库脚本同路径执行
10. Google Play internal 轨道可见真实 AAB；App Store Connect 可见真实 IPA，显式提交审核路径可验证
11. custom 上传按结果文件确认；授权后无可信回执保持 unknown，不重发；上传/审核/公开状态分别记录
12. 同一触发 key 网络重试只产生一批结果与构建号；不同内容复用 key 被拒；误启动第二控制端被拒
13. 失联后同名 build 与原节点保持保护，停止确认和上传确认分别解除；审批挂起取消不启动用户脚本
14. 旧本地 JUnit 文件不算本次通过；报告失败/缺失阻止审批与上传，审批后产物被改写不能发布

## 明确不做（一期）

- 不做 Web UI（纯 CLI）
- 不做插件系统 / 自定义步骤 DSL
- 不做 Docker 隔离；支持多构建节点，不做多控制端高可用或单条流水线跨节点迁移
- 不做 RBAC / 多租户（token 角色只三档）
- 不自动注册商店账号或代填合规元数据；不在 MVP 内置其他分发渠道
- 不做并行步骤块、矩阵构建

## 已确定的部署与范围

1. 一个控制端（Linux/macOS）管理多个 Agent。iOS 仅在 macOS 节点构建，Android 可在 Linux/macOS；客户端远程命令跨平台。
2. 数据库默认 SQLite，支持 PostgreSQL；初期用 GORM `AutoMigrate` 建表，需要改名或回填时再增加显式版本迁移。
3. 审批通知渠道按 飞书（oapi-sdk-go）/ 企业微信 / 钉钉 / 通用 webhook 实现，无内置邮件与短信；企微与钉钉机器人走 stdlib HTTP POST。
4. 构建号用「每项目自增整数」，首次注册允许指定兼容商店既有版本的起始值；应用版本由 YAML 参数或触发参数提供，一期忽略 tag 事件。
5. 原生与 Flutter 模板及用户脚本进入 MVP；Google Play/App Store 与 custom upload 进入 MVP，平台与商店身份分别校验。
