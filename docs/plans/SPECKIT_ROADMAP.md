# mybuilds 的 Spec Kit 实施路线

本文件是功能拆分与操作案例，业务案例不代表已生成正式 spec 或开始实现；项目初始化见 [specs/000-project-bootstrap](../../specs/000-project-bootstrap/spec.md)。产品与技术决策以 [PLAN.md](PLAN.md) 为准。
按可独立验收的用户能力拆分 feature，不按 config / store / API 等目录分别开发。

## 开始前

- 当前使用 Spec Kit 1.0.13，Codex 已设为默认集成，技能位于 `.agents/skills/speckit-*/SKILL.md`；下文使用 `$speckit-xxx` 写法，在 Codex 聊天中逐条调用，不是 shell 命令。
- Pi 集成保留在 `.pi/prompts/`，使用 `/speckit.xxx` 写法；本项目的 bug 扩展也已注册为 Codex 技能。技能列表未更新时重新打开 Codex 会话。
- 项目原则已在 `.specify/memory/constitution.md` 更新为 2.1.0（多节点、内置构建模板与两大商店分发）；Git 基线与 `000-project-bootstrap` 初始化已建立。后续沿用原则与已有入口。
- 不同 feature 在前置依赖验收并集成后可并行，各用独立 worktree，并在 plan 中声明文件归属；同功能仍完整执行 Spec Kit 流程。主代理串行集成共享文件并复验，每个功能验收后一次本地提交，不自动 push。编号不代表执行顺序，正式目录名以 Spec Kit 的实际生成结果为准，具体批次见 [MVP 执行计划](MVP_EXECUTION.md)。

项目原则的输入示例：

```text
$speckit-constitution
以 docs/plans/PLAN.md 为依据制定项目原则：注释与文档用中文；Go 单模块、客户端/控制端/Agent 三种 CLI，单控制端管理多个 macOS/Linux 构建节点；
标准库优先、按功能引入依赖；原生/Flutter 提供可编辑模板，Google Play/App Store 封装第三方 fastlane；不增加动态插件系统、Web UI 或多控制端高可用。
严格校验外部输入；配置和日志不泄露密钥；节点独立授权与租约持久化；审批固定原节点恢复；不可确认的上传不得自动重发。
非平凡逻辑必须有最小可运行验证，安全、事务、恢复和进程取消必须有相应测试；不设虚构的覆盖率指标。
完成标准是验收行为通过，而非仅完成任务勾选。
```

## 功能路线

