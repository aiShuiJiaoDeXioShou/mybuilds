# Android 验收工程

一个仅依赖 Android SDK 的 Java 应用，用来验证 mybuilds 的版本参数、release 签名、APK/AAB、R8 mapping 和产物快照。

## 环境

需要已安装的 Go 1.25+、JDK 17 或 21、Android SDK platform 35 与 build-tools 35.0.0。工程锁定 Gradle 8.13 和 AGP 8.11.1；首次缺缓存时会从官方源下载 Gradle/Maven 依赖，不安装或更新 SDK。`gradle.properties` 明确禁止 SDK 自动下载。

设置 `JAVA_HOME` 和 `ANDROID_HOME` 指向已有工具；若设置了 `ANDROID_SDK_ROOT`，它必须与 `ANDROID_HOME` 指向同一真实目录。不要把未知用户密钥复制到这个测试工程。

## 准备临时副本

在 mybuilds 仓库根目录执行，后续只修改临时副本：

```bash
android_demo="$(mktemp -d)/project"
cp -R examples/android "$android_demo"
go build -o "$android_demo/mybuilds" ./cmd/mybuilds
cd "$android_demo"
```

生成仅用于此示例的临时签名。下面的密码是公开测试值，不用于商店发布：

```bash
export ANDROID_KEYSTORE="$PWD/test.jks"
export ANDROID_KEY_ALIAS=mybuilds_test
export ANDROID_KEYSTORE_PASSWORD=android_example_store_7391
export ANDROID_KEY_PASSWORD=android_example_key_9163
"$JAVA_HOME/bin/keytool" -genkeypair -storetype JKS \
  -keystore "$ANDROID_KEYSTORE" -alias "$ANDROID_KEY_ALIAS" \
  -keyalg RSA -keysize 2048 -dname 'CN=mybuilds Android example' -validity 30 \
  -storepass:env ANDROID_KEYSTORE_PASSWORD -keypass:env ANDROID_KEY_PASSWORD
```

## 检查与构建

```bash
./mybuilds doctor --platform android --json \
  --keystore "$ANDROID_KEYSTORE" --key-alias "$ANDROID_KEY_ALIAS" \
  --store-password-env ANDROID_KEYSTORE_PASSWORD --key-password-env ANDROID_KEY_PASSWORD
./mybuilds init --framework native --platform android
./mybuilds run --dry-run --param version=1.2.3 --param build_number=42
./mybuilds run --param version=1.2.3 --param build_number=42
```

doctor 报告实际 Java、已安装 SDK 包和工程 wrapper 版本；完整声明签名后检查私钥条目及两个密码。未声明签名时是 `skipped/not_declared`，不会假称签名有效。基础检查不解析任意 Gradle 脚本的 AGP/compileSdk 要求，完整兼容性由真实构建核验。

init 生成可编辑的 `builds.android`，只有普通 run/artifact，不添加上传或审批。已有 `mybuilds.yml` 拒绝覆盖。run 成功后的 JSON 给出临时 `result_dir`、独立快照路径、大小和 SHA-256；stdout 是结果，stderr 是 UTC 脱敏日志。

## 工程如何消费参数

[app/build.gradle](app/build.gradle) 使用 `providers.environmentVariable` 显式读取：

| 环境变量 | 用途 |
|---|---|
| `APP_VERSION` | `versionName`，示例默认 1.0.0 |
| `BUILD_NUMBER` | `versionCode`，正整数且不超过 2100000000 |
| `ANDROID_KEYSTORE` | keystore 路径；相对路径按 app 模块解释，示例使用绝对路径 |
| `ANDROID_KEY_ALIAS` | 私钥 alias |
| `ANDROID_KEYSTORE_PASSWORD` | keystore 密码 |
| `ANDROID_KEY_PASSWORD` | 私钥密码 |

模板把 `version/build_number` 参数映射到 `APP_VERSION/BUILD_NUMBER`，签名变量通过明确宿主引用传入。`MYBUILDS_` 前缀为流水线上下文保留；本地构建号是用户参数，不伪造远端计数器。任意 `-Pversion` 不会自动改变用户应用，需按此工程示例接入 Gradle。

默认任务为 `:app:assembleRelease :app:bundleRelease`，默认路径为：

```text
app/build/outputs/apk/release/*.apk
app/build/outputs/bundle/release/*.aab
app/build/outputs/mapping/release/mapping.txt
```

本工程 release 开启 R8，产生非空 mapping。普通未混淆工程应删除 mapping 模式，或将其拆为有条件的独立 artifact 步骤；每个有效产物模式零匹配都会失败。模块名、flavor、任务、工作目录和输出路径不同的工程应编辑 YAML；产物路径始终相对仓库根目录。

## 核对包与缓存

```bash
"$ANDROID_HOME/build-tools/35.0.0/aapt" dump badging app/build/outputs/apk/release/app-release.apk
"$ANDROID_HOME/build-tools/35.0.0/apksigner" verify --verbose --print-certs app/build/outputs/apk/release/app-release.apk
"$JAVA_HOME/bin/jarsigner" -verify app/build/outputs/bundle/release/app-release.aab
export APP_VERSION=1.2.3 BUILD_NUMBER=42
./gradlew :app:assembleRelease :app:bundleRelease --no-daemon --offline -Pandroid.builder.sdkDownload=false
```

APK 的应用标识应为 `com.example.mybuilds`，版本为 `1.2.3/42`。APK 与 AAB 都须验证签名；临时自签名证书不表示商店发布授权。AAB manifest 的实际版本核验和快照摘要命令见 [004 验收指南](../../specs/004-android-build/quickstart.md)。

再次 `--offline` 构建验证缓存已齐全；不会清空共享 Gradle 缓存或停止其他 Gradle daemon。取消 mybuilds 构建使用 Ctrl-C，既有执行器负责终止本次进程组；失败时保留日志与已有快照。

官方 Gradle wrapper 保留上游 Apache-2.0 许可头，分发和 wrapper jar 摘要锁定见 [研究记录](../../specs/004-android-build/research.md)。项目开发约定见 [AGENTS.md](../../AGENTS.md)。
