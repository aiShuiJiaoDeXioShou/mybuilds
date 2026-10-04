# mybuilds 的 Spec Kit 实施路线

本文件是功能拆分与操作案例，不代表已生成正式 spec 或开始实现。产品与技术决策以 [PLAN.md](PLAN.md) 为准。
按可独立验收的用户能力拆分 feature，不按 config / store / API 等目录分别开发。

## 开始前

- 当前使用 Spec Kit 1.0.13，Codex 已设为默认集成，技能位于 `.agents/skills/speckit-*/SKILL.md`；下文使用 `$speckit-xxx` 写法，在 Codex 聊天中逐条调用，不是 shell 命令。
- Pi 集成保留在 `.pi/prompts/`，使用 `/speckit.xxx` 写法；本项目的 bug 扩展也已注册为 Codex 技能。技能列表未更新时重新打开 Codex 会话。
- `.specify/memory/constitution.md` 仍是占位模板，先确定项目原则；当前目录尚未初始化 Git，若采用分支工作流，先建立仓库和基线提交。
- 一次只推进一个 feature。编号为建议顺序，正式目录名以 Spec Kit 的实际生成结果为准；后续 feature 基于已完成的代码继续实现。

项目原则的输入示例：

```text
$speckit-constitution
以 docs/plans/PLAN.md 为依据制定项目原则：注释与文档用中文；Go 双 CLI、单机 macOS 执行；
标准库优先、按功能引入依赖、飞书使用官方第三方 SDK；不增加插件系统、Web UI 或分布式执行器。
严格校验外部输入；配置和日志不泄露密钥；审批和构建进度持久化；不可确认的上传不得自动重发。
非平凡逻辑必须有最小可运行验证，安全、事务、恢复和进程取消必须有相应测试；不设虚构的覆盖率指标。
完成标准是验收行为通过，而非仅完成任务勾选。
```

## 功能路线

每一行对应一个 feature 的 spec / plan / tasks。阶段对应 PLAN 中的 P0–P6；依赖表示需要复用哪些已完成能力。

| 建议编号与目录后缀 | 阶段 / 依赖 | 实施顺序 | 功能完成的验收点 |
|---|---|---|---|
| 001-pipeline-preview | P0；无 | 双 CLI 入口与版本 → init → 配置结构和严格校验 → 按字段插值与脱敏 → dry-run | 可生成配置并预览；错误字段和缺失密钥报错；预览没有外部动作 |
| 002-local-run | P1；001 | 顺序执行 run → 分步骤日志 → 失败收尾 → 取消与平台进程控制 | 假项目可运行；失败不继续；取消后无该次构建残留进程；当前工作树不被重置 |
| 003-build-artifacts | P1；002 | glob 收集 → 路径与符号链接边界 → 产物清单、大小与摘要 | 产物可定位和校验；越界访问与不符合匹配要求的配置被拒绝 |
| 004-android-build | P1；003 | 本机 Android doctor → 版本和签名环境 → Gradle 模板 → 缓存复用 | 真项目产出 apk / aab / mapping；版本正确；错误 JDK 或签名配置可诊断 |
| 005-ios-build | P1；003 | 本机 iOS doctor → 临时 keychain 与 DerivedData → archive/export → 资源清理 | 真项目产出签名有效的 ipa / dSYM；失败与取消后临时签名资源清理 |
| 006-server-builds | P2；004、005 | 双数据库与事务 → 项目 / token 管理 → 固定 SHA 工作区和配置快照 → 持久化排队 → API 与远程 CLI | trigger、ls/show、logs、cancel、产物下载闭环；同项目串行；鉴权、双数据库测试通过 |
| 007-build-recovery | P3；006 | 启动状态扫描 → 恢复排队 → 中断任务标记 → 原提交重试 | 重启不丢排队任务；普通执行中任务变 interrupted；retry 用原 SHA / 配置 / 参数与新构建号 |
| 008-build-notifications | P3；006、007 | 飞书 SDK Webhook 验证 → 企微 / 钉钉 / generic → 统一收尾接入 | 成功、失败、取消都有对应通知；通知失败不覆盖构建结果；凭据不泄露 |
| 009-release-approval | P3；007、008 | 审批位置落库 → 挂起释放槽 → 角色鉴权和决定记录 → 5s 扫描重新入队 → 重启恢复 | 待审批时其他项目可构建；批准后从正确位置继续；重复决定、取消竞态不能覆盖状态 |
| 010-webhook-trigger | P4；006、007 | 注册 provider → GitHub / GitLab 库适配 → Gitee / 自建解析 → 校验、过滤 → 事件与任务事务入库 | 四类来源可触发；伪造请求被拒；重复事件只入队一次；相同 SHA 不同参数可构建 |
| 011-scheduled-builds | P4；010 | ls-remote 与分支游标 → 游标和入队事务 → 固定间隔轮询 → cron 定时 | 代码变更能触发；入队失败不丢变更；定时构建复用同一触发入口 |
| 012-artifact-distribution | P5；003、009 | 唯一产物匹配 → 上传操作记录 → 蒲公英 / fir / generic → 结果查询及人工确认 | 发布审批过的原产物；真上传闭环；远端已接收而本地未记录时不自动重发 |
| 013-service-operations | P5/P6；012 | retention → launchd → 优雅关闭 → doctor 和日志重连打磨 → 基于测量调并发 | 自动启动；停机状态可恢复；清理保护待审批和未知结果；无敏感信息回显 |

