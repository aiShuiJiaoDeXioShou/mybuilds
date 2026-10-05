# 009 数据与状态边界

> 2026-10-05用户最新交付边界：先完成完整可执行代码与必要自动检查。005签名API只等真实代码冻结交接，不等合法Apple材料人工验收；真实签名／双平台／最终联验保留全部原要求，由用户最后统一人工验收并独立记录pending。源码集成或提交不宣称这些门PASS。无stub、第二Run、宿主未知身份或安全降级。

## 1. 构建定义与框架

现有 config.Runner 增加 `Framework string`，YAML 名为 `framework`，仅省略/native/flutter；省略等价 native。Flutter要求Platform明确android/ios；已有runner仍须platform；旧无runner generic保持合法，labels沿精确集合匹配。无需新步骤、framework表或跨build依赖。Flutter模板的build命名为android/ios；组合只合并build映射，根version仅一个。

完整Definition通过已有config验证、冻结快照与008 retry保存，framework不从命令正文或标签推断；未知框架/平台与重复字段固定错误，不回显输入。纯预览增加已校验的platform/framework摘要，不执行体检或准备资源。

## 2. 已解析参数与实际编号

| 参数 | Flutter默认／约束 | 消费 |
|---|---|---|
| version | 字符串1.0.0，三段非负十进制整数；拒控制/NUL和非法表示 | APP_VERSION→Flutter build-name、原生实际元数据检查 |
| channel | 字符串internal；合法普通渠道值可有空格和shell字符 | APP_CHANNEL，仅经引用env供用户脚本，不切SDK分支 |
| flavor | 空；非空作为一个已准备变体名，不以eval解释 | APP_FLAVOR→单个--flavor参数；iOS明确对应共享scheme |
| build_number | 本地字符串1，正十进制整数及所选平台合法范围 | 仅本地Run的内部可信build.number来源 |
| application_id | Android模板必填，与实际应用/变体一致 | APK/AAB包标识校验；不是商店发布授权 |
| xcode_project、scheme、bundle_id | iOS模板明确必填，沿005字段/路径/材料校验 | IOS_PROJECT/IOS_SCHEME；BundleID进ios_signing |
| export_method | iOS沿005默认debugging与其四个合法值 | 005生成明确导出plist；channel不推导export方式 |

version/local number的默认与原生005模板不同，是FR008明确的Flutter默认；不修改005模板。Flutter iOS本地编号使用正整数，不接受原生005三段构建号参数。iOS格式沿005与当前Apple规则（一至三段十进制整数），Flutter约定使用一个正整数；当前官方未给数值上限，不新增历史四位限制或改变控制端编号。Android沿2100000000。

实际编号关系：本地Flutter Run在标准参数已解析/合法后内部设置build.number；远程始终从已冻结task.Number设置，同batch的两个build可不同号。用户Facts白名单保持不允许伪造build.number、ios.*或签名env。本地纯预览的运行事实保持pending，不调用工具或读取密钥。未声明标准参数的自定义Flutter脚本不被强行补params；引用所需值时仍由既有模板校验拒绝缺失。

本地PreviewOptions与Run增加沿用远程命名覆盖的BuildParams：每build的最终参数为声明默认<共享Params<该build命名覆盖，再ResolveParams。Android application_id和iOS工程身份参数不应用到另一build；同scope重复、未知/未选择scope都整批拒绝。

整批参数预检查检查已声明标准字段与本次有效编号。shell依旧重复检查作为用户编辑后的防线，但不能用已启动shell的失败代替“任一非法参数在整批用户动作前拒绝”。错误仅固定已知字段/原因，不输出值。

## 3. 工具与节点事实

NodeReport沿既有OS/Arch/Capacity/Tools字段，不新增消息结构。固定工具名增加flutter、dart、cocoapods；public版本只规范数字点，Reason沿原固定集合。Flutter与Dart配对来自同一已初始化SDK真实执行；缺缓存/坏工具不安装。

Claim仍检查当前node enabled、有效session、项目授权、shell/git、platform及精确labels，再按框架检查工具。native缺省不因Flutter或pod缺失而失去既有资格。Flutter Android要求flutter/dart/java/android_aapt2/android_apksigner真实passed；Flutter iOS要求macOS、flutter/dart/xcode/ios_signing与所锁CocoaPods工程的cocoapods。Linux ARM不能由OS/Arch推断任何Android工具可执行。

ios_signing机制passed只在005真实验收并接入后由其实际可用原生能力报告；不代表任意profile授权。每次任务完整材料仍必须Validate，再Prepare重验。005实际代码未交接时保持skipped/unsupported，不以编译或标签放行。

## 4. 一次执行与签名状态

既有Run：所选整批配置/参数/工具/显式签名前提→逐build条件/step选择→有实际run才Prepare→普通run/artifact→有效权下被选user post→独立有限预算系统Close。

Prepare、Flutter工程配置、编译、archive/export、普通回执确认延迟均扣累计普通预算；post扣独立post预算，不重置普通预算。Close使用005独立预算。artifact-only、全skip、前置拒绝不准备资源。部分准备失败照样独立清理。用户取消沿现有post规则；租约到期/revoke/持久化失败禁止新用户动作/用户post，但仍做安全系统清理。

Reason保持首个实际失败/取消/超时，CleanupFailed单独记录；未知物理停止、keychain/profile/目录替换均保留保护，阻止后续build或同名任务，不能靠后续PID消失覆写已发StopConfirmed事实。恢复和重试沿008，不新建Flutter状态机。

## 5. 本次产物与报告

Android默认AAB+fat APK；flavor选择后只收集对应确定输出。仅声明R8 mapping时要求本次对应mapping，Dart符号另由用户配置。APK/AAB分别核验appID/版本/实际编号/签名；不得仅由APK推出AAB版本。iOS同一实际archive产出IPA、archive.zip和dSYM.zip，核验BundleID/版本/编号/签名及符号关联。

collector沿003/007 Root路径、普通文件、symlink、每模式非零匹配、大小/hash与完整快照。普通与post独立版本；完整诊断副本可保留但不能转成功。中央沿同attempt/phase/index/name和完整manifest/cursors确认，再CLI下载复算字节；不增加Flutter-specific产物表。

019报告仍沿同一报告定义/摘要/XML，所选ordinary最终缺报告/测试失败阻止后续发布。封存前明确新鲜性，post不修改审批前证据；默认Flutter模板没有reports或upload，不预建商店状态。

## 6. 组件就绪不是平台能力

独立Flutter/Android组件仅消费已验收004/007/008/019。IOSSigning纯配置实体归005，009只在其冻结验证交接后消费；未交接时iOS组合Parse/Preview保持待检查，不定义替代实体。工具-only iOS doctor可先真实检查Xcode/pod；材料调用、生效iOS Run和Node ios_signing passed必须等待005整功能签名验收。未实现签名路径整批明确unsupported，不读材料或创建资源；这个当前安全拒绝不是IOSSigningAvailable占位实现。组件检查、平台签名验收和整功能完成分别记录，原全部FR/SC/AC仍是交付门。
