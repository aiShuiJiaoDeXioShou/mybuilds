# 任务：005 原生 iOS 构建

输入 spec/plan/contracts；共享接口已冻结、实施可做部分完成，真实Apple产物由用户人工验收。资源缺口不缩减SC-002/003，不阻塞完整实现与必要自动检查后的本地提交。

## Phase 1：规范与研究
- [x] T001 形成 specs/005-ios-build 的规范、质量检查、官方研究与共享契约草案（FR-001–FR-014、SC-001–SC-005）。
- [x] T002 主代理只读分析、冻结 specs/005-ios-build/contracts/go-api.md 的配置/上下文/DoctorOptions/系统预算与 internal/process stdin，同步依赖文件。

## Phase 2：US1 体检与模板
目标：只读诊断与初始化；独立 test 为有效模板/无副作用预览、缺失/坏工具与材料。
- [x] T003 [US1] 在 internal/mobile/ios.go、ios_test.go、ios_template_test.go 实现参数/文件与profile metadata规则并验证实际模板版本边界；先写边界测试（FR-001–FR-004、FR-008、FR-012、SC-001/004）。
- [x] T004 [US1] 在 internal/mobile/ios_doctor.go、ios_doctor_test.go 实现 IOSDoctor，复用主代理 tool helper；不扫未知 profile、不选择宿主私钥（FR-001/002/012、SC-001/005）。
- [x] T005 [US1] 在 internal/mobile/templates/native-ios.yml、ios_template.go、ios_template_test.go、examples/native-ios.yml 生成 IOSTemplate，本区接 init/doctor CLI（FR-003/007–FR-009、SC-001/004）。

## Phase 3：US3 签名安全原型与生命周期
目标：真实自有 keychain、半失败清理；独立 test 为系统资源与用户 default/search list不变。
- [x] T006 [US3] 在 internal/mobile/ios_signing_darwin.go、ios_signing_other.go、ios_signing_test.go 用受管隐藏helper及原生内存调用实现 keychain/P12/CMS固定Apple锚+profile marker 检查、自签profile拒绝与错误密码原型，按冻结预算方式验证（FR-002/005/012、SC-003/004）。
- [x] T007 [US3] 在 internal/mobile/ios_signing.go 与对应测试实现具体 IOSResources，Validate无副作用、排他profile副本/OutputDir目录/ExportOptions/有限匿名stdin partition/幂等与独立半准备Close/ErrIOSCleanup；本区接 pipeline 独立系统生命周期（FR-004–FR-007、FR-009–FR-013、SC-003/004）。
- [x] T008 [US3] 在 internal/mobile/ios_signing_test.go 与当前 pipeline/CLI 集成测试检查成功/失败/取消/post失败/资源替换/清理失败，保留原原因并阻止后续动作（FR-010/011/013、SC-003/004）。

## Phase 4：US2 实际构建
目标：真实签名 IPA/dSYM与版本；独立验收为指定工程的 archive/export/签名/快照。
- [x] T009 [US2] 在 internal/mobile/ios_template_test.go 和 specs/005-ios-build/validation.md 验证 unsigned编译、归档实际版本检查、导出方式与隔离输出；不把unsigned当签名成功（FR-007–FR-009、SC-002/004/005）。
- [x] T010 [US2] 在 specs/005-ios-build/quickstart.md 写齐明确授权工程/P12/profile/Bundle ID的IPA/dSYM与失败取消人工步骤，validation.md标人工待验；收到材料后由用户执行真实验证（FR-005–FR-011/014、SC-002–SC-005）。

## Phase 5：集成与收敛
- [x] T011 全量 test/vet/相关race/跨编译并在 specs/005-ios-build/validation.md 记录；执行 converge，修复代码/自动检查缺口，真实Apple项标人工待验并备齐步骤，本区更新README，Root最终串行整合实施历史与本地提交（全部SC、FR-014、原则I/V）。

## 依赖与并行
T001→T002→T003；T004/T005 可不同文件并行，但本区唯一写入者。T006 先原型，T007 依赖 T006/主代理process/config分区唯一writer同步，T008依赖真实pipeline接入。T009依赖模板/签名机制，T010人工指南依赖T008/009实际接口；用户执行才依赖合法材料。T011必须保留真实人工待验记录，必要自动门通过后可实现交付；不以工具或样例编译替代真实签名结论。

实施策略：先安全原型与只读工具，再模板与共享接入，必要自动检查后完整实现交付，真实Apple产物/组合取消留人工验收；本worktree共享文件已全部释放给A，不同时编辑doctor/config/pipeline/CLI；Root最终串行合并。

## Phase 6: Convergence

