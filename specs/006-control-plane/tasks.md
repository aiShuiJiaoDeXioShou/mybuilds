# 任务：006 控制端与持久化队列

**Input**: `spec.md`、`plan.md`、`research.md`、`data-model.md`、`contracts/`、`quickstart.md`。
**Prerequisites**: 基线`2ab8991`（003/004已验收）；主代理已复核具体契约，需完成本任务集的只读analyze后才能实现。005未验收代码不参与。
**Tests**: 规范明确要求真实行为检查；安全/事务/锁/取消及双数据库不以mock/DryRun替代，先建立失败用例再实现。不按任务提交，整功能验收后主代理一次提交。
**Owners**: A=config001（store），B=engine003（server+SCM），C=preview001（config+CLI）；主代理独占依赖/README/产品专题/历史/validation/集成。分区使用独立worktree，同一文件只一个writer；任何共享API变更先协调契约再同步。

## Phase 1：准备与实际依赖

- [x] T001 主代理核对 `specs/006-control-plane/plan.md`、`contracts/go-api.md` 与分区基线/文件所有权，完成analyze零阻塞后同步给A/B/C，不借005代码或stub。
- [x] T002 主代理在 `go.mod`、`go.sum` 锁定GORM1.31.2、glebarez/sqlite1.11.0、postgres1.6.3、Viper1.21.0及modernc/sqlite1.55.0+libc1.74.1，记录SQLite3.53.3/纯Go实际兼容与官方WAL补丁依据到 `specs/006-control-plane/validation.md`。
- [x] T003 主代理在 `specs/006-control-plane/validation.md` 登记自有PG16.14夹具与独立schema/数据库写owner、临时Git/秘密文件/端口规则，明确真实PG suite不可用时保持待验证，不访问用户数据。

## Phase 2：当前真实消费者的公共声明

这层只准备A/B/C实际调用的具体模型与错误，不预建后续Node/Lease/Approval/Upload表或Agent/protocol包。

- [x] T004 C在 `internal/config/server.go`、`client.go`、`project.go` 定义冻结ServerConfig/LoadOptions/ClientConfig/ProjectSettings类型，复用既有YAML私有checkTree/checkType；主代理串行同步至A/B，保留流水线Document API不变。
- [x] T005 A在 `internal/store/models.go`、`errors.go` 定义具体Store模型、Actor/Page/过滤/安全DTO、typed BuildSnapshot/PreparedBuild/BatchResult及固定安全sentinel；采用非空GroupID外键RESTRICT、名称唯一、身份+key唯一、项目+非空编号唯一，主代理同步B/C。
- [x] T006 [P] B在 `internal/scm/git_test.go`、`internal/server/http_test.go` 准备自有真实仓库/httptest服务夹具与安全HTTP断言，只建立本区真实夹具；不加入fakeexecutor/repository/testhook。

## Phase 3：US1 初始化可独占控制端（P1）

**Goal**：严格配置、迁移、单控制端运行权、首次身份与真实HTTP生命周期。
**Independent Test**：两个真实进程竞争同库，正常退出后重取；替换自有SQLite锁文件或终止自有PGsession后不能写业务；坏配置无敏感输出。

