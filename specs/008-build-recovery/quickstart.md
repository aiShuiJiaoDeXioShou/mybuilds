# 008 真实验证指南

007已验收；以下是008验收门，实际运行结论与证据见[validation.md](validation.md)，指南本身不代表通过。仅使用本目标自有仓库/密钥/CA/端口/进程和私有目录，保留旧unknown证据；不修改用户系统信任，不杀旧PID或无关进程。

## 前置与命令

实现由tasks/analyze后授权。沿007配置：一控制端、两个不同token/session/data_dir的合格macOS Agent和一个Linux通用Agent；HTTPS证书有合法SAN，显式ca_file；SQLite与独立PG库跑同suite，先核实实际版本。通用脚本不设置虚构runner，Android能力只实际Doctor通过才声明。

```sh
go test -p 1 ./...
go test -p 1 -race ./...
go vet ./...
go build -o "$FIXTURE/mybuilds" ./cmd/mybuilds
go build -o "$FIXTURE/server" ./cmd/mybuilds-server
go build -o "$FIXTURE/agent" ./cmd/mybuilds-agent
"$FIXTURE/server" --config "$FIXTURE/server.yml" serve
"$FIXTURE/agent" --config "$FIXTURE/agent.yml" serve
"$FIXTURE/mybuilds" --config "$FIXTURE/client.yml" build retry "$ORIGINAL_ID" --idempotency-key "$KEY" --json
"$FIXTURE/mybuilds" --config "$FIXTURE/client.yml" build show "$ORIGINAL_ID" --json
"$FIXTURE/mybuilds" --config "$FIXTURE/client.yml" build ls --project "$PROJECT" --json
```

全套测试使用 `-p 1` 隔离跨包真实未知孤儿故障注入，测试与应用验收顺序运行；这不会降低产品unknown保护。

FIXTURE/ID/KEY/PROJECT由自有夹具实际生成，token仅私有配置/内存；20并发请求复用同KEY，不在终端输出凭据。交叉构建沿007三入口darwin/linux/windows矩阵；Linux关键场景必须运行真正目标二进制，不能只编译。

## 必须实际观察

1. 原仓库固定SHA排queued与实际长普通动作；在lease内停/重启控制端，再推进分支。仍有效Ref/号/NS预算不变，原进程未重启、续租成功，queued执行旧SHA且只一次。第二控制端/真实锁丢失拒绝接入，损坏可调度快照启动失败不重置queued。
2. 普通与always各自Started后断网/退出Agent；记录PGID和自有无关sleep，在期限内仍按原lease，超时真实回收本组，无关进程仍活。中央保interrupted+guard/原reason/post/NS；新Agent不能旧PIDkill、自动post或重跑。精确真实停止确认只清guard。
3. 原构建完成后推进HEAD、修改项目参数设置；retry实际新号/新attempt输出与binary artifact证明原SHA/定义/最终参数/条件。20同key只一个新build/一个新号，响应丢失重发同结果；同key不同原ID/allow_upload与trigger冲突拒绝。旧记录/日志/产物/原因/预算摘要前后相同。
4. 活动/queued/skipped/guard、角色/跨node、缺project/损坏facts/快照/非法输入、发布权限负例不消耗号；无合格工具保持queued，原commit缺失只固定Checkout失败，不回HEAD。
5. 真正拦截build_finished已提交后的HTTP响应：中央完整终态/manifest，Agent保PendingEvent；重启同data_dir用当前node凭据只读查询，精确匹配可清该条，随后按合法session窗口注册。ordinary ACK可重发不等于terminal可重放。缺receipt/旧无Kind/StopKnown、错seq/digest/ref、active/guard/crossnode/unknownclaim、非法文件全部保持journal，新进程无用户动作或信号。核查readonly查询前后中央eventseq/预算/status完全不变。

所有场景SQLite/PostgreSQL一致；macOS/Linux重复关键重启/期限/终态ACK路径。验收记录记录UTC、版本、真实命令退出码、安全IDs/摘要与证据SHA；失败保留不覆盖。019尚未实现报告相关集成不计PASS，未来报告/审批/上传unknown联合门保留。
