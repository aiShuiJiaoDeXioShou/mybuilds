# 005 技术研究

## 密码与临时 keychain
决策：显式 P12 与密码使用原生 SecPKCS12Import，target 固定自有临时 keychain；错误只给固定原因。SecKeychainCreate/Unlock 使用内存密码、promptUser=false。禁止省略 target 导致默认 keychain 导入。[Apple import options](https://developer.apple.com/documentation/security/keychain-import-and-export-options)

Apple 公开 StorageManager 的 shouldAddToSearchList 只为 System/login keychain 注册列表；随机目录下 signing.keychain 不触发。必须用实际创建/清理前后列表与 default 比较验证当前系统行为，不调用列表快照/恢复。dynamic preference domain 不能切换/写入，不能用它假装隔离。[Apple Security StorageManager](https://github.com/apple-oss-distributions/Security/blob/main/OSX/libsecurity_keychain/lib/StorageManager.cpp)

macOS 15 起 kSecImportToMemoryOnly=true 可以只在内存验证 P12，不落入任何 keychain；本地 Xcode SDK 的 SecImportExport.h 明确该可用版本。旧系统若缺该特性必须明确说明，不向用户 default 退化。

## 非交互 codesign
原生 import 指定 SecAccess 只包含 /usr/bin/codesign。partition 使用 `/usr/bin/security -i -q` 的匿名 stdin，仅喂一条 set-key-partition-list（apple:、自产 hex 密码、自有路径），禁止 verbose，捕获输出而不写日志。OS argv 无密码；一次只喂一条是为避免后续命令覆盖退出状态。路径/长度严格校验，不能把密码传给 shell。[Apple CLI 源码](https://github.com/apple-oss-distributions/Security/blob/main/SecurityTool/macOS/security.c)、[partition 实现](https://github.com/apple-oss-distributions/Security/blob/main/SecurityTool/macOS/keychain_find.c)

codesign --keychain 只限制 identity，证书链仍可能读取系统/用户 public chain；可承诺只用指定临时私钥，不能宣称完全不读取宿主证书链。已有 app identity 不作为默认签名材料。

## Profile 与真实性
CMSDecoder 必须检查 CopySignerStatus 的输出，不能仅凭 OSStatus=0 或解码成功认定签名有效。验证期满、application-identifier、TeamIdentifier、DeveloperCertificates 与导入证书绑定。BasicX509 的系统信任不能证明 Apple profile 发行真实性，不用 CN 作授权。采用公开 SecTrustSetAnchorCertificates + Only(true)，仅接受 Apple官方公开DER根并禁网络fetch；CMS签名先独立验证。额外公开DER检查profile用途marker，不调用privatepolicy。最终有效iOS签名仍需真实Xcode导出与核验。[Apple TN3125](https://developer.apple.com/documentation/technotes/tn3125-inside-code-signing-provisioning-profiles)

Xcode16 起 profile 目录为 `~/Library/Developer/Xcode/UserData/Provisioning Profiles`，兼容旧目录。不是 Xcode27 新迁移。[Xcode16 release notes](https://developer.apple.com/documentation/xcode-release-notes/xcode-16-release-notes)

## 当前导出与版本
本机只读 `xcodebuild -version/-help` 确认 Xcode27.0（27A266a）、method 新值与手动 provisioningProfiles/signingCertificate/teamID。固定 destination=export、manageAppVersionAndBuildNumber=false，不传 allowProvisioningUpdates 或 upload 参数。[Apple archive/export](https://developer.apple.com/library/archive/technotes/tn2339/_index.html)、[导出文件](https://help.apple.com/xcode/mac/current/en.lproj/deva1f2ab5a2.html)

市场版本为三段数字，构建版本为一至三段数字；命令设置 MARKETING_VERSION/CURRENT_PROJECT_VERSION 并核验实际归档 Info.plist，拒绝不使用这些变量的硬编码配置，避免假报版本正确。[CFBundleVersion](https://developer.apple.com/documentation/bundleresources/information-property-list/cfbundleversion)

## 系统调用预算
同步 cgo 不能由 context 强制中止；Go goroutine 超时后抛弃进行中的 native create 会丢资源所有权，因此不采用。已冻结同一CLI隐藏 __ios-signing helper，经既有 process 和有限匿名 stdin 执行原生动作，父层预先持有所有路径/清理所有权。Close及半清理独立15s，不因原context取消而跳过；ErrIOSCleanup区分清理不确定。helper内security子进程继承父进程组，不逃逸另组，不新增执行器或框架。

## 当前证据及资源缺口
已确认工具版本与帮助；只读 research agent 验证 security -i -q 的 help stdin 无回显。没有使用宿主已有 app 身份、profile 或未知项目。仍未收到用户指定的工程/scheme/Bundle ID/P12/profile/password 引用；真实签名 IPA/dSYM 不具备验收条件。

## 固定 Apple 信任锚证据
从 [Apple PKI](https://www.apple.com/certificateauthority/) 页面链接通过HTTPS实际下载DER（2026-10-04），下列SHA256均与 [Apple公开摘要](https://support.apple.com/en-ie/105116) 一致。仅嵌入这三枚公开根，不信任宿主自行导入根，不嵌入私钥。
- https://www.apple.com/appleca/AppleIncRootCertificate.cer：1215字节，b0b1730ecbc7ff4505142c49f1295e6eda6bcaed7e2c68c5be91b5a11001f024。
- https://www.apple.com/certificateauthority/AppleRootCA-G2.cer：1430字节，c2b9b042dd57830e7d117dac55ac8ae19407d38e41d88f3215bc3a890444a050。
- https://www.apple.com/certificateauthority/AppleRootCA-G3.cer：583字节，63343abfb89a6a03ebb57e9b3f5fa7be7c4f5c756f3017b3a8c488c3653e9179。

Apple [公开Security源码](https://github.com/apple-oss-distributions/Security/blob/main/OSX/sec/Security/SecPolicy.c) 的 SecPolicyCreateiPhoneProvisioningProfileSigning 仍使用根/三证链/CN，注释描述profile marker 1.2.840.113635.100.6.2.2.1；本项目不调用privatepolicy或复制CN授权。选择公开DER检查该marker作为保守拒绝边界；尚无指定实际Apple profile证明兼容性，不能宣称此路径已接受真实profile。自签CMS可用于拒绝负例。

补充只读证据：Apple Security仓库公开回归夹具 [ios_provisioning_profile.cer](https://github.com/apple-oss-distributions/Security/blob/main/OSX/shared_regressions/si-20-sectrust-policies-data/ios_provisioning_profile.cer) 通过HTTPS取得后用系统openssl解码，实际含 `1.2.840.113635.100.6.2.2.1` 扩展；证书有效期2008-05-21至2020-05-21，现已过期。这证明历史Apple profile signer确有该marker，并非只从源码注释猜测；不证明用户当前profile兼容，不导入该公开证书、不替代T012或SC-002。

## 2026-10-05 当前基线移植决策

决策：基于已验收 fee97e8，保留当前唯一 process/Run/Agent；只移植旧 iOS 平台代码并做最小适配。依据为本地源码只读对比：当前 process 已有 OnStart/出生身份/脱离组停止确认，Run 已有 reports/远端预算，Agent 已有 journal 与租约；直接复制旧文件会丢失这些已验收消费者。拒绝复制旧执行器与新建签名框架。

决策：固定 helper stdin 为 []byte，沿当前真实 toolOutput 传递。上限64KiB、实际15s，不用可能阻塞的任意Reader；错误/原生材料不进入公共工具日志。新的读取仅用于明确材料，Mac实际工具/钥匙串本轮尚未执行。

决策：005 纯 schema/Preview 组件先独立冻结供009消费；完整签名资源生命周期仍需真实 Apple 门，入口未接前明确拒绝。没有新依赖、未新增 SDK 安装或环境修改。

2026-10-05用户明确验收调整：以上实际Apple兼容性资源缺口转为人工待验，不阻塞完整代码与必要自动检查的实现提交；固定信任校验及原生资源安全设计保持，历史研究不算新版本真实签名证据。
