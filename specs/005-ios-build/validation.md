# 005 实施与验证记录

日期：2026-10-04。状态：**实现中／待真实验证**。前置003已验收b9d23a1，复用004已提交2ab8991的公共process/doctor；本分区未提交或push，整功能由主代理验收后提交。

## 流程与检查

已执行项目Spec Kit specify→plan→tasks→analyze→implement→converge。requirements质量检查5/5，扩展hooks={}。接口更正为共享passed/failed/skipped后重跑只读analyze：14FR+5SC、11原任务，覆盖100%、无阻塞。Converge检查19FR/SC、12故事验收场景、7计划决策及5原则，追加T012–T014；三项HIGH partial均为缺少指定真实Apple材料的验收工作，没有原则冲突或未经需求支持的新框架。

| 实际命令 | 结果 |
|---|---|
| `go test ./...` | PASS，CLI/config/mobile/pipeline/process |
| `go vet ./...` | PASS |
| `go test -race ./internal/mobile ./internal/pipeline ./internal/process` | PASS |
| `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./cmd/mybuilds ./cmd/mybuilds-server` | PASS |
| `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./cmd/mybuilds ./cmd/mybuilds-server` | PASS |
| `CGO_ENABLED=0 go test ./internal/mobile -run 'TestIOSHelper\|TestIOSTemplate\|TestIOSResources\|TestIOSValidation\|TestIOSDoctor'` | PASS，原生签名明确不支持，其他能力可用 |
| `go test ./internal/mobile -run 'TestIOSNativeKeychainPrototype\|TestIOSMaterial\|TestIOSDoctor\|TestIOSResources\|TestIOSTemplate' -v` | PASS，真实原型及边界 |
| `MYBUILDS_IOS_UNSIGNED_TEST=1 go test ./internal/mobile -run 'TestIOSUnsignedArchive\|TestIOSTemplateShellAndVersion' -v` | PASS，实际Xcode无签名archive与dSYM |
| `git diff --check` | PASS |

首轮全量测试遇到旧Android初始化负例，与004已支持Android冲突；主代理同步更正为unknown平台后最终全量通过，本分区未修改共享测试。

## 已运行的真实机制

- Xcode27.0（27A266a）及iphoneos工具链只读核对；当前导出method使用debugging/release-testing/app-store-connect/enterprise，不自动下载签名材料或上传。
- 原型仅生成自己的RSA codeSigning证书/P12与MachO。P12密码及自产keychain密码通过匿名stdin/原生内存参数传入，不写配置、脚本argv或公共输出。
- 真SecKeychainCreate/Unlock、指定keychain的SecPKCS12Import、错误密码半创建后SecKeychainDelete、匿名`security -i -q` partition、明确SHA1身份的codesign与strict验证均通过。
- 用户default/search list前后相同，原型keychain及-db路径均无残留。第二枚自有keychain替换原叶文件后，IOSResources.Close因inode不符返回ErrIOSCleanup并保留替换文件；恢复原叶后正常原生删除。
- Validate使用macOS15起公开kSecImportToMemoryOnly；无keychain/profile/outputdir创建。自产自签CMS在固定Apple锚处拒绝，不通过CN/privatepolicy接受；预检查前后用户列表不变。
- 相对Workspace及相对P12/profile由真实子进程回归验证：Workspace规范化后读到正确材料，无双拼接。profile安装字节必须与helper返回的已验证SHA256相同；P12导入使用同一读取的字节。
- 自签profile导致Prepare半失败后，自有工作区OutputDir及独立TMPDIR内临时目录无残留。取消已发生时Prepare不创建目录。被替换目录/profile内容保留并报清理失败；重复Close幂等。
- FIFO及材料叶symlink被明确拒绝；FIFO实际非阻塞返回。helper未知字段、多JSON对象、超限输入均安全拒绝，不回显敏感标记。
- doctor停止未确认会保留固定failed清理原因并停止后续工具/材料调用；整体15s预算，无资源时仅工具摘要/身份数量与skipped材料，不选宿主身份。
- 自产单app Objective-C工程实际执行iphoneos无签名archive，得到App.app及对应dSYM；模板的真实归档Info.plist核验正确Bundle ID、1.2.3版本与42构建号，错号拒绝；ditto压缩dSYM非空。模板两个run的Bash语法及非法版本边界通过。

