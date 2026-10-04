# 004 Go接口契约（主代理冻结）

```go
type DoctorCheck struct { Name, Status, Version, Reason string }
type AndroidDoctorOptions struct {
    Workspace, GradleWrapper string
    Keystore, KeyAlias, StorePasswordEnv, KeyPasswordEnv string
}
func AndroidDoctor(context.Context, AndroidDoctorOptions) []DoctorCheck
func AndroidTemplate() []byte
// 同包公共私有helper，由主代理提供。
type toolCommand struct {
    Workspace, Executable string
    Args, ExtraEnvNames []string
}
func toolOutput(context.Context, toolCommand) (string, error)
```

DoctorCheck与helper属于主代理，AndroidDoctorOptions与具体函数属于Android分区。顺序java/sdk/gradle/signing，Status passed/failed/skipped，空签名skipped不使整个doctor失败。Reason固定安全code，版本只严格解析数字格式。任何失败项使CLI非零。

Workspace默认cwd；GradleWrapper默认gradlew，必须工程内相对路径。doctor整体30s。helper单工具15s且继承更早ctx，合并stdout/stderr32KiB，超限主动取消。受限九项宿主env加明确ExtraEnvNames，校验变量名/缺失，密码值不进argv/公开日志；keytool使用:env。

mobile不得import pipeline，根提取现有实际进程组实现到internal/process供双方直接复用，旧pipeline进程实现/同文件测试删除，测试迁移process；run_types的Command仅类型别名。捕获的工具输出只内部严格解析，不持久化原文。

任一工具停止未确认时，AndroidDoctor保留cleanup_error并将后续检查置failed/cleanup_error，不再调用外部工具；SDK双环境变量按真实同一目录比较，根内路径别名不误报冲突。

AndroidTemplate只返回普通YAML副本，builds.android、run/artifact，无工具/网络/密码读取。init沿用现有严格Parse与排他写。