- [x] T007 [P] [US1] C在 `internal/config/server_test.go` 写严格未知/重复/null/标量类型、地址/driver/DSN、路径/tilde、默认缺文件/显式缺文件、默认<文件<白名单env<CLI覆盖负例，包含Viper数字弱转换不得通过及敏感标记不回显。
- [x] T008 [P] [US1] A在 `internal/store/lock_test.go` 写SQLite/PG真实双进程独占、规范化alias与hardlink拒绝、正常Close重取、锁file替换/自有PGsession终止后no writes回归。
- [x] T009 [US1] C在 `internal/config/server.go` 实现LoadServer与严格节点校验后局部Viper typed合并，约束“database.driver仅sqlite/postgres”“concurrency正整数”、路径基于配置目录、0700数据目录/0600秘密文件；不启AutomaticEnv/全局Viper/热重载。
- [x] T010 [US1] A在 `internal/store/store.go`、`lock_unix.go`、`lock_other.go` 实现Open/Close/CheckLock：SQLite普通路径/拒URI内存与nlink>1、flock文件不unlink且identity复检；PG固定数据库范围advisory key同专用sql.Conn持锁与私有mutex写事务，失效不可pool重连继续写；Windows本机控制端明确未支持；PG DSN初始化也需有界：不隐式读取宿主PG环境/service/passfile，显式TLS材料限额普通文件读取并内存验证，自有FIFO真实子进程回归。
- [x] T011 [US1] A在 `internal/store/store_test.go`、`store.go` 实现并验证Migrate/default组/metadata、每连接WAL/foreign_keys=ON/busy_timeout=5000、重复迁移与重启保留，并在 `internal/store/query.go`、`query_test.go` 实现与验证真实Store.Status最小projects/queued/skipped计数（空库为0、查询不得执行构建）；SQLite实际版本必须含WAL补丁，双驱动固定安全错误映射不只依赖GORM FK sentinel。
- [x] T012 [US1] A在 `internal/store/token_test.go`、`token.go` 实现Bootstrap/Authenticate基础，随机高熵摘要、初始化sticky metadata；空bootstrap无操作，已有初始化不得复活；PG写使用持锁session。完整三角色管理验收在US4。
- [x] T013 [US1] B在 `internal/server/server.go`、`server_test.go` 实现具体New/Handler/ListenAndServe取消Shutdown及失锁拒绝新写、固定安全status：实际调用T011的Store.Status，并使用T012的Authenticate校验Bearer身份及基础角色规则（status三角色，管理仅admin，未知/撤销身份401），为US2路由建立不可绕过的鉴权入口；使用真实Store，无Agent/用户shell/调度器。
- [x] T014 [US1] C在 `internal/cli/server/serve.go`、`serve_test.go`、`root.go` 接入serve/migrate与signal context、显式覆盖/错误退出码；migrate与本机管理取得相同独占，serve在线不从本机命令绕锁写入。

**Checkpoint**：US1真实启动/重启/独占/失锁可独立验收；已有身份通过真实Authenticate与基础角色鉴权查询真实Store.Status计数，缺失/错误身份被拒；不依赖US4未实现的查询能力，无节点执行。

## Phase 4：US2 项目组与项目注册（P1）

**Goal**：本机/远程复用同一业务事务，default、改名、迁移、配置、历史保护正确。
**Independent Test**：同一组CRUD/竞态/历史/设置suite跑双库；真实客户端读取settings当前目录，file仍是仓库路径。

- [x] T015 [P] [US2] A在 `internal/store/project_test.go` 写两驱动default不可改名删除、名称唯一、空组FK竞态、默认归属/目标缺失/无操作迁移、历史/计数保持与审计UTC回归，包含“Name<=64字节”“NextNumber正int64”“default_node属于AllowedNodes”。
- [x] T016 [P] [US2] C在 `internal/config/project_test.go` 写settings严格结构、auto/repo、仓库相对file、旧pipeline.params归default且与builds互斥、命名默认参数、null/未知/未来profile/通知/triggers/retention拒绝；--settings/--file冲突不能覆盖顺序决定。
- [x] T017 [US2] C在 `internal/config/project.go` 实现ParseProjectSettings/ValidateProjectSettings，provided块整体替换，路径拒绝绝对/盘符/反斜杠/控制字符/任意..段/glob，不混入pipeline Document。
- [x] T018 [US2] A在 `internal/store/group.go`、`project.go` 实现冻结组/项目CRUD/Get/过滤/Set/Move/Delete与audit；外键RESTRICT防竞态，有历史或queued项目拒删；settings、节点授权与正初始号统一校验，策略变更递增PolicyVersion。
- [x] T019 [US2] B在 `internal/server/http.go`、`http_test.go` 接入admin组/项目路由及安全ProjectView、PATCH group/settings二选一；本机/HTTP同Store事务，不公开repo秘密、settings或参数值。
- [x] T020 [US2] C在 `internal/cli/server/manage.go`、`internal/cli/client/project.go`、`group.go` 及对应tests接入本机/remote组和项目命令；settings客户端当前目录读取后发送，file保存仓库相对路径，列表表格/JSON同DTO，不挂未来framework/hook空能力。

