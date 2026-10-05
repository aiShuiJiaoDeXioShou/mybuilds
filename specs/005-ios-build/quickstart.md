# 005 验证指南（共享契约已冻结）

1. 全量 `go test ./...`、`go vet ./...`；受影响 mobile/pipeline race；无 cgo与其他平台编译说明签名未支持，CLI 预览仍可用。
2. doctor 对工具/SDK、缺少材料及明确坏材料分别验证；不得输出身份名称/密码/profile正文。
3. `init --framework native --platform ios` 得到 ios run/artifact；dry-run 不准备 keychain/profile，不读取引用；已有目标不覆盖。
4. 原生临时 keychain 真测试记录创建、错误 P12/密码、半准备清理、重复 Close、default/search list 不变；自产 test identity 只验非交互私钥机制。
5. 真实资源准备好后，在用户明确指定的仓库运行生成模板，指定 xcode_project/scheme/version/build_number/bundle_id/export_method，并设置明确引用；禁止把密码写入命令参数或配置 literal。
6. 成功获得 IPA 与对应 dSYM zip，核验 archive 实际 Bundle ID/版本、codesign 有效和快照 SHA-256；保存脱敏证据。
7. 在归档/导出/post 中制造失败、SIGINT/预算耗尽；检查本组停止、本次 keychain/profile/builddir/outputdir 清理，用户 default/search list/其他profile/无关进程不变。
8. 用户尚未指定实际工程/P12/profile/Bundle ID：继续实现和可执行负例/无签名编译，将SC-002及依赖合法材料的组合列为人工待验；完整实现、必要自动检查及以下人工步骤就绪后可代码交付，不报告真实签名PASS。

完整预览示例（不读取签名环境引用）：
```sh
mybuilds init --framework native --platform ios
mybuilds run --dry-run --file mybuilds.yml --build ios --param xcode_project=App.xcodeproj --param scheme=App --param bundle_id=org.example.app --param version=1.2.3 --param build_number=42 --param export_method=debugging
```

实际资源原型：`go test ./internal/mobile -run TestIOSNativeKeychainPrototype -v`。
实际无签名归档：`MYBUILDS_IOS_UNSIGNED_TEST=1 go test ./internal/mobile -run TestIOSUnsignedArchive -v`。
以上均为自产隔离夹具，不能代替 SC-002 的真实签名IPA。仍需核对实际Apple profile marker与Xcode archive/export对不加入用户search list的临时identity发现能力。

## 当前移植阶段（2026-10-05）

基线fee97e8，worktree ios005-current。init/doctor/Run/Agent已接当前唯一执行器；纯组件可供009复用。明确工具窗口内已运行自产native机制、无签名工程、资源Plan/Restore/Close、真prepare取消、双库关闭guard及原receipt兼容门，见validation。真实Apple材料与签名产物仍人工待验。

后续无 Apple 材料仍可验证严格 YAML/模板/参数、秘密不解引用、假事实丢弃、无cgo/其他平台明确unsupported及有限helper输入边界。明确窗口允许后，才运行自产隔离 native 机制和 unsigned 工程；这些均不能替代 T012–T014 的合法 Apple profile/P12、真实 archive/export 与生命周期。

## 用户人工验收记录要求

1. 仅在自己授权的工程与隔离工作目录中，给出共享scheme、准确Bundle ID、合法且未过期配套P12/profile；密码只通过明确环境引用提供，不填argv/配置literal。不要求向开发代理提供未知钥匙串或账号。
2. 按上文init与完整参数运行实际模板，验证材料CMS/固定Apple根/证书用途/期限/team/bundle/method绑定。错密码、错应用或过期材料应在整批用户动作前失败，日志不回显材料。
3. 成功后独立检查签名IPA、archive与IPA的Bundle ID/市场版本/构建号、对应dSYM ZIP和结果快照Size/SHA；若export找不到未加入search list的临时identity，保留固定错误并报告，不通过修改用户search list规避。
4. 在归档、导出、准备、post中分别取消/超时/失败，核对独立Close、用户default/search list/未知profile/无关PID不变；资源替换必须保留替换文件并闭锁，原失败原因不能被清理错误覆盖。远端任务还需核对journal/fence/停止及资源关闭证据，失权不能重跑签名动作。
5. 记录工具版本、实际命令UTC/exit及脱敏结果/摘要，禁止导出私钥、profile正文、密码或完整env。未做或缺材料的检查写人工待验；开发自动检查或unsigned成功均不算本清单已通过。


以下命令在完整接线版本执行，仅使用自己已经明确设置的三个引用；`IOS_P12_PASSWORD`值不写入命令参数：

```sh
mybuilds doctor --platform ios --json --working-dir "$PWD" \
  --p12 "$IOS_P12_FILE" --profile "$IOS_PROFILE_FILE" \
  --password-env IOS_P12_PASSWORD --bundle-id org.example.app --export-method debugging
mybuilds run --file mybuilds.yml --build ios \
  --param xcode_project=App.xcodeproj --param scheme=App \
  --param bundle_id=org.example.app --param version=1.2.3 \
  --param build_number=42 --param export_method=debugging > ios-result.json
```

从结果中的实际artifact快照元数据定位本次IPA与dSYM ZIP，逐个比较实际文件大小/SHA，不能取旧文件。将IPA解包到新建的自有检查目录，使用`codesign --verify --strict <Payload内实际App.app>`检查；再用`/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' <实际App.app/Info.plist>`及同一命令读取CFBundleShortVersionString/CFBundleVersion，分别应为org.example.app/1.2.3/42。`unzip -t <本次dSYM ZIP>`须通过，并确认实际dSYM属于本次归档。若没有这些真实文件或独立核验失败，人工结论应为未通过，不更改自动构建结果以凑成功。

原生资源未知时Agent保留journal并停止领取任务。重启只关闭能从原checkpoint验证身份的资源，不重跑导入/archive/export；准备中崩溃缺少叶身份时人工调查，不能按旧PID或父目录猜清理。管理员停止确认必须逐个核对原完整Ref、keychain/profile/临时目录归属与关闭结果；只有确已关闭才可在原confirm-stopped命令加--ios-cleanup-confirmed，不能用该参数掩盖未知资源。
