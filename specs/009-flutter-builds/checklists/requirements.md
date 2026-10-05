# 需求质量检查清单：Flutter 双平台构建与可编辑模板

**Purpose**：在进入规划前检查需求完整性、边界与可验证性。
**Created**：2026-10-04
**Feature**：[spec.md](../spec.md)

**审查归属**：本清单由 specify 阶段审查维护；勾选仅表示需求质量通过，不表示实现、平台能力或真实签名验收完成。

## 内容质量

- [x] CHK001 无实现语言、库选型、内部 API 或源码布局设计；既有 CLI／配置名仅用于描述用户行为。
- [x] CHK002 围绕初始化、诊断、真实产物、用户编辑与资源安全的用户价值。
- [x] CHK003 使用中文面向工程维护者，术语对应已有产品能力。
- [x] CHK004 具备用户故事／验收、边界、功能需求、关键实体、成功标准和假设。

## 需求完整性

- [x] CHK005 没有待澄清标记；常规默认、前置依赖及待规划选择明确说明。
- [x] CHK006 28 项功能需求都有可验证行为及失败边界。
- [x] CHK007 8 项成功标准具有数量、对应证据和明确通过／不通过判据。
- [x] CHK008 成功标准描述用户结果，不依赖内部实现、库或数据布局。
- [x] CHK009 19 个故事验收场景覆盖正反例及真实工具／产物要求。
- [x] CHK010 覆盖特殊参数、flavor、签名、路径、旧文件、缓存、取消、失权与未确认清理。
- [x] CHK011 009 限定模板／体检／平台复用及既有节点消费，不提前实现发布、方案绑定或第二执行体系。
- [x] CHK012 004 已验收和 005/007 待验收区分；工具存在不代替能力，真实 Apple 门与后续依赖保留。

## 功能准备程度

- [x] CHK013 FR-001–007 对应故事1，FR-008–013 对应故事2，FR-014–017 对应故事3，FR-018–024 对应故事3／4；FR-025–028 由跨故事真实验收、范围和 SC-003–008 覆盖。
- [x] CHK014 单／双平台初始化、选平台诊断、版本／flavor、编辑脚本、真实签名与本地／远程证据均有主流程。
- [x] CHK015 四个故事共同覆盖 SC-001–008；无需模拟 Flutter、unsigned IPA 或旧文件即可定义完成条件。
- [x] CHK016 需求没有泄漏实现方案；后续版本锁定、消息扩展和文件归属明确留给 plan。

## 审查记录

- 2026-10-04 末轮逐项审查：16/16 通过；28 FR、8 SC、4 用户故事、19 个验收场景，无需求澄清标记。
- 已核对 BUILD_DISTRIBUTION／SPECKIT_ROADMAP／constitution 2.1.0、005 pending spec／validation、root 当前 007 spec／contracts。本轮没有运行 plan、tasks、analyze、implement 或应用验收。
- preset resolve 的 spec-template 和 checklist-template 均来自本 worktree `.specify/templates` 的 core 层；按必备章节中文生成。before_specify／after_specify 的扩展配置为 `hooks: {}`，无可执行 hook。
- 有效 feature selector 为 ignored `.specify/feature.json` 的 `specs/009-flutter-builds`。009 的 plan／实现须等待 005 与 007 正式验收集成；清单通过不授予越过此前置的权限。
- 末轮复核明确用户取消仍沿既有有限预算 post 规则，失权／保存失败才禁止后续用户收尾；可下载归档包属于实际产物，不把临时目录存在当完整证据。
- 本清单由 speckit-specify 的内建质量生命周期生成；不是额外执行 speckit-checklist，也不是实现任务表。
