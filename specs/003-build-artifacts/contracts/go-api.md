# 003 冻结接口

```go
type ArtifactRecord struct {
    SourcePath string `json:"source_path"`
    SnapshotPath string `json:"snapshot_path"`
    Size int64 `json:"size"`
    SHA256 string `json:"sha256"`
}
// 主代理向RunResult增加ResultDir string `json:"result_dir,omitempty"`。
// 向StepRun增加Artifacts []ArtifactRecord `json:"artifacts,omitempty"`、LogPath string `json:"log_path,omitempty"`。
func validateArtifactPatterns(patterns []string) error
func collectArtifacts(ctx context.Context, workspace *os.Root, destination string, patterns []string) ([]ArtifactRecord,error)
```

A collector只在完整成功返回清单；destination必须尚不存在（已有任何目标拒绝，不覆盖）；在其父受控目录创建独立stage，写files/<source>与manifest.json，完成后原子rename。返回SnapshotPath相对destination，B拼接结果根相对前缀。destination由Run结果根与已验证build/step名称组合，禁止源码目录。collector不得读取环境/执行命令/回显原路径或OS错误；预算/取消ctx来自既有执行器。支持**，根内普通链接，Root实际访问；每模式独立零合法文件报错。取消检查涵盖无匹配遍历和复制。

Run创建/关闭独立结果root，仅在整批预检查后且至少一个选中步骤生效。B将artifact paths经既有renderField逐个渲染并validateArtifactPatterns，允许kind artifact，其余未支持能力仍拒绝。实际collector开始才started=true，失败/取消对应原状态和post，累计耗时不漏；普通/post每步目标独立。

artifact配置保持仅有paths，不新增自定义timeout；收集使用构建或post的剩余预算，并响应整个运行的取消。

C保留newRunLogger/stream，同时增加：

```go
// logger.root *os.Root由B在日志前设置，nil保留原只写Output行为。
func (*runLogger) close() error
```

logger.root归Run管理，C只关闭自己的文件。每条已脱敏记录在共享锁下写终端与对应logs/<build>/<step>.log（两个流同文件、来源前缀保留），Root.MkdirAll/Root.OpenFile权限0700/0600，文件仅写本次日志，不能经链接越界。关闭所有文件，固定安全错误；logger已有sticky输出错误沿用。B必须在Run返回前处理close：原成功可改failed/log_error，原失败/取消不覆盖；返回固定批次error。StepRun.LogPath只指向已创建的实际记录文件，不伪造不存在的路径；提供 `(*runLogger) logPath(build,step string) string` 查询已创建文件的结果根相对路径。
artifact输出只写安全system摘要/固定失败原因，保证其日志可定位；文件内容不进入日志。
