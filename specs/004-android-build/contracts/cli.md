# 004 CLI契约

```text
mybuilds init --framework native --platform android
mybuilds doctor --platform android --json [--working-dir DIR] [--gradle-wrapper PATH]
mybuilds doctor --platform android --keystore PATH --key-alias ALIAS --store-password-env NAME --key-password-env NAME
```

本期无platform默认Android，后续005可组合已交付检查；doctor不读取远端凭据。JSON为DoctorCheck数组，普通输出只固定标识/状态/安全版本/原因。至少一项failed时非零；未声明签名skipped。

native/android与--template互斥，已有文件不覆盖；无参数init维持default，不支持的选项明确失败。

模板version/build_number默认1.0.0/1，env为APP_VERSION/BUILD_NUMBER，不声明保留MYBUILDS_前缀或伪造远程上下文。签名显式来源ANDROID_KEYSTORE/ANDROID_KEY_ALIAS/ANDROID_KEYSTORE_PASSWORD/ANDROID_KEY_PASSWORD宿主变量。用户工程必须按examples/android接入读取；release混淆、app模块/任务和输出模式可编辑。