- [x] T012 在 specs/005-ios-build/quickstart.md 备齐合法Apple profile/P12的固定根CMS/marker兼容人工验证步骤，validation.md明确人工待验；用户验证后按真实证据最小修正内部校验（FR-002、US1/AC2，人工待验）。
- [x] T013 在 specs/005-ios-build/quickstart.md 备齐明确授权工程archive/export、临时identity、版本/BundleID/IPA/dSYM/快照人工步骤，validation.md明确人工待验；不可用unsigned或自产证书冒充（FR-005/007/009、SC-002，人工待验）。
- [x] T014 在 specs/005-ios-build/quickstart.md 备齐合法Apple材料成功/失败/取消/准备中取消/post失败的独立Close人工步骤；自动自产机制/失败与取消门先完整通过，validation.md明确哪些组合人工待验，不阻塞代码交付（FR-010/011、SC-003/004、T008/T010/T011，人工待验）。

## Phase 7：当前已验收基线移植

本节执行于 fee97e8 基线的新 worktree；上文已勾选任务为历史版本证据，不代表此版已通过。以下勾选表示当前实现、自动检查或人工指南已就绪；真实Apple执行结果另列人工待验，不由指南勾选代替。

- [x] T015 在 specs/005-ios-build/plan.md、contracts/go-api.md、validation.md 更新实际基线、分区、阶段与 stdin []byte，执行现有 setup-plan/setup-tasks 和只读 analyze（FR-013/014、SC-005、原则I–V）。
- [x] T016 [US1] A 先在 internal/config/ios_signing_test.go 建立当前 Parse/Validate 的真实拒绝门，再新增 ios_signing.go、types.go/parse.go 最小接线；Root 在 validate.go 调用实际函数，验证五字段、类型、引用、模板、默认/必填参数与安全错误，无环境解引用（FR-003/004/008/012、SC-001/004）。
- [x] T017 [US1] A 在 internal/config/template.go 和 internal/pipeline/preview.go/ios_preview_test.go 接同一 RenderField，严格检查五字段、丢弃伪 ios.output_dir、无值pending、无外部动作；冻结005纯组件供009 T003消费，生命周期未接阶段的 run 明确拒绝，当前完整接入后开放（FR-004/007/012/013、SC-001/004）。
- [x] T018 A 在 internal/process/process.go、process_unix.go、internal/mobile/doctor.go 接 Stdin []byte 与当前结果临时目录逻辑提取；真实有限输入/null stdin兼容/取消与安全错误测试，不覆盖现有 scope/OnStart（FR-005/010/013、SC-003/004）。
- [x] T019 [US3] A 在 internal/mobile/ios*.go、对应测试及 templates/native-ios.yml、examples/native-ios.yml 移植平台代码，消费实际 shared stdin/TemporaryDirectory；保持 fixed Apple anchors、材料读取限制与五必填模板参数，不创建第二执行器（FR-001–FR-009/012/013、SC-001/004/005）。
- [x] T020 [US3] A 在 internal/pipeline/run.go 与 CLI client/agent 隐藏入口、internal/agent/secrets.go、Store实际事件原因接首次Prepare→ordinary/artifact→用户post→独立Close；整批预检查/真实Started/预算/报告兼容/cleanup保原原因并停止后续，不能复制旧Run（FR-004–FR-007/010–FR-013、SC-003/004）。
- [x] T021 [US3] A 在 internal/agent/journal.go、recovery实际路径及doctor.go 接具体native资源所有权与Close确认；准备前持久、crash不重跑、不将物理Stop替代Close、失权仅清自有资源，未知闭锁；真实恢复/租约/报告receipt兼容测试（FR-010–FR-013、SC-003/004）。
- [x] T022 [US2] A 在 internal/mobile/ios_native_test.go、ios_unsigned_test.go 及 specs/005-ios-build/validation.md 记录明确窗口内当前版本机制/无签名工程门；Mac实际动作hold解除后才运行，不将自产证书/unsigned计为T012–014（FR-002/005/008/011/014、SC-004/005）。
- [x] T023 在 specs/005-ios-build/validation.md 记录此版本全量/相关race/vet/矩阵及T012–014人工步骤及真实待验状态，A完成README和整功能converge，Root串行整合并提交；缺Apple材料明确人工待验，但完整代码与必要自动检查就绪允许实现交付（全部FR/SC、原则I/V）。

移植依赖：T015→T016→T017 为纯组件批；T018→T019 为mobile适配；T016/T017/T019完成后才T020→T021；T022需明确Mac窗口，T012–014人工步骤需T020/T021实际消费者契约；用户真实执行需合法材料，T023需完整实现、必要自动门和人工步骤就绪。A 的纯config与Root stdin不同文件可并行；Root明确释放后本worktree同一共享文件只A写，Root最终串行整合。009仅消费T016/017已冻结组件；完整执行消费者在实现与必要自动检查通过后交付，真实Apple签名仍人工待验。
