# 011 配置与CLI契约

沿config.Step既有upload字段，不新增kind或发布DSL；字段类型和公共类型单一来源[Go契约](go-api.md)。以下为待实现配置示例，不表示当前Run支持发布。

```yaml
steps:
  - kind: run
    name: archive
    run: ./ci/archive-and-export.sh
  - kind: artifact
    name: ipa
    paths: [output/*.ipa]
  - kind: upload
    name: app-store
    target: app_store
    file: output/*.ipa
    app_identifier: com.example.app
    credentials: '${APPLE_API_KEY_FILE}'
    submit_for_review: false
    automatic_release: false
```

file只匹配本attempt已经完整收集的普通IPA快照；恰好一个，不读取任意新工作区文件替换。允许原文件模式与已收集来源关联，不接受多IPA/链接/不同build/未知路径。实际副作用前重核size/SHA256、应用/项目Number/VersionName、IPA结构/主app签名/profile分发Team，与005已验收实际核验共用，嵌套extension身份/签名亦不能错误忽略；不重新构建/改签名。

credentials必须完整`${NAME}`节点env引用，严格名字、缺失拒绝；值是节点私有API key JSON文件路径，不是key明文。本功能采用fastlane官方Team API key JSON：必填key_id（有界ASCII标识）、issuer_id（UUID）、key（内嵌PKCS8 P-256私钥PEM）；可选duration为1..1200秒整数、缺省200，in_house只允许false、缺省false。拒绝未知/重复字段、null、空值、错误类型以及key_filepath/key_content/filepath；不另设p8路径格式。JSON≤64KiB，自有0600普通文件/no symlink/FIFO/nonregular；在受限读取后解析并校验密钥，不从issuer_id推断签名TeamID。

同进程自产0700HOME/0600材料副本，key/JWT/密码不放argv，受限request/result文件或匿名stdin传实际受控入口。只允许实际原型验证的API key文件认证后端；需要`-jwt`明文argv的Java/JWT分支拒绝，不静默fallback。不扫描宿主私钥/keychain/Apple ID/session/应用密码。节点bundle执行目录来自明确安装路径而非仓库替换的Fastfile/Gemfile；PATH仍明确工具路径和现九项环境白名单，不继承FASTLANE_USER/SESSION/DEBUG/代理跳TLS/完整环境。

上传目标app_store只允许上述Apple字段，track/release_status/channel、custom argv/query_argv/result_file等跨目标字段拒绝；AppIdentifier安全反向域名，模板只按既有一次渲染且运行后严格校验，不从IPA暗推另一应用。Runner必须ios、macOS真实Xcode/签名/fastlane能力，Flutter和native共用publisher。

两布尔缺省false，automatic_release=true且submit_for_review!=true拒绝；submit=true同一原Run预算内处理/前提齐备才执行6动作。默认上传不修改现有版本/元数据/发布选项。未授submit的处理超时保uploaded+未提交，不重传、不在terminal之后继续提交。Run本地生效upload在任何脚本前拒绝，dry-run纯渲染脱敏，不读node secrets/调用工具/访问商店。

## 待实现命令

沿010共同app binding与publish ls/show/query/confirmCLI及同一safeDTO；目标参数为--store app_store、bundle ID、--node原授权节点、--credentials-env APPLE_API_KEY_FILE。凭据只从指定受限node材料获取，CLI不接受p8/JSON明文或私有值argv。

`mybuilds trigger PROJECT --build ios --allow-upload --idempotency-key KEY`沿现权限；`build retry`仍admin+allow-upload、原snapshot参数/定义及新号，不沿用旧发布/报告pass。提交/auto选择来自冻结YAML，没有terminal `publish submit`命令或额外强制授权flag。

`publish ls/show/query/confirm`沿共同当前管理员/approver边界，query/confirm不授写Apple权。工具缺失实际doctor失败，未指定商店材料本地doctor skipped；节点远程明确材料doctor使用当前独立身份任务，不要求用户token进入节点。安全工具版本严格数字格式≤128bytes，无rawoutput/路径/环境。

## 原型与工具锁

fastlane2.240.1仅候选。实施先真实Ruby/Bundler无凭据REST fault计数与JSON/file材料schema，证实具体非幂等请求无未知重发，再由root生成实际安装组合的共同Gemfile.lock并核对版本/摘要；锁本身不是安全证明。Transporter真实会话/分块/最终提交边界在明确授权材料的首个ASC上传门核实，可复用该次上传证据、不为检查重发；没有充分证据不得标doctor工具已安全核验。不安装/更新宿主工具，不创建Gem假锁，不把没有真实签名/AppStore应用的模拟退出0当验收。
