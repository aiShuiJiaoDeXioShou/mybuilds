# 011 App Store 代码交付与人工验收指南

## 1 前置与安全准备

已接入005/009/019与共同发布授权、锁定工具包、六个独立Apple动作和查询/确认。按照用户2026-10-05指示先交付代码及自动检查；真实合法签名IPA、ASC上传与App Review由用户统一人工验收，当前没有这些真实外部成功证据。只用明确授权测试应用/API key与自有受限文件，不使用个人账户、宿主keychain或未知profile，不自动接受协议/安装更新。

三CLI实际二进制、SQLite和独立PG测试DB、至少两个mac节点及Linux能力负例；沿007 HTTPS真实证书、每节点token/session/data_dir。材料只写私有夹具引用，stdout/log/DTO注入秘密扫描不得命中。

## 2 首实施工具门

记录Ruby/Bundler/fastlane及实际Appletransport版本与锁摘要。使用真实Ruby客户端+自有loopback端点计数（不是fake执行器）验证POST/PATCH 500/504/429、接受后断开、token刷新不会第二个副作用请求；GET只读可有界重试。验证upload握手/分块/完成身份，证明允许的重试是同会话幂等传输。未证明非幂等边界不能进入实际发布门，也不能锁宣称已安全。

## 3 默认上传闭环

原生与Flutter iOS各真实签名IPA，在原artifact收集后执行target=app_store，默认两布尔false。通过真实doctor核对实际授权/应用/签名/工具，发起admin --allow-upload触发。核对中央原number/SHA/IPA摘要/report seal、实际ASC build ID/应用/版本/构建号；没有select/version/metadata/review/release请求。退出0不单独证明远端状态。

错误多IPA/链接/变更摘要/包bundle ID/version/build/signature、缺p8/授权/工具、报告缺失/失败/封存后变化都在外部副作用前拒绝，原run/artifact/log/预算门保持。

## 4 显式提交

准备应用version/元数据/合规与权限，upload步骤明确submit_for_review=true，automatic_release默认false。同Run原剩余普通NS内有界等待processing，逐真实请求保存意图/回执；原build ID被选定，release策略MANUAL，精确review submission/item正式submitted。缺前提、process未完成至预算到期、过期ref、非管理员与automatic=true但submit=false都正确拒绝。保uploaded/未授submit事实，不重传。

只有明确授权的测试范围才验证automatic_release=true设置AFTER_APPROVAL；不默认扩大公开副作用，不等待外部审核批准。已terminal后仅query，不能另起提交。

## 5 双库并发与保护

同一套实际Store/HTTP/CLI分别SQLite/独立PG：20同app申请一次grant，跨项目绑定拒绝、不同app独立、旧fence/错误node/撤销token拒绝、应用改组保归属、第二controller拒绝。持久化intent失败或本地save失败不能启动。授权response丢失保请求前自产IntentID；活动slot的not_found不能清unknown，真实step_finished关闭后精确只读确认、terminal核对完整集合。每个真实变更stage丢ACK/断网/重启都unknown+appguard，不重跑前stage；StopKnown只解普通运行保护，不解appguard。

## 6 原动作核对

实际显式query只GET：精确app/build/version/submission/item关系足够才确认；上传存在不能确认未知submit，零/多候选/缺来源关联保持unknown；抓服务计数保证没有创建version/submission或其它写请求。查询不刷新lease/预算、不占构建slot。admin有依据confirm保存身份/UTC/原动作/digest/决定，相同幂等，冲突/错归属/仅进程依据拒绝；不授权重发。

## 7 安全与后续任务

admin读写、approver只读safeDTO/log/证据、trigger无发布详情或修改权，node token仅自身协议。表格与JSON同字段、无材料路径/完整环境/原脚本/机密错误。上传结束下一queued实际执行，不等待审核；后续query展示processing/submitted/published仅按实证。unknown及必要原IPA/report/audit受保留保护，014/020联验保留。

## 8 最终门与证据

执行go test ./...、go test -race相关包、go vet ./...、三入口跨平台构建/help/version、local有效upload整批拒绝和纯dryrun无网络/密钥读取。记录UTC、依赖提交、两个数据库/节点实际版本、具体命令、原号/SHA/逐动作IntentID/远端ID、脱敏证据路径和摘要。真实签名/ASC上传/显式App Review任一缺失保持待真实验证，必要自动检查与代码收敛通过后进行本地功能提交；真实商店门由用户执行登记，不以工具故障测试代替。
