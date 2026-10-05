# Implementation Plan：App Store 上传、显式提交与未知结果核对

**Branch**：`011-app-store-planning` | **Date**：2026-10-05 | **Spec**：[spec.md](spec.md)

## Summary

唯一签名IPA通过受控fastlane发布到已准备应用。默认仅上传；显式submit_for_review在同一Run的原身份、纳秒累计预算内继续提交，处理等待有界。每个实际远端变更先持久化意图/应用保护再授一次权；未知不重发。终态后的查询仅核对原动作，不复活执行权或提交。当前完成Phase0/1规划；tasks/analyze可先准备，源码实施仍等待真实前置验收，不创建Gem假锁或商店材料。

## Technical Context

**Language/Version**：Go≥1.25；节点Ruby/Bundler与fastlane精确版本由实施首门实测锁定，候选fastlane2.240.1不是已验收锁。
**Primary Dependencies**：现有Cobra/YAML/Viper/GORM及stdlib；无新增Go依赖。fastlane deliver/Spaceship成熟第三方能力，由项目薄Ruby入口限定实际请求与重试；共用010的实际runner、Gem锁。
**Storage**：现有SQLite/PostgreSQL相同Store.write短事务；应用绑定、发布意图/逐动作回执/保护及核对审计；文件仍受限中央/节点数据目录，不在SQL存材料。
**Testing**：实际process.Run、真实Store双库/20竞争、真实loopback计数与故障、macOS原生/Flutter签名IPA、实际App Store Connect上传和App Review提交。fake lane退出0不算商店验收。
**Target Platform**：macOS授权发布节点；macOS/Linux控制端；三CLI保持既有跨平台编译，Linux无Apple能力明确拒绝。
**Project Type**：现有单Go模块/三CLI/鉴权HTTP；不是独立发布服务。
**Performance Goals**：20个同应用申请至多一个动作授权；任务终态后不占构建槽等待审核；有限预算及每GET30s上界，分页/响应有界。
**Constraints**：有效Authority和剩余普通NS、声明报告封存、唯一原IPA；默认不提交/发布；未知持应用保护；读查询不写Apple；不扫描个人账户/hostkeychain/完整环境。
**Scale/Scope**：原生及Flutter iOS同一publisher；一个控制端；单构建不跨节点；不管理商店素材/账号/协议/内购、TestFlight、审核撤回或终态后另起发布生命周期。

## Constitution Check

| 原则 | Phase0 | Phase1复核 |
|---|---|---|
| I 规范驱动 | spec28FR/8SC/17AC、quality16/16冻结；只plan | 七docs覆盖全部；tasks/analyze由根随后执行 |
| II 单模块/三入口/一个Run | 复读Run/RemoteOptions、Agent journal、Store.write | 发布callback为两个实际publisher消费者；不另造Run或调度器 |
| III 最小实现/按需依赖 | 成熟fastlane，版本/重试先真实证明 | root唯一共享类型/Gem锁writer；无registry/repository interface/新Go依赖 |
| IV 边界/持久化/不可未知重发 | 每次变更先授权即unknown，权限/fence末尾复核 | 读取无副作用、逐动作结果、秘密受限、保护不因StopKnown释放 |
| V 真实验收/中文 | 官方primary研究、已有消费者核查 | 原签名/AppReview及双库/Mac/Linux门保留，缺材料不宣称完成 |

两阶段均PASS，无原则豁免。外部材料/工具原型是实施验收门，不是已通过证据；research已选择最小受控方法，不留用户需求澄清项。

## Project Structure

```text
specs/011-app-store/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
└── contracts/{go-api,http,config-cli}.md
```

实际新增/改动仅随实施消费者产生：internal/distribute/apple.go、apple_test.go及受控apple.rb；internal/store发布记录；internal/server发布路由；internal/config上传验证；internal/agent发布journal与callback；internal/pipeline唯一Run上传分支；客户端publish及doctor。共享fastlane.go/共同Ruby入口/Gemfile/Gemfile.lock/protocol/入口由根串行集成，不预建空包。

## Phase0/1交付与实施次序

