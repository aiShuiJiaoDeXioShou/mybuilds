# 功能规范：本地流水线执行

**Feature Branch**: `002-local-run`
**Created**: 2026-10-04
**Status**: 已实现，集成验证通过
**Input**: 在已验收的 001 基础上实现 MVP 本地顺序 shell、条件、受限环境、取消、预算和 post，不重置用户工作树。

## User Scenarios & Testing

### User Story 1 - 在当前仓库执行构建（Priority: P1）
开发者通过同一份配置执行一个或多个 build，脚本使用显式映射的参数与真实本地上下文。
**Why this priority**: 让配置成为可运行的构建流程。
**Independent Test**: 两步写入并核对文件；工作树修改保留，条件不满足的步骤不执行。
**Acceptance Scenarios**:
1. 按选择顺序执行多个 build；每步启动独立 shell，前一步 cd/export 不延续；文件可共享。
2. build.env 被 step.env 覆盖，参数不自动导出；脚本正文不插值，声明值不会被当作命令执行。
3. branch/params 条件正确，changes 在手动本地执行忽略；不能确定必需分支时报错。
4. 全批在执行任何流水线脚本前校验；生效上传或尚未实现的能力明确拒绝，不能先运行前一个 build。

### User Story 2 - 失败、超时和取消安全结束（Priority: P1）
开发者中止构建时，本次进程退出且不会误杀其他进程，并得到准确结果。
**Why this priority**: 防止构建无限挂起与取消后的副作用。
**Independent Test**: 超时/取消带后台子进程的脚本，检查本次进程停止且无关进程存活。
**Acceptance Scenarios**:
1. 非零退出停止当前 build 的普通步骤，后续其他 build 仍执行；取消停止整个本地批次。
2. build 累计预算与 step 超时取更小值，独立 post 预算默认两分钟。
3. success/failure 再 always；运行中取消只执行 always；从未开始、全部跳过或预检查失败不执行 post。
4. post 某项失败仍尝试剩余项；预算耗尽标记未执行；post 错误不覆盖原失败/取消原因，也不重新进入 failure。

### User Story 3 - 安全诊断与日志（Priority: P2）
开发者能定位失败阶段，日志包含时间和来源，密钥不会泄露。
**Why this priority**: 实际脚本需要可用且受限的诊断。
**Independent Test**: 同时输出两路日志，分片输出声明密钥，检查脱敏、UTC 时间与长行。
**Acceptance Scenarios**:
1. 只继承既定工具/系统环境，宿主 token、数据库变量和 shell 启动注入变量不进入子进程。
2. 密钥仅由显式环境引用注入相应步骤，错误和结果不回显正文或敏感值；日志跨 Write 分片仍脱敏。
3. stdout/stderr 带 UTC 时间、build/step/stream，长行不丢失；输出写失败结束当前命令。
4. dry-run 保持无副作用；Windows 客户端可编译但本地执行明确未支持。

### Edge Cases
- 分支条件下 detached HEAD/非 Git 目录；无条件的普通 shell 不要求 Git 仓库。
- 参数包含换行、引号、等号或命令片段；空值、声明缺失与整批校验。
- 工作目录符号链接越界、被前一步替换；后台进程继承输出管道、忽略 TERM。
- 所有步骤 skipped、post 失败/超时/取消；原命令退出后仍有后台子进程。
- 极长输出行、两流并发、密钥跨分片或相互重叠、日志写失败。

## Requirements

### Functional Requirements
- **FR-001**: 本地 run 在当前工作树顺序执行选择的 build；不 checkout/reset/clean/fetch，不连接控制端。
- **FR-002**: 所选参数、模板、条件与可能执行的步骤先整批校验；生效 upload/尚未实现的 artifact/approval/reports/notifications 在任何流水线脚本前拒绝。只读 Git 获取预检查事实允许。
- **FR-003**: when 复用既定 AND/OR 和手动 changes 豁免；必需分支不可可靠确定报错；远程专属上下文不伪造。
- **FR-004**: sh 默认以 -e 执行，bash 以非登录/非交互的 -e -o pipefail 执行；脚本正文原样，每步独立进程和环境。
- **FR-005**: 环境继承只含 PATH/HOME/TMPDIR/LANG/LC_ALL/JAVA_HOME/ANDROID_HOME/ANDROID_SDK_ROOT/DEVELOPER_DIR；build.env→step.env→引擎上下文，参数只经显式映射。
- **FR-006**: 只解析声明字段中的密钥引用，不继承其他密钥；缺失引用安全报错。工作目录相对仓库根且真实路径受限，启动每步前复检。
- **FR-007**: 失败即停当前 build；全部普通步骤跳过则 build skipped，未开始执行不运行 post；其他 build 的独立失败不取消批次剩余 build。
- **FR-008**: 累计普通执行时间受 build.timeout 限制，每步用自身与剩余预算的最小值；超时记录固定原因与失败结果。
- **FR-009**: 取消只终止本次进程组，TERM 后限时 KILL 并等待回收；主 shell 提前退出/后台继承管道不导致无限 Wait。正常结束也不得遗留本次后台进程。
- **FR-010**: post 具有独立预算（默认 2m），先根据原结果选择 success/failure，再 always；取消只 always。每项失败继续剩余可执行项，原成功可变失败而原失败/取消原因不被覆盖。
- **FR-011**: 日志两流标明 UTC 时间/build/step/stream，串行写输出；密钥分片/重叠和长行得到正确处理，输出失败中止当前命令且不泄露底层错误。
- **FR-012**: 返回脱敏的 build/step/post 状态、固定原因和耗时；CLI 批次失败/取消非零退出，支持信号取消。
- **FR-013**: --step 仅执行选中单 build 的指定普通步骤，仍完整校验选择集合与发布限制；dry-run 行为和既有 CLI 契约保留。
- **FR-014**: 本地执行支持 macOS/Linux；其他平台明确未支持且远程客户端仍可编译；runner ios 只在 macOS，android 支持 macOS/Linux，具体工具链 doctor 后续接入。

### Key Entities
- 运行选项：选择、参数、工作区、输出与已知事实；不含虚构远端身份。
- 运行结果：build/步骤/收尾状态与耗时，原失败和收尾错误分开保存。
- 本次子进程：进程组、完整受限环境、工作目录、预算及输出流。

## Success Criteria

### Measurable Outcomes
- **SC-001**: 顺序、选择、参数、独立环境、when 与全批拒绝均有真实 shell 验证，用户工作树字节不变。
- **SC-002**: 失败/累计超时/取消后的本次主进程和后台子进程停止，无关进程继续存活。
- **SC-003**: success/failure/always、全部跳过及预算耗尽结果正确，原失败/取消证据保留。
- **SC-004**: 输出带时间和来源；敏感标记无泄露，长行与写失败有可运行验证。
- **SC-005**: init/预览/版本/帮助保持，客户端跨平台可编译，未交付能力明确失败。

## Assumptions
- 前置功能 001 已验收提交 `7ffa264`；产物、交互审批、报告与移动 doctor 按后续功能接入，002 不提前实现或静默忽略。
- run --step 用于独立步骤调试，不自动执行前序依赖；完整配置/参数/发布能力仍需预检查，依赖文件须已准备。
- 只读 Git 可检测本地 SHA/分支，不联网；detached HEAD 不猜分支。可信脚本不能主动脱离进程组、恶意输出转换后的密钥或绕过路径边界，此类任意 shell 不作为沙箱。
- post 默认 2m、进程 TERM 宽限期 500ms；预算内始终检查取消，取消后的 always 使用独立且有限的清理上下文。
