# Implementation Plan: Webhook触发、固定窗口与changes

**Branch**: `015-webhook-trigger-planning` | **Date**: 2026-10-05 | **Spec**: [spec.md](spec.md)

**Input**: `specs/015-webhook-trigger/spec.md`，28FR/7SC/17AC。规范、任务、只读分析及实现已完成；必要验证见validation，外部/完整人工矩阵待验。

## Summary

四来源固定为GitHub/GitLab/Gitee/generic。控制端验证有界原body和独立项目密钥，在短事务内保存安全事件与固定窗口后立即响应；现有Server生命周期处理due窗口，以可信仓库HEAD固定SHA，沿012来源选择和同一Trigger准备路径解析定义/参数/Preview，再以窗口/项目/基线的最后CAS原子Enqueue。changes是冻结事实，供唯一Run使用；不添加调度器、hook框架、第二执行器或节点Webhook。

## Technical Context

**Language/Version**: Go1.25.4，单模块；与接受后的008/012/014基线复核实际版本。
**Primary Dependencies**: 标准库net/http、crypto/hmac/sha256/subtle、encoding/json；既有Cobra/Viper/YAML/GORM/SQLite/pgx/doublestar与process.Run；按产品接入`github.com/go-playground/webhooks/v6@v6.4.0`候选，仅GitHub/GitLab真实消费者，主代理在第一原型门冻结依赖/校验sum，当前不改go.mod。Gitee/generic为有限解析，不安装JSON-pointer或事件总线库。
**Storage**: 既有SQLite/PostgreSQL、控制端唯一锁/Store.write短事务；新增policy、receipt、window记录和最小快照事实，密钥独立0700/0600文件，不存原payload。
**Testing**: 原包真实HTTP/Git/双库事务、三入口进程、race/vet与编译矩阵；四provider实际push另有强制门，不用curl自制payload替代。
**Target Platform**: 控制端macOS/Linux，客户端既有跨平台；Agent沿已验收平台与能力，无新增工具假报。
**Project Type**: 既有CLI+鉴权HTTP控制面。
**Performance Goals**: 接收不执行Git，10秒内成功或安全失败；30秒示例窗口正常可读仓库截止后60秒内有唯一结果。单次关闭总45秒，初次deadline不重置。
**Constraints**: 006/007接受；008/012/014/019规划仅参考，008/012/014正式接受/集成且各共享权限实际门满足后才实现。019报告真实门不能被Webhook豁免。请求/JSON/Git/路径/窗口分页数字仅一处定义于[输入与条件契约](contracts/config-changes.md)。
**Scale/Scope**: 一控制端、多项目、多命名build；四来源push、固定等待/去重/changes。轮询/cron、通用hook插件、多控制端HA不进入本次。

## Constitution Check

| 原则 | Phase0前 | Phase1后 |
|---|---|---|
| I 规范与接受前置 | spec/checklist16/16；前置未过不代码 | 同门，独立规划WT实际tasks/analyze，root串行复核 |
| II 单模块/单Run/节点权 | 复用Trigger→Store→Agent→Run | 只增加可信触发来源与条件事实，不授节点/审批/发布额外权 |
| III 最小依赖 | 首选stdlib，沿产品webhooks/v6 | 库两实际consumer，无registry/interface/新worker框架 |
| IV 边界与未知保护 | 不读host材料或payloadURL，独立密钥 | 接收/关闭末尾复核锁与策略；原应用unknown/stopguard/receipt不动 |
| V 真实行为/中文 | 文档中文，四来源与双库/双OS门明确 | 缺真实provider材料保持待验收，无模拟成功声明 |

结论：设计检查通过，无原则豁免。完成Phase1后重新读取constitution2.1.0；仅规划，不表示必要真实门已执行。

## 具体接线与实施顺序

1. 接受前置后由根冻结[Go契约](contracts/go-api.md)实际字段及库版本；源代码红测试先于实现。现有Project.Provider正好为四值；Repository不可变，项目范围沿PolicyVersion。
2. 管理员经原settings导入triggers/hook；`--hook`初始化/轮换仅一次返回自产秘密；0600独立项目key或明确hook envfile引用都不进入普通DTO。配置和DB业务只存凭据身份/引用与指纹，不能把Webhook文件传给SCM两键loader。
3. HTTP唯一公开`POST /hook/{project}`先CheckLock/当前启用→有界raw读取→固定provider认证→JSON/仓库/分支判断→ReceiveWebhook。库输入仅已限额不可变bytes克隆；未知metadata可忽略但重复JSON keys/坏形状拒绝。token模式不宣称body签名完整性。
4. Server现有控制循环每1秒处理due页，单个窗口不持DB锁做Git。首事件固定截止，接收半开区间；截止后事件另开下一代，旧due窗口仍可准备关闭。无“closing lease”或后台接管执行模型。
5. 准备关闭时读窗口、当前项目、原选择与参数输入，固定可信branch HEAD；012 resolvePipeline沿此精确SHA读取唯一来源；先验证全部参数/定义/upload资格。比较键和成功基线按[data-model.md](data-model.md)冻结，SCM只比较两个固定tree；不信provider提交/文件列表。
6. Preview及buildCondition复用同一纯when判断并接收Changes；动态build编号/节点/工作区仍pending，不能拿Preview的pending当build.when跳过。静态condition、定义、参数、SHA、完整Changes随快照保存；Run/Agent只消费此事实、不再次Git diff。
7. CloseWebhookWindow最后短事务复核锁、有效HookActor/项目PolicyVersion、原窗口revision/state/deadline、全部比较键最新成功baseline、当前节点/应用/allow_upload；调用从原Enqueue提取的私有创建事务体，所有fresh/reused/skipped结果与计数器、窗口closed同提交。冲突重新准备，不能复用旧baseline或部分分号。
8. 同delivery跨credential轮换仍按项目/provider/标识唯一，认证后同raw摘要返原接收归属；无ID相同自动语义key只重用现有明确状态的同SHA结果。手动/ retry仍有其原key规则，不被自动去重；变化比较可引用同范围手动/retry已确认成功，但不改其执行条件。
9. Restart只恢复pending窗口/未关闭任务，deadline保持，不重新解释已closed快照。未知执行、审批/应用guard与终态receipt沿008/014，不重放用户步骤。

