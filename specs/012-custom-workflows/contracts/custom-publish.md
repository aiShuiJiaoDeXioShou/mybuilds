# 012 custom发布输入、回执与查询契约

## 原配置

```yaml
version: 1
params:
  version: {default: "1.2.3"}
steps:
  - kind: run
    name: package
    run: bash ci/package.sh
  - kind: artifact
    name: collect
    paths: [dist/package.bin]
  - kind: upload
    name: custom-release
    target: custom
    app_identifier: org.example.application
    file: dist/package.bin
    credentials: "${CUSTOM_PUBLISH_CREDENTIAL}"
    argv: [bash, ci/upload.sh]
    working_dir: .
    result_file: .mybuilds-custom-result.json
    query_argv: [bash, ci/query.sh]
```

示例仅可在可信项目、已明确custom应用绑定及admin --allow-upload的remote执行。result_file既存拒绝，用户不得用它引用旧证据或未知文件；working_dir的`.`沿现有validatePath合法仓库根约定，不放宽越界规则。argv/query_argv原文不插值；脚本读取系统输入路径及声明env。只有普通run/artifact在发布段之前，reports按019封存；有效approval须014已验收，不自动添加或跳过审批。

custom要求明确唯一file、app_identifier与结果文件；credentials可省略代表无凭据自有渠道，但给出时必须完整节点env引用、不接受inlinesecret。所有标准限额/strict/null/未知字段沿config，不能新增shell拼接或rawresult对象字段。

## 实际输入与秘密

固定系统env MYBUILDS_PUBLISH_INPUT 指向本次私有0600 JSON；MYBUILDS_PUBLISH_RESULT 指向本次已经验证的结果位置；MYBUILDS_PUBLISH_CREDENTIAL仅显式声明此步凭据时注入，无token继承。用户env不能覆盖MYBUILDS_保留键。

输入schema=1包含intent_id/authorization_digest、完整ref、app_identifier/version_name/version_code、artifact{id,path,size,sha256}、report_seal_digest/report_ids、params声明的最终普通值、release参数中的channel（如声明）。artifact.path是本次私有原快照副本而非原可变工作区；输入不包含admin/user/node token、整份环境/秘密材料文件正文。credential参数由脚本从固定受限env读取；已声明file reference实际路径属于节点秘密，不入argv/central。数组显式[]，参数输入仅私有不回显。

初始准备输入不授动作；grant成功或可能已提交后才写已授权input并fsync，Upload仅调用一次原process.Run。输入被替换/失败在授权后属unknown，不以未Started假定未发送；当前fullRef/ordinaryNS/Authority仍有效才启动。应用(同store/app)guard由中央授予时持有，原应用/产物/JUnit/版本与输入全部核对。

## 输出schema（唯一新result文件）

成功示例是协议形状，非实际发布证据：

```json
{
  "schema": 1,
  "intent_id": "<original-uuid>",
  "authorization_digest": "<64-lowerhex>",
  "app_identifier": "org.example.application",
  "artifact_id": "<original-artifact-uuid>",
  "artifact_sha256": "<original-64-lowerhex>",
  "version_name": "1.2.3",
  "version_code": 17,
  "status": "uploaded",
  "evidence_code": "remote_receipt",
  "remote_id": "receipt-17",
  "action_confirmed": true
}
```

完整允许字段仅上述加request_sha256/response_sha256（可选有限摘要）；未知/duplicate/null/多值/坏UTF8/深度值数超限拒。status枚举uploaded/processing/submitted/published/failed/unknown；不是脚本退出码映射。remote_id≤128安全非控制字符，无URL/token/任意payload。status=failed仅evidence_code=confirmed_not_sent或remote_rejected且用户脚本明确确认无副作用；started不自报，由process真实OnStart决定。成功必须exit0、action_confirmed=true、精确意图/AuthDigest/app/artifact/version及非空remote关联、evidence_code=remote_receipt或remote_state，固定原字段任一不一致仍unknown；claimed published只是可信脚本的实际remote证据主张，系统不假设审核或公开上架。

