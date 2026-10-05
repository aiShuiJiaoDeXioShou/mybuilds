# Google Play 配置、工具与证据契约

## 流水线与预检查

沿既有四种step，发布段必须在全部普通run/artifact之后；审批与custom各由对应模块接入；尚未完成的能力在用户脚本前明确拒绝。分发配置如下：

```yaml
version: 1
builds:
  android:
    runner: {platform: android, labels: [android-sdk, play-publisher]}
    params:
      version: "1.0.0"
    env:
      APP_VERSION: "{{version}}"
      BUILD_NUMBER: "{{build.number}}"
    reports:
      junit: {paths: [app/build/test-results/**/*.xml], required: true}
    steps:
      - kind: run
        name: release-and-test
        run: ./gradlew :app:testReleaseUnitTest :app:bundleRelease --no-daemon
      - kind: artifact
        name: aab
        paths: [app/build/outputs/bundle/release/*.aab]
      - kind: upload
        name: play
        target: google_play
        file: app/build/outputs/bundle/release/*.aab
        app_identifier: com.example.prepared
        credentials: "${GOOGLE_PLAY_JSON}"
```

file只匹配本次同attempt已中央确认的普通artifact SourcePath，不重收集工作树；必须恰一个AAB。匹配APK/其它build/post产物/multiple/changed拒绝。空track规范化internal；track只接受官方实际精确存在且binding.AllowedTracks允许的名字，无alias回落。production必须原admin显式选定，配置/参数模板最终值原样冻结；retry沿原选项，不提升。

release_status省略completed；支持draft/completed。现有语法允许但本功能缺少rollout fraction等充分字段的inProgress/halted，整批明确unsupported，不猜分阶段策略。Google commit固定changes_not_sent_for_review=false写原意图，rescue_changes_not_sent_for_review=false；远端要求另一选择不会自动改参数再commit。app_identifier/track/file按指定字段一次渲染，插入字符串不再解析。params.version必须实际声明/解析为期望versionName；Number来自原Task而非用户可改计数器。声明tests须passed+sealed，即使required=false最终missing也拒绝上传；未声明无假passed。

选中定义有upload，无论build/step.when false，trigger/retry先admin+allow_upload；有效upload才做节点秘密/商店工具检查。local有效upload在任何用户动作前拒，dry-run只验证引用/模板/安全摘要，不获取token/商店。整批检查不启动Publisher；与019兼容按已验收实际能力校验后放开trigger/enqueue/Run三道拒绝，不复制入队路径。

## 节点工具与材料

AgentConfig新增可选publish_tools:{bundle_dir,bundletool}，均由agent.yml相对目录解析为自有受限工具位置；BundleDir是项目维护Fastfile/Gemfile/Gemfile.lock的只读部署副本，记录包摘要；Bundletool是明确已锁JAR。未配置不加载业务gems、不影响generic run/本机doctor。扩展本机 `doctor --target google-play --agent-config ... --app-id ... --credentials-env GOOGLE_PLAY_JSON` 仅显式诊断输入，不要求控制端token，也不读取宿主未知材料。

GOOGLE_PLAY_JSON是Agent已声明secret来源中的**私有文件路径**，由完整${NAME}引用选择；材料内容必须自有0600普通文件、无symlink/hardlink/FIFO，非阻塞打开+fstat、≤1MiB、copy/hash一致，JSON为明确service_account。token_uri/授权scope固定Google官方；不接受ADC、authorized_user/external_account、外部credential_source、用户HTTProot_url或proxyenv。中央只存引用名称，不存实际路径/JSON/token；Node本次日志secret集合同时包含原密钥JSON/private_key/token/私有路径值，不公开工具原文。

密钥、短期token、工具请求放私有0600输入文件；argv仅固定lane名称与自有中性请求文件路径，不把凭据/参数明文拼fastlane参数。HOME为自有空目录，去除宿主GOOGLE_*/FASTLANE_*/BUNDLE_*/RUBYOPT/HTTP*_PROXY/DEBUG等，仅明确工具执行必需运行环境和固定控制值。无用户lane/Pluginfile/仓库Fastfile加载、交互提示、dotenv自动扩散或默认gcloud。短token有效期不够本次剩余受限动作窗口时授权前失败；不可在发布写请求内刷新token。

## 实际AAB核验与command

同attempt原普通产物大小/ID/SHA已中央确认，节点打开稳定快照并复制到只属于本次command的0700目录，fd/identity/hash前后核验。调用固定bundletool validate/dump和jarsigner完整签名验证，公开upload证书指纹等binding登记值；params.version与包内versionName完全相同，versionCode=Task.Number且≤2100000000。未知签名/未签内容/篡改/结构失败固定错误；不改签名、不重新构建，post不替代此前证据。

具体Fastfile `mybuilds_play_preflight` 只明确材料OAuth与GET诊断，产物只读校验；token存本次私有文件。`mybuilds_play_upload` 是唯一受授权publisher命令，使用Supply::Client的实际官方Ruby库，固定service.client网络连接/retries0/token字符串，关闭commit rescue及外层retry，不走高层配置打印的Uploader。流程：一次insert-edit→读取该edit当前APK/AAB/track核对version conflict→一次AAB上传→核对返回versionCode/SHA256→一次目标track更新（单本次code、releaseName=intent标记）→一次commit。原version conflict拒绝不暗调计数；同app外部编辑导致冲突不自动重新建edit或再commit。

正常resumable允许一次start与一次content/finalize，不允许未知后query-resume重发内容；响应丢失终止其余链。所有网络响应有限读取、连接/read/write期限受本次ctx，协议异常/过大/未知字段结构失败按unknown。生产endpoint仅官方TLS，无redirect自动跟随或关闭验证。故障服务器只能是测试私有接线，生产无通用root_url/testhook配置；其实际endpoint计数是锁依赖验收门。

rawstdout/stderr各≤64KiB独立bounded缓冲，任何超限/解码/秘密失败闭锁，不能写控制端raw日志；公开只有固定safe summary。结构result独立自有0600文件≤64KiB，签名无私有原文；receipt必须绑定intent/AuthDigest/Ref/expectedartifact/阶段完成证据。exit0但没有可信完整result仍unknown。procgroup OnStart沿现有StartInfo真实journal/step started消费者；停止只作用本次PID/PGID。CleanupFailed停止所有后续且保journal/guard，不能以文件删除冒充进程回收。

## 结果与查询

完整BundleSHA、code、trackupdate、commit被证实才uploaded；中间receipt不清guard，仅upload step_finished短事务核对完整本step动作无unknown且实际停止后释放。不等待公开可见。unknown即保持应用保护，Node失联/未报告启动/StopConfirmation不清保护。failed只接受可信明确未发发布写请求或远端明确拒绝且此前无已接受/未知commit的完整结构链，普通网络/退出码不充分。

GET-only `applications/{package}/tracks/{track}/releases`：最多20返回且obsolete不出现、无AABhash。只记录safe releaseName、code、有限lifecycle、观察时间；多个/无匹配/权限或网络错误保持unknown。原unknown没有可信AAB对应完成证据，不凭标记/code推成uploaded；安全远端结果供admin精确confirm。原可信uploaded可精确匹配GET进一步证实submitted/PUBLISHED；published在internal只表示internal用户可用，不声称production。

无insert-edit/delete-edit/queryargv、更没有upload/update/commit核对动作，不让query成为绕过保护的副作用入口。核对请求整体30s、每node同时1，由当前原Node管理消费者运行同process而非新Run，不占构建槽、不延原lease/ordinarybudget、不借旧Ref写权。Node不在线query失败保持原记录，admin可用外部控制台证据确认。
