# 019 实施与验证记录

当前源码已集成，本地/中央JUnit检查、原XML确认封存与查询下载已可运行。全量普通、race、vet及12次三入口跨平台编译已通过；双库名义应用检查点与macOS/Linux实机门通过。最终双库20故障场景与192个断言已通过，正式收敛无缺口，整功能提交见实施历史。下列记录按实施顺序保留历史状态、失败和修正；旧检查点不冒称最终源码。

## 规划阶段历史

当前仅文档，尚无源码实现、产品验收或功能提交。007基线85b46bf；008仍在完整验收且两个实证缺口修复中，019实施依赖主代理在008实际验收提交后FF/rebase并冻结真实字段，不能借未验收源码。

已有18FR/6SC/4P1故事/12AC、质量16/16；七Plan完成后实际setup-tasks/prerequisites生成46项连续任务。第一次只读analyze发现3项HIGH：共享writer错配、严格Store.Enqueue接受报告与checked互等、parser与CommitArtifact正例依赖循环。在analyze外只修Plan/contract writer与tasks依赖，未缩需求；最终只读前后SHA相等，24/24要求与12/12AC全映射，任务46/46映射，0问题/歧义/重复/未映射，原则2.1.0五项PASS、hooks={}、diffcheck PASS。仅T010/T018/T032有有效并行，模型与真实consumer前置保持。

冻结manifest reports019-tasks-analysis-final.json SHAa8d687522ba91cb7ca95131ddbe275b1db96d031a00a25eb66aba06c4e1e165f；spec/checklist继续原SHA，root逐SHA串行复制十文件。主selector仍008，不执行019implement，也不把019文档混入008功能提交。下一步等待008及共享process实证修复正式提交，再由root校正基线/归属与接口、只读analyze并实施。

root仅更新spec当前时态（007已验收、Plan/tasks已完成），没有修改18FR/6SC/12AC及需求；其SHA因此与原冻结值不同，此修改单独记录，不伪称所有root字节仍原SHA。019尚未implementation。

等待008最终实证期间，B只读盘点实际encoding/xml语义与纯parser8组行为门。根在analyze之外补明确技术边界到reports/go-api契约：合法父子矩阵及可缺name；标准声明/BOM；PathKey由实际consumer补、集合重限及诊断UTF-8字段值字节口径；实际有限Reader的ctx检查/固定IO错误。没有变更FR/SC/AC或编写源码/tests，原接口仍一个ParseJUnit，后续须按现有任务重新只读核对依赖/覆盖。两契约当前字节不再与最初冻结manifest相同，差异有此记录。

## 正式008基线已验收（实施准备）

根已实际提交008 504dc6fa8581f74a15ecc146a474976d5ae33a22，共享process dd8fb4a86135af1933fed11743ad47d699a697ea。新019集成分支由504dc6建立；仍保持18FR/6SC/12AC、46任务和原唯一writer。两项parser契约澄清后再执行只读analyze，零阻塞后才启动实现。未借005待验收源码，不降低iOS/商店真实门，不新增依赖。

## 实施基础进度（T001–004）

正式analyze在504dc6基线复验：18FR+6SC=24/24、12AC全覆盖、46任务、0阻塞，原十份规范SHA精确一致。实际prerequisite选择019、hooks={}；需求质量16/16，Go产物/selector忽略已核实，无新增技术忽略文件需要创建。根完成原README/计划/原则及全部019设计阅读。

T003三条真实wire测试在旧协议执行exit1（reports/manifest及JUnit用途丢失、缺私有报告字段），不是用undefined符号编译红；T004加入有限具体消息、omitempty可选字段、私有快照通道及三种显式空数组JSON输出，原无报告消息不增加字段。同命令go test -count=1 ./internal/protocol通过，未放开Reports执行或添加第二Run。C独立config-cli工作区以504dc6建立并同步精确三共享文件，T006先红后绿实施；迁移双库基础T005仍待执行。

## 基础门通过（T005–007）

T005旧007/008真实迁移红测exit1（缺report_revision）后，原Migrate具体字段接入；最终SQLite/PostgreSQL两库×两旧版本均通过，重复迁移不改旧记录/预算/XML用途。008夹具实际保留kind=build_finished、stop_known=true，007仍为空/false；旧FK和retry关系保持。最终同进程四组迁移/约束门共10子项PASS，go test -count=1 ./internal/store -run 迁移及节点约束 精确命令同此前记录，总2.660s；独立数据库mybuilds019_tests。

T006 C在独立worktree完成7边界实际红→绿；严格配置/preview目标普通测试1.026/0.482s、race2.049/1.637s、vet/diff均PASS。root逐SHA核对四文件后复制，manifest reports019-t006-manifest.json SHA70e4f2346ea435c393b7efdec97057183700a017d3f0f9deb230b419306e51e8。

T007协议真实wire通过，迁移双库门通过；基础字段与C路径限额冻结同步A/B，只有具体协议/模型/配置，无空parser/Store/Run函数，未放开Reports执行或Trigger/Enqueue。新增VerifiedJUnit只作server内部字段，空数组输出规范不会代替Store入参非null校验。A负责真实parser及Store新文件，B负责真实collection及Agent新文件，root串行接所有共享入口。

## US4 入队与消息形状真实红测（进行中）

root新增enqueue_report_test.go：两库严格有效Reports定义当前真实invalid_request，未放开unsupported；非法整批原本已正确拒绝且不消耗编号，不人为造红。首轮夹具把GetProject误用ID已按现有name契约修正，产品源码无变更；最终红测总0.756s，仅有效入队待真实T027consumer。

