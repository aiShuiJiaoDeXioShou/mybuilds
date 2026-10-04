# 实施计划：原生 Android 构建

**Branch**: `004-android-build` | **Date**: 2026-10-04 | **Spec**: [spec.md](spec.md)

## Summary

具体 Android doctor 和可编辑内嵌 YAML 复用 run/artifact。独立真实工程消费环境参数，核验 APK/AAB、签名、版本、mapping、缓存和取消。主代理独占共享接入，本分区只修改列明的 Android 文件。

## Technical Context

- Go 1.25 标准库，无新增 Go 依赖。
- 验收工具锁定 Gradle8.13、AGP8.11.1、compile/target SDK35、build-tools35.0.0；JDK17/21。用户工程使用其自身 wrapper。
- 存储：只读嵌入模板、临时复制验收工程及测试 JKS；产物沿用003快照。
- 测试：Go 行为测试、真实 keytool、真实 Gradle 产物和签名、race/vet/既有取消回归。
- 平台：Linux/macOS 实际执行，Windows可编译并明确不支持本机执行。
- 预算：doctor整体30s，公共helper每工具15s且继承更早ctx截止；合并输出上限32KiB，超限主动取消本次进程组。
- 范围：一个最小app模块release，无商店/远程能力和新DSL。

## Constitution Check

| 原则 | 设计前/后结论 |
|---|---|
| I规范驱动 | 先specify/plan/tasks/analyze；缺真实证据保持待验证 |
| II单模块与节点 | mobile/pipeline共用提取的process实现，CLI薄接入 |
| III最小依赖 | 无新Go依赖、仅官方锁版工具，不建interface框架 |
| IV输入执行 | 密码显式env，诊断固定code、输出和预算有界、仅本次组取消 |
| V中文验收 | 中文文档/注释，真实签名和产物；根同步README并一次提交 |

全部门禁通过。hooks={}，无前后钩子。

## Project Structure

### 文档

specs/004-android-build：spec/plan/research/data-model/quickstart/tasks/validation、contracts/go-api与cli、checklists/requirements。

### 本分区所有权与最少工程方案

```text
internal/mobile/android.go
internal/mobile/android_test.go
internal/mobile/templates/android.yml
examples/android/{settings.gradle,build.gradle,gradle.properties,README.md,gradlew,.gitignore}
examples/android/gradle/wrapper/{gradle-wrapper.jar,gradle-wrapper.properties}
examples/android/app/build.gradle
examples/android/app/src/main/AndroidManifest.xml
examples/android/app/src/main/java/com/example/mybuilds/MainActivity.java
```

SDK Activity即可，不加AndroidX/Kotlin；gradle.properties禁止SDK自动下载；官方wrapper锁校验和。keystore/local.properties/.gradle/build/mybuilds.yml均在临时副本生成，不提交。

### 主代理独占共享文件

```text
internal/process/{process.go,process_unix.go,process_other.go,process_unix_test.go}
internal/pipeline/{run.go,process_unix.go,process_other.go,process_unix_test.go,run_types.go}
internal/mobile/{doctor.go,doctor_test.go}
internal/cli/client/{root.go,init.go,doctor.go,doctor_test.go}
README.md
docs/IMPLEMENTATION_HISTORY.md
```

process提取既有实际取消/Wait实现，pipeline/run.go直接调用process.Run和HostEnvironment，旧pipeline/process_unix.go、process_other.go及原同文件测试删除；run_types只保留shellCommand类型别名，测试迁移process。不能第二套executor。mobile不能import pipeline，避免后续pipeline导入mobile循环。共享文件由主代理串行修改/同步/复验。

## Implementation Design

1. Java实际执行并严格解析数字版本，最低17；SDK来源ANDROID_HOME/ANDROID_SDK_ROOT，冲突失败；验证已安装数字平台的android.jar及数字build-tools中的工具，报告安全包版本，不解析任意GradleDSL。
2. GradleWrapper默认gradlew，工程内安全相对路径、可执行普通文件，验证wrapper jar/properties并调用实际wrapper --version --no-daemon。返回严格匹配数字版本，不回显原输出。
3. 全空签名为skipped；部分声明failed。完整声明用keytool -list验证PrivateKeyEntry，用-certreq验证私钥密码；密码:env，经helper明确变量名注入；不写keystore，CSR只内部捕获。
4. AndroidTemplate返回嵌入YAML副本。builds.android参数version/build_number默认1.0.0/1，显式env映射；run执行./gradlew :app:assembleRelease :app:bundleRelease --no-daemon -Pandroid.builder.sdkDownload=false。模板签名来源明确宿主env变量，不预览读取密码。
5. 版本不能为空，构建号限制正整数且<=2100000000，shell引用均加引号。工程显式读取APP_VERSION/BUILD_NUMBER与签名env；MYBUILDS_前缀仅既有受限上下文所有，不声明或伪造远程构建号；release混淆/任务/路径接入文档清楚。三个artifact模式均强制普通文件匹配。
6. 验收真实APK使用aapt/apksigner核对metadata/签名；AAB复用锁定AGP8.11.1运行时加载的官方bundletool模块1.18.1（Google Maven来源/校验摘要见research），临时Gradle init脚本调用DumpCommand dump manifest，再jarsigner验证签名；不新增下载要求，不能用ZIP字符串替代真实metadata。官方独立bundletool CLI可选但不是验收依赖。非空mapping必须包含示例类。
7. 取消/失败与日志快照复用002/003；不clean、不停止无关daemon、不更新SDK。第二次--offline真实构建验证缓存。

## 实施与并行策略

共享契约已冻结；先完成analyze并通知根，根提供实际helper/process迁移。Android自有模板/示例可与根CLI并行；doctor编译依赖helper同步。集成后真实验收与converge，根同步README/历史并一次提交。

## Complexity Tracking

无原则例外，不新增注册框架、ToolRunner interface或执行器。
