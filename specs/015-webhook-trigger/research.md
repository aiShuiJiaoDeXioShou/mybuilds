# 015 Phase0研究与决定

研究日期2026-10-05；本阶段只有源码/官方资料读取及Spec Kit脚本。没有实际provider写入、push、数据库、节点或VM验收。前置008/012/014/019未验收，规划类型不当成已交付API。

## 1. 产品四来源与现有入口

Decision：GitHub/GitLab/Gitee/generic；Gitea/Gogs只研究generic兼容，不新增两个必验provider。Rationale：PLAN/INTERFACES/MVP_EXECUTION一致，当前Store.normalizeProject与CLI已只接受此四值；根纠正初始Gitea/Gogs指令回产品。Alternative：删除Gitee或专用providerregistry均越范围。

现有真实消费者：server.Trigger先FindRequest→GetProject/branch→scm.ReadPipeline→config.Parse/Select/ResolveParams→upload权限→Preview/buildCondition→Store.Enqueue；Enqueue在Store.write复核身份/PolicyVersion、原子建batch/build/steps并CAS计数。scm.ReadPipeline每请求自有bare与受限process.Run，不运行hooks、submodules、filter或payloadURL。015提取同准备/同私有enqueueTx给自动入口，不能构造admin Actor或另建queue。

012规划resolvePipeline固定SHA的repo/profile/auto来源，015只在其实际接受后消费；008 Retry保原快照事实，019封存报告，014产品审批权不得由hook替代。根当前未找到014正式spec artifacts，不编造该API。

## 2. GitHub