消息形状独立负例真实红：六种旧事件会接纳LocalReports私有路径。root给原validProgress加私有字段与生命周期位置门，未放开新Kind；目标形状及旧ExecutionEvent完整回归go test -count=1 ./internal/store -run '^(TestReportFieldsStayAtTheirLifecycleBoundary|TestExecutionEventOrderReceiptBudgetAndTerminal)$' PASS总0.515s。当前event.go SHA76e0430a578e520b20665ae8044f9b5bf311169ab11501010f0373bc2f0f78ac，等待A真实checked/sealed私有函数再串行dispatch。

## 首批实际parser与用途边界红测交接

A首批ParseJUnit/MergeJUnit为实际标准库实现；T008红exit1（符号不存在，不是行为红）→T009普通0.950s、race1.392s、vet0。root逐SHA审读并复制root/B/C：junit.go 7caa987f8be1c0190a6974340837f3a079bbd60b5c0439f89fcafde6c3894d1d、junit_test.go ee9730964c8b52ded4fd64aabc6071bec9ee1e7bcdc32bd9c134ea59c96be129。B实际consumer发现补PathKey后诊断显示cap会自然越界，A在原需求内继续红测修正，不把首批当全部T017完成。

C新增独立server/report_test.go SHAfe9f568bda618310654f0027c58e6effd38b2e7458f8de91493c4644ae7f755f；11用途边界真实红（0.457s），StrictJSON与旧HTTP上传回归0.521s通过。root逐SHA复制测试；用途Store/server接线仍待真实T027–029，不造空stage/parser。原预算实际consumer缺少有限读取接口，root/A/C在analyze之外冻结go-api新增具体ReportUploadBudget实现细节，仍原FR013/015，无新schema/cursor/interface。

## 实际本地Run与双库共享入口（2026-10-04 UTC，进行中）

root逐SHA复制A实际Store报告函数、B实际collector，串行接既有Run/remote与Store event/enqueue/artifact/query。共享配置模板从原preview精确提取到internal/config/template.go，不新增执行器或第二模板语义；Reports路径使用params与build层真实facts，不使用workspace/step.name等私有步骤局部值。config/pipeline定向回归分别0.278/0.797s通过。

root实际双库命令（独立mybuilds019_tests，DSN仅私有环境）go test -p 1 -count=1 ./internal/store -run '^(TestReports|TestReportUpload|TestReportFields|TestEnqueue.*Reports|TestExecutionEventOrderReceiptBudgetAndTerminal|TestArtifactCanonicalLimitsAndCompleteTerminal)' PASS 3.029s。有效Reports入队原真实红已绿；原事件/制品回归通过。当前sealed后公开读取才带Reports；用户JUnit文件列表/下载仅当前sealed版本，批次HTTP沿既有batchView只返回摘要，无需新权限包装。

B发现候选Files/Diagnostics slice别名能污染本地seal比较，真实Size/Source/Message三红后改为私有canonical字节及逐File完整当前快照比较，seal返回slice独立。root严格复制reports.go b6b7f9e842b09b3d15cd22b6bbbb2ddc16651e71f4b02abffade4145c22b9608与reports_seal_integrity_test.go c7d2119b66ee4c3c3f9785e8c0bf3fa508d7f630c9b016dbe3bca47d36f23a72；实际go test -p 1 -count=1 ./internal/pipeline -run '^TestReport' PASS 1.740s。共享交接实际manifest为reports019-shared-runtime-2.json，未把B工作区独立测试代替root验证。

server用途声明11边界真实红后，root在原artifactDeclaration接具体用途/revision/key/ordinary/8MiB门，StrictJSON原规则保留；go test -p 1 -count=1 ./internal/server -run '^TestReportArtifactDeclaration' PASS 0.454s。此时完整稳定stage/HTTP上传仍待接，不宣称T029完成。

实际客户端重新构建后运行8份新隔离工作区，命令均mybuilds run --file mybuilds.yml --build junit：通过1case、合法0case、required缺失、optional缺失、optional坏XML、旧XML不改、1→5case替换失败、命令exit7并产生失败报告，8/8断言PASS。前两及optional缺失exit0，其余exit1；每份均核对真实manifest canonical SHA/独立原XML Size/SHA/真实来源步骤。替换最终仅5case，不合计6；失败/always标记出现而普通/success哨兵未出现，post-pass工作树不改变失败快照；exit7保持原exit Reason，旧XML inode/mtime/size/SHA完全保持且未计入。

完整命令/UTC/退出码/Counts/原XML摘要证据：/tmp/mybuilds-mvp.zKtK0e/reports019-fixtures-bqy3ncau/local019-z_znx1hz/evidence.json SHA7d2192da0a37463aab1d1d8d8076435e35cfdeb60f9860117af404f1736c0044；runner run-local.py已计入证据SHA，源码夹具初始manifest仍not_run作为历史，没有回写造假。以上只是实际本地检查点；CLI专门展示、全部Authority/预算门、双节点与中央HTTP/下载、整功能收敛仍未验收，不提交019。

## 预算实际红绿与Stage接入（进行中）