## 真实验收门

没有收到用户明确指定的工程/scheme、配套P12/profile、Bundle ID及密码环境引用。未扫描未知工程或导入已有宿主应用身份。以下仍未验收：

1. 实际Apple profile signer是否具备当前保守marker；固定官方DER根/CMS/signature用途/团队/证书/期限联合校验的实际材料兼容性（T012）。
2. Xcode archive规划和export能否在不加入用户search list的前提发现显式临时identity；help未提供export专属keychain参数，直接codesign原型成功不能推出Xcode兼容。需要真实工程archive/export、签名IPA与对应dSYM及快照摘要（T013、SC-002）。
3. 合法材料下成功/普通失败/准备中取消/归档导出取消/post失败后的独立系统Close、日志敏感值及用户其他资源/进程不变（T014、T008/T010、SC-003/004）。已有半失败和机制负例不替代这些组合。

因此T008/T010/T011及Convergence任务仍未勾选，不能报告005整功能完成或以unsigned/自产证书替代Apple签名IPA。

## 末轮外部临时目录边界修复

主代理把003既有结果临时目录检查提取为 `process.TemporaryDirectory(workspace,prefix)`，pipeline与IOSResources两个真实消费者复用。IOSResources不再直接信任宿主TMPDIR，也没有新增第二套路径helper。TMPDIR位于工作区或内部symlink时回退工作区外；实际Prepare用自签profile半失败后确认输出已清理、仓库内TMPDIR仍为空，外部正常TMPDIR同样无残留。

最后最小修复后执行：`go test ./internal/mobile ./internal/pipeline ./internal/process -run 'TestIOS|TestTemporaryDirectory' -v` 与 `go vet ./...` 均PASS（包含真实native原型、relative材料、替换/half-cleanup、pipeline四个真实负例与共享临时目录回退）。全量/race/跨编译表记录的是该目录复用修复前已通过的检查，主代理整合后负责最终全量复验。复核后资源边界MUST已满足，Converge仍为T012–T014三项真实材料工作；没有把该修复或自产原型当作SC-002验收。

## 主代理根目录最终复验

005代码与规范已集成到根目录分支 `005-ios-build`，保留未验收状态、没有提交。末轮外部临时目录复用修复之后，以下检查在根目录真实通过：

| 命令 | 最终结果 |
|---|---|
| `go test ./...` | PASS；mobile 28.012s、pipeline 4.794s、process 10.446s、client 3.179s |
| `go vet ./...` | PASS |
| `go test -race ./internal/mobile ./internal/pipeline ./internal/process ./internal/cli/client ./internal/config` | PASS；mobile 39.785s、pipeline 8.078s、process 11.489s、client 3.837s、config 3.163s |
| `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./cmd/mybuilds ./cmd/mybuilds-server` | PASS，编译不代表真实Linux节点验收 |
| `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./cmd/mybuilds ./cmd/mybuilds-server` | PASS |
| `CGO_ENABLED=0 go test ./internal/mobile -run 'TestIOSHelper\|TestIOSTemplate\|TestIOSResources\|TestIOSValidation\|TestIOSDoctor'` | PASS，0.838s |
| `git diff --check` | PASS |

真实Apple材料验收仍对应既有T012–T014，未以复验通过替代签名IPA/真实生命周期门；T008/T010/T011保持未完成。

## 根真实客户端验收

