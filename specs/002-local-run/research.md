# 002 研究

选择：仅复用标准库和 001 现有 helper。CommandContext 默认只杀主进程，WaitDelay 也不是组取消；使用独立 PGID、自定义 Cancel 同步 TERM→500ms→KILL，sync.Once 使正常清理/取消共用，WaitDelay 处理后台继承输出管道。Start 后必须 Wait；组停止未确认与原退出原因分开保存并停止后续动作。nil Env 需转为空切片，不继承完整宿主环境。
依据：[Go 1.25.4 os/exec](https://pkg.go.dev/os/exec@go1.25.4)、[官方源码](https://github.com/golang/go/blob/go1.25.4/src/os/exec/exec.go)、[POSIX kill](https://pubs.opengroup.org/onlinepubs/009604499/functions/kill.html)、[POSIX wait](https://pubs.opengroup.org/onlinepubs/9699919799/functions/wait.html)。只回收直接 shell，同组后代由其父/系统回收；可信脚本主动 setsid 离组不在保证范围。

选择：共享模板扫描器返回内部渲染值，不引入模板引擎。预览只保留三态摘要；执行不能直接拿脱敏 PreviewPlan 启动。全批预检查时解析显式环境引用、获取必要只读 Git 事实，启动前复检目录。
选择：逐流增量脱敏、共享输出锁和 UTC 前缀，不用 Scanner 的64KiB行限制；保留最长秘密的未决后缀，长行分块，Write错误取消命令。输出 writer 必须能完成 Write，有限进程等待无法解除永久阻塞的外部 writer。
替代：仅主PID kill不满足；引入进程库/日志框架/模板引擎无必要；全部输出落内存会无界增长。无需额外用户澄清。
