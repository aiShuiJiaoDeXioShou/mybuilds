# 功能规范：原生 Android 构建

**Feature Branch**: `004-android-build`
**Created**: 2026-10-04
**Status**: 已实现并验收
**Input**: 提供原生 Android doctor、可编辑的构建与产物模板，并以独立真实工程验收版本、签名、产物、缓存和取消行为。依赖已验收的 003。

## User Scenarios & Testing

### User Story 1 - 构建前诊断环境与声明签名 (Priority: P1)

开发者检查本机 Android 构建能力，能够定位 Java、SDK、项目 wrapper 或已声明签名资源的缺失和错误。

**Why this priority**: 移动工具环境错误应在漫长构建前得到可操作、无秘密的诊断。
**Independent Test**: 在准备好的工程运行 doctor，核对实际工具版本；分别移除一个工具、使用错误密码或取消检查，核对失败原因和退出状态。

**Acceptance Scenarios**:
1. **Given** 工具和项目 wrapper 有效，**When** 检查 Android，**Then** 报告实际 Java、SDK 包和 wrapper 版本，返回成功。
2. **Given** Java/SDK/wrapper 缺失、不兼容或不可运行，**When** 检查，**Then** 指明失败的检查项，不回显工具错误原文或环境值。
3. **Given** 显式声明 keystore、alias 和密码环境变量名，**When** 检查，**Then** 验证可读取的私钥条目和密码；未声明签名时报告未检查，不假称签名有效。
4. **Given** 检查命令阻塞或派生后台进程，**When** 超时/取消，**Then** 有界结束并清理本次进程组，不影响其他进程。

### User Story 2 - 生成并编辑构建模板 (Priority: P1)

开发者初始化原生 Android 配置，按文档让自己的工程显式消费版本和签名环境变量，再使用普通流水线构建。

**Why this priority**: 配置可编辑和真实工程接入是产品核心，不能依赖隐式工具参数约定。
**Independent Test**: init 生成配置，通过现有严格解析和 dry-run；确认只有 android 命名 build、run/artifact 步骤；修改任务和参数后仍由同一引擎执行。

**Acceptance Scenarios**:
1. **Given** 新目录，**When** init 选择 native/android，**Then** 生成 builds.android，含版本与构建号参数、显式 env 映射、wrapper 构建及 APK/AAB/mapping 收集，默认无上传。
2. **Given** 已有配置或混用本地模板选项，**When** init，**Then** 拒绝覆盖或冲突；无参数 init 保持最小 default 配置。
3. **Given** 工程遵循文档中的环境变量读取约定，**When** 覆盖版本和构建号运行，**Then** 最终包携带指定值，不把未消费的参数假称成功应用。

### User Story 3 - 核验真实发布构建及证据 (Priority: P1)

开发者用独立最小 Android 工程和测试签名验证真实 APK、AAB、混淆映射与快照，并重复构建复用既有缓存。

**Why this priority**: 命令退出成功不足以证明包的版本、签名和收集结果正确。
**Independent Test**: 构建两次、离线复验缓存；检查包元数据、签名、映射内容及所有快照摘要；错误签名和取消均不得报告成功。

**Acceptance Scenarios**:
1. **Given** 最小真实工程及临时测试 keystore，**When** 以版本 1.2.3、构建号 42 构建，**Then** APK/AAB 均含指定版本，签名可验证，生成非空 mapping，全部快照大小/摘要与文件吻合。
2. **Given** 首次构建依赖已缓存，**When** 再次离线构建，**Then** 成功复用缓存，不更新 SDK、不清空共享缓存。
3. **Given** 错误签名或真实构建正在执行，**When** 构建失败或取消，**Then** 状态和固定原因正确、秘密不泄露、本次进程已停止且无伪造产物成功记录。

### Edge Cases

