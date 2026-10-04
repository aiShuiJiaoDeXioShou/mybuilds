# 002 实施与验证记录

2026-10-04，主分支002-local-run，依赖验收提交7ffa264；Darwin arm64/Go1.25.4。implement、集成验证与converge通过；完整功能随后一次本地提交。

## 交接
- 主代理：run_types.go/specs/README/历史；A：process002 worktree进程；B：engine002执行与preview helper；C：logcli002日志和CLI。三分区基于7ffa264，规范/契约/types同版，未使用stub。
- specify/plan/tasks/analyze通过：14 FR、5 SC、三用户故事、五原则、九任务，无阻塞；T001/T002完成。
- A/B/C限定文件均已集成，零新增依赖。全量 `go test ./...`、`go vet ./...`、`go test -race ./internal/pipeline ./internal/cli/...` 通过。
- 真实shell顺序、独立环境、显式参数与密钥不递归、白名单、条件/Git事实、全批预检查、--step、输入不变、目录启动前复检均通过；保持001回归。
- 失败只停当前build、批次取消、累计预算与step超时、独立post及原失败保留通过。集成修复预算耗尽显式拒绝、真实Started与post入口取消，避免从未启动的执行跑收尾。
- 进程组前台/后台/忽略TERM、继承管道/重定向、正常清理、timeout、写失败取消与无关进程保护：macOS真实测试通过，race三轮及取消十轮通过；系统调用EINTR重试后未复现首轮一次停止确认失败。
- 日志分片/重叠秘密、并发两流、200KiB长行、UTF-8中文、安全Write/Close与JSON写错误通过。
- 主代理真实二进制 `/tmp/mybuilds-mvp.zKtK0e/verify002.py`：字面参数不执行、受限环境、UTC脱敏、post、用户文件SHA-256保留、timeout与SIGINT非零JSON、本次后台PID消失、无关sleep存活，通过。仓库内对应回归保存在run_test/process_unix_test/log_test/local_run_test。
- Windows/Linux amd64客户端交叉编译通过；Linux本次未实际执行，不将编译计作真实节点验收，007补验。尚未交付artifact/approval/reports/notifications以及本地upload明确拒绝，不读取通知秘密；有效通知可enabled:false关闭。
- converge核对14FR/5SC/三用户故事/五原则/全部九任务：missing/partial/contradicts/unrequested均0，收敛阶段tasks保持原字节，无追加空phase。README和历史已更新，完整功能一次本地提交，不push。
- 最新post取消修复后全量test/vet与受影响post/budget/cancel、CLI race再通过，二进制smoke再次通过；local-run.yml在真实临时工作区执行通过。

只读Git允许预检查，dry-run不调用Git或读取秘密；不重置工作树。进程组方案不承诺清理主动setsid离组的可信脚本；孙进程由其父/系统回收。输出writer必须能返回，不能解除永久阻塞的外部writer。本功能不交付移动doctor/模板、产物/报告、交互审批或控制端/Agent。

## 全局环境预检（不作为平台功能验收）
Darwin arm64，Go1.25.4；Xcode27.0/27A266a；Flutter3.38.6/Dart3.10.7；JDK21.0.2；Android SDK存在，platforms到android-36、build-tools到36.0.0；系统返回3个有效代码签名身份。尚未核对真实工程、profile、商店应用、授权凭据或Linux测试节点。只记录存在与版本，不把它们当作签名/分发验证已通过。

## 后续短超时清理修复

003集成时重复原预算检查发现首次停止未确认并未由EINTR处理穷尽：Darwin短超时退出边界存在瞬时EPERM随后ESRCH。独立缺陷[评估](../../.specify/bugs/short-timeout-cleanup/assessment.md)→[修复](../../.specify/bugs/short-timeout-cleanup/fix.md)→[验证](../../.specify/bugs/short-timeout-cleanup/test.md)已执行；只在原有限窗口内复查EPERM，不把它当作成功，仍以ESRCH确认且保持500ms grace。
主代理003集成树复验新增100轮30ms/活组保护与原两预算count3通过（14.755s），全量test/vet通过（pipeline10.385s），真实002二进制SIGINT/timeout/子进程消失/无关进程保护smoke再通过。缺陷源码/测试与三报告单独本地提交，不夹带未提交003。