**Checkpoint**：项目身份/历史不随改组变化，当前组过滤正确；本机serve在线的写管理失败，远程同规则成功。

## Phase 5：US3 固定提交与多build原子排队（P1）

**Goal**：真实只读Git固定SHA，整批校验、条件冻结、命名参数、幂等及统一编号，无仓库命令副作用。
**Independent Test**：自有真实Git分支推进/不可达ref/坏配置；20相同key一批、20不同key无重复号；全skipped不占号，坏整批全回滚。

- [x] T021 [P] [US3] B在 `internal/scm/git_test.go` 写真实SHA1/SHA256精确branch/完整commitref/annotated tag拒绝、分支推进仍固定blob、symlink/tree/gitlink/父symlink/缺失/超限/坏path，空hooks/template/globalconfig与dirty源worktree保持、deadline/cancel/output cap无残留回归。
- [x] T022 [P] [US3] A在 `internal/store/enqueue_test.go` 写实际双库20相同/不同key、同身份/跨身份key、原请求冲突、queued+skipped/全skipped、回滚与int64溢出、身份撤销/策略变化条件更新、步骤唯一与预算重启保持用例。
- [x] T023 [US3] B在 `internal/scm/git.go` 实现ReadPipeline：每请求自有0700临时bare/空hooks，禁global/systemconfig/helper/askpass/ext/unsafeprotocol/submodules/auto maintenance；精确ls-remote行仅判40/64format、matching init/fetch后pin实际SHA；ref对象类型commit+祖先检查，literal树mode100644/100755普通blob与1MiB限额，无checkout/filter/textconv。
- [x] T024 [US3] B在 `internal/scm/credentials.go`、`credentials_test.go` 实现仅SSH两键GIT_SSH_KEY_FILE/GIT_SSH_KNOWN_HOSTS_FILE受限0600普通无symlink材料与自有副本、固定受限ssh派生；实际自有sshd成功/错key/错host验证，无宿主agent/default identity；匿名HTTPS验证TLS，HTTP仅loopback，URL密码与认证HTTPS未支持明确失败，不接任意sshcommand。
- [x] T025 [US3] B在 `internal/server/trigger_test.go` 写选择/共享+命名参数/required+choices/未选scope、模板pending不改变build.when、branch/params条件+manual_ignored、所有selected upload权限先于when、runner/default_node、完整post模板及未知ios_signing拒绝的整批负例，无marker。
- [x] T026 [US3] B在 `internal/server/trigger.go` 实现Trigger预准备：身份/key原请求摘要先查→授权分支→一次SCM→config Parse/Select/ResolveParams/纯Preview全校验→独立build.when判断→typed BuildSnapshot/步骤进度；不调用Run、不解析节点秘密或将pending当跳过。请求“最多64个所选build”“参数最多128项/值<=4096字节”“key<=128字节”有界。
- [x] T027 [US3] A在 `internal/store/enqueue.go` 实现FindRequest/Enqueue短事务：复检actor/ProjectVersion/key，计数器/批次/build/snapshot/steps/幂等一起提交；BuildSnapshot由A编码，项目+非空Number唯一，skipped无编号或节点，无budget为NULL、post默认2m，step phase ordinary/success/failure/always、Index从1开始；禁止Git/网络/shell入事务。
- [x] T028 [US3] B在 `internal/server/http.go`、`trigger_test.go` 接入Idempotency-Key触发路由，首201/重放200/不同内容409与安全batch响应；相同key不再读取Git，发布排队不授上传权。
- [x] T029 [US3] C在 `internal/cli/client/trigger.go`、`trigger_test.go` 接入--build/--all、共享--param key=value与命名--param build:key=value、version/channel重复冲突、--allow-upload、一次crypto/rand幂等key与显式--idempotency-key复用；真实CLI触发只排队。
- [x] T030 [US3] 主代理按 `specs/006-control-plane/quickstart.md`、`validation.md` 跑实际双库20并发/Git分支推进/失败整批/全skipped/固定快照与marker无副作用，记录真实SHA/摘要/号与提交基线；不将研究原型冒充功能验收。

