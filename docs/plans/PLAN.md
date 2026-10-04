# mybuilds — 移动端构建发布工具（Go / CLI）

> 目标目录：`/Users/linghe/project/mybuilds`（项目已初始化，业务功能按路线逐步实现）

## Context

要造一个 Jenkins 式的构建发布工具，但**只服务移动端**，用 Go 实现，**纯 CLI 形态**（不做 Web 前端）。

与 Jenkins 的差异是刻意的：手机开发的痛点不是「缺一个调度器」，而是
**版本号/渠道/签名/分发链路**。Jenkins 需要插件拼装才能干的事，这里做成内置能力。

已确认的需求：

| 维度 | 决策 |
|---|---|
| 形态 | **客户端 + 控制端 + Agent 三种 CLI**；一个控制端管理多个构建节点，无浏览器 UI |
| 节点架构 | Agent 主动连接控制端；按平台、标签和容量调度；单次流水线固定节点 |
| 数据库 | 默认 SQLite，支持 PostgreSQL，统一通过 **ORM（GORM）** 访问 |
| 技术选型 | 标准库覆盖的能力直接使用；CLI / ORM / Webhook / cron / 飞书等采用成熟第三方库 |
| 配置 | 默认优先仓库内 `mybuilds.yml`，缺失时使用项目绑定的可复用构建方案 |
| 项目组 | 客户端注册项目时选择归属组，未指定进入 default；支持组改名与项目迁移 |
| 多 build | 一个项目绑定一个仓库，可定义多个命名 build；一次选择一个、多个或全部，分别调度和记录结果 |
| MVP 执行控制 | build/步骤 when、参数约束、build 总超时、post 收尾、日志时间戳、变更筛选、触发等待窗口、项目保留策略和测试报告 |
| 代码源 | GitLab / GitHub / Gitee / 任意自建 Git（通用 git 协议） |
| 触发 | Webhook（含签名校验）+ 轮询，外加手动触发 |
| 审批 | 支持发布审批（流水线中途挂起等人放行） |
| 隔离 | 裸机 shell，不用 Docker |
| 工程类型 | MVP 内置原生 Android / iOS 与 Flutter 模板，支持仓库脚本和用户本地模板 |
| 主要分发渠道 | MVP 支持 Google Play / App Store，统一封装第三方 fastlane；自定义发布共用 upload 契约 |

节点工具链增加 Flutter 与商店发布所需的 Ruby/Bundler/fastlane，按实际任务安装与锁定版本。
原规划记录的本机环境：Go 1.25.4 · Xcode 27.0 · JDK 21 · Android SDK 齐全 · Node 24。
实施前由 `doctor` 重新检测，构建记录保存实际工具版本，不把这份环境记录当成固定要求。

**硬约束**：iOS 打包（`xcodebuild` + 签名）只能在 macOS 构建节点执行。
控制端 `mybuilds-server` 可部署 Linux/macOS，Agent 在具备工具链的节点执行任务：
macOS 节点支持 iOS/Android，Linux 节点支持 Android；客户端的远程命令保持跨平台。
支持一个控制端加一个或多个 Agent，同机部署同样使用 Agent。节点协议、故障边界见 [多节点设计](MULTI_NODE.md)。
当前000–004、006–008已验收，原生Android模板、控制端与独立多节点执行已交付；005真实Apple签名、Flutter及商店分发等后续功能仍待验收。008重启核对、原快照retry与停止确认修复已通过双库/跨平台/真实移动端验收；实际状态见[实施历史](../IMPLEMENTATION_HISTORY.md)。
MVP 构建、分发及扩展边界见 [BUILD_DISTRIBUTION.md](BUILD_DISTRIBUTION.md)。

## 需求与技术决策摘要

- Go 单模块、客户端/控制端/Agent 三种 CLI；单控制端管理多个 Linux/macOS 节点，Agent 主动通过 HTTPS 连接，控制端不执行构建 shell。
- 标准库优先；Cobra、严格 YAML、Viper、GORM 及 SQLite/PostgreSQL、Webhook、glob、fastlane 等依赖按功能接入并锁定，不提前安装后续能力。
- 普通 run/artifact/approval/upload 四类步骤，共用本地与 Agent 引擎。单条流水线顺序执行且固定节点；同项目同名 build 跨节点串行，不同 build 可在容量允许时并行。
- 仓库定义或绑定方案二选一，auto 仅在仓库文件缺失时回退；固定 SHA、配置、参数和条件事实。项目组只管理归属，同项目所有 build 共用构建号计数器。
- 本地 run 调试构建，生效 upload 在命令执行前拒绝；发布经控制端授权和 Agent 执行。审批封存原产物/报告并在原节点恢复；未知上传不自动重发，停止确认与上传确认分别处理。
- MVP 完成 001–012、014–015、019–020 共 16 个待实现功能。原生/Flutter 双平台、两大商店/custom、when/参数/超时/post/日志、Webhook/changes/窗口、CLI 审批、JUnit 与保留策略均须验收。
- 013 通知、016 轮询/cron、017 其他内置分发、018 部署打磨后置；安全、租约、恢复、真实平台验证和发布结果保护不能后置。

## 专题导航与旧章节去向

旧 PLAN 的二级章节整体迁移，配置示例、接口和验收细节保留在下列专题中；旧章节锚点在此提供导航兼容。

| 原 PLAN 二级章节 | 新位置 |
|---|---|
| Context | [本文项目定位](#context) |
| <a id="关键设计选择"></a>关键设计选择 | [技术选择](ARCHITECTURE.md#关键设计选择) |
| <a id="架构"></a>架构 | [架构与持久化状态](ARCHITECTURE.md#架构) |
| <a id="配置文件"></a>配置文件 | [配置全文](CONFIGURATION.md#配置文件) |
| <a id="触发链路"></a>触发链路 | [触发与等待窗口](INTERFACES.md#触发链路) |
| <a id="文件清单"></a>文件清单 | [目标目录](INTERFACES.md#文件清单) |
| <a id="服务端-http-apimybuilds-server"></a>服务端 HTTP API（mybuilds-server） | [HTTP 与 Agent 接口](INTERFACES.md#http-api) |
| <a id="cli-面"></a>CLI 面 | [三种 CLI](INTERFACES.md#cli-面) |
| <a id="实施步骤"></a>实施步骤 | [P0–P6](DELIVERY.md#实施步骤) |
| <a id="verification"></a>Verification | [单元、端到端与真实验证](DELIVERY.md#verification) |
| <a id="明确不做一期"></a>明确不做（一期） | [一期排除项](DELIVERY.md#明确不做一期) |
| <a id="已确定的部署与范围"></a>已确定的部署与范围 | [确定范围](DELIVERY.md#已确定的部署与范围) |

补充决策：[多节点设计](MULTI_NODE.md)、[构建与分发设计](BUILD_DISTRIBUTION.md)。
执行入口：[Spec Kit 路线与 001 案例](SPECKIT_ROADMAP.md)、[MVP 批次与集成规则](MVP_EXECUTION.md)、[实施历史](../IMPLEMENTATION_HISTORY.md)。

本文件是产品入口，专题是完整约定，specs 中的规范、计划、任务及验收记录承载实际实现；规划勾选不代表功能已实现。
