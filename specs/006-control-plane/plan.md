# 实施计划：006 控制端与持久化队列

**Branch**: `006-control-plane` | **Date**: 2026-10-04 | **Spec**: [spec.md](spec.md)

**Input**: 本功能规范20项FR、6项SC与4个P1用户故事。基线为已验收003/004的`2ab8991`；005工作树与未验收代码不作为依赖。

## Summary

控制端只读取得授权分支的固定提交，严格校验所选流水线、命名参数及权限，事务保存配置/条件/预算/ordinary与post进度并排队。默认SQLite、可选PostgreSQL，通过具体GORM Store统一访问。唯一控制端运行权覆盖serve、migrate与本机管理写入；HTTP和客户端使用同一组业务规则。当前没有Agent，任务只能queued或skipped，不执行用户shell、取得租约或授予上传权限。

## Technical Context

- **Language/Version**：Go1.25，实际工具Go1.25.4；单模块、macOS/Linux控制端；客户端Windows/Linux/macOS可编译。
- **Primary Dependencies**：既有Cobra1.10.2、YAML3.0.5、doublestar4.10.2；新增Viper1.21.0、GORM1.31.2、glebarez/sqlite1.11.0、postgres driver1.6.3。明确锁定间接modernc.org/sqlite1.55.0及其libc1.74.1，实际SQLite3.53.3；不能沿用驱动默认的3.41.2。依据和真实兼容原型见research。
- **Storage**：GORM AutoMigrate；SQLite普通本地数据库路径、WAL、每连接foreign_keys=ON/busy_timeout=5000；PG独立session advisory lock。0700数据目录、0600秘密文件与SQLite数据库，快照不经公开JSON序列化。
- **Testing**：标准testing、真实Git/HTTP/CLI与两个真实锁竞争进程；同一suite跑实际SQLite和PG16.14，20个并发相同/不同触发；race/vet、纯Go客户端跨编译。
- **Target Platform / Project Type**：两个现有CLI与net/http控制面；不创建Agent、协议空包、调度器或发布执行器。
- **Performance Goals**：只验证有界请求、短事务和20并发编号/幂等；不虚构吞吐指标。
- **Constraints**：Git不得在数据库事务中执行；不运行仓库hook/filter/shell，不改用户工作树；秘密、普通参数值、配置正文均不出公共详情或错误；锁无法确认即停止写业务。
- **Scale/Scope**：一个控制端，项目组/项目/三角色/固定SHA/批量队列；分页默认20最大200；Git/配置1MiB上限；HTTP1MiB；本阶段只有repo/auto仓库来源。

## Constitution Check

| 原则 | 设计前门禁 | 设计后门禁 |
|---|---|---|
| I规范驱动 | specify与checklist完成，真实验收未宣称通过 | plan交主代理冻结，再tasks/analyze；通过后才implement/converge |
| II单模块与节点 | 控制端与既有pipeline边界成立 | server不执行Run；未来Agent复用现有引擎；没有005代码依赖 |
| III最小实现 | 技术专题要求Viper/GORM及双驱动 | 具体Store/Server与函数，无repository接口、testhook、消息队列、通用RPC或第二执行器 |
| IV输入与执行 | 需要统一输入、身份、锁与事务边界 | 严格解析、秘密引用、写锁检查、幂等原子事务；不发租约/上传授权 |
| V中文与实际验收 | 需要真实双库与Git/API/CLI证据 | 真实PG夹具已可用，纯方言原型不算双库验收；README/历史由主代理同步 |

无原则例外。hooks={}，before/after_plan无需执行钩子。技术未知经research解决，具体契约由主代理审核冻结；主代理随后执行analyze。

## Project Structure

### Documentation

```text
specs/006-control-plane/
  spec.md                 # 主代理
  checklists/requirements.md # 主代理
  plan.md research.md data-model.md quickstart.md
  contracts/go-api.md contracts/http.md contracts/config-cli.md
  tasks.md
  validation.md           # 实施阶段主代理维护真实证据
```

### Source Code

```text
internal/store/             # A：具体Store、模型、独占锁、CRUD/身份/原子入队/查询与实际双库测试
internal/server/            # B：具体Server、HTTP、队列准备与安全公开视图
internal/scm/               # B：固定SHA只读Git函数及真实仓库负例
internal/config/{server,client,project}.go # C：严格配置与Viper；相应tests
internal/cli/client/{remote,project,group,trigger,build,status}.go # C
internal/cli/server/{serve,manage}.go # C
internal/cli/{client,server}/root.go # C唯一写入者，保留本地入口
```

文件可因实际实现小拆同包，不能借机预建后续包。原pipeline/Run/Preview和mobile/Android无功能修改；复用config.Parse/Validate/Select/ResolveParams、pipeline.Preview与process.Run。pipeline若确需导出已有纯校验入口，先提具体差异由主代理唯一修改，不新建通用模板框架。

### Ownership 与交接

- 规范分区：本代理独占上述plan/research/data-model/contracts/quickstart/tasks；不改spec/checklist/代码/提交。
- 实施A：独占internal/store及测试。配置的DatabaseConfig/ProjectSettings与store对外类型先按go-api由主代理协调一次真实声明，再同步至其他worktree；不能以stub冒充依赖。
- 实施B：独占internal/server、internal/scm及测试，不修改A/C文件。
- 实施C：独占新配置及客户端/服务端CLI，保留本地init/run/doctor/help/version，不修改pipeline流水线schema。
- 主代理：go.mod/go.sum唯一依赖writer，README、产品专题、历史、validation、共享接入与最终验收/提交；其他分区只提补丁建议。每个分区另建从相同已验收基线的独立worktree，公共声明/真实依赖串行同步，禁止同文件并发改写。