B真实Run报告checked持久回调写自有JSONL/fsync并等原600ms普通ctx到期，原真实failed/report_error红；root修为只在报告事件ctx真deadline、原Authority仍有效且ordinary确耗尽时保留timeout，仍闭锁禁止always。实际新Run预算/Authority/save失败/用户取消门复制reports_run_budget_test.go SHA951ee6ce3addb2987e6553e35cb8ae694c27dabb0b3de1e0568348275b08d8ed；root解析+Report组普通测试PASS0.531/1.971s。旧Run批预检查尚认为合法Reports未支持导致完整race真实失败7.905s；root更新该项为缺本地build.number的真实报告模板负例，整批无脚本副作用断言保持，root目标普通测试PASS0.521s；B完整pipeline race随后PASS8.485s、vet/diff0，未将旧失败删除。

A纯parser扫描百万实际已声明候选值取消压力旧4.08s超限红，逐值ctx检查后绿，root精确复制junit.go 1d6d9a733638e3aaa1f24281d4d4826443bd27bd4f9db3820f2b6894d46815dc与junit_cancel_test.go 8ff709f87834a0e0fe7a80967d8dbce7983fd3470ae603376d6babb7b210049b。PG实际末尾锁查询发现Report预算可能越过，root追加原Store.write中具体transactionReportDeadline，在最后锁查询后、COMMIT前复核trusted receipt绝对deadline；A实现真实captureReportBudget，未新增DB字段或重置预算，最终该红门仍须root复核。

C稳定stage实际FD门通过，root严格复制reports.go 76b3e42c7f2c7c7c7c0f6b69d37f9e9f05a407296456e5c3377384ec4859536f和report_test.go 46c659265ad9efb2c0636f4d25c3cb928bda7c8d5387ac1aa5ef901afd538cb4。初始仅共享Run迁移调用缺同步导致编译受阻，不冒称Stage行为红。有限同fd原字节/Size/SHA/metadata/8MiB/取消与非法边界root目标测试PASS1.383s。root复用原stage body，junit独用O_RDWR，hash/fsync后真实解析，再排他发布/目录sync与短Store Verified提交；原用途路径保留。上传受原2m与普通剩余deadline较小者限制、500ms真运行权/预算核对。原实际Artifact HTTP回归PASS1.747s；新JUnit完整HTTP由C继续实际检查，不提前宣称通过。

README与DELIVERY已明确本地报告增量、实际示例examples/local-reports.yml直接取8case实际执行过的替换失败夹具。README事实发现由A/B/C并行核对；README技能依赖的accelint-english-manager在两skills根/plugins实际搜索不存在，最终英语润色步骤未执行，中文事实状态按AGENTS授权同步。

## Store最终实际批与Agent首个整链（进行中）

A最终17owned文件严格按manifest reports019-parser-store-final.json SHA927f2d679efbee435e87ecd5a482e7c086abf5fe632ffe47252d9b4d11bf9a69逐字节复制。含真实API/SQL rollback/PG末尾锁查询/共享128文件4GiB配额/原命令原因/零动作/失锁与Retry/Recover；生产仍junit1d6d、Store379a。root实际独立双库命令go test -p 1 -count=1 ./internal/store -run 'TestReports|TestReportUploadBudget|TestReportEvidence|TestEnqueue.*Reports' PASS6.649s。PG末尾锁查询跨100ms budget现在拒ErrBudgetInvalid并回滚候选/cursor；终态包含精确report manifest才有StopKnown，Retry继承Reports定义而新revision/IDs/seq/seal全空，Recover保持原sealed证据和预算，不重解析post XML。A完整双库Store/race23.891/116.584s与整理后Reports/race6.923/45.085s是分区证据，root整功能全量仍未跑。

B实际Store/HTTP→固定Git checkout→唯一Run→普通artifact与final XML共用上传seq→post改写→终态→中央原XML下载，原缺consumer真实红13.362s。root先精确copy Agent reports5529/teste890，串行接journal concrete checkpoint、清LocalReports、final checked ACK后沿原uploadArtifacts、匹配完整新增purpose/revision/key，root真实该整链PASS3.157s。随后B发现private artifact phase/ref/name损坏seal仍接受的真实红，owned最小加强完整元数据：root复制reports.go5633802575f2b4f652c04ab383923b29cc9f4a04cd820b01e26003ee017c1325与reports_checkpoint_test.go d8f3d8ed7599870c8decd050b368d59589911676ddadd750f77090733fa413f1，后续根复验继续；没有第二Run或假artifact步骤。

公开Trigger报告门在完整具体consumer已接通后才解除：trigger_report_test.go原真实unsupported红0.763s；root仅去掉Reports专用unsupported，go test -p 1 -count=1 ./internal/server -run '^TestActualTrigger' PASS5.483s，新用例确认报告系统模板待定不等同when跳过、skipped不占编号/不造结果、推进HEAD后相同key仍冻结原SHA；原角色/upload/参数/整批失败门通过。当前外部三实际二进制双库应用夹具app019-salckc27已经构建并开始执行；尚无最终结果，不记PASS。

## 双库三实际二进制检查点及恢复修复（进行中）