## Project Structure

### Documentation

```text
specs/015-webhook-trigger/
├── spec.md
├── checklists/requirements.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
└── contracts/{go-api.md,http.md,config-changes.md}
```

### Source Code（前置接受后才创建）

```text
internal/config/webhook.go              # 严格项目块与独立有限secret读取
internal/scm/hook.go                    # 固定provider验证/正规化
internal/scm/changes.go                 # 有界固定SHA tree diff，原gitRunner
internal/store/webhook.go               # policy/event/window与同事务关闭
internal/server/webhook.go              # 唯一公开入口与管理safeDTO
internal/server/webhook_window.go       # 原Server生命周期的due consumer
internal/pipeline/changes.go            # 原evaluateWhen消费固定事实
```

## 文件唯一归属与共享接线

| 分区 | 独占候选新文件与tests | 与根共享的现有文件 |
|---|---|---|
| A持久化 | store/webhook.go、webhook_models.go及实际双库tests | store模型/迁移/Enqueue私有tx提取、Recover/Retry/query；根串行 |
| B来源/条件 | scm/hook.go、changes.go；pipeline/changes.go及tests | scm/git.go私有bare重复部分最小提取、preview.go/run_types/run.go；根串行 |
| C配置/HTTP管理 | config/webhook.go、server/webhook.go/window.go及tests | config.ProjectSettings/LoadServer、server.Trigger/http/lifecycle、CLI flags/DTO；根串行 |
| 根 | protocol.ChangeFacts、共享文件、go.mod/sum、README/history/validation | 接受前置再逐SHA同步；不互写同一共享文件 |

tasks已明确A持久化/B来源条件/C配置HTTP/root共享与CLI；CLI由root唯一承担，前置接受后root再复核实际分区。不能为了避免共享修改复制Enqueue/条件解析或新建仓库接口。没有新增Agent业务worker；仅原RunOptions/PreviewOptions和Task.Snapshot的冻结Changes字段消费。唯一发布/审批/报告消费者不因自动来源而改变。

## 覆盖与必须真实门

| 意图 | 设计/验证落点 |
|---|---|
| FR001–008、SC001/006、US1.AC1–3/5 | provider表、独立secret/JSON/原body门；四真实push、错密钥/身份/大小 |
| FR009–010、019、027、US1.AC4/5、US4.AC1/4 | admin import、含upload先权限、当前policy末尾CAS；真实approval/report/app/node联验 |
| FR011–018、SC002–004、US2.AC1–4 | receipt唯一/窗口分代/HEAD冻结/closeTx；20竞争与4退出点双库 |
| FR020–025、SC005、US3.AC1–4 | 比较键/原成功receipt时间/diff与when事实；新增删改改名公共目录/首缺/手动retry |
| FR026、SC006、US4.AC2/3 | safeDTO/固定reason、semantic reuse/failTx，无raw秘密 |
| FR028、SC007、US4.AC4 | quickstart全源、双库、mac/Linux真实三入口及原行为检查 |

tasks/analyze仅规划一致性检查，converge留待实施验收；所有28FR/7SC/17AC有设计落点，必须门尚未运行。前置、hosted账号/仓库/公网TLS缺失只记待验收，不降低四来源硬门。

## Complexity Tracking

无原则偏离，无新增框架。事件/窗口表只服务当前Webhook恢复；自动authority具体结构避免伪装管理员Actor；私有Enqueue事务体供手动/自动两实际消费者，不建立通用事务调度层。

## 当前实施授权与边界（2026-10-05）

基线已更新为2602094，008、019、020真实接受；012/014及两商店消费者并行交付，不能把它们的规划或未集成源码说成已验收。用户最新要求MVP各模块完成代码与必要自动检查，四外部provider授权push移至最终统一人工验收，保留原FR/SC/AC材料门待验，不将自制payload当真实来源。通知013仍后置，本功能不发送通知或增加飞书依赖。

本轮唯一实现writer为webhook015-implementation，拥有本WT的015新文件及真正共享consumer变更，root只在冻结后串行整合；不用旧A/B/C并写同文件。新增事实仅供当前SCM/Preview/Store/Agent/Run消费者，来源resolver沿012实际交接，不提前造stub。必要自动门包含本机实际HTTP、可信Git、SQLite/PostgreSQL同套事务与幂等/签名/预算/秘密；未知发布和审批授权不可虚构或绕过。
