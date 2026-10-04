# 008 HTTP与CLI

沿007HTTPS/ca_file、Bearer、严格JSON与1MiB普通请求/响应限额、固定安全错误封装。客户端/server同批升级安全BuildView；无新配置或API版本协商框架。

## 显式retry

`POST /api/builds/{uuid}/retry`

- 用户Bearer，当前admin/trigger可用；approver、node、无token拒绝，节点路由鉴权不能被此入口借用。
- 必须 `Idempotency-Key`，沿既有合法key边界。严格JSON对象仅 `{"allow_upload": false}`；缺省false，未知字段/参数替换/branch/ref/build选择全部拒绝，不提供输入覆写能力。
- 首次201，原样幂等重放200，返回既有安全BatchView且只含一个新build；relation在BuildView.retry_of。重放不能通过自动换key制造新号。
- invalid_request400；unauthorized/node身份错误401；forbidden403；not_found404；conflict/stop_unconfirmed409；control_lock_lost503；数据库/其它内部失败沿既有固定internal_error500映射。当前上传权限按定义先检查，unsupported沿原安全边界，错误不含原脚本/最终参数/秘密/路径。

客户端：`mybuilds --config client.yml build retry <id> --idempotency-key <key> [--allow-upload] [--json]`。idempotency-key必填、不自动生成/替换或后台重发；失败stderr保留安全key用于原样重试。成功table/JSON显示新ID/号/状态/原ID；JSON沿trigger额外返回request_key；`build show`/`build ls`现有输出增加retry_of，旧/新任务独立可查，无配置正文和私有journal路径。本地run/help/version/doctor不连接此接口、不读取远端恢复状态。

## 只读终态回执

`POST /api/agent/terminal-receipt`

严格JSON沿 [protocol.md](protocol.md) 的TerminalReceiptRequest/TerminalReceipt；当前独立node Bearer，admin/trigger/approver不能使用此node入口。200只给本节点精确已确认终态且StopKnown；错误分类同Go契约，禁止返回Task/完整ExecutionProgress、参数、日志正文、StorageID、PID/PGID或路径。

此接口可在新Agent尚未注册session时调用，旧Ref仅证据不鉴权。尚未确认claim、活动记录、seq/digest不符、跨node、未知停止或未知receipt均拒绝。控制端独占锁失效一律拒绝；Agent不能把查询成功当续租/旧写权限。没有用户CLI“恢复旧任务”或“按旧PID清理”命令，既有admin/node停止确认保持独立。

## 启动

沿现有server migrate/serve入口，不增加自动维修命令。Store.Open/Migrate既有锁之后、HTTP监听前Recover；任何失败不进入accept/Claim。Agent serve先持本地目录锁，有限只读核对可证明的旧终态journal，再实际Doctor与新session注册。不能绕旧活动/unknown journal；正常本地doctor仍只读不取token/连接，不删除文件。
