---
name: mybuilds-operate
description: 使用 mybuilds CLI 管理项目和节点，触发构建、跟踪日志、下载制品，以及按授权处理审批和发布核对。
---

# mybuilds 日常操作

先运行 `mybuilds version` 和 `mybuilds status --json`，已有连接配置默认 `~/.mybuilds/client.yml`；`--config` 是连接配置，不是流水线。完整选项查询相应 `--help`，命令总结见[CLI文档](https://github.com/aiShuiJiaoDeXioShou/mybuilds/blob/main/docs/CLI.md)。不要打印配置中的token或秘密文件。

## 项目接入

`mybuilds init` 在当前目录生成流水线；`mybuilds project init` 向控制端登记项目。两者不能互换。

```bash
mybuilds project init PROJECT --repo REPO_URL --nodes NODE --framework native --platform android
mybuilds trigger PROJECT --build android --param version=1.2.3 --json
```

以用户实际项目、可信仓库和已授权节点替换大写占位。Android/iOS选择 `native`，Flutter选择 `flutter`；双平台用 `--platform android,ios`。快捷注册使用auto：仓库YAML缺失才回退绑定方案，配置错误不会回退。`--settings` 可导入控制端方案绑定；配置块整体替换，不可遗漏已有build。

仓库流水线使用 `project init … --file mybuilds.yml`；`--file` 是仓库内相对路径。未声明runner的Java/Maven或通用脚本，必须 `--nodes NODE --default-node NODE`，Agent需有对应工具。

本地预览用 `mybuilds run --file PATH --dry-run`，脚本工作目录仍是调用目录。本地run不会加载控制端方案，也不执行生效upload步骤。

## 查询与下载

```bash
mybuilds build ls --project PROJECT --json
mybuilds build show BUILD_ID --json
mybuilds logs BUILD_ID --follow
mybuilds artifact ls BUILD_ID --json
mybuilds artifact download ARTIFACT_ID --output ./artifact.bin
```

BUILD_ID 来自trigger返回的 `builds[].id`；ARTIFACT_ID来自制品列表。先确认构建状态，不能把成功排队当作成功构建。下载会校验大小与SHA-256，拒绝覆盖；不要替用户删除已有文件。Windows原生客户端暂不支持下载，用WSL客户端。报告摘要在build详情，原XML沿制品入口下载。

触发返回 `request_key`；响应丢失时同请求用同幂等key，避免新构建。`build retry BUILD_ID --idempotency-key KEY` 使用原SHA、参数和配置快照，产生新编号，不能修改原参数。

## 权限与变更

admin管理项目/节点、查询/触发/取消；trigger只能触发或重试普通构建，不能读日志/制品；approver能查询和审批，不能触发。不要通过切换未授权身份规避权限失败。

节点暂时停接单用 `node drain NODE`；`disable` 或token撤销会撤销执行授权。取消用 `build cancel BUILD_ID`，请求成功不等于进程已停止。`confirm-stopped` 只用于用户提供真实停止证据的恢复操作，不自动清除保护。

发布由远程流水线的approval/upload步骤驱动；触发发布必须admin和 `--allow-upload`。用户要求普通构建不等于批准发布。审批使用同一当前记录的 `--approval-id`、`--revision`、`--checkpoint-digest`，不能复用旧记录。未知发布结果先 `publish query INTENT_ID` 并查询结果，不重新上传或擅自 `publish confirm`。远程脚本部署同样会产生外部修改，先确认当前请求确实包含部署意图。
