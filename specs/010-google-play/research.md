# 010 研究与决策

研究日期2026-10-05。只使用Google/Android、fastlane、RubyGems和依赖维护者官方资料。执行了源码只读下载/解包及本机ruby/bundle版本检查；没有安装gems/JAR、读取商店材料、获取真实token或请求发布API。

## R1 工具候选与实际版本边界

**决定**：按产品选择受控 `bundle exec fastlane` 入口，不用用户lane或Go商店SDK。官方fastlane2.240.1存在，要求Ruby≥3.1；它是原型候选，不能把上游开发锁直接作为本项目已验证锁。[发布页](https://rubygems.org/gems/fastlane/versions/2.240.1)、[tagged gemspec](https://github.com/fastlane/fastlane/blob/2.240.1/fastlane.gemspec)。

**依据**：上游该tag Gemfile.lock实际为androidpublisher_v3 0.99.0、core0.18.0、googleauth1.16.2、httpclient2.9.0，不能把main的Faraday实现误认作这个锁。[上游锁](https://github.com/fastlane/fastlane/blob/2.240.1/Gemfile.lock)。本机实测Ruby2.6.10/Bundler1.17.2，不满足候选。未来原型显式选择已发布core1.2.5及固定Faraday/net_http依赖，兼容后生成项目锁，记录完整transitive版本/摘要；未通过时doctor拒绝，不自动安装/更新。

**替代**：直接运行完整Supply::Uploader会输出配置、默认大量元数据并弱化原回执；另写GoPublisher绕开第三方选择也不采用。具体Fastfile复用Supply::Client的官方库并保留原始Bundle哈希和commit结果，只有这一个实际consumer，不抽象FastlaneClient。

## R2 内部非幂等重试是强制接入门

**决定**：不能仅设SUPPLY_UPLOAD_MAX_RETRIES=0。tagged Supply::Client.initialize将Google default retries设5；commit_current_edit! 默认允许按错误内容改changes_not_sent_for_review后再次commit。包装入口强制外层0、实例request_options.retries=0、关闭commit rescue；不根据错误扩张审核授权。[具体tagged client](https://github.com/fastlane/fastlane/blob/2.240.1/supply/lib/supply/client.rb)。

core0.18.0 HttpCommand.do_retry除retries+1外还有refreshable authorization的独立2次auth retry；其upload调用链为生成service.upload_edit_bundle→BaseService.make_upload_command→ResumableUploadCommand.execute_once→start/query/content→HttpCommand.do_retry。0重试仍不等于一HTTP请求，因为正常resumable本身分启动与传输；要求每个具体写endpoint一次，禁止未知后restart/resume。[core0.18.0 HTTP](https://github.com/googleapis/google-api-ruby-client/blob/google-apis-core/v0.18.0/google-apis-core/lib/google/apis/core/http_command.rb)、[upload](https://github.com/googleapis/google-api-ruby-client/blob/google-apis-core/v0.18.0/google-apis-core/lib/google/apis/core/upload.rb)、[service](https://rubygems.org/gems/google-apis-androidpublisher_v3/versions/0.99.0)。

httpclient2.9.0有KeepAliveDisconnected再次yield路径，故不把旧开发锁当已证明单发。[维护者源码](https://github.com/nahi/httpclient/blob/v2.9.0/lib/httpclient.rb)。候选core1.2.5提供service.client=具体Faraday连接；禁止retry和follow_redirect中间件，TLS验证保持，采用已存在net_http3.4.4的max_retries=0设置；token在授予发布权前有界获取，并给service.authorization赋普通token字符串，不能在写请求中刷新。[core1.2.5](https://github.com/googleapis/google-api-ruby-client/blob/google-apis-core/v1.2.5/google-apis-core/lib/google/apis/core/base_service.rb)、[adapter3.4.4](https://github.com/lostisland/faraday-net_http/blob/v3.4.4/lib/faraday/adapter/net_http.rb)。token存在本节点私有0600会话材料，不进argv/回执/中央。

**验收门**：实际锁版本后用自有loopback故障服务器，分别让insert-edit、resumable start/content、track update和commit收到请求后断开，以及401/429/500、redirect、迟响应。逐endpoint实际请求计数必须1且不在原失败后继续后阶段；不得用mock计数当真实library证据。响应丢失记unknown；这次尚未执行该库原型，不声称通过。

## R3 Google真实证据与查询充分性

Bundle返回versionCode及上传字节sha256；Supply高层upload_bundle只返回versionCode，具体入口保留底层完整Bundle结果。[Bundle定义](https://developers.google.com/android-publisher/api-ref/rest/v3/edits.bundles)。轨道Release的name是发布名称，不能代替包内versionName。[tracks](https://developers.google.com/android-publisher/api-ref/rest/v3/edits.tracks)。

当前直接GET applications.tracks.releases.list能查询生命周期，但排除obsolete且最多20条、ArtifactSummary只有versionCode；不含AAB摘要，空结果绝不证明没副作用。[list](https://developers.google.com/android-publisher/api-ref/rest/v3/applications.tracks.releases/list)、[ReleaseSummary](https://developers.google.com/android-publisher/api-ref/rest/v3/applications.tracks.releases)。生成releaseName为本系统intent UUID标记；积极匹配track/name/code仍需原BundleSHA补足才能自动解除unknown。published只由该轨道PUBLISHED的正证据映射，不等于production公开；IN_REVIEW→submitted、其它保守映射/保留raw有限enum，不臆测processing。

**决定**：显式query仅GET该轨道发布summary，不创建edit、不上传/update/commit。Google新edit会使原edit失效；即使停止本地进程也不能证明远端inflight已完成，所以不为补摘要扩大核对副作用。[Edits](https://developers.google.com/android-publisher/edits)。现有未知动作若没有可信原Bundle摘要/完成回执，仅版本/标记匹配不足解除unknown；保存有限证据供admin有依据确认。已明确uploaded且完整原摘要已可信绑定的记录，可以根据精确标记/code/track的GET进一步记录submitted/published。空/obsolete/多候选保持原记录，不failed。成功commit只证明接受变更，不当published。可信远端明确拒绝必须具体阶段、结构状态和此前写链证明没有发布副作用，泛用HTTP错误不自动failed。

## R4 AAB与版本签名

**决定**：匹配中央同attempt原artifactID而非重新glob可变工作树；使用只读稳定副本、bundletool validate和dump manifest核对package/versionCode/versionName，版本码等原Task.Number（1..2100000000）；versionName等本build冻结params.version。无version参数或不匹配在授权前拒绝。[Android版本](https://developer.android.com/studio/publish/versioning)、[官方bundletool1.18.3](https://github.com/google/bundletool/releases/tag/1.18.3)、[dump源码](https://github.com/google/bundletool/blob/1.18.3/src/main/java/com/android/tools/build/bundletool/commands/DumpCommand.java)。

AAB是JAR签名，不能用apksigner当AAB验证器。[Android工具说明](https://developer.android.com/tools/bundletool)。以jarsigner校验完整内容签名，核对管理员登记upload证书SHA256；Android合法自签upload证书不能因不在公共CA被误拒，使用本功能自有公开证书truststore、不读用户默认keystore。unsigned entry/算法禁用/过期/内容改变仍拒。仅证书presence或工具exit0不足；真实已签AAB正例与篡改/未签entry负例必须证明。[Oracle jarsigner](https://docs.oracle.com/en/java/javase/25/docs/specs/man/jarsigner.html)。不改签名、不读取签名私钥，bundletool候选尚未下载或验收。

## R5 认证、首次接入与授权

只接受指定0600普通service_account JSON，拒ADC/authorized_user/external_account/宿主gcloud；JSON token_uri必须官方OAuth固定HTTPS，无外部credential_source/endpoint配置。材料读入私有内存/副本后不再次解释模板，工具环境只明确Ruby/Java/工具运行必需和本次值。[Google接入](https://developers.google.com/android-publisher/getting_started)、[supply首次设置](https://docs.fastlane.tools/actions/upload_to_play_store/)。doctor的GET只证明读权限，不宣称已经具有全部写权限；创建edit/首上传API的实际权限拒绝须在原受控动作中明确报告，不靠试传或权限字符串冒充验证。应用记录、service account权限、初次手动上传、Play App Signing与协议/隐私前提由用户准备；doctor只验证现有，不代接受。

track省略internal，显式测试track须存在且不得别名回落production；production必须admin原显式授权。commit涉及审核工作流：固定changes_not_sent_for_review选择写入原intent，禁止rescue改选择；Google要求其它选项则失败/unknown依据原动作证据，不能自行放宽。

## R6 已有边界与最小共享

008只读终态回执证明停止，不证明商店结果；retry新号但不能绕同app unknown，旧意图不重放。019原XML/seal与AAB是两个实际中央证据，未声明测试不造passed，已声明optional missing也不满足010上传门；publish前完整通过seal才许可。现有server/trigger.go与store/enqueue.go的能力拒绝同样须按真实签名放开，不能只改Run。011前后副作用分意图；Google多阶段一个受控上传流程不自动再次commit。020只在实际guard/关联字段完成后联验，不为未来审批造假状态。

## 实际研究证据与限制

只读RubyGems解包两项（未安装）：core0.18.0 gem37376B SHA256 `96b057816feeeab448139ed5b5c78eab7fc2a9d8958f0fbc8217dedffad054ee`；androidpublisher_v3 0.99.0 gem112640B SHA256 `a0452fdd99cb7672cc95cac07429305f8aaee54c22e4875224cf675b1ab59729`。证据目录 `/tmp/mybuilds-mvp.zKtK0e/play010-official-source/manifest.json`。当前main commit f65f0d5768d2f5fafd9e2ab55d715659c8b1c1a8仅用于比较，决策引用版本tag而不依赖浮动main。

仍待实施验证：完整Gemfile.lock+故障请求计数，Java签名边界、原生/Flutter真实AAB与首次应用条件、真实internal可见、真实上传后断连unknown、production授权负例、双库应用保护/确认竞争。未访问个人材料、商店账号或真实发布；没有把工具源码、模型测试或模拟Google响应冒充SC001/004。
