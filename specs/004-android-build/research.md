# 004 研究与锁定决策

日期2026-10-04；仅依据官方一手来源。

## 工具链与环境

决策：验收工程锁AGP8.11.1、Gradle8.13、SDK35/build-tools35.0.0，JDK17/21，不跟随最新。
[AGP8.11官方兼容表](https://developer.android.com/build/releases/agp-8-11-0-release-notes) 要求Gradle8.13/build-tools35/JDK17，最高支持API36。实际java为GraalVM21.0.2，另有17.0.15/21.0.8；SDK平台25–36和对应build-tools、Gradle8.13/AGP8.11.1缓存已存在；本工作树尚无wrapper或Android工程。

替代：全局Gradle和自动SDK安装会改变用户环境，不采用。doctor只报告可用SDK包，项目完整需求由实际构建核对。

## wrapper、缓存和网络

采用官方wrapper脚本/jar/properties，工程固定distributionSha256Sum；缓存沿用默认位置，不clean或全局--stop。
[官方wrapper指南](https://docs.gradle.org/current/userguide/gradle_wrapper.html)支持分发及jar校验。从官方services.gradle.org取得：
- Gradle8.13-bin SHA-256：20f1b1176237254a6fc204d8434196fa11a4cfb387567519c61556e8710aed788。
- Gradle8.13-wrapper.jar SHA-256：81a82aaea5abcc8ff68b3dfcb58b3c3c429378efd98e7433460610fecd7ae45f。

首次缺失Maven依赖仅从Google/Maven Central下载固定版本到缓存，复验--offline；不安装SDK。[官方SDK说明](https://developer.android.com/studio/intro/update#download-with-gradle)要求android.builder.sdkDownload=false禁用自动下载，模板和示例均显式禁用。

## 版本、签名和mapping

env通过providers.environmentVariable由示例工程读取，设置versionName/versionCode，不假设任意工程消费-P。依据：[应用模块设置](https://developer.android.com/build/configure-app-module)、[应用签名](https://developer.android.com/studio/publish/app-signing)、[R8](https://developer.android.com/topic/performance/app-optimization/enable-app-optimization)。release明确启用minify产生mapping。

[JDK21 keytool](https://docs.oracle.com/en/java/javase/21/docs/specs/man/keytool.html)支持-storepass:env/-keypass:env；-list检查条目，-certreq签署请求验证私钥可用，不修改keystore。用独立JKS测试不同store/key密码；不使用未知用户密钥。

[apksigner](https://developer.android.com/tools/apksigner)验证APK；AAB使用jarsigner与真实manifest核验，不假称APK工具验证AAB。

实际采用已锁AGP8.11.1运行时加载的官方bundletool模块1.18.1，临时Gradle init脚本获取Android插件classloader，调用FlagParser和DumpCommand.fromFlags(...).execute()读取真实AAB快照manifest；不调用会System.exit的BundleToolMain，也不解析protobuf或ZIP字符串。

实际jar源路径显示com.android.tools.build/bundletool/1.18.1；SHA-1为76bb1aa21a19495ca070bfb6abd194daf91dccd1，与[Google Maven官方摘要](https://dl.google.com/dl/android/maven2/com/android/tools/build/bundletool/1.18.1/bundletool-1.18.1.jar.sha1)一致；实际SHA-256为a73341a7945abcb0e6b8971c7b1b2801bd765006447ca0d2437a4260d572ceac。源码和调用可查[官方bundletool](https://github.com/google/bundletool)。

可选独立[官方bundletool1.18.2 CLI](https://github.com/google/bundletool/releases/tag/1.18.2)的发布SHA-256为378b5434cd1378bef6b2bc527b8c7f0ff2584b273830335bce54d6d0813c8584。实际下载31MB过慢，有界45s尝试失败，另一完整尝试已仅取消本次会话；未成功下载不作为证据，不再要求它安装或下载，也不将jar提交仓库。

## 执行与接口

共享已冻结：AndroidTemplate、AndroidDoctor、DoctorCheck{Name,Status,Version,Reason}；Status passed/failed/skipped。公共toolCommand{Workspace,Executable;Args,ExtraEnvNames}和toolOutput由根提供，32KiB合并输出、15s单工具预算、限定env及固定错误。提取现有process避免mobile→pipeline循环依赖，不另建执行器。
