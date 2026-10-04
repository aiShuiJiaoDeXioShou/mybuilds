# 004 数据模型

- DoctorCheck：Name固定android_java/android_sdk/android_gradle/android_signing；Status为passed/failed/skipped；Version仅数字包/工具版本；Reason为固定安全code。skipped不导致整体失败。
- AndroidDoctorOptions：Workspace、GradleWrapper、Keystore、KeyAlias、StorePasswordEnv、KeyPasswordEnv字符串。签名四字段全空为未检查，部分非空要求完整；env名称为标识符。
- 密码：仅显式env与进程内存，不写日志/记录；测试keystore仅临时目录。
- 模板：内嵌普通配置文本，每次返回副本，不扩展配置类型。
- 证据：沿用003产物快照与日志；post同源路径仍新增快照，不改原证据。
