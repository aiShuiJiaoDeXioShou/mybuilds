# 009 实际验收指南

> 2026-10-05用户最新交付边界：先完成完整可执行代码与必要自动检查。005签名API只等真实代码冻结交接，不等合法Apple材料人工验收；真实签名／双平台／最终联验保留全部原要求，由用户最后统一人工验收并独立记录pending。源码集成或提交不宣称这些门PASS。无stub、第二Run、宿主未知身份或安全降级。

本文为未来实施/真实验证步骤，当前没有009二进制或构建PASS。每项只在实际组件前置就绪及明确自有工程上执行，签名动作另须合法材料；不能把规划检查、unsigned IPA、旧预检工程产物或mock工具当结果。

## 1. 前提与记录

独立组件源码基线为已验收fee97e8，含004/007/008/019及停止修复。Android、独立Flutter/Dart与工具-only iOS诊断无需等Apple凭据。iOS/双模板严格Parse/Preview须005纯配置/预览组件在当前基线冻结验证交接；真实iOS材料doctor/签名Run/节点能力须005实际签名API冻结交付；合法材料T012–T014及009真实签名/双平台全门由用户最后统一人工验收，不反向阻塞完整代码实现/集成/提交。Linux Flutter环境及009全部平台门仍待验证，当前规划没有源码或应用PASS。保留本次UTC、二进制版本/commit、工具版本、工程固定SHA、可重复命令和脱敏证据位置。

使用明确自有最小Flutter app，实施阶段创建examples/flutter对应工程；本轮不创建。锁Flutter3.38.6/Dart3.10.7、Gradle8.14/AGP8.11.1/Kotlin2.2.20、SDK36/NDK28.2与真实pubspec.lock；iOS明确共享scheme/BundleID/Podfile.lock（CocoaPods1.16.2）。签名仅自有Android测试JKS与用户明确授权Apple P12/profile。宿主已有SDK缓存不清空，不扫描未知keychain、账户或私钥。

Linux工具与依赖缓存需独立准备并实跑，不复制用户全HOME/私有依赖，不接受新SDK条款或默认装巨型Flutter。任何必要下载先核官方版本与摘要，没取得摘要不报告安装成功。已有macOS SDK36/NDK目录只是环境观察，仍需实际编译。

## 2. 初始化、参数与纯预览

先构建对应已实现组件的客户端后，在自有空工程目录执行；Android先可验证，下列双平台组合须005纯配置交接（未满足保持pending，不从pending WT复制类型）：

```bash
mybuilds init --framework flutter --platform android,ios
mybuilds run --all --dry-run \
  --param version=1.2.3 --param channel=internal --param build_number=42 \
  --param android:application_id=com.example.flutter \
  --param ios:xcode_project=ios/Runner.xcworkspace \
  --param ios:scheme=Runner --param ios:bundle_id=com.example.flutter
```

预期：单文件恰好android/ios，platform/framework摘要正确，所需运行事实pending；不执行工具/网络/脚本，不读密钥，不创建结果或签名目录。依次核android单平台、ios单平台；重复/未知平台、已有文件、--template冲突、未知参数/未选择scope/同scope重复均拒绝且文件不变。旧default/native/custom初始化与shared覆盖保持。

## 3. 实际doctor正反例

```bash
mybuilds doctor --framework flutter --platform android --working-dir . --json
mybuilds doctor --framework flutter --platform ios --working-dir . --json
mybuilds doctor --framework flutter --platform android,ios --working-dir . --json
```

实际版本与同SDK Dart配对，所选平台外缺失不阻塞。无签名声明skipped；显式004签名flags才核Android材料，密码仅环境引用。iOS材料检查待005实际代码交接；此前工具-only可执行，签名请求明确unsupported且不读秘密。通过自己的进程隔离环境验证缺失可执行文件、真实不可执行/错格式工具、坏SDK缓存、版本不配对、超时/取消/输出超限；这些负例验证边界，不能用模拟成功工具当SC002正例。Run预检不能调用gradlew用户脚本；CLI明确doctor的旧wrapper诊断仍保留。SDK未初始化/engine信息缺失不得偷偷下载；直接snapshot与全局抑制参数不自动证明此边界，须实际核对SDK/宿主偏好前后不变，doctor临时HOME仅自有隔离。清理未知后不再启动工具。

## 4. 真实Android默认、flavor与用户脚本

在工程Gradle显式配置本次JKS env签名，保留声明application_id，确定pub依赖/cache；模板默认无发布。实际run选android：

```bash
mybuilds run --build android --param version=1.2.3 --param build_number=42 \
  --param application_id=com.example.flutter
```

