# Flutter 双平台 MVP 集中验收

使用同一仓库的一个`mybuilds.yml`，分别在两个节点执行Android/iOS。默认`channel=internal`只构建和测试；改为`store`才进入审批与分发。Apple默认不提交审核、不自动正式发布。真实签名、Google Play与ASC结果由用户执行登记；生成配置和dry-run通过不等于商店验收通过。

## 创建独立案例

从mybuilds项目根目录执行，需要Go、Git、Python3及已经安装的移动工具链：

```bash
mkdir -p bin
go build -o bin/mybuilds ./cmd/mybuilds
go build -o bin/mybuilds-server ./cmd/mybuilds-server
go build -o bin/mybuilds-agent ./cmd/mybuilds-agent
python3 examples/mvp/prepare.py "$PWD/bin/mybuilds" /绝对路径/新的验收目录
```

生成`repo/`真实Flutter示例、双平台配置、测试脚本、server/client/两个Agent配置和项目settings，已有目录拒绝覆盖。`ci/test.sh`执行真实Flutter测试，再将该命令聚合成一个JUnit case；它不伪造框架逐case统计。先编辑settings中的应用标识，并同步Android/Xcode工程，使其对应你已经准备的商店应用。签名材料放仓库之外。

以下示例在**同一台macOS**上启动控制端和两个独立节点，方便集中验收。跨主机时将server地址改为有效HTTPS，按各机路径保存配置并指定私有`ca_file`；loopback HTTP不能直接照搬到远程地址。每个节点保留独立token、0700数据目录与0600秘密文件。

## 服务端与客户端

把已构建的三个CLI目录加入PATH，进入新验收目录：

```bash
umask 077
export MYBUILDS_BOOTSTRAP_ADMIN_TOKEN="$(openssl rand -hex 32)"
export MYBUILDS_CLIENT_TOKEN="$MYBUILDS_BOOTSTRAP_ADMIN_TOKEN"
mybuilds-server --config server.yml migrate
mybuilds-server --config server.yml serve
```

在另一个终端注入**相同管理员token**并执行：

```bash
mybuilds --config client.yml status --json
mybuilds --config client.yml group create mobile
mybuilds --config client.yml node create flutter-android --labels flutter,android-sdk --capacity 1 --json > android-node.private.json
mybuilds --config client.yml node create flutter-ios --labels flutter,xcode --capacity 1 --json > ios-node.private.json
git -C repo init --initial-branch=main
git -C repo add .
git -C repo commit -m 'init(case): 初始化Flutter双平台验收仓库'
mybuilds --config client.yml project init flutter-demo --repo "$PWD/repo" \
  --nodes flutter-android,flutter-ios --group mobile --settings ./settings.yml
```

`--settings`是客户端当前目录相对路径；仓库使用一个YAML的两个命名build。已有Git身份缺失时先在这个新案例仓库配置user.name/email。实际跨机仓库应使用明确只读SSH/HTTPS地址及凭据，而不是此处本机路径。

## 两个节点和商店工具

在案例目录下部署项目`internal/distribute/fastlane`完整副本到`tools/fastlane`。使用Ruby3.4.1、Bundler2.6.2和已提交Gemfile.lock，明确安装到该目录的`gems`，运行时不会安装或更新工具：

```bash
BUNDLE_GEMFILE="$PWD/tools/fastlane/Gemfile" BUNDLE_PATH="$PWD/tools/fastlane/gems" \
  BUNDLE_FROZEN=true bundle install
```

Google节点还需准备bundletool1.18.3，放在配置中的明确路径；JAR SHA-256为`a099cfa1543f55593bc2ed16a70a7c67fe54b1747bb7301f37fdfd6d91028e29`。Apple节点需实际Xcode、CocoaPods与合法分发签名。Flutter、Java及Android SDK通过各节点已准备的工具环境提供，不自动下载SDK或接受协议。

创建两个**自有0600**秘密文件，每项一行`NAME=value`，不使用shell展开，也不要把文件加入Git：