首外部app019-salckc27实际SQLite92门PASS，PG在最后错误assertion“所有journal文件必须为空”失败；实际剩余唯一record是未领取Ref=null、无Started/artifacts/reports/pendingEvent的空claim，不是执行残留。保留failure.json与runner-correction.json；只修夹具判定，不改产品、不删旧记录。新独立App/DB app019-verified-pcztwytv重跑，SQLite/PG各92门PASS，共184；403条实际命令/UTC/退出码包含verified HTTPS/CA/私有身份、8真实报告cases、20幂等同批编号、HEAD推进仍原SHA、原XML下载/失败post不替换、admin/approver/trigger/node权限、Agent退出后中央下载、重启及第二控制端拒绝。证据evidence.json SHA6db11d190eff773b27565e1eede519a7101d2bfa4f5f8d8aca25c59738954f34；123生产文件当时source-manifest.json SHAa861d9003cebb92b9020e5f42d1d64c8748462569e6b7824d2bbf0d9f2607115。二进制SHA：client c2c6b2b38189ca41d0ed1fbf92a2c245b4b2fd9a1fe16ce5cfcb13bf1cec0887；server135de734942a88c077629221e2539b3f7f4b661cae27da22dff8bb48bc6915f7；agentfc750d0ebb929829b815c365959badaf8172f4bc2833cf6ecefb172def026c6f。此检查点后Agent恢复validator与CLI展示又改，最终源码仍须必要复验，不把旧二进制冒充最终版本。

A只读审计发现新增Reports私有checkpoint未核初始一致性：旧无manifest三红真实4.579s；真实Run→XML上传→seal→中央terminal丢ACK后counts/file_metadata/confirmed_metadata/checkpoint_missing四红真实10.248s，manifest缺失已有digest保护为绿，原完整恢复1.975s为绿。独立reports_recovery_test.go SHA24cd9f3c0959c6376f75df07da1255fbb02ae05cae9f9e512871d8c340025107根逐SHA复制。B owned修具体terminal canonical/fullmeta核对，sealed/terminal复用confirmedReportFiles，无新框架。root串行在readTerminalJournal复用build_finished的只读checkSealed分支；精准复制reports.go517af72737da3b1f5f8fd5ca3b86ebf18b0d3fc7eb67cb4bd588531f1810fe52与checkpoint testcecc9108e129e0b65439838b3c09d5f474f3da4c9a75264eaa5b3abb522ead77。root实际恢复/ServeHTTP/Checkpoint目标组PASS17.408s；不一致证据原样保留，完整旧/新终态精确receipt才清本条，没有重执行或改中央状态。

C最小CLI两源及本地/远端/下载tests按SHA精确复制：sealed摘要Table含Counts/ns/seal/来源ID，artifact列表含真实用途/revision/key；现JSON消息自然承载同真实字段，无新report命令。root go test -p 1 -count=1 ./internal/cli/client -run '^TestReports' PASS0.739s；同一regex在server无tests匹配，明确不计HTTP验收，随后以实际TestReportHTTP/Stage命名重新执行。C新HTTP正负例与进一步stream边界继续root复核。真实Linux资源目前只准备，尚不记报告验收通过。

## 最终源码冻结与补充门（2026-10-04 UTC）

B最后4文件按完整SHA逐个验证复制：collector ab45a604a4dfb15de9d03edb2bdbf7ff6e50906e7567040437ebba8429f68185、真实source竞争测试51b24dd988e774af2e14bdd45fa40142c5796b3e54d6c0747764cb92ba46d97e、IO/context测试e6ce0e90a90fc009e4db11c69ab0f3d47273c2046334c1ca07bb117a5231e9d7、附加实际Run门4d422620ef5efd48379e8ef618817630f3c94bc181fdc1d5079e057405eb9900。真实open FD/offset证明源替换、保mtime改字节和Root读中关闭；旧末尾IO误分report_invalid的实际红1.120s、deadline分支误分invalid/save两红0.484s保留；仅修真实context/IO分类，不扩大权限或预算。每pattern required、--step prepare排旧XML原inode/hash/mtime保持、post退出/取消保持原报告失败、实际日志保存失败闭锁均有具体测试。root完整报告pipeline组 go test -p 1 -count=1 ./internal/pipeline -run '^TestReport' PASS3.067s。

C8文件冻结manifest reports019-c-http-cli-manifest.json SHAa0abd612ff9d9cec37d57629d41a691e0e292588a099d153b081469d628e5f35。root HTTP/Stage/声明组真实PASS3.734s；追加binding测试SHA02703e94a6f47deaf1c506e7b9addd4e128d06a24b11773c0ffb9ddc3fd1e911后HTTP全组PASS3.963s。旧revision/epoch/source/未知或重复JSON字段均拒；SQL真实失败候选不可见、不推进seq，流式预算/失权真实EOF与有界退出。C精确目标race server10.164/client2.612s及vet/diff PASS；不是root全量替代。

最终生产与测试源码冻结在 /tmp/mybuilds-mvp.zKtK0e/reports019-final-3y4liwqb/source-manifest.json；本机三二进制与Linux ARM64 Agent均实际编译。最终双库nominal、真实Linux报告及双库故障门将使用这批二进制；此处仅记录启动验收，尚无最终PASS。

最终三实际二进制nominal验收已通过：reports019-final-3y4liwqb/evidence.json SHA177cc6075718adeb8c7b8dbb12bc32e9807644053f06e5634c1fba1c9e7efd25，SQLite/PG各92断言、424命令。最终client/server/agent SHA分别59ecb2e18d87ebd5e7f6ec650c42b017fbcc5051517411712b58f19804c11186、97f4a902429361dffc81ef2fcbf350c77697ebda34712a43a84fd9a77819e2f7、40c1fe1c6e14b2d962985944a7384548ca2a661b5150577b02d1ce34ec1e7100；版本均dev(commit unknown,built unknown)，不冒充安装或已提交版本。

