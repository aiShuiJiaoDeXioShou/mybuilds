# Implementation Plan: 可复用构建方案与用户自定义发布

**Branch**: `012-build-presets-planning`（selector目录012-custom-workflows） | **Date**: 2026-10-05 | **Spec**: [spec.md](spec.md)
**Input**: 30FR/8SC/18AC。已有tasks；当前进入独立源码实施，不宣称真实签名或商店验收通过。

## Summary

在现有server.Trigger里选择一个来源：固定SHA仓库配置或已加载的单build方案绑定集合，然后仍用config.Parse/Select/ResolveParams、纯Preview、buildCondition及唯一Store.Enqueue。auto只对精确缺失回退；profile仍固定真实Git提交但不读取配置文件。来源证据随每条BuildSnapshot固化，008retry直接复制。自定义发布以distribute/custom.go具体消费者接既有RemoteOptions.Publish、完整lease/Authority、中央原artifact/JUnitSeal、010应用绑定/意图/unknown/回执/查询；继续process.Run，不增加执行器或插件注册层。

## Technical Context

- **Language/Version**: 原Go1.25单模块，标准库context/HTTP/JSON/crypto/os.Root及有限文件读取；文档与注释中文。
- **Primary Dependencies**: 基线Cobra1.10.2、Viper1.21.0、YAML3.0.5、GORM1.31.2、glebarez/sqlite1.11.0+modernc1.55.0及PG驱动1.6.3；本功能不新增Go/Ruby依赖。custom用户自行管理其工具版本，不自动安装。010/011受控Gem锁尚待实际首门，不能编造已锁版本。
- **Storage**: 复用Project.Settings、BuildSnapshot与010发布表/guard；增加optional具体Origin/Custom字段，旧记录未知不补猜，没有profiles数据库CRUD或缓存表。
- **Testing**: 先真实红绿；SQLite/PG相同来源/原子入队/retry/publish竞争suite；真实CLI/Server/Agent、固定Git、私有模板文件、自有接收端及fault/PGID证据；四种真实工程中央原产物。
- **Target Platform**: 控制端macOS/Linux；Agent沿原工具/框架标签与iOS机制；客户端原三入口跨编译矩阵。本地run不发布。
- **Project Type**: 现有三CLI、标准HTTP控制面；没有Web UI或新协议执行层。
- **Performance Goals**: 不新增吞吐承诺；文件/JSON受限，事务不等待Git/工具/流，沿现有原子请求与编号CAS。
- **Constraints**: 项目≤64命名build；单方案≤1MiB、YAML深度≤32/节点≤10000；加载≤64自定义方案且总≤16MiB。custom argv/query_argv各≤128项、单项≤4096字节、总≤64KiB；输入/结果/查询JSON≤64KiB、深度≤16/值≤4096，原stdout/stderr合计≤64KiB仅私有。所有更早ctx/普通NS/Authority取交集；query单次30s不占构建槽，Close独立15s只自有资源。
- **Scale/Scope**: 三source/四builtin/单build自定义方案、named覆盖与custom一种target；无多项目批量、继承、合并、通用registry，20同key真实竞争门。

## Constitution Check

Phase0前、Phase1后原则2.1.0均PASS，无例外。I：本轮spec→plan结束，后续analyze/implement/converge按已验收基线与真实冻结组件接口推进。II：三个入口、单Store控制端、多Agent、一条Run。III：复用已有类型/消费者，无新增执行器/通用repo/模板DSL/依赖。IV：可信仓库、strict输入、原SHA、事务权限与lease/NS/JUnit/原产物；unknown不重发，任意用户脚本不能保证remote只读/内部不重试。V：中文设计、真实红绿与完整平台材料门，准备不计验收。

## Project Structure

### Documentation (this feature)

spec/checklists由specify冻结；本轮plan.md、research.md、data-model.md、quickstart.md及contracts/{config-sources.md,go-api.md,custom-publish.md}七份设计文件。后续tasks/validation由主代理另阶段创建，此轮没有。

### Source Code (repository root)