独立证据目录 `/tmp/mybuilds-cli-005-accept.oc7tioai`，实际二进制的14/14断言通过（commands.json、assertions.json和分命令输出）。帮助隐藏helper；无材料doctor返回工具passed与材料skipped（Xcode/SDK27.0）；init与五参数dry-run成功、重复init拒绝且文件字节不变；签名环境未设置/设置两次预览JSON相同，不生成资源。隐藏helper匿名stdin畸形输入/未知动作只返回固定 `ios_helper_input_invalid`。两build整批在后一个明确不存在签名材料时拒绝，前一个普通脚本哨兵未出现，stdout空、无产物或结果目录。未使用宿主profile/私钥，这些负例不代替真实签名验收。

## 2026-10-05 只读迁移准备

新 worktree ios005-current 基于已验收 fee97e8ea32fc4f582abfb445c8d33f690f6f70e。旧 ios005-pending 全部文件与原证据保持原字节；以上测试均为旧版本历史记录，本轮未运行 Go/Mac Run/钥匙串或材料工具，不新增PASS，不改变T008/T010/T011/T012–T014未验收状态。

纯组件优先：config新增 ios_signing.go/test，Build五字段指针、Node结构检查、现有Validate接具体函数；Preview继续现有config.RenderField，系统ios.output_dir pending且丢弃CallerFacts伪值。009消费这一相同schema/预览组件，不重新定义签名schema。未接系统生命周期的执行入口保持明确拒绝。

平台迁移新文件共21个：internal/mobile 的 ios.go、ios_apple_roots.go、ios_doctor.go、ios_files_darwin.go、ios_files_other.go、ios_signing.go、ios_signing_darwin.go、ios_signing_other.go、ios_template.go，以及 ios_doctor_test.go、ios_files_darwin_test.go、ios_native_test.go、ios_signing_test.go、ios_template_test.go、ios_test.go、ios_unsigned_test.go；templates/native-ios.yml 与 examples/native-ios.yml；config/ios_signing_test.go、pipeline/ios_test.go、cli/client/ios_test.go。纯Preview测试应从旧混合pipeline测试拆出并适配当前测试入口，不能覆盖现有TestMain。mobile代码消费Root实际stdin，不复制旧工具wrapper。

共享最小差异由Root串行接：process.Command/toolCommand.Stdin为[]byte，非nil才bytes.NewReader；TemporaryDirectory提取当前逻辑；config.validate/template、Preview、CLI init/doctor/隐藏helper与唯一Run生命周期。当前Agent需自身隐藏helper、taskSecrets的三个明确引用、真实诊断能力，以及准备前持久native资源所有权/关闭证据。旧内存IOSResources不足以证明Agent崩溃后已清理；process_group_reaped不能代替原生Close。Store固定reason/真实Started与StopKnown需兼容当前完整Ref、receipt、manifest、报告、累计预算；不复制旧run/process或新增第二executor。

允许后续纯解析/预览先红绿、无cgo/平台编译；Mac机制与unsigned工程待明确窗口再执行。完整Apple材料兼容、真实签名IPA/dSYM/版本/摘要及成功/失败/取消/清理未知门仍待T012–T014，不读ambient keychain或未知材料。


### 当前纯组件实际记录

2026-10-05 fresh worktree 执行 setup-plan/setup-tasks，selector选择本功能，hooks={}，质量checklist5/5；只读analyze输入前后相同，14FR/5SC/12AC、23唯一任务覆盖100%，5原则均映射，0CRITICAL/HIGH。旧pending原文件未修改。

证据目录 `/tmp/mybuilds-mvp.zKtK0e/ios005-component-2sr_vlqw`。先用原Parse运行TestIOSSigning，合法配置实际因未知字段RED；加入schema但未接Validate后，非法值和直接构造对象实际RED。Root真实接入ValidateIOSSigning后，`go test -count=1 -json ./internal/config -run '^TestIOSSigning'` GREEN 0.346s；包含五top及其正反例。中途新增参数测试一次误用Select签名导致编译失败，已更正为当前Select([]string,bool)真实消费者，未以此替代原行为RED。

