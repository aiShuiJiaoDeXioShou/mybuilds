# 007 验证与交接记录

## 当前阶段（2026-10-04）

基线 `8e1397e`，已集成共享process缺陷提交 `58bc9c8`；007-node-agents 根分支。Spec Kit specify/plan/tasks/analyze/implement 已执行，28FR/7SC/17AC与74任务可追溯，质量16/16、hooks={}。三独立macOS/Linux节点、实际调度与停止/日志/中央文件、SQLite/PostgreSQL各51项、Android真实签名与取消、全量test/race/vet与三入口跨平台编译通过；最终Linux取消及T072文档完成、T073收敛零缺口，现执行T074整功能本地交付。005与全MVP真实Apple/商店/Linux Android门保持。下面按时间保留各轮准备、失败与修复记录，当前最终结果以末尾验收为准。

## 实施所有权与前置检查

主代理唯一维护spec/checklist/validation/README/历史/go.mod/protocol与共享串行集成；拟分区A Store、B Agent/SCM checkout/单一pipeline、C server/config/CLI。正式分区与契约在plan冻结后由tasks指定，不提前写stub或第二executor。

006实际双库各37项CLI/API、全量/race/vet与Linux37项闭环通过并整功能提交。005的48个本目标文件在 `/tmp/mybuilds-mvp.zKtK0e/ios005-pending` 双份SHA256确认保存，清单 `/tmp/mybuilds-mvp.zKtK0e/ios005-pending-snapshot.json`；根仅显式还原/移除该清单本任务文件后FF006，未夹带005未验收签名代码。005+007真实签名/清理联验及全MVP Apple/商店/Linux Android门仍保留。

## 自有真实资源（准备不算007验收）

- 自有PG16.14 socket-only服务沿用006 fixture，主代理负责生命周期。新研究数据库由研究者自行独立创建/删除，不碰mybuilds006或accept006_final；实现功能suite使用正式独立库/schema。
- 自有Ubuntu24.04 ARM64 Lima实例linux006在 `LIMA_HOME=/tmp/mybuilds-mvp.zKtK0e/lima006`；无宿主挂载、agent或用户SSH密钥。已安装Git2.43，仅证明通用Linux协议/执行可测试，不作为官方Android工具能力证明。
- 自有跨VM TLS材料 `/tmp/mybuilds-mvp.zKtK0e/tls007-luc8st6v`，CA SHA256 `9ab55b1f4d1cfb7078ed8f80029c7d4a758ba9698a798cff785b984db6a360d6`，SAN localhost/127.0.0.1/192.168.5.2；私有文件0600，openssl链验证通过，未修改系统信任。真实Linux guest→macOS host网络原型合法CA/hostname通过，错误hostname/未知CA实际拒绝；证据同目录network-verification.json，自有probe已停止。007应用连接门仍待实现，不把网络原型计为功能验收。
- 新真实Android仓库 `/tmp/mybuilds-mvp.zKtK0e/android007-8pjtwr1y/repo`，SHA `b75b11d990cd07d5f71af7e69224186e19a44660`，配置SHA256 `84b46c84420c1e1c4cd1281e18429f09e39cf7a25f2115410dfeb95dfd597e63`；复制已验收examples，Gradle8.13/AGP8.11.1/SDK35/JDK17，模板离线并使用真实远端build.number。秘密为本目标004自建测试JKS/公开示例密码，单独0600envfile，未读取用户未知密钥；无商店授权，远程构建未运行。

## 已核实的设计风险

现有Run只复制git.sha/branch、LookupEnv及整次返回，必须接入当前远程消费者；always使用WithoutCancel，必须独立AuthorityContext阻止失租post，不能仅取消主ctx。动作intent与实际Started/cleanup、纳秒预算、日志spool及时失败/背压、中央原子文件与DB提交间隙都需正式契约及真实行为回归。先修analyze阻塞再implement，不能以夹具准备或类型声明认定完成。

## Phase 0/1 完成与真实网络准备

计划沿用现有依赖，无新增；具体协议统一在contracts/go-api.md，冻结NodeName身份核对、独立Authority、旧expires末尾复核、真实OnStart、纳秒预算和完整日志/ArtifactSteps终态核对。constitution 2.1.0、hooks={}复核通过；只有tasks生成并完成readonly analyze后才实施。

A双库研究原型 `/tmp/mybuilds-007-store-prototype-_idn6f34` 的SQLite/PG真实竞争、部分唯一索引、等值到期、guard占容量、条件更新/回滚及race通过；独立研究库已删除，原服务/006证据未修改。原型不算007最终代码验收。

自有SSH夹具 `/tmp/mybuilds-mvp.zKtK0e/ssh007-k6uuo9rc` 使用本任务新生成独立key/hostkey/known_hosts与单仓库强制只读upload-pack；控制端Mac与真实Linux ARM分别成功获取同一SHA `b75b11d990cd07d5f71af7e69224186e19a44660`，不借宿主默认身份。网络准备不算应用验收。自有nested Ubuntu24.04 AMD64已真实启动并SSH报告x86_64；正在安装实际JDK/固定Linux SDK。旧版已安装35.0.0 license与当前XML有差异，仅复用已接受同版历史证据，不改写accepted hash，不把Mac工具复制为Linux能力。

