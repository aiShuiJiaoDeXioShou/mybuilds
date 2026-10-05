# MVP 验收指南

本指南用于记录 MVP 的实际验收结果。命令以 macOS/Linux 的 bash 或 zsh 为例；完整 Flutter 双平台验收需要 macOS 15+、启用 cgo 编译的 Agent、Xcode/iOS SDK、Android/Flutter 工具链及合法签名材料。生成案例还需 Python 3，随机 token 使用 OpenSSL。

从下面的本地检查开始，再按[Flutter 双平台集中验收指南](../examples/mvp/acceptance.md)启动控制端和两个节点。建议先完成 `internal` 构建、测试与下载，再运行 `store` 审批与分发。已执行的自动检查见[案例验证记录](../examples/mvp/validation.md)和[118 项联合检查摘要](../examples/mvp/evidence.json)；真实签名、商店及外部 Git 平台投递需要另行记录结果。

## 导航

- [本地检查](#1-本地检查无需移动工具链和商店账号)
- [生成双平台案例](#2-生成-flutter-双平台案例)
- [启动控制端和节点](#3-启动控制端和两个节点)
- [构建与审批分发](#4-构建测试下载与审批分发)
- [扩展、故障与记录](#5-扩展故障与验收记录)
- [验收记录模板](#验收记录模板)

## 1. 本地检查：无需移动工具链和商店账号

先在项目根目录按 [README 的安装说明](../README.md#安装)构建三个二进制，然后执行：

```bash
export MYBUILDS_ROOT="$PWD"
export PATH="$MYBUILDS_ROOT/bin:$PATH"
mybuilds version
mybuilds-server version
mybuilds-agent version
mybuilds run --file "$MYBUILDS_ROOT/examples/pipeline-preview.yml" --all --dry-run

local_case_dir="$(mktemp -d)"
(
  cd "$local_case_dir"
  mybuilds run --file "$MYBUILDS_ROOT/examples/local-artifacts.yml" > result.json
)
```

**通过标准：**三入口正常响应，dry-run 输出脱敏计划；本地执行退出码为 0，`result.json` 包含日志、产物快照和 `result_dir`。普通步骤的两个文件快照保持 `original-*` 内容，post 快照保存改写后的 `post-*` 内容，两者分别有大小和 SHA-256。这一步验证执行与快照，不验证移动端构建或远程调度。

## 2. 生成 Flutter 双平台案例

继续在 mybuilds 项目根目录执行。目标案例目录必须尚不存在，生成器会拒绝覆盖：

```bash
case_parent="$(mktemp -d)"
export CASE_DIR="$case_parent/flutter-mvp"
python3 examples/mvp/prepare.py "$MYBUILDS_ROOT/bin/mybuilds" "$CASE_DIR"
cd "$CASE_DIR/repo"
mybuilds run --file ./mybuilds.yml --all --dry-run \
  --param android:application_id=dev.mybuilds.mvp_flutter --param android:flavor=production \
  --param ios:xcode_project=ios/Runner.xcworkspace --param ios:scheme=Runner \
  --param ios:bundle_id=dev.mybuilds.mvpFlutter --param ios:export_method=app-store-connect
```

生成的文件各司其职：

| 文件或目录 | 用途 |
|---|---|
| `repo/mybuilds.yml` | 同一个 Flutter 仓库中的 `android`、`ios` 两个命名 build；包含真实测试、JUnit、收尾、审批与上传步骤 |
| `settings.yml` | 项目选择仓库配置，绑定应用标识和各 build 参数；不是另一份流水线 |
| `profile-settings.yml` | 切换到可复用内置构建方案的独立验收设置 |
| `server.yml`、`client.yml` | 控制端和客户端配置；本例为同机回环 HTTP、SQLite、全局并发 2 |
| `agent-android.yml`、`agent-ios.yml` | 两个独立节点的身份、数据目录、秘密文件和工具路径 |

这里的参数与生成的 settings 示例一致，接入你自己的应用时同步替换。本地 run 不读取项目 settings，所以需显式传入必填参数；远程 trigger 使用登记后的 settings。**通过标准：**只用一个仓库 YAML 就能预览两个 build，默认 `channel=internal` 时审批和上传步骤跳过。生成/dry-run 不读取签名材料，也不执行测试或构建。

## 3. 启动控制端和两个节点

首次验收建议在一台 macOS 上开四个终端：控制端、Android Agent、iOS Agent、客户端。每个终端设置相同的 `MYBUILDS_ROOT`、`CASE_DIR` 绝对路径和 `PATH`，进入 `CASE_DIR` 后操作；各 Agent 的 token 与数据目录独立。

先按[节点和工具准备](../examples/mvp/acceptance.md#两个节点和商店工具)配置应用标识、签名与秘密文件。生成器不复制商店工具：需另外部署锁定的 fastlane 目录、安装其 Gemfile.lock 依赖，并准备明确版本的 bundletool。`internal` 也会执行 release 构建，仍需要真实签名材料；它只跳过商店步骤。

控制端终端执行以下命令。首次生成的管理员 token 保存到案例私有文件，供客户端终端读取同一值：

```bash
cd "$CASE_DIR"
umask 077
export MYBUILDS_BOOTSTRAP_ADMIN_TOKEN="$(openssl rand -hex 32)"
printf '%s\n' "$MYBUILDS_BOOTSTRAP_ADMIN_TOKEN" > admin-token.private.txt
chmod 0600 admin-token.private.txt
mybuilds-server --config server.yml migrate
mybuilds-server --config server.yml serve
```

客户端终端执行：

```bash
cd "$CASE_DIR"
umask 077
export MYBUILDS_CLIENT_TOKEN="$(cat admin-token.private.txt)"
mybuilds --config client.yml status --json
mybuilds --config client.yml group create mobile
mybuilds --config client.yml node create flutter-android --labels flutter,android-sdk --capacity 1 --json > android-node.private.json
mybuilds --config client.yml node create flutter-ios --labels flutter,xcode --capacity 1 --json > ios-node.private.json
git -C repo init --initial-branch=main
git -C repo add .
git -C repo commit -m 'init(case): 初始化Flutter双平台验收仓库'
mybuilds --config client.yml project init flutter-demo --repo "$CASE_DIR/repo" \
  --nodes flutter-android,flutter-ios --group mobile --settings ./settings.yml
```

缺少 Git 提交身份时，先在此案例仓库设置 `user.name`、`user.email`。按[两个节点启动命令](../examples/mvp/acceptance.md#两个节点和商店工具)分别读取节点 token 并启动 Agent；客户端检查：

```bash
mybuilds --config client.yml node ls --json
mybuilds --config client.yml doctor --node flutter-android --json
mybuilds --config client.yml doctor --node flutter-ios --json
```

**通过标准：**两个节点在线，实际工具报告满足对应平台；同一项目授权两个节点，Android/iOS 按平台分配。只有项目登记和 queued 记录不算执行通过。跨主机时仓库必须可被控制端与节点共同读取，并使用验证证书的 HTTPS 入口，详见[节点部署](USAGE.md#独立节点日志与制品)。

## 4. 构建、测试、下载与审批分发

先用默认 `internal` 验收双平台构建：

```bash
mybuilds --config client.yml trigger flutter-demo --all --allow-upload \
  --param version=1.0.0 --idempotency-key flutter-build-001 --json > internal-trigger.json
mybuilds --config client.yml build ls --project flutter-demo --json
# 用下表中的真实返回值替换 BUILD_ID、ARTIFACT_ID 等占位符。
mybuilds --config client.yml build show BUILD_ID --json
mybuilds --config client.yml logs BUILD_ID --follow --stream-timeout 15m
mybuilds --config client.yml artifact ls BUILD_ID --json
mybuilds --config client.yml artifact download ARTIFACT_ID --output ./downloaded-artifact
```

| 要填写的值 | 实际返回来源 |
|---|---|
| `BUILD_ID` | trigger 的 `builds[]` 中对应 `build_name` 的 `id`；顶层 `batch_id` 是批次 ID |
| `ARTIFACT_ID` | artifact ls 的 `items[].id`；按 `name` 和 `purpose` 区分包与 `junit` 原 XML |
| `APPROVAL_ID`、`REVISION`、`CHECKPOINT_DIGEST` | approvals 数组同一项的 `id`、`revision`、`checkpoint_digest`；其 `build_id` 决定批准哪个构建 |
| `INTENT_ID`、`QUERY_ID` | publish ls/show 的原发布意图 ID；publish query 返回的 `id` 是查询 ID |

**通过标准：**两个 build 最终 `succeeded`，共享提交 SHA、各有独立编号与正确节点；真实测试通过，报告封存，APK/AAB/IPA 和 JUnit 原 XML 可下载。用 `shasum -a 256 ./downloaded-artifact` 核对列表中的摘要，每个文件使用不同的新输出路径。只准备 Android 材料时可先用 `--build android`，iOS 保持待验。

随后按[绑定应用与商店轮次](../examples/mvp/acceptance.md#一次构建下载与审批分发)绑定你已经准备的 Google Play、App Store 应用，等待 `project app ls flutter-demo --json` 中对应绑定为 `verified`。需要重新诊断时执行 `project app doctor BINDING_ID --json`；节点 doctor 不替代应用权限验证。再运行：

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

分别批准 Android、iOS 的精确检查点。**通过标准：**等待审批释放容量，批准后原节点继续，SHA、编号、报告和包摘要保持；Google Play internal 实际可见，App Store Connect 中应用、版本、构建号对应原 IPA。Apple 默认只上传，不提交 App Review 或自动正式发布；显式提审另按[Apple 指南](../specs/011-app-store/quickstart.md)验收。

定义包含 upload 时，即使它被 `when` 跳过，也要求管理员显式 `--allow-upload`。手动 trigger 不受 `when.changes` 路径筛选，同一提交可以切换 channel；重复使用同一幂等 key 和相同输入应返回原批次，不产生新编号。发起新一轮才换 key，商店构建号须满足渠道要求。

## 5. 扩展、故障与验收记录

下面各项按指南独立执行，不在正常商店发布过程中临时制造故障：

| 验收项 | 通过标准与操作入口 |
|---|---|
| 项目组迁移 | `group create migrated` 后执行 `project move flutter-demo --group migrated --json`；项目 ID、历史和计数器保持 |
| 两次审批与 custom | 同一 build 连续两次精确批准，只上传一次，原包下载和 metadata 查询可核对；[案例](../examples/mvp/acceptance.md#两次审批与custom联合导航)无需商店账号 |
| Webhook、changes、固定窗口 | 实际提交命中路径，再用辅助脚本发送；重复投递不重复分号、窗口不延长、执行原 SHA；[案例](../examples/mvp/acceptance.md#generic-webhook等待窗口015)与[外部平台指南](../specs/015-webhook-trigger/quickstart.md) |
| repo/auto/profile 与脚本 | profile 使用完整绑定方案；auto 仅在仓库文件缺失时回退，非法配置报错；[案例](../examples/mvp/acceptance.md#可复用方案与custom012) |
| 失败测试、when、超时与 post | 失败报告阻止发布，跳过步骤不执行，超时停止进程，收尾与原证据保留；[本地报告示例](../examples/local-reports.yml)与[集中清单](../examples/mvp/acceptance.md#集中验收清单) |
| 拒绝、取消、重启与 retry | 拒绝不上传；已开始任务不自动迁移或重跑，停止未知保留保护；显式 retry 新编号、原快照不变；[恢复说明](USAGE.md#重启与原快照重试) |
| 保留、下载与 unknown | 活动、待审批、未知发布及下载中的证据受保护，清理后不重置编号；[保留指南](../specs/020-project-retention/quickstart.md) |

每项记录“通过 / 失败 / 待验”，附 UTC、mybuilds 版本/提交、工具版本、触发 key、仓库 SHA、build/编号/节点、报告和包摘要、审批/意图/远端 ID，以及脱敏日志位置。保存为案例目录中的 `acceptance-record.md`，秘密文件、密钥和 token 留在私有目录。只有真实构建与渠道结果齐备，才把对应人工项改为通过；失败时保留原记录和证据。

遇到 queued，先看节点在线、标签、容量和项目授权；遇到预检查失败，按报告补齐工具/材料后再发新请求。发布 `unknown` 时先用 query 只读核对，必要时按[发布指南](../specs/010-google-play/quickstart.md#核对未知与证据确认)提供精确人工确认，不把构建 retry 当成重传按钮。

## 验收记录模板

在案例目录创建 `acceptance-record.md`，按下列格式记录。未执行的项目填写“待验”，失败时附原日志和复现命令。

```markdown
# MVP 验收记录

- UTC：
- mybuilds 版本与提交：
- 操作系统、节点与工具版本：
- 仓库 SHA：

| 验收项 | 结果（通过/失败/待验） | build/编号/节点 | 报告与包摘要 | 审批/发布/远端 ID | 脱敏证据或失败原因 |
|---|---|---|---|---|---|
| 本地执行与快照 | 待验 | | | | |
| Flutter Android | 待验 | | | | |
| Flutter iOS | 待验 | | | | |
| 审批与原节点续执行 | 待验 | | | | |
| Google Play internal | 待验 | | | | |
| App Store Connect 上传 | 待验 | | | | |
| Webhook/方案/custom | 待验 | | | | |
| 取消/恢复/保留 | 待验 | | | | |
```
