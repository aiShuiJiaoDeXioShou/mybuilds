# Phase1 Data Model：Apple发布与逐动作证据

字段单一来源为[Go契约](contracts/go-api.md)及010共享发布契约；这里只说明持久关系与约束。

## 应用绑定与保护

ApplicationBinding由(Store,AppIdentifier)全局唯一，ProjectID不可变，只有实际授权节点GET核验Apple AppID与bundle ID后verified。project改组不改归属；删除项目/解绑不得孤立历史、活动或unknown证据。保护行BindingID唯一、无TTL：原节点槽/进程租约与应用保护分别存储，StopKnown/StopConfirm不能解应用锁。

同Run发布链持一个应用保护，每次真实变更有独立动作意图。后动作授权须原链当前guard、前动作confirmed、原Ref有效、普通NS>0、当前admin/allow-upload、原IPA和report seal仍一致。前动作confirmed不重做，后动作未授不是unknown；授予时即unknown，不能据未Started推未发送。

## 原快照与苹果动作

发布链绑定原BuildID/number/SHA/DefinitionDigest/ParamsDigest、完整执行Ref、ArtifactID/size/SHA256与报告seal/IDs，以及原明确submit/automatic布尔。存不可变摘要/ID，不存p8/密码/凭据内容/完整脚本；credential仅节点引用，安全DTO不返路径。retry使用008新身份/编号和新报告证据，不复制旧发布权或回执。

每个Apple动作就是一条共享PublishIntent，不新增AppleAction表或第二序号系统。节点请求前生成IntentID并持久化；Action、PreviousIntentID、请求摘要/授权UTC、AppleEvidence及状态/安全原因附于原意图。同Ref/Index/Action唯一；后动作引用同链前条已确认IntentID，固定顺序，不接受改变原body或以成功动作重新领取。confirmed仅代表该动作，不等于published；第一未确认动作unknown阻断后续。

动作链：upload_binary→（仅明确submit且processing有效）select_build→set_release_policy→create_review→add_review_item→submit_review。已有草稿若不属于本链/已有其它items，拒绝而不接管；预存在且精确相同的选择/策略只是下一动作的只读前提，不发变更请求、不造意图/回执/Started。缺编辑version/合规/元数据由管理员在外部准备，不自动创建或补齐。

## 状态与恢复

- 未授动作：not_authorized，不启动；超预算保已确认uploaded，提交事实false，不产生unknown。
- 已授动作：unknown+持guard，即使启动ACK丢失或取消/过期也不重发。
- 动作已确认：confirmed，安全结果可能uploaded/processing/submitted；published只能远端真实可分发证据。
- 确证未发送或明确拒绝：failed；必须明确该具体动作/来源，不从exit/timeout/kill推导。
- 实际upload step_finished关闭授权slot；同一链全部已授动作已确认或无副作用失败、且真实停止才解guard，RecordPublish中间回执不解锁。关闭后按010只读查精确候选ID及完整集合，不能用活动slot的404判未授权；terminal必须完整manifest。处理/审核中仍只query，不占普通构建槽。
- App Review REJECTED是已提交后外部结果，不抹上传/提交，不把原动作改成未发送。

控制端重启不授新权；expired/ref/session revoked及停止未知规则沿008/007。终态后的查询由原授权节点当前独立身份获取只读请求，旧Ref仅原证据不鉴权；不能创建新提交、Run或号。

## 核对与审计

QueryJob仅精确原Intent/Action、request key、当前node身份、过期界限；GET-only、独立有限超时，不占构建slot或刷新原lease/预算。足够的真实关系证据可确认原动作；零/多候选、缺receipt来源/ID、权限/网络错误维持unknown，保safe evidence。

ConfirmAudit关联精确Intent/Action/当前revision/digest、admin ID、UTC、受限非机密依据和决定。相同决定幂等；冲突/错归属/仅停止依据拒绝。人工确认不得重授执行权。必要原产物/report/审计/unknown guard标记被020保护。

## 双数据库约束

相同迁移/事务suite涵盖应用唯一、BindingID guard唯一、原IntentID唯一、同链前条关系、receipt相同摘要幂等/冲突、授权slot关闭、audit决定唯一与外键RESTRICT。Store.write使用既有短串行tx；授权和执行回报tx末尾再次检查控制端锁、原fence、UTC期限和策略，不能跨到期提交。20同app多build多node竞争仅一个授权；不同app独立。文件/网络不放事务内，读取后的Commit仍复核。
