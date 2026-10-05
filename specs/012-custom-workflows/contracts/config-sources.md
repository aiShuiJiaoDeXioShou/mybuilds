# 012 配置、来源、命令契约

此页是规划，不代表命令/字段已实现。source既定枚举为auto/repo/profile；bindings不是第四source。

## 服务端方案

```yaml
build_profiles:
  company-android:
    file: ./profiles/company-android.yml
  company-flutter:
    template: flutter-android
```

每项严格template/file二选一，不接受params/extends/builds/其它字段；名称沿safeName≤64，保留四builtin名称不得覆盖它们。template只四内置名称。file支持~及相对server.yml目录，加载后canonical路径仅私有；不得从仓库请求提供控制端路径。全部明确声明方案在server.New启动前受限加载并验证，一项坏即启动失败；不执行/下载文件、不热重载。每次新进程读取当前方案副本，正在排队/运行/retry不受影响。

file必须version:1根级单build YAML，不允许builds键，即使仅有一个build；notifications根级原语义保留，其它字段仍严格Parse/Validate。四builtin提取对应唯一android/ios Build，再对每个项目绑定深复制。用户local init --template接受现有合法single或named完整文档，不受方案只能单build的限制；共用相同strict字段/长度/文件安全与config引擎。

## 项目绑定

```yaml
pipeline:
  source: auto
  file: mybuilds.yml
  builds:
    android:
      profile: company-android
      params: {version: "1.2.3", channel: internal}
    ios:
      profile: native-ios
      params: {version: "1.2.3"}
```

旧pipeline.profile/pipeline.params规范为default；两者都与pipeline.builds互斥（旧params-only repo default合法保持）。仅对有效profile来源，每个输出build必须明确profile，不猜同名内置。repo来源允许同名params-only build设置；绑定不会拼进repo集合；任何对所选不存在build/未声明参数的实际覆盖拒绝。profile来源缺绑定或有params-only项拒绝。Pipeline.File缺省mybuilds.yml，仍安全仓库相对路径、无模板/glob/控制字符/..；profile不读取该文件，不以它作为admin路径。

| source | ReadPipeline.FileMode | 最终选用 |
|---|---|---|
| auto/空默认 | optional | 固定SHA路径普通文件存在→repo；仅确实缺失→完整绑定集合；缺失且无绑定报错 |
| repo | 空/required | 固定SHA仓库完整文档；缺失报错 |
| profile | none | 只固定Git SHA、不读取该配置；只绑定集合 |

目录、symlink父/叶、gitlink、超限、无法读取/获取提交、坏YAML绝不当Missing。无需为fallback查询第二个HEAD。未选择的repo build不执行但原Parse完整文档严格结构检查仍保留；所有所选参数/模板/权限在when之前整批校验。repo与profile名称集合不合并，改来源不映射default↔android/ios。

参数优先级named trigger > shared trigger > pipeline.builds.<name>.params（default兼容pipeline.params） > 原定义default。shared项必须所有所选定义都声明；每scope内重复、未选择scope、unknown/required/choices拒绝。normal值进入内部冻结定义，公开只有parameter_keys；secret保持节点引用而不解析。

## CLI管理与本地行为

```text
mybuilds project init <name> --repo <trusted> --nodes <names> --framework native|flutter --platform android|ios|android,ios
mybuilds-server project add <name> ...同上framework/platform...
mybuilds project set <name> --settings <local.yml>
mybuilds-server project set <name> --settings <local.yml>
mybuilds init --template <local.yml>
mybuilds init --framework native|flutter --platform android|ios|android,ios
```

framework显式必须配platform；仅platform时framework默认native；两个platform按android、ios规范化，重复/空/未知拒绝。框架平台flags形成source:auto+对应命名build绑定，不写仓库；与--settings或显式--file互斥。未传flags沿现有默认auto/file且无绑定，缺文件仍报错；--file和--settings继续互斥。settings客户端按当前cwd受限读取，HTTP只传typed内容，不把本机路径发控制端。set顶层块未给保留、显式给整个替换，null拒绝；与以后retention/triggers独立，不增加组继承。

沿现有POST /api/projects及PUT /api/projects/{name} typed ProjectRequest.Settings；不增加profile网络CRUD。管理员本机操作在线时仍独占拒绝，用remote管理。project安全视图只profile名称/来源/file/parameter_keys，不返回方案文件、参数值或原steps。列表详情可加origin安全摘要字段，不提供执行命令下载。

trigger选择--build/--all及命名--param仍原入口，全批固定同SHA/原子创建；相同请求key按既有摘要返回原batch，重放不读新方案。请求key不包含当前scheme内容，避免方案改变导致旧key另建；首次prepared Origin需在Enqueue受控事务校验current project policyversion。省略选择仅一个build合法。

本地run无仓库文件明确失败（包括显式--file缺失），不读取remote配置或server profiles；dry-run不doctor/secret/IO执行，只验证参数、模板与安全摘要。实际local有效upload仍在任一用户动作前unsupported。native/Flutter工程版本、build.number、runner.framework/labels沿004/005/009，scheme不自动添加商店upload。

## custom管理请求的实际扩展

沿010唯一POST /api/projects/{project}/applications。target custom时原BindApplicationInput.Custom含source=manual_attested、evidence_code（仅明确人工归属依据）、note及evidence_sha256；当前admin审计，凭据只ref，安全ApplicationView含VerificationSource但不raw Note/秘密。没有远端doctor成功的假回执；与两店202/pending→doctor_verified清楚区分。CLI `project app bind PROJECT --store custom --app-id APP --node NODE --verification-file PRIVATE_JSON` 是此同入口的真实consumer候选，不新增binding registry服务；file为0600普通≤64KiB严格JSON，不接受inline凭据或无限Note。给出credentials时沿010 `--credentials-env NAME`，省略只代表此自有渠道无凭据，不继承宿主环境。

query/confirm沿010原HTTP与CLI，不增加另一个执行/恢复/submit路由；Custom variant由共同Store/Server严格核对，不允许Google/Apple请求借manual替代其doctor核验。
