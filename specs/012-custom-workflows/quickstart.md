# 012 规划后的实际验收指南

当前012代码及自有接收端必要自动验证已实现，以下同时列出可执行命令和未执行人工联验；真实具体前置接口冻结并整合后，以实际三二进制、自有HTTPS/CA与数据库跑可执行门。007基线或模板解析不能替代真实Android/iOS/Flutter、报告和发布能力；005Apple材料缺失仍待验证。

## 1. 准备与冻结

以独立0700夹具、SQLite与专用PG数据库、合法当前admin/trigger/approver和私有node token运行同套suite，不动已有服务。client.yml/agent.yml/secrets文件0600，token只环境引用；controller/Agent跨主机验证HTTPS与明确CA，禁止skipTLS。先构建008/009/010/011/019正式验收基线上的012二进制，记commit、UTC、工具版本、完整Git SHA、模板hash及原证据SHA。

准备明确自有native/Flutter真实工程：Android签名、正确版本/编号、APK/AAB/mapping；iOS用户指定合法P12/profile密码引用、bundle/scheme/export、IPA/archive/dSYM。同工程含android/ios布局，两个合法节点能力不同且满足原labels；不自动扫描宿主材料或安装/升级SDK。native和Flutter两组分别验证单/双平台。自定义方案/脚本是明确受信文件；自有HTTP接收端只测试夹具，不操作用户商店或真实公开发布。

## 2. G1 无仓库配置与真实工程

自有仓库固定提交确实没有mybuilds.yml。命令：

```bash
mybuilds project init app-native --repo "$PRESET_REPO" \
  --nodes linux-android-a,mac-ios-a --framework native --platform android,ios
mybuilds project init app-flutter --repo "$FLUTTER_REPO" \
  --nodes linux-android-a,mac-ios-a --framework flutter --platform android,ios
mybuilds trigger app-native --build android --param android:version=1.2.3 \
  --idempotency-key native-android-one --json
mybuilds trigger app-flutter --build android,ios \
  --param version=1.2.3 --param android:application_id=org.example.app \
  --param ios:xcode_project=ios/Runner.xcworkspace --param ios:scheme=Runner \
  --param ios:bundle_id=org.example.app --idempotency-key flutter-both-one --json
```

实际profile必填工程参数沿最终004/005/009模板；上述参数示范命名覆盖，不能把缺材料当构建通过。核同batch固定SHA、android/ios不同真实number/buildID/ref、对应node/labels/workspace；构建号来自控制端可信Number，不能用户build_number覆盖远程编号。一个失败另一个仍独立完成，同名串行。核完整binary实际版本、证书/profile签名、archive/dSYM UUID及中央大小/hash，不用前次文件或unsigned证明。

本地新目录 `mybuilds init --framework flutter --platform android,ios` 生成一个YAML；`init --template ./ci/local.yml` 保原合法自定义build名、拒覆盖，default旧命令不读坏client配置。非法重复平台、显式framework缺platform、settings/file冲突全部先拒且无项目行。repo/default名称不自动改android/ios。

## 3. G2 来源矩阵与严格读取

在私有server.yml声明本地single root方案，方案参数/普通run/artifact用自有最小脚本产生可辨识binary；path基于server.yml目录，不能靠shell占位mock执行。project set导入完整pipeline块；repo里的所有构建和绑定名刻意不同以证明集合未合并。

```bash
mybuilds project set app-native --settings ./settings-auto.yml
mybuilds trigger app-native --all --idempotency-key source-auto-present --json
mybuilds project set app-native --settings ./settings-profile.yml
mybuilds trigger app-native --all --idempotency-key source-forced-profile --json
mybuilds project set app-native --settings ./settings-repo.yml
```

| 条件 | 必须结果 |
|---|---|
| auto+存在合法文件+有binding | 仅repo完整集合，Origin.Kind=repo/rawblobhash |
| auto+固定SHA确实缺失+绑定 | 仅profile集合，Origin.Profile/Template/内容与定义hash |
| auto+缺失无绑定；repo+缺失 | 明确错误、无任务/号 |
| profile+存在合法或坏repo配置 | 不读该配置，但仍原repo可读且固定真实SHA |
| auto+坏YAML/unknown/duplicate/null/超深/超限 | 拒绝，无fallback/部分入队 |
| exact叶目录/symlink/gitlink、父symlink、越界 | 拒绝，绝不Missing |
| Git凭据/权限/timeout/branch/ref失败 | 拒绝，不用binding绕过 |
| profile文件FIFO/叶symlink/多link/替换/超大 | 启动有限拒绝，不执行/挂住 |

自定义方案不能包含builds即使一项，也不能混template/file/覆写builtin；alias合法。旧settings.profile/params default与builds混写拒绝。源改变后选择不存在旧名必须错误，不自动映射。原source error输出不含私有path/原YAML/secret。

## 4. G3 参数、授权、原快照重试

用不同definition默认、项目build.params、共享与命名trigger覆盖逐层验证真正脚本输入；所有所选项未知/required/choices/未选择scope/重复在首个用户动作前拒，falsewhen也不能规避。build.when仅分支/参数+手动changes规则，未定编号/节点/路径模板保持pending，不把它误当条件false。skip无号无节点。