**Checkpoint**：SC-003/004可独立验证，queued有真实持久化证据，仍不执行任何流水线或分配节点。

## Phase 6：US4 身份边界与队列证据查询（P1）

**Goal**：三角色与撤销/一次性初始化；分页安全详情、预算/ordinary/post初始化进度；远程配置隔离本地命令。
**Independent Test**：真实HTTP/CLI三角色与唯一敏感标记、分页、重启；损坏client.yml仍能本地init/run/doctor/help/version。

- [x] T031 [P] [US4] A在 `internal/store/token_test.go`、`query_test.go` 写随机token摘要/创建仅一次/撤销与bootstrap sticky、三角色、BuildSnapshot派生参数键/条件、limit1..200/offset0..1000000/CreatedAt+ID降序、project+group交集与迁移历史查询双库用例。
- [x] T032 [P] [US4] C在 `internal/config/client_test.go`、`internal/cli/client/remote_test.go` 写0600 literal/envref客户端凭据、空/弱token、坏地址/HTTP非loopback/TLS/timeout、默认与显式文件缺失、敏感标记；损坏client配置/缺token/remote --timeout不得影响本地init/run/doctor/help/version或YAML预算。
- [x] T033 [US4] A在 `internal/store/token.go`、`query.go` 实现Create/List/Revoke、安全ListBuilds/GetBuild，复用T011已实现的Store.Status而不重复定义，token摘要不出公开JSON；BuildView只安全字段，参数键排序/来源摘要/固定预算与步骤状态不刷新，所有管理写统一actor与锁检查。
- [x] T034 [US4] B在 `internal/server/http_test.go`、`http.go` 在T013已有Authenticate/基础角色鉴权/status之上扩展token/build路由与完整权限验收、固定安全错误，admin全部、trigger仅status/非发布触发、approver只读build证据；请求1MiB、未知/重复/null/多对象/错误查询严格拒绝，公共DTO/普通日志不显示秘密/配置正文/参数值。
- [x] T035 [US4] C在 `internal/config/client.go`、`internal/cli/client/remote.go` 实现远程限定LoadClient/实际HTTP调用、默认30s与root --timeout，仅loopbackHTTP/验证HTTPS、禁止redirect泄token；本地入口无全局加载，错误不打印raw网络/响应/URL/token。
- [x] T036 [US4] C在 `internal/cli/client/build.go`、`status.go`、`root.go`、`internal/cli/server/manage.go` 及对应tests接入安全列表/详情/status/本机token，表格/JSON同视图、limit20最大200、offset非负；所有真实命令保留help/version，unsupported未来命令不挂stub。
- [x] T037 [US4] 主代理按 `specs/006-control-plane/quickstart.md`、`validation.md` 以真实二进制/HTTP跑三角色/撤销/bootstrap/本机独占/远程完整group→project→trigger→query路径，扫描自有唯一敏感标记无泄露；CreateToken唯一授权返回除外。

**Checkpoint**：SC-005闭合，独立身份与可查询队列证据可真实演示；尚无Agent/发布/iOS签名执行声明。

## Phase 7：跨故事验收与收敛

