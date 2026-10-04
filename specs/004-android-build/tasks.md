# 004 Android 实施任务

输入：[spec.md](spec.md)、[plan.md](plan.md)、research/data-model/contracts/quickstart。所有共享文件仅主代理修改，本分区不得并发改写。

## Phase 1：规范与冻结

- [x] T001 在 specs/004-android-build/spec.md、plan.md、contracts/ 完成需求、文件所有权、官方依赖/摘要和接口冻结。
- [x] T002 对 specs/004-android-build/spec.md、plan.md、tasks.md 执行 analyze，主代理复核并确认零阻塞后实现。

## Phase 2：共享基础（主代理）

- [x] T003 提取既有实际进程实现到 internal/process/process.go、process_unix.go、process_other.go，internal/pipeline/run.go直接调用process.Run/HostEnvironment，run_types.go保留Command别名；删除旧pipeline/process_unix.go/process_other.go/同文件测试，测试迁移internal/process/process_unix_test.go并保留run回归，验证取消/后台/正常清理/无关进程/race/Started，禁止第二执行器。（FR-004/011/012）
- [x] T004 在 internal/mobile/doctor.go、doctor_test.go 提供 DoctorCheck、toolCommand/toolOutput，Status严格passed/failed/skipped、单工具15s/合并32KiB/受限九项env加明确ExtraEnvNames；先验证超量输出/超时/取消/非法或缺失env/安全错误，后实现。（FR-001/004/012）

## Phase 3：US1 doctor（本分区及共享CLI）

独立验收：正常与至少六类错误检查项正确，签名未声明skipped不使整体失败，输出无密码。

- [x] T005 [US1] 在 internal/mobile/android_test.go 先写有意义行为测试，覆盖缺/错Java、SDK冲突/缺包、工程wrapper路径/启动错误、未声明/部分声明签名、真实临时JKS的store/key密码错误/alias/非私钥及开始前取消；测试先失败。（FR-001/002/003/004/012；SC-001）
- [x] T006 [US1] 在 internal/mobile/android.go 实现冻结 AndroidDoctorOptions/AndroidDoctor，安全数字版本解析、Java>=17、SDK实际可用包、实际wrapper、签名只读list/certreq，整体30s，固定Reason，不记录输出/密码。（FR-001/002/003/004/012）
- [x] T007 [US1] 主代理在 internal/cli/client/doctor.go、doctor_test.go、root.go 接入doctor --platform android/default与--json/working-dir/wrapper/签名flags；passed/failed/skipped数组，失败非零，不要求远端凭据。（FR-001/003/004/012）

## Phase 4：US2 模板与工程接入

独立验收：模板严格Parse和dry-run成功，仅一个android build和run/artifact，参数显式env，无发布步骤。

- [x] T008 [US2] 在 internal/mobile/android_test.go 先写AndroidTemplate的Parse/选择/env/参数/run原文/输出模式/副本测试，验证version=1.2.3、build_number=42，恶意构建号不被shell执行。（FR-005/006/007；SC-002）
- [x] T009 [US2] 在 internal/mobile/templates/android.yml、android.go 实现嵌入模板/AndroidTemplate，version/build_number默认1.0.0/1、构建号正整数<=2100000000、APP_VERSION/BUILD_NUMBER显式env且不使用保留MYBUILDS_前缀、--no-daemon、禁SDK自动下载、APK/AAB/mapping三模式。（FR-005/006/007/010）
- [x] T010 [US2] 主代理在 internal/cli/client/init.go、doctor_test.go 或现有pipeline_test.go 接入native/android排他创建，测试--template冲突/已有文件/default保持/未知选项失败，无工具或密码读取。（FR-005/012；SC-002）
- [x] T011 [US2] 在 examples/android/README.md、settings.gradle、build.gradle、gradle.properties、app/build.gradle、app/src/main/AndroidManifest.xml、app/src/main/java/com/example/mybuilds/MainActivity.java、gradlew、gradle/wrapper/gradle-wrapper.jar与properties 创建最小独立工程，官方wrapper校验，AGP8.11.1/Gradle8.13/SDK35/build-tools35.0.0，明确读取env和启用R8/signing，无新增业务依赖。（FR-006/007/008/010）

## Phase 5：US3 真实工程证据

独立验收：真实APK/AAB为1.2.3/42且签名通过，非空mapping，快照全部一致；二次离线成功，错误签名/取消无本次残留。

- [x] T012 [US3] 在 specs/004-android-build/validation.md 记录临时工程与JKS的真实doctor/构建命令、实际工具版本、APK/AAB应用身份/版本/签名、mapping内容及003快照大小/SHA256，运行生成配置与可编辑自定义任务；未执行项列待验证。（FR-008/009/011；SC-003）
- [x] T013 [US3] 在 specs/004-android-build/validation.md 记录二次--offline真实构建缓存复用、SDK未变、错误store/key签名构建失败、真实构建取消后的本次组清理及无关进程存活；诊断不含密码。（FR-004/009/010/011；SC-001/004）

## Phase 6：集成与收敛

- [x] T014 运行 go test/vet/race及Linux/Windows编译，更新 specs/004-android-build/validation.md、quickstart.md，执行converge；主代理同步README.md、docs/IMPLEMENTATION_HISTORY.md，全部真实验收后一次本地提交。（FR-012与全部SC）

## Dependencies & Parallel Opportunities

T001→T002→T003/T004；T005→T006依赖T004，T007依赖T006；T008→T009，T010依赖T009；T011可与主代理T003/T004/T007/T010在不同文件并行，但同一android_test.go的T005/T008由本分区串行写。T012依赖所有功能代码与CLI集成，T013随后，T014最后。

## Implementation Strategy

先完成doctor行为及模板可预览增量，再真实工程完整验收；缺环境保留待真实验证，不缩减要求。自有文件测试先失败再实现；根对共享能力执行同样安全行为检查。不按任务提交、不push。