根最终go vet ./... exit0。首轮go test -p 1 -count=1 ./... exit1：Agent119.186s的实际恢复manifest_missing子项在触发阶段返回400(<nil>解码错误)，其他Agent/全部包未报告失败；未设置PG DSN故Store只覆盖SQLite，不声称本轮全量双库。此实际失败保留，正在定向复现原因，整功能全量门未通过；没有放宽生产进程安全边界或修改断言。

全量唯一恢复trigger400门定向真实复跑5模式全部PASS8.605s（manifest_missing1.51s）；尚不能判定第一次失败原因，也不以定向PASS替代全量。

root只读映射核对发现Reports实际预检查缺口：prefix{{workspace}}/result.xml会把私有root嵌入相对path，启动用户动作后Store因缺该fact拒绝。真实新增TestReportRunRemoteWorkspaceFactRejectedBeforeActions红1.129s；root仅在reports build层fact副本删workspace，步骤原workspace不变。此补丁前nominal184、Linux8与matrix均作为检查点保持，不冒称最终字节。

private workspace报告模板负门修正后PASS0.459s。这是一项执行前fact副本删除，不影响run.env中真实workspace与step.name；契约显式记build层报告可用事实。原T038完整确认恢复已测，但reports_checked/sealed/JUnit PUT三种丢ACK尚待外部夹具执行，T038重新未勾选，不用旧artifact ACK或终态腐败门代替。

最终workspace预检查修正版完整matrix已PASS：reports019-final-matrix-fy3da73l/evidence.json SHA98e52d49159a5bf62e4c1e6de66ea54a59e927ce032e0e0839ccb7ddeb61825d，UTC21:32:33–21:32:47；source-manifest SHA210072b4b8f4d2898cdfe84267a2a91eb0d6b78278261e2fe89abeabd5a3eb13，279条Go源及mod/sum前后相同。CGO0 Darwin amd64/arm64、Linux amd64、Windows amd64×三入口12次编译exit0；最新本机三入口help/version共6exit0，未知命令/余参共6exit1，foreign未执行。根最终vet/diffcheck/gofmt/mod/sum均PASS。旧um5矩阵SHA435c103...3450保留，属于修正前检查点。根最终完整双库Store组PASS24.529s，原普通Store运行不含PG的记录未改。

## Linux真实节点最终交付

A安全export /tmp/mybuilds-mvp.zKtK0e/linux019-prep-6ueljrr8/runtime/safe-export/manifest.json SHA18c2c3dc032c1fcd4267ff890c8736ffded64607702e0371ddbe94b64c191e95；root独立复算68文件全部Size/SHA与安全相对path通过。五主证据共75named checks：八case601–608属于workspace预检查修正前checkpoint，固定e21e66d、实际HEAD推进、8独立workspace、4原XML中央逐bytes/Size/SHA/Counts/Source/canonicalseal、post不可改/权限/离线下载均通过。取消609亦为此前checkpoint：真实shell14088/child14090同PGID/birth，497连续samples证明本次Gone上界早于ordinary.finished.At 61.298ms，终态后2.045s与自有无关进程全存活；精确Ref/seq/digest只读StopKnown receipt及取消原XML字节通过，不借延迟推断。

最新source-manifest精确210072b4...a3eb13与ARM Agent SHA414335923763bb1c6345c3f7d8d14083e754a34f09ba30444117edfc1b8ca7f0的新session：610静态通过、611–613声明秘密raw/全数字实体编码/相对路径三面实际Run全failed/report_secret、sealed/Files[]/Counts0/中央Artifact列表空。36实际Nodeevents/log请求原bytes、编码及解码三面扫描安全，CLI日志/公开视图/hostServer和guestAgent日志安全。新固定fixtureGit dd46d9f1e230bca0643339ceb995f7fb9572faf0，受信SSH52947，未改原Android夹具。

最新Agent14121/birth4421078用pidfd归属精确停止，/proc gone且journal0；停后中央新XML下载仍通过。旧13310/birth3676900仍活，旧204/207保护未触。独立控制端88943/backend54867与verifiedTLS54868仅供只读核对；其Server是旧checkpoint SHA97f4...e2f7（本轮变更只在Agent调用的Run报告预检查，控制端业务未改），没有冒称全部旧结果是新三binary最终字节。原driver不存在session_id字段与harness argv规范化两次夹具失败保留，只修夹具并继续读原构建，没有重触制造PASS。

根最终normal双库第二轮仍exit1：仅旧006TestLockLostDuringTransactionRollsBack/postgres失败mid-transaction loss:nil；Agent完整118.440s及其它包通过。定位旧测试pg_locks查询未限定自己的database/session，能误中止C另一自有库preparedserver锁会话；C实际control_lock_lost日志同期保留并已精确停自己服务。生产write固定s.conn/checkLock正确。另建SpecKit缺陷postgres-fixture-session评估并隔离仅test修复，未将该失败删除或称产品修复已验收；根race进行中，故障窗口资源已隔离。

根最终完整双库race实际PASS：2026-10-04T21:37:05–21:44:12，go test -race -p 1 -count=1 ./... exit0，Agent133.577s/client26.560s/process36.225s/server27.624s/Store119.752s。全部生产和测试源当时仍精确freeze210072...，记录final-race-dual.json SHA064b5d977e31f123c1cc19016f527758453f6a8550da86ae521fc3cee4a2140c，完整日志SHA392a7d42a43bb992a99cc5ea97c2a030c8c0967acf4966d1bc9c2d465524d462。随后仅复制独立缺陷的lock_test.go SHA626f1d047c9a27e877cd9a3ff7e30443df84894f7b3004289551eacbb21af6ce（交易前取自身session PID），生产/CLI编译源保持完全相同，更新后的源码manifest单独保存source-manifest-after-test-fixture-fix.json；不将测试修正冒称生产变更。正在最终normal双库复验。

