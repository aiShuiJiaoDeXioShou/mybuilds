# 015 实施验证交接（2026-10-05）

当前分区基线2602094，未提交；主代理串行集成012/014/商店共享文件并最终检查与收敛。用户最新安排将四外部平台真实投递移至统一人工案例验收，本记录不把本地签名payload称为GitHub/GitLab/Gitee真实push。013通知仍后置。

## 已运行自动检查

- `go test -race ./internal/scm ./internal/server ./internal/config ./internal/cli/client ./internal/cli/server ./internal/protocol ./internal/pipeline -run 'TestWebhook|TestChange|TestReadChanges|TestActualTrigger|TestLocalProject' -count=1`：七包PASS，最终边界日志为外部 `webhook015-final/final-boundaries.log`。
- 新本机分页门先红（缺events/windows入口）再实现；`go test -race ./internal/cli/server ./internal/scm ./internal/cli/client ./internal/server -run 'TestWebhook|TestActualTrigger|TestLocalProject' -count=1`：四包PASS，`local-pages-red.log`、`local-pages-green.log`保留。
- 同套SQLite/私有PG schema：`go test -race ./internal/store -run '^TestWebhook' -count=1` PASS3.722s，包含20并发接收、真实第二INSERT失败整体回滚、编号不丢、去重/CAS/精确成功回执基线与retry冻结事实；旧完整Store兼容正常套件PASS82.841s，未重复全项目测试。
- 关联八包 `go vet` PASS；`git diff --check` PASS；CGO0本机client/server/agent三入口实际build成功。工具：Go1.25.4、Git2.47.1、PostgreSQL16.14；纯Go SQLite依赖沿已接受modernc1.55.0，webhooks/v6实际锁v6.4.0，仅GitHub/GitLab两个有限bytes消费者。
- 自有三二进制、受验证CA/HTTPS、可信Git及实际Agent Run：SQLite与PG各34断言，共68全部通过。覆盖固定等待窗口内合并与真实HEAD、20重投相同归属、脚本只执行一次、完整停止、成功baseline、docs-only自动skipped无号、手动changes豁免新号、旧凭据401及安全公开视图。证据在外部 `webhook015-application/evidence.json`，最终source-before/after一致与每个自有Popen已wait退出在 `webhook015-final/application-close-proof.json`。

## 真实红与修正

保留初始编译红、SDK提取错误清理defer、CLI秘密输出门、generic字段存在性与项目非法settings500等原日志。项目配置现在先Validate再材料准备；generic before显式null拒绝而缺省允许；缺认证401、已认证未知event400；本机/远程rotate需要JSON，公共查询无secret。首次工具版本查看错误使用不存在的--version，仅为夹具命令错误，随后三入口真实 `version` 命令均返回dev/unknown（本分区未提交）。

## 集成与待人工材料

012真实Origin字段已对齐；固定SHA来源的repo-only本轮闭环已过，Root应将Trigger及关闭窗口的同一prepareTrigger接到012实际resolvePipeline（两路径一致，不能再fetch后换定义）。主代理现014/发布/签名共有原文件按2602094三方补丁串行保留：Agent仅增加冻结Changes消费，protocol.Task新增omitempty Changes，process Stdin仅真实Gitbatch-check消费者；不覆盖SourcePath、旧发布链或签名生命周期。

仍待主代理：最新已集成012/014/商店权限/paused保护的实际联验、全项目一次检查与双OS必要门、收敛/整功能提交。外部四来源授权push、真实generic post-receive、30秒0/10/29秒场景与四退出点的完整实机证明保持人工/后续集中案例待验，不能用本轮68断言冒充其完成。所有70任务不凭本轮源清单整批勾选，主代理按行为证据标记。


## Root最终集成收敛增量

共享Run保留Changes与Resume；所有源模式均通过012唯一resolvePipeline，人工与窗口关闭共用prepareTrigger和原事务；PublishIDs/Origin/Changes与审批当前状态共同保留，没有另起执行器。只读converge核28FR/7SC/17AC、5原则及唯一来源/静态权限/短事务/固定SHA/条件/未知恢复六项计划决定，发现3项HIGH：单build推导绕过profile来源、审批状态语义复用遗漏、body别名永久跨失败/策略范围合并。按append-only新增T071–T073后implement，保存原稳定IDs。

新增真实用例先恢复旧消费者验证：profile-only删除仓库YAML后启用失败；同一已真实Claim/ApplyEvent/审批checkpoint/Decide的waiting_approval/approved复投新建记录；failed窗口和截止后相同无ID body错误复用。旧源4用例实际失败；修补后server/store全部Webhook SQLite检查通过（1.728s/1.028s），红绿日志保留外部mvp-hook-convergence目录。PostgreSQL与最终必要race、全项目结果另登记，不将本段作为其PASS。


## Root最终代码交付与收敛

前置本地提交：008 504dc6f、019 fee97e8、020 2602094、005 8e6240f、009 802cb0c、010/011 1d1e323、012 76d3000、014 cd58300；发布HTTP限额修复55750d1。Root共享接线、profile唯一来源、approval历史Grant、静态发布权限及原条件/报告/保留真实消费者均串行合并。

T071–T073修后双库race：Store5.971/Server5.615/SCM2.729/Protocol1.361s全部通过。最终pipeline/config/client/serverCLI/Server受影响race2.825/3.426/8.723/6.737/9.232s通过，完整normal及已发现失败的必要修后复验汇总见014 validation；完整Store normal105.194s已通过。最终whole vet、gofmt/diff和18平台编译/6本机入口通过。原全项目失败日志保留，不冒称第一轮全绿；只重验实际改动和失败范围。

实际三个最后二进制+独立Git/文件profile/验证CA的custom接收器，SQLite与PostgreSQL各manual及auto同一流水线：118/118检查全部通过。首自动构建同SHA新无ID事件在waiting_approval复用exact原BuildID，BuildIDs含全部选择结果、ReusedBuildIDs单列，BatchID空且不占号。手动与自动独立；两批准同attempt原workspace，最后post实际执行，上传每构建一次。原binary/XML完整下载、epoch2旧Grant readonly核对和metadata GET结果不改终态/manifest。8自有Popen全部wait退出，生产与mod/sum前后每文件SHA一致；脱敏证据80文件逐SHA复核。

正式converge核28FR/7SC/17AC、六项计划决定、5原则，T071–T073关闭后0新代码缺口、0新增任务，没有追加空阶段。四来源外部授权push、30s完整时点、四退出点全矩阵、双OS真实provider/移动签名等仍按用户安排人工待验，保留原未勾任务，不用这118项冒充其通过。

证据SHA256：`0c1922150daeab9af079a46cdaac7557affdb7a7b71221ea1a8cb50760ca5eeb`；清单`fb80c40c246f199c2ae43a43ade4643a6caa3a4c8da7175f6277af3e0bda89ff`；生产集合摘要`1e390b1e06b5e4ed0d7ca3c1cc5ed58418b33d3c93ef419328ae7d1cd5204cc1`。持久案例摘要见examples/mvp/evidence.json。
