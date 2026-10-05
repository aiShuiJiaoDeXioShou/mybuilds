# 010 Google Play 代码交付与人工验收指南

已接入受控第三方工具、发布授权、原产物与报告绑定、应用保护、远程查询/确认和CLI。按照用户2026-10-05指示，必要自动检查完成后先交付代码；真实Google Play上传由用户统一人工验收，尚未执行真实商店发布。验证范围见[验证记录](validation.md)。

## 准备

用户提供已准备Google Play测试应用、首次手动上传/协议/App Signing、service account授权、公开upload证书摘要、原生和Flutter可信工程。缺任一材料记录待真实验证；不访问宿主未知账户，不测试production公开发布。Agent部署实际锁定Ruby/Bundler/fastlane工具包和bundletool，配置publish_tools的两明确路径；node secrets.env中的GOOGLE_PLAY_JSON指向自有0600普通JSON文件，不将JSON/token写argv。

client.yml为0600，token取环境引用，server HTTPS/CA验证沿007；application publisher节点注册私有token，示例使用已有命名node，实际测试另建0700夹具目录与专用数据库/代理，不干扰现有节点。

## 先验证工具单发门

先在无凭据自有loopback故障服务器跑实际锁依赖包装入口，所有写endpoint收到请求后断连、401/429/500/redirect均计数≤1；断连不得继续track/commit或resumable重发。锁完整版本/摘要、Ruby/Java版本、UTC命令、服务器实际计数、process回收证据保存；未通过禁止真实商店调用。这一步不代替真实商店验收。

## 绑定与明确internal构建

以下命令用于人工验收，token预先由受限client.yml/环境读取，命令不含secret值：

```bash
mybuilds doctor --target google-play --agent-config ./private/agent.yml --app-id com.example.prepared --credentials-env GOOGLE_PLAY_JSON
mybuilds project app bind demo --store google_play --app-id com.example.prepared --node mac-android-a --credentials-env GOOGLE_PLAY_JSON --upload-cert-sha256 "$UPLOAD_CERT_SHA256" --track internal
mybuilds project app ls demo --json
mybuilds trigger demo --build android --param version=1.0.0 --allow-upload --idempotency-key play-native-001 --json
mybuilds build show "$BUILD_ID" --json
mybuilds publish ls --project demo --json
mybuilds publish show "$INTENT_ID" --json
```

`trigger <project>`与`--idempotency-key`沿真实006/007入口，HTTP request_key及命名build/param/admin明确许可不变；不新增另一trigger命令。目标build引用contracts/google-play.md的普通run/artifact/upload顺序，reports final封存必须先于upload。binding等待指定节点真正doctor verified，再启动真实发布。原生与Flutter各一次，核对原SHA/build名/Number/AABbytesSHA/versionName、JUnit XML中央seal、原Intent、单命令及Google internal实际可见；production默认不执行。

## 核对未知与证据确认

```bash
mybuilds publish query "$INTENT_ID" --json
mybuilds publish query-show "$QUERY_ID" --json
mybuilds publish show "$INTENT_ID" --json
mybuilds publish confirm "$INTENT_ID" --decision-file ./private/confirmed-evidence.json --json
```

confirm文件按contracts/go-api.md完整精确intent、decisionKey、expectedDigest、真实外部证据填写，自有0600；不能把停止/非零/空query写成未发送依据。实际发布丢回执后unknown与appguard跨server/Agent重启仍存在，20个同app申请不能第二次upload；GET-only空/partial仍unknown，管理员核对控制台原版本/原产物证据后明确决定。相同决定重发原结果，冲突决定409。查询不分配构建号、不占节点槽、不发送商店写请求。

## 必要负例和验收

| 门 | 实际行为 |
|---|---|
| G1 | APK、多AAB、leaf symlink/FIFO/替换、signed AAB未签entry/篡改、错package/code/name/cert、旧post产物；声明JUnit missing/fail/changed全部授权前拒绝 |
| G2 | SQLite/PG同20并发、失锁、意图SQL失败、旧完整Ref/expiry、whenfalse权限、trigger/approver禁止、production未显式、跨项目绑定、版本冲突不改号 |
| G3 | 真Google接受后丢回执、grant丢失/启动前失联、重启无重发、unknown保护、GET空/obsolete/多匹配/不足、精确admin证据/重复/冲突/stop不足 |
| G4 | 真process ordinary/failure/always预算与取消、本次PGID回收且无关own sleep存活、下一排队任务执行、query满capacity也不占槽、日志/表格/JSON秘密扫描 |
| G5 | 人工验收完成原生/Flutter internal可见、原固定SHA与完整中央原XML/AAB、localupload先拒与dry-run无商店/secret、007/008/009/019回归；014/020各自接入后实际联合门计入整MVP验收，不形成反向前置 |

完成后按必要变化执行Go test ./...、go test -race ./...、go vet ./...及三入口跨平台构建、实际二进制双库/HTTPS测试；记录版本、UTC、脱敏命令与receipt/hash，不能把fake Google结果当SC001/004。Android签名已有产物不等于已授权应用发布；缺真实应用/账号仍待验证，整MVP保持未完成。代码实现按已有Spec Kit文档推进，自动检查后本地提交；以上真实商店门保持人工待验。
