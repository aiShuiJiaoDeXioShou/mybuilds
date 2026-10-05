# 011 HTTP与权限契约

共同发布HTTP/DTO单一定义引用[010 node-http](../../010-google-play/contracts/node-http.md)；Apple增加本页目标、动作与证据规则，不另造客户端transport或版本协商。字段类型引用[Go契约](go-api.md)。同一项目admin/approver安全表格/JSON不含credentials/path/argv/env/rawerror；当前未发布006/007API与新CLI一起升级，保持既有行为与配置兼容。

## 用户入口

管理员登记 app_store + bundle ID +原授权node + credential ref，中央仅保存引用并请求当前独立节点GET-only doctor核验绑定，跨项目同store/app唯一拒绝。未知/活动/历史必要关系禁止解绑删除。

沿共同application binding、publish ls/show/query/confirm入口；没有submit、release-now、重新上传、创建版本或刷新旧授权入口。触发/retry仍admin+明确allow-upload，即使when=false也先校验；approver只读publish/log/关联证据，trigger仅原status/允许trigger，不读私有发布详情，node token不当用户。

query只请求精确原Intent动作，有限request_key，POST到本项目只排只读核对工作，节点对Apple只能GET；admin明确发起，离线pending可见，不占构建槽。人工confirm必须原IntentID/ExpectedIntentDigest/Outcome/EvidenceCode/Note/EvidenceSHA256及确切remote evidence，与原动作/当前revision匹配；保存admin/UTC/决定审计，相同幂等、冲突409、错身份403，不能拿StopKnown作依据。

## 节点入口

共同authorize/receipt、FindNodePublish只读授权事实与management query claim/result路由引用010。node Bearer仅当前独立身份，自身NodeID/当前SessionID且项目授权；PublishAuthorization.Ref原普通执行权必须未过期且snapshot/step/原budget匹配。授权即unknown+guard；重复不再给执行grant。每个Apple实际变更单独授权，前后不是同一回执。IntentID在请求前节点fsync；活动授权slot的404不证明未提交，真实step_finished关闭slot后再逐条精确查全部持久化候选，禁止未知授权从terminal manifest消失，不增批量查询API。

严格JSON拒未知/重复key/null错误类型/超额64KiB；摘要/IDs/状态白名单与真实绑定匹配。只写固定reason，无远端body透传。OnStart/StopKnown与副作用确认分别存，进程停止不解unknown appguard。

PublishView在010共同安全字段之外，Apple evidence引用go-api的安全ID/状态/摘要字段；原完整Ref只不可授权opaque归属证据，不含授权granttoken或秘密。PublishIntents用于真实已授动作完整manifest；它与019报告/007artifact/log终态集合一起校验，缺一不宣称完整成功。

## HTTP结果

共同status/错误码沿010：400严格输入，401缺/失效身份，403角色/授权，404安全缺失，409状态/重复冲突/应用保护/旧fence，503控制锁或不可用；不会以HTTP200或子进程exit0单独认定远端动作。安全错误message固定。网络/超时/未知响应保持unknown，服务端明确无副作用拒绝才可failed。

## Apple只读边界

固定官方origin https://api.appstoreconnect.apple.com，正常证书/hostname校验、禁止redirect到异源；每个查询method只有GET，path由已保存安全ID构造，不接受用户任意URL。应用/build/版本/submission/items/release是有限GET集合；next分页仍核对同origin、白名单path/上界，不能调用ensure_version/create_review_submission等带写方法。wrongCA/hostname/凭据和零/多候选全部保unknown。

bind doctor也只GET，本地doctor可工具/自有明确材料检查；没有材料skipped，不扫描个人account。未知upload缺原关联、未知submit仅见build时返回safe evidence和unknown；查询须显示“证据不足”，不能反复写Apple补关联。