每一行对应一个 feature 的 spec / plan / tasks。阶段对应 [实施专题](DELIVERY.md#实施步骤) 的 P0–P6；依赖表示需要复用哪些已完成能力。

| 建议编号与目录后缀 | 阶段 / 依赖 | 实施顺序 | 功能完成的验收点 |
|---|---|---|---|
| 000-project-bootstrap | P0；无 | Go 单模块 → 双 CLI 与共享版本 → README / AI 阅读入口 → 本地提交 | 帮助与版本可运行；规范与文档指引齐全（已完成） |
| 001-pipeline-preview | P0；000 | 复用 CLI → default/builds → 参数约束与选择 → when/timeout/post/reports 及发布段顺序校验 → 脱敏预览 | 字符串简写/对象参数有效；条件结果或待确定可见；非法结构拒绝；无外部动作 |
| 002-local-run | P1；001 | 顺序 shell → 分支/参数 when → 步骤 skipped → 累计超时/独立 post → 时间戳日志与取消 | 条件正确；参数完整；失败即停；post 不覆盖原证据；超时/取消无残留；不重置工作树；生效 upload 预检查拒绝 |
| 003-build-artifacts | P1；002 | glob 收集 → 路径与符号链接边界 → 产物清单、大小与摘要 | 产物可定位和校验；越界访问与不符合匹配要求的配置被拒绝 |
| 004-android-build | P1；003 | 本机 Android doctor → 版本和签名环境 → Gradle 模板 → 缓存复用 | 模板默认只构建/收集；真项目产出 apk / aab / mapping；版本正确；错误 JDK 或签名配置可诊断 |
| 005-ios-build | P1；003 | 本机 iOS doctor → 临时 keychain 与 DerivedData → archive/export → 资源清理 | 模板默认只构建/收集；真项目产出签名有效的 ipa / dSYM；失败与取消后临时签名资源清理 |
| 006-control-plane | P2；003、004 | 双数据库/单控制端锁/项目组/token → SHA/条件事实 → build when → 请求幂等与原子入队 → API/远程 CLI | skipped 不占节点/构建号；批量校验失败不部分入队；重试请求不重复编号；第二控制端被拒；保存预算/进度 |
| 007-node-agents | P2；006 | Agent/租约 → 项目+build 互斥 → 上下文/条件快照 → 超时/post → 时间戳日志与产物 → 管理停止确认 | 同名串行、不同 build 并行；失效回报拒绝；失联不迁移执行/收尾；超时正确 |
| 008-build-recovery | P3；007 | 租约核对 → 排队恢复 → 剩余预算/收尾记录 → 中断/停止未确认保护 → 原提交重试 | 不误判续租节点；预算不重置；已开始未知收尾不重跑；停止未确认不解互斥/不清理；retry 保留条件事实与原 SHA/配置 |
| 009-flutter-builds | P1；004、005 | Flutter doctor → Android/iOS 模板 → 版本/flavor/签名 → SDK 标签与产物 | 真 Flutter 工程双平台可构建；用户可编辑命令；复用已有签名、取消、产物与节点协议 |
| 010-google-play | MVP 发布；008、009、019 | fastlane 原型与锁版本 → 受控参数/应用唯一项目绑定 → 持久化发布意图 → AAB 上传及轨道 → 回执/查询/人工确认 | internal 可见真实 AAB；production 显式选择；错应用/凭据拒绝；失联 unknown 不重发 |
| 011-app-store | MVP 发布；008、009、019 | fastlane deliver 与 API key 原型 → 应用唯一项目绑定/IPA 核对 → 上传/处理状态 → 显式提交审核 → 回执/人工确认 | App Store Connect 可见构建；提交审核与正式上架分开；缺失前提明确失败；原产物和未知结果保护 |
| 012-custom-workflows | MVP 扩展；003、007、010、011 | 复用脚本参数 → 单 build 可复用方案 → project init 框架/双平台绑定 → auto/repo/profile 与命名 build 参数 → 快照 → custom 发布 | 无 YAML 可绑定双平台；来源集合不合并；错误配置不回退；原快照重试；同应用发布互斥；unknown 不重发 |
| 013-build-notifications | 后续 P3；008 | 项目直接配置 Webhook → 全局 defaults 与字段继承 → 显式关闭/列表替换 → URL 校验和受限凭据存储 → 飞书 SDK / 其他渠道 | 无需全局注册；未配置时继承；空列表或发送失败不改投默认；快照有效；URL/签名密钥不泄露 |
| 014-release-approval | MVP P3；008、010、011 | CLI 审批/鉴权 → approval when → 产物与进度落库 → 释放槽 → 原节点恢复 | 不依赖机器人通知；跳过审批不放行上传；等待不计超时；重复决定/挂起取消安全；报告/产物封存；恢复重新检查原节点 |
| 015-webhook-trigger | MVP P4；006、008、012、014 | provider 校验 → 显式 build 范围 → quiet_period → changes/when → 原子入队与发布权限 | 首构建/缺失基线完整执行；删除/重命名/公共目录有效；窗口重启/去重正确；不绕过发布授权 |
| 016-scheduled-builds | 后续 P4；015 | ls-remote 与游标/触发请求事务 → 固定轮询 → cron → 同一触发入口 | 创建触发请求失败不丢变更；定时任务复用等待窗口、队列和项目发布策略 |
| 017-extra-distribution | 后续 P5；012、014 | 按需求增加 fir.im / generic 内置适配，复用现有发布记录与用户扩展契约 | 验收原产物上传、远端确认及未知结果；MVP 不依赖本功能 |
| 018-service-operations | 后续 P6；012 | macOS/Linux 自启 → drain/停机 → doctor/日志打磨 → 测量后调整容量 | 独立部署；复用 MVP 清理/日志；基础错误处理和资源清理不能后置 |
| 019-test-reports | MVP P1/P2；003、007 | reports.junit → 受限 XML 解析 → 本地新鲜性/按路径替换汇总 → 发布前证据封存 → CLI/JSON 摘要与原始产物 | 测试失败不发布；准备步骤未出报告不误失败；旧报告不能放行；重复计数/非法 XML/越界/缺失可诊断；post 不改放行证据 |
| 020-project-retention | MVP P5；008、019 | 项目/全局字段继承 → 终态筛选 → 保护状态复核 → 中央删除记录/下载保护 → Agent 独立删除指令与幂等确认 | 项目跨 build 计数；活动/审批/停止未确认/unknown/下载中数据不误删；离线节点不假报完成；计数器保留 |

**MVP 必须完成 001–012、014–015、019–020**：同仓库多 build、双平台、多节点、项目组、两大商店、when、参数约束、总超时、post、时间戳日志、变更筛选、触发等待窗口、审批、保留策略和测试报告。
000 已完成；013 通知、016 轮询/cron、017 额外渠道、018 部署打磨可后置。不能后置发布授权、租约、unknown、原产物绑定、测试失败阻止发布及显式确认。
保留现有编号，编号不是执行顺序；019 必须先于 010/011，MVP 不以完成 012 单独判定。
建议顺序：000 → 001 → 002 → 003 → 004/005；004验收后可独立推进006 → 007 → 008/019，004/005均验收后推进009 → 010/011 → 012 → 014 → 015/020。
斜线表示前置满足后可分别推进；依赖检查使用拓扑顺序，不要求依赖编号更小。
006 建立控制端业务与排队能力，不执行移动构建，依赖已验收003/004的配置、预览与公共process基础；005真实签名验收不阻塞其独立worktree，修正依据见MVP_EXECUTION。007 闭合远程构建、节点 doctor、日志 SSE 与产物下载，保留后续iOS签名与租约取消联验。
009 复用 004/005 的平台能力；010/011 共用受控 fastlane 调用与发布记录，不把上传当成已公开上架。
012 扩展现有执行路径，不加载动态插件；有独立前提的功能可按依赖提前推进。
006 管理项目 pipeline 设置并默认要求仓库配置；012 增加控制端方案和缺失回退，之前选择尚未支持的来源应明确报错。
006 的 project init 是客户端远程注册，支持 --group，缺省归入 default；001 的无参数 init 仅生成本地最小 default shell 流水线；平台模板在对应功能接入后统一生成平台命名 build。
001 先建立 default/builds 格式、--build/--all 与 --param 的预览规则，002 执行 shell 及本地多 build，预检查拒绝生效 upload；平台模板随 004/005/009 接入。
006 支持一个仓库选择多个 build，固定一个 SHA 并原子创建独立执行；007 将互斥细化为同项目同名 build，不同 build 可在容量允许时并行。
012 接入无需 YAML 的 --framework/--platform 方案绑定，双平台组合已有模板，不新增执行器；此前相关选项明确报未支持。
shell 的字符串 params、显式 env 映射、位置参数、受限上下文和 working_dir/timeout 在[配置设计](CONFIGURATION.md#shell-执行与脚本参数)中统一定义，不另外设计 Jenkins 参数 DSL。
项目组用于归属和查询，构建方案用于配置复用；改组不改变项目身份、构建号、节点授权、通知或运行中的配置快照。
013 增加项目直接配置 Webhook 与全局默认值，不要求仓库配置存在；方案切换不能扩大项目节点或发布授权。
014 不依赖 013，MVP 审批通过 CLI 查询/放行，notify 默认 false；通知模块接入后显式开启，不提前注册空通知能力。
001 校验 when 与参数约束；002 执行本地条件、post 和预算；006/007 保存/执行冻结事实；008 恢复预算与收尾；015 闭合自动路径筛选和等待窗口。
019/020 独立实现报告和清理；未完成对应功能时，显式配置 reports 或 retention 的实际执行必须报未支持，不能静默忽略。
新增 Flutter/主商店/用户扩展后，原 009–014 尚未实现的路线调整为 013–018；无需修改已有 spec 目录。
001 校验 runner、商店/custom 的配置结构，本地 run 检查宿主能力；未实现步骤的执行须明确失败。
无 runner 的远程构建必须有项目默认节点；无匹配节点保持排队，不任意降级执行。
协议边界见 [MULTI_NODE.md](MULTI_NODE.md)，模板/发布契约见 [BUILD_DISTRIBUTION.md](BUILD_DISTRIBUTION.md)。

## 每个 feature 的工作流

```text
specify → clarify（有实质歧义时）→ plan → tasks → analyze → implement → converge
```

1. `specify`：描述本功能的用户行为、范围、验收与前置依赖；把 PLAN 及对应专题的相关要求带入 spec，不只放一个文档链接。
2. `plan`：选择本功能的实现方式，读取已有代码，列出文件和接口；复用现有能力，不提前实现后续 feature。
3. `tasks`：按用户故事拆成有路径、有完成条件的任务，显式包含必要测试，避免模板的“测试可选”导致漏项。
4. `analyze`：检查 spec / plan / tasks 是否一致，要求与验收是否均有任务覆盖；修正阻塞问题后再实现。
5. `implement`：按依赖顺序完成任务，运行检查并记录结果；未实现的步骤必须明确报错，不能静默跳过。
6. `converge`：核对当前代码与该 feature 的文档，存在缺口时追加任务；再 implement → converge，直到验收闭合。

每个 feature 产生 `specs/<实际功能目录>/spec.md`、`plan.md` 和 `tasks.md`；
按当前 plan 工作流同时生成简短的 research、data-model、contracts 与 quickstart。
CLI 契约写命令、输出和退出码；不为 CLI 功能杜撰 HTTP API 或数据库模型。
业务变化更新对应 feature 文档，跨功能技术决策变化同步 PLAN；constitution 只承载稳定原则。

流程参考 [Spec Kit 官方说明](https://github.com/github/spec-kit)，具体调用和产物以项目安装的提示词为准。

## 案例：001 流水线初始化与预览

这是在 000 初始化基础上开始的第一个业务 feature。下面是输入和期望产物示例，不是已执行的命令。

### 第一步：描述需求

```text
$speckit-specify
实现 mybuilds 的“流水线初始化与预览”，依据 docs/plans/DELIVERY.md 的 P0 与 docs/plans/CONFIGURATION.md。
用户能运行 init 在当前目录生成最小 mybuilds.yml，已有文件时拒绝覆盖；
用户能运行 run --dry-run 预览步骤顺序、名称和参数，不执行命令、checkout、上传、通知或创建构建工作区。
支持原单流水线作为 default，以及一份文件内的命名 builds，拒绝混写；--build/--all 选择，多个定义不允许隐式全选。
--param key=value 覆盖已声明的字符串参数，未知/重复键报错；只预览 run 的 shell/working_dir/env/timeout，不启动 shell 或探测目录。
参数支持字符串默认值或 default/description/required/choices 对象，合并后校验；when 只接受 branches/params/changes，字段 AND、列表 OR。
校验 build.timeout、post 的 run/artifact 列表与独立 timeout、reports.junit 结构；含 upload 时拒绝普通 run/artifact 与发布段交错；仅展示条件结论、原因或待确定，changes 不产生 Git 网络请求。
只接受规划中的四种步骤结构；runner 支持 platform: android|ios 与 labels 字符串列表；upload 按 google_play/app_store/custom 校验不同字段；严格拒绝未知字段、无效步骤及未知模板变量。
env 和明确的凭据字段支持引用当前环境变量，缺失时报错；run 正文中的 shell 变量保持原样。
运行时才能确定的已知变量在预览中标为待确定，不伪造实际构建号或状态。
引用的密钥与敏感字段在预览及错误信息中不得显示明文。
notifications.webhooks 接受 type/url 列表，直接填写的 Webhook URL 同样属于敏感字段；预览不发送请求，也不创建凭据文件。
本功能只解析和预览步骤与节点条件，不进行节点选择，也不实现步骤执行、数据库、HTTP 服务或分发。
请包含必要的配置校验、变量处理、脱敏、无副作用和 init 不覆盖测试。
```

`spec.md` 应说明以下故事与验收，而不是只罗列内部包：

| 用户故事 | 验收案例 |
|---|---|
| US1（P1）：初始化配置 | 空目录 init 成功且生成配置可解析；已有配置时失败并保持原文件不变 |
| US2（P1）：预览流水线 | 合法配置输出有序步骤；命令、checkout、上传、通知不发生；不创建工作区 |
| US3（P2）：定位错误与保护密钥 | 拼错字段、缺失引用或未知变量时返回非零退出码与字段位置；敏感值不出现在 stdout / stderr |

明确边界：缺失配置、重复步骤名称、非法步骤结构、run 正文包含 `${LOCAL_VAR}`、
单/多 build 格式混写、重复 build 键、未知名称、多 build 未选择、参数重复/未声明与正文中的引用保持原样、
已知但尚未产生的运行时变量、输入里声明了未来阶段才会执行的 approval / upload。
声明 approval / upload 可以预览，upload 凭据只保留引用，dry-run 不探测商店；非 dry-run 执行在后续功能完成前必须报未支持。

完成标准：生成的最小配置能预览；所有错误用例返回非零；在 `run` 中放置写文件命令后，dry-run 不产生该文件；
以一个唯一测试密钥检查 stdout / stderr，两者均无该密钥。

### 第二步：制定实现计划

```text
$speckit-plan
读取 docs/plans/PLAN.md、docs/plans/ARCHITECTURE.md、docs/plans/CONFIGURATION.md、docs/plans/INTERFACES.md 和现有代码，使用 Go、Cobra、go.yaml.in/yaml/v3。
复用 cmd/mybuilds、cmd/mybuilds-server、internal/cli/client、internal/cli/server 和 internal/version，仅增加本功能需要的 internal/config。
配置直接解码到结构体并严格校验；env / 凭据按字段解析，run 正文不插值。
预览渲染不调用执行器，已知运行时变量显示待确定；明确 init / dry-run 的输出与退出码。
用 Go 自带 testing 编写上述行为测试。不引入 Gin、GORM、Viper、cron 或飞书 SDK，它们在后续功能接入。
本功能无数据库；数据模型仅说明配置结构，contracts 描述 CLI 契约，quickstart 给出可重复验收步骤。
```

`plan.md` 中的实施顺序为：复用入口 → 配置结构与校验 → 字段插值和脱敏 → 命令接入 → CLI 验收。
两个入口只提供当前阶段所需命令，不提前挂一整棵空子命令树。

### 第三步：拆成任务

```text
$speckit-tasks
按 US1/US2/US3 生成任务，包含需求中指定的测试和具体路径。
非平凡行为先有失败用例再实现；只为当前功能增加代码。
```

`tasks.md` 的粒度示例（具体任务由实际 plan 生成）：

```text
- [ ] T001 复用 go.mod、Cobra 与双版本入口，按本功能引入并锁定 YAML
- [ ] T002 定义 internal/config/pipeline.go 的 default/builds、参数约束、when/timeout/post/reports、四种步骤与执行字段；增加对应失败用例
- [ ] T003 [US1] 在 internal/cli/client/root_test.go 写 init 生成与拒绝覆盖用例
- [ ] T004 [US1] 在 internal/cli/client/root.go 实现 init，生成最小可预览配置
- [ ] T005 [US2] 在 internal/config/pipeline_test.go 写合法配置、正文原样保留与运行时变量用例
- [ ] T006 [US2] 在 internal/config/pipeline.go 实现严格解码与按字段变量处理
- [ ] T007 [US2] 在 internal/cli/client/root_test.go 写 dry-run 有序输出、无命令执行、无工作区创建用例
- [ ] T008 [US2] 在 internal/cli/client/root.go 实现纯预览路径与退出码
- [ ] T009 [US3] 在 internal/config/pipeline_test.go 和 internal/cli/client/root_test.go 写未知字段、缺失密钥及输出脱敏用例
- [ ] T010 [US3] 在 internal/config/pipeline.go 和 internal/cli/client/root.go 完成错误定位与预览脱敏
- [ ] T011 按 specs/<实际功能目录>/quickstart.md 验收 init / dry-run，运行 go test ./... 并记录结果
```

示例按顺序执行，不把修改同一文件的任务标记为可并行。

### 第四步：检查、实现和闭合

在代理聊天中分别调用：

```text
$speckit-analyze
$speckit-implement
$speckit-converge
```

每一步完成后再执行下一步。analyze 的阻塞问题先修正；converge 追加了缺口任务就继续实现，
无缺口且行为验收通过后，推进 002-local-run。

001 的演示配置：

```yaml
version: 1
env:
  TEST_SECRET: ${MYBUILDS_TEST_SECRET}
steps:
  - kind: run
    name: preview-only
    run: |
      echo "$TEST_SECRET" > should-not-exist.txt
```

在临时目录中设置测试环境变量并执行 `mybuilds run --dry-run`，应能看到步骤名且密钥隐藏，
`should-not-exist.txt` 不存在。暂不调用普通 `run`，它属于下一个 feature。