Decision：只JSON push、固定X-GitHub头、HMAC-SHA256原body，精确delivery持久去重。Rationale：官方要求原body的sha256=十六进制与恒时比较，重投保持deliveryID；push可能包含tag/删除，payload after/ref不能当任意URL。Provider本身cap25MB，本产品可设置更小明确上限并安全拒绝；成功接收在10秒内，不等待Git或节点。[验证签名](https://docs.github.com/en/webhooks/using-webhooks/validating-webhook-deliveries)、[事件/大小/头](https://docs.github.com/en/webhooks/webhook-events-and-payloads)、[重投与10秒响应](https://docs.github.com/en/webhooks/using-webhooks/best-practices-for-using-webhooks)。

Alternative：SHA1、反序列化后重算、要求不存在的request timestamp、用User-Agent身份，均拒绝。归一字段/忽略规则仅一处见[http](contracts/http.md)。

## 3. GitLab

Decision：MVP沿产品明确X-Gitlab-Token模式，不依据外部signature头自动切认证策略。Rationale：官方legacy token仍支持；现代webhook-id/Idempotency-Key在重投间稳定，Event-UUID递归事件可复用，不优先当delivery，Webhook-UUID是hook身份不能当事件。push commits可能截断或为空，changes必须可信Git diff。[Webhook头](https://docs.gitlab.com/user/project/integrations/webhooks/)、[Push事件](https://docs.gitlab.com/user/project/integrations/webhook_events/)、[实际事件API](https://docs.gitlab.com/api/project_webhooks/)。

官方19.x新增signing token确有webhook-timestamp（秒）、webhook-id与HMAC-SHA256(id.timestamp.rawbody)，whsec_去前缀base64密钥，多个v1/base64签名。这个协议与旧token不同，不能发明通用时间戳或默认fallback；本次只接受管理员token模式，不宣称支持该新增模式。未来若有明确需求必须真实wire门后单独扩配置，不提前加插件/签名registry。[官方signing说明](https://docs.gitlab.com/user/project/integrations/webhooks/#signing-tokens)。

## 4. Gitee

Decision：MVP沿产品密码模式，X-Gitee-Token恒时比对，Gitee JSON push取ref/after与repository.id，无已验证deliveryID则语义合并。Rationale：官方JSON文档区分password/sign、旧form不维护；hook_id表示hook不是delivery，timestamp也不是唯一事件ID。官方示例有Push Hook头与hook_name=push_hooks；真实门需覆盖版本实际头，解析固定二者且相互一致，不靠任意别名猜测。[原JSON/头](https://help.gitee.com/webhook/gitee-webhook-push-data-format)、[添加](https://help.gitee.com/webhook/how-to-add-webhook)。

官方加签文档使用毫秒timestamp与HMAC(secret,timestamp+newline+secret)，一小时差值、Base64后urlEncode，但正文又混有机器人SEC及URL参数表述；它没有为body提供GitHub式HMAC完整性。不可自行认定头传输rawbase64/百分号编码或用密码模式宽松兼容，当前明确签名模式unsupported；真正扩展需自有Gitee材料取得原wire再冻结。密码模式必须HTTPS部署，rawbody只用于本地digest/严格解析，不谎称token证明body完整性。[官方算法](https://help.gitee.com/webhook/how-to-verify-webhook-keys/)。

## 5. generic与Gitea/Gogs研究

Decision：管理员可信post-receive脚本默认固定JSON/event头、原body HMAC；可配置的JSON pointer只取有限ref/after/repository/before及固定auth/event/delivery头，不执行表达式、网络URL或动态代码。Rationale：产品明确任意自建Git/有限pointer，普通仓库hook脚本是当前实际consumer，不需要所有托管平台插件。token模式只显式允许，HMAC默认，不新增timestamp标准。

Gitea原生raw-body十六进制X-Gitea-Signature无sha256前缀，Gogs源码同HMAC、X-Gogs-Delivery/Event；可用generic固定header/profile表达，同样需真实自有push门才能称兼容，不能以GitHub别名头自动换provider。[Gitea官方](https://docs.gitea.com/usage/repository/webhooks/)、[Gogs官方源码](https://github.com/gogs/gogs/blob/main/internal/database/webhook.go)。

## 6. 第三方依赖实际边界

Decision：沿产品选webhooks/v6候选v6.4.0，只Github/Gitlab两consumer；stdlib负责先限额raw读、duplicate-key/深度、独立secret、安全错误与额外identity验证，库只接已限制bytes克隆。Rationale：v6.4.0 tagged源的Parse会io.ReadAll并drain/Close，不能直接把网络无限Reader交库；GitLab此版只有token而不实现新signing。库错误不回显providerbody。[官方release](https://github.com/go-playground/webhooks/releases/tag/v6.4.0)、[Github源](https://github.com/go-playground/webhooks/blob/v6.4.0/github/github.go)、[GitLab源](https://github.com/go-playground/webhooks/blob/v6.4.0/gitlab/gitlab.go)。

Alternative：安装全部平台适配/JSONpath库/重写全部GithubGitlab解析不必要。依赖候选是计划建议，不是已安装/已验收；第一门必须实际模块锁定和同raw签名/JSON限额负例，root唯一改go.mod/sum。

## 7. 固定窗口、关闭与比较键

Decision：Store内部UTC决定ReceivedAt，首事件固定[OpenedAt,Deadline)；0窗口包含创建它的首事件但不接纳第二事件。晚事件新代，due旧代不受修改。关闭的可信HEAD、来源和基线在DB外准备，最后window revision+current PolicyVersion+baseline identity复核后同事务closed/Enqueue/计数；不持DB锁Git，冲突重新完整准备。Rationale：等窗口有唯一持久关闭结果，又不引入窗口租约/第二调度框架。Alternative：滚动延长期限、按最后payload的after、只标closing再独立enqueue，都会漏/重复或选错提交。

比较键包括ProjectID/branch/name/排序静态选择集合/最终参数/原完整Definition摘要/来源摘要身份；不含build.id/number/node/workspace。新manual/auto创建均固定此键，retry继承原键。成功baseline优先原build的精确终态receipt（008的Kind=build_finished、中央CreatedAt与LastEventSeq/attempt吻合），按该时间/id稳定排序，不能拿UpdatedAt/节点At冒充完成时刻；旧无Kind证据或无比较键视缺基线，不猜迁移，保守full。

## 8. 可信Git diff和条件事实

Decision：SCM沿原gitRunner/bare/fullSHA，把已读取目标commit与成功baseline commit作tree-to-tree diff；不根据commits列表/API/工作区mtime计算。`git diff --name-status -z --no-ext-diff --no-textconv --find-renames=50% -l1000 <base> <target> --`，禁止用户diff驱动/hook/filter；NUL解析、改名双路径，无rename匹配时D+A同样覆盖。固定上下文预算、输出/路径限额，溢出安全失败，不截断伪无变化。[官方git-diff](https://git-scm.com/docs/git-diff)。

目标读取成功且base在已成功取得的授权完整历史中确实不存在，可记录baseline_unavailable/full；不能把fetch网络/权限失败当base缺失。已有两个commit即使非祖先也直接diff其tree，force push本身不导致漏变化。未知status/非法UTF8/控制路径安全失败，不把Git错误原文输出。

既有evaluateWhen只手动忽略changes，015增加具体冻结ChangeFacts给Preview/buildCondition/Run共享判断；full模式使changes为true，diff[]明确无变化，nil沿旧手动豁免。所有普通/post仍原顺序/预算/Authority，retry不重新比路径。

## 9. 秘密/审计与验收结论

Decision：--hook无外部引用时自产随机key，独立受限文件仅一次admin初始化/轮换返回；有完整env引用只读显式webhook_secrets_file，不Lookup宿主env、不混SCM/Agent秘密。DB只保存CredentialID/引用/指纹与安全审计；HTTP接收原body不落盘/DB/日志，Giteepassword等正文秘密仅内存。生成文件DB失败只留不可见自产孤儿，不凭孤儿授权请求；清理不进入本次retention框架。

本次已完成：基线/代码/产品/官方来源读取、core模板解析/selector/setup-plan、hooks={}语法读取与前后原则检查。未执行：任何providerpush、Git/DB业务原型、真实构建/审批/发布、race/vet或deps安装。SC/FR全部仍需后续实际验收；不存在“浏览文档就四来源成功”的证据。