## 实施设计

### 独占与事务

Store.Open先规范化数据库身份并取锁，再允许迁移/业务。SQLite拒绝内存/URI数据库及已有nlink>1文件，消除hardlink对应不同WAL/锁路径；邻接flock锁文件不删除，保持文件身份，写前和提交前复检。PG使用同一数据库范围的固定advisory key，与DSN文本无关，专用sql.Conn持锁；所有PG写事务在这条仍存活、未重连的连接上由私有mutex串行执行，读连接池独立。连接失效不可借pool重连继续写；锁检测失败是不可恢复运行权丢失，HTTP停止新写，serve关闭。真实测试替换自有SQLite锁文件/终止自有PG锁session，随后不得写入。

事务校验身份未撤销、项目策略版本未变化；分配计数器、批次、全部build/step快照及幂等记录一起提交，任何唯一约束/溢出/写入失败全部回滚。组删除用外键RESTRICT，改组保持ID与历史；当前组迁移不产生虚假审计。

### US1独立运行的最小计数与鉴权

A在T011随迁移提供真实Store.Status（projects/queued/skipped计数），T012提供Bootstrap/Authenticate；空库计数为0，无Agent不会伪造运行结果。B的T013依赖两者真实交付，status必须实际调用Store.Status、Bearer认证及基础角色鉴权；后续US2管理路由从首次接入就限制admin，不能等US4才补鉴权。US4的T033复用Status，只扩展安全build查询与token管理，T034在既有基础上扩展完整路由/权限验收，不重复定义或stub。这样US1启动/重启/独占/失锁/status可独立验收。

### 固定SHA与条件

Server.Trigger先鉴权并以原请求规范化摘要查询幂等键，已有相同键不读取Git；之后读取项目、检查授权分支，SCM每请求在DataDir/scm下建立独立0700临时bare/空hooks/0600明确凭据副本，不共享可变缓存refs；一次获取精确branch，固定该HEAD或可达完整ref。仅读取固定tree中普通blob，禁止hook、filter、checkout和工作树改写。SHA1/SHA256先精确ls-remote探测format再init/fetch，实际fetch结果才是快照；完整tagOID须按对象类型拒绝。认证只支持匿名HTTPS/loopback HTTP/本地Git及明确SSH key+known_hosts pair，固定派生受限ssh，不退到宿主身份；HTTP凭据当前明确未支持。

按Select确定build集合；合并默认/项目命名覆盖/请求共享/请求命名覆盖后完整参数校验。每个build用已冻结分支/SHA/项目/name事实调用纯Preview完成字段模板校验；已知模板缺少node/workspace/number/id可pending，不表示build.when待定。build.when仅branches/params/手动changes豁免，明确判断后queued/skipped；steps与post的条件/模板pending分别保存，节点执行时再解析。所选build含upload无论when结果均要求admin+allow_upload，先鉴权再判条件；入队不授上传权。

具体store.BuildSnapshot由A定义、B准备、A事务编码；snapshot保存已选规范化定义、未渲染的模板/秘密引用、最终普通参数、来源/内容摘要和固定条件事实；编号由事务分配而不预造。ordinary/post逐项保存进度，当前为pending或条件skipped；预算是初始化值，未执行不消耗。仅有build.when false才使build跳过；不能把steps全部模板pending当skipped。

### 配置、HTTP、CLI

文件YAML沿用严格Node检查，Viper仅在局部实例合并已验证值；默认<文件<明确白名单env<明确CLI覆盖，不启用AutomaticEnv/全局实例/热重载。client token来自0600受限文件中的literal/完整环境引用或明确环境覆盖，runtime字段不可JSON。secrets_file只定位受限文件，不能导入完整环境。

HTTP未知/重复字段、多对象、超限、非法查询均固定安全错误；公开DTO排除repo敏感URL、配置正文、普通参数值、token摘要和秘密。admin全部，trigger仅status/非发布触发，approver只读脱敏build证据，实际审批未接入。绑定方案/自动触发/通知/retention/报告执行未实现时明确拒绝，不静默丢弃。

CLI只在remote命令加载client.yml/token。根--timeout为普通HTTP预算，不能启动本地命令的连接配置加载，也不能改变pipeline YAML build/post预算；help/version和本地doctor保持既有行为。migrate/本机管理取得与serve相同独占；serve在线时本机写命令拒绝，应使用远程管理。

## 实施与验收顺序

1. 主代理审核plan/contracts、真实依赖版本与分区，再生成tasks/analyze。
2. A/C同步真实类型后，各分区先编写对应行为检查；A先实际交付T011 Status和T012 Authenticate供US1独立验收，B的T013随后接入真实计数/鉴权并用真实SCM准备和真实Store组装，C用真实HTTP/Store接入，不写future hook/stub。
3. 同一suite跑实际双库；20并发、跨进程锁、失锁、组竞态、真实Git/API/CLI及安全负例全部验收。
4. 主代理串行集成，共享文件复验、全量/race/vet/跨编译，converge缺口闭合，README明确当前只排队；整功能一次本地提交。
5. 005/009及005+007真实Apple签名门、全MVP真实验证门保持；006拒绝基线未知ios_signing，后续005集成时复验快照往返。

## Complexity Tracking

无原则例外。数据库原型与实际有限Git消费者驱动本功能依赖，不增加抽象repository或插件框架。