## 最终全量普通检查与数据库夹具独立修复

UTC 2026-10-04 21:46:03.814927–21:50:22.525288，根在最终源码、独立 PostgreSQL `mybuilds019_tests` 上执行 `go test -p 1 -count=1 ./...`，exit=0；SQLite与PostgreSQL同时覆盖，Agent 118.352s、Store 25.607s。命令记录 `reports019-final-3y4liwqb/final-normal-fixed-fixture-dual.json`，输出日志 `final-normal-fixed-fixture-dual.log`，实际文件摘要见下。此前竞态检查的生产源码与本次一致，唯一差异为数据库测试夹具锁会话定位，不重复声明全量竞态覆盖了改动后的测试文件。

实际全量检查暴露旧测试从所有数据库会话选择 advisory lock owner，可能误终止另一自有验收数据库控制端；按 Spec Kit bug-assess→bug-fix→bug-test 独立修正为本测试 Store 的 `pg_backend_pid()`，保持原锁丢失与事务回滚断言。目标双库、全量Store和Store race（75.608s）均通过；并行自有控制端锁会话10657始终相同、锁仍授予、HTTPS状态200。具体被旧夹具误选的PID未捕获，不补写猜测。独立验收提交 `b2e659a2d81c3da2caf66588922b95daf8c9df32`（fix(test): 限定数据库故障夹具自身会话），只包含测试文件及三份缺陷记录，019尚未提交。

- `final-normal-fixed-fixture-dual.json` SHA256 `9e2d3928a794f259a2cc9600d29e66703046e134ecea07e07ce935a11b7b0a2a`。

- `final-normal-fixed-fixture-dual.log` SHA256 `c476b52f0eec84f2ae6797c435ebc08ba107115b71a9653e3e6b17f6a420f082`。

- `source-manifest-after-test-fixture-fix.json` SHA256 `3c27549969af300b29350d50472fdad544f86944916c438668607d3ed0c5767a`。

## 需求与行为证据定位

下表区分源码行为测试和真实应用检查点；尚待完成的故障组与最终收敛不计为通过。任务中的parser/Store/Run/HTTP/CLI文件都由其唯一writer交付并逐SHA核对，原共用入口由root串行接入，无新依赖或第二Run。

| 规范 | 已实现消费者及行为验证 |
|---|---|
| FR001、FR008；US2.AC3 | config/validate.go、config/template.go、preview.go；ReportsStrictSchemaAndRequired、ReportsPreviewValidatesWholeSelection、RunRemoteWorkspaceFactRejectedBeforeActions；整批模板先验，非法输入无用户动作 |
| FR002；SC001、US1.AC1 | reports/junit.go实际token parser、MergeJUnit；ActualCasesAndNestedCounts、AllowedStructuresAndZeroCases、RejectsInvalidSemantics、ExactBoundaries；本地与双库应用原XML/Counts逐一对应 |
| FR003–006；SC001–002、US1.AC1–2、US2.AC1–2 | pipeline/reports.go与唯一Run；RealReplacementAndSnapshot、FreshnessAndMissing、NewIdentitySameBytes、RequiredEachPatternKeepsActualCounts、SelectedPreparationKeepsOldXMLExcluded；实际8场景与中央替换/旧XML检查点 |
| FR007；US4.AC1 | Agent原checkout/journal/Reports消费者，Store真实Source/Ref/Revision约束；CheckedRequiresTrueStoppedSource、HTTPCurrentRevisionAndRunSourceFence；实际macOS/Linux固定SHA、独立工作区、多build及原HEAD推进 |
| FR008–010；SC002、SC005、US2.AC3 | 普通fd/Root稳定快照、parser完整原字节及解码秘密检查，server稳定stage重解析；实际FIFO/替换/keepmtime/Root.Close、诊断截断后秘密、HTTP拒坏摘要；Linux raw/实体编码/路径三面秘密不上传 |
| FR009、FR014–015；SC004、US1.AC3、US4.AC1–3 | Store最终用途与seal过滤、server原下载、client现有artifact命令；ActualJUnitFinalRevisionAndOriginalDownload、HTTPSealedOriginalAndRoleBoundaries、CLICentralDownloadAfterNodeDisabledAndRoles、CLIDownloadShortAndDigestMismatchNoOutput；三二进制离线中央原XML下载 |
| FR011–012；SC003、US3.AC1–3 | 唯一Run逐ordinary检查/final检查→原XML确认→seal→post，Store intent/post/terminal校验；RunFailureBlocksSentinelAndPreservesCommand、PostFailureAndCancelPreserveOriginalReportReason、SealRecomputesActualParsedEvidence、PostCannotResumeUnfinishedCollection；实际准备/失败普通哨兵/failure/always/post-pass不改封存 |
| FR013；SC005、US4.AC3 | 原ordinary预算/Authority、Agent原journal和有界回执；RemoteBudgetBoundsCheckpointPersistence、AuthorityAndSaveFailureClosePost、ActualLogSaveFailureClosesReportAndAlways；实际SQL末尾失锁/到期、HTTP慢流预算/失权，实际checked/PUT/seal丢ACK与快照保存失败20场景通过 |
| FR015；SC004–005、US4.AC2 | ReportsCommitRealSQLFailureIsInvisibleAndCanonicalRetry、FinalLockQueryCannotExtendReportBudget、CheckpointTerminalRequiresCanonicalAndConfirmedMetadata、RecoveryActualSealRejectsCorruptCheckpoint；真实完整terminal丢ACK/精准只读恢复不重执行，部分文件与假summary不能清journal |
| FR016–017；SC004–005 | 相同Store SQLite/PG与三二进制应用；双库nominal各92断言、Linux75 named checks及68文件复算；最后20故障192断言通过，未用编译代替实机 |
| FR018；SC006 | 已有Spec Kit规范/plan/tasks/analyze与红绿记录、README/实施历史；全量普通/竞态/vet/12编译已通过，整功能converge/提交作为最后交付步骤 |