- [x] T038 主代理串行集成A/B/C限定差异，检查 `go.mod`、共享CLI/config类型与既有 `internal/pipeline`、`internal/mobile` 行为；处理冲突后只对受影响行为复验，不复制005未验收schema。
- [x] T039 主代理运行 `go test ./...`、`go vet ./...`、相关race、CGO=0 Linux/Windows客户端跨编译与真实双库/跨进程锁/失锁/Git/API/CLI必要检查，完整记录到 `specs/006-control-plane/validation.md`；PG不可用/任何失败不勾验收。
- [x] T040 主代理更新 `README.md`、`docs/IMPLEMENTATION_HISTORY.md` 与受影响产品专题，说明已实现控制面仅排队、未有Agent/iOS签名/发布；005/009及005+007真实签名联验门保持。
- [x] T041 主代理执行speckit-converge对照 `specs/006-control-plane/spec.md`、`tasks.md`、`validation.md` 闭合全部FR/SC缺口，必要时追加任务再implement/converge，不因预算/环境宣称完成。
- [x] T042 主代理按git-commit-message技能检查工作区/暂存差异，仅暂存006相关规范与实现，全部验收通过后整功能一次本地提交并在 `docs/IMPLEMENTATION_HISTORY.md` 记录哈希/信息，不自动push。

## Dependencies 与并行安排

- 主链：T001→T002/T003→T004→T005→US1基础→US2项目→US3入队→US4完整证据→T038–T042。
- 公共类型：C的T004先供A/B；A的T005再供B/C；声明不等于实现，B/C待真实Store接口后集成，不stub。类型同步由主代理串行执行。
- A：T008/010/011/012→T015/018→T022/027→T031/033；B：T006→T013→T019，以及独立真实GitT021/023/024→T025/026/028；C：T007/009/014→T016/017/020→T029→T032/035/036。每一包文件仅其owner写，tests先于实现；具体CLI集成依赖真实A/B交付。
- T027依赖T026准备schema契约（已冻结）而不依赖B实现完成，A可用typed实际输入先写事务；T026完整集成依赖T018/023/024/027真实实现。T030/T037由主代理用已集成代码验收。
- US1运行时依赖：A在T011实际实现Store.Status，在T012实现Bootstrap/Authenticate；B的T013必须等待T011/T012真实交付，建立status及基础角色鉴权，T019的US2管理路由复用它。T033只重用Status，T034扩展完整路由/权限；不能把这些US1基础能力拖到US4或挂未鉴权管理API。

## Parallel Examples

- US1：A独占锁行为T008与C严格配置T007可并行；B的真实Server依赖A在T011/T012实际交付的Status/Authenticate，不使用后期US4接口或stub。
- US2：A组/归属竞态T015与Csettings校验T016可并行；B HTTP和C CLI随后用同一真实Store。
- US3：B真实GitT021与A事务T022可并行；C可准备参数/幂等CLI行为检查，完整接线待B。
- US4：A安全查询/身份T031与C连接隔离T032可并行；B真实鉴权HTTP与C实际HTTP客户端通过共享契约接入。

## Implementation Strategy

US1是最小可运行控制端增量，随后US2/US3/US4各有独立真实验收；整个006四故事与6SC全部通过才提交。单个故事完成不能替代完整功能验收。003/004前置调整只解除非实际依赖；005真实材料/009/全MVP门不变化。

## Requirement Coverage

| 要求 | 核心任务 |
|---|---|
| FR001/018 | T007–009、T032/034/035 |
| FR002/003 | T008/010/011、T022/027/039 |
| FR004–007 | T015–020；T025/026 |
| FR008/009 | T012、T025/026、T031/033/034/037 |
| FR010/011/012 | T021/023/024/025/026/029/030 |
| FR013/014/015 | T022/026/027/028/030/031/033 |
| FR016/017 | T013/014/019/020/029/034–037 |
| FR019/020 | T001/026/038–042 |
| SC001/002 | T008/011/015/022/031/039 |
| SC003/004 | T021–030 |
| SC005 | T014/020/029/032/034–037 |
| SC006 | T038–042 |

before/after_tasks hooks={}。任务格式：42个顺序ID，故事阶段均带US标签，路径明确；[P]只表示不同文件且不依赖尚未完成任务的检查机会，不允许同一文件多writer。
