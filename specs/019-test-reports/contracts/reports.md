# 019 配置、XML与结果契约

## 配置

```yaml
version: 1
builds:
  test:
    reports:
      junit:
        paths: [build/test-results/**/*.xml]
        required: true
    steps:
      - kind: run
        name: test
        run: sh ci/test.sh
```

报告paths可使用参数及project、build.name/build.id/build.number、node.name、git.sha/git.branch这些build层事实；workspace/step.name属于私有步骤上下文，报告路径不能使用，执行前明确拒绝。

required省略=true；paths沿既有非空相对glob/一次模板语法，渲染后也拒绝绝对路径、反斜杠、盘符、控制字符和任何..段。模板引用的参数与系统fact不再次解释，run正文不插值。--step仍预检查完整选中build，使用本次新鲜性与最终required，不读另一build/旧产物。

required=true要求普通结束每个声明模式至少一个当前新鲜报告；准备步骤匹配0不会提前失败。required=false只允许缺失，有非法文件照样失败。无reports、build.when false、precheck零动作失败或全部条件跳过不创建假seal、报告目录或required失败。

## 新鲜性

基线匹配保存identity/mtime/SHA，不解析/删除旧XML；实际每build第一个ordinary动作前建立，在同一Run普通预算内。首次文件或identity/mtime/hash任一变化接受本次；完全相同视旧。当前已接受但没再变化的文件保持一次计数；路径改写替换，路径删除移除。远程全新clone也基线排除仓库预置XML。复制前/打开后/复制后identity与字节hash必须稳定，不允许源竞争修改污染快照。

每个实际ordinary run finished之后且先确认本次进程停止，再检查已有新XML；已知原失败/取消优先，报告错误不改写原Reason。final普通检查和原XML中央确认之后封存；post的任何改写都不重新解析/封存普通证据。

## 有界JUnit子集

UTF-8 XML单root testsuite或testsuites，无namespace。允许嵌套testsuite、testcase、failure/error/skipped、properties/property、system-out/system-err的合法父子位置；允许metadata属性但重复属性、未知语义子结构拒绝。标准xml开头声明允许，其它processing instruction、Directive/DOCTYPE、外部实体、未知entity拒绝，不安装自定义实体/编码resolver、不联网。

合法父子矩阵：testsuites仅含testsuite；testsuite可含testsuite/testcase/properties/system-out/system-err；testcase可含failure/error/skipped/properties/system-out/system-err；properties仅含property；其它叶元素仅文本。结构容器除子元素外只能有空白文本。suite/case name可省略，缺case name的诊断Case为空，有值仍遵守名称限额。标准声明仅在可选UTF-8 BOM之后的文件开头出现一次，version必须1.0，可选encoding仅UTF-8、standalone仅yes/no；不依赖Decoder对PI的宽松接受。namespace声明本身也拒绝，不能仅靠Strict。

每个testcase计一次，至多一个failure/error/skipped结局；嵌套suite的tests/failures/errors/skipped如果声明必须与实际子树计数一致，不再次加总。缺计数属性时按case推导。空文件/多root/坏结构非法；合法零case suite允许0计数。名称/文本属于不可信诊断，不能直接当终端控制序列。Diagnostic.Outcome只为failure/error；skipped计数不重复作失败摘要。

time为非负十进制秒，最多9位小数，整数部分有界；不接受负数/指数/NaN/Inf。Counts.DurationNS为case time和，case未声明time为0；suite汇总time只作已验证metadata，不再叠加也不强制等于case和，避免并行/overhead重复。所有累加检查溢出。

| 输入/结果 | 限额 |
|---|---|
| patterns/匹配文件 | 最多32模式，当前最多64文件；重复匹配按相对路径去重 |
| 原XML | 单文件8MiB、当前集合64MiB；baseline亦有界，巨大旧文件拒绝而不无限hash |
| XML深度/属性 | 深度64、每元素属性64；属性名≤256B、值≤4096B |
| testcase | 当前集合100000；case名称≤512B |
| 耗时 | 单case≤24h、汇总≤365天，ns整数 |
| Diagnostic | 最多20条、总20KiB，Case≤512B、Message≤1024B；先处理控制字符和脱敏再UTF-8安全截断 |
| 相对Path | ≤1024B，非绝对、无控制/反斜杠/..，leaf沿现有255B文件名边界 |
| checkpoint/final纯本地检查 | 每次总≤10s，且不超过ordinary剩余ns；上传/回执按原协议时限并受ordinary预算；0不启动，不退无限 |
| 中央所有文件 | 同attempt所有purpose合计128文件/4GiB；XML另受64文件/64MiB，不可各用途另拿配额 |

检查所有raw字节和所有解码属性/文本/路径中的已声明非空secret；命中report_secret，不上传或公开该XML，不重写原字节伪造报告。诊断按同一声明秘密脱敏。合法原XML下载字节、Size、SHA256须相符。

诊断总20KiB按UTF-8字段值字节之和计算（PathKey/Case/Outcome/Message），不含JSON结构。纯parser无文件Key，PathKey为空；真实文件consumer补经验证Key后，集合聚合再次执行20条/20KiB、100000case和365天耗时限额。达到诊断显示上限后仍必须验证剩余全部XML/秘密/计数，不把截断当解析成功。

## 结论与安全结果

公共BuildRun.reports/BuildView.reports在未配置/零动作时省略，其它情况下包含ReportEvidence及SealDigest（本地无需远端确认但有实际快照manifest）。Counts为Tests/Failures/Errors/Skipped/DurationNS；安全Files按Path排序，含UUID、Key、相对Path、真实ordinary run来源、Size/SHA，不含绝对workspace/私有snapshot。

Outcome：pending（尚未final）、passed、failed、missing（只optional最终缺失）；Reason限空/report_failed/report_invalid/report_missing/report_secret/report_error/timeout。Failures或Errors非0→report_failed。required缺失→report_missing。有非法内容/路径/限额→report_invalid；读写快照错误→report_error；耗尽普通budget→timeout。原命令Reason/ExitCode仍优先。

检查/解析/上传耗时与确认回执都计普通budget。用户cancel但Authority仍有效时，可以用受10s及ordinary剩余budget限制的取消独立上下文收集已经产生的XML；不因WithoutCancel延长Authority。Authority、journal/日志/进度/文件确认持久化失败沿007闭锁，不能再开始下一步或always；原失败优先保留。

生效approval/upload/reports之外尚未交付能力仍按原规则拒绝。019提供同一执行封存证据，不能声称已实现商店/审批放行。将来进入approval/首次upload前只核验已有seal+文件，不重新读post的工作树；变更/缺失不能放行。