无回执、非零exit、含声明secret、坏文件、不一致、失权、logger或journal失败都不改写原结果以求success；授权可能已提交后unknown/guard保持。原命令Reason优先；真正clean/stop由process与Close决定，不靠result_file自报。stderr/stdout原文合计64KiB受限私有捕获、不镜像公开日志；超限主动回收，仍不能推断无发送。公开只固定摘要/安全有限state与receiptIDs。

结果必须本次新regular单link文件，安全真实parent、NOFOLLOW/NONBLOCK有限读取、读取前后identity复核；Reject非法文件不删除unknown替换。普通预算包括文件准备、命令、解析、封存与回执，0不得退回无限；Close独立系统期限只自产。post不修改原报告或产物放行证据，Authority失权/cleanup不确定禁止后续always。

## unknown与人工/查询

grant前错误没有已授意图，不启动command；grant可能提交后的启动前断连同unknown，不能靠NotStarted解锁。真实upload.finished先关slot、拒lateAuthorize，再对原私有候选逐条精确FindNodePublish生成完整manifest；已授unknown条不能消失。build终态及008终态只读确认不改变appguard，物理stop只解执行保护；不会自动重跑custom命令。

沿010 `publish query <intent-id>`/`publish confirm <intent-id>` 当前admin入口，不引入另外retry/submit命令。query数组不存在→明确not_supported且unknown保持；存在则当前NodeActor/session在30s管理任务中，以原SHA/QueryArgv/绑定节点隔离Checkout并执行一次process，不走原Run普通/post。输入是原intent/context，不含新grant；由系统写MYBUILDS_PUBLISH_INPUT/RESULT，输出同有限schema。用户声明query只读，系统不能证明任意脚本无remote写；管理员须明确执行，所有不足/空结果/exit0无有效回执均保持unknown，没有自动解除或换应用。

上传input的artifact.path必需，是本次稳定原快照副本；query input明确省略path，只含原artifact id/size/sha256及原版本/seal/intent/AuthDigest等关联上下文，不下载旧产物或重建/调用PrepareUpload。原私有CustomQueryContext由Store派生并按当前同node/session管理身份取得，节点重启后也不依赖旧journal。query的project/build.name/git.branch/step.name是原冻结事实，sha/id/number来自原精确context；node.name与workspace是当前受授权查询身份及本次隔离目录，不冒称原执行环境。只解析一次原声明env/工作路径和当前明确secret引用，argv原式。task整体≤64KiB，过限拒绝不截断；该私有context不得出现在公开列表详情中。

足够证据按原intent/action/artifact/app/version精确核对才共同CompletePublishQuery更新状态；confirmed_not_sent/remote_rejected须实际可信脚本明确证据而非零搜索结果。manual confirm需要ExpectedIntentDigest+非机密依据/evidence SHA/Note（有界），当前admin同一transaction复核保护，重复同决定幂等/冲突拒。确认不授第二次custom或商店动作，不允许只StopKnown/PID/脚本exit解除。

custom binding明确ManualEvidence.Source=manual_attested，以现有BindApplication admin输入审计原project/node/store/app及安全依据；其verified归属不表示远端账户/应用真实性已GET验证。错误凭据或外部格式身份由可信用户脚本明确核对，所有AAB/IPA已有平台核验仍复用，两店doctor验证来源不被覆盖。

## 实际验收要求

自有receiving endpoint与actual trusted script：一次命令/单intent收件计数1；授权响应与结果ACK分别丢失，无第二发送、guard保持；并发另一build不能入发布，精确查询或manual决定只更新原intent。所有文件/声明secret/无效JSON/无result/非零exit/失权/NS超时/清理/取消实际负例，真实PID/PGID与无关sleep安全，双库同suite。用户自行调用Fastfile内部重试属于用户脚本，系统不谎称已限制其所有网络请求；受控内置两店single-action规则仍010/011负责。
