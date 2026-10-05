# 009 CLI 与配置契约

> 2026-10-05用户最新交付边界：先完成完整可执行代码与必要自动检查。005签名API只等真实代码冻结交接，不等合法Apple材料人工验收；真实签名／双平台／最终联验保留全部原要求，由用户最后统一人工验收并独立记录pending。源码集成或提交不宣称这些门PASS。无stub、第二Run、宿主未知身份或安全降级。

## 本地初始化

```text
mybuilds init --framework flutter --platform android
mybuilds init --framework flutter --platform ios
mybuilds init --framework flutter --platform android,ios
```

只创建当前目录mybuilds.yml；不新增init --file选项，不创建工程、脚本、密钥或SDK。平台列表拒空/重复/未知项，规范组合顺序android、ios。已有文件排他拒覆盖；--template与任一显式framework/platform冲突；无参default、native及自定义模板名称保持。生成的YAML只有普通run/artifact，用户可编辑。

## 本地与远程doctor

```text
mybuilds doctor --framework flutter --platform android --working-dir PATH --json
mybuilds doctor --framework flutter --platform ios --working-dir PATH --json
mybuilds doctor --framework flutter --platform android,ios --working-dir PATH --json
```

framework默认native，platform默认沿旧android；Flutter双平台只检测所选集合，Flutter/Dart公共项只出现一次。Android签名flags沿004（keystore/key-alias/store-password-env/key-password-env），iOS材料flags沿005（p12/profile/password-env/bundle-id/export-method），不改名或猜材料；未交接材料CLI时不创建假doctor，显式材料请求固定unsupported且不取秘密，005实际代码交接后才真实检查。声明不完整/跨不相关平台选项固定拒绝；没声明签名单独skipped，不能总体假失败或宣称签名可用。

Flutter/Dart名称为flutter.sdk/flutter.dart；原生check名字沿原合同，pod为flutter.cocoapods，所选实际工具补充项使用flutter.android_java/flutter.android_aapt2/flutter.android_apksigner或flutter.ios_xcode；沿已有native wrapper/签名诊断，避免同名重复。Status passed/failed/skipped，任何failed非零；新增Flutter工具项为数字点Version，旧native SDK/platform摘要保持既有契约，各项使用固定Reason，不输出原始版本正文/机器路径/credential。cancel/timeout/cleanup_error保持各自真实原因，不继续不安全后续工具。

现有doctor --server/--node不允许与本地framework/platform/工程签名flags混用，远程仍读取既有安全NodeReport；不新开HTTP路由。客户端本地doctor不写用户Flutter配置、不查询Xcode账户、不接受许可证。

## 可编辑配置形状

```yaml
version: 1
builds:
  android:
    runner: {platform: android, framework: flutter, labels: [flutter]}
    params:
      version: {default: "1.0.0"}
      channel: {default: internal}
      flavor: {default: ""}
      build_number: {default: "1"}
      application_id: {required: true}
    env:
      APP_VERSION: "{{version}}"
      APP_CHANNEL: "{{channel}}"
      APP_FLAVOR: "{{flavor}}"
      BUILD_NUMBER: "{{build.number}}"
      ANDROID_KEYSTORE: "${ANDROID_KEYSTORE}"
      ANDROID_KEY_ALIAS: "${ANDROID_KEY_ALIAS}"
      ANDROID_KEYSTORE_PASSWORD: "${ANDROID_KEYSTORE_PASSWORD}"
      ANDROID_KEY_PASSWORD: "${ANDROID_KEY_PASSWORD}"
```

iOS同一build模型；必填xcode_project/scheme/bundle_id，version/build_number等Flutter默认沿上表；ios_signing沿005的p12/profile/password完整env引用及bundle_id/export_method。用户env用APP_/BUILD_NUMBER等，不能使用保留MYBUILDS_前缀；MYBUILDS_IOS_*只由系统准备注入。示例不包含literal密码。

本地run选择/--all/--step/--dry-run沿已有契约；两个build必须显式选择。--param新增复用remote trigger既有build:key=value命名覆盖，shared参数仍要求所有所选build均声明，scoped覆盖shared；未知/未选择scope和同scope重复项在整批动作前拒绝。Android/iOS不同必填参数以对应scope传入，避免应用到另一build形成unknown参数。远程trigger --build android,ios/--all与命名参数沿现有操作；只用control可信构建号。框架只是工具/调度要求，不代表商店渠道，channel不自动决定track/export_method。

## 严格输入与未来功能

框架字段必须string，未知/重复/null/alias/merge等仍拒绝；总字节/深度/节点限制不变。未知参数和非法标准参数在所选整批用户脚本前拒绝，错误不回显输入标量。纯预览不调用Flutter/原生工具/网络或secret读取，运行上下文可pending。

009默认无approval/upload/通知。尚未交付的生效功能按当前明确unsupported门，不以模板自动跳过；019实际接入后用户显式reports使用其原格式/封存机制。项目无YAML绑定与profile方案仍012范围，本功能不扩project init --framework/--platform来冒充该交付。

组件阶段：Android init/Parse/dry-run与Flutter/Dart/Xcode/pod工具-only诊断可使用当前已验收接口。iOS/双模板严格Parse/Preview须005纯配置组件真实交接，签名执行须005真实代码接口冻结交付；未就绪字段或签名路径安全拒绝，不能把模板文本生成当签名通过。
