# 010 实现与验证记录

## 交付边界

用户2026-10-05要求先完成全部模块代码和必要自动检查，Apple/Google Play由用户最后人工验收。本文区分代码检查与真实商店验收；尚未上传到真实Google Play应用，不声称internal可见或整个MVP已经验收。

## 已实现路径

管理员应用绑定与节点只读诊断 → 原固定SHA/命名build/构建号入队 → 同一Agent与Run收集原产物 → 声明JUnit完整中央封存 → 单次发布意图及应用保护事务 → 受控fastlane客户端 → 精确回执 → 实际步骤停止后释放已知保护。授权后缺可信回执保持unknown；恢复、retry和停止确认不重发上传。

`publish ls/show/query/query-show/confirm`、`project app bind/ls/doctor`与显式本机发布doctor已经接线。查询使用当前节点身份和独立30s预算，不领取build槽。密钥只读取指定节点私有文件，公共视图不显示材料、脚本或授权摘要。

| 要求 | 实际消费者与自动检查 |
|---|---|
| FR001–005、FR012、FR014 | `internal/distribute`真实文件核验、AAB签名与bundletool、受限材料、锁版本Ruby HTTP单发故障测试 |
| FR006–007、FR025 | `pipeline/publish.go`原Collector快照与JUnit封存屏障；Store真实XML解析/封存后才允许upload intent；原005/009 Run与020资源回调保留 |
| FR008–011、FR013 | 原trigger/retry管理员许可、冻结轨道、唯一应用、完整Ref/expiry与20竞争事务 |
| FR015–019、FR023 | Storeunknown/guard/精确决定、Recover与FK/unique约束、Agent候选及grant私有journal、原终态manifest与020保留保护 |
| FR020–022、FR024 | 同一安全DTO与CLI/HTTP角色、部分查询只观察、本地有效upload整批先拒、dry-run无外部动作 |
| FR026、SC001/004 | 真实商店原生/Flutter上传及真实丢回执组合为人工待验，不用协议测试代替 |

## 自动证据

- 第三方分区冻结清单：`/tmp/mybuilds-mvp.zKtK0e/publish-channels-final/manifest.json`，SHA-256 `15145d526595214e1c70fd708fc300813e545aea7f0dfc7d06d4eec68cb83d3a`。21个具体分发源码文件及共同协议已逐字核对并集成。
- Ruby3.4.1、Bundler2.6.2、fastlane2.240.1；103个Gem真实解析锁及checksum。Google官方库core1.2.5/androidpublisher_v3 0.109.0；bundletool1.18.3 JAR SHA-256 `a099cfa1543f55593bc2ed16a70a7c67fe54b1747bb7301f37fdfd6d91028e29`。
- 实际锁版本Ruby客户端向自有故障HTTP端点发出49种变更测试请求，每个端点计数1；普通14项9.532s、race20.761s、vet通过。这证明该工具调用的单发边界，不证明真实商店接受。
- Root实际SQLite/PostgreSQL同套发布/报告/恢复门通过；20同应用竞争恰一grant，精确回执重放、活动步骤回执不提前释放保护、unknown跨Recover、约束与被破坏grant拒绝均有真实Store测试。
- 原XML解析→checked→final→中央Commit→sealed→upload intent联合通过；未完成普通run拒绝final。Pipeline使用真实收集快照与原报告，合成有限回执仅用于内部协议检查。
- 初轮五包必要race通过：Store5.234s/server2.792s/agent2.465s/pipeline2.330s/config1.378s。当前新增SourcePath与构建视图联合普通目标：Store2.973s/agent1.319s/pipeline1.262s；此前实际CLI目标通过0.541s，含public pending绑定而非伪verified。
- `BuildView.publish_ids`与doctor新平台选项混用先真实RED，修复后Store/CLI/Agent/Pipeline目标分别3.118s/1.548s/1.525s/1.460s。原节点的Collector SourcePath通过私有声明持久化，中央逐原producer.paths/upload.file唯一匹配；旧缺SourcePath只允许唯一字面producer路径保守兼容。

## 尚待收敛与人工验收

共同管理query在真实process前持久化原QueryID/session且Ref为空；实际自有工具私有目录被替换后Close返回ErrCleanup，原journal保留、下一次Serve拒绝，真实GET计数仍1且无新上传。该实际故障检查7.001s通过，测试随012的共同真实夹具交付。014历史epoch联验随审批模块实际接入。全量检查最终统一执行。

人工验收按[指南](quickstart.md)准备实际应用、service account、公开upload证书摘要与原生/Flutter签名产物。核对真实internal、错误应用/凭据/版本、真实已接受后丢回执、跨重启无重发、GET不足仍unknown及有依据精确确认。生产轨道不作为默认案例。统一Flutter案例最后由Root交付。

## 最终模块自动检查与收敛

- 2026-10-05，Apple共享patch SHA-256 `ec00a3c81f33d0ee48614e6d94ca5a7db5a5cc573a7f6c2fca6ca5c8c06b6812`逐字校验。原六动作Store授权/回执及最后unknown→错误GET不提升→完整GET同事务确认在SQLite/PostgreSQL均通过；锁版本Ruby真实10 GET/0变更，具体原ID不符拒绝。
- 共同发布请求/响应真实64KiB边界：合法原HTTP请求65536字节成功202、65537拒400；节点超限发布响应拒绝，普通响应仍保原1MiB预算。独立真实HTTP检查Server1.678s/Agent1.106s通过。AppleMatches上限16，原nonce/30s期限与严格JSON不变。
- Root最新双库普通发布目标：Store4.575s/distribute2.934s/Agent1.767s/Server3.074s全部退出0。共享最终patch隔离race：Store9.713s（双库）/distribute4.144s/Agent2.977s/Server3.657s均退出0；此前共同管道、客户端、配置race/vet已通过。
- 当前源码三入口macOS/Linux/Windows × amd64/arm64共18个CGO=0编译，以及6个实际本机help/version均退出0；证据保存在私有 publisher-cross/evidence.json。
- Spec Kit当前意图库存与原原则核对：代码交付无剩余模块缺口；真实签名/商店SC、全MVP最后源码复验和014联合门分别登记，不以这些自动检查替代外部验收。共同授权/协议/具体两渠道互相引用，同一完整功能提交包含010和011，不制造残缺中间实现。
- [集中Flutter案例](../../examples/mvp/acceptance.md)已生成并经双平台internal/store参数dry-run验证；真实Flutter构建、合法IPA与商店结果人工待验。

发布证据路径采用同一canonical DataDir/ResultDir核相对边界；单独解析DataDir会在macOS `/var`别名下使原私有证据门失败，已保留该失败并修正两边规范化，原private-file/替换拒绝门复验通过。原state路径与文件身份不重写。
