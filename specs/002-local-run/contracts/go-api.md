# 002 冻结 Go 接口

主代理在 implement 阶段建立 run_types.go：

```go
type RunOptions struct { PreviewOptions; Workspace string; Output io.Writer }
type RunResult struct { Builds []BuildRun `json:"builds"` }
type BuildRun struct {
    Name string `json:"name"`
    Status string `json:"status"`
    Reason string `json:"reason,omitempty"`
    DurationMS int64 `json:"duration_ms"`
    Steps []StepRun `json:"steps"`
    Post []StepRun `json:"post,omitempty"`
}
type StepRun struct {
    Name string `json:"name"`
    Kind string `json:"kind"`
    Status string `json:"status"`
    Reason string `json:"reason,omitempty"`
    ExitCode int `json:"exit_code"`
    DurationMS int64 `json:"duration_ms"`
    CleanupFailed bool `json:"cleanup_failed,omitempty"`
}
func Run(context.Context, *config.Document, RunOptions) (*RunResult, error)
type shellCommand struct { Path string; Args []string; Dir string; Env []string }
type shellResult struct { Started bool; ExitCode int; Reason string; Duration time.Duration; CleanupFailed bool }
func runShell(context.Context, shellCommand, io.Writer, io.Writer) shellResult
```

预检查失败返回 nil/安全 error；已开始执行返回完整结果，任何失败/取消同时返回固定批次 error。其他 build 的独立失败继续，取消批次剩余未启动 build 没有 post。
runShell 成功 Reason 为空，失败用固定 exit/timeout/cancelled/start_error/log_error/cleanup_error；不回显底层 error。正常和取消均清理本组，写错误主动取消命令。CleanupFailed 独立保存停止未确认，不覆盖原 Reason；此时 B 禁止 post/后续 build。Path 为预检查过的绝对 shell 路径，Args 为固定 sh/bash flags 加 -c 与原正文，Env nil 必须转换为空切片，禁止自动继承。

C 提供 B 使用的日志：

```go
func newRunLogger(output io.Writer, secrets []string) *runLogger
func (*runLogger) stream(build, step, stream string) io.WriteCloser
```

并发安全，Close 刷新末尾；秘密仅为显式引用取得的非空实际值（去重、重叠优先最长）。各流独立缓冲，共享输出锁。Write/Close 返回固定安全错误；A runShell 处理 Write 错误主动取消，B 处理 Close 错误。
B 提取 `renderField(value, field string, params, context map[string]string, secrets, notification bool) (string, bool, error)`，checkField 保留摘要调用。Run 对 env 再解析显式 ${NAME}，预览不解析密钥，run 正文不调用渲染器。

启动标记由 runShell 在 Start 成功后设置 Started；StepRun 内部 started 不序列化，只用于判断真正开始过的 build 才能运行 post，不能从退出码或 reason 猜测。预算耗尽必须显式禁止启动，非正剩余预算不能退化为无限超时。