| 唯一writer候选 | 路径与职责 | 共享前置 |
|---|---|---|
| A 配置/方案 | internal/config/profiles.go、profiles_test.go；project.go/server.go/private读取实际扩展；internal/cli/client/init.go/template测试 | 009 mobile模板真实接口冻结后；原parse.go根串行最小辅助复用 |
| B custom节点 | internal/distribute/custom.go/custom_test.go；internal/agent/publish_custom.go及tests | 010/011完整Publish入口/应用保护与019封存已交付；Agent原publish.go由根串行接 |
| C 来源/管理 | internal/server/pipeline_source.go及tests、trigger.go/project.go与CLI管理flags/tests；internal/scm/git.go/tests FileMode | 006/007现有入口；各文件唯一owner，不与A改同配置 |
| root 共享集成 | internal/store/{models,enqueue,retry,recovery,query,publish}.go；internal/protocol发布optional字段；pipeline/{run_types,run,preview}.go；Agent共同入口；README/历史/go.mod | 每个新增字段先有真实consumer；008/019/020已验收基线；005/009/010/011具体组件按SHA串行集成 |

同一阶段只有root+A+B+C四槽，不固定人员名。若实际分区调整，根在tasks阶段锁writer；同文件不并写、不跨story假[P]。config解析全包由A一个writer，共享parse最小改动根串行，不复制配置框架。方案展开放server是当前Trigger唯一消费者，不另建preset registry包。用户模板生成仍共用init路径，不执行模板。

## Phase 0研究与依赖

见[research.md](research.md)。本源码WT基线2602094，008/019/020已验收；005/009/010/011具体组件由根冻结后串行集成，不复制dirty源码。012原路线直接依赖003/007/010/011；其完整模板/恢复/报告验收还依赖004/005/008/009/019，不放宽。014可与012并行，但有效approval在014验收前仍unsupported；015只规划接冻结名称与来源，不提前做自动changes。020不得删保护中的意图/原快照，后续联验。

## Phase 1设计与实施顺序

1. 在正式前置基线上冻结BuildProfile/BuildSettings.Profile、Origin与custom具体variant。先旧配置/receipt摘要/migration兼容红绿，再双方共享字段落地；没有stub。
2. A严格加载方案和模板、单build校验；C实现真实ReadPipeline FileMode与Trigger来源二选一，完整所选参数/权限→Preview→when→唯一Enqueue。双库来源正负例及整批rollback先通过。
3. 根接Origin持久/查询/Recover和008retry复制，复验更改template/settings/HEAD后原快照执行；C接project框架平台flags及partial顶层块更新，A本地template生成和四builtin组合沿009。
4. B接custom Prepare/Upload/Query/Close及严格结果；根以010共同Store/protocol/Agent/Run Publish授权一次、unknown、原日志/NS、upload.finished关闭slot及终态manifest串行集成。custom binding明确manual来源/依据审计，不冒称商店GET验证。
5. 真实双库、两个合法节点、四种工程、中央binary与自有接收端故障；全量test/race/vet/构建、权限/秘密扫描、quickstart、converge、一次整功能提交。缺Apple/商店材料仍完成可执行代码和必要自动验证；真实签名、上传及完整联合场景留用户最终人工验收，未跑门明确pending。

## Coverage / Gates

| 门 | US/AC | FR | SC | 实际验收 |
|---|---|---|---|---|
| G1 内置/模板/绑定 | US1/1–4 | 001–003,007,008,013,014 | 001,008 | 四真实平台、单双组合、版本/签名/标签、旧本地回归 |
| G2 来源/严格读取 | US2/1–5 | 002,004–006,015 | 002,008 | 固定Git真实缺失/目录/symlink/坏YAML/权限/FIFO，三mode不合并 |
| G3 参数/快照/授权 | US3/1–4 | 007–012,015,027,028 | 003,004,006,007 | 双库20idempotent、批次失败无号、旧snapshot/Origin/retry保持 |
| G4 custom一次发布 | US4/1,3,5 | 016–020,025,026,028 | 005–008 | 真实process/接收端、原artifact/JUnit seal、取消/普通NS/独立清理 |
| G5 unknown/核对 | US4/2,4 | 018,021–024,027–030 | 005–008 | grant前后失联/丢回执、guard/slot/fullmanifest、manual证据/查询不足、双库 |

全部30FR/8SC/18AC覆盖；G1/G4与G5依已验收能力相接，不声称故事完全独立于共同安全基础。

## Complexity Tracking

无原则例外。只增加实际来源证据与一种custom发布variant；没有泛型source resolver、publisher registry、模板框架或第二executor。