| 文件 | 声明 |
|---|---|
| `android.secrets.env` | ANDROID_KEYSTORE绝对路径；ANDROID_KEY_ALIAS；ANDROID_KEYSTORE_PASSWORD；ANDROID_KEY_PASSWORD；GOOGLE_PLAY_JSON私有service-account JSON绝对路径 |
| `ios.secrets.env` | IOS_P12_FILE、IOS_PROFILE_FILE绝对路径；IOS_P12_PASSWORD；APP_STORE_KEY私有API-key JSON绝对路径 |

Apple JSON使用`key_id`、`issuer_id`、`key`（完整PEM字符串），不是p8路径。具体材料规则见[Apple指南](../../specs/011-app-store/quickstart.md)。分别在独立终端读取原创建结果中的token并启动：

```bash
export MYBUILDS_AGENT_TOKEN="$(python3 -c 'import json; print(json.load(open("android-node.private.json"))["token"])')"
mybuilds-agent --config agent-android.yml serve
```

iOS终端对应使用`ios-node.private.json`与`agent-ios.yml`。客户端终端执行`node ls --json`及`doctor --node flutter-ios --json`，确认实际工具能力和两节点在线。

## 一次构建、下载与审批分发

```bash
mybuilds --config client.yml trigger flutter-demo --all --allow-upload \
  --param version=1.0.0 --idempotency-key flutter-build-001 --json
mybuilds --config client.yml build ls --project flutter-demo --json
mybuilds --config client.yml build show BUILD_ID --json
mybuilds --config client.yml artifact ls BUILD_ID --json
mybuilds --config client.yml artifact download ARTIFACT_ID --output ./downloaded-artifact
```

编号由控制端分配；两个build使用同一SHA但各自独立记录，原产物/版本/编号和下载SHA-256一致。默认未进入商店步骤，即使定义中有upload仍要求管理员显式`--allow-upload`；许可本身不把不满足条件的upload变成生效。

下一轮前，绑定实际应用；用settings中的Android/iOS应用ID替换示例值：

```bash
mybuilds --config client.yml project app bind flutter-demo --store google_play \
  --app-id dev.mybuilds.mvp_flutter --node flutter-android --credentials-env GOOGLE_PLAY_JSON \
  --upload-cert-sha256 "$UPLOAD_CERT_SHA256" --track internal --json
mybuilds --config client.yml project app bind flutter-demo --store app_store \
  --app-id dev.mybuilds.mvpFlutter --node flutter-ios --credentials-env APP_STORE_KEY --json
mybuilds --config client.yml project app ls flutter-demo --json
```

等待原节点真实doctor完成并显示verified，pending不能发布。商店首次应用/协议、Google App Signing、Apple元数据由用户准备。再发起明确分发轮次：

```bash
mybuilds --config client.yml trigger flutter-demo --all --allow-upload \
  --param channel=store --param version=1.0.1 --idempotency-key flutter-store-001 --json
mybuilds --config client.yml approvals --state pending --json
mybuilds --config client.yml approve BUILD_ID --approval-id APPROVAL_ID --revision REVISION \
  --checkpoint-digest CHECKPOINT_DIGEST --note '已核对本次原产物与测试' --json
mybuilds --config client.yml publish ls --project flutter-demo --json
mybuilds --config client.yml publish show INTENT_ID --json
mybuilds --config client.yml publish query INTENT_ID --json
mybuilds --config client.yml publish query-show QUERY_ID --json
```

从本次安全审批结果填写精确ID/revision/digest，分别批准两个build；同内容重放应幂等，不能用第一次审批批准下一次。核对原节点恢复、原SHA/编号/报告/产物不变，Google internal实际可见、ASC实际构建应用/版本/编号对应。Apple明确App Review另按[011指南](../../specs/011-app-store/quickstart.md)改配置并提交新Git版本，保持默认automatic_release=false。

## 集中验收清单

| 场景 | 应观察的结果 |
|---|---|
| 组迁移、同仓库双build、多节点 | 项目ID/计数与历史不变；同名build串行，不同build按容量调度 |
| 同key重复触发、参数与when | 原批次/编号不重复；单build scoped参数不覆盖另一build；跳过不生成动作 |
| 真实Flutter测试失败/缺XML | 不批准发布；原日志和XML可核对；系统资源独立Close |
| 审批等待/拒绝/原节点离线 | 等待释放槽且不扣剩余预算；拒绝不上传；离线不迁移已开始任务 |
| 取消/重启/retry | 原物理停止与执行归属核验；已开始动作不静默重跑；retry新编号保原快照 |
| unknown发布 | 不因退出/停止/重启清应用保护，不自动重传；GET不足仍unknown；有依据的精确confirm才改变结果 |
| 保留与下载 | 活动、等待审批、unknown及下载中原证据保持；已确认清理不重置项目编号 |