006 包含远程 `doctor`、列表 / 详情 `--json`、构建工具版本与耗时记录、受鉴权的产物下载；
013 在已有能力上打磨，不重复实现。004、005 尽早验收真实工程，避免服务端完成后才发现签名或 SDK 环境不适用。
010、011 共同复用 006 的触发入口，不能各建一套队列或构建号分配逻辑。

## 每个 feature 的工作流

```text
specify → clarify（有实质歧义时）→ plan → tasks → analyze → implement → converge
```

1. `specify`：描述本功能的用户行为、范围、验收与前置依赖；把 PLAN 中的相关要求带入 spec，不只放一个文档链接。
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

这是可从当前空业务代码状态开始的第一个 feature。下面是输入和期望产物示例，不是已执行的命令。

### 第一步：描述需求

```text
$speckit-specify
实现 mybuilds 的“流水线初始化与预览”，依据 docs/plans/PLAN.md 的 P0。
用户能运行 init 在当前目录生成最小 mybuilds.yml，已有文件时拒绝覆盖；
用户能运行 run --dry-run 预览步骤顺序、名称和参数，不执行命令、checkout、上传、通知或创建构建工作区。
只接受规划中的四种步骤结构，严格拒绝未知字段、无效步骤及未知模板变量。
env 和明确的凭据字段支持引用当前环境变量，缺失时报错；run 正文中的 shell 变量保持原样。
运行时才能确定的已知变量在预览中标为待确定，不伪造实际构建号或状态。
引用的密钥与敏感字段在预览及错误信息中不得显示明文。
本功能只解析和预览步骤，不实现任何步骤执行、数据库、HTTP 服务或分发。
请包含必要的配置校验、变量处理、脱敏、无副作用和 init 不覆盖测试。
```

`spec.md` 应说明以下故事与验收，而不是只罗列内部包：

| 用户故事 | 验收案例 |
|---|---|
| US1（P1）：初始化配置 | 空目录 init 成功且生成配置可解析；已有配置时失败并保持原文件不变 |
| US2（P1）：预览流水线 | 合法配置输出有序步骤；命令、checkout、上传、通知不发生；不创建工作区 |
| US3（P2）：定位错误与保护密钥 | 拼错字段、缺失引用或未知变量时返回非零退出码与字段位置；敏感值不出现在 stdout / stderr |

明确边界：缺失配置、重复步骤名称、非法步骤结构、run 正文包含 `${LOCAL_VAR}`、
已知但尚未产生的运行时变量、输入里声明了未来阶段才会执行的 approval / upload。
声明 approval / upload 可以预览，但非 dry-run 执行在后续功能完成前必须报未支持。

完成标准：生成的最小配置能预览；所有错误用例返回非零；在 `run` 中放置写文件命令后，dry-run 不产生该文件；
以一个唯一测试密钥检查 stdout / stderr，两者均无该密钥。

### 第二步：制定实现计划

```text
$speckit-plan
读取 docs/plans/PLAN.md 和现有代码，使用 Go、Cobra、go.yaml.in/yaml/v3。
只建立必要的 cmd/server、cmd/client、internal/cli、internal/config 和 internal/version。
配置直接解码到结构体并严格校验；env / 凭据按字段解析，run 正文不插值。
预览渲染不调用执行器，已知运行时变量显示待确定；明确 init / dry-run 的输出与退出码。
用 Go 自带 testing 编写上述行为测试。不引入 Gin、GORM、Viper、cron 或飞书 SDK，它们在后续功能接入。
本功能无数据库；数据模型仅说明配置结构，contracts 描述 CLI 契约，quickstart 给出可重复验收步骤。
```

`plan.md` 中的实施顺序为：必要入口 → 配置结构与校验 → 字段插值和脱敏 → 命令接入 → CLI 验收。
两个入口只提供当前阶段所需命令，不提前挂一整棵空子命令树。

### 第三步：拆成任务

```text
$speckit-tasks
按 US1/US2/US3 生成任务，包含需求中指定的测试和具体路径。
非平凡行为先有失败用例再实现；只为当前功能增加代码。
```

`tasks.md` 的粒度示例（具体任务由实际 plan 生成）：

```text
- [ ] T001 初始化 go.mod 并锁定 Cobra / YAML，建立 cmd/client/main.go 与 cmd/server/main.go 的版本入口
- [ ] T002 定义 internal/config/pipeline.go 的四种步骤结构与字段规则
- [ ] T003 [US1] 在 internal/cli/client_test.go 写 init 生成与拒绝覆盖用例
- [ ] T004 [US1] 在 internal/cli/client.go 实现 init，生成最小可预览配置
- [ ] T005 [US2] 在 internal/config/pipeline_test.go 写合法配置、正文原样保留与运行时变量用例
- [ ] T006 [US2] 在 internal/config/pipeline.go 实现严格解码与按字段变量处理
- [ ] T007 [US2] 在 internal/cli/client_test.go 写 dry-run 有序输出、无命令执行、无工作区创建用例
- [ ] T008 [US2] 在 internal/cli/client.go 实现纯预览路径与退出码
- [ ] T009 [US3] 在 internal/config/pipeline_test.go 和 internal/cli/client_test.go 写未知字段、缺失密钥及输出脱敏用例
- [ ] T010 [US3] 在 internal/config/pipeline.go 和 internal/cli/client.go 完成错误定位与预览脱敏
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