未配置报告的旧wire omitempty/digest和无扫描/无报告事件门、零动作不假seal及既有007/008全量回归已核对；RetryStartsFreshAndRecoveryRetainsFrozenSeal证明原配置与固定SHA继承、报告新Revision/文件/Seal/cursor不继承。原商店/审批/upload仍拒绝，不以019封存代替后续真实发布验收。

## 整合后独立只读源码核对

A以当前18FR、6SC、12AC、46任务及原则2.1.0五项核对全链：严格配置/预览→唯一Run/报告集合→Parse/Merge→Agent checkpoint/只读恢复→稳定stage→双库checked/文件确认/seal/post/terminal→用户详情/下载与008Retry/Recover。新增实质源码缺口0；这是正式converge前的源码核对，不代替最后故障门或正式收敛。

实际定位：`run.go:312`报告build事实去workspace；collection `reports.go:97`基线、`:468`按路径替换/required、`:635`只私有快照seal；Store `reports.go:140`真实ordinary run来源、`:294`下步/post门、`:352`统一receipt绝对预算、`:481`服务端已解析原文件重算与完整终态manifest；Agent `reports.go:107`canonical/fullmeta、`recovery_journal.go:63`同validator及原完整网络digest；Server `artifact.go:136`上传预算/Authority。无新依赖、第二执行器、发布stub或借005未验收源码。A全程只读，无额外Go/Agent/VM运行。

## 最终双库三二进制20故障场景

C实际私有verified HTTPS/固定可信Git、自有SQLite与PostgreSQL `mybuilds019_faults_dphhjxf`，当前最终三二进制：client SHA66141dc00be51606c547fc6918884368e5db77d05d5885dfbf64fdc545abcf63、server SHAe38f1ed1ac9dbb062b58bd511e48052f0b70aecde40302b677a02cf245dea6ea、agent SHAe534393c902fb8a4ecb04709d43fe94138df3bed39d4ff37d54a21821a1ef075（均dev/unknown，未冒充发布版本）。每库10场景、85实际断言通过，最后只读22断言通过，总192；完整消息/1013命令与逐场景UTC/Git SHA/身份归属在 `reports019-faults-dphhjxf_/final-evidence.json`（1254953B，SHA18f4e4c9baeb9bb3a380ac97becfcfabee5d67e54daaf0fb478f29f431f24206）。最终安全manifest（16329B）SHA2a68e695547ef06f83c67b1e6f56bfda5fc64416b0e594796c01a4eac2376df1，root独立复算18文件全部大小/SHA和安全相对路径一致。

- reports_checked、JUnit PUT、reports_sealed、build_finished各在真实中央提交后丢ACK。原事件Seq/Digest/剩余预算重发，PUT用原ID及完整Ref只读恢复已确认receipt；普通与always脚本各一次、仅真实两次报告检查revision与一个最终文件，完整封存原XML可中央下载。终态丢ACK实际Agent重启仅核对原Ref/Seq/Digest/完整manifest，无第二Run，预算单调不重置。
- ordinary与always各真实取消/禁用。记录本次实际PID/birth/PGID并核ESRCH，无关自有进程存活；取消保原cancelled，禁用为interrupted/authority_lost，不写假的完整报告终态。禁用原独立停止事实精确确认后guard解除，不把StopKnown当报告结果。
- ordinary exit0/exit7后实际新XML已生成，再阻断私有snapshot目录（ENOTDIR）。StopConfirmed且无CleanupFailed，下一普通与always不执行，没有可信报告seal/完整terminal。exit0中央最终interrupted/lease_expired，exit7原exit原因保留且中央interrupted/exit；未确认journal与guard均保留，不冒称failed/report_error或自动确认。
- 最后只读复核原XML字节/Size/SHA、精确terminal receipt和预算/当前revision、公共日志/XML与Node消息的声明秘密扫描；未执行新Run。实际Go1.25.4、macOS27.0、PostgreSQL16.14，原锁定SQLite模块不变。自有服务均Popen.wait确认退出，根既有服务/旧保护未触。

夹具失败原样保存：GET回执URL query断言、disable误加不支持的--json、默认shell -e遇wait137导致尚未生成XML、早期PG准备锁会话受旧测试误选。仅修夹具，未命中保存门的旧构建不计通过；已成功原构建只读继续，不重触制造通过。旧unknown的journal/guard保留，不删除。实际回执与文件失败由C外部三二进制消费者覆盖T038，原任务预计reports_receipt_test.go未新增，现有Agent recovery/checkpoint测试与该外部证据共同验证同一真实机制。

## 正式收敛与提交检查