## generic Webhook等待窗口（015）

本案例的登记provider为`generic`，repository_key固定`flutter-demo`。已经按上文初始化项目后，保留原settings参数与应用归属，只另存Hook策略：

```bash
umask 077
cp settings.yml hook-settings.yml
cat >> hook-settings.yml <<'YAML'
hook:
  enabled: true
  repository_key: flutter-demo
triggers:
  builds: [android, ios]
  quiet_period: 3s
  allow_upload: true
YAML
mybuilds --config client.yml project set flutter-demo --settings ./hook-settings.yml --json > hook-init.private.json
mybuilds --config client.yml project hook enable flutter-demo --json
mybuilds --config client.yml project hook rotate flutter-demo --json > hook-rotation.private.json
chmod 600 hook-init.private.json hook-rotation.private.json
mybuilds --config client.yml project hook show flutter-demo --json
```

尚未创建项目时，可将上文`project init`的settings改为`hook-settings.yml`，并加`--hook --hook-repository-key flutter-demo --json`；已有项目使用`set`，不要重复init。本例只由管理员静态许可定义中的upload：`allow_upload=true`不会绕过`when`、应用归属、doctor、报告或审批。Hook不传参数，默认`channel=internal`，两平台实际构建与测试，商店步骤继续跳过，无外部商店动作。

`rotate --json`的**顶层secret**用于下面脚本；脚本拒绝非当前用户的0600普通单链接文件、无限响应和重定向，不从命令行接收明文secret。先在案例`repo/`真实提交一个`lib/`改动，然后从案例目录执行：

```bash
export MYBUILDS_HOOK_DELIVERY="flutter-demo-$(git -C repo rev-parse HEAD)"
python3 /绝对路径/mybuilds/examples/mvp/hook_push.py --repo ./repo --secret-file ./hook-rotation.private.json
# 相同HEAD与相同投递ID重放：应返回原event/window归属，不再分号。
python3 /绝对路径/mybuilds/examples/mvp/hook_push.py --repo ./repo --secret-file ./hook-rotation.private.json
mybuilds --config client.yml project hook events flutter-demo --limit 20 --offset 0 --json
mybuilds --config client.yml project hook windows flutter-demo --limit 20 --offset 0 --json
mybuilds --config client.yml build ls --project flutter-demo --json
```

默认发送到本机`http://127.0.0.1:8787/hook/flutter-demo`，只签实际`git rev-parse HEAD`完整SHA的同一原始JSON正文。跨主机使用受验证HTTPS，例如追加`--url https://control.example/hook/flutter-demo --ca-file ./ca.pem`；系统信任的HTTPS可省略CA，禁止远程HTTP、URL凭据/query及关闭TLS验证。`MYBUILDS_HOOK_DELIVERY`是公开投递标识，可重复；不要把secret写进环境示例或Git仓库。

首次接收固定3秒窗口，后续push不延长；接收2xx不代表执行成功，核对窗口最终固定SHA、两个build ID、编号、原节点与报告/产物。首次缺成功baseline按full构建，后续只有公共声明路径或对应平台路径命中才执行。原XML/日志通过既有build/artifact命令核对；本脚本不发送通知，也不证明四外部托管平台真实push已经验收。

安全反例：将**独立Hook策略副本**的`allow_upload`改为`false`后显式`project set --settings ... --json`，提交一个新改动并使用新的投递ID。当前两个定义含upload，即使默认channel=internal也应窗口`failed/hook_forbidden`、无新编号、无商店动作；重投旧已接收ID仍只查询原归属。完成反例后重新应用上面的true策略，不修改已有构建或未知发布保护。

可复用profile/custom集中案例在012整合后补充。验收记录逐项保存UTC、Git SHA、build/number/node、artifact/report摘要、approval/intent/远端ID与通过或失败结果；机密材料只存节点私有文件，不收录到验收记录。
