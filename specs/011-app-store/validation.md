# 011 实现与验证记录

用户2026-10-05要求先实现并自动检查，真实Apple/Google Play由用户集中人工验收。App Store代码已接入共同发布路径；当前没有合法Apple签名IPA、真实ASC上传或真实App Review成功证据。

## 实际实现

`PrepareApple`验证本次稳定IPA快照、应用、版本、可信编号、Apple签名链与005实际分发Team。默认只上传；明确提交时沿同一Run/原预算依次进行upload_binary、select_build、set_release_policy、create_review、add_review_item、submit_review，每个变更独立意图及原请求摘要。已经精确满足的远端前提只读复用，不伪造变更回执。

二进制上传由原Go `process.Run`直接执行实际`/usr/bin/xcrun`，fastlane生成明确API-key上传命令；不借第二PTY/隐藏子进程上传。REST动作使用锁定fastlane客户端，未知副作用不重发。查询GET总30s，不创建版本、素材、审核草稿或修改合规内容。准备/提交前提不足保已确认上传，不在终态后恢复审核动作。

## 自动证据与要求映射

| 要求 | 实际代码/检查 |
|---|---|
| FR001–007、FR026–027 | `distribute/apple.go`、`apple_transport.go`真实材料/IPA边界；原005/009签名Run与Collector、019报告封存和共同Store消费者 |
| FR008–010、FR014–018 | 六动作单次grant、原flags/RequestSHA/PreviousIntent链；真实Ruby NextAction GET与受控请求body测试 |
| FR011–013、FR019–022 | 原管理员许可、应用唯一绑定、共同20竞争、unknown/guard与精确confirm，原完整Ref/expiry、原回执manifest |
| FR023–025 | 共同CLI/安全DTO、独立节点管理query、原步骤结束释放普通执行槽，unknown仍保护原证据 |
| FR028、SC001/002/005 | 合法真实IPA/ASC上传/实际审核提交与丢回执为人工待验，不把HTTP故障或自产证书视为通过 |

第三方冻结清单与共同自动检查见[010记录](../010-google-play/validation.md)。实际Ruby NextAction测试包含19次GET、5个后续动作选择；49个真实变更故障端点每个计数1。实际SDK对象字段使用review submittedDate及WAITING_FOR_REVIEW/IN_REVIEW/COMPLETING/COMPLETE，避免读取不存在的submitted字段；review item核对实际submission/items归属。

传输结果限64KiB，原命令启动与关闭沿唯一process.Run，结果不以退出0推断已公开上架。独立Close仅清自产资源；身份替换或停止不确定保持ErrCleanup和私有保护。协议中的合成回执只验证原Store/Run绑定，不构成真实Apple验收。

## 最后交付与人工范围

充分关联的Apple查询已消费原version/build/submission/item和具体动作请求摘要；完整GET证据在原事务内新增精确query决定与审计，经closePublishStep释放已知保护。错误item、仅同版本的upload、空或歧义仍不提升。原审批epoch历史grant联合核验随014实际接入，管理Close故障检查见010记录。

用户按照[人工指南](quickstart.md)准备实际应用/API key、合法分发签名及版本元数据。先核对默认仅上传，再在明确范围中验submit_for_review=true、automatic_release=false的MANUAL路径。需要自动正式发布时才明确设true；不等待外部审核通过，不把TestFlight上传当App Review验收。原生与Flutter各自的真实签名与分发结果需分别登记。

## 最终模块自动检查与收敛

- 2026-10-05，Apple共享patch SHA-256 `ec00a3c81f33d0ee48614e6d94ca5a7db5a5cc573a7f6c2fca6ca5c8c06b6812`逐字校验。原六动作Store授权/回执及最后unknown→错误GET不提升→完整GET同事务确认在SQLite/PostgreSQL均通过；锁版本Ruby真实10 GET/0变更，具体原ID不符拒绝。
- 共同发布请求/响应真实64KiB边界：合法原HTTP请求65536字节成功202、65537拒400；节点超限发布响应拒绝，普通响应仍保原1MiB预算。独立真实HTTP检查Server1.678s/Agent1.106s通过。AppleMatches上限16，原nonce/30s期限与严格JSON不变。
- Root最新双库普通发布目标：Store4.575s/distribute2.934s/Agent1.767s/Server3.074s全部退出0。共享最终patch隔离race：Store9.713s（双库）/distribute4.144s/Agent2.977s/Server3.657s均退出0；此前共同管道、客户端、配置race/vet已通过。
- 当前源码三入口macOS/Linux/Windows × amd64/arm64共18个CGO=0编译，以及6个实际本机help/version均退出0；证据保存在私有 publisher-cross/evidence.json。
- Spec Kit当前意图库存与原原则核对：代码交付无剩余模块缺口；真实签名/商店SC、全MVP最后源码复验和014联合门分别登记，不以这些自动检查替代外部验收。共同授权/协议/具体两渠道互相引用，同一完整功能提交包含010和011，不制造残缺中间实现。
- [集中Flutter案例](../../examples/mvp/acceptance.md)已生成并经双平台internal/store参数dry-run验证；真实Flutter构建、合法IPA与商店结果人工待验。

发布证据路径采用同一canonical DataDir/ResultDir核相对边界；单独解析DataDir会在macOS `/var`别名下使原私有证据门失败，已保留该失败并修正两边规范化，原private-file/替换拒绝门复验通过。原state路径与文件身份不重写。