## tasks / analyze / implement 启动

任务T001–T074完整且连续，35/35 FR/SC与17/17 AC映射通过；readonly analyze两次无阻塞/原则冲突，hooks={}。第二次仅明确节点状态精确值、标签集合匹配、QueueStatus七项与StepProgress实际字段，现有任务完整覆盖。requirements checklist16/16、实际prereq和Go ignore复核PASS。

T001完成实际基线/所有权确认。T002协议真实编码测试先编译失败（undefined新消息），T003最小具体类型接入后 `go test ./internal/protocol` PASS（3.087s）；检验nil与零预算、纳秒UTC、完整归属、私有字段不编码、终态零cursors/显式空清单、真实Progress/Records摘要和序号溢出。协议未加验证框架，权限/序列语义由实际Store/HTTP消费者校验。

三个新worktree基线均8e1397e：nodes007-store(A)、nodes007-agent(B)、nodes007-server-cli(C)；旧006分区差异未复制。根独占protocol/docs/deps，B明确获得process归属。只同步冻结设计和协议，当前共享消费者开发不算功能验收。

## C首批实际配置交接

T004先因LoadAgent缺失真实编译失败，T006实现11个config文件后分区config/race/vet通过。root按实际字节/SHA串行同步root/A/B，清单 `/tmp/mybuilds-mvp.zKtK0e/nodes007-config-batch1.json`；root `go test ./internal/config ./internal/protocol` PASS（config1.100s/protocol0.999s）。具体LoadAgent/AgentConfig/AgentLoadOptions、Server心跳/租约、ClientCA与TLSRoots可消费；无新依赖/stub/系统trust修改。HTTP/Agent真实TLS消费者和全部实机门仍未验收，整功能尚未提交。

## A首批实际节点存储交接

T005双库真实red为缺少nodes模型；T007具体模型/索引/NULL/RESTRICT/receipt唯一/rollback/墓碑/重复迁移及T008/T011/T012实际身份和session通过。A使用独立PG `mybuilds007_store_a`：store全包3.312s、race Node3.199s、vetPASS，不触碰root既有库/服务。根按10文件完整快照/SHA同步root/B/C（nodes007-store-batch1.json），未创建未来业务stub。running/guard测试已实际DB覆盖，但完整Claim/Stop消费者后须关联回归。

C第二批实际remote CA与time.Time严格请求4文件同步root/A/B（nodes007-http-tls-batch2.json）；root两新行为 `TestNodeMessageStrictTime|TestRemoteExplicitCAAndLocalIsolation` PASS（server0.464s/client0.572s）。At合法纳秒UTC不误拒，unknown/duplicate/null仍拒；HTTPS实际显式CA成功/unknownCA失败、loopback不读坏CA、本地命令隔离。任务相关步骤仅部分接入，HTTP管理/Agent/最终门仍待验收。

## 实际执行核心、存储与HTTP增量（2026-10-04）

SHA快照交接清单位于自有fixture nodes007-{agent-batch1,agent-batch2,process-batch1,store-batch2,server-batch3,cli-batch4,store-batch3,server-batch5,views-batch1,pipeline-batch1}.json。全部只复制对应唯一writer冻结文件。Agent doctor/目录锁、真实注册心跳与CLI入口已通过真实Store/Server/HTTPS测试；process OnStart在实际cmd.Start后给出PID/PGID，回调失败同一Wait回收进程组。

Store第二批实现真实Claim/Renew/Event/Log，双库race37.407s、第三批Stop/Artifact双库test6.225s/vet通过。全局容量33允许、节点容量1–32；20竞争/幂等ClaimKey/精确标签/原expiry在事务内跨越回滚/日志完整cursor/事件历史等已测。Disable/revoke/rotate立刻保留interrupted停止保护，enable不复活旧attempt；完整失败/取消诊断快照仍进入精确manifest。最终COMMIT前expiry与失锁横切仍在T065补验证，未计最终验收。

C真实HTTP Claim/Renew/Event/Log首批全Server6.006s、race3.004s/vet及Linux/Windows编译通过；日志通过受限stage/hash/fsync/排他Root.Link/末尾DB fence确认，DB失败留下不可见孤立文件，实际SQLite锁被替换时拒绝提交。安全CLI running视图测试真实Store→Server→remoteCLI，仍不是Agent实际Run端到端。

B沿唯一Run实现Remote单build、可信Facts/声明Secrets、真实intent/Started/collector、ns预算与独立Authority/post闭锁，全部pipeline race5.192s/vet通过；实际Store+Claim+ApplyEvent与中央fsync日志测试包括无产物成功及全skipped。没有新增executor或测试注入。Root集成 `go test ./internal/store ./internal/server ./internal/pipeline ./internal/agent ./internal/cli/...` 全部通过（Store2.713s/Server6.699s/Pipeline4.103s/Agent4.220s），只证明当前已同步增量。US1实机/T036完整Agent执行/T064中央文件/T068双库最终门等仍未通过，007无提交。