mobile平台18文件已按现有工具helper最小适配byte stdin；`go test -run '^$' ./internal/mobile` 首轮只编译仍缺process.TemporaryDirectory而失败，原日志保留。等待Root提取真实共享方法后重验，未用stub，未运行任何Mac工具/native/signing/Run。此处仅记录组件阶段，不代替T012–014。


### 五字段组件冻结（导出后的当前字节）

具体函数直接改名为 config.ValidateIOSSigning，由当前Validate及后续Preview/Run消费，无wrapper或第二规则。Root实际validate调用接入后重新执行：schema五top正常PASS1.044s，race PASS1.461s，config vet exit0；这些都是纯配置检查，没有工具/文件材料读取。实际mobile仅compile PASS1.043s；TestIOSTemplatePureConfiguration PASS0.280s，使用真实Parse/Select/ResolveParams验证五必填、默认method、普通run/artifact与模板字节隔离，不执行shell。

`ios005-component-2sr_vlqw/component-freeze.json` SHA256 0c4df5faae5874cf3518297c701af8a26427dbccfec7ac16ecb51baf27157617，记录22 owned文件测试前后SHA相同以及go.mod/sum。只有T015/016此次完成；T017当前真实Preview尚在Root接线，T018实际stdin/runtime、T019资源机制及完整生命周期不能因纯编译标为通过。旧AppleT012–T014与Mac actual window hold仍保留。


## 2026-10-05 用户验收安排覆盖旧提交限制

用户最新明确：Apple/GooglePlay先全部实现，真实材料/上传由用户人工验收，其他模块完成后统一Flutter案例验证。因此上文旧版本“缺材料不能提交”的限制不再是当前代码交付条件；历史真实记录仍不改写。当前005必须完整实现安全消费者并通过必要自动检查，quickstart备齐人工步骤后可由Root本地提交；合法Apple材料兼容、签名IPA/dSYM及依赖材料的生命周期组合继续人工待验，不能标真实PASS。Mac实际窗口目前仍hold，不能据此安排修改而运行ambient keychain或未知材料。


### 当前移动端实际自动门

Root确认C全部服务退出后放行本次Mac机制窗口。只跑受影响targeted mobile组一次：11top normal PASS7.833s，实际自产keychain/P12错误密码、指定私钥codesign/strict verify/default与search list不变/自有keychain删除与替换保护、FIFO/symlink、目录/profile替换与Close幂等、匿名stdin Bash语法/版本/实际归档Info.plist拒绝错号，以及自产unsigned iphoneos archive+dSYM ZIP。未使用未知Apple材料或宿主app私钥，未跑全process或PID故障suite。

随后仅NativeKeychainPrototype/Resources四top必要race PASS10.210s，mobile vet exit0；不重复unsigned或纯组件。正常和race测试均退出，自有临时keychain清理由真实断言通过，没有未确认取消或原生异常。最新22owned source与前冻结完全一致。

最终外部清单 `ios005-component-2sr_vlqw/mobile-platform-delivery.json`记录全部owned路径/size/SHA、UTC/命令/退出及日志SHA；normal日志aad2f690076400786be7070dc1dd8f238a4c007ace63c601177703968b3f57d7。签名Apple IPA、合法profile兼容及其生命周期组合明确人工待验，自动自产机制不冒充这些结果。Run/Agent完整共享消费者仍由Root接入，此处不把仅mobile自动门当整功能收敛。


## 2026-10-05 当前完整实现与自动检查交付

本节取代旧历史版本的实现状态，不改写历史失败/通过。当前worktree `ios005-current` 基于fee97e8；Root明确释放全部005文件后，本区完成当前Run/Agent/Store/CLI接线，不复制旧执行器。真实Apple材料继续人工待验；用户已明确必要实现与自动检查就绪允许整功能交付。