```bash
for i in $(seq 1 20); do
  mybuilds trigger app-native --build android --param android:version=1.2.3 \
    --idempotency-key preset-one-request --json > "$EVIDENCE_DIR/trigger-$i.json" &
done
wait
mybuilds build show "$ORIGINAL_BUILD_ID" --json
mybuilds build retry "$ORIGINAL_BUILD_ID" --idempotency-key preset-original-retry --json
```

并发20次同key应一批一号，丢响应原key显式重发仍同结果，不自动换key。初次排队后编辑profile/template/server config并推进HEAD/settings；重启controller实际执行仍原Definition/Params/Facts/Origin/SHA；retry继承原来源hash与完整定义、新号/新证据，旧详情/日志/产物完整JSON及hash不变。current node交集和branch/上传权限再查，不能新方案扩权。SQL失败/失锁/竞争整批rollback零新号。

## 5. G4 custom原产物与命令结果

自有可信仓库中package.sh生成可辨识package.bin，upload.sh读取系统输入JSON、向专用接收端发送并新建结构化结果，query.sh只为显式查询提供本次原ID证据。脚本和结果协议按[custom-publish.md](contracts/custom-publish.md)；不把代码或完整测试套件嵌在本指南，也不用fake command输出代替actual process。以admin私有verification-file的manual_attested依据绑定归属，记录该来源不是remote GET验证。

custom 沿两店共同CLI入口：

```bash
mybuilds project app bind custom-app --store custom --app-id org.example.application \
  --node linux-android-a --verification-file ./private-custom-binding.json
mybuilds trigger custom-app --build default --allow-upload \
  --idempotency-key custom-once --json
mybuilds publish ls --project "$CUSTOM_PROJECT_ID" --json
mybuilds publish show "$PUBLISH_INTENT_ID" --json
mybuilds artifact ls "$CUSTOM_BUILD_ID" --json
mybuilds artifact download "$ARTIFACT_ID" --output ./downloaded-package.bin
```

verification-file仅custom、0600普通≤64KiB严格JSON，只manual来源/evidencecode/note/hash；credential需要时沿既有--credentials-env NAME不传值。绑定/发布命令已接入实际010/011共同入口；人工商店凭据与发布仍未验收。hash/大小逐字节核中央原artifact；报告配置用019真实seal/XML，fail/missing/changed不能grant。审批必须由014真实中央不可变批准链核对后才能授权，不能伪approve取得grant；本分区不复制另一套审批状态，Root集成014后补联合门，不反向成为012代码交付前置。

成功需process真实Start/Stop+有效回执，接收端count1、原app/artifact/version/number/digest一致。含shell字符参数进入私有input/env单值，argv原文不替换，不形成额外命令。local run含生效upload应首动作前拒绝；dry-run仅安全验证、无网络/秘密读。原命令失败/取消/普通timeout在后续publish/Close失败后保持原Reason，post独立budget，Authority失权或CleanupFailed不继续always。

## 6. G5 unknown、查询、独立停止与全门

在自有代理分别断grant响应（中央可能已授，命令不得猜复发）、接收端已接收后断结果ACK、启动前失联、ordinary/always取消/lease到期。保存原IntentID与完整Ref、真实PID/PGID/停止证明、无关sleep存活、中央guard；不手工改状态、不删未知journal。未知情况下第二个同应用build不能获得发布授权；租约到期或独立Stop确认不释放appguard。terminal完整manifest按slot关闭后的精确候选lookup核对，lateAuthorize拒。

```bash
mybuilds publish query "$PUBLISH_INTENT_ID" --json
mybuilds publish query-show "$QUERY_ID" --json
mybuilds publish confirm "$PUBLISH_INTENT_ID" --decision-file ./private-decision.json --json
```

query当前node/session/管理task原SHA+冻结QueryArgv，30sbounded实际process；无query、空结果、exit0无关联、错误app/version/artifact或私有secret均保unknown。用户查询声明只读不等于系统可证明无副作用；管理员明确发起，人工确认须原digest与充分非秘密证据，相同决定幂等、冲突拒绝，不授第二命令。实际角色admin/trigger/approver/node负例不占号/不授意图；结果FIFO/symlink/multi-link/replacement/旧文件/坏JSON/超限/声明secret逐项真实拒，zero普通budget拒启动。

双库重复G2–G5完全相同suite，真实两节点执行G1/参数/取消。正式最后go test ./...、go test -race ./...、go vet ./...及原三入口12构建、本机help/version；UTC/完整commit/工具/二进制SHA/命令/安全ID/原产物XML与收件计数归档。可执行实现、必要自动验证和代码收敛通过后由主代理一次本地提交；真实签名与商店上传待用户最终统一人工验收；014审批/020retention接入后的实际保护联合门另计整MVP，不造成反向循环。本指南区分已执行自动门和待人工门，不作未执行PASS声明。

当前可编辑示例见 [examples/custom](../../examples/custom/README.md)，自动门详情见 [validation.md](validation.md)。
