# 005 数据模型

- IOSSigning 定义：P12/profile/password 完整引用，BundleID/export method；解析阶段不解引用，预览不显示。
- IOSSigningOptions：已解析的明确文件/密码、仓库根、Run预检查生成的仓库相对OutputDir、BundleID/method；只在内部生命周期使用，不能序列化进公共输出；仅有限匿名stdin内部协议传入受控helper。
- IOSResources：仅记录自有 keychain/profile副本/构建与输出目录、证书指纹/UUID/team、ExportOptions；Prepare 失败自身清理，Close 幂等。
- IOSDoctorOptions/DoctorCheck：明确材料选择与只读摘要；无材料 pending，不据身份数量判定授权或可签名。
- 资源状态：未准备→已拥有→已关闭；部分准备失败→独立清理；关闭失败保留失败证据并停止后续 build。普通失败/取消原因与资源清理错误分开。
- 单 app/profile 是当前输入模型，不猜 extension profile；最终产物仍为 003 普通快照实体，无新增持久化框架。

- Helper协议：仅inspect/prepare/close动作，固定argv、有限stdin、安全摘要/原因输出；父层持有资源路径，独立cleanup不丢所有权。
- Apple roots：三枚官方公开DER+固定SHA256，public anchorsOnly；不调用privatepolicy、不把CN当授权。

## 当前基线移植约束

IOSSigning 为 Build 的可选指针，五个字符串字段严格校验；纯组件不保存解析后的秘密。ios.output_dir 仅来自本次 Run 系统所有权，Preview 中未获得该事实时 pending，用户不能伪造。

远端恢复沿当前 Agent journal 完整 Ref/seq/digest/budget/checkpoint。新增 native 资源所有权必须在外部准备前持久，关闭确认与物理进程停止分别保留；原生清理未知不得因已有进程 StopReceipt 解除 guard。当前阶段不引入业务表或未来审批/发布状态。


## 当前具体恢复记录

- IOSResourceOwnership 是私有journal记录，包含Version、随机Token、Workspace/Output/Temporary/Keychain/Profile及实际Device/Inode/Mode；原材料路径和密码不进入记录。准备前持久Preparing intent，完成后持久叶身份和profile SHA256；恢复仅Close，不重做Prepare。没有可靠叶证据的prepare中崩溃保持未知闭锁。
- IOSCleanupConfirmed与IOSResourceDigest是终态/独立Stop的可选字段，旧nil签名定义省略，旧wire摘要不变。Digest绑定已关闭Ownership；Agent只读终态恢复重新计算后才查精确receipt。中央stop_confirmations仅追加默认false与空摘要列，不增加业务表。
- BuildRun私有iosTeamID取真实准备资源Environment，不接受用户env或序列化为公共构建值；后续同一发布消费者按已验证资源使用。
