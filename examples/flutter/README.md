# Flutter 示例工程

工程来自 Flutter3.38.6/Dart3.10.7 实际创建结果；源码和 pubspec.lock 保留真实解析结果，尚未验证签名构建。Gradle8.14、AGP8.11.1、Kotlin2.2.20、SDK36/NDK28.2.13676358/JDK17+，工具和缓存须先明确准备，不自动安装或接受许可。

在此目录使用 `mybuilds init --framework flutter --platform android`。Android keystore 使用明确绝对路径和环境引用：ANDROID_KEYSTORE、ANDROID_KEY_ALIAS、ANDROID_KEYSTORE_PASSWORD、ANDROID_KEY_PASSWORD；不在配置或命令参数写密码。

Android 示例预配置 production/staging 两个 flavor，分别应用标识 dev.mybuilds.mvp_flutter 与 dev.mybuilds.mvp_flutter.staging。工程仅在APP_FLAVOR非空时声明已准备变体；空值走标准默认任务，production对应默认身份，staging追加.staging。使用 `--param flavor=staging` 与对应application_id切换，不把已存在变体包充作默认产物。

模板单独核 APK/AAB 的应用标识、版本、可信编号和明确 JKS 证书，默认收集 build/mybuilds-flutter 两个已核叶文件。示例 release 明确启用 R8，可在 artifact.paths 追加对应 build/app/outputs/mapping/productionRelease/mapping.txt 或 stagingRelease/mapping.txt；无 R8 工程不要声明 mapping。

需要仓库脚本参数示例时，在 release 前追加普通 run `./verify-parameters.sh`，只使用已声明 env。channel 即使包含空格或 shell 字符也是一个数据值，不通过 eval 执行。

iOS 目录从实际生成工程增加共享 staging scheme、三种 -staging configuration和Podfile映射；默认Runner与staging分别对应dev.mybuilds.mvpFlutter及追加.staging。CocoaPods1.16.2及实际Podfile.lock仍须在明确依赖环境解析，不伪造锁定文件；明确scheme和合法Apple材料后，使用005真实代码提供的Validate/Prepare/Close以及009的手动archive/export模板。尚未有本示例的合法签名IPA/归档/dSYM证据，真实签名与统一案例由用户最后人工验收，代码交付不代表通过。

渠道通过单个 `--dart-define MYBUILDS_CHANNEL=...` 实参传入 Flutter 编译，示例 UI 实际读取该常量；值含空格或 shell 字符仍作为数据。
