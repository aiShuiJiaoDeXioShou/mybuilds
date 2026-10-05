# Phase0 Research：App Store可控发布

研究日期2026-10-05；只读官方primary与现有源码，不访问个人账户或真实材料，不把研究当发布验收。

## R1 成熟工具与锁定门

**Decision**：保留用户选型fastlane；候选2.240.1与010共同验证，不宣称已锁。实施先用实际Ruby/Bundler安装到自有目录、真实loopback故障计数证明调用，然后根生成Gemfile.lock。共享runner只有Google/Apple两个具体消费者，调用原process.Run。
**Rationale**：宪法要求成熟第三方/锁工具，不允许规划虚造Gem锁或mock成功。
**Alternatives**：直写Go全量商店客户端、registry、用户任意Fastfile作为内置adapter均排除。工具环境材料缺失记录待验证，不能删App Review范围。

凭据采用[fastlane官方API key JSON](https://docs.fastlane.tools/app-store-connect-api/)的Team key三必填项与两个有界可选项，具体校验单一定义在config-cli；不把action的key_filepath选项误当JSON字段，也不扫描个人账户。

## R2 deliver不是单一副作用

**Decision**：上传只调用受控binary能力，关闭版本/元数据/截图/price等附带变更。submit拆成contracts/go-api列明的真实请求，每请求意图/授权/回执。准备好的版本/合规/素材缺失只失败，不调用ensure_version或补声明。固定build ID，不选latest，不取消既有submission。
**Rationale**：候选官方[deliver runner](https://raw.githubusercontent.com/fastlane/fastlane/2.240.1/deliver/lib/deliver/runner.rb)会执行version/metadata/binary/precheck/submit；[SubmitForReview](https://raw.githubusercontent.com/fastlane/fastlane/2.240.1/deliver/lib/deliver/submit_for_review.rb)内部还有选择构建、合规、创建/添加/提交多次变更。原封不动run整lane不满足动作级未知保护。
**Alternatives**：一条deliver命令失败后重跑全lane排除。使用锁版本的具体Ruby方法与少量本项目受控调用边界；不开发泛用工作流。

## R3 HTTP内部重试

**Decision**：对副作用POST/PATCH关闭该具体客户端的未知自动重发；每个具体允许method/path/body摘要绑定一个已确认grant，结果丢失停止。GET允许有界重试。实现原型必须真实loopback返回500/504/429、服务端接收后断开、401刷新等，计数证明非幂等一次；上层原Spaceship::Client、middleware也纳入审查，不仅覆盖with_asc_retry。
**Rationale**：[候选APIClient](https://raw.githubusercontent.com/fastlane/fastlane/2.240.1/spaceship/lib/spaceship/connect_api/api_client.rb)中post/patch共享重试，500/504/429与token刷新路径可重复请求。不能仅设置tries=1以为覆盖全部backoff分支。
**Alternatives**：默认retry、仅禁Go重试、任何错误一概failed均排除。无法限制未知非幂等重发则首实施门阻塞，修复后才接真实发布，不缩MVP。

## R4 Transporter实际上传边界

**Decision**：一次授予一次binary上传会话；只有已证实同一upload会话/同range内容的幂等分块恢复可重试，不重复创建/完成未知会话。实际transport选择、握手、远端唯一版本及故障计数必须在首原型和真实IPA门记录；没有证据时保unknown。
**Rationale**：[Apple Transporter手册](https://help.apple.com/itc/transporteruserguide/en.lproj/static.html)明确-k是Kbit/s限速，并支持resumable uploads；不能把-k100000或fastlane注释当整包重复发布证据。[候选transport源码](https://raw.githubusercontent.com/fastlane/fastlane/2.240.1/fastlane_core/lib/fastlane_core/itunes_transporter.rb)显示API key临时文件与多个实际后端，需逐一核实当前选择而非猜测。
**Alternatives**：固定传输等待/退出0当商店状态、禁止全部分块重试、把Transporter注释推断为重复提交均排除。

同一候选源码中Java/JWT分支会将JWT放argv；本功能排除此分支，只允许实际验证的API key文件认证后端。自产HOME/p8副本、命令参数及内部fallback均需首门实测，不能只脱敏日志而留下私钥/JWT argv。

## R5 提交前提与正式发布

**Decision**：仅同Run显式submit时在原剩余NS内等待processing；版本、处理、权限、合规和所需元数据只GET检查。automatic_release=false对应MANUAL，true对应AFTER_APPROVAL并要求submit=true；设置策略也是单独变更。终态后仅query，不授submit，不等待审核通过，不添加人工release endpoint。
**Rationale**：[Apple提交说明](https://developer.apple.com/help/app-store-connect/manage-submissions-to-app-review/submit-an-app/)要求准备元数据/选构建，并区分Add for Review与正式提交；[发布方式](https://developer.apple.com/help/app-store-connect/manage-your-apps-availability/select-an-app-store-version-release-option/)区分手动与审核后自动。处理超时保uploaded和未授submit事实。
**Alternatives**：新PublishGrant生命周期/终态submit命令/复活预算、默认自动公开、TestFlight替代App Review均排除。

## R6 只读查询与充分关联

**Decision**：实际Spaceship具体GET方法读取精确app/build/version/submission/item/release状态，严格allowlist只GET；不创建version/submission/edit补查询。未知上传若仅找到同版本构建而缺原receipt/transport关联不自动解除；未知提交须证明原submission及item关系和已submitted，不以uploaded替代。不足结果保存safe evidence供admin有依据confirm，零结果也不证明未发送。
**Rationale**：[Apple review创建API](https://developer.apple.com/documentation/appstoreconnectapi/post-v1-reviewsubmissions)和[review资源](https://developer.apple.com/documentation/appstoreconnectapi/reviewsubmission)明确草稿/items与正式submitted不同；[版本状态](https://developer.apple.com/documentation/appstoreconnectapi/appversionstate)区别审核/等待发布/真实可分发。GET-only不能补无回执的精确来源证明。
**Alternatives**：查询里创建草稿/按时间或版本猜关联/缺记录当failed均排除。

## R7 现有消费者核查

config.Step已有Target/File/Credentials/AppIdentifier/SubmitForReview/AutomaticRelease，严格kind验证与发布段次序可扩目标约束，不添步骤DSL。pipeline.Run当前有效upload预检查拒绝；RemoteOptions已有Authority/Progress/Log与真实NS预算；Agent执行journal/spool/唯一Run与Store.write的控制锁/fence可接具体Publish callback，不解析日志或造第二executor。008/009/019接口以其正式验收提交冻结，现007 WT不存在iOS签名执行或报告seal的已验收证明，不能假称可发布。

## Resolved Decisions / 未执行门

无用户需求NEEDS CLARIFICATION。工具候选的精确安装锁、内部重试控制及Transporter会话关联是明确实施首门；005真实签名、008/009/019及App Store Connect受权测试应用/元数据为完整验收依赖。此阶段均未执行上传/提交/安装Gem或账户操作。
