# 015 Webhook快速验收指南

当前代码与必要自动验证已完成，结果见[validation](validation.md)，集中案例见[Flutter指南](../../examples/mvp/acceptance.md)。四个外部来源的真实push、完整双OS故障矩阵由用户按本文人工验收；不会用自制payload冒充托管平台投递。

## 准备与隔离

使用自己的临时目录/端口/0700data、独立SQLite与PostgreSQL测试DB、两真实macOS/Linux节点、显式SSH key/known_hosts及正常验证证书的HTTPS入口；不读宿主未知材料、不停既有服务、不drop其它库。按007快速指南登记node与client config，保留独立token/session；控制端从可信登记仓库读Git，节点能力来自doctor，不能假工具。

四来源分别准备自己授权的GitHub、GitLab、Gitee测试仓库与一个自有generic Git服务的post-receive脚本；实际GitHub/GitLab/Gitee设置JSON/push/HTTPS并启用官方TLS校验。没有公网/hosted管理权时先完成本地负例，四来源仍标待真实验证。Gitee必须password模式；GitLab本次为legacy token，现代signing不假称支持。

在测试repo准备两个命名build（例如android/ios或两个真实通用build），均实际短run+artifact；为平台目录、公共目录、ci与mybuilds.yml显式changes。默认不upload，发布权限门单独在010/011/014/019实际接受夹具上跑，不从脚本绕上传。

## 管理入口

```bash
umask 077
./bin/mybuilds --config ./client.yml project init hook-demo   --repo "$OWN_REPO" --provider github --branches main   --nodes "$OWN_NODES" --default-node "$OWN_DEFAULT_NODE"   --settings ./project-settings.yml --hook --hook-repository-key "$OWN_REPOSITORY_ID"   --json > ./hook-created.private.json
./bin/mybuilds --config ./client.yml project ls --json
./bin/mybuilds --config ./client.yml project hook events hook-demo --json
./bin/mybuilds --config ./client.yml project hook windows hook-demo --json
```

settings只含pipeline/triggers，不再含hook块，与--hook冲突明确拒绝。triggers.builds显式选名，quiet_period=30s、allow_upload=false。初始化输出一次自产secret，文件0600由用户私下填到对应provider的secret/password设置，不能放URL或argv；ProjectView/list只安全字段。明确外部secret引用则另配受限webhook_secrets_file，不混Git两键文件、不返回引用值。

本机mybuilds-server project相同管理动作只有未Serve持锁时合法；远程接口实际Server.ConfigureWebhook消费，先验证/准备全部显式块再同事务写。普通旧pipeline-only管理/local命令不加载hook材料。

## 四来源真实接收门

对每来源换独立项目/provider/repository_key，实际登记Webhook后在自有clone改一个文件并push：

```bash
git -C "$OWN_CLONE" push origin HEAD:refs/heads/main
./bin/mybuilds --config ./client.yml project hook events "$OWN_PROJECT" --json
./bin/mybuilds --config ./client.yml project hook windows "$OWN_PROJECT" --json
./bin/mybuilds --config ./client.yml build ls --project "$OWN_PROJECT" --json
```

必须保存provider实际delivery记录/脱敏头类型、对应真实commitSHA与接收UTC、中央event/window/resultID、Agent固定SHA和日志/产物SHA。generic门由真正post-receive脚本发送原JSON/HMAC，不能以手写curl“自建push”替代。Gitea/Gogs可用显式generic描述另验证，不替代Gitee，不列成第5/6必验分支。

各来源另在其自有设置中改错secret再实际push，记录401/noevent/no号；GitHub/generic原body修改签名拒，token模式只证明明确token验证/TLS部署，不能谎称密码签名body完整性。tag/PR/分支删除不构建；只有可靠ping可识别测试，等同push的provider UI测试可能产生测试构建。

## 去重、固定窗口与恢复

1. 通过provider实际重投原delivery，并用有依据的捕获原安全fixture做20并发重复HTTP回归（此回归补事务，不替代四真实push）；同ID同内容回原归属，同ID异内容409，编号只一次。
2. 30秒窗口里分别在0/10/29秒实际push，核对初次deadline未延长；同时不同branch、输入params/所选build配置策略各自窗口隔离。截止恰好时的新事件属于下一代。
3. 故意先投新push后重投旧push；关闭取得当时授权branch HEAD，检查统一固定SHA与后续再push不改已排队结果。force push只能自有repo授权执行，最终SHA保持真实可达；目标失败不退回旧payload或新HEAD重解释。
4. 仅停自己的Server，在接收/关闭commit前后4点分别重启；确认已202的事件不丢、closed不再分号、queued/有效lease/approval/unknown均不改。另起第二Server拒绝；控制锁丢失不继续关闭。
5. CAS准备间实际产生新的成功baseline或改project策略，旧关闭不得提交；重新准备或安全failed，不出现部分新结果/号。没有fresh（全部reuse）也不造空batch。

## changes、原条件与权限

- 自有repo按新增/修改/删除/改名/公共目录/无变化逐项真实commit+push；核对改名新旧均匹配、case-sensitive glob和字段AND/列表OR、build跳过无编号/节点，步骤跳过依原Run顺序。
- 首次、原baseline不可得（只操作自己的repo/fixture）走full并固定reason；repofetch/配置/diff超限/非法路径错误不伪full或empty。
- 改参数、定义/来源或静态选择形成独立ComparisonKey；baseline时间按已确认终态receipt，不因停止确认/UpdatedAt变化选错。
- 手动trigger/local绕changes，不绕branch/params；008retry在advance HEAD与当前设置改变后仍原SHA/条件事实、产生新号，不等quiet window。
- 含upload时allow_upload=false即使when=false拒绝；开启也仍由实际审批/报告/appguard/node授权决定，不能approval跳过变默认批准或重发unknown。窗口禁用/rotate/policy变化不使用旧权，旧running不会被hook自动取消。

## 两库与平台门

同一套事务/管理/事件/窗口/20竞争/重启/比较CAS用例分别运行SQLite与独立PostgreSQL，旧008满记录迁移/FK保持。macOS/Linux真实Server/Agent/Client入口执行相同固定提交；真实mobile能力门按既有工程，不以通用任务宣称Linux/iOS能力。

```bash
go test -p 1 ./...
go test -race -p 1 ./internal/store ./internal/scm ./internal/server ./internal/pipeline ./internal/config
go vet ./...
```

上述命令为复验入口；本轮已实际执行的完整及必要目标检查见validation，不将未执行的大矩阵标通过。若接受后的008基线包含全宿主可见的未知SIP孤儿负例，按其实际验收方式串行隔离跨包故障注入；这不放宽产品unknown判断。根再执行受影响原功能门、CLIhelp/版本/跨平台build和converge；正式validation由根记UTC/提交/OS/provider版本/路径/指纹和未满足项，不保存原秘密请求。

## 证据清单与完成定义

完整人工验收继续核四来源真实push+错secret、固定截止/隔离/乱序/4退出点、双OS与材料场景。代码与必要自动检查、Spec Kit收敛完成后已本地提交，具体已通过项见validation。只浏览资料或模拟JSON通过不勾真实来源门；材料缺失保待验收，不自动向他人索取材料/发消息。