保存实际Flutter appbundle/apk/Gradle输出；分别核APK aapt2版本/code、apksigner证书，AAB官方bundletool manifest与jarsigner/keytool签名，期待1.2.3/42及本次JKS。若模板任何独立核验失败，整个build不能返回成功。复算collector size/SHA256、ZIP完整性；开启R8的真实工程显式增加mapping模式并核实际R8 compiler内容，不把Dart符号当mapping。

创建至少一个真实Android flavor，与默认应用标识/可辨识内容不同；再以--param flavor=staging和对应application_id运行，空flavor不传显式参数。未准备flavor、错版本/号、debug fallback、错误alias/password、应有mapping缺失和预置旧variant输出均不能成功。修改生成YAML添加真实仓库脚本，实际消费version/channel/flavor；合法特殊渠道值作为一个env值，不产生额外命令。

全部步骤在真实macOS和Linux Android工具环境各运行；交叉编译或只见OS/Arch不算。缓存再次使用不升级SDK，不复制前一构建输出当新证据。

## 5. 真实iOS签名与资源门

先完成005真实Apple材料T012–T014，再在同Flutter工程配置共享Runner及staging scheme/profile身份。明确IOS_P12_FILE/IOS_PROFILE_FILE/IOS_P12_PASSWORD环境引用，不写入命令参数/配置值：

```bash
mybuilds run --build ios --param xcode_project=ios/Runner.xcworkspace \
  --param scheme=Runner --param bundle_id=com.example.flutter \
  --param version=1.2.3 --param build_number=42 --param export_method=debugging
```

实际经历Flutter config-only/no-codesign、005手动archive/export、collector与系统Close。核签名IPA内部BundleID/1.2.3/42、合法embedded profile、严格codesign；同archive.zip和dSYM.zip实际完整且UUID对应，复算每份快照大小/hash。改非空flavor及匹配scheme/profile再核另一真实身份；不同材料不能自动回退宿主身份。

完整组合门：成功、合法材料下准备半失败、编译/导出失败、普通取消、累计超时、post失败。系统Close独立执行，default/search list前后不变，本次keychain/profile/外部目录无残留，无关自有PID保持存活；替换路径/无法停止明确保留保护，原Reason不被覆盖。后一个所选build错材料在前一个用户动作前拒绝。全skip/artifact-only不Prepare。缺材料则本节全部保持待验证。

## 6. 真实远程与停止时序

用正式真实Server/Agent和自有受信Git仓库固定SHA，登记已授权Flutter Android Linux/macOS节点及合法iOS节点；只操作本次fixture，不重启用户服务或读取未知材料。项目登记pipeline文件与对应nodes，trigger沿既有--build/--all及命名参数。

核缺Flutter/pod/平台能力的节点不Claim，native/generic不退化；实际工具failed不能靠labels伪造passed。actualfreshCheckout固定SHA后唯一Run，构建号为中央task.Number，用户build_number/Facts不替代；两build独立进度、日志、编号和证据。

每个成功build用CLI artifact ls/download实际下载APK/AAB/mapping或IPA/archive/dSYM，Size/SHA与中央meta及节点完整快照逐项一致，再在对应真实工具环境验证下载字节。禁止只比URL/文件名。

真实Gradle/Flutter/Xcode动作运行时发送用户cancel，并连续约100–200ms采样本次PID/start-time/父链/PGID/SID/CPU与finished/terminal At；同一中央StopConfirmed前确认包括独立daemon的本次进程已停。无关自有PID不受影响。失权/持久化失败不启动新的post或补传旧终态，系统清理继续；未知停止保持原节点隔离和同名互斥，不人工kill后宣称产品PASS。

## 7. 008/019联验与正式收敛

008实际重启/retry保留framework/原SHA/参数/静态条件并新号，不重新执行旧Flutter或未知post，不增长原预算。019实际报告定义在同Run使用，测试失败/最终缺失不能被post产物或修改报告覆盖，完整原XML下载与封存机制一致；无报告的默认模板行为保持。

必要检查：

```bash
go test -p 1 ./...
go vet ./...
go test -race -p 1 ./internal/config ./internal/mobile ./internal/process ./internal/pipeline ./internal/agent ./internal/store ./internal/cli/client
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./cmd/mybuilds ./cmd/mybuilds-server ./cmd/mybuilds-agent
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./cmd/mybuilds
```

纯Go构建只证明接口跨编译，不能证明iOS签名。最终validation记录28FR/8SC/19AC各项真实证据、缺口与前置提交；可执行实现与必要自动检查通过后进行源码converge与一次功能代码提交；真实平台/签名/远程联验由用户最后统一人工验收，未实际通过项不勾完。
