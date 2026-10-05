# 015 配置、输入限额与冻结条件

以下候选由根在前置接受基线上冻结。管理配置严格null/unknown/type/重复键检查；旧未提供字段维持行为。所有路径读遵守已有受限普通文件helper，不继承任意host env。

## 管理设置

```yaml
hook:
  enabled: true
  repository_key: '12345678'  # hosted仓库实际正整数ID；generic为管理员约定稳定字符串
  # secret: '${APP_HOOK_SECRET}' # 可选；省略则首次启用自产随机secret、一次返回
triggers:
  builds: [android, ios]
  quiet_period: 30s
  allow_upload: false
```

新增ProjectSettings.Hook/Triggers只由admin settings导入；每个显式块整体替换，项目Repository/Provider仍不可改。hook默认disabled；triggers defaults quiet_period=0s/allow_upload=false。显式builds非空/唯一合法名称；省略仅admin启用时真实来源唯一build可推导，保存实际名称；多build拒绝启用，不能用all自动扩大。修改选择/参数/input与provider认证都增加PolicyVersion，旧window关闭policy_changed。

Secret只完整`${NAME}`且NAME严格identifier，禁止literal/password/template/空null；不读os.LookupEnv。声明引用需ServerConfig.webhook_secrets_file，路径相对server.yml；独立自有0600普通文件，NAME=value沿现有限定envfile字符串规则，不执行shell/展开变量，最多128项且重复/非法NAME/NUL拒绝。只解析声明项，未知普通业务名字不借本文件注入Git/Agent；secret值至少32B、最多4096B且无控制字符，空/缺失安全失败。

无Secret引用：Server admin启用/rotate生成32字节crypto/rand，base64url字符串；仅独立hooks目录文件储存，不写DB或settings明文。外部reference配置不一次回显secret值，CLI提示明确引用；生成模式首次/rotate响应的secret一次仅admin可得。后续GET/重复启用不读回key。文件准备/DB提交未知不再生成另一key冒充重放，admin可查询safecredential状态后显式rotate。

`mybuilds[/-server] project init/add ... --hook --hook-repository-key ID`沿原provider/settings；hook flag与settings内hook冲突拒绝。已登记项目可`project hook enable/disable/rotate`（管理请求中的key只幂等管理输入，不代替WebhookdeliveryID）。enable/rotate返回JSON私有secret一次，普通table/list从不打印；调用者重定向0600保存。未配置hook不加载其材料，不影响本地init/run/doctor/help/version。查询固定project hook events/windows <name> [--json]，管理固定project hook enable/disable/rotate <name>；不出现hooktoken argv。

## generic有限描述

```yaml
hook:
  enabled: true
  repository_key: demo
  generic:
    auth: hmac_sha256             # 默认；token只能显式选择
    auth_header: X-Mybuilds-Signature
    signature_prefix: 'sha256='   # 只能空或sha256=
    event_header: X-Mybuilds-Event
    push_event: push
    delivery_header: X-Mybuilds-Delivery # 可省略，走无ID语义合并
    ref_pointer: /ref
    after_pointer: /after
    before_pointer: /before       # 可省略
    repository_pointer: /repository
```

generic只此有限字段；header合法HTTP token且不是Authorization/Cookie/Host/Content-*或其它provider认证头，名称间不重叠。token模式无signature_prefix；HMAC原body，不自创timestamp；JSON pointer是RFC6901的固定字段查找（~0/~1），最多16段，禁止根空pointer/数组通配/表达式/脚本。ref/after/before必须字符串；repository字符串或正整数标量规范成固定key；payload不得携params/build选择/allow_upload来授权。unknown metadata可忽略，generic声明字段存在且合法；无Gitea/Gogs自动探测，只admin显式描述可表达兼容。

## 所有限额（实现选定值，不伪称产品既有数字）

| 输入/消费 | 限额/期限 |
|---|---|
| raw request | 8MiB，MaxBytesReader+有界读取，超一拒；只application/json，拒非identity Content-Encoding与form |
| 接收 | 整体10s父请求期限；不Git/不等待节点；全服务Header总32KiB |
| 关键header | 单值≤4096B，认证/event/delivery/repository键要求一份、不逗号拼接；delivery≤128B安全非空字符串 |
| JSON | UTF8单object、depth≤64、tokens≤200000、member name≤256B、任一string≤64KiB；全树duplicate key拒，尾随值/null非法 |
| branch/ref | 沿既有check-ref-format，branch≤1024B；只能refs/heads/，剥一次；SHA40或64hex规范小写、非零且当前对象格式相符 |
| repository_key | ≤1024B，无控制字符；hosted规范正整数ID，generic安全字符串/正整数 |
| 选择/参数 | ≤64build；每build参数≤128、单值≤4096B；既有未知/必填/choices严格校验；不从pushbody取参数 |
| quiet_period | 0至24h，duration解析且不能负/null；默认0s，截止溢出拒绝 |
| secret文件 | 总1MiB、128项，每值≤4096B；目录0700/叶文件0600、自有普通文件、拒links/特殊文件/替换 |
| 窗口关闭 | 父总45s，读取/source与全部diff共用，不因CAS重试重置；server tick1s、due页最多100，逐个受ctx取消/锁控制 |
| Git | 原process.Run/SSH两键/whole预算；每普通命令stderr32KiB；diff stdout总8MiB（也受更小父ctx），不回显raw |
| 变化路径 | 每build最多100000个、合计8MiB；单path≤1024B、叶≤255B；UTF8相对、非控制/反斜杠/盘符/..段，sorted unique |
| 图与rename | 直接两固定commit tree；rename -l1000，有界输出；未识别rename以D+A双路径同样有效，不依赖检测质量 |

无法有界验证或超限是hook_limit/scm_changes_limit，不截断结论、不伪full或empty。payload metadata strings超限同样拒，以免库另持无界内存。

## changes与权限

配置When不增语法，沿既有doublestar相对glob。nil Changes=旧manual/local忽略；full=基线缺失/不可得、变化条件通过；diff=真实Paths，即使[]也已确定无变化。分支与参数仍分别AND。每build Snapshot独立Changes+ComparisonKey，所有step与post沿同一事实；Agent RunOptions只拿Task已验证snapshot，用户Facts的changes.*无权伪造。preview dry-run缺此事实沿手动豁免，不能发Git网络。

build-level仅真实branch/params/changes判ready/skipped，不拿编号/node/workspace模板pending误跳过。任一所选upload先检查当前admin导入allow_upload，no-upload请求仍受scope；14审批跳过不批准发布，19报告失败不封成pass，node/app/unknown保护都不被Webhook跳过。

手动Trigger/local不等窗口；008retry保存原Changes和staticconditions/ComparisonKey但新AutomaticWindowID为空，不查询当下baseline。定义/参数/范围改变使comparison key不同→full，不能混旧成功条件。

唯一build推导仅admin启用准备阶段：以main（若获授权）或首个明确非glob授权分支真实resolvePipeline；无法选具体分支/来源不唯一则要求显式triggers.builds，不新增探测分支协议字段，不在接收请求中Git。