实际speckit-converge初始化一次、读取唯一意图及原则，36规范验收项/9实施决定/5原则/46原任务全核对，missing/partial/contradicts/unrequested均0；tasks前后字节SHA相同，未追加任务。收敛记录由implement交付保存[convergence.md](convergence.md)。工作区只包含019相关实现与本目标后续规划，提交只暂存019实现、对应规范与例子、README/DELIVERY/实施历史；后续009–012/014–015/020规划不随019暂存。无新依赖、不push。

## 2026-10-08 数量增量验收

基线 main@21116f7；不访问真实凭据、不改用户部署、不执行商店发布。扩展019，默认256份，max_files=1–1024；普通制品128份独立计数，XML/累计字节/cases/诊断/纯检查预算保持。MaxFiles省略时JSON也省略，旧冻结定义编码不因新增nil字段改变。

- 红：TestReportsMaxFiles 在新字段落地前编译失败（缺FileLimit）；默认与1/256/1024、0/负数/1025/null/字符串/小数/bool/重复字段绿。
- 本地真实XML：256/256、256/257、65/65、64/65、1024/1024、1024/1025，增量扫描、基线超限与checkpoint恢复通过；完整边界组10.06s，其中1024收集+恢复+final共7.58s，非声称单阶段耗时。
- Store SQLite：128普通制品与256报告共存、4GiB拒绝不推进序号、8MiB/64MiB/10万cases和显式1024证据边界、发布SQL先过滤报告后Limit均通过。未配置MYBUILDS_TEST_POSTGRES_DSN，本次未跑真实PostgreSQL；SQL使用两库支持的COUNT/CASE。
- 元数据实际HTTP：1024份1024字节路径（含Go HTML转义）事件大于1MiB、完整严格解码/Store接纳和审批摘要通过；超过8MiB返回413。大journal实际原子写读/启动iOS前置解析、超过64MiB不覆盖旧文件、CLI大于1MiB完整响应/超过8MiB拒绝均通过。
- 首次真实1024上传已达1025文件，但最终确认仍沿普通128-ID上限失败；已改共享ID验证器调用的有效限额，发布报告ID亦同配额。首次大规模审批恢复栈定位到每候选反复计算完整审批摘要，约百万次JSON编码；保留校验、按本次Ref复用结果后重新验证。失败/主动终止定位记录不算通过。
- 最终实际Run+Git+Store+HTTP：`go test -p 1 ./internal/agent -timeout 8m -run 'TestServeApprovalMaximumReports|TestServeActualJUnitMaximumFiles' -count=1 -v` exit0；审批1024份暂停→退出→重启→批准→封存终态102.82s，普通1024份+普通/post制品→原XML逐份下载97.29s；包202.153s。覆盖原工作区/固定SHA不重跑、审批报告不伪final、终态journal精确清理。
- `go vet ./...` exit0。全量回归分组执行：上述两个大量真实案例已完成，其余用 `MYBUILDS_CLIENT_TOKEN='' go test -p 1 ./... -timeout 15m -count=1 -skip 'TestServeActualJUnitMaximumFiles|TestServeApprovalMaximumReports'`；隔离客户端token以避免本机已部署服务影响旧doctor测试。全量、必要race与构建结果随后补录，当前不把运行中标为通过。

FR019→config/tests；FR020→pipeline/Agent/Store/manifest/publish/retention与混合文件真实案例；FR021→protocol/server/HTTP/journal全部消费者与长路径案例；FR022/SC007→README/配置/规范/任务及本节最终检查。无新依赖或第二执行器。

### 最终回归与检查结果（2026-10-08 11:16 UTC）

全量按两组覆盖，保留首轮实际非零结果：全量命令的其余包全部通过（client54.578s、pipeline18.472s、server83.527s、store27.031s等）；Agent312.895s首轮两项失败。审批夹具使用的自定义token变量被进程MYBUILDS_CLIENT_TOKEN空值覆盖，已改用标准变量并限定单构建容量；退出时须等真实204空claim已在本地移除，再在下一轮询前取消，不删除unknown证据。普通审批连续两次exit0（37.52s、24.13s）；恢复confirmed_metadata单独exit0（2.24s），未放宽原12秒退出边界。最新完整1024审批案例再跑exit0（93.87s，包96.586s），最大普通构建/逐份原XML下载仍沿上文实际202.153s组合记录。全套测试均有通过记录，首轮字面命令没有被改写为exit0。

必要race按相关真实消费者验证：config2.374s、Agent10.195s（含实际小型Run/HTTP/下载、metadata journal及完整确认反例）、server6.591s、client2.025s通过。首轮将最大案例与race并发运行，1024纯检查在race下超过原10s、Store无续租配额夹具超过租期，该轮非零；不增加生产预算。最大规模独立普通模式实测；pipeline的256/65/64配置收集与恢复race5.349s通过，Store配额夹具每32文件沿真实Renew保留同Ref与原预算，配额/证据/发布筛选race24.137s通过。

最新go vet ./... exit0；CGO_ENABLED=0三CLI在linux/amd64与windows/amd64共六次构建exit0（仅编译，不声明原生Windows Agent支持）。没有新的依赖或发布/部署动作。README、配置与019契约/数据模型/研究/quickstart已同步；diff检查通过。

配额续租夹具修正后的普通Store配额与发布筛选复验exit0（4.413s）。正式speckit-converge核对22FR/7SC/5用户故事、原9项与增量5项plan决定、五项原则及52任务，零缺口；tasks前后SHA256相同，收敛未写入任务。T052的记录、勾选与本地提交属于implement交付收尾，详见[convergence.md](convergence.md)。