1. 实际无凭据原型先验证候选Ruby/Bundler/fastlane的REST逐请求边界、POST/PATCH未知无重发及stdin/文件材料隔离，再生成实际安装组合的精确Gem锁；锁本身不是安全证明。Transporter真实会话/分块/最终提交须在明确授权材料的首个ASC上传门核实，可复用该次真实上传证据，不为检查重发；没有充分证据不得标工具已安全核验，失败先修阻塞。
2. 任务与只读分析可在规划阶段准备；源码实施前必须核实008、009、019已验收集成及005真实签名门，重基线并复核当前具体类型消费者，不能借未验收dirty实现。
3. 共用010应用绑定/保护/意图/查询/人工确认双库门→Apple受控publisher与IPA核验→唯一Run/Agent接线→安全HTTP/CLI与真实商店验证。
4. 014审批未实现时生效approval整批拒绝，不能跳过审批放行；020后续保护联验。019真实seal/report IDs必须来自原attempt，新retry不沿用旧pass。

## 文件归属

| 分区 | 独占范围 | 串行边界 |
|---|---|---|
| A Store | 发布业务/测试/migrations（010共用部分由根指定唯一owner） | 两库同suite、控制锁/fence/应用保护 |
| B Apple publisher | distribute/apple.go/apple_test.go/apple.rb、对应doctor测试 | fastlane.go/runner/Gem锁由根唯一writer；只既有process.Run |
| C 接线 | root指定的config/HTTP/CLI/Agent/Pipeline接线与测试 | 同文件不并行，原Run/protocol由根冻结同步 |
| root | 所有共享类型/依赖/README/history/validation与跨010集成 | 不因功能分区复制执行器/模型 |

实施任务细化为A唯一Store Apple新业务文件、B唯一distribute Apple文件、C唯一config/server/client Apple新接线文件；root唯一现有Store/Agent/Pipeline/protocol/CLI入口与共同工具文件，按实际API交接串行集成，不并写。此表为未来实施归属；规划阶段双方不互写共享源码。

## Coverage

| 要求 | 设计/验收入口 |
|---|---|
| FR001–007、SC001/003、US1.AC1–4 | IPA/签名/报告seal与唯一快照，config-cli/go-api、quickstart2–3 |
| FR008–011、SC002、US2.AC1–4 | 同Run显式选择、有界处理、6个实际变更，go-api、quickstart4 |
| FR012–020、SC004/005、US3.AC1–4 | app绑定/guard、双库授权、逐动作unknown，data-model、quickstart5–6 |
| FR021–022、US3.AC5–6 | GET-only充分证据/人工精确确认，http/go-api、quickstart6 |
| FR023–025、SC006/007、US4.AC1–3 | safeDTO/状态/槽释放/保留保护，http、quickstart7 |
| FR026–028、SC008 | 本地拒绝/纯dryrun/依赖门/完整真实证据，quickstart1/8 |

计数28FR、8SC、17AC；没有以quality勾选代产品验收。

## 执行记录

本WT真实setup-plan.sh --json exit0，输出FEATURE_DIR=specs/011-app-store、BRANCH=011-app-store；resolve-template.sh plan-template --json exit0；无preset目录，采用core plan-template；selector为ignored .specify/feature.json。before/after hooks={}，无可执行hook。原则2.1.0 read-after-write复核；未写源码/tasks/依赖/提交。

## Complexity Tracking

无原则违规或复杂性豁免。逐动作字段仅服务Apple真实变更，不扩通用事务workflow。

## 2026-10-05 用户验收安排与实施归属（覆盖原阶段门措辞）

用户明确要求先完成全部模块代码及必要自动验证，真实 Apple/Play 上传在最后统一案例由用户人工验收；没有凭据不阻塞代码交付/本地提交。原真实远端需求与 SC/AC 不删除、不宣 PASS，验证记录标人工待验；无凭据自动门仍须实际锁版本、真实本机故障端点单发、未知保护与安全失败。008/019/020已验收基线2602094；005/009按其实际模块接口接入而不虚构真实素材通过。

唯一 writer：publish-channels 分区独占 internal/distribute/**（两店具体 Go/Ruby、Fastfile、Gemfile/lock、测试）及新 internal/protocol/publish.go；root独占既有protocol/node.go、Store、Agent、Pipeline、config、server、CLI共享消费者与全局文档。原表/任务中分散在B/root的 distribute 与公共发布新类型任务统一归publish-channels；其他任务仍root协调唯一writer。一个Run/原process.Run、完整Ref与guard/unknown不变，无registry/第二执行器。