实际接入：整批预检查、首次普通run前Plan/Preparing intent/Prepare、普通与artifact、用户post、独立15s Close；Close失败保留原原因并停止后续。Agent准备前保存真实目录所有权，准备结束保存叶身份，关闭后终态绑定Ownership SHA；恢复只Close，prepare中未知叶保持闭锁。Store签名快照不能凭process StopKnown解除原生未知guard；旧nil字段JSON省略不改变旧wire。BuildRun私有iosTeamID只取真实资源Environment。

当前证据仍在 `/tmp/mybuilds-mvp.zKtK0e/ios005-component-2sr_vlqw`，没有触碰020/旧pending/原服务。先保存真实compile RED、helper重复/null字段runtime RED、原Agent“永远unsupported”断言失败，以及生命周期fixture的0700/预览必填参数错误；分别修实际生产规则或错误fixture，原失败日志均保留，不冒充全部首轮通过。

| 检查 | 真实结果 |
|---|---|
| 当前IOS targeted九包normal | 138项通过，2个环境不适用skip；Store实际SQLite+PG 1.336s |
| 受影响包normal | config/mobile/pipeline/store/server/cli/client/cli-agent/cli-server/protocol/scm通过；Agent旧能力断言修正后独立全组131项PASS119.426s |
| 受影响六包IOS race | config/mobile/pipeline/agent/client/process全部PASS；120项通过，2个环境不适用skip |
| 有限匿名stdin/所有权恢复/停止保护 | 实际自有文件/临时keychain机制、prepare intent取消、半准备原失败、未知替换闭锁、checkpoint摘要、双库终态和独立Stop真实API门通过 |
| 新签名Store双库race | 3 top、8个SQLite/PG子项、0skip，PASS3.387s |
| 最终 `go vet ./...` / `git diff --check` | exit0 / exit0 |
| 三入口无cgo编译矩阵 | Darwin ARM64、Linux ARM64、Windows AMD64 × mybuilds/mybuilds-agent/mybuilds-server，9项exit0；仅编译不代表平台实际签名 |

完整命令、UTC、日志exit/size/SHA和全部源冻结见最终delivery manifest。必要checks覆盖本次影响；遵Root明确窗口限制不再跑全process/SIP故障suite，原scope/OnStart保留。Go模块依赖未新增。

人工待验仅是合法Apple材料兼容、授权工程真正签名IPA/dSYM/版本/Bundle ID/证书/快照，以及成功/失败/归档导出取消/post组合的真实签名资源清理。quickstart已备齐明确输入和核验步骤；自动自产P12/keychain与unsigned archive不能代替这些PASS。T012–014勾选表示人工指南已交付，未声称实际Apple材料已验证。


### 当前收敛结论

实际执行项目speckit-converge，prerequisite PASS，hooks={}；核对14FR/5SC/12AC、23唯一任务与5原则，0 missing/partial/contradicts/unrequested代码缺口；收敛输入与tasks前后SHA一致，没有追加空Convergence阶段。人工材料执行按FR014/SC005及用户明确安排保留待验，不是本次代码交付阻塞，也没有记录真实Apple PASS。外部 `converge-final.json` 保存收敛输入及映射；Root将完整005最小差异串行合并当前020并执行集成复验后统一提交，本区未commit/push。


### Root 当前020基线串行集成

71个交付文件逐size/SHA核对通过；以2602094为前置最小三方合并，保留020的ResultCreated登记和retention命令，签名IOSCheckpoint并入原Run。九包IOS/nativeIOS/retention资源目标检查含真实SQLite/PostgreSQL通过（Store3.933s、Agent6.644s），日志SHA 2b61e414ccdad8e3bc3d04d9747cbb536735738b041e2c27eb6981e6759c0d6b。原自动normal/race/vet/9编译证据保持，Apple人工门未标通过。实际规范14FR/5SC/12AC/23任务与五原则映射核对，无新增集成缺口；tasks未因收敛改写。无新依赖、无push。