自有Linux AMD64环境实际OpenJDK17.0.20.1/Git2.43/aapt2 2.19-11948202/apksigner0.9已运行；首次Gradle网络超时，以已经验收的公开Gradle8.13分发缓存转移后实际启动。完整环境preflight遇到JIT编译帧SIGSEGV（hs_err_pid3192），正用明确-Xint做隔离试验；解释模式依据[Java17官方手册](https://docs.oracle.com/en/java/javase/17/docs/specs/man/java.html)。这不是Android应用或007通过证据，真实Linux Android门继续保留。

## T017 US1 三个实际二进制节点通过

自有SQLite控制端 PID58606 与临时TLS反向代理 PID58607，应用fixture `/tmp/mybuilds-mvp.zKtK0e/us1-app007-48aqb9rf`。真实构建三入口：mybuilds007-us1/mybuilds-server007-us1/mybuilds-agent007-us1；Linux Agent CGO0 arm64交叉编译后在真实Ubuntu24.04 ARM64执行。独立mac-a/mac-b/Linux-generic凭据、session和0700数据目录，两个Mac实际PID58995/58996，Linux PID3298。Linux通过已验证CA/SAN192.168.5.2跨主机HTTPS连接，Mac通过同CA/SAN127.0.0.1。

实际CLI/API 37项断言PASS，完整安全证据 `us1-assertions.json`：三个节点健康、心跳真实推进/容量1、状态统计、角色/token隔离、无token公开；Mac实际Java17.0.15/aapt2 2.20/apksigner0.9/Xcode27.0，Linux实际Git2.43/shell且Android工具missing，不谎报能力；ios_signing均unsupported。Agent help/version/local doctor忽略损坏配置与缺token，doctor不创建未初始化目录；同data_dir第二进程和同节点不同data_dir替换健康session均实际拒绝；drain/enable、备用节点rotate/revoke/disable/enable/delete/墓碑名不复用通过。实际应用TLS入口未知CA与错误hostname均拒绝，未修改OS信任。

TLS代理仅为本目标自有验收设施，1MiB JSON转发用于US1，不是新增产品TLS监听能力，也不代表后续SSE/产物流门已验收。注册/心跳已完成，不计US2真正Run调度、Android中央文件或全功能验收；三个Agent保留用于后续真实执行，尚无007本地提交。

## 最终Store与SCM、停止/SSE路由交接

nodes007-store-final1.json九文件：完整双库race47.954s；合法OS/Arch仅真实Store注册/Claim声明回归race4.776s，不代表那些CPU实际工具验收。所有执行事务在最终checkLock查询后COMMIT前再次核对最初oldExpires；实际自有PG schema的pg_locks透传view只在Renew更新后延迟最终锁查询，移除复核真实红测 revived old expiry、恢复后回滚PASS，未修改公共对象；SQLite真实触发器跨expiry也拒绝。实际PG持锁session丢失与SQLite锁文件替换导致全部节点写拒绝、无状态/cursor写入；完整receipt/manifest/预算限额通过，T065通过当前Store全套。

B SCM九文件 nodes007-scm-batch1.json：新Checkout复用旧prepareGitRunner，真实SHA1/SHA256固定HEAD、分支推进、独立工作区、用户dirty树不变、own sshd显式pair与wrongkey/hostkey拒绝；实际HTTP取消后回收StopConfirmed=true、hook/filter/helper/submodule/replace哨兵无执行、元数据输出上限。SCM test7.746s/race8.701s/vet与Windows编译通过。

C nodes007-server-batch6.json十文件：真实SSE完整chunk分页≤1MiB/cursor续读、100ms普通WriteTimeout下确认连接后300ms才写日志仍收到chunk；实际Stop API/CLI与精确fence/note/独立身份、queued取消和running cancel_requested、禁用guard停止确认保持interrupted；实际200ms到期ticker仅中断到期项，启动也核对。Server test6.519s/client2.068s/对应race4.154s+1.809s/vet通过。Root交接后实际 `go test ./internal/scm ./internal/store ./internal/server ./internal/cli/client` PASS8.589s/3.722s/7.707s/3.450s。

自有TLS代理已换成bounded流式转发（PID66680），为后续实际SSE/产物端到端准备；重新查询真实status健康节点3，不把设施准备当通过。Agent完整execute/journal/spool/lease及中央产物接口/客户端流消费者仍由B/C实现；007不提交、不宣称MVP完成。

## 后续实机执行准备与实际网络中断观察

最新Stop/SSE控制端二进制接线后在同自有SQLite重启，PID70637；重启间隙两个Mac Agent真实heartbeat出现agent_network_error退出，Linux Agent随后继续心跳。此时无running，两个Mac未假报健康，新同名session要等原最后心跳2个lease策略；早先T017成功证据仍为此前实际时刻，不把目前离线状态写成健康。预登记generic007项目经新SCM明确0600 controller.env独立SSH凭据读取固定SHA8c8751e；race-a/race-b原子排队编号1/2，完整响应us2-trigger-prepared.json，尚未实际执行，不能计T036。默认server secrets_file已明确自有路径，未借用户默认秘密文件。

## 单Run消费者末尾预算边界修正

实机fixture原generic shell把模板写入run正文；沿已有002行为run脚本不改写，故改为显式env传可信事实，node使用既有node.name。新自有repo SHA `a5c5d82a70203d87dc3bf64c715060610353f553`，dry-run完整通过；已排队8c8751e快照不改写，用于后续分支变化不影响原任务核验。

B消费者发现最后ordinary finished确认后的日志/完整快照回传延迟可耗尽普通预算，Store原按stepsummary只接受success post。已冻结最小修正：post_selected有限预算0/ordinary确有Started且原本全成功可Reason=timeout选择failure，步骤真实succeeded不重写、原失败保留，终态failed/timeout。A/B在原owner分区追加真实消费者及双库回归，尚未把未完成修正计最终通过。

## 产物HTTP已可供真实Agent消费

C六文件 `nodes007-artifact-server-batch1.json` 按SHA同步所有分区：真实binary含NUL/0xff上传、固定声明幂等、实际100ms普通Read/WriteTimeout下分段300ms上传仍ACK，最终旧fence/原expiry再验。2m专用流期限，实际CheckExecution每500ms唤醒失权慢body；中央相同fd完整Size/SHA核对后下载，节点禁用后仍与原字节一致、同长度篡改拒绝。全Server7.521s/针对race2.892s/vet通过；root集成Server8.783s/Agent3.466s/Pipeline4.942s通过。完整安全负例与CLI流消费者仍由C继续，B真实执行上传消费者尚待接通，未据此勾全T055/T058/T060或整功能门。

009真实自有测试工程 `/tmp/mybuilds-mvp.zKtK0e/flutter009-fixture-pzo5xz7h/project` 通过既有Flutter3.38.6 --suppress-analytics create --empty --platforms=android,ios生成73文件，依赖解析成功。CLI自动读到了既有保存的签名选择；只移除自有新fixture pbxproj DEVELOPMENT_TEAM，不修改全局Flutter配置、不用未知身份签名。仅准备后续测试工程，不算原生/Flutter iOS验收。

A末尾普通预算窄边界真实红绿完成：两个测试先失败（正确timeout post遭拒、预算0空Reason success错误被接受），修后双库test1.527s/关联race10.472s/vet通过；真实ordinary artifact succeeded→20ms确认延迟→CommitArtifact/Log→预算0→failure/timeout，保持stepsucceeded及全部诊断manifest。9种错误nil/nonzero/allskipped/原exit/cancel/错误phase/reason组合拒绝且事件cursor不推进。仅event.go/post_budget_test.go按报告SHA同步root/B/C（nodes007-post-budget-store-batch1.json），T065关联再次通过；B单Run真实超时消费者尚待联接回归，不能计最终门。

C客户端10文件 nodes007-stream-client-batch1.json已按SHA同步：真实最大合法HTTP日志chunk进入Store/file/SSE/CLI，16records全显示且ESC安全转义，普通timeout50ms不截断1s独立stream；真实NUL/0xff下载metadata/同目录0600stage/Size/SHA/排他Root.Link，存在目标拒绝且无残留stage。分区针对test1.552s/race3.032s/vet通过；root `go test ./internal/cli/client -run 'Test.*(Stream|Download|Log)' -count=1` PASS1.294s。README同步已有Agent入口/目录和仅注册心跳的当前实际阶段，仍明确整功能未验收。

## 完整 Agent 与 CLI 消费者、首轮实机执行通过

B19文件 execution-agent-batch1 与后续 stop-agent-batch1 五文件按SHA串行同步root/A/C；真实完整Agent/SCM/pipeline/process race27.198s/5.251s，Stop独立失权后实际等中央guard再确认、保留pending回执等四包race43.682s/6.563s通过。普通末尾ACK延迟耗尽预算保持步骤succeeded而选择timeout/failure；post末尾ACK延迟耗尽独立预算则failed/post_error，原失败保持，真实Store关联race3.623s通过。无第二执行器、source模板改写或伪manifest。

C16文件 server-final1：实际禁用后慢artifact body以过去ReadDeadline和Connection:close唤醒，不再清deadline导致HTTP排空阻塞；1byte后disable实际约0.53s拒绝/no stage/meta，原合法慢上传仍200。config/server/allCLI全包test/race/vet、Linux/Windows三入口纯Go编译通过。最后单新增evidence_test.go真实Server/Store/Git+Agent CLI0600配置→claim→Run→artifact/post→build JSON/history/SSE/中央NUL+ff字节download→running cancel/always→角色隔离→Agent停后下载，分区test4.125s/race6.107s，root接入同用例5.118s通过。

应用fixture us1-app007-48aqb9rf已升级完整执行二进制：server PID95634、两个Mac Agent95652/95653、真实Ubuntu24.04ARM64 Agent3714（各独立session/token/data_dir）。实际两Mac分别完成race-a/race-b，UTC执行区间确有重叠，触发前冻结8c8751e SHA/定义未受repo后来a5c5d8影响；双方Started/StopConfirmed与实际ns正确。无runner generic仅交给default Linux-generic，编号3、a5c5d8固定SHA/可信project/build/number/node/git/marker env正确，真实result.txt完整回传中央；CLI下载字节remote-baseline\n、SHA256485d30a2e72aadedfcbf3d5a14a18926781b78829320ab69cb248d632cfb277c与metadata一致，已有目标拒绝且不覆盖。证据us2-first-assertions.json，脚本us2-first007.py实际exit0。仍仅首轮正向实机证据；完整T036调度、T053失联、T064文件负例/T068双库/T070Android最终门未据此声明通过，007尚无提交。

B最终四个新增真实测试（agent authority/artifact_source/concurrent_secrets、pipeline remote_facts）SHA快照agent-final-tests1.json同步所有分区，生产无增量。实际always运行时disable回收自身PGID、外部自有sleep存活、后续脚本未运行；中央log ACK后将journal目录改0500造成真实cursor保存失败，spool/PendingLog/fence保留、当前与always全部停止、重启拒绝；撤销旧凭据不能解除guard，已回收proof保journal，admin精确确认只清guard不改interrupted原reason。并发capacity2真实任务Started区间重叠、独立workspace/声明秘密、不宿主fallback、日志脱敏/journal无秘密；可信Facts只来自Task，参数保留文字不二次展开。collector实际symlink/FIFO/hardlink/publicmode/size/hash/越界/cancel拒绝通过。B最终Agent44.101s/Pipeline5.190s、四包race50.872s/8.536s及process/SCM/vet通过，root定向接入agent10.005s/pipeline1.743s通过；T045/T049/T056/T066完成代码与真实行为检查，不替代最终跨VM/Android/双库应用门。

Mac远程Android首次固定e7ab37c工程执行46个真实Gradle任务，到签名packaging正确失败（自有测试JKS与fixture密码不配对），status failed/exit、Started/StopConfirmed、剩余预算及后续artifact skipped正确，中央日志秘密脱敏。只在自有fixture生成新的10天RSA2048 JKS，使用同声明密码env实际keytool验证配对并更新自有Agent私有env文件；未修改任何用户签名或产品行为，第二次触发继续真正签名构建，不把失败包当成功。

C新增scheduling_evidence_test.go SHA be68c30b38f5e437d9ade5daa9411f6adf5f824bba5920553da88576ebadf844按唯一writer快照同步root/A/B。三个真实Agent admin容量1/config容量2、global2，实际同名第二条queued/build_name_locked、不同项目不同节点并行、第三项目capacity_wait，真正脚本屏障释放后各只执行一次；20并发CLI同key真实只创建一Batch/编号并执行一次。无runner只用default，不任意取其他健康节点；无default在trigger整批拒绝无记录，授权未注册节点/缺label保持queued无attempt。实际ordinary exit7/failure exit9/always成功，中央安全视图保原reason exit/PostPhasefailure、Started/StopConfirmed/ns与private扫描通过。C四组合test11.880s/race14.285s/vet通过；root相同新增组合test10.196s通过。T043已补齐；T036实机跨VM补记录后方全勾。

真正Android第二次配对签名后Gradle50任务BUILD SUCCESSFUL而step failed/exit_code0、未执行artifact，定位已验收共享process.Run的500ms WaitDelay误把同步远程日志排空当child失败。已执行speckit-bug-assess，slug slow-log-pipe-drain，valid/high；隔离接受基线8e1397e的slow-log-bug006 worktree优先最小修复、先实际慢writer红回归，不忽略强关pipe错误/增加任意等待/压低Gradle日志来过门。修复与全MVP/007验收均未宣称完成。

T036完成现有调度组合与跨VM二进制证据交叉核对：C实际三Agent同名串行/跨项目并行/双层容量/20幂等脚本一次/default/授权标签排队，root实际两Mac执行时间重叠/一次终态/固定SHA、真实Linux无runner只default，证据scheduling_evidence_test.go与us2-first-assertions.json。未授权和工具缺失是实际排队/noattempt，非失权后自动迁移。

T042新增root实际frozen-context007.py PASS：drainLinux先真实queued无attempt，冻结e7ab37c SHA/SourceDigest/显式marker；修改项目settings marker、repo默认与定义至后续7b7480804790a586d214635c1cf8149034b0430b后enable，真实Linux仍执行旧SHA/定义/marker=frozen-explicit、后续默认未注入；保留中央日志/产物。传build.number伪参数在trigger整批拒绝且next_number不增长，可信节点/project/git/env核对正确。证据frozen-context-assertions.json。声明秘密/参数不二次展开/host不回退/真正普通与post纳秒耗时/失权/未实现能力零动作由已同步B真实消费者检查覆盖；Android实际签名成功门因slow-log-pipe-drain仍待修复复验，不计T070。

T064现有实际HTTP/file/SSE/download安全检查root接入集成复验：server4.084s/client4.823s/agent3.782s通过，含全binary/摘要/短流/路径/header/限额/重发/失权慢body/特殊中央leaf、合法慢流专用期限、source快照、节点禁用后可下载。root真正CLI在实际TLS入口follow已完成Linux构建收到完整三条中央日志并end退出0；新自有TLS转发夹具仅转发真实Server SSE，在第一完整event后真实关闭连接，客户端以after_seq/LastEventID0→1自动重连，输出与无断流逐字节一致、无重复/丢失，sse-reconnect007.py exit0/evidence sse-reconnect-assertions.json。没有fake数据/强制进度或产品代理改动。

## T053 实际二进制七个故障场景完成

C独立fixture mybuilds-t053-k1ixthhg与awuozkn7，仅自有SQLite/受信Git/私有CA/TLS端口/独立Agent token/session/data/PID，不触碰root节点/服务。默认heartbeat5s/lease30s，普通和always断网、ordinarydisable、alwaysrevoke、drain、controller真实PID重启、AgentSIGTERM退出全部完成；合并t053-combined-evidence.json SHA2563e3616719aa687c93f2567bfb8fd3106dec391583936df527fb1b1ee25f8e464由root核对，7个PGID再次killpg0全部ESRCH。真实TERM忽略leader/backgroundchild先由journal+ps确认归属，然后观察整组消失，无关ownsleep仍存活。以最后实际claim/renew请求开始为更保守基准，断网ordinary5.431s/always5.399s、disable5.410s/revoke5.369s/Agentexit0.979s，用户最后trace动作也≤25s；不以计时当回收。

网络中断和Agentexit保旧journal、重启journal_unconfirmed exit1/文件SHA不变；中央guard保留至node/admin完整fence确认204，错epoch409；disable原node在真正reap后自动process_group_reaped确认清guard/保interrupted原reason由DB确认，revoke旧token不能越权确认。drain两次真实renew仍running→实际释放屏障→succeeded；controller重启保同fence/剩余预算，renew继续→succeeded。全部自有进程由脚本结束，原始run.py/gateway.py/requests.jsonl/私有journal与脱敏结果保留。此批Agent二进制SHA23ad5f0尚在slowlog修复前；当前故障边界已通过，共享process修复后将按实际改动复验取消/后台/慢日志相关部分，Android最终门不据此跳过。

## T064 实际中央证据负例补齐

C新增artifact_publication_test.go SHA7b5cf872484f346ad67b3d625a2340ec3d1c59f92be7db4ef985ae0fd7fc1802，真实自有SQLite第二连接BEFORE INSERT查询自己不存在table，实际HTTP stage/hash/fsync/排他Link之后SQL失败固定500；完整孤立UUID文件保留，metadata/download404、List/FindNode空、artifact seq0无确认记录。只drop自有trigger后同声明真正200canonical/seq1，重复仍原canonical且只删除本候选，原orphan SameFile保留。分区test0.538s/race1.866s/vet通过，root接入2.036s通过，无生产testhook/假状态/清unknown文件。

root实际lost-log-receipt007.py：自有verifiedTLS代理第一次中央CommitLogChunk成功后真实断开响应连接，Agent重传同seq1/offset/digest/完整bodySHA，中央只保存一次；后续seq2/3和终态真实完成，完整三records/no重复，实际Run+快照成功。证据lost-log-receipt-assertions.json、构建1c626b8f-19c1-43e2-8d19-df2bcde19e94，脚本exit0且自有代理/Agent已结束。结合前述SSE断流重连、实际取消/slow合法流、source边界/manifest/cursor完整、节点禁用后中央可读及spool/journal实际保存错误闭锁，T064现有安全与完整性门闭合；slow-log-pipe-drain成功进程吞吐缺陷仍单独修复，不忽略其未验收状态。

root同名跨实际二进制节点串行补验 same-name-real007.py PASS：第一race-a实际mac-a Started后drain该节点（已有lease继续），第二同项目同名保持queued无attempt直到第一StopConfirmed终态；然后仅mac-b领取执行，两组实际stdout UTC结束<后者开始且各一次执行，真正节点不同。混合池Linux轮询会将最近claim诊断写为capability_mismatch，合法固定码不等于同名互斥被绕过；夹具不再对立即一次查询假定全池稳定单一reason，保留真实queued snapshot/完整两时间线，same-name-serial-assertions.json。first-v1仅诊断断言过强失败材料保留，未把夹具预期错当生产缺陷。

slow-log-pipe-drain最小修复已移植root/A/C：process3文件pipefix-batch1按SHA同步，分别真实childWait/两OS pipe完整EOF/原TERM-KILL回收，read idle500ms不把同步writer耗时计进去；仅实际Write共享mutex保留同一writer的原串行语义，无executor/队列/依赖。root定向process race12.073s PASS，B已验006全包/007四包最终race51.936s/8.547s/18.288s/10.495s+vet通过。新三二进制pipefix全build0，独立mac-android-pipefix真实Agent PID37718。原症状实际复验构建62582d29-ac47-46d6-8c56-124c4ebfb6e1固定7b7480、source digest同原AndroidYAML、项目编号101，50Gradle任务BUILD SUCCESSFUL→真实artifact3文件→中央完整terminal succeeded/PostPhase success；Started/StopConfirmed/noCleanupFailed/Exit0、实际ns15s与回传准备预算正确。完整中央日志76records，无旧failed/exit误判；三份CLI中央下载size/SHA/byte相符、mapping含实际示例类，证据android101-central-download/central-evidence.json。签名/manifest和取消/Linux进程回归继续核验，007/整个MVP未计完成。

## T068 双数据库真实最终应用通过

自有独立fixture mybuilds-t068-c-tj6nme5q，SQLite与实际PG16.14数据库mybuilds007_app_c串行相同51项，共102项PASS；565条UTC命令记录2026-10-04T13:28:58.816919Z–13:29:45.405071Z。统一三个真实pipefix二进制、每库独立Node token/session/data_dir、受信Git与privateCA/TLS，管理/固定SHA与settings参数冻结/actual Run+ordinary和失败post保原reason/20并发同key一执行/真实running cancel→PGID回收+always/三次renew/SSE独立流期限与终态/中央NUL+ff字节SizeSHA下载及Agent离线下载/三role和node隔离/第二控制端与online本机写拒绝/旧journal SHA不变拒绝重放/错epoch与精确admin停止确认通过。8个自有子进程及4真实执行组最终ps全不存在，root3节点/VM/服务未触碰；实际PG SELECTversion与统一binary SHA/go version-m附录。

证据evidence.json SHA84d4269c00cec5937feaaaabd9d9ef99c02273b936ab87d9044a4c6e62d5262b、run.py SHA37eec32e941ed55182b56ab55ccf84216bb472accf585fd71b0f67ba6fce3df0、gateway.py SHAf867083f4bab0c43f7fc6a9a84f61b7da48301387fc28542fd3ce28221634acd，root校验冻结文件。首轮20连接只11到Python测试TLS代理，修正该代理listen backlog5→128后同产品budget/timeout全部通过，旧失败fixture g770zn6h保留，不算产品缺陷。T068正式完成，无PG DryRun/mock或旧研究替代。

共享process独立缺陷验收实际AndroidAABmanifest/sign同证书、真实LinuxARM process全包执行通过。按git-commit-message仅6个相关文件一次本地提交58bc9c818267da5204630691d533660f9afa9a00（fix(process): 防止慢日志回传误判成功进程），root安全FF：仅4个自己的process/bug文件先完整SHA备份/逐个移开，其他007/用户修改未碰；合并后恢复007 OnStart增量，其余bug字节一致。007当前accepted基线含此fix，原0068e1397e仍为规范起点，不把独立缺陷提交当007整功能验收。

## T070 真实远程 Android 签名与取消完成

以新独立 mac-android-pipefix 节点、声明的私有 SSH/签名环境与已修复共享 Run 执行固定提交 7b7480804790a586d214635c1cf8149034b0430b。构建 62582d29-ac47-46d6-8c56-124c4ebfb6e1、编号101，真实50个Gradle任务成功；普通步骤Started/StopConfirmed、exit0、真实耗时15.060569083秒及134ms快照均正确。中央确认3个文件：APK8567B/SHA2568dc846ca2f3870cd215047fc4479f9aabe25cce68cc907351f976b737b088f1b、AAB7243B/c29d7f873356d959c4c0e21886194c751ab1d11e73a4bd1645a40f240cd071c6、mapping465B/dae525e42d4beedaa2f7cde5f133962189c428d553e13a3d7c2573889fc8c51e。真实CLI中央下载后字节/大小/摘要均一致，apksigner核验APK签名、jarsigner核验AAB签名与自有测试JKS同证书，证书摘要82571519db6c5fc99a8bc05d01a0dd89367e86107bf21af1dbc161167001e142；aapt2与工程AGP实际bundletool1.18.1解析APK/AAB应用com.example.mybuilds、版本1.7.0与build.number=101。证据同fixture android101-central-download/{central-evidence,signature-assertions}.json及真实命令输出；无未知用户签名材料或商店发布。

随后真实Gradle正在运行时触发CLI取消：8dc9a8e1-14af-429d-9088-734ec7d62c12、编号103，私有journal与ps确认该任务真实PGID54206，观察取消后整组killpg0为ESRCH；无关自有sleep仍存活。中央终态cancelled，Started/StopConfirmed、无CleanupFailed，未执行后续artifact、中央产物列表空。证据android-cancel-assertions.json，脚本android-cancel007.py实际exit0。前一次编号102也真实保存取消意图；验收脚本最初把cancel_requested误当终态，修正仅fixture等待集合后新编号103完整复验，不改产品语义、不抹旧历史。

以上完成AC3.1–3.4/SC3的007通用及Android闭环；真实Apple005与整个MVP Linux Android签名门仍保留。

## T071 最终集成检查通过

在已集成 slow-log-pipe-drain 修复及全部007冻结交付源码后，实际运行 go test ./... 全通过（Agent57.549s/client25.359s/process15.107s/Store8.830s）；go test -race ./... 全通过（Agent59.680s/client32.515s/process16.427s/Store51.145s）；go vet ./... exit0。没有用编译替代真实平台工具。CGO_ENABLED=0对三个入口分别编译darwin/arm64、linux/amd64、linux/arm64、windows/amd64共12次全部成功；本机三个实际二进制help/version共6次exit0、共享version一致。完整命令和结果保存在 /tmp/mybuilds-mvp.zKtK0e/final007-build-evidence.json。真实Linux ARM进程test二进制已执行全部19项PASS；实际SQLite/PostgreSQL各51应用检查、三个原独立节点、Android签名/中央下载/取消的证据见前述各节。Linux Android预检仍单独进行，不计为已完成。

最终证据摘要复核：us1-assertions.json 907b7981ac3932cac3b26dcaf310d93d6e562c16be8c27f839259668830a4648；us2-first-assertions.json 9bc974c02ce071cf2e2ec3b4b6c24e3ad629acb1f850d12416204f1cf554a1cd；same-name-serial-assertions.json 49a5e98c74ac79c36d8dd407f96b0f483486577a5ccdb65f9225863774f85934；frozen-context-assertions.json 403dfbc3f5b6fa2e17be45d49cdd4a30029ea5904cbbaf820ba1346e11a151cb；sse-reconnect-assertions.json 40e7344ffb11fb86fd9d70a0433714b69e764616b47fdfecb279edb82589f37f；lost-log-receipt-assertions.json 06ba95e8e5bd2a3d2b077b90bdd770d6d21bab340f5245b24bd4777a28c4de3a；android-cancel-assertions.json c6d1cce2f3aaa33740df11b9f788e7ea2f51dbfe79d987a01fe4903979d44012；final007-build-evidence.json 8f19c197c97b14cbf0d79060018a898a98ae6d58786c3db7db0f17a1874acc81。原始私有凭据仅保留自有受限fixture，源码/公共日志/文档不复制其明文。

T072文档批次已由C唯一writer冻结并按SHA交接root，随后root串行核对实际行为：Agent SSH键名为MYBUILDS_GIT_SSH_KEY/MYBUILDS_GIT_KNOWN_HOSTS、控制端为GIT_SSH_KEY_FILE/GIT_SSH_KNOWN_HOSTS_FILE；实际节点名与示例名分开，日志声明secret脱敏与公共详情隐藏参数值明确。README/DELIVERY/quickstart相对链接41项、Bash脚本20块bash-n、git diff --check与所有改动Go文件gofmt-l通过。147个本次修改/新增文件扫描自有管理员/节点私有token，明文匹配0；未读取未知用户秘密。当前最后Linux取消/收敛尚未计完成，未暂存或提交。

## T069 最后真实Linux取消强化完成

新独立节点linux-pipefix/session56347b92开头/私有data_dir/verified HTTPS，在真实Ubuntu24.04 ARM64执行linux-cancel007编号2，build ad7b8b5b-aded-4ce3-899d-c5a2ad94c603，固定SHA7b7480804790a586d214635c1cf8149034b0430b。ordinary真实PID/PGID4749及后台进程忽略TERM，Started→cancel_requested→cancelled；有效权限内always实际OnStart PID/PGID4780/phasealways曾存活，随后succeeded/exit0。双方StopConfirmed/noCleanup，两个group最终ESRCH、无关自有sleep存活，日志含ready=id与user-always=id；root再次在guest实际killpg0核实4749/4780均ESRCH。

完整证据linux-pipefix-cancel-b2/{result,final-build,physical-stop-proof,all-groups-stopped,private-group-starts,final-private-isolation}.json，manifest SHA256b12c1286f9f1949b69220e456089e624367f74cd322603d1cd457b4f3a3c0474由root核对全部文件，guest Agent SHAd4ad126f726ae9c814fd29454b60456fbadab4e2903a95dd7de43c9056247c49与root产物相同。只停自己的新Agent，原linux-generic3714未受影响。第一次夹具误把cancel_requested当终态后停自身Agent，旧记录真实interrupted/lease_expired、journal保留，依据实际ESRCH完整fence admin仅清guard，不改旧终态/原因或重放；失败材料linux-pipefix-cancel-b保留，不改产品行为。

结合三节点37项身份/TLS/真实能力门、两Mac并行与跨节点同名串行、七种失权故障、日志重发/SSE重连/中央下载，本次Linuxordinary/always取消完成T069/FR026/SC1/4/6。T070真实Android签名和取消、T071最终检查、T072文档也均完成，当前仅T073收敛与T074交付。Linux ARM仍仅证明通用工具；005真实Apple与全MVP Linux Android门保持。

## T073 最终Spec Kit收敛

实际prereq --require-spec --require-tasks --include-tasks定位007，读取spec/plan/tasks与constitution2.1.0，按需求映射核对28FR、7SC、17AC、74任务、10项架构/依赖/持久化/执行边界规划决策与5项原则。节点身份/策略/私有目录、事务领取/原expiry、固定SCM/唯一Run/可信事实/逐步secret、intent/Started/ns/post、独立Authority/真实取消/guard确认、spool/fsync与中央日志/完整manifest/安全文件/SSE/下载、CLI/双库/真实Mac-Linux-Android门均有实现与实际行为证据；未实现后续能力仍清楚拒绝。missing/partial/contradicts/unrequested与各严重度均0，无原则例外。T073/T074为本次核对与接下来的授权交付，不是缺失业务实现。

Converge自身不改任何源码/规范/规划/任务：tasks前后SHA256均decca628de428f5044ceb90eec31e483ed20dec3849ff713013fa71c92e4f487，不追加空阶段；hooks={}，无前后执行项。当前功能满足规范、规划与任务，无进一步implement业务缺口；不把007完成当整个MVP完成。随后在交付阶段记录本节、勾选T073/T074并完成本地整功能提交，无push。

T074交付前核对本次全部改动为007规范/实现/行为检查/文档；同步PLAN/MULTI_NODE/BUILD_DISTRIBUTION三处实际状态，保留DELIVERY verification锚点兼容已有导航。先检查工作区和暂存差异，再整功能一次本地提交，信息`feat(agent): 实现多节点构建与中央证据`；实际哈希通过Git日志与实施历史追溯，不夹005待验收或用户修改、不push。
