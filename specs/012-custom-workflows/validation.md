# 012 规范与计划记录

当前仅Spec Kit specify/plan完成；30FR/8SC/18AC与质量16/16，未tasks/analyze/implement/converge、无源码/提交。独立worktree从已验收007的85b46bf开始，未借入未来前置代码。

实际spec/checklist/plan模板resolver、setup-plan、prerequisite、hooks={}与原则2.1.0两阶段复核通过。九份spec/checklist/Plan逐SHA复制根，清单SHAcf535075ade4291c5e99e564f8140f6764828754cba3feaae8bc8a55caeee328；覆盖映射SHA09372dd76cc4510841f039d93e1cd2b8c6613007fd8ba628eb19b0df10c1bdc3。根已阅读具体Go契约/Plan，当前selector仍008。

选定原Trigger唯一来源展开、实际ReadPipeline三种FileMode/精确Missing、启动加载单build方案、optional来源证据与008原样retry。custom只接010原Publish/intent/guard/receipt/query和唯一process.Run；manual应用绑定明确归属依据审计，不冒称远端doctor已验证。用户query脚本只读属于信任约定，系统不能证明无写或无内部重试，缺证据仍unknown保锁。

正式实施须前置真实验收再冻结最终具体类型；四种真实原生/Flutter工程、来源负例、双库入队/retry、真实custom接收端与未知故障/查询门均未执行。014/020接入后的保护联验计入整MVP，不反向形成当前功能的提交循环。

## tasks / analyze 完成（后续规划阶段）

46项任务覆盖30FR/8SC/18AC，只读分析零阻塞/原则冲突/缺口。根逐SHA接收任务471b10c809fd92c6c25d7971f03738fcfd7c71227823a6d215257eed64684117；批准窄CustomQueryContext仅由原intent/snapshot派生并给当前原node/session私有管理task，原SHA/query参数/声明env引用/必要四Facts/精确产物和授权摘要；query metadata不含artifact.path、不授下载/旧lease/新grant，无旧journal依赖、不走Run。workspace/node为当前查询环境，不能假原工作区。真实前置、商店和receiver门未完成，不等于源码交付。

## 2026-10-05 代码交付边界修正

用户明确要求先完成所有模块可执行代码与必要自动验证，真实签名/商店上传由用户最终统一人工验收。当前独立源码WT基线2602094；原30FR/8SC/18AC和16项质量条件保持。005/009/010/011具体接口只能真实冻结后消费，不造stub；材料缺失不阻独立实现，不把未执行人工门记PASS。协议与publish表由根唯一writer，012只扩custom实际variant，不增加第二发布记录或执行器。

## 2026-10-05 实际012代码交接（人工门独立）

- 独立源码WT基线2602094，随后只同步Root冻结真实005/009/010/011接口；captured baseline共447项，最终69文件delta冻结，源码清单 `/tmp/mybuilds-mvp.zKtK0e/reuse012-final/source-manifest.json` SHA `2c2c170ae5937392b37b8901688a6244d8e3e6e6b0b72c541d78d9a62a6f8e96`，不是对260的整包Git差异。
- 实际selector012/prereq require-spec/tasks/include-tasks exit0，hooks={}；原30FR/8SC/18AC/46任务映射不改需求；唯一原Run/process/Publish与原发布表，无新增Go依赖、stub或第二执行器。
- 目标race覆盖config2.202s、双库Store8.463s、Server5.371s、CLI2.937s、distribute6.199s、Agent19.078s均exit0；该轮pipeline/scm没有匹配测试，不作行为通过声明。
- 后续真正pipeline/scm目标race各2.302s/3.813s，protocol2.038s、distribute5.280s通过；新增无query测试第一次因夹具显式空列表被Validate正确拒绝，保留原日志；改为真正省略后双库CustomQuery组race2.820s exit0，没有放宽输入或生产预算。
- 实际Agent Serve→原Git固定SHA→Run生成包→中央快照→custom真实POST一次→always改原包→terminal→metadata-only GET一次通过；独立query Close未知真实门7.001s保Ref=nil journal、阻新session，不借旧PID或自动重跑。
- 原发布证据私有文件替换门及真实/tmp nominal已复验，DataDir与directory均EvalSymlinks只用于Rel边界，原OpenRoot/出生identity不变；Store1.410s、Agent7.610s exit0。
- `go test ./... -run '^$'`全包编译、十相关包 `go vet`、`git diff --check`均exit0；三实际native二进制build/help/version各exit0，未知命令各exit1，custom示例dry-run exit0，未执行实际上传或读取秘密。
- 真SQLite/自有PG库 `mybuilds012_custom_tests` 复用同套Store授权/Origin/retry/充分query/审计，既有Google/Apple digest golden保持；共享Apple充分GET提升与SourcePath/fixedcode补丁已由Root先接收。
- 源码已冻结；Root最终三方合并保014不可变审批历史Ref/当前授权与015固定SHA ReadChanges，不伪造当前WT尚无的批准消费者；全量最终检查、统一实例案例、双真实节点故障和四平台签名/用户商店发布不在本分区宣PASS。
- 只读代码收敛检查原30FR/8SC/18AC和46任务，未发现需要新增框架或代码占位的新缺口；原T013/T014/T038–T044的真实四平台及整应用联验仍明确pending，按用户要求不阻本次代码集成，Root完成最终正式converge和整功能提交，不按任务提交。

## Root实际合并与模块门

逐69文件SHA及captured447 baseline核验，8个共享冲突已串行保留既有PublishIDs、Apple证据和私有路径身份。九相关包普通目标通过；初轮并发包共用PG数据库的排它advisory锁导致四fixture open control_locked，保留该轮失败，单Store串行复验8.180s通过，不改生产控制锁。Store/Agent必要race6.531s/17.770s、十相关包vet通过。最终全项目使用-p1避免自有同库fixture竞争。按用户代码交付范围，Spec Kit收敛核30FR/8SC/18AC/5原则无剩余代码缺口；真正四平台与最终跨模块案例仍分别记录。