- SDK 环境变量冲突、平台或 build-tools 缺失、wrapper 无执行位/缺 jar/启动失败。
- alias 存在但不是私钥；store 密码正确而 key 密码错误；仅声明部分签名字段。
- 版本为空、构建号非正整数/溢出；shell 参数含空格或元字符。
- 工程根目录非当前目录、输出目录被修改、mapping 因工程未启用混淆而缺失。
- 工具输出过量、输出包含密码或路径、取消发生在启动前。
- 首次依赖下载失败；真实环境不足时保留待验证状态和可执行命令。

## Requirements

### Functional Requirements

- **FR-001**: 支持显式选择 Android 的本机 doctor，提供稳定检查项、成功/失败/未检查状态、实际版本和固定诊断原因；失败返回非零。
- **FR-002**: 检查实际使用的 Java、SDK 目录与可用 SDK 包、工程内实际 wrapper；不使用全局 Gradle 替代 wrapper，不自动安装或更新 SDK。
- **FR-003**: 仅检查显式声明的签名资源；完整声明时验证 keystore 的私钥 alias、store/key 密码；未声明时明确未检查，部分声明失败。
- **FR-004**: doctor 的工具调用必须受限环境、有界输出、预算和进程组取消；不回显秘密、原始工具输出或未经验证的版本文字。
- **FR-005**: native/android init 生成可编辑 builds.android，仅使用普通 run/artifact，包含适当平台标签；已有文件与冲突选项仍拒绝。
- **FR-006**: 模板的版本和构建号参数经显式 env 传递；文档和真实工程必须展示 Gradle 消费方式，不假设任意工程消费 -P。
- **FR-007**: 默认模板构建和收集 release APK/AAB/mapping，不包含 approval/upload；启用混淆、模块与任务接入前提必须清楚。
- **FR-008**: 使用独立真实验收工程生成签名有效的 APK/AAB/非空 mapping，核对应用身份、版本名和构建号；测试密钥仅临时生成。
- **FR-009**: 复用现有产物快照、大小和 SHA-256，失败或取消保留已有证据，不伪造产物成功。
- **FR-010**: 复用既有依赖缓存，不隐式 clean、停止无关 Gradle daemon、更新系统 SDK或修改用户工程；第二次离线构建须成功。
- **FR-011**: 错误工具/签名和构建取消必须有真实行为检查，取消不遗留本次进程；工具或签名环境缺口列为待真实验证。
- **FR-012**: Android 能力在 Linux/macOS 执行；其他平台客户端可编译并明确诊断不支持本机检查；不引入新执行器、步骤类型或框架。

### Key Entities

- **检查项**：固定标识、状态、已验证版本、固定原因；不保存原始工具输出和凭据。
- **Android 模板**：普通配置文本，描述 android 命名 build、参数、环境、构建任务与产物模式。
- **签名声明**：显式 keystore/alias 与密码环境变量名，不从用户未知目录自动搜集密钥。
- **验收证据**：工具版本、命令、包元数据/签名、快照摘要、缓存和取消结果；沿用 003 的存储。

## Success Criteria

### Measurable Outcomes

- **SC-001**: 正常环境和至少六类缺失/错误工具或签名情形均产生正确检查状态；所有诊断包含零个测试密码。
- **SC-002**: 默认配置通过严格解析和无副作用预览，生成一个 android build、零个发布步骤。
- **SC-003**: 一次真实构建产出 APK、AAB、mapping 三类证据，两个包均为 1.2.3/42 且签名通过，快照大小和 SHA-256 全部一致。
- **SC-004**: 第二次离线构建成功；真实失败和取消均正确报告且不遗留本次进程。

## Assumptions

- 工程可信、管理员提供 JDK/SDK；doctor 的实际 wrapper 调用遵循工程已锁定版本，必要的 Gradle 分发下载仅进入缓存，不安装系统 SDK。
- 默认工程模块为 app、release 变体启用混淆；用户可编辑任务和输出模式适配工程，不自动解析任意 Gradle 脚本。
- 本地构建号由显式字符串参数传入，不伪造后续控制端全局计数器。
- SDK doctor 报告可用平台和 build-tools，不解析任意工程脚本的编译版本；签名由明确选项声明，密码从明确环境变量读取，不持久化。
