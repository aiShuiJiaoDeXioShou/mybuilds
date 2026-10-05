# 009 模板、工程与产物契约

> 2026-10-05用户最新交付边界：先完成完整可执行代码与必要自动检查。005签名API只等真实代码冻结交接，不等合法Apple材料人工验收；真实签名／双平台／最终联验保留全部原要求，由用户最后统一人工验收并独立记录pending。源码集成或提交不宣称这些门PASS。无stub、第二Run、宿主未知身份或安全降级。

## 共同规则

模板为可编辑普通YAML，组合只创建一个文件的android/ios定义。version/channel/flavor/build_number明确参数及env，shell使用引用变量/Bash数组，禁止eval；仅非空flavor传一次--flavor。工程依赖和初始化SDK须预先满足；构建不会安装/升级Flutter、Android SDK、Xcode或Ruby，不自动接受许可证。pubspec.lock、Gradle wrapper及平台锁文件记录真实解析版本。

默认先显式 `flutter pub get`，之后构建使用--no-pub；离线项目可编辑为pub get --offline，并预先准备正确缓存。缺缓存失败而不是升级或切换镜像。Flutter CLI每次使用已核实的--no-version-check/--suppress-analytics全局参数；不调用disable-analytics修改永久偏好，不以这些flags代替事先准备平台缓存。用户需要额外仓库脚本时显式env映射，channel含shell字符仍为单个值。

所有旧文件排除在本次确定输出之外：按实际所选variant使用确定路径，构建前明确清理本次输出叶文件/检查产物新鲜性，不全目录glob混旧variant。清理普通产物是可编辑模板的显式动作，不由Run暗中reset用户工作树；Gradle/Pub共享缓存保留。实际输出路径/名称由本版本真实工程核对并锁入模板测试，不能据不同新版文档猜测。

## Android

实际普通构建使用appbundle/apk --release、同build-name/build-number及同可选flavor；默认不split-per-abi，使实际versionCode与可信编号一致。工程Gradle明确使用ANDROID_KEYSTORE/KEY_ALIAS/KEYSTORE_PASSWORD/KEY_PASSWORD配置release签名，不保留Flutter新工程的debug release fallback；application_id参数声明预期变体身份。

普通验证步骤必须在artifact之前分别检查APK与AAB的应用标识、市场版本、实际编号、签名证书。APK用真实SDKaapt2/apksigner；AAB用真实JDKjarsigner/keytool及官方bundletool manifest。期望证书来自本次明确keystore与alias，密码只env传给keytool，不入argv。签名密码不写Gradle文件或临时properties。

首个真实工程锁AGP8.11.1，复用其已解析bundletool1.18.1模块。可编辑验证命令以本次临时Gradle init task从实际应用插件classloader调用DumpCommand，从受限本次AAB读取manifest；没有该模块/不兼容工程固定失败并要求显式适配，不能跳验证或伪造manifest。init内容/路径和期望值经固定env传入，不插入参数生成Groovy代码；不新增生产Go protobuf解析器、插件框架或standalone安装流程。

APK/AAB对应确定flavor输出被核验后收集，mapping只在工程开启R8且用户声明时收集对应路径。模板默认收集APK/AAB，文档给出明确R8 mapping追加方式；验收工程开启R8并实际追加mapping。无mapping工程不虚构文件。Dart --obfuscate/--split-debug-info由用户显式配置，不能把Dart符号当R8 mapping。

## iOS

明确xcode_project/scheme/bundle_id/export_method与005ios_signing引用。空flavor不传Flutter flavor，使用明确scheme；非空必须与已准备共享scheme对应，并选该scheme的Release-flavor配置，不回退Runner/Release。工程Podfile须包含这些configuration映射；首个工程锁CocoaPods1.16.2，不自动安装Ruby/gems或改用户配置。

普通阶段先Flutter build ios --release --config-only --no-codesign --no-pub加实际版本/编号及可选flavor，生成正确Flutter配置并处理工程依赖；随后005同xcodebuild archive动作明确CODE_SIGN_STYLE、TEAM、identity SHA1、profile UUID、OTHER_CODE_SIGN_FLAGS --keychain、DerivedData/archive路径、MARKETING_VERSION/CURRENT_PROJECT_VERSION与dwarf-with-dsym。Flutter预配置不是unsigned产物验收，真正归档必须手动签名。

export使用系统生成MYBUILDS_IOS_EXPORT_OPTIONS，沿005方法/签名材料，不开放自动provisioning或宿主账号选择。仅从同一实际archive导出IPA、ditto压缩archive为App.xcarchive.zip并压缩dSYM为App.dSYM.zip；复制到MYBUILDS_IOS_OUTPUT_DIR后collector沿ios.output_dir收集这三类文件。系统Close只清理本次真实所有权的keychain/profile/外部build目录和可删除的源输出，已确认独立快照仍保留。

必须独立核对IPA内部Info.plist、embedded profile、codesign严格验证、对应archive app版本/identity和MachO/dSYM UUID；大小/hash复算。005未完成真实Apple材料门时，iOS本模板可准备文本与工程说明；严格Parse/Preview等005纯配置组件交接，真实资源/归档执行等005真实代码接口冻结交付，不生成签名PASS。

## 选择、预算、取消与完整证据

整批参数/工具/显式签名先查，任何后续配置非法都不启动前一build用户脚本。条件跳过/artifact-only不Prepare，不读取或注入密码。首次实际run才Prepare，普通/artifact和所选user post完成后系统独立Close；半准备失败同样清理。框架模板不提供清理hook接口。

普通阶段包含Flutter准备、所有命令及确认等待，沿累计预算；post另预算，Close再独立预算。失权/persist失败禁止新用户动作和post；清理/物理停止未知保留保护。用户post失败不能覆写原失败或修改封存证据。实际Gradle单次daemon的取消门必须在StopConfirmed时证明所有本次进程已停，不能仅稍后看最终gone。

完整产物/报告按前置不可变证据与同attempt确认，中央CLI下载字节可复算。完整诊断副本保留但不升级为成功发布；本模板不含发布、审批或商店授权。
