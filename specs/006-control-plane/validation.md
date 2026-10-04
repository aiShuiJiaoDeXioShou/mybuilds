# 006 验证记录

## 规划与实施前门禁（2026-10-04）

- 基线 `2ab8991`，003/004已验收；不使用005未验收schema或实现。
- Spec Kit specify/plan/tasks已执行。最终只读analyze：20FR、6SC均有任务覆盖；42任务唯一顺序，无未映射项、歧义、重复或阻塞。此前唯一HIGH依赖冲突已修正：T011真实Status、T012身份与T013鉴权在US1交付，US4复用。
- 最终分析tasks SHA256：`9af1d5d8f3383d6e76de8e2344fb12b89a1e8070bced31a44ca340da460ccd4a`；分析未修改文档。原则2.1.0五项通过，无例外。
- implement前置脚本通过；requirements checklist 16/16、未选0。extensions hooks={}；Go/Git ignore覆盖既有实际项目，无需新增工具ignore。
- A(config001)唯一写store；B(engine003)唯一写server/SCM；C(preview001)唯一写config/CLI；root唯一写依赖、共享集成与文档。每个分区独立worktree、实际消费者，无stub、repository接口或第二执行器。

## 实际依赖研究（不是功能验收）

锁定GORM1.31.2、glebarez/sqlite1.11.0、postgres1.6.3、Viper1.21.0与间接modernc.org/sqlite1.55.0/libc1.74.1，Go1.25.4可用。真实原型SQLite为3.53.3，包含[官方WAL-reset修补](https://sqlite.org/wal.html#walreset)；不可回退驱动默认3.41.2。证据位于 `/tmp/mybuilds-006-patched-smbkdgu7/probe_test.go`：4连接WAL/FK/busy、回滚/约束、8×50计数并发、integrity/race/跨编译均通过。Viper会弱转换数字字符串，必须先严格YAML节点校验。实际源代码suite尚待运行。

## 自有真实验收资源

- PG16.14 socket-only临时夹具：`/tmp/mybuilds-mvp.zKtK0e/postgres006-ldh7rr0w`。0700目录、自有trust数据库，无用户数据库或公网监听；root持有启动/关闭责任。
- 测试DSN通过 `MYBUILDS_TEST_POSTGRES_DSN` 提供：自有socket、port55436、userlinghe、dbmybuilds006。A是数据库写测试owner；每suite创建自有唯一schema/库，其他分区不得drop同资源；root集成验收在A结束后串行运行。
- advisory原型5/5通过（首次锁、第二拒绝、终止自有session、旧连接失败、新session重取），`advisory-prototype.json`；不是功能锁测试。
- B自行建立临时Git仓库/sshd/密钥/known_hosts，仅绑定loopback、自有端口，清理仅其自有进程与目录。材料0700/0600，不读取宿主agent、用户默认密钥或未知仓库。
- 006 Git原型证据 `/tmp/mybuilds-scm006-evidence.20jj457f/evidence/recommendation.md`：26Git、4SSH、3有界进程检查；源代码功能证据待实施。

## 功能验收状态

最终源码实现及必要检查通过，完整证据见末尾“最终集成验收”；下面首批交付与红测保留为实施过程记录，不代表最终状态。005真实Apple签名、009与全MVP门保持。

## A首批Store / C配置实际交付（尚非完整006验收）

A在store006报告真实双库suite 1.480s通过：SQLite3.53.3四连接WAL/FK1/busy5000、PG16.14、两个真实测试进程竞争/关闭重取、路径别名/hardlink、SQLite锁替换/自有PGsession终止后不可写、迁移/重启/Status、Bootstrap/Auth/revoke sticky及三角色。实际文件11个已串行同步给root/B/C；Project/Enqueue/Build查询仍未交付。C首批严格LoadServer/ProjectSettings配置suite通过；实际公共5文件同步给root/A/B；LoadClient及CLI仍待实现。主代理完整集成检查尚待最后交付后进行。

## 实施中发现并修复的PG输入边界（FR018，尚待修复验证）

C/A核实官方pgx5.10.0 ParseConfig内部会合并PG环境/defaults，并同步读取service/passfile/TLS文件；连接context无法限制前置ReadFile。A实际红测：自有FIFO分别通过DSN passfile和PGPASSFILE进入，Store.Open context100ms仍卡住，父子进程测试在2s截止终止，4.422s失败。此为实际源代码缺口，完整006验收保持未通过，T010重新置未完成；A唯一owner在既有Open入口修复。不能以研究、原有普通DSN suite或编译通过掩盖此问题。

### PG边界修复与其它实施缺口已闭合

A实际修复Open前DSN/TLS入口：8个DSN/URL/PG-env FIFO真实子进程有界返回ErrInvalid；自有同次TLS字节删除原文件后真实双向握手、错hostname/未知CA拒绝、require+显式CA语义。无全局env变更或第二parser消费；具体pgx.ConnConfig经stdlib.OpenDB交GORM，停止原DSN二次解析。真实双库race14.966s、最终CGO0双库1.565s、vet/跨编译通过；19个Go文件已集成。依据[官方pgx源码](https://github.com/jackc/pgx/blob/v5.10.0/pgconn/config.go)，限制同步到实际契约。T010完成。

B另用实际HTTP红绿回归修复显式build_number_start=0被默认为1：缺省仍1，显式0/-1安全400且不创建项目或消耗号；完整server race7.345s/vet通过，2文件已集成。C修复独立DataDir与SQLite父目录不同的实际serve消费者：创建并确认0700、弱权限固定拒绝、不修改用户目录权限；最终配置/CLI tests/race/vet/跨编译通过。

### 首轮主代理实际二进制SQLite验收（完整最终门仍待运行）

`/tmp/mybuilds-mvp.zKtK0e/control006-acceptance.py` 对编译后的实际两CLI运行37项检查PASS，证据 `/tmp/mybuilds-mvp.zKtK0e/mybuilds-control006-accept-refy1q5o/sqlite-391fea4b/assertions.json`：真实serve/bootstrap、独占migrate拒绝、remote group/project/settings、固定SHA/来源摘要/预算/steps+post、混合和全skipped、20同key一批/20不同key号3–22、严格JSON/query/超限、三角色/撤销、迁移历史按当前组、重启不重置、真实CLI触发。全程脚本marker不存在，普通stdout/stderr/API/日志敏感标记扫描无泄露。此轮在最终PG入口及少数新增负例源代码集成前运行，最终源码双库/二进制与全量门另行记录。

### 独立真实Linux环境

自有Lima2.0.2 VZ plain Ubuntu24.04 ARM64已启动，官方minimal镜像229048320字节及SHA256核对通过；无宿主目录挂载/agent转发。实际uname为Linux aarch64。接受基线2ab8991的pipeline与process测试在VM真实运行PASS，含进程组取消/无关PID/TERM-ignore/短预算/产物及special-file/日志；唯一缺Git的case安装Git2.43后实际补验PASS。证据 `/tmp/mybuilds-mvp.zKtK0e/lima006/{image-verification,baseline-verification}.json`。006源代码Linux运行门待最终集成后单独执行。Linux ARM不作为官方Android工具能力证明，005真实Apple资源仍待用户提供。

## 最终集成验收（2026-10-04）

- 同一最终源码的 `go test -count=1 ./...`、受影响六包 race、`go vet ./...`、`go mod verify` 与 diff 检查通过。Store 在真实 PG16.14 与 SQLite3.53.3 运行同一 suite：全量1.700s、race16.489s；包含事务内失锁回滚、独立进程竞争、20并发、初始化sticky、分页/迁移历史、秘密边界与 PG FIFO/TLS 负例。测试日志在 `/tmp/mybuilds-mvp.zKtK0e/control006-{full-test,race}.log`。
- 重新编译最终实际 client/server，在 SQLite 和独立 PG 数据库 `accept006_final` 各跑37项 CLI/API 检查，均 PASS；证据分别 `mybuilds-control006-accept-refy1q5o/sqlite-19ad1114/assertions.json`、`postgres-e3704ad8/assertions.json`（前缀 `/tmp/mybuilds-mvp.zKtK0e/`）。两真实服务正常 SIGTERM 退出，无残留。
- 验收固定仓库 SHA `9377871b36403de0992474ec6918b1e18fda4ff9`（另一个提交 `f4098ac865b57c0885302b2fb3345d34ea778940`），普通配置 blob SHA256 `044c1d86f4610d0b4a9ef2ba3e3b5e565d2302fce8daa6f812bdf2c6e2e990cb`。混合结果 queued号1/skipped无号，20同key一批、20不同key号3–22；固定4m ordinary/7s post与全部pending步骤重启后不变；同key换ref409；非法整批400。分支推进/固定读取另由真实SCM suite覆盖。
- 真实客户端按相对settings注册/创建组/迁移/触发/查询，三角色与撤销、本机在线独占、严格JSON/分页/1MiB超限检查通过；ordinary API/日志/stdout/stderr自有敏感标记扫描零泄露，受授权token首次返回在私有证据中隐藏。全程仓库shell marker不存在，控制端不执行构建。
- CGO=0 Linux ARM64 client/server/Store test 及 Windows AMD64 client 编译通过。自有 Ubuntu24.04 Linux ARM64 VM实际运行最终Store SQLite全部检查（PG在macOS真实资源验证），包括 flock/取消/并发/SQLite3.53.3与TLS/FIFO；最终Linux两二进制再跑同样37项 CLI/API PASS，guest证据 `/tmp/mybuilds-mvp.zKtK0e/mybuilds-control006-accept-refy1q5o/sqlite-fcd0a6c0/assertions.json`。无宿主挂载或密钥继承，Linux ARM不能作为Android官方工具支持证明。
- README实际命令/目录/边界同步，Markdown引用38项、六段Bash语法与差异检查通过；产品配置与身份初始化说明同步。005源代码/材料未混入006，005+007真实签名联验仍为全MVP门。

## 最终收敛与交付

Spec Kit converge在最终源码与必要证据上核对20FR、6SC、13验收场景、42任务、9项规划决策与5项原则：missing/partial/contradicts/unrequested均0，无严重项。converge自身未修改tasks，前后SHA256 `3406019fcfb3e2278798bf69a4c3846874f8286b41ecfea131dac1204a6cf4ab` 相同；hooks={}。结束后由交付阶段标记T041/T042，整功能提交记录实际哈希见实施历史。无新增依赖例外或未验收执行声明。
